#!/usr/bin/env python3
"""Report every cross-partition synthetic channel: produced at source vs
received at destination. Names are normalised on BOTH sides — comparing a
lowercased key against a mixed-case one silently reports 0 received and
fabricates a stall."""
import json, os, subprocess, sys, threading
sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
import topology
# Read from docker-network.yml, never hardcoded: a stale partition here prints
# a row of zeros for a partition that no longer exists, which reads as a
# stalled channel and would fail the run's final check for nothing.
PARTS = [topology.scopes()[p] for p in topology.partitions()]
def q(scope):
    """The ledger as the NETWORK holds it: every node is asked and each
    field is the max across answers. One node is one point of stale truth
    (REPORTING-SPEC 1b): run 20260906T134054Z read one node whose BVN1
    executor was 348 blocks behind its siblings and reported BVN1 -> dn as
    produced 100804 against received 102177 -- the destination had received
    more than the source had sent, which is impossible, and it was the
    reader's fault. A lagging node can only under-report, so the max is the
    honest reading."""
    # Concurrently: this runs at teardown, when nodes may be wedged and every
    # one of them costs the full timeout. Serially that is nodes x timeout on
    # the final check of a twelve-hour run.
    views, lock, threads = [], threading.Lock(), []

    def ask(port):
        out = subprocess.run(["curl","-s","-m","8","-X","POST","http://localhost:%d/v3" % port,
            "-H","content-type: application/json",
            "-d",json.dumps({"jsonrpc":"2.0","id":1,"method":"query","params":{"scope":scope}})],
            capture_output=True, text=True).stdout
        try:
            res = json.loads(out).get("result",{}) or {}
            seq = (res.get("account",{}) or {}).get("sequence") or []
            # How far a stream has been sighted is derived from staging and
            # travels BESIDE the body, because a body with it written in no
            # longer hashes to the leaf its own receipt proves (#4295). The
            # body's `received` reads 0; the number is in `sighted`.
            sighted = {norm(x.get("source")): int(x.get("received") or 0)
                       for x in (res.get("sighted") or []) if x.get("source")}
            if sighted:
                seq = [dict(e, received=sighted.get(norm(e.get("url")), e.get("received") or 0))
                       for e in seq]
        except Exception: return
        with lock: views.append(seq)

    # The validators. topology.node_ports() excludes followers (#4365): this
    # is "the ledger as the NETWORK holds it", and a follower is not part of
    # the network's agreement — it only reads it.
    for port in topology.node_ports():
        t = threading.Thread(target=ask, args=(port,)); t.start(); threads.append(t)
    for t in threads: t.join()

    merged = {}
    for seq in views:
        for e in seq:
            m = merged.setdefault(norm(e.get("url")), dict(e))
            for f in ("produced", "received", "delivered"):
                m[f] = max(m.get(f) or 0, e.get(f) or 0)
    return {"sequence": list(merged.values())}
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
