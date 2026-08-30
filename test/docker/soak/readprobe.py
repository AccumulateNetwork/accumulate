#!/usr/bin/env python3
# Read-back probe: does finalized data stay readable, and how fast, as the
# chain grows?
#
# Every SAMPLE_EVERY seconds it takes one recent, committed entry per
# partition — the newest entry of that partition's ledger main chain, and the
# transaction it names — and keeps it in a bounded reservoir. Every
# PROBE_EVERY seconds it re-reads a random slice of the reservoir through the
# API, timing each read, and records the round: how many reads, the median,
# the 95th percentile, and the SLOWEST read with the age (in blocks) of the
# entry it hit. On exit it writes readprobe-report.md: every round, and the
# latency by entry age over the whole run — which is the question a storage
# backend that seals one segment per block has to answer.
#
# Two kinds of read, because they land on different records:
#   chain    the entry by index   -> <chain>.Element(I), a permanent record
#   txn      the transaction by id -> Message(H).Main + Transaction(H).Status
#
# Runs beside soakmon; RUN_DIR names where readprobe.csv / .md go.
import json
import os
import random
import signal
import sys
import time
import urllib.request

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
import topology  # noqa: E402

RUN_DIR = os.environ.get("RUN_DIR", ".")
SAMPLE_EVERY = float(os.environ.get("PROBE_SAMPLE_EVERY", "20"))
PROBE_EVERY = float(os.environ.get("PROBE_EVERY", "60"))
RESERVOIR = int(os.environ.get("PROBE_RESERVOIR", "600"))
PER_ROUND = int(os.environ.get("PROBE_PER_ROUND", "150"))
TIMEOUT = 8.0

PORTS = topology.node_ports()
SCOPES = topology.scopes()  # partition -> "dn" | "bvn-BVNx"
CSV = os.path.join(RUN_DIR, "readprobe.csv")
REPORT = os.path.join(RUN_DIR, "readprobe-report.md")


def log(msg):
    print(time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()), msg, flush=True)


_rr = 0


def endpoint():
    """Round-robin over every node, so no single node's cache answers for all."""
    global _rr
    _rr += 1
    return "http://127.0.0.1:%d/v3" % PORTS[_rr % len(PORTS)]


# Why a read did not answer. A "failed" read used to be one number, and in
# run 20260829T143322Z all 528 of them were the API's query gate refusing
# under load ("query capacity exhausted") — a node-side limit, not a storage
# miss. They are told apart now.
WHY_GATED, WHY_TIMEOUT, WHY_ERROR = "gated", "timeout", "error"


def query(scope, q):
    """One API query, timed. Returns (result-or-None, ms, why-not)."""
    body = json.dumps({"jsonrpc": "2.0", "id": 1, "method": "query",
                       "params": {"scope": scope, "query": q}}).encode()
    req = urllib.request.Request(endpoint(), data=body, headers={"content-type": "application/json"})
    t0 = time.perf_counter()
    why = None
    try:
        d = json.load(urllib.request.urlopen(req, timeout=TIMEOUT))
        r = d.get("result")
        if r is None:
            msg = str((d.get("error") or {}).get("message", d.get("error", "")))
            why = WHY_GATED if "capacity" in msg or "retry later" in msg else WHY_ERROR
    except Exception as e:
        r = None
        why = WHY_TIMEOUT if "timed out" in str(e).lower() else WHY_ERROR
    return r, (time.perf_counter() - t0) * 1000.0, why


def height(partition):
    r, _, _ = query("acc://%s.acme/ledger" % SCOPES[partition], {"queryType": "default"})
    try:
        return int(r["account"]["index"])
    except Exception:
        return None


def sample(partition):
    """The newest entry of the partition ledger's main chain, and its txid."""
    scope = "acc://%s.acme/ledger" % SCOPES[partition]
    r, _, _ = query(scope, {"queryType": "chain", "name": "main",
                            "range": {"start": 0, "count": 1, "fromEnd": True, "expand": True}})
    if not (r and r.get("records")):
        # Fall back to asking the chain its length and reading the last entry.
        c, _, _ = query(scope, {"queryType": "chain", "name": "main"})
        try:
            n = int(c["count"])
        except Exception:
            return None
        r, _, _ = query(scope, {"queryType": "chain", "name": "main",
                                "range": {"start": max(0, n - 1), "count": 1, "expand": True}})
    try:
        rec = r["records"][0]
        return {"partition": partition, "scope": scope, "index": int(rec["index"]),
                "txid": rec["value"]["id"], "height": int(r.get("total") or 0),
                "sampledAt": time.time()}
    except Exception:
        return None


def read_chain(s):
    r, ms, why = query(s["scope"], {"queryType": "chain", "name": "main",
                                    "range": {"start": s["index"], "count": 1}})
    ok = bool(r and r.get("records"))
    return ok, ms, (why or (None if ok else WHY_ERROR))


def read_txn(s):
    r, ms, why = query(s["txid"], {"queryType": "default"})
    ok = bool(r and (r.get("message") or r.get("status")))
    return ok, ms, (why or (None if ok else WHY_ERROR))


def pct(xs, p):
    if not xs:
        return 0.0
    xs = sorted(xs)
    return xs[min(len(xs) - 1, int(len(xs) * p))]


class Probe:
    def __init__(self):
        self.reservoir = []
        self.seen = 0
        self.rounds = []        # per-round summaries
        self.reads = []         # every timed read: (age_blocks, ms, kind, ok, partition)
        self.heights = {}
        self.stop = False

    def take_sample(self):
        for p in SCOPES:
            s = sample(p)
            if s is None:
                continue
            self.seen += 1
            # Reservoir sampling: ages stay spread over the whole run rather
            # than the reservoir being only the newest RESERVOIR entries.
            if len(self.reservoir) < RESERVOIR:
                self.reservoir.append(s)
            else:
                j = random.randrange(self.seen)
                if j < RESERVOIR:
                    self.reservoir[j] = s

    def run_round(self):
        for p in SCOPES:
            h = height(p)
            if h is not None:
                self.heights[p] = h
        if not self.reservoir:
            return
        picks = random.sample(self.reservoir, min(PER_ROUND, len(self.reservoir)))
        got = []
        for s in picks:
            age = max(0, self.heights.get(s["partition"], 0) - s["index"])
            for kind, fn in (("chain", read_chain), ("txn", read_txn)):
                ok, ms, why = fn(s)
                rec = (age, ms, kind, ok, s["partition"], why)
                self.reads.append(rec)
                got.append(rec)
        # A gated read is the API refusing, not storage answering slowly:
        # it is counted, but its (fast) time is kept out of the latencies.
        timed = [g for g in got if g[5] != WHY_GATED]
        ms = [g[1] for g in timed] or [0.0]
        worst = max(timed, key=lambda g: g[1]) if timed else got[0]
        failed = sum(1 for g in got if not g[3] and g[5] != WHY_GATED)
        gated = sum(1 for g in got if g[5] == WHY_GATED)
        timeouts = sum(1 for g in got if g[5] == WHY_TIMEOUT)
        row = {"time": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()), "reads": len(got),
               "p50": round(pct(ms, 0.5), 1), "p95": round(pct(ms, 0.95), 1),
               "max": round(worst[1], 1), "maxKind": worst[2], "maxAge": worst[0],
               "maxPartition": worst[4], "failed": failed, "gated": gated, "timeouts": timeouts,
               "reservoir": len(self.reservoir),
               "oldestAge": max(max(0, self.heights.get(s["partition"], 0) - s["index"]) for s in self.reservoir)}
        self.rounds.append(row)
        new = not os.path.exists(CSV)
        with open(CSV, "a") as f:
            if new:
                f.write(",".join(row.keys()) + "\n")
            f.write(",".join(str(v) for v in row.values()) + "\n")
        log("round %d: %d reads p50 %.1fms p95 %.1fms max %.1fms (%s, %s, age %d blocks) failed %d gated %d timeouts %d reservoir %d oldest %d blocks"
            % (len(self.rounds), row["reads"], row["p50"], row["p95"], row["max"], worst[2], worst[4], worst[0], failed, gated, timeouts, len(self.reservoir), row["oldestAge"]))

    def report(self):
        lines = ["# Read-back probe", "",
                 "Every %ds one recent committed entry per partition joins a reservoir (cap %d); every %ds %d of them are re-read (chain entry by index, transaction by id) and timed."
                 % (SAMPLE_EVERY, RESERVOIR, PROBE_EVERY, PER_ROUND), ""]
        if self.reads:
            gated = sum(1 for r in self.reads if r[5] == WHY_GATED)
            timeouts = sum(1 for r in self.reads if r[5] == WHY_TIMEOUT)
            reads = [r for r in self.reads if r[5] != WHY_GATED]
            ms = [r[1] for r in reads] or [0.0]
            worst = max(reads, key=lambda r: r[1]) if reads else self.reads[0]
            lines += ["**Whole run:** %d timed reads, p50 %.1f ms, p95 %.1f ms, p99 %.1f ms, **max %.1f ms** (%s read, %s, entry %d blocks old); %d failed, %d timed out (%ds), %d refused by the API's query gate (not timed)."
                      % (len(ms), pct(ms, .5), pct(ms, .95), pct(ms, .99), worst[1], worst[2], worst[4], worst[0], sum(1 for r in reads if not r[3]), timeouts, int(TIMEOUT), gated), ""]
            self.reads = reads
            lines += ["## Latency by entry age", "", "| age (blocks) | reads | p50 ms | p95 ms | max ms |", "|---|---|---|---|---|"]
            buckets = [(0, 100), (100, 1000), (1000, 5000), (5000, 20000), (20000, 10**9)]
            for lo, hi in buckets:
                xs = [r[1] for r in self.reads if lo <= r[0] < hi]
                if xs:
                    lines.append("| %d–%s | %d | %.1f | %.1f | %.1f |" % (lo, "∞" if hi >= 10**9 else hi, len(xs), pct(xs, .5), pct(xs, .95), max(xs)))
            lines += ["", "## Latency by read kind", "", "| kind | reads | p50 ms | p95 ms | max ms |", "|---|---|---|---|---|"]
            for kind in ("chain", "txn"):
                xs = [r[1] for r in self.reads if r[2] == kind]
                if xs:
                    lines.append("| %s | %d | %.1f | %.1f | %.1f |" % (kind, len(xs), pct(xs, .5), pct(xs, .95), max(xs)))
            lines += ["", "## Slowest ten", "", "| ms | kind | partition | age (blocks) |", "|---|---|---|---|"]
            for r in sorted(self.reads, key=lambda r: -r[1])[:10]:
                lines.append("| %.1f | %s | %s | %d |" % (r[1], r[2], r[4], r[0]))
        lines += ["", "## Rounds", "", "| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |", "|---|---|---|---|---|---|---|---|---|---|"]
        for r in self.rounds:
            lines.append("| %s | %d | %.1f | %.1f | %.1f | %s %s age %d | %d | %d | %d | %d |"
                         % (r["time"][11:19], r["reads"], r["p50"], r["p95"], r["max"], r["maxKind"], r["maxPartition"], r["maxAge"], r["failed"], r.get("gated", 0), r.get("timeouts", 0), r["oldestAge"]))
        with open(REPORT, "w") as f:
            f.write("\n".join(lines) + "\n")
        print("\n".join(lines[:8]), flush=True)


def main():
    pr = Probe()

    def on_signal(sig, _):
        pr.stop = True
    signal.signal(signal.SIGTERM, on_signal)
    signal.signal(signal.SIGINT, on_signal)

    log("readprobe: %d endpoints, partitions %s, sample every %ds, probe every %ds" % (len(PORTS), list(SCOPES), SAMPLE_EVERY, PROBE_EVERY))
    last_sample = last_probe = 0.0
    while not pr.stop:
        now = time.time()
        if now - last_sample >= SAMPLE_EVERY:
            pr.take_sample()
            last_sample = now
        if now - last_probe >= PROBE_EVERY:
            try:
                pr.run_round()
            except Exception as e:
                log("round failed: %r" % (e,))
            last_probe = now
        time.sleep(1)
    pr.report()
    log("readprobe: done, %d rounds, report %s" % (len(pr.rounds), REPORT))


if __name__ == "__main__":
    main()
