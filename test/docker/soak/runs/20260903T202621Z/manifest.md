# Soak run 20260903T202621Z

**Purpose:** STEADY-STATE ACCEPTANCE RUN #2 (PLAN.md). Same as run #1 (20260903T173742Z: 12h/500tps bcdb 1s blocks chaos, E7+S0+D5 on issue-4203-bcdb-durable-commit) plus BlockchainDB dcce242 (PR 85, closes #84): the per-block seal no longer holds the shard lock across fsync, both layers finish side by side and the eight shards seal concurrently. Run #1 was capped at ~80 user tps by that seal (block production 0.88-0.96 s of every 1 s block, 65 ms of it execution) and then the batch store, with no back-pressure (S3), doubled the live heap and stalled the BVNs at 45 min. THIS RUN ANSWERS: with the seal fixed, does block production drop well under the interval, does the loadgen reach 500 tps, and is memory flat over 12 h? WATCH: accumulate_dagbft_block_production_seconds avg (was 0.9 s); loadgen rate (was 80-200); mem.csv heapAllocMiB flat after hour 1; no 'Batch store over limit' storm (757k in run #1); stagedCommits/view age (captureProvableView held up to 141 s in run #1); hourly-* profiles for the hour-1 vs hour-12 comparison.

| field | value |
|---|---|
| started (UTC) | 2026-09-03T20:26:22Z |
| commit | `595c46ec1f622543fde0ae58786da54e587dd059` |
| describe | `10k-tps-676-g595c46ec1-dirty` |
| branch | `issue-4203-bcdb-durable-commit` |
| uncommitted files | 39 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:7c6c8c7680f064c1d0ccfb5b860ae45f92ac751ea45dce2745b20cc5eacdd5a5` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | on |
| target duration | 12h |
| target TPS | 500 |
| storage | BlockchainDB |
| block interval | 1s |
| memory budget | mem_limit 2048MiB, GOMEMLIMIT 1700MiB (effective, from docker inspect) |

Config as run is frozen in `config/`. Results appended below on exit.

## Stopped early by stallkill

- stopped (UTC): 2026-09-03T20:46:14Z
- reason: stalled 247s: BVN1,BVN2 (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-03T20:46:15Z |
| elapsed | 0.28h |
| driver exit | 143 (FAILED) |
| dn height | 15 -> 1011 |
| heals | 6 -> 32703 |
| chaos events | 4 |
| monitor samples | 32 |
| seizure | SEIZED at 2026-09-03T20:38:07 :: stuck=0 stuckStream= worst=BVN2->BVN1 gap=192 deliv=44850 undeliv=synthetic BVN2->BVN1 undeliv=1011 |
| reconcile pulls (#4073) | 188 |
| stalled channels at end | 4 |
| read-back probe | Whole run: 2514 timed reads, p50 1.7 ms, p95 6.0 ms, p99 21.1 ms, max 162.5 ms (txn read, BVN1, entry 618 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 1 wedge-20260903T204339Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.
