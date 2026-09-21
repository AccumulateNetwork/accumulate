#!/usr/bin/env python3
"""What soak.sh's chaos walk needs to add and remove a follower (#4364).

    followerchaos.py late                       # service container dir, one per line
    followerchaos.py state CONTAINER            # exit 0 once every partition is ACTIVE
    followerchaos.py rootmatch CONTAINER SINCE  # exit 0 once a root matches a validator's
    followerchaos.py snapshot OUT.json          # heights and every stream's delivered
    followerchaos.py unaffected A B C           # A..B before a removal, B..C after

The chaos loop is bash and stays bash; every reading it takes of the network
is here, where it can be tested without one.

**Which follower.** The one whose compose service carries the
`late-follower` profile, so that `compose up` does not start it — read from
docker-compose.yml and checked against docker-network.yml, where it must be
declared a follower. A service in that profile that the network file calls a
validator is not a follower and is not returned.

**ACTIVE** is `accumulate_node_state` reading 2 on every partition the node
exports it for (the node-state row of e11.4364a, soakmon.nodestate_from).
No gauge is not ACTIVE.

**First root match** is followerlog.first_root_match over the follower's own
log and its BVN's validators', each read with `docker logs --since` the add.

**Unaffected**, for a removal: every partition's block cadence after the
removal at least CADENCE_FLOOR of what it was before, and every stream whose
delivered count advanced before the removal still advancing after it, and
none going backwards. Read over the v3 API from the validators' own ports,
the max over several (a halted node can only under-report, soakmon
collect_heights). A window with no reading is `not measured`, never
unaffected.

NOT verified here: any of it against a running network. The tests drive
these functions on fixtures; only a run shows the readings are the ones a
live node gives.
"""
import json
import os
import re
import subprocess
import sys
import time
import urllib.request

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.dirname(HERE))
sys.path.insert(0, HERE)
import topology  # noqa: E402
import followerlog  # noqa: E402

PROFILE = "late-follower"
METRICS_URL = "http://127.0.0.1:26670/metrics"
NODE_STATE = "accumulate_node_state"
NODE_STATE_NAMES = {0: "BOOTING", 1: "WAITING", 2: "ACTIVE", 3: "COMPLETE"}
ACTIVE = 2
CADENCE_FLOOR = 0.5

_SERVICE = re.compile(r"^  ([A-Za-z0-9_.-]+):\s*$")
_PROFILES = re.compile(r"^    profiles:\s*\[([^\]]*)\]")
_CONTAINER = re.compile(r"^    container_name:\s*\"?([A-Za-z0-9_.-]+)\"?\s*$")
_GAUGE = re.compile(r'^' + NODE_STATE + r'\{([^}]*)\}\s+([-0-9.eE+]+)')


def late_followers(compose_path=None, net_path=None):
    """Followers whose compose service is in the `late-follower` profile, as
    {service, container, dir, bvn, port, partitions}, in compose order."""
    with open(compose_path or topology.COMPOSE_YML) as f:
        text = f.read()
    services, cur = [], None
    in_services = False
    for line in text.splitlines():
        if re.match(r"^services:\s*$", line):
            in_services = True
            continue
        if in_services and re.match(r"^\S", line):
            in_services = False
        if not in_services:
            continue
        m = _SERVICE.match(line)
        if m:
            cur = {"service": m.group(1), "profiles": [], "container": None}
            services.append(cur)
            continue
        if cur is None:
            continue
        m = _PROFILES.match(line)
        if m:
            cur["profiles"] = [p.strip().strip("\"'") for p in m.group(1).split(",")]
            continue
        m = _CONTAINER.match(line)
        if m:
            cur["container"] = m.group(1)
    fols = {f["container"]: f for f in topology.followers(net_path)}
    out = []
    for s in services:
        if PROFILE in s["profiles"] and s["container"] in fols:
            f = fols[s["container"]]
            out.append({"service": s["service"], "container": s["container"],
                        "dir": f["dir"], "bvn": f["bvn"], "port": f["port"],
                        "partitions": f["partitions"]})
    return out


def node_states(metrics_text):
    """accumulate_node_state per partition, from one metrics scrape."""
    out = {}
    for line in (metrics_text or "").splitlines():
        m = _GAUGE.match(line)
        if not m:
            continue
        lab = dict(re.findall(r'(\w+)="([^"]*)"', m.group(1)))
        try:
            out[(lab.get("partition") or "?").lower()] = int(float(m.group(2)))
        except ValueError:
            continue
    return out


def all_active(states):
    return bool(states) and all(v == ACTIVE for v in states.values())


def describe_states(states):
    if not states:
        return "not measured (no %s gauge)" % NODE_STATE
    return " ".join("%s=%s" % (p, NODE_STATE_NAMES.get(v, v))
                    for p, v in sorted(states.items()))


def _run(args, timeout=30):
    try:
        return subprocess.run(args, capture_output=True, text=True,
                              timeout=timeout).stdout
    except Exception:
        return ""


def scrape(container):
    return _run(["docker", "exec", container, "sh", "-c",
                 "wget -q -O - %s 2>/dev/null" % METRICS_URL], timeout=20)


def container_log(container, since):
    """`docker logs` in the `compose logs` shape followerlog reads:
    `<container> | <line>`."""
    txt = _run(["docker", "logs", "--since", since, container], timeout=60)
    return ["%s | %s" % (container, ln) for ln in txt.splitlines()]


def root_match_in(follower, lines):
    """The follower's first root match against every other container in
    `lines`, as (source, block, root) or None."""
    r = followerlog.read(lines)
    others = sorted(c for c in set(r.anchors) | set(r.identities) if c != follower)
    return followerlog.first_root_match(r, follower, others)


# --- the removal's verdict ---------------------------------------------------

def _post(port, params, timeout=3):
    body = json.dumps({"jsonrpc": "2.0", "id": 1, "method": "query",
                       "params": params}).encode()
    req = urllib.request.Request("http://127.0.0.1:%d/v3" % port, data=body,
                                 headers={"content-type": "application/json"})
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            return json.loads(resp.read())["result"]
    except Exception:
        return None


def _part(url):
    host = url.split("//", 1)[-1].split(".")[0]
    return "Directory" if host.lower() == "dn" else host.replace("bvn-", "")


def snapshot(ports=None, now=None):
    """Every partition's height and every stream's delivered count, each the
    max over `ports` (the validators'), with the time it was taken."""
    ports = ports or topology.probe_ports()
    scopes = topology.scopes()
    snap = {"t": time.time() if now is None else now, "heights": {}, "delivered": {}}
    for p in topology.partitions():
        best = None
        for port in ports:
            r = _post(port, {"scope": "acc://%s.acme/ledger" % scopes[p]})
            try:
                v = int(r["account"]["index"])
            except Exception:
                continue
            best = v if best is None else max(best, v)
        snap["heights"][p] = best
        for kind, path in (("synthetic", "synthetic"), ("anchor", "anchors")):
            for port in ports:
                r = _post(port, {"scope": "acc://%s.acme/%s" % (scopes[p], path)})
                try:
                    seq = r["account"]["sequence"] or []
                except Exception:
                    continue
                for e in seq:
                    url = e.get("url") or ""
                    if not url:
                        continue
                    key = "%s %s->%s" % (kind, _part(url), p)
                    d = int(e.get("delivered") or 0)
                    snap["delivered"][key] = max(snap["delivered"].get(key, 0), d)
    return snap


def unaffected(before, at, after, floor=CADENCE_FLOOR):
    """(verdict, text): verdict True unaffected, False affected, None not
    measured. `before`..`at` is the window ending at the removal and
    `at`..`after` the one after it."""
    dt1 = (at.get("t") or 0) - (before.get("t") or 0)
    dt2 = (after.get("t") or 0) - (at.get("t") or 0)
    if dt1 <= 0 or dt2 <= 0:
        return None, "not measured (the windows have no length)"
    bad, cad, unmeasured = [], [], []
    for p in sorted(at.get("heights") or {}):
        h0, h1, h2 = (s.get("heights", {}).get(p) for s in (before, at, after))
        if h0 is None or h1 is None or h2 is None:
            unmeasured.append(p)
            continue
        r1, r2 = (h1 - h0) / dt1, (h2 - h1) / dt2
        cad.append("%s %.2f->%.2f" % (p, r1, r2))
        if r2 <= 0 or r2 < floor * r1:
            bad.append("%s cadence %.2f->%.2f blocks/s" % (p, r1, r2))
    streams, quiet, advancing = 0, 0, 0
    for k in sorted(at.get("delivered") or {}):
        d0 = before.get("delivered", {}).get(k)
        d1 = at["delivered"][k]
        d2 = after.get("delivered", {}).get(k)
        if d0 is None or d2 is None:
            unmeasured.append(k)
            continue
        streams += 1
        if d2 < d1 or d1 < d0:
            bad.append("%s delivered went backwards %d->%d->%d" % (k, d0, d1, d2))
        elif d1 > d0 and d2 == d1:
            bad.append("%s delivered stopped at %d" % (k, d1))
        elif d1 == d0:
            quiet += 1
        else:
            advancing += 1
    if not cad and not streams:
        return None, "not measured (no height or stream answered in the windows)"
    text = ("cadence blocks/s before->after: %s; %d streams, %d advancing "
            "before and after, %d quiet before" % (
                ", ".join(cad) or "not measured", streams, advancing, quiet))
    if unmeasured:
        text += "; not measured: %s" % ", ".join(unmeasured)
    if bad:
        return False, "AFFECTED: %s; %s" % ("; ".join(bad), text)
    return True, "unaffected: " + text


def main(argv):
    if not argv:
        sys.stderr.write(__doc__)
        return 2
    cmd, args = argv[0], argv[1:]
    if cmd == "late":
        for f in late_followers():
            print(f["service"], f["container"], f["dir"])
        return 0
    if cmd == "state":
        states = node_states(scrape(args[0]))
        print(describe_states(states))
        return 0 if all_active(states) else 1
    if cmd == "rootmatch":
        follower, since = args[0], args[1]
        rec = next((f for f in topology.followers() if f["container"] == follower), None)
        bvn = rec["bvn"] if rec else None
        vals = [v["container"] for v in topology.validator_records()
                if bvn is None or v["bvn"] == bvn]
        lines = container_log(follower, since)
        for v in vals:
            lines += container_log(v, since)
        m = root_match_in(follower, lines)
        if not m:
            print("no block yet whose root equals a validator's")
            return 1
        print("source=%s block=%d root=%s" % m)
        return 0
    if cmd == "snapshot":
        with open(args[0], "w") as f:
            json.dump(snapshot(), f)
        return 0
    if cmd == "unaffected":
        snaps = []
        for p in args[:3]:
            try:
                with open(p) as f:
                    snaps.append(json.load(f))
            except Exception:
                snaps.append({})
        ok, text = unaffected(*snaps)
        print(text)
        return {True: 0, False: 1, None: 2}[ok]
    sys.stderr.write("unknown command %r\n" % cmd)
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
