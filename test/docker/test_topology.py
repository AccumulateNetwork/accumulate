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
"""

import os
import tempfile
import unittest

import topology

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
        self.assertEqual([26660, 26661, 26662, 26663],
                         topology.node_ports(self.two))

    def test_container_names_follow_the_compose_convention(self):
        self.assertEqual(
            ["acc-bvn1-val1", "acc-bvn1-val2", "acc-bvn2-val1", "acc-bvn2-val2"],
            topology.containers(self.two))

    def test_container_paths_match_the_compose_working_dirs(self):
        self.assertEqual("bvn2-1",
                         topology.container_paths(self.two)["acc-bvn2-val1"])

    def test_asymmetric_topology_is_not_flattened(self):
        """A BVN deliberately short a validator must not report as uniform."""
        self.assertEqual({"BVN1": 3, "BVN2": 1},
                         topology.nodes_per_bvn(self.lop))
        self.assertEqual(4, topology.node_count(self.lop))
        self.assertEqual(["acc-bvn1-val1", "acc-bvn1-val2", "acc-bvn1-val3",
                          "acc-bvn2-val1"], topology.containers(self.lop))


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
        self.assertEqual([26660, 26662], ports,
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
        compose = write('    ports:\n      - "26660:26660"\n')
        self.addCleanup(os.unlink, compose)
        # TWO_BVN implies four nodes; the compose publishes one.
        problem = topology.check_ports_against_compose(self.two, compose)
        self.assertIsNotNone(problem)
        self.assertIn("26663", problem, "the mismatch must name the ports")

    def setUp(self):
        self.two = write(TWO_BVN)
        self.addCleanup(os.unlink, self.two)


class DeployedTopologyTest(unittest.TestCase):
    """What is actually committed, so a bad edit to the yml fails here."""

    def test_two_bvns_of_four_validators(self):
        self.assertEqual(["Directory", "BVN1", "BVN2"], topology.partitions())
        self.assertEqual({"BVN1": 4, "BVN2": 4}, topology.nodes_per_bvn())
        self.assertEqual(8, topology.node_count())
        self.assertEqual(list(range(26660, 26668)), topology.node_ports())


if __name__ == "__main__":
    unittest.main()
