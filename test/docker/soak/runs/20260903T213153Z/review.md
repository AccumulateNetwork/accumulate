# Acceptance run #3 — review

Run #2's code (`595c46ec1`: E7 + S0 + D5 + BlockchainDB `dcce242`), **chaos
off**. 12 h / 500 tps, bcdb, 1 s blocks. Stopped by stallkill at 0.33 h: BVN2
stalled 241 s at block 810. Loadgen 559,735 generated, 500 tps until minute 16.

## The first fifteen minutes

At eleven minutes: 500.9 tps, 0 rejected; block production 0.29 s (BVN1) and
0.39 s (BVN2) per 1 s block; heap 280–510 MiB per node; RSS 686 MiB average;
0.9–1.2 cores per node; GC 0.7–1.5 cycles/s at 0.1–0.23 GC cores per node;
staged 1, oldest view 1.3 s; no warnings but the loadgen's deliberate
send-to-void failures.

But RSS was already climbing: 500–580 MiB at five minutes, 700–750 at ten,
930–1,190 at fifteen, on every node including BVN1's. Live heap on bvn2-val1
at 19 minutes (651 MB): `Worker.createBatch` 99 MB, `pubsub Unmarshal` 78,
`immutableCache.put` 56, `UnmarshalHeader` 55, `Envelope.UnmarshalBinaryFrom`
107 cumulative. The batch plane, growing before any warning.

## What ended it — with nothing else wrong

No chaos, the seal fixed, the database terms gone. The sequence, all from
the logs and `mem.csv`:

1. **Batches are one to two transactions.** `BatchTimeout` is 100 ms and a
   partition runs sixteen workers, so at 250 tps per partition each seal
   holds 1–2 transactions: BVN2's execution accounting shows 2.1–2.5 tx per
   batch and 10,000–15,000 batches a minute (C1, #4206).
2. **The store counts, and the count is six seconds.** `MaxStoredBatches` is
   1,000; at ~160 batches a second that is six seconds of traffic, and LRU
   eviction removed batches that headers still to be voted on named.
   `Missing batch for header — deferring vote` per minute: 21:41–21:44 at
   12–24, then 33, 99, 390, 652, 752.
3. **Deferred votes are commit lag, and own batches are unbounded.** At
   21:47:40 the first over-limit warning on BVN2: `ownUncommitted=496
   stored=975 storedBytes=1.4MB` — the count limit reached at 1.4 MB of an
   8 MB byte share (C2, #4207). 216,315 over-limit warnings and 75,854
   evictions in four minutes (C3, #4208).
4. **The storm is the memory growth.** BVN2 fell to 2 s per block, then
   stopped at 810; heap on the BVN2 nodes reached 1,850 MiB with the GC at
   tens of cores in the last minute; RSS max 1,740 MiB.

`captureProvableView` again held views for up to 58 s on BVN2 (S2 follow-up),
small in bytes.

## Against the criteria

| criterion | minutes 3–15 | after |
|---|---|---|
| memory flat | **no** — RSS doubled by minute 15 with no warning | no |
| GC not the workload | yes | no — tens of GC cores at the end |
| CPU flat | yes | — |
| block work bounded | yes | — |
| commits durable | yes | yes |
| consensus memory bounded | **no** — C1/C2 | no |
| logging bounded | yes until 21:47 | **no** — C3 |

## What this run settles

The steady-state question cannot be answered until S3 is closed. Runs #1 and
#2 reached the same storm through a slow store and a chaos restart; run #3
reached it with neither. The mechanism is the batch plane's own: batches
sealed by the clock, stores bounded by count, own batches bounded by
nothing, no refusal. The spec now says what each of those should be
(`docs/spec/consensus.md`, invariants 1–6); C1–C3 are filed from it. Run #4
follows their implementation, still without chaos; chaos returns with #4205.

Files: `mem.csv`, `storage-stats.csv`, `probe-20260903T215057Z` (19 min,
degrading), `wedge-20260903T215203Z`, `probe-20260903T215406Z` (at the stop).
