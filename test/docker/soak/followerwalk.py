"""The follower walk: one late follower at a time, each given its time (Paul, 2026-10-06).

    followerwalk.py RUN_DIR END_EPOCH [JOIN_SECS HOLD_SECS WINDOW_SECS]

The three lengths come from the run's conf (FOLLOWER_JOIN_SECS,
FOLLOWER_HOLD_SECS, FOLLOWER_WINDOW_SECS) through soak.sh; omitted, 1800,
900 and 30.

For each late follower in turn — one per BVN, bvn1, bvn2, bvn3, bvn1, ... —
until END_EPOCH:

  1. add it: clear its databases and consensus checkpoint, set
     join-running-network, create and start the container;
  2. give it up to JOIN_SECS (30 minutes) to be ACTIVE on every partition;
  3. once ACTIVE, hold it HOLD_SECS (15 minutes), reading how far behind the
     network it is every POLL_SECS and whether it stays ACTIVE. The hold
     PASSES only if the follower never left ACTIVE and ends it within
     CAUGHT_UP_BLOCKS of the network on every partition; otherwise it FAILS
     and the line says why;
  4. remove it, bracketed by readings WINDOW_SECS before and after, and start
     the next.

A follower that is not ACTIVE by JOIN_SECS is removed as NEVER ACTIVE and the
walk moves on: the run records the failure, it does not wait on it. A slot
that would not fit before END_EPOCH is not started.

Why one at a time and why fixed phases: three joins at once contend for this
one box's disk, which no real network does, and a removal slot on a fixed
cadence cut joins off before they could finish (runs 20261006T223712Z and
20261006T233857Z). The phases are the test; the schedule follows them.

Everything it does goes into the run's chaos.log in the lines soak.sh's
verdict rows read (`add-follower`, `follower X ACTIVE (...)`, `follower X
first in agreement (...)`, `follower X NEVER ACTIVE`, `remove-follower`,
`follower removal N, ...`), plus one `follower X held ACTIVE ...` line per
hold with what it measured. Each follower's log is kept as
node-logs-<container>-<N>.txt.
"""

import json
import os
import subprocess
import sys
import time
import urllib.request

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import followerchaos  # noqa: E402

JOIN_SECS, HOLD_SECS, WINDOW_SECS = 1800, 900, 30
POLL_SECS = 10
# A follower is running when it ends its hold this close to the network on
# every partition: ten blocks, ten seconds at the block interval. Run
# 20261007T050444Z's bvn2-fol1 held ACTIVE for 900 s at 229, then 976 blocks
# behind, stopped dead, and the hold line read like a pass.
CAUGHT_UP_BLOCKS = 10
MONITOR = "http://127.0.0.1:8099/data"
HERE = os.path.dirname(os.path.abspath(__file__))
COMPOSE = ["docker", "compose", "-f", os.path.join(HERE, "..", "docker-compose.yml")]


def utc(t=None):
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(t))


class Walk:
    def __init__(self, run_dir, end, run=subprocess.run, now=time.time, sleep=time.sleep,
                 state=None, agreement=None, behind=None, snapshot=None, unaffected=None):
        self.rd, self.end = run_dir, end
        self.chaos = os.path.join(run_dir, "chaos.log")
        self._run, self.now, self.sleep = run, now, sleep
        self.state = state or self._state
        self.agreement = agreement or self._agreement
        self.behind = behind or self._behind
        self.snapshot = snapshot or self._snapshot
        self.unaffected = unaffected or self._unaffected
        self.removals = 0

    # --- the record -------------------------------------------------------
    def log(self, line):
        with open(self.chaos, "a") as f:
            f.write("%s %s\n" % (utc(self.now()), line))

    # --- readings (replaced in tests) ----------------------------------------
    def _state(self, c):
        st = followerchaos.node_states(followerchaos.scrape(c))
        return followerchaos.all_active(st), followerchaos.describe_states(st)

    def _agreement(self, c, since):
        r = subprocess.run([sys.executable, os.path.join(HERE, "followerchaos.py"), "agreement", c, since],
                           stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True, timeout=120)
        return r.returncode == 0, r.stdout.strip()

    def _behind(self, c):
        """{partition: blocks behind} from the monitor, the numbers the dashboard shows."""
        try:
            with urllib.request.urlopen(MONITOR, timeout=5) as r:
                d = json.load(r)
            parts = d["follower"]["nodes"][c]["partitions"]
            return {p: v.get("behind") for p, v in parts.items() if v.get("measured")}
        except Exception:
            return {}

    def _snapshot(self, path):
        with open(path, "w") as f:
            json.dump(followerchaos.snapshot(), f)

    def _unaffected(self, a, b, c):
        r = subprocess.run([sys.executable, os.path.join(HERE, "followerchaos.py"), "unaffected", a, b, c],
                           stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, timeout=60)
        return r.stdout.strip()

    def sh(self, args, timeout=120):
        try:
            return self._run(args, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, timeout=timeout)
        except Exception as e:  # a hung docker call must not end the walk
            self.log("followerwalk: %s failed: %s" % (" ".join(args[:3]), e))
            return None

    # --- the phases ---------------------------------------------------------
    def add(self, f, n, of):
        d, svc, c = f["dir"], f["service"], f["container"]
        clear = " ".join("/root/.accumulate/%s/%s" % (d, p) for p in
                         ("dnn/data/accumulate.db", "bvnn/data/accumulate.db", "consensus"))
        toml = "/root/.accumulate/%s/accumulate.toml" % d
        self.sh(COMPOSE + ["run", "--rm", "--no-deps", "--entrypoint", "sh", svc, "-c", "rm -rf " + clear])
        self.sh(COMPOSE + ["run", "--rm", "--no-deps", "--entrypoint", "sh", svc, "-c",
                           "grep -q join-running-network %s || sed -i '/type = \"coreValidator\"/a\\  join-running-network = true' %s" % (toml, toml)])
        self.sh(COMPOSE + ["--profile", "late-follower", "create", "--no-recreate", svc])
        self.log("add-follower %s (%s, data %s; key in no committee; databases cleared; join-running-network; "
                 "one late follower at a time, %d of %d; up to %ds to ACTIVE, then held %ds)"
                 % (c, svc, d, n, of, JOIN_SECS, HOLD_SECS))
        self.sh(["docker", "start", c])

    def wait_active(self, c, added_at, added_utc):
        """Seconds to ACTIVE, or None by JOIN_SECS. Logs ACTIVE and first agreement."""
        matched = False
        while self.now() - added_at < JOIN_SECS:
            self.sleep(POLL_SECS)
            if not matched:
                ok, out = self.agreement(c, added_utc)
                if ok:
                    matched = True
                    self.log("follower %s first in agreement (%s), %ds after it was added" % (c, out, self.now() - added_at))
            ok, out = self.state(c)
            if ok:
                took = int(self.now() - added_at)
                self.log("follower %s ACTIVE (%s), %ds after it was added" % (c, out, took))
                return took
        return None

    def hold(self, c):
        """Hold an ACTIVE follower HOLD_SECS: worst and last blocks behind per
        partition, and every poll it was not ACTIVE."""
        start = self.now()
        worst, last, polls, dropped = {}, {}, 0, 0
        while self.now() - start < HOLD_SECS:
            self.sleep(POLL_SECS)
            polls += 1
            ok, out = self.state(c)
            if not ok:
                dropped += 1
                if dropped == 1:
                    self.log("follower %s LEFT ACTIVE during its hold (%s), %ds into it" % (c, out, self.now() - start))
            for p, b in self.behind(c).items():
                if b is None:
                    continue
                worst[p] = max(worst.get(p, 0), b)
                last[p] = b
        parts = ", ".join("%s worst %d last %d" % (p, worst[p], last.get(p, -1)) for p in sorted(worst)) or "behind not measured"
        why = []
        if dropped:
            why.append("left ACTIVE")
        if not last:
            why.append("blocks behind never measured")
        why += ["%s ended %d blocks behind" % (p, b) for p, b in sorted(last.items()) if b > CAUGHT_UP_BLOCKS]
        verdict = "FAIL (%s)" % "; ".join(why) if why else "PASS (caught up within %d blocks)" % CAUGHT_UP_BLOCKS
        self.log("follower %s held ACTIVE %ds: %s blocks behind; %d of %d polls not ACTIVE; %s"
                 % (c, int(self.now() - start), parts, dropped, polls, verdict))
        return not why

    def remove(self, c, never_active, added_at):
        self.removals += 1
        n = self.removals
        pre = os.path.join(self.rd, "follower-removal-%d" % n)
        self.snapshot(pre + "-before.json")
        self.sleep(WINDOW_SECS)
        self.snapshot(pre + "-at.json")
        if never_active:
            self.log("follower %s NEVER ACTIVE: removed %ds after it was added" % (c, self.now() - added_at))
        self.log("remove-follower %s (removal %d)" % (c, n))
        self.sh(["docker", "stop", c])
        r = self.sh(["docker", "logs", c], timeout=300)
        if r is not None:
            with open(os.path.join(self.rd, "node-logs-%s-%d.txt" % (c, n)), "w") as f:
                for line in (r.stdout or "").splitlines():
                    f.write("%s | %s\n" % (c, line))
        self.sh(["docker", "rm", "-v", c])
        self.sleep(WINDOW_SECS)
        self.snapshot(pre + "-after.json")
        self.log("follower removal %d, %ds either side: %s"
                 % (n, WINDOW_SECS, self.unaffected(pre + "-before.json", pre + "-at.json", pre + "-after.json")))

    def slot_fits(self):
        return self.now() + JOIN_SECS + HOLD_SECS + 2 * WINDOW_SECS + 120 <= self.end

    def run(self, fols):
        if not fols:
            self.log("followerwalk: no late follower in the compose's late-follower profile — nothing to walk")
            return
        self.log("followerwalk: %s in turn, one at a time; each up to %ds to ACTIVE, then held %ds, then removed"
                 % (", ".join(f["container"] for f in fols), JOIN_SECS, HOLD_SECS))
        k, passed, failed, never = 0, 0, 0, 0
        while self.slot_fits():
            f = fols[k % len(fols)]
            c = f["container"]
            added_at = self.now()
            self.add(f, k % len(fols) + 1, len(fols))
            took = self.wait_active(c, added_at, utc(added_at))
            if took is None:
                never += 1
            elif self.hold(c):
                passed += 1
            else:
                failed += 1
            self.remove(c, took is None, added_at)
            k += 1
        self.log("followerwalk: done, %d follower(s) walked: %d PASSED, %d FAILED their hold, %d NEVER ACTIVE; "
                 "the next slot would not end before the run does" % (k, passed, failed, never))


def main(argv):
    global JOIN_SECS, HOLD_SECS, WINDOW_SECS
    if len(argv) not in (2, 5):
        sys.stderr.write(__doc__)
        return 2
    if len(argv) == 5:
        JOIN_SECS, HOLD_SECS, WINDOW_SECS = (int(a) for a in argv[2:5])
    Walk(argv[0], int(argv[1])).run(followerchaos.late_followers())
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
