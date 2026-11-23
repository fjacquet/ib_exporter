# 0001. Snapshot model

- **Status:** accepted
- **Date:** 2026-11-23

## Context

Reading a fabric means running `perfquery` and friends against every node: slow, and PMA MADs load switch management CPUs. Scrape frequency must not drive that load, and a slow fabric must not make `/metrics` slow.

## Decision

A collection loop (`internal/collect/loop.go`) builds an immutable `ib.Snapshot` each cycle and swaps it into an `ib.SnapshotStore` (`internal/ib/store.go`, RWMutex pointer swap). Scrapes and OTLP read the current snapshot only. The HTTP server starts before the first cycle. A cycle swaps even when sources failed (`ib_up{fabric}` is data); a cancelled cycle does not ([ADR 0009](0009-concurrency-and-cancellation.md)).

## Consequences

- Fabric load is set by `collection.*` intervals, not by scrape count.
- Values are as old as the last cycle (up to `collection.interval`); `ib_collect_last_success_timestamp_seconds` shows freshness.
- Snapshots must never be mutated after `Swap`; slow-reader caches are copied from, never shared.
