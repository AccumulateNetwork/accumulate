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


class BatchLifecycleTest(unittest.TestCase):
    """The numbers that separate an idle network from a wedged one, and that
    keep the #4125 re-delivery skip from hiding the bug it works around."""

    def test_counters_sum_across_the_fleet(self):
        per = {
            "acc-bvn1-val1": [
                ("accumulate_dagbft_blocks_produced_total", "", 100),
                ("accumulate_dagbft_certificates_redelivered_total", "", 1),
            ],
            "acc-bvn1-val2": [
                ("accumulate_dagbft_blocks_produced_total", "", 98),
                ("accumulate_dagbft_certificates_redelivered_total", "", 2),
            ],
        }
        life = soakmon.life_from(per)
        self.assertEqual(198, life["blocks"])
        self.assertEqual(3, life["redelivered"],
                         "a re-delivery on any node is worth seeing")

    def test_idle_network_is_visible_as_empty_blocks(self):
        """The reading that cost three misdiagnoses in one night: block
        production never stopped, the ledger index never moved."""
        per = {"n": [
            ("accumulate_dagbft_blocks_produced_total", "", 2000),
            ("accumulate_dagbft_blocks_empty_total", "", 2000),
        ]}
        life = soakmon.life_from(per)
        self.assertEqual(life["blocks"], life["blocksEmpty"],
                         "every block empty means idle, not wedged")

    def test_waits_are_broken_down_by_reason(self):
        per = {"n": [
            ("accumulate_dagbft_batch_waits_total", 'reason="pruned"', 4),
            ("accumulate_dagbft_batch_waits_total", 'reason="no_record"', 7),
        ], "m": [
            ("accumulate_dagbft_batch_waits_total", 'reason="pruned"', 1),
        ]}
        life = soakmon.life_from(per)
        self.assertEqual({"pruned": 5, "no_record": 7}, life["waitsByReason"],
                         "pruned and no-record are different bugs (#4125 vs #4128)")

    def test_retention_hits_and_expiries_are_both_reported(self):
        """Sizing needs both: hits without expiries means the window is never
        tested, expiries without hits means it is too generous."""
        per = {"n": [
            ("accumulate_dagbft_batch_retention_hits_total", "", 12),
            ("accumulate_dagbft_batches_retention_expired_total", "", 400),
            ("accumulate_dagbft_batches_retained", "", 88),
        ]}
        life = soakmon.life_from(per)
        self.assertEqual(12, life["retentionHits"])
        self.assertEqual(400, life["retentionExpired"])
        self.assertEqual(88, life["retained"])

    def test_garbage_values_do_not_break_the_dashboard(self):
        per = {"n": [
            ("accumulate_dagbft_blocks_produced_total", "", "NaN-ish"),
            ("accumulate_dagbft_blocks_produced_total", "", 5),
            ("unrelated_metric", "", 1),
        ]}
        self.assertEqual(5, soakmon.life_from(per)["blocks"])

    def test_no_scrape_is_zero_not_an_error(self):
        self.assertEqual(0, soakmon.life_from({})["blocks"])
        self.assertEqual({}, soakmon.life_from(None)["waitsByReason"])


class ExecBaselineTests(unittest.TestCase):
    """#4169 step 0: the gates for sharded execution and two-round staging
    are ratios of node counters summed over the fleet."""

    def test_sums_counters_across_nodes(self):
        per = {
            "acc-bvn1-val1": [
                ("accumulate_exec_phase_seconds_total", {"phase": "serial"}, 3.0),
                ("accumulate_exec_phase_seconds_total", {"phase": "parallel"}, 1.0),
                ("accumulate_exec_blocks_total", {}, 10),
                ("accumulate_exec_flushes_total", {}, 25),
                ("accumulate_exec_synthetic_anchor_total", {"applied": "this_block"}, 2),
                ("accumulate_exec_synthetic_anchor_total", {"applied": "earlier"}, 40),
            ],
            "acc-bvn1-val2": [
                ("accumulate_exec_phase_seconds_total", {"phase": "serial"}, 1.0),
                ("accumulate_exec_phase_seconds_total", {"phase": "parallel"}, 3.0),
                ("accumulate_exec_blocks_total", {}, 10),
                ("accumulate_exec_flushes_total", {}, 5),
                ("accumulate_exec_synthetic_anchor_total", {"applied": "missing"}, 8),
            ],
        }
        ex = soakmon.exec_from(per)
        self.assertEqual(4.0, ex["serialSec"])
        self.assertEqual(4.0, ex["parallelSec"])
        self.assertEqual(20, ex["blocks"])
        self.assertEqual(30, ex["flushes"])
        self.assertEqual((2, 40, 8), (ex["anchorThisBlock"], ex["anchorEarlier"], ex["anchorMissing"]))

    def test_absent_metrics_are_zero_not_a_crash(self):
        """An older node exports none of these; the collector must not die
        (#4093 — soakmon rendered absent metrics as 0 and froze once)."""
        self.assertEqual(0, soakmon.exec_from({})["blocks"])
        self.assertEqual(0.0, soakmon.exec_from(None)["serialSec"])
        per = {"n": [("accumulate_exec_phase_seconds_total", "raw-string-label", "nan?")]}
        self.assertEqual(0.0, soakmon.exec_from(per)["serialSec"])


class DiskTests(unittest.TestCase):
    def test_sums_both_engines_and_reports_growth(self):
        first = {}
        d = soakmon.disk_from({"acc-bvn1-val1": {"dn": 1024 * 100, "bvn": 1024 * 300},
                               "acc-bvn2-val1": {"dn": 1024 * 100, "bvn": 1024 * 500}}, first, 1000.0)
        self.assertEqual(400, d["byNode"]["acc-bvn1-val1"]["dnMiB"] + d["byNode"]["acc-bvn1-val1"]["bvnMiB"])
        self.assertEqual(500, d["avgMiB"]); self.assertEqual(600, d["maxMiB"]); self.assertEqual("acc-bvn2-val1", d["maxNode"])
        self.assertIsNone(d["growthMiBPerHour"], "no growth until a second sample two minutes on")
        d = soakmon.disk_from({"acc-bvn1-val1": {"dn": 1024 * 100, "bvn": 1024 * 400},
                               "acc-bvn2-val1": {"dn": 1024 * 100, "bvn": 1024 * 600}}, first, 1000.0 + 3600)
        self.assertEqual(100.0, d["growthMiBPerHour"])

    def test_empty_is_zero_not_a_crash(self):
        self.assertEqual(0, soakmon.disk_from({}, {}, 0)["avgMiB"])
