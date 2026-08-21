# Copyright 2026 The Accumulate Authors
#
# Use of this source code is governed by an MIT-style
# license that can be found in the LICENSE file or at
# https://opensource.org/licenses/MIT.
"""Liveness verdict tests for the soak monitor.

The dashboard reported "network up" over a Directory frozen at block 121 with
zero anchors, zero synthetics and zero tx/s, because the indicator asked
whether the API answered rather than whether anything progressed.
"""

import unittest

import soakmon


class ProgressTest(unittest.TestCase):
    def setUp(self):
        soakmon._PROGRESS.clear()

    def test_frozen_height_is_stalled_not_up(self):
        """The reported case: heights readable, none of them moving."""
        frozen = {"Directory": 121, "BVN1": 41, "BVN2": 29, "BVN3": 49}
        pg = soakmon.assess_progress(frozen, 1000.0)
        self.assertEqual("up", soakmon.overall_status(True, pg),
                         "the first reading starts the clock, it cannot condemn")

        # Same heights, past the stall window.
        pg = soakmon.assess_progress(frozen, 1000.0 + soakmon.STALL_SECS + 1)
        self.assertEqual("stalled", soakmon.overall_status(True, pg))
        for part in frozen:
            self.assertEqual("stalled", pg[part]["state"], part)

    def test_advancing_network_is_up(self):
        for i in range(5):
            t = 1000.0 + i * 3
            pg = soakmon.assess_progress(
                {"Directory": 121 + i, "BVN1": 41 + i,
                 "BVN2": 29 + i, "BVN3": 49 + i}, t)
        self.assertEqual("up", soakmon.overall_status(True, pg))
        self.assertEqual("live", pg["Directory"]["state"])

    def test_one_stalled_partition_condemns_the_network(self):
        base = {"Directory": 121, "BVN1": 41, "BVN2": 29, "BVN3": 49}
        soakmon.assess_progress(base, 1000.0)
        moved = dict(base)
        for i in range(1, 4):
            moved = {"Directory": 121, "BVN1": 41 + i,
                     "BVN2": 29 + i, "BVN3": 49 + i}
            pg = soakmon.assess_progress(moved, 1000.0 + i * 5)
        # BVN1..3 advanced; the Directory never moved.
        self.assertEqual("stalled", pg["Directory"]["state"])
        self.assertEqual("live", pg["BVN1"]["state"])
        self.assertEqual("stalled", soakmon.overall_status(True, pg))

    def test_unreadable_heights_are_not_health(self):
        pg = soakmon.assess_progress(
            {"Directory": None, "BVN1": None, "BVN2": None, "BVN3": None}, 1000.0)
        self.assertEqual("down", soakmon.overall_status(True, pg),
                         "heights we cannot read must never read as up")

    def test_partially_unreadable_is_degraded(self):
        for i in range(3):
            pg = soakmon.assess_progress(
                {"Directory": 121 + i, "BVN1": 41 + i,
                 "BVN2": 29 + i, "BVN3": None}, 1000.0 + i * 2)
        self.assertEqual("unknown", pg["BVN3"]["state"])
        self.assertEqual("degraded", soakmon.overall_status(True, pg))

    def test_api_down_beats_everything(self):
        pg = soakmon.assess_progress({"Directory": 121}, 1000.0)
        self.assertEqual("down", soakmon.overall_status(False, pg))

    def test_stall_clock_reports_elapsed_time(self):
        soakmon.assess_progress({"Directory": 121}, 1000.0)
        pg = soakmon.assess_progress({"Directory": 121}, 1042.0)
        self.assertEqual(42.0, pg["Directory"]["stalledFor"])

    def test_recovery_clears_the_stall(self):
        soakmon.assess_progress({"Directory": 121}, 1000.0)
        pg = soakmon.assess_progress({"Directory": 121}, 1030.0)
        self.assertEqual("stalled", pg["Directory"]["state"])
        pg = soakmon.assess_progress({"Directory": 122}, 1031.0)
        self.assertEqual("live", pg["Directory"]["state"])
        self.assertEqual(0.0, pg["Directory"]["stalledFor"])


if __name__ == "__main__":
    unittest.main()
