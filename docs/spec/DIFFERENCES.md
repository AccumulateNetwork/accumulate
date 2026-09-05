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
(`anchor_staging.go`), and since step 3 the proven set refuses conflicting
proofs; since E10 both live in memory (`internal/core/execute/staging.go`) and
are released as the stream delivers. A proof does not carry
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

### E9. The observer reads a v1 record, and every signer's set is cleared

*[#4219](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4219)*

**Spec**: a first write never reads; a reader that reaches past the window
takes a deep reader.

**Code**: the transaction hash now reaches a chain once per transaction —
`AddChainEntry2` settles a second append to the same chain from the
transaction's own chain-update record, not by reading the chain, and honours
its `unique` argument (`test/e2e/single_append_test.go`, nine transaction
types). What remains: the observer reads the v1 `Transaction(h).Main` record
for every pending txid (`observer_prod.go:121`), a shape v2 never writes,
which is a cheap in-window miss now; `clearActiveSignatures` writes
`Signatures` of every signer in the book, absent or not; the main and scratch
chains still read their element index on append (`unique == true` from
`AddChainEntry`) as a guard against a repeat across transactions, which no
path is known to produce.

**Size**: small.

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

**Size**: large; it is the precondition for a validator restarting under load and for
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

### D7. A first write no longer reads the store; one store cannot say a version

*[#4219](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4219)*

**Spec**: a first write never reads the store to learn a version.

**Code**: done. `value.Put` asks its store for the version alone
(`database.VersionStore`): the key-value store below the outermost batch
answers zero, a batch answers from the parent's record in memory, a shard's
child answers through the parent under its mutex. The read remains only as a
fallback for a store that cannot say (the BPT's node records, which are in
memory). Proven by: a counting store showing a first write reads nothing and a
set merge reads once; the conflict cases a naive skip would break (a child
writing a key its parent wrote, siblings, three levels, shards); and a
differential run of accounts, chains, sets, child batches and shards under
both paths committing byte-identical state and the same root
(`internal/database/version_fetch_test.go`).

**Size**: none remaining.

---

### D8. The chain reads its element index only for the writer's deduplicated chains

*[#4219](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4219)*

**Spec**: the element index is written, not read then written; the writer
deduplicates.

**Code**: `merkle.Chain.AddEntry` writes the element index blind for every
chain appended with `unique == false` — root, signature, index, synthetic,
replica, anchor-sequence, block-ledger and BPT chains, the bulk of the
appends — and reads it first only for `unique == true`: the account main and
scratch chains through `AddChainEntry`. `AddChainEntry2` honours its argument,
so the anchor root and BPT chains write blind, and a transaction's second
append to the same chain is settled from its own chain-update record (E9).
The remaining read is an in-window miss for a new hash, a guard against a
repeat across transactions that no path is known to produce; it goes when
the e2e duplicate assertion has run clean long enough to say so. The
`Element` and `States` writes read nothing since D7. The element index names
the last-written occurrence live as it does after a restore, so the former D9
is closed.

**Size**: none required; the guard read is a choice.

---

## Consensus

No recorded differences.

## Healing

### H1. The producer cache exists; what it is not yet cleared by, and what still reads the store

*[#4193](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4193)*

**Spec** ([healing.md](healing.md), "The cache"): the producer keeps every
synthetic and anchor in play, keyed by hash and by stream position, with what
its proofs are built from; dispatch and healing read it and nothing else;
cleared as the destination delivers; a miss is refused and counted.

**Code**: built (`internal/core/synthcache`). Dispatch and the sequencer's
answers are built from it alone; the e2e suite fails on a dispatch miss.
Remaining: the cache is cleared by a horizon of blocks, not by the
destination's delivered index, because no signal carries that back to the
producer; the v1 simulator's sequencer still reads the store, since the v1
executor has no cache; the requester side (H8) does not exist, so nothing
asks the sequencer yet; the Directory receipt a block was dispatched under is
the only anchor a bundle can be proven under (`ProveAgainstAnchor` for any
other is `NotReady`), which is H3.

**Size**: small; the delivery signal is the open design point.

---

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
