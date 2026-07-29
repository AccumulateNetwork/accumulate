#!/usr/bin/env python3
# Rich monitor for the 1-validator (new/Jiuquan) + 6-follower (old/v1.4.4.2) test.
#
# Divergence (the point of the test) comes from ALL nodes: CometBFT height +
# app_hash per node, flagged if any two nodes at a shared height disagree.
# Everything else — heals, flows, drops-by-destination, held synthetics, stuck —
# is sourced from the VALIDATOR (acc-n1), the only node with the new binary's full
# crosschain instrumentation (the old followers can't supply it). Loadgen tps /
# account growth from the stats file. Serves http://127.0.0.1:8099.
import json, os, re, subprocess, threading, time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

NODES = [f"acc-n{i}" for i in range(1, 8)]
VAL = "acc-n1"
STATS = "/tmp/claude-1000/-home-paul-repos-gitlab-com-accumulatenetwork-accumulate/5cc8039a-bc9b-4d29-87ac-9edc0e6df199/scratchpad/oneval/stats.json"
PROM = re.compile(r'^(\w+)(?:\{([^}]*)\})?\s+([-0-9.eE+]+)')
STATE = {}
LOCK = threading.Lock()


def sh(a, t=8):
    try:
        return subprocess.run(a, capture_output=True, text=True, timeout=t).stdout
    except Exception:
        return ""


def cexec(c, cmd, t=8):
    return sh(["docker", "exec", c, "sh", "-c", cmd], t)


def prom(text):
    for ln in text.splitlines():
        if not ln or ln[0] == "#":
            continue
        m = PROM.match(ln)
        if not m:
            continue
        lbl = dict(re.findall(r'(\w+)="([^"]*)"', m.group(2) or ""))
        try:
            yield m.group(1), lbl, float(m.group(3))
        except ValueError:
            pass


def collect():
    while True:
        # per-node height + app_hash (divergence)
        rows = []
        for i, c in enumerate(NODES, 1):
            h = ah = None
            try:
                s = json.loads(cexec(c, "curl -s http://localhost:26657/status"))["result"]["sync_info"]
                h, ah = s.get("latest_block_height"), s.get("latest_app_hash", "")
            except Exception:
                pass
            rows.append({"node": c, "role": "VALIDATOR (new/Jiuquan)" if i == 1 else "follower (old/1.4.4.2)",
                         "height": h or "DOWN", "app_hash": ah or ""})
        byh = {}
        for r in rows:
            if r["height"] not in ("DOWN", None):
                byh.setdefault(r["height"], set()).add(r["app_hash"])
        diverged = any(len(v) > 1 for v in byh.values())

        # validator's rich metrics
        heals = {"synthetic": 0, "anchor": 0}
        deferred = stuck = 0
        drops = {"synthetic": 0, "anchor": 0, "byDest": {}}
        flows = {}
        for name, lbl, v in prom(cexec(VAL, "curl -s http://localhost:26670/metrics", 10)):
            iv = int(v)
            if name == "accumulate_crosschain_heals_total":
                heals[lbl.get("type", "")] = heals.get(lbl.get("type", ""), 0) + iv
            elif name == "accumulate_crosschain_heal_deferred_total":
                deferred += iv
            elif name == "accumulate_crosschain_heal_stuck_tries":
                stuck = max(stuck, iv)
            elif name == "accumulate_debug_dropped_total":
                k = lbl.get("kind", "")
                drops[k] = drops.get(k, 0) + iv
                d = lbl.get("destination", "?")
                drops["byDest"][d] = drops["byDest"].get(d, 0) + iv
            elif name == "accumulate_crosschain_sequence":
                key = (lbl.get("type"), lbl.get("src"), lbl.get("dst"))
                flows.setdefault(key, {})[lbl.get("field")] = iv
        drops["total"] = drops.get("synthetic", 0) + drops.get("anchor", 0)

        # held synthetics on the validator (the divergence trigger) — bounded log tail
        logs = sh(["docker", "logs", "--tail", "60000", VAL], 15)
        held = sum(logs.count(k) for k in ("hold synthetic", "SyntheticForAnchor"))
        rejected = logs.count("Synthetic message rejected")

        # loadgen
        lg = {}
        try:
            d = json.load(open(STATS))
            lg = {"generated": d.get("generated"), "rejected": d.get("rejected"),
                  "rate": round(d.get("rate", 0), 2), "accounts": (d.get("accounts") or {}).get("accounts")}
        except Exception:
            pass

        # synthetic flow gaps (received - delivered) — where a hold/wedge would show
        gaps = []
        for (kind, src, dst), c in flows.items():
            if kind != "synthetic":
                continue
            g = (c.get("received", 0)) - (c.get("delivered", 0))
            if g > 0:
                gaps.append((f"{src}->{dst}", g, c.get("received", 0), c.get("delivered", 0)))
        gaps.sort(key=lambda x: -x[1])

        with LOCK:
            STATE.update(nodes=rows, diverged=diverged, heals=heals, deferred=deferred,
                         stuck=stuck, drops=drops, held=held, rejected=rejected, lg=lg,
                         gaps=gaps[:8], ts=int(time.time()))
        time.sleep(4)


def page():
    with LOCK:
        s = dict(STATE)
    if not s:
        return "<html><body style='background:#111;color:#ddd'>starting…</body></html>"
    nrows = "".join(
        f"<tr style='{'background:#3a2a00' if r['node']==VAL else ''}'><td>{r['node']}</td><td>{r['role']}</td>"
        f"<td style='text-align:right'>{r['height']}</td><td style='font-family:monospace'>{r['app_hash'][:28]}</td></tr>"
        for r in s["nodes"])
    if s["diverged"]:
        banner = "<div style='background:#7a1010;padding:14px;font-size:22px'>&#9888; DIVERGENCE &mdash; a follower's app_hash differs from the validator (the mix broke the network)</div>"
    else:
        banner = "<div style='background:#0a5a1a;padding:14px;font-size:22px'>&#10003; IN LOCKSTEP &mdash; validator and all followers agree on app_hash</div>"
    dd = s["drops"]["byDest"]
    drops_by = ", ".join(f"{k}={v}" for k, v in sorted(dd.items())) or "none"
    gaps = "".join(f"<tr><td>{g[0]}</td><td style='text-align:right'>{g[1]}</td><td style='text-align:right'>{g[2]}/{g[3]}</td></tr>" for g in s["gaps"]) or "<tr><td colspan=3 style='color:#888'>none</td></tr>"
    lg = s["lg"]
    age = int(time.time()) - s["ts"]
    heldstyle = "color:#ff6" if s["held"] == 0 else "color:#f66;font-weight:bold"
    return f"""<!doctype html><html><head><meta http-equiv=refresh content=3>
<style>body{{background:#111;color:#ddd;font-family:sans-serif;margin:18px}}
table{{border-collapse:collapse;margin:6px 0}}td,th{{border:1px solid #333;padding:6px 10px}}th{{background:#222;text-align:left}}
.grid{{display:flex;gap:30px;flex-wrap:wrap}} h3{{margin:14px 0 4px}}</style></head><body>
<h2>1 validator (Jiuquan, new) vs 6 followers (old v1.4.4.2)</h2>
{banner}
<p><b style='{heldstyle}'>held-synthetic events on validator: {s['held']}</b> &mdash; this is the #4070 trigger; divergence can only occur once this is &gt; 0.
&nbsp; rejected-synthetics: {s['rejected']} &nbsp;<span style='color:#888'>(data {age}s old)</span></p>
<div class=grid>
<div><h3>nodes / divergence</h3><table><tr><th>node</th><th>role</th><th>height</th><th>app_hash</th></tr>{nrows}</table></div>
<div><h3>validator healing</h3><table>
<tr><td>synthetic heals</td><td style='text-align:right'>{s['heals'].get('synthetic',0)}</td></tr>
<tr><td>anchor heals</td><td style='text-align:right'>{s['heals'].get('anchor',0)}</td></tr>
<tr><td>deferred</td><td style='text-align:right'>{s['deferred']}</td></tr>
<tr><td>stuck (max tries)</td><td style='text-align:right'>{s['stuck']}</td></tr></table>
<h3>induced drops (validator)</h3><table>
<tr><td>synthetic / anchor / total</td><td>{s['drops'].get('synthetic',0)} / {s['drops'].get('anchor',0)} / {s['drops'].get('total',0)}</td></tr>
<tr><td>by destination</td><td>{drops_by}</td></tr></table></div>
<div><h3>synthetic flow gaps (recv-deliv)</h3><table><tr><th>stream</th><th>gap</th><th>recv/deliv</th></tr>{gaps}</table></div>
<div><h3>loadgen</h3><table>
<tr><td>rate</td><td style='text-align:right'>{lg.get('rate','?')} tps</td></tr>
<tr><td>generated</td><td style='text-align:right'>{lg.get('generated','?')}</td></tr>
<tr><td>rejected</td><td style='text-align:right'>{lg.get('rejected','?')}</td></tr>
<tr><td>accounts</td><td style='text-align:right'>{lg.get('accounts','?')}</td></tr></table></div>
</div></body></html>"""


class H(BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass

    def do_GET(self):
        b = page().encode()
        self.send_response(200)
        self.send_header("content-type", "text/html")
        self.send_header("content-length", str(len(b)))
        self.end_headers()
        self.wfile.write(b)


threading.Thread(target=collect, daemon=True).start()
print("rich monitor on http://127.0.0.1:8099")
ThreadingHTTPServer(("127.0.0.1", 8099), H).serve_forever()
