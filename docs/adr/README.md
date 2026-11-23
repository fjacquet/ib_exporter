# ADRs

Decisions are immutable once accepted; supersede rather than edit (the amendment in 0005 is the one dated exception).

| ADR | Decision |
|---|---|
| [0001](0001-snapshot-model.md) | Snapshot model |
| [0002](0002-dual-export.md) | Dual export: Prometheus unchecked collector and OTLP observables |
| [0003](0003-client-choice.md) | Exec infiniband-diags and sysfs, no SDK or REST |
| [0004](0004-counters-as-total.md) | Counters as `_total` with `rate()`; no counter reset |
| [0005](0005-identity.md) | Identity is fabric + GUID + port, never LID |
| [0006](0006-label-union.md) | One label-key set per metric name; `fabric` label |
| [0007](0007-debian-base-image.md) | Debian trixie-slim, not Alpine |
| [0008](0008-umad-device-access.md) | UMAD device access in containers |
| [0009](0009-concurrency-and-cancellation.md) | Concurrency bound and cancellation |
