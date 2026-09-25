#!/usr/bin/env python3
"""The manifest's verdict per add-follower and per remove-follower (#4364,
item 3 and the measurement contract).

The chaos walk already writes what each add and removal did into
`chaos.log`, and soakmon writes the counters beside it. What the manifest
lacks is the verdict the run-analyst reads: one line per add-follower and one
per remove-follower, each from the captured files of the run, never from a
live network.

Per add-follower:
  - time from container start to ACTIVE (`chaos.log`, `nodestate.csv`);
  - blocks behind at hand-off, the follower's `behindBlocks` in
    `follower.csv` at the sample when it went ACTIVE;
  - its first root match (`chaos.log`);
  - NotReady observed on a READ before ACTIVE, naming the service that
    answered (`readprobe-follower.csv`: time, follower, partition, service,
    outcome, written by the read probe for the follower it asks);
  - stranded 0 AND relayed-taken about equal to accepted on THAT follower,
    over that container's life only (`submissions.csv`).
Per remove-follower: block cadence and every stream's delivered unaffected
across the removal, from the readings either side of it
(`follower-removal-N-{before,at,after}.json`).

A quantity the run did not record reads `not measured`, never a pass: the
second add here never reached ACTIVE and left nothing behind, and its line
must say so rather than borrow the first add's numbers.

Like test_manifest_rows, the function is lifted out of soak.sh and run
rather than copied, and the manifest's own block is checked to call it — a
row nobody prints is not in the manifest.
"""
import calendar
import json
import os
import re
import subprocess
import tempfile
import time
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
SOAK = os.path.join(HERE, "soak.sh")
FN = "follower_verdict_rows"

CHAOS = """\
2026-09-20T00:58:00Z followers: acc-bvn3-fol2 (service acc-bvn3-fol2, data bvn3-6) is added and removed in turn, 2 pair(s) (0: unbounded)
2026-09-20T00:58:00Z sleeping 120s until the next disturbance
2026-09-20T01:00:00Z add-follower acc-bvn3-fol2 (key in no committee; databases cleared)
2026-09-20T01:00:00Z sleeping 270s until the next disturbance
2026-09-20T01:02:17Z follower acc-bvn3-fol2 ACTIVE (BVN3=ACTIVE Directory=ACTIVE), 137s after it was added
2026-09-20T01:02:50Z follower acc-bvn3-fol2 first root match (source=acc-bvn3-val1 block=812 root=9f3c1d0e), 170s after it was added
2026-09-20T01:05:00Z remove-follower acc-bvn3-fol2
2026-09-20T01:05:30Z follower removal 1, 30s either side: unaffected: cadence blocks/s before->after: BVN1 1.00->1.00, BVN2 1.00->1.00, BVN3 1.00->1.00, Directory 1.00->1.00; 2 streams, 2 advancing before and after, 0 quiet before
2026-09-20T01:05:30Z sleeping 270s until the next disturbance
2026-09-20T01:10:00Z add-follower acc-bvn3-fol2 (key in no committee; databases cleared)
2026-09-20T01:10:00Z sleeping 270s until the next disturbance
2026-09-20T01:15:00Z follower acc-bvn3-fol2 NEVER ACTIVE: removed 300s after it was added
2026-09-20T01:15:00Z follower acc-bvn3-fol2 never matched a validator's root before its removal
2026-09-20T01:15:00Z remove-follower acc-bvn3-fol2
2026-09-20T01:15:30Z follower removal 2, 30s either side: AFFECTED: synthetic BVN1->BVN3 delivered stopped at 5210; cadence blocks/s before->after: BVN1 1.00->1.00, BVN2 1.00->1.00, BVN3 1.00->1.00, Directory 1.00->1.00; 2 streams, 1 advancing before and after, 0 quiet before
"""

NODESTATE = """\
time,node,role,partition,containerStarted,state,startToActiveS,kind
2026-09-20T01:02:11Z,acc-bvn3-fol2,follower,Directory,2026-09-20T01:00:00Z,ACTIVE,131,reached
2026-09-20T01:02:17Z,acc-bvn3-fol2,follower,BVN3,2026-09-20T01:00:00Z,ACTIVE,137,reached
2026-09-20T01:20:00Z,acc-bvn3-fol2,follower,BVN3,2026-09-20T01:10:00Z,BOOTING,,final
2026-09-20T01:20:00Z,acc-bvn3-fol2,follower,Directory,2026-09-20T01:10:00Z,BOOTING,,final
"""

# behindBlocks: 311 before the hand-off, 23 at it, 2 after. The gate-0
# follower is caught up the whole time and must not stand in for the added one.
FOLLOWER_CSV = """\
time,follower,partition,followerHeight,validatorsMaxHeight,behindBlocks,maxBehindRunBlocks
2026-09-20T01:01:47Z,acc-bvn3-fol1,BVN3,700,700,0,1
2026-09-20T01:01:47Z,acc-bvn3-fol2,BVN3,100,411,311,311
2026-09-20T01:01:47Z,acc-bvn3-fol2,Directory,90,395,305,305
2026-09-20T01:02:17Z,acc-bvn3-fol1,BVN3,730,730,0,1
2026-09-20T01:02:17Z,acc-bvn3-fol2,BVN3,800,823,23,311
2026-09-20T01:02:17Z,acc-bvn3-fol2,Directory,400,405,5,305
2026-09-20T01:02:47Z,acc-bvn3-fol1,BVN3,760,760,0,1
2026-09-20T01:02:47Z,acc-bvn3-fol2,BVN3,851,853,2,311
2026-09-20T01:02:47Z,acc-bvn3-fol2,Directory,431,432,1,305
"""

SUBMISSIONS = """\
time,node,role,partition,accepted,rejected,certified,relayedTaken,relayedRefused,relayedNotReady,relayedUnreachable,acceptedNeitherCertifiedTakenNorRefused,sample
2026-09-20T01:03:00Z,acc-bvn3-fol1,follower,BVN3,9931,,0,4354,0,0,0,5577,periodic
2026-09-20T01:03:00Z,acc-bvn3-fol2,follower,BVN3,200,,0,199,0,0,0,1,periodic
2026-09-20T01:04:30Z,acc-bvn3-fol1,follower,BVN3,9931,,0,4354,0,0,0,5577,periodic
2026-09-20T01:04:30Z,acc-bvn3-fol2,follower,BVN3,412,,0,410,2,0,0,0,periodic
2026-09-20T01:12:00Z,acc-bvn3-fol1,follower,BVN3,9931,,0,4354,0,0,0,5577,periodic
"""

READPROBE = """\
time,follower,partition,service,outcome
2026-09-20T01:00:40Z,acc-bvn3-fol2,BVN3,query,not-ready
2026-09-20T01:01:40Z,acc-bvn3-fol2,Directory,query,not-ready
2026-09-20T01:03:00Z,acc-bvn3-fol2,BVN3,query,answered
2026-09-20T01:03:00Z,acc-bvn3-fol1,BVN3,query,answered
2026-09-20T01:12:00Z,acc-bvn3-fol1,BVN3,query,answered
"""


def _epoch(s):
    return calendar.timegm(time.strptime(s, "%Y-%m-%dT%H:%M:%SZ"))


def _snap(when, heights, delivered):
    return {"t": _epoch(when), "heights": heights, "delivered": delivered}


# Removal 1: every partition at one block a second either side, both streams
# advancing — unaffected. Removal 2: synthetic BVN1->BVN3 advanced before the
# removal and stopped after it — affected, and the verdict names the stream.
SNAPS = {
    1: [_snap("2026-09-20T01:04:30Z",
              {"Directory": 1000, "BVN1": 2000, "BVN2": 3000, "BVN3": 4000},
              {"synthetic BVN1->BVN3": 5000, "anchor Directory->BVN3": 900}),
        _snap("2026-09-20T01:05:00Z",
              {"Directory": 1030, "BVN1": 2030, "BVN2": 3030, "BVN3": 4030},
              {"synthetic BVN1->BVN3": 5050, "anchor Directory->BVN3": 930}),
        _snap("2026-09-20T01:05:30Z",
              {"Directory": 1060, "BVN1": 2060, "BVN2": 3060, "BVN3": 4060},
              {"synthetic BVN1->BVN3": 5100, "anchor Directory->BVN3": 960})],
    2: [_snap("2026-09-20T01:14:30Z",
              {"Directory": 1600, "BVN1": 2600, "BVN2": 3600, "BVN3": 4600},
              {"synthetic BVN1->BVN3": 5100, "anchor Directory->BVN3": 1500}),
        _snap("2026-09-20T01:15:00Z",
              {"Directory": 1630, "BVN1": 2630, "BVN2": 3630, "BVN3": 4630},
              {"synthetic BVN1->BVN3": 5210, "anchor Directory->BVN3": 1530}),
        _snap("2026-09-20T01:15:30Z",
              {"Directory": 1660, "BVN1": 2660, "BVN2": 3660, "BVN3": 4660},
              {"synthetic BVN1->BVN3": 5210, "anchor Directory->BVN3": 1560})],
}


def _function():
    """follower_verdict_rows, lifted out of soak.sh verbatim: from its
    definition to the first closing brace in column 0."""
    with open(SOAK) as f:
        src = f.read()
    m = re.search(r"^%s\(\) \{.*?\n\}\n" % FN, src, re.S | re.M)
    if not m:
        raise AssertionError(
            "no %s() in soak.sh: the manifest has no verdict per add-follower "
            "and per remove-follower" % FN)
    return m.group(0)


def _manifest_block():
    """The Result block soak.sh appends to the manifest."""
    with open(SOAK) as f:
        src = f.read()
    m = re.search(r'echo "## Result".*?\n\} >> "\$manifest"', src, re.S)
    if not m:
        raise AssertionError("cannot find the manifest's Result block in soak.sh")
    return m.group(0)


class FollowerVerdict(unittest.TestCase):
    def setUp(self):
        self.rd = tempfile.mkdtemp(prefix="followerverdict-")
        files = {"chaos.log": CHAOS, "nodestate.csv": NODESTATE,
                 "follower.csv": FOLLOWER_CSV, "submissions.csv": SUBMISSIONS,
                 "readprobe-follower.csv": READPROBE}
        for name, text in files.items():
            with open(os.path.join(self.rd, name), "w") as f:
                f.write(text)
        for n, (before, at, after) in SNAPS.items():
            for which, snap in (("before", before), ("at", at), ("after", after)):
                with open(os.path.join(self.rd, "follower-removal-%d-%s.json"
                                       % (n, which)), "w") as f:
                    json.dump(snap, f)

    def rows(self):
        script = ('#!/usr/bin/env bash\nrd="$1"\nhere="$2"\n'
                  "STEP_WINDOW_SECS=120\nSTEP_SETTLE_SECS=60\n"
                  "FOLLOWER_WINDOW_SECS=30\n"
                  + _function() + "\n%s\n" % FN)
        path = os.path.join(self.rd, "run.sh")
        with open(path, "w") as f:
            f.write(script)
        out = subprocess.run(["bash", path, self.rd, HERE], capture_output=True,
                             text=True, timeout=60)
        self.assertEqual("", out.stderr.strip(), out.stderr)
        return out.stdout.splitlines()

    def one(self, lines, kind, minute):
        got = [ln for ln in lines if kind in ln and minute in ln]
        self.assertEqual(1, len(got), "want one %s verdict at %s, got:\n%s"
                         % (kind, minute, "\n".join(lines)))
        return got[0]

    def test_each_add_and_remove_gets_a_verdict(self):
        self.assertTrue(FN in _manifest_block(),
                        "the manifest's Result block never calls %s: no verdict "
                        "per add-follower and remove-follower is printed" % FN)
        lines = self.rows()
        self.assertEqual(2, sum(1 for ln in lines if "add-follower" in ln), "\n".join(lines))
        self.assertEqual(2, sum(1 for ln in lines if "remove-follower" in ln), "\n".join(lines))

        # The first add: everything recorded, everything as the contract asks.
        add1 = self.one(lines, "add-follower", "01:00")
        self.assertIn("137", add1, "container start to ACTIVE")
        self.assertRegex(add1, r"\b23\b", "blocks behind at hand-off, the sample at ACTIVE")
        self.assertNotIn("311", add1, "the pre-hand-off lag is not the hand-off")
        self.assertIn("812", add1, "the first root match's block")
        self.assertIn("NotReady", add1, "a read refused before ACTIVE")
        self.assertIn("query", add1, "the service that answered NotReady")
        self.assertIn("412", add1, "accepted on this follower")
        self.assertIn("410", add1, "relayed-taken on this follower")
        self.assertRegex(add1, r"(?i)(stranded\W{0,3}0\b|\b0 stranded)")
        for other in ("9931", "5577"):
            self.assertNotIn(other, add1, "the gate-0 follower's counters are not this one's")
        self.assertNotIn("not measured", add1)
        self.assertNotIn("NEVER", add1)

        # The second add: nothing recorded. Not measured, never a pass, and
        # nothing borrowed from the first container's life.
        add2 = self.one(lines, "add-follower", "01:10")
        self.assertIn("NEVER ACTIVE", add2)
        self.assertIn("not measured", add2)
        for borrowed in ("137", "812", "412", "410"):
            self.assertNotIn(borrowed, add2, "a number from the first add")
        self.assertNotRegex(add2, r"(?i)\bpass(ed)?\b")

        rm1 = self.one(lines, "remove-follower", "01:05")
        self.assertIn("unaffected", rm1)
        self.assertNotIn("AFFECTED", rm1)
        self.assertNotIn("not measured", rm1)

        rm2 = self.one(lines, "remove-follower", "01:15")
        self.assertIn("AFFECTED", rm2)
        self.assertIn("BVN1->BVN3", rm2, "the stream that stopped is named")


# #4438: every late follower is added in one slot and removed together in the
# next. The ACTIVE lines interleave, and one removal (one set of readings)
# covers all of them.
CHAOS_MANY = """\
2026-09-20T01:00:00Z add-follower acc-bvn1-fol1 (bvn1-fol1, data bvn1-5; key in no committee; databases cleared; join-running-network)
2026-09-20T01:00:00Z add-follower acc-bvn2-fol1 (bvn2-fol1, data bvn2-5; key in no committee; databases cleared; join-running-network)
2026-09-20T01:00:01Z add-follower acc-bvn3-fol2 (bvn3-fol2, data bvn3-6; key in no committee; databases cleared; join-running-network)
2026-09-20T01:01:10Z follower acc-bvn2-fol1 ACTIVE (bvn2=ACTIVE directory=ACTIVE), 70s after it was added
2026-09-20T01:01:40Z follower acc-bvn1-fol1 ACTIVE (bvn1=ACTIVE directory=ACTIVE), 100s after it was added
2026-09-20T01:02:00Z follower acc-bvn1-fol1 first in agreement (acc://bvn-BVN1.acme block 640 at 2026-09-20T01:01:58Z; acc://dn.acme block 700 at 2026-09-20T01:01:40Z), 120s after it was added
2026-09-20T01:05:00Z follower acc-bvn3-fol2 NEVER ACTIVE: removed 299s after it was added
2026-09-20T01:05:00Z follower acc-bvn3-fol2 never matched a validator's root before its removal
2026-09-20T01:05:00Z follower acc-bvn2-fol1 never matched a validator's root before its removal
2026-09-20T01:05:00Z remove-follower acc-bvn1-fol1 (removal 1)
2026-09-20T01:05:00Z remove-follower acc-bvn2-fol1 (removal 1)
2026-09-20T01:05:00Z remove-follower acc-bvn3-fol2 (removal 1)
"""


class SeveralLateFollowers(FollowerVerdict):
    def setUp(self):
        super().setUp()
        for name in ("nodestate.csv", "follower.csv", "submissions.csv",
                     "readprobe-follower.csv"):
            os.unlink(os.path.join(self.rd, name))
        with open(os.path.join(self.rd, "chaos.log"), "w") as f:
            f.write(CHAOS_MANY)

    def test_each_add_and_remove_gets_a_verdict(self):
        pass  # the single-follower fixture's assertions; not this one's

    def add_row(self, lines, c):
        got = [ln for ln in lines if "add-follower %s " % c in ln]
        self.assertEqual(1, len(got), "\n".join(lines))
        return got[0]

    def test_one_row_per_follower_and_one_per_removal(self):
        lines = self.rows()
        self.assertEqual(3, sum(1 for ln in lines if "add-follower" in ln), "\n".join(lines))
        rm = [ln for ln in lines if "remove-follower" in ln]
        self.assertEqual(1, len(rm), "one removal for the slot:\n" + "\n".join(lines))
        self.assertIn("acc-bvn1-fol1, acc-bvn2-fol1, acc-bvn3-fol2 (removal 1)", rm[0])
        self.assertIn("unaffected", rm[0])

        b1 = self.add_row(lines, "acc-bvn1-fol1")
        self.assertIn("ACTIVE 100s after the add", b1)
        self.assertIn("block 640", b1)
        b2 = self.add_row(lines, "acc-bvn2-fol1")
        self.assertIn("ACTIVE 70s after the add", b2, "its ACTIVE line came while another life was open")
        self.assertIn("first in agreement not measured", b2)
        self.assertNotIn("640", b2, "a number from another follower")
        b3 = self.add_row(lines, "acc-bvn3-fol2")
        self.assertIn("NEVER ACTIVE", b3)


if __name__ == "__main__":
    unittest.main()
