# Copyright 2026 The Accumulate Authors
#
# Use of this source code is governed by an MIT-style
# license that can be found in the LICENSE file or at
# https://opensource.org/licenses/MIT.
"""The soak topology, read from docker-network.yml — the file that defines it.

Every tool in this directory used to carry its own copy of the shape of the
network: ``["Directory","BVN1","BVN2","BVN3"]`` in soakmon and streams, the
host ports ``seq 26680 26691`` in soak.sh, a partition list in blockrate, an
explicit container roster in monitor.py and monitoring.py. Six copies of one
fact.

That is fine until the fact changes. Cutting the network from 3 BVNs to 2
(2026-08-25, to free CPU for the 1000 tx/s target) breaks each copy
*differently* and none of them loudly: soakmon would poll a BVN3 that no
longer exists and report it "unknown", which the dashboard renders as a
degraded network for the whole run; soak.sh would hand the loadgen four dead
endpoints and quietly lose a third of its submission capacity; streams.py
would print a channel matrix full of zeros for a partition nobody deleted.
A monitor that misreports the topology is worse than no monitor, because a
run is judged by what it says.

So: parse the one file, derive the rest. The parse is deliberately a small
regex scan rather than PyYAML — this must work on a bare host with no pip
install, and the file's shape is fixed by `accumulated init network`.

The port mapping is the one thing NOT in docker-network.yml: the compose
publishes each node's container port 26660 on a host port, allocated in the
same order the nodes appear here, starting at 26680. That ordering is the
contract between the two files; `all_node_ports` encodes it, and
`check_ports_against_compose` verifies it rather than trusting it.

**Two kinds of node (#4365).** A node whose `dnnType`/`bvnnType` are
`follower` runs exactly the wiring a validator runs — same image, same
config, same conductor and executor — with a key that `BuildGenesisDocs`
enters in the NetworkDefinition as *inactive*, so the DAG-BFT committee,
built from active validators only (`run/dagbft.go:415`), never contains it.
It executes every committed block and it votes on and proposes nothing.

That difference has to reach every consumer of this module, because the ways
it goes wrong are all silent:

- The **load generator** must never be handed a follower's endpoint. A
  non-committee node proposes no batches, so everything submitted there
  strands, and `-max-stranded 20` fails the run for a reason that has
  nothing to do with the network.
- The **chaos loop** must never disturb it. It is what the run is measuring.
- The **network's height** is the max over the validators. A follower is
  expected to lag, and a max taken over a set containing it is unchanged —
  but `behind`, computed against that same max, is then always zero, which
  is the measurement the gate turns on.
- The **per-node memory, disk and heal readers** SHOULD see it — a follower
  that leaks is a finding — but must label it as a follower.

So `node_ports()` means *the validators*, which is what every existing
caller wanted, and the follower has its own accessors. `containers()` means
*every node*, which is what the per-node panels wanted, and the validators
have their own. Each name's meaning is in its docstring and pinned by a test.
"""

import os
import re

HERE = os.path.dirname(os.path.abspath(__file__))
NETWORK_YML = os.path.join(HERE, "docker-network.yml")
COMPOSE_YML = os.path.join(HERE, "docker-compose.yml")

# The host port the first node is published on. Subsequent nodes take the
# next port, in docker-network.yml order.
#
# Not 26660: this host runs a mainnet follower on 127.0.0.1:26660, so a
# container publishing 0.0.0.0:26660 cannot bind and that validator never
# starts, while the rest of the network comes up looking healthy.
BASE_HOST_PORT = 26680

_ID = re.compile(r'^\s*-\s*id:\s*"?([A-Za-z0-9]+)"?\s*$')
_NODE = re.compile(r'^\s*-\s*listenAddress:')
_KV = re.compile(r'^\s*(dnnType|bvnnType|peerAddress):\s*"?([A-Za-z0-9_.-]+)"?\s*$')


def _read(path):
    with open(path) as f:
        return f.read()


def node_records(path=None):
    """Every node declared in the file, in declaration order, fully derived.

    One record per node:

    ``bvn``         the BVN id it belongs to (``"BVN3"``)
    ``bvnIndex``    that BVN's 1-based position (``3``)
    ``position``    the node's 1-based position among ALL nodes of that BVN —
                    this is what `accumulated init network` names the data
                    directory by (``cmd_init_network.go:213``), so it decides
                    ``dir`` and the compose's ``-w``
    ``role``        ``"validator"``, ``"follower"``, or ``"mixed"`` when the
                    two declared types disagree
    ``dir``         ``bvn3-5`` — the directory init writes
    ``container``   ``acc-bvn3-val4`` / ``acc-bvn3-fol1``, numbered within
                    the node's own role so adding a follower renumbers no
                    validator
    ``port``        the host port the compose publishes its API on
    ``partitions``  the partitions this container serves — the Directory and
                    its own BVN, because every container here runs TWO NODES,
                    a DN node and a BVN node, in one process
    ``peerAddress`` as declared, or None

    Everything else in this module is a projection of this list. It used to be
    six separate walks of the file that agreed by luck.
    """
    text = _read(path or NETWORK_YML)
    out = []
    bvn = None
    bvn_index = 0
    per_bvn = {}
    cur = None
    for line in text.splitlines():
        m = _ID.match(line)
        if m:
            bvn = m.group(1)
            bvn_index += 1
            per_bvn[bvn] = {"n": 0, "validator": 0, "follower": 0, "mixed": 0}
            cur = None
            continue
        if bvn is None:
            continue
        if _NODE.match(line):
            per_bvn[bvn]["n"] += 1
            cur = {"bvn": bvn, "bvnIndex": bvn_index,
                   "position": per_bvn[bvn]["n"],
                   "dnnType": None, "bvnnType": None, "peerAddress": None}
            out.append(cur)
            continue
        if cur is None:
            continue
        m = _KV.match(line)
        if m:
            cur[m.group(1)] = m.group(2)

    for rec in out:
        # No declared type is a validator. The committed file declared none
        # for a year; reading absent as "follower" would empty the network.
        dnn = (rec.pop("dnnType") or "validator").lower()
        bvnn = (rec.pop("bvnnType") or "validator").lower()
        rec["dnn"], rec["bvnn"] = dnn, bvnn
        if dnn == bvnn == "validator":
            rec["role"] = "validator"
        elif dnn == bvnn == "follower":
            rec["role"] = "follower"
        else:
            rec["role"] = "mixed"
        counts = per_bvn[rec["bvn"]]
        counts[rec["role"]] += 1
        rec["roleIndex"] = counts[rec["role"]]
        rec["dir"] = "bvn%d-%d" % (rec["bvnIndex"], rec["position"])
        abbr = {"validator": "val", "follower": "fol"}.get(rec["role"], "mix")
        rec["container"] = "acc-bvn%d-%s%d" % (rec["bvnIndex"], abbr, rec["roleIndex"])
        rec["partitions"] = ["Directory", rec["bvn"]]
    for i, rec in enumerate(out):
        rec["port"] = BASE_HOST_PORT + i
    return out


def bvns(path=None):
    """The BVN ids, in declaration order — e.g. ["BVN1", "BVN2"]."""
    out = []
    for line in _read(path or NETWORK_YML).splitlines():
        m = _ID.match(line)
        if m:
            out.append(m.group(1))
    return out


def nodes_per_bvn(path=None):
    """Node count per BVN id, in declaration order — validators AND followers.

    Counted by walking the file rather than assuming a uniform fan-out: an
    asymmetric topology (one BVN deliberately short a validator, to test a
    partition running at bare quorum) must not silently report as uniform.

    This is the total, because it is what decides the data directory names
    and the port allocation. For the size of a committee, count
    `validator_records`.
    """
    counts = {b: 0 for b in bvns(path)}
    for rec in node_records(path):
        counts[rec["bvn"]] += 1
    return counts


def node_count(path=None):
    """Every DECLARED node, followers included — started or not.

    Not what `up.sh` waits for: a node behind a compose profile is declared
    here and is not started by `compose up`. That is `started_count`.
    """
    return len(node_records(path))


_SERVICE = re.compile(r'^  ([A-Za-z0-9_.-]+):\s*$')
_PROFILES = re.compile(r'^    profiles:')
_CONTAINER = re.compile(r'^    container_name:\s*"?([A-Za-z0-9_.-]+)"?\s*$')


def profiled_containers(compose_path=None):
    """Containers whose compose service sits behind a profile.

    `docker compose up -d` activates no profile, so it starts none of these.
    The added follower of #4364 is the one there is: it is declared in
    docker-network.yml, because init has to write its key and directory, and
    only the chaos walk ever starts it.

    Which nodes are started is a fact of the compose, not of
    docker-network.yml, so it is read from there rather than kept as a list
    here that the compose can drift from.
    """
    out, profiled = set(), False
    in_services = False
    for line in _read(compose_path or COMPOSE_YML).splitlines():
        if re.match(r'^[A-Za-z]', line):
            in_services = line.startswith("services:")
            profiled = False
            continue
        if not in_services:
            continue
        if _SERVICE.match(line):
            profiled = False
        elif _PROFILES.match(line):
            profiled = True
        else:
            m = _CONTAINER.match(line)
            if m and profiled:
                out.add(m.group(1))
    return out


def started_records(path=None, compose_path=None):
    """The nodes a plain `docker compose up -d` starts, in declaration order.

    Every declared node but those behind a compose profile. This is what
    `up.sh` waits on: with the late follower counted, its healthy-container
    target was one more than could ever be healthy and the wait never ended
    (#4364).
    """
    late = profiled_containers(compose_path)
    return [r for r in node_records(path) if r["container"] not in late]


def started_count(path=None, compose_path=None):
    """How many nodes `up` starts — what `up.sh` waits for, plus the bootstrap."""
    return len(started_records(path, compose_path))


def partitions(path=None):
    """Every partition, Directory first — the DN is a partition too.

    The DN is not declared in the `bvns:` list, but every container here runs
    TWO NODES — a DN node and a BVN node, in one process sharing one log
    stream — so the Directory is as real a partition as any BVN and is the one
    whose stall matters most.
    """
    return ["Directory"] + bvns(path)


def scopes(path=None):
    """Partition id -> the account-URL host that addresses its ledger."""
    s = {"Directory": "dn"}
    for b in bvns(path):
        s[b] = "bvn-%s" % b
    return s


def validator_records(path=None):
    return [r for r in node_records(path) if r["role"] == "validator"]


def followers(path=None):
    """The follower nodes (#4365) — same wiring, key in no committee."""
    return [r for r in node_records(path) if r["role"] == "follower"]


def node_ports(path=None):
    """Host ports of the **validators**, in declaration order.

    This is what the load generator is handed, what the partition ledgers are
    read from, and what the read-back probe rotates over. It excludes
    followers deliberately: a follower proposes no batches, so load submitted
    there strands, and the network's height must be the validators' so that a
    follower's lag is measurable against it (#4365).

    `all_node_ports` is the port-allocation contract; `follower_ports` is the
    other half of this one.
    """
    return validator_ports(path)


def validator_ports(path=None):
    """The same thing as `node_ports`, named for what it is."""
    return [r["port"] for r in validator_records(path)]


def follower_ports(path=None):
    return [r["port"] for r in followers(path)]


def all_node_ports(path=None):
    """Every node's host port, in declaration order.

    The contract with docker-compose.yml: ports are allocated in the order the
    nodes appear in docker-network.yml, from BASE_HOST_PORT. A follower
    declared anywhere but last therefore moves a validator's port, which is
    what `problems` refuses.
    """
    return [r["port"] for r in node_records(path)]


def probe_ports(path=None, limit=5):
    """A spread of VALIDATOR host ports to read the same height from several routes.

    A single routed query is a single point of stale truth: a chaos-restarted
    node that halts its executor keeps answering queries from its frozen
    state, and a monitor pinned to it reports a healthy network as stalled
    (run 20260824T051249Z, half an hour of a false stall). Reading from
    several nodes and keeping the max fixes that — a halted node can only
    under-report.

    The spread deliberately takes the FIRST node of each BVN before taking a
    second node from any of them, so every partition is represented before
    depth is added. Polling five ports that all live on one BVN would restore
    the single-point-of-truth problem under a different name.

    Followers never appear here: this is the reading a follower's lag is
    measured against.
    """
    by_bvn, order = {}, bvns(path)
    for r in validator_records(path):
        by_bvn.setdefault(r["bvn"], []).append(r["port"])

    ports, depth = [], 0
    while len(ports) < limit and any(depth < len(by_bvn.get(b, ())) for b in order):
        for b in order:
            block = by_bvn.get(b, ())
            if depth < len(block) and len(ports) < limit:
                ports.append(block[depth])
        depth += 1
    return ports


def containers(path=None):
    """EVERY node's container name, in declaration order — followers included.

    The name is a convention of docker-compose.yml — ``acc-bvn<N>-val<M>`` for
    a validator and ``acc-bvn<N>-fol<K>`` for a follower, N the 1-based BVN
    index and M/K the 1-based index within that role. It is derived rather
    than listed because the listed form went stale silently: monitor.py and
    monitoring.py both kept a 12-name roster, and a roster with four dead
    names reports four nodes at 0 MB, which averages into the fleet memory
    figure and understates it by a third.

    Followers are in this list because the per-node memory, disk and heal
    readers must see them — a follower that leaks is a finding. Anything that
    disturbs a node or submits work to one wants `validator_containers`.
    """
    return [r["container"] for r in node_records(path)]


def validator_containers(path=None):
    """The validators only — the chaos roster, and any fleet sum that has to
    stay comparable with runs that had no follower."""
    return [r["container"] for r in validator_records(path)]


def follower_containers(path=None):
    return [r["container"] for r in followers(path)]


def container_paths(path=None):
    """Container name -> its data directory under /root/.accumulate.

    The directory is named by the node's position among ALL of its BVN's
    nodes, because that is how `accumulated init network` writes it — a
    follower declared fifth under BVN3 is `bvn3-5` even though it is the
    first follower.
    """
    return {r["container"]: r["dir"] for r in node_records(path)}


def problems(path=None):
    """Shapes this harness cannot measure, stated before a run rather than
    mis-measured during one. Returns a list of strings; empty means fine.

    Three of them, each found by writing this module rather than by a run:

    1. A follower declared before a validator of the same BVN. Directories are
       named by position, so the follower takes `bvn<N>-1` — the first
       validator's directory and the first validator's host port — and the
       compose, the loadgen and the monitor all quietly point at the wrong
       node.
    2. A node that is a validator on one partition and a follower on the
       other. Legal in the protocol; this harness has one roster per node and
       would have to put it in both.
    3. A `peerAddress` that disagrees with the derived container name. The
       container name is a convention of a different file, and peerAddress
       here IS that container's DNS name, so the two can be checked against
       each other instead of assumed equal.
    """
    out = []
    seen_follower = {}
    for rec in node_records(path):
        if rec["role"] == "mixed":
            out.append("%s declares dnnType: %s and bvnnType: %s — this harness "
                       "measures a node as a validator or as a follower, not as "
                       "one of each" % (rec["dir"], rec["dnn"], rec["bvnn"]))
        if rec["role"] == "follower":
            seen_follower[rec["bvn"]] = rec["dir"]
        elif rec["bvn"] in seen_follower:
            out.append("%s: a validator is declared after the follower %s, so the "
                       "follower took a validator's directory and host port — "
                       "declare followers last in their BVN"
                       % (rec["dir"], seen_follower[rec["bvn"]]))
        pa = rec.get("peerAddress")
        if pa and pa != rec["container"]:
            out.append("%s: peerAddress %s but the compose names this container %s"
                       % (rec["dir"], pa, rec["container"]))
    return out


def check_ports_against_compose(net_path=None, compose_path=None):
    """Verify the derived host ports are the ones the compose actually publishes.

    `all_node_ports` encodes a convention (allocated in declaration order from
    BASE_HOST_PORT) that lives in a different file from the one it is derived from.
    Conventions drift. Returning the mismatch lets a caller fail loudly at
    startup instead of polling dead ports for twelve hours; returns None when
    they agree.

    Every node, follower included: a compose that forgot the follower's
    service would otherwise look consistent, and the run would report the
    follower as unreachable rather than as never started.
    """
    try:
        text = _read(compose_path or COMPOSE_YML)
    except OSError as e:
        return "cannot read compose: %s" % e
    published = sorted(int(p) for p in
                       re.findall(r'"\s*(\d+)\s*:\s*26660\s*"', text))
    derived = sorted(all_node_ports(net_path))
    if published != derived:
        return ("docker-network.yml implies API host ports %s but "
                "docker-compose.yml publishes %s" % (derived, published))
    return None


if __name__ == "__main__":
    import json
    import sys
    problem = check_ports_against_compose()
    probs = problems()
    print(json.dumps({
        "bvns": bvns(),
        "nodesPerBvn": nodes_per_bvn(),
        "nodeCount": node_count(),
        "startedCount": started_count(),
        "partitions": partitions(),
        "nodePorts": node_ports(),
        "followerPorts": follower_ports(),
        "followers": [{k: f[k] for k in ("container", "dir", "port", "partitions")}
                      for f in followers()],
        "probePorts": probe_ports(),
        "portCheck": problem or "ok",
        "problems": probs or "none",
    }, indent=2))
    sys.exit(1 if problem or probs else 0)
