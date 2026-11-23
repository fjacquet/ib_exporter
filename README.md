# ib_exporter

[![CI](https://github.com/fjacquet/ib_exporter/actions/workflows/ci.yml/badge.svg)](https://github.com/fjacquet/ib_exporter/actions/workflows/ci.yml)
[![Security](https://github.com/fjacquet/ib_exporter/actions/workflows/security.yml/badge.svg)](https://github.com/fjacquet/ib_exporter/actions/workflows/security.yml)
[![Release](https://github.com/fjacquet/ib_exporter/actions/workflows/release.yml/badge.svg)](https://github.com/fjacquet/ib_exporter/actions/workflows/release.yml)
[![Docs](https://github.com/fjacquet/ib_exporter/actions/workflows/docs.yml/badge.svg)](https://fjacquet.github.io/ib_exporter/)
[![Go Report Card](https://goreportcard.com/badge/github.com/fjacquet/ib_exporter)](https://goreportcard.com/report/github.com/fjacquet/ib_exporter)
[![Go Version](https://img.shields.io/github/go-mod/go-version/fjacquet/ib_exporter)](go.mod)
[![Latest Release](https://img.shields.io/github/v/release/fjacquet/ib_exporter?include_prereleases&sort=semver)](https://github.com/fjacquet/ib_exporter/releases)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![codecov](https://codecov.io/gh/fjacquet/ib_exporter/branch/main/graph/badge.svg)](https://codecov.io/gh/fjacquet/ib_exporter)

Prometheus and OTLP exporter for **InfiniBand** fabrics. One process sees every switch and HCA port,
the topology and the subnet manager through `infiniband-diags`, plus the local HCA through sysfs,
across several fabrics (`fabric` label).

> **Status: not yet validated on real InfiniBand hardware.** Development and CI use hand-written
> fixtures. Unverified: exact `perfquery -T` field names, `-T` with `-a -l`, whether `-x` carries the
> basic error counters (both are read and merged), the `ibswinfo` output format and packaging, the
> umad group on Debian trixie, `ibsim` coverage of PMA/`-T`. First real run: `ib_exporter --once --debug --trace`,
> diffed against [docs/metrics.md](docs/metrics.md).

## Quick start (no hardware)

```bash
docker compose up -d --build   # exporter (replay fixtures) + Prometheus + Grafana
curl -s 127.0.0.1:9449/metrics | grep -c '^ib_'
```

Published image: `docker compose -f docker-compose.ghcr.yml up -d`. Real hardware needs `/dev/infiniband`
and the umad gid, see [docs/deployment/docker.md](docs/deployment/docker.md).

## What it exports

Per-port data, packets, errors, discards, congestion (FECN/BECN, `PortXmitWait`) and per-lane FEC
counters (all cumulative `_total`, use `rate()`), port state/width/speed/MTU, topology (`ib_node_info`,
`ib_link_info`), SM info and state, local HCA counters and `hw_counters/`, optional switch health,
and collector health (`ib_up{fabric}`, durations, errors). Series are keyed by fabric + GUID + port, never LID.
Fabric counters are never reset. Full list: [docs/metrics.md](docs/metrics.md).

Probes: `/livez`, `/readyz`, `/health`. OTLP: set `OTEL_EXPORTER_OTLP_ENDPOINT`. Config: [docs/configuration.md](docs/configuration.md).
Five Grafana dashboards and alert rules are in `grafana/` and `deploy/prometheus/`. Design: [docs/adr/](docs/adr/README.md).

## Development

`make tools`, then `make ci`, `make check-deploy`, `make release-snapshot`. See [CLAUDE.md](CLAUDE.md).

## License

Apache-2.0.
