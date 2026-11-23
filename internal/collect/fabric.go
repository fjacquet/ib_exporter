package collect

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/fjacquet/ib_exporter/internal/config"
	"github.com/fjacquet/ib_exporter/internal/ib"
	"github.com/fjacquet/ib_exporter/internal/ib/diags"
)

// Reader names, the values of the "reader" label.
const (
	rdTopology = "topology"
	rdPerf     = "perfquery"
	rdOptional = "perfquery_optional"
	rdPortInfo = "portinfo"
	rdSM       = "sm"
	rdIbswinfo = "ibswinfo"
)

// optionalOpts are the perfquery options whose fields diags.ParsePerfquery maps. The
// per-SL/VL tables (--slrcvfecn, --slrcvbecn, --vlxmittimecc) are not exported.
var optionalOpts = []string{"-T", "-E", "-D", "--rcvcc", "--xmitcc"}

// fabricState is owned by the goroutine collecting that fabric; cycles never overlap.
type fabricState struct {
	f        config.Fabric
	haveTopo bool
	nodes    []ib.Node
	links    []ib.Link
	ports    map[string][]int // GUID -> ports linked now or ever while the node stayed in the topology
	topoAt   time.Time
	piAt     time.Time
	// Caches keyed by GUID (never LID), reused between runs of slower readers.
	optional map[string][]ib.Sample
	portInfo map[string][]ib.Sample
	health   map[string][]ib.Sample
	sm       []ib.Sample
	stats    stats
}

func newFabricState(f config.Fabric) *fabricState {
	return &fabricState{f: f, stats: newStats()}
}

func (l *Loop) collectFabric(ctx context.Context, fs *fabricState, now time.Time, optional bool) *ib.FabricSnapshot {
	fresh := l.refreshTopology(ctx, fs, now)
	var ss []ib.Sample
	if fs.haveTopo {
		ss = l.fabricSamples(ctx, fs, now, optional)
	}
	// Up: the fabric answered this cycle, by a topology read or at least one node's perfquery.
	up := fs.haveTopo && (fresh || fs.stats[rdPerf].cycOK)
	ss = append(ss, ib.Sample{Name: "ib_up", Labels: []ib.Label{{Key: ib.LabelFabric, Value: fs.f.Name}}, Value: b2f(up)})
	readers := []string{rdTopology, rdPerf, rdOptional, rdPortInfo, rdSM}
	if l.cfg.Sources.Ibswinfo.Enabled {
		readers = append(readers, rdIbswinfo)
	}
	lastErr := ""
	for _, r := range readers {
		fs.stats[r].finish(now, fs.f.Name, r)
		ss = append(ss, fs.stats[r].samples(fs.f.Name, r)...)
		if lastErr == "" {
			lastErr = fs.stats[r].lastErr
		}
	}
	return &ib.FabricSnapshot{Fabric: fs.f.Name, Up: up, Nodes: fs.nodes, Links: fs.links, Samples: ss, At: now, LastErr: lastErr}
}

// refreshTopology re-reads the topology when due and reports whether it was read this
// cycle. A failure keeps the last good topology; it is retried next cycle.
func (l *Loop) refreshTopology(ctx context.Context, fs *fabricState, now time.Time) bool {
	if fs.haveTopo && now.Sub(fs.topoAt) < l.cfg.Collection.TopologyInterval {
		return false
	}
	out, err := l.run(ctx, fs.f, fs.stats[rdTopology], "ibnetdiscover", "-p")
	if err != nil {
		fs.stats[rdTopology].record(err)
		return false
	}
	names, err := diags.LoadNodeNameMap(fs.f.NodeNameMap)
	if err != nil {
		fs.stats[rdTopology].record(err) // unreadable map: names fall back to the tool's
	}
	nodes, links, err := diags.ParseTopology(out, names)
	fs.stats[rdTopology].record(err)
	if err != nil {
		return false
	}
	fs.nodes, fs.links, fs.haveTopo, fs.topoAt = nodes, links, true, now
	// A downed port loses its link line; remember it so portinfo keeps reporting it Down.
	// Ports are forgotten only when their node leaves the topology.
	ports := map[string][]int{}
	for _, n := range nodes {
		if ps, ok := fs.ports[n.GUID]; ok {
			ports[n.GUID] = slices.Clone(ps)
		}
	}
	for _, lk := range links {
		if !slices.Contains(ports[lk.GUID], lk.Port) {
			ports[lk.GUID] = append(ports[lk.GUID], lk.Port)
		}
	}
	fs.ports = ports
	return true
}

// stats is per-reader accounting. Node workers each fill their own; the fabric
// goroutine merges them, so fs.stats and the caches are never shared across goroutines.
type stats map[string]*readerStat

func newStats() stats {
	st := stats{}
	for _, r := range []string{rdTopology, rdPerf, rdOptional, rdPortInfo, rdSM, rdIbswinfo} {
		st[r] = &readerStat{}
	}
	return st
}

// nodeResult is what one node worker returns; merged on the fabric goroutine.
type nodeResult struct {
	counters, optional, portInfo, health []ib.Sample
	st                                   stats
}

// parallel runs f(0..n-1) with at most limit goroutines and returns results in index order.
func parallel[T any](n, limit int, f func(i int) T) []T {
	out := make([]T, n)
	var g errgroup.Group
	g.SetLimit(limit)
	for i := range n {
		g.Go(func() error {
			out[i] = f(i)
			return nil
		})
	}
	_ = g.Wait() // workers always return nil
	return out
}

// fabricSamples builds a fresh slice every cycle, in topology order: cached slices are only copied from.
func (l *Loop) fabricSamples(ctx context.Context, fs *fabricState, now time.Time, optional bool) []ib.Sample {
	ss := topologySamples(fs.f.Name, fs.nodes, fs.links)
	runPI := fs.piAt.IsZero() || now.Sub(fs.piAt) >= l.cfg.Collection.PortInfoInterval
	if runPI {
		fs.piAt, fs.sm = now, l.smSamples(ctx, fs)
	}
	var nodes []ib.Node
	for _, n := range fs.nodes {
		if n.LID != 0 {
			nodes = append(nodes, n)
		}
	}
	res := parallel(len(nodes), l.cfg.Collection.MaxConcurrent, func(i int) nodeResult {
		return l.collectNode(ctx, fs, nodes[i], optional, runPI)
	})
	if optional {
		fs.optional = map[string][]ib.Sample{}
	}
	if runPI {
		fs.portInfo, fs.health = map[string][]ib.Sample{}, map[string][]ib.Sample{}
	}
	for i, n := range nodes {
		r := res[i]
		for name, st := range r.st {
			fs.stats[name].add(st)
		}
		if optional {
			fs.optional[n.GUID] = r.optional
		}
		if runPI {
			fs.portInfo[n.GUID], fs.health[n.GUID] = r.portInfo, r.health
		}
		ss = append(append(append(append(ss, r.counters...), fs.optional[n.GUID]...), fs.portInfo[n.GUID]...), fs.health[n.GUID]...)
	}
	return append(ss, fs.sm...)
}

// collectNode runs on a worker goroutine: it reads fs.f and fs.ports only and owns res.st.
func (l *Loop) collectNode(ctx context.Context, fs *fabricState, n ib.Node, optional, runPI bool) nodeResult {
	res := nodeResult{st: newStats()}
	res.counters = l.counters(ctx, fs, res.st, n)
	if optional {
		res.optional = l.optionalCounters(ctx, fs, res.st, n)
	}
	if runPI {
		res.portInfo = l.portInfoSamples(ctx, fs, res.st, n)
		if l.cfg.Sources.Ibswinfo.Enabled && n.Type == ib.NodeSwitch {
			res.health = l.switchHealth(ctx, fs, res.st, n)
		}
	}
	return res
}

// counters reads basic (errors) and -x (64-bit data/packets) for every port of n.
// Whatever succeeded is exported; one error per failing node, not per command.
func (l *Loop) counters(ctx context.Context, fs *fabricState, st stats, n ib.Node) []ib.Sample {
	lid := strconv.Itoa(int(n.LID))
	basic, errB := l.perfquery(ctx, fs, st[rdPerf], "-a", "-l", lid)
	ext, errX := l.perfquery(ctx, fs, st[rdPerf], "-x", "-a", "-l", lid)
	st[rdPerf].record(errors.Join(errB, errX))
	return portSamples(fs, n, diags.MergeCounters(basic, ext))
}

func (l *Loop) optionalCounters(ctx context.Context, fs *fabricState, st stats, n ib.Node) []ib.Sample {
	lid := strconv.Itoa(int(n.LID))
	var ss []ib.Sample
	var errs []error
	for _, opt := range optionalOpts {
		pcs, err := l.perfquery(ctx, fs, st[rdOptional], opt, "-a", "-l", lid)
		errs = append(errs, err)
		ss = append(ss, portSamples(fs, n, pcs)...)
	}
	st[rdOptional].record(errors.Join(errs...))
	return ss
}

func (l *Loop) perfquery(ctx context.Context, fs *fabricState, st *readerStat, args ...string) ([]diags.PortCounters, error) {
	out, err := l.run(ctx, fs.f, st, "perfquery", args...)
	if err != nil {
		return nil, err
	}
	return diags.ParsePerfquery(out)
}

// portSamples converts counters for n. `-a` on a CA LID answers for every CA port, but an
// HCA node is one port GUID: keep only the ports the topology links to that GUID.
func portSamples(fs *fabricState, n ib.Node, pcs []diags.PortCounters) []ib.Sample {
	var ss []ib.Sample
	for _, pc := range pcs {
		if n.Type != ib.NodeSwitch && !slices.Contains(fs.ports[n.GUID], pc.Port) {
			continue
		}
		ss = append(ss, diags.ToSamples(fs.f.Name, n, pc)...)
	}
	return ss
}

// portInfoSamples queries each linked or remembered port in parallel; each port job times itself
// into its own readerStat, merged here. One error per failing node.
func (l *Loop) portInfoSamples(ctx context.Context, fs *fabricState, st stats, n ib.Node) []ib.Sample {
	type portResult struct {
		ss  []ib.Sample
		err error
		st  readerStat
	}
	lid, ports := strconv.Itoa(int(n.LID)), fs.ports[n.GUID]
	res := parallel(len(ports), l.cfg.Collection.MaxConcurrent, func(i int) portResult {
		var r portResult
		out, err := l.run(ctx, fs.f, &r.st, "smpquery", "portinfo", lid, strconv.Itoa(ports[i]))
		var pi diags.PortInfo
		if err == nil {
			pi, err = diags.ParsePortInfo(out)
		}
		if r.err = err; err == nil {
			r.ss = diags.PortInfoSamples(fs.f.Name, n, ports[i], pi)
		}
		return r
	})
	var ss []ib.Sample
	var errs []error
	for i := range res {
		st[rdPortInfo].add(&res[i].st)
		errs = append(errs, res[i].err)
		ss = append(ss, res[i].ss...)
	}
	st[rdPortInfo].record(errors.Join(errs...))
	return ss
}

func (l *Loop) smSamples(ctx context.Context, fs *fabricState) []ib.Sample {
	out, err := l.run(ctx, fs.f, fs.stats[rdSM], "sminfo")
	var sm diags.SMInfo
	if err == nil {
		sm, err = diags.ParseSMInfo(out)
	}
	fs.stats[rdSM].record(err)
	if err != nil {
		return nil
	}
	return diags.SMSamples(fs.f.Name, sm)
}

func (l *Loop) switchHealth(ctx context.Context, fs *fabricState, st stats, n ib.Node) []ib.Sample {
	out, err := l.run(ctx, fs.f, st[rdIbswinfo], l.cfg.Sources.Ibswinfo.Path, "-d", "lid-"+strconv.Itoa(int(n.LID)))
	var h diags.SwitchHealth
	if err == nil {
		h, err = diags.ParseIbswinfo(out)
	}
	st[rdIbswinfo].record(err)
	if err != nil {
		return nil
	}
	return diags.SwitchHealthSamples(fs.f.Name, n, h)
}

// run executes one tool and adds its time (including the wait for a concurrency slot)
// to st's cycle duration. st must be owned by the calling goroutine.
func (l *Loop) run(ctx context.Context, f config.Fabric, st *readerStat, tool string, args ...string) ([]byte, error) {
	start := time.Now()
	out, err := l.r.Run(ctx, f, tool, args...)
	st.cycDur += time.Since(start)
	st.cycRan = true
	return out, err
}

// topologySamples: ib_node_info (firmware is not in ibnetdiscover -p output, so "") and ib_link_info.
func topologySamples(fabric string, nodes []ib.Node, links []ib.Link) []ib.Sample {
	ss := make([]ib.Sample, 0, len(nodes)+len(links))
	for _, n := range nodes {
		ss = append(ss, ib.Sample{Name: "ib_node_info", Value: 1, Labels: []ib.Label{
			{Key: ib.LabelFabric, Value: fabric}, {Key: "guid", Value: n.GUID}, {Key: "node_name", Value: n.Name},
			{Key: "node_type", Value: string(n.Type)}, {Key: "lid", Value: strconv.Itoa(int(n.LID))}, {Key: "firmware", Value: ""}}})
	}
	for _, lk := range links {
		ss = append(ss, ib.Sample{Name: "ib_link_info", Value: 1, Labels: []ib.Label{
			{Key: ib.LabelFabric, Value: fabric}, {Key: "guid", Value: lk.GUID}, {Key: "port", Value: strconv.Itoa(lk.Port)},
			{Key: "remote_guid", Value: lk.RemoteGUID}, {Key: "remote_port", Value: strconv.Itoa(lk.RemotePort)},
			{Key: "remote_name", Value: lk.RemoteName}}})
	}
	return ss
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
