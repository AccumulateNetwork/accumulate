# Soak run 20260904T180918Z

**Purpose:** NO-HEALER CHECK, 30 MIN, CHAOS OFF, on issue-4217-two-store-staging @bac6b320a = E8 + review fixes + the delivered-copy toss fix, WITH THE CONDUCTOR'S SYNTHETIC HEALER DELETED. Nothing is dropped, so the first criterion is heals == 0 (only anchor signature re-sends remain and are not heals). A lost entry can no longer be re-delivered or hidden: it is a gap in its stream, visible as received > delivered on the dashboard's stream table. THIS RUN ANSWERS: with no healer, does every stream end at received == delivered (dispatch is correct), or do gaps appear and where (the #4214 leg, previously masked)? Check #2 (20260904T163512Z) had the healer deliver 5,974 BVN2->BVN1 entries with no known cause. WATCH: stream table received vs delivered per stream; heals total must stay 0; block times; exec_staged_proofs_total{unbound,refused,conflict} ~0; 'Failed to process transaction' must be 0.

| field | value |
|---|---|
| started (UTC) | 2026-09-04T18:09:18Z |
| commit | `bac6b320a8709276e26c4d5cf336cfb85e76dccb` |
| describe | `10k-tps-718-gbac6b320a-dirty` |
| branch | `issue-4217-two-store-staging` |
| uncommitted files | 147 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:9a3a41c3db1198f83f02dedc00e5365bf99cb020d583b6545393240c005d9ec0` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | off |
| target duration | 30m |
| target TPS | 500 |
| storage | BlockchainDB |
| block interval | 1s |
| memory budget | mem_limit 2048MiB, GOMEMLIMIT 1700MiB (effective, from docker inspect) |

Config as run is frozen in `config/`. Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-04T18:32:17Z |
| elapsed | 0.33h |
| driver exit | 0 (clean) |
| dn height | 10 -> 1223 |
| heals | 0 -> 0 |
| chaos events | 1 |
| monitor samples | 54 |
| seizure | SEIZED at 2026-09-04T18:18:17 :: stuck=0 stuckStream= worst=BVN2->BVN1 gap=92 deliv=32952 undeliv=synthetic BVN2->BVN1 undeliv=733 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 4 |
| read-back probe | Whole run: 3444 timed reads, p50 1.8 ms, p95 14.0 ms, p99 41.1 ms, max 116.1 ms (chain read, Directory, entry 754 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 0  |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.
