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


class DashboardScriptParses(unittest.TestCase):
    def test_the_page_script_is_valid_javascript(self):
        node = shutil.which("node") or shutil.which("nodejs")
        if not node:
            self.skipTest("no node on this machine")
        m = re.search(r"<script>(.*?)</script>", open(os.path.join(HERE, "soakmon.py")).read(), re.S)
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
