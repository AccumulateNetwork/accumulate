#!/usr/bin/env python3
"""Healing is read from the families the node exports, and a family no node
reported is None, never 0 (REPORTING-SPEC 1). The rows are the shape
parse_prom yields, with values from run 20260915T042428Z's scrape of
acc-bvn1-val1 -- the run whose board read HEALS 0 beside 18,171 healed
entries."""
import os, sys, unittest
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import soakmon

NODE = [
    ("accumulate_conductor_heal_entries_total", {}, 18171.0),
    ("accumulate_conductor_heal_requests_total", {"destination": "BVN1", "outcome": "answered", "source": "BVN2"}, 68.0),
    ("accumulate_conductor_heal_requests_total", {"destination": "BVN1", "outcome": "answered", "source": "Directory"}, 58.0),
    ("accumulate_conductor_heal_requests_total", {"destination": "BVN1", "outcome": "miss", "source": "BVN2"}, 7.0),
    ("accumulate_conductor_heal_requests_total", {"destination": "BVN1", "outcome": "not-yet", "source": "Directory"}, 1515.0),
    ("accumulate_conductor_heal_requests_total", {"destination": "Directory", "outcome": "answered", "source": "BVN1"}, 3.0),
    ("accumulate_dispatcher_drops_total", {"destination": "BVN1", "reason": "queue-full"}, 5.0),
    ("accumulate_exec_staged_proofs_total", {"outcome": "duplicate"}, 11.0),
    ("accumulate_exec_staged_proofs_total", {"outcome": "staged"}, 228.0),
    ("accumulate_exec_staged_proofs_total", {"outcome": "validated"}, 8963.0),
    ("accumulate_exec_synthetic_anchor_total", {"applied": "collected"}, 20879.0),
    ("accumulate_exec_synthetic_anchor_total", {"applied": "proven"}, 630821.0),
    ("accumulate_staging_held_entries", {"ledger": "acc://dn.acme/anchors", "source": "acc://bvn-bvn1.acme"}, 33.0),
    ("accumulate_staging_held_bytes", {"ledger": "acc://dn.acme/anchors", "source": "acc://bvn-bvn1.acme"}, 5363.0),
    ("accumulate_staging_held_entries", {"ledger": "acc://bvn-bvn1.acme/synthetic", "source": "acc://bvn-bvn2.acme"}, 0.0),
]


class HealsFromExportedFamilies(unittest.TestCase):
    def test_this_runs_scrape_is_not_zero(self):
        h = soakmon.heals_from({"acc-bvn1-val1": NODE})
        self.assertTrue(h["measured"])
        self.assertEqual(h["entries"], 18171)
        self.assertEqual(h["total"], 18171)
        self.assertEqual(h["requests"]["answered"], 129)
        self.assertEqual(h["requests"]["not-yet"], 1515)
        self.assertEqual(h["requests"]["miss"], 7)
        self.assertEqual(h["requests"]["failed"], 0)
        self.assertEqual(h["requests"]["total"], 1651)
        self.assertEqual(h["errors"], 7)
        self.assertEqual(h["byStream"]["Directory->BVN1"]["not-yet"], 1515)
        self.assertEqual(h["proofs"]["validated"], 8963)
        self.assertEqual(h["judged"]["collected"], 20879)
        self.assertEqual(h["held"]["entries"], 33)
        self.assertEqual(h["held"]["bytes"], 5363)
        self.assertEqual(h["held"]["streams"][0]["source"], "acc://bvn-bvn1.acme")

    def test_counters_sum_across_nodes_and_gauges_take_the_max(self):
        other = [
            ("accumulate_conductor_heal_entries_total", {}, 100.0),
            ("accumulate_staging_held_entries", {"ledger": "acc://dn.acme/anchors", "source": "acc://bvn-bvn1.acme"}, 40.0),
        ]
        h = soakmon.heals_from({"a": NODE, "b": other})
        self.assertEqual(h["entries"], 18271)
        self.assertEqual(h["held"]["entries"], 40)

    def test_absent_is_not_zero(self):
        h = soakmon.heals_from({"a": [("accumulate_dagbft_blocks_produced_total", {}, 5.0)]})
        self.assertFalse(h["measured"])
        for k in ("entries", "total", "requests", "byStream", "proofs", "judged", "held", "errors", "stuck"):
            self.assertIsNone(h[k], k)
        # A dead family name counts for nothing, whatever it says
        h = soakmon.heals_from({"a": [("accumulate_crosschain_heals_total", {"type": "synthetic"}, 9.0)]})
        self.assertFalse(h["measured"])


class WedgesFromTheDispatcher(unittest.TestCase):
    def test_drops_by_reason_and_destination(self):
        w = soakmon.wedges_from({"a": NODE})
        self.assertTrue(w["measured"])
        self.assertEqual(w["total"], 5)
        self.assertEqual(w["byReason"], {"queue-full": 5})
        self.assertEqual(w["byDest"]["BVN1"], 5)

    def test_absent_is_not_zero(self):
        w = soakmon.wedges_from({"a": []})
        self.assertFalse(w["measured"])
        self.assertIsNone(w["total"])
        self.assertEqual(w["byDest"], {}, "not a column of zeros for the table")

    def test_the_chaos_counter_labels_its_reason_kind(self):
        w = soakmon.wedges_from({"a": [("accumulate_debug_dropped_total", {"dest": "BVN2", "kind": "synthetic"}, 3.0)]})
        self.assertEqual(w["byReason"], {"synthetic": 3}, "not a bucket called ?")
        self.assertEqual(w["byDest"]["BVN2"], 3)


if __name__ == "__main__":
    unittest.main()
