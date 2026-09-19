#!/usr/bin/env python3
"""followerlog reads gate 0's verdict (#4365) out of the node log.

Four questions, on synthetic logs so the reader is proven before a run spends
five minutes producing one:

  does the follower's state root equal the validators' at every block;
  is its key in any committee;
  did any validator ever see a certificate authored outside the committee;
  and — the one this file exists for — does an instrument that is NOT THERE
  render as absent rather than as a clean zero (REPORTING-SPEC 1).

The last is the whole point. Three of the lines a reader would reach for
first are `slog.Debug` with no `module` attribute, and the generated logging
config sets Debug only for named modules and Info for everything else, so
the build emits nothing for them. A reader that counted them and printed 0
would report "no header of the follower's was ever rejected" for a network
that never said a word on the subject.
"""
import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import followerlog

FOL = "acc-bvn3-fol1"
VALS = ["acc-bvn3-val1", "acc-bvn3-val2"]

# Every node logs its own key, per engine, at Info (consensus.go:585) — 16 hex
# characters of the raw public key, of which the author/pubkey fields elsewhere
# carry the first 8 (vote_handler.go:552).
IDENT = [
    "acc-bvn3-val1  | 2026-09-19T18:00:00Z INFO Starting consensus node partition=BVN3 numWorkers=1 validatorKey=aaaa1111aaaa1111",
    "acc-bvn3-val1  | 2026-09-19T18:00:00Z INFO Starting consensus node partition=Directory numWorkers=1 validatorKey=aaaa1111aaaa1111",
    "acc-bvn3-val2  | 2026-09-19T18:00:00Z INFO Starting consensus node partition=BVN3 numWorkers=1 validatorKey=bbbb2222bbbb2222",
    "acc-bvn3-val2  | 2026-09-19T18:00:00Z INFO Starting consensus node partition=Directory numWorkers=1 validatorKey=bbbb2222bbbb2222",
    "acc-bvn3-fol1  | 2026-09-19T18:00:00Z INFO Starting consensus node partition=BVN3 numWorkers=1 validatorKey=ffff9999ffff9999",
    "acc-bvn3-fol1  | 2026-09-19T18:00:00Z INFO Starting consensus node partition=Directory numWorkers=1 validatorKey=ffff9999ffff9999",
]

# The committee each engine built at startup, at Info (dagbft.go:427).
COMMITTEE = [
    "acc-bvn3-val1  | 2026-09-19T18:00:01Z INFO Extracted initial validators for DAG-BFT partition=BVN3 validators=4",
    "acc-bvn3-val1  | 2026-09-19T18:00:01Z INFO Extracted initial validators for DAG-BFT partition=Directory validators=12",
    "acc-bvn3-val2  | 2026-09-19T18:00:01Z INFO Extracted initial validators for DAG-BFT partition=BVN3 validators=4",
    "acc-bvn3-val2  | 2026-09-19T18:00:01Z INFO Extracted initial validators for DAG-BFT partition=Directory validators=12",
    "acc-bvn3-fol1  | 2026-09-19T18:00:01Z INFO Extracted initial validators for DAG-BFT partition=BVN3 validators=4",
    "acc-bvn3-fol1  | 2026-09-19T18:00:01Z INFO Extracted initial validators for DAG-BFT partition=Directory validators=12",
]

# A BVN anchors only to the Directory; the Directory anchors to every BVN.
# That is how the SOURCE partition of an anchor line is known — the line
# itself carries only the destination (conductor.go:308).
def anchor(node, ts, block, dest, root, bpt):
    return ("%s  | %s INFO Sending an anchor module=conductor block=%d "
            "destination=%s seq=%d root=%s bpt=%s"
            % (node, ts, block, dest, block, root, bpt))


AGREE = [
    anchor(n, "2026-09-19T18:01:0%d" % i, 100 + i, "acc://dn.acme",
           "r%02d" % i, "b%02d" % i)
    for n in VALS + [FOL] for i in range(3)
] + [
    anchor(n, "2026-09-19T18:01:1%d" % i, 200 + i, "acc://bvn-BVN3.acme",
           "dr%02d" % i, "db%02d" % i)
    for n in VALS + [FOL] for i in range(2)
]


class Identity(unittest.TestCase):
    def test_each_node_states_its_own_key_per_engine(self):
        r = followerlog.read(IDENT)
        self.assertEqual({"BVN3": "ffff9999ffff9999",
                          "Directory": "ffff9999ffff9999"}, r.identities[FOL])
        self.assertEqual("ffff9999", r.key_prefix(FOL),
                         "author= and pubkey= carry eight hex characters")

    def test_the_nodes_own_bvn_comes_from_the_log_not_the_name(self):
        r = followerlog.read(IDENT)
        self.assertEqual("BVN3", r.bvn_of(FOL))

    def test_a_node_that_never_logged_its_key_has_none(self):
        r = followerlog.read([])
        self.assertIsNone(r.key_prefix(FOL))


class Roots(unittest.TestCase):
    """The state root the follower computed for a block, against a validator's
    for the same block of the same partition."""

    def test_agreement_on_every_anchored_block(self):
        r = followerlog.read(IDENT + AGREE)
        v = followerlog.compare_roots(r, FOL, VALS)
        self.assertTrue(v["measured"])
        self.assertEqual(5, v["compared"], "3 BVN3 blocks and 2 Directory blocks")
        self.assertEqual([], v["mismatches"])
        self.assertIsNone(v["firstMismatch"])

    def test_the_source_partition_is_taken_from_the_destination(self):
        """reading-a-run.md: never compare without the partition. A BVN's
        anchors go only to the Directory and the Directory's go everywhere,
        so a block number alone names two different blocks."""
        r = followerlog.read(IDENT + AGREE)
        self.assertEqual({("BVN3", 100), ("BVN3", 101), ("BVN3", 102),
                          ("Directory", 200), ("Directory", 201)},
                         set(r.anchor_blocks(FOL)))

    def test_one_differing_root_is_named_with_its_block(self):
        bad = list(AGREE)
        bad.append(anchor(FOL, "2026-09-19T18:02:00Z", 103, "acc://dn.acme",
                          "DEAD", "b03"))
        bad.append(anchor(VALS[0], "2026-09-19T18:02:00Z", 103, "acc://dn.acme",
                          "r03", "b03"))
        v = followerlog.compare_roots(followerlog.read(IDENT + bad), FOL, VALS)
        self.assertEqual(1, len(v["mismatches"]))
        self.assertEqual(("BVN3", 103), v["firstMismatch"][:2])
        self.assertIn("DEAD", str(v["firstMismatch"]))

    def test_a_differing_bpt_counts_too(self):
        bad = list(AGREE)
        bad.append(anchor(FOL, "2026-09-19T18:02:00Z", 104, "acc://dn.acme",
                          "r04", "DEAD"))
        bad.append(anchor(VALS[0], "2026-09-19T18:02:00Z", 104, "acc://dn.acme",
                          "r04", "b04"))
        v = followerlog.compare_roots(followerlog.read(IDENT + bad), FOL, VALS)
        self.assertEqual(1, len(v["mismatches"]))

    def test_a_silent_follower_is_not_agreement(self):
        """If the follower's conductor logs no anchor, zero mismatches is not
        a measurement — it is the absence of one, and the fallback (the v3 API
        on both nodes at the same ledger index) has to be named."""
        only_validators = [ln for ln in AGREE if not ln.startswith(FOL)]
        v = followerlog.compare_roots(followerlog.read(IDENT + only_validators),
                                      FOL, VALS)
        self.assertFalse(v["measured"])
        self.assertEqual(0, v["compared"])
        self.assertIn("no anchor", v["why"])

    def test_a_block_only_the_follower_anchored_is_not_compared(self):
        """The follower running a block ahead at the moment the log was cut is
        not a mismatch; it is a block with nothing to compare against."""
        extra = AGREE + [anchor(FOL, "2026-09-19T18:03:00Z", 199,
                                "acc://dn.acme", "r99", "b99")]
        v = followerlog.compare_roots(followerlog.read(IDENT + extra), FOL, VALS)
        self.assertEqual([], v["mismatches"])
        self.assertEqual(5, v["compared"])
        self.assertEqual(1, v["uncompared"])


class Committee(unittest.TestCase):
    def test_the_committees_exclude_the_follower_by_count(self):
        r = followerlog.read(IDENT + COMMITTEE)
        v = followerlog.committee_check(r, FOL, VALS)
        self.assertTrue(v["measured"])
        self.assertEqual({"BVN3": 4, "Directory": 12}, v["sizes"])
        self.assertEqual([], v["addedDuringRun"])
        self.assertTrue(v["followerExcluded"])

    def test_disagreeing_committee_sizes_are_a_finding(self):
        lines = IDENT + COMMITTEE + [
            "acc-bvn3-val2  | 2026-09-19T18:00:01Z INFO Extracted initial "
            "validators for DAG-BFT partition=BVN3 validators=5"]
        v = followerlog.committee_check(followerlog.read(lines), FOL, VALS)
        self.assertIn("BVN3", v["disagree"])
        self.assertFalse(v["followerExcluded"])

    def test_the_follower_being_added_to_a_committee_is_caught(self):
        lines = IDENT + COMMITTEE + [
            "acc-bvn3-val1  | 2026-09-19T18:04:00Z INFO Validator added "
            "pubkey=ffff9999 stake=1"]
        v = followerlog.committee_check(followerlog.read(lines), FOL, VALS)
        self.assertEqual(1, len(v["addedDuringRun"]))
        self.assertFalse(v["followerExcluded"])

    def test_another_validator_being_added_is_reported_but_is_not_the_follower(self):
        lines = IDENT + COMMITTEE + [
            "acc-bvn3-val1  | 2026-09-19T18:04:00Z INFO Validator added "
            "pubkey=cccc3333 stake=1"]
        v = followerlog.committee_check(followerlog.read(lines), FOL, VALS)
        self.assertEqual(1, len(v["addedDuringRun"]))
        self.assertTrue(v["followerExcluded"], "it was not the follower's key")

    def test_no_committee_line_at_all_is_not_a_pass(self):
        v = followerlog.committee_check(followerlog.read(IDENT), FOL, VALS)
        self.assertFalse(v["measured"])
        self.assertIsNone(v["followerExcluded"])


class Certificates(unittest.TestCase):
    """`Invalid certificate` is Info on purpose (#4054), and
    certificate.go:170 is the refusal of a header authored outside the
    committee. It is the one place a follower's proposal, had one been
    carried, becomes visible on a validator."""

    def test_none_is_the_expected_result(self):
        v = followerlog.certificate_check(followerlog.read(IDENT + AGREE), FOL, VALS)
        self.assertTrue(v["measured"])
        self.assertEqual(0, v["nonCommitteeAuthor"])
        self.assertEqual(0, v["byFollower"])

    def test_a_certificate_the_follower_authored_is_counted_and_named(self):
        lines = IDENT + [
            "acc-bvn3-val1  | 2026-09-19T18:05:00Z INFO Invalid certificate "
            'error="header author is not in committee" round=206 '
            "author=ffff9999 digest=1234",
            "acc-bvn3-val2  | 2026-09-19T18:05:00Z INFO Invalid certificate "
            'error="header author is not in committee" round=206 '
            "author=ffff9999 digest=1234",
        ]
        v = followerlog.certificate_check(followerlog.read(lines), FOL, VALS)
        self.assertEqual(2, v["nonCommitteeAuthor"])
        self.assertEqual(2, v["byFollower"])
        self.assertEqual({"acc-bvn3-val1": 1, "acc-bvn3-val2": 1}, v["byValidator"])

    def test_an_invalid_certificate_for_another_reason_is_not_counted(self):
        lines = IDENT + [
            "acc-bvn3-val1  | 2026-09-19T18:05:00Z INFO Invalid certificate "
            'error="certificate has no signatures" round=9 author=aaaa1111 digest=1']
        v = followerlog.certificate_check(followerlog.read(lines), FOL, VALS)
        self.assertEqual(0, v["nonCommitteeAuthor"])
        self.assertEqual(1, v["otherInvalid"])


class AbsentInstruments(unittest.TestCase):
    """The three Debug lines the build does not emit. This is the class that
    keeps the report honest."""

    def test_header_and_vote_drops_render_absent_not_zero(self):
        v = followerlog.drop_check(followerlog.read(IDENT + AGREE), FOL, VALS)
        self.assertFalse(v["measured"])
        self.assertIsNone(v["headerDrops"], "absent is not zero")
        self.assertIsNone(v["voteDrops"])
        self.assertIn("slog.Debug", v["why"])
        self.assertIn("module", v["why"])

    def test_if_the_build_ever_emits_them_they_are_counted(self):
        """So that adding `\"module\", \"primary\"` to those calls turns the row
        on without another harness change."""
        lines = IDENT + [
            "acc-bvn3-val1  | 2026-09-19T18:05:00Z DEBUG Header from unknown "
            "validator module=primary author=ffff9999",
            "acc-bvn3-val1  | 2026-09-19T18:05:01Z DEBUG Vote from unknown "
            "validator module=primary author=ffff9999",
        ]
        v = followerlog.drop_check(followerlog.read(lines), FOL, VALS)
        self.assertTrue(v["measured"])
        self.assertEqual(1, v["headerDrops"])
        self.assertEqual(1, v["voteDrops"])


class Definition(unittest.TestCase):
    """The other half of "in no committee": what the NetworkDefinition says,
    read from network-status before load starts. This is what decides whether
    the run used the inactive-key form or the absent-key form."""

    DEF = {"network": {"validators": [
        {"publicKey": "aa" * 32, "partitions": [
            {"id": "Directory", "active": True}, {"id": "BVN3", "active": True}]},
        {"publicKey": "bb" * 32, "partitions": [
            {"id": "Directory", "active": True}, {"id": "BVN3", "active": True}]},
        {"publicKey": "ff" * 32, "partitions": [
            {"id": "Directory", "active": False}, {"id": "BVN3", "active": False}]},
    ]}}

    def test_active_counts_per_partition_and_the_inactive_key(self):
        v = followerlog.definition_check(self.DEF)
        self.assertEqual({"Directory": 2, "BVN3": 2}, v["active"])
        self.assertEqual(["ff" * 32], v["inactiveKeys"])
        self.assertEqual("inactive in the definition", v["followerKeyForm"])

    def test_no_inactive_entry_means_the_key_is_absent_from_the_definition(self):
        d = {"network": {"validators": self.DEF["network"]["validators"][:2]}}
        v = followerlog.definition_check(d)
        self.assertEqual([], v["inactiveKeys"])
        self.assertEqual("absent from the definition", v["followerKeyForm"])

    def test_an_unreadable_definition_is_absent_not_empty(self):
        v = followerlog.definition_check(None)
        self.assertFalse(v["measured"])
        self.assertIsNone(v["followerKeyForm"])


class TheReadersOwnDefect(unittest.TestCase):
    """Found by running it, not by a test: `read` walked the input TWICE —
    once for identities, once for anchors, because an anchor line's source
    partition needs the node's own BVN. Every test passed a list, so every
    test passed. `main` passes an open FILE, and the second walk over an
    exhausted handle yields nothing, so a real run reported "the follower
    logged no anchor line" for a follower that logged hundreds.

    A test that hands the reader a list does by hand what the caller cannot:
    it rewinds.
    """

    def test_a_single_use_iterator_reads_the_same_as_a_list(self):
        lines = IDENT + COMMITTEE + AGREE
        from_list = followerlog.compare_roots(followerlog.read(lines), FOL, VALS)
        from_iter = followerlog.compare_roots(
            followerlog.read(iter(lines)), FOL, VALS)
        self.assertEqual(5, from_list["compared"])
        self.assertEqual(from_list["compared"], from_iter["compared"])
        self.assertTrue(from_iter["measured"])

    def test_it_reads_an_actual_file_handle(self):
        import tempfile
        with tempfile.NamedTemporaryFile("w", suffix=".txt", delete=False) as f:
            f.write("\n".join(IDENT + COMMITTEE + AGREE) + "\n")
            path = f.name
        self.addCleanup(os.unlink, path)
        with open(path) as f:
            v = followerlog.compare_roots(followerlog.read(f), FOL, VALS)
        self.assertTrue(v["measured"])
        self.assertEqual(5, v["compared"])


class BehindSeries(unittest.TestCase):
    """The `behind` series out of follower.csv — the manifest's "max over the
    run and when"."""

    CSV = [
        "time,follower,partition,followerHeight,validatorsMaxHeight,behindBlocks",
        "2026-09-19T18:00:00Z,acc-bvn3-fol1,BVN3,100,101,1",
        "2026-09-19T18:00:00Z,acc-bvn3-fol1,Directory,100,105,5",
        "2026-09-19T18:00:20Z,acc-bvn3-fol1,BVN3,120,120,0",
        "2026-09-19T18:00:20Z,acc-bvn3-fol1,Directory,120,121,1",
    ]

    def test_the_worst_reading_and_when_and_where(self):
        v = followerlog.behind_summary(self.CSV)
        self.assertTrue(v["measured"])
        self.assertEqual(5, v["maxBehind"])
        self.assertEqual("2026-09-19T18:00:00Z", v["maxAt"])
        self.assertEqual("Directory", v["maxPartition"])
        self.assertEqual({"BVN3": 0, "Directory": 1}, v["endBehind"])
        self.assertEqual(4, v["samples"])

    def test_a_sample_the_follower_did_not_answer_is_counted_not_averaged(self):
        """An empty behind is silence. Read as a 0 it would make a follower
        that went away look like the best reading of the run."""
        v = followerlog.behind_summary(self.CSV + [
            "2026-09-19T18:00:40Z,acc-bvn3-fol1,Directory,,140,"])
        self.assertEqual(1, v["unanswered"])
        self.assertEqual(5, v["maxBehind"])
        self.assertEqual(4, v["samples"])

    def test_a_csv_with_no_answered_sample_is_absent_not_zero(self):
        v = followerlog.behind_summary([self.CSV[0],
                                        "2026-09-19T18:00:40Z,f,Directory,,140,"])
        self.assertFalse(v["measured"])
        self.assertIsNone(v["maxBehind"])
        self.assertEqual(1, v["unanswered"])

    def test_the_rows_carry_it_when_it_is_there_and_are_silent_when_it_is_not(self):
        r = followerlog.read(IDENT + COMMITTEE + AGREE)
        v = followerlog.verdict(r, FOL, VALS, None)
        names = [k for k, _ in followerlog.rows(v)]
        self.assertNotIn("follower behind the validators (blocks, max over the run)",
                         names)
        v["behind"] = followerlog.behind_summary(self.CSV)
        text = followerlog.render(v)
        self.assertIn("5, at 2026-09-19T18:00:00Z on Directory", text)


class Rendering(unittest.TestCase):
    def test_the_report_names_the_quantity_and_says_absent(self):
        r = followerlog.read(IDENT + COMMITTEE + AGREE)
        text = followerlog.render(followerlog.verdict(r, FOL, VALS, None))
        self.assertIn("acc-bvn3-fol1", text)
        self.assertIn("— not measured", text,
                      "the Debug-only drop counts must say so")
        self.assertIn("root/BPT mismatches (#)", text)
        self.assertIn("partitions BVN3, Directory", text)
        drops = [ln for ln in text.splitlines()
                  if ln.startswith("| headers dropped")]
        self.assertEqual(1, len(drops))
        self.assertIn("— not measured", drops[0],
                      "a line the build does not emit is not a zero")

    def test_a_run_with_no_follower_lines_at_all_says_so(self):
        text = followerlog.render(followerlog.verdict(
            followerlog.read([]), FOL, VALS, None))
        self.assertIn("— not measured", text)


if __name__ == "__main__":
    unittest.main()
