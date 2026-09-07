# E8 check run #2 — review

`issue-4217-two-store-staging` at `bc11896f8` (check #1's code plus the review
fixes), chaos off, 500 tps, 1 s blocks, bcdb. **Ran to its 30-minute deadline
and exited cleanly** — the first run in this series to finish. About nineteen
minutes of load at 499.5 tps, 569,439 transactions, 6 rejected, 7 skipped.

## Block times and memory

| minute | Directory | BVN1 | BVN2 | heap max | heals |
|---|---|---|---|---|---|
| 2 | 0.9 s | 0.9 s | 0.9 s | 235 MiB | 112 |
| 5 | 1.0 | 1.0 | 1.0 | 282 | 134 |
| 10 | 1.0 | 1.0 | 1.0 | 473 | 3,496 |
| 15 | 1.0 | 1.0 | 1.1 | 511 | 4,842 |
| 17 | 1.0 | 1.1 | 1.4 | 516 | 12,994 |

Check #1 at minute 10 had heals 62,646 and heap 1.3 GiB; at 15 the Directory
was stalled. Here the Directory never lagged: its own batch store stayed under
32 KB, its range recoveries totalled a few hundred against several thousand
per node before, and no refusal ever engaged. Heap at the end: 331–579 MiB
per node, RSS 621–769 MiB.

## Streams at the end

| stream | produced | received | delivered |
|---|---|---|---|
| BVN2 → BVN1 | 97,666 | 87,984 | 87,984 |
| BVN1 → BVN2 | 38,856 | 34,903 | 34,903 |
| BVN2 → Directory | 25,907 | 24,912 | 23,868 |
| BVN1 → Directory | 10,714 | 10,457 | 9,736 |

Received equals delivered on both BVN streams: nothing held, nothing wedged.
The difference from produced is the Directory round trip in flight at the
moment load stopped. The Directory streams hold about a thousand entries
collected for anchors still to arrive, as designed.

## Staging did its work

On `bvn1-val1`: 210,420 entries judged proven, 2,741 proofs validated, none
waiting at the end; `unbound`, `refused` and `conflict` all zero.

## What the heals were

1. **A defect of this branch, found here and fixed (`cc8c06366`).** A copy of an
   already-delivered entry that arrived before the anchor it named went through
   collection, which answered "already delivered" as an error, and that error
   failed the whole envelope — the new members beside it included. 11,248
   envelopes failed that way network-wide ("Failed to process transaction:
   sequence N already delivered"), each dropped member a hole for the healer:
   the pull bursts at minutes 7 and 16–17 sit on those failures. Tossed now
   means nothing happens. The simulator reproduces the case.
2. **The healer's own impatience (H8).** Pulls for distinct numbers on streams
   with no backlog: entries one block in flight, asked for at the next
   activation.
3. **A limit of the mirror-chain proven set.** 32,211 entries were judged
   "unproven" because their proof arrived after a later proof had seeded the
   stream's proven set, which cannot extend backwards; they executed on their
   own package proof anyway. Harmless here; the proven-set-by-index work
   (release, extension) is where it is addressed.

## Verdict

E8 holds under load with the review fixes, and the Directory spiral of check
#1 did not recur — the review's source binding and the silent toss of
delivered copies were the difference, not H8 or C6, which are still unbuilt.
A third check with `cc8c06366` will show whether heals fall to the H8 residue
alone. Thirty minutes is a check, not a claim.

Files: `mem.csv`, `storage-stats.csv`, `node-logs-live.txt`, `streams-final.txt`.
