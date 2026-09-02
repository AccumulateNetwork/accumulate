# Executor — Specification

## 1. Architecture — what we are doing

The executor turns an ordered stream of messages from consensus into blocks of
executed state. It is the only thing that writes protocol state, and it is
deterministic: every validator given the same messages in the same order
produces the same block and the same state hash.

### The path

```
consensus ──▶ envelopes ──▶ admission ──▶ staging ──▶ execution ──▶ batch ──▶ commit
                            (proof)      (ordering)   (effects)             (one, at close)
```

Read as a sequence of gates, each of which a message must pass before the next:

1. **Consensus** decides which messages exist and in what order. It is the only
   input. Batches are executed in the certificate's canonical payload order —
   any node-local order diverges chain entries and BPT roots across validators.
2. **Validity** is settled first, on the message alone. A proof that does not
   hash to its own claimed anchor is refused outright — `BadRequest`, never
   staged. This needs no state beyond the message, so it cannot be deferred and
   is not a timing question.
3. **Admission** then decides only *timing*: is the proof's terminal anchor one
   this node has yet? Because validity is already established, a negative here
   has exactly one meaning — the destination's directory-root knowledge has not
   caught up to the range the proof covers. That is ordinary lag, so the message
   is pending and retried, never failed. Failing it would be terminal: the same
   message could never be re-applied once the anchor arrived.
4. **Staging** decides whether a message is *next*. Sequenced streams execute in
   order with no gaps; a message whose predecessor is missing waits.
5. **Execution** runs the message. Nothing before this point changes protocol
   state.
6. **The database write is a side effect of execution**, not a stage of its own.
   Executors write into the block's batch as they run; the batch is committed
   once, when the block closes. There is no separate "persist" step and no
   partial commit.

Nothing later feeds back into anything earlier. What a block delivered is an
output; staging never reads it back as an input.

### Validation is a separate, earlier thing

`Executor.Validate` is the pre-consensus check — CheckTx's equivalent. It runs
against a read-only batch that is always discarded, so it writes nothing and
changes nothing. It exists so a node can refuse a malformed or unpayable
envelope before it is gossiped and sequenced, not to decide anything about
execution.

A message that reaches execution is validated again on the path above, by the
gates that can only be evaluated with block state in hand.

### Ordering within a block

Fixed, and each choice is a decision:

1. **Anchor streams**, because an anchor extends the directory root that admits
   synthetics — so a synthetic can use an anchor that arrived in the same block
   rather than waiting for the next.
2. **Synthetic streams**, decided against the chain the anchors just extended,
   so a deposit lands before a user transaction spends it. A send that would
   fail on a stale balance succeeds instead: strictly more permissive, and
   deterministic either way.
3. **User envelopes**, in arrival order — and anything staging did not place in
   a run.
4. **Drain again**, because step 3 *reveals work to the block*. An envelope that
   arrives out of sequence is recorded pending, and that makes it drainable now
   rather than next block.

Step 4 is not tidying. Without it a message whose predecessor arrives in the
same block waits a whole block for no reason.

Within each group, streams run in canonical source order — the directory first,
then partitions by ID. Any fixed rule would do; what matters is that every node
uses the same one.

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
  block's own.

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
too, which is why staging can settle them before the envelope loop without
changing where they run.

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
5. **Block state never feeds back into staging.**
6. **State changes only as a side effect of execution**, and become durable only
   at the block's single commit.
7. **A ready message executes; a not-ready message executes nothing.**

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

`streamPosition` is the block's working copy of one stream — `delivered`,
`received`, and the staged entries between. Read once per stream per block,
advanced in place, written back at close.

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

### Parallel execution

`exec_parallel.go`. `ProcessAll` runs consecutive single-identity user envelopes
across `ExecutionShards`; at a shard count of one or less it is exactly a loop
over `Process`. Staging settles anchor streams, then synthetic streams, then the
envelope loop runs, then a second drain picks up what the loop revealed.

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

## Known gaps

These contradict section 1 and are filed, not fixed here:

- **Staging state lives in an account** (accumulatenetwork/accumulate#4187).
  `PartitionSyntheticLedger.Pending` is main state of an account of type
  `AccountTypeSyntheticLedger`, so it is hashed into the BPT and rewritten every
  block. Because the record is rewritten whole each block the pending array must
  be bounded (`MaxPendingSequenced = 4096`), so receipts past the delivery point
  are refused, so the ledger reports not holding messages the node has, so the
  healer re-fetches them. That violates invariants 3, 4 and 5. Measured in soak
  `20260902T132651Z`: 8,556 distinct sequence numbers re-fetched 53,011 times,
  some 41 times each, while all three partitions stayed live.
- **`isReady` consults a position derived from that account** — block state
  feeding back into a staging decision, invariant 5.
- **How the executor's staging position is restored after a restart is not
  specified**, and will need to be once staging stops reading the ledger.
- **An anchor's quorum is assembled by execution rather than decided in
  staging.** Each validator sends a full `BlockAnchor` carrying the whole
  payload and its own signature; each is a complete message execution that
  writes `recordMessageAndStatus` and `RecordHistory` and adds one signature to
  `ValidatorSignatures()`, and the copy that crosses `ValidatorThreshold`
  executes the anchor. For an N-validator partition, N−1 of those deliveries
  exist only to deposit a signature.

  Staging already asks the right question — `admissibilityOf` calls
  `anchorIsAdmissible`, the same rule `txnIsReady` uses at execution, shared
  deliberately (#4169 step 3b) — but it has nothing to collect, because the
  signature set only grows as a side effect of those executions. So the rule is
  evaluated twice: once to decide staging, once to decide execution, over state
  that execution had to write first.

  The cost is O(validators) per anchor. Measured in run `20260902T132651Z`,
  anchors were 445 against 180,997 synthetics, so this is small today and grows
  linearly with validator count.
