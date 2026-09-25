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
  SECONDS=$(( SECONDS + ${1%%.*} ))   # chaos_wait counts wall clock ($SECONDS)
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
        late = [f["container"] for f in topology.late_followers()]
        self.assertGreater(len(late), 1, "one late follower per BVN (#4438)")
        # Every late follower is added in one slot and removed together in a
        # later one: the follower events come in runs of one kind, each run
        # naming every late follower once, add and remove alternating.
        groups = []
        for kind, name in events:
            if kind in FOLLOWER_KINDS:
                self.assertNotIn(name, validators, "%s named a validator" % kind)
                if groups and groups[-1][0] == kind:
                    groups[-1][1].append(name)
                else:
                    groups.append((kind, [name]))
            else:
                self.assertIn(name, validators,
                              "the validator walk disturbed a non-validator: %s %s" % (kind, name))
                self.assertNotIn(name, followers)
        for i, (kind, names) in enumerate(groups):
            self.assertEqual("add-follower" if i % 2 == 0 else "remove-follower", kind, groups)
            if i < len(groups) - 1 or kind == "remove-follower":
                self.assertEqual(late, names, "a slot did not add or remove every late follower")

        # In turn: all three kinds land within the first two rounds of the walk.
        self.assertTrue({"add-follower", "remove-follower"} <= set(kinds[:6]),
                        "follower kinds are not interleaved with the validator walk: %s" % kinds)

        # The docker calls: each late follower is started, later stopped and
        # removed, and is never paused or restarted.
        for name in late:
            self.started_then_removed(name, calls)
        for c in calls:
            if re.match(r"(pause|unpause|restart)\b", c):
                target = c.split()[-1]
                self.assertNotIn(target, followers,
                                 "a follower was disturbed by the validator walk: %s" % c)

    def started_then_removed(self, name, calls):
        mine = [c for c in calls if re.search(r"(^|[\s=/])%s(\s|$)" % re.escape(name), c)]
        start = [i for i, c in enumerate(mine)
                 if re.match(r"(run|create|start|compose\b.*\b(up|run|create)\b)", c)]
        remove = [i for i, c in enumerate(mine) if re.match(r"(rm|compose\b.*\b(rm|down)\b)", c)]
        self.assertTrue(start, "no docker call started %s: %s" % (name, mine))
        self.assertTrue(remove, "no docker call removed %s: %s" % (name, mine))
        self.assertLess(start[0], remove[-1], "removed before it was started: %s" % mine)


def build_step():
    """soak.sh's build step AS WRITTEN: from `# Build BEFORE up.` to the
    comment that opens the `up`."""
    with open(SOAK) as f:
        src = f.read()
    i = src.index("\n# Build BEFORE up.")
    j = src.index("\n# Surface the up error", i)
    return src[i + 1:j]


IMAGE_STUB = r"""#!/usr/bin/env bash
printf '%s\n' "$*" >> "$STUB_LOG"
if [ "$1" = image ] && [ "$2" = inspect ]; then
  name="${@: -1}"
  grep -qx "$name" "$STUB_IMAGES" || exit 1
  echo "sha256:id-of-$name [$name:latest]"
fi
exit 0
"""

BUILD_PRELUDE = r"""
set -uo pipefail
here="$SOAK_HERE"
. "$here/soak.conf"
. "$LAYER"
export COMPOSE_PROJECT_NAME=disoak
rd="$RUN_DIR"; log="$rd/soak.log"; manifest="$rd/MANIFEST.md"
mkdir -p "$rd/config"
compose_file="$here/../docker-compose.yml"
compose="docker compose -f $compose_file"
"""


class LateFollowerImageIsBuiltAndRecorded(unittest.TestCase):
    """Review e11.4364b.review.2: `compose build` with no profile skips the
    late follower's service, so add-follower ran whatever disoak-bvn3-fol2
    image was lying about (#4103), and the manifest named only bvn1-val1's.

    The build step is run as soak.sh writes it, `docker` a recording stub.
    NOT verified here: that `docker compose --profile late-follower build`
    really builds the profiled service as well as the unprofiled ones. That
    is compose's behaviour, and only a run with docker shows it.
    """

    def setUp(self):
        self.tmp = tempfile.mkdtemp(prefix="follower-image-")
        self.addCleanup(shutil.rmtree, self.tmp, True)
        bindir = os.path.join(self.tmp, "bin")
        os.mkdir(bindir)
        stub = os.path.join(bindir, "docker")
        with open(stub, "w") as f:
            f.write(IMAGE_STUB)
        os.chmod(stub, 0o755)
        self.stub_log = os.path.join(self.tmp, "docker.log")
        open(self.stub_log, "w").close()
        self.images = os.path.join(self.tmp, "images.txt")
        self.layer = os.path.join(self.tmp, "layer.conf")
        self.manifest = os.path.join(self.tmp, "MANIFEST.md")
        with open(self.manifest, "w") as f:
            f.write("| field | value |\n|---|---|\n| image | `disoak-bvn1-val1` |\n"
                    "| image id | `sha256:id-of-disoak-bvn1-val1` |\n"
                    "| executor version | **x** |\n")
        self.env = dict(os.environ, PATH=bindir + os.pathsep + os.environ["PATH"],
                        STUB_LOG=self.stub_log, STUB_IMAGES=self.images,
                        SOAK_HERE=HERE, LAYER=self.layer, RUN_DIR=self.tmp)

    def run_step(self, images, followers):
        with open(self.images, "w") as f:
            f.write("\n".join(images) + "\n")
        with open(self.layer, "w") as f:
            f.write("CHAOS_FOLLOWERS=%s\n" % followers)
        p = subprocess.run(["bash", "-c", BUILD_PRELUDE + build_step() + "\necho REACHED-UP\n"],
                           env=self.env, capture_output=True, text=True, timeout=60)
        with open(self.stub_log) as f:
            calls = f.read().splitlines()
        with open(self.manifest) as f:
            manifest = f.read()
        return p, calls, manifest

    def late(self):
        late = [ln.split() for ln in subprocess.run(
            [sys.executable, os.path.join(HERE, "followerchaos.py"), "late"],
            capture_output=True, text=True, check=True).stdout.splitlines()]
        self.assertTrue(late, "no follower is in the compose's late-follower profile")
        return late

    def test_the_build_names_the_late_follower_profile(self):
        late = self.late()
        p, calls, _ = self.run_step(
            ["disoak-bvn1-val1"] + ["disoak-" + svc for svc, _, _ in late], "on")
        self.assertIn("REACHED-UP", p.stdout, p.stdout + p.stderr)
        builds = [c for c in calls if re.search(r"^compose\b.*\bbuild\b", c)]
        self.assertTrue(builds, "the build step never ran `compose build`: %s" % calls)
        for c in builds:
            self.assertRegex(c, r"--profile late-follower\b.*\bbuild\b",
                             "a build that names no profile skips the late follower's "
                             "service, and add-follower then runs a stale image (#4103)")

    def test_the_manifest_records_the_late_followers_image_id(self):
        late = self.late()
        p, _, manifest = self.run_step(
            ["disoak-bvn1-val1"] + ["disoak-" + svc for svc, _, _ in late], "on")
        self.assertIn("REACHED-UP", p.stdout, p.stdout + p.stderr)
        for svc, container, _ in late:
            rows = [ln for ln in manifest.splitlines()
                    if ln.startswith("|") and container in ln]
            self.assertEqual(1, len(rows), "no manifest row names %s:\n%s" % (container, manifest))
            self.assertIn("sha256:id-of-disoak-" + svc, rows[0])
        # The validators' row is still there, and still theirs.
        self.assertIn("| image id | `sha256:id-of-disoak-bvn1-val1` |", manifest)
        with open(os.path.join(self.tmp, "config", "image-late-follower.txt")) as f:
            frozen = f.read()
        for svc, _, _ in late:
            self.assertIn("sha256:id-of-disoak-" + svc, frozen)

    def test_a_follower_run_refuses_an_unidentifiable_follower_image(self):
        p, _, _ = self.run_step(["disoak-bvn1-val1"], "on")
        self.assertNotEqual(0, p.returncode)
        self.assertNotIn("REACHED-UP", p.stdout,
                         "a run that adds a follower went on with no follower image to name")

    def test_a_run_that_adds_no_follower_says_unknown_and_goes_on(self):
        late = self.late()
        p, _, manifest = self.run_step(["disoak-bvn1-val1"], "off")
        self.assertIn("REACHED-UP", p.stdout, p.stdout + p.stderr)
        for _, container, _ in late:
            self.assertRegex(manifest, r"\|[^\n]*%s[^\n]*unknown" % re.escape(container))


ADD_FOLLOWER_CONF = os.path.join(HERE, "5m-100tps-add-follower.conf")

# The walk's `sleep`, ended after a fixed number of SLOTS rather than events:
# a walk that performs one pair and then does nothing never reaches an event
# count, and the point of the test is what it does NOT do afterwards.
SLOTS_PRELUDE = PRELUDE[:PRELUDE.index("sleep() {")] + r"""
sleep() {
  if [ "$(grep -c ' sleeping ' "$chaos" 2>/dev/null)" -ge "$SLOTS" ]; then
    exit 0
  fi
  SECONDS=$(( SECONDS + ${1%%.*} ))   # chaos_wait counts wall clock ($SECONDS)
  return 0
}
"""


def conf_values(conf, *names):
    """The knobs as soak.sh reads them: soak.conf, then the layer over it."""
    script = '. "$1"; . "$2"; shift 2; for n in "$@"; do printf "%s=%s\\n" "$n" "${!n-}"; done'
    out = subprocess.run(["bash", "-c", script, "conf", os.path.join(HERE, "soak.conf"), conf]
                         + list(names), capture_output=True, text=True, check=True).stdout
    return dict(ln.split("=", 1) for ln in out.splitlines())


class TheAddFollowerConf(FollowerDisturbance):
    """Review e11.4364b.review.3, medium: nothing named CHAOS_VALIDATORS,
    CHAOS_FOLLOWER_CYCLES or 5m-100tps-add-follower.conf, so the run that is
    #4363's Docker proof could restart validators, or add a second follower,
    and no test would say so.

    The same walk as above, with the committed conf as the layer.
    """

    def setUp(self):
        super().setUp()
        self.env.update(LAYER=ADD_FOLLOWER_CONF, SLOTS="8")

    def walk(self):
        p = subprocess.run(["bash", "-c", SLOTS_PRELUDE + chaos_loop()], env=self.env,
                           capture_output=True, text=True, timeout=60)
        self.assertEqual(0, p.returncode, p.stderr)
        with open(self.chaos) as f:
            log = f.read()
        with open(self.stub_log) as f:
            calls = f.read().splitlines()
        return log, calls

    # The inherited walk needs validator restarts, which this conf turns off.
    test_add_then_remove_follower_are_walked_in_turn = None

    def test_it_states_what_it_is(self):
        v = conf_values(ADD_FOLLOWER_CONF, "DURATION", "TPS", "CHAOS", "CHAOS_VALIDATORS",
                        "CHAOS_FOLLOWERS", "CHAOS_FOLLOWER_CYCLES")
        self.assertEqual({"DURATION": "5m", "TPS": "100", "CHAOS": "on",
                          "CHAOS_VALIDATORS": "off", "CHAOS_FOLLOWERS": "on",
                          "CHAOS_FOLLOWER_CYCLES": "1"}, v)
        with open(ADD_FOLLOWER_CONF) as f:
            comment = "".join(ln for ln in f if ln.startswith("#"))
        self.assertRegex(comment, r"#4363's Docker proof")

    def test_the_pair_fits_inside_the_run(self):
        v = conf_values(ADD_FOLLOWER_CONF, "CHAOS_MIN", "CHAOS_JITTER", "FOLLOWER_WINDOW_SECS")
        slot = int(v["CHAOS_MIN"]) + int(v["CHAOS_JITTER"])
        # two slots, then the reading either side of the removal
        self.assertLess(2 * slot + 2 * int(v["FOLLOWER_WINDOW_SECS"]), 300,
                        "the removal's after-reading would fall outside the five minutes")

    def test_the_default_is_no_follower_kinds_and_validators_disturbed(self):
        v = conf_values(os.devnull, "CHAOS_VALIDATORS", "CHAOS_FOLLOWERS", "CHAOS_FOLLOWER_CYCLES")
        self.assertEqual({"CHAOS_VALIDATORS": "on", "CHAOS_FOLLOWERS": "off",
                          "CHAOS_FOLLOWER_CYCLES": "0"}, v,
                         "every existing conf would change what it disturbs")

    def test_one_add_one_remove_and_no_validator_touched(self):
        log, calls = self.walk()
        self.assertGreaterEqual(log.count(" sleeping "), 8, "the walk ended early:\n" + log)
        events = [ln.split()[1:3] for ln in log.splitlines()
                  if len(ln.split()) >= 3 and ln.split()[1] in VALIDATOR_KINDS | FOLLOWER_KINDS]
        late = [ln.split()[1] for ln in subprocess.run(
            [sys.executable, os.path.join(HERE, "followerchaos.py"), "late"],
            capture_output=True, text=True, check=True).stdout.splitlines()]
        self.assertEqual([["add-follower", c] for c in late] +
                         [["remove-follower", c] for c in late], events,
                         "CHAOS_FOLLOWER_CYCLES=1 is one pair — every late follower "
                         "added in one slot, removed in the next — and "
                         "CHAOS_VALIDATORS=off is no validator event, over %d slots" % 8)
        # Each removal line names the one removal it belongs to.
        self.assertEqual(len(late), len(re.findall(r"remove-follower \S+ \(removal 1\)", log)))
        # One join setting and one clearing for all of them, every directory named.
        for f in topology.late_followers():
            d = f["dir"]
            self.assertTrue(any("/root/.accumulate/%s/bvnn/data/accumulate.db" % d in c
                                for c in calls), "%s's databases were not cleared" % d)
            self.assertTrue(any("/root/.accumulate/%s/accumulate.toml" % d in c
                                and "join-running-network" in c for c in calls),
                            "%s was not given join-running-network" % d)
        self.assertRegex(log, r"validators: not disturbed \(CHAOS_VALIDATORS=off\)")
        self.assertRegex(log, r"followers: done, 1 pair\(s\) \(CHAOS_FOLLOWER_CYCLES=1\)")
        disturbed = [c for c in calls if re.match(r"(pause|unpause|restart|kill)\b", c)]
        self.assertEqual([], disturbed, "CHAOS_VALIDATORS=off and a node was still disturbed")


def shell_function(name):
    """One top-level function of soak.sh AS WRITTEN."""
    with open(SOAK) as f:
        src = f.read()
    i = src.index("\n%s() {" % name)
    return src[i + 1:src.index("\n}\n", i) + 2]


INSPECT_STUB = r"""#!/usr/bin/env bash
printf '%s\n' "$*" >> "$STUB_LOG"
case "$1" in
  inspect) grep -qx "${@: -1}" "$STUB_PS" || exit 1 ;;
  logs)    echo "a line of ${@: -1}'s log" ;;
esac
exit 0
"""


class ALateFollowerLeftUpIsRemoved(unittest.TestCase):
    """`compose down` activates no profile, so it does not reach the late
    follower: soak.sh removes it by name, before `up` and at teardown."""

    def setUp(self):
        self.tmp = tempfile.mkdtemp(prefix="follower-leftover-")
        self.addCleanup(shutil.rmtree, self.tmp, True)
        bindir = os.path.join(self.tmp, "bin")
        os.mkdir(bindir)
        stub = os.path.join(bindir, "docker")
        with open(stub, "w") as f:
            f.write(INSPECT_STUB)
        os.chmod(stub, 0o755)
        self.stub_log = os.path.join(self.tmp, "docker.log")
        open(self.stub_log, "w").close()
        self.ps = os.path.join(self.tmp, "ps.txt")
        self.env = dict(os.environ, PATH=bindir + os.pathsep + os.environ["PATH"],
                        STUB_LOG=self.stub_log, STUB_PS=self.ps, SOAK_HERE=HERE, RUN_DIR=self.tmp)
        self.late = [ln.split()[1] for ln in subprocess.run(
            [sys.executable, os.path.join(HERE, "followerchaos.py"), "late"],
            capture_output=True, text=True, check=True).stdout.splitlines()]
        self.assertTrue(self.late)

    def remove(self, existing):
        with open(self.ps, "w") as f:
            f.write("\n".join(existing) + "\n")
        script = ('set -uo pipefail\nhere="$SOAK_HERE"; rd="$RUN_DIR"; log="$rd/soak.log"\n'
                  + shell_function("rm_late_followers") + '\nrm_late_followers "left-up"\n')
        p = subprocess.run(["bash", "-c", script], env=self.env,
                           capture_output=True, text=True, timeout=60)
        self.assertEqual(0, p.returncode, p.stderr)
        with open(self.stub_log) as f:
            return f.read().splitlines()

    def test_one_that_is_up_is_removed_by_name_and_its_log_kept(self):
        calls = self.remove(topology.containers())
        for c in self.late:
            self.assertIn("rm -f -v " + c, calls)
            with open(os.path.join(self.tmp, "node-logs-%s-left-up.txt" % c)) as f:
                self.assertEqual("%s | a line of %s's log\n" % (c, c), f.read())
        removed = [x.split()[-1] for x in calls if x.startswith("rm ")]
        self.assertEqual(self.late, removed, "something other than the late follower was removed")

    def test_none_up_removes_nothing(self):
        calls = self.remove(topology.validator_containers())
        self.assertEqual([], [x for x in calls if x.startswith("rm ")])

    def test_it_runs_before_up_and_at_teardown(self):
        with open(SOAK) as f:
            src = f.read()
        sites = [m.start() for m in re.finditer(r"^rm_late_followers \"", src, re.M)]
        up = src.index("\n# Build BEFORE up.")
        down = src.rindex(" down -v")
        self.assertTrue([s for s in sites if s < up],
                        "a follower left by a killed run would hold the config volume `up` recreates")
        self.assertTrue([s for s in sites if up < s < down],
                        "teardown's `down` does not reach a profiled service; it must be removed first")


class ChaosWaitIsWallClock(unittest.TestCase):
    """The wait between disturbances ends at its deadline however long the
    follower polls take. It used to count only its own sleeps: run
    20260925T042703Z's removal due 06:30 came at 07:12, and a pair was
    removed 9443s after its add on a 3600s cadence."""

    def test_slow_polls_do_not_stretch_the_wait(self):
        with open(SOAK) as f:
            src = f.read()
        i = src.index("  chaos_wait() {")
        j = src.index("\n  }\n", i) + 4
        script = src[i:j] + r"""
calls=0
sleep() { SECONDS=$(( SECONDS + $1 )); }
follower_watch() { calls=$((calls + 1)); SECONDS=$(( SECONDS + 100 )); }  # a 100 s poll
SECONDS=0
chaos_wait 300
echo "$calls $SECONDS"
"""
        out = subprocess.run(["bash", "-c", script], capture_output=True,
                             text=True, timeout=20).stdout.split()
        calls, elapsed = int(out[0]), int(out[1])
        # 300 s of wall clock: three rounds of 10 s sleep + 100 s poll (330 s),
        # not thirty (3300 s), which is what counting only the sleeps gives.
        self.assertEqual(3, calls, out)
        self.assertLess(elapsed, 300 + 110, out)


if __name__ == "__main__":
    unittest.main()
