#!/usr/bin/env python3
"""The flow matrix reads every node and keeps the max per field, and a value
that goes backwards after that is an alarm, not lag (REPORTING-SPEC 1b)."""
import os, sys, unittest
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import soakmon


class MergeSequenceViews(unittest.TestCase):
    def test_a_lagging_node_cannot_lower_the_reading(self):
        # Node A is current, node B's executor is behind: B under-reports
        # produced. Run 20260906T134054Z read only B and showed the
        # destination having received more than the source produced.
        a = [{"url": "acc://dn.acme", "produced": 102200, "received": 0, "delivered": 0}]
        b = [{"url": "acc://dn.acme", "produced": 100804, "received": 0, "delivered": 0}]
        m = soakmon.merge_sequence_views([b, a])
        self.assertEqual(len(m), 1)
        self.assertEqual(m[0]["produced"], 102200)
        self.assertEqual(m[0]["url"], "acc://dn.acme")

    def test_fields_merge_independently_and_pending_keeps_the_longest(self):
        a = [{"url": "acc://bvn-BVN1.acme", "produced": 5, "received": 9, "delivered": 7, "pending": [1]}]
        b = [{"url": "acc://bvn-BVN1.acme", "produced": 6, "received": 8, "delivered": 8, "pending": [1, 2]}]
        m = soakmon.merge_sequence_views([a, b])[0]
        self.assertEqual((m["produced"], m["received"], m["delivered"]), (6, 9, 8))
        self.assertEqual(m["pending"], [1, 2])

    def test_a_node_that_did_not_answer_is_not_a_zero(self):
        a = [{"url": "acc://dn.acme", "produced": 3}]
        m = soakmon.merge_sequence_views([None, a, []])
        self.assertEqual(m[0]["produced"], 3)


class SequenceRegression(unittest.TestCase):
    def test_a_lower_reading_is_named(self):
        prev = (0, 100, 50, 40)   # (t, sent, recv, deliv)
        self.assertEqual(soakmon.sequence_regressions(prev, (1, 100, 50, 40)), [])
        self.assertEqual(soakmon.sequence_regressions(prev, (1, 120, 60, 45)), [])
        self.assertEqual(soakmon.sequence_regressions(prev, (1, 99, 50, 40)), [("sent", 100, 99)])
        self.assertEqual(soakmon.sequence_regressions(prev, (1, 100, 49, 39)),
                         [("recv", 50, 49), ("deliv", 40, 39)])


class RegressionGuard(unittest.TestCase):
    """A merged max that fell because a node stopped answering is not a
    sequence number going backwards, and must not raise the alarm reserved
    for one (#4279 review)."""

    def test_fewer_nodes_withholds_the_alarm(self):
        prev, cur = (0, 100, 50, 40), (1, 90, 50, 40)
        self.assertEqual(soakmon.judge_regression(prev, cur, 8, 7), ([], True))

    def test_the_same_or_more_nodes_raises_it(self):
        prev, cur = (0, 100, 50, 40), (1, 90, 50, 40)
        self.assertEqual(soakmon.judge_regression(prev, cur, 8, 8), ([("sent", 100, 90)], False))
        self.assertEqual(soakmon.judge_regression(prev, cur, 7, 8), ([("sent", 100, 90)], False))

    def test_the_first_sample_and_an_unknown_count_judge_nothing(self):
        self.assertEqual(soakmon.judge_regression(None, (1, 5, 5, 5), None, 8), ([], False))
        self.assertEqual(soakmon.judge_regression((0, 9, 9, 9), (1, 5, 5, 5), None, 8), ([("sent", 9, 5), ("recv", 9, 5), ("deliv", 9, 5)], False))


if __name__ == "__main__":
    unittest.main()


class RunLongRates(unittest.TestCase):
    """A window derivative says what the network is doing now; the run-long
    average says what the run achieved, which is the figure a result is
    quoted as. The two are stated separately, with what each is measured
    over, so neither can be read as the other."""

    def setUp(self):
        soakmon._RATE_BASE.clear()

    def test_totals_are_the_sum_and_say_what_they_are_over(self):
        # First tick establishes the base for the produced counters.
        soakmon.observe_rates(1000.0, 0, 100, 10, 0.0)
        r = soakmon.observe_rates(1100.0, 50000, 20100, 2010, 100.0)
        self.assertEqual(500.0, r["userAvg"], "generated over the loadgen's own clock")
        self.assertEqual(200.0, r["synAvg"], "produced counted from the monitor's first look")
        self.assertEqual(20.0, r["anchorAvg"])
        self.assertEqual(720.0, r["totalAvg"], "total is what the network processes")
        self.assertEqual(100.0, r["userOverSec"])
        self.assertEqual(100.0, r["producedOverSec"])

    def test_the_two_clocks_are_not_conflated(self):
        # A monitor that restarts mid-run counts produced from its own
        # start, while the loadgen's elapsed keeps running: the spans
        # differ and both are reported.
        soakmon.observe_rates(5000.0, 0, 1000, 100, 3600.0)
        r = soakmon.observe_rates(5100.0, 1800000, 3000, 300, 3700.0)
        self.assertEqual(3700.0, r["userOverSec"])
        self.assertEqual(100.0, r["producedOverSec"])
        self.assertEqual(20.0, r["synAvg"], "2000 produced over the 100s it watched")

    def test_nothing_yet_is_not_zero(self):
        r = soakmon.observe_rates(1000.0, 0, 0, 0, 0.0)
        self.assertIsNone(r["userAvg"], "no elapsed time is not a rate of zero")
        self.assertIsNone(r["totalAvg"])
