# Soak run 20260903T035139Z

**Purpose:** bcdb 12h/500tps with the batch-store fixes. THE TWO CHANGES UNDER TEST (#4165): (1) batches a deferred vote is waiting on are PINNED and cannot be evicted -- a header names several batches, one is missing, and the ones we hold were being evicted during the wait, so the next rebroadcast found a different one missing; (2) TWO BOUNDARIES -- at 75% of the store the worker stops SEALING (backpressure to submitters via the pending caps) and eviction stays at 100%, because a store is drained by commits pruning settled batches, not by discarding the work it holds. Previous run 20260902T231641Z died at 23min: BVN2 777 fetches for 172 distinct batches, one asked 29 times, 17 evictions/sec against a 1000-batch store (1.7x turnover per second, sub-second retention), block production stopped, stallkill at 240s. WATCH: Missing-batch asks/distinct should collapse toward 1; skippedPinned should be non-zero; 'Not sealing ... applying backpressure' should appear BEFORE any eviction warning. Also still under test: #4189 durable staging, #4201 heal cadence, two bcdb read caches, tally reduced to counters.

| field | value |
|---|---|
| started (UTC) | 2026-09-03T03:51:39Z |
| commit | `3860a4271eda81a76169a12c75f8eb50abe5ab69` |
| describe | `10k-tps-659-g3860a4271` |
| branch | `issue-4189-staging-out-of-account` |
| uncommitted files | 2 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:40d47681766ce8e9f0ae83352a698a670de194e14d182ddaaa8ef6c2ef8e6002` |
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
