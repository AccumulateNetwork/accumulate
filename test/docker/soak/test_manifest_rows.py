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
import datetime
import os
import re
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
SOAK = os.path.join(HERE, "soak.sh")
sys.path.insert(0, HERE)
import runseries

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
        # `here` is soak.sh's own variable, and the helpers pass it so they
        # can import runseries — the series is built in one place for the
        # stranded row and the step table both (#4364).
        script = ('#!/usr/bin/env bash\nrd="$1"\nhere="%s"\n' % HERE
                  + self.fns + "\n" + what + "\n")
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

    The figure is NOT monotone though its inputs are, so a level is the
    MINIMUM over a window and never one reading. Two mistakes were made in
    the windows, each printing the opposite of the truth, and both are
    pinned below:

    - a minimum over a WHOLE interval sits at its start, so a rise in the
      middle of a quiet stretch is billed to the next disturbance — a
      violation printed as compliance;
    - an after-window starting at the disturbance's own second picks up
      the sample stamped there, which still reads the pre-effect level
      because a relay must time out before it gives up — so an ordinary
      lossy restart prints +0 and its loss as the creep after it,
      compliance printed as a violation.

    Every fixture here samples every 30s, the real `submissions.csv`
    cadence; at sparser spacing the windows hold one reading and the
    minimum is not a floor.
    """

    def chaos(self, *lines):
        with open(os.path.join(self.rd, "chaos.log"), "w") as f:
            for l in lines:
                f.write(l + "\n")

    def ramp(self, first, last, *changes):
        """A sample every 30s from `first` to `last`, taking the latest
        value at or before each tick. `changes` is (time, value), earliest
        first, and the first one must be at or before `first`."""
        def t(x):
            return datetime.datetime.strptime(
                x, "%Y-%m-%dT%H:%M:%SZ").replace(tzinfo=datetime.timezone.utc)
        pts, now, end = [], t(first), t(last)
        ch = [(t(a), b) for a, b in changes]
        while now <= end:
            v = [b for a, b in ch if a <= now][-1]
            pts.append((now.strftime("%Y-%m-%dT%H:%M:%SZ"), v))
            now += datetime.timedelta(seconds=30)
        self.write(*["%s,acc-bvn3-fol1,follower,BVN3,0,,0,0,0,0,0,%d,periodic"
                     % (a, b) for a, b in pts])

    def two_restarts(self):
        self.chaos("2026-09-20T01:10:00Z restart acc-bvn3-val1",
                   "2026-09-20T01:20:00Z restart acc-bvn2-val1")

    # --- absence ----------------------------------------------------------

    def test_no_chaos_log_is_not_measured(self):
        self.write()
        self.assertIn("not measured (no `chaos.log`)",
                      self.call("steps_rows follower"))

    def test_chaos_off_says_so_rather_than_showing_nothing(self):
        self.chaos("2026-09-20T01:00:00Z DISABLED for this run (CHAOS=off)")
        self.ramp("2026-09-20T01:00:00Z", "2026-09-20T01:02:00Z",
                  ("2026-09-20T01:00:00Z", 0))
        self.assertIn("chaos off", self.call("steps_rows follower"))

    def test_disturbances_but_no_stranded_series_is_not_measured(self):
        self.chaos("2026-09-20T01:10:00Z restart acc-bvn3-val1")
        self.write()
        self.assertIn("not measured", self.call("steps_rows follower"))

    # --- the two window mistakes -------------------------------------------

    def test_a_creep_between_disturbances_is_not_the_next_ones_step(self):
        """Flat through a restart at 01:10, +3 at 01:16 with nothing
        happening, flat through a restart at 01:20. Measured over whole
        intervals this printed `01:20Z restart … (+3)` — a criterion
        violation rendered as a disturbance's cost."""
        self.two_restarts()
        self.ramp("2026-09-20T01:07:00Z", "2026-09-20T01:23:00Z",
                  ("2026-09-20T01:07:00Z", 0), ("2026-09-20T01:16:00Z", 3))
        got = self.call("steps_rows follower")
        self.assertIn("| 01:10Z restart acc-bvn3-val1 | stranded 0 -> 0 (+0)",
                      got)
        self.assertIn("| 01:20Z restart acc-bvn2-val1 | stranded 3 -> 3 (+0)",
                      got)
        self.assertIn("| between 01:10Z and 01:20Z | crept +3 |", got)
        self.assertIn("| largest step at a disturbance | +0,", got)
        self.assertIn("| largest climb between disturbances | +3, "
                      "01:10Z to 01:20Z |", got)

    def test_a_loss_one_sample_after_the_restart_is_that_restarts_step(self):
        """The mirror. The sample stamped at 01:10:00 still reads 0 — a
        relay has to time out before it gives up — and the loss of 3
        appears at 01:10:30. An after-window starting at 01:10:00 takes
        the 0, prints `+0`, and reports the loss as the creep that
        follows: compliance printed as a violation, on the ordinary case."""
        self.two_restarts()
        self.ramp("2026-09-20T01:07:00Z", "2026-09-20T01:23:00Z",
                  ("2026-09-20T01:07:00Z", 0), ("2026-09-20T01:10:30Z", 3))
        got = self.call("steps_rows follower")
        self.assertIn("| 01:10Z restart acc-bvn3-val1 | stranded 0 -> 3 (+3)",
                      got)
        self.assertIn("| between 01:10Z and 01:20Z | crept +0 |", got)
        self.assertIn("| largest step at a disturbance | +3, at 01:10Z "
                      "restart acc-bvn3-val1 |", got)
        self.assertIn("| largest climb between disturbances | +0,", got)

    def test_a_loss_two_samples_after_the_restart_is_still_its_step(self):
        self.two_restarts()
        self.ramp("2026-09-20T01:07:00Z", "2026-09-20T01:23:00Z",
                  ("2026-09-20T01:07:00Z", 0), ("2026-09-20T01:11:00Z", 3))
        got = self.call("steps_rows follower")
        self.assertIn("| 01:10Z restart acc-bvn3-val1 | stranded 0 -> 3 (+3)",
                      got)
        self.assertIn("| between 01:10Z and 01:20Z | crept +0 |", got)

    # --- jitter, pauses, baseline ------------------------------------------

    def test_jitter_between_samples_is_neither_a_step_nor_a_creep(self):
        """Isolated spikes while relays are in flight, returning to 0 each
        time. Every window still holds a low reading, so every floor is 0."""
        self.two_restarts()
        self.ramp("2026-09-20T01:07:00Z", "2026-09-20T01:23:00Z",
                  ("2026-09-20T01:07:00Z", 0),
                  ("2026-09-20T01:09:00Z", 6), ("2026-09-20T01:09:30Z", 0),
                  ("2026-09-20T01:11:00Z", 7), ("2026-09-20T01:11:30Z", 0),
                  ("2026-09-20T01:19:00Z", 4), ("2026-09-20T01:19:30Z", 0),
                  ("2026-09-20T01:21:00Z", 5), ("2026-09-20T01:21:30Z", 0))
        got = self.call("steps_rows follower")
        self.assertIn("| 01:10Z restart acc-bvn3-val1 | stranded 0 -> 0 (+0)",
                      got)
        self.assertIn("| 01:20Z restart acc-bvn2-val1 | stranded 0 -> 0 (+0)",
                      got)
        self.assertIn("| largest step at a disturbance | +0,", got)
        self.assertIn("the figure did not climb", got)

    def test_a_pauses_step_is_dated_at_the_un_pause(self):
        """chaos logs `pause <node> <p>s` when it STARTS and never logs the
        end, so the effective moment is 01:20:30 and the after-window runs
        from 01:21:30. A loss from 01:21:30 is the pause's."""
        self.chaos("2026-09-20T01:10:00Z restart acc-bvn3-val1",
                   "2026-09-20T01:20:00Z pause acc-bvn2-val2 30s")
        self.ramp("2026-09-20T01:07:00Z", "2026-09-20T01:23:00Z",
                  ("2026-09-20T01:07:00Z", 0), ("2026-09-20T01:21:30Z", 6))
        got = self.call("steps_rows follower")
        self.assertIn("| 01:10Z restart acc-bvn3-val1 | stranded 0 -> 0 (+0)",
                      got)
        self.assertIn("| 01:20Z pause acc-bvn2-val2 | stranded 0 -> 6 (+6)",
                      got)

    def test_the_baseline_is_stated(self):
        """Otherwise the first step is measured against nothing and a run
        that was already losing reads as if the disturbance caused it."""
        self.chaos("2026-09-20T01:10:00Z restart acc-bvn3-val1")
        self.ramp("2026-09-20T01:07:00Z", "2026-09-20T01:14:00Z",
                  ("2026-09-20T01:07:00Z", 12))
        got = self.call("steps_rows follower")
        self.assertIn("| baseline (the first 120s of the run) | 12 |", got)
        self.assertIn("stranded 12 -> 12 (+0)", got)

    # --- what cannot be separated ------------------------------------------

    def test_two_disturbances_inside_one_window_say_they_are_not_separable(self):
        """A short run's chaos cadence is 25s. The step and the creep beside
        it are then measured over the same samples, and a reader must be
        told rather than left to believe the attribution."""
        self.chaos("2026-09-20T01:10:00Z restart acc-bvn3-val1",
                   "2026-09-20T01:10:40Z restart acc-bvn2-val1")
        self.ramp("2026-09-20T01:08:00Z", "2026-09-20T01:13:00Z",
                  ("2026-09-20T01:08:00Z", 0))
        self.assertIn("not separable", self.call("steps_rows follower"))

    def test_a_second_disturbance_inside_the_settle_is_not_separable(self):
        """A gap wider than the settle but narrower than the window leaves
        no room for the first one's effect to show before the second
        lands. Caught by the window clamp, which is why there is no
        separate settle check: while the settle is shorter than the
        window, one implies the other."""
        self.chaos("2026-09-20T01:10:00Z restart acc-bvn3-val1",
                   "2026-09-20T01:10:50Z restart acc-bvn2-val1")
        self.ramp("2026-09-20T01:08:00Z", "2026-09-20T01:13:00Z",
                  ("2026-09-20T01:08:00Z", 0))
        self.assertIn("not separable", self.call("steps_rows follower"))

    def test_a_settle_that_swallows_the_window_is_refused_once(self):
        """The misconfiguration the settle makes possible. Said once and
        plainly, rather than as "no sample in the window" against every
        disturbance of a twelve-hour run — which reads as a broken series
        and sends the reader to the wrong file."""
        self.chaos("2026-09-20T01:10:00Z restart acc-bvn3-val1")
        self.ramp("2026-09-20T01:07:00Z", "2026-09-20T01:14:00Z",
                  ("2026-09-20T01:07:00Z", 0))
        got = self.call(
            "STEP_SETTLE_SECS=200 STEP_WINDOW_SECS=120 steps_rows follower")
        self.assertIn("not measured", got)
        self.assertIn("STEP_SETTLE_SECS=200 is not less than "
                      "STEP_WINDOW_SECS=120", got)
        self.assertNotIn("stranded 0 ->", got)

    def test_an_interval_with_no_sample_says_so(self):
        self.chaos("2026-09-20T01:10:00Z restart acc-bvn3-val1",
                   "2026-09-20T01:10:30Z restart acc-bvn2-val1")
        self.ramp("2026-09-20T01:05:00Z", "2026-09-20T01:05:30Z",
                  ("2026-09-20T01:05:00Z", 0))
        got = self.call("steps_rows follower")
        self.assertIn("no complete sample in the", got)
        # And an unmeasured disturbance does not vanish from the summary:
        # it used to leave both rows silent about it (#4364, F1).
        self.assertIn("2 of 2 disturbances not measured", got)


class AnIncompleteSampleIsNotAReading(Rows):
    """A fleet total is a reading only when every (node, partition)
    reported. A container mid-restart answers no scrape — `_scrape_one`
    stores an empty list on a failed curl and the container stays in the
    set, so its topology-seeded rows are blank — and summing what is left
    makes the series DIP by that node's real count on exactly the sample it
    was unreachable.

    The window minimum then reads the dip as the settled level: the loss at
    the restart is masked, or the step goes negative. On a chaos run that
    is every restart, which is every disturbance the table exists to
    measure (reviewer M on #4364).

    An incomplete sample is dropped, not carried forward — carrying forward
    invents a reading at a time nobody measured — and both rows say how
    many were dropped, because a node absent for a long stretch is a
    finding and not noise.
    """

    A = "acc-bvn3-fol1"
    B = "acc-bvn3-val1"

    def row(self, t, node, part, stranded):
        v = "" if stranded is None else str(stranded)
        return ("%s,%s,follower,%s,%s,,,%s,,,,%s,periodic"
                % (t, node, part, "10" if stranded is not None else "",
                   "10" if stranded is not None else "", v))

    def sample(self, t, a, b):
        """One sample: node A's pair and node B's pair, either a count or
        blank (the node answered no scrape)."""
        return [self.row(t, self.A, "BVN3", a), self.row(t, self.B, "BVN3", b)]

    def chaos(self, *lines):
        with open(os.path.join(self.rd, "chaos.log"), "w") as f:
            for l in lines:
                f.write(l + "\n")

    def test_a_node_unreachable_for_one_sample_does_not_mask_a_loss(self):
        """B is unreachable at 01:10:30, the sample right after the
        restart, and A really lost 4 there. Summing what is left reads 4
        where the truth is 4 + B's 6 = 10 — a dip, which the window
        minimum takes as settled and reports as no loss at all."""
        self.chaos("2026-09-20T01:10:00Z restart acc-bvn3-val1")
        rows = []
        for t, a, b in (("2026-09-20T01:08:00Z", 0, 6),
                        ("2026-09-20T01:08:30Z", 0, 6),
                        ("2026-09-20T01:09:00Z", 0, 6),
                        ("2026-09-20T01:09:30Z", 0, 6),
                        ("2026-09-20T01:10:30Z", 4, None),   # B unreachable
                        ("2026-09-20T01:11:00Z", 4, 6),
                        ("2026-09-20T01:11:30Z", 4, 6),
                        ("2026-09-20T01:12:00Z", 4, 6)):
            rows += self.sample(t, a, b)
        self.write(*rows)
        got = self.call("steps_rows follower")
        # 6 before, 10 after: the loss is visible and not masked by the dip.
        self.assertIn("stranded 6 -> 10 (+4)", got)
        self.assertIn("| samples dropped as incomplete | 1 of 8", got)

    def test_a_node_with_no_row_at_all_drops_the_sample_too(self):
        """Not only a blank: a sample where one pair has no row is the
        same fact and must read the same way."""
        self.chaos("2026-09-20T01:10:00Z restart acc-bvn3-val1")
        rows = []
        for t, a, b in (("2026-09-20T01:08:00Z", 0, 6),
                        ("2026-09-20T01:08:30Z", 0, 6),
                        ("2026-09-20T01:09:00Z", 0, 6),
                        ("2026-09-20T01:09:30Z", 0, 6),
                        ("2026-09-20T01:11:00Z", 4, 6),
                        ("2026-09-20T01:11:30Z", 4, 6),
                        ("2026-09-20T01:12:00Z", 4, 6)):
            rows += self.sample(t, a, b)
        rows += [self.row("2026-09-20T01:10:30Z", self.A, "BVN3", 4)]  # no B
        self.write(*rows)
        got = self.call("steps_rows follower")
        self.assertIn("stranded 6 -> 10 (+4)", got)
        self.assertIn("| samples dropped as incomplete | 1 of 8", got)

    def test_the_stranded_row_skips_them_and_says_so(self):
        """`sub_row`'s headline is "0 at the last sample after the drain" —
        a dip in the FINAL row would read as a drained network."""
        rows = []
        for t, a, b in (("2026-09-20T01:08:00Z", 4, 6),
                        ("2026-09-20T01:08:30Z", 4, 6),
                        ("2026-09-20T01:09:00Z", 4, None)):   # the last one
            rows += self.sample(t, a, b)
        self.write(*rows)
        got = self.call("sub_row follower 2026-09-20T01:09:00Z")
        self.assertTrue(got.startswith("10,"), got)
        self.assertIn("1 sample skipped as incomplete", got)
        self.assertIn("INCLUDING THE LAST", got)

    def test_no_complete_sample_at_all_is_not_measured(self):
        rows = self.sample("2026-09-20T01:08:00Z", 4, None)
        rows += self.sample("2026-09-20T01:08:30Z", 4, None)
        self.write(*rows)
        self.assertIn("not measured",
                      self.call("sub_row follower 2026-09-20T01:09:00Z"))


class ARemovedAndReAddedNode(Rows):
    """#4364's own run removes and re-adds the follower, so this is the
    run and not an edge case.

    Prometheus counters are process-local: a re-added node starts again at
    0, its pre-restart cumulative loss LEAVES the fleet series, and the
    floor steps down. Two consequences, both silent:

    - a later loss of that size is masked — the series only has to climb
      back to where it was before anything reads as a climb;
    - "the figure does not climb between disturbances" is satisfied by
      construction at every re-add, which is the criterion the whole table
      exists to judge.

    A decrease in a pair's `accepted` is the reset signal (counters are
    monotone within a process), and from then on the pair carries an
    offset of everything it had stranded before.
    """

    P = "acc-bvn3-fol1"

    def chaos(self, *lines):
        with open(os.path.join(self.rd, "chaos.log"), "w") as f:
            for l in lines:
                f.write(l + "\n")

    def write_run(self, *pts):
        """(time, accepted, stranded) — either both numbers or both blank."""
        self.write(*[
            "%s,%s,follower,BVN3,%s,,,%s,,,,%s,periodic"
            % (t, self.P, "" if a is None else a, "" if a is None else a,
               "" if x is None else x)
            for t, a, x in pts])

    REVIEWERS_CASE = (
        # stranded 4, settled, before the disturbance
        ("2026-09-20T01:18:00Z", 100, 4), ("2026-09-20T01:18:30Z", 110, 4),
        ("2026-09-20T01:19:00Z", 120, 4), ("2026-09-20T01:19:30Z", 130, 4),
        # removed at 01:20 — four samples with nothing to report
        ("2026-09-20T01:20:30Z", None, None), ("2026-09-20T01:21:00Z", None, None),
        ("2026-09-20T01:21:30Z", None, None), ("2026-09-20T01:22:00Z", None, None),
        # re-added: a NEW process, so the counters start again at 0
        ("2026-09-20T01:22:30Z", 5, 0), ("2026-09-20T01:23:00Z", 12, 0),
        ("2026-09-20T01:23:30Z", 20, 0),
        # and then it loses two
        ("2026-09-20T01:24:00Z", 28, 2), ("2026-09-20T01:24:30Z", 35, 2),
        ("2026-09-20T01:25:00Z", 42, 2), ("2026-09-20T01:25:30Z", 50, 2),
        ("2026-09-20T01:26:00Z", 57, 2), ("2026-09-20T01:26:30Z", 64, 2))

    def test_the_series_does_not_step_down_at_the_re_add(self):
        self.chaos("2026-09-20T01:20:00Z restart acc-bvn3-fol1")
        self.write_run(*self.REVIEWERS_CASE)
        got = self.call("steps_rows follower")
        # 4 before, 4 after: the re-add cost nothing, and the 4 it had
        # stranded before is still in the series.
        self.assertIn("| 01:20Z restart acc-bvn3-fol1 | stranded 4 -> 4 (+0)",
                      got)
        self.assertIn("| largest step at a disturbance | +0,", got)

    def test_the_loss_after_the_re_add_is_a_creep(self):
        self.chaos("2026-09-20T01:20:00Z restart acc-bvn3-fol1")
        self.write_run(*self.REVIEWERS_CASE)
        got = self.call("steps_rows follower")
        self.assertIn("crept +2", got)
        self.assertIn("| largest climb between disturbances | +2,", got)

    def test_the_reset_is_stated(self):
        """A reader who does not know a reset happened cannot account for
        the floor, so it is a row and not a silent correction."""
        self.chaos("2026-09-20T01:20:00Z restart acc-bvn3-fol1")
        self.write_run(*self.REVIEWERS_CASE)
        got = self.call("steps_rows follower")
        self.assertIn("| counter resets seen | 1 (acc-bvn3-fol1/BVN3 at "
                      "01:22Z carrying 4)", got)
        self.assertIn("does not step down", got)
        self.assertIn("| samples dropped as incomplete | 4 of", got)

    def test_without_the_offset_the_loss_is_masked(self):
        """What the uncorrected series reads, so the test says what the
        correction is worth: the floor drops to 0 at the re-add, the step
        is -4, and the later loss of 2 never reaches 4 — so nothing
        climbs and the criterion passes on a run that lost two."""
        S = self.series()
        raw = [s["total"] - sum(
            S["resets"] and [4] or [0]) for s in runseries.complete(S)]
        self.assertEqual(4, runseries.complete(S)[0]["total"])
        self.assertEqual(4, runseries.complete(S)[4]["total"],
                         "corrected: the re-added node still carries its 4")
        self.assertEqual(6, runseries.complete(S)[-1]["total"],
                         "corrected: 4 carried plus the 2 it just lost")
        self.assertEqual(0, raw[4], "uncorrected, the floor steps down to 0")
        self.assertLess(raw[-1], 4,
                        "uncorrected, the later loss never reaches the "
                        "old floor, so nothing reads as a climb")

    def series(self):
        self.write_run(*self.REVIEWERS_CASE)
        return runseries.load(os.path.join(self.rd, "submissions.csv"),
                              "follower")

    def test_the_stranded_row_carries_the_reset_too(self):
        self.write_run(*self.REVIEWERS_CASE)
        got = self.call("sub_row follower 2026-09-20T01:26:30Z")
        self.assertTrue(got.startswith("6,"), got)
        self.assertIn("1 counter reset carried forward", got)
        self.assertIn("4 samples skipped as incomplete", got)


class TheAfterFloorIsTheWholeWindow(Rows):
    """reviewer M1. The after-floor took the first two complete samples
    from the settle, on the reasoning that a later loss should not be
    billed to the disturbance. A minimum already ignores a later loss — a
    loss RAISES the figure — so all that could do was exclude later,
    LOWER readings, overstating the step and understating the creep
    beside it.

    This is also the CALL SITE's test, which the reviewer asked for by
    name: remove the argument from `floor_of` alone and the suite stayed
    green, because the earlier mutation changed the function and not its
    caller. These fixtures fail either way, because they read the number
    the manifest prints.
    """

    P = "acc-bvn3-fol1"

    def chaos(self, *lines):
        with open(os.path.join(self.rd, "chaos.log"), "w") as f:
            for l in lines:
                f.write(l + "\n")

    def write_run(self, *pts):
        self.write(*[
            "%s,%s,follower,BVN3,%s,,,%s,,,,%s,periodic"
            % (t, self.P, "" if a is None else a, "" if a is None else a,
               "" if x is None else x)
            for t, a, x in pts])

    # 4 settled before; after the disturbance the relays drain: 5, 4, 3, 3.
    # The settled level after is 3 — so the disturbance cost nothing and
    # the figure came DOWN. With the first two samples it reads 4, the
    # step reads +0 instead of -1, and the creep after it is understated
    # by the same 1.
    DRAINING = (("2026-09-20T01:18:00Z", 100, 4),
                ("2026-09-20T01:18:30Z", 110, 4),
                ("2026-09-20T01:19:00Z", 120, 4),
                ("2026-09-20T01:19:30Z", 130, 4),
                ("2026-09-20T01:21:00Z", 140, 5),
                ("2026-09-20T01:21:30Z", 150, 4),
                ("2026-09-20T01:22:00Z", 160, 3),
                ("2026-09-20T01:22:30Z", 170, 3),
                ("2026-09-20T01:23:00Z", 180, 3),
                ("2026-09-20T01:23:30Z", 190, 3))

    WIDE = "STEP_WINDOW_SECS=180 STEP_SETTLE_SECS=60"

    def test_a_lower_later_sample_is_part_of_the_floor(self):
        """Read at a window wide enough to hold more than two samples —
        see `test_the_shipped_window_holds_exactly_two_samples` for why
        that matters."""
        self.chaos("2026-09-20T01:20:00Z restart acc-bvn3-fol1")
        self.write_run(*self.DRAINING)
        got = self.call(self.WIDE + " steps_rows follower")
        self.assertIn("| 01:20Z restart acc-bvn3-fol1 | stranded 4 -> 3 (-1)",
                      got, "the settled level after is 3, not 4")
        self.assertNotIn("stranded 4 -> 4 (+0)", got)

    def test_the_window_still_ends_at_W(self):
        """A sample past the window is not in the floor, however low: the
        window is chosen by its BOUNDS, which is what `first_n` was
        reaching for and got wrong."""
        self.chaos("2026-09-20T01:20:00Z restart acc-bvn3-fol1")
        self.write_run(*(self.DRAINING[:4] + (
            ("2026-09-20T01:21:00Z", 140, 9),
            ("2026-09-20T01:21:30Z", 150, 9),
            ("2026-09-20T01:22:00Z", 155, 9),
            ("2026-09-20T01:22:30Z", 158, 9),
            ("2026-09-20T01:23:30Z", 160, 0),   # past t + W
            ("2026-09-20T01:24:00Z", 170, 0))))
        got = self.call(self.WIDE + " steps_rows follower")
        self.assertIn("stranded 4 -> 9 (+5)", got)

    def test_the_shipped_window_holds_exactly_two_samples(self):
        """Worth recording rather than leaving to be rediscovered: at the
        shipped `STEP_WINDOW_SECS=120` and `STEP_SETTLE_SECS=60` the
        after-window is [t+60, t+120), which at the 30s submissions.csv
        cadence holds exactly TWO samples — so `first_n=2` was a no-op on
        today's configuration and the overstatement it caused could not
        occur. It would the moment either knob moved, which is why the
        argument is gone rather than left because it happens to be
        harmless."""
        self.chaos("2026-09-20T01:20:00Z restart acc-bvn3-fol1")
        self.write_run(*self.DRAINING)
        got = self.call("steps_rows follower")          # the shipped knobs
        self.assertIn("stranded 4 -> 4 (+0)", got)
        wide = self.call(self.WIDE + " steps_rows follower")
        self.assertIn("stranded 4 -> 3 (-1)", wide)


class ANodeAwayThroughTheSettleWindow(Rows):
    """F1. A node unreachable through the whole settle window used to give
    `— not measured` on its step and then VANISH from both summary rows,
    with the creep after it never measured at all — so a disturbance
    nobody could measure read as a run with nothing to report."""

    P = "acc-bvn3-fol1"

    def chaos(self, *lines):
        with open(os.path.join(self.rd, "chaos.log"), "w") as f:
            for l in lines:
                f.write(l + "\n")

    def write_run(self, *pts):
        self.write(*[
            "%s,%s,follower,BVN3,%s,,,%s,,,,%s,periodic"
            % (t, self.P, "" if a is None else a, "" if a is None else a,
               "" if x is None else x)
            for t, a, x in pts])

    def test_a_late_after_floor_is_taken_and_says_how_late(self):
        """Back at 01:22:30, which is 90s past the settle and past the
        window. The floor is the first two complete samples from there."""
        self.chaos("2026-09-20T01:20:00Z restart acc-bvn3-fol1")
        self.write_run(("2026-09-20T01:18:30Z", 100, 4),
                 ("2026-09-20T01:19:00Z", 110, 4),
                 ("2026-09-20T01:19:30Z", 120, 4),
                 ("2026-09-20T01:20:30Z", None, None),
                 ("2026-09-20T01:21:00Z", None, None),
                 ("2026-09-20T01:21:30Z", None, None),
                 ("2026-09-20T01:22:00Z", None, None),
                 ("2026-09-20T01:22:30Z", 130, 9),
                 ("2026-09-20T01:23:00Z", 140, 9),
                 ("2026-09-20T01:23:30Z", 150, 9))
        got = self.call("steps_rows follower")
        self.assertIn("stranded 4 -> 9 (+5)", got)
        self.assertIn("after-floor is 90s late", got)
        self.assertNotIn("not measured", got)

    def test_an_unmeasured_disturbance_stays_in_the_summary(self):
        """The node is there for the first disturbance and never comes
        back after the second, so the second's step cannot be taken at
        all. It used to vanish from both summary rows; now they say how
        many were not measured, beside the ones that were.

        An event is unmeasurable only when the node is absent from the
        settle right through to the next disturbance — and since that also
        empties the NEXT one's before-window, the only way to have one of
        two is for the absence to run to the end of the run."""
        self.chaos("2026-09-20T01:20:00Z restart acc-bvn3-fol1",
                   "2026-09-20T01:30:00Z restart acc-bvn2-val1")
        self.write_run(("2026-09-20T01:18:30Z", 100, 4),
                       ("2026-09-20T01:19:00Z", 110, 4),
                       ("2026-09-20T01:19:30Z", 120, 4),
                       ("2026-09-20T01:21:00Z", 130, 4),
                       ("2026-09-20T01:21:30Z", 140, 4),
                       ("2026-09-20T01:28:30Z", 200, 4),
                       ("2026-09-20T01:29:00Z", 210, 4),
                       ("2026-09-20T01:29:30Z", 220, 4),
                       ("2026-09-20T01:30:30Z", None, None),
                       ("2026-09-20T01:31:00Z", None, None),
                       ("2026-09-20T01:31:30Z", None, None))
        got = self.call("steps_rows follower")
        self.assertIn("| 01:20Z restart acc-bvn3-fol1 | stranded 4 -> 4 (+0)",
                      got)
        self.assertIn("| 01:30Z restart acc-bvn2-val1 | — not measured", got)
        self.assertIn("1 of 2 disturbances not measured", got)
        self.assertIn("| largest step at a disturbance | +0,", got)


class AbsentIsNotZero(Rows):
    """A labelled Prometheus counter has no child series until it is first
    incremented, so three facts arrive looking alike and must not read alike
    (REPORTING-SPEC 1, and run `20260919T231856Z`, where `relayedRefused`,
    `relayedNotReady` and `relayedUnreachable` were EMPTY in 300 of 300 rows
    and the manifest printed `0 refused / 0 target not ready / 0
    unreachable`). The run-analyst had to open `relay.go` to learn the
    zeros were true.

    - the family is exported and this outcome never happened -> a real `0`,
      and the row says `0 (series present)`;
    - the family is not exported at all -> `— not measured`;
    - nothing was written -> `— not measured`, with the reason.
    """

    TAKEN_ONLY = ("%s,acc-bvn3-fol1,follower,BVN3,1146,,,1146,,,,0,periodic")
    NOTHING = ("%s,acc-bvn3-fol1,follower,BVN3,1146,,,,,,,0,periodic")

    def test_an_outcome_that_never_happened_reads_zero_and_says_so(self):
        self.write(self.TAKEN_ONLY % "2026-09-20T01:00:00Z",
                   self.TAKEN_ONLY % "2026-09-20T01:00:30Z")
        got = self.call("relay_row follower")
        self.assertIn("1146 taken", got)
        self.assertIn("0 (series present) refused", got)
        self.assertIn("0 (series present) target not ready", got)
        self.assertIn("0 (series present) unreachable", got)
        self.assertNotIn("| 0 refused", got)

    def test_a_family_nobody_exports_is_not_measured(self):
        self.write(self.NOTHING % "2026-09-20T01:00:00Z",
                   self.NOTHING % "2026-09-20T01:00:30Z")
        got = self.call("relay_row follower")
        self.assertIn("not measured", got)
        self.assertIn("accumulate_dagbft_relayed_total", got)
        self.assertNotIn("0 taken", got)

    def test_a_series_that_starts_mid_run_counts_as_present(self):
        """Family presence is judged over the whole file, not the last
        sample: a counter first incremented mid-run is exported from then
        on, and the last sample alone would call the earlier ones absent."""
        self.write(self.NOTHING % "2026-09-20T01:00:00Z",
                   "2026-09-20T01:00:30Z,acc-bvn3-fol1,follower,BVN3,"
                   "1146,,,1140,6,,,0,periodic",
                   self.TAKEN_ONLY % "2026-09-20T01:01:00Z")
        got = self.call("relay_row follower")
        self.assertIn("1146 taken", got)
        self.assertIn("0 (series present) refused", got)


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

    def test_the_storage_row_matches_what_docker_network_yml_holds(self):
        """The pattern was `[a-z]+` and the file says `BlockchainDB`, so run
        20260919T231856Z recorded a blank storage backend — on a run that
        was verifiably on BlockchainDB. Run the manifest's own expression
        against the real file, not a copy of it."""
        net = os.path.join(HERE, "..", "docker-network.yml")
        line = next(l for l in open(net) if l.startswith("database:"))
        self.assertIn("BlockchainDB", line, "the fixture this pins moved")
        expr = re.search(r"sed -nE 's/\^database:[^']*'", self.SRC)
        self.assertIsNotNone(expr, "the storage row's sed is gone")
        out = subprocess.run(["sed", "-nE",
                              expr.group(0).split("'")[1], net],
                             capture_output=True, text=True, timeout=30)
        self.assertEqual("BlockchainDB", out.stdout.strip().splitlines()[0])

    def test_a_missing_database_line_is_not_a_blank_cell(self):
        self.assertIn("no `database:` line in docker-network.yml", self.SRC)

    def test_the_uncommitted_count_and_the_patch_are_the_same_set(self):
        """`status --porcelain` counts tracked changes AND untracked files;
        `git diff` captured only unstaged tracked ones, so the manifest read
        `uncommitted files | 1` beside a zero-byte patch and a reader could
        not tell a dirty tree from a broken capture."""
        self.assertIn('diff HEAD > "$rd/config/uncommitted.patch"', self.SRC)
        self.assertIn("--untracked-files=no", self.SRC)
        self.assertIn("ls-files --others --exclude-standard", self.SRC)
        self.assertIn("untracked (in no patch)", self.SRC)
        self.assertIn("the capture failed", self.SRC)
        self.assertNotIn('diff > "$rd/config/uncommitted.patch"', self.SRC)

    def test_both_helpers_are_given_the_path_to_runseries(self):
        """The series is built in one place or the two rows drift apart —
        which is how one of them printed a violation as compliance while
        the other said nothing (#4364). Both helpers import `runseries`,
        and both are handed `$here` to find it."""
        for fn in ("sub_row", "steps_rows"):
            body = re.search(r"^%s\(\) \{.*?\nPYEOF\n\}" % fn,
                             self.SRC, re.S | re.M)
            self.assertIsNotNone(body, fn)
            self.assertIn('"$here"', body.group(0), fn)
            self.assertIn("import runseries", body.group(0), fn)

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
        self.assertIn("after-window starts", flat)
        self.assertIn("still reads the pre-effect", flat)
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


class NodeStateRow(unittest.TestCase):
    """The manifest's start-to-ACTIVE row (#4364), read from the nodestate.csv
    soakmon writes — produced here by soakmon's own writer, so the file the
    row parses is the file the monitor makes."""

    def setUp(self):
        import sys
        sys.path.insert(0, HERE)
        import soakmon
        self.soakmon = soakmon
        self.rd = tempfile.mkdtemp(prefix="nodestaterow-")
        with open(SOAK) as f:
            m = re.search(r"^nodestate_row\(\) \{.*?\nPYEOF\n\}\n", f.read(), re.S | re.M)
        self.assertIsNotNone(m, "no nodestate_row in soak.sh")
        self.fn = m.group(0)

    def row(self, events, role):
        if events is not None:
            with open(os.path.join(self.rd, "nodestate.csv"), "w") as f:
                f.write(self.soakmon.NODESTATE_CSV_HEADER + "\n")
                for line in self.soakmon.nodestate_csv_rows(events, "2026-09-21T00:00:00Z"):
                    f.write(line + "\n")
        path = os.path.join(self.rd, "run.sh")
        with open(path, "w") as f:
            f.write('#!/usr/bin/env bash\nrd="$1"\n' + self.fn + "\nnodestate_row %s\n" % role)
        out = subprocess.run(["bash", path, self.rd], capture_output=True, text=True, timeout=60)
        self.assertEqual("", out.stderr.strip(), out.stderr)
        return out.stdout.strip()

    def ev(self, node, part, started, s2a, kind, state="ACTIVE", role="validator"):
        return {"node": node, "role": role, "partition": part, "containerStarted": started,
                "state": state, "startToActiveS": s2a, "kind": kind}

    def test_no_file_is_not_measured(self):
        self.assertIn("not measured", self.row(None, "validator"))

    def test_worst_restart_and_a_start_that_never_became_active(self):
        got = self.row([
            self.ev("acc-bvn1-val1", "bvn1", 100, 4000.0, "already"),
            self.ev("acc-bvn1-val2", "bvn1", 5000, 35.0, "reached"),
            self.ev("acc-bvn1-val2", "directory", 5000, 90.0, "reached"),
            self.ev("acc-bvn2-val1", "bvn2", 6000, None, "final", state="BOOTING"),
            self.ev("acc-bvn3-fol1", "bvn3", 7000, 12.0, "reached", role="follower"),
        ], "validator")
        self.assertIn("worst 90.0s (acc-bvn1-val2 directory) over 2 start(s)", got)
        self.assertIn("1 ACTIVE at first sight", got)
        self.assertIn("NEVER ACTIVE: acc-bvn2-val1 bvn2 (BOOTING)", got)
        self.assertNotIn("4000", got, "an upper bound is not the worst")

    def test_a_final_row_completed_after_a_monitor_restart_is_not_stuck(self):
        got = self.row([
            self.ev("acc-bvn3-fol1", "bvn3", 7000, None, "final", state="BOOTING", role="follower"),
            self.ev("acc-bvn3-fol1", "bvn3", 7000, 300.0, "already", role="follower"),
        ], "follower")
        self.assertIn("every start reached ACTIVE", got)
