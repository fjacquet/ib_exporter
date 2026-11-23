# 0004. Counters exposed as `_total` cumulative counters; no counter reset

- **Status:** accepted
- **Date:** 2026-11-23

## Context

The family rule is to export gauges and never use `rate()`, for sources that report ready-made rates. InfiniBand PMA counters are cumulative hardware counters that wrap or are reset only if someone resets them (`perfquery -r`, `ibqueryerrors -R`).

## Decision

- Samples have `Counter: true` (`internal/ib/model.go`) and are exposed as Prometheus counters and OTLP monotonic sums named `*_total`; dashboards and alerts use `rate()` / `increase()`. This is a deliberate deviation from the family gauge rule.
- The exporter never resets fabric counters (no `-r`, `-R`), because other tools and operators read the same counters.
- Data counters are units of 4 octets and are multiplied by 4 into `_bytes_total`.

## Consequences

- Counter resets caused by switch reboots show as normal Prometheus counter resets.
- An unreadable value is an absent sample, never zero.
- The demo replay fixtures are static, so `rate()` reads 0 there.
