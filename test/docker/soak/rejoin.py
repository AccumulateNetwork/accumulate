#!/usr/bin/env python3
"""Did a restarted node rejoin? The manifest's start rows, from the run's files.

    rejoin.py RUN_DIR ROLE [--max-behind N] [--log node-logs-live.txt]

**Rejoined means executing with the partition, not a gauge (#4404).** A start
of a node, per partition, is REJOINED when all three hold:

1. **the gauge** — `accumulate_node_state` read ACTIVE after the start
   (`nodestate.csv`, kind `reached`);
2. **execution** — it was ACTIVE with its executed block within N blocks of
   the partition's height (kind `caught-up`), and it was still within N at
   the start's last reading (kind `final` or `superseded`). The partition's
   height is the highest block any OTHER of its validators that answered the
   sample executed (heights.py; a sample where none answered is no reading);
3. **agreement** — every anchor the node wrote for that partition after its
   start agrees with its peers': the same (root, BPT) as the other
   validators' at the same block, and the same (block, root, BPT) under the
   same sequence number to the same destination. This is the reading the
   follower's verdict already makes (followerlog.py, `Sending an anchor` /
   `Anchor not sent`, source read from the line), made of any node.

The gauge alone cannot be the reading: it goes ACTIVE at the join's first root
match, mid-join (internal/node/join/state.go:857 -> tracker.go:194), and is
never demoted. Run 20260924T052134Z read three failed starts as ACTIVE:

    acc-bvn1-val1 bvn1       ACTIVE 12.1 s; diverged at 204, stuck at 214 vs 1376
    acc-bvn3-val1 directory  ACTIVE 26.5 s, 81 s before a handoff that failed; 613 vs 1364
    acc-bvn2-val2 directory  ACTIVE 29.4 s, 262 s before a handoff that failed; then
                             signed seq 1154 as block 1300 root f5b4979b, where its
                             peers' seq 1154 is block 1301 root de98b6c8

Execution is measured by the monitor, agreement by the log, so each can be
missing. A missing reading is said, per start: the start is then **not
established**, never rejoined. A failing reading makes it **NOT rejoined**
whatever else is missing.

Starts are judged only if seen booting (a restart inside the run) or never
ACTIVE. A start ACTIVE at the monitor's first sight of it is the network's
own launch and is counted, not judged.
"""
import argparse
import calendar
import csv
import os
import re
import sys
import time
from collections import Counter

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import followerlog  # noqa: E402
from heights import canon_part  # noqa: E402

# Healthy validators' spread, measured from run 20260924T052134Z's anchors
# per second with disturbed nodes excluded: Directory p50 0, p99 1, max 3
# blocks over 1,345 s; BVNs p99 0 (review F6). 5 clears the max with room for
# the scrape's own skew and still flags a node a few seconds behind.
DEFAULT_MAX_BEHIND = 5
# How long after a pause ends a node's last reading is still inside it: a
# paused node returns ~10 blocks/s (93 behind to 0 in 9 s, review F6).
DEFAULT_RECOVERY = 30


def pauses_from(lines):
    """{container: [(iso start, seconds)]} from chaos.log's `pause` lines. The
    chaos walk logs the pause and never the un-pause; the un-pause is the
    start plus the logged seconds."""
    out = {}
    for line in lines or ():
        m = re.match(r"^(\S+Z) pause (\S+) (\d+)s", line)
        if m:
            out.setdefault(m.group(2), []).append((m.group(1), int(m.group(3))))
    return out


def _in_pause(pauses, node, t, recovery):
    """The pause whose span, plus the recovery allowance, holds time t."""
    if t is None:
        return None
    for start, secs in (pauses or {}).get(node, ()):
        a = _epoch(start)
        if a is not None and a <= t <= a + secs + recovery:
            return (start, secs)
    return None


# A start whose last answer is older than this before its final row has no
# reading at the end: three of soakmon's 5 s scrapes (soak.conf REJOIN_SILENT_SECS).
DEFAULT_SILENT_AFTER = 15


def _epoch(s):
    s = (s or "").strip()
    if not s:
        return None
    try:
        return calendar.timegm(time.strptime(s[:19], "%Y-%m-%dT%H:%M:%S"))
    except ValueError:
        return None


def _int(v):
    try:
        return int(float(v))
    except (TypeError, ValueError):
        return None


# -- the log --------------------------------------------------------------

def anchor_events(lines):
    """Every anchor a node stated, as (container, epoch, source, destination,
    block, seq, (root, bpt)). `Sending an anchor` and `Anchor not sent`
    carry the same reading (followerlog's docstring); a line with no
    `source` (a build before #4370) is skipped, not guessed."""
    out = []
    for c, ts, ev, f in followerlog.parse(lines):
        if ev not in ("anchor", "anchor-not-sent"):
            continue
        src = followerlog._anchor_source(f)
        blk = followerlog._int(f, "block")
        if src is None or blk is None:
            continue
        out.append((c, _epoch(ts), canon_part(src), f.get("destination"),
                    blk, followerlog._int(f, "seq"), (f.get("root"), f.get("bpt"))))
    return out


class Anchors:
    """The peers' reading per (source, block) and per (source, destination,
    seq), each the majority value among the peers that stated one."""

    def __init__(self, events):
        self.events = events
        self.by_node = {}
        for e in events:
            self.by_node.setdefault(e[0], []).append(e)

    def _majority(self, peers, key_of, val_of):
        """One vote per PEER per value, never per line: a node that re-sends
        one anchor 304 times (acc-bvn2-val2, 05:45:44) is one peer."""
        votes = {}
        for p in peers:
            mine = set()
            for e in self.by_node.get(p, ()):
                k = key_of(e)
                if k is not None:
                    mine.add((k, val_of(e)))
            for k, v in mine:
                votes.setdefault(k, Counter())[v] += 1
        return {k: c.most_common(1)[0][0] for k, c in votes.items()}

    def last_blocks(self, node, partition, peers):
        """(the node's last anchored block, its peers' highest) for the
        partition — log evidence of how far it got, when the monitor's
        executed height is missing."""
        part = canon_part(partition)
        last = lambda c: max((e[4] for e in self.by_node.get(c, ()) if e[2] == part), default=None)
        theirs = [b for b in (last(p) for p in peers if p != node) if b is not None]
        return last(node), (max(theirs) if theirs else None)

    def agreement(self, node, partition, since, until, peers):
        """Compare `node`'s anchors for `partition` stated in [since, until]
        with `peers`'. Returns {"compared", "disagree": [str], "measured"}."""
        part = canon_part(partition)
        mine = [e for e in self.by_node.get(node, ())
                if e[2] == part and e[1] is not None
                and (since is None or e[1] >= since)
                and (until is None or e[1] <= until)]
        if not mine:
            return {"measured": False, "compared": 0, "disagree": [],
                    "why": "it stated no %s anchor after its start" % partition}
        peers = [p for p in peers if p != node]
        by_block = self._majority(peers, lambda e: (e[2], e[4]) if e[2] == part else None,
                                  lambda e: e[6])
        by_seq = self._majority(peers,
                                lambda e: (e[2], e[3], e[5]) if e[2] == part and e[5] is not None else None,
                                lambda e: (e[4], e[6]))
        compared, bad, seen = 0, [], set()
        for (_c, _t, src, dest, blk, seq, val) in mine:
            k1 = (src, blk)
            if k1 in by_block and ("b", k1) not in seen:
                seen.add(("b", k1))
                compared += 1
                if by_block[k1] != val:
                    bad.append("block %d root %s, its peers' %s" % (blk, val[0], by_block[k1][0]))
            k2 = (src, dest, seq)
            if seq is not None and k2 in by_seq and ("s",) + k2 not in seen:
                seen.add(("s",) + k2)
                compared += 1
                pb, pv = by_seq[k2]
                if (pb, pv) != (blk, val):
                    bad.append("seq %d to %s as block %d root %s, its peers' block %d root %s"
                               % (seq, dest, blk, val[0], pb, pv[0]))
        if not compared:
            return {"measured": False, "compared": 0, "disagree": [],
                    "why": "none of its %d %s anchor line(s) after its start has a peer's to compare"
                           % (len(mine), partition)}
        return {"measured": True, "compared": compared, "disagree": bad, "why": None}


# -- nodestate.csv ----------------------------------------------------------

def starts(rows):
    """{(node, partition, containerStarted): {"role", kind: row}} — the last
    row of each kind per start."""
    out = {}
    for r in rows:
        k = (r["node"], r["partition"], r["containerStarted"])
        s = out.setdefault(k, {"role": r.get("role")})
        s[r["kind"]] = r
    return out


def judge(key, s, max_behind, anchors=None, peers=(), has_cols=True,
          silent_after=DEFAULT_SILENT_AFTER, pauses=None, recovery=None):
    """One start's verdict: {"verdict": rejoined|NOT rejoined|not established,
    "reasons": [...], "toActiveS", "toRejoinS"}."""
    node, part, started = key
    recovery = DEFAULT_RECOVERY if recovery is None else recovery
    active = s.get("reached") or s.get("already")
    end = s.get("superseded") or s.get("final")
    fails, missing = [], []
    to_active = active and active.get("startToActiveS")
    if active and "reached" not in s:
        # ACTIVE at the monitor's first sight of this start: its boot was
        # not watched, so the figure is an upper bound on the gauge's time
        # and says nothing of the join. Judged on height and anchors alike.
        to_active = "≤%ss (boot time not measured: ACTIVE at the monitor's first sight of this start)" % to_active
    elif to_active:
        to_active = "%ss" % to_active
    if not active:
        state = (end or {}).get("state") or "state unknown"
        fails.append("never ACTIVE (%s)" % state)
    if not has_cols:
        note = "executed height not measured (nodestate.csv predates #4404)"
        if anchors is not None:
            mine, theirs = anchors.last_blocks(node, part, peers)
            if mine is not None and theirs is not None:
                note += "; in the log its last %s anchor is block %d, its peers' %d" % (part, mine, theirs)
        missing.append(note)
    elif end is None:
        missing.append("no last reading (the monitor wrote no final row)")
    else:
        ex, ph = _int(end.get("executedBlock")), _int(end.get("partitionHeight"))
        last, at = _epoch(end.get("lastAnswered")), _epoch(end.get("time"))
        silent = (end.get("kind") == "final" and last is not None and at is not None
                  and at - last > silent_after)
        if silent:
            # Its last answer is not its state at the end (review F3): judged
            # on it, a node that rejoined and then went dark reads rejoined.
            missing.append("silent since %s: its last answer, %ds before the end, was executed %s; "
                           "the partition was at %s at the end"
                           % (end.get("lastAnswered"), at - last,
                              "?" if ex is None else ex, "?" if ph is None else ph))
        elif ex is None or ph is None:
            if ex is not None and _int(end.get("validatorsAnswered")) == 0:
                missing.append("no other validator of %s answered at its last reading" % part)
            else:
                missing.append("executed height not measured (no accumulate_node_executed_block at its last reading)")
        elif ph - ex > max_behind:
            ans = _int(end.get("validatorsAnswered"))
            gap = ("executed %d vs partition %d at its last reading (%d behind; bound %d%s)"
                   % (ex, ph, ph - ex, max_behind,
                      "" if ans is None else "; %d validators answered" % ans))
            p = _in_pause(pauses, node, _epoch(end.get("time")), recovery)
            if p:
                # A pause is a disturbance the node is expected to recover
                # from at ~10 blocks/s (93 behind to 0 in 9 s on run
                # 20260924T052134Z): a last reading inside one says nothing
                # about the rejoin (review F6, first edge).
                missing.append("paused at its last reading (paused %s for %ds, recovery allowed %ds): %s"
                               % (p[0], p[1], recovery, gap))
            else:
                fails.append(gap)
        if active and "caught-up" not in s and ex is not None and ph is not None:
            fails.append("never ACTIVE and within %d blocks of the partition at one sample" % max_behind)
    if anchors is None:
        missing.append("anchor agreement not measured (no node log)")
    else:
        until = _epoch((end or {}).get("time")) if (end or {}).get("kind") == "superseded" else None
        a = anchors.agreement(node, part, _epoch(started), until, peers)
        if not a["measured"]:
            missing.append("anchor agreement not measured: %s" % a["why"])
        elif a["disagree"]:
            fails.append("anchor disagrees with its peers: %s%s"
                         % (a["disagree"][0], "" if len(a["disagree"]) == 1
                            else " (and %d more)" % (len(a["disagree"]) - 1)))
    sup = s.get("superseded")
    if sup and not active:
        # Restarted again before this start was ever ACTIVE (review F7): not
        # a node stuck booting, and not a verdict. The next start is judged.
        return {"verdict": "superseded", "reasons": ["restarted again at %s, before it was ACTIVE"
                                                    % sup.get("time")],
                "toActiveS": None, "toRejoinS": None}
    caught = s.get("caught-up")
    to_rejoin = caught.get("startToCaughtUpS") if caught else None
    if fails:
        verdict = "NOT rejoined"
    elif missing:
        verdict = "not established"
    else:
        verdict = "rejoined"
    return {"verdict": verdict, "reasons": fails + missing,
            "toActiveS": to_active or None, "toRejoinS": to_rejoin or None}


def row(rows, role, max_behind=DEFAULT_MAX_BEHIND, anchors=None, silent_after=DEFAULT_SILENT_AFTER,
        pauses=None):
    """The manifest's cell for one role."""
    all_rows = rows
    rows = [r for r in rows if not role or r.get("role") == role]
    if not rows:
        return "— not measured (no %s row in `nodestate.csv`)" % (role or "node")
    peers = sorted({r["node"] for r in all_rows if r.get("role") == "validator"})
    has_cols = "executedBlock" in (rows[0] or {})
    st = starts(rows)
    # The launch: every container started before the monitor's first
    # sample. Only those are the network's own start and go unjudged. Any
    # start after it is judged, however it was first seen (review F1): a
    # join that completes inside one scrape interval, and a restart that
    # spans a monitor restart (a new soakmon has an empty track, so it sees
    # every node `already` ACTIVE), both read ACTIVE at first sight.
    launch = min((t for t in (_epoch(r.get("time")) for r in all_rows) if t is not None),
                 default=None)
    judged, first_sight = {}, 0
    for k, s in st.items():
        started = _epoch(k[2])
        after_launch = launch is None or started is None or started > launch
        if "reached" in s or "already" not in s or after_launch:
            judged[k] = judge(k, s, max_behind, anchors, peers, has_cols, silent_after, pauses)
        else:
            first_sight += 1
    parts = []
    if not judged:
        parts.append("no start after the network's launch")
    else:
        ok = {k: v for k, v in judged.items() if v["verdict"] == "rejoined"}
        n = sum(1 for v in judged.values() if v["verdict"] != "superseded")
        head = "rejoined %d of %d start(s) after the launch" % (len(ok), n)
        timed = [(float(v["toRejoinS"]), k) for k, v in ok.items() if v["toRejoinS"]]
        if timed:
            w, k = max(timed)
            head += ", worst %.1fs from container start to executing with its partition (%s %s)" % (w, k[0], k[1])
        parts.append(head)
        for label in ("NOT rejoined", "not established", "superseded"):
            bad = [(k, v) for k, v in sorted(judged.items()) if v["verdict"] == label]
            if bad:
                parts.append("%s: %s" % (label, "; ".join(
                    "%s %s (%s%s)" % (k[0], k[1],
                                      "gauge ACTIVE at %s; " % v["toActiveS"] if v["toActiveS"] else "",
                                      "; ".join(v["reasons"]))
                    for k, v in bad)))
    if first_sight:
        parts.append("%d started before the monitor's first sample (the network's launch: not judged)"
                     % first_sight)
    return "; ".join(parts)


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("run_dir")
    ap.add_argument("role", nargs="?", default="")
    ap.add_argument("--max-behind", type=int, default=DEFAULT_MAX_BEHIND)
    ap.add_argument("--silent-after", type=int, default=DEFAULT_SILENT_AFTER)
    ap.add_argument("--log", default=None, help="default RUN_DIR/node-logs-live.txt")
    a = ap.parse_args(argv)
    path = os.path.join(a.run_dir, "nodestate.csv")
    try:
        with open(path) as f:
            rows = list(csv.DictReader(f))
    except OSError:
        print("— not measured (no `nodestate.csv`: no node exported accumulate_node_state, "
              "or no container start was read)")
        return 0
    anchors = None
    logp = a.log or os.path.join(a.run_dir, "node-logs-live.txt")
    if os.path.exists(logp):
        with open(logp, errors="replace") as f:
            anchors = Anchors(anchor_events(f))
    pauses = {}
    try:
        with open(os.path.join(a.run_dir, "chaos.log")) as f:
            pauses = pauses_from(f)
    except OSError:
        pass
    print(row(rows, a.role, a.max_behind, anchors, a.silent_after, pauses))
    return 0


if __name__ == "__main__":
    sys.exit(main())
