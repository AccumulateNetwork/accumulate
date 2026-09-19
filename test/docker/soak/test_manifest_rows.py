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
    """The manifest's row helpers, lifted out of soak.sh verbatim."""
    with open(SOAK) as f:
        src = f.read()
    fns = [m.group(0) for m in
           re.finditer(r"^(relay_row|sub_row|steps_rows)\(\) \{"
                       r".*?\nPYEOF\n\}\n", src, re.S | re.M)]
    if len(fns) != 3:
        raise AssertionError(
            "expected relay_row, sub_row and steps_rows in soak.sh, found "
            "%d — the extraction is stale and this suite is testing nothing"
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


class StepsPerDisturbance(Rows):
    """#4364's acceptance criterion — "the figure does not climb between
    disturbances, and every step is attributable to one of them" — was
    computable from `submissions.csv` and `chaos.log` and judgeable from
    neither: `sub_row`'s trend looks at the last five samples of a
    twelve-hour run.

    Two numbers, because the criterion is two claims: a **step** is what a
    disturbance cost, a **creep** is the climb in the quiet stretch between
    two of them, and the criterion's number is the largest creep.

    The figure is NOT monotone though its inputs are — it rises when a
    submission is accepted and falls when the relay is answered — so a
    level is the MINIMUM over a window and never one reading. The windows
    are LOCAL to each disturbance: a minimum over a whole interval sits at
    the interval's start, so a rise in the middle of a quiet stretch first
    appears as the next interval's minimum and gets billed to the next
    disturbance. That printed a violation as compliance, which is the
    defect these tests were rewritten for.
    """

    def chaos(self, *lines):
        with open(os.path.join(self.rd, "chaos.log"), "w") as f:
            for l in lines:
                f.write(l + "\n")

    def series(self, *pairs):
        self.write(*["%s,acc-bvn3-fol1,follower,BVN3,0,,0,0,0,0,0,%d,periodic"
                     % (t, v) for t, v in pairs])

    def test_no_chaos_log_is_not_measured(self):
        self.write()
        self.assertIn("not measured (no `chaos.log`)",
                      self.call("steps_rows follower"))

    def test_chaos_off_says_so_rather_than_showing_nothing(self):
        self.chaos("2026-09-20T01:00:00Z DISABLED for this run (CHAOS=off)")
        self.series(("2026-09-20T01:00:00Z", 0))
        self.assertIn("chaos off", self.call("steps_rows follower"))

    def test_disturbances_but_no_stranded_series_is_not_measured(self):
        self.chaos("2026-09-20T01:10:00Z restart acc-bvn3-val1")
        self.write()
        self.assertIn("not measured (no stranded series",
                      self.call("steps_rows follower"))

    def test_a_creep_between_disturbances_is_not_the_next_ones_step(self):
        """THE case this table exists for, and the one it got wrong.

        Flat through a restart at 01:10, +3 at 01:16 with nothing
        happening, flat through a restart at 01:20. Measured over whole
        intervals this printed `01:20Z restart … (+3)` and `largest step
        +3 at 01:20Z restart` — a criterion violation rendered as a
        disturbance's cost, and the manifest's own sentence said the
        opposite of the arithmetic.
        """
        self.chaos("2026-09-20T01:10:00Z restart acc-bvn3-val1",
                   "2026-09-20T01:20:00Z restart acc-bvn2-val1")
        self.series(("2026-09-20T01:07:30Z", 0), ("2026-09-20T01:08:30Z", 0),
                    ("2026-09-20T01:09:30Z", 0), ("2026-09-20T01:10:30Z", 0),
                    ("2026-09-20T01:11:30Z", 0), ("2026-09-20T01:16:00Z", 3),
                    ("2026-09-20T01:18:30Z", 3), ("2026-09-20T01:19:30Z", 3),
                    ("2026-09-20T01:20:30Z", 3), ("2026-09-20T01:22:30Z", 3))
        got = self.call("steps_rows follower")
        self.assertIn("| 01:10Z restart acc-bvn3-val1 | stranded 0 -> 0 (+0)",
                      got)
        self.assertIn("| 01:20Z restart acc-bvn2-val1 | stranded 3 -> 3 (+0)",
                      got)
        self.assertIn("| between 01:10Z and 01:20Z | crept +3 |", got)
        self.assertIn("| largest step at a disturbance | +0,", got)
        self.assertIn("| largest climb between disturbances | +3, "
                      "01:10Z to 01:20Z |", got)

    def test_jitter_between_samples_is_neither_a_step_nor_a_creep(self):
        """The case the minimum exists for: the figure spikes to 7 and 5
        while relays are in flight and returns to 0 each time."""
        self.chaos("2026-09-20T01:10:00Z restart acc-bvn3-val1",
                   "2026-09-20T01:20:00Z pause acc-bvn2-val2 30s")
        # The real cadence: a sample every 30s, so a 120s window holds four
        # readings and a spike that resolves inside it is a spike.
        self.series(("2026-09-20T01:08:00Z", 4), ("2026-09-20T01:08:30Z", 0),
                    ("2026-09-20T01:09:00Z", 2), ("2026-09-20T01:09:30Z", 0),
                    ("2026-09-20T01:10:30Z", 7), ("2026-09-20T01:11:00Z", 0),
                    ("2026-09-20T01:11:30Z", 4), ("2026-09-20T01:12:00Z", 0),
                    ("2026-09-20T01:18:30Z", 0), ("2026-09-20T01:19:00Z", 3),
                    ("2026-09-20T01:19:30Z", 0), ("2026-09-20T01:20:40Z", 5),
                    ("2026-09-20T01:21:00Z", 0), ("2026-09-20T01:21:30Z", 2),
                    ("2026-09-20T01:22:00Z", 0))
        got = self.call("steps_rows follower")
        self.assertIn("| 01:10Z restart acc-bvn3-val1 | stranded 0 -> 0 (+0)",
                      got)
        self.assertIn("| 01:20Z pause acc-bvn2-val2 | stranded 0 -> 0 (+0)",
                      got)
        self.assertIn("| largest step at a disturbance | +0,", got)
        self.assertIn("the figure did not climb", got)

    def test_a_real_loss_is_attributed_to_the_disturbance_that_caused_it(self):
        self.chaos("2026-09-20T01:10:00Z restart acc-bvn3-val1",
                   "2026-09-20T01:20:00Z pause acc-bvn2-val2 30s")
        self.series(("2026-09-20T01:07:30Z", 0), ("2026-09-20T01:09:30Z", 0),
                    ("2026-09-20T01:10:30Z", 9), ("2026-09-20T01:11:30Z", 3),
                    ("2026-09-20T01:18:40Z", 3), ("2026-09-20T01:19:40Z", 3),
                    ("2026-09-20T01:20:40Z", 3), ("2026-09-20T01:22:40Z", 3))
        got = self.call("steps_rows follower")
        self.assertIn("| 01:10Z restart acc-bvn3-val1 | stranded 0 -> 3 (+3)",
                      got)
        self.assertIn("| 01:20Z pause acc-bvn2-val2 | stranded 3 -> 3 (+0)",
                      got)
        self.assertIn("| largest step at a disturbance | +3, at 01:10Z "
                      "restart acc-bvn3-val1 |", got)
        self.assertIn("| largest climb between disturbances | +0,", got)

    def test_a_pauses_step_is_dated_at_the_un_pause(self):
        """chaos logs `pause <node> <p>s` when it STARTS and never logs the
        end, so a sample taken during the pause belongs before it. Here the
        loss appears at 01:20:40, after a 30s pause begun at 01:20:00."""
        self.chaos("2026-09-20T01:10:00Z restart acc-bvn3-val1",
                   "2026-09-20T01:20:00Z pause acc-bvn2-val2 30s")
        self.series(("2026-09-20T01:07:30Z", 0), ("2026-09-20T01:09:00Z", 0),
                    ("2026-09-20T01:11:00Z", 0), ("2026-09-20T01:18:40Z", 0),
                    ("2026-09-20T01:20:10Z", 0), ("2026-09-20T01:20:40Z", 6),
                    ("2026-09-20T01:22:40Z", 6))
        got = self.call("steps_rows follower")
        self.assertIn("| 01:10Z restart acc-bvn3-val1 | stranded 0 -> 0 (+0)",
                      got)
        self.assertIn("| 01:20Z pause acc-bvn2-val2 | stranded 0 -> 6 (+6)",
                      got)

    def test_the_baseline_is_stated(self):
        """Otherwise the first step is measured against nothing and a run
        that was already losing reads as if the disturbance caused it."""
        self.chaos("2026-09-20T01:10:00Z restart acc-bvn3-val1")
        self.series(("2026-09-20T01:08:30Z", 12), ("2026-09-20T01:09:30Z", 12),
                    ("2026-09-20T01:11:00Z", 12))
        got = self.call("steps_rows follower")
        self.assertIn("| baseline (the first 120s of the run) | 12 |", got)
        self.assertIn("stranded 12 -> 12 (+0)", got)

    def test_two_disturbances_inside_one_window_say_they_are_not_separable(self):
        """A short run's chaos cadence is 25s. The step and the creep beside
        it are then measured over the same samples, and a reader must be
        told rather than left to believe the attribution."""
        self.chaos("2026-09-20T01:10:00Z restart acc-bvn3-val1",
                   "2026-09-20T01:10:40Z restart acc-bvn2-val1")
        self.series(("2026-09-20T01:08:00Z", 0), ("2026-09-20T01:10:20Z", 0),
                    ("2026-09-20T01:11:00Z", 0), ("2026-09-20T01:13:00Z", 0))
        got = self.call("steps_rows follower")
        self.assertIn("not separable", got)

    def test_an_interval_with_no_sample_says_so(self):
        self.chaos("2026-09-20T01:10:00Z restart acc-bvn3-val1",
                   "2026-09-20T01:10:30Z restart acc-bvn2-val1")
        self.series(("2026-09-20T01:05:00Z", 0), ("2026-09-20T01:15:00Z", 0))
        got = self.call("steps_rows follower")
        self.assertIn("no sample in the", got)


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

    def test_the_steps_table_is_in_the_manifest(self):
        """A helper nothing calls measures nothing."""
        self.assertIn("steps_rows follower", self.SRC)
        self.assertIn("Stranded across disturbances", self.SRC)
        self.assertIn("| disturbance | stranded |", self.SRC)

    def test_the_manifest_says_how_the_numbers_are_taken(self):
        """A reader who does not know a level is a minimum over a local
        window will try to reproduce it from two rows and get the jitter —
        or from whole intervals and bill a creep to a disturbance."""
        flat = " ".join(self.SRC.split())
        self.assertIn("Settled means the MINIMUM over a window", flat)
        self.assertIn("The windows are LOCAL", flat)
        self.assertIn("a rise in the middle of a quiet", flat)
        self.assertIn("A pause is dated at its un-pause", flat)

    def test_the_manifest_names_which_number_the_criterion_is(self):
        """Two numbers on one table is how the wrong one gets quoted."""
        flat = " ".join(self.SRC.split())
        # The prose is built from several `echo` lines, so match on
        # fragments that survive the line breaks rather than a sentence.
        self.assertIn("a **step** is what a disturbance cost", flat)
        self.assertIn("a **creep** is the", flat)
        self.assertIn("The criterion's", flat.replace('" echo "', " "))
        self.assertIn("number is \\`largest climb between disturbances\\`",
                      flat)


if __name__ == "__main__":
    unittest.main()
