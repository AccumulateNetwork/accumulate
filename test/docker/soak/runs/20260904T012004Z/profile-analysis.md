# Run #5 — what the profiles say about memory and CPU

Node `acc-bvn2-val1`, three captures: 18 minutes (`probe-20260904T013858Z`),
1 hour (`hourly-20260904T022250Z`), and the wedge at 1 h 45 (`wedge-20260904T030542Z`).
Each has a heap profile (in-use and allocation since start) and a 30 s CPU profile.

## Memory: what is live

| in-use, MB | 18 min | 1 h | wedge | what it is |
|---|---|---|---|---|
| total | 282 | 420 | 625 | |
| `pubsub/pb.(*Message).Unmarshal` | 61 | 76 | 60 | peers' batches, aliasing the wire buffer (bounded, S3) |
| `types.UnmarshalHeader` + `UnmarshalCertificate` | 51 | 62 | 70 | the DAG's headers and certificates within GC depth |
| `bcdb.(*immutableCache).put` | 54 | 42 | 45 | the two bcdb caches, 200,000 entries × 2 generations, count-bounded (S5) |
| `Worker.Submit` (pending copies) | 6 | 49 | 71 | transactions accepted and not yet sealed — grows as blocks slow |
| `Worker.createBatch` | — | 18 | 31 | own uncommitted batches (bounded by refusal) |
| `bcdb.preImages` → `SegmentStore.findActive` | — | — | **217** | D5's pre-image overlays: 243 commits' worth held for open readers |
| `primary.getParentCertsForRound` | 4 | 15 | 14 | |

Three observations.

1. **Steady state, the live heap is ~300 MB and it is the consensus plane plus the
   two caches.** Nothing there grows with height. The batch plane is bounded by
   S3; the DAG by GC depth; the caches by count (which is S5: a count, not bytes).
2. **The two terms that grew are both "blocks got slow".** Pending transaction
   copies (6 → 71 MB) and own batches grow while commits lag; they shrink when
   blocks return to one a second.
3. **At the wedge the largest object is the D5 overlays, 217 MB.** `staged` was
   234–248 on every BVN2 node with the oldest view 34–315 s old. The holders,
   from the warning: `Sequencer.captureProvableView` (788 lines) and — new —
   `Executor.Begin` (372): the block's own batch, open for as long as the block
   takes, which during the storm was minutes. Pre-images are small per commit
   (~0.9 MB) but 243 of them are 217 MB. This is the S2 follow-up (capture the
   provable view only when a snapshot is about to be pinned) plus a fact about
   slow blocks: a block that takes minutes pins minutes of commits.

So memory is not growing with height. It grows with **latency**: everything
that holds a transaction, a batch or a pre-image until a block commits scales
with how long the block takes. Keep blocks at one second and the heap is flat
at ~300 MB.

## CPU: where a second goes at 1 hour (BVN2 node, 30 s sample = 54.7 s of CPU, 1.8 cores)

| share | path | what it is doing |
|---|---|---|
| **34.9%** | `api/v3/message.Handler.Handle` → `Sequencer.Sequence` → `getSynth` | **serving healing pulls**. 86% of it is `getRootContinuation` → `getDirectoryReceiptForBlock`: for every pull, locate the anchor index entry for the block, then walk the anchor chain from that entry to the head and build a receipt. Per request, from the database. |
| **25.1%** | `ExecutorBridge.ProduceBlock` | executing blocks — of which `bcdb.getAt` 24.6% |
| **21.2%** | `BlockchainDB.lookupHistory` → `segment.lookup` → `bloomTest` | reads of accounts older than the window walk ~40 history segments per shard testing bloom filters (BlockchainDB#86) |
| **15.7%** | `runtime.gcBgMarkWorker` | garbage collection |
| 5.4% | `ed25519.Verify` | signatures — the irreducible work |
| 3.8% | `Batch.Commit` → `bcdb.commit` | the write path, including D5's pre-image reads |

Syscalls (`Syscall6`, 19% flat) are the preads behind the bloom tests and the
seal's fsyncs; `memmove`/`memclr` (8.6%) are the marshaling below.

At the wedge the mix shifts to the store: `filePool.shard`, `Bloom.Set`,
`Bloom.ByteMask` appear in the flat top as the store rebuilds filters while
the executor stalls.

## Allocation: what the GC is paying for (1 h, 1.78 TB allocated since start)

| share | path | source |
|---|---|---|
| **63%** | `ProduceBlock` | all of block execution |
| **31%** | `SyntheticMessage.MarshalBinary` | of which 27% via `encoding.Hash` |
| 24% | `AnnotatedReceipt.MarshalBinary` | the receipt inside every synthetic message |
| 23% | `Handler.Handle` → `getSynth` | serving pulls |
| 19.5% | `RecordStore.GetValue` | reads decoding values |
| 6.5% | `merkle.hashList.MarshalBinary` | chain state writes |
| 3.4% + 2.2% | `merkle.State.Copy` / `CopyAsInterface` | every `Get` of a chain state deep-copies it |
| 3.0% | `Batch.UpdateBPT` | |

**Who hashes a synthetic message** (482 GB, 27% of everything):

| share of `SyntheticMessage.Hash` | caller |
|---|---|
| 40% | `SyntheticMessage.ID()` |
| 36% | `MessageContext.recordMessageAndStatus` |
| 13% | `MessageContext.checkStatus` |
| 12% | `SyntheticMessage.Process` |

Every one of those marshals the whole message — the synthetic transaction plus
its `AnnotatedReceipt`, which is a merkle receipt with a hash per level — and
SHA-256s the bytes, then throws the bytes away. A message is hashed at least
four times on its way through one block, and never caches the result. That
is the S4 fix (#4211): compute the hash once and keep it on the message.

Note what this means for the API share: `getSynth` builds the same receipt
the healer will hash four times; with H1/H6 the pulls mostly disappear, and
with #4211 what remains is hashed once.

## What to do, in the order the numbers say

1. **Stop the healing traffic** (H7 fixed on the branch; H6 #4212, H1 #4193):
   at 1 h, 35% of CPU and 23% of allocation was serving pulls that a
   monotonic healer with a source-side cache would not make.
2. **Hash a message once** (#4211): 27% of allocation, most of the GC's 16%.
3. **Cache the receipt a pull needs** — `getDirectoryReceiptForBlock` per
   block, not per request — if pulls remain at all after (1).
4. **History bloom walks** (BlockchainDB#86): 21% of CPU, the store's.
5. **Bound the caches in bytes** (S5): 42–54 MB, the largest steady-state
   object we own.
6. **Provable view** (S2 follow-up) and slow-block pre-image retention: the
   217 MB at the wedge exists only because blocks took minutes; fixing what
   makes blocks slow removes it, capturing the provable view lazily removes
   the rest.

Signature verification, at 5%, is the only term here that is the protocol's
own work. Everything above it is either serving healing that should not be
happening, re-doing a hash, or walking a store structure that should be
indexed.
