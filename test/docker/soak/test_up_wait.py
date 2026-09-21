#!/usr/bin/env python3
"""up.sh waits for the containers `compose up -d` starts, and no others (#4364).

docker-network.yml declares every node the network can have, and since #4364
one of them — acc-bvn3-fol2, the added follower — sits behind the compose
profile `late-follower`, which a plain `docker compose up -d` does not
activate. up.sh took its healthy-container target from
`topology.node_count()`, which counts every declared node, so the target went
from 14 to 15 while 14 stayed the most that can ever be healthy: the
until-loop never ends, and the monitor behind it never starts. A network
running with no monitor is the thing up.sh exists to prevent.

This runs up.sh AS WRITTEN, from a copy of the directory (it finds its files
by its own path), with `docker`, `sleep`, `nohup` and `curl` replaced by stubs
on PATH. The docker stub reports healthy exactly the containers a plain up
starts: the twelve validators, the follower of #4365 and the bootstrap. The
sleep stub ends the script after a few turns of the wait, so the bug fails
this test rather than hanging it.

NOT verified here: a real `docker compose up -d` against this compose file.
That the late-follower profile keeps acc-bvn3-fol2 out of it rests on
compose's documented behaviour, and only a run shows it.
"""
import os
import shutil
import subprocess
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
DOCKER = os.path.dirname(HERE)

# What a plain `compose up -d` leaves healthy: init exits, and the
# late-follower profile is not active.
STARTED = (["acc-bootstrap"]
           + ["acc-bvn%d-val%d" % (b, v) for b in (1, 2, 3) for v in (1, 2, 3, 4)]
           + ["acc-bvn3-fol1"])

DOCKER_STUB = r"""#!/usr/bin/env bash
printf '%s\n' "$*" >> "$STUB_LOG"
if [ "$1" = ps ]; then cat "$STUB_PS"; fi
exit 0
"""

# The wait is `until …; do sleep 5; done`. A wait that cannot be satisfied
# sleeps forever; three sleeps is already two more than a satisfied one takes.
SLEEP_STUB = r"""#!/usr/bin/env bash
echo sleep >> "$STUB_SLEEPS"
if [ "$(wc -l < "$STUB_SLEEPS")" -ge 3 ]; then kill -TERM "$PPID"; fi
exit 0
"""

# The monitor is not started: nohup runs nothing, and curl says it answered.
NOOP_STUB = "#!/usr/bin/env bash\nexit 0\n"


class UpWait(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp(prefix="up-wait-")
        self.addCleanup(shutil.rmtree, self.tmp, True)
        self.dir = os.path.join(self.tmp, "docker")
        os.mkdir(self.dir)
        for name in ("up.sh", "topology.py", "docker-network.yml", "docker-compose.yml"):
            shutil.copy(os.path.join(DOCKER, name), self.dir)
        bindir = os.path.join(self.tmp, "bin")
        os.mkdir(bindir)
        for name, body in (("docker", DOCKER_STUB), ("sleep", SLEEP_STUB),
                           ("nohup", NOOP_STUB), ("curl", NOOP_STUB)):
            path = os.path.join(bindir, name)
            with open(path, "w") as f:
                f.write(body)
            os.chmod(path, 0o755)
        self.stub_log = os.path.join(self.tmp, "docker.log")
        self.sleeps = os.path.join(self.tmp, "sleeps.log")
        for path in (self.stub_log, self.sleeps):
            open(path, "w").close()
        ps = os.path.join(self.tmp, "ps.txt")
        with open(ps, "w") as f:
            f.write("\n".join(STARTED) + "\n")
        self.env = dict(os.environ, PATH=bindir + os.pathsep + os.environ["PATH"],
                        STUB_LOG=self.stub_log, STUB_PS=ps, STUB_SLEEPS=self.sleeps)

    def test_the_wait_ends_when_every_started_container_is_healthy(self):
        p = subprocess.run(["bash", os.path.join(self.dir, "up.sh")], env=self.env,
                           capture_output=True, text=True, timeout=60)
        with open(self.stub_log) as f:
            calls = f.read().splitlines()
        ups = [c for c in calls if " up " in " %s " % c]
        self.assertEqual(1, len(ups), calls)
        self.assertNotIn("--profile", ups[0],
                         "up.sh now starts a profile, so STARTED above is no "
                         "longer what it starts: %s" % ups[0])
        self.assertEqual(0, p.returncode,
                         "up.sh never stopped waiting with all %d started "
                         "containers healthy:\n%s%s" % (len(STARTED), p.stdout, p.stderr))
        self.assertIn("monitor:", p.stdout,
                      "the wait ended but up.sh never reached the monitor")


if __name__ == "__main__":
    unittest.main()
