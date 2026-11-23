package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fjacquet/ib_exporter/internal/ib"
)

func clearEnv(t *testing.T) {
	t.Helper()
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", "")
}

func TestSetup_EnvUnsetIsNoop(t *testing.T) {
	clearEnv(t)
	shutdown, err := Setup(context.Background(), &ib.SnapshotStore{})
	require.NoError(t, err)
	require.NoError(t, shutdown(context.Background()))
}

func TestSetup_PostsToOTLPEndpoint(t *testing.T) {
	for _, env := range []string{"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_METRICS_ENDPOINT"} {
		t.Run(env, func(t *testing.T) {
			clearEnv(t)
			var posts atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					posts.Add(1)
				}
			}))
			defer srv.Close()
			url := srv.URL
			if env == "OTEL_EXPORTER_OTLP_METRICS_ENDPOINT" {
				url += "/v1/metrics"
			}
			t.Setenv(env, url)

			shutdown, err := Setup(context.Background(), &ib.SnapshotStore{})
			require.NoError(t, err)
			require.NoError(t, shutdown(context.Background())) // flushes one export
			require.Positive(t, posts.Load())
		})
	}
}
