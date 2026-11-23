package collect

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/fjacquet/ib_exporter/internal/config"
	"github.com/fjacquet/ib_exporter/internal/ib"
)

const (
	swGUID  = "0x0002c90300aa0001"
	hcaGUID = "0x0002c90300bb0001"
	diagsTD = "../ib/diags/testdata/"
	sysfsTD = "../ib/sysfs/testdata/sysfs"
)

// fakeRunner serves outputs keyed by "<fabric>|<tool> <args>"; a missing key is an error.
type fakeRunner struct {
	mu            sync.Mutex
	out           map[string]string
	calls         map[string]int
	inflight, max int
	delay         time.Duration
}

func newFake() *fakeRunner { return &fakeRunner{out: map[string]string{}, calls: map[string]int{}} }

func key(fabric, tool string, args ...string) string {
	return fabric + "|" + strings.Join(append([]string{tool}, args...), " ")
}

func (r *fakeRunner) Run(_ context.Context, f config.Fabric, tool string, args ...string) ([]byte, error) {
	k := key(f.Name, tool, args...)
	r.mu.Lock()
	r.inflight++
	r.max = max(r.max, r.inflight)
	r.calls[k]++
	r.mu.Unlock()
	time.Sleep(r.delay)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inflight--
	o, ok := r.out[k]
	if !ok {
		return nil, fmt.Errorf("no fixture for %s", k)
	}
	return []byte(o), nil
}

func (r *fakeRunner) set(k, v string) { r.mu.Lock(); r.out[k] = v; r.mu.Unlock() }
func (r *fakeRunner) del(k string)    { r.mu.Lock(); delete(r.out, k); r.mu.Unlock() }
func (r *fakeRunner) count(k string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls[k]
}

func readTD(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(diagsTD + p)
	require.NoError(t, err)
	return string(b)
}

func topology(swLID, hcaLID int) string {
	return fmt.Sprintf("SW %5d  1 %s 4x EDR - CA %5d  1 %s ( 'sw1' - 'hca1' )\n"+
		"SW %5d  2 %s 4x EDR\n"+
		"CA %5d  1 %s 4x EDR - SW %5d  1 %s ( 'hca1' - 'sw1' )\n",
		swLID, swGUID, hcaLID, hcaGUID, swLID, swGUID, hcaLID, hcaGUID, swLID, swGUID)
}

// basic carries SymbolErrorCounter = lid so tests can see which LID was queried.
func basic(lid int, ports ...int) string {
	var b strings.Builder
	for _, p := range ports {
		fmt.Fprintf(&b, "# Port counters: Lid %d port %d (CapMask: 0x5A00)\nSymbolErrorCounter:..............%d\n"+
			"LinkDownedCounter:...............1\nPortXmitData:....................4294967295\nPortRcvPkts:.....................100\n", lid, p, lid)
	}
	return b.String()
}

func ext(lid int, ports ...int) string {
	var b strings.Builder
	for _, p := range ports {
		fmt.Fprintf(&b, "# Port extended counters: Lid %d port %d (CapMask: 0x5A00)\n"+
			"PortXmitData:....................10000000000\nPortRcvPkts:.....................400000000\n", lid, p)
	}
	return b.String()
}

// register serves a coherent fabric: switch (2 ports) linked on port 1 to a 1-port HCA.
// The HCA answers perfquery with blocks for ports 1 and 2; only port 1 is in the topology.
func register(t *testing.T, r *fakeRunner, fabric string, swLID, hcaLID int) {
	t.Helper()
	r.set(key(fabric, "ibnetdiscover", "-p"), topology(swLID, hcaLID))
	for _, lid := range []int{swLID, hcaLID} {
		l := fmt.Sprint(lid)
		r.set(key(fabric, "perfquery", "-a", "-l", l), basic(lid, 1, 2))
		r.set(key(fabric, "perfquery", "-x", "-a", "-l", l), ext(lid, 1, 2))
		r.set(key(fabric, "perfquery", "-T", "-a", "-l", l), readTD(t, "perfquery/speeds.txt"))
		r.set(key(fabric, "perfquery", "-E", "-a", "-l", l), readTD(t, "perfquery/rcverr.txt"))
		r.set(key(fabric, "perfquery", "-D", "-a", "-l", l), readTD(t, "perfquery/xmitdisc.txt"))
		r.set(key(fabric, "perfquery", "--rcvcc", "-a", "-l", l),
			"# PortRcvConCtrl counters: Lid 5 port 1\nPortPktRcvFECN:..................11\nPortPktRcvBECN:..................12\n")
		r.set(key(fabric, "perfquery", "--xmitcc", "-a", "-l", l),
			"# PortXmitConCtrl counters: Lid 5 port 1\nPortXmitTimeCong:................13\n")
		r.set(key(fabric, "smpquery", "portinfo", l, "1"), readTD(t, "portinfo/active.txt"))
	}
	r.set(key(fabric, "sminfo"), readTD(t, "sminfo/master.txt"))
	r.set(key(fabric, "ibswinfo", "-d", fmt.Sprintf("lid-%d", swLID)), readTD(t, "ibswinfo/basic.txt"))
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newCfg(fabrics ...string) *config.Config {
	c := &config.Config{Collection: config.Collection{Interval: time.Minute, TopologyInterval: 10 * time.Minute,
		PortInfoInterval: 5 * time.Minute, MaxConcurrent: 4, OptionalEvery: 5}}
	for _, f := range fabrics {
		c.Fabrics = append(c.Fabrics, config.Fabric{Name: f, CA: "mlx5_0", Port: 1})
	}
	return c
}

func newLoop(cfg *config.Config, r *fakeRunner, clk *clock, opts ...Option) (*Loop, *ib.SnapshotStore) {
	st := &ib.SnapshotStore{}
	return NewCollectionLoop(cfg, r, st, append([]Option{WithClock(clk.now)}, opts...)...), st
}

func labelsOf(s ib.Sample) map[string]string {
	m := map[string]string{}
	for _, l := range s.Labels {
		m[l.Key] = l.Value
	}
	return m
}

// find returns the samples named name whose labels include all of want.
func find(ss []ib.Sample, name string, want map[string]string) []ib.Sample {
	var out []ib.Sample
	for _, s := range ss {
		if s.Name != name {
			continue
		}
		lm, ok := labelsOf(s), true
		for k, v := range want {
			ok = ok && lm[k] == v
		}
		if ok {
			out = append(out, s)
		}
	}
	return out
}

func one(t *testing.T, ss []ib.Sample, name string, want map[string]string) ib.Sample {
	t.Helper()
	got := find(ss, name, want)
	require.Len(t, got, 1, "%s %v", name, want)
	return got[0]
}

func allSamples(s *ib.Snapshot) []ib.Sample {
	out := slices.Clone(s.HCA)
	for _, f := range s.Fabrics {
		out = append(out, f.Samples...)
	}
	return out
}

// assertInvariants: every sample has a fabric label, one label-key set per name, no duplicate series.
func assertInvariants(t *testing.T, s *ib.Snapshot) {
	t.Helper()
	keys := map[string]string{}
	seen := map[string]bool{}
	for _, smp := range allSamples(s) {
		var ks, kv []string
		for _, l := range smp.Labels {
			ks = append(ks, l.Key)
			kv = append(kv, l.Key+"="+l.Value)
		}
		require.Contains(t, ks, ib.LabelFabric, smp.Name)
		k := strings.Join(ks, ",")
		if prev, ok := keys[smp.Name]; ok {
			require.Equal(t, prev, k, "label keys of %s", smp.Name)
		}
		keys[smp.Name] = k
		id := smp.Name + "{" + strings.Join(kv, ",") + "}"
		require.False(t, seen[id], "duplicate series %s", id)
		seen[id] = true
	}
}

var readers = []string{"topology", "perfquery", "perfquery_optional", "portinfo", "sm", "ibswinfo"}

func TestLoop_BuildsSnapshotPerFabric(t *testing.T) {
	r := newFake()
	register(t, r, "a", 1, 2)
	register(t, r, "b", 21, 22)
	cfg := newCfg("a", "b")
	cfg.Sources.LocalHCA = true
	cfg.Sources.Ibswinfo = config.Ibswinfo{Enabled: true, Path: "ibswinfo"}
	clk := &clock{time.Unix(1000, 0)}
	l, st := newLoop(cfg, r, clk, WithHCARoot(sysfsTD))
	require.NoError(t, l.Once(context.Background()))

	snap := st.Load()
	require.Equal(t, clk.t, snap.BuiltAt)
	require.Len(t, snap.Fabrics, 2)
	assertInvariants(t, snap)
	for _, fab := range []string{"a", "b"} {
		fs := snap.Fabrics[fab]
		require.True(t, fs.Up, fab)
		require.Equal(t, fab, fs.Fabric)
		require.Len(t, fs.Nodes, 2)
		require.Len(t, fs.Links, 2)
		ss := fs.Samples
		sw := map[string]string{"fabric": fab, "guid": swGUID, "port": "1"}
		require.InDelta(t, 4e10, one(t, ss, "ib_port_xmit_data_bytes_total", sw).Value, 0)
		require.InDelta(t, 4e8, one(t, ss, "ib_port_rcv_packets_total", sw).Value, 0)
		one(t, ss, "ib_port_symbol_error_total", sw)
		one(t, ss, "ib_port_fec_corrected_blocks_total", map[string]string{"fabric": fab, "guid": swGUID, "port": "1", "lane": "1"})
		one(t, ss, "ib_port_local_physical_errors_total", sw)
		one(t, ss, "ib_port_xmit_inactive_discards_total", sw)
		one(t, ss, "ib_port_rcv_fecn_total", sw)
		one(t, ss, "ib_port_xmit_time_cong_total", sw)
		one(t, ss, "ib_port_state", sw)
		one(t, ss, "ib_sm_info", map[string]string{"fabric": fab})
		require.NotEmpty(t, find(ss, "ib_switch_fan_rpm", map[string]string{"fabric": fab, "guid": swGUID}))
		require.Empty(t, find(ss, "ib_switch_fan_rpm", map[string]string{"guid": hcaGUID}), "ibswinfo only on switches")
		one(t, ss, "ib_node_info", map[string]string{"fabric": fab, "guid": swGUID, "node_name": "sw1", "node_type": "switch", "firmware": ""})
		one(t, ss, "ib_link_info", map[string]string{"fabric": fab, "guid": hcaGUID, "port": "1", "remote_guid": swGUID, "remote_port": "1", "remote_name": "sw1"})
		// The HCA answered for port 2 too, but its port GUID only owns port 1.
		require.Empty(t, find(ss, "ib_port_symbol_error_total", map[string]string{"guid": hcaGUID, "port": "2"}))
		one(t, ss, "ib_port_symbol_error_total", map[string]string{"guid": hcaGUID, "port": "1"})
		require.InDelta(t, 1, one(t, ss, "ib_up", map[string]string{"fabric": fab}).Value, 0)
		for _, rd := range readers {
			lbl := map[string]string{"fabric": fab, "reader": rd}
			e := one(t, ss, "ib_collect_errors_total", lbl)
			require.True(t, e.Counter)
			require.InDelta(t, 0, e.Value, 0, rd)
			require.InDelta(t, 1000, one(t, ss, "ib_collect_last_success_timestamp_seconds", lbl).Value, 0, rd)
			one(t, ss, "ib_collect_duration_seconds", lbl)
		}
	}
	// local HCA mlx5_0 port 1 is bound to both fabrics' CA/port; the first fabric wins.
	hca := find(snap.HCA, "ib_hca_link_downed_total", map[string]string{"device": "mlx5_0", "port": "1"})
	require.Len(t, hca, 1)
	require.Equal(t, "a", labelsOf(hca[0])["fabric"])
	require.InDelta(t, 0, one(t, snap.HCA, "ib_collect_errors_total", map[string]string{"fabric": "", "reader": "sysfs"}).Value, 0)
}

func TestLoop_ResolvesGUIDToCurrentLID(t *testing.T) {
	r := newFake()
	register(t, r, "a", 1, 2)
	clk := &clock{time.Unix(1000, 0)}
	l, st := newLoop(newCfg("a"), r, clk)
	require.NoError(t, l.Once(context.Background()))
	sw := map[string]string{"guid": swGUID, "port": "1"}
	require.InDelta(t, 1, one(t, st.Load().Fabrics["a"].Samples, "ib_port_symbol_error_total", sw).Value, 0)

	// The switch is re-addressed: old LID gone, new LID 7.
	r.del(key("a", "perfquery", "-a", "-l", "1"))
	r.del(key("a", "perfquery", "-x", "-a", "-l", "1"))
	register(t, r, "a", 7, 2)
	clk.t = clk.t.Add(10 * time.Minute)
	require.NoError(t, l.Once(context.Background()))
	fs := st.Load().Fabrics["a"]
	require.InDelta(t, 7, one(t, fs.Samples, "ib_port_symbol_error_total", sw).Value, 0)
	require.InDelta(t, 0, one(t, fs.Samples, "ib_collect_errors_total", map[string]string{"reader": "perfquery"}).Value, 0)
	require.Equal(t, 1, r.count(key("a", "perfquery", "-x", "-a", "-l", "7")))
}

func TestLoop_OptionalReadersEveryNthCycle(t *testing.T) {
	r := newFake()
	register(t, r, "a", 1, 2)
	cfg := newCfg("a")
	cfg.Collection.OptionalEvery = 3
	l, st := newLoop(cfg, r, &clock{time.Unix(1000, 0)})
	fec := map[string]string{"guid": swGUID, "port": "1", "lane": "1"}
	for cycle := 0; cycle < 4; cycle++ {
		require.NoError(t, l.Once(context.Background()))
		ss := st.Load().Fabrics["a"].Samples
		require.Len(t, find(ss, "ib_port_fec_corrected_blocks_total", fec), 1, "cycle %d reuses prior optional samples", cycle)
		one(t, ss, "ib_port_rcv_fecn_total", map[string]string{"guid": swGUID, "port": "1"})
	}
	require.Equal(t, 2, r.count(key("a", "perfquery", "-T", "-a", "-l", "1")), "cycles 0 and 3")
	require.Equal(t, 4, r.count(key("a", "perfquery", "-x", "-a", "-l", "1")), "-x every cycle")
}

func TestLoop_FabricFailureSetsUpZeroOthersContinue(t *testing.T) {
	r := newFake()
	register(t, r, "b", 21, 22)
	clk := &clock{time.Unix(1000, 0)}
	l, st := newLoop(newCfg("a", "b"), r, clk)
	require.NoError(t, l.Once(context.Background()), "b is up")
	snap := st.Load()
	assertInvariants(t, snap)
	a := snap.Fabrics["a"]
	require.False(t, a.Up)
	require.Empty(t, a.Nodes)
	for _, s := range a.Samples {
		require.True(t, s.Name == "ib_up" || strings.HasPrefix(s.Name, "ib_collect_"), s.Name)
	}
	require.InDelta(t, 0, one(t, a.Samples, "ib_up", nil).Value, 0)
	require.InDelta(t, 1, one(t, a.Samples, "ib_collect_errors_total", map[string]string{"reader": "topology"}).Value, 0)
	require.Empty(t, find(a.Samples, "ib_collect_last_success_timestamp_seconds", map[string]string{"reader": "topology"}))
	require.True(t, snap.Fabrics["b"].Up)
	one(t, snap.Fabrics["b"].Samples, "ib_port_symbol_error_total", map[string]string{"guid": swGUID, "port": "1"})

	// A later topology failure keeps the stale topology but marks the fabric down.
	r.del(key("b", "ibnetdiscover", "-p"))
	clk.t = clk.t.Add(10 * time.Minute)
	require.Error(t, l.Once(context.Background()), "every fabric down")
	b := st.Load().Fabrics["b"]
	require.False(t, b.Up)
	require.Len(t, b.Nodes, 2)
	one(t, b.Samples, "ib_port_symbol_error_total", map[string]string{"guid": swGUID, "port": "1"})
	require.InDelta(t, 1, one(t, b.Samples, "ib_collect_errors_total", map[string]string{"reader": "topology"}).Value, 0)
	require.InDelta(t, 2, one(t, st.Load().Fabrics["a"].Samples, "ib_collect_errors_total", map[string]string{"reader": "topology"}).Value, 0)
}

func TestLoop_UnparsableTopologyIsDown(t *testing.T) {
	r := newFake()
	r.set(key("a", "ibnetdiscover", "-p"), "SW broken line\n")
	l, st := newLoop(newCfg("a"), r, &clock{time.Unix(1000, 0)})
	require.Error(t, l.Once(context.Background()))
	a := st.Load().Fabrics["a"]
	require.False(t, a.Up)
	require.Empty(t, a.Nodes)
	for _, s := range a.Samples {
		require.True(t, s.Name == "ib_up" || strings.HasPrefix(s.Name, "ib_collect_"), s.Name)
	}
	require.InDelta(t, 1, one(t, a.Samples, "ib_collect_errors_total", map[string]string{"reader": "topology"}).Value, 0)
}

// registerWide serves one switch (LID 1) with n HCAs (LIDs 101..) on ports 1..n.
func registerWide(r *fakeRunner, fabric string, n int) {
	var topo strings.Builder
	for i := 1; i <= n; i++ {
		g := fmt.Sprintf("0x0002c90300cc%04x", i)
		fmt.Fprintf(&topo, "SW     1 %2d %s 4x EDR - CA %5d  1 %s ( 'sw1' - 'h%d' )\n", i, swGUID, 100+i, g, i)
		fmt.Fprintf(&topo, "CA %5d  1 %s 4x EDR - SW     1 %2d %s ( 'h%d' - 'sw1' )\n", 100+i, g, i, swGUID, i)
	}
	r.set(key(fabric, "ibnetdiscover", "-p"), topo.String())
	lids := []int{1}
	for i := 1; i <= n; i++ {
		lids = append(lids, 100+i)
	}
	for _, lid := range lids {
		r.set(key(fabric, "perfquery", "-a", "-l", fmt.Sprint(lid)), basic(lid, 1))
		r.set(key(fabric, "perfquery", "-x", "-a", "-l", fmt.Sprint(lid)), ext(lid, 1))
	}
}

func TestLoop_MaxConcurrentRespected(t *testing.T) {
	t.Run("single fabric fills the limit", func(t *testing.T) {
		r := newFake()
		r.delay = 2 * time.Millisecond
		registerWide(r, "a", 10)
		cfg := newCfg("a")
		cfg.Collection.MaxConcurrent = 3
		l, st := newLoop(cfg, r, &clock{time.Unix(1000, 0)})
		require.NoError(t, l.Once(context.Background()))
		require.True(t, st.Load().Fabrics["a"].Up)
		require.Len(t, find(st.Load().Fabrics["a"].Samples, "ib_port_xmit_data_bytes_total", nil), 11)
		require.LessOrEqual(t, r.max, 3)
		require.Equal(t, 3, r.max, "nodes of one fabric run in parallel up to maxConcurrent")
	})
	t.Run("limit is global across fabrics", func(t *testing.T) {
		r := newFake()
		r.delay = 2 * time.Millisecond
		names := []string{"f1", "f2", "f3", "f4", "f5"}
		for _, n := range names {
			registerWide(r, n, 6)
		}
		cfg := newCfg(names...)
		cfg.Collection.MaxConcurrent = 2
		l, st := newLoop(cfg, r, &clock{time.Unix(1000, 0)})
		require.NoError(t, l.Once(context.Background()))
		require.Len(t, st.Load().Fabrics, 5)
		require.LessOrEqual(t, r.max, 2)
		require.Equal(t, 2, r.max)
	})
}

func TestLoop_SampleOrderIsDeterministic(t *testing.T) {
	r := newFake()
	registerWide(r, "a", 8)
	names := func() []string {
		l, st := newLoop(newCfg("a"), r, &clock{time.Unix(1000, 0)})
		require.NoError(t, l.Once(context.Background()))
		var ns []string
		for _, s := range st.Load().Fabrics["a"].Samples {
			if !strings.HasPrefix(s.Name, "ib_collect_") {
				ns = append(ns, fmt.Sprint(s.Name, s.Labels))
			}
		}
		return ns
	}
	first := names()
	for range 5 {
		require.Equal(t, first, names())
	}
}

func TestLoop_StaleLIDYieldsAbsentSamplesAndErrorCount(t *testing.T) {
	r := newFake()
	register(t, r, "a", 1, 2)
	r.del(key("a", "perfquery", "-a", "-l", "2"))
	r.del(key("a", "perfquery", "-x", "-a", "-l", "2"))
	l, st := newLoop(newCfg("a"), r, &clock{time.Unix(1000, 0)})
	for want := 1; want <= 2; want++ {
		require.NoError(t, l.Once(context.Background()))
		fs := st.Load().Fabrics["a"]
		require.True(t, fs.Up)
		for _, n := range []string{"ib_port_symbol_error_total", "ib_port_link_downed_total", "ib_port_rcv_packets_total"} {
			require.Empty(t, find(fs.Samples, n, map[string]string{"guid": hcaGUID}), n)
		}
		require.Empty(t, find(fs.Samples, "ib_port_xmit_data_bytes_total", map[string]string{"guid": hcaGUID}))
		one(t, fs.Samples, "ib_port_xmit_data_bytes_total", map[string]string{"guid": swGUID, "port": "1"})
		require.InDelta(t, float64(want), one(t, fs.Samples, "ib_collect_errors_total", map[string]string{"reader": "perfquery"}).Value, 0)
	}
}

func TestLoop_DataSamplesOnlyFromExtended(t *testing.T) {
	r := newFake()
	register(t, r, "a", 1, 2)
	r.del(key("a", "perfquery", "-x", "-a", "-l", "2"))
	l, st := newLoop(newCfg("a"), r, &clock{time.Unix(1000, 0)})
	require.NoError(t, l.Once(context.Background()))
	ss := st.Load().Fabrics["a"].Samples
	hca := map[string]string{"guid": hcaGUID, "port": "1"}
	require.InDelta(t, 2, one(t, ss, "ib_port_symbol_error_total", hca).Value, 0)
	one(t, ss, "ib_port_link_downed_total", hca)
	require.Empty(t, find(ss, "ib_port_xmit_data_bytes_total", hca), "32-bit basic data never exported")
	require.Empty(t, find(ss, "ib_port_rcv_packets_total", hca))
	require.InDelta(t, 1, one(t, ss, "ib_collect_errors_total", map[string]string{"reader": "perfquery"}).Value, 0)
}

func TestLoop_LocalHCAOnlyNoFabrics(t *testing.T) {
	cfg := newCfg()
	cfg.Sources.LocalHCA = true
	l, st := newLoop(cfg, newFake(), &clock{time.Unix(1000, 0)}, WithHCARoot(sysfsTD))
	require.NoError(t, l.Once(context.Background()))
	snap := st.Load()
	require.Empty(t, snap.Fabrics)
	assertInvariants(t, snap)
	hca := find(snap.HCA, "ib_hca_link_downed_total", map[string]string{"device": "mlx5_0", "port": "1"})
	require.Len(t, hca, 1)
	require.Empty(t, labelsOf(hca[0])["fabric"])

	l, st = newLoop(cfg, newFake(), &clock{time.Unix(1000, 0)}, WithHCARoot(t.TempDir()+"/missing"))
	require.Error(t, l.Once(context.Background()))
	require.Error(t, l.Once(context.Background()))
	snap = st.Load()
	require.InDelta(t, 2, one(t, snap.HCA, "ib_collect_errors_total", map[string]string{"fabric": "", "reader": "sysfs"}).Value, 0)
	for _, s := range snap.HCA {
		require.False(t, strings.HasPrefix(s.Name, "ib_hca_"), s.Name)
	}
}

func TestLoop_RunCyclesUntilCancelled(t *testing.T) {
	r := newFake()
	register(t, r, "a", 1, 2)
	cfg := newCfg("a")
	cfg.Collection.Interval = time.Millisecond
	l, st := newLoop(cfg, r, &clock{time.Unix(1000, 0)})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { l.Run(ctx); close(done) }()
	require.Eventually(t, func() bool { return r.count(key("a", "perfquery", "-x", "-a", "-l", "1")) >= 3 }, 5*time.Second, time.Millisecond)
	cancel()
	<-done
	require.True(t, st.Load().Fabrics["a"].Up)
}

// ctxRunner fails every call with ctx.Err() once ctx is done, like a killed exec.
type ctxRunner struct{ *fakeRunner }

func (r ctxRunner) Run(ctx context.Context, f config.Fabric, tool string, args ...string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return r.fakeRunner.Run(ctx, f, tool, args...)
}

func TestLoop_CancelledCycleDoesNotSwap(t *testing.T) {
	r := newFake()
	register(t, r, "a", 5, 6)
	clk := &clock{t: time.Unix(1000, 0)}
	st := &ib.SnapshotStore{}
	l := NewCollectionLoop(newCfg("a"), ctxRunner{r}, st, WithClock(clk.now), WithHCARoot(t.TempDir()))
	require.NoError(t, l.Once(context.Background()))
	good := st.Load()
	require.True(t, good.Fabrics["a"].Up)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := l.Once(ctx)
	require.ErrorIs(t, err, context.Canceled)
	require.Same(t, good, st.Load(), "a cancelled cycle must not replace the snapshot")

	require.NoError(t, l.Once(context.Background()))
	e := one(t, st.Load().Fabrics["a"].Samples, "ib_collect_errors_total", map[string]string{"reader": "perfquery"})
	require.InDelta(t, 0, e.Value, 0, "errors caused only by cancellation are not counted")
}
