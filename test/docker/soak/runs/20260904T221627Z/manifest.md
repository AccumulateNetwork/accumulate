# Soak run 20260904T221627Z

**Purpose:** HISTORY-READ ATTRIBUTION, 40 MIN, CHAOS OFF, on issue-4219-absence-reads: mutable misses no longer walk permanent history (R1); stats.json now has shallowMisses by shape, fallbackWalks, and historyReads {hits, misses, distinct, callers} per shape. THIS RUN ANSWERS: which permanent shapes are still read from history, by whom, and are the same keys re-read (cache pays) or each read once (the reader must carry the record). Also: does removing the mutable walks change BVN2 block time / executor lag vs 20260904T180918Z (same load, same code otherwise + E8 backwards extension). A 40-minute run is a read-pattern measurement, not a stability claim.

| field | value |
|---|---|
| started (UTC) | 2026-09-04T22:16:27Z |
| commit | `42263d7dab003a2df32726192c199f13c0be2f51` |
| describe | `10k-tps-723-g42263d7da-dirty` |
| branch | `issue-4219-absence-reads` |
| uncommitted files | 162 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:99b248894512d5aa82b1834ec0a2d3c451bb4d3fc6e6227289fbab9199d1b230` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | off |
| target duration | 40m |
| target TPS | 500 |
| storage | BlockchainDB |
| block interval | 1s |
| memory budget | mem_limit 2048MiB, GOMEMLIMIT 1700MiB (effective, from docker inspect) |

Config as run is frozen in `config/`. Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-04T23:02:22Z |
| elapsed | 0.7h |
| driver exit | 0 (clean) |
| dn height | 11 -> 2551 |
| heals | 0 -> 0 |
| chaos events | 1 |
| monitor samples | 79 |
| seizure | SEIZED at 2026-09-04T22:31:10 :: stuck=0 stuckStream= worst=BVN2->BVN1 gap=252 deliv=62288 undeliv=synthetic BVN2->BVN1 undeliv=1112 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 4 |
| read-back probe | Whole run: 10350 timed reads, p50 2.1 ms, p95 9.9 ms, p99 30.8 ms, max 307.9 ms (txn read, BVN1, entry 922 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 0  |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.
