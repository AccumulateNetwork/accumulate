# Soak run 20260903T173742Z

**Purpose:** STEADY-STATE ACCEPTANCE RUN #1 (PLAN.md, 'Steady state - RAM and CPU under load'). 12h/500tps bcdb at 1s blocks on issue-4203-bcdb-durable-commit (bf62168ac), which stacks #4202 E7 (block ledger as a chain on the ledger account: the indexing.Log that was 41-46% of live heap and grew with height is gone), #4204 S0 (mem.csv/storage-stats.csv/hourly profiles/effective memory budget/dagbft histograms/GC CPU export/bcdb staged_commits+oldest_view_age gauges) and #4203 D5 (commits durable at commit; readers isolated by pre-image overlays pruned as they close; view opener named in a warning; event service bounded). Same settings as 20260903T121819Z, which hit GOMEMLIMIT in 10 min and stalled at 0.26h. THIS RUN ANSWERS: is memory flat with E7 and D5 closed? Criteria (PLAN table): RSS growth <1%/h after hour 1; live heap <70% of GOMEMLIMIT; GC cycles/s flat; s/block at 1s and CPU per tx flat hour 1 vs 12; bytes/block not correlated with height; stagedCommits<=1 and oldest view <1 block; read-probe p99 flat, deepFallbacks 0; hour-12 heap profile top-10 == hour-1. Not expected to meet the GC/churn criteria yet (S3-S5 are open). WATCH: mem.csv heapAllocMiB per node; 'A reader is holding an old database version' warnings name the D5 holder; no 'Batch store over limit' storms; BVN2 keeps producing.

| field | value |
|---|---|
| started (UTC) | 2026-09-03T17:37:42Z |
| commit | `bf62168ac3df1ace3c5724e848c023dc6c1fd207` |
| describe | `10k-tps-673-gbf62168ac-dirty` |
| branch | `issue-4203-bcdb-durable-commit` |
| uncommitted files | 23 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:6b96677cadbdf9fc18ce54fd9ec2f59b6a089d71e6b457f20f67c2dd9ca69788` |
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

- stopped (UTC): 2026-09-03T18:26:00Z
- reason: stalled 242s: BVN1,BVN2 (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-03T18:26:00Z |
| elapsed | 0.75h |
| driver exit | 143 (FAILED) |
| dn height | 15 -> 2523 |
| heals | 6 -> 133630 |
| chaos events | 10 |
| monitor samples | 83 |
| seizure | SEIZED at 2026-09-03T17:45:41 :: stuck=0 stuckStream= worst=BVN1->BVN2 gap=77 deliv=6375 undeliv=synthetic BVN2->BVN1 undeliv=2212 |
| reconcile pulls (#4073) | 717 |
| stalled channels at end | 4 |
| read-back probe | Whole run: 9567 timed reads, p50 43.3 ms, p95 647.7 ms, p99 991.0 ms, max 8040.1 ms (chain read, BVN1, entry 1798 blocks old); 13 failed, 13 timed out (8s), 27 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 1 wedge-20260903T182332Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.
