# 0006. One label-key set per metric name, and a `fabric` label everywhere

- **Status:** accepted
- **Date:** 2026-11-23

## Context

The Prometheus collector is unchecked ([ADR 0002](0002-dual-export.md)), so `client_golang` does not enforce one label-key set per name; a mismatch would fail a whole scrape. OTLP observables have the same constraint per instrument.

## Decision

- A name carries one label-key set across all series: an empty string where a label does not apply (for example `lane=""` for port-level FEC). A test over fixtures enforces it.
- At runtime `internal/ib/metrics.go` drops any sample whose label keys or counter/gauge kind differ from the first sample of that name, in both outputs.
- Every series carries `fabric`, so several fabrics share one process. Exceptions: `ib_exporter_build_info`; local HCA ports not bound in config carry `fabric=""`.

## Consequences

- A divergent sample is silently dropped at runtime; the test is the real gate.
- Adding a label to an existing name means adding it to every producer of that name.
