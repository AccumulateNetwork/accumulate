# Soak run 20260902T231641Z

**Purpose:** bcdb soak at 2048m/1700MiB with the two read caches. Carries: #4189 durable unhashed staging (MaxPendingSequenced and its silent refusal deleted), #4201 heal cadence (4 blocks, 2 senders for PULLS only), d8d636b52 (Received answered from staging on read), #4165 write tally reduced to counters (~190MB/node freed -- it was 38% of live heap), and #4165 two read caches: Account(U).Url served from the DYNAMIC layer and synthetic/anchor chain records from PERM, both written through on commit. Previous leveldb run died at 11min (BVN1 stalled 193s) with BVN2 on the GC ceiling; first bcdb run showed GetDeep at 32% of read time, which the URL cache targets. WATCH: recv-deliv must never pin at 4096; heals must not reach six figures against a stalled stream; deepFallbacks should fall.

| field | value |
|---|---|
| started (UTC) | 2026-09-02T23:16:41Z |
| commit | `bf6e3f4b6b8fc64d91898b0d27db094c5adb9801` |
| describe | `10k-tps-657-gbf6e3f4b6-dirty` |
| branch | `issue-4189-staging-out-of-account` |
| uncommitted files | 3 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:ec87d47d54f5627106cfee4ed5cd0133901676eae774e2a03ef06258258f6291` |
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

- stopped (UTC): 2026-09-02T23:40:09Z
- reason: stalled 253s: BVN2 (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-02T23:40:11Z |
| elapsed | 0.34h |
| driver exit | 143 (FAILED) |
| dn height | 14 -> 417 |
| heals | 8 -> 12621 |
| chaos events | 5 |
| monitor samples | 5 |
| seizure | SEIZED at 2026-09-02T23:30:17 :: stuck=0 stuckStream= worst=BVN1->BVN2 gap=261 deliv=22331 undeliv=synthetic BVN2->BVN1 undeliv=2872 |
| reconcile pulls (#4073) | 34 |
| stalled channels at end | 4 |
| read-back probe | Whole run: 3664 timed reads, p50 2.0 ms, p95 146.1 ms, p99 706.6 ms, max 2378.5 ms (txn read, BVN1, entry 232 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 1 wedge-20260902T233744Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.
