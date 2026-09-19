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

1. **Does it compute the same state?** Every node's conductor logs
   ``Sending an anchor`` per block at Info, with ``root`` and ``bpt``
   (conductor.go:308) — the same line `reading-a-run.md` uses for a
   divergence. Compare the follower's against any validator's, per
   (partition, block).
2. **Is its key in any committee?** Each engine logs the committee it built
   at Info (``Extracted initial validators for DAG-BFT``, dagbft.go:427) and
   every later change (``Validator added``, certificate_handler.go:379).
3. **Did any validator ever see a certificate it authored?**
   ``Invalid certificate`` is Info deliberately (#4054) and
   certificate.go:170 refuses ``header author is not in committee``.
4. **Which form of "in no committee" actually ran?** The NetworkDefinition
   says whether the follower's key is in it and inactive, or not in it at
   all. The manifest has to record which.

**Never compare without the partition, and the destination is not the
partition.** Every container runs a BVN engine and a Directory engine, both
reach the same block numbers at the same second, and the log line carries a
destination but not its source. The obvious rule — "a line addressed to
dn.acme came from this node's BVN" — is WRONG, and reading it that way is
H1: `conductor.go:283-288` iterates `Network.Partitions`, which
`init.go:203` seeds with the Directory, so **the Directory anchors to
itself as well as to every BVN**. On run 20260917T212457Z, acc-bvn1-val1
logged 1156 lines to each BVN and 2316 to dn — 1156 Directory
self-anchors plus ~1160 BVN1 anchors — and at block 500 it logged dn.acme
twice, eight seconds apart, with different roots. Filing both under the
node's BVN, last write wins, reports a mismatch on a block two nodes agree
on, or agreement on a block where they differ, depending on log order.

The rule used here: **only the Directory anchors to a BVN**, so every
`(root, bpt)` a node addressed to a BVN — anywhere in the log — is one its
Directory engine computed. A `dn.acme` line carrying one of those values is
that engine's copy to itself; a `dn.acme` line carrying a value the node
never sent to a BVN is its own BVN engine's. The set is taken over the
node's whole log rather than over the one block, because the two engines
need not be at the same block number at the moment either of them anchors —
`reading-a-run.md` says they usually are, and "usually" is not a rule to
file evidence by.

A `dn.acme` line from a node that never addressed a BVN at all — a log
fragment that lost the Directory engine's lines — is **ambiguous**: nothing
in it separates the two engines. Those are counted and not compared, never
guessed, and the count is a row. The values are `(root, bpt)` together,
eight bytes, so two engines of one node colliding on both is not a case
worth designing for; the conflict detector below would catch its
consequence anyway.

A `partition` attribute on that log line would make all of this
unnecessary; that is Go, and it is the lead's.

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
    """Yield (node, ts, event, fields) for every line this reader knows."""
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


def _is_dn_destination(dest):
    d = (dest or "").lower()
    return "//dn." in d or d.endswith("//dn.acme")


def _is_bvn_destination(dest):
    return "bvn-" in (dest or "").lower()


AMBIGUOUS = object()


def _source_of(dest, own_bvn, dn_values, value):
    """The partition that PRODUCED this anchor line. See the module docstring.

    `dn_values` is every (root, bpt) this node addressed to a BVN anywhere in
    the log — its Directory engine's, because only the Directory anchors to a
    BVN. Returns a partition id, None (a destination this reader does not
    understand), or AMBIGUOUS.
    """
    if _is_bvn_destination(dest):
        return "Directory"
    if not _is_dn_destination(dest):
        return None
    if value in dn_values:
        return "Directory"          # the Directory's copy to itself
    if dn_values:
        return own_bvn              # this node's BVN engine
    return AMBIGUOUS                # this node never anchored to a BVN


class Report:
    """Everything the log said, indexed by what the gate asks of it."""

    def __init__(self):
        self.identities = {}        # node -> {partition: 16-hex key}
        self.committees = {}        # (node, partition) -> size
        self.anchors = {}           # node -> {(partition, block): (root, bpt)}
        self.invalid = []           # (node, ts, author, error)
        self.changes = []           # (node, ts, kind, pubkey)
        self.drops = {"header": {}, "vote": {}}   # kind -> {node: count}
        self.sawDropLine = False
        # dn.acme lines at a block where the node logged no bvn-* line: the
        # two engines cannot be told apart there, so they are counted and
        # not filed (H1).
        self.ambiguous = {}         # node -> count
        # One (node, partition, block) given two different values. Log order
        # used to decide this silently; it is a finding.
        self.conflicts = []         # (node, partition, block, first, second)

    # -- identity -------------------------------------------------------
    def key_prefix(self, node):
        """The eight hex characters that `author=` and `pubkey=` carry.

        `hexEncode` (vote_handler.go:552) is the first 8 characters of
        `HeaderDigest(key).String()`, and HeaderDigest is a cast of the
        32-byte public key, not a hash of it — so it is the key's own hex,
        and the 16 characters a node logs for itself share that prefix.
        """
        ids = self.identities.get(node)
        if not ids:
            return None
        return sorted(ids.values())[0][:8]

    def bvn_of(self, node):
        """The node's own BVN, taken from what it logged rather than from its
        container name."""
        for p in self.identities.get(node, {}):
            if p != "Directory":
                return p
        m = re.match(r"acc-(bvn\d+)-", node or "")
        return m.group(1).upper() if m else None

    def anchor_blocks(self, node):
        return list(self.anchors.get(node, {}))


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
    for node, ts, ev, f in parse(lines):
        if ev == "anchor":
            pending.append((node, f))
            continue
        if ev == "identity":
            part = f.get("partition")
            key = f.get("validatorKey")
            if part and key:
                r.identities.setdefault(node, {})[part] = key
        elif ev == "committee":
            n = _int(f, "validators")
            part = f.get("partition")
            if part and n is not None:
                r.committees[(node, part)] = n
        elif ev in ("validator-added", "validator-removed"):
            r.changes.append((node, ts, ev.split("-")[1], f.get("pubkey")))
        elif ev == "invalid-cert":
            r.invalid.append((node, ts, f.get("author"), f.get("error", "")))
        elif ev == "header-drop":
            r.sawDropLine = True
            r.drops["header"][node] = r.drops["header"].get(node, 0) + 1
        elif ev == "vote-drop":
            r.sawDropLine = True
            r.drops["vote"][node] = r.drops["vote"].get(node, 0) + 1
    # Anchors are resolved after the pass — they need each node's own BVN,
    # from its identity line, AND every line of their own (node, block)
    # group, because which engine logged a dn.acme line is decided by the
    # bvn-* lines beside it (H1). The input is still read exactly once.
    # What each node's DIRECTORY engine computed, over the whole log: every
    # value the node addressed to a BVN. Only the Directory anchors to a BVN.
    dn_values = {}
    for node, f in pending:
        if _is_bvn_destination(f.get("destination")):
            dn_values.setdefault(node, set()).add((f.get("root"), f.get("bpt")))

    groups = OrderedDict()
    for node, f in pending:
        blk = _int(f, "block")
        if blk is None:
            continue
        groups.setdefault((node, blk), []).append(f)

    for (node, blk), lines in groups.items():
        own = r.bvn_of(node)
        mine = dn_values.get(node, set())
        for f in lines:
            value = (f.get("root"), f.get("bpt"))
            part = _source_of(f.get("destination"), own, mine, value)
            if part is AMBIGUOUS:
                r.ambiguous[node] = r.ambiguous.get(node, 0) + 1
                continue
            if part is None:
                continue
            slot = r.anchors.setdefault(node, {})
            prev = slot.get((part, blk))
            if prev is not None and prev != value:
                # The Directory's four copies are identical by construction,
                # so this is the node contradicting itself — not something
                # log order should quietly resolve.
                r.conflicts.append((node, part, blk, prev, value))
                continue
            slot[(part, blk)] = value
    return r


def compare_roots(report, follower, validators):
    """The follower's state root and BPT root against the validators', per
    (partition, block).

    Only blocks BOTH sides anchored are compared. A block only the follower
    reached — it was a block ahead when the log was cut — is counted as
    uncompared, not as a mismatch; a follower silent on every block is
    `— not measured`, and the fallback is the v3 API on the follower and a
    validator at the same ledger index.
    """
    mine = report.anchors.get(follower, {})
    if not mine:
        return {"measured": False, "compared": 0, "uncompared": 0,
                "mismatches": [], "firstMismatch": None,
                "ambiguous": report.ambiguous.get(follower, 0),
                "conflicts": [c for c in report.conflicts if c[0] == follower],
                "why": "the follower logged no anchor line — fall back to the "
                       "v3 API on the follower and a validator at the same "
                       "ledger index (query with includeReceipt)"}
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
            "ambiguous": report.ambiguous.get(follower, 0),
            "conflicts": [c for c in report.conflicts if c[0] == follower],
            "why": None}


def committee_check(report, follower, validators):
    """The committee every engine built, and every change to one during the run.

    The follower is excluded when: every node that reported a size for a
    partition reported the SAME size, and the follower's key was never added
    to a committee. Disagreeing sizes are a finding in their own right — two
    nodes running different committees is the divergence gate 0 would
    otherwise be measured against.
    """
    sizes, disagree = {}, {}
    for (node, part), n in report.committees.items():
        seen = sizes.setdefault(part, n)
        if seen != n:
            disagree.setdefault(part, set()).add(seen)
            disagree[part].add(n)
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
    for node, _, _, _ in non_committee:
        by_val[node] = by_val.get(node, 0) + 1
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
        for node, n in report.drops[kind].items():
            by.setdefault(node, {})[kind] = n
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


def behind_summary(lines):
    """The run's `behind` series, out of follower.csv.

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
        ("anchor lines whose engine could not be determined (#)",
         _n(r.get("ambiguous"))),
        ("blocks where the follower contradicted itself (#)",
         _n(len(r.get("conflicts") or []))),
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
                v["behind"] = behind_summary(f)
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
