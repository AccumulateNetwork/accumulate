# Executor — Specification

## 1. Architecture — what we are doing

The executor turns an ordered stream of messages into blocks of executed state.
It is the only thing that writes protocol state, and it is deterministic: every
validator given the same messages in the same order produces the same block and
the same state hash.

### The pipeline

```
consensus ──▶ executor ──▶ block ──▶ state
              (staging)
```

- **Consensus is the only input.** The executor takes messages from consensus
  and nothing else.
- **Staging belongs to the executor.** Messages that cannot be executed yet are
  held there.
- **Blocks are output.** Nothing a block writes feeds back into a staging
  decision.

Every validator receives the same messages in the same order, so every
executor stages identically. Staging is therefore consistent without being
agreed, and needs no consensus state of its own.

### Ordering

Cross-partition messages are **sequenced**: each stream — one source, one
destination, one kind — numbers its messages, and they must execute in order
with no gaps. A message whose predecessor has not arrived cannot execute; it
waits.

Within a block the order is fixed and is a decision, not an accident:

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
2. **Everything received is held until it can be processed.** Staging is
   unbounded, because the protocol offers no alternative: a message that cannot
   be dropped and cannot yet execute must be kept.
3. **A message that reaches staging has been accepted.** Accepted means
   recorded. There is no state in which the node holds a message and reports
   not holding it.
4. **Block state never feeds back into staging.** What a block delivered is an
   output.
5. **A ready message executes; a not-ready message executes nothing.** The two
   are decided before execution, from the stream's position alone.

### Versioning

Behaviour that changes what a block produces is gated on an `ExecutorVersion`
so that nodes at different versions do not disagree about the same block —
`V2Baikonur` (6), `V2Jiuquan` (8), `V2Kourou` (10, collection proofs). A gate
exists to protect a network that is running; it is not ceremony to apply to
code that has never been deployed.

## 2. Specification — how it is implemented

### Interfaces

`internal/core/execute/execute.go`:

```go
type Executor interface {
    LastBlock() (*BlockParams, [32]byte, error)
    Init(validators []*ValidatorUpdate) (additional []*ValidatorUpdate, err error)
    Validate(envelope *messaging.Envelope, recheck bool) ([]*protocol.TransactionStatus, error)
    Begin(BlockParams) (Block, error)
}

type Block interface {
    Params() BlockParams
    Process(envelope *messaging.Envelope) ([]*protocol.TransactionStatus, error)
    Close() (BlockState, error)
}
```

`BlockState` reports whether the block was empty, whether it completed a major
block, whether it updated validators, and carries the change set and block hash.

### Message executors

`internal/core/execute/v2/block/msg_*.go`. Each message type registers an
executor into `messageExecutors` at init:

- `registerSimpleExec[T]` — always available.
- `registerConditionalExec[T]` — available only when a predicate holds, which
  is how executor-version gating is expressed. `SyntheticProof` is registered
  only when `V2KourouEnabled`, because what a node is willing to accept is
  consensus critical.

Types include transactions, signatures, sequenced messages, synthetic messages
and their proofs, block anchors, credit payments, signature requests, network
updates and maintenance operations.

### Staging

`exec_stage.go`, `exec_stage_run.go`, `stream*.go`. Before anything executes,
each stream's work for the block is settled into a `streamRun`:

```go
type streamRun struct {
    stream stream
    run    []runEntry   // executes this block, in order
    stage  []*arrival   // held for a later block
}
```

`executionOrder` composes the runs in the group order above. The decision is
made once per stream per block, not per message — the executor previously read
the ledger inside every message's own child batch, and because a child does not
share its parent's value each read deep-copied the whole ledger, making a drain
of n messages cost O(n²) (`TestSequenceLedgerCostIsPerRead`).

`streamPosition` is the block's working copy of a stream's position: `delivered`,
`received`, and the staged entries between them. Read once per stream per block,
advanced in place as the block executes, written back when the block closes.

### Sequenced messages

`msg_sequenced.go`. `isReady` asks the block's position — not the ledger —
whether the message is next on its stream. Ready messages execute; not-ready
messages are recorded pending and execute nothing. `Process` records the
message and its status first, then advances the stream, so an advance lands only
once everything the message records has.

## Known gaps

These contradict section 1 and are filed, not fixed here:

- **Staging state lives in an account** (accumulatenetwork/accumulate#4187).
  `PartitionSyntheticLedger.Pending` is main state of an account of type
  `AccountTypeSyntheticLedger`, so it is hashed into the BPT and rewritten every
  block. That violates invariants 2, 3 and 4: because the record is rewritten
  whole each block the pending array must be bounded
  (`MaxPendingSequenced = 4096`), so receipts past the delivery point are
  refused, so the ledger reports not holding messages the node has, so the
  healer re-fetches them. Measured in soak `20260902T132651Z`: 8,556 distinct
  sequence numbers re-fetched 53,011 times, some 41 times each, while all three
  partitions stayed live.
- **`isReady` consults a position derived from that account**, which is block
  state feeding back into a staging decision — invariant 4.
- **How the executor's staging position is restored after a restart is not
  specified**, and will need to be once staging stops reading the ledger.
