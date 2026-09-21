#!/usr/bin/env python3
"""The stranded series, read directly (#4364).

`runseries.py` exists because the manifest's stranded row and its
per-disturbance step table are readings of the SAME series, and each was
separately wrong in a way that cancelled a loss into silence. Building it
in one place is only worth something if that place is tested as a place —
through `soak.sh` as well (test_manifest_rows), but here first, because a
test that can only reach the logic through a shell heredoc tests the
heredoc.
"""
import os
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import runseries

HEADER = ("time,node,role,partition,accepted,rejected,certified,relayedTaken,"
          "relayedRefused,relayedNotReady,relayedUnreachable,"
          "acceptedNeitherCertifiedTakenNorRefused,sample")


class Series(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp(prefix="runseries-")
        self.path = os.path.join(self.dir, "submissions.csv")

    def write(self, *rows):
        with open(self.path, "w") as f:
            f.write(HEADER + "\n")
            for r in rows:
                f.write(r + "\n")

    def row(self, t, node, part, accepted, stranded, sample="periodic"):
        return ("%s,%s,follower,%s,%s,,,%s,,,,%s,%s"
                % (t, node, part,
                   "" if accepted is None else accepted,
                   "" if accepted is None else accepted,
                   "" if stranded is None else stranded, sample))

    def load(self):
        return runseries.load(self.path, "follower")


class CounterResets(Series):
    """Prometheus counters are process-local. A restarted or re-added node
    starts again at 0, its pre-restart cumulative loss leaves the series,
    and the floor steps DOWN — masking every later loss of that size, and
    satisfying "the figure does not climb" by construction at every
    re-add. #4364's own run removes and re-adds the follower."""

    def test_a_decrease_in_accepted_is_a_reset(self):
        self.write(self.row("2026-09-20T01:00:00Z", "n", "BVN3", 100, 4),
                   self.row("2026-09-20T01:00:30Z", "n", "BVN3", 110, 4),
                   self.row("2026-09-20T01:01:00Z", "n", "BVN3", 5, 0))
        s = self.load()
        self.assertEqual([("2026-09-20T01:01:00Z", "n", "BVN3")], s["resets"])

    def test_what_it_had_stranded_is_carried_forward(self):
        self.write(self.row("2026-09-20T01:00:00Z", "n", "BVN3", 100, 4),
                   self.row("2026-09-20T01:00:30Z", "n", "BVN3", 5, 0),
                   self.row("2026-09-20T01:01:00Z", "n", "BVN3", 12, 2))
        got = [x["total"] for x in runseries.complete(self.load())]
        self.assertEqual([4, 4, 6], got,
                         "the floor holds at 4 and the later loss of 2 shows")

    def test_without_it_the_floor_steps_down_and_masks_the_loss(self):
        """What the uncorrected series would read, so the test says what
        the correction is worth: 4 -> 0 -> 2, which never climbs back to
        4, so nothing reads as a climb on a run that lost two."""
        self.write(self.row("2026-09-20T01:00:00Z", "n", "BVN3", 100, 4),
                   self.row("2026-09-20T01:00:30Z", "n", "BVN3", 5, 0),
                   self.row("2026-09-20T01:01:00Z", "n", "BVN3", 12, 2))
        raw = [4, 0, 2]
        self.assertLess(raw[1], raw[0], "the floor steps down at the re-add")
        self.assertLess(raw[2], raw[0], "and the later loss never reaches it")

    def test_a_counter_that_rises_is_not_a_reset(self):
        self.write(self.row("2026-09-20T01:00:00Z", "n", "BVN3", 100, 4),
                   self.row("2026-09-20T01:00:30Z", "n", "BVN3", 101, 5))
        self.assertEqual([], self.load()["resets"])

    def test_each_pair_resets_on_its_own(self):
        """A restart is one node's, not the fleet's."""
        self.write(self.row("2026-09-20T01:00:00Z", "a", "BVN3", 100, 4),
                   self.row("2026-09-20T01:00:00Z", "b", "BVN3", 100, 3),
                   self.row("2026-09-20T01:00:30Z", "a", "BVN3", 5, 0),
                   self.row("2026-09-20T01:00:30Z", "b", "BVN3", 110, 3))
        s = self.load()
        self.assertEqual(1, len(s["resets"]))
        self.assertEqual("a", s["resets"][0][1])
        self.assertEqual([7, 7], [x["total"] for x in runseries.complete(s)])

    def test_two_resets_on_one_pair_both_carry(self):
        self.write(self.row("2026-09-20T01:00:00Z", "n", "BVN3", 100, 4),
                   self.row("2026-09-20T01:00:30Z", "n", "BVN3", 5, 3),
                   self.row("2026-09-20T01:01:00Z", "n", "BVN3", 2, 1))
        got = [x["total"] for x in runseries.complete(self.load())]
        self.assertEqual([4, 7, 8], got)
        self.assertEqual(2, len(self.load()["resets"]))

    def test_the_manifest_row_names_when_and_where(self):
        self.write(self.row("2026-09-20T01:00:00Z", "n", "BVN3", 100, 4),
                   self.row("2026-09-20T01:20:00Z", "n", "BVN3", 5, 0))
        line = runseries.resets_row(self.load())
        self.assertIn("1 (n/BVN3 at 01:20Z)", line)
        self.assertIn("does not step down", line)

    def test_no_resets_is_no_row(self):
        self.write(self.row("2026-09-20T01:00:00Z", "n", "BVN3", 100, 4))
        self.assertIsNone(runseries.resets_row(self.load()))


class Completeness(Series):
    def test_a_sample_missing_a_pair_is_not_complete(self):
        self.write(self.row("2026-09-20T01:00:00Z", "a", "BVN3", 10, 1),
                   self.row("2026-09-20T01:00:00Z", "b", "BVN3", 10, 2),
                   self.row("2026-09-20T01:00:30Z", "a", "BVN3", 11, 1))
        s = self.load()
        self.assertEqual(1, s["dropped"])
        self.assertEqual([3], [x["total"] for x in runseries.complete(s)])

    def test_a_blank_count_is_the_same_as_a_missing_row(self):
        self.write(self.row("2026-09-20T01:00:00Z", "a", "BVN3", 10, 1),
                   self.row("2026-09-20T01:00:00Z", "b", "BVN3", 10, 2),
                   self.row("2026-09-20T01:00:30Z", "a", "BVN3", 11, 1),
                   self.row("2026-09-20T01:00:30Z", "b", "BVN3", None, None))
        self.assertEqual(1, self.load()["dropped"])

    def test_the_forced_final_row_does_not_double_a_counter(self):
        """It can share a second with a periodic row, and two readings of
        one counter are one reading."""
        self.write(self.row("2026-09-20T01:00:00Z", "a", "BVN3", 10, 3),
                   self.row("2026-09-20T01:00:00Z", "a", "BVN3", 10, 3,
                            sample="final"))
        self.assertEqual([3], [x["total"]
                               for x in runseries.complete(self.load())])


class Floors(Series):
    def test_a_level_is_a_minimum_and_never_one_reading(self):
        pts = [(0.0, 7), (30.0, 0), (60.0, 4), (90.0, 0)]
        self.assertEqual((0, 0.0), runseries.floor_of(pts, 0, 120))

    def test_first_n_reads_the_samples_nearest_the_disturbance(self):
        """An after-floor takes the first two complete samples from the
        settle, not everything up to the next disturbance — otherwise a
        loss late in the stretch would be read as the disturbance's."""
        pts = [(0.0, 9), (30.0, 9), (60.0, 0), (90.0, 0)]
        self.assertEqual((9, 0.0), runseries.floor_of(pts, 0, 120, first_n=2))

    def test_an_empty_range_is_none_and_not_zero(self):
        self.assertEqual((None, None), runseries.floor_of([(0.0, 3)], 10, 20))

    def test_it_says_when_the_first_point_used_was(self):
        """The step row reports how late an after-floor was taken."""
        pts = [(200.0, 5), (230.0, 5)]
        self.assertEqual((5, 200.0), runseries.floor_of(pts, 60, 400,
                                                        first_n=2))


class MissingFile(Series):
    def test_no_file_is_an_error_not_an_empty_series(self):
        s = runseries.load(os.path.join(self.dir, "nope.csv"), "follower")
        self.assertTrue(s["error"])
        self.assertEqual([], s["samples"])

    def test_a_header_with_no_rows_is_an_error_too(self):
        self.write()
        self.assertTrue(self.load()["error"])

    def test_a_role_nobody_matched_is_an_error(self):
        self.write(self.row("2026-09-20T01:00:00Z", "a", "BVN3", 10, 1))
        self.assertTrue(runseries.load(self.path, "validator")["error"])


if __name__ == "__main__":
    unittest.main()
