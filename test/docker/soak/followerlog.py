#!/usr/bin/env python3
"""Gate 0's verdict (#4365), read out of the node log.

    followerlog.py runs/<id>/node-logs-live.txt
        [--follower acc-bvn3-fol1] [--definition runs/<id>/network-definition.json]

A follower is a node with a validator's wiring and a key that is in no
committee (executor spec, "Sync" step 5: *a follower differs only in what it
does with the blocks it processes — it does not vote or propose*). The gate
asks four things of a five-minute run, and this answers all four from the
log the run already captures, plus the NetworkDefinition it captures before
load starts:

1. **Does it compute the same state?** Every node's conductor logs its
   block's anchor per block at Info, with ``source``, ``root`` and ``bpt``
   (conductor.go; ``source`` since #4370) — the same line
   `reading-a-run.md` uses for a divergence. Compare the follower's against
   any validator's, per (source partition, block).

   **A validator logs ``Sending an anchor``; a node in no committee logs
   ``Anchor not sent``.** Since #4367 a node outside the committee neither
   signs nor dispatches an anchor — it sent 1,997 and every one was refused
   at msg_block_anchor.go:285 on run `20260919T191634Z` — so it states the
   root it computed instead, once per block rather than once per
   destination, with the same fields. Both messages carry the same reading
   and are compared the same way; which one a node wrote is itself a result,
   and the rows below count them separately. A follower that logs
   ``Sending an anchor`` at all has regressed.
2. **Is its key in any committee?** Each node logs the committee it built
   at Info (``Extracted initial validators for DAG-BFT``, dagbft.go:427) and
   every later change (``Validator added``, certificate_handler.go:379).
3. **Did any validator ever see a certificate it authored?**
   ``Invalid certificate`` is Info deliberately (#4054) and
   certificate.go:170 refuses ``header author is not in committee``.
4. **Which form of "in no committee" actually ran?** The NetworkDefinition
   says whether the follower's key is in it and inactive, or not in it at
   all. The manifest has to record which.

**Never compare without the partition, and read it from the line.** Every
container runs **two nodes** — a DN node and a BVN node — in one process,
sharing one log stream (Paul, 2026-09-19: *"There are no engines. Every
container runs two nodes: a DN node and a BVN node, in one process, sharing
one log stream."*). `acc-bvn1-val1` is a DN node plus a BVN1 node. The DN
node anchors to every partition **including `dn.acme`, itself**; the BVN
node anchors to `dn.acme`. So one container emits two different
`(root, bpt)` against `destination=acc://dn.acme` for the same block —
two nodes, not two halves of one — proven on run `20260917T212457Z`,
`acc-bvn1-val1`, block 500: `root=4597dc3a bpt=6fcdbe82` at 21:34:26Z and
`root=96c2b37b bpt=d2ce8d05` at 21:34:34Z.

The line therefore has to say which node sent it, and since #4370 it does:
`source=<partition id>` (`Directory`, `BVN1`, …). Anchors are grouped by
`(source, destination, block)` read from the line — **no inference**. An
earlier version of this reader guessed the source by matching a `dn.acme`
line's `(root, bpt)` against the same container's `bvn-*` lines; the guess
was removable because the emitter can simply identify itself, and a
heuristic in this one tool would have left the same ambiguity in the
run-analyst's divergence verdict and in `reading-a-run.md`'s recipe.

A log whose anchor lines carry no `source` — anything built before #4370 —
is **not compared**: the root section renders `— not measured` and names
the issue. It does not fall back to the guess.

**What this cannot see, and says so.** ``Header from unknown validator``
(vote_handler.go:284), ``Vote from unknown validator`` (:35) and
``Created certificate`` (:160) are ``slog.Debug`` with no ``module``
attribute. The generated config (cmd_init_network.go:222-228) sets Debug for
named modules and Info as the default for everything else, and a rule with no
modules sets the default level (run/logging.go:29-36) — so a Debug line with
no module is dropped and this build emits none of the three. They render
``— not measured``, never 0. Counting them and printing zero would report
"no header of the follower's was ever rejected" about a network that never
said a word on the subject, which is the #4095 failure with a new name.
The counters below are live anyway, so adding ``"module", "primary"`` to
those three calls turns the rows on with no change here.
"""
import argparse
import json
import re
import sys
from collections import OrderedDict

ANSI = re.compile(r"\x1b\[[0-9;]*m")

# The messages this reads. Each is Info in the build the soak runs, except
# the two Debug ones, which are listed so that a build that DOES emit them is
# counted rather than ignored.
_MSGS = OrderedDict([
    ("Starting consensus node", "identity"),
    ("Extracted initial validators for DAG-BFT", "committee"),
    ("Updated committee", "committee-update"),
    ("Validator added", "validator-added"),
    ("Validator removed", "validator-removed"),
    ("Sending an anchor", "anchor"),
    ("Anchor not sent", "anchor-not-sent"),
    ("Invalid certificate", "invalid-cert"),
    ("Header from unknown validator", "header-drop"),
    ("Vote from unknown validator", "vote-drop"),
])
_MSG_RE = re.compile(
    r"^(?P<node>\S+)\s*\|\s*(?P<ts>\S+)\s+(?:INFO|DEBUG|WARN|ERROR)\s+"
    r"(?P<msg>" + "|".join(re.escape(m) for m in _MSGS) + r")\s*(?P<kv>.*)$")
# Values may be quoted, because an error message has spaces in it.
_KV = re.compile(r'(\w+)=("[^"]*"|\S+)')


def parse(lines):
    """Yield (container, ts, event, fields) for every line this reader knows.

    The first field is the log's line prefix, which is the CONTAINER — it
    runs two nodes, a DN node and a BVN node, sharing this one stream."""
    for raw in lines:
        line = ANSI.sub("", raw.rstrip("\n"))
        m = _MSG_RE.match(line)
        if not m:
            continue
        f = {}
        for k, v in _KV.findall(m.group("kv")):
            f[k] = v[1:-1] if v.startswith('"') else v
        yield m.group("node"), m.group("ts"), _MSGS[m.group("msg")], f


def _int(f, key):
    """A field as a number, or None. A log line is text; one odd token in a
    multi-gigabyte log must not abort the analysis in a comparison."""
    try:
        return int(str(f.get(key, "")).replace(",", ""))
    except (TypeError, ValueError):
        return None


def _anchor_source(f):
    """The partition that produced this anchor, from the line's own
    `source` attribute (#4370). None when the build predates it.

    #4370 logs `c.Partition.ID`, so the value is a bare partition id —
    `Directory`, `BVN1`. A partition URL is accepted too and reduced to the
    id, so that this reader is not broken by the emitter being changed to
    log `protocol.PartitionUrl(...)` instead; the id is what every other
    reading in this file is keyed by.
    """
    src = (f.get("source") or "").strip()
    if not src:
        return None
    if "//" in src:                       # acc://dn.acme, acc://bvn-BVN1.acme
        host = src.split("//", 1)[1].split(".")[0]
        return "Directory" if host.lower() == "dn" else host.replace("bvn-", "")
    return src


class Report:
    """Everything the log said, indexed by what the gate asks of it."""

    def __init__(self):
        # Keyed by CONTAINER — the log's line prefix. A container runs two
        # nodes, a DN node and a BVN node, so a container is not a node and
        # the two words are kept apart here deliberately.
        self.identities = {}        # container -> {partition: 16-hex key}
        self.committees = {}        # (container, partition) -> size
        self.anchors = {}           # container -> {(source, block): (root, bpt)}
        # Which of the two lines each container wrote. A node in the
        # committee dispatches; a node outside it states its root and sends
        # nothing (#4367). Counted apart, because "the follower dispatched
        # nothing" is a result and not a detail of how the roots were read.
        self.dispatched = {}        # container -> `Sending an anchor` lines
        self.stated = {}            # container -> `Anchor not sent` lines
        self.invalid = []           # (container, ts, author, error)
        self.changes = []           # (container, ts, kind, pubkey)
        self.drops = {"header": {}, "vote": {}}   # kind -> {container: count}
        self.sawDropLine = False
        # Anchor lines with no `source` attribute: a build before #4370.
        # Counted and NOT filed — the container's two nodes cannot be told
        # apart without it, and this reader does not guess.
        self.sourceless = {}        # container -> count
        # One (container, source, block) given two different values. Log
        # order used to decide this silently; it is a finding.
        self.conflicts = []         # (container, source, block, first, second)

    # -- identity -------------------------------------------------------
    def key_prefix(self, container):
        """The eight hex characters that `author=` and `pubkey=` carry.

        Both of a container's nodes run with the same key here, so one value
        is the container's. `hexEncode` (vote_handler.go:552) is the first 8 characters of
        `HeaderDigest(key).String()`, and HeaderDigest is a cast of the
        32-byte public key, not a hash of it — so it is the key's own hex,
        and the 16 characters a node logs for itself share that prefix.
        """
        ids = self.identities.get(container)
        if not ids:
            return None
        return sorted(ids.values())[0][:8]

    def bvn_of(self, container):
        """The partition of the container's BVN node, taken from what it
        logged rather than from its container name."""
        for p in self.identities.get(container, {}):
            if p != "Directory":
                return p
        m = re.match(r"acc-(bvn\d+)-", container or "")
        return m.group(1).upper() if m else None

    def anchor_blocks(self, container):
        return list(self.anchors.get(container, {}))


def read(lines):
    """One pass over `lines`, which may be a FILE and therefore single-use.

    This read the input twice at first — once for the identities, once for
    the anchors, because an anchor line's source partition is derived from
    the node's own BVN and that is only known from its identity line. Every
    test passed a list, so every test passed; `main` passes an open file,
    and the second pass over an exhausted handle yielded nothing, so a real
    run would have reported "the follower logged no anchor line" for a
    follower that logged four hundred of them. The anchor events are held
    and resolved after the pass instead.
    """
    r = Report()
    pending = []
    for container, ts, ev, f in parse(lines):
        if ev in ("anchor", "anchor-not-sent"):
            sent = ev == "anchor"
            pending.append((container, f, sent))
            slot = r.dispatched if sent else r.stated
            slot[container] = slot.get(container, 0) + 1
            continue
        if ev == "identity":
            part = f.get("partition")
            key = f.get("validatorKey")
            if part and key:
                r.identities.setdefault(container, {})[part] = key
        elif ev == "committee":
            n = _int(f, "validators")
            part = f.get("partition")
            if part and n is not None:
                r.committees[(container, part)] = n
        elif ev in ("validator-added", "validator-removed"):
            r.changes.append((container, ts, ev.split("-")[1], f.get("pubkey")))
        elif ev == "invalid-cert":
            r.invalid.append((container, ts, f.get("author"), f.get("error", "")))
        elif ev == "header-drop":
            r.sawDropLine = True
            r.drops["header"][container] = r.drops["header"].get(container, 0) + 1
        elif ev == "vote-drop":
            r.sawDropLine = True
            r.drops["vote"][container] = r.drops["vote"].get(container, 0) + 1
    # Anchors are resolved after the pass, because they need each
    # container's identity lines. Grouped by (source, destination, block)
    # read from the line itself (#4370) — the source partition is stated,
    # never inferred.
    seen = OrderedDict()
    for container, f, _sent in pending:
        blk = _int(f, "block")
        if blk is None:
            continue
        src = _anchor_source(f)
        if src is None:
            # A build before #4370. The container's DN node and BVN node
            # both address dn.acme, so without `source` there is nothing in
            # the line that separates them. Counted; not filed; not guessed.
            r.sourceless[container] = r.sourceless.get(container, 0) + 1
            continue
        value = (f.get("root"), f.get("bpt"))
        key = (container, src, f.get("destination"), blk)
        prev = seen.get(key)
        if prev is not None and prev != value:
            r.conflicts.append((container, src, blk, prev, value))
            continue
        seen[key] = value

    # The DN node sends the same anchor to every partition, so one
    # (source, block) arrives once per destination with identical values.
    # Two destinations of one source disagreeing is the node contradicting
    # itself — not something log order should quietly resolve.
    for (container, src, _dest, blk), value in seen.items():
        slot = r.anchors.setdefault(container, {})
        prev = slot.get((src, blk))
        if prev is not None and prev != value:
            r.conflicts.append((container, src, blk, prev, value))
            continue
        slot[(src, blk)] = value
    return r


def compare_roots(report, follower, validators):
    """The follower's state root and BPT root against the validators', per
    (source partition, block) — the source read from the line (#4370).

    Only blocks BOTH sides anchored are compared. A block only the follower
    reached — it was a block ahead when the log was cut — is counted as
    uncompared, not as a mismatch; a follower silent on every block is
    `— not measured`, and the fallback is the v3 API on the follower and a
    validator at the same ledger index.
    """
    mine = report.anchors.get(follower, {})
    sourceless = report.sourceless.get(follower, 0)
    base = {"measured": False, "compared": 0, "uncompared": 0,
            "mismatches": [], "firstMismatch": None,
            "sourceless": sourceless,
            "dispatched": report.dispatched.get(follower, 0),
            "stated": report.stated.get(follower, 0),
            "anchorLines": (report.dispatched.get(follower, 0)
                            + report.stated.get(follower, 0)),
            "conflicts": [c for c in report.conflicts if c[0] == follower]}
    if not mine and sourceless:
        # Every anchor line it logged predates #4370. Comparing them would
        # mean guessing which of the container's two nodes sent each one,
        # and a guess in a root verdict is worse than no verdict.
        return dict(base, why="anchor lines carry no source partition; #4370")
    if not mine:
        return dict(base, why="the follower logged no anchor line — fall back "
                              "to the v3 API on the follower and a validator "
                              "at the same ledger index (query with "
                              "includeReceipt)")
    theirs = {}
    for v in validators:
        for k, val in report.anchors.get(v, {}).items():
            theirs.setdefault(k, (v, val))
    compared, uncompared, bad = 0, 0, []
    for (part, blk) in sorted(mine):
        if (part, blk) not in theirs:
            uncompared += 1
            continue
        compared += 1
        who, ref = theirs[(part, blk)]
        if mine[(part, blk)] != ref:
            bad.append((part, blk, mine[(part, blk)], ref, who))
    return {"measured": True, "compared": compared, "uncompared": uncompared,
            "mismatches": bad, "firstMismatch": bad[0] if bad else None,
            "sourceless": sourceless,
            "dispatched": report.dispatched.get(follower, 0),
            "stated": report.stated.get(follower, 0),
            "anchorLines": (report.dispatched.get(follower, 0)
                            + report.stated.get(follower, 0)),
            "conflicts": [c for c in report.conflicts if c[0] == follower],
            "why": None}


def first_root_match(report, follower, validators):
    """The earliest block at which the follower's (root, bpt) equals a
    validator's for the same source partition — the moment an added
    follower is first seen computing the network's state (#4364).

    Returns (source, block, root) or None. Blocks are taken in order, per
    source; a block only one side anchored is skipped, and so is a block
    where the two differ: that is compare_roots' finding, not this one's.
    """
    mine = report.anchors.get(follower, {})
    theirs = {}
    for v in validators:
        for k, val in report.anchors.get(v, {}).items():
            theirs.setdefault(k, val)
    best = None
    for (part, blk) in sorted(mine, key=lambda k: (k[1], k[0])):
        if theirs.get((part, blk)) == mine[(part, blk)]:
            best = (part, blk, mine[(part, blk)][0])
            break
    return best


def committee_check(report, follower, validators):
    """The committee every node built, and every change to one during the run.

    The follower is excluded when: every node that reported a size for a
    partition reported the SAME size, and the follower's key was never added
    to a committee. Disagreeing sizes are a finding in their own right — two
    nodes running different committees is the divergence gate 0 would
    otherwise be measured against.
    """
    sizes, disagree = {}, {}
    for (container, part), n in report.committees.items():
        seen = sizes.setdefault(part, n)
        if seen != n:
            disagree.setdefault(part, set()).add(seen)
            disagree[part].add(n)  # two nodes of one partition disagreeing
    if not sizes:
        return {"measured": False, "sizes": {}, "disagree": {},
                "addedDuringRun": [], "followerExcluded": None,
                "why": "no node logged `Extracted initial validators for DAG-BFT`"}
    key = report.key_prefix(follower)
    added = [c for c in report.changes if c[2] == "added"]
    follower_added = [c for c in added if key and (c[3] or "").startswith(key)]
    return {"measured": True, "sizes": sizes,
            "disagree": {k: sorted(v) for k, v in disagree.items()},
            "addedDuringRun": added,
            "followerExcluded": (not disagree) and not follower_added,
            "why": None}


def certificate_check(report, follower, validators):
    """Certificates a validator refused because their author was outside the
    committee, and how many of those the follower authored."""
    key = report.key_prefix(follower)
    non_committee = [c for c in report.invalid if "not in committee" in (c[3] or "")]
    by_val = {}
    for container, _, _, _ in non_committee:
        by_val[container] = by_val.get(container, 0) + 1
    mine = [c for c in non_committee if key and (c[2] or "").startswith(key)]
    return {"measured": True,
            "nonCommitteeAuthor": len(non_committee),
            "byFollower": len(mine),
            "byValidator": by_val,
            "otherInvalid": len(report.invalid) - len(non_committee),
            "firstByFollower": mine[0] if mine else None}


def drop_check(report, follower, validators):
    """Headers and votes validators dropped for a non-committee author.

    Absent in this build, on purpose: see the module docstring. If a later
    build gives those calls a `module` attribute they are counted here with
    no other change.
    """
    if not report.sawDropLine:
        return {"measured": False, "headerDrops": None, "voteDrops": None,
                "byValidator": {},
                "why": "`Header from unknown validator` and `Vote from unknown "
                       "validator` are slog.Debug with no `module` attribute "
                       "(vote_handler.go:34,284); the generated logging config "
                       "sets Debug per module and Info as the default, so this "
                       "build emits neither line"}
    hd = sum(report.drops["header"].values())
    vd = sum(report.drops["vote"].values())
    by = {}
    for kind in ("header", "vote"):
        for container, n in report.drops[kind].items():
            by.setdefault(container, {})[kind] = n
    return {"measured": True, "headerDrops": hd, "voteDrops": vd,
            "byValidator": by, "why": None}


def definition_check(status, key_prefix=None):
    """What the NetworkDefinition says about THE FOLLOWER'S OWN key.

    `status` is the `result` of a `network-status` call, captured before load
    starts. A validator entry is active on a partition or it is not, and the
    DAG-BFT committee is built from the active ones only
    (run/dagbft.go:415) — so the follower's key with no active partition is
    precisely a node that is in the definition and in no committee.

    `key_prefix` is the eight hex characters the follower logged for itself
    (`Report.key_prefix`); the capture carries the full `publicKey` hex, so
    the two are matched by prefix. It is looked up rather than inferred from
    "is there any inactive entry": if init misbehaved and the follower's key
    came out ACTIVE — the gate's premise failing — the inferred answer was
    "absent from the definition", the most reassuring row on the page,
    printed beside a count that said the partition had one validator too
    many (M5).
    """
    try:
        vals = status["network"]["validators"]
    except (TypeError, KeyError):
        return {"measured": False, "active": {}, "inactiveKeys": [],
                "followerKey": None, "followerKeyForm": None,
                "followerActiveOn": [], "followerInNoCommittee": None,
                "why": "no network-status capture for this run"}
    active, inactive = {}, []
    for v in vals:
        parts = v.get("partitions") or []
        on = [p["id"] for p in parts if p.get("active")]
        for p in on:
            active[p] = active.get(p, 0) + 1
        if not on:
            inactive.append(v.get("publicKey"))

    out = {"measured": True, "active": active, "inactiveKeys": inactive,
           "followerKey": None, "followerKeyForm": None,
           "followerActiveOn": [], "followerInNoCommittee": None, "why": None}
    if not key_prefix:
        out["why"] = ("the follower logged no identity line, so its key is "
                      "not known and its entry cannot be looked up")
        return out

    pre = key_prefix.lower()
    mine = [v for v in vals if (v.get("publicKey") or "").lower().startswith(pre)]
    if len(mine) > 1:
        out["followerKeyForm"] = ("%d entries share the follower's key prefix "
                                  "%s — the definition cannot be read" %
                                  (len(mine), key_prefix))
        return out
    if not mine:
        out["followerKeyForm"] = "absent from the definition"
        out["followerInNoCommittee"] = True
        return out

    v = mine[0]
    on = [p["id"] for p in (v.get("partitions") or []) if p.get("active")]
    out["followerKey"] = v.get("publicKey")
    out["followerActiveOn"] = on
    if on:
        # The premise of the gate failing. Say it, loudly, in the row.
        out["followerKeyForm"] = ("**ACTIVE in the definition on %s** — this "
                                  "node IS in a committee and is not a "
                                  "follower" % ", ".join(sorted(on)))
        out["followerInNoCommittee"] = False
    else:
        out["followerKeyForm"] = "inactive in the definition"
        out["followerInNoCommittee"] = True
    return out


def behind_summary(lines, follower=None):
    """The run's `behind` series, out of follower.csv, for ONE follower.

    `follower` is the container the report is about. follower.csv carries a
    row per follower per partition per sample, and summarising all of them
    under one name folded a late follower's catch-up into fol1's numbers:
    fol1 steady one block behind read `maxBehind 497` because fol2 had just
    been added at height 3 (#4389). None keeps every row, for a caller that
    has only one follower's file.

    Columns: time, follower, partition, followerHeight, validatorsMaxHeight,
    behindBlocks — and a sample where the follower did not answer writes an
    EMPTY behind, not a 0 (soakmon.follower_csv_rows). Those samples are
    counted as unanswered, never averaged in as caught up: a follower that
    went silent for the second half of a run must not read as the best half
    of it.
    """
    rows, unanswered, marks = [], 0, []
    for ln in lines:
        p = ln.rstrip("\n").split(",")
        if len(p) < 6 or p[0] == "time":
            continue
        if follower is not None and p[1] != follower:
            continue
        # The monitor's own high-water mark, carried in every row since M4.
        # A run directory written before that column exists has six fields;
        # it stays readable and the report says which source it used.
        if len(p) >= 7 and p[6] != "":
            try:
                marks.append((p[0], p[2], int(p[6])))
            except ValueError:
                pass
        if p[5] == "":
            unanswered += 1
            continue
        try:
            rows.append((p[0], p[1], p[2], int(p[5])))
        except ValueError:
            continue
    if not rows:
        return {"measured": False, "samples": 0, "unanswered": unanswered,
                "maxBehind": None, "maxAt": None, "maxPartition": None,
                "maxSource": None, "endBehind": {},
                "why": "follower.csv carried no answered sample"}
    worst = max(rows, key=lambda r: r[3])
    best = (worst[0], worst[2], worst[3])
    source = "the largest of the samples written to follower.csv"
    if marks:
        # The mark is what the board showed, taken every tick rather than
        # every write, so it is the one the manifest must state (M4).
        m = max(marks, key=lambda x: x[2])
        if m[2] >= best[2]:
            best, source = m, "the monitor's high-water mark over every tick"
    last_ts = rows[-1][0]
    end = {r[2]: r[3] for r in rows if r[0] == last_ts}
    return {"measured": True, "samples": len(rows), "unanswered": unanswered,
            "maxBehind": best[2], "maxAt": best[0], "maxPartition": best[1],
            "maxSource": source, "endBehind": end, "why": None}


def verdict(report, follower, validators, definition=None):
    return {"follower": follower,
            "validators": validators,
            "partitions": sorted(report.identities.get(follower, {})) or None,
            "roots": compare_roots(report, follower, validators),
            "committee": committee_check(report, follower, validators),
            "certificates": certificate_check(report, follower, validators),
            "drops": drop_check(report, follower, validators),
            "definition": definition_check(definition,
                                           report.key_prefix(follower))}


ABSENT = "— not measured"


def _rb(pair):
    """A (root, bpt) pair as a reader reads it. Both are the first four bytes
    in hex, as the conductor logs them."""
    root, bpt = pair
    return "root %s / bpt %s" % (root or "?", bpt or "?")


def _n(v, measured=True):
    return str(v) if measured and v is not None else ABSENT


def _no_lines(r):
    """The reason the two anchor-line counts read ABSENT, or nothing."""
    if r.get("anchorLines"):
        return ""
    return (" (the follower logged no anchor line of either kind, so nothing "
            "was counted)")


def rows(v):
    """The manifest's Result rows, as (name, value) — each naming the quantity
    it counts, not the method that read it."""
    r, c, ct, d, df = (v["roots"], v["committee"], v["certificates"],
                       v["drops"], v["definition"])
    first = r["firstMismatch"]
    out = [
        ("follower", "`%s`, partitions %s"
         % (v["follower"], ", ".join(v.get("partitions") or []) or ABSENT)),
        ("follower key in the NetworkDefinition",
         (df["followerKeyForm"] or ABSENT)
         + ("" if df["followerKeyForm"] else " (%s)" % (df["why"] or ""))),
        ("active validators per partition, from the NetworkDefinition",
         ", ".join("%s %d" % kv for kv in sorted(df["active"].items()))
         if df["measured"] else ABSENT),
        ("committee size per partition (validators, at genesis)",
         ", ".join("%s %d" % kv for kv in sorted(c["sizes"].items()))
         if c["measured"] else ABSENT + " (%s)" % c["why"]),
        ("committees that disagreed across nodes (#)",
         _n(len(c["disagree"]), c["measured"])),
        ("follower in no committee",
         "yes" if c["followerExcluded"] else
         ("NO" if c["followerExcluded"] is False else ABSENT)),
        ("validators added to a committee during the run (#)",
         _n(len(c["addedDuringRun"]), c["measured"])),
        # A count of anchor lines is a real count whenever the follower wrote
        # one of either kind — `Sending an anchor` (a validator) or
        # `Anchor not sent` (a node in no committee, #4367). When it wrote
        # NEITHER, nothing was read, and printing 0 dispatched would be a
        # clean bill for a follower the reader never saw: the log cut before
        # its first block, or the name in run.json not matching the
        # container. 0 there is a read counter, not a result.
        ("anchors the follower dispatched (#4367: must be 0) (#)",
         _n(r.get("dispatched"), bool(r.get("anchorLines"))) + _no_lines(r)),
        ("blocks the follower stated a root for without sending (#)",
         _n(r.get("stated"), bool(r.get("anchorLines"))) + _no_lines(r)),
        ("anchored blocks compared, follower vs a validator (#)",
         _n(r["compared"], r["measured"])
         + ("" if r["measured"] else " (%s)" % r["why"])),
        ("root/BPT mismatches (#)", _n(len(r["mismatches"]), r["measured"])),
        ("first mismatching block",
         "%s block %d: follower %s, `%s` %s" % (
             first[0], first[1], _rb(first[2]), first[4], _rb(first[3]))
         if first else ("none" if r["measured"] else ABSENT)),
        ("blocks the follower anchored that no validator had (#)",
         _n(r.get("uncompared"), r["measured"])),
        ("anchor lines carrying no source partition (#4370) (#)",
         _n(r.get("sourceless"))),
        # Only meaningful where something was attributed: with no source on
        # the lines nothing was filed, so no contradiction could be seen and
        # 0 would claim a check nobody made (N2, REPORTING-SPEC 1).
        ("blocks where the follower contradicted itself (#)",
         _n(len(r.get("conflicts") or []), r["measured"])),
        ("certificates refused for a non-committee author (#)",
         _n(ct["nonCommitteeAuthor"])),
        ("...of those, authored by the follower (#)", _n(ct["byFollower"])),
        ("headers dropped by validators for a non-committee author (#)",
         _n(d["headerDrops"], d["measured"])
         + ("" if d["measured"] else " (%s)" % d["why"])),
        ("votes dropped by validators for a non-committee author (#)",
         _n(d["voteDrops"], d["measured"])),
    ]
    b = v.get("behind")
    if b is not None:
        out += [
            ("follower behind the validators (blocks, max over the run)",
             ("%d, at %s on %s — %s"
              % (b["maxBehind"], b["maxAt"], b["maxPartition"], b["maxSource"]))
             if b["measured"] else ABSENT + " (%s)" % b["why"]),
            ("follower behind the validators (blocks, at the last sample)",
             ", ".join("%s %d" % kv for kv in sorted(b["endBehind"].items()))
             if b["measured"] else ABSENT),
            ("samples where the follower did not answer (#)",
             _n(b["unanswered"])),
        ]
    return out


def render(v):
    lines = ["# Follower (#4365) — gate 0", "",
             "One node with a validator's wiring and a key in no committee, "
             "launched with the network. What the run's log and its "
             "NetworkDefinition capture say about it.", "",
             "| what | value |", "|---|---|"]
    for k, val in rows(v):
        lines.append("| %s | %s |" % (k, val))
    r = v["roots"]
    if r["mismatches"]:
        lines += ["", "## Every mismatching block", "",
                  "| partition | block | follower root/bpt | validator | its root/bpt |",
                  "|---|---|---|---|---|"]
        for part, blk, mine, ref, who in r["mismatches"][:50]:
            lines.append("| %s | %d | %s | `%s` | %s |"
                         % (part, blk, _rb(mine), who, _rb(ref)))
    ct = v["certificates"]
    if ct["byValidator"]:
        lines += ["", "## Certificates refused, per validator", ""]
        for node, n in sorted(ct["byValidator"].items()):
            lines.append("- `%s`: %d" % (node, n))
    return "\n".join(lines) + "\n"


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("log")
    ap.add_argument("--follower", default="acc-bvn3-fol1")
    ap.add_argument("--validators", default="",
                    help="comma-separated; default is every other acc-* node "
                         "the log mentions")
    ap.add_argument("--definition", help="network-status capture (JSON)")
    ap.add_argument("--behind", help="follower.csv from this run")
    ap.add_argument("--rows", action="store_true",
                    help="print only the manifest's Result rows")
    a = ap.parse_args(argv)
    with open(a.log, errors="replace") as f:
        report = read(f)
    vals = [v for v in a.validators.split(",") if v]
    if not vals:
        seen = set(report.identities) | set(report.anchors)
        vals = sorted(n for n in seen if n != a.follower)
    definition = None
    if a.definition:
        try:
            with open(a.definition) as f:
                definition = json.load(f)
        except Exception:
            definition = None
    v = verdict(report, a.follower, vals, definition)
    if a.behind:
        try:
            with open(a.behind) as f:
                v["behind"] = behind_summary(f, a.follower)
        except Exception:
            v["behind"] = behind_summary([])
    if a.rows:
        for k, val in rows(v):
            sys.stdout.write("| %s | %s |\n" % (k, val))
        return 0
    sys.stdout.write(render(v))
    return 0


if __name__ == "__main__":
    sys.exit(main())
