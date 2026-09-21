#!/usr/bin/env python3
"""add-follower and remove-follower are disturbances soak.sh can perform
(#4364 items 1-2; PLAN.md E11 second pass, "Then bootstrapping a follower").

Today the chaos walk restarts or pauses one existing validator container, the
topology is fixed at compose up, and followers are kept out of the roster on
purpose (#4365). A join is only exercised by a run if the run starts a node
that was not there before, so the walk needs two more kinds: add-follower
(a fresh follower container against the running network — its own key in no
committee, peers via the bootstrap, storage per docker-network.yml) and
remove-follower (stop and remove it).

This runs soak.sh's chaos loop AS WRITTEN — the text between `( end=` and
`CHAOS=$!` — under bash, with `docker` replaced by a recording stub on PATH
and `sleep` by a function that ends the walk after a fixed number of events.
No docker, no network. The knob that turns the follower kinds on is set in a
layered .conf over soak.conf, the way `soak.sh -c` does it: config files, not
the environment.

NOT verified here: that the started container really has a key in no
committee, reaches its peers through the bootstrap, or uses the storage
docker-network.yml names. The stub records the command; only a run shows the
node joined.
"""
import os
import re
import shutil
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
SOAK = os.path.join(HERE, "soak.sh")
sys.path.insert(0, os.path.dirname(HERE))
import topology  # noqa: E402

# Validator events may keep the words soakmon counts today or take the
# -validator suffix; follower events are the two new kinds.
VALIDATOR_KINDS = {"restart", "restart-validator", "pause", "pause-validator"}
FOLLOWER_KINDS = {"add-follower", "remove-follower"}
EVENTS = 9


def chaos_loop():
    """soak.sh's chaos subshell, run in the foreground."""
    with open(SOAK) as f:
        src = f.read()
    i = src.index("\n( end=")
    j = src.index("\nCHAOS=$!", i)
    body = src[i + 1:j].rstrip()
    assert body.endswith("&"), "the chaos loop is no longer `( ... ) &`"
    return body[:-1]


DOCKER_STUB = r"""#!/usr/bin/env bash
printf '%s\n' "$*" >> "$STUB_LOG"
if [ "$1" = ps ]; then cat "$STUB_PS"; fi
exit 0
"""

PRELUDE = r"""
set -uo pipefail
here="$SOAK_HERE"; repo="$(cd "$here/../../.." && pwd)"
. "$here/soak.conf"
. "$LAYER"
rd="$RUN_DIR"; chaos="$rd/chaos.log"; log="$rd/soak.log"
compose_file="$here/../docker-compose.yml"
compose="docker compose -f $compose_file"
duration_seconds=600; CHAOS_MIN=1; CHAOS_JITTER=1
sleep() {
  if [ "$(grep -cE ' (add-follower|remove-follower|restart|restart-validator|pause|pause-validator) ' "$chaos" 2>/dev/null)" -ge "$EVENTS" ]; then
    exit 0
  fi
  return 0
}
"""


class FollowerDisturbance(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp(prefix="follower-disturbance-")
        self.addCleanup(shutil.rmtree, self.tmp, True)
        bindir = os.path.join(self.tmp, "bin")
        os.mkdir(bindir)
        stub = os.path.join(bindir, "docker")
        with open(stub, "w") as f:
            f.write(DOCKER_STUB)
        os.chmod(stub, 0o755)
        self.stub_log = os.path.join(self.tmp, "docker.log")
        open(self.stub_log, "w").close()
        ps = os.path.join(self.tmp, "ps.txt")
        with open(ps, "w") as f:
            f.write("\n".join(topology.containers()) + "\n")
        layer = os.path.join(self.tmp, "followers.conf")
        with open(layer, "w") as f:
            f.write("CHAOS_FOLLOWERS=on\n")
        self.env = dict(os.environ, PATH=bindir + os.pathsep + os.environ["PATH"],
                        STUB_LOG=self.stub_log, STUB_PS=ps, SOAK_HERE=HERE,
                        LAYER=layer, RUN_DIR=self.tmp, EVENTS=str(EVENTS))
        self.chaos = os.path.join(self.tmp, "chaos.log")

    def walk(self):
        p = subprocess.run(["bash", "-c", PRELUDE + chaos_loop()], env=self.env,
                           capture_output=True, text=True, timeout=60)
        self.assertEqual(0, p.returncode, p.stderr)
        events = []
        with open(self.chaos) as f:
            for ln in f:
                parts = ln.split()
                if len(parts) >= 3 and parts[1] in VALIDATOR_KINDS | FOLLOWER_KINDS:
                    events.append((parts[1], parts[2]))
        with open(self.stub_log) as f:
            calls = f.read().splitlines()
        return events, calls

    def test_add_then_remove_follower_are_walked_in_turn(self):
        events, calls = self.walk()
        kinds = [k for k, _ in events]
        self.assertIn("add-follower", kinds,
                      "a chaos walk configured for followers never added one: %s" % events)
        self.assertIn("remove-follower", kinds, events)
        self.assertTrue({"restart", "restart-validator"} & set(kinds),
                        "the validator walk stopped restarting validators: %s" % events)

        validators = set(topology.validator_containers())
        followers = set(topology.follower_containers())
        added = None
        for kind, name in events:
            if kind == "add-follower":
                self.assertIsNone(added, "a second follower added before the first was removed")
                self.assertNotIn(name, validators, "add-follower named a validator")
                added = name
            elif kind == "remove-follower":
                self.assertEqual(added, name, "remove-follower named a container never added")
                added = None
            else:
                self.assertIn(name, validators,
                              "the validator walk disturbed a non-validator: %s %s" % (kind, name))
                self.assertNotIn(name, followers)

        # In turn: all three kinds land within the first two rounds of the walk.
        self.assertTrue({"add-follower", "remove-follower"} <= set(kinds[:6]),
                        "follower kinds are not interleaved with the validator walk: %s" % kinds)

        # The docker calls: the follower is started, later stopped and removed,
        # and is never paused or restarted.
        name = next(n for k, n in events if k == "add-follower")
        mine = [c for c in calls if re.search(r"(^|[\s=/])%s(\s|$)" % re.escape(name), c)]
        start = [i for i, c in enumerate(mine)
                 if re.match(r"(run|create|start|compose\b.*\b(up|run|create)\b)", c)]
        remove = [i for i, c in enumerate(mine) if re.match(r"(rm|compose\b.*\b(rm|down)\b)", c)]
        self.assertTrue(start, "no docker call started %s: %s" % (name, mine))
        self.assertTrue(remove, "no docker call removed %s: %s" % (name, mine))
        self.assertLess(start[0], remove[-1], "removed before it was started: %s" % mine)
        for c in calls:
            if re.match(r"(pause|unpause|restart)\b", c):
                target = c.split()[-1]
                self.assertNotIn(target, followers | {name},
                                 "a follower was disturbed by the validator walk: %s" % c)


if __name__ == "__main__":
    unittest.main()
