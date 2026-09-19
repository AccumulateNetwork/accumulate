#!/usr/bin/env python3
"""The follower row group (#4365): how far behind the validators it is, live.

`collect_heights` reads every partition's ledger index from several nodes and
keeps the MAX (REPORTING-SPEC 1b), because a halted node can only
under-report. A follower is expected to lag slightly, so it is invisible in
that max — and `behind`, computed against a max that included it, would be
zero by construction. That is the reason this is its own reading and not a
thirteenth probe port.

Three things have gone wrong on this dashboard before and must not here: an
absent instrument rendering as 0 (REPORTING-SPEC 1), an impossible state —
a negative lag — rendering as a number (1a), and a count with no stated
window (Paul's rule: name the quantity and the window).
"""
import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import soakmon


class Behind(unittest.TestCase):
    def test_an_ordinary_lag(self):
        v = soakmon.judge_behind(998, 1000)
        self.assertTrue(v["measured"])
        self.assertEqual(2, v["behind"])
        self.assertEqual(0, v["ahead"])

    def test_caught_up(self):
        v = soakmon.judge_behind(1000, 1000)
        self.assertEqual(0, v["behind"])

    def test_a_follower_ahead_is_skew_not_a_negative_lag(self):
        """Two HTTP reads a moment apart, and the validators' max taken from
        nodes that had not yet closed the block the follower had. It is skew,
        exactly as a flow cell's is (judge_gap), and never a negative depth."""
        v = soakmon.judge_behind(1001, 1000)
        self.assertEqual(0, v["behind"])
        self.assertEqual(1, v["ahead"])

    def test_a_follower_that_did_not_answer_is_not_zero_behind(self):
        v = soakmon.judge_behind(None, 1000)
        self.assertFalse(v["measured"])
        self.assertIsNone(v.get("behind"))
        self.assertIn("did not answer", v["why"])

    def test_an_unreadable_network_height_is_not_a_follower_fault(self):
        v = soakmon.judge_behind(998, None)
        self.assertFalse(v["measured"])
        self.assertIn("validators", v["why"])

    def test_the_bound_is_stated_and_used(self):
        """"Within a block or two" has to be a number the board can colour on."""
        self.assertEqual(2, soakmon.BEHIND_BOUND)
        self.assertFalse(soakmon.judge_behind(998, 1000)["over"])
        self.assertTrue(soakmon.judge_behind(990, 1000)["over"])


class Collect(unittest.TestCase):
    """`collect_follower` turns the readings into the row group, and keeps the
    worst reading of the run so the manifest can state it."""

    def setUp(self):
        self._f, self._r = soakmon.FOLLOWERS, soakmon._read_ledger_index
        soakmon.FOLLOWERS = [{"container": "acc-bvn3-fol1", "port": 26692,
                              "dir": "bvn3-5", "bvn": "BVN3",
                              "partitions": ["Directory", "BVN3"]}]
        soakmon._FOLLOWER_WORST.clear()

    def tearDown(self):
        soakmon.FOLLOWERS, soakmon._read_ledger_index = self._f, self._r
        soakmon._FOLLOWER_WORST.clear()

    def test_it_reports_per_partition_against_the_validators_max(self):
        soakmon._read_ledger_index = lambda port, part: {
            ("Directory"): 498, ("BVN3"): 499}[part]
        v = soakmon.collect_follower({"Directory": 500, "BVN3": 500,
                                      "BVN1": 500, "BVN2": 500}, now=10.0)
        self.assertTrue(v["measured"])
        n = v["nodes"]["acc-bvn3-fol1"]
        self.assertEqual(2, n["partitions"]["Directory"]["behind"])
        self.assertEqual(1, n["partitions"]["BVN3"]["behind"])
        self.assertEqual(2, n["worstBehind"])
        self.assertNotIn("BVN1", n["partitions"],
                         "a follower is only asked about the partitions it runs")

    def test_the_worst_reading_of_the_run_is_kept_with_when(self):
        soakmon._read_ledger_index = lambda port, part: 400
        soakmon.collect_follower({"Directory": 405, "BVN3": 402}, now=10.0)
        soakmon._read_ledger_index = lambda port, part: 500
        v = soakmon.collect_follower({"Directory": 501, "BVN3": 501}, now=99.0)
        n = v["nodes"]["acc-bvn3-fol1"]
        self.assertEqual(1, n["worstBehind"], "the reading now")
        self.assertEqual(5, n["maxBehindRun"], "the worst of the run")
        self.assertEqual(10.0, n["maxBehindAt"])

    def test_a_follower_that_answers_nothing_is_absent_not_caught_up(self):
        soakmon._read_ledger_index = lambda port, part: None
        v = soakmon.collect_follower({"Directory": 500, "BVN3": 500}, now=10.0)
        n = v["nodes"]["acc-bvn3-fol1"]
        self.assertIsNone(n["worstBehind"])
        self.assertFalse(n["partitions"]["Directory"]["measured"])
        self.assertIsNone(n["maxBehindRun"])

    def test_no_follower_in_the_topology_is_not_a_broken_instrument(self):
        soakmon.FOLLOWERS = []
        v = soakmon.collect_follower({"Directory": 500}, now=10.0)
        self.assertFalse(v["measured"])
        self.assertEqual({}, v["nodes"])
        self.assertIn("no follower", v["why"])


class CsvRows(unittest.TestCase):
    def setUp(self):
        self._f, self._r = soakmon.FOLLOWERS, soakmon._read_ledger_index
        soakmon.FOLLOWERS = [{"container": "acc-bvn3-fol1", "port": 26692,
                              "dir": "bvn3-5", "bvn": "BVN3",
                              "partitions": ["Directory", "BVN3"]}]
        soakmon._FOLLOWER_WORST.clear()

    def tearDown(self):
        soakmon.FOLLOWERS, soakmon._read_ledger_index = self._f, self._r
        soakmon._FOLLOWER_WORST.clear()

    def test_one_row_per_partition_per_sample(self):
        soakmon._read_ledger_index = lambda port, part: 498
        v = soakmon.collect_follower({"Directory": 500, "BVN3": 499}, now=10.0)
        rows = soakmon.follower_csv_rows(v, "2026-09-19T18:00:00Z")
        self.assertEqual(2, len(rows))
        self.assertEqual(["2026-09-19T18:00:00Z,acc-bvn3-fol1,BVN3,498,499,1,1",
                          "2026-09-19T18:00:00Z,acc-bvn3-fol1,Directory,498,500,2,2"],
                         rows)

    def test_an_unanswered_partition_writes_a_blank_not_a_zero(self):
        soakmon._read_ledger_index = lambda port, part: None
        v = soakmon.collect_follower({"Directory": 500, "BVN3": 499}, now=10.0)
        rows = soakmon.follower_csv_rows(v, "2026-09-19T18:00:00Z")
        self.assertEqual("2026-09-19T18:00:00Z,acc-bvn3-fol1,Directory,,500,,",
                         rows[1], "an empty field, never a 0")

    def test_the_high_water_mark_rides_along_in_every_row(self):
        """M4: the board's `fmax` is a high-water mark over every tick
        (I_HEIGHT = 1 s); the CSV is written every I_MEM = 30 s. A one-tick
        excursion past the bound — what a five-minute gate exists to catch —
        was red on the board and absent from the manifest, under the same
        label. The mark travels in the row, so the two cannot disagree."""
        soakmon._read_ledger_index = lambda port, part: 480
        soakmon.collect_follower({"Directory": 500, "BVN3": 500}, now=10.0)
        soakmon._read_ledger_index = lambda port, part: 600
        v = soakmon.collect_follower({"Directory": 600, "BVN3": 600}, now=40.0)
        rows = soakmon.follower_csv_rows(v, "2026-09-19T18:00:30Z")
        # behind now is 0; the run's worst was 20 and the row still says so.
        self.assertEqual("2026-09-19T18:00:30Z,acc-bvn3-fol1,BVN3,600,600,0,20",
                         rows[0])


class TheRowGroupIsOnTheBoard(unittest.TestCase):
    """The dashboard's own text, checked as text: the panel exists, the labels
    name the quantity and the window, and nothing says "not measured" as 0."""

    with open(os.path.join(os.path.dirname(os.path.abspath(__file__)),
                           "soakmon.py")) as _fh:
        PAGE = _fh.read()
    del _fh

    def test_the_panel_and_its_ids_exist(self):
        for tag in ("<h4>follower", "id=fbehind", "id=fheight", "id=fmax",
                    "id=fres"):
            self.assertIn(tag, self.PAGE, tag)

    def test_the_labels_name_the_quantity_and_the_window(self):
        self.assertIn("behind (blocks, now)", self.PAGE)
        self.assertIn("behind (blocks, whole run)", self.PAGE)

    def test_every_follower_id_has_a_tooltip(self):
        for tag in ("fbehind", "fheight", "fmax", "fstate", "fres"):
            self.assertIn(" %s:\"" % tag, self.PAGE,
                          "%s has no TIPS entry — undefined is the only "
                          "unacceptable state" % tag)


if __name__ == "__main__":
    unittest.main()
