#!/usr/bin/env python3
# Live web monitor for the synthetic-healing soak (#4064/#4067).
#
# Serves a self-refreshing dashboard on http://127.0.0.1:8099 showing, for the
# running soak:
#   - the loadgen transaction-type mix (generated / rejected / skipped, live)
#   - wedge counts (dropped synthetics and anchors) with a per-destination and
#     per-node breakdown, parsed from the nodes' own logs
#   - healing counts (syntheticHeals / anchorHeals) per partition and per node,
#     queried from each validator's ConsensusStatus
#   - account growth, network height, chaos events, and short time-series
#
# Data sources are the soak's own artefacts and the live network; nothing here
# influences the run. Read-only.
#
#   ./soakmon.py                 # then open http://127.0.0.1:8099
#   PORT=9000 ./soakmon.py
import json, os, re, subprocess, threading, time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

HERE = os.path.dirname(os.path.abspath(__file__))
COMPOSE = os.path.join(HERE, "docker-compose.yml")

# soak.sh writes each run's output to runs/<timestamp>/ and passes that path in
# RUN_DIR. Falling back to HERE keeps the monitor usable standalone, but when a
# run is live its stats and chaos log are NOT here — reading the old fixed
# locations is why the dashboard showed loadgen: null and no chaos events.
RUN_DIR = os.environ.get("RUN_DIR") or HERE
STATS = os.path.join(RUN_DIR, "loadgen-stats.json")
CHAOS = os.path.join(RUN_DIR, "chaos.log")
API = "http://localhost:26660/v3"
PORT = int(os.environ.get("PORT", "8099"))

# Refresh cadences (seconds): cheap things often, docker-heavy things rarely.
I_STATS, I_HEIGHT, I_WEDGE, I_HEAL, I_CHAOS, I_FLOW = 1, 1, 5, 5, 5, 1
HIST_MAX = 600  # ~10 min of 1s ticks kept for the sparklines' recent window

STATE = {"ok": False, "started": int(time.time())}
LOCK = threading.Lock()
_peerid = {}  # container -> peerID (stable per node identity; cached)


def sh(args, timeout=25):
    try:
        return subprocess.run(args, capture_output=True, text=True, timeout=timeout).stdout
    except Exception:
        return ""


def curl_api(method, params):
    body = json.dumps({"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
    out = sh(["curl", "-s", "-m", "8", "-X", "POST", API,
              "-H", "content-type: application/json", "-d", body], timeout=12)
    try:
        return json.loads(out)
    except Exception:
        return None


def containers():
    out = sh(["docker", "ps", "--filter", "name=acc-bvn", "--format", "{{.Names}}"])
    return sorted(n for n in out.split() if n.startswith("acc-bvn"))


def read_stats():
    try:
        with open(STATS) as f:
            d = json.load(f)
        d["stale"] = (time.time() - d.get("updatedUnix", 0)) > 20
        return d
    except Exception:
        return None


def collect_height():
    r = curl_api("query", {"scope": "acc://dn.acme/ledger"})
    h = None
    try:
        # the DN system-ledger's index is the DN block height
        h = int(r["result"]["account"]["index"])
    except Exception:
        pass
    up = curl_api("network-status", {"partition": "Directory"}) is not None
    return {"api": "up" if up else "down", "dnHeight": h}


ANSI = re.compile(r"\x1b\[[0-9;]*m")


PARTITIONS = ["Directory", "BVN1", "BVN2", "BVN3"]
SCOPE = {"Directory": "dn", "BVN1": "bvn-BVN1", "BVN2": "bvn-BVN2", "BVN3": "bvn-BVN3"}


def _plabel(u):
    h = u.split("//")[1].split(".")[0]
    return {"dn": "Directory"}.get(h, h.replace("bvn-", ""))


def collect_heights():
    heights = {}
    for p in PARTITIONS:
        r = curl_api("query", {"scope": "acc://%s.acme/ledger" % SCOPE[p]})
        try:
            heights[p] = int(r["result"]["account"]["index"])
        except Exception:
            heights[p] = None
    return heights


# --- Prometheus scrape (authoritative source: node /metrics) -----------------
METRICS_PORT = 26670
NS = "accumulate"
PROM_LINE = re.compile(r'^([a-zA-Z_:][\w:]*)(?:\{([^}]*)\})?\s+([-0-9.eE+]+)')


def parse_prom(text):
    for line in text.splitlines():
        if not line or line[0] == "#":
            continue
        m = PROM_LINE.match(line)
        if not m:
            continue
        name, lbls, val = m.group(1), m.group(2) or "", m.group(3)
        labels = dict(re.findall(r'(\w+)="([^"]*)"', lbls))
        try:
            yield name, labels, float(val)
        except ValueError:
            continue


def _scrape_one(c, out, lock):
    txt = sh(["docker", "exec", c, "sh", "-c",
              "curl -s -m5 http://localhost:%d/metrics" % METRICS_PORT], timeout=20)
    rows = list(parse_prom(txt))
    with lock:
        out[c] = rows


def collect_metrics():
    # Scrape every node's /metrics and aggregate. Counters (heals/drops) sum
    # across a partition's validators; gauges (sequence) take the max, since all
    # validators of a partition report the same ledger state. Each src->dst pair
    # is labelled by its true direction, so produced (from src) and
    # received/delivered (from dst) merge cleanly on the same key.
    cs = containers()
    per, threads, lock = {}, [], threading.Lock()
    for c in cs:
        t = threading.Thread(target=_scrape_one, args=(c, per, lock))
        t.start(); threads.append(t)
    for t in threads:
        t.join()

    heals = {"synthetic": 0, "anchor": 0, "deferred": 0, "errors": 0, "focus": 0, "stuck": 0, "stuckStream": "", "byPartition": {}}
    drops = {"synthetic": 0, "anchor": 0, "byDest": {p: 0 for p in PARTITIONS}}
    heal_types = set()
    seq = {}

    def pslot(p):
        return heals["byPartition"].setdefault(p, {"synthetic": 0, "anchor": 0, "deferred": 0, "errors": 0, "focus": 0})

    def N(s):
        return NS + "_" + s

    for rows in per.values():
        for name, lab, v in rows:
            iv = int(v)
            if name == N("crosschain_heals_total"):
                # Tolerate heal types this script has never heard of. The label
                # set is owned by the node, not by the monitor: #4087 added
                # "synthetic-range" and "anchor-range", and indexing a fixed dict
                # killed the collector thread outright with a KeyError. The
                # dashboard then served its last good sample forever — reporting a
                # frozen height and a frozen heal count while the network ran on,
                # which reads exactly like a seizure and hides a real one.
                t = lab.get("type", "")
                heals[t] = heals.get(t, 0) + iv
                heal_types.add(t)
                s = pslot(lab.get("partition", ""))
                s[t] = s.get(t, 0) + iv
            elif name == N("crosschain_heal_deferred_total"):
                heals["deferred"] += iv; pslot(lab.get("partition", ""))["deferred"] += iv
            elif name == N("crosschain_heal_errors_total"):
                heals["errors"] += iv; pslot(lab.get("partition", ""))["errors"] += iv
            elif name == N("crosschain_heal_focus_total"):
                heals["focus"] += iv; pslot(lab.get("partition", ""))["focus"] += iv
            elif name == N("crosschain_heal_stuck_tries"):
                if iv > heals["stuck"]:
                    heals["stuck"] = iv
                    heals["stuckStream"] = "%s<-%s" % (lab.get("partition", "?"), lab.get("remote", "?"))
            elif name == N("debug_dropped_total"):
                k = lab.get("kind", ""); drops[k] = drops.get(k, 0) + iv
                d = lab.get("destination", "?"); drops["byDest"][d] = drops["byDest"].get(d, 0) + iv
            elif name == N("crosschain_sequence"):
                cell = seq.setdefault((lab.get("type"), lab.get("src"), lab.get("dst")), {})
                f = lab.get("field", "")
                cell[f] = max(cell.get(f, 0), iv)

    # Per-node size. Reported as min/avg/max rather than a single number because
    # the spread is the interesting part: nodes are restarted by chaos at
    # different times, so a fleet average hides both the freshly-started node and
    # the one that has been up longest. Goroutines ride along because an
    # unbounded goroutine count is what #4089 looked like before anyone noticed
    # the memory — and it shows up there hours earlier than RSS does.
    nodes = {"count": 0, "rssMinMiB": 0, "rssAvgMiB": 0, "rssMaxMiB": 0, "rssMaxNode": "",
             "grMin": 0, "grAvg": 0, "grMax": 0, "grMaxNode": "", "byNode": {}}
    rss, gor = {}, {}
    for c, rows in per.items():
        for name, lab, v in rows:
            if name == "process_resident_memory_bytes":
                rss[c] = float(v) / 1048576.0
            elif name == "go_goroutines":
                gor[c] = int(float(v))
    if rss:
        vals = sorted(rss.values())
        nodes["count"] = len(vals)
        nodes["rssMinMiB"] = round(vals[0])
        nodes["rssAvgMiB"] = round(sum(vals) / len(vals))
        nodes["rssMaxMiB"] = round(vals[-1])
        nodes["rssMaxNode"] = max(rss, key=rss.get)
    if gor:
        gv = sorted(gor.values())
        nodes["grMin"], nodes["grMax"] = gv[0], gv[-1]
        nodes["grAvg"] = round(sum(gv) / len(gv))
        nodes["grMaxNode"] = max(gor, key=gor.get)
    for c in sorted(set(rss) | set(gor)):
        nodes["byNode"][c] = {"rssMiB": round(rss.get(c, 0)), "goroutines": gor.get(c, 0)}

    # Sum the heal TYPES actually seen on heals_total, not a fixed pair, so a
    # recovery path added later cannot go uncounted. Deliberately not a sum over
    # `heals`, which also holds deferred/errors/focus/stuck — those are not heals.
    heals["total"] = sum(heals.get(t, 0) for t in heal_types)
    drops["total"] = drops["synthetic"] + drops["anchor"]
    flows = {"synthetic": {}, "anchor": {}}
    for (kind, src, dst), cell in seq.items():
        if kind not in flows or src not in PARTITIONS or dst not in PARTITIONS:
            continue
        flows[kind].setdefault(src, {})[dst] = {
            "sent": cell.get("produced", 0), "recv": cell.get("received", 0), "deliv": cell.get("delivered", 0)}
    # Network-wide produced totals; their time-derivative is the tx production
    # rate (synthetics/anchors emitted per second across all partition pairs).
    syn_prod = sum(c.get("produced", 0) for (k, _, _), c in seq.items() if k == "synthetic")
    anc_prod = sum(c.get("produced", 0) for (k, _, _), c in seq.items() if k == "anchor")

    # The accumulate_crosschain_sequence gauge exists only on the dagbft
    # lineage. On the release lineage the nodes serve heals_total and nothing
    # else, so the flow matrix comes back EMPTY — and an empty matrix reads as
    # "no gaps anywhere", which is indistinguishable from healthy and silently
    # disarms seizewatch. Fall back to the ledgers over the API, which carry the
    # same produced/received/delivered per source on every lineage.
    if not flows["synthetic"] and not flows["anchor"]:
        af, asp, aap = collect_flows_api()
        if af["synthetic"] or af["anchor"]:
            flows, syn_prod, anc_prod = af, asp, aap

    return {"heals": heals, "wedges": drops, "flows": flows,
            "synProduced": syn_prod, "ancProduced": anc_prod, "nodeStats": nodes,
            "nodes": len(cs), "scraped": sum(1 for r in per.values() if r)}


def collect_flows_api():
    # Flow matrix from the ledgers, for lineages that do not emit the
    # accumulate_crosschain_sequence gauge. Each partition's synthetic/anchor
    # ledger holds one entry per remote partition carrying BOTH directions:
    # `produced` counts what THIS partition sent to the remote, while
    # `received`/`delivered` count what the remote sent to THIS one. So a single
    # pass over all four ledgers fills every src->dst cell.
    flows = {"synthetic": {}, "anchor": {}}
    syn_prod = anc_prod = 0

    def cell(kind, src, dst):
        return flows[kind].setdefault(src, {}).setdefault(
            dst, {"sent": 0, "recv": 0, "deliv": 0, "pending": 0})

    for kind, path in (("synthetic", "synthetic"), ("anchor", "anchors")):
        for dst in PARTITIONS:
            r = curl_api("query", {"scope": "acc://%s.acme/%s" % (SCOPE[dst], path)})
            try:
                seq = r["result"]["account"]["sequence"] or []
            except Exception:
                continue
            for e in seq:
                url = e.get("url") or ""
                if not url:
                    continue
                try:
                    remote = _plabel(url)
                except Exception:
                    continue
                if remote not in PARTITIONS:
                    continue
                # remote -> dst
                inbound = cell(kind, remote, dst)
                inbound["recv"] = max(inbound["recv"], int(e.get("received") or 0))
                inbound["deliv"] = max(inbound["deliv"], int(e.get("delivered") or 0))
                inbound["pending"] = max(inbound["pending"], len(e.get("pending") or []))
                # dst -> remote
                produced = int(e.get("produced") or 0)
                outbound = cell(kind, dst, remote)
                outbound["sent"] = max(outbound["sent"], produced)
                if kind == "synthetic":
                    syn_prod += produced
                else:
                    anc_prod += produced

    # undeliv = produced at the source minus received at the destination. This is
    # the ONLY signal that catches a missing PREFIX or a trailing drop: when the
    # very first messages of a stream are lost and nothing follows them, the
    # destination never forms a pending window, so recv == deliv == 0 and the
    # recv-deliv gap stays 0 forever while the messages are gone. A 23h soak ran
    # with DN->BVN1 stuck at produced=2 received=0 and every gap-based check
    # reported healthy.
    for kind in flows:
        for src, row in flows[kind].items():
            for dst, c in row.items():
                c["undeliv"] = max(0, c.get("sent", 0) - c.get("recv", 0))

    # Anchor "sent" from the source's anchor-sequence CHAIN height. The anchor
    # ledger has no outbound `produced` writer on any lineage (checked 2026-08:
    # no branch writes it), so reading it yields the impossible display
    # "received 18 / sent 0" — received is PROOF of sent. The sequence chain is
    # the actual bookkeeping: a BVN anchors only to the DN, and the DN sends
    # the same sequence to every BVN, so the chain height IS the sent count.
    for src in PARTITIONS:
        r = curl_api("query", {"scope": "acc://%s.acme/anchors" % SCOPE[src],
                               "query": {"queryType": "chain", "name": "anchor-sequence"}})
        try:
            h = int(r["result"]["total"])
        except Exception:
            continue
        if h <= 0:
            continue
        dsts = ["Directory"] if src != "Directory" else [p for p in PARTITIONS if p != "Directory"]
        for dst in dsts:
            c = cell("anchor", src, dst)
            c["sent"] = max(c["sent"], h)
            anc_prod = max(anc_prod, h)

    # No impossible states: delivery is proof of sending, so sent is bounded
    # below by both received and delivered (REPORTING-SPEC.md 1a). If an
    # instrument ever disagrees with that inference, the bound wins and the
    # cell is marked inferred so the broken instrument is visible.
    for kind in flows:
        for src, row in flows[kind].items():
            for dst, c in row.items():
                floor = max(c.get("recv", 0), c.get("deliv", 0))
                if c.get("sent", 0) < floor:
                    c["sent"] = floor
                    c["inferred"] = True

    # Self-streams stay in the matrix. They are real sequenced streams that
    # carry real traffic (measured: BVN2->BVN2 produced=1 delivered=1 in the
    # #4103 bisection) and can wedge like any other; deleting the diagonal as
    # "bookkeeping" made a whole class of wedge invisible (REPORTING-SPEC 1a).
    return flows, syn_prod, anc_prod


def collect_wedges():
    out = sh(["docker", "compose", "-f", COMPOSE, "logs", "--no-color"], timeout=60)
    total = syn = anc = 0
    # 0-fill every partition so the DN is visibly tracked even when nothing was
    # wedged toward it, rather than silently absent.
    by_dest = {p: 0 for p in PARTITIONS}
    by_node = {}
    for line in out.splitlines():
        if "dropping synthetic envelope" not in line and "dropping sequenced envelope" not in line:
            continue
        line = ANSI.sub("", line)  # nodes colourise their own logs; --no-color only strips compose's
        total += 1
        node = line.split("|", 1)[0].strip() if "|" in line else "?"
        by_node[node] = by_node.get(node, 0) + 1
        is_anchor = "anchor=true" in line
        if is_anchor:
            anc += 1
        else:
            syn += 1
        m = re.search(r"destination=acc://([a-zA-Z0-9-]+)\.acme", line)
        dest = m.group(1) if m else "?"
        dest = {"dn": "Directory"}.get(dest, dest.replace("bvn-", ""))
        by_dest[dest] = by_dest.get(dest, 0) + 1
    return {"total": total, "synthetic": syn, "anchor": anc,
            "byDest": by_dest, "byNode": by_node}


HEAL_SNIPPET = r'''
nid=$(curl -s -m5 -X POST http://localhost:26660/v3 -H "content-type: application/json" -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"node-info\",\"params\":{}}" | grep -oE "\"peerID\":\"[^\"]+\"" | cut -d"\"" -f4)
for part in Directory BVN1 BVN2 BVN3; do
  r=$(curl -s -m5 -X POST http://localhost:26660/v3 -H "content-type: application/json" -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"consensus-status\",\"params\":{\"partition\":\"$part\",\"nodeID\":\"$nid\"}}")
  s=$(echo "$r" | grep -oE "\"syntheticHeals\":[0-9]+" | cut -d: -f2); a=$(echo "$r" | grep -oE "\"anchorHeals\":[0-9]+" | cut -d: -f2)
  echo "$part ${s:-0} ${a:-0}"
done
'''


def _heal_one(c, out_map, lock):
    txt = sh(["docker", "exec", c, "sh", "-c", HEAL_SNIPPET], timeout=20)
    node = {}
    for ln in txt.splitlines():
        p = ln.split()
        if len(p) == 3:
            node[p[0]] = {"synthetic": int(p[1]), "anchor": int(p[2])}
    with lock:
        out_map[c] = node


def collect_heals():
    cs = containers()
    per_node, threads, lock = {}, [], threading.Lock()
    for c in cs:
        t = threading.Thread(target=_heal_one, args=(c, per_node, lock))
        t.start(); threads.append(t)
    for t in threads:
        t.join()
    syn = anc = 0
    by_part = {}
    for node in per_node.values():
        for part, v in node.items():
            syn += v["synthetic"]; anc += v["anchor"]
            bp = by_part.setdefault(part, {"synthetic": 0, "anchor": 0})
            bp["synthetic"] += v["synthetic"]; bp["anchor"] += v["anchor"]
    # per-node totals for the node table
    node_tot = {c: sum(v["synthetic"] + v["anchor"] for v in node.values())
                for c, node in per_node.items()}
    return {"synthetic": syn, "anchor": anc, "total": syn + anc,
            "byPartition": by_part, "byNode": node_tot}


def collect_chaos():
    counts = {"restart": 0, "pause": 0, "skip": 0}
    recent = []
    try:
        with open(CHAOS) as f:
            lines = f.read().splitlines()
        for ln in lines:
            for k in counts:
                if f" {k} " in f" {ln} " or ln.strip().endswith(k):
                    counts[k] += 1
                    break
        recent = lines[-8:]
    except Exception:
        pass
    return {"counts": counts, "recent": list(reversed(recent))}


def collector():
    last = {"height": 0, "metrics": 0, "chaos": 0}
    hist = []
    while True:
        now = time.time()
        upd = {}
        upd["loadgen"] = read_stats()
        if now - last["height"] >= I_HEIGHT:
            upd["network"] = collect_height()
            upd["heights"] = collect_heights()
            last["height"] = now
        # One authoritative scrape of every node's /metrics feeds heals, wedges
        # (drops), and the flow matrices — no log parsing, no ledger scraping.
        if now - last["metrics"] >= I_FLOW:
            m = collect_metrics()
            upd["heals"] = m["heals"]
            upd["wedges"] = m["wedges"]
            upd["synProduced"] = m["synProduced"]
            upd["ancProduced"] = m["ancProduced"]
            upd["scrape"] = {"nodes": m["nodes"], "scraped": m["scraped"]}
            upd["nodeStats"] = m.get("nodeStats", {})
            with LOCK:
                heights = STATE.get("heights") or {}
            upd["matrix"] = {"flows": m["flows"], "heights": heights, "parts": PARTITIONS}
            last["metrics"] = now
        if now - last["chaos"] >= I_CHAOS:
            upd["chaos"] = collect_chaos(); last["chaos"] = now
        with LOCK:
            STATE.update(upd)
            STATE["ok"] = True
            STATE["now"] = int(now)
            lg = STATE.get("loadgen") or {}
            w = STATE.get("wedges") or {}
            h = STATE.get("heals") or {}
            hist.append({"t": int(now),
                         "generated": lg.get("generated", 0),
                         "wedges": w.get("total", 0),
                         "heals": h.get("total", 0),
                         "sProd": STATE.get("synProduced", 0),
                         "aProd": STATE.get("ancProduced", 0)})
            if len(hist) > HIST_MAX:
                del hist[0:len(hist) - HIST_MAX]
            STATE["history"] = list(hist)
        time.sleep(I_STATS)


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass

    def _send(self, code, body, ctype):
        b = body.encode() if isinstance(body, str) else body
        self.send_response(code)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(b)))
        self.end_headers()
        self.wfile.write(b)

    def do_GET(self):
        if self.path.startswith("/data"):
            with LOCK:
                body = json.dumps(STATE)
            self._send(200, body, "application/json")
        else:
            self._send(200, PAGE, "text/html; charset=utf-8")


PAGE = r"""<!doctype html><html lang=en><head><meta charset=utf-8>
<meta name=viewport content="width=device-width,initial-scale=1">
<title>Synthetic-healing soak monitor</title>
<style>
:root{--bg:#0d1117;--panel:#161b22;--panel2:#1c2430;--bd:#2a3240;--fg:#e6edf3;--mut:#8b949e;
--acc:#58a6ff;--grn:#3fb950;--red:#f85149;--yel:#d29922;--pur:#bc8cff;--cyan:#39c5cf}
@media (prefers-color-scheme:light){:root{--bg:#f6f8fa;--panel:#fff;--panel2:#f0f3f6;--bd:#d0d7de;
--fg:#1f2328;--mut:#636c76;--acc:#0969da;--grn:#1a7f37;--red:#cf222e;--yel:#9a6700;--pur:#8250df;--cyan:#1b7c83}}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--fg);
font:14px/1.45 -apple-system,BlinkMacSystemFont,'Segoe UI',Helvetica,Arial,sans-serif}
.wrap{max-width:1200px;margin:0 auto;padding:18px}
header{display:flex;align-items:baseline;gap:14px;flex-wrap:wrap;margin-bottom:6px}
h1{font-size:18px;margin:0;font-weight:650}
.sub{color:var(--mut);font-size:12px}
.badge{padding:2px 9px;border-radius:20px;font-size:12px;font-weight:600}
.up{background:rgba(63,185,80,.15);color:var(--grn)} .down{background:rgba(248,81,73,.15);color:var(--red)}
.phase{background:rgba(88,166,255,.15);color:var(--acc)}
.grid{display:grid;gap:12px}
.cards{grid-template-columns:repeat(auto-fit,minmax(104px,1fr));gap:8px;margin:10px 0}
.card{background:var(--panel);border:1px solid var(--bd);border-radius:8px;padding:7px 9px}
.card .k{color:var(--mut);font-size:10px;text-transform:uppercase;letter-spacing:.04em;white-space:nowrap}
.card .v{font-size:19px;font-weight:680;margin-top:1px;font-variant-numeric:tabular-nums;line-height:1.15}
.card .d{color:var(--mut);font-size:10.5px;margin-top:1px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.panel{background:var(--panel);border:1px solid var(--bd);border-radius:10px;padding:14px 16px;margin-bottom:12px}
.panel h2{font-size:13px;margin:0 0 10px;color:var(--mut);text-transform:uppercase;letter-spacing:.04em;font-weight:650}
.two{display:grid;grid-template-columns:1fr 1fr;gap:12px}
@media(max-width:800px){.two{grid-template-columns:1fr}}
.bar{height:6px;background:var(--panel2);border-radius:4px;overflow:hidden;margin-top:8px}
.bar>i{display:block;height:100%;background:var(--acc)}
table{width:100%;border-collapse:collapse;font-variant-numeric:tabular-nums}
th,td{text-align:right;padding:4px 6px;border-bottom:1px solid var(--bd)}
th:first-child,td:first-child{text-align:left}
th{color:var(--mut);font-weight:600;font-size:11px;text-transform:uppercase;letter-spacing:.03em}
td.name{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:12.5px}
.volcell{position:relative}
.volcell>i{position:absolute;left:0;top:2px;bottom:2px;background:rgba(88,166,255,.16);border-radius:3px;z-index:0}
.volcell>span{position:relative;z-index:1}
.mut{color:var(--mut)} .red{color:var(--red)} .grn{color:var(--grn)} .yel{color:var(--yel)}
.pills{display:flex;gap:14px;flex-wrap:wrap;margin-top:4px}
.pill .n{font-size:20px;font-weight:680;font-variant-numeric:tabular-nums}
.pill .l{color:var(--mut);font-size:11px}
.spark{width:100%;height:44px;display:block}
.chaoslog{font-family:ui-monospace,Menlo,monospace;font-size:12px;color:var(--mut);max-height:150px;overflow:auto}
.foot{color:var(--mut);font-size:11px;text-align:center;margin-top:14px}
.dot{display:inline-block;width:8px;height:8px;border-radius:50%;margin-right:5px;vertical-align:middle}
.mx{border-collapse:separate;border-spacing:3px;width:100%}
.mx th,.mx td{text-align:center;padding:0;border:none}
.mx th{color:var(--mut);font-size:11px;font-weight:600}
.mx td.rh{color:var(--mut);font-size:12px;font-weight:600;text-align:right;padding-right:6px;font-family:ui-monospace,Menlo,monospace}
.mx td.cell{border-radius:6px;padding:5px 4px;font-variant-numeric:tabular-nums;min-width:60px}
.cell .rd{font-size:13px;font-weight:600;line-height:1.15}
.cell .rd b{font-weight:750}
.cell .st{font-size:10px;color:var(--mut)}
.heights{display:flex;gap:20px;flex-wrap:wrap}
.heights .n{font-size:20px;font-weight:680;font-variant-numeric:tabular-nums}
.heights .l{color:var(--mut);font-size:11px;margin-left:4px}
.legend{color:var(--mut);font-size:11px;margin-top:8px}
.strip{display:flex;flex-wrap:wrap;gap:6px 22px;align-items:baseline}
.sgrp{display:flex;align-items:baseline;gap:5px;white-space:nowrap}
.sgrp b{font-size:16px;font-weight:680;font-variant-numeric:tabular-nums}
.sh{color:var(--mut);font-size:10px;text-transform:uppercase;letter-spacing:.04em;margin-right:2px}
.sl{color:var(--mut);font-size:10.5px;margin-right:5px}
.strip .n{font-size:16px;font-weight:680;font-variant-numeric:tabular-nums}
.strip .l{color:var(--mut);font-size:10.5px;margin-left:3px;margin-right:6px}
</style></head><body><div class=wrap>
<header>
  <h1>Synthetic-healing soak</h1>
  <span id=phase class="badge phase">—</span>
  <span id=net class="badge down">network ?</span>
  <span class=sub id=sub></span>
</header>
<div class="grid cards" id=cards></div>
<div class=panel style="padding:9px 12px">
  <div class=strip>
    <div class=sgrp><span class=sh>tx/s</span>
      <b id=ruser>—</b><span class=sl>user</span>
      <span class=mut id=rtgt>—</span><span class=sl>target</span>
      <b id=rsyn>—</b><span class=sl>syn</span>
      <b id=ranc>—</b><span class=sl>anc</span>
      <span class=mut id=rratio>—</span><span class=sl>syn/user</span>
    </div>
    <div class=sgrp><span class=sh>node RSS</span>
      <b id=nrssavg>—</b><span class=sl>avg</span>
      <b id=nrssmax>—</b><span class=sl>max</span>
      <span class=mut id=nrssmin>—</span><span class=sl>min</span>
      <span class=sl id=nrssnode></span>
    </div>
    <div class=sgrp><span class=sh>goroutines</span>
      <b id=ngravg>—</b><span class=sl>avg</span>
      <b id=ngrmax>—</b><span class=sl>max</span>
      <span class=mut id=ngrmin>—</span><span class=sl>min</span>
      <span class=sl id=ngrnode></span>
    </div>
    <div class=sgrp><span class=sh>heights</span><span id=heights></span></div>
  </div>
</div>
<div class=two>
  <div class=panel>
    <h2>Synthetic flow &nbsp;·&nbsp; src&nbsp;▸&nbsp;dst</h2>
    <div style="overflow-x:auto"><table class=mx id=mxSyn></table></div>
    <div class=legend>cell = received / <b>delivered</b>, and <span class=mut>sent</span> below. <b style="color:var(--red)">Red</b> = received but not delivered (wedge). <b style="color:var(--yel)">Amber</b> = sent but not received (in transit / lost).</div>
  </div>
  <div class=panel>
    <h2>Anchor flow &nbsp;·&nbsp; src&nbsp;▸&nbsp;dst</h2>
    <div style="overflow-x:auto"><table class=mx id=mxAnc></table></div>
    <div class=legend>Directory anchors to every BVN; each BVN anchors only to the Directory — so BVN↔BVN cells stay empty by design.</div>
  </div>
</div>
<div class=two>
  <div class=panel>
    <h2>Wedges — dropped cross-partition messages</h2>
    <div class=pills>
      <div class=pill><div class=n id=wsyn>0</div><div class=l>synthetic drops</div></div>
      <div class=pill><div class=n id=wanc>0</div><div class=l>anchor drops</div></div>
      <div class=pill><div class=n id=wtot>0</div><div class=l>total</div></div>
    </div>
    <svg class=spark id=spWedge viewBox="0 0 300 44" preserveAspectRatio=none></svg>
    <table id=wdest><thead><tr><th>destination</th><th>drops</th></tr></thead><tbody></tbody></table>
  </div>
  <div class=panel>
    <h2>Healing — receiver-pull recoveries</h2>
    <div class=pills>
      <div class=pill><div class="n grn" id=hsyn>0</div><div class=l>synthetic heals</div></div>
      <div class=pill><div class="n grn" id=hanc>0</div><div class=l>anchor heals</div></div>
      <div class=pill><div class="n yel" id=hdef>0</div><div class=l>deferred (unprovable)</div></div>
      <div class=pill><div class="n" id=hfoc>0</div><div class=l>focus activations</div></div>
      <div class=pill><div class="n red" id=herr>0</div><div class=l>pull errors</div></div>
      <div class=pill><div class="n red" id=hstuck>0</div><div class=l id=hstuckl>stuck (churn)</div></div>
    </div>
    <svg class=spark id=spHeal viewBox="0 0 300 44" preserveAspectRatio=none></svg>
    <table id=hpart><thead><tr><th>partition</th><th>synthetic</th><th>anchor</th></tr></thead><tbody></tbody></table>
  </div>
</div>
<div class=panel>
  <h2>Transaction type mix (live)</h2>
  <table id=mix><thead><tr><th>type</th><th>generated</th><th>rejected</th><th>skipped</th></tr></thead><tbody></tbody></table>
</div>
<div class=two>
  <div class=panel>
    <h2>Accounts created</h2>
    <div class=pills id=acct></div>
  </div>
  <div class=panel>
    <h2>Chaos events</h2>
    <div class=pills id=chaosc></div>
    <div class=chaoslog id=chaoslog></div>
  </div>
</div>
<div class=foot id=foot>connecting…</div>
</div>
<script>
const $=id=>document.getElementById(id);
const fmt=n=>(n==null?'—':n.toLocaleString());
const dur=s=>{s=Math.max(0,s|0);const h=(s/3600|0),m=(s%3600/60|0);return h+'h '+String(m).padStart(2,'0')+'m';};
function card(k,v,d){return `<div class=card><div class=k>${k}</div><div class=v>${v}</div><div class=d>${d||''}</div></div>`;}
function spark(el,pts,color){
  if(!pts||pts.length<2){el.innerHTML='';return;}
  const n=pts.length,mx=Math.max(1,...pts),W=300,H=44;
  const d=pts.map((y,i)=>`${(i/(n-1)*W).toFixed(1)},${(H-2-y/mx*(H-6)).toFixed(1)}`).join(' ');
  el.innerHTML=`<polyline fill=none stroke="${color}" stroke-width=1.6 points="${d}"/>`+
    `<polyline fill="${color}" opacity=.10 stroke=none points="0,${H} ${d} ${W},${H}"/>`;
}
function deltas(hist,key){const o=[];for(let i=1;i<hist.length;i++)o.push(Math.max(0,hist[i][key]-hist[i-1][key]));return o;}
const shortP=p=>p==='Directory'?'DN':p;
function matrix(el,mat,parts){
  if(!el)return;
  if(!mat||!parts){el.innerHTML='<tr><td class=mut>waiting…</td></tr>';return;}
  let h='<tr><th></th>'+parts.map(p=>`<th>${shortP(p)}</th>`).join('')+'</tr>';
  for(const s of parts){
    h+=`<tr><td class=rh>${shortP(s)}</td>`;
    for(const d of parts){
      if(s===d){h+='<td class=cell style="color:var(--mut)">·</td>';continue;}
      const c=(mat[s]&&mat[s][d])||{};
      const sent=c.sent||0,recv=c.recv||0,deliv=c.deliv||0;
      if(!sent&&!recv&&!deliv){h+='<td class=cell style="color:var(--bd)">·</td>';continue;}
      const dgap=recv-deliv,tgap=sent-recv;
      let bg='rgba(63,185,80,.10)';
      if(dgap>0){const f=Math.min(1,dgap/Math.max(recv,1));bg=`rgba(248,81,73,${(0.14+0.5*f).toFixed(2)})`;}
      else if(tgap>0){const f=Math.min(1,tgap/Math.max(sent,1));bg=`rgba(210,153,34,${(0.14+0.4*f).toFixed(2)})`;}
      h+=`<td class=cell style="background:${bg}"><div class=rd>${fmt(recv)}/<b>${fmt(deliv)}</b></div><div class=st>sent ${fmt(sent)}</div></td>`;
    }
    h+='</tr>';
  }
  el.innerHTML=h;
}
async function tick(){
  let s;try{s=await(await fetch('/data')).json();}catch(e){$('foot').textContent='monitor unreachable';return;}
  if(!s.ok){$('foot').textContent='waiting for first sample…';return;}
  const lg=s.loadgen||{},nw=s.network||{},w=s.wedges||{},h=s.heals||{},ch=s.chaos||{},mx=s.matrix||{};
  // flow matrices + heights
  const parts=mx.parts||['Directory','BVN1','BVN2','BVN3'];
  matrix($('mxSyn'),(mx.flows||{}).synthetic,mx.parts&&parts);
  matrix($('mxAnc'),(mx.flows||{}).anchor,mx.parts&&parts);
  const hh=mx.heights||{};
  $('heights').innerHTML=parts.map(p=>`<span class=n>${fmt(hh[p])}</span><span class=l>${shortP(p)}</span>`).join('')||'<span class=mut>—</span>';
  // header
  const ph=lg.phase||'—';$('phase').textContent=ph;
  const nu=nw.api==='up';$('net').className='badge '+(nu?'up':'down');$('net').innerHTML=`<span class=dot style="background:${nu?'var(--grn)':'var(--red)'}"></span>network ${nw.api||'?'}`;
  const tgt=lg.target||0,gen=lg.generated||0;
  $('sub').textContent=`elapsed ${dur(lg.elapsedSec||0)} · ${(lg.rate||0).toFixed(2)} tx/s`+(lg.stale?' · loadgen stats stale':'');
  // cards
  const pct=tgt?(100*gen/tgt):0;
  const ns=s.nodeStats||{};
  // Heals split by mechanism, not just by kind: range pulls are the path #4087
  // added, and lumping them into one total hides whether it is doing anything.
  const hRange=(h['anchor-range']||0)+(h['synthetic-range']||0);
  const hPer=(h.synthetic||0)+(h.anchor||0);
  $('cards').innerHTML=[
    card('DN height',fmt(nw.dnHeight),`${fmt(gen)} tx · ${pct.toFixed(0)}% of plan`),
    card('Heals',`<span class=grn>${fmt(h.total||0)}</span>`,`${fmt(hRange)} range · ${fmt(hPer)} per-msg`),
    card('Anchor',fmt((h.anchor||0)+(h['anchor-range']||0)),`${fmt(h['anchor-range']||0)} by range`),
    card('Synthetic',fmt((h.synthetic||0)+(h['synthetic-range']||0)),`${fmt(h['synthetic-range']||0)} by range`),
    card('Wedges',`<span class="${(w.total||0)?'yel':''}">${fmt(w.total||0)}</span>`,`${fmt(w.synthetic||0)} syn · ${fmt(w.anchor||0)} anc`),
    card('Heal errors',`<span class="${(h.errors||0)?'red':''}">${fmt(h.errors||0)}</span>`,`stuck ${fmt(h.stuck||0)}`),
    card('Rejected',`<span class="${(lg.rejected||0)?'red':''}">${fmt(lg.rejected||0)}</span>`,`${fmt(lg.skipped||0)} skipped`),
    card('Nodes',fmt(ns.count||0),`${fmt(ns.rssAvgMiB||0)} MiB avg · ${fmt(ns.rssMaxMiB||0)} max`),
  ].join('');
  const mib=v=>v?fmt(v)+' MiB':'—';
  $('nrssavg').textContent=mib(ns.rssAvgMiB); $('nrssmax').textContent=mib(ns.rssMaxMiB);
  $('nrssmin').textContent=mib(ns.rssMinMiB);
  $('nrssnode').textContent=ns.rssMaxNode?('max '+ns.rssMaxNode):'';
  $('ngravg').textContent=ns.grAvg!=null?fmt(ns.grAvg):'—';
  $('ngrmax').textContent=ns.grMax!=null?fmt(ns.grMax):'—';
  $('ngrmin').textContent=ns.grMin!=null?fmt(ns.grMin):'—';
  $('ngrnode').textContent=ns.grMaxNode?('max '+ns.grMaxNode):'';
  // wedges / heals pills
  $('wsyn').textContent=fmt(w.synthetic||0);$('wanc').textContent=fmt(w.anchor||0);$('wtot').textContent=fmt(w.total||0);
  $('hsyn').textContent=fmt(h.synthetic||0);$('hanc').textContent=fmt(h.anchor||0);
  $('hdef').textContent=fmt(h.deferred||0);$('hfoc').textContent=fmt(h.focus||0);$('herr').textContent=fmt(h.errors||0);
  $('hstuck').textContent=fmt(h.stuck||0);$('hstuckl').textContent=(h.stuck>0?('stuck: '+(h.stuckStream||'')):'stuck (churn)');
  // sparklines from history deltas
  const hist=s.history||[];
  // transaction rates: derivative of cumulative counters over ~30s of history
  const rateOf=(k,secs)=>{if(hist.length<2)return null;const now=hist[hist.length-1];let old=hist[0];for(let i=hist.length-2;i>=0;i--){old=hist[i];if(now.t-hist[i].t>=secs)break;}const dt=now.t-old.t;if(dt<=0)return null;const d=now[k]-old[k];return d>=0?d/dt:null;};
  let ru=rateOf('generated',30); if(ru==null) ru=lg.rate;
  const rs=rateOf('sProd',30), ra=rateOf('aProd',30);
  $('rtgt').textContent=(lg.targetTps!=null)?lg.targetTps.toFixed(2):'—';
  $('ruser').textContent=(ru!=null)?ru.toFixed(2):'—';
  $('rsyn').textContent=(rs!=null)?rs.toFixed(2):'—';
  $('ranc').textContent=(ra!=null)?ra.toFixed(2):'—';
  $('rratio').textContent=(rs!=null&&ru>0.01)?(rs/ru).toFixed(1)+'×':'—';
  spark($('spWedge'),deltas(hist,'wedges'),getComputedStyle(document.documentElement).getPropertyValue('--yel').trim());
  spark($('spHeal'),deltas(hist,'heals'),getComputedStyle(document.documentElement).getPropertyValue('--grn').trim());
  // wedge by dest
  const wd=Object.entries(w.byDest||{}).sort((a,b)=>b[1]-a[1]);
  $('wdest').querySelector('tbody').innerHTML=wd.map(([k,v])=>`<tr><td class=name>${k}</td><td>${fmt(v)}</td></tr>`).join('')||'<tr><td class=mut colspan=2>none yet</td></tr>';
  // heal by partition
  const hp=Object.entries(h.byPartition||{}).sort((a,b)=>(b[1].synthetic+b[1].anchor)-(a[1].synthetic+a[1].anchor));
  $('hpart').querySelector('tbody').innerHTML=hp.map(([k,v])=>`<tr><td class=name>${k}</td><td class=grn>${fmt(v.synthetic)}</td><td class=grn>${fmt(v.anchor)}</td></tr>`).join('')||'<tr><td class=mut colspan=3>none yet</td></tr>';
  // mix table
  const pt=lg.perType||{};const rows=Object.entries(pt).sort((a,b)=>b[1].generated-a[1].generated);
  const mxv=Math.max(1,...rows.map(r=>r[1].generated));
  $('mix').querySelector('tbody').innerHTML=rows.map(([k,v])=>{
    const wpc=(100*v.generated/mxv).toFixed(1);
    const rej=v.rejected?`<span class=red>${fmt(v.rejected)}</span>`:'0';
    return `<tr><td class=name>${k}</td><td class=volcell><i style="width:${wpc}%"></i><span>${fmt(v.generated)}</span></td><td>${rej}</td><td class=mut>${fmt(v.skipped)}</td></tr>`;
  }).join('')||'<tr><td class=mut colspan=4>waiting for loadgen…</td></tr>';
  // accounts
  const a=lg.accounts||{};
  $('acct').innerHTML=[['identities',a.identities],['key-books',a.keyBooks],['key-pages',a.keyPages],['token-issuers',a.tokenIssuers],['accounts',a.accounts]]
    .map(([l,n])=>`<div class=pill><div class=n>${fmt(n||0)}</div><div class=l>${l}</div></div>`).join('');
  // chaos
  const cc=ch.counts||{};
  $('chaosc').innerHTML=[['restarts',cc.restart],['pauses',cc.pause],['skips',cc.skip]]
    .map(([l,n])=>`<div class=pill><div class=n>${fmt(n||0)}</div><div class=l>${l}</div></div>`).join('');
  $('chaoslog').innerHTML=(ch.recent||[]).map(x=>x.replace(/</g,'&lt;')).join('<br>')||'<span class=mut>no chaos yet</span>';
  const age=s.now?(Math.round(Date.now()/1000)-s.now):0;
  $('foot').textContent=`updated ${age}s ago · flow+wedges+heals refresh ~1s · monitor read-only`;
}
tick();setInterval(tick,1000);
</script></body></html>"""


def main():
    threading.Thread(target=collector, daemon=True).start()
    srv = ThreadingHTTPServer(("127.0.0.1", PORT), Handler)
    print(f"soak monitor on http://127.0.0.1:{PORT}  (Ctrl-C to stop)")
    try:
        srv.serve_forever()
    except KeyboardInterrupt:
        pass


if __name__ == "__main__":
    main()
