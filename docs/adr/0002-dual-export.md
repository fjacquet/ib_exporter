# 0002. Dual export: Prometheus unchecked collector and OTLP observables

- **Status:** accepted
- **Date:** 2026-11-23

## Context

The family exports to Prometheus and OTLP from one source of truth. Metric names here depend on what the fabric reports (for example per-lane FEC, `hw_counters/` files), so they cannot be declared up front.

## Decision

- Prometheus: an unchecked collector (`internal/ib/prometheus.go`; `Describe` emits nothing) turns each snapshot sample into a const metric, counters as `CounterValue`.
- OTLP (`internal/ib/otlp.go`, wired in `internal/telemetry`): one observable instrument per name, a counter or a gauge by kind, enabled by `OTEL_EXPORTER_OTLP_ENDPOINT`. OTel instruments must exist before a collection, and the SDK holds its pipeline lock while running callbacks, so a new name cannot be registered from inside one. The bridge pre-registers instruments for the names in the store at construction (plus `alwaysPresent`: `ib_up`, `ib_collect_errors_total`, so an empty store still registers a callback) and registers late names from a goroutine after a collection.
- Both outputs share the export rules in `internal/ib/metrics.go` (valid names, one label-key set and kind per name, no duplicate series).

## Consequences

- A name first seen in a snapshot reaches OTLP one collection later (one-collection lag). Prometheus has no lag.
- Instruments are never removed; an absent name observes nothing.
- Tests assert both outputs (Prometheus gather and OTLP `ManualReader`).
