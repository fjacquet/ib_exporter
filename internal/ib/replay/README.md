# replay

`replay.Runner{Dir}` implements `diags.Runner` by reading files instead of running tools.

Layout: `<Dir>/<fabric name>/<tool>__<args joined with "_">.txt`

- `args` are the arguments passed to `Run`, without the `-C <CA> -P <port>` that `ExecRunner` appends.
- Every character outside `A-Za-z0-9._-` becomes `-` (so `1/2` is `1-2`).
- Example: `perfquery -x -a -l` for fabric `prod` is `<Dir>/prod/perfquery__-x_-a_-l.txt`.
- A missing file is the error `replay: no fixture <path>`.
