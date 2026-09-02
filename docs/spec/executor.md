# Executor — Specification

## 1. Architecture — what we are doing

The executor turns an ordered stream of messages from consensus into blocks of
executed state. It is the only thing that writes protocol state, and it is
deterministic: every validator given the same messages in the same order
produces the same block and the same state hash.

### The path

```
consensus ──▶ sort ──▶ ┌─ anchors:    evaluate ▸ drain ▸ execute ─┐
              (streams)│  synthetics: evaluate ▸ drain ▸ execute  │─▶ batch ─▶ commit
                       └─ user:       evaluate ▸ drain ▸ execute ─┘   (one, at close)
```

A message must satisfy each of the following before it executes. Validity is
decided on the message alone; admissibility and readiness are decided *by
staging*, which is why they are not separate passes:

1. **Consensus** decides which messages exist and in what order. It is the only
   input. Batches are executed in the certificate's canonical payload order —
   any node-local order diverges chain entries and BPT roots across validators.
2. **Validity** is settled first, on the message alone. A proof that does not
   hash to its own claimed anchor is refused outright — `BadRequest`, never
   staged. This needs no state beyond the message, so it cannot be deferred and
   is not a timing question.
3. **Admissibility** is a question of *timing*: is the proof's terminal anchor
   one this node has yet? Because validity is already established, a negative
   has exactly one meaning — the destination's directory-root knowledge has not
   caught up to the range the proof covers. That is ordinary lag, so the message
   is pending and retried, never failed. Failing it would be terminal: the same
   message could never be re-applied once the anchor arrived.
4. **Readiness** is whether the message is *next* on its stream. Sequenced
   streams execute in order with no gaps; a message whose predecessor is missing
   waits. A user transaction is on no stream and is always ready.
5. **Execution** runs the message. Nothing before this point changes protocol
   state.
6. **The database write is a side effect of execution**, not a stage of its own.
   Executors write into the block's batch as they run; the batch is committed
   once, when the block closes. There is no separate "persist" step and no
   partial commit.

The three groups run in sequence, each finished before the next is evaluated.
Executing one group changes the state the next is evaluated against — anchors
extend the chain synthetics are judged by — and that is the sequence doing its
job, not a feedback loop. Within a group nothing is re-asked.

What must never feed back is the block's persisted output. Staging reads the
streams' positions and the anchor chain as the block builds them; it does not
read the ledger record the block writes at its close.

### Validation is a separate, earlier thing

`Executor.Validate` is the pre-consensus check — CheckTx's equivalent. It runs
against a read-only batch that is always discarded, so it writes nothing and
changes nothing. It exists so a node can refuse a malformed or unpayable
envelope before it is gossiped and sequenced, not to decide anything about
execution.

A message that reaches execution is validated again on the path above, by the
gates that can only be evaluated with block state in hand.

### Draining

A stream's messages arrive out of order. One that is not next cannot execute, so
staging holds it — and when the message it was waiting for arrives, everything
held behind it becomes executable at once.

**Draining a stream is executing that backlog**: the contiguous run of staged
messages starting at the delivery point, in order, until the next gap. It is the
only way a stream advances by more than one message, and it is why a single
missing message stalls a stream and a single arrival can release thousands.

The word is used for one other thing in this document, and they are unrelated:
the **delivery queues** drained at `Begin` hold locally produced messages, not
staged stream messages. Where the distinction matters the text says which.

### Sort, then three groups in turn

Everything consensus delivers is **sorted once**, in a single pass. Each message
is asked which stream it belongs to; if it belongs to one it is recorded as an
arrival on that stream, keyed by its sequence number, and the first sighting of
a number wins — the same message may appear twice in a block and applies at most
once. A message belonging to no stream is a user transaction.

So anchors and synthetics are sorted up front. There is no later step that
discovers more of them.

Then **three groups, each finished before the next begins**. A group is
evaluated, drained and executed; only then is the next group evaluated.

1. **Anchors.** Evaluated — each is admissible or not, by quorum or proof —
   drained, and executed.
2. **Synthetics.** Evaluated *after* the anchors have executed, so the directory
   anchor chain they are judged against already carries this block's anchors.
   That frees as much of the synthetic backlog as can be freed. Drained, and
   executed.
3. **User transactions.** Drained and executed last, so a deposit has landed
   before a transaction spends it. A send that would fail on a stale balance
   succeeds instead: strictly more permissive, and deterministic either way.

Within a group, streams run in canonical source order — the directory first,
then partitions by ID. Any fixed rule would do; what matters is that every node
uses the same one.

**Each group is evaluated once**, and the sequence is what makes that
sufficient:

- a synthetic's admissibility depends on the anchor chain, and the anchors that
  extend it have already executed by the time synthetics are evaluated;
- a stream's run is computed from its arrivals **and what is already staged**,
  so a message arriving this block that unblocks a backlog from earlier blocks
  is part of that stream's run when it is computed — nothing about it becomes
  true later;
- a user transaction is on no stream and cannot unblock one. What it produces
  for this partition goes on the delivery queue and executes next block, so it
  cannot free a synthetic within this block either.

Nothing is re-asked, because nothing that could change an answer happens after
the answer is given.

`maxRunPerBlock` (1,024) bounds how far one evaluation may carry a single stream
in a block, so no block inherits an unbounded run.

The predecessor of this design re-read the ledger after every single delivery,
which is where its O(n²) came from. One evaluation per stream per block reads
the position once.

### Restart

**Staging survives a restart.** It is durable, and a node that has lost it has
not lost performance — it has lost agreement.

The reason is that staging decides what executes. A block delivers the
contiguous run starting at `Delivered + 1`, taken from this block's arrivals
*and from what is already held*; so two nodes holding different things execute
different runs from the same block. Suppose peers hold 5 through 9 and a
restarted node holds nothing. Number 4 arrives. The peers deliver 4 through 9;
the restarted node delivers 4 alone. Different `Delivered`, different account
state, different BPT root — a divergent block hash. That is a consensus fault,
not a node that is briefly behind, and healing cannot repair it because healing
is asynchronous and the divergence is immediate.

So "empty on restart, refilled by healing" is not available. It would be sound
only if staging were an optimisation, and it is not: it is an input to what the
block does.

The two halves are therefore both durable, and they are durable in different
ways because they answer to different things:

- **`Delivered` is block output and is hashed.** What a block executed is part
  of what the block agreed. It lives in the stream's ledger, in an account, in
  the BPT.
- **What is HELD is durable and NOT hashed.** It is a deterministic function of
  the consensus stream — every node is fed the same messages and holds the same
  set — so it does not need to be hashed to be agreed. Hashing it is what forced
  it into an account, and being in an account is what forced it to be bounded.

Not hashing it is the whole of the fix. The bound existed because the held set
was an array in a record rewritten and hashed every block; a record written per
entry and outside the BPT has no such limit.

Two consequences follow, and both are requirements rather than remarks:

- **Staging is part of a snapshot.** A snapshot is what a new node starts from,
  so a snapshot without staging produces exactly the divergence above on the
  first block the new node executes.
- **Nothing derived from staging is written into hashed state** unless it is
  derived through execution. `Delivered` qualifies: it is what executed. A
  convenience copy of "how far this stream has been sighted" does not — a node
  restoring from an older snapshot would write a different number than its
  peers, for a field nothing needs.

The message bodies were never the problem: they are recorded when they are
accepted, before the stream advances, and they are already durable. What was
lost across a restart was only the INDEX of which numbers are held — which is
precisely what the old bound discarded, and what this makes durable instead.

### A block does not begin with an empty slate

Opening a block finishes the previous one. Before any of this block's messages
are seen, `Begin`:

- captures the previous block's BPT root, and where the directory anchor chain
  stood before this block applies anything to it;
- **finalizes the previous block** — records its anchor if it has not been
  recorded, and dispatches the synthetic messages it produced. These are
  independent duties: skipping the anchor must not skip the synthetics;
- resets the ledger's transient values and refuses to move backwards — a block
  index that does not increase is a panic, not an error;
- records the previous block's votes and evidence, unless that block was empty;
- **drains the delivery queues** — everything the previous block queued, local
  synthetics and locally produced messages, is delivered before any of this
  block's own. These are the delivery queues, not staged streams.

So "the executor's input is consensus" is true of *messages*; the block's work
also includes the tail of the block before it.

### Parallel execution

A block may execute independent user envelopes in parallel across shards. What
may be parallelised is decided by identity, and the rule is adversarial:

**Classification never trusts a submitter's claim.** A remote stub's principal
and a signature's TxID account are claims. The executor loads the real
transaction by hash and writes *its* principal's records, so classifying by the
claim would let a crafted envelope execute another identity's writes on the
wrong shard. Claims are resolved to the real transaction, and anything
unresolvable is serial.

Serial by construction: any non-user message, any signature that is not a user
key signature, any system or partition identity, ACME, any held transaction,
and any envelope spanning more than one identity. Sequenced messages are serial
too — they belong to streams, and streams are settled by staging — so only user
transactions are ever candidates for a shard.

### The invariants

1. **A stream executes in order, with no gaps.** There is no skip.
2. **An invalid proof is refused; a not-yet-provable one waits.** The two are
   different answers to different questions and must not be conflated. A proof
   that does not verify is a `BadRequest` and never enters staging. A proof that
   verifies but names an anchor this node lacks is pending, never terminally
   failed.
3. **Everything received is held until it can be processed.** Staging is
   unbounded, because the protocol offers no alternative: a message that cannot
   be dropped and cannot yet execute must be kept.
4. **A message that reaches staging has been accepted, and accepted means
   recorded.** There is no state in which the node holds a message and reports
   not holding it.
5. **Block state never feeds back into staging.** The only thing the executor
   reads from a stream's ledger is `Delivered` — what has been processed.
   Nothing else about an inbound stream lives there.
6. **Staging is identical on every node.** It is fed only by consensus, so it
   is a deterministic function of the same input everywhere; it is durable so a
   restart cannot make it otherwise. A node whose staging differs from its
   peers' will execute a different run and produce a different block hash.
7. **State changes only as a side effect of execution**, and become durable only
   at the block's single commit.
8. **A ready message executes; a not-ready message executes nothing.**

### Versioning

Behaviour that changes what a block produces is gated on an `ExecutorVersion` so
nodes at different versions do not disagree about the same block — `V2Baikonur`
(6), `V2Jiuquan` (8), `V2Kourou` (10, collection proofs). A gate protects a
network that is running; it is not ceremony for code that has never been
deployed.

## 2. Specification — how it is implemented

### Opening a block

`block_begin.go`, `Executor.Begin`:

1. Opens the block's writable batch, discarded if anything below fails.
2. Publishes `WillBeginBlock`.
3. Reads the previous BPT root into `State.PreviousStateHash`, and
   `dnAnchorsAtStart` — the directory anchor chain's height before this block
   applies anchors (#4169 step 0c), which staging uses to tell an anchor applied
   this block from one applied earlier.
4. `finalizeBlock` for the previous block: records its anchor if unrecorded, and
   sends the synthetics it produced. The anchor is recorded for the last
   *non-empty* block rather than strictly the previous one — under CometBFT at a
   block a second the two nearly always coincide, but DAG-BFT produces a block
   per committed certificate, dozens a second, and a one-block window that is
   missed stalls the anchor sequence (#4054: 4 anchors recorded of 55 anchored
   blocks). That changes when anchors are recorded, which is state, so it is
   version-gated to preserve replay of pre-Kourou history.
5. Loads the ledger and **panics** if the index does not increase.
6. Resets transient ledger values: index, timestamp, pending updates, ACME
   burnt, anchor.
7. Captures votes and evidence as data entries, unless the previous block was
   empty.
8. `drainDeliveryQueues` — delivers what the previous block queued (#4146).

### Entry from consensus

`pkg/consensus/adapter/executor_bridge.go`, `ProduceBlock`:

1. `executor.Begin(BlockParams)` opens the block.
2. Every batch named by the committed certificate is walked **in payload
   order**. A nil batch is fatal: `CollectBatches` guarantees a complete set,
   and executing a certificate without one of its batches silently diverges
   state (#4116/#4119).
3. Each transaction is unmarshalled into an envelope and processed —
   `ProcessAll` when the block supports parallel execution (#4145), otherwise
   `Process` per envelope.
4. `block.Close()` produces the block state; `state.Hash()` then
   `state.Commit()`.

Accounting is emitted per non-empty block — arrived, executed, unmarshalFailed,
processFailed, statusFailed, sharded, serial, shardsUsed — because 95 of 100
submitted transactions once vanished between acceptance and execution with no
log line anywhere (#4132). This is the seam where consensus hands to execution,
so it is where "lost in consensus" and "lost in execution" separate.

### Validation

`exec_validate.go`, `Executor.Validate`: begins a read-only batch, normalizes
the envelope, rejects unsigned transactions, and calls each message's validator
through a bundle whose block is a shell. The batch is discarded unconditionally.

### Execution

`exec_process.go`:

- `Block.Process(envelope)` normalizes, calls `processEnvelope`, then merges the
  resulting bundles into block state. The merge is the caller's so that under
  parallel execution every touch of shared block state happens serially, in a
  deterministic order.
- `processMessages` runs the messages and every pass of additional messages they
  cascade into.
- `bundle.callMessageExecutor` finds the executor registered for the message
  type and calls `Process`. Internal message types are refused on the first
  pass.
- Statuses returned to consensus are **cleaned**: the result and a success code,
  or a generic error code. Error messages are porcelain and differing text
  across nodes would be a consensus failure; the API reads real status from the
  database instead.

### Message executors

`msg_*.go`, registered at init into `messageExecutors`:

- `registerSimpleExec[T]` — always available.
- `registerConditionalExec[T]` — available only when a predicate holds, which is
  how version gating is expressed. `SyntheticProof` registers only under
  `V2KourouEnabled`, because what a node is willing to accept is consensus
  critical.

### Validity — the proof itself

`msg_synthetic.go`, `SyntheticMessage.check`, before anything else:

- A collection proof (`ReceiptList`) is refused unless `V2Kourou` is active, and
  a proof may carry a receipt or a receipt list, never both.
- `ReceiptList.Validate` replays `MerkleState` through `Elements`, requires the
  last element to be the receipt's start and the recomputed anchor to equal the
  receipt's anchor, then validates the receipt. So a list proves every element
  it carries, in any order, without needing any other element — and it also
  proves each element's absolute index, because the state is counted.
- A list is validated **once per envelope**, not per member: a package's members
  share one proof, and rehashing it per member is a CheckTx denial of service.
- `MaxReceiptListElements` (4,096) bounds how many elements a proof may carry.
  Unlike the other bounds in this document, a receipt list is untrusted input
  and verification hashes every element before it can know the proof is junk, so
  the bound limits what an attacker can make a validator do. It binds in three
  places and all three must agree: the sender will not build a package whose
  span exceeds it (`packageSpanFits`), the sequencer refuses a range request
  larger than it, and the receiver rejects a proof carrying more.
- Failing any of these is `BadRequest`. The message does not reach admission or
  staging.

### Admission — the timing gate

`msg_synthetic.go`, `SyntheticMessage.process`:

- A replica-accepted message (#4140) carries no proof; its proof was checked and
  absorbed when it first arrived.
- Otherwise `isAdmissible` asks one question and does no verification:
  `AnchorChain(Directory).Root().IndexOf(anchor)`. The anchor's own signatures
  were checked when it was applied to that chain, and the proof's integrity was
  checked above, so the only thing left to establish is whether this node has
  the anchor yet. Individual and collection proofs terminate at the same trust
  root, so one check covers both. `provingAnchorIndex` returns where in the
  chain it sits, which staging uses to tell an anchor applied THIS block from
  one applied earlier (#4169 step 0c).
- A missing anchor returns `errors.Pending` under `V2Kourou`, not a failure:
  failing it terminally wedges recovery, because the same message could never be
  re-applied once the anchor arrived (#4048).

An anchor's gate is a validator signature quorum rather than a proof to a
directory root, with one shortcut: a collection proof under a known directory
root authorizes the anchor by itself (#4056). A not-yet-arrived anchor does not
reject it — it falls through to the quorum, because healing resubmits until a
current anchor extends the destination's knowledge past the proven range.

So a message that is not yet provable never reaches sequencing, and one that can
never be proven never reaches admission.

### Staging — the ordering gate

`exec_stage.go`, `exec_stage_run.go`, `stream*.go`. Before anything executes,
each stream's work for the block is settled:

```go
type streamRun struct {
    stream stream
    run    []runEntry   // executes this block, in order
    stage  []*arrival   // held for a later block
}
```

`executionOrder` composes the runs in the group order above. The decision is
made once per stream per block, not per message: the executor previously read
the ledger inside every message's child batch, and because a child does not
share its parent's value each read deep-copied the whole ledger, making a drain
of n messages cost O(n²) (`TestSequenceLedgerCostIsPerRead`).

`streamPosition` is the block's working copy of one stream: `delivered`, plus a
reference to staging for what is held. It is built once per stream per block
from the ledger's `Delivered` and advanced in place, and at close **only
`Delivered` is written back**. It holds a reference rather than a copy of the
held set — a copy is a snapshot, and a snapshot of what the node holds
disagreeing with what the node holds is the whole defect.

### Staging is a store, and this is what it answers

Staging is one store per node, shared by everything that needs it. It is not the
block's, and it is not the healer's: both ask the same store, because two views
of what the node holds is exactly the disagreement that livelocked the network.

A stream is named by the account whose ledger tracks it and the partition the
messages come from. Anchors and synthetics between the same pair of partitions
are separate streams — anchors tracked by the anchor pool, synthetics by the
synthetic account — so conflating them would let an anchor's position gate a
synthetic's.

| question | asked by | answer |
|---|---|---|
| hold this number | the executor, on a receipt | recorded; the first sighting of a number wins |
| do we hold *n* | the executor, building a run | the message ID, or no |
| how far has this stream been sighted | healing | the highest number ever held: what says a stream is behind |
| what is missing in (delivered, through] | healing | the contiguous runs nothing holds, oldest first, bounded |
| which streams exist | healing | every stream holding anything |
| release through *n* | the block, on commit | everything at or below *n* is dropped |

Four rules govern it, and each of them is a defect that has actually happened:

**The first sighting of a number wins.** A number can be offered twice — a block
discarded and re-executed, a healed message racing the original — and both carry
the same message, because the number identifies it. Keeping the first means the
same input always produces the same staging.

**The set of streams is staging's, not the ledger's.** A stream that has only
staged has delivered nothing, so it has no ledger entry to be found by. Before
the split, an entry existed regardless, because receipts were written into the
ledger — the coupling being removed. A stream staging holds nothing for has
nothing to heal: it is either current, or it has lost a tail nobody here can
see, which is reconcile's job and reaches it from the source's `Produced`.

**The high-water mark does not go backwards.** Releasing what was delivered
drops the held entries but leaves the mark: "this stream was behind" is what
makes a hole below the mark a hole, and forgetting it says the stream had never
been behind at all.

**Release happens on COMMIT, not at flush.** Until the batch commits the
delivery has not happened. Dropping a staged message for a block that is then
discarded makes the node fetch back across the network something it still holds,
which is the failure this whole change removes, reintroduced from the other end.

### Staging in a snapshot

A snapshot is what a new node starts from, so staging has to be in it — a node
restored without it holds nothing and executes a shorter run than its peers on
the first block where a gap closes.

Two mechanics make that true, and both fail SILENTLY when they are not:

- The records are **account state, not an index.** Snapshot collection walks
  with indices ignored, so an index record is simply absent from the snapshot,
  with nothing said about it.
- A parameterised record is walked only if it can be **enumerated**, so each
  carries a function listing its keys. Without one it is skipped, again
  silently. Enumeration comes from a small set of the sources staging holds
  anything from — bounded by the number of peers — and, for each, the numbers
  between that stream's `Delivered` and how far it has been sighted.

Only what is HELD is collected. A record below `Delivered` is a message that has
executed; nothing consults it, and carrying every stream's whole history into
every snapshot would restore state that answers no question.

Both mechanics are the kind that a test has to pin, because neither announces
itself: the snapshot is written, the restore succeeds, and the node diverges a
block later.

### What the stream ledger is for

Exactly one field, in the inbound direction: **`Delivered`** — what has been
processed. It is read to place the stream and written when the block closes.

Nothing else about an inbound stream lives there. There is no pending array,
because the held set is staging's; there is no received mark, because how far a
stream has been sighted is staging's and writing it back would put a per-node
value into hashed state (see Restart). `Produced` remains, but it belongs to the
other direction — what this partition has produced FOR that one.

And a message at or below `Delivered` requires nothing at all. It is not healed,
not re-recorded, and not counted: it has been processed, and that is the end of
it.

**`Received` is answered, not stored.** Removing the field from the record does
not remove the question, and the question is the one every operator surface
asks: how far is this stream behind. The API fills it in on the way out from
staging's sighted mark, computed on read and never written, so the account on
disk carries no trace of it and nothing about consensus depends on the answer.

Writing it back instead would be the mistake. A value derived from staging,
placed in an account, makes a staging discrepancy a divergent block hash rather
than a wrong number on a dashboard. And simply dropping it is the other
mistake: every reader then sees zero, which does not read as "no data" — it
reads as "nothing ever arrived", and paints a healthy stream as stalled.

`msg_sequenced.go`, `SequencedMessage`: `isReady` asks the block's position
whether the message is next. Ready messages execute; not-ready messages record
pending and execute nothing. `Process` records the message and its status, then
advances the stream — the advance is deferred so it lands only once everything
the message records has, and never on a path that discards.

### Closing a block

`block_end.go`, in order — the order is part of the contract, because each step
depends on the last:

1. Write each stream's advances to its ledger, once per stream (#4169 step 7).
2. Decide whether this completes a major block.
3. Process events: expiring transactions and signature sets, against the major
   block height just decided.
4. Settle the block's produced messages, `produceBlockMessages`. This must run
   before the anchor decision, because whether an anchor is needed depends on
   whether anything was *sequenced*, which is only known after the split below.
5. Decide whether an anchor must be sent.
6. **If the block is empty, stop.** Nothing below runs.
7. Record the previous block's state hash on the BPT chain.
8. Record pending transactions; process chain updates; record block entries.
9. Add the synthetic chain to the root chain, index the root chain, update the
   transaction-chain index.
10. Update major index chains if this is a major block.
11. Execute post-update actions.
12. **Update the BPT**, and only then active globals.

### Signatures

Signatures are dispatched twice, through two registries.

- `messageExecutors` handles `SignatureMessage`, which checks the wrapper — a
  signature and a transaction ID must both be present — and then calls the
  signature executor for the signature's *type*.
- `signatureExecutors` holds the type-specific executors: `UserSignature` (the
  ordinary key signatures, plus a conditional variant), `AuthoritySignature`,
  and `EthereumDataSignature` under its own condition.

So a user signature is not special: it is an ordinary message carrying a
signature, and the second dispatch exists only because the signature's type
decides how it is verified.

### Anchor signatures are not that path

A block anchor is a **message** executor, `BlockAnchor` in `messageExecutors`,
and never reaches `signatureExecutors`. It is authorized in one of two ways:

- **A validator signature**, counted towards a quorum. The signer must be a
  validator of the anchor's *source* partition, and the source must parse as a
  partition URL.
- **A collection proof under a known directory root** (#4056). This authorizes a
  *healed* anchor without re-gathering a quorum, because a historical quorum may
  be impossible to re-gather after validator churn while the proof depends only
  on the current directory root, which every synced node has.

The proof is checked the same way a synthetic's is — receipt list only, never
both forms, bounded by `MaxReceiptListElements`, and `Validate`d — and the same
bound applies. An anchor with neither a signature nor a proof is `BadRequest`.

The anchor itself must be a sequenced anchor transaction whose destination's
root identity matches the transaction's principal, and a remote placeholder is
resolved to the real transaction by hash before the type is checked.

So the difference is not the signature algorithm. It is **when** authorization
is decided:

- **A user signature is checked as part of execution.** Authority, thresholds
  and delegation are state-dependent, and evaluating them *is* execution work.
- **An anchor is authorized before execution.** Quorum or proof is a
  precondition, not an effect, so it is a staging decision.

### Anchor authorization belongs to staging

Signatures for an anchor route to **staging**, not through execution. They are
inputs to a decision, not state changes.

Staging holds the one anchor and collects the signatures that arrive for it,
packs them together, and evaluates the quorum — or the collection proof, which
needs no accumulation at all. Once it answers yes the anchor is **authorized**,
and it executes once, in one block, **with no further checking**: everything an
executor would re-verify has already been verified, so execution applies the
anchor rather than re-deciding whether it may.

The payload deduplicates naturally under this shape. Copies of an anchor from
different validators are identical, so they collapse to one; today they cannot,
because each `BlockAnchor` embeds a different signature and therefore hashes
differently, and `checkStatus` never short-circuits.

Determinism is free. Signatures arrive through consensus like everything else —
there is no out-of-band path — so every node accumulates the same set by the
same block and authorizes at the same block.

This also removes an asymmetry. An anchor authorized by proof executes on first
arrival; one authorized by quorum takes as many executions as it takes
signatures. Under this shape both are a staging decision followed by one
execution.

### There is no cascade

A message does not queue further messages into a later pass of the running
bundle. Staging decides the whole run before anything executes, so a successor
does not need to be discovered while its predecessor is running.

What replaced it is the run entry. Staging places the messages that will execute
this block, and each is entered through an internal `MessageIsReady` naming the
staged message; the executor loads it and calls its executor.

**A run entry enters at pass 1, not pass 0**, and the number is load-bearing.
Internal message types cannot be marshalled, so one arriving in a submitted
envelope would have to be forged — the executor therefore refuses internal types
at pass 0. A run entry is internally generated in exactly the same sense as the
old mechanism's queued message, which was handed to a *later* pass, so it must
enter at a later pass too.

Getting it wrong is silent. The guard returns an error *status* rather than an
error, so a staged entry looks like it ran: every staged entry fails, every run
stops at its first one, and only freshly arrived messages are ever delivered.
The symptom is a backlog that cannot close — 40 delivered per block against 40
arriving — and it is invisible in the statuses. It is found by asking the ledger
whether it moved.

The one thing still carried to a later pass is a **network update**, produced
when a directory anchor brings one. That is a genuine consequence of executing
the anchor, not a deferred delivery.

### Parallel execution

`exec_parallel.go`. `classify` sorts every envelope once: each message is asked
for its stream through `streamOf`, recorded as an arrival keyed by sequence
number with first sighting winning, and an envelope whose messages belong to no
stream is a user transaction. `stageRuns` then decides one kind of stream's runs.
`ProcessAll` runs the user transactions across `ExecutionShards`; at a shard
count of one or less that is exactly a loop over `Process`.

`envelopeIdentity` returns the one identity an envelope's messages belong to, or
`(nil, false)` meaning serial. Serial covers any non-user message, any signature
that is not a user key signature, any system or partition identity, ACME, any
held (`HoldUntil`) transaction, any transaction that cannot be resolved to its
real content, and anything spanning more than one identity. Network accounts are
serial even though they are user-writable, because they mutate shared executor
state.

Claims are resolved rather than trusted: a remote stub's principal and a
signature's TxID account are claims, and classifying by them would let a crafted
envelope execute another identity's writes on the wrong shard (#4149).

Timing is booked as serial versus parallel share, so a run can say whether
sharding helped or whether nothing was shardable.

### Produced messages, and the local/remote split

`exec_local_queue.go`. A message a block produces is either **remote** — its
destination routes to another partition — or **local**, routing back to the
partition that produced it.

- **Remote** messages are sequenced and dispatched: they take a sequence number,
  a position on the synthetic main chain, a proof continued to a DN-anchored
  root, and anchor-pool validation on arrival.
- **Local** messages take none of that. They go on a persisted queue and execute
  at the start of the *next* block as ordinary messages with their own
  principal — no sequence number, no synthetic-chain position (so they consume
  no collection-proof span), no dispatch, and no anchoring dependency (#4146).

Next block rather than the same block, and that falls out of the geometry: the
queues are drained at the *start* of a block and written at the *end* of one, so
nothing queued while draining can execute before the next block. A local
synthetic that produces further locals therefore creates ordered work in the
following block — no cascade, and no termination bound to reason about.

The input must already be in canonical order (#4144); the queue preserves it,
which is what makes the drain deterministic. Each entry is
`destination.WithTxID(msgHash)`, so the drain re-checks routing without
re-deriving the destination from the message type.

### The database write

Executors write into the block's `*database.Batch` as they run. A message
executor takes a sub-batch and commits it into its parent on success or discards
it on failure, so a failed message leaves nothing. The block's batch reaches
disk exactly once, at `state.Commit()` in `ProduceBlock`. `state.Hash()` is
taken before the commit, and a failure to hash discards rather than commits.

---

Where the implementation departs from this specification, see
[DIFFERENCES.md](DIFFERENCES.md).
