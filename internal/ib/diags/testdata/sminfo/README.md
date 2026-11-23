master.txt: `sminfo` line, from rdma-core infiniband-diags/sminfo.c
("sminfo: sm lid %d sm guid 0x%lx, activity count %u priority %d state %d %s").
smir.txt: `saquery SMIR` (SMInfoRecord) dump from saquery.c dump_sm_info_record.
NOTE: `saquery -s` does NOT print SMInfo; it lists IsSM ports (PortInfoRecords). Use `saquery SMIR`.
Hand-written, NOT captured from hardware. State: 0 NotActive, 1 Discovering, 2 Standby, 3 Master.
