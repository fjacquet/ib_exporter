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
