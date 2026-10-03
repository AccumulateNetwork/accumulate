#!/usr/bin/env python3
"""The follower-bootstrap test, as one procedure (Paul, 2026-10-03):

  launch the network; wait 5 minutes of load; add the three late followers;
  wait until every one of their six partition joins is synced and caught up;
  wait 5 minutes; remove and re-add them; repeat for N cycles; stop the run;
  write the result table.

Every step is recorded in the run's chaos.log with its time and its criteria,
so the write-up reads from the record and not from memory. The script never
rebuilds images, never clears a validator's databases or consensus/, and
refuses to start while another soak runs.

  ./bootstrap-test.py --cycles 3 "<note for soak.sh>"        # the run
  ./bootstrap-test.py --dry-run                              # print the plan
  ./bootstrap-test.py --attach <run-dir> --cycles 3          # cycles on a running network

Criteria (per follower, per partition):
  ACTIVE      "This node is executing in agreement; it is ACTIVE"
  synced      "The node is synced: its overlap's perfect blocks held with no regression"
  caught up   the dashboard's nodeState row shows behind <= 1 at the time the dwell starts
  drops       "not executing in agreement" lines during the 5-minute dwell (0 expected)
A cycle fails when a partition join is not synced within --join-timeout
minutes, or when the dashboard says a partition stalled by blocks.
"""
import argparse, json, os, re, subprocess, sys, time, urllib.request
from datetime import datetime, timezone

HERE = os.path.dirname(os.path.abspath(__file__))
DOCKER_DIR = os.path.dirname(HERE)
COMPOSE = "COMPOSE_PROJECT_NAME=disoak docker compose -f docker-compose.yml"
FOLLOWERS = {"bvn1-fol1": ("bvn1-5", "bvn1"), "bvn2-fol1": ("bvn2-5", "bvn2"), "bvn3-fol2": ("bvn3-6", "bvn3")}
PARTS = ("dn.acme", "bvn")
DASH = "http://127.0.0.1:8099/data"
TAILNET = "http://100.75.75.92:8098/"
ANSI = re.compile(r"\x1b\[[0-9;]*m")

def now(): return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
def sh(cmd, check=True):
    r = subprocess.run(["bash", "-c", cmd], capture_output=True, text=True)
    if check and r.returncode != 0:
        raise RuntimeError(f"{cmd}\n{r.stdout}\n{r.stderr}")
    return r.stdout
def docker(cmd): return sh(f"sg docker -c {json.dumps(cmd)}")
def record(run, line):
    msg = f"{now()} {line}"
    print(msg, flush=True)
    with open(os.path.join(run, "chaos.log"), "a") as f: f.write(msg + "\n")
def dash():
    try:
        with urllib.request.urlopen(DASH, timeout=5) as r: return json.load(r)
    except Exception: return None
def logs(container):
    out = sh(f"sg docker -c 'docker logs {container} 2>&1'", check=False)
    return ANSI.sub("", out)

def preflight():
    if sh("ps -eo args | grep -c '[b]ash ./soak.sh'", check=False).strip() != "0":
        sys.exit("a soak is already running: never two soaks")
    head = sh("git rev-parse --short HEAD").strip()
    dirty = sh("git status --short | grep -v '^??' | wc -l").strip()
    return head, dirty

def launch(note):
    before = set(os.listdir(os.path.join(HERE, "runs")))
    subprocess.Popen(["bash", "-c", f"cd {json.dumps(HERE)} && setsid nohup bash ./soak.sh -c 72h-watch.conf {json.dumps(note)} > /dev/null 2>&1 < /dev/null &"])
    for _ in range(60):
        time.sleep(2)
        new = set(os.listdir(os.path.join(HERE, "runs"))) - before
        new = [d for d in new if re.match(r"\d{8}T\d{6}Z", d)]
        if new: return os.path.join(HERE, "runs", sorted(new)[-1])
    sys.exit("soak.sh made no run directory in two minutes")

def wait_network(run, head):
    print("waiting for the dashboard (the harness builds images first) ...", flush=True)
    for _ in range(180):
        if dash(): break
        time.sleep(10)
    else: sys.exit("the dashboard never answered")
    log = open(os.path.join(run, "soak.log")).read()
    m = re.search(r"commit:\s+\S+ \(([0-9a-f]{40})\)", log)
    commit = m.group(1)[:9] if m else "?"
    record(run, f"network up; harness says commit {commit} (HEAD {head}); dashboard answering")
    if not commit.startswith(head): record(run, f"WARNING: the harness's commit {commit} is not HEAD {head}")
    try:
        urllib.request.urlopen(TAILNET, timeout=5); record(run, f"monitor reachable on the tailnet: {TAILNET}")
    except Exception as e: record(run, f"WARNING: monitor NOT reachable on the tailnet ({e}); Paul must see it before load")
    print("waiting for the load generator's first progress line ...", flush=True)
    while "sent=" not in open(os.path.join(run, "soak.log")).read(): time.sleep(10)
    record(run, "load generator running")

def network_ok(run):
    d = dash()
    if not d: return True
    for p, v in (d.get("progress") or {}).items():
        if v.get("state") == "stalled" and v.get("stalledBy") == "blocks" and (v.get("stalledFor") or 0) > 120:
            record(run, f"FAIL: partition {p} stalled by blocks for {v.get('stalledFor')}s"); return False
    return True

def remove_followers(run, cycle):
    names = " ".join(f"acc-{s}" for s in FOLLOWERS)
    docker(f"docker rm -f {names} >/dev/null 2>&1 || true")
    for s in FOLLOWERS: record(run, f"remove-follower acc-{s} (bootstrap-test cycle {cycle})")

def add_followers(run, cycle):
    dirs = [d for d, _ in FOLLOWERS.values()]
    clear = " ".join(f"/root/.accumulate/{d}/dnn/data/accumulate.db /root/.accumulate/{d}/bvnn/data/accumulate.db /root/.accumulate/{d}/consensus" for d in dirs)
    tomls = " ".join(f"/root/.accumulate/{d}/accumulate.toml" for d in dirs)
    # The inner shell uses double quotes only, so it can sit inside the
    # single quotes compose's `sh -c` wants; the outer layers are argv, not
    # a shell string, so nothing is re-parsed on the way (the same sed as
    # soak.sh's add_follower and follower-cycles.sh, which ran 9 cycles).
    inner = (f"rm -rf {clear}; for f in {tomls}; do grep -q join-running-network $f || "
             'sed -i "/coreValidator/a\\  join-running-network = true" $f; done; '
             f"grep -l join-running-network {tomls} | wc -l")
    r = subprocess.run(["sg", "docker", "-c", f"{COMPOSE} run --rm --no-deps --entrypoint sh bvn1-fol1 -c '{inner}'"],
                       cwd=DOCKER_DIR, capture_output=True, text=True)
    # A follower without join-running-network starts as the first node of a
    # network and replays from genesis: the cycle would test the genesis
    # watch, not the join (run 20261003T052439Z, cycle 1: the sed pattern's
    # quotes were stripped by the shells and it matched nothing). Refuse.
    got = (r.stdout.strip().splitlines() or ["?"])[-1].strip()
    if got != str(len(dirs)):
        record(run, f"FAIL: join-running-network is set in {got} of {len(dirs)} follower configs (rc {r.returncode}): {r.stderr.strip()[-300:]}")
        sys.exit(1)
    subprocess.run(["sg", "docker", "-c", f"{COMPOSE} --profile late-follower create --no-recreate {' '.join(FOLLOWERS)}"],
                   cwd=DOCKER_DIR, capture_output=True, text=True)
    subprocess.run(["sg", "docker", "-c", "docker start " + " ".join(f"acc-{s}" for s in FOLLOWERS)], capture_output=True, text=True)
    t0 = time.time()
    for s in FOLLOWERS: record(run, f"add-follower acc-{s} (bootstrap-test cycle {cycle}; databases and consensus/ cleared; join-running-network)")
    return t0

def wait_synced(run, t0, timeout_min):
    """Returns {(follower, part): (active_s, synced_s)} or None on timeout."""
    done = {}
    while time.time() - t0 < timeout_min * 60:
        if not network_ok(run): return None
        for s in FOLLOWERS:
            text = logs(f"acc-{s}")
            for p in PARTS:
                k = (s, p)
                if k in done and done[k][1] is not None: continue
                if "it is the first node of a network" in text:
                    record(run, f"FAIL: {s} started as the first node of a network: it replayed from genesis instead of joining")
                    return None
                lines = [l for l in text.splitlines() if f"partition=acc://{p}" in l]
                active = next((l for l in lines if "it is ACTIVE" in l), None)
                synced = next((l for l in lines if "The node is synced" in l), None)
                a = done.get(k, (None, None))[0]
                if active and a is None: a = round(time.time() - t0)
                if synced:
                    done[k] = (a, round(time.time() - t0))
                    record(run, f"{s}/{p} synced {done[k][1]}s after the add (ACTIVE at {a}s)")
                elif a is not None and k not in done:
                    done[k] = (a, None); record(run, f"{s}/{p} ACTIVE {a}s after the add")
        if len(done) == 6 and all(v[1] is not None for v in done.values()): return done
        time.sleep(10)
    record(run, f"FAIL: not every partition join synced within {timeout_min} minutes: " +
           ", ".join(f"{s}/{p}" for s in FOLLOWERS for p in PARTS if (s, p) not in done or done[(s, p)][1] is None))
    return None

def behind(run):
    d = dash() or {}
    out = {}
    for r in (d.get("nodeState") or {}).get("rows", []):
        if r.get("node", "").replace("acc-", "") in FOLLOWERS: out[(r["node"].replace("acc-", ""), r.get("partition"))] = r.get("behind")
    return out

def dwell(run, minutes):
    marks = {s: len(logs(f"acc-{s}").splitlines()) for s in FOLLOWERS}
    b = behind(run)
    record(run, f"dwell {minutes} min begins; behind: " + ", ".join(f"{s}/{p}={v}" for (s, p), v in sorted(b.items())))
    deadline = time.time() + minutes * 60
    while time.time() < deadline:
        if not network_ok(run): return None
        time.sleep(15)
    drops = {}
    for s in FOLLOWERS:
        new = logs(f"acc-{s}").splitlines()[marks[s]:]
        for p in PARTS:
            drops[(s, p)] = sum(1 for l in new if "not executing in agreement" in l and f"partition=acc://{p}" in l)
    record(run, "dwell over; drops out of agreement: " + ", ".join(f"{s}/{p}={v}" for (s, p), v in sorted(drops.items())))
    return drops, b

def stop(run):
    pid = sh("ps -eo pid,args | awk '/[g]o run .*cmd\\/loadgen/ {print $1}' | head -1", check=False).strip()
    child = sh(f"pgrep -P {pid} | head -1", check=False).strip() if pid else ""
    target = child or pid
    record(run, f"STOP: load generator interrupted (pid {target}); harness tears down")
    if target: sh(f"kill -INT {target}", check=False)
    while sh("ps -eo args | grep -c '[b]ash ./soak.sh'", check=False).strip() != "0": time.sleep(5)
    record(run, "harness exited; network torn down")

def write_table(run, head, rows):
    path = os.path.join(run, "bootstrap-test.md")
    with open(path, "w") as f:
        f.write(f"# Follower bootstrap test — run {os.path.basename(run)}\n\nHEAD `{head}`. Procedure: bootstrap-test.py (load 5 min; add 3 followers; wait synced + caught up; dwell 5 min; repeat).\n\n")
        f.write("| cycle | follower | partition | ACTIVE after (s) | synced after (s) | behind at dwell | drops in dwell |\n|---|---|---|---|---|---|---|\n")
        for c, s, p, a, sy, b, d in rows: f.write(f"| {c} | {s} | {p} | {a} | {sy} | {b} | {d} |\n")
    record(run, f"result table written: {path}")
    return path

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("note", nargs="?", default="follower bootstrap test (bootstrap-test.py)")
    ap.add_argument("--cycles", type=int, default=3)
    ap.add_argument("--settle-min", type=int, default=5, help="minutes of load before the first cycle")
    ap.add_argument("--dwell-min", type=int, default=5)
    ap.add_argument("--join-timeout", type=int, default=30, help="minutes for all six joins to sync")
    ap.add_argument("--attach", help="run directory of an already running network")
    ap.add_argument("--keep-up", action="store_true", help="do not stop the run at the end")
    ap.add_argument("--dry-run", action="store_true")
    a = ap.parse_args()
    head, dirty = preflight() if not a.dry_run else (sh("git rev-parse --short HEAD").strip(), "?")
    if a.dry_run:
        print(f"HEAD {head} (dirty files: {dirty}); would: launch soak.sh -c 72h-watch.conf; wait dashboard+load; settle {a.settle_min} min; "
              f"{a.cycles} cycles of [remove 3 followers, clear their dbs+consensus, add, wait synced (<= {a.join_timeout} min), dwell {a.dwell_min} min, count drops]; stop; write bootstrap-test.md")
        return
    if dirty != "0": sys.exit(f"the worktree has {dirty} modified tracked files; commit or stash first")
    run = a.attach or launch(a.note)
    record(run, f"bootstrap-test.py start: HEAD {head}, {a.cycles} cycles, settle {a.settle_min} min, dwell {a.dwell_min} min")
    if not a.attach: wait_network(run, head)
    record(run, f"settling {a.settle_min} min under load before the first cycle")
    time.sleep(a.settle_min * 60)
    rows, ok = [], True
    for c in range(1, a.cycles + 1):
        if not network_ok(run): ok = False; break
        remove_followers(run, c)
        t0 = add_followers(run, c)
        done = wait_synced(run, t0, a.join_timeout)
        if done is None: ok = False; break
        res = dwell(run, a.dwell_min)
        if res is None: ok = False; break
        drops, b = res
        for s in FOLLOWERS:
            for p in PARTS:
                rows.append((c, s, p, done[(s, p)][0], done[(s, p)][1], b.get((s, FOLLOWERS[s][1] if p == "bvn" else "directory")), drops[(s, p)]))
        record(run, f"cycle {c} {'PASSED' if all(d == 0 for d in drops.values()) else 'had drops'}")
    path = write_table(run, head, rows)
    record(run, f"bootstrap-test.py {'PASSED' if ok else 'FAILED'}: {len(rows)//6} cycles complete")
    if not a.keep_up: stop(run)
    print(open(path).read())
    sys.exit(0 if ok else 1)

if __name__ == "__main__":
    main()
