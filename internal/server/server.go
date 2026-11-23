// Package server wires the exporter's HTTP endpoints.
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/fjacquet/ib_exporter/internal/config"
	"github.com/fjacquet/ib_exporter/internal/ib"
)

// RegisterEndpoints registers the probes, the index and the metrics path.
func RegisterEndpoints(mux *http.ServeMux, cfg *config.Config, store *ib.SnapshotStore, reg *prometheus.Registry) {
	registerEndpoints(mux, cfg, store, reg, config.ReservedURIs)
}

// registerEndpoints registers each reserved route and panics when the routes and
// the reserved set differ, so a new route cannot be added without reserving it.
func registerEndpoints(mux *http.ServeMux, cfg *config.Config, store *ib.SnapshotStore, reg *prometheus.Registry, reserved map[string]bool) {
	metricsURI := cfg.Server.URI
	routes := map[string]http.Handler{
		"/livez":  http.HandlerFunc(staticOKHandler),
		"/readyz": http.HandlerFunc(staticOKHandler),
		"/health": healthHandler(store),
		"/":       indexHandler(metricsURI),
	}
	if len(routes) != len(reserved) {
		panic(fmt.Sprintf("server: %d routes but %d reserved URIs", len(routes), len(reserved)))
	}
	for uri := range reserved {
		if routes[uri] == nil {
			panic(fmt.Sprintf("server: reserved URI %q has no handler", uri))
		}
	}
	for uri, h := range routes {
		mux.Handle(uri, h)
	}
	mux.Handle(metricsURI, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
}

// staticOKHandler answers /livez and /readyz: no state, never a reason to restart.
func staticOKHandler(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

type fabricHealth struct {
	Fabric     string `json:"fabric"`
	OK         bool   `json:"ok"`
	LastScrape string `json:"last_scrape"`
	Err        string `json:"err"`
}

// healthHandler is informational and always 200; a down fabric is data, not a failure.
func healthHandler(store *ib.SnapshotStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		snap := store.Load()
		fabrics := make([]fabricHealth, 0, len(snap.Fabrics)+1)
		for _, f := range snap.Fabrics {
			h := fabricHealth{Fabric: f.Fabric, OK: f.Up, LastScrape: f.At.UTC().Format(time.RFC3339), Err: errText(f.LastErr)}
			if !f.Up && h.Err == "" {
				h.Err = "fabric down: see ib_up and ib_collect_errors_total"
			}
			fabrics = append(fabrics, h)
		}
		if snap.HCA != nil { // sources.localHCA is on
			fabrics = append(fabrics, fabricHealth{Fabric: "(local hca)", OK: snap.HCAOK,
				LastScrape: snap.BuiltAt.UTC().Format(time.RFC3339), Err: errText(snap.HCAErr)})
		}
		sort.Slice(fabrics, func(i, j int) bool { return fabrics[i].Fabric < fabrics[j].Fabric })
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"built_at": snap.BuiltAt.UTC().Format(time.RFC3339),
			"fabrics":  fabrics,
		})
	})
}

// errText keeps the first line of a reader error, at most 300 runes.
func errText(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	if r := []rune(s); len(r) > 300 {
		s = string(r[:300])
	}
	return s
}

// indexHandler serves the landing page at "/" only; any other path is a 404.
func indexHandler(metricsURI string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, "<html><head><title>ib_exporter</title></head><body><h1>ib_exporter</h1><p><a href=%q>Metrics</a></p></body></html>\n", metricsURI)
	})
}
