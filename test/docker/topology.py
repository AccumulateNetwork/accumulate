# Copyright 2026 The Accumulate Authors
#
# Use of this source code is governed by an MIT-style
# license that can be found in the LICENSE file or at
# https://opensource.org/licenses/MIT.
"""The soak topology, read from docker-network.yml — the file that defines it.

Every tool in this directory used to carry its own copy of the shape of the
network: ``["Directory","BVN1","BVN2","BVN3"]`` in soakmon and streams, the
host ports ``seq 26660 26671`` in soak.sh, a partition list in blockrate, an
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
same order the nodes appear here, starting at 26660. That ordering is the
contract between the two files; `node_ports` encodes it, and
`check_ports_against_compose` verifies it rather than trusting it.
"""

import os
import re

HERE = os.path.dirname(os.path.abspath(__file__))
NETWORK_YML = os.path.join(HERE, "docker-network.yml")
COMPOSE_YML = os.path.join(HERE, "docker-compose.yml")

# The host port the first node is published on. Subsequent nodes take the
# next port, in docker-network.yml order.
BASE_HOST_PORT = 26660

_ID = re.compile(r'^\s*-\s*id:\s*"?([A-Za-z0-9]+)"?\s*$')
_NODE = re.compile(r'^\s*-\s*listenAddress:')


def _read(path):
    with open(path) as f:
        return f.read()


def bvns(path=None):
    """The BVN ids, in declaration order — e.g. ["BVN1", "BVN2"]."""
    out = []
    for line in _read(path or NETWORK_YML).splitlines():
        m = _ID.match(line)
        if m:
            out.append(m.group(1))
    return out


def nodes_per_bvn(path=None):
    """Node count per BVN id, in declaration order.

    Counted by walking the file rather than assuming a uniform fan-out: an
    asymmetric topology (one BVN deliberately short a validator, to test a
    partition running at bare quorum) must not silently report as uniform.
    """
    counts = {}
    current = None
    for line in _read(path or NETWORK_YML).splitlines():
        m = _ID.match(line)
        if m:
            current = m.group(1)
            counts[current] = 0
        elif current and _NODE.match(line):
            counts[current] += 1
    return counts


def node_count(path=None):
    return sum(nodes_per_bvn(path).values())


def partitions(path=None):
    """Every partition, Directory first — the DN is a partition too.

    The DN is not declared in the `bvns:` list, but every validator here runs
    a DN engine alongside its BVN engine (dnnType/bvnnType are both
    "validator"), so the Directory is as real a partition as any BVN and is
    the one whose stall matters most.
    """
    return ["Directory"] + bvns(path)


def scopes(path=None):
    """Partition id -> the account-URL host that addresses its ledger."""
    s = {"Directory": "dn"}
    for b in bvns(path):
        s[b] = "bvn-%s" % b
    return s


def node_ports(path=None):
    """Host ports for every node's API, in docker-network.yml order."""
    return [BASE_HOST_PORT + i for i in range(node_count(path))]


def probe_ports(path=None, limit=5):
    """A spread of host ports to read the same height from several routes.

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
    """
    counts = nodes_per_bvn(path)
    order = bvns(path)
    # Starting host port of each BVN's block of nodes.
    starts, run = {}, BASE_HOST_PORT
    for b in order:
        starts[b] = run
        run += counts[b]

    ports, depth = [], 0
    while len(ports) < limit and any(depth < counts[b] for b in order):
        for b in order:
            if depth < counts[b] and len(ports) < limit:
                ports.append(starts[b] + depth)
        depth += 1
    return ports


def containers(path=None):
    """Validator container names, in docker-network.yml order.

    The name is a convention of docker-compose.yml — ``acc-bvn<N>-val<M>``,
    with N the 1-based BVN index and M the 1-based node index within it. It is
    derived rather than listed because the listed form went stale silently:
    monitor.py and monitoring.py both kept a 12-name roster, and a roster with
    four dead names reports four nodes at 0 MB, which averages into the fleet
    memory figure and understates it by a third.
    """
    counts = nodes_per_bvn(path)
    out = []
    for n, b in enumerate(bvns(path), start=1):
        for m in range(1, counts[b] + 1):
            out.append("acc-bvn%d-val%d" % (n, m))
    return out


def container_paths(path=None):
    """Container name -> its data directory under /root/.accumulate."""
    counts = nodes_per_bvn(path)
    out = {}
    for n, b in enumerate(bvns(path), start=1):
        for m in range(1, counts[b] + 1):
            out["acc-bvn%d-val%d" % (n, m)] = "bvn%d-%d" % (n, m)
    return out


def check_ports_against_compose(net_path=None, compose_path=None):
    """Verify the derived host ports are the ones the compose actually publishes.

    `node_ports` encodes a convention (allocated in declaration order from
    26660) that lives in a different file from the one it is derived from.
    Conventions drift. Returning the mismatch lets a caller fail loudly at
    startup instead of polling dead ports for twelve hours; returns None when
    they agree.
    """
    try:
        text = _read(compose_path or COMPOSE_YML)
    except OSError as e:
        return "cannot read compose: %s" % e
    published = sorted(int(p) for p in
                       re.findall(r'"\s*(\d+)\s*:\s*26660\s*"', text))
    derived = sorted(node_ports(net_path))
    if published != derived:
        return ("docker-network.yml implies API host ports %s but "
                "docker-compose.yml publishes %s" % (derived, published))
    return None


if __name__ == "__main__":
    import json
    import sys
    problem = check_ports_against_compose()
    print(json.dumps({
        "bvns": bvns(),
        "nodesPerBvn": nodes_per_bvn(),
        "nodeCount": node_count(),
        "partitions": partitions(),
        "nodePorts": node_ports(),
        "probePorts": probe_ports(),
        "portCheck": problem or "ok",
    }, indent=2))
    sys.exit(1 if problem else 0)
