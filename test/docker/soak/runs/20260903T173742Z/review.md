# Acceptance run #1 — review

12 h / 500 tps, bcdb, 1 s blocks, chaos on, image from `bf62168ac`
(`issue-4203-bcdb-durable-commit`: E7 + S0 + D5). Stopped by stallkill at
0.75 h: BVN1 and BVN2 stalled 242 s. Loadgen: 547,295 generated, 202 tps
average, ~80 tps for most of the run.

## What the run answered

**E7 holds.** `indexing.(*Block).MarshalBinary` is absent from every heap
profile. The largest heap term of the previous run is gone.

**D5 holds.** Every commit reached the store at commit; the bcdb terms in the
heap are `preImages` at 15 MB (17:56) and below the top twenty by the end. The
instrument named the reader on every node within three minutes:
`Sequencer.captureProvableView` (341 of 354 warnings; `Querier.Query` 4,
`Executor.Begin` 7, `Sequence` 1, `Validate` 1). It held versions for up to
141 s, 26–48 overlays. Small in bytes now, but it is by design a view that
outlives blocks and the S2 criterion says none may. Follow-up: capture the
provable view only when a snapshot is about to be pinned.

**S0 holds.** `mem.csv`, `storage-stats.csv`, the block-production and round
histograms, GC CPU, staged commits and view age all recorded. The manifest's
memory line read the effective 2048 MiB / 1700 MiB. The storage-stats rows were
duplicated eight times (every container sees every node's directory); fixed in
soak.sh after the run.

## What it found instead

**S8a — the store's per-block seal is the throughput wall (BlockchainDB#84).**
Block production averaged 0.88 s per 1 s block at 17:56 and 0.96 s at 18:25,
of which serial execution was 65 ms. Every node's block producer, sampled at
17:56, was inside `fsync` in `SegmentStore.seal` (index file, manifest,
directory, live file) — about forty serialized fsyncs across eight shards at
17 ms each on this host — with `KV2.Seal` holding the shard's write lock, so
`KV2.Get` in submit validation waited. Sixteen loadgen submitters at ~190 ms
each is 84 tps; the loadgen sat at 0% CPU. Server-side queries not behind a
seal answered in 1 ms.

**S3 — consensus memory has no back-pressure, so a slow store becomes a stall.**
From 18:17 the batch store went over limit: `ownUncommitted=751`,
`stored=8549` against a limit of 1000, `storedBytes` 19.5 MB against 8 MB,
per worker. 756,948 over-limit warnings and 208,150 LRU evictions in nine
minutes; peers' batches evicted, then "Waiting for batches of committed
certificate", then the stall. Live heap on bvn1-val2 went from 312 MB (17:56)
to 659 MB (18:25): `pubsub/pb.(*Message).Unmarshal` 44 → 162 MB,
`Worker.createBatch` 47 → 72 MB, `UnmarshalHeader` 33 → 49 MB,
`SubmitTransaction` 10 → 34 MB, `BufferPool.Get` 48 MB, `readMessage` 40 MB.
RSS 700 → 1,490 MiB is that live heap plus GC headroom (GC at 0–0.7 cycles/s,
next-GC target ~2 GB, GOMEMLIMIT 1700 MiB never reached).

So memory was not flat, and the reason is no longer the database: it is the
gossip and batch plane filling because commits are slower than the offered
load, and nothing tells the submitter to slow down.

## Against the criteria

| criterion | result |
|---|---|
| memory flat | **no** — live heap doubled 17:56→18:25, all of it batch-store and gossip buffers |
| GC not the workload | yes — 0–0.7 cycles/s, 0.27 GC cores summed |
| CPU flat | n/a — nodes idle at 22–40%; blocks fsync-bound |
| block work bounded | yes for the database (E7); block time 0.88→0.96 s is the seal, not height |
| commits durable | yes — staged is overlays only; every commit sealed at commit |
| reads bounded | not measured at the rate offered |
| consensus memory bounded | **no** — S3 |
| logging bounded | **no** — 757k warnings in nine minutes; S7 |

## Order of work, revised

S8a (store) and S3 (back-pressure) before acceptance run #2. S7 alongside. The
captureProvableView follow-up under S2. Nothing about a 12-hour run can be
claimed from this one; it ran 45 minutes.

Files: `mem.csv`, `storage-stats.csv` (rows ×8, dedupe on node+time),
`probe-20260903T175620Z` (healthy, 19 min), `wedge-20260903T182332Z`,
`probe-20260903T182527Z` (at the stop), `loadgen-stats.json`.
