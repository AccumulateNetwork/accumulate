# Soak run 20260904T012004Z

**Purpose:** STEADY-STATE ACCEPTANCE RUN #5 (PLAN.md), CHAOS OFF, on issue-4210-batch-plane-c4-c5 = run #4 (S3) plus C4 #4209 (own batches and the peer cache have separate budgets, so a full own store no longer empties the cache the next header needs) and C5 #4210 (re-proposal asks the DAG: a batch a certified header names is never proposed again, so one batch cannot land in two certificates). Run #4 (20260903T222843Z) held 500 tps for 35 min with batches of 40 and stores in budget, then BVN2 stalled on exactly those two defects. THIS RUN ANSWERS: with the batch plane bounded correctly, are memory and CPU flat for 12 h at 500 tps (PLAN table)? WATCH: 'Missing batch for header' must stay flat while a worker refuses; 'Re-proposed uncommitted batches' should be rare; no 'Waiting for batches ... retention-expired'; batch_store_bytes own vs peer; BVN2 s/block vs BVN1 (the open asymmetry); mem.csv heap per node hour over hour.

| field | value |
|---|---|
| started (UTC) | 2026-09-04T01:20:04Z |
| commit | `cf4ea995f3f5bab3ab8d141c07dfe29e217f228c` |
| describe | `10k-tps-682-gcf4ea995f-dirty` |
| branch | `issue-4210-batch-plane-c4-c5` |
| uncommitted files | 85 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:b058dd8053183fa1dac6da8f79c06e143a176fe1c43adbcaa4cad51371b338ce` |
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

- stopped (UTC): 2026-09-04T03:08:09Z
- reason: stalled 242s: Directory (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-04T03:08:09Z |
| elapsed | 1.75h |
| driver exit | 143 (FAILED) |
| dn height | 10 -> ? |
| heals | 0 -> 327732 |
| chaos events | 1 |
| monitor samples | 189 |
| seizure | SEIZED at 2026-09-04T01:34:50 :: stuck=0 stuckStream= worst=BVN2->Directory gap=160 deliv=17042 undeliv=synthetic BVN2->BVN1 undeliv=1615 |
| reconcile pulls (#4073) | 106 |
| stalled channels at end | 0 |
| read-back probe | Whole run: 27371 timed reads, p50 5.6 ms, p95 244.5 ms, p99 845.8 ms, max 3625.4 ms (txn read, Directory, entry 5747 blocks old); 1218 failed, 0 timed out (8s), 79 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 1 wedge-20260904T030542Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.
