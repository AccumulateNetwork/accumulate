#!/usr/bin/env python3
"""stallkill dates its stop at the decision, with the capture end beside it
(#4425).

Run 20260924T093936Z's stallkill.log reads

    2026-09-24T10:06:16Z STOPPING: stalled 248s: BVN3
      2026-09-24T10:06:16Z CAPTURE[probe]: manual capture (--now) …
      2026-09-24T10:08:24Z CAPTURE[probe]: complete — network left UP on purpose
    2026-09-24T10:08:24Z signalling loadgen: …

and its manifest said `stopped (UTC): 2026-09-24T10:08:24Z` — the end of the
evidence capture, taken after it, two minutes after the decision a timeline
has to be read against (the pause at 10:06:30 came 14 s AFTER the decision,
and 2 minutes BEFORE the date the manifest gave it).

The real stallkill.sh is run here, copied beside a stand-in wedgewatch, with
`date`, `curl`, `pgrep`, `docker` and `sleep` stubbed on PATH: the clock
reads the run's decision second until the capture moves it to the run's
capture end. Nothing here reaches Docker.
"""
import json
import os
import shutil
import stat
import subprocess
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
DECISION = "2026-09-24T10:06:16Z"
CAPTURE_END = "2026-09-24T10:08:24Z"


def _exe(path, body):
    with open(path, "w") as f:
        f.write("#!/usr/bin/env bash\n" + body)
    os.chmod(path, os.stat(path).st_mode | stat.S_IXUSR)


class TheStopIsDatedAtTheDecision(unittest.TestCase):
    def setUp(self):
        self.d = tempfile.mkdtemp(prefix="stallkill-stop-")
        soak = os.path.join(self.d, "soak")
        self.run_dir = os.path.join(self.d, "run")
        stub = os.path.join(self.d, "bin")
        for p in (soak, self.run_dir, stub):
            os.makedirs(p)
        shutil.copy(os.path.join(HERE, "stallkill.sh"), soak)
        self.clock = os.path.join(self.d, "clock")
        with open(self.clock, "w") as f:
            f.write(DECISION)
        # The capture takes the run's two minutes: the clock moves on.
        _exe(os.path.join(soak, "wedgewatch.sh"),
             'echo "$(cat %s) CAPTURE[probe]: manual capture (--now)"\n'
             'echo %s > %s\n'
             'echo "$(cat %s) CAPTURE[probe]: complete"\n'
             % (self.clock, CAPTURE_END, self.clock, self.clock))
        payload = json.dumps({"progress": {"BVN3": {"state": "stalled",
                                                    "stalledFor": 248}},
                              "life": {"blocks": 0, "blocksEmpty": 0}})
        _exe(os.path.join(stub, "date"), 'cat %s\n' % self.clock)
        _exe(os.path.join(stub, "curl"), "echo '%s'\n" % payload)
        _exe(os.path.join(stub, "pgrep"), "exit 1\n")
        _exe(os.path.join(stub, "docker"), "exit 0\n")
        _exe(os.path.join(stub, "sleep"), "exit 0\n")
        with open(os.path.join(self.run_dir, "manifest.md"), "w") as f:
            f.write("# Soak run\n")
        self.keeper = subprocess.Popen(["/bin/sleep", "60"])
        env = dict(os.environ, PATH=stub + ":" + os.environ["PATH"],
                   RUN_DIR=self.run_dir, SOAK_PID=str(self.keeper.pid),
                   STALL_KILL_SECS="240", COMPOSE_PROJECT_NAME="stallkill-test-none")
        out = subprocess.run(["bash", os.path.join(soak, "stallkill.sh")],
                             env=env, capture_output=True, text=True, timeout=60)
        self.log = out.stdout + out.stderr
        with open(os.path.join(self.run_dir, "manifest.md")) as f:
            self.manifest = f.read()

    def tearDown(self):
        self.keeper.kill()
        self.keeper.wait()
        shutil.rmtree(self.d, ignore_errors=True)

    def test_the_log_line_and_the_manifest_carry_the_same_second(self):
        self.assertIn("%s STOPPING: stalled 248s: BVN3" % DECISION, self.log)
        self.assertIn("- stopped (UTC, the decision): %s" % DECISION,
                      self.manifest)

    def test_the_capture_end_is_beside_it(self):
        self.assertIn("- evidence capture ended, load generator signalled "
                      "(UTC): %s (128 s after the decision)" % CAPTURE_END,
                      self.manifest)

    def test_the_capture_end_is_not_given_as_the_stop(self):
        self.assertNotIn("- stopped (UTC): %s" % CAPTURE_END, self.manifest)

    def test_the_heading_soak_sh_looks_for_is_kept(self):
        self.assertIn("## Stopped early by stallkill", self.manifest)
        self.assertIn("- reason: stalled 248s: BVN3 (threshold 240s)",
                      self.manifest)


if __name__ == "__main__":
    unittest.main()
