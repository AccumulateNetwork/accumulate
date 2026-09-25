#!/usr/bin/env python3
"""A declared follower that is not running is not a follower of the run (#4389).

docker-network.yml declares acc-bvn3-fol2 so that init writes its key and
directory; the compose puts it in the late-follower profile, so `compose up`
never starts it and only the add-follower disturbance does. Every consumer of
`topology.followers()` counted it anyway. Run 20260924T052134Z, which never
added it, reported:

- 2 followers and 14 nodes in run.json and the manifest's topology row;
- 1,206 of 1,206 read-probe reads failed against a container that did not exist;
- follower.csv rows for it in every sample, which `behind_summary` counted
  against acc-bvn3-fol1 as "did not answer".

Each consumer is driven here with that declared-but-not-running follower.
"""
import os
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
sys.path.insert(0, os.path.dirname(HERE))
import followerlog
import readprobe
import soakmon
import topology

FOL1 = {"container": "acc-bvn3-fol1", "port": 26692, "dir": "bvn3-5",
        "bvn": "BVN3", "partitions": ["Directory", "BVN3"]}
FOL2 = {"container": "acc-bvn3-fol2", "port": 26693, "dir": "bvn3-6",
        "bvn": "BVN3", "partitions": ["Directory", "BVN3"]}
ADD = "2026-09-20T01:00:00Z add-follower acc-bvn3-fol2 (key in no committee; databases cleared)\n"
REMOVE = "2026-09-20T01:05:00Z remove-follower acc-bvn3-fol2\n"


class TheTopologyNamesTheFollowersARunHas(unittest.TestCase):
    """Against the committed docker-network.yml and docker-compose.yml."""

    def test_declared_is_four_started_is_one(self):
        self.assertEqual(["acc-bvn1-fol1", "acc-bvn2-fol1", "acc-bvn3-fol1", "acc-bvn3-fol2"],
                         [f["container"] for f in topology.followers()])
        self.assertEqual(["acc-bvn3-fol1"],
                         [f["container"] for f in topology.started_followers()])
        self.assertEqual(["acc-bvn1-fol1", "acc-bvn2-fol1", "acc-bvn3-fol2"],
                         [f["container"] for f in topology.late_followers()])

    def test_a_run_without_the_add_follower_walk_has_one_follower(self):
        fols, late = topology.run_followers(chaos_followers=False)
        self.assertEqual(["acc-bvn3-fol1"], [f["container"] for f in fols])
        self.assertEqual([], late)
        self.assertEqual(13, len(topology.validator_records()) + len(fols),
                         "run.json's `nodes` is the nodes the run has")

    def test_a_run_with_the_walk_names_the_late_ones_as_late(self):
        fols, late = topology.run_followers(chaos_followers=True)
        self.assertEqual(["acc-bvn3-fol1", "acc-bvn1-fol1", "acc-bvn2-fol1", "acc-bvn3-fol2"],
                         [f["container"] for f in fols])
        self.assertEqual(["acc-bvn1-fol1", "acc-bvn2-fol1", "acc-bvn3-fol2"],
                         [f["container"] for f in late])

    def test_running_is_started_plus_a_late_one_between_add_and_remove(self):
        up = lambda lines: [f["container"] for f in topology.running_followers(lines)]
        self.assertEqual(["acc-bvn3-fol1"], up([]), "never added: not running")
        self.assertEqual(["acc-bvn3-fol1", "acc-bvn3-fol2"], up([ADD]))
        self.assertEqual(["acc-bvn3-fol1"], up([ADD, REMOVE]), "removed: not running")

    def test_the_manifest_counts_what_the_run_has(self):
        """soak.sh's topology block, run as soak.sh runs it."""
        with open(os.path.join(HERE, "soak.sh")) as f:
            src = f.read()
        self.assertIn("topology.run_followers(", src)
        self.assertNotIn("f = topology.followers()", src,
                         "the manifest must not count declared followers")


class TheMonitorWritesNoRowsForIt(unittest.TestCase):
    def setUp(self):
        self._f, self._r = soakmon.FOLLOWERS, soakmon._read_ledger_index
        self._c, self._l = soakmon.CHAOS, soakmon._LATE
        soakmon.FOLLOWERS = [FOL1, FOL2]
        soakmon._LATE = {"acc-bvn3-fol2"}
        soakmon.CHAOS = os.path.join(tempfile.mkdtemp(), "chaos.log")
        soakmon._FOLLOWER_WORST.clear()
        soakmon._read_ledger_index = lambda port, part: 499

    def tearDown(self):
        soakmon.FOLLOWERS, soakmon._read_ledger_index = self._f, self._r
        soakmon.CHAOS, soakmon._LATE = self._c, self._l
        soakmon._FOLLOWER_WORST.clear()

    def rows(self):
        v = soakmon.collect_follower({"Directory": 500, "BVN3": 500}, now=1.0)
        return soakmon.follower_csv_rows(v, "t0")

    def test_never_added_writes_only_fol1(self):
        # The issue's in-process reproduction wrote four rows, two of them
        # `t0,acc-bvn3-fol2,BVN3,,500,,`.
        self.assertEqual(["t0,acc-bvn3-fol1,BVN3,499,500,1,1",
                          "t0,acc-bvn3-fol1,Directory,499,500,1,1"], self.rows())

    def test_added_it_has_rows(self):
        with open(soakmon.CHAOS, "w") as f:
            f.write(ADD)
        self.assertEqual(4, len(self.rows()))

    def test_removed_it_has_none(self):
        with open(soakmon.CHAOS, "w") as f:
            f.write(ADD + REMOVE)
        self.assertEqual(2, len(self.rows()))


class TheReportIsAboutOneFollower(unittest.TestCase):
    """#4389's reproduction: fol1 steady one behind; fol2 waiting two
    samples, then added at height 3 and 497 then 300 behind."""

    CSV = ["time,follower,partition,followerHeight,validatorsMaxHeight,behindBlocks,maxBehindRunBlocks",
           "t0,acc-bvn3-fol1,BVN3,499,500,1,1",
           "t0,acc-bvn3-fol2,BVN3,,500,,",
           "t1,acc-bvn3-fol1,BVN3,499,500,1,1",
           "t1,acc-bvn3-fol2,BVN3,,500,,",
           "t2,acc-bvn3-fol1,BVN3,499,500,1,1",
           "t2,acc-bvn3-fol2,BVN3,3,500,497,497",
           "t3,acc-bvn3-fol1,BVN3,599,600,1,1",
           "t3,acc-bvn3-fol2,BVN3,300,600,300,497"]

    def test_fol1s_summary_is_fol1s(self):
        b = followerlog.behind_summary(self.CSV, "acc-bvn3-fol1")
        self.assertEqual(0, b["unanswered"])
        self.assertEqual(1, b["maxBehind"])
        self.assertEqual({"BVN3": 1}, b["endBehind"])

    def test_before_the_fix_this_read_497(self):
        """The unfiltered reading, kept to show what the filter removes."""
        b = followerlog.behind_summary(self.CSV)
        self.assertEqual(497, b["maxBehind"])
        self.assertEqual(2, b["unanswered"])

    def test_the_command_line_filters_by_its_follower(self):
        d = tempfile.mkdtemp()
        csvp, logp = os.path.join(d, "follower.csv"), os.path.join(d, "log")
        with open(csvp, "w") as f:
            f.write("\n".join(self.CSV) + "\n")
        open(logp, "w").close()
        import io, contextlib
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            followerlog.main([logp, "--follower", "acc-bvn3-fol1", "--behind", csvp, "--rows"])
        row = [l for l in out.getvalue().splitlines() if "max over the run" in l]
        self.assertTrue(row, out.getvalue())
        self.assertTrue(row[0].rstrip(" |").split("| ")[-1].startswith("1,"), row[0])


class TheProbeDoesNotAskIt(unittest.TestCase):
    def setUp(self):
        self._saved = (readprobe.FOLLOWERS, readprobe.CHAOS, readprobe.query,
                       readprobe.FOL_CSV)
        d = tempfile.mkdtemp()
        readprobe.FOLLOWERS = [FOL1, FOL2]
        readprobe.CHAOS = os.path.join(d, "chaos.log")
        readprobe.FOL_CSV = os.path.join(d, "readprobe-follower.csv")
        readprobe.REPORT_SAVED = readprobe.REPORT
        readprobe.REPORT = os.path.join(d, "readprobe-report.md")
        self.asked = []
        readprobe.query = lambda scope, q, url=None: (self.asked.append(url), ({}, 1.0, None))[1]

    def tearDown(self):
        (readprobe.FOLLOWERS, readprobe.CHAOS, readprobe.query,
         readprobe.FOL_CSV) = self._saved
        readprobe.REPORT = readprobe.REPORT_SAVED

    RES = [{"partition": "BVN3", "index": 2, "scope": "acc://bvn-BVN3.acme/ledger"}]

    def test_never_added_it_is_not_asked_counted_or_reported_as_failing(self):
        pr = readprobe.Probe()
        pr.run_follower_round(self.RES)
        self.assertEqual(["http://127.0.0.1:26692/v3"], self.asked)
        self.assertEqual(["acc-bvn3-fol1"], sorted(pr.fol_reads))
        import io, contextlib
        with contextlib.redirect_stdout(io.StringIO()):
            pr.report()
        with open(readprobe.REPORT) as f:
            text = f.read()
        self.assertIn("acc-bvn3-fol2**: declared (late-follower profile) and not running", text)
        self.assertNotIn("acc-bvn3-fol2** (partitions", text)

    def test_added_it_is_asked(self):
        with open(readprobe.CHAOS, "w") as f:
            f.write(ADD)
        readprobe.Probe().run_follower_round(self.RES)
        self.assertEqual(["http://127.0.0.1:26692/v3", "http://127.0.0.1:26693/v3"],
                         self.asked)


if __name__ == "__main__":
    unittest.main()
