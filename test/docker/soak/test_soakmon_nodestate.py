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



class StartToActive(unittest.TestCase):
    """The number the verdict wants on a restart and an add-follower: the
    time from the container's start to ACTIVE, per partition (#4364)."""

    def sample(self, track, now, started, **parts):
        per = {"acc-bvn1-val2": scrape(**parts)}
        ns = soakmon.nodestate_from(per, now=now, disturbed=started)
        return soakmon.track_start_to_active(track, ns, started, now)

    def test_a_restart_is_measured_from_start_to_active(self):
        track, st = {}, {"acc-bvn1-val2": NOW}
        self.assertEqual([], self.sample(track, NOW + 5, st, directory=0, bvn1=0))
        got = self.sample(track, NOW + 40, st, directory=2, bvn1=0)
        self.assertEqual([("directory", 40.0, "reached")],
                         [(e["partition"], e["startToActiveS"], e["kind"]) for e in got])
        got = self.sample(track, NOW + 90, st, directory=2, bvn1=2)
        self.assertEqual([("bvn1", 90.0, "reached")],
                         [(e["partition"], e["startToActiveS"], e["kind"]) for e in got])
        # Reported once per start, not every sample after it.
        self.assertEqual([], self.sample(track, NOW + 95, st, directory=2, bvn1=2))
        # A second restart is a new start and is measured again.
        st2 = {"acc-bvn1-val2": NOW + 1000}
        self.sample(track, NOW + 1003, st2, directory=0, bvn1=0)
        got = self.sample(track, NOW + 1020, st2, directory=2, bvn1=2)
        self.assertEqual({20.0}, {e["startToActiveS"] for e in got})

    def test_active_at_first_sight_is_only_an_upper_bound(self):
        track, st = {}, {"acc-bvn1-val2": NOW - 3600}
        got = self.sample(track, NOW, st, directory=2)
        self.assertEqual(["already"], [e["kind"] for e in got])

    def test_a_start_that_never_reaches_active_is_pending(self):
        track, st = {}, {"acc-bvn1-val2": NOW}
        self.sample(track, NOW + 5, st, bvn1=0)
        pend = soakmon.pending_starts(track, NOW + 700)
        self.assertEqual([("acc-bvn1-val2", "bvn1", "BOOTING", 700.0)],
                         [(e["node"], e["partition"], e["state"], e["sinceStartS"]) for e in pend])
        line = soakmon.nodestate_csv_rows(pend, "T")[0]
        self.assertEqual(len(soakmon.NODESTATE_CSV_HEADER.split(",")), len(line.split(",")))

    def test_docker_start_times_parse(self):
        self.assertEqual(0, soakmon._parse_started("1970-01-01T00:00:00.123456789Z"))
        self.assertIsNone(soakmon._parse_started("0001-01-01T00:00:00Z"))


class EveryTrackedStartGetsAFinalRow(unittest.TestCase):
    """#4414: the manifest's rejoin verdict reads each start's `final` row
    from nodestate.csv, and a start with none has no last reading.

    Run 20260924T074702Z's review reported acc-bvn3-val1/bvn3 without one.
    That file has one (08:21:00Z, `ACTIVE,16.4,final,1926`), and every one
    of its 26 tracked pairs has one (`test_the_run_has_one_for_every_pair`).
    What was really fragile is the exit hook: the submissions, mem and
    node-state final writes shared one `try`, so a fault in either of the
    first two dropped the node-state final rows for EVERY node — the one
    file whose last reading decides "rejoined"."""

    RUN = os.path.join(os.path.dirname(os.path.abspath(__file__)),
                       "runs", "20260924T074702Z", "nodestate.csv")

    def setUp(self):
        import tempfile
        self._saved = (soakmon.RUN_DIR, dict(soakmon._NODESTATE_TRACK),
                       soakmon.write_submissions_csv, soakmon.write_mem_csv,
                       dict(soakmon.STATE))
        soakmon.RUN_DIR = tempfile.mkdtemp(prefix="nodestatefinal-")
        soakmon._NODESTATE_TRACK.clear()

    def tearDown(self):
        (soakmon.RUN_DIR, track, soakmon.write_submissions_csv,
         soakmon.write_mem_csv, state) = self._saved
        soakmon._NODESTATE_TRACK.clear()
        soakmon._NODESTATE_TRACK.update(track)
        soakmon.STATE.clear()
        soakmon.STATE.update(state)

    def track(self):
        """Two nodes, both partitions each, one of them restarted — the
        restarted start is the one tracked at exit."""
        for node, started, when in (("acc-bvn3-val2", NOW, NOW + 5),
                                    ("acc-bvn3-val1", NOW, NOW + 5),
                                    ("acc-bvn3-val1", NOW + 600, NOW + 620)):
            per = {node: scrape(bvn3=2, directory=2)}
            ns = soakmon.nodestate_from(per, now=when, disturbed={node: started})
            soakmon.track_start_to_active(soakmon._NODESTATE_TRACK, ns,
                                          {node: started}, when)
        return {("acc-bvn3-val1", "bvn3"), ("acc-bvn3-val1", "directory"),
                ("acc-bvn3-val2", "bvn3"), ("acc-bvn3-val2", "directory")}

    def finals(self):
        import csv
        path = os.path.join(soakmon.RUN_DIR, "nodestate.csv")
        if not os.path.exists(path):
            return set()
        with open(path) as f:
            return {(r["node"], r["partition"]) for r in csv.DictReader(f)
                    if r["kind"] == "final"}

    def test_every_tracked_pair_gets_one(self):
        want = self.track()
        soakmon.STATE["nodeStats"] = {}
        soakmon._final_rows()
        self.assertEqual(want, self.finals())

    def test_a_fault_in_another_final_write_does_not_drop_them(self):
        want = self.track()
        soakmon.STATE["nodeStats"] = {"submissions": {"byNode": {}},
                                      "mem": {"byNode": {}}}

        def boom(*a, **k):
            raise OSError("disk full")
        soakmon.write_submissions_csv = boom
        soakmon.write_mem_csv = boom
        soakmon._final_rows()     # must not raise either
        self.assertEqual(want, self.finals())

    def test_the_run_has_one_for_every_pair(self):
        import csv
        with open(self.RUN) as f:
            rows = list(csv.DictReader(f))
        pairs = {(r["node"], r["partition"]) for r in rows}
        finals = {(r["node"], r["partition"]) for r in rows if r["kind"] == "final"}
        self.assertEqual(26, len(pairs))
        self.assertEqual(pairs, finals)
        self.assertIn(("acc-bvn3-val1", "bvn3"), finals)


if __name__ == "__main__":
    unittest.main()
