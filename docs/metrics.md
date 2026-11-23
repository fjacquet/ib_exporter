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
