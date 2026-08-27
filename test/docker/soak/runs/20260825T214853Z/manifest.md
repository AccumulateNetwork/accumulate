# Soak run 20260825T214853Z

**Purpose:** SHARD A/B run, arm=8. Fixed 250 tps, chaos OFF, 30m. Paired with the arm=1 run immediately before it: same binary, same genesis recipe, same rate, ONE variable (ACC_EXECUTION_SHARDS). Decisive number is sharded vs serial in the block accounting — only user transactions shard, so if the loadgen's cross-partition mix classifies serial the shard count cannot matter and the lever is #4133 (workerFor(nil,4)=1 constant, proven: worker 1 takes 100% of traffic) or the workload mix, not sharding.

| field | value |
|---|---|
| started (UTC) | 2026-08-25T21:48:53Z |
| commit | `32217c9039f4e0a4e1f1587c45ff98cb3d830312` |
| describe | `10k-tps-532-g32217c903-dirty` |
| branch | `4138-conductor-owns-recovery` |
| uncommitted files | 4 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:6ab4b3a6e5969c9871efbfcade2f5c30bc2f770b65ae6b64b3d98fb8c0f6f804` |
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
| ended (UTC) | 2026-08-25T22:13:12Z |
| elapsed | 0.32h |
| driver exit | 0 (clean) |
| dn height | 8 -> 405 |
| heals | 0 -> 0 |
| chaos events | 1 |
| monitor samples | 53 |
| seizure | SEIZED at 2026-08-25T22:08:13 :: stalled stream, undelivered for 20 polls :: stuck=0 stuckStream= worst=BVN1->Directory gap=0 deliv=214 undeliv=synthetic BVN2->BVN1 undeliv=1204 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 4 |
| wedge captures (#4125) | 0  |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
