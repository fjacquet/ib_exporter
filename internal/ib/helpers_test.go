package ib_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/fjacquet/ib_exporter/internal/collect"
	"github.com/fjacquet/ib_exporter/internal/config"
	"github.com/fjacquet/ib_exporter/internal/ib"
	"github.com/fjacquet/ib_exporter/internal/ib/diags"
	"github.com/fjacquet/ib_exporter/internal/ib/replay"
)

const (
	diagsTD = "diags/testdata/"
	sysfsTD = "sysfs/testdata/sysfs"
)

func readTD(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(diagsTD + p)
	require.NoError(t, err)
	return b
}

// writeReplay lays out replay fixtures for fabric from the shared diags testdata:
// every node of ibnetdiscover/fabric.txt answers every reader.
func writeReplay(t *testing.T, dir, fabric string) {
	t.Helper()
	fdir := filepath.Join(dir, fabric)
	require.NoError(t, os.MkdirAll(fdir, 0o755))
	put := func(file string, b []byte) {
		require.NoError(t, os.WriteFile(filepath.Join(fdir, file), b, 0o644))
	}
	topo := readTD(t, "ibnetdiscover/fabric.txt")
	put("ibnetdiscover__-p.txt", topo)
	put("sminfo__.txt", readTD(t, "sminfo/master.txt"))
	nodes, links, err := diags.ParseTopology(topo, nil)
	require.NoError(t, err)
	// congestion.txt holds the --rcvcc block then the --xmitcc block.
	cong := readTD(t, "perfquery/congestion.txt")
	i := strings.Index(string(cong), "# PortXmitConCtrl")
	require.Positive(t, i)
	for _, n := range nodes {
		lid := fmt.Sprint(n.LID)
		put("perfquery__-a_-l_"+lid+".txt", readTD(t, "perfquery/basic.txt"))
		put("perfquery__-x_-a_-l_"+lid+".txt", readTD(t, "perfquery/extended.txt"))
		put("perfquery__-T_-a_-l_"+lid+".txt", readTD(t, "perfquery/speeds.txt"))
		put("perfquery__-E_-a_-l_"+lid+".txt", readTD(t, "perfquery/rcverr.txt"))
		put("perfquery__-D_-a_-l_"+lid+".txt", readTD(t, "perfquery/xmitdisc.txt"))
		put("perfquery__--rcvcc_-a_-l_"+lid+".txt", cong[:i])
		put("perfquery__--xmitcc_-a_-l_"+lid+".txt", cong[i:])
		put("ibswinfo__-d_lid-"+lid+".txt", readTD(t, "ibswinfo/basic.txt"))
	}
	for _, lk := range links {
		for _, n := range nodes {
			if n.GUID == lk.GUID {
				put(fmt.Sprintf("smpquery__portinfo_%d_%d.txt", n.LID, lk.Port), readTD(t, "portinfo/active.txt"))
			}
		}
	}
}

// fixtureStore runs one real collection cycle over replay fixtures for the given
// fabrics (only those in withData get fixtures; the others are down) plus the
// sysfs fixture, whose mlx5_0/1 is bound to the first fabric and foo_0 is unbound.
func fixtureStore(t *testing.T, fabrics []string, withData ...string) *ib.SnapshotStore {
	t.Helper()
	dir := t.TempDir()
	for _, f := range withData {
		writeReplay(t, dir, f)
	}
	cfg := &config.Config{Collection: config.Collection{Interval: time.Minute, TopologyInterval: 10 * time.Minute,
		PortInfoInterval: 5 * time.Minute, MaxConcurrent: 4, OptionalEvery: 5}}
	for _, f := range fabrics {
		cfg.Fabrics = append(cfg.Fabrics, config.Fabric{Name: f, CA: "mlx5_0", Port: 1})
	}
	cfg.Sources.LocalHCA = true
	cfg.Sources.Ibswinfo.Enabled, cfg.Sources.Ibswinfo.Path = true, "ibswinfo"
	st := &ib.SnapshotStore{}
	l := collect.NewCollectionLoop(cfg, replay.Runner{Dir: dir}, st, collect.WithHCARoot(sysfsTD),
		collect.WithClock(func() time.Time { return time.Unix(1000, 0) }))
	_ = l.Once(context.Background()) // down fabrics are data, not a failure here
	return st
}

func fullStore(t *testing.T) *ib.SnapshotStore {
	return fixtureStore(t, []string{"prod"}, "prod")
}

// rawSamples flattens a snapshot exactly as stored (no export-side filtering).
func rawSamples(s *ib.Snapshot) []ib.Sample {
	out := slices.Clone(s.HCA)
	for _, f := range s.Fabrics {
		out = append(out, f.Samples...)
	}
	return out
}

// series is one exported observation, labels rendered sorted by key.
type series struct {
	name, labels string
	value        float64
	counter      bool
}

func render(kv map[string]string) string {
	keys := make([]string, 0, len(kv))
	for k := range kv {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + kv[k]
	}
	return strings.Join(parts, ",")
}

func gatherProm(t *testing.T, c prometheus.Collector) ([]*dto.MetricFamily, []series) {
	t.Helper()
	reg := prometheus.NewRegistry()
	reg.MustRegister(c)
	mfs, err := reg.Gather()
	require.NoError(t, err)
	var out []series
	for _, mf := range mfs {
		for _, m := range mf.GetMetric() {
			kv := map[string]string{}
			for _, lp := range m.GetLabel() {
				kv[lp.GetName()] = lp.GetValue()
			}
			s := series{name: mf.GetName(), labels: render(kv)}
			if mf.GetType() == dto.MetricType_COUNTER {
				s.value, s.counter = m.GetCounter().GetValue(), true
			} else {
				s.value = m.GetGauge().GetValue()
			}
			out = append(out, s)
		}
	}
	return mfs, out
}

func collectOTLP(t *testing.T, reader *sdkmetric.ManualReader) []series {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
	attrs := func(set attribute.Set) string {
		kv := map[string]string{}
		for _, a := range set.ToSlice() {
			kv[string(a.Key)] = a.Value.AsString()
		}
		return render(kv)
	}
	var out []series
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			switch d := m.Data.(type) {
			case metricdata.Gauge[float64]:
				for _, dp := range d.DataPoints {
					out = append(out, series{name: m.Name, labels: attrs(dp.Attributes), value: dp.Value})
				}
			case metricdata.Sum[float64]:
				require.True(t, d.IsMonotonic, m.Name)
				require.Equal(t, metricdata.CumulativeTemporality, d.Temporality, m.Name)
				for _, dp := range d.DataPoints {
					out = append(out, series{name: m.Name, labels: attrs(dp.Attributes), value: dp.Value, counter: true})
				}
			default:
				t.Fatalf("%s: unexpected data type %T", m.Name, m.Data)
			}
		}
	}
	return out
}

func newOTLP(t *testing.T, st *ib.SnapshotStore) *sdkmetric.ManualReader {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })
	require.NoError(t, ib.NewOTLPBridge(st, mp.Meter("ib_exporter_test")))
	return reader
}

func sorted(ss []series) []series {
	slices.SortFunc(ss, func(a, b series) int {
		return strings.Compare(a.name+"|"+a.labels, b.name+"|"+b.labels)
	})
	return ss
}
