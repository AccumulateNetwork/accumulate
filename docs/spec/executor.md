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
2. **Admission** decides whether a message is *provable*. A synthetic message
   carries a proof that must terminate at a directory anchor this node has. If
   the anchor has not arrived, the message is not refused — it is pending, and
   is retried when the anchor does arrive.
3. **Staging** decides whether a message is *next*. Sequenced streams execute in
   order with no gaps; a message whose predecessor is missing waits.
4. **Execution** runs the message. Nothing before this point changes protocol
   state.
5. **The database write is a side effect of execution**, not a stage of its own.
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

1. **Anchors**, because an anchor extends the directory root that admits
   synthetics — so a synthetic can use an anchor that arrived in the same block
   rather than waiting for the next.
2. **Synthetics**, so a deposit lands before a user transaction spends it. A
   send that would fail on a stale balance succeeds instead: strictly more
   permissive, and deterministic either way.
3. **User envelopes**, in arrival order.

Within each group, streams run in canonical source order — the directory first,
then partitions by ID. Any fixed rule would do; what matters is that every node
uses the same one.

### The invariants

1. **A stream executes in order, with no gaps.** There is no skip.
2. **Nothing executes until it is admissible.** An unproven synthetic is
   pending, never executed and never terminally failed — failing it would mean
   it could not be re-applied when its anchor arrives.
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

### Admission — the proof gate

`msg_synthetic.go`, `SyntheticMessage.process`:

- A replica-accepted message (#4140) carries no proof; its proof was checked and
  absorbed when it first arrived.
- Otherwise `isAdmissible` verifies the proof terminates at a directory anchor.
  Individual and collection proofs terminate at the same trust root, so one
  check covers both. The check is shared with staging (#4169 step 3); what a
  negative *means* is decided here.
- A missing anchor returns `errors.Pending` under `V2Kourou`, not a failure:
  failing it terminally wedges recovery, because the same message could never be
  re-applied once the anchor arrived (#4048).

So an unproven synthetic never reaches sequencing.

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
