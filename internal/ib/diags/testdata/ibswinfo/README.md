UNVERIFIED. `ibswinfo` is not packaged in Debian and no real output was available. basic.txt is a
guessed `key : value` layout (labels such as `PSU1 power`, `FAN1`, `temperature`, `uptime`).
The parser is tolerant: unknown lines are ignored, N/A values are dropped, and input where
nothing is recognised is an error. Re-check against a real switch before relying on it.
