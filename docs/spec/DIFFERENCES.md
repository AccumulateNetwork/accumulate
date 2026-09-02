# Differences — where the code and the spec disagree

The specification says what we are doing. It does not describe the
implementation's departures from it, because a spec that documents its own
exceptions stops being normative.

This document holds those departures. Each entry names what the spec requires,
what the code does instead, and the evidence. It is the working list that
issues are written from once a part of the spec is settled — not a substitute
for them, and not a backlog in itself.

**An entry is removed when the code matches the spec, not when an issue is
filed.**

---

## Executor

### E1. Staging state lives in an account

**Spec** ([executor.md](executor.md)): staging belongs to the executor.
Everything received is held until it can be processed; a message that reaches
staging has been accepted, and accepted means recorded; block state never feeds
back into staging.

**Code**: `PartitionSyntheticLedger.Pending` is main state of an account of type
`AccountTypeSyntheticLedger`. It is hashed into the BPT and rewritten whole
every block, so the array must be bounded — `MaxPendingSequenced = 4096`. Past
that bound `stream_position.go:174` refuses to record the receipt, silently
(`return nil`, logged at Debug).

**Consequence**: the node holds a message and reports not holding it. The
reconciler believes the ledger and the healer re-fetches across the network what
is already in the local database.

**Evidence**: soak `20260902T132651Z` — 8,556 distinct sequence numbers
re-fetched 53,011 times, some 41 times each; `recv − deliv = 4,096` exactly on
two samples eight minutes apart; 44,206 heals with `errors 0`, because nothing
errors. All three partitions stayed live throughout.

**Size**: large. Moves staging out of the account model and removes the bound.

### E2. `isReady` reads block state

**Spec**: block state never feeds back into staging.

**Code**: `isReady` consults a stream position derived from the synthetic ledger
account — the record the block writes.

**Size**: follows from E1.

### E3. Staging position after a restart is unspecified

**Spec**: silent. Once staging stops reading the ledger, something must define
how the executor's position is recovered.

**Code**: the position comes from the ledger, so the question does not arise
today.

**Size**: design question, blocking E1.

### E4. An anchor's quorum is assembled by execution

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

### E5. `CascadeDeliveryQueue` is dead state that is still hashed

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

**Spec**: a windowed store answers ordinary reads from its window and a deep
reader reaches history; a backend that cannot answer a read must say so, never
guess.

**Code**: `kvtest` does not exercise `BeginDeep`. Nothing verifies that a
windowed backend answers a deep read correctly, or that an ordinary read reports
absence rather than guessing.

**Size**: small. A conformance test.

---

## Healing

### H1. The healing cache does not exist

**Spec** ([healing.md](healing.md)): healing caches what it fetches, keyed by
source, destination and sequence number; only healing uses it; it lives in
Accumulate and is indifferent to the storage backend.

**Code**: no such cache. One was built in the BlockchainDB adapter and removed —
on the storage read path it answered 0.40% of lookups, because it cached the
executor's reads rather than the healer's fetches.

**Size**: small.

### H2. Healing is not ordered

**Spec**: heal newest gap to lowest, and skip what is already staged locally.

**Code**: neither. The healer re-fetches messages the node already holds, in no
particular order.

**Size**: medium, and entangled with E1 — until the ledger stops disagreeing
with the database, ordering only makes the loop more efficient.

### H3. Proof extension does not exist

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
