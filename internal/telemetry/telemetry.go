// Package telemetry wires the optional OTLP metric export.
package telemetry

import (
	"context"
	"os"

	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"

	"github.com/fjacquet/ib_exporter/internal/ib"
)

// Setup starts OTLP/HTTP export of the store when OTEL_EXPORTER_OTLP_ENDPOINT or
// OTEL_EXPORTER_OTLP_METRICS_ENDPOINT is set; otherwise it does nothing. The
// exporter itself reads the standard OTEL_* variables (endpoint, headers, interval).
func Setup(ctx context.Context, store *ib.SnapshotStore) (shutdown func(context.Context) error, err error) {
	noop := func(context.Context) error { return nil }
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" && os.Getenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT") == "" {
		return noop, nil
	}
	exp, err := otlpmetrichttp.New(ctx)
	if err != nil {
		return noop, err
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exp)),
		sdkmetric.WithResource(resource.NewSchemaless(semconv.ServiceName("ib_exporter"))),
	)
	if err := ib.NewOTLPBridge(store, mp.Meter("ib_exporter")); err != nil {
		_ = mp.Shutdown(ctx)
		return noop, err
	}
	return mp.Shutdown, nil
}
