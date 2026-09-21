#!/usr/bin/env python3
"""The node-state row (#4364): `accumulate_node_state`, per node and per partition.

Nothing on the board read the gauge, and #4368's gate names a harness row for
it (PLAN.md, E11 second pass, item 4). The spec's states are two (executor.md,
"Sync"): `BOOTING` refuses, `ACTIVE` serves; `COMPLETE`/`WAITING` are retired.
The gauge says 0 booting, 2 active (internal/core/bootstrap/nodestate).

What the row must hold, each of which is a way this board has lied before:

- **A row per node AND per partition.** One process runs the Directory beside
  its BVN and the gauge is labelled by partition, so one node is two rows. A
  row per container would fold a booting BVN under an active Directory.
- **ACTIVE is value 2 and is the predicate.** Nothing else reads as active.
- **BOOTING is shown and named** — never hidden, and not an alarm by itself: a
  node that has just joined is booting and that is the join working.
- **BOOTING long after the disturbance that made it join IS an alarm.** The
  bound is a stated number, so the board can colour on it.
- **No gauge is `not measured`, never ACTIVE** (REPORTING-SPEC 1). Reading
  absent as fine is exactly how a monitor can never assert a node is alive.

The scrape is container -> [(name, labels, value)], as `collect_metrics`
hands it to every `*_from`.
"""
import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import soakmon

GAUGE = "accumulate_node_state"
NOW = 1_000_000.0


def scrape(**parts):
    """The gauge for each partition named, beside a family every node exports."""
    rows = [("process_resident_memory_bytes", {}, 1.07e9)]
    for part, v in parts.items():
        rows.append((GAUGE, {"partition": part}, float(v)))
    return rows


class NodeStateRow(unittest.TestCase):
    def rows_by_key(self, result):
        rows = result["rows"]
        out = {}
        for r in rows:
            key = (r["node"], r["partition"])
            self.assertNotIn(key, out, "one row per node per partition: %r twice" % (key,))
            out[key] = r
        return out

    def test_booting_is_shown_and_active_is_the_predicate(self):
        bound = soakmon.BOOTING_BOUND_S
        self.assertIsInstance(bound, (int, float))
        self.assertGreater(bound, 0)

        per = {
            # Healthy: both partitions this process runs are ACTIVE.
            "acc-bvn1-val1": scrape(directory=2, bvn1=2),
            # Just restarted by chaos: its BVN is booting, its Directory is
            # already active. Two rows, and they differ.
            "acc-bvn1-val2": scrape(directory=2, bvn1=0),
            # Restarted long ago and STILL booting on both: the join is stuck.
            "acc-bvn2-val1": scrape(directory=0, bvn2=0),
            # A build that exports no gauge: not measured, never active.
            "acc-bvn3-val1": [("process_resident_memory_bytes", {}, 1.0e9)],
            # A retired state value is not ACTIVE: 2 is the predicate.
            "acc-bvn3-val2": scrape(bvn3=3),
        }
        disturbed = {
            "acc-bvn1-val2": NOW - 5.0,
            "acc-bvn2-val1": NOW - bound * 10,
        }
        got = soakmon.nodestate_from(per, now=NOW, disturbed=disturbed)
        rows = self.rows_by_key(got)

        # A row per node AND per partition; the partition is canonical, so
        # the Directory beside BVN1 is its own row.
        for key in [("acc-bvn1-val1", "directory"), ("acc-bvn1-val1", "bvn1"),
                    ("acc-bvn1-val2", "directory"), ("acc-bvn1-val2", "bvn1"),
                    ("acc-bvn2-val1", "directory"), ("acc-bvn2-val1", "bvn2"),
                    ("acc-bvn3-val2", "bvn3")]:
            self.assertIn(key, rows)

        # ACTIVE is value 2, and is the predicate.
        for key in [("acc-bvn1-val1", "directory"), ("acc-bvn1-val1", "bvn1"),
                    ("acc-bvn1-val2", "directory")]:
            r = rows[key]
            self.assertTrue(r["measured"], key)
            self.assertEqual("ACTIVE", r["state"], key)
            self.assertTrue(r["active"], key)
            self.assertFalse(r["alarm"], key)
        retired = rows[("acc-bvn3-val2", "bvn3")]
        self.assertTrue(retired["measured"])
        self.assertFalse(retired["active"], "only 2 is ACTIVE")
        self.assertNotEqual("ACTIVE", retired["state"])

        # BOOTING is shown and named, and a fresh one is not an alarm.
        fresh = rows[("acc-bvn1-val2", "bvn1")]
        self.assertTrue(fresh["measured"])
        self.assertEqual("BOOTING", fresh["state"])
        self.assertFalse(fresh["active"])
        self.assertFalse(fresh["alarm"], "booting just after a restart is the join working")

        # BOOTING long after the disturbance is an alarm, on every partition.
        for part in ("directory", "bvn2"):
            stuck = rows[("acc-bvn2-val1", part)]
            self.assertEqual("BOOTING", stuck["state"])
            self.assertFalse(stuck["active"])
            self.assertTrue(stuck["alarm"], "booting %ds after the restart" % (bound * 10))

        # No gauge: not measured, never ACTIVE, and still a row (never hidden).
        absent = [r for (n, _), r in rows.items() if n == "acc-bvn3-val1"]
        self.assertTrue(absent, "a node with no gauge must still have a row")
        for r in absent:
            self.assertFalse(r["measured"])
            self.assertFalse(r["active"])
            self.assertNotEqual("ACTIVE", r["state"])
            self.assertIn("not measured", r["why"])

        # The fleet predicate: all active only if every row is measured and 2.
        self.assertFalse(got["allActive"])
        healthy = soakmon.nodestate_from(
            {"acc-bvn1-val1": per["acc-bvn1-val1"]}, now=NOW, disturbed={})
        self.assertTrue(healthy["allActive"])


if __name__ == "__main__":
    unittest.main()
