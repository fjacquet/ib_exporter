# Demo replay fixtures

These files are **hand-written** from the rdma-core `infiniband-diags` output formats. They were
**not captured from hardware**. Fabric `demo`: leaf1 (LID 101, SM master), leaf2 (LID 102),
HCAs h1-h3 and the dual-port h4 (LIDs 14 and 15).

- Counters are static: `rate()` reads 0 and `increase()`-based alerts never fire in the demo.
  Absolute error counters are non-zero on leaf1 port 3 and leaf2 port 5 so tables are not empty.
- Naming: `<fabric>/<tool>__<args joined "_">.txt`, see `internal/ib/replay/README.md`.
- To extend: run `go run . --config <config with replay.dir pointing here> --once --debug` and
  create each file named in a `replay: no fixture <path>` error. To use real data, replace a file
  with the actual tool output (`perfquery -a -l <lid>` etc., without `-C`/`-P`).
