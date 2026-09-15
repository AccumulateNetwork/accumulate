#!/usr/bin/env python3
"""The shell watchdogs read /data through embedded Python. They are run here
against a payload shaped like the monitor's, because the seizure watchdog
read an unmeasured `stuck` as a healthy zero and could never trip on it, and
status.sh printed `None` for every healing field (#4279 review)."""
import json, os, re, subprocess, sys, unittest

HERE = os.path.dirname(os.path.abspath(__file__))


def embedded(script, start, end):
    """The Python between two markers of a shell script, exactly as run."""
    s = open(os.path.join(HERE, script)).read()
    i = s.index(start) + len(start)
    return s[i:s.index(end, i)]


def run(code, payload):
    p = subprocess.run([sys.executable, "-c", code], input=json.dumps(payload), capture_output=True, text=True)
    if p.returncode:
        raise AssertionError(p.stderr)
    return p.stdout


SEIZE = embedded("seizewatch.sh", "python3 -c '", "' 2>/dev/null)\"")
STATUS = embedded("status.sh", "python3 -c '", "'\n")


class Seizewatch(unittest.TestCase):
    def payload(self, stuck, gap):
        return {"heals": {"stuck": stuck, "stuckStream": ""},
                "matrix": {"flows": {"synthetic": {"BVN1": {"Directory": {
                    "sent": 33846, "recv": 32469 + gap, "deliv": 32469}}}}}}

    def test_an_unmeasured_stuck_count_is_not_zero(self):
        out = run(SEIZE, self.payload(None, 9))
        self.assertIn("stuck=n/a", out)
        self.assertNotIn("stuck=0", out)
        # The shell trips on `stuck=[0-9]+`; n/a does not match, so it
        # falls back to 0 for the comparison only.
        self.assertIsNone(re.search(r"stuck=\d+", out))

    def test_a_measured_stuck_count_is_printed(self):
        self.assertIn("stuck=7", run(SEIZE, self.payload(7, 0)))

    def test_the_gap_and_undelivered_signals_survive(self):
        out = run(SEIZE, self.payload(None, 9))
        self.assertIn("worst=BVN1->Directory gap=9", out)
        self.assertIn("undeliv=synthetic BVN1->Directory undeliv=1368", out)


class Status(unittest.TestCase):
    def test_reads_the_families_the_monitor_serves(self):
        d = {"started": 0, "heights": {}, "chaos": {"counts": {}}, "wedges": {"total": 5, "byReason": {"queue-full": 5}, "byDest": {"BVN1": 5}},
             "heals": {"entries": 18171, "requests": {"answered": 129, "not-yet": 1515, "miss": 7, "failed": 0},
                       "held": {"entries": 33, "bytes": 5363}},
             "matrix": {"flows": {"synthetic": {}}}}
        out = run(STATUS, d)
        self.assertIn("entries=18171", out)
        self.assertIn("answered=129", out)
        self.assertIn("held        33 entries, 5363 bytes", out)
        self.assertNotIn("None", out)

    def test_absent_is_said_not_printed_as_none(self):
        d = {"started": 0, "heights": {}, "chaos": {"counts": {}}, "wedges": {"total": None, "byReason": {}, "byDest": {}},
             "heals": {"entries": None, "requests": None, "held": None}, "matrix": {"flows": {}}}
        out = run(STATUS, d)
        self.assertIn("not measured", out)
        self.assertNotIn("None", out)


if __name__ == "__main__":
    unittest.main()
