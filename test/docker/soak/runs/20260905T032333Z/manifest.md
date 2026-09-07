# Soak run 20260905T032333Z

**Purpose:** NO HISTORY WALKS + STAGING IN MEMORY, 45 MIN, CHAOS OFF, on issue-4193-producer-cache @bff07d2d6: H1 (dispatch + sequencer read the producer cache), D7 (first write reads nothing), D8 (element index written blind except unique chains), R3 (no adapter fallback; one deep reader for referenced pending txs), E10 (staging is memory: no Sequenced/Sighted/StagedProofs/Collected/replica records, no pending status for out-of-order arrivals). THIS RUN ANSWERS: stats.json shallowMisses by shape (expect dynamic-only misses + main-chain ElementIndex), historyReads = deep reads (expect ~0), synthcache misses (expect 0), executor write volume down (no staging records), BVN2 block time and executor lag vs 20260904T221627Z (lag began minute 17; executed tps fell 380->170 by min 45). A 45-minute run is a measurement, not a stability claim.

| field | value |
|---|---|
| started (UTC) | 2026-09-05T03:23:33Z |
| commit | `bff07d2d69e99f213614c1ab2e0851b40ee59c1d` |
| describe | `10k-tps-739-gbff07d2d6-dirty` |
| branch | `issue-4193-producer-cache` |
| uncommitted files | 169 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:00962ca0d4ee53fb7c09f6c073052ae52adb81f9f888993bd3b69a9ab1e2c78b` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | off |
| target duration | 45m |
| target TPS | 500 |
| storage | BlockchainDB |
| block interval | 1s |
| memory budget | mem_limit 2048MiB, GOMEMLIMIT 1700MiB (effective, from docker inspect) |

Config as run is frozen in `config/`. Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-05T04:14:28Z |
| elapsed | 0.78h |
| driver exit | 0 (clean) |
| dn height | 10 -> 2843 |
| heals | 0 -> 0 |
| chaos events | 1 |
| monitor samples | 87 |
| seizure | SEIZED at 2026-09-05T03:28:58 :: stuck=0 stuckStream= worst=BVN1->BVN2 gap=1173 deliv=3352 undeliv=synthetic BVN2->BVN1 undeliv=709 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 4 |
| read-back probe | Whole run: 11820 timed reads, p50 12.4 ms, p95 187.4 ms, p99 480.7 ms, max 4089.8 ms (txn read, Directory, entry 2311 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 0  |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.
