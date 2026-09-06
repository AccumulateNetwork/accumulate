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

---

# Second pass (2026-09-06, later): what the first pass missed

Four more reviews, from angles the layer-by-layer pass did not take: an
adversarial re-check of everything the first pass called bounded plus this
week's new code; what is written and kept on disk and in the store's dynamic
layer (measured on run 20260905T153920Z's storage statistics, not estimated);
lifecycle and scaling paths (restart, resync, major blocks, partitions,
validators, metrics cardinality, goroutines, log volume); and a repo-wide
sweep for growth patterns. Everything marked verified was re-read in the
source by the author. Numbers are for one BVN at 500 tps unless said.

## The short version

1. **The dynamic layer, not the heap, is where the write path grows without
   end.** 72 records per user transaction, 57 % of them in the dynamic layer,
   which nothing prunes: eight statuses per transaction (one per wrapper,
   mutable until Delivered), the Produced, Cause, History and Signers sets
   rewritten whole on every add, the chain head rewritten with up to 256
   hashes on every append, and `Account.Chains` per dirty account. The eight
   copies of the transaction body are a separate, permanent-layer cost —
   content-addressed records are routed there and written once each (route.go,
   `Main`) — about 6.6 GB per hour. Roughly 25 to 55 bytes written per byte of
   input; about 85 % avoidable.
2. **Every cleared set becomes a tombstone exception the adapter keeps in a
   map for the life of the process** — two per delivered transaction
   (`Payments`, `Votes`, both `Put(nil)`) plus one per signer's `Signatures`
   set; the Payments count is measured (367 K clears in 37 minutes), the
   others are read from the code. Roughly 300–450 MB of heap per hour at 500
   tps, plus a file re-read whole at open. Neither pass had seen this. It is a
   candidate for the BlockchainDB soak growth the first pass could not place.
3. **Pre-images run on every commit that finds a read batch open**, and under
   load that is most of them: every CheckTx opens one, so did the sequencer's
   pinned view (now removed) and every API read. A pre-image for a key the
   dynamic layer does not hold walks the dynamic history, because a mutable
   store never short-circuits a miss (BlockchainDB `segstore.go:1794-1797`,
   read in the module source). How often a batch is open at commit under 500
   tps is inferred from CheckTx duration, not measured.
4. **A restarted validator has no way back into consensus.** It starts at
   round zero (`primary.go:287`, read); the catch-up window is the DAG GC depth
   of 2000 rounds; the "64 rounds per second" pull rate and the "about eight
   minutes" are the reviewer's, from `cert_sync.go:33` and a 09-04 log sample,
   not re-measured. Past the window it logs forever. The rejoin hook exists
   and nothing calls it; the recovery manager has no caller.
5. **A disconnected event subscriber leaves the API loading every block's
   ledger forever**, one leaked goroutine per disconnect.
6. **Two defects in this week's code**, fixed the same day in two steps
   (978b8eac6, then the correction below): the cache release walked the
   claimed number line (endless at the maximum) and a list-proven copy's
   Delivered was unauthenticated. The first fix verified the signature but
   not the signer — any key would do — and recorded a claim over an empty
   stream, which would have suppressed every later release. Both corrected:
   the signer must be a current validator of the source, and a claim over
   nothing is not recorded.

## Findings, ranked

### 17. Chain heads carry up to 256 hashes and are rewritten per append — VERIFIED, MEASURED

- **Where**: `pkg/database/merkle/chain.go:106-123` — `Head().Put(head)` on
  every `AddEntry`, and `head.HashList` holds the current mark set (up to
  `markFreq` = 256 hashes, average ~128 → ~4.4 KB) until the set closes.
  Every hash in it is also stored as `Element(i)`. Measured 800 head writes
  per block at 188 tx (main, signature and both index chains' heads).
- **Grows with**: per (chain, block) after write collapse; bytes with the
  mark set's fill.
- **Magnitude**: 17–34 GB per hour written to the dynamic layer, garbage until
  compaction folds it after it leaves the window; ~85 % of dynamic-layer bytes.
- **Remedy**: keep `Pending` (log n hashes) in the head and rebuild the mark
  set from `Element(mark..count)` when a receipt needs it, or keep it in
  memory. Head becomes ~500 B.

### 18. Cleared sets leave tombstone exceptions in memory forever — VERIFIED

- **Where**: `pkg/database/values/set.go:195-203` (an empty set marshals to
  zero bytes) → `pkg/database/keyvalue/bcdb/database.go:974-979` (`if
  len(value) == 0 { d.except(h) }`) → `:448-453` (`d.dyna[h] = true`,
  appended to `pendingDyna`) → `:411-426` (`persistExceptions` appends 32 B
  to the exceptions file) → `:391-405` (`loadExceptions` reads the whole file
  into the map at open). Cleared per delivered transaction: `Payments`,
  `Votes` (`msg_transaction.go:437-442`) and `Signatures`
  (`sig_common.go:296-300`) — three exceptions per transaction. Measured
  734 K clears in 37 minutes on run 153920Z.
- **Grows with**: per transaction, for the process lifetime and across
  restarts (the file).
- **Magnitude**: 5.4 M entries per hour → **~350–450 MB of heap per hour**
  (32-byte key plus map overhead), 173 MB per hour of file, and a start-up
  that reads it all. Neither pass had this; it is the first candidate for the
  memory growth of the earlier BlockchainDB soaks that the first pass could
  not place.
- **Remedy**: clearing a set nobody will read again should write nothing
  (delete the key rather than write an empty value), or these shapes are
  classified so a tombstone needs no exception; bound the exceptions map.

### 19. Eight statuses and eight bodies per user transaction; Produced and Cause written twice — VERIFIED, MEASURED

- **Where**: statuses — `msg_common.go:416`, `msg_transaction.go:276`,
  `synthetic.go:316`, `transaction.go:511`: a `Transaction.(hash).Status` for
  every wrapper (transaction, signature, credit payment, authority signature,
  synthetic transaction, sequenced message; the destination writes three
  more), 2.95 M in 37 minutes, dynamic layer, never deleted, and a status read
  for an old transaction walks all of dynamic history
  (BlockchainDB `segstore.go:1785-1797`). Bodies — `msg_common.go:385`,
  `msg_transaction.go:245`, `transaction.go:497`, `synthetic.go:310`: the
  transaction body is stored inside `TransactionMessage`, again inside
  `SequencedMessage`, again inside `SyntheticMessage`, and the same three at
  the destination, 8.2 per transaction, ~3.3 KB for ~540 B of input. Sets —
  `msg_common.go:391-413`, `synthetic.go:125-131`: `Cause` 4.4, `Message.Produced`
  4.3 and `Transaction.Produced` 2.7 per transaction, each a whole-set
  read-modify-write; `Produced` is written under both the message and the
  transaction.
- **Magnitude**: statuses 3.7 GB/h live and unpruned; bodies 6.6 GB/h
  permanent; sets 3.9 GB/h live. Per transaction: 72 records, 31.5 permanent
  and 41 dynamic; 14–31 KB written per 540 B input.
- **Remedy**: one status per message that has an outcome of its own (the
  transaction, the outer message) — a spec decision, since database.md names
  statuses as the dedup record; the transaction body stored once and referenced
  by hash from the wrappers; `Produced` written once, `Cause` derived from it.

### 20. Pre-images on nearly every commit; a mutable-key miss walks history — VERIFIED

- **Where**: `bcdb/database.go:477-480` (every read batch, including the
  one `exec_validate.go:22` opens per CheckTx, registers a view), `:1003`
  (`readers := len(d.views) > 0`), `:1043-1058` (`preImages`: a `GetDyna`
  per non-permanent entry of the commit); BlockchainDB
  `segstore.go:1794-1797`: a mutable key that is not in the window does not
  short-circuit, so the pre-image of a *new* dynamic key (every fresh status,
  produced set, signature set) falls to `lookupHistory`, a bloom probe per
  history segment.
- **Grows with**: entries per commit × dynamic history segments.
- **Magnitude**: ~5 K `GetDyna` per block × 5–10 segments ≈ 10⁵ page reads per
  block, on every node, always. Removing the sequencer's pinned view (finding
  5) did not remove this; #4225 was closed too early on that count.
- **Remedy**: CheckTx reads "latest", not a snapshot — it should not register
  an isolated view; and pre-image only keys the window holds (a BlockchainDB
  change: a window-only `GetDyna` for pre-images).

### 21. A restarted validator cannot rejoin consensus after ~8 minutes down — VERIFIED (code), not run

- **Where**: `internal/node/dagbft/service.go:384-397` (`initializeGenesis`
  restores the block index and nothing else), `pkg/consensus/primary/primary.go:287`
  (`currentRound: 0`), `vote_handler.go:333-337, 535-550` (a header more than
  `DefaultDAGGCDepth` = 2000 rounds ahead strands the node with one Warn per
  minute and an Info per header), `consensus.go:850` (`Rejoin` — uncalled
  since the fast-sync seed was removed today), `service.go:1035-1053`
  (`RequestStateSync` logs and returns nil).
- **What happens**: catch-up is 64 rounds per second within 2000 rounds; at
  ~4 rounds per second a node down longer than about eight minutes never
  rejoins and logs ~32 lines per second forever.
- **Remedy**: seed the round from the executor's own last block (the hook
  exists), or persist the checkpoint `pkg/consensus/recovery.go` already
  models; measure catch-up in blocks, not rounds. Part of E11.

### 22. DAG garbage collection stops when the executor halts — VERIFIED

- **Where**: `internal/node/dagbft/service.go:471-474` (the production loop
  returns on unrecoverable batches and nothing drains `committed`),
  `pkg/consensus/consensus.go:908-915` (the flush blocks on the 5000-group
  channel), `:931-934` (`GarbageCollect` runs only after a flush).
- **Grows with**: rounds after the channel fills (~2.8 h at one round per
  second); ~90–130 MB per hour per partition, two per dual node.
- **Remedy**: collect on round advance, not on flush; or stop consensus with
  the executor.

### 23. A disconnected event subscriber pins the pump and per-block ledger loading — VERIFIED

- **Where**: `internal/api/v3/event.go:172-173` (`subscribers.Add(1)`,
  deferred `Add(-1)`), `:192, :195, :214` (bare `ch <- ...` sends on a
  one-slot channel, no `select` on `ctx.Done`), consumer
  `pkg/api/v3/message/events.go:38-42` (returns on a failed write without
  draining), `event.go:101-103` (`if s.subscribers.Load() > 0 {
  s.loadBlockInfo(e) }`).
- **What happens**: one disconnect parks the producer on its second send
  forever; the counter never decrements; `loadBlockInfo` (a database view,
  the block ledger, one entry load per chain entry — thousands per block at
  500 tps) runs on every block with nobody listening.
- **Remedy**: `select` on `ctx.Done()` at every send; drain on exit.

### 24. The cache is seeded for 32 blocks but healing is served for 3600 — VERIFIED

- **Where**: `synth_cache_seed.go:22` (`seedCacheBlocks = 32`),
  `sequencer_cache.go:117-123` (past the seed: `NotFound`, "a miss is a
  defect"). Also lost at restart: `cache.received`, the Directory anchors
  executed but not yet dispatched, so their blocks' synthetics are never
  dispatched and only healing can fill them — from a cache that no longer
  holds them.
- **Remedy**: seed to the smallest remote `Delivered` (the release signal now
  gives it), or rebuild a block on a miss by position (the seed code already
  does it per block).

### 25. Behind the retention window the requester asks forever — VERIFIED

- **Where**: `requester.go:351-363` (empty stage asks the whole span),
  `:313-321` (`NotFound` is a miss), `:452-460` (back-off caps at 32 blocks).
- **What happens**: past the cache horizon a stream is `NotFound` every 32
  blocks forever; execution never passes the hole; everything above it is
  held. No halt, no state, one Error per stream per 32 blocks. E11's
  territory, now with no path but genesis.
- **Remedy**: a "stranded" state that stops asking and says so once.

### 26. A forged sequence number sizes the stage — VERIFIED

- **Where**: `msg_synthetic.go:149, 195` (a signature is required but
  verified only for own-proof copies), `:405` (`maxSequenceAhead` =
  2,000,000), `staging.go:126-138` (`hold` appends nils to the offset).
- **What it costs**: one list-proven copy re-wrapped with a forged number
  appends up to 2 M slots (16 MB) per stream, kept until the number passes
  (~an hour at 500 tps), plus a first-sighting squat costing one heal round.
- **Remedy**: verify the signer for every synthetic (the fix for the
  Delivered claim does this for that field; the hold should demand it too);
  size the stage by the source's produced count.

### 27. `Account.Chains` rewritten for every dirty account every block — VERIFIED

- **Where**: `internal/database/account.go:132-143` (`Chains().Add(...)`
  unconditionally; `set.Add` always `Put`s). ~270 per block, ~1 GB/h of
  dynamic-layer churn, entirely avoidable.

### 28. Requester re-asks collected-unvalidated entries; proofs stack under one block — VERIFIED

- **Where**: `requester.go:384-387` (the "staged proof is not a gap" guard
  was removed by 8d90814c0), `staging.go:527-541` (proofs appended per block,
  no dedup), `anchor_staging.go:80-84` (refusal counts blocks, not proofs).
- **Magnitude**: under Directory-anchor lag L: L/12 × ≤16 spans × ≤128 KB
  per source ≈ 43 MB at L = 256, then refusal and a heal storm. Adds to #4229.

### 29. Log volume ~490 MB per hour per node; the container cap keeps ~50 minutes — MEASURED (09-04 build)

- 650 lines per second per node, 99.6 % Info; "Received vote via gossip" is
  39 %; a twelve-hour run keeps only its last 50 minutes on the box
  (`docker-compose.yml:27-31`, 2 × 200 MB). Adds to #4231.

### 30. Smaller, real

- The dispatcher's `retries` map pins an envelope whose retry cycle then
  fails at the transport (`dispatcher.go:36, 125-139, 203-205`). Adds to #4222.
- Every executed sequenced message is deep-copied whether or not it carries a
  placeholder (`msg_sequenced.go:205-208`); `adjust64` marshals every produced
  synthetic twice to measure it (`synthetic.go:185-192`); the submit path
  builds a per-message ID string for an ungated Debug line
  (`internal/node/dagbft/api.go:196-202`).
- Info lines not in #4231: "Directory receipt for own block" per receipt per
  block (`block_begin.go:412`, added 2026-09-06), "Ignoring re-delivered
  certificate" per certificate (`service.go:628-630`), "Missing parent for
  header" per parent (`vote_handler.go:366-368`).
- Bootstrap server metrics label `http_requests_total` with the request
  path (`cmd/accumulated-bootstrap/info_server.go:686-696`): unbounded
  cardinality against any caller. No node-side vector has an unbounded label.
- Bootstrap reconnect spawns an undeadlined dial per peer every 15 s
  (`p2p/discovery.go:85-103`).
- Websocket handler: unsynchronised map and bare sends (`websocket/handler.go:121-158`); cold.
- `anchorRecord` opens a deep view and reads a signature set per answered
  anchor (`sequencer_cache.go:225-245`): 4096 views per restart-gap span.
  Keep the signatures in the cache's anchor entry instead.
- Released cache blocks keep their `Block`, `Streams` header and Directory
  receipt until the horizon (~20 MB per node, bounded); a stream whose
  destination produces nothing back is never released. Adds to #4232.
- `Events.Major.Pending` grows for the major period under multi-signature
  load (`block_end.go:431-433`); the backlog drain rewrites the whole record
  per block (`msg_transaction.go:632`). Zero under the load generator.
- Chain entries are re-read at block close to recover a hash known at append
  (`block_end.go:163-176`).
- Own batches have no byte cap and stale ones re-broadcast whole every 15 s
  (`worker.go:479, 739-759, 1165-1184, 1265-1273`); bounded by the queue.

## Fixed the same day

- The cache release walk is clamped to the highest held number and walks the
  held set when the claim outruns it; a `Delivered` is taken only from a
  message whose validator signature verifies (978b8eac6).

## First-pass claims re-confirmed as bounded

Stream positions; `buildRun` caps; anchor staging's block bounds (proofs per
block are not bounded — finding 28); the cache seed's cost (its coverage is
finding 24); `getDnHeight` (only for `HoldUntil` transactions); block ledger;
`Chain2.Get` registration (miss-only); index-chain search and `StateAt` (off
the block path); BPT pending per batch; retention and active stores; primary
per-header maps; cert-sync maps; Bullshark dedup; tombstones ring; publish
goroutines; no lock across I/O in cache or staging; bcdb caches (two
generations of 200 K, ~120 MB); the dynamic in-memory index sealed per shard;
`received` drained per block; validator set changes retain nothing; all
node-side metric vectors have bounded labels; timers and tickers are stopped
under context-checked loops.

## Not verified

Average mark-set fill (drives finding 17's range); dynamic-layer bytes on disk
(counts only in the run's statistics); whether the load generator's flow is
bidirectional per partition pair (finding 30, one-way streams); that a
restarted node sits at round zero in a running Docker network (read, not
run); the current build's log rate (the sample is the 09-04 build);
BlockchainDB's per-segment probe cost.

## Issues (second pass)

Chain heads #4234 (17); tombstone exceptions #4235 (18); write amplification
#4236 (19); pre-images per commit #4237 (20); restart cannot rejoin #4238 (21);
DAG GC on halt #4239 (22); event subscriber leak #4240 (23); cache seed vs
horizon #4241 (24); stranded requester #4242 (25); forged sequence number and
unverified signer #4243 (26); `Account.Chains` rewrite #4244 (27); per-message
churn #4245 (30); bootstrap metrics cardinality #4246 (30). Notes added to
#4222, #4229, #4231, #4232. The umbrella #4223 carries this whole document.
