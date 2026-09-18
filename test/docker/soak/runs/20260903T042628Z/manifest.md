# Soak run 20260903T042628Z

**Purpose:** bcdb 12h/500tps. Submit does not refuse work: crossing a pending boundary seals SYNCHRONOUSLY and keeps the envelope that crossed it, bounding pending at one envelope past the boundary. The attempt before this only SIGNALLED the seal -- a non-blocking send that coalesces -- so pending grew unbounded and the network seized in seconds; the attempt after that died on a stale loadgen still holding port 8091, not on code. Also carries the batch pin (missing-batch asks/distinct 4.5 -> 1.00 measured), #4189 durable staging, #4201 heal cadence, two bcdb read caches, tally as counters. WATCH FIRST: seizure. THEN: BVN1 received-from-BVN2 must track BVN2 produced-for-BVN1 (last real run: 46,428 produced vs 10,378 received, both frozen).

| field | value |
|---|---|
| started (UTC) | 2026-09-03T04:26:28Z |
| commit | `dcc371e60bef8fe7f1ff4d4b7aecd8a123d4f2cf` |
| describe | `10k-tps-661-gdcc371e60-dirty` |
| branch | `issue-4189-staging-out-of-account` |
| uncommitted files | 8 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:1351b149dff6ac6eb18eca4c6a27960d954fb4c608d3f10d722de30d9725423b` |
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

## Stopped early by stallkill

- stopped (UTC): 2026-09-03T04:34:52Z
- reason: stalled 244s: BVN2 (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-03T04:34:53Z |
| elapsed | 0.08h |
| driver exit | 143 (FAILED) |
| dn height | 8 -> 113 |
| heals | 0 -> 357 |
| chaos events | 2 |
| monitor samples | 2 |
| seizure | none detected |
| reconcile pulls (#4073) | 11 |
| stalled channels at end | 3 |
| read-back probe | Whole run: 420 timed reads, p50 1.1 ms, p95 2.4 ms, p99 90.8 ms, max 183.9 ms (txn read, Directory, entry 33 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 1 wedge-20260903T043249Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.
