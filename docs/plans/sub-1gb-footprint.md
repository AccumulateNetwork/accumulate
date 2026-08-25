# Getting under 1 GB at 1000 TPS sustained

Measured 2026-08-24. Harness: `exp/blockfile-sim` (committed). Workload models
1000 TPS: per transaction ~7 immutable writes (~1.25 KB: transaction,
signatures, status, chain entries) plus 6 BPT node rewrites over a bounded
working set, batched one commit per block, with a read mix of 3 random
historical entries and 3 random BPT nodes per transaction. All four layouts
see byte-identical work.

## The headline

**The footprint is set by configured caches, not by storage layout.** At an
identical 256 MB block cache the append-only layout and LevelDB have the same
RSS (787 MB vs 774 MB). Moving immutable entries out of LevelDB does not free
a single byte on its own.

What it buys is the *ability to shrink the cache*, because LevelDB then indexes
13-byte locators instead of 1.25 KB values — roughly 20x more keys per MB of
cache. That is the lever.

## Measured: cache sweep, 2M transactions (~33 min of 1000 TPS)

| layout | cache | RSS | heap post-GC | write-stall | read (imm) | tx/s |
|---|---|---|---|---|---|---|
| leveldb | 256 MB | 762 MB | 420 MB | 12.0 s | 15.0 µs | 10267 |
| leveldb | 64 MB | 257 MB | 103 MB | 17.4 s | 14.3 µs | 10124 |
| leveldb | 32 MB | 175 MB | 61 MB | 10.8 s | 18.5 µs | 9095 |
| hybrid | 256 MB | 769 MB | 446 MB | 0.16 s | 11.8 µs | 14138 |
| hybrid | 64 MB | 258 MB | 112 MB | 0.043 s | 12.3 µs | 13340 |
| hybrid | 32 MB | 175 MB | 66 MB | 0.035 s | 12.9 µs | 12653 |
| hybrid | 16 MB | 138 MB | 43 MB | 0.386 s | 17.7 µs | 10458 |

"hybrid" = immutable entries in append-only block files, LevelDB holding only
locators plus the same mutable BPT churn.

Two results matter:

1. **Today's 256 MB block cache buys nothing on this workload.** Dropping it to
   64 MB cut RSS from 762 MB to 257 MB and read latency got *better*
   (15.0 → 14.3 µs). We are paying ~500 MB per engine, and there are two
   engines per container, for no measured benefit.
2. **The hybrid tolerates a small cache where plain LevelDB does not.** At
   32 MB both sit at 175 MB RSS, but the hybrid does 12653 tx/s with a 35 ms
   write-stall against 9095 tx/s with a 10.8 s stall — 39% more throughput and
   300x less stall at identical memory. Write amplification drops 4.2x → 2.9x
   and compactions 499/72/382 → 333/61/286.

## Measured: the real backends, head to head

`pkg/database/keyvalue/block` already implements the append-only design —
records appended to block files, an on-disk index tree mapping key hash →
(block, offset, length). Its index is **not** held in RAM: `databaseView.Get`
checks a small in-memory vmap, then falls through to the index files. It is
already selectable as `StorageTypeExpBlockDB`, but LevelDB is the default.

Driving both real backends through the same workload:

| | 50k txs (uncapped) | 2M txs, hard 1 GiB cgroup |
|---|---|---|
| real LevelDB | heap 430 MB, RSS 794 MB, reads 4.8 µs | heap 479 MB, RSS 716 MB, 2919 tx/s, reads 54.7 µs, 1175 MB disk, **completed** |
| real block store | heap 9 MB, RSS 216 MB, reads 2.1 µs | heap 15–26 MB early, **degrades** (see below) |

At small scale the block store is spectacular: **48x less heap** (430 MB → 9 MB)
and 2x faster reads.

**But it does not hold at scale, and the reason is disqualifying as-is.** In the
1 GiB-capped 2M-transaction run the block store's heap climbed (15 → 26 → 207 →
244 MB) and throughput collapsed: it reached block 751 of 2000 in 21 minutes,
where LevelDB finished all 2000 in 11.4 minutes — roughly **16x slower** by the
end, and still decelerating. The run was stopped at 751/2000; it did not
complete, and no claim here rests on it having done so.

Cause: **the block store never reclaims space for overwritten keys.** There is
no compaction or GC anywhere in the package — only vmap's in-memory level
compaction. Every BPT rewrite appends a new record and the superseded one is
kept forever, so the index tree grows without bound, and index lookups walk more
and more on-disk entries. Actual disk (`du`, not apparent size — the store
preallocates sparse index files, so `Size()` overstates it by ~2x) reached
3.2 GB at block 751 against LevelDB's 1175 MB for the whole run.

## The design conclusion

Paul's framing was right and it is sharper than either pure option: LevelDB is
used for two different jobs, and they want different stores.

- **Immutable entries** (transactions, signatures, statuses, chain entries)
  never change. They belong in append-only major block files with a locator in
  the index. No compaction is ever needed *because they are never overwritten* —
  which is precisely the property the block store's missing GC relies on.
- **Mutable data** (BPT nodes, ledgers) is overwritten constantly. It belongs
  in LevelDB, whose compaction is what reclaims superseded versions. Putting it
  in an append-only store is what broke the run above.

So: the hybrid, not the wholesale switch. That is the configuration the sweep
measured, and it is the one that tolerated a 32 MB cache.

## Budget for a 1 GB container

Configured, resident, per dual DN+BVN container (2 engines):

| consumer | today | proposed |
|---|---|---|
| LevelDB block cache (2x) | 512 MB | **64 MB** (2x32) |
| LevelDB write buffers (2x, mem+frozen) | 64–96 MB | 64 MB |
| recordCache (2x64 MB nominal, ~165 MB actual) | ~165 MB | ~80 MB (2x32) |
| DAG, 2000 rounds | ~140 MB | ~140 MB |
| Worker batch stores, active + retained (2x) | 128 MB | 128 MB |
| Worker pending queues (2x) | 20 MB | 20 MB |
| **configured subtotal** | **~1030 MB** | **~496 MB** |

The configured caches alone already exceed 1 GB today. No amount of GC tuning
reaches the target without cutting them; the sweep says cutting them is close to
free.

## Ordered actions

1. **`GOMEMLIMIT` below the container limit.** It was 2750 MiB against a 1536 m
   cgroup — the soft limit could never engage, so the OOM killer was the only
   backstop. Fixed to 1200 MiB (commit `5af7c24d5`). *Nothing else in this list
   is enforceable until this holds.*
2. **Block cache 256 MB → 32 MB per engine.** Measured: −500 MB RSS per engine,
   no read-latency cost at 64 MB and a small one at 32 MB.
3. **`sentVotes` unbounded map.** Written with `votedHeaders`, read only behind
   a `votedHeaders` hit, never deleted — grew with uptime x round rate, per
   partition. Fixed with a regression test (commit `0b67fa055`). A candidate
   explanation for the "drifts upward for the first hours of every run" symptom.
4. **Byte-cap the gossip batch channel.** `gossip.go:131` is
   `make(chan *types.Batch, 1000)` — a *count* cap on a variable-size item at up
   to 500 KiB each, i.e. ~500 MB per partition, ~1 GB per container, unbounded
   in bytes. Same bug class as the batch-store OOM that `5909219dc` fixed by
   byte-capping the store; the queue in front of it was never converted.
5. **Cap gossipsub.** One shared router with stock defaults
   (`p2p/discovery.go:123`): 1 MiB max message, 5-heartbeat message cache
   holding full bodies (~5000 messages resident at 1000 msg/s), 32-RPC outbound
   queue per peer, and `WithFloodPublish(true)` sending to every peer rather
   than the mesh degree. `pb.(*Message).Unmarshal` held 319 MB (16.7%) in the
   production profile. Set `WithMaxMessageSize` to the 500 KiB batch cap and
   shrink `GossipSubHistoryLength`.
6. **Then the hybrid store**, as the structural change: immutable entries to
   block files, BPT staying in LevelDB. It is worth doing for the write-stall
   collapse (12 s → 35 ms) as much as for memory.

Items 1–5 are configuration and bug fixes and get the configured budget to
~496 MB. Item 6 is the design change and is what makes a small cache safe under
load.

## What is NOT yet proven

- No 12-hour run at 1000 TPS with these settings. Every number here is from a
  33-minute-equivalent simulation, not the node.
- The block store's scale behaviour was measured to 751/2000 blocks, not to
  completion.
- The simulation writes identical payload bytes, so snappy compresses LevelDB's
  values better than real distinct values would. This understates LevelDB's
  disk and cache pressure — i.e. it flatters the baseline, so the comparison is
  conservative in the hybrid's favour.
- Block files rely on OS page cache, which is charged to the cgroup but is
  reclaimable; under a hard limit it is evicted rather than OOM-killing. Not
  separately measured.
