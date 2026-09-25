#!/usr/bin/env python3
"""followerchaos: the readings the chaos walk takes around an added follower
(#4364), on fixtures — which follower is the late one, when it is ACTIVE, and
whether removing it left the network's cadence and streams alone.

NOT verified here: that a live node's gauge, log and ledgers read this way.
"""
import os
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import followerchaos  # noqa: E402

NET = """id: "T"
bvns:
  - id: "BVN1"
    nodes:
      - listenAddress: "0.0.0.0"
        peerAddress: "acc-bvn1-val1"
      - listenAddress: "0.0.0.0"
        peerAddress: "acc-bvn1-fol1"
        dnnType: "follower"
        bvnnType: "follower"
      - listenAddress: "0.0.0.0"
        peerAddress: "acc-bvn1-fol2"
        dnnType: "follower"
        bvnnType: "follower"
"""

COMPOSE = """services:
  bvn1-val1:
    container_name: acc-bvn1-val1
    profiles: ["late-follower"]
  bvn1-fol1:
    container_name: acc-bvn1-fol1
  bvn1-fol2:
    profiles: ["late-follower"]
    container_name: acc-bvn1-fol2

volumes:
  network-config:
"""


class LateFollower(unittest.TestCase):
    def setUp(self):
        d = tempfile.mkdtemp(prefix="followerchaos-")
        self.net = os.path.join(d, "net.yml")
        self.compose = os.path.join(d, "compose.yml")
        with open(self.net, "w") as f:
            f.write(NET)
        with open(self.compose, "w") as f:
            f.write(COMPOSE)

    def test_only_a_follower_in_the_profile_is_late(self):
        got = followerchaos.late_followers(self.compose, self.net)
        self.assertEqual(["acc-bvn1-fol2"], [f["container"] for f in got],
                         "fol1 is started by up; val1 is in the profile but a validator")
        self.assertEqual("bvn1-fol2", got[0]["service"])
        self.assertEqual("bvn1-3", got[0]["dir"])

    def test_the_committed_files_declare_one_per_bvn(self):
        got = followerchaos.late_followers()
        self.assertEqual([("bvn1-fol1", "acc-bvn1-fol1", "bvn1-5"),
                          ("bvn2-fol1", "acc-bvn2-fol1", "bvn2-5"),
                          ("bvn3-fol2", "acc-bvn3-fol2", "bvn3-6")],
                         [(f["service"], f["container"], f["dir"]) for f in got])


class NodeState(unittest.TestCase):
    SCRAPE = "\n".join([
        "# HELP accumulate_node_state ...",
        'accumulate_node_state{partition="directory"} 2',
        'accumulate_node_state{partition="bvn3"} 0',
        'accumulate_node_state_other{partition="bvn3"} 2',
    ])

    def test_booting_on_one_partition_is_not_active(self):
        s = followerchaos.node_states(self.SCRAPE)
        self.assertEqual({"directory": 2, "bvn3": 0}, s)
        self.assertFalse(followerchaos.all_active(s))
        self.assertEqual("bvn3=BOOTING directory=ACTIVE", followerchaos.describe_states(s))

    def test_active_on_every_partition(self):
        s = followerchaos.node_states(self.SCRAPE.replace("} 0", "} 2"))
        self.assertTrue(followerchaos.all_active(s))

    def test_no_gauge_is_not_active(self):
        self.assertFalse(followerchaos.all_active(followerchaos.node_states("")))
        self.assertIn("not measured", followerchaos.describe_states({}))


class RootMatch(unittest.TestCase):
    def line(self, c, blk, root):
        return ("%s | 2026-09-21T00:00:%02dZ INFO Anchor not sent block=%d bpt=b "
                "module=conductor root=%s seq=1 source=BVN3" % (c, blk % 60, blk, root))

    def test_docker_logs_prefixed_by_container_are_read(self):
        lines = [self.line("acc-bvn3-fol2", 10, "x"),
                 self.line("acc-bvn3-val1", 10, "y"),
                 self.line("acc-bvn3-fol2", 11, "z"),
                 self.line("acc-bvn3-val1", 11, "z")]
        self.assertEqual(("BVN3", 11, "z"),
                         followerchaos.root_match_in("acc-bvn3-fol2", lines))


def snap(t, heights, delivered):
    return {"t": t, "heights": heights, "delivered": delivered}


class Unaffected(unittest.TestCase):
    def test_steady_cadence_and_advancing_streams(self):
        ok, text = followerchaos.unaffected(
            snap(0, {"BVN1": 100}, {"synthetic BVN2->BVN1": 10, "anchor Directory->BVN1": 5}),
            snap(30, {"BVN1": 130}, {"synthetic BVN2->BVN1": 20, "anchor Directory->BVN1": 5}),
            snap(60, {"BVN1": 158}, {"synthetic BVN2->BVN1": 31, "anchor Directory->BVN1": 5}))
        self.assertTrue(ok, text)
        self.assertIn("unaffected", text)
        self.assertIn("1 advancing", text)

    def test_a_cadence_that_halves_is_affected(self):
        ok, text = followerchaos.unaffected(
            snap(0, {"BVN1": 100}, {}), snap(30, {"BVN1": 130}, {}),
            snap(60, {"BVN1": 140}, {}))
        self.assertFalse(ok)
        self.assertIn("BVN1 cadence", text)

    def test_a_stream_that_stops_is_affected(self):
        ok, text = followerchaos.unaffected(
            snap(0, {}, {"synthetic BVN2->BVN1": 10}),
            snap(30, {}, {"synthetic BVN2->BVN1": 20}),
            snap(60, {}, {"synthetic BVN2->BVN1": 20}))
        self.assertFalse(ok)
        self.assertIn("stopped at 20", text)

    def test_nothing_answered_is_not_measured(self):
        ok, text = followerchaos.unaffected({}, {}, {})
        self.assertIsNone(ok)
        ok, text = followerchaos.unaffected(
            snap(0, {"BVN1": None}, {}), snap(30, {"BVN1": None}, {}),
            snap(60, {"BVN1": None}, {}))
        self.assertIsNone(ok)
        self.assertIn("not measured", text)


if __name__ == "__main__":
    unittest.main()
