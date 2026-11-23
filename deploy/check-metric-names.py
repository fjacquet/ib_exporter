#!/usr/bin/env python3
"""Fail if a dashboard or alert rule uses an ib_* metric name that docs/metrics.md does not list."""
import glob, re, sys

pat = re.compile(r"\bib_[a-z0-9_]+")
catalog = set(pat.findall(open("docs/metrics.md").read()))
bad = []
for f in glob.glob("grafana/dashboards/*.json") + glob.glob("deploy/prometheus/*.rules.yml"):
    for name in sorted(set(pat.findall(open(f).read()))):
        if name not in catalog:
            bad.append(f"{f}: {name}")
if bad:
    sys.exit("metric names not in docs/metrics.md:\n" + "\n".join(bad))
print("ok: all dashboard/alert metric names are in docs/metrics.md")
