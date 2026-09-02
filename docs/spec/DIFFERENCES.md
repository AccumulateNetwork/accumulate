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

## Healing

### H1. The healing cache does not exist

*[#4193](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4193)*

**Spec** ([healing.md](healing.md)): healing caches what it fetches, keyed by
source, destination and sequence number; only healing uses it; it lives in
Accumulate and is indifferent to the storage backend.

**Code**: no such cache. One was built in the BlockchainDB adapter and removed —
on the storage read path it answered 0.40% of lookups, because it cached the
executor's reads rather than the healer's fetches.

**Size**: small.

### H5. Requests are generated outside staging, per node, with jitter

*[#4201](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4201)*

**Spec** ([healing.md](healing.md)): requests are computed from staging at a
block boundary — the moment the gap set is final — on the blocks where healing
activates. Every validator computes the
same gaps and therefore the same requests, and staging dedupes several askers of
the same gap into one request. Two of them send, selected by the previous
block's hash, and send immediately.

**Code**: generation lives in the `Conductor`, outside staging and outside the
block. `claimSyntheticRequest` schedules a per-node **random jittered delay** on
first sight of a gap and enforces a back-off, "so a stalled stream isn't
hammered by every validator on every block", relying on the first answer landing
before the other validators fire. No pair is selected: every validator
eventually fires for every gap, and the jitter only staggers them.

That is a heuristic standing in for agreement the system already has. Every
validator sees the same consensus stream and could compute the same request set
directly; the jitter exists only because the computation was moved out of the
place that knows.

The `Conductor` also carries machinery the design does not need: a per-source
back-off, a failure breaker, and per-gap scheduling state. With two senders and
an activation every few blocks there is no load to manage, and a lost request
costs nothing because the gap is still a gap at the next activation.

There is no cadence either: the `Conductor` scans on its own schedule rather
than activating on a block, so "when to ask again" is answered by the back-off
window (`syntheticHealWindow`, 10 s) instead of by a number of blocks.

**Size**: medium, and it lands with H4 — both are consequences of healing asking
staging rather than reading what the block wrote. Most of it is deletion.

### H2. Healing is not ordered

*[#4191](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4191)*

**Spec**: heal newest gap to lowest, and skip what is already staged locally.

**Code**: neither. The healer re-fetches messages the node already holds, in no
particular order.

**Size**: medium, and entangled with E1 — until the ledger stops disagreeing
with the database, ordering only makes the loop more efficient.

### H3. Proof extension does not exist

*[#4192](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4192)*

**Spec**: a destination that needs more reach asks for an extension — the last
hash of the proof it holds, that hash's index, and how far back is wanted — and
the source answers with the earlier merkle state and the intervening hashes. The
same request fills holes in a held proof, not only its tail.

**Code**: no extension request. A destination further behind than
`MaxReceiptListElements` (4,096) cannot be covered at all: the sender will not
build the package (`packageSpanFits`), the sequencer will not serve the range,
and the receiver would reject the proof.

**Evidence**: the gap in soak `20260902T132651Z` was 8,556 — more than twice the
cap.

**Size**: medium. A message type, a request path, and assembly at the
destination.
