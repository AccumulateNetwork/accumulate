# Restart and quorum-loss test: run 20261003T182048Z

The run used lead `84f951d33` with `restart-test.py` (load at 1 tx/s, then steps A to D). The step record is in `chaos.log` and the table is in `restart-test.md`. The run started at 18:20Z and ended at 19:16Z on 2026-10-03.

## Result

Every partition kept producing after every disturbance, except BVN2 while it was deliberately below its quorum. Steps C and D are marked FAILED for one reason only. bvn1-val3's Directory node never recovered from the kill in step B, and each later step requires every node to be ACTIVE.

| step | driver | what happened |
|---|---|---|
| A: graceful whole-network restart | PASSED | All 13 nodes were ACTIVE within 3 s. The partitions made 286 to 292 blocks in the next 5 min. |
| B: kill whole-network restart (`docker rm -f`) | PASSED | All 13 containers were back within 2 s, and the dwell counted about 1,000 blocks per partition (see "Not explained"). bvn1-val3's Directory did not recover (see below). |
| C: one validator's store torn | FAILED only because of B's casualty | `tear-store` cut bvn2-val4's BVN2 ledger RootChain Tail(0) from 8 hashes to 1. The node found the tear (R1, `CheckStore`), refused to resume from its own store, joined from its peers, and was ACTIVE and caught up. |
| D: two of BVN2's four validators lost, then restored | FAILED only because of B's casualty | With val3 and val4 stopped, BVN2 made 0 blocks in 5 min while the other partitions made 257 to 294. bvn2-val1 logged "This partition has lost its quorum … producing=2 quorum=3". After a `docker start` of the two, BVN2 produced again within 11 s and made 295 blocks in the final 5 min. |

## How the restored validators came back (step D)

Both restored validators came back at block 1889, BVN2's last sealed block.

- **bvn2-val4** found too few peers serving to sign an anchor, so it resumed from its own block. A signed anchor proved its root at block 1891.
- **bvn2-val3** joined from its peers at block 1894 and was ACTIVE from there.

No other partition was read, and nothing rolled back. This matches the design in executor.md ("A partition that loses a quorum…").

## The failure: bvn1-val3's Directory after the kill

- **The store's records disagreed.** After `docker rm -f`, the Directory store's `block.json` said 866, while every permanent shard manifest said 865. The dynamic manifests ranged from 547 to 764.
- **The adapter resumed one block short.** It took its version from Shard0000's manifest. The store then refused every seal: "seal: shard 0: block 865 is already closed; now accumulating block 866".
- **The join gave up.** The node logged "Restored consensus position block=723". Its first pull failed on a write, and the join returned: "The join did not complete; this node is not executing". The node stayed BOOTING at 723 for the rest of the run, with its spine stalled at entry 654.

**Fixed in `e01819de4`, which postdates this run:**
- **Adapter:** it resumes at the higher of `block.json` and the manifest.
- **Join:** a failed pull is logged, counted and retried, and no longer ends the join.

Each fix has a test that fails without it. Kill evidence added to BlockchainDB#99: the block record and the manifests are not written as one unit, and the dynamic layer lost blocks.

## Found during the run, and fixed

**A node that resumed never said it was synced.** acc-bvn3-fol1, the follower that starts with the network, resumed from its own block after A and B. A signed anchor proved its root 2 s later, and it ran 0 blocks behind. The dashboard never showed it synced, because only a join's overlap started the 100-block hold. Fixed in `8baa8e7eb`, where the hold now starts at the first proved block. `TestAWholeNetworkRestartResumes` requires it.

## Not explained

- **The step B dwell counted 1,000 blocks per partition in 5 min.** Every other dwell counted about 290, which is 1 block per second. The heights come from the dashboard. A kill that lost recent blocks from the dynamic layer would read low at the start of the dwell and count the catch-up as production. This is unverified.
- **bvn2-val4's join kept polling after it became ACTIVE.** From its tear restart on, its Directory join logged "The join is waiting … the newest anchor number" about once a minute, at about 23 asks per second, even though it was ACTIVE at 18:40:28. The anchor collector keeps asking after the handoff to serve the root watch, and its asks set the join's waiting step again. The polling looks expected. The log line saying the join is waiting is wrong.
- **Captures `wedge-20261003T185731Z` and `delivery-stall-20261003T190135Z`.** They fall inside step D, while BVN2 was deliberately stopped. They are expected and were not read.

## What it proves and what it does not

On a 13-node network over about 1 hour, the run showed the following.

- **Restarts:** a graceful restart and a kill restart of the whole network resume without a consensus stall.
- **Torn store:** a torn store is detected and repaired by a join.
- **Quorum loss:** a partition below its quorum holds its state, says why, and resumes when its validators return, with no other partition involved.

It does not prove the fixes in `e01819de4` and `8baa8e7eb`. Those need the same test re-run on images built from them. It says nothing about long-run behaviour, which takes at least 12 hours.

Bulk logs deleted after the summary: `node-logs-live.txt` (209 MB) and `node-logs.txt` (157 MB).
