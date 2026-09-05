# Differences — where the code and the spec disagree

The specification says what we are doing. It does not describe the
implementation's departures from it, because a spec that documents its own
exceptions stops being normative.

This document holds those departures. Each entry names what the spec requires,
what the code does instead, and the evidence. It is the working list that
issues are written from once a part of the spec is settled — not a substitute
for them, and not a backlog in itself.

**An entry is removed when the code matches the spec, not when an issue is
filed or closed.** The issue link under each heading is where the work is
tracked; the entry itself is the difference.

---

## Executor

### E4. An anchor's quorum is assembled by execution

*[#4198](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4198)*

**Spec**: anchor authorization is a staging decision. Signatures route to
staging, staging packs them with the one anchor and evaluates quorum or proof,
and the anchor executes once with no further checking.

**Code**: each validator sends a full `BlockAnchor` carrying the whole payload
and its own signature. Each is a complete message execution — writes
`recordMessageAndStatus` and `RecordHistory`, adds one signature to
`ValidatorSignatures()` — and the copy that crosses `ValidatorThreshold`
executes the anchor. For an N-validator partition, N−1 deliveries exist only to
deposit a signature. Copies cannot deduplicate because each embeds a different
signature and therefore hashes differently.

Staging already asks the right question — `admissibilityOf` calls
`anchorIsAdmissible`, the same rule `txnIsReady` uses at execution, shared
deliberately (#4169 step 3b) — but has nothing to collect, so the rule is
evaluated twice over state that execution had to write first.

**Size**: medium. Cost is O(validators) per anchor: 445 anchors against 180,997
synthetics in run `20260902T132651Z`, so small today, linear in validator count.

### E5. Staging is re-evaluated in a loop

*[#4197](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4197)*

**Spec** ([executor.md](executor.md)): each stream is evaluated once. A
stream's run is computed from its arrivals and what is already staged, anchors
are evaluated and executed before synthetics so the chain already carries this
block's anchors, and a user transaction cannot unblock a stream because what it
produces locally executes next block.

**Code**: `stageRuns` "decides one kind of stream's runs AT THE MOMENT IT IS
CALLED, and is meant to be called more than once per block". `drainRevealed`
re-runs it up to `maxDrainRounds` (8) times, stopping when a round delivers
nothing.

Its stated reasons are the two the ordering above is supposed to remove: that
deciding synthetics before anchors run judges them against a chain missing this
block's anchors, and that a message recorded pending by something processed this
block becomes drainable within it. The second is backed by a measurement — with
runs decided once per block, delivery settled into exact lockstep with arrival,
40 in and 40 out, leaving a block of lag that never closed
(`TestNoLaggingChannels`).

That measurement is evidence that *something* was incomplete when runs were
decided once, not that repeated evaluation is the design. A loop that re-asks
compensates for a run that was not computed completely the first time. What
needs establishing is which of the two reasons still bites once the run is
computed from arrivals **and** the staged set, and anchors execute first — and
if neither does, the loop and its bound both go.

**Size**: medium, and it is a correctness question before it is a performance
one: `maxDrainRounds` exists so that a round which always reports progress
cannot hang a block, which is a guard against a condition the design says
cannot arise.

### E6. `CascadeDeliveryQueue` is dead state that is still hashed

*[#4195](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4195)*

**Spec**: there is no cascade.

**Code**: nothing writes the queue, but it survives as an account field with its
accessor, dirty tracking, walk and commit; snapshots carry it
(`snapshot.go:790`); `observer_debug` reads it; and `observer_prod:70` folds it
into the **account hash** beside `LocalDeliveryQueue` (#4155).

**Consequence**: inert only because it is always empty, so it never contributes
to the hash. One accidental writer from changing account hashes.

**Size**: small, but touches the account model and the hasher.

---

### E8. Staging is one store, proofs are not keyed by their anchor, and an unproven entry is parked outside it

*[#4217](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4217)*

**Spec** ([executor.md](executor.md), "Collection", "Proof", "Anchor staging"):
two stores — entries by stream and index, proofs by the Directory block of their anchor; a
proof waits for its anchor and is validated or discarded by it; a validated
proof marks its index range proven; an entry executes when proven and next;
nothing is recorded pending outside staging; a gap is a proven index without
an entry or a held index without a proof.

**Code**: one store of held entries (`internal/core/execute/v2/block/staging.go`).
A collection proof is staged under its anchor block since E8 step 2
(`anchor_staging.go`), and since step 3 the proven set (`synthetic_replica.go`,
the `synthetic-replica:<stream>` mirror chain) is excluded from the account
hash and refuses conflicting proofs; it is still not released below the
delivered point, so it grows with the stream. A proof does not carry
its anchor's block before E8 (`AnchorMetadata.SourceBlock`, filled and read since E8 steps 1–2); the destination tests the proof's terminal root
against its directory anchor chain at execution (`admissible.go`). Since step 4 a
package member whose anchor has not executed is collected — held in staging at
its number — and staging judges proof-less entries by the proven set, so the
hole the healer had to fill no longer opens (`test/e2e/collection_test.go`). The healer's reconcile path infers a lost tail from the
source's `Produced`, which the spec no longer needs.

**Evidence**: run `20260904T035906Z`: `exec_synthetic_anchor_total{applied="missing"}`
outnumbered `earlier` nine to one on BVN2; a third of everything BVN1 received
from BVN2 had been pulled by the healer; the lost numbers came in runs the size
of one package.

**Consequence**: the delivery race between a package and the anchor that proves
it is decided by whichever executes first, and losing it costs a heal per entry.

**Remaining**: release of the proven set and of held entries below the
delivered point (`internal/core/execute/staging.go` deletes nothing); the
sequenced layer still records a Pending status and an `Account.Pending()` entry
for an out-of-order arrival (`msg_sequenced.go`, `recordPending`) beside the
hold — a status outside staging the spec says must not exist; a proof that does
not name its anchor block is left to its message executor rather than refused,
until H8 retires the paths that produce such proofs; intake writes an entry
durably only when it cannot execute this block; the gap questions by index
("proven and missing", "held and unproven") and the retirement of the
reconcile-by-`Produced` path, which land with H8's request set.

### E9. The executor reads permanent records past the window with no deep reader, and appends a transaction's hash from two sites

*[#4219](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4219)*

**Spec**: a reader that reaches past the window takes a deep reader; a message's
hash is appended to a chain once, from one site (database spec, "Duplication").

**Code**: `Message.Main` is read for pending transactions for up to
`PendingMajorBlocks` (14 major blocks) through an ordinary batch —
`BeginDeep` exists in the adapter and nothing in `internal/` calls it.
(Dispatch's reads are H1's, not a deep reader's.) The
observer reads the v1 `Transaction(h).Main` record for every pending txid
(`observer_prod.go:121`), a permanent shape never written by v2.
`clearActiveSignatures` writes `Signatures` of every signer in the book, absent
or not. `SyntheticIndexIndex` is write-once and read after the window on every
dispatch. The transaction hash reaches the principal's main chain from the
state cache (`state_cache.go:207`) and again from the success path
(`transaction.go:581`); the chain's uniqueness check absorbs the second, and a
lost append would be swallowed as `ErrNotFound` at `transaction.go:582`.

**Size**: medium; the deep-reader plumbing is the larger part. The
single-site append is small and state-neutral when the state-cache append is
the one skipped (the success path runs for every type that reaches it).

---

### E10. Staging is written to the database and snapshotted

*[#4217](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4217), [#4205](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4205)*

**Spec** ([executor.md](executor.md), "Sync", invariants 4 and 6): staging is
memory, the state before any persistence; a node that joins or restarts
replays the committed stream from its last executed block and rebuilds it,
executing nothing until it has caught up. A snapshot carries executed state
only.

**Code**: E8 built staging as durable, unhashed records — `Sequenced`,
`Sighted`, `StagedSources`, `StagedProofs`, `StagedProofBlocks`, `Collected`
and the `synthetic-replica:<stream>` chain — written at intake, enumerated for
snapshots (`snapshot_anchor_staging_test.go`) and restored with them. Every
one of those writes is a first write paying the absence proofs of D7 and D8,
and the collected message body is written twice: at intake and again at
execution. The spec text that justified durability ("Restart", "Staging in a
snapshot") was written with E8, not agreed, and is withdrawn.

**Size**: medium. The staging structures move to memory behind the same
questions (`Hold`, proven set, anchor staging); the durable records and their
snapshot enumeration go; a joining node's replay of the committed stream from
its last executed block is #4205's restart recovery, which becomes the
precondition for executing.

---

### E11. A node cannot sync from the running protocol

*[#4205](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4205)*

**Spec** ([executor.md](executor.md), "Sync"): every node, validator or
follower, pulls the state of the chains down from the running protocol,
verified against the anchored root, while collecting messages from consensus
into staging, and processes transactions only once the state matches and
staging holds what its peers hold.

**Code**: a node starts from genesis or from a snapshot file it was given, and
consensus "catches up" by fetching batches from peers' retention
(`pkg/consensus/recovery.go`, `DefaultCatchUpTimeout` 60 s). A peer further
behind than retention is told `absence=no-record` and has no way back; a
validator restarted under load could not rejoin and stalled its partition
(#4205, run `20260903T202621Z`). Nothing pulls chain state from peers, nothing
verifies it against an anchored root, and nothing gates execution on staging
being complete.

**Size**: large; it is the precondition for E10 (staging in memory) and for
chaos returning to a soak.

---

## Database abstraction

### D1. Record placement is a second, hand-maintained model

*[#4199](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4199)*

**Spec** ([database.md](database.md)): a store maps a key to an opaque value
and does not interpret it.

**Code**: `pkg/database/keyvalue/bcdb/route.go` classifies records as write-once
or mutable by inspecting key shapes — a second model of the record model,
maintained by hand and not derived from the first.

**Evidence**: wrong twice, both found in soaks rather than by construction —
`Data.Transaction(H)` and the BSN's `ElementIndex(H)` (#4174), and
`Account(U).Url`, whose misplacement cost 96,303 deep history walks per BVN
engine over 200 commits (`c37c2eeb0`).

**Size**: medium. Either derive placement from the record model or make the
divergence detectable without a soak.

### D2. Badger does not verify isolation

*[#4194](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4194)*

**Spec** ([database.md](database.md)): a change set is isolated — changes are
invisible to anyone else until `Commit`. A backend is correct when it passes the
five `kvtest` cases, and one that does not run `kvtest` is unspecified.

**Code**: every backend runs `TestDatabase`, `TestDelete`, `TestPrefix` and
`TestSubBatch`. **Badger alone does not run `TestIsolation`** — v2 and v4 both
omit it, with no comment saying why. So a shipped backend does not verify the
invariant the record model depends on most.

**Size**: small — add the case and see whether it passes. If it does not, the
difference is larger than the test.

### D3. The window is not part of the backend contract

*[#4196](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4196)*

**Spec**: a windowed store answers ordinary reads from its window and a deep
reader reaches history; a backend that cannot answer a read must say so, never
guess.

**Code**: `kvtest` does not exercise `BeginDeep`. Nothing verifies that a
windowed backend answers a deep read correctly, or that an ordinary read reports
absence rather than guessing.

**Size**: small. A conformance test.

### D4. The bcdb window is advisory, so absence is never reported

*[#4200](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4200)*

**Spec**: an ordinary read is answered from the window; a read that needs
history requires a deep reader. A backend that cannot answer must say so.

**Code**: `getAt` (`bcdb/database.go:717`) falls back to `GetDeep` when a
**shallow** reader misses, counting the fallback rather than returning
not-found. So no shallow read ever reports absence, and the window is a
performance property rather than a contract.

This is deliberate and is documented in place: enforcing the window blind would
turn any read the adapter has not accounted for into a silent not-found, which
in the executor is a consensus fault. `DeepFallbacks` in `stats.json` is the
instrument — zero over a soak is the evidence that the fallback can be removed.

**Where it stands**: `Account(U).Url` was the only shape falling back (96,303
over 200 commits, ~482 history walks a block); routing it to the dynamic layer
took the count to none. So the evidence for enforcement now exists and has not
been acted on.

**Size**: small, and it depends on D3 — enforcement without a conformance test
for `BeginDeep` swaps a measured fallback for an unverified one.

---

### D6. The adapter walks permanent history for a permanent shape's miss

*[#4219](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4219)*

**Spec**: a mutable record is answered by the dynamic layer alone; a permanent
record from the window; nothing reaches into history to prove an absence.

**Code**: `getAt` (`bcdb/database.go`) no longer walks history for a mutable
shape — the dynamic layer's miss is the answer — and counts every shallow miss
by shape (`ShallowMisses`, `FallbackWalks` in `stats.json`). For a permanent
shape it still falls back to `GetDeep`, because the readers that legitimately
reach past the window have no deep batch (E9). Run 20260904T221627Z, with
the rule in: 113.8 M walks over eight BVN stores in 40 minutes, 99.2% of
them proving a key absent before its first write (D7, D8, and a dead read of
the v1 `Transaction.Main`), ~6,300 a block a node; the reads history actually
answered were dispatch's (once per key) and the root-index search (63 reads
per key).

**Size**: the rest is E9: once those readers take `BeginDeep`, the branch goes
and the permanent misses must read zero over a soak.

---

### D7. A first write reads the store to learn a version the store does not hold

*[#4219](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4219)*

**Spec**: a first write never reads the store to learn a version.

**Code**: `value.Put` (`values/value.go:164`) calls `Get` when the value is
unloaded, "for proper versioning". The read's value and status are discarded;
only `version` survives, and it is copied from the parent batch's in-memory
record — `LoadBytes` never sets it, so the store read contributes nothing.
Every first write of a record therefore costs a full miss. A naive skip is
wrong: a shard child writing a key its parent wrote earlier in the block would
raise a spurious conflict and poison the block. The fix is a version-only
fetch that resolves the parent record without reading the store.

**Size**: small in code; the proof is the A/B golden run plus unit tests for
the child-after-parent and sibling-conflict cases, which do not exist.

---

### D8. The chain checks uniqueness on every append, and the flag that governs it is ignored

*[#4219](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4219)*

**Spec**: chains unique by construction are appended without asking; the
writer deduplicates the rest.

**Code**: `merkle.Chain.AddEntry` reads `ElementIndex(hash)` for every chain
before consulting `unique`, and `Element(count)` and `States(count-1)` are
blind writes that pre-read (D7). `ChainUpdates.AddChainEntry2` passes `true`
whatever its caller said. Instrumented over the e2e and executor suites, the
only rejected duplicates were the double append of E9 and v1's signature path;
every other duplicate was a permitted repeat on a root or signature chain.

**Size**: small once D7 is in: a per-chain mode for the chains the writer
deduplicates, the flag honoured or removed. `merkle.OnDuplicate` now records
every duplicate append in test builds and the e2e suite fails on any outside
the permitted repeats, so a removal here is checked by the suite.

---

### D9. Restore writes the element index as the last occurrence

*[#4219](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4219)*

**Spec**: the element index is the first occurrence, preserved across restore.

**Code**: `merkle_snapshot.go` and `rebuildChainIndexes` write
`ElementIndex` in order, so a later duplicate overwrites the earlier index;
a live append keeps the first. The BSN indexer preserves first occurrence
deliberately because the store classifies the index write-once. Not consensus
state; the test that guards restore uses unique values only.

**Size**: small.

---

## Consensus

### C6. Consensus outruns execution, and own batches pin until execution

*[#4215](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4215)*

**Spec** ([consensus.md](consensus.md), invariant 9): a validator whose
executor is more than a bound of blocks behind the last commit proposes empty
headers and refuses user work until it catches up.

**Code**: the proposer never reads the executor's height. Committed
certificates queue in `committed` (`DefaultCommitBufferSize` = 5,000 blocks)
"and the DAG regardless" (`pkg/consensus/consensus.go`, commit loop); own
batches are released by `PruneCommitted` when their block **executes**, so
while execution lags every own batch of the lag is "uncommitted", the own
store exceeds its share through `Submit` (system traffic, never refused) and
`SubmitUser` refuses indefinitely, with no reason distinguishing a full store
from execution lag (invariant 10).

**Evidence**: run `20260904T035906Z` at 04:48. BVN1 voting on rounds
5,794–5,911 while executing blocks whose leader round was 3,472–3,554; BVN2
5,787–5,904 vs 2,532–2,568; the Directory executes the round it certifies.
Consensus 118 rounds a minute on every partition, execution 84 (BVN1) and 36
(BVN2). Own store 50 MB against a 32 MB share on BVN1, growing 1.1 MB a
minute; `batch_store_refusing=1` on both BVNs from 04:25 to the end; user
throughput 7 tps. Transactions executing at 04:45 had been submitted before
04:25.

**Consequence**: an executor slower than consensus — here because the healer
was most of each block (the old healer, since deleted) — turns into a permanent refusal that reports the
wrong thing, twenty minutes of submit-to-execute latency, and unbounded
memory in the commit queue and own store.

**Size**: small — the executor's executed leader round must be reported to the
node after each commit (no such feedback exists today); the header builder skips `ConsumeAvailableBatches` and the worker sets
refusing when the lag exceeds the bound. A test: an executor that executes
one block in three keeps the DAG within the bound and the own store within
its share.

## Healing

### H1. The producer cache does not exist

*[#4193](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4193)*

**Spec** ([healing.md](healing.md), "The cache"): a partition keeps every
synthetic and anchor it produced over the healing window, keyed by hash and by
stream position, and serves every heal request from it; a miss is a counted
defect.

**Code**: no cache on either side. The sequencer rebuilds message, receipt and
signature from the database on every request (`getSynth`,
`getDirectoryReceiptForBlock`): 35% of a source's CPU at hour one of run
`20260904T012004Z`. Dispatch reads every body and companion by hash and
searches the root index chain for the block's position
(`sendSyntheticTransactionsForBlock`, `getRootReceiptForBlock`) — the reads
the cache exists to prevent. Run `20260904T221627Z`: 442,652 body reads and
424,392 root-index reads answered from history on eight BVN nodes in 40
minutes, the only history reads that found anything. One cache was built earlier in the BlockchainDB adapter and
removed — on the storage read path it answered 0.40% of lookups, because it
cached the executor's reads rather than what healing asks for.

**Size**: small for the cache itself; it is the foundation of H8.

### H8. The node has no synthetic healing; the spec's lives in staging and is unbuilt

*[#4216](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4216)*

**Spec** ([healing.md](healing.md)): staging computes the gaps by index; a
selected validator sends one request naming hashes and index spans; the
source answers from its producer cache with a bundle; intake takes it. It is
the recovery for **dropped** entries; with nothing dropped it does nothing.

**Code**: none. The conductor's synthetic healer — per-number pulls through the
source's sequencer, a reconcile by the source's `Produced`, range recovery
under source roots that no destination could accept — was deleted on
2026-09-04 (`issue-4217-two-store-staging`): it was not the spec's mechanism,
it was most of every block under load, and it hid staging's own defects by
re-delivering what dispatch had lost. What remains in the conductor is the
anchor signature re-send on the cadence, which is a validator's own
contribution and not healing. The end-to-end tests that drop an entry and
expect recovery are skipped, named, and are H8's acceptance tests.

**Consequence**: until H8 is built, a lost synthetic is a stalled stream with a
visible gap between received and delivered — which, with no drops, is the
measurement of dispatch and staging that the old healer prevented.

**Size**: medium. The producer cache (H1), a hash-set and span request on the
sequencer service, bundles through the dispatcher, the decision in staging with
two cycles of patience, the counting table.

### H3. Proof extension does not exist

*[#4192](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4192)*

**Spec** ([healing.md](healing.md)): a destination that needs more reach asks for
an extension — a stream and two indices — and the source answers with the merkle
state at the new start and the intervening hashes. The same request fills holes
in a held proof, not only its tail.

**Code**: no extension request exists.

**The trigger is not lag.** This entry used to say a destination further behind
than `MaxReceiptListElements` (4,096) could not be covered at all, and cited the
8,556-deep gap of soak `20260902T132651Z`. That is wrong, and the correction
matters because it decides whether this is urgent.

A collection proof spans from the requested range to the **block boundary
covering it**, not to the chain head: `SequenceRange` builds
`GetReceiptList(chain, indices[0], mainAnchorEntry.Source)` where
`mainAnchorEntry` is found by `SearchIndexChain(..., MatchAfter, ...)` on the
LAST requested index, and the send path says the same thing —
`packageSpanFits` bounds "from its FIRST member to the block's last synthetic
element". Every enforcement point bounds the RANGE (`sequencer.go:512`,
`collection_proof.go:36`, `synthetic.go:526`, `anchoring.go:400`), and healing
chunks at `syntheticHealBatch` regardless. So proof length does not grow with
how far behind a destination is.

The soak agrees: 44,206 heals with **errors 0**. A bound that was refusing
8,556-deep pulls would have produced errors. The gap was deep because staging
refused receipts (E1), not because proofs were too long.

**What does trigger it**: a single block producing more than ~4,096 synthetics
to one destination, so a range starting early in that block spans past the cap
to the block's end. That is a throughput condition. The SEND path already has a
fallback — `packageSpanFits` ships an oversized message with its own receipt —
and the HEAL path has none, which is the actual hole.

**Size**: medium, and **not urgent until measured**. A message type, a request
path, and assembly at the destination. Build it when a run shows a proof-length
rejection, which names the real trigger rather than a supposed one.
