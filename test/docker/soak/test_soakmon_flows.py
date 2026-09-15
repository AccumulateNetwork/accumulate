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
