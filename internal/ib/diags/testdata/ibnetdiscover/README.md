Fixtures follow the `ibnetdiscover -p` ("ports report") format taken from rdma-core
infiniband-diags/ibnetdiscover.c dump_ports_report():
local:  "%2s %5d %2d 0x%016x %s %s"   (type, lid, port, portguid, width, speed)
remote: " - %2s %5d %2d 0x%016x ( '%s' - '%s' )" (type, lid, port, guid, localname, remotename)
unconnected: 36 spaces then 'name'.
Verified against the C source (fetched 2026-11-23); the sample lines themselves are hand-written.
