#!/usr/bin/env python3
# Copyright 2026 The Accumulate Authors
#
# Use of this source code is governed by an MIT-style
# license that can be found in the LICENSE file or at
# https://opensource.org/licenses/MIT.

"""Measure block production rate against a running Accumulate network (#4099).

Height comes from the partition's system ledger — `acc://<part>.acme/ledger`,
whose `index` is the block height — not from logs and not from consensus
internals. That matters twice over: it works identically under CometBFT and
DAG-BFT, so the two are directly comparable, and it does not depend on getting
debug logging routed out of a container, which is what defeated the first
attempt to measure this.

Rate is reported over the whole sampling window rather than between adjacent
samples. Block production is bursty — a batch, then a pause — so an
instantaneous rate says more about when you looked than about the network.

Usage:
    blockrate.py --api http://localhost:26660/v3 --duration 120 --interval 5
    blockrate.py --api ... --json baseline.json     # record for before/after
"""

import argparse
import json
import subprocess
import sys
import time

DEFAULT_PARTS = ["Directory", "BVN1", "BVN2", "BVN3"]
SCOPE = {"Directory": "dn"}


def scope_for(part):
    """Ledger URL for a partition. The DN is 'dn'; BVNs are 'bvn-<ID>'."""
    return SCOPE.get(part, "bvn-%s" % part)


def call(api, method, params, timeout=10):
    body = json.dumps({"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
    try:
        out = subprocess.run(
            ["curl", "-s", "-m", str(timeout), "-X", "POST", api,
             "-H", "content-type: application/json", "-d", body],
            capture_output=True, text=True, timeout=timeout + 5,
        ).stdout
        return json.loads(out)
    except Exception:
        return None


def height(api, part):
    """Block height of a partition, or None if it cannot be read."""
    r = call(api, "query", {"scope": "acc://%s.acme/ledger" % scope_for(part)})
    try:
        return int(r["result"]["account"]["index"])
    except Exception:
        return None


def discover(api):
    """Partitions the network reports, falling back to the usual four."""
    r = call(api, "network-status", {"partition": "Directory"})
    try:
        parts = [p["id"] for p in r["result"]["network"]["partitions"]]
        return parts or DEFAULT_PARTS
    except Exception:
        return DEFAULT_PARTS


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--api", default="http://localhost:26660/v3")
    ap.add_argument("--duration", type=int, default=120, help="seconds to sample")
    ap.add_argument("--interval", type=int, default=5, help="seconds between samples")
    ap.add_argument("--partitions", help="comma-separated; default: discovered")
    ap.add_argument("--json", help="write the result to this file")
    ap.add_argument("--label", default="", help="recorded in the output, e.g. 'before #4098'")
    args = ap.parse_args()

    parts = args.partitions.split(",") if args.partitions else discover(args.api)

    first, last = {}, {}
    t0 = time.time()
    samples = 0

    print("sampling %s for %ds every %ds" % (",".join(parts), args.duration, args.interval),
          file=sys.stderr)

    while time.time() - t0 < args.duration:
        for p in parts:
            h = height(args.api, p)
            if h is None:
                continue
            first.setdefault(p, (h, time.time()))
            last[p] = (h, time.time())
        samples += 1
        # A partition that never answers is reported as unreachable rather than
        # silently omitted — an absent partition and a stalled one are different
        # findings and must not look the same.
        time.sleep(args.interval)

    result = {"label": args.label, "api": args.api, "samples": samples, "partitions": {}}
    for p in parts:
        if p not in first or p not in last:
            result["partitions"][p] = {"status": "unreachable"}
            continue
        h0, s0 = first[p]
        h1, s1 = last[p]
        span = s1 - s0
        blocks = h1 - h0
        result["partitions"][p] = {
            "status": "ok",
            "firstHeight": h0,
            "lastHeight": h1,
            "blocks": blocks,
            "seconds": round(span, 1),
            "blocksPerSec": round(blocks / span, 3) if span > 0 else None,
            "secsPerBlock": round(span / blocks, 3) if blocks > 0 else None,
        }

    print(json.dumps(result, indent=2))
    if args.json:
        with open(args.json, "w") as f:
            json.dump(result, f, indent=2)

    # Non-zero exit when nothing advanced anywhere: a stalled network is a
    # result worth failing a script over, not a zero to be averaged into a
    # dashboard.
    if all(v.get("blocks", 0) == 0 for v in result["partitions"].values()):
        print("no partition advanced during the window", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
