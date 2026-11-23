# Docker deployment

## Demo stack (no InfiniBand hardware)

```bash
docker compose up -d --build                      # exporter built from ./Dockerfile
docker compose -f docker-compose.ghcr.yml up -d   # or the published image (IB_TAG=...)
```

Services: `ib_exporter` (:9449), Prometheus (:9090, scrapes the exporter and loads
`deploy/prometheus/ib.rules.yml`), Grafana (:3000, `admin` / `$GRAFANA_ADMIN_PASSWORD`,
default `admin`, five provisioned dashboards, see [dashboards](../dashboards.md)).
Copy `.env.example` to `.env` to change the password or image tag.

The exporter runs in replay mode: `deploy/demo/config.yaml` sets `replay.dir: /replay`, and
`deploy/demo/replay/demo/` holds hand-written (not captured from hardware, see `deploy/demo/replay/README.md`) `infiniband-diags` output for a small fabric
(2 leaf switches, 4 HCAs, one dual-port; the SM is on `leaf1`). File naming is described in
`internal/ib/replay/README.md`. The fixtures are static, so counters never change and every
`rate()` reads 0 in the demo; `increase()` alerts therefore stay quiet, while the noisy ports
(`leaf1` port 3, `leaf2` port 5) show non-zero absolute error counters. `sources.localHCA` is
off because the local-HCA reader has no configurable sysfs root.

Check: `curl -s 127.0.0.1:9449/metrics | grep -c '^ib_'`, then `docker compose down`.

## Real hardware

The image is `debian:trixie-slim` with `infiniband-diags`, running as user `ib` (uid/gid 10001).
Debian, not Alpine: the diag tools are glibc binaries. To query a real fabric, use your own
config (no `replay.dir`, `fabrics[].ca`/`port` set to your HCA), drop the `/replay` mount, and
pass the umad devices to the container:

```yaml
services:
  ib_exporter:
    devices:
      - /dev/infiniband
    group_add:
      - "${UMAD_GID}"
```

`UMAD_GID` is the numeric group that owns the umad devices (4th column):

```bash
ls -ln /dev/infiniband/umad*
```

Put it in `.env`. Equivalent `docker run`: `--device /dev/infiniband --group-add <gid>`.
Mount a node-name map read-only if you use `fabrics[].nodeNameMap`.

## Health

`HEALTHCHECK` is `curl -fsS http://127.0.0.1:9449/livez`; `/livez` does not depend on the fabric.
