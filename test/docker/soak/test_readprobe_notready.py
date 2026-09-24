#!/usr/bin/env python3
"""The validator read-probe's NotReady refusals are their own column (#4425).

A joining node answers every read `NotReady` (JSON-RPC -33504): that is the
designed answer of a BOOTING node, the protocol's "ask someone else", and
the follower probe has always counted it apart. The validator probe folded
it into `failed`, so run 20260924T093936Z's manifest read `1541 failed` —
failures that begin at the first restart and jump when acc-bvn3-val1 joins,
and that nobody could tell from a storage miss, because the probe recorded
no cause.

Two readings are pinned here: a round's NotReady refusals land in their own
column and outside the latencies (as the query gate's already do), and the
manifest row read from an OLDER report — this run's — does not call the
unsorted count `failed`.
"""
import contextlib
import io
import os
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import readprobe  # noqa: E402

RUN = os.path.join(HERE, "runs", "20260924T093936Z")


class ARoundWithJoiningNodes(unittest.TestCase):
    """The run's last round (10:08:25Z): 300 reads, 135 not answered, 6 of
    them timed out. The other 129 are what a round over twelve validators,
    four of them BOOTING, gets back — refusals, not failures."""

    def setUp(self):
        self._saved = (readprobe.query, readprobe.CSV, readprobe.REPORT,
                       readprobe.FOLLOWERS, readprobe.CHAOS)
        d = tempfile.mkdtemp(prefix="readprobe-notready-")
        readprobe.CSV = os.path.join(d, "readprobe.csv")
        readprobe.REPORT = os.path.join(d, "readprobe-report.md")
        readprobe.CHAOS = os.path.join(d, "chaos.log")
        readprobe.FOLLOWERS = []
        self.n = 0

        def query(scope, q, url=None):
            if q.get("queryType") == "default" and scope.endswith("/ledger"):
                return {"account": {"index": 1600}}, 1.0, None     # height()
            self.n += 1
            if self.n <= 129:
                return None, 0.4, readprobe.WHY_NOT_READY
            if self.n <= 135:
                return None, 8000.0, readprobe.WHY_TIMEOUT
            if q.get("queryType") == "chain":
                return {"records": [{}]}, 2.0, None
            return {"message": {"type": "transaction"}}, 3.0, None
        readprobe.query = query

    def tearDown(self):
        (readprobe.query, readprobe.CSV, readprobe.REPORT,
         readprobe.FOLLOWERS, readprobe.CHAOS) = self._saved

    def round(self):
        pr = readprobe.Probe()
        pr.reservoir = [{"partition": p, "scope": "acc://x.acme/ledger",
                         "index": 1000 + i, "txid": "acc://tx%d" % i}
                        for i, p in enumerate(["BVN1", "BVN2", "BVN3", "Directory"] * 38)][:150]
        with contextlib.redirect_stdout(io.StringIO()):
            pr.run_round()
            pr.report()
        return pr

    def test_the_csv_has_a_not_ready_column_apart_from_failed(self):
        self.round()
        with open(readprobe.CSV) as f:
            head, row = f.read().splitlines()[:2]
        r = dict(zip(head.split(","), row.split(",")))
        self.assertEqual("300", r["reads"])
        self.assertEqual("129", r["notReady"])
        self.assertEqual("6", r["failed"])
        self.assertEqual("6", r["timeouts"])

    def test_a_refusal_is_not_a_timed_read(self):
        """A NotReady comes back in under a millisecond: timing it drags the
        median toward zero on exactly the rounds the network is joining."""
        self.round()
        with open(readprobe.REPORT) as f:
            text = f.read()
        self.assertIn("171 timed reads", text)

    def test_the_whole_run_line_names_the_refusals(self):
        self.round()
        with open(readprobe.REPORT) as f:
            text = f.read()
        self.assertIn("6 failed (6 of them timed out, 8s), 129 refused "
                      "NotReady (a joining node's designed answer; not "
                      "timed), 0 refused by the API's query gate (not timed)",
                      text)
        self.assertEqual(
            text.split("**Whole run:**", 1)[1].split("\n", 1)[0].replace("**", "").strip(),
            readprobe.whole_run_row(readprobe.REPORT).split("Whole run:", 1)[1].strip())

    def test_the_rounds_table_has_the_column(self):
        self.round()
        with open(readprobe.REPORT) as f:
            text = f.read()
        self.assertIn("| failed | NotReady | gated | timeouts |", text)


class TheRunsOwnReport(unittest.TestCase):
    """Run 20260924T093936Z's report predates the column: its `1541 failed`
    holds NotReady refusals, errors and 25 timeouts together, and nothing
    in the run separates them. The manifest row must say that, not
    `failed`."""

    def test_the_unsorted_count_is_not_called_failed(self):
        got = readprobe.whole_run_row(os.path.join(RUN, "readprobe-report.md"))
        self.assertNotIn("1541 failed", got)
        self.assertIn("1541 not answered (25 of them timed out, 8s; the other "
                      "1516 are NotReady refusals and errors together — this "
                      "probe predates #4425 and recorded no cause, and its timed reads "
                      "and percentiles include the refusals)", got)
        self.assertIn("5744 timed reads", got)

    def test_no_report_is_not_measured(self):
        self.assertEqual("— not measured (no `readprobe-report.md`)",
                         readprobe.whole_run_row("/nonexistent/readprobe-report.md"))


if __name__ == "__main__":
    unittest.main()
