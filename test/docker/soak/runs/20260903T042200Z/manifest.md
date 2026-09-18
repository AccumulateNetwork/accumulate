# Soak run 20260903T042200Z

**Purpose:** bcdb 12h/500tps. Submit does not refuse work: crossing a pending boundary seals SYNCHRONOUSLY and keeps the envelope that crossed it. The previous attempt (254dc6d07) removed the refusals but only signalled the seal -- a non-blocking send that coalesces -- so pending grew without limit and the network seized in seconds. Sealing inline bounds the queue at one envelope past the boundary; it may block the submitter, which is the intended backpressure. Also carries the batch pin (asks/distinct 4.5 -> 1.00), #4189 durable staging, #4201 heal cadence, two bcdb read caches, tally as counters. WATCH FIRST: does it seize again. THEN: BVN1 received-from-BVN2 must track BVN2 produced-for-BVN1.

| field | value |
|---|---|
| started (UTC) | 2026-09-03T04:22:01Z |
| commit | `dcc371e60bef8fe7f1ff4d4b7aecd8a123d4f2cf` |
| describe | `10k-tps-661-gdcc371e60-dirty` |
| branch | `issue-4189-staging-out-of-account` |
| uncommitted files | 6 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:308b4ba690a1f839458d0b7152fd5808f4d8cdeac2e55b62a8c7eafd7c07c2e0` |
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
| ended (UTC) | 2026-09-03T04:24:47Z |
| elapsed | ?h |
| driver exit | 1 (FAILED) |
| dn height | ? -> dnHeight |
| heals | ? -> heals |
| chaos events | 2 |
| monitor samples | 0 |
| seizure | none detected |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 2 |
| read-back probe | Whole run: 6 timed reads, p50 1.4 ms, p95 2.4 ms, p99 2.4 ms, max 2.4 ms (txn read, BVN1, entry 12 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 0  |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.
