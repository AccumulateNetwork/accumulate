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

### E7. The block ledger is a paged log that rewrites itself, or an account per block

*[#4202](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4202)*

**Spec** ([executor.md](executor.md), "The block ledger"): a chain on the
system ledger account plus one keyed record per block. Written once, cost
bounded by the block's contents (invariant 9), committed by the ledger account's
hash, no migration at activation.

**Code**: two forms, neither of them that.

- **Jiuquan and later** (`block_end.go:180`, the only form the DAG-BFT line has
  ever written): `ledger.BlockLedger().Append(...)` into an `indexing.Log` with
  a page size of 4096 (`internal/database/utils.go:41`). Each entry carries the
  block's whole `BlockLedger` **inline** in the level-0 page, and `Append`
  writes the **entire page** back on every block (`pkg/database/indexing/log.go`,
  `append2`). The page grows by one block's entry list per block until it fills
  at 4096 blocks. The page's key (`...BlockLedger.Head`) is rewritten every
  block, so it sits in the dynamic layer. The log is not in the account hash
  (`observer_prod.hashState` hashes main, secondary, chains, pending), so the
  state root stopped committing to block ledgers when this form arrived.
- **Before Jiuquan** (mainnet, at Vandenberg): a `protocol.BlockLedger`
  **account** at `<partition>.acme/ledger/<index>`, one BPT entry per block,
  permanently. Mainnet's Directory is at height 35,168,475 (2026-09-03).
- **Reads** (`internal/database/indexing/block.go`): `Find(...).Exact()` on the
  log decodes the whole level-0 page — up to 4096 block ledgers — to return
  one.
- **#4147** proposes activating the log form on mainnet, with the in-band
  migration at `block_end.go:295-332`: for every past block, append a
  placeholder to the log and delete the account's BPT entry, in one block. The
  characterization tests pinned that it is not idempotent, that the transition
  block keeps its own BPT entry, and that genesis writes an account regardless
  of version. `metrics.go:63` still queries the account form.

**Evidence**: soak `20260903T121819Z` (bcdb, 1 s blocks, 500 tps). The
marshaled log page was 41–46% of the live heap on every node (776 MB on
`acc-bvn2-val1`) and the largest allocation site in our code (19.9 GB of
197 GB). Measured against the real code with 1000 entries per block, the page is
27 MB at block 700 and 9.5 GB have been written cumulatively; both grow with
height. The nodes hit GOMEMLIMIT in ten minutes, GC ran eight times a second,
CPU went to six cores, and the run stalled at 0.26 h. Review:
`test/docker/soak/runs/20260903T121819Z/review-memory-cpu.md`.

**Consequence**: the cost of a block grows with the height of the chain, which
breaks invariant 9 and is the wall every 500 tps soak now hits; the block ledger
is not consensus state; and mainnet cannot reach Kourou without either running
this form or a one-block migration of thirty-five million entries.

**Size**: medium. A chain and a keyed record on the ledger (model.yml), the
write at `block_end.go:180`, `LoadBlockLedger`, a write-once case in `route.go`
(D1), the metrics query, and a version gate. Delete `indexing.Log` — the block
ledger is its only user. The #4147 migration is not needed and should not run.

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

### D5. A bcdb commit returns before its data reaches the store

*[#4203](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4203)*

**Spec** ([database.md](database.md), invariant 5): durability is the commit
of the outermost change set. Invariant 2: a change set is isolated.

**Code**: `bcdb.commit` appends the batch to `d.staged` and calls `drain`, which
writes through only the staged batches that no open reader predates
(`database.go:504`, "A reader predates this commit and must not see it"). If any
batch begun at an older version is still open, `commit` returns nil with the
data in memory and not on disk — `reportStats` says so in its own words:
"nothing staged is on disk". `closeView` does not drain; the next commit does.
Isolation is bought by delaying durability instead of by versioning the read.

**Evidence**: run `20260903T121819Z`, `stats.json` at commit 700 on every BVN
database: `stagedCommits: 18` (the DN: 2; earlier bcdb runs: 2–3). Eighteen
committed blocks were in memory and not in the store, and with them eighteen
copies of the E7 log page — the 776 MB `indexing.(*Block).MarshalBinary` line in
the heap profile. The reader that held the version is not identified; the dumps
were taken after the stall. `getAt` also walks the staged list newest-first on
every read, so a deep queue slows every reader.

**Consequence**: a crash with N staged commits loses N committed blocks from the
store while consensus has them; memory held per staged commit is the size of a
block's writes, so a long-lived reader turns into unbounded heap; and there is
no metric for either — the depth is in a stats file rewritten every 50 commits.

**Size**: medium. Either write through at commit and serve an open view from a
per-view overlay (memory still held while the view is open, but durability is
restored and bounded by the view, not by the writer), or version the store's
reads (`view_kv.go` in BlockchainDB may already be that). In both cases: export
staged depth and the age of the oldest open view, tag views with their opener,
and add a `kvtest` case — commit while a view is open, reopen, the data is there.

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
