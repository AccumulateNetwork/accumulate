# Soak run 20260829T003712Z

**Purpose:** STORAGE A/B, arm=blockchainDB (#4165). 8-node 2-BVN compose, chaos OFF, 50 tps, ACC_EXECUTION_SHARDS=8, 30m. Paired with the other arm run immediately before/after: same binary (b18570d54), same genesis recipe, same rate, ONE variable (ACC_STORAGE). Compare block interval, CPU, RSS over time; for blockchainDB also stats.json permWalkPct and perm segment count. 30 minutes is a RELATIVE reading, not a claim about long-run degradation (BlockchainDB#30/#31 bite with segment count).

| field | value |
|---|---|
| started (UTC) | 2026-08-29T00:37:12Z |
| commit | `a0d734cc6856b7a4d7308a87757e29c48c347e1e` |
| describe | `10k-tps-579-ga0d734cc6-dirty` |
| branch | `bcdb-storage-backend` |
| uncommitted files | 3 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:34abff569a2cdd3681217067ee3966f219c26ef82f4427a95e80b486c1d3b29c` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | off |
| target duration | 30m |
| target TPS | 50 |
| storage | blockchainDB |

Config as run is frozen in `config/`. Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-08-29T00:59:05Z |
| elapsed | 0.32h |
| driver exit | 0 (clean) |
| dn height | 8 -> 411 |
| heals | 0 -> 8 |
| chaos events | 1 |
| monitor samples | 54 |
| seizure | SEIZED at 2026-08-29T00:54:05 :: stalled stream, undelivered for 20 polls :: stuck=0 stuckStream= worst=BVN1->Directory gap=0 deliv=779 undeliv=synthetic BVN2->BVN1 undeliv=239 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 4 |
| wedge captures (#4125) | 0  |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
