#!/usr/bin/env python3
"""Every background job soak.sh starts is named at teardown (#4364).

The symptom: after both runs on 2026-09-19 a bare `sleep 300` was left on the
box and Paul killed it by hand. Two causes, both in `soak.sh`:

1. Three sampler loops were started as `( while … ) &` with **no `$!`**, so
   teardown's kill list could not name them. They ended only when their own
   `while kill -0 $DRIVER` next ran — after the sleep, up to
   `STORAGE_STATS_INTERVAL`, which `soak.conf` sets to 300.
2. Even with the PID, a plain `kill` orphans the `sleep` the subshell is
   waiting on. Measured on this machine with two identical
   `( while true; do sleep 300; done ) &` jobs:

       plain kill: subshell gone | its 'sleep 300' ALIVE
       stop_bg   : subshell gone | its 'sleep 300' gone

This cannot be proved against a live run from here, and a run must confirm
it: `soak.log` must carry no `survived teardown` warning, and `pgrep -x
sleep` must be clean once the script exits. What CAN be pinned is the shape
— that no background job is started without recording its PID, and that
every recorded PID reaches `stop_bg`. That is the part that rots: the last
three were added one at a time, each copying the one above it.
"""
import os
import re
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
SOAK = os.path.join(HERE, "soak.sh")

with open(SOAK) as _fh:
    SRC = _fh.read()
LINES = SRC.splitlines()

# Jobs whose PID is deliberately not in the stop_bg list, with the reason.
# STALLKILL is not killed here at all: when it is the one ending the run it
# waits for this script to finish recording and then takes the network down,
# so killing it would leave the containers up (soak.sh says so at teardown).
# READPROBE is signalled and waited on separately, before the network goes,
# so its last round and its report land.
EXEMPT = {"STALLKILL": "it takes the network down after this script exits",
          "READPROBE": "signalled and waited on separately so its report lands",
          "DRIVER": "the load generator; `wait $DRIVER` is the run"}


def background_starts():
    """(line number, the variable the next line assigns from `$!`, the line)."""
    out = []
    for i, line in enumerate(LINES):
        if not line.rstrip().endswith("&") or line.rstrip().endswith("2>&1") \
                or re.search(r"\d>&\d\s*$", line):
            continue
        if line.lstrip().startswith("#"):
            continue
        # The assignment is on the next non-blank line.
        nxt = ""
        for j in range(i + 1, min(i + 3, len(LINES))):
            if LINES[j].strip():
                nxt = LINES[j].strip()
                break
        m = re.match(r"([A-Z_][A-Z0-9_]*)=\$!", nxt)
        out.append((i + 1, m.group(1) if m else None, line.strip()))
    return out


class EveryBackgroundJobRecordsItsPid(unittest.TestCase):
    def test_there_are_background_jobs_to_check(self):
        """A parser that finds nothing passes every assertion below."""
        self.assertGreaterEqual(len(background_starts()), 8)

    def test_none_is_started_without_capturing_its_pid(self):
        orphans = [(n, l) for n, v, l in background_starts() if v is None]
        self.assertEqual(
            [], orphans,
            "a background job with no `$!` cannot be named at teardown — "
            "this is exactly how the `sleep 300` sampler outlived two runs")

    def test_the_three_sampler_loops_are_the_ones_that_regressed(self):
        names = {v for _, v, _ in background_starts()}
        for v in ("MONLOOP", "STORELOOP", "PROFLOOP"):
            self.assertIn(v, names, v)


class TeardownNamesThemAll(unittest.TestCase):
    def test_stop_bg_kills_the_child_before_the_subshell(self):
        """Order is the fix: `pkill -P` makes the `sleep` return, so the
        subshell does not leave it orphaned for the rest of its interval."""
        body = re.search(r"stop_bg\(\)\s*\{(.*?)\n\}", SRC, re.S)
        self.assertIsNotNone(body, "stop_bg is gone")
        b = body.group(1)
        self.assertLess(b.index("pkill -P"), b.index('kill "$p"'),
                        "children first, or the sleep is orphaned")

    def test_every_captured_pid_is_passed_to_stop_bg(self):
        call = re.search(r"\nstop_bg ((?:.|\\\n)*?)\n(?=[^ ])", SRC)
        self.assertIsNotNone(call, "teardown does not call stop_bg")
        passed = set(re.findall(r"\$\{([A-Z_][A-Z0-9_]*):-\}", call.group(1)))
        for _, v, line in background_starts():
            if v is None or v in EXEMPT:
                continue
            self.assertIn(v, passed, "%s is started and never stopped: %s"
                          % (v, line))

    def test_the_exemptions_are_stated_not_forgotten(self):
        """Each one is a decision, and the decision is in the script."""
        for v in EXEMPT:
            self.assertIn(v, SRC, v)

    def test_a_survivor_is_reported_rather_than_left_to_be_noticed(self):
        self.assertIn("survived teardown", SRC)


if __name__ == "__main__":
    unittest.main()
