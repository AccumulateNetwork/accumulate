# Soak run 20260829T143322Z

**Purpose:** 4h at 500 tps on BlockchainDB v0.1.1 (#4165, BlockchainDB#50) RUN-4H-500: readers share the store lock (RLock in KV2.Get and SegmentStore.Get), block sets packed into one file; adapter with write-through outside the lock (8b3f91951) and MergeLag 64 (6a1533301). Loadgen with pacer fix + both page-mutation race fixes. Read-back probe, 5-min pprof, database-size panel. 8 nodes, chaos OFF, 8 shards, mem_limit 2560m + GOMEMLIMIT 2GiB. Prior run 20260829T060833Z on f5ab54d: BVN2 blocks 3.0 s to block 448 then 3.7-5.2 s as segment.lookup rose 4%-15% of CPU under one exclusive read lock. Watch: BVN2 s/block flat at 3.0 over 4h; bvn2-val1 CPU above one core; read-probe max; perm segment count bounded by the 64-block merge; disk growth.

| field | value |
|---|---|
| started (UTC) | 2026-08-29T14:33:22Z |
| commit | `93a7110a6bfb4e447bb8685688e6777cf6ddb3a6` |
| describe | `10k-tps-600-g93a7110a6` |
| branch | `bcdb-storage-backend` |
| uncommitted files | 4 (see config/uncommitted.patch) |
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

## Result (stopped by hand at 16:05Z, 1.5h)

Driver killed while the network was in a heal storm; no harness verdict.
DN 8 → ~1,200. Loadgen: 1,341,527 sent, 2,812 rejected, 383 tx/s average
(498 in the first 15 min, 150–220 by the end), 34 ADIs. Network torn down.

## Reading

**What v0.1.1 fixed, confirmed:** at block 590 all three partitions were at
3.0 s/block in lockstep — the previous run (f5ab54d) had slowed BVN2 to
3.7 s by block 449. `segment.lookup` was 3.9 % of CPU vs 15 %; permanent-
layer walks 3.2 % of lookups vs 8–9 %; bvn1's BVN database held 186–237
permanent segments instead of ~650 (MergeLag 64 merging); no node was
CPU-bound; RSS 1.7–1.9 GiB under the 2 GiB GOMEMLIMIT throughout.

**What ended it: the maintenance pause, every 128 blocks, growing with the
dynamic layer.** At `version % 128 == 0` the adapter's write-through calls
Compress and MergeBelow; the store holds its lock for the copy; the commit
waits on its own write-through; block production stops on every node at
once. From the nodes' own watchdog (`Partition resumed … stalledFor`):

| block | BVN1 pause | BVN2 pause | bvn1 BVN dyna layer |
|---|---|---|---|
| 400 | 12 s | — | — |
| 528 | 15 s | 12 s | — |
| 656 | 16 s | 13 s | 5.9 GB / 292 segments |
| 784 | 19 s | 18 s | 7.0 GB / 326 |
| 912 | 20 s | 17 s | — |
| 1,040 | 23 s | 32 s | 9.8 GB / 400 |

~2.3 s of pause per GB of dynamic layer, and the layer grew ~2 GB per ten
minutes on the treasury-side engine (database avg 13.8 GB/node at 14 GB/h).
Each pause seeded a synthetic backlog; healing engaged on it (0 → 570 →
6,132 → 26,351 → 54,135 heals from 15:00 to 16:03) and both BVNs slipped to
3.6–3.7 s/block, 27–34 blocks behind the DN. Memory and CPU were never the
limit this time.

**Read-back probe (15,410 reads):** p50 1.7 ms, p95 43 ms, p99 92 ms, max
8,040 ms (the 8 s timeout, during a pause); 528 "failed" — those are the
API's query gate refusing under load (`query capacity exhausted`), not
storage misses; the probe should report them as such. Median flat with age:
0–100 blocks 1.4 ms, 100–1,000 1.6 ms, 1,000–5,000 2.4 ms.

**Loadgen:** submit-bound (29,684 ticks waited for a submitter) because the
API query gates refused its account queries; 500 was never the network's
limit in the clean phase — 498 tx/s achieved for the first 15 minutes.

**For BlockchainDB (#31):** compaction and block-set merging must not hold
the store lock against commits, and the dynamic layer needs incremental
compaction — its write amplification (14 GB/h per node at 500 tx/s) is its
own problem. Store correctness held throughout: 0 conflicts, 0 misroutes.
