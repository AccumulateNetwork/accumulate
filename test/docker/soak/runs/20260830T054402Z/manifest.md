# Soak run 20260830T054402Z

**Purpose:** 4h at 500 tps on BlockchainDB eea0b86 (main after #58 / issue #57 two-tier locking) — acceptance re-run of 20260829T143322Z (v0.1.1) on THIS machine, after the thelio attempt (20260830T044359Z) was confounded: host load 54 shared with a mainnet follower, one node OOM-killed at the 2560m cgroup limit at 30 min, no heap profile captured. Same adapter tip (e2e387eaa), same budget (mem_limit 2560m + GOMEMLIMIT 2GiB), chaos OFF, 8 shards; ONE variable vs the v0.1.1 run: the store. Store window N=128 (default) with the adapter unchanged, so MergeBelow(version-64) merges only what has left the window (accumulate#4177): merges begin after block ~256 and lag up to 256 blocks; per-shard segment count plateaus higher than v0.1.1, not grows. A sidecar captures heap + 30 s CPU profiles from every node every 5 min into profiles/ so an OOM can be attributed to the store or the node. Watch: NO block-time spikes at Compress/MergeBelow points (the 12-32 s pauses), s/block flat at 3.0 across partitions, read-probe max bounded (thelio run: 390 ms vs 8 s on v0.1.1), RSS vs 2 GiB, no heal storm, conflicts/misroutes 0. SOAK_FORCE=1: the one-soak guard counts its own fork (idle host verified).

| field | value |
|---|---|
| started (UTC) | 2026-08-30T05:44:02Z |
| commit | `e2e387eaa614abc082335ddd3c64511bd646236a` |
| describe | `10k-tps-602-ge2e387eaa-dirty` |
| branch | `bcdb-storage-backend` |
| uncommitted files | 5 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:78e285a3536240ad6ee06880d23827705baee90fbae3e2eaa15b945dfe693546` |
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

## Stopped early by stallkill

- stopped (UTC): 2026-08-30T06:29:22Z
- reason: stalled 240s: BVN2,Directory (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result (stopped by hand at 06:30Z, 0.77h)

Driver sent SIGTERM while BVN2 was wedged; no harness verdict. DN 8 → 773,
BVN1 726, **BVN2 stuck at 528–537 from 06:14Z**. Loadgen 753,606 sent,
3,219 rejected, 429 tx/s average (460 in the first 25 min). Read-back
probe, whole run: 11,832 reads, p50 1.7 ms, p95 113 ms, p99 325 ms,
**max 8,041 ms** (txn read, BVN2, entry 533 blocks old); 927 failed —
the max and the failures are all from BVN2 after it wedged.

## Reading

**Up to block ~500 the #57 fix held:** DN at 3.0 s/block in every 5-min
sample (114 → 215 → 315 → 416 → 516), no pause at the 128-commit
Compress points where v0.1.1 paused 12 s at block 400; permanent-layer
walks 3.4% of lookups (v0.1.1: 3.2%); conflicts/misroutes 0.

**Then BVN2 died of memory, and it is the store.** At commit 512 all four
BVN2 nodes went to 2.37–2.48 GiB of 2.5 GiB and 320–415% CPU; a 15 s CPU
profile on bvn2-val1 was 76% garbage collector. Heap in use 2.94 GB, of
which **`SegmentStore.mergeInputs` 1.13 GB + `writeMergedSegment` 0.88 GB**
— one `CompactHistory` call, in flight via commit → writeThrough →
Compress, confirmed in the goroutine dump. Dyna layer on that node 5.3 GB,
66 segments. Five minutes earlier the store held ~60 MB of a 904 MB heap.
Compaction materialises every key of its run in two maps; before #58 it
did the same under the lock (the node stopped allocating around it), now
it runs concurrently and the GC cannot keep up under GOMEMLIMIT.
Filed BlockchainDB#59: stream the merge over the sorted indexes.

The adapter's `tally` was the largest non-store allocation (0.30 GB,
comment says ~128 MB). Not the cause here; worth its own look.

Profiles: `profiles/` has heap + 30 s CPU from every node every 5 min
(8 snapshots); the wedge-time goroutine/heap/CPU capture from bvn2-val1
and bvn2-val3 is in the session scratchpad.

## Reading (added 2026-08-30 by the other session's review)

Run by the owner from another session as the acceptance test for
BlockchainDB#57/#58 (`eea0b86`). stallkill stopped it at 06:29Z (45 min):
BVN2 stalled 127 s. Rate 500 → 349 average; 49,395 rejected (backpressure);
heals 30k; 910 "Partition stalled" lines.

**Faster degradation than v0.1.1, and the wedge capture says why.**
`wedge-20260830T061712Z/acc-bvn2-val1.goroutines.txt`: the wedged goroutine
is `blockProductionLoop → ProduceBlock → bcdb.commit → drain → writeThrough →
KV2.Compress → CompactHistory`. #58 took the store lock off maintenance so
that other readers and writers proceed during the copy — but the adapter
called Compress/MergeBelow on the committing goroutine, which is the block
producer's, and waited for them. So block production still stopped for the
whole compaction, and with #58's history compaction rewriting a larger run
the pauses got LONGER: BVN2 19–30 s at block 272, BVN1 26–31 s at 402,
**BVN2 88–100 s at block 400**.

Adapter fix: maintenance runs in a background goroutine (accumulate
bcdb-storage-backend, "run history maintenance in the background"). The
store's own read-path gains held: read probe p50 ~1.5 ms between pauses.
