# Development plan

Ordered from [DIFFERENCES.md](DIFFERENCES.md). The order is by dependency and
by what is actually stopping the network, not by size.

## What is stopping us

Soaks die in twenty minutes and have done for five consecutive runs. The
mechanism is now known and it is one difference: **E1**. Staging lives in an
account, so the pending array must be bounded, so receipts past the bound are
refused, so the ledger reports not holding messages the node has, so the healer
re-fetches them across the partition — 8,556 sequence numbers fetched 53,011
times in run `20260902T132651Z`, while all three partitions stayed live.

Everything in phase 2 exists to close that. Phases 1 and 3 are real work but
neither is why the network stops.

---

## Phase 0 — settle the design question — DONE

**E3 — how the executor's staging position is restored after a restart.**

Answered in [executor.md](executor.md): it is not restored, it is rebuilt.
`Delivered` is block output and survives; everything above it is staging, held
in memory, empty on restart, refilled by consensus and — for anything not
re-delivered — by healing, which is what a gap already means. Staging can
therefore be in memory, and persisting it would mean writing every block a state
reconstructible from what is already durable.

E1 implements it.

## Alongside — cheap and independent

Small, unrelated to the livelock, closeable on their own and in parallel with
anything. **Not sequenced ahead of the critical path**, which is Phase 0 → E1.

| | |
|---|---|
| **D2** | Badger does not run `TestIsolation`. Add it and see whether it passes. If it does not, the difference is much larger than the test. |
| **E6** | `CascadeDeliveryQueue` is dead state still folded into the account hash. Remove the field, the hasher contribution, the snapshot entry, the debug observer. |
| **D3** | `kvtest` does not exercise `BeginDeep`. Add the case: a windowed backend must answer a deep read correctly and report absence rather than guessing on a shallow one. |

## Phase 2 — the livelock

In order. Each depends on the one before.

**E1 — move staging out of the account model.** Staging becomes executor state
fed only by consensus. `Pending` and `Received` leave `PartitionSyntheticLedger`;
`Delivered` stays, because what a block delivered is its output.
`MaxPendingSequenced` and the refusal at `stream_position.go:174` go with them:
everything received is held until it can be processed, and a message that
reaches staging is recorded.

**E2 — `isReady` stops reading block state.** Falls out of E1; listed
separately because it is the invariant being restored, not a side effect.

**H2 — order healing.** Newest gap to lowest, skipping what is already staged
locally. Only meaningful after E1: until the ledger stops disagreeing with the
database, ordering makes a pointless loop more efficient.

**H3 — proof extension.** Without it a destination further behind than
`MaxReceiptListElements` (4,096) cannot be covered at all, and the gap in the
last run was 8,556. E1 stops the livelock; H3 is what lets a deep gap actually
close. A message type, a request path, assembly at the destination.

**H1 — the healing cache.** Last, and small. Keyed by source, destination and
sequence number, in Accumulate, used only by healing. It turns 53,011 fetches
into 8,556 — worth having, but it optimises a loop that E1 and H2 have already
stopped.

## Phase 3 — correctness debt

Not urgent, not optional.

**E5 — the re-evaluation loop.** `stageRuns` is documented as callable more
than once per block and `drainRevealed` runs it up to eight times. The spec says
three groups in sequence, each evaluated once. Establish whether either stated
reason still bites once the run is computed from arrivals *and* the staged set
with anchors executing first; if neither does, the loop and `maxDrainRounds` go.
A correctness question before a performance one: that bound exists to stop a
round that always reports progress from hanging a block, which is a guard
against a condition the design says cannot arise.

**E4 — anchor authorization into staging.** Signatures route to staging,
staging packs them with the one anchor and evaluates quorum or proof, and the
anchor executes once with no further checking. Removes N−1 executions per anchor
whose only product is a signature, and lets the payload deduplicate. O(validators)
per anchor: small at four, linear as that grows.

**D1 — record placement.** `route.go` is a second model of the record model,
maintained by hand, wrong twice and both times caught by a soak. Either derive
placement from the record model or make divergence detectable without a soak.

---

## Order of work

```
E3 ─▶ E1 ─▶ E2 ─▶ H2 ─▶ H3 ─▶ H1        the critical path
D2, E6, D3                               parallel, any time
E5, E4, D1                               after
```

An entry leaves [DIFFERENCES.md](DIFFERENCES.md) when the code matches the
spec, not when its issue is filed or closed.
