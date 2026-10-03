#!/usr/bin/env python3
"""The restart and quorum-loss test, as one procedure (Paul, 2026-10-03:
"Run the dockertest with graceful restart and the rest").

  launch; settle 5 min under load; then, each followed by a 5-minute dwell
  in which every partition must produce and every node be ACTIVE:
    A  graceful whole-network restart      (restart-network.sh graceful)
    B  kill whole-network restart          (restart-network.sh kill; chaos case)
    C  one validator's store torn on purpose (bvn2-val4; tear-store) -> it must
       find the tear, join instead of resuming, and BVN2 keep producing
    D  two of BVN2's four validators lost  -> BVN2 must stand still and say it
       lost its quorum while BVN1, BVN3 and the Directory produce; then they
       are restored with their stores and keys and BVN2 must resume
  stop; write restart-test.md.

Every step and its criteria go to the run's chaos.log. It never clears a
validator's consensus/ directory and never rebuilds images mid-run.

  ./restart-test.py "<note>"         ./restart-test.py --attach <run-dir>
"""
import argparse, importlib.util, json, os, re, subprocess, sys, time

HERE = os.path.dirname(os.path.abspath(__file__))
spec = importlib.util.spec_from_file_location("bt", os.path.join(HERE, "bootstrap-test.py"))
bt = importlib.util.module_from_spec(spec); spec.loader.exec_module(bt)
record, dash, logs, sh, preflight, launch, wait_network, stop = bt.record, bt.dash, bt.logs, bt.sh, bt.preflight, bt.launch, bt.wait_network, bt.stop
SCRATCH = os.environ.get("TEAR_SCRATCH", "/tmp/restart-test-scratch")
TEAR = os.path.join(SCRATCH, "tear-store")
PARTS = ("Directory", "BVN1", "BVN2", "BVN3")
results = []

def heights():
    d = dash() or {}
    return dict(d.get("executedHeights") or d.get("heights") or {})

def not_active():
    d = dash() or {}
    return sorted(f"{r['node']}/{r['partition']}" for r in (d.get("nodeState") or {}).get("rows", [])
                  if r.get("state") != "ACTIVE")

def wait_all_active(run, label, minutes=10):
    deadline = time.time() + minutes * 60
    while time.time() < deadline:
        na = not_active()
        if not na and dash():
            record(run, f"{label}: every node ACTIVE")
            return True
        time.sleep(15)
    record(run, f"FAIL {label}: not ACTIVE after {minutes} min: {', '.join(not_active())}")
    return False

def dwell(run, label, minutes=5, must_move=PARTS, must_stand=()):
    h0 = heights(); time.sleep(minutes * 60); h1 = heights()
    moved = {p: (h1.get(p, 0) or 0) - (h0.get(p, 0) or 0) for p in PARTS}
    ok = all(moved[p] > 0 for p in must_move) and all(moved[p] == 0 for p in must_stand)
    record(run, f"{label}: dwell {minutes} min, blocks produced {moved}; must move {list(must_move)}, must stand {list(must_stand)} -> {'ok' if ok else 'FAIL'}")
    return ok

def step(run, name, fn):
    record(run, f"=== step {name} begins")
    try:
        ok = fn(run)
    except Exception as e:
        record(run, f"FAIL step {name}: {e}"); ok = False
    results.append((name, ok))
    record(run, f"=== step {name} {'PASSED' if ok else 'FAILED'}")
    return ok

def restart(run, mode):
    sh(f"cd {json.dumps(HERE)} && ./restart-network.sh {json.dumps(run)} {mode}")
    ok = wait_all_active(run, f"{mode} restart")
    return dwell(run, f"after the {mode} restart") and ok

def tear(run):
    c = "acc-bvn2-val4"; store = "/root/.accumulate/bvn2-4/bvnn/data/accumulate.db"
    os.makedirs(SCRATCH, exist_ok=True)
    local = os.path.join(SCRATCH, "bvn2-val4-store")
    sh(f"rm -rf {json.dumps(local)}")
    record(run, f"tear: docker stop {c}; copy its BVN2 store out, tear-store it, copy it back, start it")
    bt.docker(f"docker stop -t 30 {c}")
    bt.docker(f"docker cp {c}:{store} {local}")
    out = sh(f"{TEAR} {local} BVN2").strip().splitlines()[-1]
    record(run, f"tear: {out}")
    bt.docker(f"docker cp {local}/. {c}:{store}")
    sh(f"rm -rf {json.dumps(local)}")
    bt.docker(f"docker start {c}")
    deadline = time.time() + 300
    while time.time() < deadline and "store is torn" not in logs(c):
        time.sleep(5)
    found = "store is torn" in logs(c)
    record(run, f"tear: {c} {'found the tear and refused to resume' if found else 'did NOT report a torn store'}")
    ok = wait_all_active(run, "after the tear")
    return dwell(run, "after the tear") and ok and found

def quorum_loss(run):
    lost = ["acc-bvn2-val3", "acc-bvn2-val4"]
    record(run, f"quorum loss: docker stop {' '.join(lost)} (two of BVN2's four)")
    bt.docker("docker stop -t 30 " + " ".join(lost))
    time.sleep(60)
    halted = dwell(run, "BVN2 below its quorum", must_move=("Directory", "BVN1", "BVN3"), must_stand=("BVN2",))
    said = "lost its quorum" in logs("acc-bvn2-val1")
    record(run, f"quorum loss: bvn2-val1 {'said' if said else 'did NOT say'} the partition lost its quorum")
    record(run, f"quorum loss: restoring {' '.join(lost)} with their stores and keys (docker start)")
    bt.docker("docker start " + " ".join(lost))
    ok = wait_all_active(run, "after the restore")
    return dwell(run, "after the restore") and ok and halted and said

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("note", nargs="?", default="restart test (restart-test.py)")
    ap.add_argument("--attach")
    ap.add_argument("--settle-min", type=int, default=5)
    a = ap.parse_args()
    if not os.path.exists(TEAR):
        sys.exit(f"build the tear tool first: go build -o {TEAR} ./tools/cmd/tear-store")
    head, dirty = preflight(a.attach)
    if dirty != "0":
        sys.exit(f"the worktree has {dirty} modified tracked files; commit first")
    run = a.attach or launch(a.note)
    record(run, f"restart-test.py start: HEAD {head}")
    if not a.attach:
        wait_network(run, head)
    record(run, f"settling {a.settle_min} min under load")
    time.sleep(a.settle_min * 60)
    for name, fn in (("A graceful whole-network restart", lambda r: restart(r, "graceful")),
                     ("B kill whole-network restart", lambda r: restart(r, "kill")),
                     ("C one validator's store torn", tear),
                     ("D two of BVN2's four lost, then restored", quorum_loss)):
        step(run, name, fn)
    path = os.path.join(run, "restart-test.md")
    with open(path, "w") as f:
        f.write(f"# Restart and quorum-loss test — run {os.path.basename(run)}\n\nHEAD `{head}`. Procedure: restart-test.py; the record of every step is chaos.log.\n\n| step | result |\n|---|---|\n")
        for n, ok in results:
            f.write(f"| {n} | {'PASSED' if ok else 'FAILED'} |\n")
    passed = all(ok for _, ok in results)
    record(run, f"restart-test.py {'PASSED' if passed else 'FAILED'}; table {path}")
    stop(run)
    # The next step, so a passing run does not end in a question (Paul,
    # 2026-10-03): the write-up, then the open items in PLAN.md.
    record(run, "NEXT: write SUMMARY.md from restart-test.md and chaos.log, commit by name, delete bulk logs by literal path; then PLAN.md's open items (#4511, #4504, #4514)")
    sys.exit(0 if passed else 1)

if __name__ == "__main__":
    main()
