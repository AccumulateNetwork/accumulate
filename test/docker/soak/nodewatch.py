#!/usr/bin/env python3
# Copyright 2026 The Accumulate Authors
#
# Use of this source code is governed by an MIT-style
# license that can be found in the LICENSE file or at
# https://opensource.org/licenses/MIT.
"""nodewatch.py -- what every node of a running network is actually doing.

This answers one question per node: is it executing blocks, is it joining,
or is it stuck -- and how far from its peers has it drifted. It is not a
soak monitor: it needs no run directory, no load generator and no manifest,
and it may be pointed at a network nobody is testing.

WHY IT DOES NOT ASK THE API FOR A HEIGHT
    `query acc://bvn-BVN1.acme/ledger` is ROUTED. Asked of a node that is
    wedged at block 76, the router hands the question to a healthy peer and
    the answer comes back 700 -- the node's own state never appears. On
    2026-09-18, with acc-bvn1-val1 stopped at block 81 and never rejoined,
    port 26680 answered `index: 700` for its own partition. A monitor built
    on that reads twelve healthy nodes off a network with a dead one.

    So height here is the node's OWN executed height, read with
    `consensus-status` addressed to that node's OWN peer ID (from
    `node-info` on the same port). That call is answered locally and cannot
    be satisfied by a peer. It is the only height on this display.

WHAT MAKES A NODE "WEDGED" RATHER THAN "BEHIND"
    A node that is catching up and a node that will never catch up look
    identical in a single sample: both are behind. Three things separate
    them, and all three are shown:
      - accumulate_node_state, the node's own join state machine
        (0 booting, 1 waiting, 2 active, 3 complete). NOTE: this gauge is
        only registered while a node is in that state machine. An executing
        node does not export it at all, so ABSENT means "not joining" and
        is displayed as such, never as unknown.
      - stalledFor, which the node itself prints on every join round
        ("Joining: collecting committed blocks, executing none ...
        stalledFor=<ns>"). This is the node's own measure and needs no
        sampling window, so a wedge is visible in the first sample.
      - whether its own height moved between our samples.

EVERY NUMBER CARRIES ITS QUANTITY, ITS UNIT AND ITS WINDOW. "Own Executed
Height (block)", "Blocks Executed (last 60 s)". No column is named after
the method used to compute it.

Usage:
    nodewatch.py                      # one sample, pasteable text, to stdout
    nodewatch.py --watch              # re-sample forever, text to stdout
    nodewatch.py --serve              # live view on :8099 + /status.txt
    nodewatch.py --serve --port 8099 --interval 3
"""

import argparse
import json
import os
import re
import subprocess
import sys
import textwrap
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
sys.path.insert(0, os.path.join(HERE, ".."))
import topology  # noqa: E402
from promparse import parse_prom  # noqa: E402

METRICS_PORT = 26670  # in-container; answers every path, so always ask /metrics

# A node whose own height has not moved for this long, while its partition
# has moved, is not slow -- it is stuck. Blocks are produced roughly every
# second on this network even with no load (the heartbeat, #4277), so 45 s
# is ~45 missed blocks and cannot be a quiet moment.
WEDGE_SECS = 45.0

# Blocks behind the highest node of its own partition before a node is
# called out. One or two blocks is the ordinary spread between peers that
# commit at slightly different instants.
BEHIND_BLOCKS = 5

# How far back to read each container's log for join state. The join loop
# prints several lines a second; 200 lines covers well over the 20 s window
# and keeps `docker logs` cheap on a box that is running the network.
LOG_TAIL = 200
LOG_SINCE = "30s"

ANSI = re.compile(r"\x1b\[[0-9;]*m")
_KV = re.compile(r'(\w+)=("[^"]*"|\S+)')
_MSG = re.compile(r'"message":"([^"]*)"')

STATE_WORDS = {0: "booting", 1: "waiting", 2: "active", 3: "complete"}

# Display states, worst first. The order is the priority: a node gets the
# first of these its readings justify.
ST_DOWN = "DOWN"
ST_NOANSWER = "NO-ANSWER"
ST_WEDGED = "WEDGED"
ST_STALLED = "STALLED"
ST_JOINING = "JOINING"
ST_BEHIND = "BEHIND"
ST_EXEC = "EXEC"
BAD_STATES = (ST_DOWN, ST_NOANSWER, ST_WEDGED, ST_STALLED)


# ---------------------------------------------------------------- shell ----

_DOCKER_PREFIX = None


def docker_prefix():
    """How to invoke docker here: directly, or through `sg docker -c`.

    Detected once, at startup, by running `docker ps`. Hardcoding either
    form makes the tool report a whole network as DOWN on the box where the
    other form is the working one.
    """
    global _DOCKER_PREFIX
    if _DOCKER_PREFIX is None:
        try:
            r = subprocess.run(["docker", "ps", "-q"], capture_output=True,
                               text=True, timeout=20)
            _DOCKER_PREFIX = [] if r.returncode == 0 else ["sg", "docker", "-c"]
        except Exception:
            _DOCKER_PREFIX = ["sg", "docker", "-c"]
    return _DOCKER_PREFIX


def dsh(cmd, timeout=20):
    """Run a docker command line (a string) and return stdout, or ""."""
    pre = docker_prefix()
    argv = [cmd] if not pre else pre + [cmd]
    try:
        r = subprocess.run(argv if pre else ["sh", "-c", cmd],
                           capture_output=True, text=True, timeout=timeout)
        return r.stdout
    except Exception:
        return ""


def curl_json(port, method, params, timeout=6):
    body = json.dumps({"jsonrpc": "2.0", "id": 1, "method": method,
                       "params": params})
    try:
        r = subprocess.run(
            ["curl", "-s", "-m", str(timeout), "-X", "POST",
             "http://localhost:%d/v3" % port,
             "-H", "content-type: application/json", "-d", body],
            capture_output=True, text=True, timeout=timeout + 4)
        return json.loads(r.stdout)
    except Exception:
        return None


# ------------------------------------------------------------- parsing ----

def canon_part(name):
    """Canonical partition id. The metrics label is lower case
    ("directory", "bvn1"); the API and the logs use "Directory", "BVN1".
    Three spellings of one partition silently split a display into three
    columns, two of which are always empty."""
    if not name:
        return name
    n = name.strip().strip('"')
    if "//" in n:                       # acc://bvn-BVN1.acme -> BVN1
        n = n.split("//")[1].split(".")[0]
        if n.startswith("bvn-"):
            n = n[4:]
        if n == "dn":
            return "Directory"
    low = n.lower()
    if low in ("dn", "directory"):
        return "Directory"
    if low.startswith("bvn"):
        return "BVN" + low[3:].lstrip("-")
    return n


def kv(line):
    """key=value pairs out of one structured log line."""
    return {k: v.strip('"') for k, v in _KV.findall(line)}


def parse_join_logs(text):
    """Join state per partition, from one container's log tail.

    Returns {partition: {"buffered": int, "lastBlock": int,
                         "stalledForSecs": float, "at": str}} plus, under
    the key "_error", the most recent join error message seen -- the thing
    that says WHY it is not joining, which is the first question anyone
    asks once the display says WEDGED.
    """
    out = {}
    for raw in text.splitlines():
        line = ANSI.sub("", raw)
        if "Joining: collecting committed blocks" in line:
            f = kv(line)
            part = canon_part(f.get("partition", ""))
            if not part:
                continue
            rec = {"at": line.split(" ", 1)[0]}
            for src, dst, cast in (("buffered", "buffered", int),
                                   ("lastBlock", "lastBlock", int),
                                   ("round", "round", int)):
                try:
                    rec[dst] = cast(f[src])
                except (KeyError, ValueError):
                    rec[dst] = None
            try:                        # stalledFor is nanoseconds
                rec["stalledForSecs"] = round(int(f["stalledFor"]) / 1e9, 1)
            except (KeyError, ValueError):
                rec["stalledForSecs"] = None
            out[part] = rec
        elif "module=join" in line and "error=" in line:
            m = _MSG.search(line)
            if m:
                out["_error"] = m.group(1)
    return out


def metrics_of(rows):
    """The readings this display needs, out of a parsed scrape.

    Absent is kept as absent (None / {}), never coerced to zero: a scrape
    that failed and a node reporting zero are different facts, and a
    monitor that renders them the same has lost the only signal that says
    "I could not see".
    """
    m = {"nodeState": {}, "execLag": {}, "execBlocksTotal": None,
         "blocksProduced": None, "refusing": {}, "goroutines": None}
    for name, lbl, val in rows:
        if name == "accumulate_node_state":
            m["nodeState"][canon_part(lbl.get("partition"))] = int(val)
        elif name == "accumulate_dagbft_execution_lag_blocks":
            m["execLag"][canon_part(lbl.get("partition"))] = int(val)
        elif name == "accumulate_exec_blocks_total":
            m["execBlocksTotal"] = int(val)
        elif name == "accumulate_dagbft_blocks_produced_total":
            m["blocksProduced"] = int(val)
        elif name == "accumulate_dagbft_batch_store_refusing" and val:
            key = (canon_part(lbl.get("partition")), lbl.get("reason", "?"))
            m["refusing"][key] = int(val)
        elif name == "go_goroutines":
            m["goroutines"] = int(val)
    return m


# ------------------------------------------------------------ sampling ----

class Watch:
    """Holds the roster, the peer-ID cache and the height history.

    History exists for exactly two derived quantities, both of which name
    their window on the display: blocks executed in the last sample window,
    and blocks executed since this watch started.
    """

    def __init__(self, wedge_secs=WEDGE_SECS, behind_blocks=BEHIND_BLOCKS):
        self.containers = topology.containers()
        self.ports = topology.node_ports()
        self.bvn_of = {}
        counts = topology.nodes_per_bvn()
        i = 0
        for n, b in enumerate(topology.bvns(), start=1):
            for _ in range(counts[b]):
                self.bvn_of[self.containers[i]] = b
                i += 1
        self.port_of = dict(zip(self.containers, self.ports))
        self.peerid = {}
        self.hist = {}      # (container, partition) -> [(t, height), ...]
        self.first = {}     # (container, partition) -> (t, height)
        self.resets = {}    # container -> count of backwards height moves
        self.started = time.time()
        self.wedge_secs = wedge_secs
        self.behind_blocks = behind_blocks
        self.errors = []

    # -- collection ------------------------------------------------------
    def peer_of(self, c):
        if c in self.peerid:
            return self.peerid[c]
        r = curl_json(self.port_of[c], "node-info", {})
        try:
            self.peerid[c] = r["result"]["peerID"]
        except Exception:
            return None
        return self.peerid[c]

    def own_height(self, c, part):
        """This node's own executed height for one partition, or None.

        Addressed to the node's own peer ID so the router cannot answer it
        from a healthy neighbour -- see the module docstring.
        """
        pid = self.peer_of(c)
        if not pid:
            return None
        r = curl_json(self.port_of[c], "consensus-status",
                      {"nodeID": pid, "partition": part})
        try:
            return int(r["result"]["lastBlock"]["height"])
        except Exception:
            return None

    def _one(self, c, running, out, lock):
        parts = ["Directory", self.bvn_of[c]]
        rec = {"container": c, "short": c.replace("acc-", ""),
               "port": self.port_of[c], "bvn": self.bvn_of[c],
               "parts": parts, "running": running.get(c) is not None,
               "containerStatus": running.get(c, "absent"),
               "heights": {}, "join": {}, "metrics": None}
        if rec["running"]:
            for p in parts:
                rec["heights"][p] = self.own_height(c, p)
            txt = dsh("docker exec %s wget -qO- -T 5 http://localhost:%d/metrics"
                      % (c, METRICS_PORT))
            rec["metrics"] = metrics_of(parse_prom(txt)) if txt.strip() else None
            logs = dsh("docker logs --since %s --tail %d %s 2>&1"
                       % (LOG_SINCE, LOG_TAIL, c))
            rec["join"] = parse_join_logs(logs)
        with lock:
            out[c] = rec

    def sample(self):
        now = time.time()
        running = {}
        for line in dsh("docker ps --filter name=acc- --format '{{.Names}}\t{{.Status}}'").splitlines():
            if "\t" in line:
                n, s = line.split("\t", 1)
                running[n] = s
        out, lock, threads = {}, threading.Lock(), []
        for c in self.containers:
            t = threading.Thread(target=self._one, args=(c, running, out, lock))
            t.start()
            threads.append(t)
        for t in threads:
            t.join()
        nodes = [out[c] for c in self.containers if c in out]
        self._record(nodes, now)
        return build_snapshot(self, nodes, now)

    def _record(self, nodes, now):
        for n in nodes:
            for p, h in n["heights"].items():
                if h is None:
                    continue
                key = (n["container"], p)
                hist = self.hist.setdefault(key, [])
                if hist and h < hist[-1][1]:
                    # Height went backwards: the node restarted (or was
                    # replaced). Rebase rather than emit a negative rate --
                    # a negative "blocks executed" is a misreport, and the
                    # restart is itself worth saying out loud.
                    hist.clear()
                    self.first.pop(key, None)
                    self.resets[n["container"]] = self.resets.get(n["container"], 0) + 1
                hist.append((now, h))
                del hist[:-600]
                self.first.setdefault(key, (now, h))

    # -- derived ---------------------------------------------------------
    def advanced(self, c, p, now, window):
        """(blocks, seconds) this node's own height moved over `window`
        seconds, or (None, 0) when there is no second sample yet."""
        hist = self.hist.get((c, p)) or []
        if len(hist) < 2:
            return None, 0.0
        cutoff = now - window
        base = hist[0]
        for t, h in hist:
            if t <= cutoff:
                base = (t, h)
            else:
                break
        return hist[-1][1] - base[1], max(0.0, hist[-1][0] - base[0])

    def since_start(self, c, p):
        hist = self.hist.get((c, p)) or []
        f = self.first.get((c, p))
        if not hist or not f:
            return None, 0.0
        return hist[-1][1] - f[1], max(0.0, hist[-1][0] - f[0])

    def idle_secs(self, c, p, now):
        """Seconds since this node's own height last CHANGED, and whether
        that is a true measurement or a floor.

        Before a change has been seen, the honest answer is "at least as
        long as we have been watching" -- returned with observed=False so
        the display can mark it, rather than printing a number that reads
        like a measured stall.
        """
        hist = self.hist.get((c, p)) or []
        if not hist:
            return None, False
        last = hist[-1][1]
        first_at_last = hist[-1][0]
        for t, h in reversed(hist):
            if h != last:
                # Timed from when the CURRENT height was first seen, not
                # from the previous sample: the change happened somewhere in
                # between, and dating it to the earlier bound would add a
                # whole sample interval of imaginary idleness to every node
                # on every cycle -- which is how a monitor invents a stall.
                return round(now - first_at_last, 1), True
            first_at_last = t
        return round(now - hist[0][0], 1), False


def build_snapshot(w, nodes, now):
    """Everything the text and the web view render, and nothing derived
    later: one structure, one set of definitions, two presentations."""
    window = min(60.0, max(1.0, now - w.started))
    parts = topology.partitions()

    # Partition lead: the highest OWN height any node reports. A lagging
    # node can only under-report, so the max is what the partition holds.
    lead, lead_node = {}, {}
    for p in parts:
        best, who = None, None
        for n in nodes:
            h = n["heights"].get(p)
            if h is not None and (best is None or h > best):
                best, who = h, n["short"]
        lead[p], lead_node[p] = best, who

    rows = []
    for n in nodes:
        m = n["metrics"] or {}
        per = {}
        for p in n["parts"]:
            h = n["heights"].get(p)
            adv, adv_secs = w.advanced(n["container"], p, now, window)
            tot, tot_secs = w.since_start(n["container"], p)
            idle, idle_seen = w.idle_secs(n["container"], p, now)
            per[p] = {
                "height": h,
                "behind": None if (h is None or lead[p] is None) else lead[p] - h,
                "blocksWindow": adv, "windowSecs": round(adv_secs, 1),
                "blocksSinceStart": tot, "sinceStartSecs": round(tot_secs, 1),
                "idleSecs": idle, "idleMeasured": idle_seen,
                "nodeState": (m.get("nodeState") or {}).get(p),
                "execLagBlocks": (m.get("execLag") or {}).get(p),
                "join": n["join"].get(p),
            }
        row = dict(n)
        row.pop("metrics", None)
        row["per"] = per
        row["joinError"] = n["join"].get("_error")
        row["goroutines"] = m.get("goroutines")
        row["metricsOK"] = n["metrics"] is not None
        row["restarts"] = w.resets.get(n["container"], 0)
        row["state"], row["reason"] = classify(row, lead, w, now)
        rows.append(row)

    part_rows = {}
    for p in parts:
        movers = [r["per"][p]["blocksWindow"] for r in rows
                  if p in r["per"] and r["per"][p]["blocksWindow"] is not None]
        secs = max([r["per"][p]["windowSecs"] for r in rows
                    if p in r["per"]] or [0.0])
        part_rows[p] = {
            "leadHeight": lead[p], "leadNode": lead_node[p],
            "spreadBlocks": None if lead[p] is None else
            lead[p] - min([r["per"][p]["height"] for r in rows
                           if p in r["per"] and r["per"][p]["height"] is not None]
                          or [lead[p]]),
            "leadBlocksWindow": max(movers) if movers else None,
            "windowSecs": round(secs, 1),
            "nodes": sum(1 for r in rows if p in r["per"]),
        }

    bad = [r for r in rows if r["state"] in BAD_STATES]
    return {
        "asOf": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(now)),
        "asOfUnix": int(now),
        "watchAgeSecs": round(now - w.started, 1),
        "windowSecs": round(window, 1),
        "network": topology_network_id(),
        "wedgeSecs": w.wedge_secs,
        "behindBlocks": w.behind_blocks,
        "partitions": part_rows,
        "nodes": rows,
        "fleet": {
            "validators": len(rows),
            "executing": sum(1 for r in rows if r["state"] == ST_EXEC),
            "notExecuting": len(rows) - sum(1 for r in rows if r["state"] == ST_EXEC),
            "bad": [r["short"] for r in bad],
        },
    }


def topology_network_id():
    try:
        with open(os.path.join(HERE, "..", "docker-network.yml")) as f:
            m = re.search(r'^id:\s*"?([^"\n]+)"?', f.read(), re.M)
            return m.group(1) if m else "?"
    except OSError:
        return "?"


def classify(row, lead, w, now):
    """One word for what this node is doing, and the reason for it.

    The rules are here, in one function, so the web view and the text can
    never disagree about what "WEDGED" means -- and so a test can hold them
    to it without a network.
    """
    if not row["running"]:
        return ST_DOWN, "container %s" % row["containerStatus"]
    if not row["metricsOK"] and all(v["height"] is None for v in row["per"].values()):
        return ST_NOANSWER, "no metrics and no consensus-status answer"

    states = {p: v["nodeState"] for p, v in row["per"].items()
              if v["nodeState"] is not None}
    joining_parts = [p for p, s in states.items() if s < 2]
    # A join line in the last 30 s of log does not mean the node is still
    # joining -- it may have gone active ten seconds ago. The gauge wins
    # where it exists; the log only speaks for partitions the gauge says
    # nothing about, or says are not yet active.
    logged_join = [p for p, v in row["per"].items()
                   if v.get("join") and (v["nodeState"] is None
                                         or v["nodeState"] < 2)]
    stalled_for = [v["join"]["stalledForSecs"] for v in row["per"].values()
                   if v.get("join") and v["join"].get("stalledForSecs") is not None]
    worst_stall = max(stalled_for) if stalled_for else None

    behind = [(v["behind"], p) for p, v in row["per"].items()
              if v["behind"] is not None]
    worst_behind, behind_part = max(behind) if behind else (None, None)

    moved = [v["blocksWindow"] for v in row["per"].values()
             if v["blocksWindow"] is not None]
    observed = min([v["idleSecs"] for v in row["per"].values()
                    if v["idleSecs"] is not None] or [0.0])

    if joining_parts or logged_join:
        parts = sorted(set(joining_parts) | set(logged_join))
        word = ", ".join("%s=%s" % (p, STATE_WORDS.get(states.get(p), "?"))
                         for p in parts)
        if worst_stall is not None and worst_stall >= w.wedge_secs:
            return ST_WEDGED, ("joining %s; the node reports %.0f s unable to "
                               "execute" % (word, worst_stall))
        if moved and max(moved) == 0 and observed >= w.wedge_secs:
            return ST_WEDGED, ("joining %s; own height unchanged for %.0f s"
                               % (word, observed))
        return ST_JOINING, "joining %s" % word

    if moved and max(moved) == 0 and observed >= w.wedge_secs:
        others = [lead[p] for p in row["per"] if lead.get(p) is not None]
        return ST_STALLED, ("own height unchanged for %.0f s while its "
                            "partition is at %s" % (observed, others))
    if worst_behind is not None and worst_behind >= w.behind_blocks:
        return ST_BEHIND, "%d blocks behind the highest node of %s" % (
            worst_behind, behind_part)
    return ST_EXEC, "executing"


# ------------------------------------------------------------ text view ----

TEXT_WIDTH = 76  # a chat window on a phone; past this, lines wrap into noise


def fmt(v, dash="-"):
    return dash if v is None else str(v)


def wrapped(text, indent="  ", width=TEXT_WIDTH):
    """Prose, folded to the display width. A 99-column explanation of why a
    node is wedged is unreadable in the place it is actually read."""
    return textwrap.wrap(text, width=width, subsequent_indent=indent) or [""]


def headline(s):
    """One line: how many nodes are executing, and what is wrong with the
    ones that are not."""
    f = s["fleet"]
    if not f["notExecuting"]:
        return "ALL %d validators executing." % f["validators"]
    bits = []
    for n in s["nodes"]:
        if n["state"] == ST_EXEC:
            continue
        worst = max([(v.get("behind") or 0, p) for p, v in n["per"].items()]
                    or [(0, "")])
        stall = [v["join"]["stalledForSecs"] for v in n["per"].values()
                 if v.get("join") and v["join"].get("stalledForSecs") is not None]
        bit = "%s %s" % (n["state"], n["short"])
        detail = []
        if worst[0]:
            detail.append("%s %d blocks behind" % (worst[1], worst[0]))
        if stall:
            detail.append("stuck %.0f s" % max(stall))
        if detail:
            bit += " (%s)" % ", ".join(detail)
        bits.append(bit)
    return "%d of %d executing; %s" % (f["executing"], f["validators"],
                                       "; ".join(bits))


def render_text(s, width_note=True):
    """The pasteable status. This is a first-class output, not a fallback:
    it is what goes into a chat window when the person who has to judge the
    network is on a phone."""
    L = []
    A = L.append
    A("ACCUMULATE NODE STATUS  %s" % s["asOf"])
    # A one-line verdict, second line, because a chat preview shows two
    # lines and the answer to "is the network all right" must be in them.
    A(headline(s))
    A("network %s | %d validators | watching for %s"
      % (s["network"], s["fleet"]["validators"], hms(s["watchAgeSecs"])))
    A("")
    A("Partition Executed Height, highest node (block):")
    line = []
    for p, d in s["partitions"].items():
        line.append("%s %s" % (p, fmt(d["leadHeight"])))
    A("  " + "   ".join(line))
    A("Partition Height Spread, highest minus lowest node (block):")
    A("  " + "   ".join("%s %s" % (p, fmt(d["spreadBlocks"]))
                        for p, d in s["partitions"].items()))
    A("")
    # A rate needs two samples. Until the second one lands there is no
    # window, and a column headed "EXEC/1s" full of dashes claims one that
    # does not exist. Say so instead.
    w = s["windowSecs"]
    measured = any(v.get("blocksWindow") is not None
                   for n in s["nodes"] for v in n["per"].values())
    col = "EXEC/%ds" % round(w) if measured else "EXEC/--"
    A("%-11s %-9s %6s %6s %6s %6s %8s %7s"
      % ("NODE", "STATE", "DIR", "BHND", "BVN", "BHND", col, "STUCK s"))
    for n in s["nodes"]:
        d = n["per"].get("Directory", {})
        b = n["per"].get(n["bvn"], {})
        ex = [v for v in (d.get("blocksWindow"), b.get("blocksWindow"))
              if v is not None]
        stall = [v["join"]["stalledForSecs"] for v in n["per"].values()
                 if v.get("join") and v["join"].get("stalledForSecs") is not None]
        if stall:
            stuck = "%.0f" % max(stall)
        elif n["state"] in BAD_STATES:
            idles = [v["idleSecs"] for v in n["per"].values()
                     if v.get("idleSecs") is not None]
            stuck = ("%.0f" % max(idles)) if idles else "-"
        else:
            stuck = "-"
        A("%-11s %-9s %6s %6s %6s %6s %8s %7s"
          % (n["short"], n["state"],
             fmt(d.get("height")), fmt(d.get("behind")),
             fmt(b.get("height")), fmt(b.get("behind")),
             fmt(sum(ex) if ex else None), stuck))
    A("")
    f = s["fleet"]
    A("FLEET (this sample): %d of %d executing, %d not."
      % (f["executing"], f["validators"], f["notExecuting"]))
    if not measured:
        A("EXEC/-- : no second sample yet, so no window exists to state a")
        A("  rate over. Heights, behind and STUCK are single-sample readings")
        A("  and are exact. Use --watch or --serve for the rate.")
    for n in s["nodes"]:
        if n["state"] == ST_EXEC:
            continue
        L.extend(wrapped("%s %s: %s" % (n["state"], n["short"], n["reason"])))
        for p, v in sorted(n["per"].items()):
            j = v.get("join")
            if j:
                L.extend(wrapped(
                    "  %s: buffered %s blocks, last executed block %s, "
                    "join round %s" % (p, fmt(j.get("buffered")),
                                       fmt(j.get("lastBlock")),
                                       fmt(j.get("round"))), indent="    "))
            if v.get("execLagBlocks"):
                L.extend(wrapped(
                    "  %s: execution lag %d blocks (node's own gauge)"
                    % (p, v["execLagBlocks"]), indent="    "))
        if n.get("joinError"):
            L.extend(wrapped("  last join error: %s" % n["joinError"],
                             indent="    "))
        if n.get("restarts"):
            L.extend(wrapped(
                "  height went backwards %d time(s) since this watch "
                "started (restart); rates rebased" % n["restarts"],
                indent="    "))
    if width_note:
        A("")
        A("LEGEND")
        A("  DIR / BVN  this node's OWN executed height (block), read with")
        A("             consensus-status addressed to its own peer ID. Never")
        A("             a routed query, which would answer from a healthy peer.")
        A("  BHND       blocks this node is behind the highest node of the")
        A("             column it sits under (block).")
        if measured:
            A("  EXEC/%-5s blocks this node executed in the last %d s"
              % ("%ds" % round(w), round(w)))
            A("             (Directory + its BVN added).")
        else:
            A("  EXEC/--    blocks executed per window (block); no window yet")
            A("             (a window needs two samples).")
        A("  STUCK s    seconds the node itself reports it has been unable to")
        A("             execute (dagbft stalledFor); for a non-joining node")
        A("             that has stopped, seconds since its height last moved.")
        A("             '-' means the node is executing, not unknown.")
        A("  STATE      EXEC executing | BEHIND >=%d blocks behind its"
          % s["behindBlocks"])
        A("             partition | JOINING in the join state machine and")
        A("             still moving | WEDGED joining or stopped with no")
        A("             progress for >=%.0f s | STALLED executing node whose"
          % s["wedgeSecs"])
        A("             height stopped | DOWN container not running |")
        A("             NO-ANSWER container up, node answers nothing.")
    return "\n".join(L)


def hms(secs):
    secs = int(secs)
    if secs < 90:
        return "%d s" % secs
    if secs < 5400:
        return "%d m %02d s" % (secs // 60, secs % 60)
    return "%d h %02d m" % (secs // 3600, (secs % 3600) // 60)


# ------------------------------------------------------------- web view ----

PAGE = """<!doctype html>
<html><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Accumulate node status</title>
<style>
 body{background:#12151a;color:#e8edf2;font:14px/1.45 -apple-system,
   BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;margin:0;padding:12px}
 h1{font-size:16px;margin:0 0 2px}
 .sub{color:#93a1b0;font-size:12px;margin-bottom:10px}
 .grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(150px,1fr));
   gap:8px}
 .card{background:#1b2028;border-radius:8px;padding:8px 10px;
   border-left:4px solid #3a4452}
 .EXEC{border-left-color:#3ecf8e}.BEHIND{border-left-color:#e0b341}
 .JOINING{border-left-color:#4aa3e0}.WEDGED{border-left-color:#e05252}
 .STALLED{border-left-color:#e05252}.DOWN{border-left-color:#e05252}
 .NOANSWER{border-left-color:#e05252}
 .nm{font-weight:600}.st{font-size:12px;letter-spacing:.04em}
 .st.EXEC{color:#3ecf8e}.st.BEHIND{color:#e0b341}.st.JOINING{color:#4aa3e0}
 .st.WEDGED,.st.STALLED,.st.DOWN,.st.NOANSWER{color:#ff6b6b}
 .q{color:#93a1b0;font-size:11px;margin-top:4px}
 .v{color:#e8edf2;font-variant-numeric:tabular-nums}
 .bad{background:#2a1a1c;border:1px solid #6b2b2b;border-radius:8px;
   padding:8px 10px;margin:10px 0;color:#ffb4b4;font-size:13px}
 pre{background:#0d1014;border-radius:8px;padding:10px;overflow:auto;
   font-size:11px;color:#cbd5e1}
 .bar{display:flex;gap:10px;flex-wrap:wrap;margin:8px 0 12px}
 .pill{background:#1b2028;border-radius:6px;padding:6px 9px;font-size:12px}
 .pill b{font-variant-numeric:tabular-nums}
</style></head><body>
<h1>Accumulate node status</h1>
<div class="sub" id="sub">loading</div>
<div class="bar" id="bar"></div>
<div id="alerts"></div>
<div class="grid" id="grid"></div>
<h1 style="margin-top:16px">Pasteable status</h1>
<div class="sub">the same sample as above, as text &mdash;
  also at <code>/status.txt</code></div>
<pre id="txt"></pre>
<script>
async function tick(){
 try{
  const s = await (await fetch('status.json')).json();
  const t = await (await fetch('status.txt')).text();
  document.getElementById('txt').textContent = t;
  document.getElementById('sub').textContent =
    s.asOf + ' \\u00b7 network ' + s.network + ' \\u00b7 ' +
    s.fleet.executing + ' of ' + s.fleet.validators + ' validators executing';
  document.getElementById('bar').innerHTML = Object.entries(s.partitions)
   .map(([p,d]) => '<div class="pill">' + p +
     ' &middot; Executed Height, highest node <b>' + (d.leadHeight ?? '-') +
     '</b> block &middot; Height Spread <b>' + (d.spreadBlocks ?? '-') +
     '</b> block</div>').join('');
  document.getElementById('alerts').innerHTML = s.nodes
   .filter(n => n.state !== 'EXEC')
   .map(n => '<div class="bad"><b>' + n.state + ' ' + n.short + '</b> &mdash; ' +
     n.reason + (n.joinError ? '<br>last join error: ' + n.joinError : '') +
     '</div>').join('');
  document.getElementById('grid').innerHTML = s.nodes.map(n => {
    const cls = n.state === 'NO-ANSWER' ? 'NOANSWER' : n.state;
    const d = n.per['Directory'] || {}, b = n.per[n.bvn] || {};
    const ex = [d.blocksWindow, b.blocksWindow]
      .filter(v => v !== null && v !== undefined).reduce((a,c)=>a+c, 0);
    return '<div class="card ' + cls + '"><div class="nm">' + n.short +
     '</div><div class="st ' + cls + '">' + n.state + '</div>' +
     '<div class="q">Own Executed Height (block)<br>Directory <span class="v">' +
     (d.height ?? '-') + '</span> &nbsp; ' + n.bvn + ' <span class="v">' +
     (b.height ?? '-') + '</span></div>' +
     '<div class="q">Behind Highest Node (block)<br>Directory <span class="v">' +
     (d.behind ?? '-') + '</span> &nbsp; ' + n.bvn + ' <span class="v">' +
     (b.behind ?? '-') + '</span></div>' +
     '<div class="q">Blocks Executed (last ' + Math.round(s.windowSecs) +
     ' s)<br><span class="v">' + ex + '</span> block</div></div>';
  }).join('');
 }catch(e){ document.getElementById('sub').textContent = 'collector unreachable: ' + e; }
}
tick(); setInterval(tick, 3000);
</script></body></html>
"""


class Server(BaseHTTPRequestHandler):
    snapshot = {"text": "collecting the first sample...", "json": {}}

    def log_message(self, *a):
        pass

    def _send(self, code, body, ctype):
        b = body.encode()
        self.send_response(code)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(b)))
        self.send_header("Cache-Control", "no-store")
        self.end_headers()
        self.wfile.write(b)

    def do_GET(self):
        path = self.path.split("?")[0].rstrip("/") or "/"
        if path in ("/", "/index.html"):
            self._send(200, PAGE, "text/html; charset=utf-8")
        elif path == "/status.json":
            self._send(200, json.dumps(Server.snapshot["json"]),
                       "application/json")
        elif path == "/status.txt":
            self._send(200, Server.snapshot["text"], "text/plain; charset=utf-8")
        else:
            self._send(404, "no such path: %s\n" % path, "text/plain")


def main():
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--serve", action="store_true",
                    help="serve the live view and /status.txt over HTTP")
    ap.add_argument("--port", type=int, default=int(os.environ.get("PORT", "8099")))
    ap.add_argument("--watch", action="store_true",
                    help="keep sampling and print the text each time")
    ap.add_argument("--interval", type=float, default=5.0,
                    help="seconds between samples (default 5)")
    ap.add_argument("--settle", type=float, default=10.0,
                    help="seconds between the two samples a one-shot takes, "
                         "so the printed rate has a real window (default 10; "
                         "0 prints a single sample and no rate)")
    ap.add_argument("--json", action="store_true", help="print JSON, not text")
    ap.add_argument("--no-legend", action="store_true")
    ap.add_argument("--wedge-secs", type=float, default=WEDGE_SECS)
    ap.add_argument("--behind-blocks", type=int, default=BEHIND_BLOCKS)
    a = ap.parse_args()

    bad = topology.check_ports_against_compose()
    if bad:
        sys.stderr.write("topology mismatch: %s\n" % bad)

    w = Watch(wedge_secs=a.wedge_secs, behind_blocks=a.behind_blocks)

    if a.serve:
        def loop():
            while True:
                try:
                    s = w.sample()
                    Server.snapshot = {"json": s,
                                       "text": render_text(s, not a.no_legend)}
                except Exception as e:      # never let one bad sample end the watch
                    sys.stderr.write("sample failed: %s: %s\n"
                                     % (type(e).__name__, e))
                time.sleep(a.interval)
        threading.Thread(target=loop, daemon=True).start()
        srv = ThreadingHTTPServer(("0.0.0.0", a.port), Server)
        sys.stderr.write("nodewatch on http://localhost:%d/  "
                         "(text at /status.txt)\n" % a.port)
        srv.serve_forever()
        return

    if not a.watch and a.settle > 0:
        # Two samples, `settle` apart: a pasted status that says "0 blocks in
        # the last 10 s" for one node and "10" for the others is the whole
        # point, and one sample cannot say it.
        w.sample()
        time.sleep(a.settle)

    while True:
        s = w.sample()
        print(json.dumps(s, indent=2) if a.json
              else render_text(s, not a.no_legend))
        if not a.watch:
            return
        print("")
        time.sleep(a.interval)


if __name__ == "__main__":
    main()
