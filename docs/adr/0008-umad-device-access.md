# 0008. UMAD device access in containers

- **Status:** accepted
- **Date:** 2026-11-23

## Context

`perfquery` and the other tools send MADs through `/dev/infiniband/umad*`, which is owned by root or a host group. The container user is uid 10001.

## Decision

Do not run as root. Pass the devices with `--device /dev/infiniband` (compose `devices:`) and add the numeric host gid that owns `umad*` with `--group-add` (compose `group_add: ["${UMAD_GID}"]`). `UMAD_GID` is read with `ls -ln /dev/infiniband/umad*` (4th column) and set in `.env`. The replay demo needs none of this.

## Consequences

- The default umad group and mode on trixie are **unverified**; docs tell the operator to read the gid from the host.
- udev rules on the host may need to give the group access to the umad devices.
- See `docs/deployment/docker.md` and `docker-compose.ghcr.yml`.
