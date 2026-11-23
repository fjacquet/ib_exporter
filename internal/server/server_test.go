package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fjacquet/ib_exporter/internal/config"
	"github.com/fjacquet/ib_exporter/internal/ib"
)

func newMux(t *testing.T, store *ib.SnapshotStore) *http.ServeMux {
	t.Helper()
	reg := prometheus.NewRegistry()
	reg.MustRegister(ib.NewSnapshotCollector(store))
	mux := http.NewServeMux()
	RegisterEndpoints(mux, &config.Config{Server: config.Server{URI: "/metrics"}}, store, reg)
	return mux
}

func get(mux *http.ServeMux, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func downStore() *ib.SnapshotStore {
	st := &ib.SnapshotStore{}
	st.Swap(&ib.Snapshot{BuiltAt: time.Unix(100, 0).UTC(), Fabrics: map[string]*ib.FabricSnapshot{
		"prod": {Fabric: "prod", Up: false, At: time.Unix(90, 0).UTC(),
			Samples: []ib.Sample{{Name: "ib_up", Labels: []ib.Label{{Key: "fabric", Value: "prod"}}, Value: 0}}},
	}})
	return st
}

func TestLivezReadyz_AlwaysOK(t *testing.T) {
	for name, st := range map[string]*ib.SnapshotStore{"empty": {}, "all down": downStore()} {
		mux := newMux(t, st)
		for _, p := range []string{"/livez", "/readyz"} {
			rec := get(mux, p)
			assert.Equal(t, http.StatusOK, rec.Code, "%s %s", name, p)
			// the "/" catch-all serves an html index; a real probe handler has an empty body
			assert.Empty(t, rec.Body.String(), "%s %s must hit its own handler, not the index", name, p)
		}
	}
}

func TestHealth_200EvenWhenFabricDown_BodyListsFabrics(t *testing.T) {
	rec := get(newMux(t, downStore()), "/health")
	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		BuiltAt string `json:"built_at"`
		Fabrics []struct {
			Fabric     string `json:"fabric"`
			OK         bool   `json:"ok"`
			LastScrape string `json:"last_scrape"`
			Err        string `json:"err"`
		} `json:"fabrics"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.NotEmpty(t, body.BuiltAt)
	require.Len(t, body.Fabrics, 1)
	assert.Equal(t, "prod", body.Fabrics[0].Fabric)
	assert.False(t, body.Fabrics[0].OK)
	assert.NotEmpty(t, body.Fabrics[0].LastScrape)
}

func TestHealth_EmptyStore(t *testing.T) {
	rec := get(newMux(t, &ib.SnapshotStore{}), "/health")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"fabrics":[]`)
}

func TestMetricsEndpoint_ServesIbSamples(t *testing.T) {
	rec := get(newMux(t, downStore()), "/metrics")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `ib_up{fabric="prod"} 0`)
}

func TestIndex_LinksMetricsPath(t *testing.T) {
	assert.Contains(t, get(newMux(t, &ib.SnapshotStore{}), "/").Body.String(), `href="/metrics"`)
	assert.Equal(t, http.StatusNotFound, get(newMux(t, &ib.SnapshotStore{}), "/nope").Code)
}

func TestRegisterEndpoints_ReservedMapDrift(t *testing.T) {
	mux := http.NewServeMux
	st := &ib.SnapshotStore{}
	cfg := &config.Config{Server: config.Server{URI: "/metrics"}}
	reg := prometheus.NewRegistry()

	// reserved entry without a registration
	assert.Panics(t, func() {
		registerEndpoints(mux(), cfg, st, reg, map[string]bool{"/": true, "/health": true, "/livez": true, "/readyz": true, "/extra": true})
	})
	// registration not reserved
	assert.Panics(t, func() {
		registerEndpoints(mux(), cfg, st, reg, map[string]bool{"/": true, "/health": true, "/livez": true})
	})
	// same length, different names
	assert.Panics(t, func() {
		registerEndpoints(mux(), cfg, st, reg, map[string]bool{"/": true, "/health": true, "/livez": true, "/other": true})
	})
	assert.NotPanics(t, func() { registerEndpoints(mux(), cfg, st, reg, config.ReservedURIs) })
}
