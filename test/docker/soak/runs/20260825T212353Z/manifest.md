# Soak run 20260825T212353Z

**Purpose:** SHARD A/B run, arm=1. Fixed 250 tps (the knee found earlier today at 8 nodes), chaos OFF, 30m. ONE variable vs the other arm: ACC_EXECUTION_SHARDS=1. Question: is the 250-tps knee execution-bound? Block accounting now reports sharded/serial/shardsUsed so a null result is attributable — only user transactions shard, so 'no gain' could mean nothing was shardable OR execution was never the bottleneck. Also watching worker_transactions_received_total by worker_id to test #4133 (routeKey computed then discarded, so workerFor(nil,4) is constant and 3 of 4 workers should be idle).

| field | value |
|---|---|
| started (UTC) | 2026-08-25T21:23:53Z |
| commit | `32217c9039f4e0a4e1f1587c45ff98cb3d830312` |
| describe | `10k-tps-532-g32217c903-dirty` |
| branch | `4138-conductor-owns-recovery` |
| uncommitted files | 3 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:74ef69860b5b402d2eef20bf713d4647e4f282564551f70f6ae68e6dea93acff` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | off |
| target duration | 30m |
| target TPS | 250 |

Config as run is frozen in `config/`. Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-08-25T21:48:15Z |
| elapsed | 0.32h |
| driver exit | 0 (clean) |
| dn height | 8 -> 405 |
| heals | 0 -> 0 |
| chaos events | 1 |
| monitor samples | 53 |
| seizure | SEIZED at 2026-08-25T21:43:15 :: stalled stream, undelivered for 20 polls :: stuck=0 stuckStream= worst=BVN1->Directory gap=0 deliv=211 undeliv=synthetic BVN2->BVN1 undeliv=1487 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 4 |
| wedge captures (#4125) | 0  |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
