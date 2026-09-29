# Copyright 2026 The Accumulate Authors
#
# Use of this source code is governed by an MIT-style
# license that can be found in the LICENSE file or at
# https://opensource.org/licenses/MIT.
"""The stall clock runs on the executed height, not the ledger index alone.

Run 20260928T235326Z, attached after a whole-network recreate at 10 tps: every
partition's ledger index sat still for five minutes while each executed about
one block a second (monitor.csv's exec.* columns, from
accumulate_node_executed_block), and stallkill stopped the run as
"stalled 307s: BVN1,BVN2,BVN3,Directory". The ledger index moves only when a
block writes something; an empty block is executed and never written.
"""

import unittest

import soakmon

PARTS = ("Directory", "BVN1", "BVN2", "BVN3")


def scrape(dn, bvn, part):
    """One validator's parsed /metrics, as the node prints it."""
    txt = ('accumulate_node_executed_block{partition="%s"} %d\n'
           'accumulate_node_executed_block{partition="directory"} %d\n'
           % (part, bvn, dn))
    return list(soakmon.parse_prom(txt))


class ExecutedStallTest(unittest.TestCase):
    def setUp(self):
        for d in (soakmon._PROGRESS, soakmon._RATE, soakmon._FIRST):
            d.clear()

    def test_frozen_ledger_with_executing_partitions_is_not_stalled(self):
        """The run's readings: ledger index still, executed height moving."""
        ledger = {"Directory": 14795, "BVN1": 12348, "BVN2": 12498, "BVN3": 12513}
        t0 = 1000.0
        for i in range(0, 310, 5):
            per = {
                "acc-bvn1-val1": scrape(14785 + i, 12342 + i, "bvn1"),
                "acc-bvn2-val1": scrape(14785 + i, 12473 + i, "bvn2"),
                "acc-bvn3-val1": scrape(14785 + i, 12502 + i, "bvn3"),
            }
            ex = soakmon.executed_heights_from(per)
            pg = soakmon.assess_progress(
                soakmon.progress_heights(ledger, ex), t0 + i)
        for p in PARTS:
            self.assertEqual("live", pg[p]["state"], (p, pg[p]))
        self.assertEqual("up", soakmon.overall_status(True, pg))

    def test_executed_height_frozen_is_still_a_stall(self):
        """The fix must not blind the watchdog: nothing executing is a stall."""
        ledger = {"BVN1": 500}
        ex = {"BVN1": 520}
        soakmon.assess_progress(soakmon.progress_heights(ledger, ex), 1000.0)
        pg = soakmon.assess_progress(soakmon.progress_heights(ledger, ex),
                                     1000.0 + soakmon.STALL_SECS + 1)
        self.assertEqual("stalled", pg["BVN1"]["state"])

    def test_restart_dropping_the_reading_is_not_progress(self):
        """A restarted validator answers low, then climbs back to where the
        partition already was. Neither the drop nor the climb is progress."""
        seq = [1000, 214, 1000, 214, 1000]
        for i, h in enumerate(seq):
            pg = soakmon.assess_progress(
                soakmon.progress_heights({"BVN1": None}, {"BVN1": h}),
                1000.0 + i * 5)
        self.assertEqual("stalled", pg["BVN1"]["state"], pg["BVN1"])

    def test_unreadable_both_ways_is_unknown(self):
        pg = soakmon.assess_progress(
            soakmon.progress_heights({"BVN1": None}, {}), 1000.0)
        self.assertEqual("unknown", pg["BVN1"]["state"])


if __name__ == "__main__":
    unittest.main()
