# 0007. Base image: Debian trixie-slim, not Alpine

- **Status:** accepted
- **Date:** 2026-11-23

## Context

The family standard is Alpine with uid 10001. `infiniband-diags` (`perfquery`, `ibnetdiscover`, `smpquery`, `saquery`, `sminfo`) are glibc binaries and libibumad/libibmad come from Debian packages.

## Decision

`docker.io/library/debian:trixie-slim` with `infiniband-diags`, `curl` (for `HEALTHCHECK` on `127.0.0.1:9449/livez`) and `ca-certificates`, in `Dockerfile` and `Dockerfile.goreleaser`. Non-root user `ib`, uid/gid 10001, as the rest of the family. The exporter binary is static (`CGO_ENABLED=0`).

## Consequences

- Larger image than Alpine; a deliberate drift from `stack.md`, recorded in the family drift table.
- `apt-cache policy infiniband-diags` is printed at build time so the tool version is in the build log.
- `ibswinfo` is not in the image ([ADR 0003](0003-client-choice.md)).
