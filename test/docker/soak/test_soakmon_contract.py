#!/usr/bin/env python3
"""The two things unit tests over helper functions cannot catch: that
`collect_metrics` actually runs, and that the dashboard's script parses.

Both were broken at once by a change whose own unit tests were all green
(#4279 review): deleting a block of `collect_metrics` took `life` and the
`nodes` aggregation with it, so every tick raised NameError and every
watchdog that reads /data silently stopped tripping; and a `const` in the
new dashboard code shadowed one already in `tick()`, which is a parse-time
SyntaxError, so the page never rendered at all.
"""
import json, os, re, shutil, subprocess, sys, tempfile, unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import soakmon

SCRAPE = [
    ("process_resident_memory_bytes", {}, 1.07e9),
    ("go_goroutines", {}, 268.0),
    ("accumulate_dagbft_blocks_produced_total", {}, 687160.0),
    ("accumulate_dagbft_blocks_empty_total", {}, 509972.0),
    ("accumulate_conductor_heal_entries_total", {}, 18171.0),
    ("accumulate_conductor_heal_requests_total",
     {"source": "Directory", "destination": "BVN1", "outcome": "not-yet"}, 1515.0),
    ("accumulate_dispatcher_drops_total", {"destination": "BVN1", "reason": "queue-full"}, 5.0),
    ("accumulate_staging_held_entries",
     {"ledger": "acc://dn.acme/anchors", "source": "acc://bvn-bvn1.acme"}, 33.0),
]


class CollectMetricsRuns(unittest.TestCase):
    """No docker, no curl: only that the function completes and its contract
    holds. Every consumer of /data reads these keys."""

    def setUp(self):
        self._c, self._s, self._f = soakmon.containers, soakmon._scrape_one, soakmon.collect_flows_api
        soakmon.containers = lambda: ["acc-bvn1-val1", "acc-bvn2-val1"]
        soakmon._scrape_one = lambda c, out, lock: out.setdefault(c, list(SCRAPE))
        soakmon.collect_flows_api = lambda: ({"synthetic": {}, "anchor": {}}, 0, 0)

    def tearDown(self):
        soakmon.containers, soakmon._scrape_one, soakmon.collect_flows_api = self._c, self._s, self._f

    def test_it_runs_and_keeps_its_contract(self):
        m = soakmon.collect_metrics()
        # The keys every reader of /data depends on. stallkill.sh and
        # wedgewatch.sh read life; ladder.sh asserts nodeStats is a dict;
        # seizewatch.sh reads heals and flows.
        for k in ("heals", "wedges", "flows", "life", "exec", "nodeStats",
                  "synProduced", "ancProduced", "nodes", "scraped"):
            self.assertIn(k, m, k)
        self.assertIsInstance(m["nodeStats"], dict)
        self.assertIsInstance(m["life"], dict)
        self.assertEqual(m["nodeStats"]["count"], 2)
        # Every node says whether it is a follower (#4365), so a reader of
        # /data never has to infer a role from a container name.
        for c, v in m["nodeStats"]["byNode"].items():
            self.assertIn("follower", v, c)
        self.assertEqual(2, m["nodeStats"]["validatorCount"])
        self.assertEqual([], m["nodeStats"]["followers"])
        self.assertEqual(m["life"]["blocks"], 687160)
        self.assertEqual(m["heals"]["entries"], 2 * 18171)
        self.assertEqual(m["wedges"]["total"], 2 * 5)
        # It must survive being serialized: /data is json.dumps(STATE).
        json.dumps(m)

    def test_it_survives_a_node_that_answers_nothing(self):
        soakmon._scrape_one = lambda c, out, lock: out.setdefault(c, [])
        m = soakmon.collect_metrics()
        self.assertEqual(m["nodeStats"]["count"], 0)
        self.assertFalse(m["heals"]["measured"])
        self.assertIsNone(m["heals"]["entries"], "absent is not zero")
        json.dumps(m)


class TheFollowerRowGroupIsAlwaysThere(unittest.TestCase):
    """#4365 added a row group to the board. It must render on a run WITHOUT a
    follower too, and it must say so rather than showing zeros: a board whose
    panels come and go with the topology teaches the reader that a missing
    panel means nothing happened."""

    def setUp(self):
        self._f, self._r = soakmon.FOLLOWERS, soakmon._read_ledger_index
        soakmon._FOLLOWER_WORST.clear()

    def tearDown(self):
        soakmon.FOLLOWERS, soakmon._read_ledger_index = self._f, self._r
        soakmon._FOLLOWER_WORST.clear()

    def test_with_no_follower_it_is_absent_and_says_why(self):
        soakmon.FOLLOWERS = []
        v = soakmon.collect_follower({"Directory": 9}, now=1.0)
        self.assertFalse(v["measured"])
        self.assertIn("no follower", v["why"])
        json.dumps(v)

    def test_with_a_follower_it_carries_the_bound_and_serializes(self):
        soakmon.FOLLOWERS = [{"container": "acc-bvn3-fol1", "port": 26692,
                              "dir": "bvn3-5", "bvn": "BVN3",
                              "partitions": ["Directory", "BVN3"]}]
        soakmon._read_ledger_index = lambda port, part: 8
        v = soakmon.collect_follower({"Directory": 9, "BVN3": 9}, now=1.0)
        self.assertEqual(soakmon.BEHIND_BOUND, v["bound"])
        self.assertEqual(1, v["nodes"]["acc-bvn3-fol1"]["worstBehind"])
        json.dumps(v)   # /data is json.dumps(STATE)


class FleetTotalsAndTheFollower(unittest.TestCase):
    """M6. `soakmon.containers()` is `docker ps --filter name=acc-bvn`, which
    matches `acc-bvn3-fol1`, so every aggregate — heals, wedges, exec, the
    RSS average — summed over thirteen nodes while `soak.sh`'s `heals`
    column kept to the twelve. One run, two heal totals, one word.

    The aggregates now mean the VALIDATORS, like the CSV; the follower's own
    figures are beside them under their own name; and every one of them
    carries a `scope` saying which. The per-node table still holds all
    thirteen, because a follower that leaks is a finding.
    """

    def setUp(self):
        self._c, self._s, self._f = (soakmon.containers, soakmon._scrape_one,
                                     soakmon.collect_flows_api)
        self._fol = soakmon.FOLLOWERS
        soakmon.FOLLOWERS = [{"container": "acc-bvn3-fol1", "port": 26692,
                              "dir": "bvn3-5", "bvn": "BVN3",
                              "partitions": ["Directory", "BVN3"]}]
        soakmon.containers = lambda: ["acc-bvn1-val1", "acc-bvn2-val1",
                                      "acc-bvn3-fol1"]
        soakmon._scrape_one = lambda c, out, lock: out.setdefault(c, list(SCRAPE))
        soakmon.collect_flows_api = lambda: ({"synthetic": {}, "anchor": {}}, 0, 0)

    def tearDown(self):
        (soakmon.containers, soakmon._scrape_one,
         soakmon.collect_flows_api) = self._c, self._s, self._f
        soakmon.FOLLOWERS = self._fol

    def test_the_totals_are_the_validators_like_the_csv(self):
        m = soakmon.collect_metrics()
        self.assertEqual(2 * 18171, m["heals"]["entries"],
                         "two validators, not three nodes")
        self.assertEqual(2 * 5, m["wedges"]["total"])
        self.assertEqual(2, m["nodeStats"]["count"])

    def test_every_total_says_whose_it_is(self):
        m = soakmon.collect_metrics()
        for k in ("heals", "wedges", "life", "exec", "nodeStats"):
            self.assertIn("scope", m[k], k)
            self.assertIn("validator", m[k]["scope"].lower(), k)

    def test_the_followers_own_figures_are_beside_them_not_inside_them(self):
        m = soakmon.collect_metrics()
        f = m["nodeStats"]["followerStats"]
        self.assertEqual(1, f["count"])
        self.assertEqual(18171, f["healEntries"])
        self.assertIn("acc-bvn3-fol1", m["nodeStats"]["byNode"],
                      "the per-node table still holds every node")

    def test_a_run_with_no_follower_says_so_rather_than_showing_zeros(self):
        soakmon.FOLLOWERS = []
        soakmon.containers = lambda: ["acc-bvn1-val1", "acc-bvn2-val1"]
        m = soakmon.collect_metrics()
        self.assertIsNone(m["nodeStats"]["followerStats"])
        self.assertEqual(2, m["nodeStats"]["count"])
        json.dumps(m)


class DashboardScriptParses(unittest.TestCase):
    def test_the_page_script_is_valid_javascript(self):
        node = shutil.which("node") or shutil.which("nodejs")
        if not node:
            self.skipTest("no node on this machine")
        with open(os.path.join(HERE, "soakmon.py")) as fh:
            src = fh.read()
        m = re.search(r"<script>(.*?)</script>", src, re.S)
        self.assertIsNotNone(m, "the dashboard has a script")
        with tempfile.NamedTemporaryFile("w", suffix=".js", delete=False) as f:
            # --check parses without executing, so no DOM is needed.
            f.write(m.group(1))
            path = f.name
        try:
            p = subprocess.run([node, "--check", path], capture_output=True, text=True)
            self.assertEqual(p.returncode, 0, "dashboard script does not parse:\n" + p.stderr)
        finally:
            os.unlink(path)




class GapIsNeverNegative(unittest.TestCase):
    """A flow cell's in-flight gap is sent minus received, and received can
    exceed sent only when the source was read over fewer or staler nodes than
    the destination. That is skew, not a negative lag: the board must never
    show an impossible state (REPORTING-SPEC 1a) and must name the stale read
    (1b)."""

    def test_ordinary_gap(self):
        self.assertEqual(soakmon.judge_gap(120, 100), (20, 0))

    def test_caught_up(self):
        self.assertEqual(soakmon.judge_gap(100, 100), (0, 0))

    def test_recv_ahead_is_skew_not_negative(self):
        gap, skew = soakmon.judge_gap(100, 130)
        self.assertEqual(gap, 0, "never a negative depth")
        self.assertEqual(skew, 30, "the excess is reported as skew")

    def test_missing_fields(self):
        self.assertEqual(soakmon.judge_gap(None, None), (0, 0))


if __name__ == "__main__":
    unittest.main()
