#!/usr/bin/env python3
"""Rejoined is executing with the partition, not a gauge (#4404, items 1 and 2).

Driven with synthetic series shaped on run 20260924T052134Z, through the
monitor's own code (soakmon.nodestate_from and track_start_to_active write
nodestate.csv) and the manifest's (rejoin.py reads it and the log):

    acc-bvn1-val1 bvn1       gauge ACTIVE 12.1 s after its start; its anchor at
                             block 204 differs from its peers'; stuck at 214 while
                             the partition reached 1376 (1364 in this series)
    acc-bvn3-val1 directory  gauge ACTIVE 26.5 s; never executed past 613, peers 1364
    acc-bvn2-val2 directory  gauge ACTIVE 29.4 s; executed 1300 vs 1364, and signed
                             seq 1154 as block 1300 root f5b4979b where its peers'
                             seq 1154 is block 1301 root de98b6c8
    acc-bvn1-val1 directory  the one that did rejoin: caught up, stayed, agreed

and against the run's own nodestate.csv, which the old row read as "worst
29.4 s over 4 start(s)" — every one of those four reaching ACTIVE.
"""
import csv
import os
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
sys.path.insert(0, os.path.dirname(HERE))
import heights
import rejoin
import soakmon

RUN = os.path.join(HERE, "runs", "20260924T052134Z")
T0 = 1790227300          # an epoch near the run's start; only differences matter
VALS = ["acc-bvn%d-val%d" % (b, v) for b in (1, 2, 3) for v in (1, 2, 3, 4)]


def scrape(state, executed):
    """One node's parsed /metrics: {partition: gauge value}, {partition: block}."""
    rows = [(soakmon.NODE_STATE, {"partition": p}, v) for p, v in state.items()]
    rows += [(heights.EXECUTED, {"partition": p}, b) for p, b in executed.items()]
    return rows


class Series:
    """A network of twelve validators sampled over time, one node restarted."""

    def __init__(self):
        self.track, self.events = {}, []

    def sample(self, t, per, started):
        ns = soakmon.nodestate_from(per, now=t, disturbed=started)
        self.events += [(t, e) for e in soakmon.track_start_to_active(
            self.track, ns, started, t, max_behind=10)]

    def finish(self, t):
        self.events += [(t, e) for e in soakmon.final_starts(self.track, t)]

    def write(self, rd):
        with open(os.path.join(rd, "nodestate.csv"), "w") as f:
            f.write(soakmon.NODESTATE_CSV_HEADER + "\n")
            for t, e in self.events:
                ts = "%sZ" % __import__("time").strftime("%Y-%m-%dT%H:%M:%S", __import__("time").gmtime(t))
                for line in soakmon.nodestate_csv_rows([e], ts):
                    f.write(line + "\n")


def healthy(parts_heights, bvn):
    return scrape({"directory": 2, bvn: 2},
                  {"directory": parts_heights["directory"], bvn: parts_heights[bvn]})


def network(t, dn, bvn, restarted=None):
    """Every validator at the partition heights; `restarted` overrides one."""
    per = {}
    for c in VALS:
        b = "bvn" + c[7]
        per[c] = healthy({"directory": dn, b: bvn[b]}, b)
    if restarted:
        per.update(restarted)
    return per


def anchor_line(node, ts, src, dest, block, seq, root, bpt="bbbbbbbb", sent=True):
    msg = "Sending an anchor" if sent else "Anchor not sent"
    return ("%s  | %s INFO %s block=%d bpt=%s destination=%s module=conductor "
            "root=%s seq=%d source=%s\n" % (node, ts, msg, block, bpt, dest, root, seq, src))


def iso(t):
    import time
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(t))


class TheRunsThreeFailedStarts(unittest.TestCase):
    """Each fails the reading for its own reason, and the fourth passes."""

    def setUp(self):
        self.rd = tempfile.mkdtemp(prefix="rejoin-")
        s = Series()
        start = {c: T0 - 3600 for c in VALS}
        s.sample(T0, network(T0, 200, {"bvn1": 200, "bvn2": 200, "bvn3": 200}), start)

        # acc-bvn1-val1 restarts; both partitions boot, then the gauge goes
        # ACTIVE 12 s later while it is right on the partition's height —
        # and then its BVN1 executor stops at 214 while BVN1 climbs.
        st = dict(start, **{"acc-bvn1-val1": T0 + 10})
        booting = scrape({"directory": 0, "bvn1": 0}, {"directory": 190, "bvn1": 190})
        s.sample(T0 + 15, network(0, 205, {"bvn1": 205, "bvn2": 205, "bvn3": 205},
                                  {"acc-bvn1-val1": booting}), st)
        for i, (dn, b1) in enumerate([(210, 212), (400, 400), (900, 900), (1364, 1376)]):
            me = scrape({"directory": 2, "bvn1": 2}, {"directory": dn - 1, "bvn1": min(214, b1)})
            s.sample(T0 + 22 + 60 * i, network(0, dn, {"bvn1": b1, "bvn2": b1, "bvn3": b1},
                                               {"acc-bvn1-val1": me}), st)

        # acc-bvn3-val1 restarts; Directory reads ACTIVE at 26 s with its
        # executor at 600 against 620 and never moves past 613.
        st["acc-bvn3-val1"] = T0 + 400
        for t, ex, dn in [(T0 + 405, 600, 620), (T0 + 426, 605, 640), (T0 + 500, 613, 700),
                          (T0 + 900, 613, 1364)]:
            me = scrape({"directory": 0 if t < T0 + 426 else 2, "bvn3": 0}, {"directory": ex, "bvn3": 600})
            s.sample(t, network(0, dn, {"bvn1": dn, "bvn2": dn, "bvn3": dn},
                                {"acc-bvn3-val1": me, "acc-bvn1-val1": healthy({"directory": dn, "bvn1": 214}, "bvn1")}), st)

        # acc-bvn2-val2 restarts; Directory ACTIVE at 29 s, executes to 1300
        # against 1364 at the end.
        st["acc-bvn2-val2"] = T0 + 1000
        for t, ex, dn in [(T0 + 1005, 1200, 1250), (T0 + 1029, 1240, 1260), (T0 + 1300, 1300, 1364)]:
            me = scrape({"directory": 0 if t < T0 + 1029 else 2, "bvn2": 0}, {"directory": ex, "bvn2": 1000})
            s.sample(t, network(0, dn, {"bvn1": dn, "bvn2": dn, "bvn3": dn},
                                {"acc-bvn2-val2": me,
                                 "acc-bvn1-val1": healthy({"directory": dn, "bvn1": 214}, "bvn1"),
                                 "acc-bvn3-val1": scrape({"directory": 2, "bvn3": 0}, {"directory": 613, "bvn3": 600})}),
                     st)
        s.finish(T0 + 1400)
        s.write(self.rd)
        with open(os.path.join(self.rd, "nodestate.csv")) as f:
            self.rows = list(csv.DictReader(f))

        # The log: every validator agrees on Directory anchors through block
        # 1301; acc-bvn1-val1's BVN1 block 204 differs; acc-bvn2-val2 signs
        # seq 1154 as block 1300 with another root.
        lines = []
        for c in VALS:
            for blk, seq in [(900, 1000), (1301, 1154)]:
                lines.append(anchor_line(c, iso(T0 + 1350), "Directory", "acc://bvn-BVN1.acme",
                                         blk, seq, "de98b6c8" if blk == 1301 else "0000900a"))
            if c.startswith("acc-bvn1"):
                lines.append(anchor_line(c, iso(T0 + 100), "BVN1", "acc://dn.acme", 204, 180,
                                         "fb7a2faf" if c == "acc-bvn1-val1" else "3c2c702b"))
        lines = [l for l in lines if not (l.startswith("acc-bvn2-val2 ") and "seq=1154" in l)]
        lines.append(anchor_line("acc-bvn2-val2", iso(T0 + 1350), "Directory", "acc://bvn-BVN1.acme",
                                 1300, 1154, "f5b4979b"))
        # acc-bvn3-val1 stopped executing: it states nothing after its start.
        lines = [l for l in lines if not l.startswith("acc-bvn3-val1 ")]
        self.anchors = rejoin.Anchors(rejoin.anchor_events(lines))
        self.cell = rejoin.row(self.rows, "validator", 10, self.anchors)

    def verdict(self, node, part):
        s = rejoin.starts(self.rows)
        key = next(k for k in s if k[0] == node and k[1] == part and s[k].get("reached"))
        return rejoin.judge(key, s[key], 10, self.anchors, VALS)

    def test_the_gauge_alone_read_all_four_as_active(self):
        """The old reading: every one of these starts has a `reached` row."""
        reached = {(r["node"], r["partition"]) for r in self.rows if r["kind"] == "reached"}
        self.assertEqual({("acc-bvn1-val1", "bvn1"), ("acc-bvn1-val1", "directory"),
                          ("acc-bvn3-val1", "directory"), ("acc-bvn2-val2", "directory")},
                         reached)

    def test_bvn1_stuck_at_214_is_not_rejoined(self):
        v = self.verdict("acc-bvn1-val1", "bvn1")
        self.assertEqual("NOT rejoined", v["verdict"], v)
        self.assertIn("executed 214 vs partition 1364", " ".join(v["reasons"]))
        self.assertIn("block 204 root fb7a2faf, its peers' 3c2c702b", " ".join(v["reasons"]))

    def test_the_directory_that_never_executed_is_not_rejoined(self):
        v = self.verdict("acc-bvn3-val1", "directory")
        self.assertEqual("NOT rejoined", v["verdict"], v)
        self.assertIn("executed 613 vs partition 1364", " ".join(v["reasons"]))
        self.assertIn("never ACTIVE and within 10 blocks", " ".join(v["reasons"]))

    def test_the_directory_that_signed_another_body_is_not_rejoined(self):
        v = self.verdict("acc-bvn2-val2", "directory")
        self.assertEqual("NOT rejoined", v["verdict"], v)
        self.assertIn("seq 1154 to acc://bvn-BVN1.acme as block 1300 root f5b4979b, its peers' block 1301 root de98b6c8",
                      " ".join(v["reasons"]))

    def test_the_directory_that_did_rejoin_is_rejoined(self):
        v = self.verdict("acc-bvn1-val1", "directory")
        self.assertEqual("rejoined", v["verdict"], v)
        self.assertIsNotNone(v["toRejoinS"])

    def test_the_manifest_cell_says_so(self):
        self.assertIn("rejoined 1 of 6 start(s) after the launch", self.cell)
        for n in ("acc-bvn1-val1 bvn1", "acc-bvn3-val1 directory", "acc-bvn2-val2 directory"):
            self.assertIn(n, self.cell.split("NOT rejoined:")[1])

    def test_a_resend_storm_is_one_peer_not_three_hundred(self):
        """acc-bvn2-val2 re-sent its conflicting anchor 304 times. Voting by
        line would make its body the peers' majority and fail every node
        that agreed with the other ten."""
        storm = [anchor_line("acc-bvn2-val2", iso(T0 + 1350), "Directory", "acc://bvn-BVN1.acme",
                             1300, 1154, "f5b4979b")] * 304
        ok = [anchor_line(c, iso(T0 + 1350), "Directory", "acc://bvn-BVN1.acme", 1301, 1154, "de98b6c8")
              for c in VALS if c != "acc-bvn2-val2"]
        a = rejoin.Anchors(rejoin.anchor_events(storm + ok))
        got = a.agreement("acc-bvn1-val1", "Directory", None, None, VALS)
        self.assertEqual([], got["disagree"])


class AStartActiveAtFirstSightIsJudged(unittest.TestCase):
    """Review F1. A start the monitor first sees already ACTIVE was filed as
    the network's launch and never judged. Two routes reach it: a join that
    completes inside one scrape interval, and a restart that spans a monitor
    restart (soak.sh restarts soakmon on any exit; the new process's track is
    empty, so every node is `already`). Both: restarted at T0+10, stuck at
    320 on a divergent root while the partition reaches 1395."""

    def stuck(self, s, t, st):
        me = scrape({"directory": 2, "bvn1": 2}, {"directory": 1395, "bvn1": 320})
        s.sample(t, network(0, 1395, {"bvn1": 1395, "bvn2": 1395, "bvn3": 1395},
                            {"acc-bvn1-val1": me}), st)

    def cell(self, s):
        rd = tempfile.mkdtemp(prefix="rejoin-f1-")
        s.write(rd)
        with open(os.path.join(rd, "nodestate.csv")) as f:
            return rejoin.row(list(csv.DictReader(f)), "validator", 10, None)

    def launch(self, s):
        start = {c: T0 - 3600 for c in VALS}
        s.sample(T0, network(T0, 200, {"bvn1": 200, "bvn2": 200, "bvn3": 200}), start)
        return start

    def test_a_join_inside_one_scrape_interval(self):
        s = Series()
        st = dict(self.launch(s), **{"acc-bvn1-val1": T0 + 10})
        # First sample of the start, 5 s later: already ACTIVE, already caught up.
        me = scrape({"directory": 2, "bvn1": 2}, {"directory": 320, "bvn1": 320})
        s.sample(T0 + 15, network(0, 320, {"bvn1": 320, "bvn2": 320, "bvn3": 320},
                                  {"acc-bvn1-val1": me}), st)
        self.stuck(s, T0 + 600, st)
        s.finish(T0 + 600)
        cell = self.cell(s)
        self.assertIn("rejoined 0 of 2 start(s) after the launch", cell)
        self.assertIn("acc-bvn1-val1 bvn1 (gauge ACTIVE at ≤5s (boot time not measured", cell)
        self.assertIn("executed 320 vs partition 1395", cell)

    def test_a_restart_that_spans_a_monitor_restart(self):
        old = Series()
        st = dict(self.launch(old), **{"acc-bvn1-val1": T0 + 10})
        booting = scrape({"directory": 0, "bvn1": 0}, {"directory": 190, "bvn1": 190})
        old.sample(T0 + 15, network(0, 205, {"bvn1": 205, "bvn2": 205, "bvn3": 205},
                                    {"acc-bvn1-val1": booting}), st)
        old.finish(T0 + 16)            # the old soakmon exits: `final BOOTING`
        new = Series()                 # the supervisor's new soakmon: empty track
        new.sample(T0 + 40, network(0, 330, {"bvn1": 330, "bvn2": 330, "bvn3": 330},
                                    {"acc-bvn1-val1": scrape({"directory": 2, "bvn1": 2},
                                                             {"directory": 330, "bvn1": 320})}), st)
        self.stuck(new, T0 + 600, st)
        new.finish(T0 + 600)
        both = Series()
        both.events = old.events + new.events
        cell = self.cell(both)
        self.assertIn("NOT rejoined: acc-bvn1-val1 bvn1", cell)
        self.assertIn("executed 320 vs partition 1395", cell)
        self.assertNotIn("never ACTIVE", cell.split("acc-bvn1-val1 bvn1")[1].split(")")[0],
                         "the new process saw it ACTIVE: the old `final BOOTING` is not the reading")


class EveryDestinationIsCompared(unittest.TestCase):
    """Review F5: the seen-key was (source, seq) while the peers' reading is
    keyed (source, destination, seq), so only the first destination's line at
    a sequence number was ever compared."""

    def test_a_conflict_on_the_second_destination_is_found(self):
        lines = []
        for c in VALS:
            for dest in ("acc://bvn-BVN1.acme", "acc://bvn-BVN2.acme"):
                blk, root = 10, "aaaaaaaa"
                if c == "acc-bvn1-val1" and dest.endswith("BVN2.acme"):
                    blk, root = 11, "cccccccc"
                lines.append(anchor_line(c, iso(T0 + 50), "Directory", dest, blk, 5, root))
        a = rejoin.Anchors(rejoin.anchor_events(lines))
        got = a.agreement("acc-bvn1-val1", "Directory", None, None, VALS)
        self.assertEqual(["seq 5 to acc://bvn-BVN2.acme as block 11 root cccccccc, "
                          "its peers' block 10 root aaaaaaaa"], got["disagree"])


class TheBoardShowsActiveButBehind(unittest.TestCase):
    """The board listed only rows that were not ACTIVE, so acc-bvn1-val1 bvn1
    — ACTIVE by the gauge from 05:26:03, stuck at 214 — was never on it."""

    def test_an_active_row_far_behind_its_partition_is_named(self):
        per = network(0, 1364, {"bvn1": 1376, "bvn2": 1376, "bvn3": 1376},
                      {"acc-bvn1-val1": scrape({"directory": 2, "bvn1": 2},
                                               {"directory": 1364, "bvn1": 214})})
        ns = soakmon.nodestate_from(per, now=T0)
        lag = [r for r in ns["rows"] if r.get("lagging")]
        self.assertEqual([("acc-bvn1-val1", "bvn1")], [(r["node"], r["partition"]) for r in lag])
        self.assertIn("ACTIVE by the gauge, executed 214 vs partition 1376 (1162 behind; bound 10)",
                      lag[0]["why"])
        self.assertEqual(1, ns["lagging"])


class TheRunsOwnNodestateCsv(unittest.TestCase):
    """nodestate.csv as run 20260924T052134Z wrote it (no executed columns)."""

    def setUp(self):
        with open(os.path.join(RUN, "nodestate.csv")) as f:
            self.rows = list(csv.DictReader(f))

    def test_the_old_reading_had_four_starts_reach_active(self):
        self.assertEqual(4, sum(1 for r in self.rows
                                if r["kind"] == "reached" and r["role"] == "validator"))

    def test_the_new_reading_rejoins_none_of_them(self):
        """Without the executed columns nothing is established — and nothing
        is called rejoined on the gauge's word."""
        cell = rejoin.row(self.rows, "validator", 10, None)
        self.assertIn("rejoined 0 of 6 start(s) after the launch", cell)
        self.assertIn("nodestate.csv predates #4404", cell)
        self.assertIn("NOT rejoined: acc-bvn2-val2 bvn2 (never ACTIVE (BOOTING)", cell)

    def test_the_follower_row_says_nothing_of_starts_that_did_not_happen(self):
        cell = rejoin.row(self.rows, "follower", 10, None)
        self.assertEqual("no start after the network's launch; 2 started before the monitor's "
                         "first sample (the network's launch: not judged)", cell)


class ThePartitionsHeight(unittest.TestCase):
    """monitor.csv's Directory height is the partition's, not the restarted
    node's (#4404 item 2). Run 20260924T052134Z's column sat at 207 from
    05:26:19 to 05:27:24 while every other Directory node went 215 -> 323."""

    def executed(self, restarted_at, others_at):
        ex = {c: {"directory": others_at} for c in VALS}
        ex["acc-bvn1-val1"] = {"directory": restarted_at}
        return ex

    def test_the_restarted_node_does_not_hold_the_column(self):
        for others in (215, 275, 323):
            got = heights.partition_heights(self.executed(207, others), VALS)["directory"]
            self.assertEqual({"height": others, "answered": 12}, got)

    def bvn1(self, **heights_by_node):
        """BVN1's four validators; a node absent from the kwargs did not answer."""
        per = {}
        for c in ("acc-bvn1-val1", "acc-bvn1-val2", "acc-bvn1-val3", "acc-bvn1-val4"):
            k = c.replace("-", "_")
            if k in heights_by_node:
                per[c] = scrape({"directory": 2, "bvn1": 2},
                                {"directory": 1000, "bvn1": heights_by_node[k]})
        return soakmon.nodestate_from(per, now=T0)

    def row_of(self, ns, node):
        return next(r for r in ns["rows"] if r["node"] == node and r["partition"] == "bvn1")

    def test_a_stuck_node_is_not_the_partition_when_others_are_missing(self):
        """Review F2 case F: val1 stuck at 214, val2 paused, val3 missed the
        scrape. The majority of the answering set [1000, 214] was 214 and
        val1 read caught up against its own height."""
        ns = self.bvn1(acc_bvn1_val1=214, acc_bvn1_val4=1000)
        r = self.row_of(ns, "acc-bvn1-val1")
        self.assertEqual(1000, r["partitionHeight"])
        self.assertEqual(786, r["behind"])
        self.assertTrue(r["lagging"])
        self.assertEqual(2, r["validatorsAnswered"])

    def test_two_stuck_of_four_are_both_flagged(self):
        ns = self.bvn1(acc_bvn1_val1=214, acc_bvn1_val2=214, acc_bvn1_val3=1000, acc_bvn1_val4=1000)
        self.assertEqual({"acc-bvn1-val1", "acc-bvn1-val2"},
                         {r["node"] for r in ns["rows"] if r.get("lagging")})

    def test_the_height_does_not_go_backwards_as_the_answering_set_shrinks(self):
        seen = [heights.partition_heights({c: {"bvn1": b} for c, b in zip(VALS[:4], xs)},
                                          VALS[:4])["bvn1"]["height"]
                for xs in ([1000, 1000, 214, 1000], [1000, 214], [1000, 1000, 214, 214],
                           [1010, 214])]
        self.assertEqual([1000, 1000, 1000, 1010], seen)

    def test_the_row_carries_every_node_in_its_own_column(self):
        cols = heights.columns(followers=["acc-bvn3-fol1"])
        ex = self.executed(207, 323)
        ex["acc-bvn1-val1"]["bvn1"] = 214
        head = heights.header(cols).split(",")
        body = heights.row(ex, VALS, "154", "600", "0", cols).split(",")
        self.assertEqual(len(head), len(body))
        got = dict(zip(head, body))
        self.assertEqual("323", got["dnHeightMax"])
        self.assertEqual("12", got["dnValidatorsAnswered"])
        self.assertEqual("207", got["exec.acc-bvn1-val1.directory"])
        self.assertEqual("214", got["exec.acc-bvn1-val1.bvn1"])
        self.assertEqual("", got["exec.acc-bvn3-fol1.bvn3"],
                         "a node that did not answer is blank, never 0")
        self.assertNotIn("exec.acc-bvn3-fol2.bvn3", got, "a follower the run does not have has no column")

    def test_the_prom_text_is_read_per_partition(self):
        text = ('# HELP x\naccumulate_node_executed_block{partition="directory"} 1364\n'
                'accumulate_node_executed_block{partition="bvn1"} 214\n'
                'accumulate_node_state{partition="bvn1"} 2\n')
        self.assertEqual({"directory": 1364, "bvn1": 214}, heights.executed_from_prom(text))

    def test_the_monitor_loop_no_longer_asks_one_node(self):
        with open(os.path.join(HERE, "soak.sh")) as f:
            src = f.read()
        loop = src[src.index("# Monitor: heights"):src.index("MONLOOP=$!")]
        self.assertFalse("localhost:26680" in loop,
                         "the monitor's Directory height must not be one node's ledger index")
        self.assertTrue('heights.py" row' in loop)


if __name__ == "__main__":
    unittest.main()
