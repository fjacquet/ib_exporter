package ib

import "github.com/prometheus/client_golang/prometheus"

// snapshotCollector is an unchecked collector: Describe sends nothing, so the set
// of metric names can change between snapshots.
type snapshotCollector struct{ store *SnapshotStore }

// NewSnapshotCollector exports the store's current snapshot on every scrape.
func NewSnapshotCollector(store *SnapshotStore) prometheus.Collector {
	return snapshotCollector{store: store}
}

func (snapshotCollector) Describe(chan<- *prometheus.Desc) {}

func (c snapshotCollector) Collect(ch chan<- prometheus.Metric) {
	descs := map[string]*prometheus.Desc{}
	for _, s := range exportable(c.store.Load()) {
		keys, vals := splitLabels(s.Labels)
		d, ok := descs[s.Name]
		if !ok {
			d = prometheus.NewDesc(s.Name, helpFor(s.Name), keys, nil)
			descs[s.Name] = d
		}
		t := prometheus.GaugeValue
		if s.Counter {
			t = prometheus.CounterValue
		}
		// exportable already rejected what NewConstMetric would; never panic on a scrape.
		if m, err := prometheus.NewConstMetric(d, t, s.Value, vals...); err == nil {
			ch <- m
		}
	}
}
