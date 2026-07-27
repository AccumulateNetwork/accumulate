#!/usr/bin/env python3
"""Report every cross-partition synthetic channel: produced at source vs
received at destination. Names are normalised on BOTH sides — comparing a
lowercased key against a mixed-case one silently reports 0 received and
fabricates a stall."""
import json, subprocess, sys
PARTS = ["dn", "bvn-BVN1", "bvn-BVN2", "bvn-BVN3"]
def q(scope):
    out = subprocess.run(["curl","-s","-m","8","-X","POST","http://localhost:26660/v3",
        "-H","content-type: application/json",
        "-d",json.dumps({"jsonrpc":"2.0","id":1,"method":"query","params":{"scope":scope}})],
        capture_output=True, text=True).stdout
    try: return json.loads(out).get("result",{}).get("account",{}) or {}
    except Exception: return {}
def norm(u): return (u or "").strip().lower().rstrip("/")
led = {p: q("acc://%s.acme/synthetic" % p) for p in PARTS}
def entry(ledger, other):
    want = norm("acc://%s.acme" % other)
    for e in (ledger.get("sequence") or []):
        if norm(e.get("url")) == want: return e
    return {}
print("  src -> dst                produced  received  delivered  UNDELIVERED")
bad = []
for src in PARTS:
    for dst in PARTS:
        if src == dst: continue
        prod = entry(led[src], dst).get("produced") or 0
        e = entry(led[dst], src)
        recv = e.get("received") or 0; deliv = e.get("delivered") or 0
        if prod == 0 and recv == 0: continue
        u = prod - recv
        if u > 0: bad.append((src, dst, u))
        print("  %-9s -> %-9s  %-9s %-9s %-10s %s%s" % (src, dst, prod, recv, deliv, u,
              "  <<< STALLED" if u > 0 else ""))
print()
print("stalled channels:", len(bad), bad if bad else "")
sys.exit(1 if bad else 0)
