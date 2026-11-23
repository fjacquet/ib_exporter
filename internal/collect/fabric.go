package collect

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"time"

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
	ports    map[string][]int // GUID -> connected ports, from links
	topoAt   time.Time
	piAt     time.Time
	// Caches keyed by GUID (never LID), reused between runs of slower readers.
	optional map[string][]ib.Sample
	portInfo map[string][]ib.Sample
	health   map[string][]ib.Sample
	sm       []ib.Sample
	stats    map[string]*readerStat
}

func newFabricState(f config.Fabric) *fabricState {
	fs := &fabricState{f: f, stats: map[string]*readerStat{}}
	for _, r := range []string{rdTopology, rdPerf, rdOptional, rdPortInfo, rdSM, rdIbswinfo} {
		fs.stats[r] = &readerStat{}
	}
	return fs
}

func (l *Loop) collectFabric(ctx context.Context, fs *fabricState, now time.Time, optional bool) *ib.FabricSnapshot {
	up := l.refreshTopology(ctx, fs, now)
	var ss []ib.Sample
	if fs.haveTopo {
		ss = l.fabricSamples(ctx, fs, now, optional)
	}
	ss = append(ss, ib.Sample{Name: "ib_up", Labels: []ib.Label{{Key: ib.LabelFabric, Value: fs.f.Name}}, Value: b2f(up)})
	readers := []string{rdTopology, rdPerf, rdOptional, rdPortInfo, rdSM}
	if l.cfg.Sources.Ibswinfo.Enabled {
		readers = append(readers, rdIbswinfo)
	}
	for _, r := range readers {
		fs.stats[r].finish(now)
		ss = append(ss, fs.stats[r].samples(fs.f.Name, r)...)
	}
	return &ib.FabricSnapshot{Fabric: fs.f.Name, Up: up, Nodes: fs.nodes, Links: fs.links, Samples: ss, At: now}
}

// refreshTopology re-reads the topology when due. A failure keeps the last good
// topology but reports the fabric down for this cycle; it is retried next cycle.
func (l *Loop) refreshTopology(ctx context.Context, fs *fabricState, now time.Time) bool {
	if fs.haveTopo && now.Sub(fs.topoAt) < l.cfg.Collection.TopologyInterval {
		return true
	}
	out, err := l.run(ctx, fs, rdTopology, "ibnetdiscover", "-p")
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
	fs.ports = map[string][]int{}
	for _, lk := range links {
		fs.ports[lk.GUID] = append(fs.ports[lk.GUID], lk.Port)
	}
	return true
}

// fabricSamples builds a fresh slice every cycle: cached slices are only copied from.
func (l *Loop) fabricSamples(ctx context.Context, fs *fabricState, now time.Time, optional bool) []ib.Sample {
	ss := topologySamples(fs.f.Name, fs.nodes, fs.links)
	runPI := fs.piAt.IsZero() || now.Sub(fs.piAt) >= l.cfg.Collection.PortInfoInterval
	if optional {
		fs.optional = map[string][]ib.Sample{}
	}
	if runPI {
		fs.piAt, fs.portInfo, fs.health = now, map[string][]ib.Sample{}, map[string][]ib.Sample{}
		fs.sm = l.smSamples(ctx, fs)
	}
	for _, n := range fs.nodes {
		if n.LID == 0 {
			continue
		}
		ss = append(ss, l.counters(ctx, fs, n)...)
		if optional {
			fs.optional[n.GUID] = l.optionalCounters(ctx, fs, n)
		}
		if runPI {
			fs.portInfo[n.GUID] = l.portInfoSamples(ctx, fs, n)
			if l.cfg.Sources.Ibswinfo.Enabled && n.Type == ib.NodeSwitch {
				fs.health[n.GUID] = l.switchHealth(ctx, fs, n)
			}
		}
		ss = append(append(append(ss, fs.optional[n.GUID]...), fs.portInfo[n.GUID]...), fs.health[n.GUID]...)
	}
	return append(ss, fs.sm...)
}

// counters reads basic (errors) and -x (64-bit data/packets) for every port of n.
// Whatever succeeded is exported; one error per failing node, not per command.
func (l *Loop) counters(ctx context.Context, fs *fabricState, n ib.Node) []ib.Sample {
	lid := strconv.Itoa(int(n.LID))
	basic, errB := l.perfquery(ctx, fs, rdPerf, "-a", "-l", lid)
	ext, errX := l.perfquery(ctx, fs, rdPerf, "-x", "-a", "-l", lid)
	fs.stats[rdPerf].record(errors.Join(errB, errX))
	return portSamples(fs, n, diags.MergeCounters(basic, ext))
}

func (l *Loop) optionalCounters(ctx context.Context, fs *fabricState, n ib.Node) []ib.Sample {
	lid := strconv.Itoa(int(n.LID))
	var ss []ib.Sample
	var errs []error
	for _, opt := range optionalOpts {
		pcs, err := l.perfquery(ctx, fs, rdOptional, opt, "-a", "-l", lid)
		errs = append(errs, err)
		ss = append(ss, portSamples(fs, n, pcs)...)
	}
	fs.stats[rdOptional].record(errors.Join(errs...))
	return ss
}

func (l *Loop) perfquery(ctx context.Context, fs *fabricState, reader string, args ...string) ([]diags.PortCounters, error) {
	out, err := l.run(ctx, fs, reader, "perfquery", args...)
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

func (l *Loop) portInfoSamples(ctx context.Context, fs *fabricState, n ib.Node) []ib.Sample {
	lid := strconv.Itoa(int(n.LID))
	var ss []ib.Sample
	var errs []error
	for _, p := range fs.ports[n.GUID] {
		out, err := l.run(ctx, fs, rdPortInfo, "smpquery", "portinfo", lid, strconv.Itoa(p))
		var pi diags.PortInfo
		if err == nil {
			pi, err = diags.ParsePortInfo(out)
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}
		ss = append(ss, diags.PortInfoSamples(fs.f.Name, n, p, pi)...)
	}
	fs.stats[rdPortInfo].record(errors.Join(errs...))
	return ss
}

func (l *Loop) smSamples(ctx context.Context, fs *fabricState) []ib.Sample {
	out, err := l.run(ctx, fs, rdSM, "sminfo")
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

func (l *Loop) switchHealth(ctx context.Context, fs *fabricState, n ib.Node) []ib.Sample {
	out, err := l.run(ctx, fs, rdIbswinfo, l.cfg.Sources.Ibswinfo.Path, "-d", "lid-"+strconv.Itoa(int(n.LID)))
	var h diags.SwitchHealth
	if err == nil {
		h, err = diags.ParseIbswinfo(out)
	}
	fs.stats[rdIbswinfo].record(err)
	if err != nil {
		return nil
	}
	return diags.SwitchHealthSamples(fs.f.Name, n, h)
}

// run executes one tool and adds its wall time to the reader's cycle duration.
func (l *Loop) run(ctx context.Context, fs *fabricState, reader, tool string, args ...string) ([]byte, error) {
	st := fs.stats[reader]
	start := time.Now()
	out, err := l.r.Run(ctx, fs.f, tool, args...)
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
