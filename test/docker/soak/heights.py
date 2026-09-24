#!/usr/bin/env python3
"""A partition's height, and each node's own, from accumulate_node_executed_block.

    heights.py header FOLLOWERS    the columns monitor.csv carries after `time`
    heights.py row FOLLOWERS DIR HEALS CPU FOLHEALS
                                   one monitor.csv row body, from DIR/<container>.prom

FOLLOWERS is soak.sh's FOL_LIST — the followers this run has, comma separated,
or `-` — so a declared follower the run never starts has no column (#4389).

**Why not the ledger of one node (#4404).** monitor.csv's `dnHeight` was the
Directory ledger's `index` as answered by host port 26680 — one node,
acc-bvn1-val1. On run 20260924T052134Z that node was the one chaos restarted
first, and the column sat at 207 from 05:26:19 to 05:27:24 while every other
Directory node went 215 -> 323: the restarted node's own stall, printed as the
network's (REPORTING-SPEC 1b: one node is one point of stale truth).

**The partition's height is the HIGHEST block any of its validators that
answered in the sample has executed** — `dnHeightMax` for the Directory —
with the number that answered beside it (`dnValidatorsAnswered`). An
executor cannot pass what its partition certified, so the highest answer is
where the partition is; stuck nodes cannot drag it down, however many of
them there are. A majority of the ANSWERING set could: on a 4-validator BVN
with one node stuck at 214, one paused and one missing one scrape, the
answers are [1000, 214], their "majority" is 214, and the stuck node read
caught up on its own height (review F2 on #4404). The max can fall only when
the leading validators all miss a sample, which is why the count that
answered is recorded with it. Followers are not in it: a follower is expected
to lag, and the height its lag is measured against is the validators'
(#4365). A node executing a divergent fork past its partition would raise
it; that node is caught by anchor agreement, not by height.

**Each node's own height, in its own column** (#4345): `exec.<container>.<partition>`,
the block THAT node's executor last executed for that partition. A node that
did not answer the scrape at that sample has an EMPTY cell, never 0: 0 would
say "executed nothing", which is a different fact (REPORTING-SPEC 1).

The gauge is a position, not a count (nodestate/metrics.go): on an idle
network it runs ahead of the ledger index, because an empty block is executed
and never written. It is the number that tells a wedged node from a healthy
one, which is what these columns are for.
"""
import os
import re
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.dirname(HERE))
import topology  # noqa: E402

EXECUTED = "accumulate_node_executed_block"
_LINE = re.compile(r'^' + EXECUTED + r'\{([^}]*)\}\s+([-0-9.eE+]+)\s*$')


def canon_part(p):
    """A partition as accumulate_node_state spells it: lower case, the
    Directory by name. The exec_/dagbft_ families spell it "BVN1",
    "Directory"; one spelling here keeps one node-and-partition one key."""
    p = (p or "").strip().lower()
    return {"dn": "directory"}.get(p, p.replace("bvn-", ""))


def executed_from_prom(text):
    """{partition: executed block} out of one node's /metrics text."""
    out = {}
    for line in (text or "").splitlines():
        m = _LINE.match(line.strip())
        if not m:
            continue
        lab = dict(re.findall(r'(\w+)="([^"]*)"', m.group(1)))
        try:
            out[canon_part(lab.get("partition"))] = int(float(m.group(2)))
        except ValueError:
            continue
    return out


def executed_from_rows(rows):
    """The same, out of soakmon's parsed scrape: [(name, labels, value)]."""
    out = {}
    for name, lab, v in rows or ():
        if name != EXECUTED:
            continue
        try:
            out[canon_part((lab or {}).get("partition"))] = int(float(v))
        except (TypeError, ValueError, AttributeError):
            continue
    return out


def partition_heights(executed, validators):
    """{partition: {"height", "answered"}} over the VALIDATORS that answered.

    `executed` is {container: {partition: block}} for this sample;
    `validators` the containers that count. `height` is the highest block
    any of them executed, `answered` how many reported the partition. A
    partition no validator reported is absent."""
    by = {}
    for c in validators:
        for p, b in (executed.get(c) or {}).items():
            by.setdefault(p, []).append(b)
    return {p: {"height": max(v), "answered": len(v)} for p, v in by.items()}


def columns(followers=(), records=None):
    """(container, partition) for every node the run has, in declaration
    order: the validators, then the named followers."""
    if records is None:
        fol = set(followers or ())
        records = topology.validator_records() + [
            r for r in topology.followers() if r["container"] in fol]
    return [(r["container"], canon_part(p)) for r in records for p in r["partitions"]]


def _fol_arg(s):
    return [x for x in (s or "").split(",") if x and x != "-"]


def header(cols):
    return ",".join(["dnHeightMax", "heals", "cpuPct", "followerHeals", "dnValidatorsAnswered"]
                    + ["exec.%s.%s" % c for c in cols])


def row(executed, validators, heals, cpu, fol_heals, cols):
    """monitor.csv's row after `time`: the Directory's height (the highest
    block any answering validator executed), the heal and CPU columns as
    before, how many Directory validators answered, then each node's own
    executed block."""
    dn = partition_heights(executed, validators).get("directory") or {}
    fmt = lambda v: "" if v is None else str(v)
    return ",".join([fmt(dn.get("height")), heals, cpu, fol_heals, str(dn.get("answered", 0))]
                    + [fmt((executed.get(c) or {}).get(p)) for c, p in cols])


def read_dir(d):
    """{container: {partition: block}} from d/<container>.prom files."""
    out = {}
    for name in sorted(os.listdir(d)):
        if name.endswith(".prom"):
            with open(os.path.join(d, name), errors="replace") as f:
                out[name[:-5]] = executed_from_prom(f.read())
    return out


def main(argv):
    if len(argv) == 2 and argv[0] == "header":
        print(header(columns(_fol_arg(argv[1]))))
        return 0
    if len(argv) == 6 and argv[0] == "row":
        executed = read_dir(argv[2])
        vals = [r["container"] for r in topology.validator_records()]
        print(row(executed, vals, argv[3], argv[4], argv[5], columns(_fol_arg(argv[1]))))
        return 0
    sys.stderr.write(__doc__)
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
