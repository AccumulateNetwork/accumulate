# Acceptance run #2 — review

Run #1 plus BlockchainDB `dcce242` (PR 85, closes #84: the seal off the lock,
shards concurrent). 12 h / 500 tps, bcdb, 1 s blocks, chaos on. Stopped by
stallkill at 0.28 h: BVN2 stalled at block 716 from 20:41:30, BVN1 at 904
shortly after.

## The seal fix worked

| | run #1 | run #2, minutes 3–14 |
|---|---|---|
| loadgen | 80–200 tps | **500.0 tps, every minute, 0 rejected** |
| block production (`accumulate_dagbft_block_production_seconds`) | 0.88–0.96 s per 1 s block | **0.38 s at 10 min, 0.46 s at 17 min** |
| batch-store over-limit warnings | storm from minute 40 | **none until 20:40** |
| staged commits / oldest view | up to 48 / 141 s | 1 / 1.1 s (BVN2 nodes); 5–10 / up to 34 s where `captureProvableView` held a view |
| heap per node | 320–530 MiB | 270–470 MiB |
| RSS average | ~700 MiB | 650–800 MiB |
| CPU per node | 0.3–0.4 cores idle | 1.0–1.4 cores doing 500 tps |
| GC | 0–0.7 cycles/s | 1.5–2.3 cycles/s, 0.2–2.5 GC cores |

Twelve minutes at the target rate with every resource flat. That is the
first time this network has done that on bcdb.

## What ended it

`chaos.log`: `20:40:43Z restart acc-bvn1-val3`. Then, within a minute:

1. **The restarted node cannot rejoin.** It comes back at round 0 for both its
   partitions ("Header round out of range headerRound=0" 800/min on every
   other node), and waits for the batches of the first certificates it must
   execute: `Waiting for batches of committed certificate absence=no-record
   attempts=201 peerAsks=1407 peerHits=0 round=10`. The peers pruned those
   batches long ago (retention is a window); a node restarted under load has
   nothing to fetch them from. It stays at Directory 731 / BVN1 722 for the
   rest of the run (60 "Partition stalled" lines). This is the restart form
   of the laggard trap in #4162.
2. **BVN2 stalls, and it did not restart.** All four BVN2 validators stayed
   up; BVN2's own round advanced from 1432 to 1433 and stopped. In the same
   minute the batch store on every BVN2 node went over limit —
   `ownUncommitted=987` against 1000, `stored` 5,600–7,200 against 1,000 —
   with 22,000 over-limit warnings a minute per node and LRU evictions of
   peers' batches (195,843 in five minutes). Evicted batches are missing
   batches, missing batches defer votes, deferred votes are no quorum. The
   coupling from the BVN1 restart to BVN2's commit lagging is not
   established here; every container also runs a Directory validator, and the
   Directory absorbed the restarted node's round-0 flood and 140 batch
   requests a second. That is the next thing to read from the wedge capture.
3. BVN1 followed at 904 as its stores filled the same way.

The loadgen kept offering 500 tps into a partition that had stopped
committing, and nothing told it to stop: the S3 mechanism, exactly as the
plan describes it, turning a liveness fault into memory growth (heap 400 →
730–1075 MiB on the seven affected nodes in four minutes).

## Against the criteria

| criterion | minutes 3–14 | after the restart |
|---|---|---|
| memory flat | yes | no — batch store, S3 |
| GC not the workload | yes (0.2–2.5 GC cores across 8 nodes) | — |
| CPU flat | yes | — |
| block work bounded | yes | — |
| commits durable | yes | yes |
| consensus memory bounded | yes while committing | **no** — S3 |
| logging bounded | yes | **no** — 500k warnings in five minutes, S7 |

## Consequences

- The steady-state criteria and chaos are separate questions, and this run
  cannot answer the first while the second fails in minute 14. **Run #3 runs
  with chaos off** to get the 12-hour resource answer; chaos returns once a
  restarted validator can rejoin.
- Filed: restart recovery (accumulate), from the evidence above.
- S3 stands, with sharper evidence: back-pressure at the submitter is what
  keeps a stalled partition from becoming a memory problem.
- `captureProvableView` still holds views for up to 34 s (S2 follow-up).

Files: `mem.csv`, `storage-stats.csv` (deduplicated, 16 rows a tick),
`wedge-20260903T204339Z`, `probe-20260903T204541Z`, `chaos.log`,
`loadgen-stats.json`.
