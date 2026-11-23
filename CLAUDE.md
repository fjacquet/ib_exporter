# CLAUDE.md

Guidance for working in `ib_exporter`.

## Overview
Prometheus + OTLP exporter for InfiniBand fabrics (family standard: `exporter-standards`). Metric prefix `ib_`, port 9449. Fabric data comes from the `infiniband-diags` CLIs, the local HCA from sysfs. **Not validated on real hardware.**

## Commands
- `make ci` lint + race tests + build + govulncheck (the CI gate); `make check-deploy` validates alert rules, Prometheus config, dashboards and metric names (needs `promtool`).
- `make cli` builds `bin/ib_exporter`; run `./bin/ib_exporter --config config.yaml [--once] [--debug] [--trace]`.
- `make release-snapshot` goreleaser dry run (linux/darwin x amd64/arm64, SBOM, checksums, image); `make tools` installs pinned tooling.
- `make docs` builds the MkDocs site with `--strict`. Demo: `docker compose up -d --build` (replay mode, no hardware).

## Architecture
Collection loop (`internal/collect`) -> immutable `ib.Snapshot` -> `ib.SnapshotStore` -> unchecked Prometheus collector + OTLP observables (`internal/ib`). Readers: `internal/ib/diags` (exec + parsers, `Runner`), `internal/ib/sysfs`, `internal/ib/replay` (fixtures behind the same `Runner`). `main.go` wires server, loop supervisor, reload, `--once`. Details: `docs/architecture.md`, `docs/adr/`.

## Load-bearing constraints
- Counters are cumulative `_total` series, use `rate()` (deliberate deviation from the family gauge rule, ADR 0004). Never reset fabric counters (`-r`, `-R`). Never emit a fake zero: unreadable = absent sample.
- Identity = `fabric` + `guid` + `port`, never LID. `ibnetdiscover -p` only gives port GUIDs; HCA `guid` is the port GUID, switches share the node GUID (ADR 0005).
- One label-key set and one kind per metric name; empty string where not applicable; every series has `fabric` (ADR 0006). A test enforces it.
- Both `perfquery` (errors) and `perfquery -x` (64-bit data/packets) are read and merged. Data counters are units of 4 octets, exported x4.
- Width, speed and rate are emitted only when the port is Active.
- `maxConcurrent` bounds execs globally; a cancelled cycle never swaps a snapshot; reload restarts the loop (ADR 0009).
- Base image is `debian:trixie-slim`, not Alpine (glibc tools), uid 10001 (ADR 0007). UMAD access via `--device` + `group_add: UMAD_GID` (ADR 0008).
- `ibswinfo` is optional, off by default, unverified. `//go:build linux` readers need darwin stubs so macOS builds compile.
- OTLP: a metric name first seen in a snapshot is exported from the next collection (ADR 0002).
- Update `docs/metrics.md` in the same change as any metric.

## Testing
No hardware: fixtures are hand-written (rdma-core man pages and source), not captured. The first real-hardware run must use `--once --debug --trace` and be diffed against `docs/metrics.md`; every [unverified] item stays open until then. Tests assert Prometheus gather and OTLP `ManualReader`, plus the label-union test and probe tests on the real mux.

## CI/CD
Thin callers of `fjacquet/ci@v1` in `.github/workflows/{ci,security,release,docs}.yml`; never inline steps or pin actions here. Secrets: `CODECOV_TOKEN` (ci), `HOMEBREW_TAP_GITHUB_TOKEN` (release, optional; the cask self-skips). Release: tag `v*` runs GoReleaser (`.goreleaser.yaml`, `Dockerfile.goreleaser`, image `ghcr.io/fjacquet/ib_exporter`). Dependabot: `gomod` and `docker` only.
