# 0005. Identity is fabric + GUID + port, never LID

- **Status:** accepted (amended 2026-11-23)
- **Date:** 2026-11-23

## Context

LIDs are assigned by the subnet manager and change after an SM re-sweep, so a LID-keyed series would break history. GUIDs are stable.

## Decision

A series is keyed by `fabric` + `guid` + `port`. Each cycle resolves GUID to LID from the latest topology snapshot (`internal/collect/fabric.go`); a stale LID that fails gives an absent sample and increments `ib_collect_errors_total{fabric,reader}`. Counters, caches and the optional/portinfo samples are keyed by GUID.

**Amendment 2026-11-23 (Task 3 review ruling).** `ibnetdiscover -p` only carries port GUIDs. For switches every port shares the switch GUID, so `guid` is the node GUID. For HCAs `guid` is the **port GUID**, which is also what `perfquery -G` addresses; a dual-port HCA therefore has two `guid` values, grouped by `node_name`. Local sysfs series carry the same port GUID (from `ports/<n>/gids/0`) plus `node_guid` in `ib_hca_info`, so fabric and local series join.

## Consequences

- A node name map must list port GUIDs for HCAs.
- LIDs appear only as `lid` on `ib_node_info` and `ib_hca_lid`.
