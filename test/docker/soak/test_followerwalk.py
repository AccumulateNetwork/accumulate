"""followerwalk's schedule, against a fake clock and fake readings."""

import os
import tempfile
import unittest

import followerwalk as fw

FOLS = [{"service": "bvn%d-fol1" % i, "container": "acc-bvn%d-fol1" % i, "dir": "bvn%d-5" % i} for i in (1, 2, 3)]


class Clock:
    def __init__(self):
        self.t = 1_000_000.0

    def now(self):
        return self.t

    def sleep(self, s):
        self.t += s


def walk(active_after, end_after, behind=None):
    """active_after: container -> seconds after its add it reads ACTIVE (None: never)."""
    clk = Clock()
    rd = tempfile.mkdtemp()
    added = {}
    calls = []

    def run(args, **kw):
        calls.append(args)
        if args[:2] == ["docker", "start"]:
            added[args[2]] = clk.t
        class R: stdout = ""
        return R()

    def state(c):
        a = active_after.get(c)
        ok = a is not None and clk.t - added[c] >= a
        return ok, "bvn=ACTIVE" if ok else "bvn=BOOTING"

    w = fw.Walk(rd, clk.t + end_after, run=run, now=clk.now, sleep=clk.sleep, state=state,
                agreement=lambda c, s: (False, ""), behind=behind or (lambda c: {"BVN": 3}),
                snapshot=lambda p: open(p, "w").write("{}"), unaffected=lambda a, b, c: "unaffected")
    w.run(FOLS)
    with open(os.path.join(rd, "chaos.log")) as f:
        lines = [l.split(" ", 1)[1].strip() for l in f]
    return lines, calls


class TestWalk(unittest.TestCase):
    def test_one_at_a_time_in_turn_with_a_full_hold(self):
        lines, calls = walk({c["container"]: 300 for c in FOLS}, end_after=3 * 3600)
        adds = [l.split()[1] for l in lines if l.startswith("add-follower")]
        self.assertEqual(adds[:4], ["acc-bvn1-fol1", "acc-bvn2-fol1", "acc-bvn3-fol1", "acc-bvn1-fol1"])
        # never two up at once: every add follows the previous one's removal
        up = 0
        for l in lines:
            if l.startswith("add-follower"):
                up += 1
                self.assertEqual(up, 1, "a follower was added while another was up")
            if l.startswith("remove-follower"):
                up -= 1
        held = [l for l in lines if " held ACTIVE " in l]
        self.assertTrue(held and all("held ACTIVE 900s" in l or "held ACTIVE 90" in l for l in held), held)

    def test_a_follower_not_active_by_the_deadline_is_removed_and_the_next_starts(self):
        lines, _ = walk({"acc-bvn1-fol1": None, "acc-bvn2-fol1": 60, "acc-bvn3-fol1": 60}, end_after=3 * 3600)
        i = next(i for i, l in enumerate(lines) if "NEVER ACTIVE" in l)
        self.assertIn("acc-bvn1-fol1", lines[i])
        self.assertIn("removed 18", lines[i])  # 1800s + the 30s window, not the hold
        self.assertTrue(lines[i + 1].startswith("remove-follower acc-bvn1-fol1"))
        self.assertTrue(any(l.startswith("add-follower acc-bvn2-fol1") for l in lines[i:]))

    def test_no_slot_is_started_that_cannot_finish(self):
        lines, _ = walk({c["container"]: 60 for c in FOLS}, end_after=1800)
        self.assertFalse(any(l.startswith("add-follower") for l in lines))

    def test_the_hold_reports_the_worst_behind(self):
        seq = iter([1, 7, 2] + [0] * 500)
        lines, _ = walk({c["container"]: 60 for c in FOLS}, end_after=3000,
                        behind=lambda c: {"BVN1": next(seq)})
        held = next(l for l in lines if " held ACTIVE " in l)
        self.assertIn("BVN1 worst 7", held)


if __name__ == "__main__":
    unittest.main()
