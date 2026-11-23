# Metrics reference

## Port counters from perfquery -T

Source: rdma-core `libibmad/fields.c` (PortExtendedSpeedsCounters tables) and
`infiniband-diags/perfquery.c` (`extended_speeds_query`), master branch, read 2026-11-23.
`perfquery -T` prints `# PortExtendedSpeedsCounters counters: <portid> port <n>`, or
`# PortExtendedSpeedsCounters with RS-FEC Active counters: ...` when PortInfoExtended
reports RS-FEC active. Field names below are the real libibmad labels; the mapping to
metrics and the output on real hardware are **UNVERIFIED** until a cluster run
(`--once --debug --trace`). Fixtures in `internal/ib/diags/testdata/perfquery/` are hand-written.

Common to both variants: `PortSelect`, `CounterSelect` (ignored).

| Field | Variant | Metric | `lane` label |
|---|---|---|---|
| `SyncHeaderErrorCounter` | both | `ib_port_sync_header_errors_total` | none |
| `UnknownBlockCounter` | both | `ib_port_unknown_blocks_total` | none |
| `ErrorDetectionCounterLane0..11` | plain | `ib_port_error_detection_total` | 0..11 |
| `FECCorrectableBlockCtrLane0..11` | plain | `ib_port_fec_corrected_blocks_total` | 0..11 |
| `FECUncorrectableBlockCtrLane0..11` | plain | `ib_port_fec_uncorrectable_blocks_total` | 0..11 |
| `FECCorrectableSymbolCtrLane0..11` | RS-FEC | `ib_port_fec_corrected_symbols_total` | 0..11 |
| `PortFECCorrectableBlockCtr` | RS-FEC | `ib_port_fec_corrected_blocks_total` | `""` |
| `PortFECUncorrectableBlockCtr` | RS-FEC | `ib_port_fec_uncorrectable_blocks_total` | `""` |
| `PortFECCorrectedSymbolCtr` | RS-FEC | `ib_port_fec_corrected_symbols_total` | `""` |

Metrics with a `lane` label always carry it (label-union invariant): per-lane values use
`"0".."11"`, port-level values use `""`. `lane` is appended after the canonical
`fabric,node_type,guid,node_name,port` labels.

## Other perfquery fields (from libibmad, also UNVERIFIED on hardware)

- Basic and `-x`: `PortXmitData`/`PortRcvData` are in units of 4 octets and exported x4 as
  `ib_port_{xmit,rcv}_data_bytes_total`; packets, unicast/multicast, errors and `PortXmitWait`
  map to the names in the design spec. `-x` values override basic ones when merged.
- `-E` (PortRcvErrorDetails): `ib_port_{local_physical,malformed_packet,buffer_overrun,dlid_mapping,vl_mapping,looping}_errors_total`.
- `-D` (PortXmitDiscardDetails): `ib_port_xmit_{inactive,neighbor_mtu,sw_lifetime_limit,sw_hoq_lifetime_limit}_discards_total`.
- Congestion: `PortPktRcvFECN`, `PortPktRcvBECN`, `PortXmitTimeCong` -> `ib_port_rcv_fecn_total`, `ib_port_rcv_becn_total`, `ib_port_xmit_time_cong_total`. Per-SL/VL congestion tables are not exported.

## Port state (`smpquery portinfo <lid> <port>`)

Fields from rdma-core `libibmad` (fields.c names, dump.c strings; verified against source 2026-11-23,
fixtures hand-written, output on real hardware **UNVERIFIED**). Gauges with the `ib.PortLabels` set.

| Field | Metric | Value |
|---|---|---|
| `LinkState` | `ib_port_state` | 1 Down, 2 Init, 3 Armed, 4 Active, 5 ActiveDefer |
| `PhysLinkState` | `ib_port_phys_state` | 1 Sleep, 2 Polling, 3 Disabled, 4 PortConfigurationTraining, 5 LinkUp, 6 LinkErrorRecovery, 7 PhyTest |
| `LinkWidthActive` | `ib_port_link_width_active` | lanes (`4X` = 4) |
| `LinkSpeedActive` / `LinkSpeedExtActive` | `ib_port_link_speed_active` | per-lane signalling Gbps (`10.0 Gbps` = 10; ext used when `LinkSpeedActive` is `Extended speed`) |
| width x speed | `ib_port_rate_bytes_per_second` | lanes x nominal data Gbps/lane (2.5->2, 5->4, 10->8, 14.0625->14, 25.78125->25, 53.125->50, 106.25->100) / 8 x 1e9 |
| `NeighborMTU` | `ib_port_mtu_bytes` | bytes |

Width, speed and rate are only emitted when the port is Active (a down port reports meaningless
values). N/A or unparsable fields give no sample. Unknown speeds give no rate.

## SM (`sminfo`, `saquery SMIR`)

`ib_sm_info{fabric,sm_guid,sm_lid} 1`, plus `ib_sm_state` (0 NotActive, 1 Discovering, 2 Standby,
3 Master), `ib_sm_priority`, `ib_sm_activity_count` with the same labels. `sminfo.c` prints
`sminfo: sm lid N sm guid 0x..., activity count N priority N state N NAME`. Note: `saquery -s` does
not print SMInfo (it lists IsSM ports); use `saquery SMIR`. Only the first record is parsed.
Fixtures are hand-written, **UNVERIFIED** on hardware.

## Switch health (`ibswinfo -d lid-<LID>`, optional) - UNVERIFIED

`ibswinfo` is not in Debian and its output format is unknown; the parser is tolerant of
`key : value` text and the fixture is a guess. Labels `fabric,node_type,guid,node_name` plus
`sensor` (`ib_switch_temperature_celsius`), `fan` (`ib_switch_fan_rpm`), `psu` (`ib_switch_psu_watts`),
`psu`+`state` (`ib_switch_psu_status_info`, value 1), none (`ib_switch_uptime_seconds`),
`part_number,serial_number,firmware_version,psid` (`ib_switch_hardware_info`, value 1).
Unrecognised input is an error; N/A readings are absent, never zero.

## Local HCA (`/sys/class/infiniband`, `internal/ib/sysfs`) - UNVERIFIED on hardware

Read from `<root>/<device>/ports/<n>/`; fixture `internal/ib/sysfs/testdata/sysfs/` is hand-written, modelled on the mlx5
layout. Labels `fabric,device,port,guid`; `fabric=""` when the port is not bound in config. `guid` is the **port** GUID
(low 64 bits of `gids/0`, `0x%016x`). Device names are never assumed. Unreadable or unparsable files give no sample.

| Source | Metric | Notes |
|---|---|---|
| `counters/*`, `counters_ext/*` | `ib_hca_<name>_total` | `port_` prefix and `_64` suffix dropped, lowercased (`link_downed` -> `ib_hca_link_downed_total`); `counters_ext` overrides `counters` |
| `port_{rcv,xmit}_data[_64]` | `ib_hca_{rcv,xmit}_data_bytes_total` | units of 4 octets (kernel `sysfs-class-infiniband`), exported x4 |
| every file in `hw_counters/` | `ib_hca_hw_counter_total{name}` | enumerated, no hard-coded list |
| `state` (`4: ACTIVE`), `phys_state` (`5: LinkUp`) | `ib_hca_state`, `ib_hca_phys_state` | leading number, same codes as `ib_port_*` |
| `rate` (`100 Gb/sec (4X EDR)`) | `ib_hca_rate_bytes_per_second` | leading number x 1e9 / 8 |
| `lid`, `sm_lid` | `ib_hca_lid`, `ib_hca_sm_lid` | gauges |
| `link_layer` | `ib_hca_link_layer_info{link_layer}` | value 1 |
| `node_guid`, `board_id`, `fw_ver` | `ib_hca_info{board_id,firmware,node_guid}` | value 1 |

Only port-level `hw_counters/` is read (not the device-level directory some drivers also expose).

## Port counter names (perfquery basic, `-x`, `-E`, `-D`)

Cumulative counters (`_total`, use `rate()`), labels `fabric,node_type,guid,node_name,port`.
Field mapping is in `internal/ib/diags/perfquery.go`; output on real hardware **UNVERIFIED**.

| Reader | Metrics |
|---|---|
| basic / `-x` data, packets | `ib_port_xmit_data_bytes_total`, `ib_port_rcv_data_bytes_total`, `ib_port_xmit_packets_total`, `ib_port_rcv_packets_total`, `ib_port_unicast_xmit_packets_total`, `ib_port_unicast_rcv_packets_total`, `ib_port_multicast_xmit_packets_total`, `ib_port_multicast_rcv_packets_total` |
| basic errors | `ib_port_symbol_error_total`, `ib_port_link_error_recovery_total`, `ib_port_link_downed_total`, `ib_port_rcv_errors_total`, `ib_port_rcv_remote_physical_errors_total`, `ib_port_rcv_switch_relay_errors_total`, `ib_port_xmit_discards_total`, `ib_port_xmit_constraint_errors_total`, `ib_port_rcv_constraint_errors_total`, `ib_port_local_link_integrity_errors_total`, `ib_port_excessive_buffer_overrun_errors_total`, `ib_port_vl15_dropped_total`, `ib_port_qp1_dropped_total`, `ib_port_xmit_wait_total` |
| `-E` | `ib_port_local_physical_errors_total`, `ib_port_malformed_packet_errors_total`, `ib_port_buffer_overrun_errors_total`, `ib_port_dlid_mapping_errors_total`, `ib_port_vl_mapping_errors_total`, `ib_port_looping_errors_total` |
| `-D` | `ib_port_xmit_inactive_discards_total`, `ib_port_xmit_neighbor_mtu_discards_total`, `ib_port_xmit_sw_lifetime_limit_discards_total`, `ib_port_xmit_sw_hoq_lifetime_limit_discards_total` |
| `--rcvcc`, `--xmitcc` | `ib_port_rcv_fecn_total`, `ib_port_rcv_becn_total`, `ib_port_xmit_time_cong_total` |

## Local HCA counter names in the sysfs fixture

`ib_hca_<name>_total` follows whatever files `counters/` and `counters_ext/` hold, so this list is
what the mlx5 fixture yields, not a closed set: `ib_hca_xmit_data_bytes_total`,
`ib_hca_rcv_data_bytes_total`, `ib_hca_xmit_packets_total`, `ib_hca_rcv_packets_total`,
`ib_hca_unicast_xmit_packets_total`, `ib_hca_symbol_error_total`, `ib_hca_rcv_errors_total`,
`ib_hca_vl15_dropped_total`.

## Topology (`ibnetdiscover -p`)

Gauges, value 1, refreshed every `collection.topologyInterval`.

| Metric | Labels |
|---|---|
| `ib_node_info` | `fabric,guid,node_name,node_type,lid,firmware` (`firmware` is `""`: not in `ibnetdiscover -p` output) |
| `ib_link_info` | `fabric,guid,port,remote_guid,remote_port,remote_name` (one series per direction) |

## Exporter

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `ib_up` | gauge | `fabric` | 1 when a topology is available and the fabric answered this cycle: the topology was read, or at least one node's perfquery succeeded on the cached topology. Else 0 (also when a cached topology exists but every perfquery failed). A down fabric is data, not a scrape failure. |
| `ib_collect_errors_total` | counter | `fabric,reader` | Reader failures since start: one per failing node (or command group), not per command. |
| `ib_collect_duration_seconds` | gauge | `fabric,reader` | Sum of the reader's exec durations in its last cycle that ran, **including waits for a free exec slot** (`collection.maxConcurrent`). Readers run in parallel, so it is work time, not wall-clock time, and can exceed the interval. Absent until the reader first ran. |
| `ib_collect_last_success_timestamp_seconds` | gauge | `fabric,reader` | Unix time of the reader's last successful cycle. Absent until it first succeeded. |
| `ib_exporter_build_info` | gauge | `version,goversion` | Constant 1. The only series without `fabric`: it describes the process. |

`reader` is `topology`, `perfquery`, `perfquery_optional`, `portinfo`, `sm`, `ibswinfo` (when
enabled), or `sysfs` (local HCA, `fabric=""`).

Export rules (`internal/ib/metrics.go`): both Prometheus and OTLP skip a sample with an invalid
name, label or UTF-8 value, a label-key set or counter/gauge kind other than the first sample of
that name in the snapshot, or a duplicate series. OTLP registers instruments for new names
asynchronously, so a name observed for the first time is exported from the next OTLP collection on.
