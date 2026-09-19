#!/usr/bin/env python3
"""Blocks produced is a fact about a PARTITION, not about a node.

#4345 gave accumulate_dagbft_blocks_produced_total a partition label, because
a node runs the Directory and a BVN in one process and the unlabelled counter
was one series summing two chains. Taking the max across nodes of a labelled
counter, as the old code did, silently reports the LARGEST SINGLE PARTITION
and calls it the network's block count.

So the reading is: per partition, the highest count any node reported; then
summed over partitions. A node still exporting the unlabelled series is
counted on its own, which reproduces the old reading for that node.
"""
import os, sys, unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import soakmon


class LifeCountsBlocksPerPartition(unittest.TestCase):
    def test_labelled_counters_are_summed_across_partitions(self):
        rows = [
            ("accumulate_dagbft_blocks_produced_total", {"partition": "Directory"}, 100.0),
            ("accumulate_dagbft_blocks_produced_total", {"partition": "BVN1"}, 90.0),
            ("accumulate_dagbft_blocks_empty_total", {"partition": "Directory"}, 10.0),
            ("accumulate_dagbft_blocks_empty_total", {"partition": "BVN1"}, 7.0),
        ]
        # Two nodes of the same partitions, one a block behind. Every
        # validator produces the same blocks, so the fleet must not multiply
        # them.
        behind = [(n, l, v - 1) for n, l, v in rows]
        life = soakmon.life_from({"a": rows, "b": behind})
        self.assertEqual(life["blocks"], 190)
        self.assertEqual(life["blocksEmpty"], 17)

    def test_an_unlabelled_counter_still_reads_as_it_did(self):
        rows = [("accumulate_dagbft_blocks_produced_total", {}, 687160.0)]
        life = soakmon.life_from({"a": rows, "b": rows})
        self.assertEqual(life["blocks"], 687160)

    def test_per_node_events_still_sum(self):
        rows = [("accumulate_dagbft_certificates_redelivered_total", {}, 5.0)]
        life = soakmon.life_from({"a": rows, "b": rows})
        self.assertEqual(life["redelivered"], 10)


if __name__ == "__main__":
    unittest.main()


class MixedFleetDuringARoll(unittest.TestCase):
    """A roll puts both spellings on the fleet at once, and that is when the
    board is most likely to be believed and most likely to be wrong."""

    def rows(self, *pairs):
        return [("accumulate_dagbft_blocks_produced_total", lab, v) for lab, v in pairs]

    def test_an_unlabelled_node_is_not_added_to_the_labelled_ones(self):
        per = {
            "new": self.rows(({"partition": "Directory"}, 100), ({"partition": "BVN1"}, 90)),
            "old": self.rows(({}, 190)),
        }
        self.assertEqual(soakmon.life_from(per)["blocks"], 190)

    def test_all_labelled_is_unchanged(self):
        per = {"a": self.rows(({"partition": "Directory"}, 100), ({"partition": "BVN1"}, 90))}
        self.assertEqual(soakmon.life_from(per)["blocks"], 190)

    def test_all_unlabelled_is_unchanged(self):
        per = {"a": self.rows(({}, 190)), "b": self.rows(({}, 188))}
        self.assertEqual(soakmon.life_from(per)["blocks"], 190)
