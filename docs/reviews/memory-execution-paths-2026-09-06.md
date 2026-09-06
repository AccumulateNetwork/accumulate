# Review: what grows — memory and execution cost across the processing paths

2026-09-06, branch `issue-4193-producer-cache` at 8752bd3a2. Read-only.

**Target the review is measured against:** a steady state at 500 tps, two or
three BVNs by four validators (eight Directory validators), one-second
blocks, twelve-hour runs, memory flat under a 1700 MiB ceiling.

**Method:** four parallel reviews, one per layer — the executor's block path;
staging, the requester and the producer cache; the API sequencer and the store;
consensus and the node — each asked the same question: what increases memory
or execution requirements, and with what does it grow (per transaction, per
block, per chain height, per validators, per destinations, per lag, per
wall-clock retention). Every finding marked **verified** below was then read
again in the source by the author of this document; findings the reviewers
marked "suspected" are kept as such. Magnitudes are estimates from the code,
not measurements, unless a run is named.

Two of the eleven top findings are in code written this week (E12 step 3);
they are marked. Nothing here is fixed: this is the review Paul asked for, and
the order of work at the end is a proposal.

---

## The short version

1. **The producer cache is the footprint.** Every produced synthetic, with its
   full companion transaction, is kept for 3600 blocks on every node. At 500
   tps that plateaus at roughly 2–4 GB per BVN node after an hour. No
   delivery signal releases anything sooner. (Found independently by three of
   the four reviews.)
2. **Lag is unbounded in bytes.** Staging holds full bodies for everything
   received and not executed, bounded only by counts in the millions. One
   minute behind at 500 tps is tens of MB; an hour is gigabytes.
3. **The dispatcher drops the rest of a cycle on the first transport error**
   and has no deadline. Nothing requeues or counts the loss. This is the
   likeliest mechanism for issue 4222 (anchor copies lost between a
   validator's conductor and the destination).
4. **A read view is held open across blocks by the sequencer's snapshot
   capture**, which makes every BlockchainDB commit read back a pre-image for
   every dynamic record it writes, and makes every read scan the undo
   versions.
5. **Per-copy costs multiply by validators.** Each of the eight (Directory)
   copies of an anchor is stored in full, appended to a signature chain, read
   and rewritten as a set, and logged at Info twice.

---

## Findings, ranked

### 1. Producer cache: every entry, with its transaction, for an hour — VERIFIED

- **Where**: `internal/core/synthcache/cache.go:28-45` (`DefaultHorizon =
  3600`, `Entry{Seq, Companion}`), `:260-327` (`Commit`, `trimLocked` — the
  horizon is the only eviction); filled at
  `internal/core/execute/v2/block/synthetic.go:105-117` (`entry.Companion =
  txn` loaded from the store).
- **Grows with**: per synthetic × time. Flat after the first hour; the
  plateau is the problem.
- **Magnitude**: 1.3–2.7 KB per entry (sequenced message + decoded companion
  transaction + two map slots + a slot in `Block.Entries` + a 32-byte
  element). 500/s × 3600 ≈ 1.8 M entries ≈ **2–5 GB per BVN node**, on every
  node, not only the leader. At 100 synthetics per block still 0.5–0.9 GB.
- **Also**: per block per destination a `Stream.Segment` whose `Before` state
  is a full `merkle.State` with a `HashList` of up to 256 hashes (~7 KB
  average) plus `Pending` and a root receipt: ~10 KB × destinations × 3600 ≈
  70–110 MB. `merkle/types_gen.go:151-167` copies `HashList` in `State.Copy`.
- **Remedy**: release a block's entries once dispatched and past
  `InFlightBlocks`, or on the destination's `Delivered` (the requester already
  learns it); hold `Companion` by hash and re-read on a heal (it is recent by
  construction); keep only `Pending` in the segment's `Before`. Size the
  horizon in bytes, not blocks.

### 2. Staging under lag: bodies for everything not yet executed — VERIFIED

- **Where**: `internal/core/execute/staging.go:66-78` (`Held{Message,
  Companion}`, `MaxStageSpan = 4<<20`), `:126-149`, `:263-300`;
  `internal/core/execute/v2/block/msg_synthetic.go:382` (`maxSequenceAhead =
  2_000_000`), `:417-427`; `stream_position.go:499-506` ("There is no upper
  bound").
- **Grows with**: per transaction × lag, per stream. Bounds are counts (2 M /
  4 M slots per stream ≈ 5–10 GB), never bytes; nothing back-pressures
  acceptance.
- **Magnitude**: 500 tps × lag: one minute ≈ 75 MB, ten minutes ≈ 750 MB, an
  hour ≈ 4.5 GB.
- **Related**: `anchor_staging.go:80-97` refuses a package once a source has
  proofs waiting on 256 distinct Directory blocks; per block the proof count
  is unbounded (`staging.go:531`). Under Directory-anchor lag past ~4 minutes
  the destination refuses every package, which turns lag into a heal storm
  (finding 9).
- **Remedy**: a byte budget per stage that turns `Hold` into "not held, ask
  later", and count proofs, not blocks, in anchor staging.

### 3. Staging release pins a lag's backlog after it drains — VERIFIED, E12 step 3 (mine)

- **Where**: `staging.go:154-176`, the `drop > len(entries)` branch:
  `st.entries = st.entries[:0]` keeps the backing array and every `*Held`
  pointer in it reachable; only the other branch compacts. Same for
  `validated[:0]` (32 B per slot, pointer-free).
- **Grows with**: once per lag episode; retained for the process lifetime.
- **Magnitude**: peak backlog × ~2.5 KB never returned; a 100 K backlog leaves
  ~250 MB live. This is the common path: most entries deliver from their own
  block, so `drop` usually exceeds `len`.
- **Remedy**: `clear()` before truncating, and apply the compaction test in
  both branches. One-line fix.

### 4. The dispatcher drops the rest of a send cycle on the first error, and has no deadline — VERIFIED (mechanism), SUSPECTED (deadline)

- **Where**: `pkg/api/v3/message/transport.go:91-144` — `RoundTrip` iterates
  requests serially and `return`s on the first dial, write or read error;
  `internal/node/daemon/dispatcher.go:161-206` — on `err != nil` the batch is
  neither requeued nor settled, one error line; only "worker backpressure"
  responses retry (`retryLater`, 10 cycles). `block_begin.go:87` and
  `conductor.go:165` pass `context.Background()`; the stream read
  (`pkg/api/v3/p2p/dial/stream.go:61`) sets no deadline.
- **What it costs**: one bad dial to one destination ends the cycle and every
  envelope queued behind it, to any partition, is lost silently: an anchor
  and the synthetic packages behind it. A peer that accepts the stream and
  never answers pins the goroutine and a block's outbound bytes forever.
  Losses are logged, not counted, so no metric shows them.
- **Grows with**: per block × destinations × peer failures.
- **Why it matters here**: soak 20260905T225751Z lost Directory anchor copies
  with no error but eight startup dial failures; issue 4222.
- **Remedy**: continue past per-request errors, requeue non-client errors
  with a bounded retry budget, count drops in a metric, and put a deadline of
  a few block intervals on `Send`.

### 5. A read view held open across blocks makes every commit take pre-images — VERIFIED

- **Where**: `internal/api/v3/snapshot_range.go:98-124` (`captureProvableView`
  begins a batch on the API's deep database at every provable commit and
  keeps it until the next); `pkg/database/keyvalue/bcdb/database.go:1003`
  (`readers := len(d.views) > 0`), `:1043-1058` (`preImages`: one
  `GetDyna` per non-permanent entry of the commit), `:1064-1071`
  (`preImageAt` scans `undoVersions` linearly on every read).
- **Grows with**: per block, execution: N mutable writes × one extra store
  read; memory: one undo map per block for as long as any older view is
  pinned (a slow API request, or `pinSnapshot`'s whole-database `Collect`).
- **Magnitude**: 500 tps writes roughly 4–6 K dynamic records per block, so
  4–6 K extra reads per block on the producer's commit path, always; undo
  memory normally one or two blocks of pre-images, unbounded during a pin.
- **Remedy**: do not hold a live batch across blocks — record the provable
  block index and re-open at pin time; expire views older than N blocks.

### 6. Anchor copies: full body, chain append, set rewrite and two Info lines per validator copy — VERIFIED

- **Where**: `msg_block_anchor.go:50-149`; `msg_common.go:372-420`
  (`recordMessageAndStatus` puts `m.message`, the whole `BlockAnchor` with
  the anchor body and one receipt per BVN, under a hash that differs per
  signer); `RecordHistory` (signature-chain append, `History` and `Signers`
  set read-modify-writes, `signatures.go:88-112`); `ValidatorSignatures().Add`
  (`values/set.go:47-60`: `Get` + sorted insert + `Put`) and two more `Get`s;
  Info at `exec_process.go:222` and `msg_block_anchor.go:127` (the per-copy
  "Anchor signature" line added on 2026-09-06 to find the stall).
- **Grows with**: validators × blocks (× partitions on the Directory: ~20
  copies per Directory block).
- **Magnitude**: a BVN stores 8 × 2–5 KB per block ≈ 0.7–1.7 GB of disk per
  twelve hours in anchor bodies alone, plus eight signature-chain entries per
  block; 16–40 Info lines per second network-wide.
- **Remedy**: store the body once (first copy), later copies as signature
  only; demote per-copy Info to Debug once 4222 is closed.

### 7. Nested batches with copy-on-read, three to seven levels per transaction — VERIFIED

- **Where**: `msg_transaction.go:230` → `:336` → `transaction.go:84`
  (`NewStateManager(..., batch.Begin(true), ...)`), wrapped by
  `msg_sequenced.go:124`, `msg_synthetic.go:264`, and the shard child at
  `exec_parallel.go:314`; `internal/database/batch.go:72-86` (`Begin`
  allocates and `fmt.Sprintf`s an id); `pkg/database/values/value.go:319-325`
  (a child's first read deep-copies the value).
- **Grows with**: per transaction × depth × records touched.
- **Magnitude**: 500 tps × ~10 records × 3–5 levels ≈ 15–25 K value copies and
  1.5–2.5 K batch allocations per second. Churn, not retention.
- **Remedy**: one savepoint per message instead of one per wrapper level;
  drop the id string; share immutable values on read.

### 8. A chain head copied once per produced message, 499 of 500 discarded — VERIFIED, E12 step 1 (mine)

- **Where**: `synthetic.go:279-283` (`Head().Get()` then `before.Copy()` for
  every produced message; `produceSyntheticInto` uses it only when a
  destination's segment is first created, `:97-103`).
- **Grows with**: per synthetic (allocation churn).
- **Magnitude**: ~8 KB × 500/s ≈ 4 MB/s, ~14 GB per hour through the
  collector on the block path.
- **Remedy**: copy the head only when the destination's segment is created.

### 9. The requester under lag: duplicates by the thousand, one envelope per signature — VERIFIED (code), the storm itself seen in earlier runs

- **Where**: `requester.go:384-400` (`decide` treats a held entry whose proof
  is staged but not yet anchored as a gap), `:471-551` (`requestSpan`);
  source side `sequencer_cache.go:107-186` (up to 4096 records, each
  ED25519-signed, each with its companion); `requestAnchorSpan` `:577-583`
  (one envelope per signature per record, since 2026-09-06).
- **What it costs**: once a destination is ≥ 8 source blocks behind on
  Directory anchors, each activation asks up to 16 spans × 4096; the source
  answers ~8 MB and 4096 signatures per span; the destination bundles it all
  back into its own consensus although `Hold` will toss every one (first
  sighting wins); re-asked every 12 blocks. For anchors after a restart with
  an hour's gap: 3600 records × 4–8 signatures ≈ 14–29 K envelopes of full
  `DirectoryAnchor` bodies in one activation.
- **Also**: the empty-stream probe fires every activation, not once per
  patience window — `asked()` is recorded only on `err == nil`
  (`requester.go:295-297`) and a quiet stream answers `NotReady` — and it
  counts a synth-cache "miss" although nothing is missing
  (`sequencer_cache.go:113`, `cache.go:407`). The asked-once memory allocates
  one heap object per index (`:417-431`; ≤ 3.7 MB per stream worst case).
- **Remedy**: an entry whose proof is staged is not a gap; one anchor
  envelope carrying all signatures; record the probe on `NotReady`; count a
  miss only when the number was produced.

### 10. `AddChainEntry2` scans the block's chain-update list on every append — VERIFIED

- **Where**: `internal/core/execute/v2/chain/state_state.go:168-172` — `for
  _, e := range u.Entries { if e.Chain == chain.Name() &&
  e.Account.Equal(chain.Account()) ...`, `url.Equal` comparing strings.
- **Grows with**: O(E²) per block, E = chain updates in the block (2–4 per
  transaction).
- **Magnitude**: E ≈ 1.5–2.5 K at 500 tps → 1–3 M comparisons per block, most
  on "main" chains where the URL compare runs: tens of milliseconds per
  block, quadratic in block size.
- **Remedy**: index `u.Entries` by (account, chain) alongside the slice.

### 11. DAG certificate retention and a whole-DAG garbage collection per certificate — VERIFIED (retention), reviewer-verified (call frequency)

- **Where**: `pkg/consensus/dag/dag.go:45-59, 296-324` (`rounds`,
  `digestIndex`, `certifiedBatches`; `GarbageCollect` iterates every retained
  round and every certified-batch entry), `pkg/consensus/consensus.go:48`
  (`DefaultDAGGCDepth = 2_000`), `:933` (called from the commit path per
  processed certificate).
- **Grows with**: validators² × rounds retained (memory); per certificate ×
  depth (CPU).
- **Magnitude**: Directory with eight validators ≈ 8 × 2000 × ~3 KB ≈ 50 MB
  per DAG (issue 4164 measured 144 MB at twelve validators); a dual node holds
  two. ~16 certificates/s × (2000 rounds + ~16 K batch entries) ≈ 300 K map
  iterations per second.
- **Remedy**: collect only when the cutoff moved; bucket `certifiedBatches`
  by round; size the depth by time (500 rounds ≈ 4 minutes).

### 12. Root-segment receipts replay the block's appends per destination — VERIFIED (code), magnitude estimated

- **Where**: `block_end.go:880-897` (`rootSeg.Receipt(st.RootPos, height-1)`
  per stream); `pkg/database/merkle/segment.go:72-88` (`stateAt` replays from
  `Before` with a `Copy` per call, once per intermediate).
- **Grows with**: per block, O(R log R) hashes, R = chains anchored this block
  (one root entry each), × destinations.
- **Magnitude**: R ≈ 1–2 K at 500 tps → ~100 K `AddEntry` per block across
  three streams: tens of milliseconds.
- **Remedy**: memoize prefix states in the segment, or build the root receipt
  once from the final state.

### 13. Per-block bound is per header, so a block can carry validators × 256 KiB; system traffic is never refused — VERIFIED

- **Where**: `pkg/consensus/primary/header_builder.go:58-75` (256 KiB per
  header), `primary.go:107` (lag bound 8), `worker.go:409, 449-455` (`Submit`
  never refuses), `worker.go:1269-1272` (own batches never evicted).
- **Grows with**: validators × lag (pinned batches); inbound system bytes ×
  lag (own store).
- **Magnitude**: a Directory leader group spans ~2 rounds × 8 headers → up to
  4 MiB per block, and eight lagged groups pin up to 32 MiB, the whole
  per-partition active-store budget; own-store growth ≈ 50 KB/s × lag seconds
  while lagging.
- **Remedy**: budget per block (`MaxHeaderBytes / V`) or bound lag in bytes;
  give `Submit` an own-bytes ceiling or keep proposing system traffic while
  lagging.

### 14. Info-level logging in per-message hot paths — VERIFIED

- **Where**: consensus: `gossip/gossip.go:391-394, 413-416`,
  `primary/vote_handler.go:109-112, 257-260, 430-434`, `primary.go:532-536`,
  `certificate_handler.go:71-75, 186-189` (every header, vote and vote-added
  event, hex-formatted); executor: `exec_process.go:207-252` builds the
  key-value slice and calls `msg.ID()` for every message before any level
  check; `msg_transaction.go:369-375` logs every failed transaction at Info;
  the per-copy anchor line (finding 6).
- **Magnitude**: ~40 lines per round → 80–100 lines/s per partition before
  any sync traffic; ~2 K allocations per second at 500 tps for log arguments
  that are never emitted.
- **Remedy**: gate on `Enabled`; one summary per round; Debug for per-message.

### 15. Per-transaction copies and re-marshals on the consensus path — VERIFIED

- **Where**: `worker.go:491-492` (copy on submit), `executor_bridge.go:409-430`
  (unmarshal + full `Validate` per transaction), `types/batch.go:81-110`
  (`Marshal` re-serializes; called at digest, broadcast, every re-broadcast
  and every peer fetch), `internal/node/dagbft/api.go:215, 234`,
  `executor_bridge.go:267-283` (a hex digest string per transaction).
- **Magnitude**: four or five full copies of every transaction's bytes plus
  hex and URL strings ≈ 0.5–0.6 MB/s avoidable allocation.
- **Remedy**: keep the wire buffer with the batch; drop per-transaction
  strings.

### 16. Smaller, bounded, still worth a line

- `HeldByID`/`HeldTransaction` scan the block's own additions linearly
  before the indexed base (`staging.go:331-337, 353-359`): O(arrivals ×
  runs) per block; a 5000-arrival catch-up block is 25 M iterations.
- Package proof checks are O(members × span) and repeated three times
  (`msg_synthetic.go:45-63, 140, 220`, `anchor_staging.go:344-367`); at the
  4096 cap ≈ 50–100 ms per package per node.
- `SequenceRange` from the cache joins segment elements per block with a
  fresh copy (`sequencer_cache.go:151`): quadratic when a destination gets
  few entries per block.
- Every API miss walks all of history (`cmd/accumulated/run/api.go` uses
  `Deep()` for all services; `bcdb/database.go:906-912`): `anchorRecord`'s
  new held-signature lookup misses for BVN anchors; status polls for
  not-yet-executed transactions miss by design.
- bcdb `immutableCache.get` takes the exclusive lock on every hit to promote
  (`cache.go:78-85`): contention, not memory; the chains cache turns over
  every 200–400 blocks at 500 synthetics per block.
- `trimLocked` walks every held block and anchor on every commit under the
  write lock (`cache.go:305-327`): ~100 µs per block.
- Peer-fetch loops try every host peer serially, two seconds each
  (`consensus.go:303-314, 472-487`): a missing batch can cost 40 s of
  executor wall time per pass with ~20 host peers.
- The committed-group channel holds 5000 groups (`consensus.go:49, 271`):
  bounded at ~24 MB, but sized for a lag far past the 8-block bound.
- Leader dispatch signs and marshals per synthetic (`block_begin.go:604-632,
  724-759`): ~30–50 ms CPU per second at 500 tps; a list-proven message
  needs no signature of its own.
- GossipSub flood-publish with library default queues
  (`p2p/discovery.go:125-128`): a slow peer can hold 16 MB of batches
  (suspected; library defaults not re-verified).
- Suspected: the per-major `Pending` set (`block_end.go:415-436`) is a
  read-modify-write of a set that accumulates for the major-block period, if
  the load's transactions leave pending marks at close (not verified).

---

## Checked and found bounded

Stream positions (one ledger read and write per stream per block); `buildRun`
stops at the first gap and is capped at 1024 × 8 rounds; anchor staging's
`maxAnchorAhead` and `maxStagedProofBlocks`; receipts built from in-memory
segments; the cache seed (32 blocks, once); `getDnHeight`; events per block
(100); the block ledger (E7); `Chain2.Get` registration (a per-account set of
~5, cached per batch); index-chain search and `StateAt` (≤ 256 replays from
the prior mark); BPT inserts deferred per dirty account; leveldb's caches
(64 MB record cache, 64 MB block cache per engine, buffer pool disabled);
`commitRounds` ≤ 16384; `historyKeys` ≤ 200 K per shape; the memory change
set caching writes only; consensus retention and active stores byte- and
age-capped; primary per-header state bounded to 2–10 rounds; cert-sync
in-flight and answered maps; Bullshark dedup pruning; tombstones; signature
work per round (~3 ms on the Directory); `Discard` paths freeing their maps;
no lock held across I/O in the cache, staging or the block hook.

## Not verified

Per-entry byte sizes in the producer cache (estimated, not measured); how many
API reads actually miss the window in a soak (`historyReads` in `stats.json`
would say); the pubsub queue defaults; whether the per-major `Pending` set
grows under the load generator's flow; the memory the E12 build takes on
BlockchainDB at all — every Docker run of 2026-09-05 was on leveldb.

## Proposed order of work

1. Producer cache retention (finding 1): release on dispatch + in-flight or
   on `Delivered`; companion by hash. This is the plateau.
2. Dispatcher: continue past errors, requeue with a budget, count drops, a
   deadline (finding 4). Closes the mechanism behind issue 4222.
3. Staging release pinning and the per-message head copy (findings 3, 8): two
   one-line fixes in this week's code.
4. The provable view (finding 5): re-open at pin time instead of holding.
5. Anchor copies stored once, per-copy logs to Debug (findings 6, 14).
6. A byte budget on staging with back-pressure (finding 2), and proofs
   counted not blocks.
7. `AddChainEntry2` index (finding 10); DAG GC cadence (finding 11); nested
   batch depth (finding 7).
8. The requester's duplicate storm and probe accounting (finding 9).

Each becomes an issue with the finding's text; none is started from this
document.

---

## Plan (2026-09-06, after Paul's review of the short version)

Paul's responses, and what follows from each.

**1. The producer cache.** DONE the same day (ccb80592e): every dispatched
synthetic carries the sender's `Delivered` for the reverse stream and the
destination releases what it holds at or below it. Forty transfers each way
leave three entries in each cache, not forty. Remaining: the signal rides only
on synthetic dispatch, so an idle reverse stream falls back to the horizon;
produced anchors still go by the horizon (one transaction per block, small).
Follow-up: carry the same word on anchors so the anchor cache and an idle
stream's tail clear too; then shrink `DefaultHorizon` from an hour to a few
minutes, since it is a backstop.

**2. Staging under lag — Paul: "We only stage what hasn't been executed. Once
executed, messages are removed from staging."** Correct, and the finding was
overstated. What staging holds is exactly what consensus has accepted and
execution has not yet run; it is emptied by execution. The size question is
therefore "how far can accepted outrun executed", and that is already bounded
on the consensus side: the execution-lag bound (C6, eight blocks) stops
headers carrying batches when the executor is behind. The one part not bounded
by C6 is the *collected* set — synthetics that arrived before the Directory
anchor that proves them — which grows with Directory-anchor latency times the
inbound rate. Plan: no byte budget on staging; instead (a) a gauge of held
entries and bytes per stream so the collected set is visible, (b) an alarm
when it exceeds a few blocks' worth, because that means anchors are late, and
(c) the anchor-latency work itself (anchors through the stage, issue 4222's
dispatch fix) is what keeps it small.

**3. Staging release pins a drained backlog — Paul: "This needs fixing."**
FIXED the same day: `release` clears the dropped pointers and compacts in both
branches; `TestStaging_ReleaseReturnsTheBacklog` holds 5000, releases them,
and checks the capacity is returned and nothing stays indexed.

**4. The dispatcher — Paul: "The dispatcher needs to be isolated from the data
being dispatched. On a failure, a write should be retried."** Plan, as issue
4222's fix: the dispatcher owns an outbound queue per destination that is
independent of the block that produced the envelopes — the block hands
envelopes over and is done. Each destination's queue is sent on its own
stream with its own deadline (a few block intervals), so one unreachable
partition never blocks another. A request that fails for any reason other
than the destination refusing it as invalid is kept and retried with a
bounded back-off; a refusal is settled and counted; a request that exhausts
its retries is counted and logged with its destination and kind. Metrics:
queued, sent, retried, dropped, per destination. Acceptance: a simulator hook
that fails the first send to one partition for N blocks; every anchor and
synthetic still arrives, and the drop counter stays zero. Second acceptance:
the four-validator Docker topology with one node's submit path cut for a
minute shows no lost anchor copies.

**5. The sequencer's read view — Paul: "I have no idea what the sequencer means
here."** The sequencer is the API service that answers healing requests
(`Sequence`, `SequenceRange`) and the snapshot endpoint
(`internal/api/v3/sequencer.go`). For the snapshot endpoint it wants a
database view at a provable block, so at every block commit it opens a
read-only database transaction and keeps it open until the next commit
replaces it. BlockchainDB must keep a consistent view for any open reader, so
while that transaction is open every commit reads back the old value of every
record it changes and keeps those old values until the reader closes. At 500
tps that is four to six thousand extra reads per block, always, and a slow
snapshot pins several blocks' worth of old values. Plan: the sequencer records
only the provable block index at commit and opens the view when a snapshot is
actually requested, checking the index still matches; and BlockchainDB expires
any reader view older than a fixed number of blocks. Acceptance: with no
snapshot in progress, `preImages` is never called (a counter), and a snapshot
request still produces a provable snapshot.

**6. Anchor copies — Paul: "Explain more clearly?"** A Directory anchor is one
transaction, but each of the eight Directory validators sends its own signed
copy of it to each BVN, and the BVN needs six of those signatures before it
executes the anchor. Today the BVN treats each copy as a separate message: it
stores the whole anchor body — the `DirectoryAnchor` with a receipt for every
BVN, several kilobytes — eight times under eight different hashes, appends
each copy to the transaction's signature chain, reads and rewrites the
validator-signature set for each, and logs each at Info twice. So one anchor
costs eight bodies on disk per block per BVN (roughly a gigabyte per twelve
hours), eight chain appends, eight set rewrites and sixteen log lines, and on
the Directory about twenty of each per block. Plan: store the anchor body
once, keyed by the transaction hash, when the first copy arrives; record every
later copy as its signature only, against that transaction; keep one chain
append per distinct signer (that is the signature chain's purpose) but write
the set once per block from the copies the block brought, not once per copy;
move the per-copy log lines to Debug once issue 4222 is closed. Acceptance:
the anchor transaction is stored once per BVN block, the signature set is
written once per block per anchor, and `TestAnchorThreshold` still executes on
the second distinct signer.

**7 onward.** Filed as separate issues from this document: the
`AddChainEntry2` scan (finding 10), DAG garbage collection cadence (11),
nested batch depth (7), the requester's duplicate storm and probe accounting
(9), the per-header byte bound (13), per-message Info logging (14), root
receipt replay (12), and the smaller items (16).

## Issues

Umbrella (this document in full): #4223. Per finding: anchor copies #4224 (6);
sequencer snapshot view #4225 (5); `AddChainEntry2` scan #4226 (10); DAG GC
cadence #4227 (11); nested batches #4228 (7); requester under lag #4229 (9);
per-header byte bound #4230 (13); Info logging #4231 (14); cache and block-close
residue #4232 (1, 8, 12); staging visibility #4233 (2); dispatcher #4222 (4).
Done: finding 1 (ccb80592e), finding 3 (4ff104e6f).
