# Soak run 20260904T035906Z

**Purpose:** STEADY-STATE ACCEPTANCE RUN #6 (PLAN.md), CHAOS OFF, on issue-4210-batch-plane-c4-c5 = run #5 plus H7 #4213 (reconcile single-flight, stops at its deadline, per-source back-off 1-2-4..64 blocks, one failure line per activation; API timed-out reads at Debug) and C5b (a retired or certified batch is never proposed from the availability queue). Run #5 (20260904T012004Z) held for 1.75 h: parity and near-empty stores for 15 min, CPU plateau ~15 fleet cores from GC (#4211) and store history walks (BlockchainDB#86), then the reconcile storm (9.4M failed requests in 40 min) took the Directory down and BVN2 waited on a batch retired 57 min earlier. THIS RUN ANSWERS: without the storm, does the network hold 12 h, and is memory flat? Healing at zero drops (H6 #4212, H1 #4193) is still open and expected to show as heals and CPU. WATCH: 'Reconcile: failed to request' must stay rare; 'backing off' lines name a failing source; no 'Waiting for batches ... retention-expired'; mem.csv heap per node hour over hour; hourly-* profiles.

| field | value |
|---|---|
| started (UTC) | 2026-09-04T03:59:06Z |
| commit | `eacb4aaa0e2528c61bf026da0f87f394c9937ec3` |
| describe | `10k-tps-685-geacb4aaa0-dirty` |
| branch | `issue-4210-batch-plane-c4-c5` |
| uncommitted files | 102 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:89a6566cf148c5cd6360f18ea31930099d2f1b1357d6bfe84ce315b3a36addef` |
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

- stopped (UTC): 2026-09-04T05:04:37Z
- reason: stalled 240s: BVN1 (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-04T05:04:38Z |
| elapsed | 1.03h |
| driver exit | 143 (FAILED) |
| dn height | 10 -> 2159 |
| heals | 0 -> 587949 |
| chaos events | 1 |
| monitor samples | 113 |
| seizure | SEIZED at 2026-09-04T04:10:44 :: stuck=0 stuckStream= worst=BVN2->Directory gap=64 deliv=11598 undeliv=synthetic BVN2->BVN1 undeliv=2308 |
| reconcile pulls (#4073) | 2247 |
| stalled channels at end | 4 |
| read-back probe | Whole run: 16014 timed reads, p50 3.7 ms, p95 37.8 ms, p99 74.4 ms, max 967.3 ms (txn read, BVN2, entry 1548 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 1 wedge-20260904T050204Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.
