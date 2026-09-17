# Review — run 20260905T051008Z

Acceptance run #7, third start: 500 tps, 1 s blocks, chaos off, BlockchainDB,
`issue-4193-producer-cache` @f8e4bbc50 (H1, D7, D8, R3, E10, E9, C6, H8).
Stopped by stallkill at 2.28 h (07:30Z): BVN2 produced no block for 263 s.

## What actually happened

**Every synthetic stream froze at minute 5 of load and never moved again.**

| stream | produced | received | delivered |
|---|---|---|---|
| BVN1 → Directory | 35,560 | 841 | 841 |
| BVN1 → BVN2 | 100,748 | 4,576 | 4,576 |
| BVN2 → Directory | 66,986 | 2,304 | 2,304 |
| BVN2 → BVN1 | 212,248 | 12,219 | 12,219 |

Received equals delivered on every stream: the destinations executed everything
they were given, and were given nothing after 05:15Z. The load generator kept
submitting user transactions (1.67M generated, 204 tps after C6 refusals began),
so the BVNs kept producing synthetics into streams nobody delivered.

**Cause: every Directory anchor was rejected at the BVNs.** 28,507 rejections
in the last hour of logs, on all eight BVN nodes, all with one message:
`receipt 0 is invalid: result does not match the anchor`
(`internal/core/execute/v2/chain/directory_anchor.go:48`). A BVN dispatches a
block's synthetics only when the Directory's receipt for that block comes back
inside a Directory anchor; with the anchors refused, no receipt ever arrived,
`sendSyntheticTransactions` had nothing to send, and the healing requester
could only report "not yet" (the source had dispatched nothing) or, once the
entries aged past the cache horizon, "miss".

**Root cause: a mark-point state read as absent past the window, and the chain
rebuilt itself from nothing.** The Directory builds a receipt for each BVN
anchor from its intermediate anchor chain (one entry per BVN block) into its
root chain. Both are slow chains, and `merkle.Chain.StateAt` reconstructs the
state at an index from the previous mark point (every 256 entries) plus the
hashes since — a mark point written hundreds of blocks ago. R3 (in
`bff07d2d6`) stopped the store adapter from walking permanent history on a
shallow miss, and mark points were routed to the permanent layer, so past the
20-block window the read returned "absent". `StateAt` took "absent" for a
truncated chain and started from an empty state, silently. The receipt was
internally consistent and ended at a root that exists nowhere; the anchor body's
root, read from the chain head, was the real one; the BVN compared the two.
The first mark point of a per-BVN anchor chain lands at entry 255, about four
minutes of BVN blocks; twenty blocks later it is behind the window. Minute 5.

Confirmed three ways: run `20260905T032333Z` (the first on `bff07d2d6`) has the
same 20,496 rejections and the same frozen streams (its review said otherwise;
see its erratum); the two runs before it have zero; and a bisect run
(`20260905T131424Z`, tip with E9 reverted) froze the same way at 240 delivered,
so E9 is not the cause. A unit test now builds a chain over sixty commits on
the store and proves a receipt from early entries validates and ends at the
chain's anchor; it fails on the old routing with the new, loud error.

**Fix** (this branch, after the run):
- Mark-point states are routed to the **dynamic** layer, like account URLs:
  written once, read by every receipt the chain ever builds
  (`pkg/database/keyvalue/bcdb/route.go`; database.md, "Duplicates are caught
  at entry").
- A missing mark point is an **error**, never an empty state
  (`merkle.Chain.StateAt`), and `buildReceipt` no longer ignores that error.

## What the run still says

- **C6 holds** while the network was live: BVN2's execution lag oscillated
  between 1 and 14 blocks in the first half hour, refusal engaging past 8 and
  clearing within twenty seconds. Later readings (lag 168 at the wedge
  capture) are from a BVN2 that was executing a backlog it could not drain
  while the whole network's cross-partition traffic was frozen; not a C6
  finding.
- **The requester asked for nothing it should not have.** With the source
  unable to dispatch, it recorded "not yet" (84 on BVN2) and, once entries aged
  out of the cache, "miss" (52–63). Zero entries pulled. The misses are the
  horizon, not a defect at the source.
- **Memory.** BVN2 RSS 1.5 GB, BVN1 1.1 GB at two hours; BVN2's heap in use
  1.2–1.4 GB. Same asymmetry as #4220, on a frozen network; to be re-read on a
  delivering one.
- **healAnchors reads anchor bodies through a shallow batch** and fails past
  the window: 21,393 `Message.Main not found` errors. It should read the
  producer cache's anchors. Not fixed here.
- **The synthetic cache gauges are unlabelled**, and the Directory's and the
  BVN's caches on one node overwrite each other's value. Not fixed here.

## Not verified

- Whether the fixed build delivers every stream for twelve hours: the next
  run.
- BVN2's memory on a delivering network.
