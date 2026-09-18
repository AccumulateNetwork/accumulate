# Soak run 20260903T041627Z

**Purpose:** bcdb 12h/500tps. Submit no longer REFUSES work: crossing a pending boundary now seals a batch and keeps the envelope that crossed it, and the batch-store-full rejection is gone (accepting a tx does not grow the store; only sealing does). Replaces the two-watermark seal gate, which blocked sealing when the store was 75% full -- but ~60% of the store is peers' gossiped batches, so peers filling our cache stopped us producing, and sealing is the only path to the commits that drain it. Previous run 20260903T035139Z: BVN2 produced 46,428 for BVN1 while BVN1 received 10,378 with both watermarks frozen and BVN1 producing blocks at 6/s -- healing was pulling the right sequence numbers and its re-submissions were being refused. Also carries the batch pin (a header's deferred vote pins every batch it names; missing-batch asks/distinct went 4.5 -> 1.00 last run), #4189 durable staging, #4201 heal cadence, two bcdb read caches, tally reduced to counters. WATCH: BVN1 received-from-BVN2 must track BVN2's produced-for-BVN1; no ErrBackpressure refusals; asks/distinct near 1.

| field | value |
|---|---|
| started (UTC) | 2026-09-03T04:16:27Z |
| commit | `254dc6d078ef1100d31efcfc38e33d494981d84d` |
| describe | `10k-tps-660-g254dc6d07-dirty` |
| branch | `issue-4189-staging-out-of-account` |
| uncommitted files | 4 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:a630181261c52f7ef7c6222746d14d4214f3584502bd8b396953de07ce6e3fc3` |
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
| memory budget | mem_limit 1536m, GOMEMLIMIT 1200MiB |

Config as run is frozen in `config/`. Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-03T04:19:01Z |
| elapsed | ?h |
| driver exit | 1 (FAILED) |
| dn height | ? -> dnHeight |
| heals | ? -> heals |
| chaos events | 2 |
| monitor samples | 0 |
| seizure | none detected |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 2 |
| read-back probe | Whole run: 6 timed reads, p50 1.7 ms, p95 2.2 ms, p99 2.2 ms, max 2.2 ms (txn read, BVN1, entry 12 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 0  |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.
