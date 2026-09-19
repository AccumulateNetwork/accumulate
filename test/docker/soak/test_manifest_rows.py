#!/usr/bin/env python3
"""The manifest's two submission rows, run as `soak.sh` defines them (#4364).

These are shell functions embedded in `soak.sh`, and the test extracts them
from that file and runs them rather than keeping a copy — a copy would pass
while the script it stands for was wrong, which is the failure this project
names most often.

Two defects they had, both found on review and both silent:

**N1 — two readings of one counter summed.** The final row is forced past
the 30-second interval, timestamps are whole seconds, so roughly one run in
thirty lands it in the same second as a periodic row. Summing every row at
the last timestamp then reported **1,600 taken for 800**, and the stranded
headline contradicted its own trend — `80` beside `rising 80 -> 160 …
(stranded)` — in the direction of a false alarm, on the one row the
acceptance rule is read against.

**N2 — a lost final row read as the drained sample.** "0 at the last sample
after the drain" is unreadable unless a reader can tell whether the final
write landed. The instruction was in a note; nothing in the harness checked
it. Now the row says so, from the `sample` column and from the load
generator's exit time — two separate facts, neither inferred from the other,
because an idle tail puts periodic rows after the load generator too.
"""
import os
import re
import subprocess
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
SOAK = os.path.join(HERE, "soak.sh")

HEADER = ("time,node,role,partition,accepted,rejected,certified,relayedTaken,"
          "relayedRefused,relayedNotReady,relayedUnreachable,"
          "acceptedNeitherCertifiedTakenNorRefused,sample")


def _functions():
    """`relay_row` and `sub_row`, lifted out of soak.sh verbatim."""
    with open(SOAK) as f:
        src = f.read()
    fns = [m.group(0) for m in
           re.finditer(r"^(relay_row|sub_row)\(\) \{.*?\nPYEOF\n\}\n",
                       src, re.S | re.M)]
    if len(fns) != 2:
        raise AssertionError(
            "expected relay_row and sub_row in soak.sh, found %d — the "
            "extraction is stale and this suite is testing nothing"
            % len(fns))
    return "".join(fns)


class Rows(unittest.TestCase):
    def setUp(self):
        self.rd = tempfile.mkdtemp(prefix="manifestrows-")
        self.fns = _functions()

    def write(self, *rows):
        with open(os.path.join(self.rd, "submissions.csv"), "w") as f:
            f.write(HEADER + "\n")
            for r in rows:
                f.write(r + "\n")

    def call(self, what):
        script = '#!/usr/bin/env bash\nrd="$1"\n' + self.fns + "\n" + what + "\n"
        path = os.path.join(self.rd, "run.sh")
        with open(path, "w") as f:
            f.write(script)
        out = subprocess.run(["bash", path, self.rd], capture_output=True,
                             text=True, timeout=60)
        self.assertEqual("", out.stderr.strip(), out.stderr)
        return out.stdout.strip()


class TwoReadingsOfOneCounterAreOneReading(Rows):
    """N1. Counters, not increments: the same counter read twice in one
    second is one reading, and the later row is the fresher scrape."""

    PERIODIC = "2026-09-20T03:00:30Z,acc-bvn3-fol1,follower,BVN3,880,,0,800,50,20,10,80,periodic"
    FINAL = "2026-09-20T03:00:30Z,acc-bvn3-fol1,follower,BVN3,880,,0,800,50,20,10,80,final"
    EARLIER = "2026-09-20T03:00:00Z,acc-bvn3-fol1,follower,BVN3,880,,0,800,50,20,10,80,periodic"

    def test_the_relay_row_does_not_double(self):
        self.write(self.EARLIER, self.PERIODIC, self.FINAL)
        got = self.call('relay_row follower')
        self.assertIn("800 taken / 50 refused / 20 target not ready / "
                      "10 unreachable", got)
        self.assertNotIn("1600", got)

    def test_the_stranded_row_does_not_double(self):
        self.write(self.EARLIER, self.PERIODIC, self.FINAL)
        got = self.call('sub_row follower 2026-09-20T03:00:20Z')
        self.assertTrue(got.startswith("80,"), got)
        self.assertNotIn("160", got)

    def test_the_headline_does_not_contradict_its_own_trend(self):
        """The failure as it read: `80, … rising 80 -> 160 … (stranded)`."""
        self.write(self.EARLIER, self.PERIODIC, self.FINAL)
        got = self.call('sub_row follower 2026-09-20T03:00:20Z')
        self.assertIn("flat at 80 -> 80", got)
        self.assertNotIn("rising", got)


class WhetherTheFinalRowLanded(Rows):
    """N2. Two facts, separately: did soakmon's exit write happen, and does
    the row postdate the load generator."""

    DRAINING = "2026-09-20T03:00:00Z,acc-bvn3-fol1,follower,BVN3,600,,0,560,0,0,0,40,periodic"
    FINAL = "2026-09-20T03:00:45Z,acc-bvn3-fol1,follower,BVN3,880,,0,880,0,0,0,0,final"
    PERIODIC = "2026-09-20T03:00:30Z,acc-bvn3-fol1,follower,BVN3,880,,0,880,0,0,0,0,periodic"

    def test_a_clean_run_says_the_final_row_was_written(self):
        self.write(self.DRAINING, self.FINAL)
        got = self.call('sub_row follower 2026-09-20T03:00:20Z')
        self.assertIn("final row written", got)
        self.assertIn("25s after the load generator exited", got)
        self.assertNotIn("MISSING", got)

    def test_a_lost_final_write_is_named_not_silent(self):
        self.write(self.DRAINING, self.PERIODIC)
        got = self.call('sub_row follower 2026-09-20T03:00:20Z')
        self.assertIn("FINAL ROW MISSING", got)
        self.assertIn("mid-drain", got)

    def test_a_row_that_predates_the_load_generators_exit_says_so(self):
        """An idle tail puts periodic rows after the load generator too, so
        the timestamp alone cannot answer the first question — and the
        `sample` column alone cannot answer this one."""
        self.write(self.DRAINING, self.PERIODIC)
        got = self.call('sub_row follower 2026-09-20T03:01:00Z')
        self.assertIn("BEFORE the load generator exited", got)

    def test_a_file_without_the_column_is_unknown_not_missing(self):
        """A run from before this column existed did not lose its final
        row; nobody recorded one. Those are different facts."""
        old = HEADER.rsplit(",", 1)[0]
        with open(os.path.join(self.rd, "submissions.csv"), "w") as f:
            f.write(old + "\n")
            f.write(self.PERIODIC.rsplit(",", 1)[0] + "\n")
        got = self.call('sub_row follower 2026-09-20T03:00:20Z')
        self.assertIn("final row: unknown", got)
        self.assertNotIn("MISSING", got)

    def test_stallkill_says_the_sample_is_not_drained(self):
        """stallkill kills the load generator mid-flight. The final row
        still lands — soakmon gets TERM, not KILL — but it is not the
        drained sample the acceptance rule means."""
        self.write(self.DRAINING, self.FINAL)
        got = self.call('sub_row follower 2026-09-20T03:00:20Z stallkill')
        self.assertIn("stopped by stallkill", got)
        self.assertIn("NOT a drained sample", got)

    def test_without_stallkill_no_such_warning(self):
        self.write(self.DRAINING, self.FINAL)
        got = self.call('sub_row follower 2026-09-20T03:00:20Z')
        self.assertNotIn("stallkill", got)


class AbsenceStillReadsAsAbsence(Rows):
    def test_no_file(self):
        got = self.call('sub_row follower 2026-09-20T03:00:20Z')
        self.assertIn("not measured", got)

    def test_a_header_with_no_rows(self):
        self.write()
        self.assertIn("not measured",
                      self.call('sub_row follower 2026-09-20T03:00:20Z'))
        self.assertIn("not measured", self.call('relay_row follower'))


class SoakShPassesWhatTheHelpersNeed(unittest.TestCase):
    """The helpers are only as good as their call sites."""

    with open(SOAK) as _fh:
        SRC = _fh.read()
    del _fh

    def test_the_load_generators_exit_time_is_captured(self):
        self.assertIn("lg_exit=$(date -u +%FT%TZ)", self.SRC)
        self.assertLess(self.SRC.index("wait $DRIVER"),
                        self.SRC.index("lg_exit=$(date"),
                        "it must be taken when the generator exits, not later")

    def test_stallkill_is_detected_from_the_manifest(self):
        self.assertIn("Stopped early by stallkill", self.SRC)
        self.assertIn("stopped_early=stallkill", self.SRC)

    def test_both_call_sites_pass_them(self):
        calls = re.findall(r"\$\(sub_row [^)]*\)", self.SRC)
        self.assertEqual(2, len(calls), calls)
        for c in calls:
            self.assertIn('"$lg_exit"', c, c)
            self.assertIn('"$stopped_early"', c, c)


if __name__ == "__main__":
    unittest.main()
