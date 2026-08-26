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

## MEASURED: the assumption is FALSE, and it sinks phase 1 as designed

Phase 1 above decides pending-vs-delivered before execution. That is only sound
if the inner transaction's status cannot itself be `Pending` — the flag comes
from `st.Pending()`, i.e. AFTER `callMessageExecutor`.

Reading suggested it held. A probe at the exact call site, run over the whole
e2e suite, says otherwise:

    214  isAnchor=false  body=syntheticDepositTokens
    144  isAnchor=true   body=directoryAnchor
     18  isAnchor=true   body=blockValidatorAnchor

The anchors are unsurprising — an anchor waits on a validator signature
threshold. The 214 are not: **ordinary synthetic deposits go Pending after
execution.** So the ledger update genuinely depends on the execution result and
phase 1 cannot finalise it.

### Why a third phase does not rescue it

owner -> shards -> owner would let phase 3 apply delivery marks in stream
order. But `isReady` gates on `Delivered+1 == seq.Number`, so message N+1 of a
stream cannot be dispatched until N's mark lands in phase 3 — one message per
stream per block. That is #4163's ceiling again, which is the thing this design
was supposed to preserve.

### What that leaves

The readiness gate makes each SOURCE STREAM an inherently serial chain: N+1's
eligibility depends on N's outcome. So the available parallelism is ACROSS
streams, not within one.

Sharding by `seq.Source` would give each stream one owner, keep the ledger
consistent with no lost update, and let different sources run concurrently. The
cost is that the inner transaction then executes on the STREAM's shard rather
than the destination's, so a synthetic and a user transaction touching the same
account could land on different shards — trading this collision for a worse
one.

Resolving that tension is the open design question. It is not a small one, and
this plan should not be implemented until it is answered.

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
