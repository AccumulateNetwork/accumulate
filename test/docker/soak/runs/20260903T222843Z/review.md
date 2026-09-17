# Acceptance run #4 — review

Run #3's code plus S3 (`64cbb38b4`: one-second seal floor, one worker per
node, byte-only budgets, `SubmitUser` refusal, transition logging). 12 h /
500 tps, bcdb, 1 s blocks, chaos off. Stopped by stallkill at 0.58 h: BVN2
stalled 249 s at block 1238.

## What S3 did

| | run #3 | run #4 |
|---|---|---|
| transactions per batch (BVN2) | 2.1–2.5 | 36–45 |
| batches per minute per node | 10,000–15,000 | ~1,000 |
| over-limit warnings | 216,315 in four minutes | 98 transition lines in 35 minutes |
| LRU eviction summaries | 75,854 in four minutes | 4,279, rate-limited |
| refusal | none existed | engaged on BVN2 at 23:00: `batch_store_refusing=1`, loadgen skipped 34k → 168k in five minutes |
| time to the stall | 16 min | 35 min |

The store stayed within budget while it could, and when own batches filled
the share the partition refused user work and said so once. That is
invariants 1, 4 and 5 working.

## What ended it — two defects in S3 as built, and one open question

**BVN2 fell behind from 22:44.** Blocks per minute on bvn2-val1 (Directory and
BVN2 together): 117 at 22:35, 105 at 22:45, 86 at 22:50, 76 at 22:55, 62 at
23:01, then none. BVN2 at 1.4 s/block became 2.4 s, then stopped. Its CPU
profile at the wedge is the same as BVN1's (syscalls, memmove, sha256,
ed25519), so it was waiting, not working. Consensus rounds ran at the same
237 a minute on both partitions throughout; execution did not lag the DAG
(block ≈ round/2 to the end). What slowed was the commit rate itself.

1. **Own batches starved the peer cache (invariant 3).** Own and peer batches
   share one 32 MB share per worker, and eviction can only ever take peers'.
   As BVN2's own uncommitted bytes grew (11 MB at 22:49, 23 MB at 22:54,
   37 MB at the wedge) the LRU emptied the peer side to zero —
   `absence="evicted-lru (store over limit (0 batches / 33554432 bytes))"` —
   so every header's batches were missing when it arrived: `Missing batch for
   header — deferring vote` went 1–3 a minute to 168 a minute at 22:55. A
   vote deferred is a round slowed, a round slowed is a commit delayed, a
   commit delayed is more own bytes. The refusal bounded the memory; it could
   not unwind the spiral because the peer cache was already gone. **Fix**:
   separate budgets — own batches bounded by refusal against their own share,
   the peer cache with its own LRU budget that own batches cannot consume.
2. **A batch was proposed twice, and retention could not serve the second
   certificate.** `ReproposeAfter` is 15 s; once BVN2's commit latency
   exceeded it, every batch was re-proposed into a later header while the DAG
   had already certified the first (`Re-proposed uncommitted batches` 12 a
   minute from 22:45). The first certificate executed at block 1067–1071 and
   pruned the batch to retention; the second, minutes later, found
   `absence="retention-expired ... 5m55s ago"`: retention is 32 MB of bytes,
   about thirty seconds of traffic at these batch sizes, and 10 minutes of
   age never came into it. `Waiting for batches of committed certificate`
   ×140, attempts in the thousands, and the partition stopped at 23:01.
   **Fix**: re-proposal is for a batch no header has certified; a batch in a
   certified header is never proposed again, whatever the executor has done
   with it. That is a DAG question, not a store one, and it needs a spec
   statement (consensus.md, invariant 7). Retention by bytes is then only for
   lagging peers, as invariant 6 says.
3. **Why BVN2 and not BVN1** is open. The two partitions have identical
   validators, the same rounds per minute and the same CPU profile. BVN2 was
   pulling ~1,000 synthetics a minute from BVN1 (`Requested missing synthetic
   transaction`), and seizewatch flagged the BVN2→BVN1 stream at 22:42, three
   minutes before BVN2's blocks slowed. The healer's re-submissions enter
   through `Submit`, unbounded by design; whether they are what tipped BVN2's
   own store first is the thing to read next, from `wedge-20260903T230356Z`
   and the two probes.

## Against the criteria

| criterion | minutes 3–20 | after |
|---|---|---|
| memory flat | yes — heap 330–660 MiB, RSS 730–920 | own batches bounded, peer cache starved |
| GC not the workload | yes — 0.5 GC cores per node | — |
| CPU flat | yes, 1.7–2.0 cores at 500 tps | — |
| commits durable | yes | yes |
| consensus memory bounded | **yes** — first run where it was | yes, but at the cost of the peer cache |
| logging bounded | **yes** — 98 + 4,279 lines vs 292,000 | yes |

## Harness

Run #3's monitor survived its teardown and held port 8099, so this run's
monitor could not bind for 22 minutes and the watchers read the dead run;
killed by pid at 22:50. soak.sh now kills its own monitor at teardown and
refuses to start while another run's holds the port.

Files: `mem.csv` (from 22:50), `storage-stats.csv`, `wedge-20260903T230356Z`,
`probe-20260903T230600Z`, `loadgen-stats.json`.
