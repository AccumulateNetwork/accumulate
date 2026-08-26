# Sharding synthetic delivery (#4145 follow-on)

Synthetic transactions execute against their destination account, exactly like
user transactions, and everything transaction-shaped about them already shards:
state, the principal's main/scratch chain, message records (hash-keyed).

They are excluded from parallel execution anyway, by a blanket type check:

    // exec_parallel.go, envelopeIdentity
    default:
        return nil, false // anchors, synthetics, sequenced, internal — serial

That rule is stated as hazard (i) and never tested against what a synthetic
actually writes. This plan routes them like user transactions and gives the one
genuinely shared component an owner.

## The only real blocker

`SequencedMessage.process` calls `updateLedger` unconditionally after executing
the inner message, and it read-modify-writes ONE record per partition:

    u := ctx.Executor.Describe.Synthetic()     // same for every destination
    batch.Account(u).Main().GetAs(&ledger)
    partLedger.Add(...)
    batch.Account(ledger.GetUrl()).Main().Put(ledger)

Route two synthetics by destination and they land on different shards. Under
`BeginConcurrent` each child memoizes privately, so both read the same base and
both write their whole copy back. Commits are serial, so the last writer wins:

    S1 -> alice, seq 100    both children load Delivered=99
    S2 -> bob,   seq 101    both call Add(delivered)

`Add` advances `Delivered` by max (survives) but shifts `Pending` positionally
(`s.Pending = s.Pending[1:]`). Two deliveries, one shift applied — `Pending` is
then misaligned against its `sequenceNumber - Delivered - 1` index.

The replica (#4140) does NOT avoid this: it removes the proof requirement, not
the ledger update.

Same account, same problem: `CascadeDeliveryQueue()`.

## Why not defer the ledger write to block end

It is the obvious fix and it is wrong. `isReady` gates on
`partitionLedger.Delivered+1 == seq.Number` — strictly the next in the stream.
If the write is deferred, message N+1 in the same block still sees the
pre-block `Delivered`, fails its readiness check, and is recorded pending.

That is exactly the #4163 ceiling the cascade window was built to remove: a
~600-message backlog drained at **1.04 messages per block**, barely above its
own refill rate. Deferring re-introduces it.

## The design: ledger first, then the account

Two phases inside one block.

**Phase 1 — the ledger owner.** One shard owns
`<partition>.acme/synthetic`. It walks each stream's incoming messages in
sequence order and, per message:

  - verifies the proof anchor (a READ of the anchor pool)
  - checks `isReady` against the live ledger
  - not ready, or unanchored -> record receipt, stop that stream
  - ready -> advance `Delivered`, dispatch the inner transaction onward

Because one owner applies the updates sequentially in stream order, `Pending`'s
positional shift stays correct with no sorting, and message N+1 sees N's
advance — intra-block stream progress is preserved, so #4163 stays fixed.

**Phase 2 — the identity shards.** The dispatched transaction executes on
`shard(destination)` like any user transaction.

Cost: a barrier between the phases. Cheap for user-heavy blocks; it serializes
the front of a synthetic-heavy block. That is the same fixed cost as any
owner-based scheme, paid at the cheap stage (a watermark update) rather than
the expensive one (execution).

## MEASURED: the assumption HOLDS

Phase 1 decides pending-vs-delivered before execution. That is sound only if a
message that actually EXECUTED cannot come back `Pending` — the flag comes from
`st.Pending()` after `callMessageExecutor`.

A first probe appeared to refute this (376 hits). It was placed wrong: it sat
after BOTH branches of

    if ready { st, err = ctx.callMessageExecutor(batch, msg) }
    else     { st, err = ctx.childWith(seq.Message).recordPending(batch) }

so every out-of-sequence message that never ran counted as "pending after
execution". Re-probed with the branch recorded, over the whole e2e suite:

    214  ready=false  isAnchor=false  syntheticDepositTokens
    144  ready=false  isAnchor=true   directoryAnchor
     18  ready=false  isAnchor=true   blockValidatorAnchor

**Zero with `ready=true`.** Every Pending is the not-ready branch, and
not-ready is precisely what `isReady` determines from the ledger — knowable in
phase 1, before any execution.

This is consistent with the proof ordering: the anchor proof is verified in
`SyntheticMessage.process` BEFORE the sequenced executor runs, and an unanchored
message returns `errors.Pending` there, short-circuiting so `SequencedMessage`
never executes. Proofs and their signatures settle first; by the time a message
reaches the ledger it is either in-sequence and executable, or out of sequence.

So phase 1 can finalise the ledger, and the two-phase design stands.

### Encode the invariant

The design depends on it, so it should be asserted rather than assumed: a
message with `ready=true` whose status comes back `Pending` must fail loudly,
not silently corrupt the watermark. Absence over one suite is evidence, not
proof — e2e may not cover a synthetic to an account that requires authority.

## Other shared writes on the path, for completeness

| write | scope | disposition |
|---|---|---|
| sequence ledger `.Main()` | one per partition | the blocker; phase 1 owns it |
| `CascadeDeliveryQueue()` | same account | phase 1 owns it |
| `seedSyntheticReplica` | per source stream | partitions cleanly |
| `SyntheticSequenceChain` | per destination partition | partitions cleanly |
| `ValidatorSignatures` | signer (partition account) | absent for replica-accepted (#4140) |

## Out of scope

Outbound recording (synthetic main chain, its index chain, the per-destination
sequence chains) is written from this block's produced set, so it cannot start
until the producing transactions have executed. #4144 already made it
deterministic and it already runs at block end after `sortProduced`. An owner
shard would gain partial overlap at the price of a new barrier; not worth it
until the inbound side is done and measured.

Note for whoever touches chain routing: `account_chains.go:51` registers
`SyntheticSequenceChain` as `ChainTypeTransaction` with the comment "Bug, this
is actually an index chain". Dispatch keyed on chain TYPE inherits that bug.
Every non-index chain also carries its own index chain (`Chain2.Index()`),
whose entries store positions INTO the base — so a chain and its index must
share an owner and an order.
