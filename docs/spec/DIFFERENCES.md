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

**Serving staging (#4291, done)**: a running validator serves its staging as
of its last committed block through the private API, paged by stream, with
the block index on every page (healing.md, "Staging snapshot"). The join
reads it (#4294). Also not done: the refusal is "this node has
executed no block", not the node state of step 5 — a node that is `BOOTING`
will serve its stage until that lands (#4295). And a page is as of whatever
block the validator had committed when the call arrived; nothing is pinned
server side (#4302), so a reader whose pages straddle a commit starts over rather
than being served a consistent version.

**Caution — the page's `Block` is not "the state of that block" (#4291)**: it
is the consensus index of the last block the executor processed, published at
commit, and an empty block writes no state, so `SystemLedger.Index` can lag
it. That is deliberate and it is the safe direction — the index is at or above
every block whose intake the page reflects — but nothing in the page says so.
A joining node must take the entries as of that index and decide which block's
state to converge on by the anchored-root match (#4293's tracker), on a block
at or above the page's. Serving the last *written* block instead would
under-report and is the dangerous direction.

**`Delivered` on a page is memory, not a ledger read (#4291)**: the issue's
trap said to read the ledger. This branch instead made block close release
every stream the block positioned at the ledger's `Delivered`, so memory
tracks the ledger for every stream a block has touched and the value is
atomic with the rest of the page; a ledger read beside it would be a second,
unpaired read. The residue (#4302): **a stream that no block has positioned
since a restart keeps `Delivered: 0` in memory**, so a page may carry 0 for such a
stream. It is safe, because the joining node releases through its own pulled
ledger when it settles (#4292's `SettleStaging`), but it is not what the issue
asked for.

**The handoff (#4294, in progress)**: the join's orchestration exists
(`internal/node/join`) and the DAG service can be handed off to — it leaves
collecting mode at Q, stands at Q, and produces what it buffered from Q + 1,
all inside the block production loop, which is the only thing that produces
blocks. `Conductor.Rejoin`, its metric and `Executor.Collect` are gone: a
restart is a join. The simulator's `RestartNode` starts a join and
`TakeStaging`/`CompleteJoin` complete it, so
`TestOneValidatorRestartDoesNotDiverge` passes by the join path, in both the
variant where the proving anchor lands before the staging is taken and the one
where it lands during the join. A node started by `cmd/accumulated/run` joins
whenever it has executed a block before; a genesis-fresh node does not.

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
none and serves.

A node that HAS joined holds nothing of the blocks it did not execute, and
never will — there is no backfill on this line — so a request for one of them
is answered `NotReady` naming the block it joined at, not `NotFound`. The
difference matters: the requester counts `NotFound` as a miss and a run of
misses strands a stream for good (healing.md, "Stranded streams"), while
`NotReady` is "ask someone who has it".

**Differences from the issue as written**:

- **Serving resumes when the node executes (ACTIVE), not when its history is
  backfilled (COMPLETE)**, because there is no backfill: requiring COMPLETE
  would mean a node that joined never answered anything again. What it cannot
  answer it refuses by the rule above, so the difference is which answer a
  peer gets, not whether it is misled.
- **The state is not persisted** (#4300; `bootpersist` is not ported). A
  restart joins again, which reaches the same answer.
- **It is not advertised** (#4300) in the node's service record, so
  `FindService` still returns a joining node and the caller learns its state
  from the refusal rather than from the listing. The `unavailable` outcome
  label the issue asks for is therefore not added: the requester cannot tell a
  joining node from a node whose entries are in flight, and both mean "ask
  again".
- **The v3 querier is not gated** (#4297)**.** A joining node still answers
  account state and BPT pages from a store the pull has half filled. The hazard is
  another joining node pulling its unverified spine from it — `pull.Account`
  in `ModeFullSpine` is explicitly unverified, because it is what the verifier
  reads from — and then never pulling the spine again. Gating the querier
  would also stop a joining node answering ordinary reads about itself, which
  is why it is recorded rather than done.
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
won by one arbitrary node per partition (#4304). A joining node also rejected
14,643 user transactions against its own un-executed store, because the
submitter is not gated either (#4307). The Docker chaos run (`30m-100tps-
chaos.conf`, then 24 h) is the proof, and it is a human step. Two known holes
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
  (`observer_prod.hashPendingV2`; `History` is *not* hashed), the scheduled-
  events BPT on a partition ledger, and the delivery queues on the synthetic
  account; the pull fetches none of them. An account carrying any of them
  cannot be verified, so it cannot be pulled, so a partition holding one
  cannot be joined. The v3 API has no surface for most of it.
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
  first is #4294's, the rest are of the rejected trust model.
- The v3 `block` query's entry paging ignores `start`, so it cannot be used to
  page through what a block touched. Untouched here (#4302).

**Decided (Paul, 2026-09-18)**: a starting node takes its staging from a
running validator through an API, keeps it current from consensus while it
pulls the state the buffered blocks name, and executes from the block after
its root matches; a restart is the same path, not a consensus replay. PLAN
E11 lists the five steps.

**Collecting (#4292, done)**: the executor takes a peer's staging
(`Executor.LoadStaging`, from #4291's snapshot, refusing a stage that is not
empty and streams that are not this partition's), takes a committed block into
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
  join must start again from a newer snapshot; nothing yet does that restart
  (#4294).
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

**The changed set comes from the block ledger (Paul, 2026-09-18)**: the code
derives it from a block's envelopes (`collect_block.go`, `accountsNamed`),
which names principals, signers and anchor pools and therefore misses the
system accounts every block changes and admits `acc://unknown`. The block
ledger already records every `(account, chain, index)` a block changed and is
committed to by the state root, so it is servable with a proof; executor.md
"Sync" step 3 now specifies it as the source. Until that lands, no local tree
can reach an anchored root.

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
