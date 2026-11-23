Fixtures follow `smpquery portinfo <lid> <port>` output: libibmad `mad_dump_portinfo`
(`Name:....value`, dots pad to col 32) with field names from `libibmad/fields.c`
(LinkState, PhysLinkState, LinkWidthActive, LinkSpeedActive, LinkSpeedExtActive, NeighborMTU)
and value strings from `libibmad/dump.c` (e.g. `4X`, `10.0 Gbps`, `Extended speed`,
`No Extended Speed`, `Active`, `LinkUp`). Verified against rdma-core source (fetched 2026-11-23).

Hand-written, NOT captured from hardware. UNVERIFIED: exact line set on real devices, and whether
state strings are `Initialize` or `Init` and whether PortState 5 (ActiveDefer) has a name; the
parser also accepts a bare number.
