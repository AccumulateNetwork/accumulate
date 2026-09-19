#!/usr/bin/env python3
"""The read-back probe asks the follower too (#4365).

"It answers requests" is one of gate 0's four conditions (executor spec,
"Sync" step 6: BOOTING and ACTIVE refuse with NotReady, COMPLETE serves; a
node launched from genesis is COMPLETE at once). Assumed, it is worth
nothing; the two things that would make the measurement worthless are (a)
reading through the router, so another node answers and the follower scores
a clean sheet without being asked, and (b) reporting "0 failed" for a
follower nobody asked anything.
"""
import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import readprobe

FOL = {"container": "acc-bvn3-fol1", "port": 26692,
       "partitions": ["Directory", "BVN3"]}

RES = [{"partition": "Directory", "index": 1, "scope": "acc://dn.acme/ledger"},
       {"partition": "BVN3", "index": 2, "scope": "acc://bvn-BVN3.acme/ledger"},
       {"partition": "BVN1", "index": 3, "scope": "acc://bvn-BVN1.acme/ledger"},
       {"partition": "BVN2", "index": 4, "scope": "acc://bvn-BVN2.acme/ledger"}]


class Targets(unittest.TestCase):
    def test_only_the_partitions_the_follower_runs_are_asked_for(self):
        got = readprobe.follower_targets(RES, FOL)
        self.assertEqual([("Directory", 1), ("BVN3", 2)],
                         [(s["partition"], s["index"]) for s in got])

    def test_a_reservoir_with_nothing_it_holds_yields_nothing(self):
        self.assertEqual([], readprobe.follower_targets(
            [{"partition": "BVN1", "index": 3, "scope": "x"}], FOL))


class Round(unittest.TestCase):
    def test_no_reads_is_absent_not_a_pass(self):
        v = readprobe.judge_follower_round([])
        self.assertFalse(v["measured"])
        self.assertIn("no sampled entry", v["why"])

    def test_answers_refusals_and_failures_are_three_different_facts(self):
        v = readprobe.judge_follower_round([
            (True, 3.0, None),
            (True, 5.0, None),
            (False, 1.0, readprobe.WHY_GATED),
            (False, 9.0, readprobe.WHY_TIMEOUT),
        ])
        self.assertTrue(v["measured"])
        self.assertEqual(4, v["reads"])
        self.assertEqual(2, v["answered"])
        self.assertEqual(1, v["refused"], "NotReady is a result, not a gap")
        self.assertEqual(1, v["failed"])

    def test_the_follower_is_read_from_its_own_port(self):
        """A read through the router proves a peer answered, not this node."""
        seen = []
        real = readprobe.query
        readprobe.query = lambda scope, q, url=None: (seen.append(url), ({}, 1.0, None))[1]
        try:
            pr = readprobe.Probe()
            pr.fol_rounds = {FOL["container"]: []}
            pr.fol_reads = {FOL["container"]: []}
            old = readprobe.FOLLOWERS
            readprobe.FOLLOWERS = [FOL]
            try:
                pr.run_follower_round(RES)
            finally:
                readprobe.FOLLOWERS = old
        finally:
            readprobe.query = real
        self.assertEqual(["http://127.0.0.1:26692/v3"] * 2, seen)


class TheValidatorsProbeIsUnchanged(unittest.TestCase):
    def test_the_rotation_is_over_validators_only(self):
        import topology
        self.assertEqual(topology.validator_ports(), readprobe.PORTS)
        self.assertNotIn(26692, readprobe.PORTS)


if __name__ == "__main__":
    unittest.main()
