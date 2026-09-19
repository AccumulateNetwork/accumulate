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

def anchor(node, ts, block, dest, root, bpt):
    return ("%s  | %s INFO Sending an anchor module=conductor block=%d "
            "destination=%s seq=%d root=%s bpt=%s"
            % (node, ts, block, dest, block, root, bpt))


# The REAL shape, from
# runs/20260917T212457Z/node-logs-live.txt, acc-bvn1-val1, block 500:
#
#   21:34:26Z ... destination=acc://bvn-BVN1.acme root=4597dc3a bpt=6fcdbe82
#   21:34:26Z ... destination=acc://bvn-BVN2.acme root=4597dc3a bpt=6fcdbe82
#   21:34:26Z ... destination=acc://bvn-BVN3.acme root=4597dc3a bpt=6fcdbe82
#   21:34:26Z ... destination=acc://dn.acme      root=4597dc3a bpt=6fcdbe82
#   21:34:34Z ... destination=acc://dn.acme      root=96c2b37b bpt=d2ce8d05
#
# The Directory engine anchors to every partition INCLUDING ITSELF —
# conductor.go:283-288 iterates Network.Partitions and init.go:203 seeds
# that list with the Directory — so four of those five lines are the DN's
# and only the last is the node's BVN engine. The fixture used to carry one
# dn.acme line per node per block and call it the BVN's, which is why the
# suite passed over a reader that filed half the Directory's anchors under
# BVN3 (H1).
BVNS = ["BVN1", "BVN2", "BVN3"]


def dn_engine(node, ts, block, root, bpt):
    """The four lines one node's Directory engine logs for one DN block, at
    one instant, with one (root, bpt)."""
    return [anchor(node, ts, block, "acc://%s.acme" % d, root, bpt)
            for d in ["dn"] + ["bvn-%s" % b for b in BVNS]]


def bvn_engine(node, ts, block, root, bpt):
    """The one line a node's BVN engine logs for one BVN block."""
    return [anchor(node, ts, block, "acc://dn.acme", root, bpt)]


def _run(nodes, blocks=3, dn_first=True):
    """A log in the real shape: per node, per block, a DN self-anchor group
    and a BVN anchor, with distinct roots."""
    out = []
    for n in nodes:
        for i in range(blocks):
            # Both engines at the same block number, as this network runs
            # them (reading-a-run.md: "both reach the same block numbers at
            # the same second"), with their own roots.
            d = dn_engine(n, "2026-09-19T18:01:%02d" % i, 100 + i,
                          "dr%02d" % i, "db%02d" % i)
            b = bvn_engine(n, "2026-09-19T18:01:%02d" % (i + 30), 100 + i,
                           "r%02d" % i, "b%02d" % i)
            out += (d + b) if dn_first else (b + d)
    return out


AGREE = _run(VALS + [FOL])


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
        self.assertEqual(6, v["compared"], "3 BVN3 blocks and 3 Directory blocks")
        self.assertEqual([], v["mismatches"])
        self.assertIsNone(v["firstMismatch"])

    def test_the_source_partition_is_taken_from_the_destination(self):
        """reading-a-run.md: never compare without the partition. A BVN's
        anchors go only to the Directory and the Directory's go everywhere,
        so a block number alone names two different blocks."""
        r = followerlog.read(IDENT + AGREE)
        self.assertEqual({("BVN3", 100), ("BVN3", 101), ("BVN3", 102),
                          ("Directory", 100), ("Directory", 101),
                          ("Directory", 102)},
                         set(r.anchor_blocks(FOL)))

    def test_one_differing_root_is_named_with_its_block(self):
        bad = list(AGREE)
        bad += bvn_engine(FOL, "2026-09-19T18:02:00Z", 103,
                          "DEAD", "b03")
        bad += bvn_engine(VALS[0], "2026-09-19T18:02:00Z", 103,
                          "r03", "b03")
        v = followerlog.compare_roots(followerlog.read(IDENT + bad), FOL, VALS)
        self.assertEqual(1, len(v["mismatches"]))
        self.assertEqual(("BVN3", 103), v["firstMismatch"][:2])
        self.assertIn("DEAD", str(v["firstMismatch"]))

    def test_a_differing_bpt_counts_too(self):
        bad = list(AGREE)
        bad += bvn_engine(FOL, "2026-09-19T18:02:00Z", 104,
                          "r04", "DEAD")
        bad += bvn_engine(VALS[0], "2026-09-19T18:02:00Z", 104,
                          "r04", "b04")
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
        extra = AGREE + bvn_engine(FOL, "2026-09-19T18:03:00Z", 199, "r99", "b99")
        v = followerlog.compare_roots(followerlog.read(IDENT + extra), FOL, VALS)
        self.assertEqual([], v["mismatches"])
        self.assertEqual(6, v["compared"])
        self.assertEqual(1, v["uncompared"])


class TheDirectoryAnchorsToItself(unittest.TestCase):
    """H1. The Directory anchors to EVERY partition including itself, so a
    `destination=acc://dn.acme` line is the node's BVN engine's only when it
    is not one of the Directory's own four.

    Proven on runs/20260917T212457Z: acc-bvn1-val1 logged 1156 lines to each
    BVN and 2316 to dn — 1156 DN self-anchors plus ~1160 BVN1 anchors — and
    at block 500 it logged dn.acme twice, eight seconds apart, with
    different roots. Filing both under the node's BVN, last write wins, can
    report a mismatch on a block two nodes agree on, or agreement on a block
    where they differ. Which of the two depends only on log order, which is
    why the original suite passed.
    """

    def test_the_dn_self_anchor_is_filed_under_the_directory(self):
        lines = IDENT + dn_engine(FOL, "2026-09-19T18:00:26Z", 500,
                                  "4597dc3a", "6fcdbe82") \
                      + bvn_engine(FOL, "2026-09-19T18:00:34Z", 500,
                                   "96c2b37b", "d2ce8d05")
        a = followerlog.read(lines).anchors[FOL]
        self.assertEqual(("4597dc3a", "6fcdbe82"), a[("Directory", 500)])
        self.assertEqual(("96c2b37b", "d2ce8d05"), a[("BVN3", 500)],
                         "the node's own BVN engine, not the Directory's copy")

    def test_log_order_does_not_change_the_filing(self):
        """The bug was last-write-wins over two lines that land on one key."""
        early = dn_engine(FOL, "2026-09-19T18:00:26Z", 500, "DD", "DB")
        late = bvn_engine(FOL, "2026-09-19T18:00:34Z", 500, "BB", "BP")
        first = followerlog.read(IDENT + early + late).anchors[FOL]
        second = followerlog.read(IDENT + late + early).anchors[FOL]
        self.assertEqual(first, second)
        self.assertEqual(("BB", "BP"), first[("BVN3", 500)])

    def test_a_false_mismatch_is_not_produced_when_the_engines_interleave(self):
        """The follower is precisely the node whose engine timing is expected
        to differ. A validator with its DN line last and a follower with its
        BVN line last agree on both partitions; the old model called it a
        root mismatch on BVN3."""
        v_lines = (bvn_engine(VALS[0], "2026-09-19T18:00:20Z", 500, "BB", "BP")
                   + dn_engine(VALS[0], "2026-09-19T18:00:26Z", 500, "DD", "DB"))
        f_lines = (dn_engine(FOL, "2026-09-19T18:00:26Z", 500, "DD", "DB")
                   + bvn_engine(FOL, "2026-09-19T18:00:34Z", 500, "BB", "BP"))
        v = followerlog.compare_roots(
            followerlog.read(IDENT + v_lines + f_lines), FOL, VALS)
        self.assertEqual([], v["mismatches"],
                         "both nodes agree on BVN3 500 and on Directory 500")
        self.assertEqual(2, v["compared"])

    def test_a_real_bvn_divergence_is_not_hidden_by_the_dn_copy(self):
        """The other half of the bug: with the DN line last on both nodes,
        both (BVN3, 500) entries held the DIRECTORY's root, so a BVN3
        divergence scored as agreement while `compared` climbed."""
        v_lines = (bvn_engine(VALS[0], "2026-09-19T18:00:20Z", 500, "GOOD", "GP")
                   + dn_engine(VALS[0], "2026-09-19T18:00:26Z", 500, "DD", "DB"))
        f_lines = (bvn_engine(FOL, "2026-09-19T18:00:20Z", 500, "FORK", "FP")
                   + dn_engine(FOL, "2026-09-19T18:00:26Z", 500, "DD", "DB"))
        v = followerlog.compare_roots(
            followerlog.read(IDENT + v_lines + f_lines), FOL, VALS)
        self.assertEqual(1, len(v["mismatches"]))
        self.assertEqual(("BVN3", 500), v["firstMismatch"][:2])
        self.assertEqual(("FORK", "FP"), v["firstMismatch"][2])

    def test_the_engines_need_not_be_at_the_same_block(self):
        """The rule is taken over the node's whole log, not over one block:
        the two engines usually reach the same block number at the same
        second, and "usually" is not a rule to file evidence by."""
        lines = IDENT + dn_engine(FOL, "2026-09-19T18:00:26Z", 500, "DD", "DB") \
                      + bvn_engine(FOL, "2026-09-19T18:00:34Z", 499, "BB", "BP")
        a = followerlog.read(lines).anchors[FOL]
        self.assertEqual(("DD", "DB"), a[("Directory", 500)])
        self.assertEqual(("BB", "BP"), a[("BVN3", 499)])

    def test_a_node_that_never_anchored_to_a_bvn_is_ambiguous(self):
        """A log fragment that lost the Directory engine's lines: nothing in
        it distinguishes the two engines, so it is counted and NOT compared —
        never guessed (REPORTING-SPEC 1)."""
        lines = IDENT + bvn_engine(FOL, "2026-09-19T18:00:34Z", 777, "X", "Y")
        r = followerlog.read(lines)
        self.assertEqual({}, r.anchors.get(FOL, {}))
        self.assertEqual(1, r.ambiguous.get(FOL))

    def test_the_directorys_own_copies_agree_with_each_other(self):
        """Four lines, one key: they carry identical values by construction,
        so this must not read as the node contradicting itself."""
        r = followerlog.read(IDENT + dn_engine(FOL, "2026-09-19T18:00:26Z",
                                               500, "DD", "DB"))
        self.assertEqual([], r.conflicts)

    def test_a_node_that_contradicts_itself_is_a_finding(self):
        """Two different values for one (node, partition, block) is not
        something log order should silently resolve."""
        lines = (IDENT
                 + dn_engine(FOL, "2026-09-19T18:00:26Z", 500, "DD", "DB")
                 + [anchor(FOL, "2026-09-19T18:00:27Z", 500,
                           "acc://bvn-BVN1.acme", "OTHER", "DB")])
        r = followerlog.read(lines)
        self.assertEqual(1, len(r.conflicts))
        self.assertEqual((FOL, "Directory", 500), r.conflicts[0][:3])


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
    the run used the inactive-key form or the absent-key form.

    M5: the form is decided by looking up the FOLLOWER'S OWN key, never by
    "is there any inactive entry". The first version said "inactive in the
    definition" whenever any entry was inactive and "absent" otherwise — so
    if init misbehaved and the follower's key came out ACTIVE, which is the
    harness's premise failing, the row read "absent from the definition",
    the reassuring answer, beside a count that said BVN3 5.
    """

    FOLKEY = "ff99" + "0" * 60
    DEF = {"network": {"validators": [
        {"publicKey": "aa" * 32, "partitions": [
            {"id": "Directory", "active": True}, {"id": "BVN3", "active": True}]},
        {"publicKey": "bb" * 32, "partitions": [
            {"id": "Directory", "active": True}, {"id": "BVN3", "active": True}]},
        {"publicKey": FOLKEY, "partitions": [
            {"id": "Directory", "active": False}, {"id": "BVN3", "active": False}]},
    ]}}

    def test_the_followers_own_key_is_looked_up(self):
        v = followerlog.definition_check(self.DEF, "ff990000")
        self.assertEqual({"Directory": 2, "BVN3": 2}, v["active"])
        self.assertEqual(self.FOLKEY, v["followerKey"])
        self.assertEqual("inactive in the definition", v["followerKeyForm"])

    def test_the_followers_key_missing_is_absent_from_the_definition(self):
        d = {"network": {"validators": self.DEF["network"]["validators"][:2]}}
        v = followerlog.definition_check(d, "ff990000")
        self.assertIsNone(v["followerKey"])
        self.assertEqual("absent from the definition", v["followerKeyForm"])

    def test_an_ACTIVE_follower_key_says_so_and_does_not_read_as_absent(self):
        """The premise of the whole gate failing must be the loudest row on
        the page, not the most reassuring one."""
        d = {"network": {"validators": [
            dict(self.DEF["network"]["validators"][0]),
            {"publicKey": self.FOLKEY, "partitions": [
                {"id": "Directory", "active": False},
                {"id": "BVN3", "active": True}]}]}}
        v = followerlog.definition_check(d, "ff990000")
        self.assertIn("ACTIVE", v["followerKeyForm"])
        self.assertIn("BVN3", v["followerKeyForm"])
        self.assertFalse(v["followerInNoCommittee"])

    def test_another_inactive_entry_does_not_answer_for_the_follower(self):
        """Exactly the confusion M5 names: an inactive key that is not the
        follower's says nothing about the follower's."""
        d = {"network": {"validators": [
            {"publicKey": "cc" * 32, "partitions": [
                {"id": "Directory", "active": False}]},
            {"publicKey": "aa" * 32, "partitions": [
                {"id": "Directory", "active": True}]}]}}
        v = followerlog.definition_check(d, "ff990000")
        self.assertEqual("absent from the definition", v["followerKeyForm"])
        self.assertEqual(["cc" * 32], v["inactiveKeys"],
                         "still reported, but it is not the follower's")

    def test_without_the_followers_key_the_form_is_not_measured(self):
        """No identity line in the log means the key is unknown; the counts
        are still real, the form is not."""
        v = followerlog.definition_check(self.DEF, None)
        self.assertTrue(v["measured"])
        self.assertEqual({"Directory": 2, "BVN3": 2}, v["active"])
        self.assertIsNone(v["followerKeyForm"])
        self.assertIn("key is not known", v["why"])

    def test_two_entries_matching_the_prefix_is_a_finding(self):
        d = {"network": {"validators": [
            {"publicKey": "ff99" + "1" * 60, "partitions": []},
            {"publicKey": "ff99" + "2" * 60, "partitions": []}]}}
        v = followerlog.definition_check(d, "ff990000"[:4] + "0000")
        v = followerlog.definition_check(d, "ff99")
        self.assertIn("2 entries", v["followerKeyForm"])
        self.assertIsNone(v["followerInNoCommittee"])

    def test_an_unreadable_definition_is_absent_not_empty(self):
        v = followerlog.definition_check(None, "ff990000")
        self.assertFalse(v["measured"])
        self.assertIsNone(v["followerKeyForm"])

    def test_the_verdict_passes_the_key_through(self):
        r = followerlog.read(IDENT)
        v = followerlog.verdict(r, FOL, VALS, self.DEF)
        self.assertEqual("ffff9999", r.key_prefix(FOL))
        self.assertEqual("absent from the definition",
                         v["definition"]["followerKeyForm"],
                         "IDENT's key is ffff9999..., not in this definition")


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
        self.assertEqual(6, from_list["compared"])
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
        self.assertEqual(6, v["compared"])


class BehindSeries(unittest.TestCase):
    """The `behind` series out of follower.csv — the manifest's "max over the
    run and when"."""

    CSV = [
        "time,follower,partition,followerHeight,validatorsMaxHeight,"
        "behindBlocks,maxBehindRunBlocks",
        "2026-09-19T18:00:00Z,acc-bvn3-fol1,BVN3,100,101,1,1",
        "2026-09-19T18:00:00Z,acc-bvn3-fol1,Directory,100,105,5,5",
        "2026-09-19T18:00:20Z,acc-bvn3-fol1,BVN3,120,120,0,1",
        "2026-09-19T18:00:20Z,acc-bvn3-fol1,Directory,120,121,1,5",
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
            "2026-09-19T18:00:40Z,acc-bvn3-fol1,Directory,,140,,5"])
        self.assertEqual(1, v["unanswered"])
        self.assertEqual(5, v["maxBehind"])
        self.assertEqual(4, v["samples"])

    def test_a_csv_with_no_answered_sample_is_absent_not_zero(self):
        v = followerlog.behind_summary([self.CSV[0],
                                        "2026-09-19T18:00:40Z,f,Directory,,140,,"])
        self.assertFalse(v["measured"])
        self.assertIsNone(v["maxBehind"])
        self.assertEqual(1, v["unanswered"])

    def test_an_excursion_between_two_csv_samples_is_not_lost(self):
        """M4. The board ticks every second and the CSV is written every
        thirty; a spike in between reached the board's `fmax` and never the
        manifest, under one label. The row carries the monitor's own
        high-water mark, and that is what the manifest states."""
        csv = [self.CSV[0],
               "2026-09-19T18:00:00Z,acc-bvn3-fol1,Directory,100,101,1,1",
               # between these two samples the follower fell 40 behind
               "2026-09-19T18:00:30Z,acc-bvn3-fol1,Directory,200,200,0,40"]
        v = followerlog.behind_summary(csv)
        self.assertEqual(40, v["maxBehind"])
        self.assertEqual("the monitor's high-water mark over every tick",
                         v["maxSource"])
        self.assertEqual({"Directory": 0}, v["endBehind"])

    def test_an_older_csv_without_the_column_still_reads(self):
        """A run directory from before this column exists must not become
        unreadable; the per-sample maximum is then what is available and the
        report says which it used."""
        old = ["time,follower,partition,followerHeight,validatorsMaxHeight,behindBlocks",
               "2026-09-19T18:00:00Z,acc-bvn3-fol1,Directory,100,105,5",
               "2026-09-19T18:00:30Z,acc-bvn3-fol1,Directory,200,200,0"]
        v = followerlog.behind_summary(old)
        self.assertEqual(5, v["maxBehind"])
        self.assertEqual("the largest of the samples written to follower.csv",
                         v["maxSource"])

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
