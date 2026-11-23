# Grafana dashboards

Provisioned from `grafana/dashboards/` into the folder "InfiniBand" (datasource uid `prometheus`).
Every dashboard has a `fabric` variable (`label_values(ib_up, fabric)`). Counters use
`rate()`/`increase()` on `_total` series; gauges are plotted directly. Metric names are in
[metrics.md](metrics.md); `make check-deploy` verifies dashboards and alerts only use those names.

| Dashboard (uid) | Shows |
|---|---|
| Overview (`ib-overview`) | fabrics up, nodes, active/inactive ports, SM state, throughput, packets, link flaps, port state table |
| Fabric errors (`ib-fabric-errors`) | symbol, receive, remote physical, link integrity errors, discards, VL15 drops, top ports |
| Congestion and FEC (`ib-congestion-fec`) | FECN/BECN, xmit wait, time congested, FEC corrected/uncorrectable, sync header, unknown blocks |
| HCA node (`ib-hca-node`) | per-HCA throughput, errors, link rate, state; local sysfs HCA panels (need `sources.localHCA`) |
| SM and topology (`ib-sm-topology`) | SM info/state/priority, nodes by type, node and link tables, collector duration and freshness |

Alert rules: `deploy/prometheus/ib.rules.yml` (fabric down, link downed, uncorrectable FEC, SM change,
symbol error rate, transmit discards, collector errors).
