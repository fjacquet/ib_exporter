# 0003. Client choice: exec infiniband-diags and sysfs, no SDK or REST

- **Status:** accepted
- **Date:** 2026-11-23

## Context

The user has no UFM. There is no mature Go library for UMAD/MAD. The host has the `infiniband-diags` tools, and local HCA data is in `/sys/class/infiniband`. There is no cluster to validate against.

## Considered options

- A: diags CLIs only. B: native Go MAD over the umad ioctl (large, untestable without a fabric). C: hybrid, diags CLIs for the fabric and sysfs for the local HCA, behind reader interfaces.

## Decision

C. `diags.Runner` (`internal/ib/diags/runner.go`) execs the tools with `-C <ca> -P <port>`; text parsers sit in `internal/ib/diags`, fixture-tested; the local HCA is read by `internal/ib/sysfs`; `internal/ib/replay` implements `Runner` from fixture files. No SDK, no REST, no UFM. A native MAD client or UFM can come later behind the same interface.

## Consequences

- Text parsing is the main risk; it is isolated in one package. Field names and output are **unverified on hardware** until a first run with `--once --debug --trace`.
- The image must carry glibc tools ([ADR 0007](0007-debian-base-image.md)) and umad device access ([ADR 0008](0008-umad-device-access.md)).
- `ibswinfo` (switch PSU, fan, temperature) is optional and **unverified**: it is not in Debian, its output format is unknown, so the parser is tolerant and the fixture is a guess. It stays disabled by default and outside the image.
