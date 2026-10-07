#!/usr/bin/env python3
"""stallkill waits on its OWN run's soak.sh, and touches nothing once that
run has torn itself down.

Run 20261006T233857Z was stopped by hand. Its soak.sh stopped the monitor and
went on writing its reports; its stallkill, two minutes blind, decided to stop
the run and then waited for "a soak.sh" to exit — `pgrep -f 'bash ./soak.sh'`,
which matched the NEXT run's. After its ten minutes it removed "the late
followers": run 20261007T050444Z's first follower, 11 s after it went ACTIVE
(05:14:53Z), and that run's walk logged it as having left ACTIVE.

The real stallkill.sh is run here with `curl` failing (the monitor is dark),
`pgrep` always finding a soak.sh (the next run's), `sleep` instant and
`docker` recorded. SOAK_PID is a process that ends when the capture runs
(soak.sh finishing on its own) — or, in the second case, never does.
"""
import os
import shutil
import stat
import subprocess
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))


def _exe(path, body):
    with open(path, "w") as f:
        f.write("#!/usr/bin/env bash\n" + body)
    os.chmod(path, os.stat(path).st_mode | stat.S_IXUSR)


class StallkillWaitsOnItsOwnRun(unittest.TestCase):
    def run_stallkill(self, soak_finishes):
        d = tempfile.mkdtemp(prefix="stallkill-own-")
        self.addCleanup(shutil.rmtree, d, True)
        soak, run_dir, stub = (os.path.join(d, p) for p in ("soak", "run", "bin"))
        for p in (soak, run_dir, stub):
            os.makedirs(p)
        shutil.copy(os.path.join(HERE, "stallkill.sh"), soak)
        # Detached, so init reaps it: a child of this process would linger as
        # a zombie that `kill -0` still finds.
        pid = int(subprocess.run(["sh", "-c", "/bin/sleep 60 >/dev/null 2>&1 & echo $!"],
                                 capture_output=True, text=True).stdout)
        self.addCleanup(subprocess.run, ["kill", str(pid)], stderr=subprocess.DEVNULL)
        # The capture is where soak.sh, writing its reports, finishes.
        _exe(os.path.join(soak, "wedgewatch.sh"),
             ("kill %d; while kill -0 %d 2>/dev/null; do /bin/sleep 0.05; done\n" % (pid, pid))
             if soak_finishes else "exit 0\n")
        with open(os.path.join(soak, "followerchaos.py"), "w") as f:
            f.write("print('bvn1-fol1 acc-bvn1-fol1')\n")
        calls = os.path.join(d, "docker-calls")
        _exe(os.path.join(stub, "docker"), 'echo "$*" >> %s\n' % calls)
        _exe(os.path.join(stub, "curl"), "exit 7\n")
        _exe(os.path.join(stub, "pgrep"), "echo 4242\n")   # the next run's soak.sh
        _exe(os.path.join(stub, "sleep"), "exit 0\n")
        with open(os.path.join(run_dir, "manifest.md"), "w") as f:
            f.write("# Soak run\n")
        env = dict(os.environ, PATH=stub + ":" + os.environ["PATH"], RUN_DIR=run_dir,
                   SOAK_PID=str(pid), ATTACH="1", COMPOSE_PROJECT_NAME="stallkill-test-none")
        out = subprocess.run(["bash", os.path.join(soak, "stallkill.sh")],
                             env=env, capture_output=True, text=True, timeout=60)
        rm = []
        if os.path.exists(calls):
            rm = [l for l in open(calls).read().splitlines() if l.startswith("rm")]
        return out.stdout + out.stderr, rm

    def test_a_run_that_tore_itself_down_is_left_alone(self):
        log, rm = self.run_stallkill(soak_finishes=True)
        self.assertIn("STOPPING: monitor unreachable", log)
        self.assertEqual(rm, [], "removed a container another run owns:\n" + log)
        self.assertIn("finished its own teardown; touching nothing", log)

    def test_a_soak_sh_that_never_finishes_has_its_followers_removed(self):
        log, rm = self.run_stallkill(soak_finishes=False)
        self.assertEqual(rm, ["rm -f -v acc-bvn1-fol1"], log)


if __name__ == "__main__":
    unittest.main()
