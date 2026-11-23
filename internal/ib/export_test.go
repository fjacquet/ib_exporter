package ib_test

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"

	"github.com/fjacquet/ib_exporter/internal/ib"
)

func TestMetrics_LabelKeySetUniquePerName(t *testing.T) {
	ss := rawSamples(fullStore(t).Load())
	paths := map[string]bool{}
	keySets := map[string][]string{}
	seen := map[string]bool{}
	for _, s := range ss {
		var keys, vals []string
		for _, l := range s.Labels {
			keys, vals = append(keys, l.Key), append(vals, l.Value)
			if l.Key == "node_type" && strings.HasPrefix(s.Name, "ib_port_") {
				paths[l.Value] = true
			}
		}
		if strings.HasPrefix(s.Name, "ib_hca_") {
			paths["local_hca"] = true
		}
		if want, ok := keySets[s.Name]; ok {
			require.Equal(t, want, keys, "label keys of %s", s.Name)
		} else {
			keySets[s.Name] = keys
		}
		id := s.Name + "{" + strings.Join(vals, "\x00") + "}"
		require.False(t, seen[id], "duplicate series %s %v", s.Name, s.Labels)
		seen[id] = true
	}
	for _, p := range []string{"switch", "hca", "local_hca"} {
		require.True(t, paths[p], "fixture snapshot does not cover the %s path", p)
	}
}

func TestPrometheus_AllSeriesCarryFabric(t *testing.T) {
	st := fullStore(t)
	mfs, ss := gatherProm(t, ib.NewSnapshotCollector(st))
	require.Len(t, ss, len(rawSamples(st.Load())), "every valid fixture sample is exported")
	fabrics := map[string]bool{}
	for _, mf := range mfs {
		for _, m := range mf.GetMetric() {
			has := false
			for _, lp := range m.GetLabel() {
				if lp.GetName() == ib.LabelFabric {
					has = true
					fabrics[lp.GetValue()] = true
				}
			}
			require.True(t, has, "%s has no fabric label", mf.GetName())
		}
	}
	require.True(t, fabrics["prod"] && fabrics[""], "bound and unbound fabrics: %v", fabrics)
}

func TestPrometheus_SkipsInvalidSamples(t *testing.T) {
	fab := func(v string, more ...ib.Label) []ib.Label {
		return append([]ib.Label{{Key: ib.LabelFabric, Value: v}}, more...)
	}
	st := &ib.SnapshotStore{}
	st.Swap(&ib.Snapshot{Fabrics: map[string]*ib.FabricSnapshot{"a": {Fabric: "a", Samples: []ib.Sample{
		{Name: "ib_x", Labels: fab("a"), Value: 1},
		{Name: "ib_x", Labels: fab("b", ib.Label{Key: "extra", Value: "z"}), Value: 2}, // other key set
		{Name: "ib_x", Labels: fab("a"), Value: 3},                                     // duplicate series
		{Name: "ib_x", Labels: fab("\xff"), Value: 4},                                  // invalid UTF-8
		{Name: "ib-bad name", Labels: fab("a"), Value: 5},                              // invalid name
		{Name: "ib_y_total", Labels: fab("a"), Value: 6, Counter: true},
		{Name: "ib_y_total", Labels: fab("b"), Value: 7}, // counter then gauge
		{Name: "ib_z", Labels: []ib.Label{{Key: ib.LabelFabric, Value: "a"}, {Key: ib.LabelFabric, Value: "b"}}, Value: 8},
	}}}})
	_, got := gatherProm(t, ib.NewSnapshotCollector(st))
	want := []series{
		{name: "ib_x", labels: "fabric=a", value: 1},
		{name: "ib_y_total", labels: "fabric=a", value: 6, counter: true},
	}
	require.Equal(t, want, sorted(got))
	require.Equal(t, want, sorted(collectOTLP(t, newOTLP(t, st))), "OTLP skips the same samples")
}

func TestPrometheus_HelpAndType(t *testing.T) {
	mfs, _ := gatherProm(t, ib.NewSnapshotCollector(fullStore(t)))
	for _, mf := range mfs {
		require.NotEmpty(t, mf.GetHelp(), mf.GetName())
		if strings.HasSuffix(mf.GetName(), "_total") {
			require.Equal(t, dto.MetricType_COUNTER, mf.GetType(), mf.GetName())
		}
	}
}

func TestOTLP_ManualReaderMatchesPrometheus(t *testing.T) {
	st := fullStore(t)
	_, prom := gatherProm(t, ib.NewSnapshotCollector(st))
	require.NotEmpty(t, prom)
	require.Equal(t, sorted(prom), sorted(collectOTLP(t, newOTLP(t, st))))
}

// The bridge is built before the first collection cycle, on an empty store.
func TestOTLP_BuiltOnEmptyStoreFollowsLaterSnapshots(t *testing.T) {
	st := &ib.SnapshotStore{}
	reader := newOTLP(t, st)
	require.Empty(t, collectOTLP(t, reader))
	st.Swap(fullStore(t).Load())
	_, prom := gatherProm(t, ib.NewSnapshotCollector(st))
	require.Eventually(t, func() bool { return slices.Equal(sorted(prom), sorted(collectOTLP(t, reader))) },
		5*time.Second, 10*time.Millisecond)
}

func TestOTLP_FollowsSnapshotAndLateNames(t *testing.T) {
	st := &ib.SnapshotStore{}
	up := func(v float64, extra ...ib.Sample) {
		st.Swap(&ib.Snapshot{Fabrics: map[string]*ib.FabricSnapshot{"a": {Fabric: "a",
			Samples: append([]ib.Sample{{Name: "ib_up", Labels: []ib.Label{{Key: ib.LabelFabric, Value: "a"}}, Value: v}}, extra...)}}})
	}
	up(1)
	reader := newOTLP(t, st)
	require.Equal(t, []series{{name: "ib_up", labels: "fabric=a", value: 1}}, collectOTLP(t, reader))

	late := ib.Sample{Name: "ib_late_total", Labels: []ib.Label{{Key: ib.LabelFabric, Value: "a"}}, Value: 9, Counter: true}
	up(0, late)
	// The collection that first sees a new name registers it asynchronously; it is
	// observed from a following collection on.
	require.Contains(t, collectOTLP(t, reader), series{name: "ib_up", labels: "fabric=a", value: 0})
	want := []series{{name: "ib_late_total", labels: "fabric=a", value: 9, counter: true}, {name: "ib_up", labels: "fabric=a"}}
	require.Eventually(t, func() bool { return slices.Equal(want, sorted(collectOTLP(t, reader))) },
		5*time.Second, 10*time.Millisecond)
}

func TestBuildInfo_Labels(t *testing.T) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(ib.NewBuildInfoCollector("v1.2.3", "go1.99"))
	const want = `
# HELP ib_exporter_build_info Exporter build information; constant 1, with the running version and Go version in the ` + "`version`" + ` and ` + "`goversion`" + ` labels.
# TYPE ib_exporter_build_info gauge
ib_exporter_build_info{goversion="go1.99",version="v1.2.3"} 1
`
	require.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(want), "ib_exporter_build_info"))
}

func TestUp_PerFabric(t *testing.T) {
	st := fixtureStore(t, []string{"prod", "lab"}, "prod")
	_, ss := gatherProm(t, ib.NewSnapshotCollector(st))
	up := map[string]float64{}
	for _, s := range ss {
		if s.name == "ib_up" {
			up[s.labels] = s.value
		}
	}
	require.Equal(t, map[string]float64{"fabric=prod": 1, "fabric=lab": 0}, up)
}

func TestMetricsDocCoversEverySampleName(t *testing.T) {
	doc, err := os.ReadFile("../../docs/metrics.md")
	require.NoError(t, err)
	names := map[string]bool{"ib_exporter_build_info": true}
	for _, s := range rawSamples(fullStore(t).Load()) {
		names[s.Name] = true
	}
	var missing []string
	for n := range names {
		if !regexp.MustCompile("`" + regexp.QuoteMeta(n) + "[`{]").Match(doc) {
			missing = append(missing, n)
		}
	}
	slices.Sort(missing)
	require.Empty(t, missing, "names missing from docs/metrics.md")
}
