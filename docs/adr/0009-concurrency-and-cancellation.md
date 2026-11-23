# 0009. Concurrency and cancellation

- **Status:** accepted
- **Date:** 2026-11-23

## Context

PMA MADs load switch management CPUs, so the exporter must bound how many run at once, however many fabrics and nodes exist. Shutdown and config reload must not publish half-collected data.

## Decision

- `limitRunner` (`internal/collect/loop.go`) is a global semaphore of size `collection.maxConcurrent` around every exec, across all fabrics and readers. Fabrics run concurrently without `errgroup.WithContext`, so one failing fabric does not cancel the others.
- Per fabric, `parallel` (`internal/collect/fabric.go`) runs a per-node worker pool with the same limit; workers fill their own stats and the fabric goroutine merges them, so no state is shared.
- A cancelled cycle returns `ctx.Err()` before swapping: killed execs would look like a down fabric, so the last good snapshot stays.
- A config reload restarts the loop through a supervisor in `main.go` (cancel, wait, start on the same store); two loops never overlap.

## Consequences

- `ib_collect_duration_seconds` includes waits for a free exec slot and can exceed the interval.
- With many nodes and a low `maxConcurrent`, a cycle can outlast `interval`; raise the interval or the bound.
