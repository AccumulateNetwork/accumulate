# Soak run 20260903T213153Z

**Purpose:** STEADY-STATE ACCEPTANCE RUN #3 (PLAN.md), CHAOS OFF. Same code as run #2 (20260903T202621Z: issue-4203-bcdb-durable-commit 595c46ec1 + BlockchainDB dcce242). Run #2 held 500.0 tps for 12 minutes with block production 0.38-0.46 s, zero batch-store warnings and every resource flat -- until chaos restarted acc-bvn1-val3 at 20:40:43Z: the restarted node could not rejoin (batch fetch for pruned batches, peerAsks=1407 peerHits=0) and BVN2 stalled within a minute with the S3 batch-store storm. Restart recovery is filed separately; it is a liveness question, not the resource question this run exists to answer. THIS RUN ANSWERS: over 12 hours at 500 tps with no disturbance, are memory and CPU flat (PLAN steady-state table: RSS <1%/h after hour 1, heap <70% GOMEMLIMIT, GC flat, s/block at 1 s, alloc/block flat, stagedCommits<=1, read-probe p99 flat, hour-12 heap top-10 == hour-1)? WATCH: mem.csv heapAllocMiB per node hour over hour; captureProvableView view age (S2 follow-up); GC cores (S4); hourly-* profiles.

| field | value |
|---|---|
| started (UTC) | 2026-09-03T21:31:53Z |
| commit | `595c46ec1f622543fde0ae58786da54e587dd059` |
| describe | `10k-tps-676-g595c46ec1-dirty` |
| branch | `issue-4203-bcdb-durable-commit` |
| uncommitted files | 40 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:7df35330e05a6d26b2ced3d16a2142e6eabf7bbc7230430c4fa2918229e9cad8` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | off |
| target duration | 12h |
| target TPS | 500 |
| storage | BlockchainDB |
| block interval | 1s |
| memory budget | mem_limit 2048MiB, GOMEMLIMIT 1700MiB (effective, from docker inspect) |

Config as run is frozen in `config/`. Results appended below on exit.

## Stopped early by stallkill

- stopped (UTC): 2026-09-03T21:54:41Z
- reason: stalled 241s: BVN2 (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-03T21:54:47Z |
| elapsed | 0.33h |
| driver exit | 143 (FAILED) |
| dn height | 10 -> 1238 |
| heals | 0 -> 17557 |
| chaos events | 1 |
| monitor samples | 38 |
| seizure | SEIZED at 2026-09-03T21:47:06 :: stuck=0 stuckStream= worst=BVN2->BVN1 gap=151 deliv=58526 undeliv=synthetic BVN2->BVN1 undeliv=3067 |
| reconcile pulls (#4073) | 336 |
| stalled channels at end | 3 |
| read-back probe | Whole run: 3737 timed reads, p50 2.2 ms, p95 61.2 ms, p99 184.8 ms, max 461.3 ms (txn read, BVN1, entry 1223 blocks old); 0 failed, 0 timed out (8s), 1 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 1 wedge-20260903T215203Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.
