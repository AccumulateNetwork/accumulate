#!/usr/bin/env python3
"""The per-node series files: `mem.csv` and `submissions.csv` (#4364).

Two gaps that run `20260919T191634Z` exposed, both of the same shape — a
number the board had and the run directory did not.

**mem.csv had twelve nodes for a thirteen-node network.** `collect_metrics`
splits the scrape so the fleet aggregates keep the validators' membership
(M6, #4365), and the file was written from the validators' half alone. The
node with no series was `acc-bvn3-fol1` — the one node that accepts
submissions it can never propose (#4366), so the one node whose RSS, heap,
goroutines and staged count anyone would want over twelve hours.

**`submissions.csv` is the other half of that:** run-analyst could not say
whether a submission was accepted at the follower and never proposed,
because nothing counts it. The families are Go and are not the harness's to
add (#4366/#4369); the harness's job is to name what it will read and to
render `— not measured` until it appears — never 0, because 0 would assert
the very thing the run could not establish.
"""
import json
import os
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import soakmon

# A scrape with the memory families every node exports, and nothing else.
SCRAPE = [
    ("process_resident_memory_bytes", {}, 1.07e9),
    ("go_goroutines", {}, 268.0),
    ("go_memstats_heap_alloc_bytes", {}, 6.4e8),
    ("go_memstats_heap_inuse_bytes", {}, 7.0e8),
    ("go_memstats_next_gc_bytes", {}, 1.01e9),
    ("go_gc_duration_seconds_count", {}, 4210.0),
    ("process_cpu_seconds_total", {}, 900.0),
    ("accumulate_bcdb_staged_commits", {"database": "bvnn"}, 4.0),
    ("accumulate_bcdb_oldest_view_age_seconds", {"database": "bvnn"}, 10.3),
]

# The same, plus the two families #4366 must export. `partition` and not
# `container`: every container runs TWO NODES, a DN node and a BVN node, and
# their submission queues are separate.
SCRAPE_WITH_SUBMISSIONS = SCRAPE + [
    ("accumulate_dagbft_submissions_total",
     {"partition": "Directory", "outcome": "accepted"}, 1200.0),
    ("accumulate_dagbft_submissions_total",
     {"partition": "Directory", "outcome": "rejected"}, 3.0),
    ("accumulate_dagbft_submissions_total",
     {"partition": "BVN3", "outcome": "accepted"}, 880.0),
    ("accumulate_dagbft_proposed_transactions_total", {"partition": "Directory"}, 1198.0),
    ("accumulate_dagbft_proposed_transactions_total", {"partition": "BVN3"}, 875.0),
]

FOLLOWER = [{"container": "acc-bvn3-fol1", "port": 26692, "dir": "bvn3-5",
             "bvn": "BVN3", "partitions": ["Directory", "BVN3"]}]


class Fleet(unittest.TestCase):
    """A two-validator, one-follower network, scraped without docker."""

    scrape = SCRAPE

    def setUp(self):
        self._saved = (soakmon.containers, soakmon._scrape_one,
                       soakmon.collect_flows_api, soakmon.FOLLOWERS,
                       soakmon.RUN_DIR)
        soakmon.FOLLOWERS = list(FOLLOWER)
        soakmon.containers = lambda: ["acc-bvn1-val1", "acc-bvn2-val1",
                                      "acc-bvn3-fol1"]
        soakmon._scrape_one = lambda c, out, lock: out.setdefault(c, list(self.scrape))
        soakmon.collect_flows_api = lambda: ({"synthetic": {}, "anchor": {}}, 0, 0)
        self.tmp = tempfile.mkdtemp(prefix="soaktest-")
        soakmon.RUN_DIR = self.tmp
        soakmon._MEM_LAST.clear()
        soakmon._MEM_CSV_T[0] = 0.0
        soakmon._SUB_CSV_T[0] = 0.0

    def tearDown(self):
        (soakmon.containers, soakmon._scrape_one, soakmon.collect_flows_api,
         soakmon.FOLLOWERS, soakmon.RUN_DIR) = self._saved
        soakmon._MEM_LAST.clear()

    def rows(self, name):
        with open(os.path.join(self.tmp, name)) as f:
            lines = [l.rstrip("\n") for l in f if l.strip()]
        return lines[0], lines[1:]


class MemCsvHoldsEveryNode(Fleet):
    def test_the_follower_has_a_row(self):
        """The defect, exactly: 12 rows for 13 nodes, and the missing one is
        the follower."""
        m = soakmon.collect_metrics()
        soakmon.write_mem_csv(m["nodeStats"]["mem"])
        head, rows = self.rows("mem.csv")
        self.assertEqual(3, len(rows), rows)
        self.assertEqual(sorted(["acc-bvn1-val1", "acc-bvn2-val1", "acc-bvn3-fol1"]),
                         sorted(r.split(",")[1] for r in rows))

    def test_the_follower_is_labelled_a_follower(self):
        """A reader that has to parse `acc-bvn3-fol1` to learn a role
        eventually parses it wrong."""
        m = soakmon.collect_metrics()
        soakmon.write_mem_csv(m["nodeStats"]["mem"])
        head, rows = self.rows("mem.csv")
        self.assertEqual("time,node,role", ",".join(head.split(",")[:3]))
        role = {r.split(",")[1]: r.split(",")[2] for r in rows}
        self.assertEqual("follower", role["acc-bvn3-fol1"])
        self.assertEqual("validator", role["acc-bvn1-val1"])

    def test_the_followers_row_carries_its_numbers_not_blanks(self):
        m = soakmon.collect_metrics()
        soakmon.write_mem_csv(m["nodeStats"]["mem"])
        head, rows = self.rows("mem.csv")
        cols = head.split(",")
        fol = dict(zip(cols, next(r for r in rows
                                  if r.split(",")[1] == "acc-bvn3-fol1").split(",")))
        self.assertEqual("1020", fol["rssMiB"])
        self.assertEqual("268", fol["goroutines"])
        self.assertEqual("4", fol["staged"])

    def test_the_aggregates_still_mean_the_validators(self):
        """The file gains the follower; the totals must not. `heapMaxMiB` and
        `stagedMax` are compared across runs, and a run with a follower has to
        stay comparable with one without (M6)."""
        m = soakmon.collect_metrics()
        mem = m["nodeStats"]["mem"]
        self.assertEqual(2, len(mem["byNode"]), "aggregates are the validators'")
        self.assertEqual(1, len(mem["followerByNode"]))
        self.assertEqual(2, m["nodeStats"]["count"])
        json.dumps(m)

    def test_a_run_with_no_follower_writes_the_same_header(self):
        """The column exists on every run, so one reader parses both."""
        soakmon.FOLLOWERS = []
        soakmon.containers = lambda: ["acc-bvn1-val1", "acc-bvn2-val1"]
        m = soakmon.collect_metrics()
        soakmon.write_mem_csv(m["nodeStats"]["mem"])
        head, rows = self.rows("mem.csv")
        self.assertEqual(soakmon.MEM_CSV_HEADER, head)
        self.assertEqual(2, len(rows))
        self.assertTrue(all(r.split(",")[2] == "validator" for r in rows))


class SubmissionsAreNotMeasuredYet(Fleet):
    """Today's build. Every consumer must say so, and none may say 0."""

    def test_the_families_are_absent_and_that_is_not_zero(self):
        m = soakmon.collect_metrics()
        sub = m["nodeStats"]["submissions"]
        self.assertFalse(sub["measured"])
        self.assertIsNone(sub["neverProposed"])
        self.assertIsNone(sub["accepted"])
        self.assertIsNone(sub["worstNeverProposed"])
        self.assertFalse(sub["follower"]["measured"])
        self.assertIsNone(sub["follower"]["neverProposed"])

    def test_the_file_is_written_with_a_header_and_no_rows(self):
        """The run directory says the harness asked. An empty field would
        read as "the node answered zero"."""
        m = soakmon.collect_metrics()
        soakmon.write_submissions_csv(m["nodeStats"]["submissions"])
        head, rows = self.rows("submissions.csv")
        self.assertEqual(soakmon.SUBMIT_CSV_HEADER, head)
        self.assertEqual([], rows)


class SubmissionsWhenTheFamilyAppears(Fleet):
    """The same harness, against a build that exports the contract."""

    scrape = SCRAPE_WITH_SUBMISSIONS

    def test_accepted_minus_proposed_per_node_and_partition(self):
        sub = soakmon.collect_metrics()["nodeStats"]["submissions"]
        self.assertTrue(sub["measured"])
        # Per validator: Directory 1200-1198 = 2, BVN3 880-875 = 5.
        self.assertEqual(7, sub["byNode"]["acc-bvn1-val1"]["neverProposed"])
        self.assertEqual(14, sub["neverProposed"], "two validators")
        self.assertEqual(2 * 2080, sub["accepted"])
        self.assertEqual(2 * 3, sub["rejected"])
        self.assertEqual(7, sub["worstNeverProposed"])

    def test_the_followers_figure_is_beside_the_total_not_inside_it(self):
        """Same membership rule as every other total (M6): the aggregate is
        the validators, the follower is named separately."""
        sub = soakmon.collect_metrics()["nodeStats"]["submissions"]
        self.assertEqual(14, sub["neverProposed"], "the validators only")
        self.assertEqual(7, sub["follower"]["neverProposed"])
        self.assertEqual("acc-bvn3-fol1", sub["follower"]["worstNode"])
        self.assertIn("acc-bvn3-fol1", sub["followerByNode"])
        self.assertNotIn("acc-bvn3-fol1", sub["byNode"])

    def test_the_csv_has_a_row_per_node_and_partition_with_its_role(self):
        m = soakmon.collect_metrics()
        soakmon.write_submissions_csv(m["nodeStats"]["submissions"])
        head, rows = self.rows("submissions.csv")
        self.assertEqual(soakmon.SUBMIT_CSV_HEADER, head)
        self.assertEqual(6, len(rows), "3 nodes x 2 partitions")
        cols = head.split(",")
        by = {(r.split(",")[1], r.split(",")[3]): dict(zip(cols, r.split(",")))
              for r in rows}
        self.assertEqual("follower", by[("acc-bvn3-fol1", "BVN3")]["role"])
        self.assertEqual("5", by[("acc-bvn3-fol1", "BVN3")]["acceptedNeverProposed"])
        self.assertEqual("880", by[("acc-bvn3-fol1", "BVN3")]["accepted"])
        # A partition that reported no rejections writes an empty field, not
        # a 0: the counter was never created, which is a different fact.
        self.assertEqual("", by[("acc-bvn1-val1", "BVN3")]["rejected"])

    def test_more_proposed_than_accepted_is_an_alarm_not_a_negative(self):
        """REPORTING-SPEC 1a: a value another value on the same panel
        disproves is an instrument fault, surfaced, never floored silently."""
        sub = soakmon.submissions_from({"acc-bvn1-val1": [
            ("accumulate_dagbft_submissions_total",
             {"partition": "BVN1", "outcome": "accepted"}, 10.0),
            ("accumulate_dagbft_proposed_transactions_total",
             {"partition": "BVN1"}, 12.0)]})
        self.assertEqual(0, sub["neverProposed"], "never negative")
        self.assertEqual(1, len(sub["impossible"]))
        self.assertIn("proposed 12 of 10 accepted", sub["impossible"][0])

    def test_a_node_exporting_only_one_half_is_measured_but_says_so(self):
        sub = soakmon.submissions_from({"acc-bvn1-val1": [
            ("accumulate_dagbft_submissions_total",
             {"partition": "BVN1", "outcome": "accepted"}, 10.0)]})
        self.assertTrue(sub["measured"])
        self.assertEqual(["submissions"], sub["families"])
        p = sub["byNode"]["acc-bvn1-val1"]["byPartition"]["BVN1"]
        self.assertIsNone(p["proposed"], "the missing half is absent, not 0")


class TheBoardSaysWhatTheNumberIs(unittest.TestCase):
    """The dashboard's own text, read as text."""

    with open(os.path.join(HERE, "soakmon.py")) as _fh:
        PAGE = _fh.read()
    del _fh

    def test_both_rows_exist(self):
        for tag in ("id=fstrand", "id=nstrand", "id=nacc", "id=nstrandnode"):
            self.assertIn(tag, self.PAGE, tag)

    def test_the_labels_name_the_quantity_and_the_window(self):
        self.assertIn("accepted, never proposed (#, whole run)", self.PAGE)
        self.assertIn("accepted, never proposed (#, whole run, worst validator)",
                      self.PAGE)
        self.assertIn("accepted (#, whole run)", self.PAGE)

    def test_every_new_id_has_a_definition(self):
        for tag in ("fstrand", "nstrand", "nacc", "nstrandnode"):
            self.assertIn(" %s:\"" % tag, self.PAGE,
                          "%s has no DEFS entry — undefined is the only "
                          "unacceptable state" % tag)

    def test_the_families_the_exporter_must_provide_are_named_in_one_place(self):
        """#4366's builder exports what this reads; the names must not drift
        between the note on that issue and the code that parses them."""
        self.assertEqual("accumulate_dagbft_submissions_total", soakmon.SUBMIT_TOTAL)
        self.assertEqual("accumulate_dagbft_proposed_transactions_total",
                         soakmon.PROPOSED_TOTAL)


if __name__ == "__main__":
    unittest.main()
