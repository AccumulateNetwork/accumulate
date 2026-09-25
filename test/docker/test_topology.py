# Copyright 2026 The Accumulate Authors
#
# Use of this source code is governed by an MIT-style
# license that can be found in the LICENSE file or at
# https://opensource.org/licenses/MIT.
"""Tests for the derived soak topology.

The reason this module exists is that six tools each carried their own copy of
the network's shape and every one of them went stale differently when the
network was cut from 3 BVNs to 2. The failure mode is silent — a monitor
polling a partition that no longer exists reports it "unknown" rather than
erroring — so the derivation itself needs tests, not just a passing import.

Followers (#4365) are the second thing that shape has to carry. A follower is
a node with the same wiring as a validator and a key that is in no committee:
it must be measured like a node and must never be handed load, disturbed by
chaos, or counted as one of the validators whose height the network's height
is read from. Every one of those is a separate function here, because the way
this goes wrong is one consumer keeping the old meaning of a name.
"""

import os
import tempfile
import unittest

import topology

B = topology.BASE_HOST_PORT

TWO_BVN = '''\
id: "DAGBFTTest"

bvns:
  - id: "BVN1"
    nodes:
      - listenAddress: "0.0.0.0"
        peerAddress: "acc-bvn1-val1"
      - listenAddress: "0.0.0.0"
        peerAddress: "acc-bvn1-val2"
  - id: "BVN2"
    nodes:
      - listenAddress: "0.0.0.0"
        peerAddress: "acc-bvn2-val1"
      - listenAddress: "0.0.0.0"
        peerAddress: "acc-bvn2-val2"
'''

# Deliberately lopsided: BVN1 has three nodes, BVN2 has one.
LOPSIDED = '''\
bvns:
  - id: "BVN1"
    nodes:
      - listenAddress: "0.0.0.0"
      - listenAddress: "0.0.0.0"
      - listenAddress: "0.0.0.0"
  - id: "BVN2"
    nodes:
      - listenAddress: "0.0.0.0"
'''

# The #4365 shape in miniature: two validators per BVN, and one follower
# declared LAST, under the last BVN, so no validator's host port moves.
WITH_FOLLOWER = '''\
bvns:
  - id: "BVN1"
    nodes:
      - listenAddress: "0.0.0.0"
        peerAddress: "acc-bvn1-val1"
        dnnType: "validator"
        bvnnType: "validator"
      - listenAddress: "0.0.0.0"
        peerAddress: "acc-bvn1-val2"
        dnnType: "validator"
        bvnnType: "validator"
  - id: "BVN2"
    nodes:
      - listenAddress: "0.0.0.0"
        peerAddress: "acc-bvn2-val1"
        dnnType: "validator"
        bvnnType: "validator"
      - listenAddress: "0.0.0.0"
        peerAddress: "acc-bvn2-val2"
        dnnType: "validator"
        bvnnType: "validator"
      - listenAddress: "0.0.0.0"
        peerAddress: "acc-bvn2-fol1"
        dnnType: "follower"
        bvnnType: "follower"
'''

# A follower declared BEFORE the validators of its BVN. `accumulated init
# network` names directories by position, so this silently gives bvn1-1 —
# the first validator's name — to a follower.
FOLLOWER_FIRST = '''\
bvns:
  - id: "BVN1"
    nodes:
      - listenAddress: "0.0.0.0"
        dnnType: "follower"
        bvnnType: "follower"
      - listenAddress: "0.0.0.0"
        dnnType: "validator"
        bvnnType: "validator"
'''

# A node that is a validator on the Directory and a follower on its BVN.
# Legal in the protocol, not something this harness can measure: it belongs
# in one roster for one partition and the other for the other.
MIXED = '''\
bvns:
  - id: "BVN1"
    nodes:
      - listenAddress: "0.0.0.0"
        dnnType: "validator"
        bvnnType: "follower"
'''

# The derived container name and the peerAddress the file states disagree.
NAME_DRIFT = '''\
bvns:
  - id: "BVN1"
    nodes:
      - listenAddress: "0.0.0.0"
        peerAddress: "acc-bvn1-node1"
        dnnType: "validator"
        bvnnType: "validator"
'''


def write(text):
    fd, path = tempfile.mkstemp(suffix=".yml")
    with os.fdopen(fd, "w") as f:
        f.write(text)
    return path


class DerivationTest(unittest.TestCase):
    def setUp(self):
        self.two = write(TWO_BVN)
        self.lop = write(LOPSIDED)
        self.addCleanup(os.unlink, self.two)
        self.addCleanup(os.unlink, self.lop)

    def test_directory_is_a_partition_too(self):
        """The DN is not in the bvns: list but every validator runs one."""
        self.assertEqual(["Directory", "BVN1", "BVN2"],
                         topology.partitions(self.two))

    def test_scopes_address_the_right_ledgers(self):
        s = topology.scopes(self.two)
        self.assertEqual("dn", s["Directory"])
        self.assertEqual("bvn-BVN1", s["BVN1"])

    def test_ports_are_allocated_in_declaration_order(self):
        self.assertEqual([B, B + 1, B + 2, B + 3],
                         topology.node_ports(self.two))

    def test_container_names_follow_the_compose_convention(self):
        self.assertEqual(
            ["acc-bvn1-val1", "acc-bvn1-val2", "acc-bvn2-val1", "acc-bvn2-val2"],
            topology.containers(self.two))

    def test_container_paths_match_the_compose_working_dirs(self):
        self.assertEqual("bvn2-1",
                         topology.container_paths(self.two)["acc-bvn2-val1"])

    def test_a_node_with_no_declared_type_is_a_validator(self):
        """The committed file declared no types for years. Absent is not a
        follower; a parser that read it that way would empty the network."""
        self.assertEqual([], topology.followers(self.two))
        self.assertEqual(4, len(topology.validator_ports(self.two)))

    def test_asymmetric_topology_is_not_flattened(self):
        """A BVN deliberately short a validator must not report as uniform."""
        self.assertEqual({"BVN1": 3, "BVN2": 1},
                         topology.nodes_per_bvn(self.lop))
        self.assertEqual(4, topology.node_count(self.lop))
        self.assertEqual(["acc-bvn1-val1", "acc-bvn1-val2", "acc-bvn1-val3",
                          "acc-bvn2-val1"], topology.containers(self.lop))


class FollowerTest(unittest.TestCase):
    """#4365: one node with the validator's wiring and a key in no committee.

    Every assertion here is a consumer that would otherwise treat it as a
    validator — and each of those is a different way to lose the run. The
    load generator handed a follower's endpoint strands every transaction
    it submits there (a non-committee node proposes no batches), which
    `-max-stranded 20` fails the run for; the chaos loop restarting it
    disturbs a node the run is measuring; the network's height read as the
    max over a set that includes a lagging follower hides nothing, but
    `behind` computed against that same max is always zero.
    """

    def setUp(self):
        self.f = write(WITH_FOLLOWER)
        self.addCleanup(os.unlink, self.f)

    def test_the_follower_is_parsed_and_named_as_one(self):
        fols = topology.followers(self.f)
        self.assertEqual(1, len(fols))
        self.assertEqual("acc-bvn2-fol1", fols[0]["container"])
        self.assertEqual("BVN2", fols[0]["bvn"])
        self.assertEqual("bvn2-3", fols[0]["dir"],
                         "init names directories by position among ALL nodes")
        self.assertEqual(B + 4, fols[0]["port"])
        self.assertEqual(["Directory", "BVN2"], fols[0]["partitions"],
                         "a container runs two nodes, a DN one and a BVN one")

    def test_the_loadgen_and_the_height_read_see_validators_only(self):
        self.assertEqual([B, B + 1, B + 2, B + 3], topology.node_ports(self.f))
        self.assertEqual(topology.validator_ports(self.f),
                         topology.node_ports(self.f))
        self.assertNotIn(B + 4, topology.node_ports(self.f))

    def test_the_port_contract_still_covers_every_node(self):
        """The compose publishes a port for the follower too; the check that
        the two files agree must compare against ALL of them or the follower's
        port reads as an extra the compose invented."""
        self.assertEqual([B, B + 1, B + 2, B + 3, B + 4],
                         topology.all_node_ports(self.f))
        self.assertEqual([B + 4], topology.follower_ports(self.f))

    def test_rosters_name_the_follower_apart(self):
        self.assertEqual(["acc-bvn2-fol1"], topology.follower_containers(self.f))
        self.assertNotIn("acc-bvn2-fol1", topology.validator_containers(self.f))
        self.assertIn("acc-bvn2-fol1", topology.containers(self.f),
                      "per-node memory and disk must still see it")
        self.assertEqual("bvn2-3",
                         topology.container_paths(self.f)["acc-bvn2-fol1"])

    def test_the_validators_keep_the_names_they_had(self):
        """A follower declared last must not renumber anything before it."""
        self.assertEqual(["acc-bvn1-val1", "acc-bvn1-val2",
                          "acc-bvn2-val1", "acc-bvn2-val2"],
                         topology.validator_containers(self.f))

    def test_a_follower_is_never_probed_for_the_network_height(self):
        ports = topology.probe_ports(self.f, limit=10)
        self.assertNotIn(B + 4, ports)
        self.assertEqual(sorted(topology.validator_ports(self.f)), sorted(ports))

    def test_role_is_stated_for_every_node(self):
        roles = [(n["container"], n["role"]) for n in topology.node_records(self.f)]
        self.assertEqual(("acc-bvn2-fol1", "follower"), roles[-1])
        self.assertTrue(all(r == "validator" for _, r in roles[:-1]))


# A compose for WITH_FOLLOWER in which the follower sits behind a profile, the
# way #4364's added follower does. The bootstrap has no profile and is no node;
# the last service has a profile too, and a `volumes:` key after the services
# that must not be read as one.
COMPOSE_LATE_FOLLOWER = '''\
services:
  bootstrap:
    container_name: acc-bootstrap
  bvn1-val1:
    container_name: acc-bvn1-val1
    ports:
      - "26680:26660"
  bvn2-fol1:
    profiles: ["late-follower"]
    build:
      context: ../..
    container_name: acc-bvn2-fol1
  bvn2-val2:
    container_name: acc-bvn2-val2

volumes:
  network-config:
    profiles: ["not-a-service"]
    container_name: acc-bvn1-val2
'''


class StartedTest(unittest.TestCase):
    """#4364: a declared node is not always a started one.

    The added follower is in docker-network.yml, because init writes its key
    and directory, and behind a compose profile, because only the chaos walk
    starts it. `up.sh` counted it among the containers it waits to see
    healthy, and waited forever.
    """

    def setUp(self):
        self.net = write(WITH_FOLLOWER)
        self.compose = write(COMPOSE_LATE_FOLLOWER)
        self.addCleanup(os.unlink, self.net)
        self.addCleanup(os.unlink, self.compose)

    def test_a_service_behind_a_profile_is_not_started(self):
        self.assertEqual({"acc-bvn2-fol1"},
                         topology.profiled_containers(self.compose))

    def test_a_profile_does_not_leak_into_the_next_service(self):
        started = [r["container"] for r in
                   topology.started_records(self.net, self.compose)]
        self.assertIn("acc-bvn2-val2", started)
        self.assertIn("acc-bvn1-val2", started,
                      "a key under `volumes:` was read as a service")

    def test_started_is_every_declared_node_but_the_profiled_one(self):
        self.assertEqual(5, topology.node_count(self.net))
        self.assertEqual(4, topology.started_count(self.net, self.compose))
        self.assertNotIn("acc-bvn2-fol1",
                         [r["container"] for r in
                          topology.started_records(self.net, self.compose)])

    def test_up_sh_waits_for_the_started_nodes(self):
        """The caller, not only the library: test/docker/soak/test_up_wait.py
        runs up.sh against a stub docker; this pins the name it must use."""
        with open(os.path.join(topology.HERE, "up.sh")) as f:
            src = f.read()
        self.assertIn("topology.started_count()", src)
        self.assertNotIn("topology.node_count()", src)


class ProblemsTest(unittest.TestCase):
    """Shapes this harness cannot measure must be refused before a run, not
    mis-measured during one."""

    def test_the_committed_topology_has_no_problems(self):
        self.assertEqual([], topology.problems())

    def test_a_follower_declared_before_a_validator_is_refused(self):
        p = write(FOLLOWER_FIRST)
        self.addCleanup(os.unlink, p)
        probs = topology.problems(p)
        self.assertTrue(probs)
        self.assertIn("bvn1-1", " ".join(probs),
                      "the problem must name the directory that would collide")

    def test_a_node_that_is_a_validator_on_one_partition_is_refused(self):
        p = write(MIXED)
        self.addCleanup(os.unlink, p)
        probs = topology.problems(p)
        self.assertTrue(probs)
        self.assertIn("dnnType", " ".join(probs))

    def test_a_peer_address_that_disagrees_with_the_name_is_refused(self):
        """The container name is a convention of the compose derived from this
        file. peerAddress in this file IS that container's DNS name, so the two
        can be checked against each other instead of assumed equal."""
        p = write(NAME_DRIFT)
        self.addCleanup(os.unlink, p)
        probs = topology.problems(p)
        self.assertTrue(probs)
        self.assertIn("acc-bvn1-node1", " ".join(probs))

    def test_a_clean_follower_topology_has_no_problems(self):
        p = write(WITH_FOLLOWER)
        self.addCleanup(os.unlink, p)
        self.assertEqual([], topology.problems(p))


class ProbeSpreadTest(unittest.TestCase):
    def setUp(self):
        self.two = write(TWO_BVN)
        self.addCleanup(os.unlink, self.two)

    def test_every_bvn_is_represented_before_depth_is_added(self):
        """Five probes all on one BVN would restore the bug they exist to fix.

        Reading a height from several nodes is what stops one halted node from
        reporting a healthy network as stalled. That only works if the probes
        are spread across partitions.
        """
        ports = topology.probe_ports(self.two, limit=2)
        self.assertEqual([B, B + 2], ports,
                         "first node of each BVN before a second of either")

    def test_probe_list_never_exceeds_the_nodes_that_exist(self):
        ports = topology.probe_ports(self.two, limit=10)
        self.assertEqual(sorted(topology.node_ports(self.two)), sorted(ports))
        self.assertEqual(len(set(ports)), len(ports), "no port polled twice")


class ConsistencyTest(unittest.TestCase):
    """The two files that jointly define the topology must agree."""

    def test_the_committed_topology_is_self_consistent(self):
        self.assertIsNone(topology.check_ports_against_compose(),
                          "docker-network.yml and docker-compose.yml disagree")

    def test_a_drifted_compose_is_caught(self):
        """The check has to actually fail when the files disagree."""
        compose = write('    ports:\n      - "%d:26660"\n' % B)
        self.addCleanup(os.unlink, compose)
        # TWO_BVN implies four nodes; the compose publishes one.
        problem = topology.check_ports_against_compose(self.two, compose)
        self.assertIsNotNone(problem)
        self.assertIn(str(B + 3), problem, "the mismatch must name the ports")

    def test_a_compose_missing_the_followers_port_is_caught(self):
        """The follower's port is part of the contract: a compose that forgot
        its service would otherwise look consistent, and the run would report
        the follower as unreachable rather than as never started."""
        f = write(WITH_FOLLOWER)
        self.addCleanup(os.unlink, f)
        compose = write("".join('      - "%d:26660"\n' % (B + i) for i in range(4)))
        self.addCleanup(os.unlink, compose)
        problem = topology.check_ports_against_compose(f, compose)
        self.assertIsNotNone(problem)
        self.assertIn(str(B + 4), problem)

    def setUp(self):
        self.two = write(TWO_BVN)
        self.addCleanup(os.unlink, self.two)


class DeployedTopologyTest(unittest.TestCase):
    """What is actually committed, so a bad edit to the yml fails here.

    This class asserted 2 BVNs, 8 nodes and ports 26660-26667 for months after
    the network went back to 3 BVNs and the base port moved to 26680 (#4158) —
    four red tests nobody ran. It states the shape #4365 runs on, and since
    #4364 the one more node that is declared and not started.
    """

    def test_three_bvns_of_four_validators_and_four_followers(self):
        """One started follower on BVN3 and a late follower on every BVN
        (#4364, #4438). Each follower is last in its BVN, so the BVN2 and
        BVN3 validators sit after BVN1's and BVN2's late followers."""
        self.assertEqual(["Directory", "BVN1", "BVN2", "BVN3"],
                         topology.partitions())
        self.assertEqual({"BVN1": 5, "BVN2": 5, "BVN3": 6},
                         topology.nodes_per_bvn())
        self.assertEqual(16, topology.node_count())
        self.assertEqual(12, len(topology.validator_ports()))
        self.assertEqual([26680, 26681, 26682, 26683, 26685, 26686, 26687, 26688,
                          26690, 26691, 26692, 26693], topology.node_ports())
        self.assertEqual([26684, 26689, 26694, 26695], topology.follower_ports())
        self.assertEqual(list(range(B, B + 16)), topology.all_node_ports())

    def test_up_starts_thirteen_nodes_and_not_the_late_followers(self):
        """The network comes up as it did before #4364: `up.sh` waits for
        these and the bootstrap, and 14 healthy is all there can be."""
        self.assertEqual({"acc-bvn1-fol1", "acc-bvn2-fol1", "acc-bvn3-fol2"},
                         topology.profiled_containers())
        self.assertEqual(13, topology.started_count())
        started = [r["container"] for r in topology.started_records()]
        self.assertEqual(topology.validator_containers() + ["acc-bvn3-fol1"],
                         started)

    def test_the_started_follower_is_bvn3s_fifth_node(self):
        fols = [f for f in topology.followers()
                if f in topology.started_records()]
        self.assertEqual(1, len(fols), "gate 0 runs exactly one follower")
        self.assertEqual("acc-bvn3-fol1", fols[0]["container"])
        self.assertEqual("bvn3-5", fols[0]["dir"])
        self.assertEqual(["Directory", "BVN3"], fols[0]["partitions"])

    def test_the_late_followers_are_each_bvns_last_node(self):
        late = {f["container"]: f for f in topology.late_followers()}
        self.assertEqual({"acc-bvn1-fol1": ("bvn1-5", 26684, ["Directory", "BVN1"]),
                          "acc-bvn2-fol1": ("bvn2-5", 26689, ["Directory", "BVN2"]),
                          "acc-bvn3-fol2": ("bvn3-6", 26695, ["Directory", "BVN3"])},
                         {c: (f["dir"], f["port"], f["partitions"])
                          for c, f in late.items()})
        self.assertIsNone(topology.check_ports_against_compose())
        self.assertEqual([], topology.problems())

    def test_the_late_follower_is_bvn3s_sixth_node_on_the_last_port(self):
        late = topology.followers()[-1]
        self.assertEqual("acc-bvn3-fol2", late["container"])
        self.assertEqual("bvn3-6", late["dir"])
        self.assertEqual(max(topology.all_node_ports()), late["port"])

    def test_the_follower_is_declared_last(self):
        """Host ports are allocated in declaration order, so a follower
        anywhere but last moves a validator's port and silently repoints the
        loadgen and the monitor."""
        self.assertEqual("follower", topology.node_records()[-1]["role"])


if __name__ == "__main__":
    unittest.main()
