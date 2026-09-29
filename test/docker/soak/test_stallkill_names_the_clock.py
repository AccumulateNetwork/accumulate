# Copyright 2026 The Accumulate Authors
#
# Use of this source code is governed by an MIT-style
# license that can be found in the LICENSE file or at
# https://opensource.org/licenses/MIT.
"""stallkill says which clock stopped the run: height or delivery.

Run 20260929T005140Z (attached after a whole-network recreate on b551c38a9)
was stopped as "stalled 92s: BVN1,BVN2,BVN3" and read as a block stall, while
every BVN closed a block a second. soakmon had every BVN `stalledBy=delivery`
with `blocksStalledFor` 0-1 s: Directory -> BVNn was "1 undelivered, delivery
STALLED" (produced 1, received 1, delivered 0) and BVN2 -> BVN1 / BVN2 -> BVN3
had 73 and 66 produced and never received. The payload below is that
capture's /data (delivery-stall-20260929T005524Z/soakmon-at-wedge.json),
reduced to what stallkill reads. The real stallkill.sh runs against it with
curl, date, docker, pgrep and sleep stubbed; nothing reaches Docker.
"""
import json
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


def _cell(sent, recv, deliv, state, note):
    return {"sent": sent, "recv": recv, "deliv": deliv, "state": state, "note": note}


OK = "caught up"
PAYLOAD = {
    "progress": {
        "Directory": {"height": 18367, "state": "live", "stalledFor": 0.0,
                      "blocksStalledFor": 0.0},
        "BVN1": {"height": 14897, "state": "stalled", "stalledFor": 122.7,
                 "blocksStalledFor": 1.1, "deliveryStalledFor": 122.7,
                 "stalledBy": "delivery"},
        "BVN2": {"height": 16021, "state": "stalled", "stalledFor": 122.7,
                 "blocksStalledFor": 0.0, "deliveryStalledFor": 122.7,
                 "stalledBy": "delivery"},
        "BVN3": {"height": 16058, "state": "stalled", "stalledFor": 122.7,
                 "blocksStalledFor": 0.0, "deliveryStalledFor": 122.7,
                 "stalledBy": "delivery"},
    },
    "life": {"blocks": 1062, "blocksEmpty": 201},
    "matrix": {"flows": {"synthetic": {
        "BVN1": {"Directory": _cell(7378, 7378, 7378, "ok", OK),
                 "BVN2": _cell(4910, 4910, 4910, "ok", OK),
                 "BVN3": _cell(4597, 4597, 4597, "ok", OK)},
        "Directory": {
            "BVN1": _cell(1, 1, 0, "red", "1 undelivered, delivery STALLED"),
            "BVN2": _cell(1, 1, 0, "red", "1 undelivered, delivery STALLED"),
            "BVN3": _cell(1, 1, 0, "red", "1 undelivered, delivery STALLED")},
        "BVN2": {"Directory": _cell(15345, 15345, 15345, "ok", OK),
                 "BVN1": _cell(15206, 15133, 15133, "red", "lag ∞ (exp 15s)"),
                 "BVN3": _cell(14981, 14915, 14915, "red", "lag ∞ (exp 15s)")},
        "BVN3": {"Directory": _cell(7202, 7202, 7202, "ok", OK),
                 "BVN1": _cell(4569, 4569, 4569, "ok", OK),
                 "BVN2": _cell(4721, 4721, 4721, "ok", OK)},
    }}},
}


class StallkillNamesTheClock(unittest.TestCase):
    def setUp(self):
        self.d = tempfile.mkdtemp(prefix="stallkill-why-")
        soak = os.path.join(self.d, "soak")
        run_dir = os.path.join(self.d, "run")
        stub = os.path.join(self.d, "bin")
        for p in (soak, run_dir, stub):
            os.makedirs(p)
        shutil.copy(os.path.join(HERE, "stallkill.sh"), soak)
        _exe(os.path.join(soak, "wedgewatch.sh"), "exit 0\n")
        with open(os.path.join(self.d, "data.json"), "w") as f:
            json.dump(PAYLOAD, f)
        _exe(os.path.join(stub, "curl"), "cat %s\n" % os.path.join(self.d, "data.json"))
        _exe(os.path.join(stub, "date"), "echo 2026-09-29T00:54:54Z\n")
        _exe(os.path.join(stub, "pgrep"), "exit 1\n")
        _exe(os.path.join(stub, "docker"), "exit 0\n")
        _exe(os.path.join(stub, "sleep"), "exit 0\n")
        with open(os.path.join(run_dir, "manifest.md"), "w") as f:
            f.write("# Soak run\n")
        self.keeper = subprocess.Popen(["/bin/sleep", "60"])
        env = dict(os.environ, PATH=stub + ":" + os.environ["PATH"],
                   RUN_DIR=run_dir, SOAK_PID=str(self.keeper.pid),
                   STALL_KILL_SECS="90", COMPOSE_PROJECT_NAME="stallkill-test-none")
        out = subprocess.run(["bash", os.path.join(soak, "stallkill.sh")],
                             env=env, capture_output=True, text=True, timeout=60)
        self.log = out.stdout + out.stderr
        with open(os.path.join(run_dir, "manifest.md")) as f:
            self.manifest = f.read()

    def tearDown(self):
        self.keeper.kill()
        self.keeper.wait()
        shutil.rmtree(self.d, ignore_errors=True)

    def stopping(self):
        lines = [l for l in self.log.splitlines() if "STOPPING" in l]
        self.assertEqual(1, len(lines), self.log)
        return lines[0]

    def test_the_stop_says_delivery_not_height(self):
        line = self.stopping()
        for part in ("BVN1", "BVN2", "BVN3"):
            self.assertIn("%s on delivery for 122s" % part, line)
        self.assertNotIn("on height", line)
        self.assertIn("height moved 1s ago", line)

    def test_the_red_flows_are_named_source_to_destination(self):
        line = self.stopping()
        self.assertIn("Directory → BVN1: 1 undelivered, delivery STALLED", line)
        self.assertIn("BVN2 → BVN1: lag ∞ (exp 15s)", line)
        self.assertIn("BVN2 → BVN3: lag ∞ (exp 15s)", line)
        self.assertNotIn("BVN3 → BVN1", line, "a green flow is not a cause")

    def test_the_manifest_reason_carries_it(self):
        self.assertIn("- reason: stalled 122s: BVN1,BVN2,BVN3 (threshold 90s)"
                      " — BVN1 on delivery", self.manifest)


if __name__ == "__main__":
    unittest.main()
