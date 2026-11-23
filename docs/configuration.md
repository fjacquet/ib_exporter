# Configuration

YAML file, `--config` (default `config.yaml`). Unknown keys are rejected. The file is reloaded on
change or `SIGHUP`; a reload that fails validation keeps the old configuration.
`server.port` / `server.uri` changes need a restart (a warning is logged). `${ENV}` references are
expanded in `server.uri`, `fabrics[].name|ca|nodeNameMap`, `sources.ibswinfo.path` and `replay.dir`.

| Key | Default | Meaning |
|---|---|---|
| `server.port` | `9449` | HTTP listen port (1-65535) |
| `server.uri` | `/metrics` | Metrics path; must start with `/` and not be a reserved URI |
| `collection.interval` | `60s` | Fast cycle: counters |
| `collection.topologyInterval` | `10m` | `ibnetdiscover -p` refresh |
| `collection.portInfoInterval` | `5m` | `smpquery`, SM and `ibswinfo` refresh |
| `collection.maxConcurrent` | `4` | Bound on concurrent `infiniband-diags` execs, global (PMA load on switch CPUs) |
| `collection.optionalEvery` | `5` | Run `-T`, `-E`, `-D`, congestion every Nth cycle |
| `fabrics[].name` | required | Value of the `fabric` label; unique |
| `fabrics[].ca` | required | HCA used for management (`-C`), e.g. `mlx5_0` |
| `fabrics[].port` | required, >= 1 | HCA port (`-P`) |
| `fabrics[].nodeNameMap` | `""` | Node name map file; must list **port** GUIDs for HCAs |
| `sources.localHCA` | `false` | Read the local HCA from `/sys/class/infiniband` |
| `sources.ibswinfo.enabled` | `false` | Switch health via `ibswinfo` (optional, unverified) |
| `sources.ibswinfo.path` | `""` | Path to `ibswinfo` (the sample config uses `/usr/bin/ibswinfo`) |
| `replay.dir` | `""` | Non-empty: read fixtures from `<dir>/<fabric name>/` instead of running tools |

Validation: at least one fabric or `sources.localHCA`; intervals > 0; `maxConcurrent` and
`optionalEvery` >= 1; fabric names non-empty and unique.

**Reserved URIs** (`config.ReservedURIs`): `/`, `/health`, `/livez`, `/readyz`.

```yaml
fabrics:
  - name: prod-ib
    ca: mlx5_0
    port: 1
    nodeNameMap: ${IB_NODE_NAME_MAP}
```

**Replay mode**: with `replay.dir` set, a replay runner implements the same `diags.Runner` and reads
`<dir>/<fabric>/<tool>__<args>.txt` (see `internal/ib/replay/README.md`). Used by the demo stack and tests.

**CLI**: `--config`, `--once` (one cycle, no HTTP), `--debug` (with `--once`, print every sample),
`--trace` (log every command with its stdout). OTLP is enabled by `OTEL_EXPORTER_OTLP_ENDPOINT` or
`OTEL_EXPORTER_OTLP_METRICS_ENDPOINT`; other `OTEL_*` variables apply.
