# ib_exporter — design

Status: DRAFT, awaiting approval. Date: 2026-11-23.
Legend: **[call]** = my decision, not agreed with the user. **[unverified]** = not checked against real hardware or docs.

## Intent

Exhaustive InfiniBand exporter for Prometheus + OTLP, in the family standard (`exporter-standards`).
One process sees the fabric from a management node (switches, HCAs, topology, SM) and the
local HCA from sysfs. Replaces three partial tools:

- `guilbaults/infiniband-exporter`: parses `ibqueryerrors` STDERR, single host, optional counter reset, no FEC/switch health.
- `treydock/infiniband_exporter`: Go, `perfquery` per port, `ibswinfo` BETA, HCA rate has no port label, no SM, no FEC.
- node_exporter `infiniband` collector: local HCA only (verified via Perplexity, sources: `collector/infiniband_linux.go`, `procfs/sysfs/class_infiniband.go`). Labels `device`,`port`. Exports `counters/` (as `node_infiniband_*_total`), `counters_ext/` (as `legacy_*`), a fixed list of 9 `hw_counters/` (`out_of_buffer`, `np_cnp_sent`, `duplicate_request`, `lifespan`, `local_ack_timeout_err`, `implied_nak_seq_err`, `out_of_sequence`, `packet_seq_err`, `resp_local_length_error`), `node_infiniband_state{state}` and `physical_state_id{phys_state}`. It reads `rate`, `link_layer`, `hca_type`, `node_guid` but does not export them. We beat it on: all `hw_counters` files (not a fixed 9, e.g. `rp_cnp_handled`, `rx_icrc_encapsulated`, `roce_adp_retrans` missing there), rate/link layer/GUID, and everything fabric-side. We do not drop HCA metrics; the user wants all of it in one place.

## Goals

- Per-port counters for every switch and HCA port in the fabric(s): data, packets, errors, discards, congestion (`PortXmitWait`, FECN/BECN), VL15/QP1 drops.
- Per-lane FEC and sync-header counters via `perfquery -T` (phase 1).
- Port state/physical state/width/speed/MTU, topology as link-info series, node inventory, SM info and state.
- Local HCA from sysfs: `counters/`, `hw_counters/` (any provider; names not hardcoded, see Local HCA metrics), state, rate, `lid`, `sm_lid`, firmware, board id.
- Optional switch health from `ibswinfo` (PSU, fan, temperature). Off by default.
- `fabric` label on every series; several fabrics per process.
- Family shape: snapshot store, dual export, `--once --debug`, `--trace`, `/livez` `/readyz` `/health`, build_info, SIGHUP reload.

## Non-goals

- UFM REST support (user has no UFM). The reader interface leaves room for it later.
- Native MAD/UMAD client in Go (approach B). Possible later behind the same interface.
- Raw BER and eye diagrams (`mlxlink`, NVIDIA-only). Phase 2.
- Reaching fabrics the host has no HCA on. Other site = another instance with its own `fabric` name.

## Anti-goals

- Never reset fabric counters (no `-r`, `-R`). Counters are cumulative.
- Never emit a fake zero. An unreadable value is an absent sample.
- Never key a series on LID. LIDs change after an SM re-sweep.

## Constraints

- No cluster access now. Dev and CI run on fixtures. **Nothing here is validated on hardware.**
- UMAD needs `/dev/infiniband/umad*`. Root, or a group that owns the device.
- PMA MADs load switch management CPUs. Low default concurrency.
- Go 1.26.4 [per stack.md], `//go:build linux` for sysfs and exec readers; darwin builds must still compile with a stub.

## Approaches considered

| | What | Trade-off |
|---|---|---|
| A | Shell out to infiniband-diags only | Fast to write; text parsing; HCA via perfquery instead of sysfs |
| B | Native Go MAD via umad ioctl | No exec, 64-bit direct; large code, no mature Go lib, untestable without fabric |
| **C (chosen)** | Hybrid: sysfs for local HCA, diags CLIs for fabric, all behind reader interfaces | Keeps B possible later; still parses CLI text (isolated in one package, fixture-tested) |

User agreed on C with the `fabric` label. Rationale: lowest risk given no hardware for validation.

## Design

### Readers (interfaces, each swappable)

| Reader | Source | Cadence |
|---|---|---|
| Topology | `ibnetdiscover -p` (`-C -P` per fabric) | slow (default 10m) |
| PortCounters | `perfquery -x -a -l <lid>` per node; see decimation | fast (default 60s) |
| PortOptional | `perfquery -T`, `-E`, `-D`, `--rcvcc`, `--xmitcc`, `--slrcvfecn/becn` | every Nth cycle (decimation) |
| PortInfo | `smpquery portinfo` / `nodeinfo` | medium (default 5m) |
| SM | `saquery -s` / `sminfo` | medium |
| Switch health | `ibswinfo -d lid-<LID>`, optional | slow |
| LocalHCA | sysfs `/sys/class/infiniband` | fast |
| Replay | fixture directory, same interfaces | demo and tests |

Corrections confirmed with rdma-core docs (context7): `-a` alone aggregates ports; per-port needs `-a -l`.
Data counters are units of 4 octets: multiply by 4 for `_bytes_total`.
Perplexity claimed `-x` output includes the basic error counters; its citations did not show it, and node_exporter reads error counters from the basic `counters/` and data/packets 64-bit from `counters_ext/`, which suggests the two are separate. **Decision [call]: always run both the basic and the `-x` read and merge** (basic for errors, `-x` for 64-bit data/packets). Re-check on real hardware.
`-T` with `-a -l`: Perplexity says `perfquery.c` routes it through the per-port path, but the optional counter depends on device support. Treat as **[unverified on hardware]**; fallback is one exec per port.
Exact `-T` field names (per-lane FEC, sync header, unlocked block): **still unknown**. Perplexity could not list them; read `perfquery.c` / `mad.h` in rdma-core when writing the parser.

### Identity

Series key = `fabric` + node GUID + port number. Each cycle resolves GUID→LID from the latest topology
snapshot. A stale LID that fails yields an absent sample for that node and increments
`ib_collect_errors_total{fabric,reader}`.

### Snapshot, export, probes

As `architecture.md`: collection loop → immutable snapshot → `SnapshotStore` → unchecked Prometheus collector + OTLP observable gauges/counters. Serve HTTP before the first cycle. `/livez` `/readyz` static 200, `/health` informational 200. `ib_up{fabric}` gauge per fabric; fabric down is data, not a failure. `ib_exporter_build_info{version,goversion}`.

### Config

```yaml
server: {port: 9449, uri: /metrics}   # [call] port 9449: next free after kemp 9448
collection:
  interval: 60s
  topologyInterval: 10m
  portInfoInterval: 5m
  maxConcurrent: 4          # PMA load on switch CPUs
  optionalEvery: 5          # decimation for -T/-E/-D/congestion
fabrics:
  - name: prod-ib
    ca: mlx5_0
    port: 1
    nodeNameMap: /etc/ib/node-name-map   # optional
sources:
  localHCA: true
  ibswinfo: {enabled: false, path: /usr/bin/ibswinfo}
replay: {dir: ""}           # non-empty = read fixtures instead of hardware
```

`${ENV}` expansion applies to string fields. Reserved URIs rejected in `Validate()` via exported `ReservedURIs`.

### Metrics (prefix `ib_`) [call: prefix]

Counters are cumulative and exposed as `_total`; dashboards use `rate()`. This deviates from the family "gauges, never `rate()`" rule on purpose (ADR). All series carry `fabric`.

- `ib_port_{xmit,rcv}_data_bytes_total`, `ib_port_{xmit,rcv}_packets_total`, unicast/multicast variants. Labels: `node_type` (switch/hca/router), `guid`, `node_name`, `port`.
- Errors/discards/congestion: `symbol_error`, `link_error_recovery`, `link_downed`, `rcv_errors`, `rcv_remote_physical_errors`, `rcv_switch_relay_errors`, `xmit_discards`, `xmit_constraint_errors`, `rcv_constraint_errors`, `local_link_integrity_errors`, `excessive_buffer_overrun_errors`, `vl15_dropped`, `qp1_dropped`, `xmit_wait` (all `_total`).
- FEC from `-T`: `ib_port_fec_corrected_blocks_total`, `ib_port_fec_uncorrectable_blocks_total`, `ib_port_sync_header_errors_total`, with `lane` where the PMA reports per lane. **[unverified: exact field names]**
- Port state: `ib_port_state`, `ib_port_phys_state`, `ib_port_link_width_active`, `ib_port_link_speed_active`, `ib_port_rate_bytes_per_second`, `ib_port_mtu_bytes`.
- Topology: `ib_link_info{guid,port,remote_guid,remote_port,remote_name} 1`, `ib_node_info{guid,node_name,node_type,lid,firmware} 1`.
- SM: `ib_sm_info{fabric,sm_guid,sm_lid} 1`, `ib_sm_state`, `ib_sm_priority`, `ib_sm_activity_count`.
- Switch health (optional): `ib_switch_temperature_celsius`, `ib_switch_fan_rpm`, `ib_switch_psu_watts`, `ib_switch_psu_status_info`, `ib_switch_uptime_seconds`, `ib_switch_hardware_info`.
- Local HCA: `ib_hca_*` mirroring counters; `ib_hca_hw_counter_total{name}` generic over `hw_counters/`: the exporter enumerates whatever files exist, never matches a device name like `mlx5_*` (verified via Perplexity/NVIDIA docs: ConnectX-9 uses the existing `mlx5` driver family, upstream Linux 6.16; there is no `mlx9` driver; device index still depends on host enumeration). Known mlx5 names include `out_of_buffer`, `np_cnp_sent`, `rp_cnp_handled`, `np_ecn_marked_roce_packets`, `rx_icrc_encapsulated`, `roce_adp_retrans`, `lifespan`; the set varies with kernel, firmware and config; `ib_hca_info{board_id,firmware,node_guid}`; `ib_hca_lid`, `ib_hca_sm_lid`. `fabric=""` for ports not bound in config.
- Exporter: `ib_up{fabric}`, `ib_collect_duration_seconds{fabric,reader}`, `ib_collect_errors_total{fabric,reader}`, `ib_collect_last_success_timestamp_seconds{fabric,reader}`.

Label-key union invariant: a name carries one label set across all series; empty string where not applicable. Test enforces it.

### Packaging

- Base image: **`debian:trixie-slim`**, not Alpine (diags tools need glibc). uid `10001`, named user, `curl` for `HEALTHCHECK` on `127.0.0.1/livez`. Exception to `stack.md`, recorded in an ADR and in the `decisions.md` drift table. `infiniband-diags` on trixie provides `perfquery`, `ibnetdiscover`, `smpquery`, `saquery`, `sminfo`, `ibqueryerrors` (verified via Perplexity/packages.debian.org). `ibswinfo` packaging **[unverified]**, so it stays optional and outside the image.
- UMAD without root: mount `/dev/infiniband`, `group_add` the host gid of the `umad*` owner (compose variable). `--device` + `--group-add <host-gid>` is the documented Docker mechanism; the default umad group/mode on trixie is **[unverified]**, so the compose variable holds the gid and docs say to read it with `ls -l /dev/infiniband/umad*`.
- `Dockerfile.goreleaser` uses the same base. CI trio via `fjacquet/ci@v1` callers. Dependabot `gomod` + `docker` only.
- Demo compose stack runs on a laptop with the replay reader and fixtures; no IB hardware needed.

### Testing without a cluster

- Fixtures: derived from rdma-core man page examples and `treydock` test data, then hand-extended for `-T`, congestion and multi-port nodes. Fixture realism is the main risk.
- Parser tests per reader. Collector tests assert via Prometheus gather **and** OTLP `ManualReader`. Label-union test. Probe tests on the real mux.
- `ibsim` is packaged on trixie (`ibsim-utils` listed; the simulator is meant for use with OpenSM and the diags tools). Worth a spike for a simulated fabric on Linux CI; whether it supports PMA/`-T` counters is **[unverified]**.
- First real-hardware run must use `--once --debug --trace` and diff against `docs/metrics.md`. Until then every [unverified] item stays open.

### ADRs to write

Snapshot model; dual export; client choice (exec + sysfs, no SDK, no REST); counters as `_total` not gauges; no counter reset; GUID+port identity; label union; base image Debian slim; UMAD access.

## Left to the builder

Parser details per CLI, fixture content, metric help strings, dashboard layout (overview, fabric errors, congestion/FEC, HCA node view, SM/topology), alert rules (link downed, FEC uncorrectable > 0, SM change, `ib_up == 0`), mkdocs content, goreleaser config.

## Open items for the user

1. Real command output and hardware list, when a cluster is available (see conversation).
2. Confirm prefix `ib_` and port 9449.
