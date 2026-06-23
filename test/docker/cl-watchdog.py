#!/usr/bin/env python3
"""Watchdog for the acc-cl 30 TPS soak test — Crucial T700 drive-fix validation.

Runs loadmix at a FIXED 30 TPS (no ramp) for 8h+. Watches every node. When a
node crashes or stalls:

    stop the node  ->  dbcheck its LevelDB stores
        corrupt -> the drive fix FAILED. Write a Claude root-cause report and
                   HALT the whole test. One corrupt node is proof enough.
        clean   -> restart the node with `docker start` (NEVER init/compose-up,
                   which would --reset and wipe the DB) and keep the test going,
                   to see how long the network survives.

The test ends cleanly (drive fix held) when loadmix finishes its duration with
no corruption ever observed.
"""
import json, os, re, signal, subprocess, sys, time
from datetime import datetime, timezone

# --- config ---------------------------------------------------------------
TPS            = 12
DURATION       = "24h"                      # total wall-clock soak time
LOAD_ON_SEC    = 55 * 60                     # transactions ON for 55 min ...
LOAD_OFF_SEC   = 5 * 60                      # ... then PAUSED for 5 min, each hour
SERVER         = "http://127.0.0.1:27680"   # loadmix derives BVN2/3 (27684/27688) itself
FAUCET_AS1     = "AS12ieB4D9CLbJ2ShfD7erRZVxqMudKCkRDbuBqw25fwwi7Bugn9B"  # seed "FAUCET"
ACTORS         = 40

DEBUG_BIN      = "/tmp/cl/debug"            # built from tools/cmd/debug (has loadmix)
DBCHECK_BIN    = "/tmp/dbcheck/dbcheck"     # leveldb integrity scanner
DATA_ROOT      = "/home/paul/acc-cl-data"   # node working dirs (on nvme0, the T700)
CLAUDE_BIN     = "/home/paul/.local/bin/claude"
REPO           = "/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate"
LOADMIX_LOG    = "/tmp/cl-loadmix.log"      # cl-monitor.py tails this

POLL_SEC       = 15      # how often to scan all nodes
STALL_SEC      = 120     # height frozen this long (while peers advance) = stalled
RESTART_GRACE  = 150     # after (re)start, skip stall checks this long (start + catch-up)
DN_RPC, BVN_RPC = 26657, 26757   # CometBFT RPC inside the container
# leveldb corruption can appear in the logs while the node KEEPS making blocks
# (e.g. tx_index) — neither a crash nor a height-freeze — so the soak's first run
# missed it. Scan logs for it directly. (dmesg "reset controller" lines would be
# the drive-side cause, but dmesg_restrict blocks unprivileged reads, so we rely
# on the leveldb checksum error the corruption produces.)
CORRUPTION_RE = re.compile(r"checksum mismatch|corruption on data-block|leveldb/table: corruption")

PARTITIONS = {
    "BVN1": ["bvn1-1", "bvn1-2", "bvn1-3", "bvn1-4"],
    "BVN2": ["bvn2-1", "bvn2-2", "bvn2-3", "bvn2-4"],
    "BVN3": ["bvn3-1", "bvn3-2", "bvn3-3", "bvn3-4"],
}
ALL_NODES = [n for ns in PARTITIONS.values() for n in ns]
BVN_OF    = {n: p for p, ns in PARTITIONS.items() for n in ns}

RUN_DIR = "/tmp/cl-watchdog/run-" + datetime.now(timezone.utc).strftime("%Y%m%d-%H%M%S")
os.makedirs(RUN_DIR, exist_ok=True)
EVENTS = os.path.join(RUN_DIR, "events.log")

# --- the prompt fed to `claude -p` on a corrupt DB ------------------------
PROMPT_TMPL = """You are diagnosing a node failure in an Accumulate consensus-load test. Work non-interactively and finish by writing exactly one markdown report file. Be factual and quote specific evidence; do not speculate beyond what the files show.

## Context
- Test `acc-cl` (#4043/#4044): 12 dual nodes, 3 BVNs x 4 validators. Every node runs a Directory (DN) validator AND its BVN validator. Working dirs: {data_root}/<node>/{{dnn,bvnn}}.
- Each sub-node (dnn, bvnn) has 5 LevelDB stores: accumulate.db (Accumulate state) and CometBFT blockstore.db, state.db, evidence.db, tx_index.db.
- CRITICAL: this run is on the main drive (nvme0, a Crucial T700 SSD) that PREVIOUSLY corrupted data — its controller timed out/reset and dropped acked writes, producing corrupt LevelDB SSTables (CRC/checksum mismatch). The drive was just fixed and the ENTIRE PURPOSE of this run is to confirm the fix held. The key question on any failure is: did the drive drop writes and corrupt the database again, or is this an unrelated software-level stall?
- Load: a loadmix generator at a fixed 30 TPS.

## What happened
Node {node} {reason} and the watchdog stopped it for forensics. The dbcheck scan flagged on-disk corruption ({failed_count} store(s) failed), which is why this report is being generated and the test is being halted. Evidence:

- Stall facts (JSON): {stall_json}
  (node, partition, reason, timestamps, node height vs peer-max height, container State = status/exit code/OOMKilled, restart count, load level at the time)
- Node logs (recent): {node_log}   (accumulated + CometBFT stderr)
- DB integrity scan: {dbcheck_json} and {dbcheck_log}
  dbcheck opened every LevelDB store under {data_root}/{node} read-only and iterated all key/value pairs (goleveldb verifies block CRCs on read). ok:false with open_err/iter_err containing "checksum"/"corrupt"/"CRC" means on-disk corruption.
- Load generator log: {loadmix_log}

## Your task
1. Read all evidence files. You MAY run additional READ-ONLY commands to dig deeper: `docker logs acc-cl-{node}`, grep the logs, inspect {data_root}/{node}, or re-run the checker:
   docker run --rm -u 0 -v {data_root}/{node}:/data:ro -v {dbcheck_bin}:/dbcheck:ro alpine /dbcheck /data
   Do NOT start/restart containers, write to the data dirs, or alter the test.
2. Determine the root cause and classify it as exactly one primary category:
   - leveldb-corruption  (SSTable/manifest CRC/checksum mismatch — drive integrity failure)
   - compaction-backlog  (goleveldb compaction behind writes; climbing .ldb count)
   - oom                 (container OOMKilled / memory exhaustion)
   - consensus-failure   (CometBFT CONSENSUS FAILURE, apphash mismatch, panic)
   - other               (explain)
3. Answer explicitly: DOES THIS INDICT THE CRUCIAL T700 DRIVE FIX? Is there evidence the drive dropped writes / corrupted data again? State confidence and the evidence for and against.
4. Build the evidence chain: quote the specific log lines (with timestamps) and dbcheck errors (db path + error string) that support your conclusion.
5. Give severity and concrete recommended next actions.

## Output
Write ONE markdown report to: {report_path}
Structure:
  # Node stall report — {node} @ {stall_time}
  ## Verdict        (one line: primary category; drive-fix indicted yes/no/uncertain; confidence)
  ## What happened  (the stall, heights, container state)
  ## Root cause     (evidence chain, quoting log/dbcheck lines)
  ## Drive-fix assessment   (direct answer to task 3)
  ## Database integrity     (per-store dbcheck results: which DBs OK/failed)
  ## Recommended actions
Keep it tight and evidence-driven. After writing the file, print the absolute report path and the one-line Verdict to stdout.
"""

# --- helpers --------------------------------------------------------------
def now():
    return time.time()

def log(msg):
    line = "[%s] %s" % (datetime.now(timezone.utc).strftime("%H:%M:%S"), msg)
    print(line, flush=True)
    with open(EVENTS, "a") as f:
        f.write(line + "\n")

def sh(cmd, timeout=30):
    try:
        r = subprocess.run(cmd, shell=True, capture_output=True, text=True, timeout=timeout)
        return r.stdout
    except Exception:
        return ""

def scan_corruption(node):
    """Look for a leveldb corruption signature in the node's recent logs.
    Returns a short 'module file' descriptor if found, else None."""
    out = sh("docker logs --since %ds acc-cl-%s 2>&1" % (POLL_SEC + 20, node), timeout=20)
    if not CORRUPTION_RE.search(out):
        return None
    clean = re.sub(r"\x1b\[[0-9;]*m", "", out)
    wm = re.search(r"module=(\w+)", clean)
    fm = re.search(r"\[file=([^\]]+)\]", clean)
    return (wm.group(1) if wm else "?") + ((" " + fm.group(1)) if fm else "")

def container_state(node):
    out = sh("docker inspect acc-cl-%s --format "
             "'{{.State.Status}}|{{.State.ExitCode}}|{{.State.OOMKilled}}|"
             "{{.State.StartedAt}}|{{.State.FinishedAt}}|{{.RestartCount}}'" % node)
    p = out.strip().split("|")
    if len(p) < 6:
        return {"status": "unknown"}
    return {"status": p[0], "exit_code": p[1], "oom": p[2] == "true",
            "started": p[3], "finished": p[4], "restarts": p[5]}

def node_height(node, port):
    out = sh("docker exec acc-cl-%s sh -c "
             "'wget -qO- http://localhost:%d/status 2>/dev/null'" % (node, port), timeout=10)
    m = re.search(r'"latest_block_height":"?(\d+)', out)
    return int(m.group(1)) if m else None

def loadmix_tail(nlines=4):
    out = sh("tail -n %d %s 2>/dev/null" % (nlines, LOADMIX_LOG))
    return out.strip()

def run_dbcheck(node):
    """Scan one node's LevelDB stores. Returns (corrupt, failed_count, json_path, log_path)."""
    jpath = os.path.join(RUN_DIR, "%s-dbcheck.json" % node)
    lpath = os.path.join(RUN_DIR, "%s-dbcheck.log" % node)
    cmd = ("docker run --rm -u 0 -v %s/%s:/data:ro -v %s:/dbcheck:ro alpine /dbcheck /data"
           % (DATA_ROOT, node, DBCHECK_BIN))
    with open(jpath, "w") as jf, open(lpath, "w") as lf:
        subprocess.run(cmd, shell=True, stdout=jf, stderr=lf, timeout=600)
    failed = -1
    try:
        with open(jpath) as f:
            failed = json.load(f).get("failed", -1)
    except Exception:
        pass
    return (failed > 0, failed, jpath, lpath)

def collect_logs(node):
    lpath = os.path.join(RUN_DIR, "%s.log" % node)
    out = sh("docker logs --tail 3000 acc-cl-%s 2>&1" % node, timeout=30)
    with open(lpath, "w") as f:
        f.write(out)
    return lpath

def call_claude(prompt, node):
    report = os.path.join(RUN_DIR, "REPORT-%s.md" % node)
    ppath = os.path.join(RUN_DIR, "prompt-%s.txt" % node)
    with open(ppath, "w") as f:
        f.write(prompt)
    log("invoking claude for root-cause report -> %s" % report)
    clog = os.path.join(RUN_DIR, "claude-%s.log" % node)
    with open(clog, "w") as cf:
        subprocess.run([CLAUDE_BIN, "-p", prompt,
                        "--dangerously-skip-permissions",
                        "--add-dir", DATA_ROOT, "--add-dir", RUN_DIR],
                       cwd=REPO, stdout=cf, stderr=cf, timeout=1800)
    return report

def stop_node(node):
    sh("docker stop -t 10 acc-cl-%s" % node, timeout=30)

def start_node(node):
    # docker start re-runs the EXISTING container's command against its existing
    # DB. NEVER `docker compose up` / init — that would --reset and wipe the DB.
    sh("docker start acc-cl-%s" % node, timeout=30)

# --- stall handling -------------------------------------------------------
def handle_stall(node, reason, heights, peak, st, loadmix_proc):
    log("STALL: %s (%s)" % (node, reason))
    stop_node(node)
    st_after = container_state(node)
    node_log = collect_logs(node)
    stall_time = datetime.now(timezone.utc).isoformat()
    stall_json = os.path.join(RUN_DIR, "%s-stall.json" % node)
    with open(stall_json, "w") as f:
        json.dump({
            "node": node, "partition": BVN_OF[node], "reason": reason,
            "stall_time": stall_time,
            "node_height": heights.get(node), "peer_max_height": peak,
            "container_state_before": st, "container_state_after": st_after,
            "loadmix_tail": loadmix_tail(),
        }, f, indent=2)

    corrupt, failed, dbjson, dblog = run_dbcheck(node)
    log("dbcheck %s: failed=%d -> %s" % (node, failed, "CORRUPT" if corrupt else "clean"))

    if corrupt:
        log("!!! DATABASE CORRUPTION on %s — the T700 drive fix FAILED. Halting test. !!!" % node)
        try:
            loadmix_proc.send_signal(signal.SIGINT)
        except Exception:
            pass
        prompt = PROMPT_TMPL.format(
            data_root=DATA_ROOT, node=node, reason=reason, failed_count=failed,
            stall_json=stall_json, node_log=node_log,
            dbcheck_json=dbjson, dbcheck_log=dblog, dbcheck_bin=DBCHECK_BIN,
            loadmix_log=LOADMIX_LOG, stall_time=stall_time,
            report_path=os.path.join(RUN_DIR, "REPORT-%s.md" % node))
        report = call_claude(prompt, node)
        with open(os.path.join(RUN_DIR, "HALTED"), "w") as f:
            f.write("corruption on %s at %s\nreport: %s\n" % (node, stall_time, report))
        log("report written: %s  — see %s" % (report, RUN_DIR))
        return True   # halt

    # clean DB: restart and keep going
    start_node(node)
    log("restarted %s (clean DB); grace %ds" % (node, RESTART_GRACE))
    return False

def parse_duration(s):
    m = re.match(r"(\d+)\s*([hm])", s)
    if not m:
        return 24 * 3600
    return int(m.group(1)) * (3600 if m.group(2) == "h" else 60)

# --- main -----------------------------------------------------------------
def main():
    total_sec = parse_duration(DURATION)
    log("=== acc-cl %d TPS soak watchdog ===  run dir: %s" % (TPS, RUN_DIR))
    log("soak: %d TPS for %s wall-clock, duty cycle %dm ON / %dm OFF each hour -> %s"
        % (TPS, DURATION, LOAD_ON_SEC // 60, LOAD_OFF_SEC // 60, LOADMIX_LOG))

    lm_log = open(LOADMIX_LOG, "a")   # append: load restarts each ON phase
    lm = {"p": None}                  # current loadmix process (None while paused)

    def start_load():
        lm_log.write("\n==== load ON @ %s ====\n" % datetime.now(timezone.utc).strftime("%H:%M:%SZ"))
        lm_log.flush()
        # --duration is an upper bound; the watchdog ends each ON phase by SIGINT.
        lm["p"] = subprocess.Popen(
            [DEBUG_BIN, "loadmix", SERVER, FAUCET_AS1,
             "--tps", str(TPS), "--duration", DURATION,
             "--actors", str(ACTORS), "--report-interval", "30s"],
            cwd=REPO, stdout=lm_log, stderr=subprocess.STDOUT)

    def stop_load():
        if lm["p"] and lm["p"].poll() is None:
            try: lm["p"].send_signal(signal.SIGINT)
            except Exception: pass
        lm["p"] = None

    last_adv = {}     # (node,chain) -> ts of last height increase
    last_h   = {}     # (node,chain) -> last height
    grace    = {n: now() + RESTART_GRACE for n in ALL_NODES}   # initial startup grace
    t0 = now()
    deadline = t0 + total_sec

    def cleanup(*_):
        log("watchdog stopping; killing loadmix")
        stop_load()
        sys.exit(0)
    signal.signal(signal.SIGINT, cleanup)
    signal.signal(signal.SIGTERM, cleanup)

    # start first ON phase
    phase = "on"
    phase_end = now() + LOAD_ON_SEC
    start_load()
    log("load ON (%d TPS) for %dm" % (TPS, LOAD_ON_SEC // 60))

    while now() < deadline:
        # duty-cycle transitions
        if now() >= phase_end:
            if phase == "on":
                stop_load()
                phase, phase_end = "off", now() + LOAD_OFF_SEC
                log("=== transactions PAUSED for %dm ===" % (LOAD_OFF_SEC // 60))
            else:
                start_load()
                phase, phase_end = "on", now() + LOAD_ON_SEC
                # idle skews the advance baseline; reset so resume isn't flagged as a stall
                for n in ALL_NODES:
                    grace[n] = now() + RESTART_GRACE
                    for c in ("dn", "bvn"):
                        last_adv[(n, c)] = now()
                log("=== transactions RESUMED (%d TPS) for %dm ===" % (TPS, LOAD_ON_SEC // 60))
        # during ON, if loadmix died early (crash, not our SIGINT), restart it
        if phase == "on" and (lm["p"] is None or lm["p"].poll() is not None):
            log("loadmix exited mid-ON (rc=%s) — restarting" % (lm["p"].returncode if lm["p"] else "?"))
            start_load()

        states  = {n: container_state(n) for n in ALL_NODES}
        heights = {}   # node -> {'dn':h,'bvn':h}
        for n in ALL_NODES:
            if states[n].get("status") == "running":
                heights[n] = {"dn": node_height(n, DN_RPC), "bvn": node_height(n, BVN_RPC)}

        # update advance tracking
        t = now()
        for n, h in heights.items():
            for chain in ("dn", "bvn"):
                key, cur = (n, chain), h.get(chain)
                if cur is None:
                    continue
                if last_h.get(key) is None or cur > last_h[key]:
                    last_h[key] = cur
                    last_adv[key] = t

        # partition peer-max per chain
        dn_max = max([h["dn"] for h in heights.values() if h.get("dn") is not None], default=0)
        bvn_max = {p: max([heights[n]["bvn"] for n in ns
                           if n in heights and heights[n].get("bvn") is not None], default=0)
                   for p, ns in PARTITIONS.items()}

        for n in ALL_NODES:
            st = states[n]

            # 0) DB corruption in logs — highest priority. Can occur while the
            # node is still making blocks (e.g. tx_index), so check before grace.
            cm = scan_corruption(n)
            if cm:
                if handle_stall(n, "leveldb corruption in logs (%s)" % cm,
                                {x: (heights.get(x) or {}).get("bvn") for x in ALL_NODES},
                                bvn_max[BVN_OF[n]], st, lm["p"]):
                    cleanup_and_exit()
                grace[n] = now() + RESTART_GRACE
                continue

            if t < grace.get(n, 0):
                continue

            # 1) crashed: container exited (restart:no keeps it down)
            if st.get("status") == "exited":
                if handle_stall(n, "container exited (code %s, oom=%s)"
                                % (st.get("exit_code"), st.get("oom")),
                                {x: (heights.get(x) or {}).get("bvn") for x in ALL_NODES},
                                bvn_max[BVN_OF[n]], st, lm["p"]):
                    cleanup_and_exit()
                grace[n] = now() + RESTART_GRACE
                continue

            # 2) hung: height frozen while partition peers advanced past it.
            # ON phase only — during the pause all nodes idle together, so a flat
            # height is expected, not a stall.
            if phase != "on" or n not in heights:
                continue
            for chain, peak in (("dn", dn_max), ("bvn", bvn_max[BVN_OF[n]])):
                cur = heights[n].get(chain)
                key = (n, chain)
                if cur is None or key not in last_adv:
                    continue
                if (t - last_adv[key]) > STALL_SEC and peak > cur + 1:
                    if handle_stall(n, "%s height frozen %ds at %d (peers at %d)"
                                    % (chain, int(t-last_adv[key]), cur, peak),
                                    {x: (heights.get(x) or {}).get(chain) for x in ALL_NODES},
                                    peak, st, lm["p"]):
                        cleanup_and_exit()
                    grace[n] = now() + RESTART_GRACE
                    for c in ("dn", "bvn"):
                        last_adv[(n, c)] = now()
                    break

        time.sleep(POLL_SEC)

    stop_load()
    log("soak complete: %s at %d TPS (%dm ON / %dm OFF duty) with NO corruption observed "
        "— control run clean on the healthy drive." % (DURATION, TPS, LOAD_ON_SEC // 60, LOAD_OFF_SEC // 60))

def cleanup_and_exit():
    log("=== TEST HALTED (corruption). Evidence + report in %s ===" % RUN_DIR)
    sys.exit(1)

if __name__ == "__main__":
    main()
