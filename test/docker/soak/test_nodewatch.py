# Copyright 2026 The Accumulate Authors
#
# Use of this source code is governed by an MIT-style
# license that can be found in the LICENSE file or at
# https://opensource.org/licenses/MIT.
"""nodewatch's readings and its verdicts, held to real bytes.

Every fixture here is a verbatim capture from the twelve-node network of
2026-09-18, on which acc-bvn1-val1 was stopped at block 81 and never
rejoined while its eleven peers ran on past block 800. The log lines keep
their ANSI colour, because that colour is in the bytes `docker logs`
actually returns and stripping it is part of the reading.

The point of these tests is the one thing a monitor can get wrong that
costs a run: calling a stuck node healthy, or calling a healthy node stuck.
"""

import os
import sys
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
sys.path.insert(0, os.path.join(HERE, ".."))
import nodewatch  # noqa: E402

E = "\x1b"

# Verbatim from `docker logs acc-bvn1-val1`, colour and all.
WEDGED_LOG = (
    E + "[90m2026-09-18T22:59:11Z" + E + "[0m INFO Joining: collecting committed "
    "blocks, executing none " + E + "[36mbuffered=" + E + "[0m768 " + E +
    "[36mlastBlock=" + E + "[0m76 " + E + "[36mmodule=" + E + "[0mdagbft " + E +
    "[36mpartition=" + E + "[0mBVN1 " + E + "[36mround=" + E + "[0m1713 " + E +
    "[36mstalledFor=" + E + "[0m724118000000 " + E + "[36mthreshold=" + E +
    "[0m10000000000\n"
    + E + "[90m2026-09-18T22:59:13Z" + E + "[0m INFO Joining: collecting committed "
    "blocks, executing none " + E + "[36mbuffered=" + E + "[0m728 " + E +
    "[36mlastBlock=" + E + "[0m81 " + E + "[36mmodule=" + E + "[0mdagbft " + E +
    "[36mpartition=" + E + "[0mDirectory " + E + "[36mround=" + E + "[0m1700 " + E +
    "[36mstalledFor=" + E + "[0m729102000000 " + E + "[36mthreshold=" + E +
    "[0m10000000000\n"
    '2026-09-18T22:57:27Z INFO A pulled account did not verify error={"code":'
    '"conflict","codeID":409,"message":"acc://dn.acme/anchors: the state served '
    'does not hash into the anchored root"} account=acc://dn.acme/anchors '
    "module=join\n"
)

# Verbatim from the wedged node's :26670 scrape.
WEDGED_METRICS = """# HELP accumulate_node_state This node's state for the partition: 0 booting, 1 waiting, 2 active, 3 complete
# TYPE accumulate_node_state gauge
accumulate_node_state{partition="bvn1"} 0
accumulate_node_state{partition="directory"} 0
accumulate_dagbft_execution_lag_blocks{partition="BVN1"} 609
accumulate_dagbft_execution_lag_blocks{partition="Directory"} 567
accumulate_exec_blocks_total 0
go_goroutines 274
"""

# Verbatim from a healthy peer's scrape. accumulate_node_state DOES NOT
# APPEAR: the gauge is only registered while a node is in the join state
# machine. This absence is the fixture -- a monitor that reads a missing
# gauge as 0 would paint all eleven healthy nodes "booting".
HEALTHY_METRICS = """accumulate_dagbft_execution_lag_blocks{partition="BVN2"} 0
accumulate_dagbft_execution_lag_blocks{partition="Directory"} 0
accumulate_exec_blocks_total 1353
accumulate_dagbft_blocks_produced_total 1353
go_goroutines 296
"""


class Rules:
    """Thresholds, standing in for a Watch without touching docker."""
    wedge_secs = 45.0
    behind_blocks = 5


def node(short="bvn1-val1", bvn="BVN1", running=True, metrics_ok=True, per=None,
         join_error=None, restarts=0):
    return {"short": short, "bvn": bvn, "running": running,
            "containerStatus": "Up 10 minutes" if running else "absent",
            "metricsOK": metrics_ok, "per": per or {}, "joinError": join_error,
            "restarts": restarts, "container": "acc-" + short, "port": 26680,
            "parts": ["Directory", bvn], "heights": {}, "join": {}}


def part(height, behind, blocks_window=None, idle=None, idle_measured=False,
         node_state=None, lag=None, join=None):
    return {"height": height, "behind": behind, "blocksWindow": blocks_window,
            "windowSecs": 10.0, "blocksSinceStart": None, "sinceStartSecs": 0.0,
            "idleSecs": idle, "idleMeasured": idle_measured,
            "nodeState": node_state, "execLagBlocks": lag, "join": join}


class TestReadings(unittest.TestCase):
    def test_join_line_survives_ansi_inside_the_key(self):
        """The colour is inside the key: '\\e[36mbuffered=\\e[0m768'. A
        parser that strips colour only at the ends of a line reads no
        fields at all and reports a joining node as having no join state."""
        j = nodewatch.parse_join_logs(WEDGED_LOG)
        self.assertEqual(j["BVN1"]["buffered"], 768)
        self.assertEqual(j["BVN1"]["lastBlock"], 76)
        self.assertEqual(j["Directory"]["lastBlock"], 81)
        self.assertEqual(j["Directory"]["round"], 1700)

    def test_stalled_for_is_converted_from_nanoseconds(self):
        """stalledFor=724118000000 is 724 seconds, not 724 billion of
        anything. A number with the wrong unit is the whole defect."""
        j = nodewatch.parse_join_logs(WEDGED_LOG)
        self.assertAlmostEqual(j["BVN1"]["stalledForSecs"], 724.1, places=1)
        self.assertAlmostEqual(j["Directory"]["stalledForSecs"], 729.1, places=1)

    def test_last_join_error_is_kept(self):
        j = nodewatch.parse_join_logs(WEDGED_LOG)
        self.assertIn("does not hash into the anchored root", j["_error"])

    def test_healthy_log_has_no_join_state(self):
        healthy = ("2026-09-18T22:56:41Z INFO Block execution accounting "
                   "arrived=8 batches=8 block=659 executed=8 round=1398\n")
        self.assertEqual(nodewatch.parse_join_logs(healthy), {})

    def test_partition_spellings_collapse_to_one(self):
        for spelling in ("bvn1", "BVN1", "acc://bvn-BVN1.acme"):
            self.assertEqual(nodewatch.canon_part(spelling), "BVN1")
        for spelling in ("dn", "directory", "Directory", "acc://dn.acme"):
            self.assertEqual(nodewatch.canon_part(spelling), "Directory")

    def test_node_state_read_per_partition(self):
        m = nodewatch.metrics_of(nodewatch.parse_prom(WEDGED_METRICS))
        self.assertEqual(m["nodeState"], {"BVN1": 0, "Directory": 0})
        self.assertEqual(m["execLag"], {"BVN1": 609, "Directory": 567})
        self.assertEqual(m["execBlocksTotal"], 0)

    def test_absent_node_state_stays_absent(self):
        """Healthy nodes do not export accumulate_node_state at all. Absent
        must not become 0 ('booting'), which would flip the whole fleet to
        red, nor 2 ('active'), which would hide a real wedge."""
        m = nodewatch.metrics_of(nodewatch.parse_prom(HEALTHY_METRICS))
        self.assertEqual(m["nodeState"], {})
        self.assertNotIn("Directory", m["nodeState"])


class TestVerdicts(unittest.TestCase):
    def test_wedged_node_is_wedged_on_the_first_sample(self):
        """No history, no window -- and it must still read WEDGED, because
        the node itself reports how long it has been unable to execute."""
        j = nodewatch.parse_join_logs(WEDGED_LOG)
        n = node(per={
            "Directory": part(81, 722, node_state=0, lag=567, join=j["Directory"]),
            "BVN1": part(76, 764, node_state=0, lag=609, join=j["BVN1"]),
        }, join_error=j["_error"])
        state, why = nodewatch.classify(n, {"Directory": 803, "BVN1": 840},
                                        Rules(), 0)
        self.assertEqual(state, nodewatch.ST_WEDGED)
        self.assertIn("729", why)

    def test_joining_and_moving_is_joining_not_wedged(self):
        """A node that is catching up looks like a wedged one in a single
        height reading. It is not, and calling it WEDGED would send someone
        after a non-defect."""
        j = {"buffered": 40, "lastBlock": 700, "round": 9, "stalledForSecs": 3.0}
        n = node(per={
            "Directory": part(700, 103, blocks_window=57, idle=0.5,
                              idle_measured=True, node_state=1, join=j),
            "BVN1": part(740, 100, blocks_window=60, idle=0.5,
                         idle_measured=True, node_state=1, join=j),
        })
        state, _ = nodewatch.classify(n, {"Directory": 803, "BVN1": 840},
                                      Rules(), 0)
        self.assertEqual(state, nodewatch.ST_JOINING)

    def test_a_node_that_just_went_active_is_not_still_joining(self):
        """The log tail is 30 s deep, so a node that finished joining ten
        seconds ago still shows join lines. The gauge says active; a
        display that keeps calling it JOINING is reporting the past."""
        j = {"buffered": 0, "lastBlock": 840, "round": 9, "stalledForSecs": 1.0}
        n = node(per={
            "Directory": part(803, 0, blocks_window=10, idle=0.5,
                              idle_measured=True, node_state=2, join=j),
            "BVN1": part(840, 0, blocks_window=10, idle=0.5,
                         idle_measured=True, node_state=2, join=j),
        })
        state, _ = nodewatch.classify(n, {"Directory": 803, "BVN1": 840},
                                      Rules(), 0)
        self.assertEqual(state, nodewatch.ST_EXEC)

    def test_healthy_node_is_exec(self):
        n = node(short="bvn2-val1", bvn="BVN2", per={
            "Directory": part(803, 0, blocks_window=10, idle=0.4, idle_measured=True),
            "BVN2": part(856, 0, blocks_window=11, idle=0.3, idle_measured=True),
        })
        state, _ = nodewatch.classify(n, {"Directory": 803, "BVN2": 856},
                                      Rules(), 0)
        self.assertEqual(state, nodewatch.ST_EXEC)

    def test_a_node_far_behind_is_never_exec(self):
        """The routed-query trap, as a rule: whatever else it reports, a
        node 764 blocks behind its partition cannot be displayed green."""
        n = node(per={
            "Directory": part(81, 722, blocks_window=0, idle=2.0, idle_measured=False),
            "BVN1": part(76, 764, blocks_window=0, idle=2.0, idle_measured=False),
        })
        state, _ = nodewatch.classify(n, {"Directory": 803, "BVN1": 840},
                                      Rules(), 0)
        self.assertNotEqual(state, nodewatch.ST_EXEC)
        self.assertEqual(state, nodewatch.ST_BEHIND)

    def test_quiet_moment_is_not_a_stall(self):
        """Zero blocks in a short observation is not evidence. Only zero
        blocks for longer than the wedge window is."""
        n = node(short="bvn3-val2", bvn="BVN3", per={
            "Directory": part(803, 0, blocks_window=0, idle=6.0, idle_measured=True),
            "BVN3": part(855, 0, blocks_window=0, idle=6.0, idle_measured=True),
        })
        state, _ = nodewatch.classify(n, {"Directory": 803, "BVN3": 855},
                                      Rules(), 0)
        self.assertEqual(state, nodewatch.ST_EXEC)

    def test_executing_node_that_stops_is_stalled(self):
        n = node(short="bvn3-val2", bvn="BVN3", per={
            "Directory": part(803, 0, blocks_window=0, idle=90.0, idle_measured=True),
            "BVN3": part(855, 0, blocks_window=0, idle=90.0, idle_measured=True),
        })
        state, why = nodewatch.classify(n, {"Directory": 803, "BVN3": 855},
                                        Rules(), 0)
        self.assertEqual(state, nodewatch.ST_STALLED)
        self.assertIn("90", why)

    def test_stopped_container_is_down(self):
        n = node(running=False, per={})
        state, why = nodewatch.classify(n, {}, Rules(), 0)
        self.assertEqual(state, nodewatch.ST_DOWN)
        self.assertIn("absent", why)

    def test_container_up_but_silent_is_not_healthy(self):
        """Twelve green containers over a node that answers nothing is the
        failure this tool exists to prevent."""
        n = node(metrics_ok=False, per={
            "Directory": part(None, None), "BVN1": part(None, None)})
        state, _ = nodewatch.classify(n, {"Directory": 803, "BVN1": 840},
                                      Rules(), 0)
        self.assertEqual(state, nodewatch.ST_NOANSWER)


class TestDisplay(unittest.TestCase):
    """The dashboard's contract: what a reader is entitled to assume."""

    def snapshot(self, measured=True):
        j = nodewatch.parse_join_logs(WEDGED_LOG)
        wedged = node(per={
            "Directory": part(81, 722, 0 if measured else None, 730.0, True,
                              0, 567, j["Directory"]),
            "BVN1": part(76, 764, 0 if measured else None, 730.0, True,
                         0, 609, j["BVN1"]),
        }, join_error=j["_error"])
        wedged["state"], wedged["reason"] = nodewatch.classify(
            wedged, {"Directory": 803, "BVN1": 840}, Rules(), 0)
        ok = node(short="bvn2-val1", bvn="BVN2", per={
            "Directory": part(803, 0, 10 if measured else None, 0.4, True),
            "BVN2": part(856, 0, 11 if measured else None, 0.3, True),
        })
        ok["state"], ok["reason"] = nodewatch.classify(
            ok, {"Directory": 803, "BVN2": 856}, Rules(), 0)
        return {
            "asOf": "2026-09-18T23:10:00Z", "asOfUnix": 0, "watchAgeSecs": 30.0,
            "windowSecs": 10.0, "network": "DAGBFTTest", "wedgeSecs": 45.0,
            "behindBlocks": 5,
            "partitions": {
                "Directory": {"leadHeight": 803, "leadNode": "bvn2-val1",
                              "spreadBlocks": 722, "leadBlocksWindow": 10,
                              "windowSecs": 10.0, "nodes": 2},
                "BVN1": {"leadHeight": 840, "leadNode": "bvn1-val2",
                         "spreadBlocks": 764, "leadBlocksWindow": 10,
                         "windowSecs": 10.0, "nodes": 1},
            },
            "nodes": [wedged, ok],
            "fleet": {"validators": 2, "executing": 1, "notExecuting": 1,
                      "bad": ["bvn1-val1"]},
        }

    def test_the_wedged_node_is_named_and_explained(self):
        t = nodewatch.render_text(self.snapshot())
        # Prose is folded to the display width, so compare on the flattened
        # text: the reader sees the sentence, wherever it breaks.
        flat = " ".join(t.split())
        self.assertIn("WEDGED", t)
        self.assertIn("bvn1-val1", t)
        self.assertIn("does not hash into the anchored root", flat)
        self.assertIn("buffered 768 blocks", flat)
        self.assertIn("the node reports 729 s unable to execute", flat)

    def test_every_number_states_its_quantity_and_unit(self):
        """Paul's rule: name the quantity and the window, never the method.
        A bare partition name, an unlabelled count or the word 'average' is
        a defect in the display."""
        t = nodewatch.render_text(self.snapshot())
        self.assertIn("Partition Executed Height, highest node (block)", t)
        self.assertIn("Partition Height Spread, highest minus lowest node (block)", t)
        self.assertIn("blocks this node executed in the last 10 s", t)
        for method_word in ("Average", "average", "Mean", "Run Average"):
            self.assertNotIn(method_word, t)

    def test_a_rate_column_never_claims_a_window_it_lacks(self):
        """One sample is not a window. The column must say so rather than
        print a plausible-looking 'EXEC/1s' full of dashes."""
        t = nodewatch.render_text(self.snapshot(measured=False))
        self.assertIn("EXEC/--", t)
        self.assertIn("no second sample yet", t)
        self.assertNotIn("EXEC/0s", t)

    def test_the_counter_that_never_clears_says_what_it_counts(self):
        t = nodewatch.render_text(self.snapshot())
        self.assertIn("execution lag 609 blocks (node's own gauge)", t)

    def test_no_python_none_reaches_the_reader(self):
        """'None' in a cell is undefined, and undefined is the one
        unacceptable state. Unknown is written '-' and the legend says so."""
        t = nodewatch.render_text(self.snapshot(measured=False))
        self.assertNotIn("None", t)

    def test_the_text_fits_a_phone(self):
        """This is the form Paul reads when he is not at the box. Anything
        past ~76 columns wraps into nonsense in a chat window."""
        for line in nodewatch.render_text(self.snapshot()).splitlines():
            self.assertLessEqual(len(line), 76, "too wide: %r" % line)

    def test_the_second_line_carries_the_verdict(self):
        """A chat preview shows two lines. If the answer to "is the network
        all right" is not in them, the paste has to be opened to be read."""
        head = nodewatch.render_text(self.snapshot()).splitlines()[1]
        self.assertIn("1 of 2 executing", head)
        self.assertIn("WEDGED bvn1-val1", head)
        self.assertIn("BVN1 764 blocks behind", head)
        self.assertIn("stuck 729 s", head)
        self.assertLessEqual(len(head), 76)

    def test_a_healthy_fleet_says_so_in_one_line(self):
        s = self.snapshot()
        s["nodes"] = [s["nodes"][1]]
        s["fleet"] = {"validators": 12, "executing": 12, "notExecuting": 0,
                      "bad": []}
        self.assertEqual(nodewatch.headline(s), "ALL 12 validators executing.")

    def test_legend_defines_the_dash(self):
        t = nodewatch.render_text(self.snapshot())
        self.assertIn("'-' means the node is executing, not unknown.", t)


class TestPageContract(unittest.TestCase):
    """The web view may only read fields the collector actually produces.

    This is the defect class that cost a run before: a dashboard column
    summing a metric that no longer existed. The column looked fine -- it
    read as zero.
    """

    def fields(self, var):
        import re
        script = re.search(r"<script>(.*?)</script>", nodewatch.PAGE,
                           re.S).group(1)
        return set(re.findall(r"\b%s\.(\w+)" % var, script))

    def test_page_reads_only_snapshot_fields(self):
        snap = TestDisplay().snapshot()
        for f in self.fields("s"):
            self.assertIn(f, snap, "the page reads s.%s, which the "
                                   "collector does not produce" % f)

    def test_page_reads_only_node_fields(self):
        node0 = TestDisplay().snapshot()["nodes"][0]
        allowed = set(node0) | {"per"}
        for f in self.fields("n"):
            self.assertIn(f, allowed, "the page reads n.%s, which a node "
                                      "record does not carry" % f)

    def test_page_reads_only_partition_fields(self):
        one = list(TestDisplay().snapshot()["partitions"].values())[0]
        # d is the partition record in the pill row, and also the
        # Directory per-partition record in a card; both must resolve.
        per = TestDisplay().snapshot()["nodes"][0]["per"]["Directory"]
        allowed = set(one) | set(per)
        for f in self.fields("d"):
            self.assertIn(f, allowed, "the page reads d.%s, which neither a "
                                      "partition nor a per-partition record "
                                      "carries" % f)

    def test_page_labels_name_the_quantity_and_the_unit(self):
        for label in ("Own Executed Height (block)",
                      "Behind Highest Node (block)",
                      "Executed Height, highest node",
                      "Height Spread"):
            self.assertIn(label, nodewatch.PAGE)
        self.assertIn("Blocks Executed (last ", nodewatch.PAGE)


class TestHistory(unittest.TestCase):
    def test_restart_rebases_instead_of_reporting_negative_blocks(self):
        """A restarted node's height drops. A window rate computed across
        that drop is negative, which is not a thing a block count can be."""
        w = nodewatch.Watch.__new__(nodewatch.Watch)
        w.hist, w.first, w.resets = {}, {}, {}
        n = {"container": "acc-bvn1-val1", "heights": {"BVN1": 300}}
        w._record([n], 100.0)
        w._record([{"container": "acc-bvn1-val1", "heights": {"BVN1": 310}}], 110.0)
        self.assertEqual(w.advanced("acc-bvn1-val1", "BVN1", 110.0, 30.0)[0], 10)
        w._record([{"container": "acc-bvn1-val1", "heights": {"BVN1": 4}}], 120.0)
        self.assertEqual(w.resets["acc-bvn1-val1"], 1)
        blocks, _ = w.advanced("acc-bvn1-val1", "BVN1", 120.0, 30.0)
        self.assertIsNone(blocks)

    def test_idle_is_a_floor_until_a_change_is_seen(self):
        """Before the height has been seen to move, 'idle 12 s' is not a
        measurement -- it is how long we have been looking. The caller is
        told which it is."""
        w = nodewatch.Watch.__new__(nodewatch.Watch)
        w.hist, w.first, w.resets = {}, {}, {}
        w._record([{"container": "c", "heights": {"BVN1": 76}}], 100.0)
        w._record([{"container": "c", "heights": {"BVN1": 76}}], 112.0)
        secs, measured = w.idle_secs("c", "BVN1", 112.0)
        self.assertEqual(secs, 12.0)
        self.assertFalse(measured)
        w._record([{"container": "c", "heights": {"BVN1": 77}}], 113.0)
        secs, measured = w.idle_secs("c", "BVN1", 118.0)
        self.assertEqual(secs, 5.0)
        self.assertTrue(measured)

    def test_a_missing_height_is_not_a_zero(self):
        """A node that did not answer must leave no sample at all. Recording
        it as 0 makes the next answer look like a thousand-block catch-up."""
        w = nodewatch.Watch.__new__(nodewatch.Watch)
        w.hist, w.first, w.resets = {}, {}, {}
        w._record([{"container": "c", "heights": {"BVN1": None}}], 100.0)
        self.assertEqual(w.hist, {})


if __name__ == "__main__":
    unittest.main()
