# 0009. Concurrency and cancellation

- **Status:** accepted
- **Date:** 2026-11-23

## Context

PMA MADs load switch management CPUs, so the exporter must bound how many run at once, however many fabrics and nodes exist. Shutdown and config reload must not publish half-collected data.

## Decision

- `limitRunner` (`internal/collect/loop.go`) is a global semaphore of size `collection.maxConcurrent` around every exec, across all fabrics and readers. Fabrics run concurrently without `errgroup.WithContext`, so one failing fabric does not cancel the others.
- Per fabric, `parallel` (`internal/collect/fabric.go`) runs a per-node worker pool with the same limit; workers fill their own stats and the fabric goroutine merges them, so no state is shared.
- A cancelled cycle returns `ctx.Err()` before swapping: killed execs would look like a down fabric, so the last good snapshot stays.
- Each exec has a deadline, `collection.commandTimeout` (`diags.ExecRunner.Timeout`). A timeout is a reader error, counted in `ib_collect_errors_total`; a cancellation (shutdown, reload) is not counted. `cmd.WaitDelay` (2s) bounds the wait for a killed script whose child still holds stdout. There is no deadline on the whole cycle: a cycle cancelled by its own deadline would never swap, so a slow but healthy large fabric would never publish.
- A config reload restarts the loop through a supervisor in `main.go` (cancel, wait, start on the same store). The wait is bounded (15s, then a warning): loops do not overlap unless the old one is stuck, and a stuck one only finishes cancelled cycles, which never swap.

## Consequences

- `ib_collect_duration_seconds` includes waits for a free exec slot and can exceed the interval.
- With many nodes and a low `maxConcurrent`, a cycle can outlast `interval`; raise the interval or the bound.
- A hung command holds an exec slot for at most `commandTimeout` plus `WaitDelay`.
- Worst case, shutdown waits 15s for the loop and then up to 10s for HTTP and OTLP, longer than Docker's default 10s stop grace period; in practice the loop stops within `WaitDelay`.
