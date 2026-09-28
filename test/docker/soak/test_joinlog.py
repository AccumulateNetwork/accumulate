#!/usr/bin/env python3
"""joinlog.py: a follower's join phase and counts, read from its log, and the
Followers panel's rendering of them, executed in node."""
import json
import os
import shutil
import subprocess
import tempfile
import unittest

import joinlog

HERE = os.path.dirname(os.path.abspath(__file__))
NODE = shutil.which("node") or shutil.which("nodejs")


def L(ts, msg, part):
    # docker's --timestamps prefix, then the node's own line, ANSI and all.
    return ("%s \x1b[36m%s\x1b[0m INFO %s module=join partition=%s block=9"
            % (ts, ts[:19] + "Z", msg, part))


LOG = [
    L("2026-09-28T05:00:00.000000001Z", "Pulling the state: the whole BPT", "acc://dn.acme"),
    L("2026-09-28T05:00:01.000000001Z", "Pulling the state: the whole BPT", "acc://bvn-BVN1.acme"),
    L("2026-09-28T05:01:00.000000001Z", "The walk has covered the peer's whole BPT", "BVN1"),
    L("2026-09-28T05:02:00.000000001Z", "Joined; executing from the block after the state", "BVN1"),
    L("2026-09-28T05:02:10.000000001Z", "This node is executing in agreement; it is ACTIVE", "BVN1"),
    L("2026-09-28T05:03:00.000000001Z", "An executed block's root is not its anchored root; repairing", "BVN1"),
    L("2026-09-28T05:03:00.000000002Z", "Repairing the state from the block ledger: every account", "BVN1"),
    L("2026-09-28T05:03:01.000000001Z", "A repair changed an account this node had executed differently", "BVN1"),
    L("2026-09-28T05:03:02.000000001Z", "This node is not executing in agreement; it is BOOTING until it hands off again", "BVN1"),
    L("2026-09-28T05:04:00.000000001Z", "This node is executing in agreement; it is ACTIVE", "BVN1"),
    "2026-09-28T05:05:00.000000001Z 2026-09-28T05:05:00Z INFO Stream produced block=10",
]


class ThePhaseAndCounts(unittest.TestCase):
    def test_latest_phase_wins_per_partition_and_counts_accumulate(self):
        r = joinlog.Reader()
        r.feed("f", LOG)
        # The join logs the URL, dagbft the ID: one partition either way.
        self.assertEqual({"directory", "bvn1"}, set(r.parts["f"]))
        dn, bvn = r.parts["f"]["directory"], r.parts["f"]["bvn1"]
        self.assertEqual("pulling", dn["phase"])
        self.assertEqual("ACTIVE", bvn["phase"])
        self.assertEqual((1, 1, 1, 1), (bvn["rootMisses"], bvn["repairs"],
                                        bvn["repairChanged"], bvn["drops"]))
        # First ACTIVE, not the one after the drop.
        self.assertEqual(joinlog._epoch("2026-09-28T05:02:10Z"), bvn["firstActiveAt"])

    def test_not_executing_is_booting_not_active(self):
        r = joinlog.Reader()
        r.feed("f", [LOG[8]])
        self.assertEqual("BOOTING", r.parts["f"]["bvn1"]["phase"])
        self.assertIsNone(r.parts["f"]["bvn1"]["firstActiveAt"])

    def test_a_line_already_read_is_not_counted_again(self):
        # `docker logs --since` returns the line at the boundary again.
        r = joinlog.Reader()
        r.feed("f", LOG[:6])
        r.feed("f", LOG[5:])
        self.assertEqual(1, r.parts["f"]["bvn1"]["rootMisses"])
        self.assertEqual(LOG[-1].split()[0], r.last["f"])

    def test_mem(self):
        self.assertEqual(761.5, joinlog.mem_mib("761.5MiB"))
        self.assertEqual(1536.0, joinlog.mem_mib("1.5GiB"))
        self.assertIsNone(joinlog.mem_mib("--"))


FOLS = [{"container": "acc-bvn1-fol1", "partitions": ["Directory", "BVN1"]},
        {"container": "acc-bvn3-fol1", "partitions": ["Directory", "BVN3"]}]


class TheRows(unittest.TestCase):
    def rows(self, lives, state, now):
        r = joinlog.Reader()
        r.feed("acc-bvn1-fol1", LOG)
        heights = {"nodes": {"acc-bvn1-fol1": {"partitions": {
            "BVN1": {"follower": 990, "network": 1000, "behind": 10}}}}}
        return joinlog.rows(FOLS, r, heights, lives, state,
                            {"acc-bvn1-fol1": 700.0}, now)

    def test_one_row_per_follower_and_partition_with_timing(self):
        add = "2026-09-28T04:59:00Z"
        rows = self.rows({"acc-bvn1-fol1": {"kind": "added", "at": add}},
                         {"acc-bvn1-fol1": {"status": "running", "exitCode": 0,
                                            "oomKilled": False, "startedAt": 1}},
                         joinlog._epoch("2026-09-28T05:09:00Z"))
        self.assertEqual(4, len(rows))
        b = next(x for x in rows if x["container"] == "acc-bvn1-fol1" and x["partition"] == "BVN1")
        self.assertEqual(("ACTIVE", 990, 1000, 10), (b["phase"], b["height"], b["network"], b["behind"]))
        self.assertEqual(600, b["sinceAddSec"])
        self.assertEqual(190, b["toActiveSec"])
        self.assertEqual(700.0, b["memMiB"])
        g = next(x for x in rows if x["container"] == "acc-bvn3-fol1")
        self.assertEqual("launched", g["life"])
        self.assertIsNone(g["status"])        # no inspect record: unknown, not running

    def test_a_follower_not_added_is_not_added(self):
        rows = self.rows({"acc-bvn1-fol1": {"kind": "waiting", "at": None}}, {}, 0)
        self.assertEqual({"not added"}, {x["phase"] for x in rows if x["container"] == "acc-bvn1-fol1"})

    def test_oomkilled_is_carried(self):
        rows = self.rows({}, {"acc-bvn1-fol1": {"status": "exited", "exitCode": 137,
                                                "oomKilled": True, "startedAt": 1}}, 10)
        self.assertTrue(rows[0]["oomKilled"])
        self.assertEqual(137, rows[0]["exitCode"])


def render(fj):
    import test_soakmon_render as tr
    src = tr.helpers()
    with tempfile.NamedTemporaryFile("w", suffix=".js", delete=False) as f:
        f.write(src + "\nprocess.stdout.write(JSON.stringify(followersView(%s)));" % json.dumps(fj))
        path = f.name
    try:
        p = subprocess.run([NODE, path], capture_output=True, text=True)
        if p.returncode != 0:
            raise AssertionError(p.stderr)
        return json.loads(p.stdout)
    finally:
        os.unlink(path)


@unittest.skipIf(NODE is None, "no node on this machine")
class ThePanelRenders(unittest.TestCase):
    def test_absent_is_not_measured(self):
        self.assertIn("— not measured", render(None))

    def test_rows_name_source_and_partition_and_state(self):
        html = render({"rows": [
            {"container": "acc-bvn1-fol1", "partition": "Directory", "life": "added",
             "phase": "BOOTING", "height": 5, "network": 9, "behind": 4,
             "sinceAddSec": 125, "toActiveSec": None, "rootMisses": 2, "repairs": 1,
             "repairChanged": 0, "drops": 1, "memMiB": 812.4,
             "status": "exited", "exitCode": 137, "oomKilled": True},
            {"container": "acc-bvn2-fol1", "partition": "BVN2", "life": "waiting",
             "phase": "not added", "rootMisses": 0, "repairs": 0, "repairChanged": 0,
             "drops": 0, "status": None}]})
        self.assertIn("acc-bvn1-fol1 → DN", html)
        self.assertIn("BOOTING", html)
        self.assertIn("5 / 9", html)
        self.assertIn("2m 05s", html)
        self.assertIn("OOMKilled (exit 137)", html)
        self.assertIn("812", html)
        self.assertIn("acc-bvn2-fol1 → BVN2", html)
        self.assertIn("not added", html)
        self.assertIn("not created", html)


if __name__ == "__main__":
    unittest.main()
