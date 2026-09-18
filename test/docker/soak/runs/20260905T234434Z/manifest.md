# Soak run 20260905T234434Z

**Purpose:** DIAGNOSTIC 2, 7 MIN, CHAOS OFF: run 233002Z confirmed the dispatch function is never reached with entries on any node while BVN1's requester hears 'the directory has not receipted the block yet' from BVN2. This build logs, at Info: the block finalizer skipping after an empty block while Directory anchors wait for dispatch (block, ledger index, waiting anchors); each dispatch pass (anchors taken, leader); each Directory receipt for an own block (block, directory block); each dispatched block. THIS RUN ANSWERS: where between the executed Directory anchor and MarkDispatched the receipts stop. A diagnostic, not a soak.

| field | value |
|---|---|
| started (UTC) | 2026-09-05T23:44:34Z |
| commit | `0d537c46b5c878c57be467c620a47bbd6bbf40c4` |
| describe | `10k-tps-770-g0d537c46b-dirty` |
| branch | `issue-4193-producer-cache` |
| uncommitted files | 281 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:5b2deb2804abdb7c3a4692c3c2560c049be233ef82bf289f8d5e769da5fd4b22` |
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

## Stopped early by stallkill

- stopped (UTC): 2026-09-05T23:53:07Z
- reason: stalled 245s: Directory (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-05T23:53:08Z |
| elapsed | ?h |
| driver exit | 143 (FAILED) |
| dn height | 38 -> 54 |
| heals | 42 -> 667 |
| chaos events | 1 |
| monitor samples | 13 |
| seizure | none detected |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 1 |
| read-back probe | Whole run: 210 timed reads, p50 0.9 ms, p95 1.8 ms, p99 2.1 ms, max 2.4 ms (txn read, BVN1, entry 269 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 1 wedge-20260905T235034Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.
