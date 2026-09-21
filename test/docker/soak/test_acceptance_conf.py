#!/usr/bin/env python3
"""12h-100tps-followers.conf is phase 1's acceptance run (#4364 item 4;
PLAN.md E11, "Done when").

Paul, 2026-09-19: "the network can run chaos for 12 hours while adding and
removing followers while carrying a 100 tps load." The run that shows it is a
layered .conf, and this test reads it the way soak.sh does: soak.conf sourced
under bash, then the -c file on top, with nothing from the launching shell.

The walk it must yield is the one soak.sh's chaos loop performs when both
walks are on (CHAOS_VALIDATORS, CHAOS_FOLLOWERS): the slots alternate
validator, follower, and the follower slots alternate add-follower,
remove-follower, for the whole run (CHAOS_FOLLOWER_CYCLES=0) with no no-op
slot (CHAOS_SKIP_ONE_IN=0). test_follower_disturbance.py runs that loop.

NOT verified here: that a twelve-hour run under this file passes. Only the run
shows that.
"""
import os
import re
import subprocess
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
SOAK_CONF = os.path.join(HERE, "soak.conf")
ACCEPTANCE = os.path.join(HERE, "12h-100tps-followers.conf")
OLD_24H = os.path.join(HERE, "24h-100tps-chaos.conf")

WORDING = ("the network can run chaos for 12 hours while adding and removing "
           "followers while carrying a 100 tps load")

KNOBS = ("DURATION", "TPS", "CHAOS", "CHAOS_VALIDATORS", "CHAOS_FOLLOWERS",
         "CHAOS_FOLLOWER_CYCLES", "CHAOS_SKIP_ONE_IN")


def layered(conf):
    """The knobs after soak.sh's `. soak.conf; . conf`, in an empty env."""
    script = '. "$1"; . "$2"; ' + "; ".join(
        'printf "%%s=%%s\\n" %s "${%s-}"' % (k, k) for k in KNOBS)
    out = subprocess.run(["bash", "-c", script, "layer", SOAK_CONF, conf],
                         env={"PATH": "/usr/bin:/bin"}, check=True,
                         capture_output=True, text=True).stdout
    return dict(l.split("=", 1) for l in out.splitlines())


def comment(path):
    """The file's comment text, one line, whitespace collapsed."""
    with open(path) as f:
        lines = [l.strip()[1:] for l in f if l.strip().startswith("#")]
    return re.sub(r"\s+", " ", " ".join(lines)).strip()


class AcceptanceConf(unittest.TestCase):

    def test_the_acceptance_conf_is_twelve_hours_of_followers_and_restarts(self):
        self.assertTrue(os.path.isfile(ACCEPTANCE),
                        "12h-100tps-followers.conf does not exist")

        got = layered(ACCEPTANCE)
        self.assertEqual(got["DURATION"], "12h")
        self.assertEqual(got["TPS"], "100")
        self.assertEqual(got["CHAOS"], "on")
        # add-follower / remove-follower / restart-validator in turn
        self.assertEqual(got["CHAOS_VALIDATORS"], "on",
                         "the walk restarts no validator")
        self.assertEqual(got["CHAOS_FOLLOWERS"], "on",
                         "the walk adds and removes no follower")
        self.assertEqual(got["CHAOS_FOLLOWER_CYCLES"], "0",
                         "followers are added and removed for part of the run only")
        self.assertEqual(got["CHAOS_SKIP_ONE_IN"], "0",
                         "the walk has no-op slots")

        self.assertIn(WORDING, comment(ACCEPTANCE).lower(),
                      "the file's comment does not carry phase 1's wording")

        self.assertNotRegex(comment(OLD_24H).lower(), r"acceptance",
                            "24h-100tps-chaos.conf still claims to be an acceptance")


if __name__ == "__main__":
    unittest.main()
