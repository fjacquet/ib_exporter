# ib_exporter

Prometheus and OTLP exporter for **InfiniBand** fabrics. One process sees the whole fabric from a
management node (switches, HCAs, topology, subnet manager) through the `infiniband-diags` tools,
and the local HCA through sysfs. Several fabrics per process, each labelled `fabric`.

!!! warning "Not yet validated on real InfiniBand hardware"
    Development and CI run on hand-written fixtures. The first real run must use
    `ib_exporter --once --debug --trace` and the output must be diffed against [Metrics](metrics.md).
    Open items: exact `perfquery -T` field names, `-T` with `-a -l`, whether `-x` carries the basic
    error counters (both are read and merged), the `ibswinfo` output format and packaging, the umad
    group on Debian trixie, `ibsim` coverage of PMA/`-T`.

- **Snapshot model**: a background loop polls the fabric; scrapes read the last snapshot ([architecture](architecture.md)).
- **Dual export**: Prometheus `/metrics` and OTLP/HTTP (set `OTEL_EXPORTER_OTLP_ENDPOINT`).
- **Counters** are cumulative `_total` series; use `rate()`. The exporter never resets fabric counters.
- Probes: `/livez`, `/readyz` (static 200), `/health` (informational 200). `ib_up{fabric}` reports a down fabric.

Start with the [Docker demo](deployment/docker.md) (replay mode, no hardware), then
[Configuration](configuration.md), [Metrics](metrics.md) and [Dashboards](dashboards.md).
Design decisions are in the [ADRs](adr/README.md).
