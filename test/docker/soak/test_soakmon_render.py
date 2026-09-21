#!/usr/bin/env python3
"""The dashboard's follower panel, EXECUTED, not described.

M3: the panel's absent branch was replaced by `innerHTML='0'` in a mutation
and all 151 tests passed. `TheFollowerRowGroupIsAlwaysThere` asserts on the
collector's dict; `TheRowGroupIsOnTheBoard` asserts that substrings appear
in this file. Neither runs a line of the rendering, so neither can fail when
the rendering starts lying.

So the follower panel's rendering is a pure function between markers in
`soakmon.py` — state in, HTML out, no DOM — and this file runs it in node
and asserts on what it produced. `— not measured` is now a property of the
output, which is the only place it means anything.

Skipped where node is not installed, like the parse test beside it.
"""
import json
import os
import re
import shutil
import subprocess
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
NODE = shutil.which("node") or shutil.which("nodejs")

WITH_FOLLOWER = {
    "measured": True, "bound": 2,
    "nodes": {"acc-bvn3-fol1": {
        "worstBehind": 1, "maxBehindRun": 4, "maxBehindAt": 10.0,
        "bvn": "BVN3", "port": 26692, "over": False,
        "partitions": {
            "Directory": {"measured": True, "behind": 1, "ahead": 0,
                          "follower": 499, "network": 500, "over": False,
                          "why": None},
            "BVN3": {"measured": True, "behind": 0, "ahead": 0,
                     "follower": 500, "network": 500, "over": False,
                     "why": None}}}}}

NODE_STATS = {"followerStats": {"count": 1, "nodes": ["acc-bvn3-fol1"],
                                "rssMaxMiB": 812, "healEntries": 0,
                                "healsMeasured": True}}


def helpers():
    """The pure render helpers, plus the two formatters they call."""
    with open(os.path.join(HERE, "soakmon.py")) as fh:
        src = fh.read()
    m = re.search(r"// --- pure render helpers.*?// --- end pure render helpers ---",
                  src, re.S)
    assert m, "the marked block is gone from soakmon.py"
    block = m.group(0)
    # fmt and shortP live above the markers; take them by name.
    extra = []
    for name in ("fmt", "shortP"):
        f = re.search(r"^function %s\(.*?\n}" % name, src, re.S | re.M)
        if f:
            extra.append(f.group(0))
        else:
            f = re.search(r"^(?:const|let)\s+%s\s*=.*?;$" % name, src, re.M)
            assert f, "cannot find %s in soakmon.py" % name
            extra.append(f.group(0))
    return "\n".join(extra) + "\n" + block


def render(follower, node_stats):
    """Run followerView in node and return its object."""
    driver = (helpers() + "\nconst out=followerView(%s,%s);"
              "\nprocess.stdout.write(JSON.stringify(out));"
              % (json.dumps(follower), json.dumps(node_stats)))
    with tempfile.NamedTemporaryFile("w", suffix=".js", delete=False) as f:
        f.write(driver)
        path = f.name
    try:
        p = subprocess.run([NODE, path], capture_output=True, text=True)
        if p.returncode != 0:
            raise AssertionError("followerView threw:\n" + p.stderr)
        return json.loads(p.stdout)
    finally:
        os.unlink(path)


@unittest.skipIf(NODE is None, "no node on this machine")
class TheAbsentBranchRenders(unittest.TestCase):
    """Replace the `ABSENT` in the no-follower branch with `'0'` and every
    one of these fails. That is the test M3 asked for."""

    def test_no_follower_renders_not_measured_in_every_slot(self):
        v = render({"measured": False, "nodes": {},
                    "why": "no follower in this topology"}, {})
        for k in ("fbehind", "fmax", "fheight", "fres"):
            self.assertIn("— not measured", v[k], k)
            self.assertNotEqual("0", v[k].strip(), k)
        self.assertIn("no follower in this topology", v["fstate"])

    def test_no_slot_renders_a_bare_zero_when_nothing_was_measured(self):
        v = render({"measured": False, "nodes": {}, "why": "x"}, {})
        for k, html in v.items():
            self.assertNotRegex(html, r"^\s*0\s*$",
                                "%s rendered a zero for an absent "
                                "instrument (REPORTING-SPEC 1)" % k)

    def test_a_follower_that_answered_nothing_is_absent_not_caught_up(self):
        fo = {"measured": True, "bound": 2, "nodes": {"acc-bvn3-fol1": {
            "worstBehind": None, "maxBehindRun": None, "over": False,
            "partitions": {"Directory": {
                "measured": False, "behind": None, "ahead": None,
                "follower": None, "network": 500, "over": False,
                "why": "the follower did not answer"}}}}}
        v = render(fo, {})
        self.assertIn("— not measured", v["fbehind"])
        self.assertIn("— not measured", v["fmax"])
        self.assertIn("did not answer", v["fheight"])


@unittest.skipIf(NODE is None, "no node on this machine")
class TheMeasuredBranchRenders(unittest.TestCase):
    def test_the_numbers_and_the_bound_appear(self):
        v = render(WITH_FOLLOWER, NODE_STATS)
        self.assertIn("1", v["fbehind"])
        self.assertNotIn("not measured", v["fbehind"])
        self.assertIn("4", v["fmax"])
        self.assertIn("DN 499/500", v["fheight"])
        self.assertIn("BVN3 500/500", v["fheight"])
        self.assertIn("bound 2 blocks", v["fstate"])
        self.assertNotIn("OVER", v["fstate"])

    def test_past_the_bound_is_red_and_said_in_words(self):
        fo = json.loads(json.dumps(WITH_FOLLOWER))
        fo["nodes"]["acc-bvn3-fol1"]["worstBehind"] = 9
        v = render(fo, NODE_STATS)
        self.assertIn("red", v["fbehind"])
        self.assertIn("OVER the bound", v["fstate"])
        self.assertIn("acc-bvn3-fol1", v["fstate"])

    def test_the_followers_own_resources_render_and_zero_heals_is_a_number(self):
        """0 heals here is a read counter, not an absent one — a follower is
        never picked as a gap requester, so 0 is the expected value and must
        be distinguishable from 'nobody looked'."""
        v = render(WITH_FOLLOWER, NODE_STATS)
        self.assertIn("812", v["fres"])
        self.assertIn("0", v["fres"])
        self.assertNotIn("not measured", v["fres"])

    def test_heals_that_were_not_measured_are_not_rendered_as_zero(self):
        ns = {"followerStats": {"count": 1, "nodes": ["acc-bvn3-fol1"],
                                "rssMaxMiB": 812, "healEntries": None,
                                "healsMeasured": False}}
        v = render(WITH_FOLLOWER, ns)
        self.assertIn("— not measured", v["fres"])

    def test_every_reading_names_its_follower_not_a_bare_partition(self):
        """With a follower added mid-run beside the one launched with the
        network, `BVN3 500/500` does not say whose (#4364)."""
        fo = json.loads(json.dumps(WITH_FOLLOWER))
        fo["nodes"]["acc-bvn3-fol1"]["life"] = {"kind": "launched", "at": None, "adds": 0}
        fo["nodes"]["acc-bvn3-fol2"] = {
            "worstBehind": None, "maxBehindRun": 311, "over": False,
            "life": {"kind": "removed", "at": "2026-09-20T01:05:00Z", "adds": 1},
            "partitions": {"BVN3": {
                "measured": False, "behind": None, "ahead": None,
                "follower": None, "network": 500, "over": False,
                "why": "removed at 01:05Z"}}}
        v = render(fo, NODE_STATS)
        self.assertIn("acc-bvn3-fol1 DN 499/500", v["fheight"])
        self.assertIn("acc-bvn3-fol1 BVN3 500/500", v["fheight"])
        self.assertIn("acc-bvn3-fol2 BVN3 removed at 01:05Z", v["fheight"])
        for part in v["fheight"].split(" · "):
            self.assertRegex(part, r"^acc-", "a reading with no follower named")
        self.assertIn("acc-bvn3-fol1 (follower launched with the network)", v["fstate"])
        self.assertIn("acc-bvn3-fol2 (follower removed 01:05Z)", v["fstate"])

    def test_an_added_follower_says_when_and_which_add(self):
        fo = json.loads(json.dumps(WITH_FOLLOWER))
        fo["nodes"]["acc-bvn3-fol1"]["life"] = {
            "kind": "added", "at": "2026-09-20T01:10:00Z", "adds": 2}
        v = render(fo, NODE_STATS)
        self.assertIn("acc-bvn3-fol1 (follower added 01:10Z (add 2))", v["fstate"])

    def test_no_follower_stats_is_absent_not_zero(self):
        v = render(WITH_FOLLOWER, {})
        self.assertIn("— not measured", v["fres"])


if __name__ == "__main__":
    unittest.main()
