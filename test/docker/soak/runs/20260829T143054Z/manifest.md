# Soak run 20260829T143054Z

**Purpose:** 4h at 500 tps on BlockchainDB v0.1.1 (#4165, BlockchainDB#50) — relaunch of the aborted 20260829T141614Z: readers share the store lock (RLock in KV2.Get and SegmentStore.Get), block sets packed into one file; adapter with write-through outside the lock (8b3f91951) and MergeLag 64 (6a1533301). Loadgen with pacer fix + both page-mutation race fixes. Read-back probe, 5-min pprof, database-size panel. 8 nodes, chaos OFF, 8 shards, mem_limit 2560m + GOMEMLIMIT 2GiB. Prior run 20260829T060833Z on f5ab54d: BVN2 blocks 3.0 s to block 448 then 3.7-5.2 s as segment.lookup rose 4%-15% of CPU under one exclusive read lock. Watch: BVN2 s/block flat at 3.0 over 4h; bvn2-val1 CPU above one core; read-probe max; perm segment count bounded by the 64-block merge; disk growth.

| field | value |
|---|---|
| started (UTC) | 2026-08-29T14:30:54Z |
| commit | `09ddb0ff3fae890212dc5358e319934b86d21ab7` |
| describe | `10k-tps-597-g09ddb0ff3` |
| branch | `bcdb-storage-backend` |
| uncommitted files | 2 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:5ae156bb39ac20dec761d78d12c481bf7d1066b22b942b57d852a2e60baf1c1b` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | off |
| target duration | 4h |
| target TPS | 500 |
| storage | blockchainDB |
| memory budget | mem_limit 2560m, GOMEMLIMIT 2GiB |

Config as run is frozen in `config/`. Results appended below on exit.
refused at start by the one-soak guard's false positive on its own launcher wrappers (fixed next commit); nothing ran
