#!/usr/bin/env python3
"""A follower's join, read from its own log (the soak monitor's Followers panel).

The monitor reads logs, not node metrics (Paul, 2026-09-28). Every line this
reads is written by internal/node/join or internal/node/dagbft and carries
``partition=<ID>``; one container runs a Directory node and a BVN node in one
log stream, so every count here is per (container, partition).

Phase, the latest line of these per partition wins:

    Pulling the state          -> pulling      (join/state.go:525)
    The walk has covered       -> walked       (join/state.go:1020)
    Joined                     -> handed off   (join/join.go:517, dagbft/collect.go:428)
    not executing in agreement -> BOOTING      (join/state.go:1599, a drop)
    executing in agreement     -> ACTIVE       (join/state.go:1585)

Counts, whole container log:

    rootMisses     "not its anchored root"          (join/root_watch.go:90)
    repairs        "Repairing the state"            (join/state.go:666)
    repairChanged  "A repair changed an account"    (join/state.go:1204)
    drops          ACTIVE -> BOOTING, i.e. every "not executing in agreement"

The log is read incrementally: ``docker logs --timestamps --since <last>``,
and a line at or before the last docker timestamp already read is skipped.
"""
import calendar
import re
import subprocess
import time

ANSI = re.compile(r"\x1b\[[0-9;]*m")
PART = re.compile(r"\bpartition=(\S+)")
# Order matters: "not executing in agreement" contains "executing in agreement".
PHASES = (
    ("not executing in agreement", "BOOTING"),
    ("executing in agreement", "ACTIVE"),
    ("Joined", "handed off"),
    ("The walk has covered", "walked"),
    ("Pulling the state", "pulling"),
)
COUNTS = (
    ("not its anchored root", "rootMisses"),
    ("Repairing the state", "repairs"),
    ("A repair changed an account", "repairChanged"),
)
_ANY = re.compile("|".join(re.escape(k) for k, _ in PHASES + COUNTS))


def _epoch(ts):
    """RFC 3339 (docker's or the node's own) to epoch seconds, or None."""
    try:
        return calendar.timegm(time.strptime(ts[:19], "%Y-%m-%dT%H:%M:%S"))
    except (ValueError, TypeError):
        return None


def part_key(p):
    """One key for a partition however the line spells it: the join logs its
    URL (``acc://dn.acme``, ``acc://bvn-BVN1.acme``), dagbft its ID
    (``Directory``, ``BVN1``). Both become ``directory`` / ``bvn1``."""
    p = (p or "").strip().strip('"').lower()
    if p.startswith("acc://"):
        p = p[len("acc://"):].split("/")[0]
        if p.endswith(".acme"):
            p = p[:-len(".acme")]
        p = {"dn": "directory"}.get(p, p)
        if p.startswith("bvn-"):
            p = p[len("bvn-"):]
    return p


def new_partition():
    return {"phase": "not started", "phaseAt": None, "firstActiveAt": None,
            "rootMisses": 0, "repairs": 0, "repairChanged": 0, "drops": 0}


class Reader:
    """Per container: the last docker timestamp read and, per partition,
    the phase and the counts."""

    def __init__(self):
        self.last = {}     # container -> docker RFC3339Nano of the last line
        self.parts = {}    # container -> {partition: new_partition()}
        self.started = {}  # container -> startedAt of the container read

    def track(self, container, st):
        """Forget a container that is gone, or that is a new container under
        the same name (a follower removed and added again): its phase and
        counts belong to the container that logged them, not to the name."""
        if st is None:
            self.forget(container)
            return
        if self.started.get(container) not in (None, st.get("startedAt")):
            self.forget(container)
        self.started[container] = st.get("startedAt")

    def forget(self, container):
        self.last.pop(container, None)
        self.parts.pop(container, None)
        self.started.pop(container, None)

    def feed(self, container, lines):
        """Take `docker logs --timestamps` lines (docker's timestamp first)."""
        last = self.last.get(container, "")
        parts = self.parts.setdefault(container, {})
        for raw in lines:
            ts, _, ln = raw.partition(" ")
            if last and ts <= last:
                continue
            last = ts
            if not _ANY.search(ln):
                continue
            ln = ANSI.sub("", ln)
            m = PART.search(ln)
            if not m:
                continue
            p = parts.setdefault(part_key(m.group(1)), new_partition())
            at = _epoch(ts)
            for key, phase in PHASES:
                if key in ln:
                    p["phase"], p["phaseAt"] = phase, at
                    if phase == "ACTIVE" and p["firstActiveAt"] is None:
                        p["firstActiveAt"] = at
                    if phase == "BOOTING":
                        p["drops"] += 1
                    break
            for key, field in COUNTS:
                if key in ln:
                    p[field] += 1
        if last:
            self.last[container] = last

    def poll(self, container):
        """Read what the container logged since the last poll. A container
        that does not exist yet reads nothing and keeps no position."""
        args = ["docker", "logs", "--timestamps"]
        if self.last.get(container):
            args += ["--since", self.last[container]]
        try:
            p = subprocess.Popen(args + [container], stdout=subprocess.PIPE,
                                 stderr=subprocess.STDOUT, text=True,
                                 errors="replace")
        except OSError:
            return False
        keep, tail = [], None
        for ln in p.stdout:
            # Keep only what feed() can use, plus the last line so the
            # position advances; the whole log is never held in memory.
            if _ANY.search(ln):
                keep.append(ln.rstrip("\n"))
            tail = ln
        p.wait()
        if p.returncode != 0:
            return False
        if tail and (not keep or keep[-1] != tail.rstrip("\n")):
            keep.append(tail.rstrip("\n"))
        self.feed(container, keep)
        return True


def docker_state(containers):
    """{container: {"status", "exitCode", "oomKilled", "startedAt"}} for the
    containers that exist; one `docker inspect` for all."""
    if not containers:
        return {}
    p = subprocess.run(
        ["docker", "inspect", "-f",
         "{{.Name}} {{.State.Status}} {{.State.ExitCode}} {{.State.OOMKilled}} {{.State.StartedAt}}"]
        + list(containers), capture_output=True, text=True, timeout=20)
    out = {}
    for line in p.stdout.splitlines():
        f = line.split()
        if len(f) == 5:
            out[f[0].lstrip("/")] = {"status": f[1], "exitCode": int(f[2]),
                                     "oomKilled": f[3] == "true",
                                     "startedAt": _epoch(f[4])}
    return out


def docker_mem(containers):
    """{container: MiB} from one `docker stats --no-stream`."""
    if not containers:
        return {}
    p = subprocess.run(["docker", "stats", "--no-stream", "--format",
                        "{{.Name}} {{.MemUsage}}"] + list(containers),
                       capture_output=True, text=True, timeout=30)
    out = {}
    for line in p.stdout.splitlines():
        f = line.split()
        if len(f) >= 2:
            out[f[0]] = mem_mib(f[1])
    return out


def mem_mib(s):
    """docker's `761.2MiB` / `1.2GiB` / `512KiB` as MiB, or None."""
    m = re.match(r"([\d.]+)\s*([KMG]i?B|B)", s or "")
    if not m:
        return None
    mul = {"B": 1 / 1048576, "KiB": 1 / 1024, "KB": 1 / 1024, "MiB": 1,
           "MB": 1, "GiB": 1024, "GB": 1024}[m.group(2)]
    return round(float(m.group(1)) * mul, 1)


def rows(followers, reader, heights, lives, state, mem, now):
    """One row per follower x partition, for /data's `followerJoins`.

    followers: topology.followers(); heights: /data's `follower` record;
    lives: {container: life} as collect_follower computed it (kind, at)."""
    out = []
    for f in followers:
        c = f["container"]
        life = lives.get(c) or {"kind": "launched", "at": None}
        st = state.get(c)
        fh = ((heights or {}).get("nodes") or {}).get(c) or {}
        # Since the add: chaos.log's add for a late follower; the container's
        # start for one launched with the network.
        added = _epoch(life.get("at")) if life.get("at") else (
            st["startedAt"] if st and life["kind"] == "launched" else None)
        for part in f["partitions"]:
            lp = (reader.parts.get(c) or {}).get(part_key(part)) or new_partition()
            h = (fh.get("partitions") or {}).get(part) or {}
            phase = lp["phase"]
            gone = st is None and life["kind"] != "waiting"
            if life["kind"] == "waiting":
                phase = "not added"
            elif life["kind"] == "removed" or gone:
                phase = "removed"
            fa = lp["firstActiveAt"]
            out.append({
                "container": c, "partition": part, "life": life["kind"],
                "phase": phase, "phaseAt": lp["phaseAt"],
                "height": h.get("follower"), "network": h.get("network"),
                "behind": h.get("behind"),
                "addedAt": added,
                "sinceAddSec": (now - added) if added else None,
                "toActiveSec": (fa - added) if (fa and added and fa >= added) else None,
                "rootMisses": lp["rootMisses"], "repairs": lp["repairs"],
                "repairChanged": lp["repairChanged"], "drops": lp["drops"],
                "memMiB": mem.get(c),
                "status": st["status"] if st else None,
                "exitCode": st["exitCode"] if st else None,
                "oomKilled": st["oomKilled"] if st else None,
            })
    return out
