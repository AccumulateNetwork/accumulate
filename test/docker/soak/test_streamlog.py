#!/usr/bin/env python3
"""streamlog reads a stall, a value that went backwards, and a numbering gap
out of the lines a block logs."""
import io, os, sys, unittest
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import streamlog

L = [
    "acc-bvn1-val1  | \x1b[90m2026-09-15T05:40:00Z\x1b[0m INFO Stream position \x1b[36mblock=\x1b[0m100 ledger=synthetic source=BVN2 delivered=50 advanced=2 sighted=52 reach=52 held=2 waiting=0",
    "acc-bvn1-val1  | 2026-09-15T05:40:01Z INFO Stream position block=101 ledger=synthetic source=BVN2 delivered=52 advanced=2 sighted=60 reach=60 held=5 waiting=55",
    "acc-bvn1-val1  | 2026-09-15T05:42:01Z INFO Stream position block=221 ledger=synthetic source=BVN2 delivered=52 advanced=0 sighted=61 reach=60 held=6 waiting=55",
    "acc-bvn1-val1  | 2026-09-15T05:42:02Z INFO Stream position block=222 ledger=anchors source=Directory delivered=9 advanced=1 sighted=9 reach=0 held=0 waiting=0",
    "acc-bvn1-val1  | 2026-09-15T05:42:03Z INFO Stream position block=223 ledger=anchors source=Directory delivered=8 advanced=0 sighted=9 reach=0 held=1 waiting=9",
    "acc-bvn1-val1  | 2026-09-15T05:40:00Z INFO Stream produced block=100 destination=Directory from=1 to=3 count=3",
    "acc-bvn1-val1  | 2026-09-15T05:40:01Z INFO Stream produced block=101 destination=Directory from=4 to=4 count=1",
    "acc-bvn1-val1  | 2026-09-15T05:40:02Z INFO Stream produced block=102 destination=Directory from=7 to=8 count=2",
    "acc-bvn1-val1  | 2026-09-15T05:40:03Z INFO Block execution accounting arrived=1",
]


class ReadsTheRecord(unittest.TestCase):
    def test_streams_and_producers(self):
        streams, producers = streamlog.read(L)
        s = streams[("acc-bvn1-val1", "synthetic", "BVN2")]
        self.assertEqual((s.delivered, s.sighted, s.waiting, s.held), (52, 61, 55, 6))
        self.assertEqual(s.advances, 2)
        self.assertEqual(s.last_advance_block, 101)
        self.assertEqual(s.regressions, [])
        a = streams[("acc-bvn1-val1", "anchors", "Directory")]
        self.assertEqual(a.regressions, [("2026-09-15T05:42:03Z", "delivered", 9, 8)])
        p = producers[("acc-bvn1-val1", "Directory")]
        self.assertEqual(p.to, 8)
        self.assertEqual(p.count, 6)
        self.assertEqual(p.gaps, [("2026-09-15T05:40:02Z", 5, 7)])

    def test_report_names_the_stall_and_the_regression(self):
        streams, producers = streamlog.read(L)
        out = io.StringIO()
        streamlog.report(streams, producers, stall=60, out=out)
        text = out.getvalue()
        self.assertIn("STALLED 120s, waiting on 55", text)
        self.assertIn("delivered WENT BACKWARDS 2026-09-15T05:42:03Z 9->8", text)
        self.assertIn("GAP 2026-09-15T05:40:02Z: expected 5, got 7", text)

    def test_filters(self):
        streams, _ = streamlog.read(L, node="acc-bvn2-val1")
        self.assertEqual(streams, {})
        streams, _ = streamlog.read(L, stream="anchors:Directory")
        self.assertEqual(list(streams), [("acc-bvn1-val1", "anchors", "Directory")])


class TheReadersOwnDefects(unittest.TestCase):
    """Found in review: it crashed on any non-numeric field, and printed the
    last SAMPLE where it said last ADVANCE and "waiting since"."""

    LINES = [
        "n | 2026-09-15T05:40:00Z INFO Stream position module=stream block=100 ledger=synthetic source=BVN2 delivered=50 advanced=2 sighted=52 reach=52 held=2 waiting=0",
        "n | 2026-09-15T05:40:01Z INFO Stream position module=stream block=101 ledger=synthetic source=BVN2 delivered=52 advanced=2 sighted=60 reach=60 held=5 waiting=55",
        "n | 2026-09-15T05:45:01Z INFO Stream position module=stream block=221 ledger=synthetic source=BVN2 delivered=52 advanced=0 sighted=61 reach=60 held=6 waiting=55",
    ]

    def test_a_non_numeric_field_does_not_abort_the_analysis(self):
        bad = self.LINES + [
            "n | 2026-09-15T05:45:02Z INFO Stream position module=stream block=222 ledger=anchors source=Directory delivered=18,446 advanced=0 sighted=bogus held=x waiting=0",
            "n | 2026-09-15T05:45:03Z INFO Stream produced module=stream block=222 destination=Directory from=what to=8 count=two",
        ]
        streams, producers = streamlog.read(bad)
        a = streams[("n", "anchors", "Directory")]
        self.assertEqual(a.delivered, 18446, "a thousands separator is still a number")
        self.assertEqual(a.sighted, 0, "an unreadable value is zero, not a crash")
        out = io.StringIO()
        streamlog.report(streams, producers, stall=60, out=out)
        self.assertIn("anchors   <- Directory", out.getvalue())

    def test_last_advance_is_the_advance_not_the_last_sample(self):
        streams, _ = streamlog.read(self.LINES)
        out = io.StringIO()
        streamlog.report(streams, {}, stall=60, out=out)
        text = out.getvalue()
        self.assertIn("last advance 2026-09-15T05:40:01Z (block 101)", text)
        self.assertNotIn("last advance 2026-09-15T05:45:01Z", text)

    def test_waiting_since_is_when_the_hole_appeared(self):
        streams, _ = streamlog.read(self.LINES)
        out = io.StringIO()
        streamlog.report(streams, {}, stall=60, out=out)
        self.assertIn("STALLED 300s, waiting on 55 since 2026-09-15T05:40:01Z", out.getvalue())

    def test_a_stream_that_never_advanced_reports_without_crashing(self):
        lines = ["n | 2026-09-15T05:40:00Z INFO Stream position module=stream block=1 ledger=synthetic source=BVN2 delivered=0 advanced=0 sighted=9 reach=0 held=3 waiting=1",
                 "n | 2026-09-15T05:50:00Z INFO Stream position module=stream block=601 ledger=synthetic source=BVN2 delivered=0 advanced=0 sighted=9 reach=0 held=3 waiting=1"]
        streams, _ = streamlog.read(lines)
        out = io.StringIO()
        streamlog.report(streams, {}, stall=60, out=out)
        self.assertIn("STALLED 600s, waiting on 1 since 2026-09-15T05:40:00Z", out.getvalue())

    def test_the_default_stall_clears_the_emitters_cadence(self):
        # A caught-up stream logs every StreamLogEvery (60) blocks; a
        # default at or under that reads healthy silence as a stall.
        self.assertGreater(streamlog.DEFAULT_STALL, 60)


if __name__ == "__main__":
    unittest.main()
