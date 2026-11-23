# Architecture

```
collection loop (internal/collect) -> immutable ib.Snapshot -> ib.SnapshotStore
                                        |-> Prometheus unchecked collector (internal/ib/prometheus.go)
                                        '-> OTLP observable instruments (internal/ib/otlp.go)
```

Scrapes never touch the fabric: they read the current snapshot. HTTP is served before the first cycle.

## Readers

| Reader | Source | Cadence |
|---|---|---|
| `topology` | `ibnetdiscover -p` | `topologyInterval` |
| `perfquery` | `perfquery -a -l <lid>` and `-x -a -l <lid>`, merged | `interval` |
| `perfquery_optional` | `-T`, `-E`, `-D`, `--rcvcc`, `--xmitcc` | every `optionalEvery`-th cycle |
| `portinfo` | `smpquery portinfo` | `portInfoInterval` |
| `sm` | `sminfo`, `saquery SMIR` | `portInfoInterval` |
| `ibswinfo` | `ibswinfo -d lid-<LID>` (optional) | `portInfoInterval` |
| `sysfs` | `/sys/class/infiniband` (local HCA) | `interval` |
| replay | fixture files, same `diags.Runner` interface | any |

## Identity

Series key = `fabric` + `guid` + `port`, never LID. `ibnetdiscover -p` carries port GUIDs; for HCAs
`guid` is the port GUID. Each cycle resolves GUID to LID from the latest topology. Local sysfs series
carry the same port GUID, so fabric and local series join. See [ADR 0005](adr/0005-identity.md).

## Snapshot

Per fabric: `Up`, nodes, links, samples. `ib_up{fabric}` is 0 when the topology could not be read;
the last good topology is kept and retried. A failing node yields absent samples and increments
`ib_collect_errors_total`; the exporter never emits a fake zero. Slow readers (optional, portinfo, SM)
cache their samples between runs.

## Concurrency and cancellation

`collection.maxConcurrent` bounds in-flight execs globally (a semaphore around the runner), and the
per-node worker pool uses the same limit. A cancelled cycle (shutdown, reload) never swaps a snapshot:
the last good one stays. Reload restarts the loop on the same store. See [ADR 0009](adr/0009-concurrency-and-cancellation.md).

## Export rules

A metric name carries one label-key set and one kind (counter or gauge); a sample that disagrees with
the first of its name is dropped from both outputs ([ADR 0006](adr/0006-label-union.md)).
