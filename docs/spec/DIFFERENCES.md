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
and its own signature. Each is a complete message execution with its own
status, and the copy that crosses `ValidatorThreshold` executes the anchor. For
an N-validator partition, N−1 deliveries exist only to deposit a signature.
Copies cannot deduplicate because each embeds a different signature and
therefore hashes differently.

What a copy writes is now bounded by what it adds (#4224, executor.md "What a
copy costs"): the body once under the transaction's hash, the copy as a
signature over a reference, one signature-chain entry per distinct signer, the
signature set once per block from the block's view of it
(`anchor_signatures.go`). What remains of this difference is the shape:
copies are still messages with statuses, and the quorum is still evaluated by
execution rather than collected by staging. Staging already asks the right
question — `admissibilityOf` calls `Block.anchorIsAdmissible`, the same rule
`txnIsReady` uses (#4169 step 3b) — but has nothing to collect.

One consequence of storing a copy as a reference: the API renders a signature
chain entry by loading the message under the entry's hash and recomputing its
ID (`load.go`), so an anchor copy's ID in the signature set differs from the
chain entry that names it. The signature itself is what the set is for, and it
is intact.

**Size**: medium. Cost is O(validators) per anchor in statuses and chain
entries; bodies and set writes are O(1) per anchor per block.

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

### E12. What a transaction still writes beyond one body, one status, one set

*[#4236](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4236)*

**Spec** ([database.md](database.md), "A record is written once per thing it
records"; [executor.md](executor.md), "The database write"): one body per
transaction, wrappers referring to it by hash; a status per message with an
outcome; `Produced` one set under the transaction; `Cause` kept as its
inverse.

**Code**: the destination now writes, keyed by a synthetic transaction's hash,
six records — `Message.Main`, `Transaction.Status`, `Message.Cause`,
`Transaction.Chains`, `Account.Payments`, `Account.Votes`
(`TestUserTransactionWrites`) — and the wrapper's own `Main` and `Status`
under the wrapper's hash. What remains beyond the spec's shape:

- **The source stores the sequenced message with the full body**
  (`buildSynthTxn`). The sequencer serves healing answers from that record
  (`sequencer.go getSynth`, `getSynthRange`) and the cache seed rebuilds from
  it, so a reference there would make every answer resolve a second record.
  Removing it is H1's work (the cache serves, the store does not), not a
  write-path change.
- **Wrapper statuses stay.** Each `SequencedMessage`, `SyntheticMessage`,
  `BlockAnchor`, `CreditPayment` and `SignatureRequest` writes its own status,
  because `checkStatus` reads it: a sequenced message re-run from staging and a
  copy landing in two blocks are caught by it. Whether staging's delivered
  index can carry that dedup alone — the spec's table already names it for
  sequenced entries — is the open question; until it does, the status is the
  record.
- **`History` and `Signers`** are written per signature (`RecordHistory`) and
  read by the API's signature-set view (`load.go`); the review proposed
  deriving them from the signature chain. Not done here.
- **`Payments` and `Votes`** are written per transaction by the transaction
  path and read by the account hash (`observer_prod.hashPendingV2`) for
  pending transactions; a delivered synthetic writes both for nothing.

**Size**: measured on run `20260905T153920Z` before this work, 72 records per
user transaction; the items above are the ones still to measure after it.

### E14. Naming the synthetic chains in the block ledger is not gated on a version

The rule is not "before the change versus after it": `c2b0e9d2f`, `44490e380` and `481dc3f32` produce three distinct state roots on the same deterministic workload, so all three are mutually incompatible. If any node, image or database anywhere was built from the intermediate commit, rebuilding together includes that one.
**Spec** ([executor.md](executor.md), "The block ledger", *Activation and
history*): what a block records changes the ledger account's hash, so it is
gated on an `ExecutorVersion` like any change to what a block produces.

**Code**: building the block ledger after `anchorSynthChains` — so that it
names the partition's synthetic chains and the entries they gained — is
unconditional. Two binaries on the same chain, one with the change and one
without, produce different state roots for any block that produced a synthetic
message, and fork.

**Why it stands**: `dagbft-integration` is not deployed, and every node on this
line is rebuilt and restarted together, so no mixed-binary window exists. That
assumption is the whole of the justification. The decision to leave it ungated
is Paul's, not this change's; the entry exists so that the day this line does
carry a deployed network, the gate is known to be missing.

**Size**: small — an `ExecutorVersion` predicate around the ordering and the
naming, if the assumption ever stops holding.

### E15. A held synthetic copy recording nothing is not gated on a version

*[#4423](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4423)*

**Spec** ([executor.md](executor.md), "Versioning"): behaviour that changes
what a block produces is gated on an `ExecutorVersion`.

**Code**: `SyntheticMessage.process` returns `errCollected` when its inner
message comes back pending (`heldOnly`), so a synthetic copy that was only
held records no outer message, no status and no validator signature
(invariant 12). Before #4423 it recorded all three at arrival. Unconditional:
any block that held a synthetic out of order has a different state root on
either side of the change, and two binaries on one chain fork there.

**Why it stands**: the fresh-install rule of E14 — `dagbft-integration` runs
no network that outlives a run, every node is rebuilt together, so no
mixed-binary window exists. The old behaviour is also the defect: it froze a
stream for good.

**Size**: an `ExecutorVersion` predicate in `heldOnly`, if the assumption ever
stops holding.

### E11. A node cannot sync from the running protocol

*[#4205](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4205)*

**Spec** ([executor.md](executor.md), "Sync", as rewritten 2026-09-19): a
node that joins — or restarts, which is a join — validates the spine first
(the network definition and the anchors its validators sign, by signature,
to a threshold of distinct members of the producing partition's set — #4301
statement (b); the operators' book keeps governance and is not read — anchors
routed by producer), pulls
the peers' *current* state and keeps it once the root it is served at is
proven — it equals a verified anchor's `StateTreeAnchor`, or the bpt chain's
history from one such root to it hashes into the root chain anchor a later
verified anchor carries (rewritten 2026-09-21 on the owner's decision, the
mechanism settled 2026-09-22; until
then it read "served *as of that anchored block*") — and collects consensus
from the moment it listens; staging
is what was collected minus what the pulled state says executed, and the
node executes the next block when no stream has a gap, else advances the
sync one block and asks again. **No peer is ever asked what it holds.** The
paragraph this replaced ("processes transactions only once the state matches
and staging holds what its peers hold") was the first pass's design, kept
below as the record of what was built.

Two departures the rewritten section names that this entry did not:

- **Producer routing** — *retired 2026-09-19 (#4301 statement (a))*: the
  Directory anchors to itself, so its own root is in `dn.acme/anchors` with
  real signatures; the inference that it was unobtainable there was wrong.
  What was different until #4301 landed (`1ea77143d`): `pull.DirectoryAnchors`
  read roots off `dn.acme/anchors` with no signature checked at all; it is
  deleted, and a root reaches the tracker only after a threshold of distinct
  members of the producing partition's set have signed it.
- **The authority is the network definition, not the operators' page**
  (#4301 statement (b)): the spec now says so; the operators' page keeps its
  governance role and is never read by a join.
- **Anchored-height serving** — *retired 2026-09-19 (#4361, merged
  `b0ec6e0fd`)*: a peer now serves an account and a BPT page as of a block
  the Directory anchored, with a receipt terminating at that block's
  `StateTreeAnchor` and the body as of that block (a mismatch is a refusal,
  never a wrong answer); out-of-window is `IncompleteChain`; retention 1024
  blocks on by default; the follow-up (`5894fc61a`) checks a historical
  page by a leaf and every block against the one above it, and a joined
  node whose main index chain starts at its open mark answers "did this
  account exist then" from the BPT instead of turning its own store miss
  into `NotFound`. **The join does not use it** (#4362, the owner's decision
  2026-09-21): a past leaf cannot be rebuilt for an account whose chains,
  directory or pending list moved, which is every account a restarted node
  lacks, so the pull asks for current state and proves the root it was served
  at through the history (`anchorsrc.ProveRoot`). The hold-and-discard
  (`maxSettleRounds`, `settleBatch`, the re-fetch of held accounts — #4352,
  #4353) is deleted with it, and
  `TestRestartedNodeWithAPopulatedDatabaseResyncs` runs and passes. The
  threat-review point stands in its new form: the block the node's state *is*
  is read from the ledger under the proven root, never from the peer-asserted
  `LocalBlock`, which is used only to break a tie between two roots of one
  pass and to tell a root the history has passed from one it has not reached. Retention's cost on BlockchainDB is unmeasured
  (#4165).
- **The bpt chain's index is the peer's word** — *retired 2026-09-22
  (#4362)*. Found 2026-09-21: `ProveRoot` found the root chain entry from the
  signed height and then compared it with the bpt chain's index entry as the
  same peer served it, unproven, so a peer that forged both the receipt and
  the index entry proved a transaction hash as a BPT root. The proof now reads
  no index: the signed root chain height alone decides which step of a
  receipt is a root chain entry (`splitAtLeaf`, `leafpath.go`), the base's
  receipt below that step rebuilds the bpt chain's merkle state, the entries
  from the base to the root are appended, and the rebuilt anchor must be the
  hash the root's own receipt enters the root chain at. The index a peer
  reports shapes the rebuild and the range asked for and enters no
  conclusion; `TestATransactionsReceiptIsRefusedThoughThePeerForgesTheBptIndex`
  runs and passes, and `TestAnAnchorsStateTreeAnchorIsTheRootOfItsBlock`
  pins that an anchor's `StateTreeAnchor` is the value the next non-empty
  block records on the bpt chain. **What stays open:** an interior node of
  the bpt chain, whose leaves are true roots, can pass for an entry when the
  peer also chooses the range it is asked for; it is not a value a peer can
  choose, and the states under it are old true ones.

**Code**: a node starts from genesis or from a snapshot file it was given, and
consensus "catches up" by fetching batches from peers' retention
(`pkg/consensus/recovery.go`, `DefaultCatchUpTimeout` 60 s). A peer further
behind than retention is told `absence=no-record` and has no way back; a
validator restarted under load could not rejoin and stalled its partition
(#4205, run `20260903T202621Z`). Nothing pulls chain state from peers and
nothing verifies it against an anchored root.

**Rejoin (#4290, done)**: a node that starts with its store intact rebuilds
staging before its first block executes — every inbound synthetic stream
pulled from its source from `Delivered + 1` up and held as a block would hold
it (healing.md, "Rejoining"), the source keeping released entries
`RejoinGrace` blocks for it. Before this, a restarted validator executed its
first block holding nothing while its peers executed what they held, and its
root chain never matched again: five restarts took the Directory below its
anchor quorum (run `20260917T223150Z`). **Not exact, and therefore not
done**: the source's cache holds what it has produced, not what the
destination's peers had received at the restart block. Run
`20260918T023054Z`: the pull handed the restarted node BVN3 → BVN1 entry 445,
still in flight to its peers, and it executed it at block 187 where its
peers executed it at 188 — one chain entry a block early, and the root chain
diverged again. The set a rejoining node must hold is "what its peers held
at block R": known only to the peers (a held entry's arrival block, which
staging does not record) or to the committed log (which is memory: ten
minutes of batches in the worker, certificates within `DAGGCDepth`). Either
is a decision on where a rejoining node reads from; the spec today says the
source's cache, never a peer's staging. Also not done: a source that no
longer holds the span leaves the node unable to rejoin by healing, logged,
not a state the node acts on; and a source whose own re-seeded cache cannot
continue its receipts (#4287) fails the pull for a minute, then the block
runs with the hole.

**Seeding (#4238, done)**: the service checkpoints its consensus position
per block and restores the one matching the executor's last block on restart
(consensus.md, "Restart"), so a restarted validator starts at its own round
rather than zero and certificate catch-up can reach the frontier within
`DAGGCDepth` (2,000 rounds, about eight minutes at four rounds a second). Not
done: a node down longer than that is beyond catch-up and only sync can bring
it back; certificates at or below the checkpoint's round that a later leader
commits are not pulled by catch-up, so the first block after a rejoin can
still differ from its peers' — sync must deliver staging and the DAG floor
together; the stranded condition is a Warn a minute, not a state. The
opposite defect is fixed (#4290): the checkpoint now carries Bullshark's
committed-digest set, without which the first leader after every restart
re-committed the whole rescue window below the floor (16 batches where the
peers committed 3, run `20260918T014155Z`).

**Removed (#4362, 2026-09-23)**: the staging API a validator served
(`Sequencer.StagingSnapshot`, the `PrivateStagingSnapshot*` messages and
their routing, #4291), the load path that consumed it
(`Executor.LoadStaging`, `Staging.Snapshot`/`Load`, #4292 step 2), the
simulator's `takeStaging`/`TakeStaging`/`CompleteJoin`, the daemon's
execute-from-own-state branch, and the pull's meeting point (ahead / level /
disagrees, #4348, and its hole #4350) are deleted, not bypassed:
`TestTheJoinAsksNoPeerForItsConclusions` (internal/node/join) fails on any
of those identifiers in the module's non-test source. A join collects into
its own buffer from the first block it hears, takes the buffer into its own
staging only through the block after the state it proved (#4398), and settles on state it proved
against a signed anchor; at an anchored height there is one correct leaf per
account, so there is nothing to meet in the middle. The departures recorded
here for serving staging — the unpinned page, `Block` as the last processed
index, `Delivered` as memory — no longer exist. Closed by removal: #4322,
#4323, #4324, #4325, #4326, #4354, #4357. The private API's
`StagingSnapshot`, `StagedStream`, `StagedEntry`, `StagedHash` and
`StagedProof` types, `StagingSnapshotter`, `StagingSnapshotRequest` and
`FetchStagingSnapshot` (`internal/api/private`) are deleted with them and
regenerated out of `types_gen.go`; no message type for staging remains.

**The handoff (#4294, in progress)**: the join's orchestration exists
(`internal/node/join`) and the DAG service can be handed off to — it leaves
collecting mode at Q, stands at Q, and produces what it buffered from Q + 1,
all inside the block production loop, which is the only thing that produces
blocks. `Conductor.Rejoin`, its metric and `Executor.Collect` are gone: a
restart is a join. The simulator's `RestartNode` starts a join and the node
is handed off through `join.Run` — the simulator's join state is the
`join.Buffer`, replaying what it buffered from Q + 1 — so
`TestOneValidatorRestartDoesNotDiverge` joins by the production loop, with
the proving anchor landing during the join (it cannot land before: the held
entries are a gap until the peers execute them, and the loop advances the
sync instead). A node started by `cmd/accumulated/run` joins
whenever it has executed a block before; a genesis-fresh node does not — which
is `nodeMustJoin(lastBlock) = lastBlock > GenesisBlock`, and was
`lastBlock > 0` until #4304. Genesis is not an execution but it does write the
ledger at block 1, so the old test was true of every node ever started.

**The messages behind pulled chain entries (#4400, 2026-09-24)**: until
this, the spine pull asked for chain entries unexpanded and nothing wrote the
message an entry names, so a restarted validator that fell one anchored block
behind could not open its first block: the producer-cache seed and
`lastAnchoredBlock` load the message behind anchor pool entries the node did
not execute (run `20260924T052134Z`, acc-bvn3-val1; reproduced by
`TestAJoinedNodeCanOpenItsFirstBlockAfterARestart`). The pull now takes a
spine transaction chain's messages with its entries, checked as executor.md
"Sync" §2–3 say, and the seed rebuilds a BVN's synthetics only for blocks
after the one the node joined at (`TestAJoinedBVNNodeCanOpenItsFirstBlockAfterARestart`
— the next failure the debugger predicted, reproduced before the fix). What is
still different, or not known:

- **Only the seed is known to keep the rule for state-only chains.** An
  account pulled state-only (`<partition>/synthetic`, every leaf account) has
  the entries of its open mark set and no message behind any entry of a block
  the node did not execute. The seed no longer reads them; no other reader of
  a message behind a chain entry (healing, the sequencer by position, the
  v3 querier's expanded entries) was audited for a block at or below the join,
  and one that reads there finds `NotFound`, which §6 says must be
  `NotReady`.
- **The seed skips a block on the store's evidence.** A block in the seed's
  window whose synthetic entries the store holds with no message behind them,
  or whose entries it does not hold, is taken as one this node did not execute
  and is neither rebuilt nor dispatched; each skipped block is logged with
  that evidence. Only that evidence skips a block: any other absence in the
  rebuild (a companion transaction, the root index chain) fails the seed
  (`TestAnExecutedBlockMissingACompanionFailsTheSeed`). It assumes an
  ordinary read of this node's store answers the message behind a synthetic
  entry of a block it executed. On leveldb and the in-memory store it does.
  **On BlockchainDB it does only within the store's read window**: nothing is
  deleted, but an ordinary batch reads a permanent record older than the last
  `DefaultMergeLag` (20) to 40 blocks as absent, and only `Deep()` sees it
  (`internal/database/database.go`, `pkg/database/keyvalue/bcdb`). The seed
  reads down to `InFlightBlocks` (8) below the newest Directory receipt of
  this partition's blocks, which is inside that window while the Directory
  keeps receipting; with no receipt in the horizon it reads down to
  `DefaultHorizon` (600) blocks, and there a block this node executed reads
  as one it did not and is skipped (and `ownReceipts`, which reads the
  anchor pool's messages down to the same bound with hard errors, may fail
  first). A skipped block is a dispatch this node does not make, never a
  wrong one. Two earlier versions on this branch were wrong and are recorded here:
  skipping every block at or below the join block left a restart that fell
  nothing behind with an empty cache — the #4241/#4277 restart hole (review
  note_3896114642, `TestAZeroGapRestartStillSeedsItsOwnBlocks`); skipping a
  span the join remembered in memory lost it with the process, so the second
  restart after a join failed its first block on the first join's gap
  (note_3896174331, `TestASecondRestartAfterAJoinStillOpens`).
- **A stored message is not content-addressed for wrappers.** The executor
  stores an anchor, sequenced or synthetic message that refers to its
  transaction by hash under the hash of the message as it arrived (#4236), so
  what a peer serves under such an entry never hashes to it. The pull proves
  it by resolving the reference — from the pass, or by `QueryMessage` to the
  same peer — and hashing the result; a reader that checks a stored message
  against its key without doing that will refuse every honest peer.
- **Messages no peer holds would make the spine unpullable.** A node asks only
  for the entries past its own height, so a restart asks only for its gap; but
  an entry whose message no peer holds — a network started from a snapshot
  without messages, or history pruned below a node's gap — is refused from
  every peer and the spine never settles. Not observed and not tested.
- **The producer's anchors are seeded from blocks the node did not execute.**
  The anchor sequence chain's messages now come with the spine, so the seed
  puts anchors produced while the node was away into its cache. An anchor is
  the partition's whoever produced it, and serving it is what a destination
  behind on the stream needs (#4277); it is stated in §6 rather than excluded.
- **A failed seed is retried, not survived.** The seed now counts only when
  it succeeds, so a seed that fails deterministically fails every block it
  opens, loudly, instead of the first one and then running on an empty cache.
  A failed handoff is no longer terminal (#4401): the DAG service goes back
  to collecting with the groups it did not produce, and the join demotes the
  node, syncs again and hands off again, counted
  (`accumulate_join_handoff_failures_total`; executor.md "Sync", step 5).

**The simulator's joining node (#4363)**: `RestartNode` builds the join's
state as `cmd/accumulated/run/dagbft.go` does at startup — `join.NewState`
over `join.QueryPeers` with the node's own peer excluded, from its executor's
last block — and the node's production querier refuses by that state's
machine, so a read addressed to a restarted or following simulator node
answers `NotReady` until its join hands off and promotes it to `ACTIVE` (#4385)
(`TestAFollowerJoinsARunningNetworkAndLeaves`,
`TestARestartedNodeRefusesReadsByItsJoinsMachine`). `Partition.StopNode`
takes a node out of the consensus hub and withdraws its services. What the
simulator still does not do as the daemon does:

- Only the querier is handed the machine. The daemon hands it to the
  sequencer, the submitter, the validator and the consensus service too; the
  simulator's sequencer answers a joining node's requests ungated.
- The harness's reads, and only the harness's, are steered to a node that
  can serve: a read that names no peer skips a node whose join machine says
  `BOOTING` (`services.Network.HarnessClient`, used by `harness.NewSim`).
  The daemon has no such oracle. It reaches a serving node only by its
  local-first dial — a client connected to a validator's API is answered by
  that validator (`p2p/dial` `newNetworkStream`) — and a `NotReady` from a
  joining peer is not redialed: it returns as an `ErrorResponse`, which
  `message.typedRequest`'s callback accepts, so the dial succeeds and
  `BadDial` is never reached. Sending a reader to a synced node is the next
  phase's work (step 6). The nodes' own routed client (`Network.Client`) has
  no oracle and can reach a joining node and be refused, as a daemon's
  non-local call can; what it does not model is a joining node's *own*
  routed reads, which the daemon answers locally with `NotReady`.
- A submission to any simulator node goes to the whole partition through the
  consensus hub (`Node.submit` → `Partition.Submit`). The follower test's
  relay assertion — the committee executed what the joining follower was
  handed — therefore proves the hub and not the daemon's relay
  (`cmd/accumulated/run/submit_relay.go`, #4366).
- `RestartNode` resets the node's staging, its join and its executor's seed
  latch (`Executor.ForgetSeed`, #4421), so the first block a restarted
  simulator node opens runs the seed over the store the join left, as a new
  process does: the anchor pool's entries and the messages behind them, and
  #4400's skip of blocks the node did not execute. Before #4421 the latch
  survived, a restarted simulator node never seeded again, and a join that
  left pool entries with no message behind them "passed". It does not rebuild
  the executor: the in-memory producer cache, the conductor, the dispatcher
  and the executor's other memory survive the "restart". The seed adds to a
  cache that still holds what the node produced before it (`Cache.Seed` skips
  a block already held), so what the seed alone would have held is not what a
  simulated restart dispatches and serves from.
- A simulator follower still votes and counts its own vote
  (`Node.isValidatorOn`), so stopping it cannot show that no quorum waited on
  it; the cadence assertion shows only that the partition runs on.
- A simulator node is one partition's node, not a process. Every BVN node
  has a Directory node beside it with its own key and peer ID
  (`test/simulator/factory.go`), and `RestartNode` and `StopNode` act on one
  partition only: the follower test restarts and stops the follower's BVN0
  node, and its Directory node never joins (never `BOOTING`, serves
  everything) and never stops. The daemon is one process for both, so a
  follower joins on both partitions and leaving stops both.
- The follower is forced into the join by `RestartNode` from genesis; the
  daemon would not join a genesis-only node at all (#4340).

**The exception is entering the join, not a flag inside it (#4304)**. The spec
says only a node that has executed no block may start without asking; on this
line such a node does not ask, because the daemon does not run the join for
it. `join.Options.Fresh` — a flag saying "execute even though you found
nobody" — is **deleted**: the daemon computed it inside `if joining`, where
`lastBlock == 0` was false by construction, so it was reachable from no caller
and true only in a hand-written test. `Run`'s `found == 0` is now
unconditionally `NotReady`, which is #4296's rule with nothing to switch it
off.

**A genesis-only node deployed into a running partition executes from block 1,
and that is wrong (#4340)**. It reads the same `lastBlock == 1` as the first
node of a new network and there is no local fact that separates them; the
difference is whether the partition has moved on, which is a network fact the
daemon does not ask for. Not reachable by any deployment path today —
`init` and netsim create every node of a network at once — and filed rather
than guessed at, because the obvious alternative (let a fresh node join and
ask) is what #4304 removed: in a fresh network every node is then joining,
every node refuses every other, and the network starts only when they all time
out.

**What the wiring does when no peer can answer**: every validator of the
partition is asked for its staging, and if none can serve any — which is what
a network that restarted as a whole looks like, since a node that has executed
no block since it started holds nothing anyone should start from — the node
executes from its own last block, producing what it buffered while it asked.
That is safe for exactly the reason the join exists: no peer holds an entry
this node lacks, because no peer holds anything.

**Serve last (#4295, partly done)**: a node that is joining refuses the
sequencer (`Sequence`, `SequenceRange`, `MajorHeaderRange`, `MinorRootRange`,
`PartitionRootRange`, `SnapshotRange`) and the staging snapshot with
`NotReady`, counted per call in `accumulate_node_not_serving_total`, and its
state is a gauge (`accumulate_node_state`). The state is the node's own,
handed to its services rather than looked up by partition: a process can run
several nodes of one partition — devnet does — and a registry keyed by
partition would give them all one node's state. A node that never joined has
no state machine and serves — **including `Submit` for a partition whose
committee it is not in, and it then drops what it accepted**: nothing on the
advertise, route, dial or submit path asks whether the node's key is in the
committee (`peer_manager.go:155-180`, `dispatcher.go:151-157`,
`dial/dialer.go:139-248`, `dagbft/api.go:186-198`); the accepted submission
goes to the node's own worker and header, which no validator votes on, and
nothing relays it. The spec (step 6, "A transaction is relayed, never
dropped, whether the node is following or syncing", Paul 2026-09-19) requires
the relay, and the code has none for a submission that reaches a
non-committee or syncing node's p2p submit service (its JSON-RPC `Submitter`
is wired to the network client and does dial onward,
`internal/node/http/handler.go:81-107`; the p2p service does not). "Not
advertised" would not close it: `connectedPeersDiscoverer`
(`pkg/api/v3/p2p/dial_network.go:133-160`) finds an installed handler by
libp2p identify ahead of the DHT. (#4366; run `20260919T191634Z`; reproduced
in-process, 1,249 accepted, 0 committed.) Landed 2026-09-19 (`a09ff3e0a`):
a node that cannot propose relays synchronously to a confirmed proposer; the
rerun `20260919T231856Z` read heals 0 → 0 and stranded 0 with 1,146 relayed
(#4365 note_3876639043). Still different: the relay target is bound to the
validator key but not yet to the peer that answered — the channel binding is
#4374's, a follow-up on the same branch; a keyless peer that never answers
is never demoted (#4374); `NotReady` from store-full is shopped ≤10× (bound
recorded, #4374).

- **A non-committee node's primary and workers still run ungated** (#4371):
  consensus.md "What a batch is" says such a node has no worker for the
  partition; on this line its primary authors a header every round and
  self-votes (`primary.go:509-585`) and its workers seal what it is handed —
  invisible in this build (#4369) and harmless to consensus because no
  validator votes for it, but not the spec's sentence. Open.

**The gauge, though, is every node's, for every partition it runs, from
start-up (#4345a).** It used to be created by the join and only by the join —
a Prometheus `GaugeVec` creates a child on the first `WithLabelValues` — so a
healthy node exported no such series at all. Measured on the live network on
2026-09-18, at one moment: the joining node exported
`accumulate_node_state{partition="bvn1"} 0` and `{partition="directory"} 0`
and a healthy peer exported nothing. Absence therefore meant "this process has
not been in the join state machine this lifetime", which is neither ACTIVE nor
unhealthy: the metric could only ever say something was wrong, and only while
it was wrong. The daemon now reports `BOOTING` for each partition as it
starts, `ACTIVE` for a node that executes without asking anyone, and the join
reports every transition of its machine through the same door.

**A node also exports the block ITS OWN executor last executed**,
`accumulate_node_executed_block{partition}` (#4345b). Nothing did: the two
block counters are process-wide, and a node runs the Directory and a BVN in
one process, so each was one series summing two chains — 5264 on
acc-bvn1-val2. Reading one node's height therefore meant a JSON-RPC query, and
the obvious query is ROUTED: asked of a node wedged at block 76, the router
answers from a healthy peer. `accumulate_exec_blocks_total`,
`accumulate_dagbft_blocks_produced_total` and
`accumulate_dagbft_blocks_empty_total` now carry a `partition` label too; the
names are unchanged, so a consumer that sums a node's series reads what it
read before, but one that took a maximum across nodes must take it per
partition (`test/docker/soak/soakmon.py`, `life_from`).

A node that HAS joined holds nothing of the blocks it did not execute, and
never will — there is no backfill on this line — so a request for one of them
is answered `NotReady` naming the block it joined at, not `NotFound`. The
difference matters: the requester counts `NotFound` as a miss and a run of
misses strands a stream for good (healing.md, "Stranded streams"), while
`NotReady` is "ask someone who has it".

**Differences from the issue as written**:

- **Serving resumes when the node executes (ACTIVE), not when its history is
  backfilled (COMPLETE)** — *retired as a difference 2026-09-19 (#4368): the
  spec now says the same.* Fully synced is a verified anchored root; there is
  no backfill on this line and there will be none in phase 1; `COMPLETE` and
  `WAITING` are retired; a node that never joined has no state machine and
  serves as `ACTIVE`. What remains different: `PromoteToWaiting`,
  `PromoteToComplete`, `CanServeHistory` and `nodestate.Restore` still exist
  with no production caller (delete, or pin by test — the builder's
  uncommitted caller scan is the pin); the gauge and the daemon's `Always{}`
  are two objects nothing keeps in agreement. (That `servingFor` gated two
  query kinds where the spec says every read is retired: since `08c0e413d`
  (#4368) it refuses every query while `BOOTING`.)
- **A node is `ACTIVE` only while executing in agreement (#4385).** The
  join promotes when a handoff succeeds (`join.State.Promote`, called by
  `stageAndHandOff` after `Buffer.Handoff` returns nil), never at a match —
  the tracker only matches and holds no machine — and demotes at the
  diverged block when a re-sync starts and at the matched block when a
  handoff fails (`join.State.Demote`). The querier, sequencer, submitter,
  validator and consensus service ask the machine on every call, and the
  gauge follows its OnChange. Proven by
  `TestAReSyncingNodeIsBootingUntilItMatchesAgain` (test/e2e: while it
  re-syncs the node's production querier refuses `NotReady`, the gauge reads
  0, and it signs no BVN anchor), `TestAFollowerJoinsARunningNetworkAndLeaves`
  (BOOTING on every round the follower is joining, matched or not), and
  `TestADemotedJoinIsRefusedByTheDaemonsServicesAndGauge` (cmd/accumulated/run:
  the daemon's querier, submitter and gauge through a failed handoff, the
  handoff, a re-sync and the second handoff). What is still different:
  - **Anchors are withheld by collecting mode, not by the machine.** A
    `BOOTING` node signs and dispatches no anchor because the join has it in
    collecting mode and a collecting node executes nothing; nothing in the
    conductor asks the machine. The one path where they part is the handoff
    window: `performHandoffAt` produces the buffered groups, anchors and all,
    before `Promote(q)` runs (#4385 review F1; the re-sync test counts one
    BOOTING-signed anchor per handoff), which is what the spec now says. Also
    (F2): the `Demote` on a failed handoff is a no-op on `PulledState`, since a
    node in a handoff is never ACTIVE; the transition step 5 describes cannot
    occur and the call is kept as a guard.
  - **The simulator gates only the querier.** Its sequencer is ungated and a
    submission to any simulator node goes to the whole partition through the
    hub (above), so the simulator test cannot show a relay; the relay from a
    `BOOTING` node is shown only by the daemon's submitter.
  - **`PulledState.Executing` still promotes on the node's own, unanchored
    root** — the whole-network-restart path — and has no production caller
    (#4385 note of 2026-09-24 05:51Z). It is the one promotion left that is
    not a handoff.
  - **A demotion is not advertised or persisted**, like every other state
    (#4300): a peer learns it only by being refused.
- **The ModeFullSpine rationale was wrong and is retired** (#4301 (c)): the
  spine was pulled "explicitly unverified because it is what the verifier
  reads from"; the definition and its signatures verify the spine like any
  leaf, and `ModeFullSpine` stays only for comparability of the every-block
  chains (executor.md §1).
- **The state is not persisted** (#4300; `bootpersist` is not ported). A
  restart joins again, which reaches the same answer.
- **It is not advertised** (#4300) in the node's service record, so
  `FindService` still returns a joining node and the caller learns its state
  from the refusal rather than from the listing. The `unavailable` outcome
  label the issue asks for is therefore not added: the requester cannot tell a
  joining node from a node whose entries are in flight, and both mean "ask
  again".
- **The v3 querier was not gated** (#4297) when this was written; it is now
  — `servingFor` (`internal/api/v3/querier.go`) refuses every query while
  the node is `BOOTING` (`08c0e413d`, #4368; it first gated only a BPT page
  and an account read carrying a receipt). What was true then: a joining node
  answered account state and BPT pages from a store the pull had half filled. The hazard is
  another joining node pulling its unverified spine from it — `pull.Account`
  in `ModeFullSpine` was explicitly unverified, because it was what the verifier
  reads from — and then never pulling the spine again.
- **A joining sibling partition is a quiet hole.** Every node runs the
  Directory beside its BVN, and the dialer answers a service the node itself
  provides before asking the network, so while one of them is joining the
  other's healing requests to it are refused locally and recorded as "in
  flight" rather than as misses. Nothing is healed and nothing alarms; the
  node-state gauge is what shows it.

**Not proven, and the tests that pass do not exercise the mechanism**: a join
has taken staging on a real network, and none has completed on one.
`TestOneValidatorRestartDoesNotDiverge` replaces steps 3 and 4 with a store
copy (`test/simulator/partition.go:96-116`), and
`TestPullReachesTheAnchoredRoot` sources from `api.Querier2{Querier:
sim.S.Services()}` (`test/e2e/state_pull_test.go:128`) — no p2p, no routing,
no self-dial — and calls `batch.UpdateBPT()` by hand at `:161` and `:187`,
which is the step the production pull omits. A test that performs by hand what
its production caller must perform proves the library and not the caller. Run
`20260918T124530Z` (30 m, 100 tps, chaos, `0259684c5`) was the first Docker
chaos run of the join and it tested the fallback rather than the join — no
node found a peer to ask, on any partition, because of #4296. Run
`20260918T131713Z`, on `0132b886c` with #4296 merged, went one step further
and stopped: the join took staging from a peer 22 times, including on the
restarted node eleven seconds after its restart (`Staging taken from a peer
block=201 partition=Directory streams=4`, `block=198 partition=BVN1
streams=1`), and every spine pull behind those succeeded — but **not one
block-named account was ever pulled** (2,171 pull rounds, `pulled=0` on all of
them), so no node reached a root match and no join completed. **The cause was
found and it is four independent defects, each sufficient alone**: the joining
node's pull is served by *itself* — `dagbft.go:461` hands it the node's own
routed client, `dial_network.go:44-45` ("Always use self-discovery") answers
locally for any service the node provides, `api.go:71` registers the querier
for every partition it serves, and the querier is not gated by node state, so
the node's own genesis store answers instead of refusing (#4303, with #4297);
the pull never calls `UpdateBPT`, so the local root cannot move however much
is pulled (#4305); the partition's `ledger` and `synthetic` accounts change
every block, are named by no envelope and are pulled once, while the page-diff
backstop is unreachable because `s.refused` stays non-empty on a name that can
never route (#4306); and those two accounts could not be verified anyway
(#4298, which is therefore not an edge case but a precondition). **Zero
verification failures in 22 MB of logs: the verifier never ran once.** In the
same run a fresh network could not start deterministically either:
`Options.Fresh` is dead code — `dagbft.go:449` gates on `lastBlock > 0` and
`:488` sets `Fresh: lastBlock == 0` inside it — so the escape #4296 added is
unreachable in the daemon, and the network started on a 20-second timeout race
won by one arbitrary node per partition (#4304). **Fixed**: a node that has
executed no block beyond genesis does not join, `Fresh` is deleted, and a
fresh netsim reaches block 2 in 1 second against 18 for the timeout race
(`TestAFreshNetworkStartsWithoutJoining`, `TestAGenesisLoadedNodeDoesNotJoin`). A joining node also rejected
14,643 user transactions against its own un-executed store, because the
submitter is not gated either (#4307). The Docker chaos run
(`30m-100tps-chaos.conf`; the 24 h run named here originally was superseded
2026-09-19 by phase 1's twelve-hour acceptance with followers, PLAN E11) is
the proof, and it is a human step. Two known holes
will meet it first — an account carrying pending signature material cannot be
verified at all (#4293's entry above, filed as #4298), and a remote
transaction stub the store cannot resolve is not collected (#4292's entry,
filed as #4299). Run `20260918T131713Z` met neither: its refusals are
`notFound` and `badRequest` answered before verification is reached, so
nothing in it exercised the verification path at all.

**State pull (#4293, partly done)**: the bootstrap-v3 packages are on this
line — `internal/core/bootstrap/{pull,enumerate,bptproof,tracker,nodestate}`
and the v3 `BptPageQuery`, served by the querier. A pulled account is verified
the way the spec now says: against the `StateTreeAnchor` the Directory
anchored for the block the peer served it at, read from `acc://dn.acme/anchors`
(`pull.DirectoryAnchors`), with the peer's receipt required to pass through the
leaf the pulled state hashes to locally, so a true receipt for one account and
a false body for it is refused and asked of another peer (`pull.AccountFrom`).
A fetch is held unwritten until its block's anchor arrives (`pull.Pending`),
because the pull runs ahead of the anchors. The BPT pages are read and nothing
of a peer's is written into the local BPT, so the root the tracker matches is
derived from state the node holds and has verified; and a pulled account
replaces what the node held for it and carries each chain's open mark set, so
the node can append to it and execute `Q + 1`.
`TestPullReachesTheAnchoredRoot` restores a database to block `R`, runs the
network to `Q`, pulls the six of twenty-one accounts whose leaf moved, and
reaches the root the Directory anchored for `Q`; the tracker then promotes.

**Not done**, and each is a hole in the same step:

- Nothing is wired into node start-up. That is #4294; today the packages are
  reachable only from tests.
- **An account's leaf is only reproduced for the state the pull fetches**
  (#4298)**.** The account hash covers a pending transaction's
  `ValidatorSignatures`, `Payments`, `Votes` and `Signatures`
  (`observer_prod.hashPendingV2`; `History` is *not* hashed); the pull
  fetches none of it. An account carrying any of it cannot be verified, so it
  cannot be pulled, so a partition holding one cannot be joined. (The
  scheduled events on a partition ledger and the delivery queues on the
  synthetic account were in this list; #4399 serves and pulls them.)
- **Per-account verification assumes accounts are independent, and they are
  not** (#4298)**.** A key page's hash covers the *book's* pending
  transactions and their signature material (`hashPending`: a page walks
  `page.GetAuthority()`'s pending too), so a page and its book must be pulled
  from the same block or neither hashes to anything anchored, and the page
  must be re-derived after the book moves. Scheduled events and the synthetic
  account's delivery queues are the same shape. The pull has no notion of an
  ordering or a group that must be fetched together; the e2e test pulls every
  stale account at one frozen block, which hides it.
- **"The accounts a block names" is not read from the blocks.** The spec says
  the buffered blocks name what to re-pull; the code names them by diffing the
  peer's BPT pages against the local leaves (`enumerate.Run`), which is a
  full scan of the peer's tree per round rather than a read of what the block
  touched. The block-named path is #4292's `CollectBlock` output — the
  principal of every transaction, the signer of every signature, the anchor
  pool for every anchor — and it is not wired.
- **BPT paging is not a consistent snapshot, so a fresh node's enumeration is
  incomplete by construction** (#4302)**.** Pages are served by key order from
  a cursor (`BPT.GetRange`), one batch each, and a leaf inserted *behind* the
  cursor between two pages is never seen; on a live network that happens
  constantly. This is why the design follows the blocks rather than trusting
  one enumeration: a scan is a starting list, and what keeps it right is re-
  pulling the accounts each observed block names, until a whole block's set is
  pulled before the next anchor arrives. Until #4292 is wired, the diff is re-
  run per round, which converges by repetition rather than by construction.
  (#4294 wired it; #4302 section 5 records what that leaves. **And wiring it
  turned the diff off**: the diff is the else-branch of "the blocks named
  something" (`join/state.go:169-181`), and `s.refused` is sticky
  (`:163-167`), so one name that can never route keeps the diff from ever
  running — #4306. The design is blocks primary with the scan as the safety
  net; the code is blocks only, with a scan in a case that no longer occurs.)
- **The Directory's spine is pulled unverified, and so is the root everything
  else is verified against** (#4301)**.** The spine is what the verifier reads
  from, so there is nothing to verify it against until it is there; and
  `pull.DirectoryAnchors` reads the `StateTreeAnchor` roots out of
  `acc://dn.acme/anchors` through the v3 API without checking a single
  `BlockAnchor` signature or counting a quorum. With one source, that source
  supplies both the root and the state that hashes into it, and the whole
  scheme proves only that the peer is consistent with itself. What would close
  it: verify the anchor transactions' `BlockAnchor` signatures against the
  Directory operators' key page of that block — which is exactly what the
  spine is pulled *for* — and require a quorum of them, with the spine itself
  taken from independent sources and cross-checked before anything is
  verified against it. The spine's stated purpose is to close this circle and
  it does not yet.
- **A peer serving a page is not held to it** (#4301)**.** A BPT page carries
  no proof, so a peer can omit a leaf and the puller will not know an account
  is missing until its root fails to match.
- **`tracker.Observe` keeps the earliest block for a repeated root, so `Q` is
  the first block with that root, not the last** (#4302)**.** That is
  intended: a run of blocks that change nothing all carry the same
  `StateTreeAnchor`, the state behind the root is the state from the first of
  them, and executing from `Q + 1` where `Q` is the earliest replays the
  blocks in between rather than skipping them — the safe direction, because a
  block replayed from the state it started at produces the same result and a
  block skipped does not.
- `orchestrator`, `anchorsrc`, `bootpersist`, `clientsrc` and `gossip` from
  bootstrap-v3 are not ported (#4302; `bootpersist` is #4300's, which settles
  first whether a joining node's state need survive a restart at all); the
  first is #4294's. **`anchorsrc` is not of a rejected model — it is the
  spine validator #4301 ports** (the signed anchor verified against the local
  operators' key page, to a threshold of distinct entries, with producer
  routing); leaving it behind is why the first pass's trust terminated in one
  peer. `clientsrc` and `gossip` are of the model bootstrap-v3 used to find
  peers, not of the trust model, and stay unported.
- The v3 `block` query's entry paging ignores `start`, so it cannot be used to
  page through what a block touched. Untouched here (#4302).

**Decided (Paul, 2026-09-18) — superseded 2026-09-19 by the validated-spine
design above (executor.md "Sync" steps 1–5; PLAN E11 second pass; #4301 →
#4361 → #4362):** a starting node takes its staging from a running validator
through an API, keeps it current from consensus while it pulls the state the
buffered blocks name, and executes from the block after its root matches; a
restart is the same path, not a consensus replay. The five steps below are
the record of what that first pass built; #4362 deletes the staging API and
`takeStaging`, and what steps 3–5 keep is said there.

**Collecting (#4292, done)**: the executor takes a committed block into
staging without executing it (`Executor.CollectBlock`), and settles staging
at the block its pulled state is (`SettleStaging`): proofs decided against
the anchors that state has executed, every stream released through the
`Delivered` the pulled ledger names. The DAG service has a collecting mode —
committed groups are collected and buffered, the block index does not move,
no checkpoint is saved, no state hash recorded, no block event published, and
the node does not report execution, so its primary reads as lagging and
proposes no batches (consensus.md, invariant 9). **Differences from the
spec**, both deliberate:

- The spec says a joining node buffers *every* committed block from the
  moment it listens. The buffer is bounded — `maxCollectedGroups` (8,192
  groups, about half an hour at four leader rounds a second) and
  `maxCollectedBytes` (1 GiB of batches, because the count alone does not
  bound the memory) — since the node holds every batch of every buffered
  block, and an unbounded buffer is a memory fault of the kind that ended
  runs `20260903T202621Z` and `20260904T*`. Past either bound, and after any
  block that could not be collected, the buffer is marked overrun and the
  join must start again from a newer state. It does (#4294, #4407): the
  join's overrun branch asks the service to collect again, and
  `Service.StartCollecting` — which otherwise keeps the buffer, so the
  daemon's call and the join's on a first start lose nothing (#4351) —
  starts a new buffer when the old one has overrun. The groups committed
  before that restart are in no buffer, so the handoff stands at the highest
  round the node collected or refused, and a state below it is refused
  (`Conflict`) as a state behind the node's own round is; the join pulls
  on. `TestJoinRun_AnOverrunDuringTheJoinResumesFromANewerBlock`
  (`internal/node/dagbft`) drives `join.Run` against the production
  `Service` and its block production loop through an overrun, with
  consensus, the pull and the gap check stood in for;
  `test/e2e/join_overrun_resume_test.go` drives the production pull with a
  stand-in buffer.
- A collected entry's `Collected` flag is decided against the store as it
  stands when the block is collected, which on a joining node is the state
  the pull has reached, not the state at that block. A joining node's store
  is *behind* its peers', so it can only over-mark: it holds collected what
  its peers hold runnable, never the reverse. An entry covered by a package
  proof closes itself — the proof is staged, and the anchor decides it. An
  entry carrying its own receipt used not to: nothing stages that proof, so
  it waited for a validated hash that might never come. Closed in #4294:
  `runnable` re-checks a collected entry's own receipt against the anchor
  chain, so runnability is a question about the entry and the state and not
  about when the entry was held (executor.md, "Collection").
- **The simulator's join still maps by block number** (#4362). The DAG
  service's handoff picks the buffered groups above the pulled ledger's
  `LeaderRound` (executor.md, "Sync", step 5); the simulator's buffer
  (`test/simulator/join.go`, `joinState.Handoff`) is handed blocks that
  already carry an index and has no leader rounds, so it maps its buffer by
  that index and the simulator's ledgers record no round. Every simulator
  join test therefore exercises a handoff rule production does not run; the
  round rule is covered only by `internal/node/dagbft` and by a live network.
  The same holds for which buffered blocks are taken into staging before the
  handoff (#4398): the DAG service stages the groups through the first one
  above the pulled ledger's `LeaderRound` (`Service.StageThrough`); the
  simulator stages the blocks whose index is at most `Q + 1`. The rule —
  nothing collected after `Q + 1` is in staging when `Q + 1` executes — is the
  same on both.
- `classify` resolves a remote transaction body from the store, so a sequenced
  message carrying a remote stub whose body this node has not pulled yet is
  not classified and not held at all — a hole on the joining node where its
  peers hold an entry. A block does not have this problem because anything
  the classifier drops still goes through its own message executor on the
  envelope pass; collecting has no second pass. #4294's join pulls the
  accounts a block names before the block is collected, which closes it —
  but the wiring names a block's accounts *after* collecting it
  (`dagbft/collect.go:385`, on the collect's output), so that closure is
  contested; #4299 holds the citations and is where it is settled.
- `intakeProof` discards a proof whose anchor block is at or below the newest
  executed Directory anchor as "never, not not-yet" (#4302). On a partially pulled
  store the anchor pool's `DirectoryAnchorBlock` field and the Directory
  anchor chain can disagree, and a proof discarded that way is discarded for
  good. The pull writes an account's state and its chains together, so the
  two are consistent per pulled account; nothing enforces it across the
  window in which the pull runs.

**Live-network defect (#4296), fixed on `issue-4296-join-finds-no-peers`,
not merged**: a service is advertised under its network's key, and
`FindService` searched the key the caller named, so the join's lookup — which
named no network — matched nothing on every live network and the node
executed from its own empty staging, the #4290 behaviour, silently (run
`20260918T124530Z`: every node on both its partitions at genesis, and the one
restarted node at Directory block 186 and BVN1 block 181). On that branch a
lookup that names no network means the node's own network; the join names it
as well; and finding no validator is no longer read as "every validator has
nothing to give". The same omission had disabled the conductor's
anchor-signature fan-out on every live network since it was written. It is
not on `dagbft-integration` and it has not run under chaos, so the sentence
above about what the wiring does when no peer can answer is still what a
deployed node does.

**The changed set comes from the block ledger (Paul, 2026-09-18)** — *done,
2026-09-18*: the code derived it from a block's envelopes (`collect_block.go`,
`accountsNamed`), which names principals, signers and anchor pools and
therefore missed the system accounts every block changes and admitted
`acc://unknown`. `accountsNamed`, `CollectedBlock.Accounts` and
`Buffer.NamedAccounts` are deleted; `State.Pull` takes no account list, because
the set is not the caller's to supply. The join reads the block ledger from a
peer for `(R, Q]` and takes `join.ChangedAccounts` of it.

**#4303, #4305, #4306, #4308, #4307 and #4297: what was fixed and what it
leaves (2026-09-18)**. Six defects, each of which alone stopped a join, and a
seventh nobody had named.

- **The pull read from this node (#4303).** `join.StateOptions.Query` is gone.
  It is `Sources` now: `join.QueryPeers` routes each account to a partition,
  looks that partition's query service up under its network's key, drops this
  node's own peer ID, and hands package pull one source per remaining peer,
  addressed with `Client.ForPeer(id).ForAddress(query:<partition>)` — an
  address carrying a service address, so the transport skips routing and the
  dialer opens a stream to that peer. The Directory's anchors are read the same
  way. There is no querier a join may be given, including its own.
- **The fetch was thrown away and taken again (#4303, second half).** `Account`
  discards on `ErrNotAnchored`, and the pull runs ahead of the anchors by
  design, so the next round re-fetched at a newer block that was not anchored
  either. `pull.FetchFrom` fetches without settling; the join holds the
  `Pending` across rounds and settles it once the root it was served at is
  proven (until #4362 it gave up after `maxSettleRounds` rounds and asked
  again; that bound is deleted — see "Anchored-height serving" above).
- **The pull did not move the state root (#4305).** `UpdateBPT` before every
  commit, in the pull and in the spine.
- **The changed set could not match (#4306).** Above, plus: the page diff runs
  on the first round, on a cadence counted in rounds that fetch (#4395),
  and whenever the walk cannot cover `(R, Q]` — never gated on the set being
  empty — and a name that cannot be routed is dropped rather than retried
  forever.
- **A block's receipt was attributed to the puller's partition (#4308).**
  `Pending.Partition` comes from `api.Receipt.Partition` when the peer names
  one.
- **A joining node was a black hole for user traffic (#4307)** and served the
  two reads another node's pull takes (#4297). `Submit`, `Validate`,
  `BptPageQuery` and an account read carrying a receipt answer `NotReady` while
  the node is joining; plain reads stay open. Since 2026-09-19 the spec draws
  the line differently for a *transaction*: a node that cannot propose it —
  following or syncing — relays it unexamined to a node that can, and never
  drops it (step 6; #4366) — built and merged (`a09ff3e0a`); for a transaction the relay
  replaces `Submit`'s `NotReady` while joining, while every *read* keeps it.
  The node state reaches the
  querier, which is configured apart from consensus, through IOC
  (`dagbftProvidesNodeState` / `querierWantsNodeState`), not a registry keyed by
  partition — a process runs several nodes of one partition.
- **The spine put another partition's accounts in this partition's tree**
  (#4309) — found while fixing the above, not previously filed. `pullSpine` pulled
  `dn.acme/{anchors,ledger,operators,operators/1}` into a BVN's store. A BVN's
  BPT holds no `acc://dn.acme` account, so those four leaves put the local root
  beyond every root the Directory ever anchored for that BVN, however perfectly
  everything else was pulled — on its own enough to stop every BVN join. It now
  pulls `SpineAccounts(<this partition>)` only; the Directory's spine is the
  Directory's join's business, and the anchors are read through the API rather
  than out of the local store. executor.md "Sync" step 3 is corrected to say so.

**Differences that remain, from this change set**

- **The block ledger records are taken on the peer's word** (#4310)**.** This
  is the one place the code knowingly contradicts the spec text, and it has an
  issue rather than only this paragraph so that the trade is decided rather
  than absorbed. The spec says a
  joining node "verifies each against the anchored root the way it verifies an
  account". It does not: the records are read through `BlockQuery` with
  `EntryRange.Expand` false, which answers from the block ledger with the
  `(account, chain, index)` triples and no receipt. The exposure is liveness
  and not safety — every account the set names is still verified against the
  anchored root before it is written, and the root match is what admits the
  node — so a lying peer can only keep a join from converging, which any peer
  can do by refusing. A receipt is possible and is the remaining work: the
  record's hash is an entry on the ledger account's `block-ledger` chain, and
  that chain's anchor is part of the account's hash.
- **Leaves with no body are gone, not defended against** (#4437)**.** From
  #4397 to #4437 the querier served "no body, and the leaf's receipt" for an
  account whose tree held a leaf and no main state, and the pull kept such a
  leaf only when every answering peer served the same one (the unanimity
  rule, #4406's hole). #4437 removed the cause — a failed transaction's
  clearing of its votes and payments dirtied the missing principal, and block
  close inserted a leaf for every dirty account — and the executor now
  inserts no leaf for an account without main state (invariant 13). The
  serving branch, the voting and `pull.ErrDissent`/`ErrUnconfirmed` are
  deleted; a body-less answer is a failed source. Nothing is migrated: this
  line runs fresh installs, so no store holds such a leaf.
- **A lone scheduled event is not bound to its block** (#4399 review F2)**.**
  The events BPT hashes values and not keys (`bpt.leaf.getHash`), and a
  one-sided branch passes its child's hash up, so an events tree holding one
  entry has a root equal to that entry's hash wherever its key sits. A peer
  can serve the one held vote (or the one pending expiry) under another block
  and the ledger's leaf check passes; the joined node then releases it at a
  different anchor from its peers. With two or more entries the positions
  bind. It is the realistic case — one pending multisig transaction — and it
  cannot be closed at the pull: the events leaf must hash its key, a
  consensus hash change. `TestALoneScheduledEventIsBoundToItsBlock` states
  the property and is skipped until then. The block lists themselves are no
  longer taken from the answer (F1); they are derived from the verified sets.
- **The page diff runs on the first round of every join** (#4302 section 8)**.**
  That is one full
  BPT page scan of the partition, names only, before the node knows whether
  its store is the state of `R`. On a large partition it is not cheap, and a
  restarting node does not need it — its store *is* the state of `R` by
  construction. Deciding that from the store rather than paying for the scan is
  work not done.
- **A held fetch keeps a batch open across rounds** (#4302 section 9)**.**
  Each round's fetch holds
  `db.Begin(true)` until the root it was served at is proven or the history
  passes it, so a version of the store is pinned for a few rounds (#4279 is
  about the cost of that). It is bounded by the next verified anchor after
  the served block and by `pull.MaxHeld`, not by a round count (#4362); it is
  not free.
- **#4298 is untouched and is still a precondition** — for the pending
  transactions' signature sets. The other half is done by #4399:
  `<partition>/ledger` hashes the scheduled-events BPT and
  `<partition>/synthetic` hashes the delivery queues (`observer_prod.go`), and
  until #4399 the pull fetched neither, so those two accounts verified only
  while both were empty — and under load the local delivery queue is never
  empty, so `/synthetic` was refused by every peer on every pass (run
  `20260924T052134Z`). What follows was written before that. This change set makes them
  **asked for** — which is #4306 — and does nothing to make them **verifiable**.
  One observation for whoever takes #4298, offered as an observation and not a
  finding: both are skipped when empty, and the local delivery queue is drained
  at the next block's `Begin`, so how often either is actually non-empty at the
  block a peer serves is a measurement nobody has made. That measurement now
  stands ahead of any design work on #4298 in PLAN E11: it decides whether
  #4298 is "no join completes" or "a join retries a few times".

- **A joined node's pulled anchors, and a stalled spine** (#4413, #4416,
  #4418, #4419). The spec (executor.md "Sync", step 6) says a node serves only
  what it can serve signed. The pull used to bring an anchor pool's entries
  and the messages behind them but not the signature history the query API
  reads an anchor's signatures from (`loadMessage`, `internal/api/v3/load.go`),
  so a node that joined served its pulled range bare and every reader refused
  it (run `20260924T074702Z`: 22 BVN3 anchors). Since #4413 a node refuses an
  anchor it holds unsigned with `NotReady`; since #4416 the pull rebuilds the
  history, the signer, the sequenced message, the cause and the validator
  signature set from the entries it takes, so a joined node serves and counts
  its pulled range as its peers do — the capacity gap is closed for the spine
  pass, and since #4421 for every pass: a spine account is taken whole
  whichever pass names it, and the pass that carries the spine fetches the
  message and those records behind any of the newest held entries that lacks
  its message, or takes the chain whole when a position is not held at all.
  What is still different: a store written between #4400 and #4416 holds
  entries with their messages and without those records, and nothing repairs
  them; a node that diverged and holds more entries than every peer is
  refused, not retaken, and the retake is unbounded and tracks no orphans
  (#4403, Paul's decision); a joiner whose every peer serves an entry bare or bodiless
  waits at it (#4418) — a wait that since #4419 costs one page call per peer
  per round, holds the cursor at the first entry no peer serves, and shows on
  `accumulate_join_spine_stalled_entry` and in one log line a minute, which
  the soak's node-state row reads as "spine stalled at entry N"; and a peer
  can still substitute one signed entry for another in a slot (#4384, measured
  there: roots are lost, never misplaced).

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

### D10. A pre-image for a new dynamic key walks the store's history

*[#4237](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4237)*

**Spec** ([database.md](database.md), "Isolation has a price"): a commit made
while a reader is pinned reads a pre-image per dynamic entry; a first write
never searches history to learn a key is absent.

**Code**: the adapter (`bcdb/database.go`, `preImages`) skips the read for a
permanent shape and answers it from its own cache for `Account(U).Url`, and
`Validate` no longer pins a reader at all, so under load with no API reader
open no pre-image is read. When a reader IS pinned — an API query, a snapshot
— every dynamic entry of an unaccounted shape is read with `GetDyna`, and
most of them are **new** keys (a fresh status, produced set or signature set
per message): BlockchainDB's `SegmentStore.Get` (`segstore.go:1794-1797`) does
not stop at the window for a dynamic key it does not hold, so each of those
reads is a bloom probe per history segment — ~5 K keys × 5–10 segments per
block. The adapter cannot tell a new dynamic key from an old one without
asking, and this repository cannot change what asking costs.

**What the store needs** (to be filed against BlockchainDB; recorded here so
the text is not lost): a *window-only* dynamic read for pre-images —
`GetDynaRecent(key)` or a `Get` option — that answers from the active tier and
reports absent without consulting history. A key the window does not hold
either is new (no pre-image) or was last written before the window, and a
reader pinned that far back is already a fault the adapter warns about
(`warnOldView`); neither case is worth a history walk on the block producer's
commit path. With it, `preImages` becomes one active-tier lookup per dynamic
entry and `preImageReads` ≈ pre-existing dynamic keys touched.

**Size**: small here (swap the call once the store has it); the store-side
change is the work.

---

## Consensus

### C7. Nothing refuses the synthetics a partition cannot execute

*(no issue yet — a design decision)*

**Spec** ([consensus.md](consensus.md), invariant 9): a partition whose
executor lags its consensus past the bound proposes empty headers and refuses
**user** work until it catches up. Synthetic packages and anchors from other
partitions are system traffic and are never refused (invariant 4).

**Code**: as specified — and it is not enough. A BVN's user work produces
synthetics for the *other* BVN, about one and a half per accepted transaction
under the soak's load, and the destination can neither refuse them nor execute
them faster than they arrive when the source keeps accepting. Refusing the
destination's own users changes nothing about that inflow. consim reproduces
it in five minutes (`pkg/consensus/consim`,
`TestOverload_UncappedHeadersDoubleTheDumpUntilThePartitionStops`, and the
command line in PLAN.md "Simulation first"): BVN1 offered 400 user tx/s and
BVN2 250, each executor good for 400 tx/s, 1.5 synthetics per accepted user
transaction; BVN2's lag runs to 42 and its dumped blocks double every cycle
until it stops. With the header cap (`MaxHeaderBytes`, 455a82ee1) the blocks
stay bounded and BVN2 keeps producing, but its lag still drifts upward — the
inflow exceeds its capacity. Soak `20260905T144928Z` is the same curve in
forty-five minutes: BVN2 produced twice BVN1's synthetics (#4220), both BVNs
oscillated on the bound, load accepted fell to 392 tps.

**What is missing** is back-pressure across partitions: a source must stop
accepting user work when a destination of its synthetics is behind. The
signal exists in principle — what a source has produced for a destination
that the destination has not yet executed (the producer cache's "in play",
healing.md "The cache", whose clearing signal is also H1's open item) — and
the refusal is the one invariant 9 already has. What must be decided is the
signal (a bound on undelivered synthetics per destination, carried back by the
destination's anchors or by the Directory) and the bound.

**Size**: medium; a spec decision first.

### C8. A committed group that fails to execute is skipped, and the lag it leaves is permanent

*(no issue yet — found working #4279)*

**Spec** ([consensus.md](consensus.md), "Execution"): a committed
certificate's batches are executed in canonical order and never skipped; a
node that cannot execute a block its peers executed halts rather than
diverge. Invariant 9 bounds the lag between commits and executions and
expects it to clear when execution catches up.

**Code**: `blockProductionLoop` halts only for `ErrBatchesUnrecoverable`. Any
other failure of `ProduceBlock` — a block that fails to close, for whatever
reason — is logged as `Failed to process committed group` and the loop takes
the next group. The failed group's transactions are never executed, on this
node or, if the failure is deterministic, on any; and `ReportExecuted` is
never called for it, so the lag it adds to `ExecutionLag` never clears. On
run `20260915T042428Z` nine Directory groups failed to close within a minute
(one bug, deterministic, every node), the lag reached 9 against a bound of
8 at 05:41:33Z, and the Directory proposed empty headers and refused every
submission — including the BVNs' anchors — for the rest of the run. One bad
block became a dead partition, with the partition reporting itself healthy
in every liveness sense.

**What must be decided** is what a deterministic execution failure does.
Halting, as an unrecoverable batch does, stops every node at the same block
and is honest about what happened; continuing past it can only be right if
the failure is known to be node-local, which nothing here can know. Either
way the lag accounting must not count a group that will never execute as
"behind": a skipped group is not lag, it is loss, and should be its own
counter and its own alarm.

**Size**: small in code; a spec decision first.

## Healing

### H0. The healing pair does not rotate on a partition whose blocks are empty

*[#4420](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4420)*

**Spec** (healing.md, "who asks"): the pair rotates with every activation.
**Code** (since #4415): the draw is seeded from the root chain's anchor as the
executor writes it, with `ledger.Index` as the fallback — both move only on a
committed block, while activation fires on the consensus index, so on a
partition executing only empty blocks the same pair asks on every activation.
Before #4415 the seed was a `RootChainAnchor` the executor stores as zeros, so
the same pair asked on every anchoring block of a BUSY partition too. The
fix is to hash the consensus index into the seed.

### H1. The producer cache exists; what it is not yet cleared by, and what still reads the store

*[#4193](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4193)*

**Spec** ([healing.md](healing.md), "The cache"): the producer keeps every
synthetic and anchor in play, keyed by hash and by stream position, with what
its proofs are built from; dispatch and healing read it and nothing else;
cleared as the destination delivers; a miss is refused and counted.

**Code**: built (`internal/core/synthcache`). Dispatch and the sequencer's
answers are built from it alone; the e2e suite fails on a dispatch miss.
Synthetic entries are released by the destination's `Delivered`, carried on
every dispatched message and package (2026-09-06); produced anchors by the
`Delivered` carried on every anchor copy, once every destination has spoken
(#4232); blocks with nothing left to prove are dropped; the horizon backstop
is ten minutes. Remaining: a synthetic stream whose reverse direction is idle
shrinks only at the horizon (anchors say nothing about synthetics); the v1
simulator's sequencer still reads the store, since the v1 executor has no
cache; the Directory receipt a block was dispatched under is
the only anchor a bundle can be proven under (`ProveAgainstAnchor` for any
other is `NotReady`), which is H3. The requester exists (H8) and asks the
sequencer by span. The stage bound for a collected entry (`maxSequenceAhead`)
is a constant, not the source's produced count: the destination learns that
count on no wire path (#4243).

The destination's `Delivered` of this stream — the release signal — lives only
in the cache. A restart therefore cannot seed "what the destination has not
delivered"; it seeds what the Directory has not receipted plus the in-flight
tail (healing.md, "The cache"; #4241). A destination lagging more than
`InFlightBlocks` of the source's blocks at the source's restart heals from a
cache that lacks its entries, and the miss is counted. The fix is to persist
the release watermark per stream as a node-local record — it is per-node
state and must not enter the hashed ledger (executor.md, "Sync") — which is
the cache's to do.

**Size**: small; the delivery signal is the open design point.

---

### H8. Healing pulls spans and the requester submits; the spec says hashes, and the source submits

*[#4216](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4216)*

**Spec** ([healing.md](healing.md)): staging computes the gaps by index; a
selected validator sends one request naming **hashes and index spans**; the
**source** answers from its producer cache and **submits the bundle into the
requesting network** through its dispatcher; intake takes it; the answer
carries no signature.

**Code**: the requesting side is built (`internal/core/crosschain/requester.go`).
On an activation block a validator selected by the previous block's root
anchor walks each stream of the executor's `execute.Staging` once, from
`Delivered` to the highest entry held: an index not held, and a held entry no
proof has validated, are the two gaps; consecutive ones coalesce into spans
(at most `MaxRequestSpans`, each within `MaxReceiptListElements`); a span
asked within `healPatience` activations is not asked again; a source whose
requests all failed is backed off, doubling to eight activations. Nothing is
timed and nothing is inferred from the source's ledger. Counted:
`accumulate_conductor_heal_requests_total{outcome}`, `heal_entries_total`, and
`HealCounters.Requests/Misses/Synthetic`. Seven of the nine dropped-entry
acceptance tests run and pass; the two that drop an anchor are H9.

Where it departs from the spec:

- **Pull, not push.** The requester calls the source's `SequenceRange` and
  submits the answer into its own partition itself, as a bundle shaped like a
  package (`SyntheticProof` first, then the entries, each with its companion).
  The source submits nothing. The push form needs a submit path from the
  source into a foreign partition's consensus that the dispatcher does not have
  today; the pull form uses the request's reply channel, which exists.
- **Spans, not hashes.** There is no entries-by-hash-set method; a
  proven-missing entry is asked for by its index inside a span, and the answer
  carries the span's proof whether or not the requester already held it.
- **A signature in the answer.** The sequencer signs each entry with the
  answering validator's key; the executor requires a key signature on a
  `SyntheticMessage`, so the requester copies it into the bundle. The spec's
  "no signature" would need the executor to accept a proven entry unsigned.
- **The answer names its anchor.** `MessageRecord.SourceAnchorBlock` and
  `MessageRecord.Companion` were added to the API record so the requester can
  build the proof's `AnchorMetadata` and include the companion without reading
  the source's history; the spec has no wire format for these.

**Size**: the push form and the hash method are each small once the executor
accepts unsigned proven entries; not urgent.

---

### H9. A pulled anchor is re-attested, not proven

*[#4056](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4056)*

**Spec** ([executor.md](executor.md), "Proof"; [healing.md](healing.md),
"Deciding, in staging"): an anchor is validated by a validator signature quorum
or by a collection proof over the source's anchor chain; a missing or
below-quorum anchor is requested from the source's cache like any other entry.

**Code**, as of 2026-09-05: anchors go through the stage — an anchor below its
quorum is held at its number, collected, and runs when the signatures reach
the threshold; one at or below `Delivered` is tossed; a copy naming its
transaction by hash resolves against the held one. The requester walks the
anchor streams with the synthetic ones and asks the source for gaps
(`requestAnchorSpan`); the source answers from its cache, a prefix at a time,
`NotReady` for anchors still in flight. The source-side re-send (`healAnchors`)
is deleted. An answer for a Directory anchor carries every validator
signature the Directory holds on its own copy of that anchor — the quorum,
accepted on the BVN's copy by the signature-reuse rule — so one pull restores
an anchor whose dispatched copies were lost (soak 20260905T225751Z: fewer
copies than the threshold arrived at a BVN, nothing re-sent them, and every
stream waited on the anchor). What remains: a BVN does not execute its own
anchors, so an answer for a BVN anchor carries only the answering node's
signature and the Directory gathers a quorum one answer at a time, from
whichever node the client dials, which does not rotate; the proof form — a `BlockAnchor` with a collection proof over
the source's anchor chain, continued to a root the destination already holds —
needs the root chain's span across blocks, which the cache does not keep, and
is what `TestAnchorQuorumStuckRecovery` expects (skipped with this reason).
`TestAnchorRangeRecovery` runs on the re-attestation form.

**An anchor executed on a collection proof would have no signature in its
history** (#4416). `BlockAnchor.process` records the copy it executes on the
pool's signature chain whether it carries a signature or a proof
(`msg_block_anchor.go`), so such an anchor's history holds one entry with no
signature: every node would serve it, and every reader (`anchorsrc.verify`)
would refuse it as signed by none of the set and pass it — a root that no
joiner can take, on every node, and no stall. The path is unreachable in this
tree and nothing is built for it: no production code constructs a
`BlockAnchor` with a `Proof` (the tools' anchor healer, `internal/core/healing`,
builds signed copies only), and no test does; only a client submitting one
past Kourou would reach it (`check` accepts it). The pull keeps such an entry
as the chain holds it (executor.md "Sync" §3). Whoever builds the proof form
above decides what a reader takes as its authorization.

**Size**: medium — the root chain span in the cache, bounded by the horizon,
and the proof built from it.

---

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

## Durability: the seal is still on the block path; the committed log is not on disk

The spec (database.md invariant 5, consensus.md "The committed log") makes the
consensus log the durability point and lets the store seal behind the commit.
The code does neither yet: `bcdb.(*Database).writeThrough` calls
`KVShard.SealBlock` on the block goroutine before the commit returns (the
~40-fsync barrier that stalled soak 20260906T134054Z for four minutes,
#4259), and the committed groups exist only in memory — the worker's batch
store and retention, the DAG's certificates — with only the consensus
*position* checkpointed (`persist.Checkpoint`). Restart therefore cannot
replay unsealed blocks, which is also why nothing may be lost today. The
store question — sealing several blocks in one call while writes continue
into the next tail — is BlockchainDB#88. Decided by Paul 2026-09-06: "If the
DAG has a log, use that."

**Size**: medium. The log append and its replay are new code on the block
production path; the lagging seal is a scheduler in the bcdb adapter; the
store change is a BlockchainDB release.

## Proofs still recompute what is stored

database.md ("Proofs are read, not searched") requires a proof to be reads and
arithmetic. Two of the three positions now are. The third is not.

~~`getIntermediate` rebuilds the Merkle state that held each sibling.~~ Done:
the cascade pairs are stored as they are computed and the proof reads them,
with the heights that do not exist answered by arithmetic. A chain written
before the record exists has none, and falls back to the rebuild.

The account state tree has the analogous gap in a different place: its blocks
are read cold on every request, because loaded blocks are pinned to the batch
and nothing caches them above it. Measured on 500,000 leaves, a proof is 4
reads and ~70 KB, paid again for every query, while the top two levels are 257
blocks (~8.7 MB) and would cover heights 0-16 outright.

**Size**: medium for the cascade store (a format addition and a migration
question); small for the block cache.

## The account proof API has no specification part

An account proof takes two calls: the account query returns a receipt to its
partition's state-tree root and says whether a second call is needed
(`Receipt.Complete`, `Receipt.Partition`), and `ProofService.AnchorReceipt`
extends that root through the partition's bpt chain to a root-chain anchor and
binds it to a directory root. Neither is described in the specification,
because the API part is not written — SPEC.md lists it as a gap.

The rules the implementation holds to, pending that part:

- A BPT is a tree of **current** state, so an account cannot be proved against
  a past BPT. The proof is built against the current root.
- On the directory the first call is already complete; elsewhere the root
  reaches a directory root only after an anchor round trip.
- `Anchored: false` means the anchor has not arrived yet, and the heartbeat
  bounds that wait. It must never mean "never" — before the bpt chain was
  anchored it could, which was the defect (#4276).
- The default is the oldest receipt that works, so the answer is stable and a
  caller can record it once.

**Size**: the API part is large and covers far more than proofs. This entry
exists so the two calls are not mistaken for unspecified behaviour.

## A pulled account can only settle on a block the Directory anchored

*Retired 2026-09-22 (#4362).* The repair the end of this section names is
what was built, with the trust anchor it said had to be chosen: the root
chain anchor and height a verified signed anchor carries. `anchorsrc.ProveRoot`
proves a root served at any block by equality with a verified
`StateTreeAnchor` or by the bpt chain's history from one to it, held to the
later anchor's signed root chain anchor (executor.md "Sync", step 2), so the
stride below no longer discards anything, `maxSettleRounds` is deleted, and
`tracker.Matched` still compares this node's own root against a verified one.
The rest of this section is kept as the record of the measurement.

executor.md ("Sync", step 3) says a fetched account is held until the anchor
for its block arrives. The implementation reads that literally:
`pull.DirectoryAnchors.AnchoredRoot` is an **exact** lookup of
`(partition, block)` against the `StateTreeAnchor` values carried by the
anchors the Directory executed, and a peer serves an account at whatever block
it is currently on.

Only some blocks send an anchor. Measured on the live twelve-node network of
2026-09-18, over a 500-block window per partition:

| partition | blocks with an anchored root | stride |
|---|---|---|
| `acc://dn.acme` | 24.0% | 4.16 |
| `acc://bvn-BVN1.acme` | 17.3% | 5.78 |
| `acc://bvn-BVN2.acme` | 17.1% | 5.85 |
| `acc://bvn-BVN3.acme` | 17.1% | 5.85 |

Every account fetched in one round is served at nearly the same block, so a
round settles wholesale or not at all: roughly five settle batches in six wait
`maxSettleRounds` for an anchor that is never coming and are then discarded and
re-fetched. The join still converges — it is a constant factor, not a wedge —
but it is a six-fold one, and it was silent until #4295 gave the discard a log
line.

**Two obvious repairs do not work, and it is worth writing down why.**

*Settle against the nearest anchored block at or after the one served.* The
receipt the peer served terminates at the peer's BPT root **as of the block it
served at**, and no other block's root equals it. Retrying the same receipt
against a later anchored root fails the second check, not the third.

*Have the peer serve the receipt at the most recent block it knows is
anchored.* A BPT is a tree of current state — every account that changes
rewrites the path to the root — so **an account cannot be proved against a past
BPT** (see "The account proof API has no specification part", above). The peer
has no past BPT to build that receipt from.

**The repair that does work is already built, for a different caller.**
`ProofService.AnchorReceipt` (#4274, #4276) extends a partition's *current*
BPT root through the partition's `bpt` chain into a root-chain anchor the
partition actually sent, and binds that anchor to a directory root. Every
block's root is on the bpt chain, so **every** root is provable — which is
exactly the property the stride denies the pull. Verifying a pulled account
through the two-call proof instead of through an exact `StateTreeAnchor`
lookup would remove the stride entirely.

What that change has to settle first, and why it was not made along with
#4295: the two-call proof terminates at a *directory root chain* anchor, not at
a BPT root, so the joining node needs a trusted directory root-chain anchor to
check it against, and today the only roots the pull trusts are the
`StateTreeAnchor` values it reads out of the Directory's anchor pool. Choosing
that trust anchor — and keeping `tracker.Matched`, which compares this node's
own BPT root against an anchored one, working alongside it — is the design
question. It is not a large change; it is a change that must not be guessed at.

**Size**: medium. `pull.Verify` and `pull.DirectoryAnchors` on one side, the
proof client and the trust anchor on the other; `tracker` unchanged if the
final root match stays as it is.
