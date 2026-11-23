package ib

import "github.com/prometheus/client_golang/prometheus"

// NewBuildInfoCollector exposes `ib_exporter_build_info{version,goversion} 1`, so a
// scrape shows which build is running. version comes from the -X main.version
// ldflag; goversion is runtime.Version(), passed in for testability. No fabric
// label: it describes the process, not a fabric.
func NewBuildInfoCollector(version, goversion string) prometheus.Collector {
	g := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   "ib_exporter",
		Name:        "build_info",
		Help:        "Exporter build information; constant 1, with the running version and Go version in the `version` and `goversion` labels.",
		ConstLabels: prometheus.Labels{"version": version, "goversion": goversion},
	})
	g.Set(1)
	return g
}
