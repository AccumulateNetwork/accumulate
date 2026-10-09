"""readprobe's per-validator states call (plan step 4c)."""

import os
import tempfile
import unittest

import readprobe as rp


class TestStatesRound(unittest.TestCase):
    def test_each_bvn_validator_is_asked_the_same_names_as_of_one_block(self):
        asked = []

        def query(scope, q, url=None):
            if q["queryType"] == "changedAccountStates":
                self.assertEqual(q["after"], 1000 - rp.STATES_BACK)
                ents = [{"account": "acc://a%03d.acme/tokens" % i} for i in range(100)]
                ents += [{"account": scope + "/ledger"}, {"account": scope + "/anchors"}]
                return {"records": [{"entries": ents}], "through": 999}, 1.0, None
            asked.append((scope, url, tuple(q["accounts"]), q["forHeight"]))
            return {"records": []}, 250.0, None

        rd = tempfile.mkdtemp()
        old = rp.query, rp.STATES_CSV
        rp.query, rp.STATES_CSV = query, os.path.join(rd, "states.csv")
        try:
            p = rp.Probe()
            p.heights = {k: 1000 for k in rp.SCOPES}
            p.run_states_round()
        finally:
            rp.query, rp.STATES_CSV = old
        self.assertEqual(len(asked), len(rp.VALIDATORS))
        self.assertEqual({a[1] for a in asked}, {"http://127.0.0.1:%d/v3" % v["port"] for v in rp.VALIDATORS})
        for scope, _, names, at in asked:
            self.assertEqual(len(names), rp.STATES_ACCOUNTS)
            self.assertFalse(any("/ledger" in n or "/anchors" in n for n in names))
            self.assertEqual(at, 999)
        with open(os.path.join(rd, "states.csv")) as f:
            lines = f.read().splitlines()
        self.assertEqual(lines[0], rp.STATES_CSV_HEADER)
        self.assertEqual(len(lines), 1 + len(rp.VALIDATORS))
        self.assertTrue(all(l.endswith(",250,ok") for l in lines[1:]), lines[1:])


if __name__ == "__main__":
    unittest.main()
