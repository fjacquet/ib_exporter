# perfquery fixtures

Hand-written, NOT captured from hardware. Built from the printf/field formats in
rdma-core `libibmad/fields.c`, `libibmad/dump.c` and `infiniband-diags/perfquery.c`
(headers `# <name> counters: Lid N port M ...`, lines `Name:....value`, dots pad to column 32).
Real `-T` output prints 12 lanes; fixtures print 4. Values are invented.
