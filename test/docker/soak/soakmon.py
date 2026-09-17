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
import atexit
import json, os, re, signal, subprocess, sys, threading, time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

# topology.py sits beside docker-compose.yml, one level up: it describes that
# network, and the ad-hoc tools up there read it too.
sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
import topology

HERE = os.path.dirname(os.path.abspath(__file__))
COMPOSE = os.path.join(HERE, "docker-compose.yml")

# soak.sh writes each run's output to runs/<timestamp>/ and passes that path in
# RUN_DIR. Falling back to HERE keeps the monitor usable standalone, but when a
# run is live its stats and chaos log are NOT here — reading the old fixed
# locations is why the dashboard showed loadgen: null and no chaos events.
RUN_DIR = os.environ.get("RUN_DIR") or HERE
STATS = os.path.join(RUN_DIR, "loadgen-stats.json")
CHAOS = os.path.join(RUN_DIR, "chaos.log")
# The API endpoint, from the topology rather than a constant. This was
# hardcoded to 26660 while topology.BASE_HOST_PORT moved to 26680, so the
# reachability probe hit a port nothing serves and the board reported
# "network down" over three partitions advancing at 0.9 s/block
# (REPORTING-SPEC 1: displayed means measured). Every other reading came
# from topology-derived ports and was correct, which is what made the
# contradiction visible.
API = "http://localhost:%d/v3" % topology.node_ports()[0]
PORT = int(os.environ.get("PORT", "8099"))

# Say something on the way out.
#
# soakmon has now died twice mid-run leaving a ZERO-BYTE log: no traceback, no
# message, nothing to attribute it to. A silent death is the worst kind,
# because the run carries on generating load against a network nobody is
# watching (runs 20260822T052535Z and 20260822T053653Z). Whatever ends this
# process, it should leave a line saying so.
def _log_exit(why):
    try:
        sys.stderr.write("%s soakmon exiting: %s\n" % (
            time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()), why))
        sys.stderr.flush()
    except Exception:
        pass


def _on_signal(sig, _frame):
    _log_exit("signal %s (%s)" % (sig, signal.Signals(sig).name
                                  if hasattr(signal, "Signals") else sig))
    os._exit(128 + sig)


for _sig in (signal.SIGTERM, signal.SIGINT, signal.SIGHUP, signal.SIGQUIT):
    try:
        signal.signal(_sig, _on_signal)
    except (ValueError, OSError):
        pass

atexit.register(lambda: _log_exit("normal exit"))


# Everything this process says must survive an abrupt death.
#
# soakmon died twice on 2026-08-22 leaving a ZERO-BYTE log, which read as "it
# said nothing". It was not: stdout is block-buffered when redirected to a
# file, so even the startup banner sat unflushed in a 4KB buffer and went down
# with the process. We were reading an artefact of buffering as evidence of
# silence, and it cost two runs. Line-buffer both streams and write diagnostics
# to stderr, which is never block-buffered.
try:
    sys.stdout.reconfigure(line_buffering=True)
    sys.stderr.reconfigure(line_buffering=True)
except Exception:
    pass


def log(msg):
    """Timestamped diagnostic. Goes to stderr so it is flushed as written."""
    try:
        sys.stderr.write("%s %s\n" % (
            time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()), msg))
    except Exception:
        pass


# Subprocess health. sh() swallowed every exception, and it runs a `docker
# exec` per node per second — so a timeout storm, a docker daemon hiccup or
# fork failure was completely invisible. Count them and surface the last error
# on the heartbeat rather than logging each one, which would be its own flood.
_SHFAIL = {"n": 0, "last": "", "calls": 0}



# Refresh cadences (seconds): cheap things often, docker-heavy things rarely.
# I_FLOW is 5, not 1: one flow read is a fan-out over every node for every
# ledger and every anchor-sequence chain (REPORTING-SPEC 1b), which at 1 Hz
# is ~70 curl subprocesses a second on the host running the soak. A flow
# matrix does not change meaningfully inside five seconds.
I_STATS, I_HEIGHT, I_WEDGE, I_HEAL, I_CHAOS, I_FLOW = 1, 1, 5, 5, 5, 5
I_MEM = 30  # seconds between rows of mem.csv, the run-long memory/GC series (PLAN S0)
# Per-node runtime series. process_* and go_memstats_* come from the default
# Go collector; go_gc_cycles_* and go_cpu_classes_gc_* need the runtime-metrics
# collector (S0/S6) and read as absent on an older image.
MEM_METRICS = {
    "process_resident_memory_bytes": "rss",
    "process_cpu_seconds_total": "cpu",
    "go_memstats_heap_alloc_bytes": "heapAlloc",
    "go_memstats_heap_inuse_bytes": "heapInuse",
    "go_memstats_next_gc_bytes": "nextGC",
    "go_gc_duration_seconds_count": "gcCount",
    "go_cpu_classes_gc_total_cpu_seconds_total": "gcCpu",
    "go_goroutines": "goroutines",
}
_MEM_LAST = {}   # node -> {field: value, "t": when}; for GC/s and GC-CPU rates
_MEM_CSV_T = [0.0]
# The rolling history: an hour at one sample a second. It was ten minutes,
# which is shorter than most things worth watching -- a memory trend, a
# healing burst after a chaos restart, the shape of a recovery -- and the
# sparklines could not show them. Each sample is a handful of integers; the
# flow matrix, which is ~20x the rest, is snapshotted every
# FLOW_HIST_EVERY samples instead of every one, so the payload grows with
# the window rather than with the window times the matrix.
HIST_MAX = 3600
FLOW_HIST_EVERY = 30
# A partition whose height has not moved for this long is stalled. Matches the
# node-side watchdog so the dashboard and the logs agree on the word.
STALL_SECS = 10

STATE = {"ok": False, "started": int(time.time())}
LOCK = threading.Lock()
_peerid = {}  # container -> peerID (stable per node identity; cached)


def sh(args, timeout=25):
    _SHFAIL["calls"] += 1
    try:
        return subprocess.run(args, capture_output=True, text=True, timeout=timeout).stdout
    except Exception as e:
        _SHFAIL["n"] += 1
        _SHFAIL["last"] = "%s: %s" % (type(e).__name__, str(e)[:160])
        return ""


def query_all_nodes(params, ports=None, method="query"):
    """Ask EVERY node the same query, in parallel, and return each answer's
    `result`. A node that does not answer contributes nothing.

    One node is one point of stale truth (REPORTING-SPEC 1b). The matrix used
    to read one API endpoint: for its own partitions that node answered from
    its own store, as current as its own executor, and for the others the
    router picked a peer, a different one each time. Run 20260906T134054Z
    ended with that node's BVN1 executor 348 blocks behind its siblings, so
    the board showed BVN1 -> Directory as produced 100,804 against received
    102,177 -- the destination had received more than the source had sent.
    Live, the same mixing reads as sent/delivered flickering between a lower
    and a higher value (#4279)."""
    ports = ports or NODE_PORTS
    out, lock, threads = [], threading.Lock(), []
    body = json.dumps({"jsonrpc": "2.0", "id": 1, "method": method, "params": params})

    def one(port):
        txt = sh(["curl", "-s", "-m", "6", "-X", "POST", "http://localhost:%d/v3" % port,
                  "-H", "content-type: application/json", "-d", body], timeout=10)
        try:
            r = json.loads(txt)["result"]
        except Exception:
            return
        with lock:
            out.append(r)

    for port in ports:
        t = threading.Thread(target=one, args=(port,))
        t.start(); threads.append(t)
    for t in threads:
        t.join()
    return out


def merge_sequence_views(views):
    """Merge one ledger's `sequence` list as read from several nodes: per
    remote, the max of each field. Every field is monotone on every node, so
    a lagging node can only under-report and the max is what the network
    holds. Pending keeps the longest list seen."""
    merged, order = {}, []
    for seq in views:
        for e in seq or []:
            url = e.get("url") or ""
            if not url:
                continue
            m = merged.get(url)
            if m is None:
                m = merged[url] = {"url": url, "produced": 0, "received": 0, "delivered": 0, "pending": []}
                order.append(url)
            for f in ("produced", "received", "delivered"):
                m[f] = max(m[f], int(e.get(f) or 0))
            if len(e.get("pending") or []) > len(m["pending"]):
                m["pending"] = e.get("pending") or []
    return [merged[u] for u in order]


# How many nodes answered the last flow read, so a drop in the merged max
# can be told from a drop in the number of nodes it was taken over.
_FLOW_ANSWERS = {"prev": None, "cur": None}


def judge_regression(prev, sample, answered_prev, answered_cur):
    """What to make of a sample against the previous one: the fields that
    went backwards, and whether the alarm must be withheld because the
    sample was read over fewer nodes than the last (a node that stopped
    answering lowers the merged max; that is the monitor losing a node, not
    a sequence number going backwards). Returns (regressions, shrank)."""
    shrank = answered_prev is not None and answered_cur is not None and answered_cur < answered_prev
    if prev is None or shrank:
        return [], shrank
    return sequence_regressions(prev, sample), shrank


def sequence_regressions(prev, cur):
    """Which of sent/recv/deliv went DOWN between two samples (t, sent, recv,
    deliv). After the max across nodes a lower reading is not lag: a sequence
    number never decreases, so it is a value that went backwards in a store,
    and it is an alarm, never absorbed by a high-water mark."""
    out = []
    for name, i in (("sent", 1), ("recv", 2), ("deliv", 3)):
        if cur[i] < prev[i]:
            out.append((name, prev[i], cur[i]))
    return out


def observe_rates(now, generated, syn, anc, elapsed):
    """Rates per second: over the whole run, and the totals behind them.

    `elapsed` is the load generator's own clock, which survives a monitor
    restart; the monitor's own base is used for the produced counters,
    which it can only count from when it started looking. Both are stated
    so a reader can tell one from the other (REPORTING-SPEC 1).
    """
    if not _RATE_BASE:
        _RATE_BASE.update({"t": now, "sProd": syn or 0, "aProd": anc or 0})
    span = now - _RATE_BASE["t"]
    def per_sec(delta, over):
        return round(delta / over, 2) if over and over > 0 and delta >= 0 else None
    user = per_sec(generated or 0, elapsed or 0)
    s = per_sec((syn or 0) - _RATE_BASE["sProd"], span)
    a = per_sec((anc or 0) - _RATE_BASE["aProd"], span)
    total = None
    if user is not None and s is not None and a is not None:
        total = round(user + s + a, 2)
    return {
        "userAvg": user, "synAvg": s, "anchorAvg": a, "totalAvg": total,
        # What the averages are over, so neither is mistaken for the other.
        "userOverSec": round(elapsed or 0, 1), "producedOverSec": round(span, 1),
        "generated": generated or 0, "synProduced": syn or 0, "anchorProduced": anc or 0,
    }


def judge_gap(sent, recv):
    """The sent-but-not-received depth of one flow, and the skew when the
    numbers cannot both be true. `sent` is the source ledger's produced count,
    `recv` the destination's received count, each merged by max over the
    nodes that answered; a destination cannot have received more than any
    source reports produced, so recv > sent is not a depth of -N, it is the
    source read over fewer or staler nodes. Never a negative (REPORTING-SPEC
    1a); the excess is reported as skew (1b)."""
    gap = (sent or 0) - (recv or 0)
    if gap < 0:
        return 0, -gap
    return gap, 0


def compact_flows(flows):
    """The matrix as [sent, recv, deliv] per cell, for the history."""
    return {k: {s: {d: [c.get("sent", 0), c.get("recv", 0), c.get("deliv", 0)] for d, c in row.items()}
                for s, row in m.items()} for k, m in (flows or {}).items()}


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
    """The DN height, and whether ANY node answers.

    Reachability is asked of every node, not one: a single node that is
    down, restarting under chaos, or paused is not the network being
    unreachable, and reporting it as such put a red badge over a healthy
    run. `nodes` says how many of them answered, so a thinning fleet is
    visible rather than binary.
    """
    h = None
    try:
        h = int(curl_api("query", {"scope": "acc://dn.acme/ledger"})["result"]["account"]["index"])
    except Exception:
        pass
    answers = query_all_nodes({"partition": "Directory"}, method="network-status")
    return {"api": "up" if answers else "down", "dnHeight": h,
            "nodes": len(answers), "ofNodes": len(NODE_PORTS)}


ANSI = re.compile(r"\x1b\[[0-9;]*m")


# Read the topology, never assume it. These were a hardcoded 3-BVN list; when
# the network was cut to 2 BVNs the monitor kept polling a partition that no
# longer existed and reported it "unknown" forever, which the dashboard renders
# as a permanently degraded network. See topology.py.
PARTITIONS = topology.partitions()
SCOPE = topology.scopes()
PROBE_PORTS = topology.probe_ports()
NODE_PORTS = topology.node_ports()  # every node: a ledger is read from all of them (REPORTING-SPEC 1b)


def _plabel(u):
    h = u.split("//")[1].split(".")[0]
    return {"dn": "Directory"}.get(h, h.replace("bvn-", ""))


def collect_heights():
    """Read each partition's ledger index from SEVERAL nodes and keep the max.

    A single routed query is a single point of stale truth: in run
    20260824T051249Z a chaos-restarted node halted its executor at block 240
    but kept answering queries from its frozen state, the router pinned to
    it, and the dashboard reported a perfectly healthy network (block 792,
    all validators in sync) as stalled for half an hour. A halted node can
    only under-report, so the max across routes is the honest reading.
    """
    heights = {}
    for p in PARTITIONS:
        best = None
        for port in PROBE_PORTS:
            body = json.dumps({"jsonrpc": "2.0", "id": 1, "method": "query",
                               "params": {"scope": "acc://%s.acme/ledger" % SCOPE[p]}})
            out = sh(["curl", "-s", "-m", "4", "-X", "POST",
                      "http://localhost:%d/v3" % port,
                      "-H", "content-type: application/json", "-d", body], timeout=6)
            try:
                v = int(json.loads(out)["result"]["account"]["index"])
            except Exception:
                continue
            best = v if best is None else max(best, v)
        heights[p] = best
    return heights


# Per-partition record of the last height CHANGE. Liveness is progress over
# time; a height that can be read says only that a node answers the phone.
_PROGRESS = {}
_RATE = {}  # part -> [(t, height)] rolling window for the block-rate display
_FIRST = {}  # part -> (t, height) at first sighting, for the run-long average
_RSS_ALARM = {}  # last RSS-alarm time, rate-limited to one per 5 min
_FLOW_HIST = {}  # (kind,src,dst) -> [(t, sent, recv)] for channel-lag rates
# The first sample's cumulative counters and its time, so a rate can be
# stated over the whole run and not only over the rolling window. A window
# derivative answers "what is it doing now"; the run-long average answers
# "what has this run achieved", which is the number a result is quoted as
# and the one a restarted monitor must not lose.
_RATE_BASE = {}


# Destination partition -> when an inbound synthetic flow first went red.
_DELIVERY_RED = {}


def stalled_by_delivery(flows, now):
    """Per destination partition, how long any inbound synthetic flow has been
    red, keyed by destination; a partition with no red inbound flow is absent.

    Block height is not liveness. On run 20260917T184129Z nine of eleven
    synthetic flows stopped delivering while every partition kept closing
    blocks at 1/s, and neither watchdog fired for two hours and twenty minutes
    because both classify a partition from its height. The flow matrix already
    knew -- it marked those flows red and wrote "delivery STALLED" -- and this
    is the wiring from that knowledge to the partition state the watchdogs
    read (#4285). The first red reading starts a clock rather than condemning,
    as assess_progress does for height.
    """
    red_now = set()
    for src, dsts in ((flows or {}).get("synthetic") or {}).items():
        for dst, c in (dsts or {}).items():
            if c and c.get("sent") and c.get("state") == "red":
                red_now.add(dst)
    out = {}
    for dst in red_now:
        _DELIVERY_RED.setdefault(dst, now)
        out[dst] = now - _DELIVERY_RED[dst]
    for dst in list(_DELIVERY_RED):
        if dst not in red_now:
            del _DELIVERY_RED[dst]
    return out


def apply_delivery_stall(progress, red_for):
    """A partition whose inbound delivery has been red for STALL_SECS is
    stalled for the watchdogs, whatever its height is doing (#4285)."""
    for dst, secs in (red_for or {}).items():
        p = (progress or {}).get(dst)
        if p is None or secs < STALL_SECS:
            continue
        p["state"] = "stalled"
        p["stalledFor"] = max(p.get("stalledFor") or 0, round(secs, 1))
        p["stalledBy"] = "delivery"
    return progress


def assess_progress(heights, now):
    """Classify each partition as live, stalled, or unknown.

    A partition is live only if its height has changed within STALL_SECS. The
    first reading starts the clock rather than declaring health, so a network
    that was already dead when the monitor attached is reported stalled once
    STALL_SECS have passed with no movement — it never gets a free pass.
    """
    out = {}
    # Driven by the reading, not by a module-level topology list: the verdict
    # is about the partitions actually sampled. collect_heights always returns
    # a key per partition (None when unreadable), so this covers the same set
    # in production while letting a test state its own topology.
    for part in heights:
        h = heights.get(part)
        if h is None:
            # Height unreadable. That is not health; say so rather than
            # omitting the partition and letting a blank read as fine.
            out[part] = {"height": None, "state": "unknown", "stalledFor": None}
            continue
        prev = _PROGRESS.get(part)
        if prev is None or prev["height"] != h:
            _PROGRESS[part] = {"height": h, "since": now}
        stalled = now - _PROGRESS[part]["since"]

        # Rolling block rate over the last ~5 minutes. A block is one
        # committed leader group (#4164), so the design target is one per
        # BlockInterval (3s); showing the average time per block is what
        # distinguishes "paced by design" from "dragging" — a slow-ticking
        # height with no rate context was read as the latter.
        win = _RATE.setdefault(part, [])
        win.append((now, h))
        while win and now - win[0][0] > 300:
            win.pop(0)
        sec_per_block = None
        if len(win) >= 2:
            dh = win[-1][1] - win[0][1]
            dt = win[-1][0] - win[0][0]
            if dh > 0 and dt > 0:
                sec_per_block = round(dt / dh, 1)

        # RUNNING average, over the whole run rather than the last five
        # minutes. The two answer different questions and a soak needs both:
        # the window says what the network is doing now, the running average
        # says what it has sustained. A partition that paces at target for
        # four minutes in every five reads healthy on the window and is not.
        _FIRST.setdefault(part, (now, h))
        t0, h0 = _FIRST[part]
        avg_sec_per_block = None
        if h > h0 and now > t0:
            avg_sec_per_block = round((now - t0) / (h - h0), 1)

        out[part] = {
            "height": h,
            "state": "stalled" if stalled >= STALL_SECS else "live",
            "stalledFor": round(stalled, 1),
            "secPerBlock": sec_per_block,
            "avgSecPerBlock": avg_sec_per_block,
            "blocksSeen": h - h0,
        }
    return out


def overall_status(api_up, progress):
    """The headline verdict, derived from progress rather than reachability.

    The old indicator was `curl network-status is not None`, which reports that
    the API process is listening. A wedged network answers that call happily,
    which is how a dashboard showed "network up" over a Directory frozen at
    block 121 with zero anchors, zero synthetics and zero tx/s.
    """
    states = [v["state"] for v in progress.values()]
    # A stalled partition is the finding, whether or not the API answers.
    if any(st == "stalled" for st in states):
        return "stalled"
    if not api_up:
        # No node answered. If partitions have been SEEN TO ADVANCE, the
        # monitor has lost its reach, not the network its health -- and
        # "down" beside three heights climbing at 0.9 s/block is an
        # impossible state (REPORTING-SPEC 1a). Advancing, not merely
        # sighted: one sample proves a height was readable once, which is
        # no evidence of progress and no reason to soften the verdict.
        advancing = any(v.get("blocksSeen") for v in progress.values())
        return "degraded" if advancing else "down"
    if not states or all(st == "unknown" for st in states):
        return "down"
    if any(st == "unknown" for st in states):
        return "degraded"
    return "up"


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


# On-disk database size per node, both engines (dnn + bvnn), in KiB — read in
# the same per-container thread as the metrics scrape. The first non-empty
# sample is kept so growth can be reported as MiB per hour over the run.
_DISK = {}
_DISK_FIRST = {}
_PATHS = topology.container_paths()


def _scrape_one(c, out, lock):
    txt = sh(["docker", "exec", c, "sh", "-c",
              "curl -s -m5 http://localhost:%d/metrics" % METRICS_PORT], timeout=20)
    rows = list(parse_prom(txt))
    sub = _PATHS.get(c)
    du = None
    if sub:
        du = sh(["docker", "exec", c, "sh", "-c",
                 "du -sk /root/.accumulate/%s/dnn/data/accumulate.db /root/.accumulate/%s/bvnn/data/accumulate.db 2>/dev/null" % (sub, sub)], timeout=20)
    with lock:
        out[c] = rows
        if du:
            sizes = {}
            for line in du.splitlines():
                parts = line.split()
                if len(parts) == 2 and parts[0].isdigit():
                    sizes["dn" if "/dnn/" in parts[1] else "bvn"] = int(parts[0])
            if sizes:
                _DISK[c] = sizes


def disk_from(disk, first, now):
    """Fleet view of database size. `disk` is container -> {dn, bvn} KiB;
    `first` is the (time, avg MiB) of the earliest sample, updated in place."""
    d = {"avgMiB": 0, "maxMiB": 0, "maxNode": "", "totalMiB": 0, "growthMiBPerHour": None, "byNode": {}}
    if not disk:
        return d
    per = {c: (v.get("dn", 0) + v.get("bvn", 0)) / 1024.0 for c, v in disk.items()}
    for c, v in disk.items():
        d["byNode"][c] = {"dnMiB": round(v.get("dn", 0) / 1024.0), "bvnMiB": round(v.get("bvn", 0) / 1024.0)}
    vals = list(per.values())
    d["avgMiB"] = round(sum(vals) / len(vals))
    d["maxMiB"] = round(max(vals))
    d["maxNode"] = max(per, key=per.get)
    d["totalMiB"] = round(sum(vals))
    if "t" not in first:
        first["t"], first["avg"] = now, sum(vals) / len(vals)
    elif now - first["t"] >= 120:
        d["growthMiBPerHour"] = round((sum(vals) / len(vals) - first["avg"]) / ((now - first["t"]) / 3600.0), 1)
    return d


# Batch lifecycle totals across the fleet. Pure, so it can be tested without a
# running network — each of these answers a question that previously took a
# grep over gigabytes of container log.
#
#   redelivered  keeps the #4125 skip honest. Skipping a re-delivered
#                certificate is correct, but a nonzero rate means commit dedup
#                is still wrong upstream and the fix is hiding it. Expect 0.
#   retention*   whether the #4128 window is sized right: hits mean it saved a
#                lagging peer, expiries without hits mean it is too generous.
#   blocks/empty an idle network commits empty rounds forever, which reads as a
#                stall to anything watching the ledger index and as health to
#                anything watching block production. Neither says "idle".
LIFE_METRICS = {
    "accumulate_dagbft_certificates_redelivered_total": "redelivered",
    "accumulate_dagbft_batch_retention_hits_total": "retentionHits",
    "accumulate_dagbft_batches_retention_expired_total": "retentionExpired",
    "accumulate_dagbft_batches_retained": "retained",
    "accumulate_dagbft_blocks_produced_total": "blocks",
    "accumulate_dagbft_blocks_empty_total": "blocksEmpty",
}

# Of those, the ones that are a fact about the PARTITION rather than an event
# on the node. Every validator produces the same blocks, so summing them over
# the fleet multiplies the chain's block count by the node count: the board
# read 687,160 blocks produced while the three partitions stood at 111,869
# (run 20260915T042428Z) -- a number no reader can act on, and an impossible
# state under REPORTING-SPEC 1a. These take the max; the rest stay sums,
# because a retained batch or a re-delivery IS a per-node event.
LIFE_MAX = {"blocks", "blocksEmpty"}

_REASON = re.compile(r'reason="([^"]+)"')

# #4169 step 0 — the baseline that gates sharded execution (group 4) and the
# anchors-then-synthetics staging round. Three ratios, each from node counters
# summed over the fleet, so the answer comes off a 12h run:
#   serial share   = serial / (serial + parallel) wall time in ProcessAll.
#                    Below 25% there is nothing for sharding to win.
#   flushes/block  = parallel runs formed per block. ~1 means the block was
#                    serial with extra steps.
#   co-arrival     = synthetics whose proving anchor was applied in the SAME
#                    block, over all synthetics judged. Below 5% the extra
#                    staging round costs more than it saves.
EXEC_METRICS = {
    "accumulate_exec_blocks_total": "blocks",
    "accumulate_exec_flushes_total": "flushes",
}


def mem_from(per):
    """Per-node memory and GC from one scrape, with rates against the previous
    scrape. Feeds the dashboard's node table and mem.csv (PLAN S0)."""
    now = time.time()
    by = {}
    for c, rows in (per or {}).items():
        cur = {"t": now}
        staged, view_age = 0.0, 0.0
        for name, lab, v in rows or ():
            key = MEM_METRICS.get(name)
            try:
                f = float(v)
            except (TypeError, ValueError):
                continue
            if key:
                cur[key] = f
            elif name == "accumulate_bcdb_staged_commits":
                staged = max(staged, f)
            elif name == "accumulate_bcdb_oldest_view_age_seconds":
                view_age = max(view_age, f)
        if len(cur) == 1:
            continue
        prev = _MEM_LAST.get(c)
        out = {"rssMiB": round(cur.get("rss", 0) / 1048576.0),
               "heapAllocMiB": round(cur.get("heapAlloc", 0) / 1048576.0),
               "heapInuseMiB": round(cur.get("heapInuse", 0) / 1048576.0),
               "nextGCMiB": round(cur.get("nextGC", 0) / 1048576.0),
               "gcCount": int(cur.get("gcCount", 0)),
               "cpuSec": cur.get("cpu", 0.0),
               "gcCpuSec": cur.get("gcCpu"),
               "goroutines": int(cur.get("goroutines", 0)),
               "staged": int(staged), "viewAgeS": round(view_age, 1),
               "gcPerSec": None, "gcCores": None, "cpuCores": None}
        if prev and now - prev["t"] > 0:
            dt = now - prev["t"]
            if "gcCount" in cur and "gcCount" in prev:
                out["gcPerSec"] = round(max(0.0, cur["gcCount"] - prev["gcCount"]) / dt, 2)
            if "gcCpu" in cur and "gcCpu" in prev:
                out["gcCores"] = round(max(0.0, cur["gcCpu"] - prev["gcCpu"]) / dt, 2)
            if "cpu" in cur and "cpu" in prev:
                out["cpuCores"] = round(max(0.0, cur["cpu"] - prev["cpu"]) / dt, 2)
        _MEM_LAST[c] = cur
        by[c] = out
    summary = {"byNode": by}
    if by:
        summary["heapMaxMiB"] = max(v["heapAllocMiB"] for v in by.values())
        summary["heapMaxNode"] = max(by, key=lambda c: by[c]["heapAllocMiB"])
        gcs = [v["gcPerSec"] for v in by.values() if v["gcPerSec"] is not None]
        summary["gcPerSecMax"] = max(gcs) if gcs else None
        cores = [v["gcCores"] for v in by.values() if v["gcCores"] is not None]
        summary["gcCoresSum"] = round(sum(cores), 2) if cores else None
        summary["stagedMax"] = max(v["staged"] for v in by.values())
        summary["viewAgeMaxS"] = max(v["viewAgeS"] for v in by.values())
    return summary


def write_mem_csv(mem):
    """Append one row per node to RUN_DIR/mem.csv every I_MEM seconds. The
    dashboard's history keeps ten minutes; this is the twelve-hour series the
    steady-state criteria are judged from."""
    now = time.time()
    if now - _MEM_CSV_T[0] < I_MEM or not mem.get("byNode"):
        return
    _MEM_CSV_T[0] = now
    path = os.path.join(RUN_DIR, "mem.csv")
    cols = ["rssMiB", "heapAllocMiB", "heapInuseMiB", "nextGCMiB", "gcCount", "gcPerSec",
            "gcCores", "cpuSec", "cpuCores", "goroutines", "staged", "viewAgeS"]
    new = not os.path.exists(path)
    ts = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(now))
    with open(path, "a") as f:
        if new:
            f.write("time,node," + ",".join(cols) + "\n")
        for c in sorted(mem["byNode"]):
            v = mem["byNode"][c]
            f.write(ts + "," + c + "," + ",".join("" if v.get(k) is None else str(v.get(k)) for k in cols) + "\n")


def exec_from(per):
    """Sum the #4169 step-0 baseline counters over every node's scrape."""
    ex = {"serialSec": 0.0, "parallelSec": 0.0, "blocks": 0, "flushes": 0,
          "anchorThisBlock": 0, "anchorEarlier": 0, "anchorMissing": 0}
    for rows in (per or {}).values():
        for name, lab, v in rows or ():
            try:
                f = float(v)
            except (TypeError, ValueError):
                continue
            lab = lab if isinstance(lab, dict) else {}
            key = EXEC_METRICS.get(name)
            if key:
                ex[key] += int(f)
            elif name == "accumulate_exec_phase_seconds_total":
                ph = lab.get("phase")
                if ph == "serial":
                    ex["serialSec"] += f
                elif ph == "parallel":
                    ex["parallelSec"] += f
            elif name == "accumulate_exec_synthetic_anchor_total":
                a = lab.get("applied")
                if a == "this_block":
                    ex["anchorThisBlock"] += int(f)
                elif a == "earlier":
                    ex["anchorEarlier"] += int(f)
                elif a == "missing":
                    ex["anchorMissing"] += int(f)
    return ex


def life_from(per):
    """Sum the batch-lifecycle metrics over every node's scrape."""
    life = {"redelivered": 0, "retentionHits": 0, "retentionExpired": 0,
            "retained": 0, "blocks": 0, "blocksEmpty": 0, "waitsByReason": {}}
    for rows in (per or {}).values():
        for name, lab, v in rows or ():
            try:
                n = int(float(v))
            except (TypeError, ValueError):
                continue
            key = LIFE_METRICS.get(name)
            if key:
                # A per-node event sums over the fleet; a partition fact does
                # not (LIFE_MAX).
                if key in LIFE_MAX:
                    life[key] = max(life[key], n)
                else:
                    life[key] += n
            elif name == "accumulate_dagbft_batch_waits_total":
                # Labels arrive as a parsed dict since the scrape refactor;
                # the regex path is kept for raw-string rows. This crashed
                # the whole collector the FIRST time a batch wait ever
                # occurred (dict is truthy, search(dict) is a TypeError) —
                # the dashboard froze at the exact moment it became
                # interesting (run 20260824T051249Z).
                if isinstance(lab, dict):
                    reason = lab.get("reason")
                else:
                    m = _REASON.search(lab or "")
                    reason = m.group(1) if m else None
                if reason:
                    life["waitsByReason"][reason] = \
                        life["waitsByReason"].get(reason, 0) + n
    return life


HEAL_OUTCOMES = ("answered", "not-yet", "miss", "failed", "lagging", "lagging-miss")
PROOF_OUTCOMES = ("validated", "staged", "disproved", "conflict", "invalid", "unbound", "refused", "duplicate")
JUDGED = ("proven", "unproven", "collected", "tossed", "anchor-collected", "anchor-tossed", "this_block", "earlier", "missing")


def heals_from(per):
    """Healing, from the families the node EXPORTS.

    The monitor read accumulate_crosschain_heals_total and five siblings for
    weeks after the last of them left the tree, and rendered the absence as
    0: a 12-hour run whose log held 4,028 span requests and 18,171 healed
    entries showed HEALS 0, ANCHOR 0, SYNTHETIC 0, HEAL ERRORS 0 and a
    healing panel reading "none yet" (#4279, run 20260915T042428Z). That is
    the clause-1 violation REPORTING-SPEC records against #4093, still open.
    A family that no node reported is None here and "not measured" on the
    board; zero means zero.

    Counters sum across nodes (each node's requester counts its own asks);
    the staging gauges take the max per stream, since staging is identical
    on every node (executor spec, invariant 6)."""
    requests = {o: 0 for o in HEAL_OUTCOMES}
    by_stream = {}
    proofs = {o: 0 for o in PROOF_OUTCOMES}
    judged = {o: 0 for o in JUDGED}
    held = {}
    seen = set()
    entries = 0
    entries_by = {}
    for rows in (per or {}).values():
        for name, lab, v in rows or ():
            try:
                n = int(float(v))
            except (TypeError, ValueError):
                continue
            lab = lab if isinstance(lab, dict) else {}
            if name == "accumulate_conductor_heal_requests_total":
                seen.add("requests")
                o = lab.get("outcome", "")
                requests[o] = requests.get(o, 0) + n
                key = "%s->%s" % (lab.get("source", "?"), lab.get("destination", "?"))
                by_stream.setdefault(key, {o: 0 for o in HEAL_OUTCOMES})
                by_stream[key][o] = by_stream[key].get(o, 0) + n
            elif name == "accumulate_conductor_heal_entries_total":
                # Split by what became of the entry, so a repaired stream can
                # be told from a healer asking for what it already has, and
                # both from nothing to do (#4283).
                seen.add("entries"); entries += n
                o = lab.get("outcome", "")
                entries_by[o] = entries_by.get(o, 0) + n
            elif name == "accumulate_exec_staged_proofs_total":
                seen.add("proofs")
                o = lab.get("outcome", ""); proofs[o] = proofs.get(o, 0) + n
            elif name == "accumulate_exec_synthetic_anchor_total":
                seen.add("judged")
                o = lab.get("applied", ""); judged[o] = judged.get(o, 0) + n
            elif name in ("accumulate_staging_held_entries", "accumulate_staging_held_bytes"):
                seen.add("held")
                key = (lab.get("ledger", "?"), lab.get("source", "?"))
                h = held.setdefault(key, {"ledger": key[0], "source": key[1], "entries": 0, "bytes": 0})
                f = "entries" if name.endswith("entries") else "bytes"
                h[f] = max(h[f], n)
    out = {
        "measured": bool(seen),
        "requests": dict(requests, total=sum(requests.values())) if "requests" in seen else None,
        "byStream": by_stream if "requests" in seen else None,
        "entries": entries if "entries" in seen else None,
        "applied": entries_by.get("applied", 0) if "entries" in seen else None,
        "notRequired": entries_by.get("not-required", 0) if "entries" in seen else None,
        "proofs": proofs if "proofs" in seen else None,
        "judged": judged if "judged" in seen else None,
        "held": None,
        # What earlier readers keyed on. `total` feeds the sparkline and the
        # index; `errors` the tile; `stuck` is what seizewatch trips on and
        # no family reports it today, so it is not measured, not 0.
        "total": entries if "entries" in seen else None,
        "errors": (requests.get("failed", 0) + requests.get("miss", 0)) if "requests" in seen else None,
        "stuck": None, "stuckStream": "",
    }
    if "held" in seen:
        streams = sorted(held.values(), key=lambda h: (-h["entries"], h["ledger"], h["source"]))
        out["held"] = {"entries": sum(h["entries"] for h in streams),
                       "bytes": sum(h["bytes"] for h in streams), "streams": streams}
    return out


HANDOFF = ("arrived", "executed", "unmarshal-failed", "process-failed", "status-failed")


def handoff_from(per):
    """What became of transactions at the consensus/execution hand-off.

    `arrived` minus `executed` is what did not execute -- the question
    #4132 exists to answer, and the one a board could not show while the
    accounting lived only in a per-block log line. Summed over the fleet:
    every validator executes the same block, so the ABSOLUTE numbers are
    a fleet multiple, but the gap is what matters and it scales the same.
    """
    out = {o: 0 for o in HANDOFF}
    seen = False
    for rows in (per or {}).values():
        for name, lab, v in rows or ():
            if name != "accumulate_dagbft_handoff_transactions_total":
                continue
            try:
                n = int(float(v))
            except (TypeError, ValueError):
                continue
            o = (lab if isinstance(lab, dict) else {}).get("outcome", "")
            out[o] = out.get(o, 0) + n
            seen = True
    if not seen:
        return {"measured": False, "arrived": None, "executed": None, "unexecuted": None}
    out["measured"] = True
    out["unexecuted"] = max(0, out["arrived"] - out["executed"])
    # Accounted for: the three failure outcomes. Anything left is loss with
    # no recorded reason, which is the alarming case.
    named = out["unmarshal-failed"] + out["process-failed"] + out["status-failed"]
    out["unaccounted"] = max(0, out["unexecuted"] - max(0, named - out["status-failed"]))
    return out


def wedges_from(per):
    """Dropped cross-partition envelopes, from the dispatcher's own counter
    (reason: deadline or queue-full) and, when chaos is on, the debug drop
    counter. None when neither family was reported."""
    by_dest = {p: 0 for p in PARTITIONS}
    by_reason = {}
    seen = False
    for rows in (per or {}).values():
        for name, lab, v in rows or ():
            try:
                n = int(float(v))
            except (TypeError, ValueError):
                continue
            lab = lab if isinstance(lab, dict) else {}
            if name in ("accumulate_dispatcher_drops_total", NS + "_debug_dropped_total"):
                seen = True
                d = lab.get("destination") or lab.get("dest") or "?"
                by_dest[d] = by_dest.get(d, 0) + n
                # `reason` on the dispatcher, `kind` on the chaos debug
                # counter -- reading only the first put every injected drop
                # in a bucket called "?".
                r = lab.get("reason") or lab.get("kind") or lab.get("type") or "?"
                by_reason[r] = by_reason.get(r, 0) + n
    if not seen:
        # Not {p: 0}: an absent instrument renders as absent, never as a
        # column of zeros (REPORTING-SPEC 1).
        return {"measured": False, "total": None, "byDest": {}, "byReason": {}}
    return {"measured": True, "total": sum(by_reason.values()), "byDest": by_dest, "byReason": by_reason}


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

    heals = heals_from(per)
    drops = wedges_from(per)
    handoff = handoff_from(per)

    # Per-node RSS and goroutines, and the memory detail behind them. An
    # unbounded goroutine count is what #4089 looked like before anyone noticed
    # the memory -- and it shows up there hours earlier than RSS does.
    nodes = {"count": 0, "rssMinMiB": 0, "rssAvgMiB": 0, "rssMaxMiB": 0, "rssMaxNode": "",
             "grMin": 0, "grAvg": 0, "grMax": 0, "grMaxNode": "", "byNode": {}}
    rss, gor = {}, {}
    mem = mem_from(per)
    nodes["mem"] = mem
    # Batch lifecycle, added after the 20260822 night. Each answers a question
    # that previously needed a grep over gigabytes of container log.
    #   redelivered -- keeps the #4125 skip honest: skipping a re-delivered
    #     certificate is correct, but a nonzero rate means commit dedup is
    #     still wrong upstream and the fix is hiding it. Should be 0.
    #   retention hits/expired/held -- whether the #4128 window is sized right.
    #   blocks vs empty -- an idle network commits empty rounds forever, which
    #     reads as a stall to anything watching the ledger index and as health
    #     to anything watching block production. Neither says "idle".
    life = life_from(per)
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
        m = mem["byNode"].get(c)
        if m:
            nodes["byNode"][c].update({"heapMiB": m.get("heapAllocMiB"), "gcPerSec": m.get("gcPerSec"),
                                       "gcCores": m.get("gcCores"), "staged": m.get("staged"),
                                       "viewAgeS": m.get("viewAgeS")})
    with lock:
        nodes["disk"] = disk_from(dict(_DISK), _DISK_FIRST, time.time())
    for c, v in nodes["disk"]["byNode"].items():
        nodes["byNode"].setdefault(c, {}).update(v)
    # The flow matrix comes from the ledgers over the API, read from every
    # node (collect_flows_api). A metrics path used to sit here waiting for
    # an accumulate_crosschain_sequence gauge that no node has exported for
    # weeks; it filled nothing and fell through every sample.
    flows, syn_prod, anc_prod = collect_flows_api()

    return {"heals": heals, "wedges": drops, "handoff": handoff, "flows": flows, "life": life, "exec": exec_from(per),
            "synProduced": syn_prod, "ancProduced": anc_prod, "nodeStats": nodes,
            "nodes": len(cs), "scraped": sum(1 for r in per.values() if r)}


def collect_flows_api():
    # Flow matrix from the ledgers. Each partition's synthetic/anchor
    # ledger holds one entry per remote partition carrying BOTH directions:
    # `produced` counts what THIS partition sent to the remote, while
    # `received`/`delivered` count what the remote sent to THIS one. So a single
    # pass over all four ledgers fills every src->dst cell.
    flows = {"synthetic": {}, "anchor": {}}
    syn_prod = anc_prod = 0
    answered = []  # nodes that answered each ledger read, for the regression guard

    def cell(kind, src, dst):
        return flows[kind].setdefault(src, {}).setdefault(
            dst, {"sent": 0, "recv": 0, "deliv": 0, "pending": 0})

    for kind, path in (("synthetic", "synthetic"), ("anchor", "anchors")):
        for dst in PARTITIONS:
            views = []
            for r in query_all_nodes({"scope": "acc://%s.acme/%s" % (SCOPE[dst], path)}):
                try:
                    views.append(r["account"]["sequence"] or [])
                except Exception:
                    continue
            answered.append(len(views))
            if not views:
                continue
            seq = merge_sequence_views(views)
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

    # Anchor "sent" from the source's anchor-sequence CHAIN height. The anchor
    # ledger has no outbound `produced` writer on any lineage (checked 2026-08:
    # no branch writes it), so reading it yields the impossible display
    # "received 18 / sent 0" — received is PROOF of sent. The sequence chain is
    # the actual bookkeeping: a BVN anchors only to the DN, and the DN sends
    # the same sequence to every BVN, so the chain height IS the sent count.
    for src in PARTITIONS:
        h = 0
        for r in query_all_nodes({"scope": "acc://%s.acme/anchors" % SCOPE[src],
                                  "query": {"queryType": "chain", "name": "anchor-sequence"}}):
            try:
                h = max(h, int(r["count"]))
            except Exception:
                continue
        if h <= 0:
            continue
        dsts = ["Directory"] if src != "Directory" else list(PARTITIONS)
        for dst in dsts:
            c = cell("anchor", src, dst)
            c["sent"] = max(c["sent"], h)
            anc_prod = max(anc_prod, h)

    # ... and it has to be filled BEFORE anything is judged from `sent`: the
    # undelivered count, the in-flight depth and the skew below all read it.
    # Judged first, every anchor cell saw sent=0 and reported "skew: dst
    # reports N more received than any source produced" over a healthy
    # stream (run 20260916T160646Z).

    # undeliv = produced at the source minus received at the destination. This is
    # the ONLY signal that catches a missing PREFIX or a trailing drop: when the
    # very first messages of a stream are lost and nothing follows them, the
    # destination never forms a pending window, so recv == deliv == 0 and the
    # recv-deliv gap stays 0 forever while the messages are gone. A 23h soak ran
    # with DN->BVN1 stuck at produced=2 received=0 and every gap-based check
    # reported healthy.
    # The weakest read of this sample: every cell was merged over at least
    # this many nodes. Carried so the judgement below can tell a value that
    # fell from a node that stopped answering.
    _FLOW_ANSWERS["prev"], _FLOW_ANSWERS["cur"] = _FLOW_ANSWERS["cur"], min(answered) if answered else 0

    for kind in flows:
        for src, row in flows[kind].items():
            for dst, c in row.items():
                c["undeliv"] = max(0, c.get("sent", 0) - c.get("recv", 0))

    # Channel lag IN SECONDS, and channel STATE. Two distinct failure modes
    # must not share a color (learned live, run 20260824T114552Z):
    #
    #  - sent-but-not-received is pipeline depth: gap / receive rate, judged
    #    against the EXPECTED latency. Expected is a fixed floor plus a
    #    per-block term — dispatch ticks, gossip hops and anchor cadence do
    #    NOT shrink with the block interval, so a pure 7x-block-time model
    #    called healthy 12-20s pipes red the moment blocks went to 1s.
    #
    #  - received-but-undelivered is messages parked IN ORDER behind holes.
    #    While delivery advances and healing fills holes, that is repair in
    #    progress (amber at worst); RED is reserved for delivery actually
    #    stopped (rate ~0 with a standing backlog) — the executor-wedge
    #    signature.
    now_t = time.time()
    with LOCK:
        spbs = [v.get("secPerBlock") for v in (STATE.get("progress") or {}).values()
                if v.get("secPerBlock")]
    spb = sorted(spbs)[len(spbs)//2] if spbs else 1.0
    expected = 8.0 + 7.0 * spb
    for kind in flows:
        for src, row in flows[kind].items():
            for dst, c in row.items():
                key = (kind, src, dst)
                hist = _FLOW_HIST.setdefault(key, [])
                sample = (now_t, c.get("sent", 0), c.get("recv", 0), c.get("deliv", 0))
                # A value can only be compared against one read over the same
                # or a larger set of nodes. Chaos restarts and pauses nodes,
                # and query_all_nodes drops one that does not answer, so the
                # merged max falls when the node holding the high-water
                # reading goes away. That is the monitor losing a node, not a
                # sequence number going backwards, and it must not raise the
                # alarm reserved for the latter.
                regressed, shrank = judge_regression(hist[-1] if hist else None, sample,
                                                     _FLOW_ANSWERS["prev"], _FLOW_ANSWERS["cur"])
                hist.append(sample)
                while hist and now_t - hist[0][0] > 90:
                    hist.pop(0)
                if regressed:
                    c["regressed"] = ["%s %d->%d" % r for r in regressed]
                    log("SEQUENCE REGRESSION %s %s->%s: %s (%s nodes answered)"
                        % (kind, src, dst, ", ".join(c["regressed"]), _FLOW_ANSWERS["cur"]))
                elif shrank:
                    c["fewerNodes"] = [_FLOW_ANSWERS["prev"], _FLOW_ANSWERS["cur"]]
                gap = c.get("sent", 0) - c.get("recv", 0)
                # `sent` is the SOURCE ledger's produced count and `recv` the
                # DESTINATION ledger's received count -- two ledgers on two
                # sets of nodes, each merged by max over whoever answered. A
                # destination cannot have received more than any source
                # reports produced, so a negative gap is not a measurement,
                # it is the source read over fewer or staler nodes than the
                # destination (REPORTING-SPEC 1a: no impossible states; 1b:
                # one node is stale truth). Shown as skew, never as a
                # negative lag.
                gap, skew = judge_gap(c.get("sent", 0), c.get("recv", 0))
                c["skew"] = skew
                pending = max(0, c.get("recv", 0) - c.get("deliv", 0))
                recv_rate = deliv_rate = 0.0
                if len(hist) >= 2:
                    dt = hist[-1][0] - hist[0][0]
                    if dt > 0:
                        recv_rate = (hist[-1][2] - hist[0][2]) / dt
                        deliv_rate = (hist[-1][3] - hist[0][3]) / dt
                lag_s = None
                if len(hist) >= 2:
                    if recv_rate > 0:
                        lag_s = gap / recv_rate
                    elif gap > 0:
                        lag_s = float("inf")
                    else:
                        lag_s = 0.0

                state, note = "ok", ""
                if skew > 1:
                    # One unit is the two instruments read a tick apart (the
                    # anchor chain height and the ledger are separate
                    # queries); more than that is a stale or missing read.
                    note = "skew: dst reports %d more received than any source view produced (read over %s nodes)" % (skew, _FLOW_ANSWERS["cur"])
                elif lag_s is not None and lag_s > 0:
                    note = "~%ds in flight" % min(lag_s, 999999)
                if lag_s is not None:
                    if lag_s >= 4 * expected:
                        state = "red"
                        note = "lag %s (exp %ds)" % ("∞" if lag_s == float("inf") else "%ds" % lag_s, expected)
                    elif lag_s >= 2 * expected:
                        state = "warn"
                        note = "lag %ds (exp %ds)" % (lag_s, expected)
                if pending > 0:
                    if deliv_rate <= max(0.2, 0.02 * recv_rate):
                        state = "red"
                        note = "%d undelivered, delivery STALLED" % pending
                    else:
                        drain_s = pending / deliv_rate
                        if drain_s >= 4 * expected:
                            state = "red"
                            note = "%d undelivered (~%ds behind)" % (pending, drain_s)
                        else:
                            if state == "ok":
                                state = "warn" if drain_s >= 2 * expected else "ok"
                            note = "%d healing, draining" % pending if state == "ok" else "%d undelivered (~%ds behind)" % (pending, drain_s)
                if not note and lag_s == 0:
                    note = "caught up"
                if regressed:
                    # Not lag and not a display choice: a sequence number
                    # went backwards after the max across nodes. Red, named.
                    state = "red"
                    note = "WENT BACKWARDS %s" % ", ".join(c["regressed"])
                c["lagS"] = None if lag_s is None else (999999 if lag_s == float("inf") else round(lag_s, 1))
                c["state"] = state
                c["note"] = note
                c["expLagS"] = round(expected, 1)

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


# The partition list is spliced in from the topology, not spelled out: asking a
# node for the heal counters of a partition it does not host returns nothing,
# and the missing rows read as "no healing happened" rather than "never asked".
HEAL_SNIPPET = r'''
nid=$(curl -s -m5 -X POST http://localhost:26660/v3 -H "content-type: application/json" -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"node-info\",\"params\":{}}" | grep -oE "\"peerID\":\"[^\"]+\"" | cut -d"\"" -f4)
for part in __PARTITIONS__; do
  r=$(curl -s -m5 -X POST http://localhost:26660/v3 -H "content-type: application/json" -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"consensus-status\",\"params\":{\"partition\":\"$part\",\"nodeID\":\"$nid\"}}")
  s=$(echo "$r" | grep -oE "\"syntheticHeals\":[0-9]+" | cut -d: -f2); a=$(echo "$r" | grep -oE "\"anchorHeals\":[0-9]+" | cut -d: -f2)
  echo "$part ${s:-0} ${a:-0}"
done
'''.replace("__PARTITIONS__", " ".join(PARTITIONS))


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


def _self_stats():
    """Resource picture of this process, for the heartbeat."""
    out = {"threads": threading.active_count(), "fds": -1, "rssMiB": -1}
    try:
        out["fds"] = len(os.listdir("/proc/self/fd"))
    except Exception:
        pass
    try:
        with open("/proc/self/statm") as f:
            out["rssMiB"] = round(int(f.read().split()[1]) * 4096 / 1048576)
    except Exception:
        pass
    return out


def heartbeat(started):
    """One line a minute proving the monitor is alive, and showing whether it
    is accumulating threads, file descriptors or subprocess failures.

    soakmon died twice mid-run with nothing to attribute it to. If it is
    leaking — it runs a `docker exec` per node per second — this is where that
    becomes visible, hours before it becomes fatal."""
    while True:
        time.sleep(60)
        st = _self_stats()
        up = int(time.time() - started)
        msg = ("heartbeat up=%dh%02dm threads=%d fds=%d rss=%dMiB "
               "shcalls=%d shfails=%d" % (
                   up // 3600, (up % 3600) // 60, st["threads"], st["fds"],
                   st["rssMiB"], _SHFAIL["calls"], _SHFAIL["n"]))
        if _SHFAIL["last"]:
            msg += " lastShErr=%s" % _SHFAIL["last"]
        log(msg)


def collector():
    last = {"height": 0, "metrics": 0, "chaos": 0}
    hist = []
    fails = 0
    while True:
        try:
            _collect_once(last, hist)
            fails = 0
        except Exception as e:
            # A collector that dies leaves the dashboard serving stale data
            # forever, which is worse than saying so — the page looked healthy
            # while nothing was being read. Log and carry on.
            fails += 1
            import traceback
            log(traceback.format_exc().strip().replace("\n", " | "))
            log("collector error (%d in a row): %s: %s" % (
                fails, type(e).__name__, str(e)[:200]))
            time.sleep(1)


def _collect_once(last, hist):
    if True:
        now = time.time()
        upd = {}
        upd["loadgen"] = read_stats()
        if now - last["height"] >= I_HEIGHT:
            upd["network"] = collect_height()
            upd["heights"] = collect_heights()
            upd["progress"] = assess_progress(upd["heights"], now)
            upd["status"] = overall_status(
                upd["network"].get("api") == "up", upd["progress"])
            last["height"] = now
        # One authoritative scrape of every node's /metrics feeds heals, wedges
        # (drops), and the flow matrices — no log parsing, no ledger scraping.
        if now - last["metrics"] >= I_FLOW:
            m = collect_metrics()
            upd["heals"] = m["heals"]
            upd["wedges"] = m["wedges"]
            upd["handoff"] = m.get("handoff", {})
            upd["synProduced"] = m["synProduced"]
            upd["ancProduced"] = m["ancProduced"]
            upd["life"] = m.get("life", {})
            upd["exec"] = m.get("exec", {})
            upd["scrape"] = {"nodes": m["nodes"], "scraped": m["scraped"]}
            upd["nodeStats"] = m.get("nodeStats", {})
            try:
                write_mem_csv(upd["nodeStats"].get("mem", {}))
            except Exception as e:
                log("mem.csv: %s" % e)
            # OOM early warning. Run 20260824T065208Z grew from 146MiB to the
            # 4GiB cgroup limit and SEVEN containers were OOM-killed (exit
            # 137) before anything said a word — the death was reconstructed
            # from artifacts. Say it while there is still time to act, and
            # say it in soak.log where the session watchers look.
            try:
                ns = upd["nodeStats"]
                if ns.get("rssMaxMiB", 0) > 3072 and time.time() - _RSS_ALARM.get("t", 0) > 300:
                    _RSS_ALARM["t"] = time.time()
                    line = "%s RSS ALARM: %s at %dMiB of 4GiB — OOM kill approaching\n" % (
                        time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
                        ns.get("rssMaxNode"), ns.get("rssMaxMiB", 0))
                    log(line.strip())
                    with open(os.path.join(RUN_DIR, "soak.log"), "a") as f:
                        f.write(line)
            except Exception:
                pass
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
            # Delivery stalls reach the partition state the watchdogs read,
            # after every merge, because assess_progress rebuilds that state
            # from height alone on each height tick (#4285).
            red = stalled_by_delivery((STATE.get("matrix") or {}).get("flows"), now)
            STATE["deliveryStall"] = red
            if STATE.get("progress"):
                apply_delivery_stall(STATE["progress"], red)
                STATE["status"] = overall_status(
                    (STATE.get("network") or {}).get("api") == "up", STATE["progress"])
            lg = STATE.get("loadgen") or {}
            w = STATE.get("wedges") or {}
            h = STATE.get("heals") or {}
            STATE["rates"] = observe_rates(
                now, lg.get("generated", 0), STATE.get("synProduced", 0),
                STATE.get("ancProduced", 0), lg.get("elapsedSec", 0))
            hist.append({"t": int(now),
                         "generated": lg.get("generated", 0),
                         "wedges": w.get("total") or 0,
                         "heals": h.get("total") or 0,
                         "notYet": ((h.get("requests") or {}).get("not-yet") or 0),
                         "sProd": STATE.get("synProduced", 0),
                         "aProd": STATE.get("ancProduced", 0),
                         "heapMax": ((STATE.get("nodeStats") or {}).get("mem") or {}).get("heapMaxMiB", 0),
                         "rssMax": (STATE.get("nodeStats") or {}).get("rssMaxMiB", 0),
                         })
            # The matrix, every FLOW_HIST_EVERY samples. It is ~20x the rest
            # of a sample, and /data re-serializes the whole history on every
            # hit (the dashboard polls at 1 Hz, three watchdogs besides); ten
            # seconds of resolution is ample for "sent was higher earlier"
            # (REPORTING-SPEC 1b).
            if len(hist) % FLOW_HIST_EVERY == 0:
                hist[-1]["flows"] = compact_flows((STATE.get("matrix") or {}).get("flows"))
            if len(hist) > HIST_MAX:
                del hist[0:len(hist) - HIST_MAX]
            STATE["history"] = list(hist)
            # "not-yet" is a request answered "that is in flight, not
            # missing": a cumulative count of it only climbs, and on the
            # Directory's streams it climbs forever by construction, so the
            # number that means something is the rate (#4288).
            try:
                old = next((e for e in hist if now - e["t"] <= 300), hist[0])
                dt = now - old["t"]
                if dt > 0 and "notYet" in old:
                    STATE.setdefault("heals", {})["notYetPerMin"] = round(60.0 * (hist[-1]["notYet"] - old["notYet"]) / dt, 1)
            except Exception:
                pass
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
/* The stats panel: one column per KIND of value, numbers right-aligned so a
   column reads as a column. Every id is unchanged; only the arrangement is. */
.stats{display:grid;grid-template-columns:repeat(auto-fit,minmax(200px,1fr));gap:6px 28px;align-items:start}
.col h4{margin:0 0 2px;color:var(--mut);font-size:10px;text-transform:uppercase;letter-spacing:.06em;font-weight:600}
.col .sub{color:var(--mut);font-size:10px;text-transform:uppercase;letter-spacing:.04em;margin-top:6px;opacity:.8}
.kv{display:grid;grid-template-columns:max-content 1fr;gap:0 8px;align-items:baseline}
.kv b,.kv .n{font-size:15px;font-weight:680;font-variant-numeric:tabular-nums;text-align:right}
.kv .mut{font-size:13px;font-variant-numeric:tabular-nums;text-align:right}
.kv .l,.kv .sl{color:var(--mut);font-size:10.5px}
.kv .cap{grid-column:1/-1;color:var(--mut);font-size:10.5px;margin-top:1px}
#heights{display:contents}
</style></head><body><div class=wrap>
<header>
  <h1>Synthetic-healing soak</h1>
  <span id=phase class="badge phase">—</span>
  <span id=net class="badge down">network ?</span>
  <span class=sub id=sub></span>
</header>
<div class="grid cards" id=cards></div>
<div class=panel style="padding:9px 12px">
  <div class=stats>
    <div class=col><h4>rates</h4>
      <div class=sub>Current tx Rates (last 30 s)</div>
      <div class=kv>
        <b id=rtotal>—</b><span class=sl>total tx/s</span>
        <b id=ruser>—</b><span class=sl>user tx/s</span>
        <span class=mut id=rtgt>—</span><span class=sl>target tx/s</span>
        <b id=rsyn>—</b><span class=sl>synthetic tx/s</span>
        <b id=ranc>—</b><span class=sl>anchors/s</span>
        <span class=mut id=rratio>—</span><span class=sl>synth+anchor share of total</span>
      </div>
      <div class=sub>Total Test Rates (whole run)</div>
      <div class=kv>
        <b id=ratotal>—</b><span class=sl>total tx/s</span>
        <b id=rauser>—</b><span class=sl>user tx/s</span>
        <span class=mut id=rasyn>—</span><span class=sl>synthetic tx/s</span>
        <span class=mut id=raanc>—</span><span class=sl>anchors/s</span>
        <span class=mut id=rashare>—</span><span class=sl>synth+anchor share of total</span>
        <span class=cap id=raover></span>
      </div>
    </div>
    <div class=col><h4>chain</h4>
      <div class=sub>heights</div>
      <div class=kv><span id=heights></span></div>
      <div class=sub>blocks</div>
      <div class=kv>
        <b id=lblocks>—</b><span class=sl>blocks produced (#)</span>
        <b id=lempty>—</b><span class=sl>empty blocks (#)</span>
        <span class=cap id=lidle></span>
      </div>
    </div>
    <div class=col><h4>nodes</h4>
      <div class=sub>resident memory</div>
      <div class=kv>
        <b id=nrssavg>—</b><span class=sl>avg</span>
        <b id=nrssmax>—</b><span class=sl>max</span>
        <span class=mut id=nrssmin>—</span><span class=sl>min</span>
        <span class=cap id=nrssnode></span>
      </div>
      <div class=sub>goroutines</div>
      <div class=kv>
        <b id=ngravg>—</b><span class=sl>avg (#)</span>
        <b id=ngrmax>—</b><span class=sl>max (#)</span>
        <span class=mut id=ngrmin>—</span><span class=sl>min (#)</span>
        <span class=cap id=ngrnode></span>
      </div>
      <div class=sub>database on disk</div>
      <div class=kv>
        <b id=ndbavg>—</b><span class=sl>avg per node</span>
        <b id=ndbmax>—</b><span class=sl>max</span>
        <span class=mut id=ndbgrow>—</span><span class=sl>growth</span>
        <span class=cap id=ndbnode></span>
      </div>
    </div>
    <div class=col><h4>delivery</h4>
      <div class=sub>retention</div>
      <div class=kv>
        <b id=lheld>—</b><span class=sl>held (#)</span>
        <b id=lhits>—</b><span class=sl>hits (#)</span>
        <span class=mut id=lexp>—</span><span class=sl>expired (#)</span>
      </div>
      <div class=sub>re-delivered</div>
      <div class=kv>
        <b id=lredel>—</b><span class=sl id=lredelnote></span>
      </div>
      <div class=sub>batch waits (#, by reason)</div>
      <div class=kv><span class=cap id=lwaits>—</span></div>
    </div>
    <div class=col><h4>execution &middot; step 0 (#4169)</h4>
      <div class=kv>
        <b id=x0a>—</b><span class=sl>serial share</span>
        <b id=x0b>—</b><span class=sl>flushes per block</span>
        <b id=x0c>—</b><span class=sl>anchor co-arrival</span>
        <span class=cap id=x0note></span>
      </div>
    </div>
  </div>
</div>
<div class=two>
  <div class=panel>
    <h2>Synthetic flow &nbsp;·&nbsp; src&nbsp;▸&nbsp;dst</h2>
    <div style="overflow-x:auto"><table class=mx id=mxSyn></table></div>
    <div class=legend>cell = received / <b>delivered</b>, sent + status below. In-flight lag is judged against the expected proof-path latency (8s + 7 block intervals): <b>caught up</b> under 2&times;, <b style="color:var(--yel)">warning</b> at 2&times;, <b style="color:var(--red)">red</b> at 4&times;. Undelivered messages draining behind holes show as <b style="color:var(--yel)">healing</b>; <b style="color:var(--red)">red</b> undelivered means delivery has actually stalled.</div>
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
      <div class=pill><div class=n id=wsyn>—</div><div class=l>queue-full (block bound)</div></div>
      <div class=pill><div class=n id=wanc>—</div><div class=l>deadline (retries exhausted)</div></div>
      <div class=pill><div class=n id=wtot>0</div><div class=l>total</div></div>
    </div>
    <svg class=spark id=spWedge viewBox="0 0 300 44" preserveAspectRatio=none></svg>
    <table id=wdest><thead><tr><th>dropped toward (destination)</th><th>drops by senders (#)</th></tr></thead><tbody></tbody></table>
  </div>
  <div class=panel>
    <h2>Healing — span requests and what came back</h2>
    <div class=pills>
      <div class=pill><div class="n grn" id=hent>—</div><div class=l>entries healed</div></div>
      <div class=pill><div class="n grn" id=hans>—</div><div class=l>requests answered</div></div>
      <div class=pill><div class="n yel" id=hnot>—</div><div class=l>not-yet (in flight at source)</div></div>
      <div class=pill><div class="n red" id=hmiss>—</div><div class=l>miss (source cache lacks it)</div></div>
      <div class=pill><div class="n red" id=hfail>—</div><div class=l>failed</div></div>
      <div class=pill><div class="n" id=hheld>—</div><div class=l id=hheldl>held in staging</div></div>
    </div>
    <svg class=spark id=spHeal viewBox="0 0 300 44" preserveAspectRatio=none></svg>
    <table id=hpart><thead><tr><th>stream (source → destination)</th><th>answered (#)</th><th>asked while in flight (#)</th><th>miss (#)</th><th>failed (#)</th></tr></thead><tbody></tbody></table>
    <table id=hheldt><thead><tr><th>held in staging</th><th>entries (#)</th><th>bytes (B)</th></tr></thead><tbody></tbody></table>
  </div>
</div>
<div class=panel>
  <h2>Transaction type mix (live)</h2>
  <table id=mix><thead><tr><th>type</th><th>generated (#)</th><th>rejected (#)</th><th>skipped (#)</th></tr></thead><tbody></tbody></table>
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
      // State and note are computed server-side (soakmon judges lag against
      // the expected proof-path latency and separates healing-in-progress
      // from a stalled executor).
      let bg='rgba(63,185,80,.10)';
      if(c.state==='red')bg='rgba(248,81,73,.40)';
      else if(c.state==='warn')bg='rgba(210,153,34,.28)';
      const note=c.note||'caught up';
      h+=`<td class=cell style="background:${bg}"><div class=rd>${fmt(recv)}/<b>${fmt(deliv)}</b></div><div class=st>sent ${fmt(sent)} · ${note}</div></td>`;
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
  const parts=mx.parts||[];  // the server always sends the real topology
  matrix($('mxSyn'),(mx.flows||{}).synthetic,mx.parts&&parts);
  matrix($('mxAnc'),(mx.flows||{}).anchor,mx.parts&&parts);
  const hh=mx.heights||{};
  // Height alone cannot distinguish a live partition from a frozen one, so
  // each height carries how long it has sat unchanged.
  const pg=s.progress||{};
  $('heights').innerHTML=parts.map(p=>{
    const g=pg[p]||{},st=g.state||'unknown';
    const col=st==='live'?'':(st==='stalled'?'var(--red)':'var(--yel)');
    // Time per block, two ways. The rolling 5-minute window says what the
    // partition is doing NOW; the running average, over the whole run, says
    // what it has sustained. A partition that paces at target for four
    // minutes in every five reads healthy on the window alone.
    const now5=(st==='live'&&g.secPerBlock)?`${g.secPerBlock}s/blk`:'';
    const avg=(st==='live'&&g.avgSecPerBlock)?` avg ${g.avgSecPerBlock}s/blk`:'';
    const spb=now5?` ${now5}${avg}`:'';
    const note=st==='live'?spb:(st==='unknown'?' unreadable':` stalled ${Math.round(g.stalledFor||0)}s`);
    return `<span class=n ${col?`style="color:${col}"`:''}>${fmt(hh[p])}</span>`+
           `<span class=l ${col?`style="color:${col}"`:''}>${shortP(p)}${note}</span>`;
  }).join('')||'<span class=mut>—</span>';
  // header
  const ph=lg.phase||'—';$('phase').textContent=ph;
  // The badge reports progress, not reachability. "up" requires every
  // partition to have advanced within the stall window; anything else is
  // named for what it is, so a frozen network cannot show green.
  const st=s.status||(nw.api==='up'?'up':'down');
  const STC={up:['var(--grn)','network up'],stalled:['var(--red)','network STALLED'],
             degraded:['var(--yel)','network degraded'],down:['var(--red)','network down']};
  let [col,lbl]=STC[st]||['var(--red)','network ?'];
  if(nw.nodes!=null&&nw.ofNodes&&nw.nodes<nw.ofNodes)lbl+=` (${nw.nodes}/${nw.ofNodes} answering)`;
  $('net').className='badge '+(st==='up'?'up':'down');
  $('net').innerHTML=`<span class=dot style="background:${col}"></span>${lbl}`;
  const tgt=lg.target||0,gen=lg.generated||0;
  $('sub').textContent=`elapsed ${dur(lg.elapsedSec||0)} · ${(lg.rate||0).toFixed(2)} tx/s`+(lg.stale?' · loadgen stats stale':'');
  // cards
  const pct=tgt?(100*gen/tgt):0;
  const ns=s.nodeStats||{};
  // Heals split by mechanism, not just by kind: range pulls are the path #4087
  // added, and lumping them into one total hides whether it is doing anything.
  // Absent is not zero (REPORTING-SPEC 1): a family no node reported is
  // null on the wire and "— not measured" here.
  // A card's value slot is a number. An absent instrument shows a muted
  // dash there and says "not measured" in the subtitle, where the words fit
  // (REPORTING-SPEC 1 asks for absence to be distinguishable, not shouted).
  const nm=v=>(v==null?'<span class=mut style="font-weight:400">—</span>':fmt(v));
  const hr=h.requests,hp=h.proofs,hld=h.held,hf=s.handoff||{};
  $('cards').innerHTML=[
    card('DN height',fmt(nw.dnHeight),`${fmt(gen)} tx · ${pct.toFixed(0)}% of plan`),
    card('Healed entries',`<span class=grn>${nm(h.entries)}</span>`,h.entries==null?'not measured':'received in answer to span requests'),
    card('Heal requests',nm(hr&&hr.answered),hr?`answered · ${h.notYetPerMin!=null?h.notYetPerMin.toFixed(0):'—'}/min asked while in flight`:'not measured'),
    card('Proofs',nm(hp&&hp.validated),hp?`${fmt(hp.staged)} staged · ${fmt(hp.disproved)} disproved`:'not measured'),
    card('Wedges',`<span class="${(w.total||0)?'yel':''}">${nm(w.total)}</span>`,w.measured?Object.entries(w.byReason||{}).map(([k,v])=>`${fmt(v)} ${k}`).join(' · ')||'none':'not measured — no node exports a drop counter'),
    card('Heal misses',`<span class="${(h.errors||0)?'red':''}">${nm(h.errors)}</span>`,hr?`${fmt(hr.miss)} miss · ${fmt(hr.failed)} failed`:'not measured'),
    card('Not executed',`<span class="${(hf.unexecuted||0)?'red':''}">${nm(hf.measured?hf.unexecuted:null)}</span>`,
      hf.measured?`${fmt(hf.arrived)} arrived · ${fmt(hf.executed)} executed`:'not measured — needs a node built after #4279'),
    card('Rejected',`<span class="${(lg.rejected||0)?'red':''}">${fmt(lg.rejected||0)}</span>`,`${fmt(lg.skipped||0)} skipped`),
    card('Nodes',fmt(ns.count||0),`${fmt(ns.rssAvgMiB||0)} MiB avg · ${fmt(ns.rssMaxMiB||0)} max · heap ${fmt((ns.mem||{}).heapMaxMiB||0)} · GC ${(ns.mem||{}).gcPerSecMax!=null?(ns.mem||{}).gcPerSecMax.toFixed(1)+'/s':'—'} ${(ns.mem||{}).gcCoresSum!=null?'· '+(ns.mem||{}).gcCoresSum.toFixed(1)+' GC cores':''} · staged ${fmt((ns.mem||{}).stagedMax||0)}`),
  ].join('');
  const mib=v=>v?fmt(v)+' MiB':'—';
  $('nrssavg').textContent=mib(ns.rssAvgMiB); $('nrssmax').textContent=mib(ns.rssMaxMiB);
  $('nrssmin').textContent=mib(ns.rssMinMiB);
  $('nrssnode').textContent=ns.rssMaxNode?('max '+ns.rssMaxNode):'';
  // Database size on disk: both engines per node, summed. Growth is MiB/hour
  // since the first sample, so a run's disk appetite is a number, not a
  // guess from the docker volume at teardown.
  const db=ns.disk||{};
  const gb=v=>v?(v/1024).toFixed(2)+' GB':'—';
  $('ndbavg').textContent=gb(db.avgMiB); $('ndbmax').textContent=gb(db.maxMiB);
  $('ndbgrow').textContent=(db.growthMiBPerHour==null)?'—':((db.growthMiBPerHour/1024).toFixed(2)+' GB/h');
  $('ndbnode').textContent=db.maxNode?('max '+db.maxNode):'';
  $('ngravg').textContent=ns.grAvg!=null?fmt(ns.grAvg):'—';
  $('ngrmax').textContent=ns.grMax!=null?fmt(ns.grMax):'—';
  $('ngrmin').textContent=ns.grMin!=null?fmt(ns.grMin):'—';
  $('ngrnode').textContent=ns.grMaxNode?('max '+ns.grMaxNode):'';
  // wedges / heals pills
  $('wsyn').innerHTML=w.measured?fmt((w.byReason||{})['queue-full']||0):nm(null);$('wanc').innerHTML=w.measured?fmt((w.byReason||{}).deadline||0):nm(null);$('wtot').innerHTML=nm(w.total);
  $('hent').innerHTML=nm(h.entries);
  $('hans').innerHTML=nm(hr&&hr.answered);$('hnot').innerHTML=nm(hr&&hr['not-yet']);
  $('hmiss').innerHTML=nm(hr&&hr.miss);$('hfail').innerHTML=nm(hr&&hr.failed);
  $('hheld').innerHTML=nm(hld&&hld.entries);$('hheldl').textContent=hld?('held in staging · '+fmt(hld.bytes)+' B'):'held in staging';
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
  $('rratio').textContent=(ru!=null&&rs!=null&&ra!=null&&(ru+rs+ra)>0.01)?(100*(rs+ra)/(ru+rs+ra)).toFixed(0)+'%':'—';
  // Total now: what the network is actually processing, user plus the
  // synthetics and anchors the user load produces.
  const rt=(ru!=null&&rs!=null&&ra!=null)?ru+rs+ra:null;
  $('rtotal').textContent=(rt!=null)?rt.toFixed(2):'—';
  // And over the whole run, which is the figure a result is quoted as.
  const ra_=s.rates||{};
  const num=v=>(v==null?'—':v.toFixed(2));
  $('ratotal').textContent=num(ra_.totalAvg);
  $('rauser').textContent=num(ra_.userAvg);
  $('rasyn').textContent=num(ra_.synAvg);
  $('raanc').textContent=num(ra_.anchorAvg);
  $('rashare').textContent=(ra_.totalAvg>0.01&&ra_.synAvg!=null&&ra_.anchorAvg!=null)?(100*(ra_.synAvg+ra_.anchorAvg)/ra_.totalAvg).toFixed(0)+'%':'—';
  $('raover').textContent=ra_.userOverSec?`over ${dur(ra_.userOverSec)} · produced counted for ${dur(ra_.producedOverSec||0)}`:'';
  spark($('spWedge'),deltas(hist,'wedges'),getComputedStyle(document.documentElement).getPropertyValue('--yel').trim());
  spark($('spHeal'),deltas(hist,'heals'),getComputedStyle(document.documentElement).getPropertyValue('--grn').trim());
  // wedge by dest
  const wd=Object.entries(w.byDest||{}).sort((a,b)=>b[1]-a[1]);
  $('wdest').querySelector('tbody').innerHTML=!w.measured?'<tr><td class=mut colspan=2>— not measured</td></tr>':
    wd.map(([k,v])=>`<tr><td class=name>${k}</td><td>${fmt(v)}</td></tr>`).join('')||'<tr><td class=mut colspan=2>none yet</td></tr>';
  // heal by partition
  const hs=Object.entries(h.byStream||{}).sort((a,b)=>(b[1].answered+b[1]['not-yet'])-(a[1].answered+a[1]['not-yet']));
  $('hpart').querySelector('tbody').innerHTML=h.byStream==null?'<tr><td class=mut colspan=5>— not measured</td></tr>':
    hs.map(([k,v])=>`<tr><td class=name>${k}</td><td class=grn>${fmt(v.answered)}</td><td class=yel>${fmt(v['not-yet'])}</td><td class=red>${fmt(v.miss)}</td><td class=red>${fmt(v.failed)}</td></tr>`).join('')||'<tr><td class=mut colspan=5>no requests yet</td></tr>';
  $('hheldt').querySelector('tbody').innerHTML=hld==null?'<tr><td class=mut colspan=3>— not measured</td></tr>':
    (hld.streams||[]).filter(x=>x.entries||x.bytes).map(x=>`<tr><td class=name>${x.source} → ${x.ledger}</td><td>${fmt(x.entries)}</td><td class=mut>${fmt(x.bytes)}</td></tr>`).join('')||'<tr><td class=mut colspan=3>nothing held</td></tr>';
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
  // Batch lifecycle. `empty` next to `produced` is what separates an idle
  // network from a wedged one — both look like a frozen ledger index.
  const lf=s.life||{};
  $('lblocks').textContent=(lf.blocks??0).toLocaleString();
  $('lempty').textContent=(lf.blocksEmpty??0).toLocaleString();
  const bp=lf.blocks||0, be=lf.blocksEmpty||0;
  $('lidle').textContent = bp? (be/bp>0.98? 'IDLE — committing empty rounds' : ''):'';
  $('lidle').style.color='var(--yel)';
  $('lheld').textContent=(lf.retained??0).toLocaleString();
  $('lhits').textContent=(lf.retentionHits??0).toLocaleString();
  $('lexp').textContent=(lf.retentionExpired??0).toLocaleString();
  const rd=lf.redelivered||0;
  $('lredel').textContent=rd.toLocaleString()+' entries';
  $('lredel').style.color = rd>0? 'var(--red)':'';
  // Zero is the only healthy value: a re-delivery is skipped safely, but it
  // means commit dedup is still wrong upstream (#4125).
  $('lredelnote').textContent = rd>0? 'commit dedup is still wrong upstream (#4125)':'';
  // #4169 step 0. Gates, not health: serial share <25% means sharded
  // execution has nothing to win; co-arrival <5% means the anchors-first
  // staging round is not paying for itself. Absent metrics show as —, not 0.
  const ex=s.exec||{};
  const tot=(ex.serialSec||0)+(ex.parallelSec||0);
  const share= tot>0? ex.serialSec/tot : null;
  $('x0a').textContent = share==null? '—' : (share*100).toFixed(1)+'%';
  $('x0b').textContent = ex.blocks? (ex.flushes/ex.blocks).toFixed(2) : '—';
  const judged=(ex.anchorThisBlock||0)+(ex.anchorEarlier||0)+(ex.anchorMissing||0);
  $('x0c').textContent = judged? ((ex.anchorThisBlock||0)/judged*100).toFixed(1)+'%' : '—';
  const notes=[];
  if(share!=null && share<0.25) notes.push('serial <25%: group 4 has nothing to win');
  if(judged && ex.anchorThisBlock/judged<0.05) notes.push('co-arrival <5%: two-round staging not worth it');
  $('x0note').textContent=notes.join(' · ');
  const wr=lf.waitsByReason||{};
  const wk=Object.keys(wr);
  $('lwaits').innerHTML = wk.length? wk.sort().map(k=>`${k}=<b>${wr[k]}</b>`).join(' · ')
    : '<span class=mut>none</span>';

  $('foot').textContent=`updated ${age}s ago · flow+wedges+heals refresh ~1s · monitor read-only`;
}
tick();setInterval(tick,1000);
</script></body></html>"""


def main():
    started = time.time()
    log("soakmon starting pid=%d port=%d runDir=%s python=%s" % (
        os.getpid(), PORT, os.environ.get("RUN_DIR", "(unset)"),
        sys.version.split()[0]))
    threading.Thread(target=collector, daemon=True).start()
    threading.Thread(target=heartbeat, args=(started,), daemon=True).start()
    try:
        srv = ThreadingHTTPServer(("127.0.0.1", PORT), Handler)
    except OSError as e:
        # Almost always "address already in use" — a previous soakmon still
        # holding the port. Say so: this exits before serving anything, and
        # a silent failure here looks identical to a mid-run death.
        log("cannot bind port %d: %s" % (PORT, e))
        raise
    log("serving on http://127.0.0.1:%d" % PORT)
    try:
        srv.serve_forever()
    except KeyboardInterrupt:
        pass
    except Exception as e:
        log("serve_forever failed: %s: %s" % (type(e).__name__, str(e)[:200]))
        raise
    log("serve_forever returned")


if __name__ == "__main__":
    main()
