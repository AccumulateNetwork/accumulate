#!/usr/bin/env python3
"""The shell watchdogs read /data through embedded Python. They are run here
against a payload shaped like the monitor's, because the seizure watchdog
read an unmeasured `stuck` as a healthy zero and could never trip on it, and
status.sh printed `None` for every healing field (#4279 review)."""
import json, os, re, subprocess, sys, unittest

HERE = os.path.dirname(os.path.abspath(__file__))


def embedded(script, start, end):
    """The Python between two markers of a shell script, exactly as run."""
    s = open(os.path.join(HERE, script)).read()
    i = s.index(start) + len(start)
    return s[i:s.index(end, i)]


def run(code, payload):
    p = subprocess.run([sys.executable, "-c", code], input=json.dumps(payload), capture_output=True, text=True)
    if p.returncode:
        raise AssertionError(p.stderr)
    return p.stdout


SEIZE = embedded("seizewatch.sh", "python3 -c '", "' 2>/dev/null)\"")
STATUS = embedded("status.sh", "python3 -c '", "'\n")


class Seizewatch(unittest.TestCase):
    def payload(self, stuck, gap):
        return {"heals": {"stuck": stuck, "stuckStream": ""},
                "matrix": {"flows": {"synthetic": {"BVN1": {"Directory": {
                    "sent": 33846, "recv": 32469 + gap, "deliv": 32469}}}}}}

    def test_an_unmeasured_stuck_count_is_not_zero(self):
        out = run(SEIZE, self.payload(None, 9))
        self.assertIn("stuck=n/a", out)
        self.assertNotIn("stuck=0", out)
        # The shell trips on `stuck=[0-9]+`; n/a does not match, so it
        # falls back to 0 for the comparison only.
        self.assertIsNone(re.search(r"stuck=\d+", out))

    def test_a_measured_stuck_count_is_printed(self):
        self.assertIn("stuck=7", run(SEIZE, self.payload(7, 0)))

    def test_the_gap_and_undelivered_signals_survive(self):
        out = run(SEIZE, self.payload(None, 9))
        self.assertIn("worst=BVN1->Directory gap=9", out)
        self.assertIn("undeliv=synthetic BVN1->Directory undeliv=1368", out)


class Status(unittest.TestCase):
    def test_reads_the_families_the_monitor_serves(self):
        d = {"started": 0, "heights": {}, "chaos": {"counts": {}}, "wedges": {"total": 5, "byReason": {"queue-full": 5}, "byDest": {"BVN1": 5}},
             "heals": {"entries": 18171, "requests": {"answered": 129, "not-yet": 1515, "miss": 7, "failed": 0},
                       "held": {"entries": 33, "bytes": 5363}},
             "matrix": {"flows": {"synthetic": {}}}}
        out = run(STATUS, d)
        self.assertIn("entries=18171", out)
        self.assertIn("answered=129", out)
        self.assertIn("held        33 entries, 5363 bytes", out)
        self.assertNotIn("None", out)

    def test_absent_is_said_not_printed_as_none(self):
        d = {"started": 0, "heights": {}, "chaos": {"counts": {}}, "wedges": {"total": None, "byReason": {}, "byDest": {}},
             "heals": {"entries": None, "requests": None, "held": None}, "matrix": {"flows": {}}}
        out = run(STATUS, d)
        self.assertIn("not measured", out)
        self.assertNotIn("None", out)


WEDGE = embedded("wedgewatch.sh", "python3 -c '", "' \"$WEDGE_SECS\" 2>/dev/null)\"")


def classify(progress, wedge_secs=120):
    """wedgewatch's reading of one /data payload: worst, names, blocks,
    empties, the capture's kind, and the reason written beside it."""
    p = subprocess.run([sys.executable, "-c", WEDGE, str(wedge_secs)],
                       input=json.dumps({"progress": progress,
                                         "life": {"blocks": 5916, "blocksEmpty": 175}}),
                       capture_output=True, text=True)
    if p.returncode:
        raise AssertionError(p.stderr)
    worst, names, blocks, empties, kind, why = p.stdout.strip().split(" ", 5)
    return int(worst), names, kind, why


class WedgeOrDeliveryStall(unittest.TestCase):
    """#4414. Run 20260924T074702Z's capture `wedge-20260924T081419Z` read
    `partitions stalled 121s: BVN2,BVN3,Directory`, and every one of those
    was stalled by DELIVERY (#4285): the partitions kept closing blocks —
    BVN2 slowed to about 40 a minute — while a synthetic flow into each was
    red. A wedge is blocks stopping; the capture's name said one thing and
    the network did the other, and the manifest counted it as a wedge."""

    AT_THE_CAPTURE = {
        # soakmon-at-wedge.json of that capture, with the height clock the
        # monitor now keeps beside the delivery clock
        "Directory": {"height": 1487, "state": "stalled", "stalledFor": 121.7,
                      "stalledBy": "delivery", "blocksStalledFor": 2.0},
        "BVN1": {"height": 1527, "state": "live", "stalledFor": 0.0,
                 "blocksStalledFor": 0.0},
        "BVN2": {"height": 1507, "state": "stalled", "stalledFor": 116.0,
                 "stalledBy": "delivery", "blocksStalledFor": 3.0},
        "BVN3": {"height": 1531, "state": "stalled", "stalledFor": 116.0,
                 "stalledBy": "delivery", "blocksStalledFor": 1.0},
    }

    def test_every_partition_closing_blocks_is_a_delivery_stall(self):
        worst, names, kind, why = classify(self.AT_THE_CAPTURE)
        self.assertEqual(121, worst)
        self.assertEqual("BVN2,BVN3,Directory", names)
        self.assertEqual("delivery-stall", kind)
        self.assertIn("delivery stalled 121s into BVN2,BVN3,Directory", why)
        self.assertIn("every partition closed a block within 120s", why)
        self.assertNotIn("wedge", why)

    def test_a_partition_that_closed_no_block_is_a_wedge(self):
        pg = json.loads(json.dumps(self.AT_THE_CAPTURE))
        pg["Directory"].update(stalledBy="blocks", blocksStalledFor=121.7)
        worst, names, kind, why = classify(pg)
        self.assertEqual("wedge", kind)
        self.assertIn("no block for 121s on Directory", why)

    def test_a_partition_with_no_height_clock_is_not_proven_to_be_closing_blocks(self):
        """An unreadable height, or a monitor that predates the clock, cannot
        say the partition kept closing blocks: the capture keeps the name
        that asks for the closer look."""
        pg = json.loads(json.dumps(self.AT_THE_CAPTURE))
        del pg["BVN1"]["blocksStalledFor"]
        self.assertEqual("wedge", classify(pg)[2])

    def test_the_capture_is_named_by_its_kind(self):
        with open(os.path.join(HERE, "wedgewatch.sh")) as f:
            self.assertIn('capture "$why" "$kind"', f.read())

    def test_the_manifest_counts_them_apart(self):
        with open(os.path.join(HERE, "soak.sh")) as f:
            src = f.read()
        self.assertIn('ls -d "$rd"/wedge-*', src)
        self.assertIn('ls -d "$rd"/delivery-stall-*', src)


class TheMonitorKeepsTheHeightClock(unittest.TestCase):
    """The delivery stall overwrote `stalledFor` with the delivery clock, so
    from /data alone nobody could tell whether blocks had stopped too."""

    def setUp(self):
        sys.path.insert(0, HERE)
        import soakmon
        self.m = soakmon
        for d in (soakmon._PROGRESS, soakmon._RATE, soakmon._FIRST,
                  soakmon._DELIVERY_RED):
            d.clear()

    def flows(self):
        return {"synthetic": {"BVN2": {"BVN3": {"sent": 100, "state": "red"}}}}

    def test_a_delivery_stall_over_moving_heights_is_named_so(self):
        m, t0 = self.m, 1000.0
        m.stalled_by_delivery(self.flows(), t0)
        for i in range(0, 130, 10):
            pg = m.assess_progress({"BVN3": 100 + i, "BVN2": 100 + i}, t0 + i)
            m.apply_delivery_stall(pg, m.stalled_by_delivery(self.flows(), t0 + i))
        self.assertEqual("stalled", pg["BVN3"]["state"])
        self.assertEqual("delivery", pg["BVN3"]["stalledBy"])
        self.assertEqual(0.0, pg["BVN3"]["blocksStalledFor"])
        self.assertEqual("delivery-stall", classify(pg)[2])

    def test_blocks_stopping_under_a_red_flow_is_a_wedge(self):
        m, t0 = self.m, 1000.0
        m.stalled_by_delivery(self.flows(), t0)
        for i in range(0, 130, 10):
            pg = m.assess_progress({"BVN3": 100, "BVN2": 100 + i}, t0 + i)
            m.apply_delivery_stall(pg, m.stalled_by_delivery(self.flows(), t0 + i))
        self.assertEqual("blocks", pg["BVN3"]["stalledBy"])
        self.assertEqual(120.0, pg["BVN3"]["blocksStalledFor"])
        self.assertEqual("wedge", classify(pg)[2])


if __name__ == "__main__":
    unittest.main()
