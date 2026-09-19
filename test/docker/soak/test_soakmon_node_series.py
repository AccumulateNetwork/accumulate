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
whether a submission was accepted at the follower and never carried into the
committed log, because nothing counts it. The families are Go and are not
the harness's to add (#4366/#4369); the harness's job is to name what it
will read and to render `— not measured` until it appears — never 0, because
0 would assert the very thing the run could not establish.

The second family counts **certified**, not proposed, and the fixtures below
are built to hold that distinction: a follower authors and broadcasts a
header carrying its own batches exactly as a validator does
(`header_builder.go:35-76`), so a "proposed" counter would tick for
everything it accepted and the gap would read 0 on the one node where
everything strands. What it never gets is 2f+1 votes — validators drop its
header at `vote_handler.go:277-284` — so `certified` is 0 for its lifetime
(reviewer H1 on #4364).

**And a third family counts the relay.** Paul, 2026-09-19: *"Followers can
relay txs. And should."* `accepted - certified` on a node in no committee
is everything it took, by construction — so under relay a follower doing
exactly what it should would have shown the largest red number on the
board under a label meaning failure. The quantity subtracts the hand-off
too, and the fixtures below carry the case that matters: a **relaying**
follower, `accepted == relayedTaken`, which must read **0 and not red**.
"""
import json
import os
import re
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
#
# A VALIDATOR: it certifies its own headers, so the gap is the in-flight
# window — the rounds not yet certified. Two and five.
SCRAPE_WITH_SUBMISSIONS = SCRAPE + [
    ("accumulate_dagbft_submissions_total",
     {"partition": "Directory", "outcome": "accepted"}, 1200.0),
    ("accumulate_dagbft_submissions_total",
     {"partition": "Directory", "outcome": "rejected"}, 3.0),
    ("accumulate_dagbft_submissions_total",
     {"partition": "BVN3", "outcome": "accepted"}, 880.0),
    ("accumulate_dagbft_certified_own_transactions_total", {"partition": "Directory"}, 1198.0),
    ("accumulate_dagbft_certified_own_transactions_total", {"partition": "BVN3"}, 875.0),
]

# A FOLLOWER THAT STRANDS — the gate-0 node, before relay exists. It
# accepted 1,200 on the Directory and 880 on BVN3, it PROPOSED every one of
# them (it authors and broadcasts headers like any node), it certified NONE
# because no validator votes on a header whose author is not in the
# committee, and it relayed nothing. The gap is everything it accepted.
#
# The first draft of this contract asked for "proposed" instead. Against a
# build that honoured it this node would have reported 1,200 and 880 proposed
# against 1,200 and 880 accepted — a gap of ZERO, rendered un-red under the
# words "accepted, never proposed", on the one node where nothing survives.
SCRAPE_FOLLOWER_SUBMISSIONS = SCRAPE + [
    ("accumulate_dagbft_submissions_total",
     {"partition": "Directory", "outcome": "accepted"}, 1200.0),
    ("accumulate_dagbft_submissions_total",
     {"partition": "BVN3", "outcome": "accepted"}, 880.0),
    ("accumulate_dagbft_certified_own_transactions_total", {"partition": "Directory"}, 0.0),
    ("accumulate_dagbft_certified_own_transactions_total", {"partition": "BVN3"}, 0.0),
    ("accumulate_dagbft_relayed_total",
     {"partition": "Directory", "outcome": "taken"}, 0.0),
    ("accumulate_dagbft_relayed_total",
     {"partition": "BVN3", "outcome": "taken"}, 0.0),
]

# A FOLLOWER THAT RELAYS — the node Paul described, working. Same 2,080
# accepted, still 0 certified because it is still in no committee, and every
# one of them handed to a node that took it. The quantity MUST read 0 here:
# subtracting only `certified` would paint this node maximal red for doing
# exactly what it is supposed to do.
SCRAPE_FOLLOWER_RELAYING = SCRAPE + [
    ("accumulate_dagbft_submissions_total",
     {"partition": "Directory", "outcome": "accepted"}, 1200.0),
    ("accumulate_dagbft_submissions_total",
     {"partition": "BVN3", "outcome": "accepted"}, 880.0),
    ("accumulate_dagbft_certified_own_transactions_total", {"partition": "Directory"}, 0.0),
    ("accumulate_dagbft_certified_own_transactions_total", {"partition": "BVN3"}, 0.0),
    ("accumulate_dagbft_relayed_total",
     {"partition": "Directory", "outcome": "taken"}, 1200.0),
    ("accumulate_dagbft_relayed_total",
     {"partition": "BVN3", "outcome": "taken"}, 880.0),
]

# A FOLLOWER RELAYING IMPERFECTLY — 1,200 on the Directory all taken; on
# BVN3, 800 taken, 50 a target validated and refused, 20 where every target
# that answered said NotReady, and 10 with no target reachable at all. What
# it does about those 80 is open for Paul (#4366); what the harness must do
# is show them under their own names — refused is policy, not-ready and
# unreachable are the network — and count them as stranded until something
# takes them.
SCRAPE_FOLLOWER_RELAY_PARTIAL = SCRAPE + [
    ("accumulate_dagbft_submissions_total",
     {"partition": "Directory", "outcome": "accepted"}, 1200.0),
    ("accumulate_dagbft_submissions_total",
     {"partition": "BVN3", "outcome": "accepted"}, 880.0),
    ("accumulate_dagbft_certified_own_transactions_total", {"partition": "Directory"}, 0.0),
    ("accumulate_dagbft_certified_own_transactions_total", {"partition": "BVN3"}, 0.0),
    ("accumulate_dagbft_relayed_total",
     {"partition": "Directory", "outcome": "taken"}, 1200.0),
    ("accumulate_dagbft_relayed_total",
     {"partition": "BVN3", "outcome": "taken"}, 800.0),
    ("accumulate_dagbft_relayed_total",
     {"partition": "BVN3", "outcome": "refused"}, 50.0),
    ("accumulate_dagbft_relayed_total",
     {"partition": "BVN3", "outcome": "not-ready"}, 20.0),
    ("accumulate_dagbft_relayed_total",
     {"partition": "BVN3", "outcome": "unreachable"}, 10.0),
]

FOLLOWER = [{"container": "acc-bvn3-fol1", "port": 26692, "dir": "bvn3-5",
             "bvn": "BVN3", "partitions": ["Directory", "BVN3"]}]


class Fleet(unittest.TestCase):
    """A two-validator, one-follower network, scraped without docker."""

    scrape = SCRAPE
    follower_scrape = None   # defaults to `scrape`

    def setUp(self):
        self._saved = (soakmon.containers, soakmon._scrape_one,
                       soakmon.collect_flows_api, soakmon.FOLLOWERS,
                       soakmon.RUN_DIR)
        soakmon.FOLLOWERS = list(FOLLOWER)
        soakmon.containers = lambda: ["acc-bvn1-val1", "acc-bvn2-val1",
                                      "acc-bvn3-fol1"]
        # The follower may answer differently from a validator — it must, for
        # the counter that matters (reviewer H1).
        soakmon._scrape_one = lambda c, out, lock: out.setdefault(
            c, list(self.follower_scrape if (self.follower_scrape and "fol" in c)
                    else self.scrape))
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
        self.assertIsNone(sub["stranded"])
        self.assertIsNone(sub["accepted"])
        self.assertIsNone(sub["relayedTaken"])
        self.assertIsNone(sub["worstStranded"])
        self.assertFalse(sub["follower"]["measured"])
        self.assertIsNone(sub["follower"]["stranded"])
        self.assertIsNone(sub["follower"]["relayedTaken"])

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
    follower_scrape = SCRAPE_FOLLOWER_SUBMISSIONS

    def test_a_validator_reads_its_in_flight_window(self):
        sub = soakmon.collect_metrics()["nodeStats"]["submissions"]
        self.assertTrue(sub["measured"])
        # Per validator: Directory 1200-1198 = 2, BVN3 880-875 = 5. It
        # certifies its own work and relays nothing, so the gap is the
        # rounds not yet certified.
        self.assertEqual(7, sub["byNode"]["acc-bvn1-val1"]["stranded"])
        self.assertEqual(0, sub["byNode"]["acc-bvn1-val1"]["relayedTaken"])
        self.assertEqual(14, sub["stranded"], "two validators")
        self.assertEqual(2 * 2080, sub["accepted"])
        self.assertEqual(2 * 3, sub["rejected"])
        self.assertEqual(7, sub["worstStranded"])

    def test_the_follower_certifies_nothing_so_the_gap_is_everything(self):
        """THE finding this whole contract turns on (reviewer H1 on #4364).

        This follower PROPOSED all 2,080 transactions it accepted — it
        authors and broadcasts headers carrying its own batches like any
        node — and certified none. Under the first draft's `proposed`
        counter the gap would have been 0, rendered as a plain un-red zero
        beside the words "never proposed", on the one node where everything
        strands. Against `certified` it is 2,080.
        """
        sub = soakmon.collect_metrics()["nodeStats"]["submissions"]
        f = sub["follower"]
        self.assertEqual(0, f["certified"], "a follower never certifies")
        self.assertEqual(2080, f["accepted"])
        self.assertEqual(0, f["relayedTaken"], "it relayed nothing")
        self.assertEqual(2080, f["stranded"],
                         "everything it accepted stranded")

    def test_the_followers_figure_is_beside_the_total_not_inside_it(self):
        """Same membership rule as every other total (M6): the aggregate is
        the validators, the follower is named separately."""
        sub = soakmon.collect_metrics()["nodeStats"]["submissions"]
        self.assertEqual(14, sub["stranded"], "the validators only")
        self.assertEqual(2080, sub["follower"]["stranded"])
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
        self.assertEqual("880", by[("acc-bvn3-fol1", "BVN3")]["accepted"])
        self.assertEqual("0", by[("acc-bvn3-fol1", "BVN3")]["certified"])
        self.assertEqual("0", by[("acc-bvn3-fol1", "BVN3")]["relayedTaken"])
        self.assertEqual(
            "880",
            by[("acc-bvn3-fol1", "BVN3")]["acceptedNeitherCertifiedNorTaken"])
        self.assertEqual(
            "5",
            by[("acc-bvn1-val1", "BVN3")]["acceptedNeitherCertifiedNorTaken"])
        # A partition that reported no rejections writes an empty field, not
        # a 0: the counter was never created, which is a different fact.
        self.assertEqual("", by[("acc-bvn1-val1", "BVN3")]["rejected"])

    def test_more_certified_than_accepted_is_an_alarm_not_a_negative(self):
        """REPORTING-SPEC 1a: a value another value on the same panel
        disproves is an instrument fault, surfaced, never floored silently.
        The likely cause is a counter counting per header rather than once
        per transaction, so a re-proposed batch is counted twice."""
        sub = soakmon.submissions_from({"acc-bvn1-val1": [
            ("accumulate_dagbft_submissions_total",
             {"partition": "BVN1", "outcome": "accepted"}, 10.0),
            ("accumulate_dagbft_certified_own_transactions_total",
             {"partition": "BVN1"}, 12.0)]})
        self.assertEqual(0, sub["stranded"], "never negative")
        self.assertEqual(1, len(sub["impossible"]))
        self.assertIn("certified 12 + relayed-taken 0 of 10 accepted",
                      sub["impossible"][0])

    def test_a_node_exporting_only_one_half_is_measured_but_says_so(self):
        sub = soakmon.submissions_from({"acc-bvn1-val1": [
            ("accumulate_dagbft_submissions_total",
             {"partition": "BVN1", "outcome": "accepted"}, 10.0)]})
        self.assertTrue(sub["measured"])
        self.assertEqual(["submissions"], sub["families"])
        p = sub["byNode"]["acc-bvn1-val1"]["byPartition"]["BVN1"]
        self.assertIsNone(p["certified"], "the missing half is absent, not 0")


class ARelayingFollowerIsNotAFailure(Fleet):
    """THE reason this contract was revised.

    Paul, 2026-09-19: "Followers can relay txs. And should." Under the
    previous quantity — `accepted - certified` — this node reads 2,080:
    the largest number on the board, red, under a label meaning failure,
    on a node doing exactly what it is supposed to do. `accepted -
    certified - relayedTaken` reads 0.
    """

    scrape = SCRAPE_WITH_SUBMISSIONS
    follower_scrape = SCRAPE_FOLLOWER_RELAYING

    def test_a_follower_that_relays_everything_strands_nothing(self):
        f = soakmon.collect_metrics()["nodeStats"]["submissions"]["follower"]
        self.assertEqual(2080, f["accepted"])
        self.assertEqual(0, f["certified"], "it is still in no committee")
        self.assertEqual(2080, f["relayedTaken"], "it handed on every one")
        self.assertEqual(0, f["stranded"],
                         "a working relay is not a stranded transaction")
        # And the number it would have shown under the old quantity, so the
        # test says out loud what the revision is worth.
        self.assertEqual(2080, f["accepted"] - f["certified"])

    def test_the_relay_outcomes_are_reported_separately(self):
        f = soakmon.collect_metrics()["nodeStats"]["submissions"]["follower"]
        self.assertEqual(0, f["relayedRefused"])
        self.assertEqual(0, f["relayedUnreachable"])

    def test_the_csv_carries_the_relay_columns(self):
        m = soakmon.collect_metrics()
        soakmon.write_submissions_csv(m["nodeStats"]["submissions"])
        head, rows = self.rows("submissions.csv")
        cols = head.split(",")
        for c in ("relayedTaken", "relayedRefused", "relayedUnreachable",
                  "acceptedNeitherCertifiedNorTaken"):
            self.assertIn(c, cols, c)
        row = dict(zip(cols, next(r for r in rows
                                  if r.split(",")[1] == "acc-bvn3-fol1"
                                  and r.split(",")[3] == "BVN3").split(",")))
        self.assertEqual("880", row["relayedTaken"])
        self.assertEqual("0", row["acceptedNeitherCertifiedNorTaken"])


class ARelayThatDoesNotAlwaysLand(Fleet):
    """Refused and unreachable are their own facts, and until something
    takes those submissions they are stranded.

    What the follower DOES about a refusal or an unreachable target is one
    of four questions open for Paul on #4366. The harness does not answer
    it — it reports both outcomes under their own names and counts what
    nobody took.
    """

    scrape = SCRAPE_WITH_SUBMISSIONS
    follower_scrape = SCRAPE_FOLLOWER_RELAY_PARTIAL

    def test_what_nobody_took_is_the_stranded_count(self):
        f = soakmon.collect_metrics()["nodeStats"]["submissions"]["follower"]
        self.assertEqual(2000, f["relayedTaken"], "1200 + 800")
        self.assertEqual(50, f["relayedRefused"])
        self.assertEqual(20, f["relayedNotReady"])
        self.assertEqual(10, f["relayedUnreachable"])
        self.assertEqual(80, f["stranded"], "50 refused + 20 not-ready + 10 lost")

    def test_a_syncing_target_is_not_a_refusal(self):
        """`refused` means a target VALIDATED it and declined — policy, and
        a statement about the submission. `not-ready` means every target
        that answered was still joining (#4307) — the network, and a
        statement about nothing at all. Filing one under the other sends
        the reader after a policy that does not exist (reviewer M2)."""
        per = (soakmon.collect_metrics()["nodeStats"]["submissions"]
               ["followerByNode"]["acc-bvn3-fol1"]["byPartition"]["BVN3"])
        self.assertEqual(50, per["relayedRefused"])
        self.assertEqual(20, per["relayedNotReady"])
        self.assertNotEqual(per["relayedRefused"], per["relayedNotReady"])
        self.assertIn("not-ready", soakmon.RELAY_OUTCOMES)

    def test_the_three_failure_outcomes_are_not_merged(self):
        per = (soakmon.collect_metrics()["nodeStats"]["submissions"]
               ["followerByNode"]["acc-bvn3-fol1"]["byPartition"]["BVN3"])
        self.assertEqual({50, 20, 10},
                         {per["relayedRefused"], per["relayedNotReady"],
                          per["relayedUnreachable"]})
        self.assertEqual(80, per["stranded"])


class APromotedNodeIsARealEventNotABrokenCounter(unittest.TestCase):
    """#4364's own disturbance: a follower is added, then promoted.

    If a relaying node keeps its own copy of what it relayed, the moment it
    is promoted it certifies that copy and `certified + relayed{taken} >
    accepted` fires — labelled an instrument fault, sending the reader to
    `tryCreateCertificateLocked` after a real event (reviewer M4). The
    contract forbids keeping the copy; the caption names promotion anyway,
    because a promotion can race an in-flight relay however the contract is
    written.
    """

    def test_it_fires_the_alarm(self):
        sub = soakmon.submissions_from({"acc-bvn3-fol1": [
            ("accumulate_dagbft_submissions_total",
             {"partition": "BVN3", "outcome": "accepted"}, 100.0),
            ("accumulate_dagbft_relayed_total",
             {"partition": "BVN3", "outcome": "taken"}, 100.0),
            ("accumulate_dagbft_certified_own_transactions_total",
             {"partition": "BVN3"}, 40.0)]}, "follower")
        self.assertEqual(1, len(sub["impossible"]))
        self.assertIn("certified 40 + relayed-taken 100 of 100 accepted",
                      sub["impossible"][0])
        self.assertEqual(0, sub["stranded"], "floored, never negative")

    def test_the_caption_names_promotion_as_a_cause(self):
        with open(os.path.join(HERE, "soakmon.py")) as fh:
            page = fh.read()
        tip = next(l for l in page.splitlines() if l.startswith(" nstrandnode:"))
        self.assertIn("PROMOTED", tip,
                      "a reader who is told 'instrument fault' goes to the "
                      "wrong code for the one event this run produces")

    def test_accepted_means_the_node_took_responsibility(self):
        """`accepted` has no arithmetic to test — it is a definition, and
        this is the third one. Both narrower readings were tried and both
        produce a false all-clear on a node that is working or a false
        alarm on one that is not:

        *"entered this node's worker batch"* — under a synchronous relay
        the envelope never does, so `accepted = 0` beside `relayed = n`:

            accepted=0, relayed-taken=880 ->
              ['… certified 0 + relayed-taken 880 of 0 accepted',
               '… relayed 880 of 0 accepted']
            stranded reads 0 (only because of the floor)

        *"`Submit` returned success to the caller"* — a relay ending
        `refused`, `not-ready` or `unreachable` returned no success, so one
        unreachable relay fires the instrument-fault alarm on a CORRECT run,
        and a node whose relays never land reads accepted 0 / stranded 0: a
        node dropping everything looks perfect (#4366 note_3869869239,
        decided at note_3869919047).

        So: the node took responsibility — the submission entered its
        worker or its relay — and the words are pinned here, where a
        builder reading the contract sees the same ones the harness was
        written against.
        """
        with open(os.path.join(HERE, "soakmon.py")) as fh:
            page = fh.read()
        self.assertIn("THE NODE TOOK RESPONSIBILITY", page)
        self.assertIn("it entered this node's worker,", page)
        self.assertIn("or it entered this node's relay", page)
        # Neither retracted reading, anywhere in the block.
        self.assertNotIn("Submit returned SUCCESS TO THE CALLER", page)
        self.assertNotIn("Submit returned success and the envelope", page)

    def test_a_relay_that_never_lands_is_accepted_and_stranded(self):
        """The arithmetic the wording has to support, and the case the
        retracted reading could not express: a node whose every relay ends
        `unreachable` took 880 and delivered none. It must read 880
        stranded — the row that should be red — and must NOT raise the
        instrument-fault alarm, because nothing here is impossible."""
        sub = soakmon.submissions_from({"acc-bvn3-fol1": [
            ("accumulate_dagbft_submissions_total",
             {"partition": "BVN3", "outcome": "accepted"}, 880.0),
            ("accumulate_dagbft_relayed_total",
             {"partition": "BVN3", "outcome": "unreachable"}, 880.0)]},
            "follower")
        self.assertEqual(880, sub["accepted"])
        self.assertEqual(880, sub["relayedUnreachable"])
        self.assertEqual(0, sub["relayedTaken"])
        self.assertEqual(880, sub["stranded"])
        self.assertEqual([], sub["impossible"],
                         "a correct run must not raise an instrument fault")

    def test_a_relay_that_mostly_lands_strands_only_the_rest(self):
        """The realistic case, and the one where the retracted reading is
        worst. 880 taken into the relay, 800 land, 80 do not.

        Under "took responsibility": accepted 880, stranded 80, no alarm —
        80 really did strand and the row is red for them.

        Under "returned success to the caller" the 80 would be `rejected`,
        so accepted 800 against 880 relayed: the instrument-fault alarm
        fires (`relayed 880 of 800 accepted`) AND stranded reads 0. Both
        wrong at once, on a run where 80 transactions were lost.
        """
        sub = soakmon.submissions_from({"acc-bvn3-fol1": [
            ("accumulate_dagbft_submissions_total",
             {"partition": "BVN3", "outcome": "accepted"}, 880.0),
            ("accumulate_dagbft_relayed_total",
             {"partition": "BVN3", "outcome": "taken"}, 800.0),
            ("accumulate_dagbft_relayed_total",
             {"partition": "BVN3", "outcome": "unreachable"}, 80.0)]},
            "follower")
        self.assertEqual(880, sub["accepted"])
        self.assertEqual(80, sub["stranded"])
        self.assertEqual([], sub["impossible"])

    def test_a_submission_refused_without_relaying_is_rejected(self):
        """`rejected` is what the node refused WITHOUT relaying, so it
        never enters the arithmetic: 0 accepted, 0 stranded, no alarm."""
        sub = soakmon.submissions_from({"acc-bvn3-fol1": [
            ("accumulate_dagbft_submissions_total",
             {"partition": "BVN3", "outcome": "rejected"}, 12.0)]}, "follower")
        self.assertEqual(12, sub["rejected"])
        self.assertEqual(0, sub["accepted"])
        self.assertEqual(0, sub["stranded"])
        self.assertEqual([], sub["impossible"])

    def test_the_contract_forbids_relaying_and_proposing_the_same_thing(self):
        with open(os.path.join(HERE, "soakmon.py")) as fh:
            page = fh.read()
        self.assertIn("A RELAYED SUBMISSION IS NOT ALSO PROPOSED BY THE "
                      "RELAYING NODE", page)


class TheLastSampleIsTheOneThatCounts(Fleet):
    """In flight and stranded are the same number at any one sample.

    soakmon is stopped after the load generator's grace drain and any idle
    tail, so the row it writes on the way out is the only one taken with
    nothing in flight — and it is written regardless of the 30-second
    interval, or there would be no such row (reviewer M3).
    """

    scrape = SCRAPE_WITH_SUBMISSIONS
    follower_scrape = SCRAPE_FOLLOWER_RELAYING

    def setUp(self):
        super().setUp()
        self._state = soakmon.STATE.get("nodeStats")

    def tearDown(self):
        # STATE is global; two of these write to it.
        if self._state is None:
            soakmon.STATE.pop("nodeStats", None)
        else:
            soakmon.STATE["nodeStats"] = self._state
        super().tearDown()

    def test_the_interval_is_honoured_without_force(self):
        m = soakmon.collect_metrics()
        soakmon.write_submissions_csv(m["nodeStats"]["submissions"])
        head, first = self.rows("submissions.csv")
        soakmon.write_submissions_csv(m["nodeStats"]["submissions"])
        head, again = self.rows("submissions.csv")
        self.assertEqual(len(first), len(again), "I_SUB not respected")

    def test_force_writes_the_final_row_anyway(self):
        m = soakmon.collect_metrics()
        soakmon.write_submissions_csv(m["nodeStats"]["submissions"])
        head, first = self.rows("submissions.csv")
        soakmon.write_submissions_csv(m["nodeStats"]["submissions"], force=True)
        head, again = self.rows("submissions.csv")
        self.assertEqual(2 * len(first), len(again))

    def test_mem_csv_takes_a_final_row_too(self):
        m = soakmon.collect_metrics()
        soakmon.write_mem_csv(m["nodeStats"]["mem"])
        head, first = self.rows("mem.csv")
        soakmon.write_mem_csv(m["nodeStats"]["mem"], force=True)
        head, again = self.rows("mem.csv")
        self.assertEqual(2 * len(first), len(again))

    def test_the_exit_hook_writes_both_and_never_raises(self):
        """It runs from a signal handler. Whatever it finds, the process is
        already going: it must not raise and must not block."""
        with soakmon.LOCK:
            soakmon.STATE["nodeStats"] = soakmon.collect_metrics()["nodeStats"]
        soakmon._final_rows()
        head, rows = self.rows("submissions.csv")
        self.assertTrue(rows, "no final submissions row")
        head, rows = self.rows("mem.csv")
        self.assertTrue(rows, "no final mem row")

    def test_the_final_row_is_marked_as_such(self):
        """A reader must be able to see that the exit write landed, not
        infer it from a timestamp: an idle tail puts periodic rows after
        the load generator too (reviewer N2)."""
        m = soakmon.collect_metrics()
        soakmon.write_submissions_csv(m["nodeStats"]["submissions"])
        soakmon.write_submissions_csv(m["nodeStats"]["submissions"], force=True)
        head, rows = self.rows("submissions.csv")
        cols = head.split(",")
        self.assertEqual("sample", cols[-1])
        kinds = [dict(zip(cols, r.split(",")))["sample"] for r in rows]
        self.assertIn("periodic", kinds)
        self.assertIn("final", kinds)
        self.assertEqual(len(rows) // 2, kinds.count("final"))

    def test_the_exit_hook_survives_an_empty_state(self):
        with soakmon.LOCK:
            soakmon.STATE["nodeStats"] = {}
        soakmon._final_rows()     # must not raise


class OutcomesThisHarnessDoesNotKnowYet(unittest.TestCase):
    """Four questions about the relay are open for Paul (#4366), and the
    answer may add an outcome. An unknown label is counted and named, never
    folded into a known one and never dropped — folding it would move a
    failure into `accepted` and read as success."""

    def test_an_unknown_outcome_is_surfaced_not_absorbed(self):
        sub = soakmon.submissions_from({"acc-bvn3-fol1": [
            ("accumulate_dagbft_submissions_total",
             {"partition": "BVN3", "outcome": "accepted"}, 100.0),
            ("accumulate_dagbft_relayed_total",
             {"partition": "BVN3", "outcome": "taken"}, 60.0),
            ("accumulate_dagbft_relayed_total",
             {"partition": "BVN3", "outcome": "deferred"}, 40.0)]}, "follower")
        self.assertEqual({"deferred": 40}, sub["unknownRelayOutcomes"])
        self.assertEqual(60, sub["relayedTaken"])
        self.assertEqual(40, sub["stranded"],
                         "an outcome we cannot read is not a hand-off")

    def test_relaying_more_than_was_accepted_is_an_alarm(self):
        sub = soakmon.submissions_from({"acc-bvn3-fol1": [
            ("accumulate_dagbft_submissions_total",
             {"partition": "BVN3", "outcome": "accepted"}, 10.0),
            ("accumulate_dagbft_relayed_total",
             {"partition": "BVN3", "outcome": "taken"}, 6.0),
            ("accumulate_dagbft_relayed_total",
             {"partition": "BVN3", "outcome": "refused"}, 9.0)]}, "follower")
        self.assertEqual(1, len(sub["impossible"]))
        self.assertIn("relayed 15 of 10 accepted", sub["impossible"][0])

    def test_relays_with_no_accepted_series_at_all_is_an_alarm(self):
        """The two checks above are guarded on the partition having
        reported `accepted`, so a build that exports `relayed_total` and
        never creates `submissions_total{outcome="accepted"}` slips past
        both: stranded floors to 0 and a node relaying everything — or
        losing everything — reads clean. Under the current wording a
        relayed submission IS accepted, so relays with no accepted series
        is a counter the node never created, not a quiet node."""
        sub = soakmon.submissions_from({"acc-bvn3-fol1": [
            ("accumulate_dagbft_relayed_total",
             {"partition": "BVN3", "outcome": "taken"}, 600.0),
            ("accumulate_dagbft_relayed_total",
             {"partition": "BVN3", "outcome": "unreachable"}, 40.0)]},
            "follower")
        self.assertEqual(1, len(sub["impossible"]), sub["impossible"])
        self.assertIn("640 relayed and no accepted series at all",
                      sub["impossible"][0])
        self.assertEqual(0, sub["stranded"], "floored, and therefore a lie")

    def test_a_reported_zero_is_not_the_same_as_no_series(self):
        """A node that says `accepted 0` beside relays is caught by the
        existing check and reads differently — a measurement that is wrong,
        not a measurement that is missing."""
        sub = soakmon.submissions_from({"acc-bvn3-fol1": [
            ("accumulate_dagbft_submissions_total",
             {"partition": "BVN3", "outcome": "accepted"}, 0.0),
            ("accumulate_dagbft_relayed_total",
             {"partition": "BVN3", "outcome": "taken"}, 600.0)]}, "follower")
        self.assertEqual(2, len(sub["impossible"]), sub["impossible"])
        self.assertIn("relayed 600 of 0 accepted", " ".join(sub["impossible"]))
        self.assertNotIn("no accepted series", " ".join(sub["impossible"]))

    def test_no_relays_and_no_accepted_series_is_not_an_alarm(self):
        """A node with neither is a node nothing reached, or a build with
        neither family. Absence is not a fault."""
        sub = soakmon.submissions_from({"acc-bvn3-fol1": [
            ("accumulate_dagbft_certified_own_transactions_total",
             {"partition": "BVN3"}, 0.0)]}, "follower")
        self.assertEqual([], sub["impossible"])

    def test_an_unknown_outcome_counts_toward_that_alarm_too(self):
        """Otherwise a build with a fourth label can relay more than it
        took with no alarm at all (reviewer L2)."""
        sub = soakmon.submissions_from({"acc-bvn3-fol1": [
            ("accumulate_dagbft_submissions_total",
             {"partition": "BVN3", "outcome": "accepted"}, 10.0),
            ("accumulate_dagbft_relayed_total",
             {"partition": "BVN3", "outcome": "taken"}, 6.0),
            ("accumulate_dagbft_relayed_total",
             {"partition": "BVN3", "outcome": "deferred"}, 9.0)]}, "follower")
        self.assertEqual({"deferred": 9}, sub["unknownRelayOutcomes"])
        self.assertEqual(1, len(sub["impossible"]),
                         "15 relayed against 10 accepted, 9 of them unread")
        self.assertIn("relayed 15 of 10 accepted", sub["impossible"][0])


class TheBoardSaysWhatTheNumberIs(unittest.TestCase):
    """The dashboard's own text, read as text."""

    with open(os.path.join(HERE, "soakmon.py")) as _fh:
        PAGE = _fh.read()
    del _fh

    def test_both_rows_exist(self):
        for tag in ("id=fstrand", "id=frelay", "id=nstrand", "id=nacc",
                    "id=nstrandnode"):
            self.assertIn(tag, self.PAGE, tag)

    def test_the_labels_name_the_quantity_and_the_window(self):
        self.assertIn(
            "accepted, neither certified here nor taken on relay "
            "(#, whole run)", self.PAGE)
        self.assertIn(
            "accepted, neither certified here nor taken on relay "
            "(#, whole run, worst validator)", self.PAGE)
        self.assertIn("accepted (#, whole run)", self.PAGE)
        self.assertIn("relayed (#, whole run): taken / refused / "
                      "target not ready / unreachable", self.PAGE)

    def test_one_word_for_the_relay_outcome_everywhere(self):
        """The family, the CSV column, the board label and the spec all say
        `taken`. Two words for one outcome is how a reader comes to think
        they are two outcomes (reviewer L1)."""
        self.assertNotIn("relayedAccepted", self.PAGE)
        self.assertNotIn("relayed-accepted", self.PAGE)
        self.assertIn("relayedTaken", soakmon.SUBMIT_CSV_HEADER)
        self.assertIn("taken", soakmon.RELAY_OUTCOMES)

    def test_no_rendered_label_says_proposed(self):
        """A follower DOES propose, so the word on a label would make the
        board wrong on the one node the row exists for (reviewer H1 on
        #4364). Prose that explains the distinction is fine and wanted; a
        `<span class=sl>` the reader sees is not."""
        labels = re.findall(r"<span class=sl>([^<]*)</span>", self.PAGE)
        self.assertTrue(labels, "no labels found — the parser is wrong")
        for lab in labels:
            self.assertNotIn("proposed", lab,
                             "a visible label says 'proposed': %r" % lab)
        self.assertNotIn("accumulate_dagbft_proposed_transactions_total",
                         self.PAGE, "the retracted family name is still here")

    def test_every_new_id_has_a_definition(self):
        for tag in ("fstrand", "frelay", "nstrand", "nacc", "nstrandnode"):
            self.assertIn(" %s:\"" % tag, self.PAGE,
                          "%s has no DEFS entry — undefined is the only "
                          "unacceptable state" % tag)

    def test_the_families_the_exporter_must_provide_are_named_in_one_place(self):
        """#4366's builder exports what this reads; the names must not drift
        between the note on that issue and the code that parses them."""
        self.assertEqual("accumulate_dagbft_submissions_total", soakmon.SUBMIT_TOTAL)
        self.assertEqual("accumulate_dagbft_certified_own_transactions_total",
                         soakmon.CERTIFIED_TOTAL)
        self.assertEqual("accumulate_dagbft_relayed_total", soakmon.RELAYED_TOTAL)
        self.assertEqual(("taken", "refused", "not-ready", "unreachable"),
                         soakmon.RELAY_OUTCOMES)
        self.assertEqual(set(soakmon.RELAY_OUTCOMES),
                         set(soakmon.RELAY_FIELD),
                         "every outcome has a field, and no field is orphaned")


if __name__ == "__main__":
    unittest.main()
