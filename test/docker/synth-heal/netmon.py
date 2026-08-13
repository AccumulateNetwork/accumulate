#!/usr/bin/env python3
"""Live network monitor: block progress, transaction rate, and the
cross-partition synthetic matrix.

The existing test/docker dashboard tracks load-client TPS and per-node CPU but
says nothing about synthetics moving between partitions, which is what actually
matters when validating cross-partition delivery. This shows both, plus the one
derived number worth watching: a stream where delivered < received is WEDGED —
later messages arrived but cannot execute behind a hole.

  ./netmon.py                     # live view, refreshing
  ./netmon.py --once              # single snapshot
  ./netmon.py --csv out.csv       # also append samples for later analysis
  ./netmon.py --api http://localhost:27660
  ./netmon.py --serve 27700       # browser dashboard on :27700, auto-refresh
                                  # plus /json for scraping
  ./netmon.py --stats FILE        # tx load from loadgen's -stats-file
  ./netmon.py --loadgen-log FILE  # tx load parsed from a running loadgen's log
                                  # (works without restarting it for -stats-file)
"""

import argparse
import html as _html
import json
import re
import sys
import threading
import time
import urllib.request
from http.server import BaseHTTPRequestHandler, HTTPServer
from datetime import datetime, timezone

PARTS = ["dn.acme", "bvn-BVN1.acme", "bvn-BVN2.acme", "bvn-BVN3.acme"]
SHORT = {"dn.acme": "DN", "bvn-BVN1.acme": "BVN1",
         "bvn-BVN2.acme": "BVN2", "bvn-BVN3.acme": "BVN3"}


def rpc(api, method, params, timeout=20):
    body = json.dumps({"jsonrpc": "2.0", "id": 1,
                       "method": method, "params": params}).encode()
    req = urllib.request.Request(f"{api}/v3", body,
                                 {"content-type": "application/json"})
    with urllib.request.urlopen(req, timeout=timeout) as f:
        return json.load(f)


def load_stats(path):
    """Transaction load as reported by loadgen's -stats-file, if present.

    Chain-side block rate says how fast the network is moving; this says how
    hard it is being driven and whether anything is being rejected. Both are
    needed — a healthy block rate under no load proves nothing.
    """
    if not path:
        return None
    try:
        with open(path) as f:
            return json.load(f)
    except Exception:
        return None


LG_RE = re.compile(
    r"sent=(?P<sent>\d+)/(?P<target>\d+)\s+rejected=(?P<rejected>\d+)\s+"
    r"skipped=(?P<skipped>\d+)\s+rate=(?P<rate>[\d.]+)/s"
    r"(?:.*?adis=(?P<adis>\d+)\s+books=(?P<books>\d+)\s+pages=(?P<pages>\d+)"
    r"\s+tokens=(?P<tokens>\d+)\s+accounts=(?P<accounts>\d+))?")


def loadgen_log_stats(path):
    """Parse the last progress line of a running loadgen's log.

    loadgen prints sent/rejected/skipped/rate every minute, so a soak already
    in flight can be observed without restarting it just to add -stats-file.
    """
    if not path:
        return None
    try:
        with open(path, "rb") as f:
            try:
                f.seek(-65536, 2)
            except OSError:
                f.seek(0)
            tail = f.read().decode("utf-8", "replace")
    except Exception:
        return None
    last = None
    for m in LG_RE.finditer(tail):
        last = m
    if not last:
        return None
    g = last.groupdict()
    out = {
        "generated": int(g["sent"]),
        "target": int(g["target"]),
        "rejected": int(g["rejected"]),
        "skipped": int(g["skipped"]),
        "rate": float(g["rate"]),
        "phase": "generating",
    }
    if g.get("accounts"):
        out["accounts"] = {
            "identities": int(g["adis"]), "keyBooks": int(g["books"]),
            "keyPages": int(g["pages"]), "tokenIssuers": int(g["tokens"]),
            "accounts": int(g["accounts"]),
        }
    return out


def sample(api):
    """One observation: block height per partition and every synthetic stream."""
    out = {"t": time.time(), "blocks": {}, "streams": {}, "err": None}
    for p in PARTS:
        try:
            r = rpc(api, "query", {"scope": f"acc://{p}/ledger"})
            out["blocks"][p] = r.get("result", {}).get("account", {}).get("index", 0)
        except Exception as e:
            out["blocks"][p] = None
            out["err"] = str(e)
        try:
            a = rpc(api, "query", {"scope": f"acc://{p}/synthetic"})
            a = a.get("result", {}).get("account", {})
            for s in a.get("sequence", []) or []:
                src = s.get("url", "?").replace("acc://", "")
                # keyed destination <- source; produced is what THIS partition
                # produced FOR src, received/delivered are inbound FROM src
                out["streams"][(p, src)] = (s.get("produced", 0),
                                            s.get("received", 0),
                                            s.get("delivered", 0))
        except Exception as e:
            out["err"] = str(e)
    return out


def render(cur, prev, api, stats=None):
    now = datetime.now(timezone.utc).strftime("%H:%M:%S")
    dt = (cur["t"] - prev["t"]) if prev else 0

    lines = []
    lines.append(f"  Accumulate network monitor — {api}   {now}Z")
    lines.append("")

    # Transaction load (client side). Rate is loadgen's cumulative average.
    if stats:
        gen = stats.get("generated", 0)
        rej = stats.get("rejected", 0)
        skip = stats.get("skipped", 0)
        rate = stats.get("rate", 0.0)
        target = stats.get("targetTps", 0.0)
        phase = stats.get("phase", "?")
        el = stats.get("elapsedSec", 0)
        lines.append("  TRANSACTION LOAD")
        lines.append(f"    phase {phase:<11} elapsed {el//3600}h{(el%3600)//60:02d}m")
        tgt = stats.get("target") or 0
        pct = f"  ({gen*100//tgt}% of {tgt})" if tgt else ""
        tps_target = f"   target {target:.2f} tx/s" if target else ""
        lines.append(f"    submitted {gen:<10} rate {rate:5.2f} tx/s{tps_target}{pct}")
        flag = "  <-- REJECTS" if rej else ""
        lines.append(f"    rejected  {rej:<10} skipped {skip}{flag}")
        acc = stats.get("accounts", {}) or {}
        if acc:
            lines.append(f"    accounts  identities={acc.get('identities',0)} "
                         f"books={acc.get('keyBooks',0)} pages={acc.get('keyPages',0)} "
                         f"accts={acc.get('accounts',0)}")
        # Every type the generator can emit, not a top-N: a type sitting at
        # zero is the interesting signal (it means that path never exercised).
        per = sorted((stats.get("perType") or {}).items())
        if per:
            lines.append("")
            lines.append(f"    {'transaction type':<28}{'sent':>8}{'rejected':>10}{'skipped':>9}")
            for name, v in per:
                g = v.get("generated", 0)
                r = v.get("rejected", 0)
                s = v.get("skipped", 0)
                mark = "  <--" if r else ""
                lines.append(f"    {name:<28}{g:>8}{r:>10}{s:>9}{mark}")
            lines.append(f"    {'TOTAL':<28}{gen:>8}{rej:>10}{skip:>9}")
        lines.append("")

    # Blocks and block rate
    lines.append("  BLOCKS")
    lines.append("    partition      height        rate")
    for p in PARTS:
        h = cur["blocks"].get(p)
        if h is None:
            lines.append(f"    {SHORT[p]:<12}   {'unreachable':>10}")
            continue
        rate = ""
        if prev and dt > 0 and prev["blocks"].get(p) is not None:
            d = h - prev["blocks"][p]
            rate = f"{d/dt*60:6.1f} blk/min"
        lines.append(f"    {SHORT[p]:<12}   {h:>10}   {rate}")
    lines.append("")

    # Cross-partition synthetic matrix. Only inbound rows matter for wedges.
    lines.append("  SYNTHETIC STREAMS  (destination <- source)")
    lines.append("    stream            produced  received  delivered   in-flight   rate")
    wedged = []
    for (dst, src), (prod, recv, deliv) in sorted(cur["streams"].items()):
        inflight = recv - deliv
        flag = ""
        if inflight > 0:
            flag = "  <-- WEDGED"
            wedged.append((dst, src, recv, deliv))
        rate = ""
        if prev and dt > 0 and (dst, src) in prev["streams"]:
            d = deliv - prev["streams"][(dst, src)][2]
            if d:
                rate = f"{d/dt*60:5.1f}/min"
        d_short = SHORT.get(dst, dst)
        s_short = SHORT.get(src, src)
        lines.append(f"    {d_short:>4} <- {s_short:<8}  {prod:>8}  {recv:>8}  {deliv:>9}   "
                     f"{inflight:>9}   {rate}{flag}")
    lines.append("")

    total_deliv = sum(v[2] for v in cur["streams"].values())
    if prev:
        prev_deliv = sum(v[2] for v in prev["streams"].values())
        if dt > 0:
            lines.append(f"  synthetic delivery rate: {(total_deliv-prev_deliv)/dt*60:.1f}/min"
                         f"    total delivered: {total_deliv}")
    else:
        lines.append(f"  total delivered: {total_deliv}")

    if wedged:
        lines.append("")
        lines.append(f"  *** {len(wedged)} WEDGED STREAM(S) — a hole is blocking delivery ***")
        for dst, src, recv, deliv in wedged:
            lines.append(f"      {SHORT.get(dst,dst)} <- {SHORT.get(src,src)}: "
                         f"received={recv} delivered={deliv} (gap {recv-deliv})")
    if cur["err"]:
        lines.append(f"  query error: {cur['err'][:80]}")
    return "\n".join(lines), len(wedged), total_deliv


PAGE = """<!doctype html><meta charset=utf-8>
<meta http-equiv=refresh content="{interval}">
<title>Accumulate network monitor</title>
<style>
 body{{background:#0f1115;color:#d7dae0;font:13px/1.5 ui-monospace,Menlo,Consolas,monospace;margin:0;padding:20px}}
 h1{{font-size:15px;font-weight:600;color:#8ab4f8;margin:0 0 14px}}
 pre{{margin:0;white-space:pre}}
 .wedge{{color:#ff6b6b;font-weight:700}}
 a{{color:#8ab4f8}}
</style>
<h1>Accumulate network monitor <span style="color:#6b7280">&mdash; {api}</span></h1>
<pre>{body}</pre>
<p style="color:#6b7280">refreshing every {interval}s &middot; <a href="/json">/json</a></p>
"""


class _Handler(BaseHTTPRequestHandler):
    state = {"text": "starting...", "wedged": 0, "total": 0, "raw": {}}
    api = ""
    interval = 10

    def log_message(self, *a):
        pass  # runs unattended; keep the console clean

    def do_GET(self):
        if self.path.startswith("/json"):
            payload = json.dumps({
                "wedged_streams": self.state["wedged"],
                "total_delivered": self.state["total"],
                "blocks": self.state["raw"].get("blocks", {}),
                "streams": {f"{d}<-{s}": v for (d, s), v
                            in self.state["raw"].get("streams", {}).items()},
            }, indent=2).encode()
            self.send_response(200)
            self.send_header("content-type", "application/json")
            self.send_header("content-length", str(len(payload)))
            self.end_headers()
            self.wfile.write(payload)
            return
        body = _html.escape(self.state["text"])
        body = body.replace("&lt;-- WEDGED", "<span class=wedge>&lt;-- WEDGED</span>")
        page = PAGE.format(api=_html.escape(self.api), body=body,
                           interval=self.interval).encode()
        self.send_response(200)
        self.send_header("content-type", "text/html; charset=utf-8")
        self.send_header("content-length", str(len(page)))
        self.end_headers()
        self.wfile.write(page)


def serve(api, port, interval, stats_path=None, lg_log=None):
    _Handler.api, _Handler.interval = api, interval

    def poll():
        prev = None
        while True:
            cur = sample(api)
            text, wedged, total = render(cur, prev, api,
                                         load_stats(stats_path) or loadgen_log_stats(lg_log))
            _Handler.state = {"text": text, "wedged": wedged,
                              "total": total, "raw": cur}
            prev = cur
            time.sleep(interval)

    threading.Thread(target=poll, daemon=True).start()
    srv = HTTPServer(("0.0.0.0", port), _Handler)
    print(f"network monitor on http://localhost:{port}  (json: /json)", flush=True)
    srv.serve_forever()


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--api", default="http://localhost:27660")
    ap.add_argument("--interval", type=float, default=10)
    ap.add_argument("--once", action="store_true")
    ap.add_argument("--csv")
    ap.add_argument("--serve", type=int, metavar="PORT",
                    help="serve a browser dashboard on PORT")
    ap.add_argument("--stats", help="loadgen -stats-file to read tx load from")
    ap.add_argument("--loadgen-log", help="running loadgen log to parse tx load from")
    args = ap.parse_args()

    stats_src = (lambda: load_stats(args.stats) or loadgen_log_stats(args.loadgen_log))
    if args.serve:
        return serve(args.api, args.serve, args.interval, args.stats, args.loadgen_log)

    csvf = None
    if args.csv:
        new = True
        try:
            open(args.csv).close()
            new = False
        except OSError:
            pass
        csvf = open(args.csv, "a")
        if new:
            csvf.write("ts,dn_block,bvn1_block,bvn2_block,bvn3_block,"
                       "total_delivered,wedged_streams\n")
            csvf.flush()

    prev = None
    try:
        while True:
            cur = sample(args.api)
            text, wedged, total = render(cur, prev, args.api, stats_src())
            if not args.once:
                print("\033[2J\033[H", end="")
            print(text, flush=True)
            if csvf:
                b = cur["blocks"]
                csvf.write("{},{},{},{},{},{},{}\n".format(
                    datetime.now(timezone.utc).strftime("%FT%TZ"),
                    b.get("dn.acme"), b.get("bvn-BVN1.acme"),
                    b.get("bvn-BVN2.acme"), b.get("bvn-BVN3.acme"),
                    total, wedged))
                csvf.flush()
            prev = cur
            if args.once:
                return 2 if wedged else 0
            time.sleep(args.interval)
    except KeyboardInterrupt:
        return 0


if __name__ == "__main__":
    sys.exit(main())
