# Review: growing memory, growing CPU, falling TPS, stall

Run 20260903T121819Z (bcdb, 1s blocks, 500 tps target, 2048m / GOMEMLIMIT 1700MiB).
Evidence: heap + goroutine profiles at 12:34:48 and 12:36:50, Prometheus scrapes at
both points, docker stats every 5 min, storage stats.json per database, node logs.

## The chain of events

| time | dn height | RSS per node | CPU per node | loadgen tps |
|---|---|---|---|---|
| 12:21 | 19 | 40-49 MiB | 12-23% | ~500 |
| 12:26 | 340 | 0.7-1.05 GiB | 22-193% | |
| 12:31 | ~500 | 1.0-1.64 GiB | 24-128% | |
| 12:36 | 655 | 1.4-1.65 GiB | 49-793% | 194 avg |

Every node reaches the GOMEMLIMIT ceiling in about 10 minutes. On acc-bvn2-val1 at
12:34:48 the Go heap was 1.99 GB against a 1.78 GB limit, the GC ran 7.9 cycles per
second (2825 -> 3793 cycles in 122 s), and the process burned 6.3 cores on average
(process_cpu_seconds 1260 -> 2032 in 122 s). That is the memory-limit GC regime: once
the live heap is at the limit every allocation is paid for with a collection, and
CPU goes to scanning instead of executing. Blocks slow down (BVN2 3.0 s/block, DN
12.5 s/block at the end), the batch store fills with own uncommitted batches, the
LRU evicts peers' batches, votes defer on missing batches, and the partitions stall.

So the question is what makes the live heap grow with height. Two things, one big.

## Finding 1: the BlockLedger index log rewrites its whole head block every block

`indexing.Log.Append` (pkg/database/indexing/log.go) loads the level-0 head block,
appends one entry, and writes the ENTIRE block back. Each entry carries its value
INLINE (`Entry.Value.data`), and the value is `database.BlockLedger`, whose
`Entries` is every chain update in the block (block_end.go:180). The block size is
4096 entries (internal/database/utils.go:41). So until a level-0 block fills at
4096 blocks, the record written on every block is the concatenation of every block
ledger since the last rollover.

Measured with the real code (memory store, 1000 BlockEntries of ~39 bytes per block):

| block | head record written | allocated by one append+commit | cumulative head bytes written |
|---|---|---|---|
| 1 | 0.04 MB | 0.6 MB | 0.04 MB |
| 100 | 3.9 MB | 8.4 MB | 196 MB |
| 300 | 11.7 MB | 66 MB | 1.76 GB |
| 500 | 19.5 MB | 82 MB | 4.87 GB |
| 700 | 27.2 MB | 56 MB | 9.54 GB |

Linear per block, quadratic in total, and it resets only at 4096 blocks, where a
single record would be ~110 MB at this entry size and several hundred MB at the
real entry size (lite-account URLs are longer than the test's).

In the run this is the top line of every heap profile:

| node / capture | `indexing.(*Block).MarshalBinary` in-use | share of heap |
|---|---|---|
| bvn2-val1 12:34 | 776 MB | 41% |
| bvn2-val1 12:36 | 468 MB | 31% |
| bvn1-val1 12:34 | 513 MB | 46% |
| bvn1-val1 12:36 | 285 MB | 30% |

and the top allocation site in our code: 19.9 GB of 197 GB total allocated on
bvn2-val1, all from `closedBlock.Commit -> Batch.Commit -> Account.Commit ->
RecordStore.PutValue`. `bytes.growSlice` (14.8 GB) and `DecodeBytes` (5.4 GB) are
mostly the same record being re-encoded and re-decoded. The per-block cost grows
with height, which is the growing CPU, and the GC pressure it creates is what puts
the heap at the ceiling.

The DN/BVN split in soakmon confirms it: BVN processes held 1.2-1.7 GiB, DN
processes 225-296 MiB. The DN's block ledger has a handful of entries; the BVN's
has thousands.

This is not new code. The log has been the block ledger since #3616 (Oct 2024). It
never mattered because mainnet blocks carry few chain updates. At 500 tps it is the
wall, on any backend.

### Why 776 MB is retained, not just churned

bcdb stages every commit (`d.staged`) and writes through only when no open reader
predates it (`drain`, database.go:504). storage-stats/stats.json at the end shows:

| database | commits | stagedCommits |
|---|---|---|
| every bvnn (8 nodes) | 700 | 18 |
| every dnn (8 nodes) | 600 | 2 |

Earlier bcdb runs (20260902T223532Z, 20260902T231641Z, 20260903T042628Z) show 2-3
on both. Eighteen staged commits on the BVN means eighteen copies of a ~43 MB head
record held in memory, which is the 776 MB. Something held a read view open for
~18 blocks on every BVN node. The goroutine dumps were taken after the stall and
show no long-lived batch holder, so the holder is NOT verified. The prime suspect
is `EventService.didCommitBlock` (internal/api/v3/event.go:68): it opens a batch
synchronously on every committed block and hands it to a goroutine that calls
`LoadBlockLedger` (decodes the same giant head block) and then loads every entry in
the block. That work grows with height and block size, one goroutine per block,
unbounded, and it is BVN-heavy and DN-light, matching 18 vs 2. Verify by dumping
goroutines while blocks are flowing, or by exporting the oldest open view's age.

Also note `getAt` walks the staged list newest-first on every read, so a deep
staging queue slows every read too.

## Finding 2: batch store thrash is downstream, but it amplifies

At 12:35 acc-bvn2-val1 logged per minute:

| message | count |
|---|---|
| WARN Batch store over limit with un-evictable own uncommitted batches (commit is lagging) | 24,875 |
| WARN Evicted batches due to storage limit (LRU) | 6,348 |
| INFO Missing batch for header, deferring vote and fetching | 418 |
| ERROR Reconcile: failed to request missing synthetic (query capacity exhausted) | 223 |

Own uncommitted batches cannot be evicted (skippedOwnUncommitted ~250 per worker,
4 workers), so when the executor lags the store exceeds its 32 MB budget without
bound, peers' batches are evicted, headers cannot be voted on, and they are
fetched again. Stored batches alias the pubsub wire buffer (types.UnmarshalBatch
takes ownership), which is why `pubsub/pb.(*Message).Unmarshal` holds 200-240 MB
in-use: 2 partitions x (32 MB active + 32 MB retained + 32 MB inbound queue) plus
the own-batch overflow. This is bounded by design except for the own-batch part,
and it does not grow with height. It is a symptom of Finding 1, but the over-limit
WARN is emitted on every submit (400/s) and the BVN2 nodes logged 30-50k lines a
minute at the end, which is its own CPU cost.

## Finding 3: allocation churn that scales with load, not height

Per bvn2-val1 alloc_space (197 GB total in 16 minutes, ~200 MB/s):

| site | allocated | note |
|---|---|---|
| `SyntheticMessage.MarshalBinary` | 17.3 GB | 89% via `encoding.Hash`: every hash of a synthetic message re-marshals it including receipts |
| `AnnotatedReceipt.MarshalBinary` | 12.3 GB | inside the above |
| `api/v3/message.Handler.Handle` | 17.6 GB | serving synthetic pulls (`Sequencer.getSynth`) for healing; 39,085 heals in 15 min |
| `merkle.(*State).Copy` | 9.2 GB | 81% from `CopyAsInterface`: every record Get of a chain state deep-copies the state |
| `Batch.UpdateBPT` | 22 GB cum | BPT rewrite per block |

None of these retain memory. They set the GC cadence, so once Finding 1 puts the
heap at the limit they are what the 8 collections a second are paying for.

## Harness findings

- manifest.md says "mem_limit 1536m, GOMEMLIMIT 1200MiB". The containers ran at
  2048m / 1700MiB (docker stats say /2GiB; docker-compose.yml defaults). soak.sh:176
  prints its own defaults instead of what compose used. The record is wrong.
- Memory is sampled every 5 minutes into stats.csv: four points for a 15-minute
  run. soakmon scrapes rss/dn/bvn per node every few seconds but its `history` only
  keeps generated/heals/sProd/aProd. Add rssMiB, heap_alloc, GC rate, and
  stagedCommits to the history so a growth curve exists.
- bcdb rewrites stats.json every 50 commits, so only the last snapshot survives.
  The harness should copy it periodically (it is 15 KB) so stagedCommits and
  deepFallbacks have a time series.
- `accumulate_dagbft_block_production_seconds` and `round_duration_seconds` are
  zero on every node. The one metric that would have shown block time growing is
  not emitting.

## What to do

1. Stop storing block ledgers inline in the index. Give `indexing.Log` a value
   store keyed by entry (Entry holds the key, the value lives at its own record),
   or store `BlockLedger` at `Account(ledger).BlockLedger(index)` and let the log
   index only keys. Append then writes one small block (16 bytes per entry) plus
   one value. This is the fix for growth, GC, and CPU at once, and it is version
   gated like any other record-layout change.
2. Instrument bcdb: export `stagedCommits` and the age of the oldest open view;
   tag views with their caller. Then find the 18-block holder. Bound
   `EventService.loadBlockInfo` (one in flight, drop the batch when nobody is
   subscribed) whatever the answer is.
3. Rate-limit the two batch-store WARNs (once per second per worker, with counts).
4. Cache the hash of a synthetic message instead of re-marshalling it (encoding.Hash),
   and stop deep-copying chain states on Get where the caller does not mutate.
5. Fix the manifest budget line, sample memory every 10-30 s, and get the dagbft
   block-time histograms emitting before the next 12-hour attempt.

## Not verified

- The holder of the 18 stale views (suspect named above, dumps were post-stall).
- The CPU attribution to GC is inferred from GC rate, heap vs limit, and the
  30 s profile of the previous run (scanobject 29%); no CPU profile exists for this run.
- Real BlockEntry size and entries per block; inferred from the alloc ratio
  (19.9 GB in the run vs 9.5 GB in the 1000-entry test), about 1,500-2,000
  entries per BVN block.
