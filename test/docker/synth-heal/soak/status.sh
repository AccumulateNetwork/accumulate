#!/usr/bin/env bash
# Compact one-screen status for the running synthetic-healing soak.
# Reads the monitor's /data endpoint (soakmon.py must be running on :8099).
curl -s -m 10 http://127.0.0.1:8099/data | python3 -c '
import sys, json, time
d = json.load(sys.stdin)
up = int(time.time()) - d.get("started", 0)
print("uptime      %dh%02dm   api=%s" % (up // 3600, (up % 3600) // 60, (d.get("network") or {}).get("api", "?")))
print("heights     %s" % d.get("heights"))
w = d.get("wedges") or {}
h = d.get("heals") or {}
print("dropped     synthetic=%s anchor=%s total=%s  byDest=%s" % (
    w.get("synthetic"), w.get("anchor"), w.get("total"), w.get("byDest")))
print("healed      synthetic=%s anchor=%s total=%s  stuck=%s errors=%s" % (
    h.get("synthetic"), h.get("anchor"), h.get("total"), h.get("stuck"), h.get("errors")))
if h.get("stuckStream"):
    print("STUCK       %s" % h["stuckStream"])
c = d.get("chaos") or {}
print("chaos       %s  recent=%s" % (c.get("counts"), (c.get("recent") or [])[-3:]))
lg = d.get("loadgen") or {}
if lg:
    keys = [k for k in ("generated", "submitted", "delivered", "rejected", "skipped", "stranded") if k in lg]
    print("loadgen     %s" % {k: lg[k] for k in keys})
# any synthetic stream with a delivery gap
gaps = []
for src, row in ((d.get("matrix") or {}).get("flows", {}).get("synthetic", {}) or {}).items():
    for dst, c2 in row.items():
        gap = (c2.get("sent", 0) or 0) - (c2.get("deliv", 0) or 0)
        if gap > 0:
            gaps.append("%s->%s gap=%d (sent=%d deliv=%d)" % (src, dst, gap, c2.get("sent", 0), c2.get("deliv", 0)))
print("synth gaps  %s" % ("; ".join(sorted(gaps)) if gaps else "none"))
'
