# No-healer run — review

`issue-4217-two-store-staging` at `bac6b320a`: E8 with the review fixes, the
delivered-copy toss fix, and **the conductor's synthetic healer deleted**.
Chaos off, 500 tps, 1 s blocks, bcdb. Ran to its 30-minute deadline and exited
cleanly: about 21 minutes of load at 499 tps.

## The criterion: heals == 0

Nothing was dropped and nothing healed. Zero heal requests, zero envelope
failures, zero refused or unbound proofs, over the whole run.

## Streams at the end of load

| stream | produced | received | delivered | in flight |
|---|---|---|---|---|
| BVN2 → BVN1 | 105,005 | 92,715 | 92,715 | 12,290 |
| BVN1 → BVN2 | 35,924 | 33,107 | 33,107 | 2,817 |
| BVN2 → Directory | 24,949 | 24,019 | 23,870 | 930 |
| BVN1 → Directory | 13,113 | 12,100 | 12,100 | 1,013 |

Received equals delivered everywhere but one stream, where 149 entries were
held for anchors still arriving as load stopped. **Nothing was lost.** The
"undelivered" column is what was produced and not yet received when the load
generator stopped: the round trip in flight.

## Why the tail grew

At minute one the tail on BVN2 → BVN1 was 1,157 entries, about 3 seconds of
traffic; at minute eighteen it was 10,926, about 160 seconds. Two legs
slowed:

- **The Directory anchor's execution at the source.** Measured on
  `bvn2-val1`: the time from the Directory producing block M to BVN2
  executing the anchor for M was 3.1–3.4 s for the first twenty minutes, then
  6.3 s and 16.4 s (max 48 s) in the last ten. A package leaves only after
  that anchor executes.
- **BVN1's blocks.** 1.0 s until minute twelve, 1.1, 1.2, 1.4 s after, with
  BVN1 in step with its own consensus (executed round = voted round), so this
  is the block taking longer, not execution lagging.

## What this settles

- The 5,974 entries the healer "delivered" in check #2 (`20260904T163512Z`)
  were not lost. They were in flight behind a slowing round trip; the old
  healer, with no patience, pulled them, then pulled them again. The loss
  hunted as #4214 was the healer's artifact, amplified by the envelope-failure
  bug fixed in `cc8c06366`.
- With no healer, dispatch delivered everything, in order, through staging.
- The work is now what the second half of a run shows: block time growing
  with height. CPU per node held near one core but fleet GC rose with heaps
  filling their caches, and history lookups walk more bloom filters as
  segments accumulate (BlockchainDB#86). One new term is the executor's own:
  28,220 entries on `bvn1-val1` were judged **unproven** because their proof
  arrived after a later proof had already seeded the stream's proven set,
  which is a chain and cannot extend backwards; each took the slow path —
  refused by the run builder, executed through the envelope loop, held by the
  sequenced layer, re-run next block.

Files: `mem.csv`, `storage-stats.csv`, `node-logs-live.txt`,
`streams-final.txt`, `probe-20260904T182523Z` (CPU + heap at 12 min),
`probe-20260904T183200Z`.
