#!/usr/bin/env python3
"""Realtime web monitor for the acc-cl consensus-load test (#4043/#4044).

Serves a live dashboard on http://127.0.0.1:18090 .

Why this exists: the previous monitor reported a partition "healthy" for ~3h
after BVN1 had actually halted, because it only checked container-up +
validator-present + validatorConsensus. It never checked whether block height
was ADVANCING. This monitor makes height-progress the primary health signal and
loudly flags a stalled chain and any LevelDB-corruption / CONSENSUS FAILURE.

Signals tracked:
  - per-partition (DN, BVN1, BVN2, BVN3) block height + advancing? + stall age
  - per-node BVN/DN height (lag), mempool depth (+ backpressure threshold)
  - container state / restart count / OOMKilled
  - CONSENSUS FAILURE / leveldb checksum-mismatch (current, over a recent window;
    clears on its own once the node stops hitting it)
  - loadmix stats (tail of the load generator log)
"""
import json, subprocess, threading, time, html, re, os
from datetime import datetime, timezone
from concurrent.futures import ThreadPoolExecutor
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

PORT          = 18090
BVN_RPC       = 26757      # block-validator CometBFT RPC (inside container)
DN_RPC        = 26657      # directory CometBFT RPC (inside container)
MEMPOOL_CAP   = 5000       # CometBFT Mempool.Size
BACKPRESS_PCT = 20         # NetworkLimits.MempoolBackpressurePercent (refreshed at start)
FAST_SEC      = 3          # height/mempool poll interval
SLOW_SEC      = 15         # container + log-scan interval
STALL_SEC     = 30         # a partition not advancing this long = STALLED
TPS_WINDOW    = 60         # committed TPS is averaged over at least this many seconds
LOADMIX_LOG   = os.environ.get("LOADMIX_LOG", "/tmp/cl-loadmix.log")

PARTITIONS = {
    "BVN1": ["bvn1-1", "bvn1-2", "bvn1-3", "bvn1-4"],
    "BVN2": ["bvn2-1", "bvn2-2", "bvn2-3", "bvn2-4"],
    "BVN3": ["bvn3-1", "bvn3-2", "bvn3-3", "bvn3-4"],
}
ALL_NODES = [n for ns in PARTITIONS.values() for n in ns]

# docker stats reports memory in binary units ("577.4MiB", "1.2GiB"). Parse to
# decimal MB (bytes/1e6) so the column is one consistent unit and aligns.
_MEM_UNIT = {"b": 1, "kib": 1024, "mib": 1024**2, "gib": 1024**3, "tib": 1024**4,
             "kb": 1e3, "mb": 1e6, "gb": 1e9, "tb": 1e12}
def mem_to_mb(s):
    m = re.match(r"([\d.]+)\s*([a-zA-Z]+)", s.strip())
    if not m:
        return None
    return float(m.group(1)) * _MEM_UNIT.get(m.group(2).lower(), 1) / 1e6
# TWO DISTINCT failure classes — do NOT conflate them:
#  - DB corruption: a leveldb checksum mismatch / corrupt data-block. A node can
#    hit this in a non-consensus store (e.g. tx_index, a rebuildable secondary
#    index) and KEEP making blocks. It is NOT by itself a consensus failure.
#  - Consensus failure: CometBFT actually failed to agree / panicked. A node
#    making blocks cannot be in this state.
# Both are scanned over the RECENT window only (non-sticky): the flag clears once
# the node stops logging it.
CORRUPTION_RE     = re.compile(r"checksum mismatch|corruption on data-block|leveldb/table: corruption")
CONSENSUS_FAIL_RE = re.compile(r"CONSENSUS FAILURE")
CFAIL_WINDOW = 45   # seconds of recent logs to scan; clears ~this long after the errors stop

# Representative node + RPC port for per-partition committed-TPS sampling.
# Committed TPS = CometBFT txs/sec actually landed in blocks (user + synthetic).
TPS_SOURCES = {
    "DN":   ("bvn1-1", DN_RPC),
    "BVN1": ("bvn1-1", BVN_RPC),
    "BVN2": ("bvn2-1", BVN_RPC),
    "BVN3": ("bvn3-1", BVN_RPC),
}

# Synthetic ledgers: host v3 API port + ledger URL per partition. A node only
# answers for the partition(s) it hosts, so query each on a node that hosts it.
# On partition X's ledger, entry[Y].produced = synthetics X produced FOR Y;
# entry[Y].delivered = synthetics Y->X that X has applied.
SYNTH_LEDGERS = {
    "DN":   (27680, "acc://dn.acme/synthetic"),
    "BVN1": (27680, "acc://bvn-BVN1.acme/synthetic"),
    "BVN2": (27684, "acc://bvn-BVN2.acme/synthetic"),
    "BVN3": (27688, "acc://bvn-BVN3.acme/synthetic"),
}
# Anchor ledgers — the block-anchor backbone that every cross-partition synthetic
# proof terminates at. Each partition's anchor ledger entry[Y] tracks anchors
# RECEIVED from Y and DELIVERED (applied); received-delivered = anchor backlog.
# The DN ledger shows BVN->DN (up); each BVN ledger shows DN->BVN (down).
ANCHOR_LEDGERS = {
    "DN":   (27680, "acc://dn.acme/anchors"),
    "BVN1": (27680, "acc://bvn-BVN1.acme/anchors"),
    "BVN2": (27684, "acc://bvn-BVN2.acme/anchors"),
    "BVN3": (27688, "acc://bvn-BVN3.acme/anchors"),
}

STATE = {
    "ts": 0, "nodes": {}, "partitions": {}, "containers": {},
    "corruption": {}, "consensus_failures": {}, "loadmix": [], "backpressure_pct": BACKPRESS_PCT,
    "mempool_cap": MEMPOOL_CAP, "alerts": [],
    "overall_tps": None, "tps_by_part": {}, "tps_window": TPS_WINDOW,
}
# per-partition progress memory: last max height + the time we last saw it rise
PROG = {p: {"height": 0, "rose_at": time.time()} for p in PARTITIONS}
STARTED = {}    # node -> last-seen container StartedAt (to detect bounces)
RESTARTS = {}   # node -> count of observed restarts (manual/rolling, beyond docker policy count)
LOCK = threading.Lock()


def dexec(node, cmd, timeout=6):
    try:
        out = subprocess.run(["docker", "exec", f"acc-cl-{node}", "sh", "-c", cmd],
                             capture_output=True, text=True, timeout=timeout)
        return out.stdout
    except Exception:
        return ""


def fetch_node(node):
    """One docker exec per node -> BVN status+mempool+net_info, DN status+mempool.
    Captures real consensus state: height, sync (catching_up), voting power, peers."""
    cmd = (f"wget -qO- http://127.0.0.1:{BVN_RPC}/status; echo '@@@';"
           f"wget -qO- http://127.0.0.1:{BVN_RPC}/num_unconfirmed_txs; echo '@@@';"
           f"wget -qO- http://127.0.0.1:{DN_RPC}/status; echo '@@@';"
           f"wget -qO- http://127.0.0.1:{DN_RPC}/num_unconfirmed_txs; echo '@@@';"
           f"wget -qO- http://127.0.0.1:{BVN_RPC}/net_info; echo '@@@';"
           f"wget -qO- http://127.0.0.1:{DN_RPC}/net_info")
    parts = dexec(node, cmd).split("@@@")

    def jget(s, path, default=None):
        try:
            d = json.loads(s)
            for k in path:
                d = d[k]
            return d
        except Exception:
            return default
    # default every field so an unreachable node (mid-restart) never raises KeyError
    r = {"node": node, "bvn_h": None, "bvn_t": None, "bvn_mp": None, "dn_h": None,
         "dn_t": None, "dn_mp": None, "bvn_catchup": None, "bvn_vp": None,
         "bvn_peers": None, "dn_peers": None, "rpc_ok": False}
    if len(parts) >= 4:
        r["rpc_ok"]      = bool((parts[0] or "").strip())
        r["bvn_h"]       = jget(parts[0], ["result", "sync_info", "latest_block_height"])
        r["bvn_t"]       = jget(parts[0], ["result", "sync_info", "latest_block_time"])
        r["bvn_catchup"] = jget(parts[0], ["result", "sync_info", "catching_up"])
        r["bvn_vp"]      = jget(parts[0], ["result", "validator_info", "voting_power"])
        r["bvn_mp"]      = jget(parts[1], ["result", "n_txs"])
        r["dn_h"]        = jget(parts[2], ["result", "sync_info", "latest_block_height"])
        r["dn_t"]        = jget(parts[2], ["result", "sync_info", "latest_block_time"])
        r["dn_mp"]       = jget(parts[3], ["result", "n_txs"])
    if len(parts) >= 6:
        r["bvn_peers"] = jget(parts[4], ["result", "n_peers"])
        r["dn_peers"]  = jget(parts[5], ["result", "n_peers"])
    for k in ("bvn_h", "bvn_mp", "dn_h", "dn_mp", "bvn_vp", "bvn_peers", "dn_peers"):
        try:
            r[k] = int(r.get(k))
        except (TypeError, ValueError):
            r[k] = None
    return r


def parse_time(s):
    """Parse a CometBFT RFC3339 timestamp (nanosecond precision, trailing Z)."""
    s = s.rstrip("Z")
    if "." in s:
        base, frac = s.split(".", 1)
        s = base + "." + (frac + "000000")[:6]
        fmt = "%Y-%m-%dT%H:%M:%S.%f"
    else:
        fmt = "%Y-%m-%dT%H:%M:%S"
    return datetime.strptime(s, fmt).replace(tzinfo=timezone.utc).timestamp()


def fetch_tps(part):
    """Committed TPS for a partition, averaged over >= TPS_WINDOW seconds.

    /blockchain returns at most 20 block_metas per call, so we page backwards
    (newest first) accumulating block tx-counts + timestamps until the covered
    time span reaches TPS_WINDOW. TPS = sum(txs in span) / span; this is robust
    to the monitor's own poll cadence and to variable block times."""
    node, port = TPS_SOURCES[part]
    st = dexec(node, f"wget -qO- http://127.0.0.1:{port}/status")
    try:
        H = int(json.loads(st)["result"]["sync_info"]["latest_block_height"])
    except Exception:
        return {"tps": None, "blocktime": None, "window_secs": 0, "blocks": 0}
    samples = {}  # height -> (time, txs)
    hi = H
    for _ in range(10):  # up to 200 blocks of lookback
        lo = max(1, hi - 19)
        out = dexec(node, f"wget -qO- 'http://127.0.0.1:{port}/blockchain?minHeight={lo}&maxHeight={hi}'")
        try:
            metas = json.loads(out)["result"]["block_metas"]
        except Exception:
            break
        for m in metas:
            try:
                samples[int(m["header"]["height"])] = (parse_time(m["header"]["time"]),
                                                       int(m.get("num_txs") or 0))
            except Exception:
                pass
        if not samples:
            break
        span = samples[max(samples)][0] - samples[min(samples)][0]
        if span >= TPS_WINDOW or lo <= 1:
            break
        hi = lo - 1
        if hi < 1:
            break
    if len(samples) < 2:
        return {"tps": 0.0, "blocktime": None, "window_secs": 0, "blocks": len(samples)}
    hs = sorted(samples)
    span = samples[hs[-1]][0] - samples[hs[0]][0]
    txsum = sum(samples[h][1] for h in hs[1:])  # txs committed within the span
    tps = txsum / span if span > 0 else 0.0
    blocktime = span / (len(hs) - 1)
    return {"tps": round(tps, 1), "blocktime": round(blocktime, 2),
            "window_secs": round(span), "blocks": len(hs)}


def fast_poll():
    with ThreadPoolExecutor(max_workers=12) as ex:
        results = list(ex.map(fetch_node, ALL_NODES))
    nodes = {r["node"]: r for r in results}
    now = time.time()

    # DN is a single partition spanning all nodes; BVNx are per-node-group.
    parts = {}
    dn_heights = [nodes[n].get("dn_h") for n in ALL_NODES if nodes[n].get("dn_h") is not None]
    groups = {"DN": ("dn_h", "dn_mp", "dn_t", ALL_NODES)}
    for p, ns in PARTITIONS.items():
        groups[p] = ("bvn_h", "bvn_mp", "bvn_t", ns)

    alerts = []
    for p, (hk, mk, tk, ns) in groups.items():
        hs = [nodes[n].get(hk) for n in ns if nodes[n].get(hk) is not None]
        mps = [nodes[n].get(mk) for n in ns if nodes[n].get(mk) is not None]
        maxh = max(hs) if hs else 0
        minh = min(hs) if hs else 0
        # Authoritative stall signal: age of the newest committed block (block
        # time vs wall clock). This is absolute -> correct immediately on the
        # first poll, no warmup, and reflects whether the chain is committing NOW.
        bts = []
        for n in ns:
            t = nodes[n].get(tk)
            if t:
                try:
                    bts.append(parse_time(t))
                except Exception:
                    pass
        newest_bt = max(bts) if bts else None
        block_age = (now - newest_bt) if newest_bt else None
        stalled = (block_age is not None and block_age > STALL_SEC) or (not hs)
        tps = STATE.get("tps_by_part", {}).get(p, {})
        parts[p] = {
            "max_h": maxh, "min_h": minh, "spread": maxh - minh,
            "advancing": not stalled,
            "stall_age": round(block_age, 1) if block_age is not None else None,
            "mp_max": max(mps) if mps else None,
            "mp_avg": round(sum(mps) / len(mps)) if mps else None,
            "responding": f"{len(hs)}/{len(ns)}",
            "tps": tps.get("tps"), "blocktime": tps.get("blocktime"),
            "tps_window": tps.get("window_secs"),
        }
        if stalled and hs:
            alerts.append(f"{p} STALLED at height {maxh} for {int(block_age or 0)}s")
        if hs and len(hs) < len(ns):
            alerts.append(f"{p}: only {len(hs)}/{len(ns)} nodes responding")

    # A partition that isn't advancing is processing nothing -> its committed TPS
    # is 0, regardless of what the (lagging >=60s) block-time window still shows.
    for p in parts:
        if not parts[p]["advancing"]:
            parts[p]["tps"] = 0.0
            parts[p]["blocktime"] = None

    # overall committed TPS = sum across the BVN partitions (user load lands there)
    bvn_tps = [parts[p]["tps"] for p in PARTITIONS if parts[p].get("tps") is not None]
    overall = round(sum(bvn_tps), 1) if bvn_tps else None
    # network total = every partition including the Directory (anchors/synthetics)
    all_tps = [parts[p]["tps"] for p in parts if parts[p].get("tps") is not None]
    total = round(sum(all_tps), 1) if all_tps else None

    # trend: compare overall to its value ~45s ago so a rising ramp is visible
    # even though each TPS number is itself a >=60s average
    hist = STATE.setdefault("_tps_hist", [])
    if overall is not None:
        hist.append((now, overall))
        while hist and now - hist[0][0] > 45:
            hist.pop(0)
    trend = 0
    if overall is not None and hist and now - hist[0][0] > 20:
        d = overall - hist[0][1]
        trend = 1 if d > 0.5 else (-1 if d < -0.5 else 0)

    with LOCK:
        STATE["ts"] = now
        STATE["nodes"] = nodes
        STATE["partitions"] = parts
        STATE["overall_tps"] = overall
        STATE["total_tps"] = total
        STATE["overall_trend"] = trend
        # keep corruption + stall alerts together; corruption added in slow loop
        STATE["alerts"] = alerts + STATE.get("_corruption_alerts", [])


def slow_poll():
    # container state / restarts / oom. docker's RestartCount only counts
    # restart-POLICY restarts, so we also detect manual/rolling restarts by
    # watching StartedAt change between polls (RESTARTS counts observed bounces).
    try:
        names = " ".join(f"acc-cl-{n}" for n in ALL_NODES)
        fmt = '{{.Name}}|{{.State.Status}}|{{.RestartCount}}|{{.State.OOMKilled}}|{{.State.StartedAt}}'
        out = subprocess.run(f"docker inspect -f '{fmt}' {names}", shell=True,
                             capture_output=True, text=True, timeout=20).stdout
        containers = {}
        for line in out.strip().splitlines():
            try:
                name, status, rc, oom, started = line.strip().lstrip("/").split("|", 4)
                node = name.replace("acc-cl-", "")
                prev = STARTED.get(node)
                if prev is not None and prev != started:
                    RESTARTS[node] = RESTARTS.get(node, 0) + 1   # observed a bounce
                STARTED[node] = started
                containers[node] = {
                    "status": status, "oom": oom == "true",
                    "policy_restarts": int(rc),
                    "restarts": int(rc) + RESTARTS.get(node, 0),  # policy + observed
                    "started": started,
                }
            except ValueError:
                continue
    except Exception:
        containers = {}

    # CPU% + memory per container (one docker stats call for all nodes)
    try:
        sfmt = '{{.Name}}|{{.CPUPerc}}|{{.MemUsage}}|{{.MemPerc}}'
        sout = subprocess.run(f"docker stats --no-stream --format '{sfmt}' {names}",
                              shell=True, capture_output=True, text=True, timeout=25).stdout
        for line in sout.strip().splitlines():
            try:
                name, cpu, memu, memp = line.strip().split("|", 3)
                node = name.replace("acc-cl-", "")
                if node not in containers:
                    continue
                containers[node]["cpu"] = float(cpu.rstrip("% "))
                containers[node]["mem_mb"] = mem_to_mb(memu.split("/")[0])  # "577.4MiB" -> 605.4 (MB)
                containers[node]["mem_pct"] = float(memp.rstrip("% "))
            except (ValueError, IndexError):
                continue
    except Exception:
        pass

    # database compaction debt: per-node accumulate.db SSTable count + size (MB).
    # A climbing .ldb count = goleveldb compaction falling behind the write rate.
    def fetch_dbstats(node):
        out = dexec(node, f"ls /root/.accumulate/{node}/bvnn/data/accumulate.db/*.ldb 2>/dev/null | wc -l;"
                          f"du -sm /root/.accumulate/{node}/bvnn/data/accumulate.db 2>/dev/null | cut -f1")
        try:
            a = out.split()
            return (int(a[0]), int(a[1]))
        except Exception:
            return (None, None)
    try:
        with ThreadPoolExecutor(max_workers=12) as ex:
            dbs = dict(zip(ALL_NODES, ex.map(fetch_dbstats, ALL_NODES)))
        for node, (nf, mb) in dbs.items():
            if node in containers:
                containers[node]["ldb"] = nf
                containers[node]["db_mb"] = mb
    except Exception:
        pass

    # Two SEPARATE scans over the RECENT window (non-sticky): DB corruption
    # (leveldb checksum mismatch — can occur while the node keeps making blocks)
    # vs an actual CometBFT consensus failure. A block-producing node is NOT in
    # consensus failure, so we never conflate them.
    corrupt, cfails = {}, {}
    ansi = re.compile(r"\x1b\[[0-9;]*m")
    for n in ALL_NODES:
        try:
            logs = subprocess.run(f"docker logs --since {CFAIL_WINDOW}s acc-cl-{n} 2>&1",
                                 shell=True, capture_output=True, text=True, timeout=15).stdout
        except Exception:
            continue
        cm = CORRUPTION_RE.findall(logs)
        if cm:
            clean = ansi.sub("", logs)
            wm = re.search(r"module=(\w+)", clean)        # e.g. txindex
            fm = re.search(r"\[file=([^\]]+)\]", clean)   # e.g. 000461.ldb
            corrupt[n] = {"count": len(cm),
                          "where": (wm.group(1) if wm else ""),
                          "file": (fm.group(1) if fm else "")}
        cf = len(CONSENSUS_FAIL_RE.findall(logs))
        if cf:
            cfails[n] = cf
    calerts  = [f"DB CORRUPTION on {n}" + (f" ({v['where']})" if v['where'] else "")
                + f" (x{v['count']} in last {CFAIL_WINDOW}s)" for n, v in corrupt.items()]
    calerts += [f"CONSENSUS FAILURE on {n} (x{c} in last {CFAIL_WINDOW}s)" for n, c in cfails.items()]

    # committed TPS per partition, averaged over >= TPS_WINDOW seconds
    try:
        parts = list(TPS_SOURCES.keys())
        with ThreadPoolExecutor(max_workers=4) as ex:
            tps_by_part = dict(zip(parts, ex.map(fetch_tps, parts)))
    except Exception:
        tps_by_part = STATE.get("tps_by_part", {})

    # loadmix tail + parsed offered/achieved (the generator's own rates rise
    # immediately with each ramp step, unlike the >=60s committed average)
    loadmix, load = [], dict(STATE.get("load", {}))
    try:
        if os.path.exists(LOADMIX_LOG):
            with open(LOADMIX_LOG) as f:
                lines = f.readlines()
            loadmix = lines[-6:]
            for ln in reversed(lines):
                m = re.search(r"tps=(\d+) actors=(\d+) active=(\d+)\s+sub=(\d+)\(([\d.]+)/s\)"
                              r" ok=(\d+)\(([\d.]+)/s\) mempoolFull=(\d+) otherErr=(\d+)"
                              r" notReady=(\d+) resent=(\d+)", ln)
                if m:
                    load = {"target": int(m[1]), "actors": int(m[2]), "active": int(m[3]),
                            "sub_total": int(m[4]), "submit_rate": float(m[5]),
                            "ok_total": int(m[6]), "ok_rate": float(m[7]),
                            "mempool_full": int(m[8]), "other_err": int(m[9]),
                            "not_ready": int(m[10]), "resent": int(m[11])}
                    break
            for ln in reversed(lines):
                m = re.search(r"BACKPRESSURE=(true|false)", ln)
                if m:
                    load["backpressure"] = (m[1] == "true")
                    break
            # 5-minute rates from the cumulative counters (committed user tx,
            # submissions, and backpressure retries/resubmissions)
            if "ok_total" in load:
                now = time.time()
                hist = STATE.setdefault("_load_hist", [])
                hist.append((now, load["ok_total"], load["sub_total"], load["resent"]))
                while hist and now - hist[0][0] > 300:
                    hist.pop(0)
                if len(hist) >= 2 and now - hist[0][0] > 5:
                    t0, ok0, sub0, res0 = hist[0]
                    dt = now - t0
                    load["ok_5min"] = round((load["ok_total"] - ok0) / dt, 1)
                    load["sub_5min"] = round((load["sub_total"] - sub0) / dt, 1)
                    load["resent_5min"] = round((load["resent"] - res0) / dt, 1)
                    load["rate_window"] = round(dt)
            # transaction-type mix (cumulative counts) from loadmix's "types:" line
            for ln in reversed(lines):
                if "types:" in ln:
                    mix = {}
                    for tok in ln.split("types:", 1)[1].split():
                        if "=" in tok:
                            k, _, v = tok.rpartition("=")
                            try:
                                mix[k] = int(v)
                            except ValueError:
                                pass
                    if mix:
                        load["mix"] = mix
                    break
    except Exception:
        pass

    # synthetic-transaction flow across partitions (intra-BVN = fast, cross = slow)
    synth = fetch_synthetic()
    if synth:
        now = time.time()
        hist = STATE.setdefault("_synth_hist", [])
        hist.append((now, synth["intra"], synth["cross"]))
        while hist and now - hist[0][0] > 300:
            hist.pop(0)
        if len(hist) >= 2 and now - hist[0][0] > 5:
            t0, i0, c0 = hist[0]; dt = now - t0
            synth["intra_5min"] = round((synth["intra"] - i0) / dt, 2)
            synth["cross_5min"] = round((synth["cross"] - c0) / dt, 2)
            synth["rate_window"] = round(dt)

    # anchor flow (block anchors: BVN->DN up, DN->BVN down) + applied/backlog rate
    anchors = fetch_anchors()
    if anchors:
        now = time.time()
        ah = STATE.setdefault("_anchor_hist", [])
        ah.append((now, anchors["up"], anchors["down"]))
        while ah and now - ah[0][0] > 300:
            ah.pop(0)
        if len(ah) >= 2 and now - ah[0][0] > 5:
            t0, u0, d0 = ah[0]; dt = now - t0
            anchors["up_5min"] = round((anchors["up"] - u0) / dt, 2)
            anchors["down_5min"] = round((anchors["down"] - d0) / dt, 2)
            anchors["rate_window"] = round(dt)

    with LOCK:
        STATE["containers"] = containers
        STATE["corruption"] = corrupt         # leveldb corruption (db, may keep making blocks)
        STATE["consensus_failures"] = cfails  # actual CometBFT consensus failure
        STATE["_corruption_alerts"] = calerts
        STATE["tps_by_part"] = tps_by_part
        STATE["loadmix"] = loadmix
        STATE["load"] = load
        if synth:
            STATE["synth"] = synth
        if anchors:
            STATE["anchors"] = anchors


def fetch_synthetic():
    """Query each partition's synthetic ledger and build the source->dest matrix.
    Returns produced counts per pair, intra-BVN vs cross-BVN totals, and per-pair
    delivery lag (produced by src for dst, minus what dst has delivered)."""
    import urllib.request as ur
    def q(port, scope):
        body = json.dumps({"jsonrpc": "2.0", "id": 1, "method": "query",
                           "params": {"scope": scope}}).encode()
        try:
            r = ur.urlopen(ur.Request("http://127.0.0.1:%d/v3" % port, data=body,
                           headers={"content-type": "application/json"}), timeout=8)
            return json.load(r).get("result", {}).get("account", {})
        except Exception:
            return {}
    def short(u):
        u = (u or "").split("//")[-1]
        if u.startswith("bvn-"):
            return u[4:].split(".")[0]
        if u.startswith("dn"):
            return "DN"
        return u
    produced, delivered = {}, {}
    for src, (port, scope) in SYNTH_LEDGERS.items():
        a = q(port, scope)
        for s in (a.get("sequence") or []):
            dst = short(s.get("url"))
            produced[(src, dst)] = int(s.get("produced") or 0)     # src -> dst
            delivered[(src, dst)] = int(s.get("delivered") or 0)   # dst -> src (applied by src)
    if not produced:
        return None
    intra = sum(v for (s, d), v in produced.items() if s == d)
    cross = sum(v for (s, d), v in produced.items() if s != d)
    lag = 0
    for (s, d), p in produced.items():
        if s == d:
            continue
        lag += max(0, p - delivered.get((d, s), 0))   # dst's delivered-from-src
    return {"produced": {f"{s}>{d}": v for (s, d), v in produced.items()},
            "intra": intra, "cross": cross, "lag": lag}


def fetch_anchors():
    """Query each partition's anchor ledger and build the anchor-exchange table.
    Each ledger (the RECEIVER) lists, per source partition, anchors received and
    delivered (applied). received-delivered = anchor backlog (head-of-line).
    Returns per-route received/delivered/lag plus up/down totals."""
    import urllib.request as ur
    def q(port, scope):
        body = json.dumps({"jsonrpc": "2.0", "id": 1, "method": "query",
                           "params": {"scope": scope}}).encode()
        try:
            r = ur.urlopen(ur.Request("http://127.0.0.1:%d/v3" % port, data=body,
                           headers={"content-type": "application/json"}), timeout=8)
            return json.load(r).get("result", {}).get("account", {})
        except Exception:
            return {}
    def short(u):
        u = (u or "").split("//")[-1]
        if u.startswith("bvn-"):
            return u[4:].split(".")[0]
        if u.startswith("dn"):
            return "DN"
        return u
    routes = {}   # (src, dst) -> {received, delivered}
    for dst, (port, scope) in ANCHOR_LEDGERS.items():
        a = q(port, scope)
        for s in (a.get("sequence") or []):
            src = short(s.get("url"))
            routes[(src, dst)] = {"received": int(s.get("received") or 0),
                                  "delivered": int(s.get("delivered") or 0)}
    if not routes:
        return None
    up   = sum(v["received"] for (s, d), v in routes.items() if d == "DN" and s != "DN")  # BVN->DN
    down = sum(v["received"] for (s, d), v in routes.items() if s == "DN" and d != "DN")  # DN->BVN
    lag  = sum(max(0, v["received"] - v["delivered"]) for v in routes.values())
    return {"routes": {f"{s}>{d}": v for (s, d), v in routes.items()},
            "up": up, "down": down, "lag": lag}


def refresh_backpressure():
    try:
        out = subprocess.run(
            ["curl", "-s", "--max-time", "8", "-X", "POST", "http://127.0.0.1:27680/v3",
             "-H", "content-type: application/json",
             "-d", '{"jsonrpc":"2.0","id":1,"method":"network-status","params":{"partition":"directory"}}'],
            capture_output=True, text=True, timeout=10).stdout
        lim = json.loads(out)["result"]["globals"]["limits"]
        STATE["backpressure_pct"] = int(lim.get("mempoolBackpressurePercent") or BACKPRESS_PCT)
    except Exception:
        pass


def poller():
    refresh_backpressure()
    last_slow = 0
    while True:
        t = time.time()
        try:
            fast_poll()
            if t - last_slow >= SLOW_SEC:
                slow_poll()
                last_slow = t
        except Exception as e:
            with LOCK:
                STATE["alerts"] = [f"monitor error: {e}"]
        time.sleep(FAST_SEC)


PAGE = """<!doctype html><html><head><meta charset=utf-8><title>acc-cl monitor</title>
<style>
body{font-family:ui-monospace,Menlo,monospace;background:#0e1116;color:#d7dde3;margin:0;padding:16px}
h1{font-size:16px;margin:0 0 4px} .sub{color:#7d8794;font-size:12px;margin-bottom:12px}
table{border-collapse:collapse;width:100%;margin:8px 0 18px;font-size:13px}
th,td{border:1px solid #232a33;padding:4px 8px;text-align:right} th{background:#161b22;color:#9aa4af}
td.l,th.l{text-align:left}
.ok{color:#3fb950} .bad{color:#f85149;font-weight:bold} .warn{color:#d29922}
.banner{padding:10px 14px;border-radius:6px;font-size:14px;margin-bottom:14px}
.green{background:#0f2e16;border:1px solid #1f6f2e;color:#56d364}
.red{background:#3d1418;border:1px solid #8e2b2b;color:#ff7b72}
.bar{display:inline-block;height:10px;background:#1f6f2e;border-radius:2px;vertical-align:middle}
.bar.hot{background:#b54a2b} .track{display:inline-block;width:90px;height:10px;background:#21262d;border-radius:2px;vertical-align:middle;margin-right:6px}
pre{background:#161b22;border:1px solid #232a33;padding:8px;border-radius:6px;font-size:12px;overflow:auto;white-space:pre-wrap}
small{color:#7d8794}
/* the load / user-tx card: readable, not tiny grey text */
#load .card{display:flex;flex-wrap:wrap;gap:10px 26px;background:#161b22;border:1px solid #232a33;border-radius:6px;padding:12px 16px;margin-bottom:16px}
#load .stat{min-width:150px}
#load .stat .lbl{color:#9aa4af;font-size:12px;text-transform:uppercase;letter-spacing:.04em}
#load .stat .val{color:#e8edf2;font-size:18px;font-weight:bold;margin-top:2px}
#load .stat .sub2{color:#7d8794;font-size:12px}
</style></head><body>
<h1>acc-cl consensus-load monitor</h1>
<div class=sub id=sub></div>
<div id=banner class=banner></div>
<div id=load></div>
<div id=parts></div>
<div id=synth></div>
<div id=anchors></div>
<div id=nodes></div>
<h3 style="font-size:13px">loadmix</h3><pre id=loadmix></pre>
<script>
function cell(v,cls){return '<td'+(cls?' class="'+cls+'"':'')+'>'+(v==null?'-':v)+'</td>'}
async function tick(){
 let s=await (await fetch('/api')).json()
 let cap=s.mempool_cap, thr=Math.round(cap*s.backpressure_pct/100)
 let otps=(s.overall_tps==null?'-':s.overall_tps.toFixed(1))
 let ttps=(s.total_tps==null?'-':s.total_tps.toFixed(1))
 let dntps=(s.partitions&&s.partitions.DN&&s.partitions.DN.tps!=null?s.partitions.DN.tps.toFixed(1):'-')
 let arrow=s.overall_trend>0?' <span class=ok>▲ rising</span>':(s.overall_trend<0?' <span class=warn>▼ falling</span>':' <span style="color:#7d8794">— steady</span>')
 let ld=s.load||{}
 let n=function(v){return v==null?'-':v.toLocaleString()}
 let f=function(v){return v==null?'-':v}
 let win5=(ld.rate_window!=null?'last '+(ld.rate_window>=60?Math.round(ld.rate_window/60)+'m':ld.rate_window+'s'):'5m')
 // committed messages per user tx (network amplification)
 let amp=(s.total_tps&&ld.ok_5min?(s.total_tps/ld.ok_5min):(s.total_tps&&ld.ok_rate?s.total_tps/ld.ok_rate:null))
 let tile=function(lbl,val,sub2,warn){return '<div class=stat><div class=lbl>'+lbl+'</div><div class=val'+(warn?' style="color:#f0883e"':'')+'>'+val+'</div>'+(sub2?'<div class=sub2>'+sub2+'</div>':'')+'</div>'}
 let loadhtml='<h3 style="font-size:13px;margin:0 0 6px">load / user transactions</h3>'
 if(ld.target==null){loadhtml+='<div class=card><div class=sub2>(no loadmix running)</div></div>'}
 else{loadhtml+='<div class=card>'
   +tile('user TPS (actual / target)', (ld.ok_5min==null?'-':ld.ok_5min)+' / '+ld.target,
         'committed user-tx '+win5+' · actors '+f(ld.active)+'/'+f(ld.actors),
         ld.ok_5min!=null&&ld.target&&ld.ok_5min<ld.target*0.8)
   +tile('total user tx', n(ld.ok_total), 'committed (submitted '+n(ld.sub_total)+')')
   +tile('committed rate ('+win5+')', (ld.ok_5min==null?'-':ld.ok_5min)+'/s', 'submit '+(ld.sub_5min==null?'-':ld.sub_5min)+'/s')
   +tile('retries (backpressure)', n(ld.resent)+(ld.resent_5min!=null?' ('+ld.resent_5min+'/s)':''), 'mempoolFull '+n(ld.mempool_full), ld.mempool_full>0)
   +tile('notReady', n(ld.not_ready), 'otherErr '+n(ld.other_err))
   +tile('committed tx / user tx', amp==null?'-':amp.toFixed(1)+'×', 'incl. anchors+synthetics')
   +(ld.backpressure?tile('backpressure','<span style="color:#f0883e">ENGAGED</span>','shedding user load'):'')
   +'</div>'
   // transaction-type mix (what the load generator is producing)
   if(ld.mix){let ent=Object.entries(ld.mix).sort((a,b)=>b[1]-a[1]);let tot=ent.reduce((x,e)=>x+e[1],0)
     let mh='<div style="font-size:12px;color:#9aa4af;text-transform:uppercase;letter-spacing:.04em;margin:2px 0 4px">transaction mix (cumulative)</div>'
       +'<table style="width:auto"><tr><th class=l>tx type</th><th>count</th><th>% of mix</th><th>signatures</th></tr>'
     for(let [k,v] of ent){let sig=k.indexOf('2of2')>=0||k.indexOf('multisig')>=0?'2 (bundled)':'1 (bundled)'
       mh+='<tr>'+cell(k,'l')+cell(v.toLocaleString())+cell((100*v/tot).toFixed(1)+'%')+cell(sig)+'</tr>'}
     mh+='<tr style="border-top:2px solid #313c49;font-weight:bold">'+cell('TOTAL','l')+cell(tot.toLocaleString())+cell('100%')+cell('')+'</tr></table>'
     loadhtml+=mh}
 }
 document.getElementById('load').innerHTML=loadhtml
 document.getElementById('sub').innerHTML='updated '+new Date(s.ts*1000).toLocaleTimeString()
   +' &middot; <b>committed TPS (&ge;'+s.tps_window+'s avg) — total all partitions, user+synth+anchor: '+ttps+'</b>'+arrow
   +' &middot; BVN total: '+otps+' &middot; DN: '+dntps
   +' &middot; mempool cap '+cap+' &middot; backpressure '+s.backpressure_pct+'% (&ge; '+thr+' sheds user txns)'
 let alerts=s.alerts||[]
 let b=document.getElementById('banner')
 if(alerts.length){b.className='banner red';b.innerHTML='⚠ '+alerts.map(a=>a.replace(/</g,'&lt;')).join(' &nbsp;|&nbsp; ')}
 else{b.className='banner green';b.innerHTML='✓ all partitions advancing, no corruption, no stalls'}
 // partitions
 let ph='<table><tr><th class=l>partition</th><th>TPS (&ge;'+s.tps_window+'s)</th><th>block s</th><th>max height</th><th>advancing</th><th>stall age</th><th>spread</th><th>mempool avg</th><th>mempool max</th><th>nodes up</th></tr>'
 for(let p of ['DN','BVN1','BVN2','BVN3']){let x=s.partitions[p];if(!x)continue
   let mpa=x.mp_avg, frac=mpa==null?0:Math.min(1,mpa/cap), hot=mpa!=null&&mpa>=thr
   let bar='<span class=track><span class="bar'+(hot?' hot':'')+'" style="width:'+(frac*90).toFixed(0)+'px"></span></span>'
   let tps=(x.tps==null?'-':x.tps.toFixed(1)), bt=(x.blocktime==null?'-':x.blocktime.toFixed(2)+'s')
   ph+='<tr>'+cell(p,'l')+cell(tps,'ok')+cell(bt)+cell(x.max_h)+cell(x.advancing?'yes':'NO',x.advancing?'ok':'bad')
     +cell(x.stall_age==null?'-':x.stall_age.toFixed(1)+'s',x.stall_age>30?'bad':'')+cell(x.spread,x.spread>3?'warn':'')
     +'<td>'+bar+(mpa==null?'-':mpa)+'</td>'+cell(x.mp_max,hot?'warn':'')+cell(x.responding,x.responding[0]<x.responding[2]?'bad':'')+'</tr>'}
 ph+='<tr style="border-top:2px solid #3a4452;font-weight:bold">'+cell('TOTAL','l')
   +cell(s.total_tps==null?'-':s.total_tps.toFixed(1),'ok')+cell('')+cell('')+cell('')+cell('')+cell('')+cell('')+cell('')+cell('')+'</tr>'
 ph+='</table>';document.getElementById('parts').innerHTML='<h3 style=\"font-size:13px\">partitions (height progress = primary health)</h3>'+ph
 // synthetic flow (intra-BVN fast vs cross-partition slow)
 let sy=s.synth
 if(sy){let P=['DN','BVN1','BVN2','BVN3'],tot=sy.intra+sy.cross
   let crossPct=(tot>0?(100*sy.cross/tot).toFixed(0):'-')
   let syh='<h3 style="font-size:13px;margin:0 0 6px">synthetic flow (intra-BVN = fast · cross-partition = slow, via DN anchor)</h3><div class=card>'
     +tile('intra-BVN', n(sy.intra), (sy.intra_5min!=null?sy.intra_5min+'/s · ':'')+'same BVN (fast)')
     +tile('cross-partition', n(sy.cross), (sy.cross_5min!=null?sy.cross_5min+'/s · ':'')+'BVN↔BVN (slow)', sy.cross>sy.intra)
     +tile('cross %', crossPct+'%', 'of all synthetics')
     +tile('delivery lag', n(sy.lag), 'produced − delivered (pending)', sy.lag>50)
     +'</div>'
   let m=sy.produced||{}
   let mh='<div style="font-size:12px;color:#9aa4af;text-transform:uppercase;letter-spacing:.04em;margin:2px 0 4px">synthetics produced: source ↓ → destination →</div>'
     +'<table style="width:auto"><tr><th class=l>src \\\\ dst</th>'+P.map(d=>'<th>'+d+'</th>').join('')+'</tr>'
   for(let sr of P){if(!P.some(d=>m[sr+">"+d]!=null))continue
     mh+='<tr><th class=l>'+sr+'</th>'+P.map(d=>{let v=m[sr+">"+d]
       return '<td'+(v==null?'':(sr==d?' class=ok':' style="color:#f0883e"'))+'>'+(v==null?'·':v)+'</td>'}).join('')+'</tr>'}
   mh+='</table>'
   document.getElementById('synth').innerHTML=syh+mh}
 else{document.getElementById('synth').innerHTML=''}
 // anchor flow (block anchors: BVN->DN up, DN->BVN down). lag = received-delivered (HOL backlog)
 let an=s.anchors
 if(an){let R=an.routes||{}
   let anh='<h3 style="font-size:13px;margin:0 0 6px">anchoring (block anchors — cross-partition trust backbone; lag = received−delivered)</h3><div class=card>'
     +tile('anchors up (BVN→DN)', n(an.up), (an.up_5min!=null?an.up_5min+'/s · ':'')+'BVNs anchored to DN')
     +tile('anchors down (DN→BVN)', n(an.down), (an.down_5min!=null?an.down_5min+'/s · ':'')+'DN anchored to BVNs')
     +tile('anchor lag', n(an.lag), 'received − delivered (unapplied)', an.lag>0)
     +'</div>'
   let order=Object.keys(R).sort()
   let mh='<div style="font-size:12px;color:#9aa4af;text-transform:uppercase;letter-spacing:.04em;margin:2px 0 4px">anchor exchange: source → destination</div>'
     +'<table style="width:auto"><tr><th class=l>route</th><th>received</th><th>delivered</th><th>lag</th></tr>'
   for(let k of order){let v=R[k],lg=Math.max(0,(v.received||0)-(v.delivered||0))
     mh+='<tr>'+cell(k.replace('>',' → '),'l')+cell(n(v.received))+cell(n(v.delivered))+cell(n(lg),lg>0?'warn':'ok')+'</tr>'}
   mh+='</table>'
   document.getElementById('anchors').innerHTML=anh+mh}
 else{document.getElementById('anchors').innerHTML=''}
 // nodes
 let nh='<table><tr><th class=l>node</th><th>container</th><th>uptime</th><th>rpc</th><th>restarts</th><th>oom</th><th>CPU%</th><th>mem</th><th>mem%</th><th>ldb (compaction)</th><th>db MB</th><th>BVN h</th><th>sync</th><th>BVN peers</th><th>DN peers</th><th>BVN mp</th><th>DN h</th><th>DN mp</th><th>corrupt</th><th>cons-fail</th></tr>'
 for(let n of Object.keys(s.nodes).sort()){let r=s.nodes[n],c=s.containers[n]||{},cr=s.corruption[n],cf=(s.consensus_failures||{})[n]
   // section off each BVN group: faint shading on alternating groups + a line at each boundary
   let gi=parseInt(n.charAt(3))||0, first=n.endsWith('-1')
   let trstyle=' style="'+(gi%2==0?'background:#141a21;':'')+(first&&n!=='bvn1-1'?'border-top:2px solid #313c49;':'')+'"'
   let upS=c.started?Math.round((Date.now()-Date.parse(c.started))/1000):null
   let up=(upS==null?'-':(upS<60?upS+'s':(upS<3600?Math.floor(upS/60)+'m':Math.floor(upS/3600)+'h')))
   let rpc=r.rpc_ok?'<span class=ok>ok</span>':'<span class=bad>DOWN</span>'
   let sync=(r.bvn_catchup==null?'-':(r.bvn_catchup?'<span class=warn>syncing</span>':'<span class=ok>caught</span>'))
   // ldb file count is the compaction-debt gauge: warn >=300, crash-zone >=450
   let ldb=(c.ldb==null?'-':c.ldb), ldbcls=(c.ldb==null?'':(c.ldb>=450?'bad':(c.ldb>=300?'warn':'ok')))
   let cpucls=(c.cpu==null?'':(c.cpu>=180?'warn':'')), memcls=(c.mem_pct==null?'':(c.mem_pct>=85?'bad':(c.mem_pct>=70?'warn':'')))
   nh+='<tr'+trstyle+'>'+cell(n,'l')+cell(c.status,c.status=='running'?'ok':'bad')
     +cell(up,(upS!=null&&upS<120)?'warn':'')+'<td>'+rpc+'</td>'
     +cell(c.restarts,c.restarts>0?'warn':'')+cell(c.oom?'YES':'no',c.oom?'bad':'')
     +cell(c.cpu==null?'-':c.cpu.toFixed(0),cpucls)+cell(c.mem_mb==null?'-':c.mem_mb.toFixed(1)+' MB')+cell(c.mem_pct==null?'-':c.mem_pct.toFixed(0),memcls)
     +cell(ldb,ldbcls)+cell(c.db_mb==null?'-':c.db_mb)
     +cell(r.bvn_h)+'<td>'+sync+'</td>'+cell(r.bvn_peers,(r.bvn_peers!=null&&r.bvn_peers<1)?'bad':'')+cell(r.dn_peers,(r.dn_peers!=null&&r.dn_peers<1)?'bad':'')
     +cell(r.bvn_mp,r.bvn_mp>=thr?'warn':'')+cell(r.dn_h)+cell(r.dn_mp,r.dn_mp>=thr?'warn':'')
     +cell(cr?((cr.where?cr.where+' ':'')+'×'+cr.count):'-',cr?'bad':'')
     +cell(cf?('×'+cf):'-',cf?'bad':'')+'</tr>'}
 nh+='</table>';document.getElementById('nodes').innerHTML='<h3 style=\"font-size:13px\">nodes</h3>'+nh
 document.getElementById('loadmix').textContent=(s.loadmix||[]).join('')||'(no loadmix log yet)'
}
tick();setInterval(tick,2000)
</script></body></html>"""


class H(BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass

    def do_GET(self):
        if self.path.startswith("/api"):
            with LOCK:
                body = json.dumps(STATE, default=str).encode()
            self.send_response(200)
            self.send_header("content-type", "application/json")
            self.send_header("content-length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
        else:
            body = PAGE.encode()
            self.send_response(200)
            self.send_header("content-type", "text/html; charset=utf-8")
            self.send_header("content-length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)


if __name__ == "__main__":
    threading.Thread(target=poller, daemon=True).start()
    print(f"acc-cl monitor on http://127.0.0.1:{PORT}  (loadmix log: {LOADMIX_LOG})")
    ThreadingHTTPServer(("127.0.0.1", PORT), H).serve_forever()
