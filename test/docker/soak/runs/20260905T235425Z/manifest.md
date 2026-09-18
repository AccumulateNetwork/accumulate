# Soak run 20260905T235425Z

**Purpose:** DIAGNOSTIC 3, 7 MIN, CHAOS OFF: run 234434Z showed every node's Directory executor dispatching for its own five anchors and no BVN executor ever delivering a Directory anchor; all eight Directory validators send identical anchors; in run 225751Z fifteen copies of Directory anchor 1 executed in one BVN1 block without reaching the six-signature quorum. This build logs every anchor signature at Info: block, source, seq, running signature count, ready, signer, txid. THIS RUN ANSWERS: how many distinct signatures accumulate per Directory anchor at a BVN, and why the count stops short of the threshold. A diagnostic, not a soak.

| field | value |
|---|---|
| started (UTC) | 2026-09-05T23:54:25Z |
| commit | `0d537c46b5c878c57be467c620a47bbd6bbf40c4` |
| describe | `10k-tps-770-g0d537c46b-dirty` |
| branch | `issue-4193-producer-cache` |
| uncommitted files | 283 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:a7056f2159ee036df017dfee78c0d142bdbd66417bcfcc2c414b5b322eb50508` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | off |
| target duration | 7m |
| target TPS | 500 |
| storage | leveldb |
| block interval | 1s |
| memory budget | mem_limit 2048MiB, GOMEMLIMIT 1700MiB (effective, from docker inspect) |

Config as run is frozen in `config/`. Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-06T00:05:37Z |
| elapsed | 0.12h |
| driver exit | 0 (clean) |
| dn height | 10 -> 467 |
| heals | 0 -> 0 |
| chaos events | 1 |
| monitor samples | 20 |
| seizure | none detected |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 4 |
| read-back probe | Whole run: 552 timed reads, p50 1.9 ms, p95 6.3 ms, p99 17.4 ms, max 57.3 ms (txn read, BVN2, entry 215 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 0  |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.
