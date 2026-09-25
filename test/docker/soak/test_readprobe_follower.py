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
import csv
import io
import json
import os
import sys
import tempfile
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
        real, real_csv = readprobe.query, readprobe.FOL_CSV
        readprobe.query = lambda scope, q, url=None: (seen.append(url), ({}, 1.0, None))[1]
        readprobe.FOL_CSV = os.path.join(tempfile.mkdtemp(), "readprobe-follower.csv")
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
            readprobe.query, readprobe.FOL_CSV = real, real_csv
        self.assertEqual(["http://127.0.0.1:26692/v3"] * 2, seen)


class FollowerCSV(unittest.TestCase):
    """readprobe-follower.csv: what the follower answered, per round and
    partition, and which service answered it — the add-follower verdict
    reads the NotReady rows before ACTIVE out of it (#4364)."""

    def test_a_not_ready_answer_is_its_own_outcome(self):
        body = json.dumps({"jsonrpc": "2.0", "id": 1, "error": {
            "code": -33504, "message": "not ready"}}).encode()
        real = readprobe.urllib.request.urlopen
        readprobe.urllib.request.urlopen = lambda req, timeout=None: io.BytesIO(body)
        try:
            r, _, why = readprobe.query("acc://dn.acme/ledger", {}, url="http://x/v3")
        finally:
            readprobe.urllib.request.urlopen = real
        self.assertIsNone(r)
        self.assertEqual(readprobe.WHY_NOT_READY, why)
        self.assertEqual(1, readprobe.judge_follower_round(
            [(False, 1.0, why)])["refused"])

    def test_one_row_per_partition_and_outcome(self):
        path = os.path.join(tempfile.mkdtemp(), "readprobe-follower.csv")
        readprobe.write_follower_rows(path, "acc-bvn3-fol2", {
            ("BVN3", "not-ready"): 3, ("Directory", "answered"): 2}, now=0)
        readprobe.write_follower_rows(path, "acc-bvn3-fol2", {}, now=30)
        with open(path) as f:
            rows = list(csv.DictReader(f))
        self.assertEqual(
            [("1970-01-01T00:00:00Z", "acc-bvn3-fol2", "BVN3", "query", "not-ready", "3"),
             ("1970-01-01T00:00:00Z", "acc-bvn3-fol2", "Directory", "query", "answered", "2")],
            [(r["time"], r["follower"], r["partition"], r["service"],
              r["outcome"], r["reads"]) for r in rows])


class TheValidatorsProbeIsUnchanged(unittest.TestCase):
    def test_the_rotation_is_over_validators_only(self):
        import topology
        self.assertEqual(topology.validator_ports(), readprobe.PORTS)
        for p in topology.follower_ports():
            self.assertNotIn(p, readprobe.PORTS)


if __name__ == "__main__":
    unittest.main()
