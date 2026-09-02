# Development plan

Ordered from [DIFFERENCES.md](DIFFERENCES.md). The order is by dependency and
by what is actually stopping the network, not by size.

## What is stopping us

Soaks die in twenty minutes and have done for five consecutive runs. The
mechanism is known and it is one difference: **E1**. Staging lives in an
account, so the pending array must be bounded, so receipts past the bound are
refused, so the ledger reports not holding messages the node has, so the healer
re-fetches them across the partition — 8,556 sequence numbers fetched 53,011
times in run `20260902T132651Z`, while all three partitions stayed live.

Phase 2 exists to close that. Phase 3 is real work but is not why the network
stops.

---

## Where we are

| | |
|---|---|
| **Spec** | Executor, database and healing written; 15 differences recorded, each with an issue. Healing was rewritten after the first pass: gaps come from staging, requests are generated in staging, two senders are chosen by the previous block's hash, and healing activates on a cadence. |
| **E3** | **Done.** Answered in [executor.md](executor.md), #4188 closed. Staging is not restored, it is rebuilt. No code change of its own — E1 implements it. |
| **D3** | **Implemented, not merged.** `TestDeep` on branch `issue-4196-kvtest-deep` (`f3339116e`). |
| **Everything else** | Not started. |

Nothing on the critical path has been written yet. That is the next thing.

## Phase 2 — the livelock

In order. Each depends on the one before.

**#4189 — E1 + H4: move staging out of the account model, and have healing ask
it.** One change, not two.

Staging becomes executor state fed only by consensus. `Pending` and `Received`
leave `PartitionSyntheticLedger`; `Delivered` stays, because what a block
delivered is its output. `MaxPendingSequenced` and the refusal at
`stream_position.go:174` go with them: everything received is held until it can
be processed, and a message that reaches staging is recorded.

H4 is inseparable. `missingRuns` finds gaps by walking `Pending` for `nil`
entries, so the moment `Pending` leaves the ledger it is empty and healing goes
from re-fetching what the node holds to re-fetching *everything*. A gap becomes
a number in `(Delivered, Produced]` that staging does not hold — which means
staging must be askable.

**#4201 — H5: generate requests in staging.** Follows immediately, because it
needs the same thing H4 does. Requests are computed at the end of the anchor and
synthetic groups on an activation block, deduped per gap, and sent by two
validators chosen from the previous block's hash. The `Conductor`'s per-node
jitter, back-off window, failure breaker and per-gap scheduling are deleted:
with two senders and an activation every few blocks there is no load to manage,
and a lost request costs nothing because the gap is still a gap next time.

**#4190 — E2: `isReady` stops reading block state.** Falls out of E1; listed
separately because it is the invariant being restored, not a side effect.

**#4191 — H2: order healing, newest gap to lowest.** Smaller than it was: "skip
what is already staged" arrives with H4, since a staged number is not a gap.
What is left is the order. Only meaningful after E1 — until the ledger stops
disagreeing with the database, ordering makes a pointless loop more efficient.

**#4192 — H3: proof extension.** Without it a destination further behind than
`MaxReceiptListElements` (4,096) cannot be covered at all, and the gap in the
last run was 8,556. E1 stops the livelock; H3 is what lets a deep gap actually
close. A message type, a request path, assembly at the destination.

**#4193 — H1: the healing cache.** Last, and small. Keyed by source, destination
and sequence number, in Accumulate, used only by healing. It turns 53,011
fetches into 8,556 — worth having, but it optimises a loop E1 and H2 have
already stopped.

## Alongside — cheap and independent

Closeable on their own, in parallel with anything. **Not sequenced ahead of the
critical path.**

| | |
|---|---|
| **#4195 — E6** | `CascadeDeliveryQueue` is dead state still folded into the account hash. Remove the field, the hasher contribution, the snapshot entry, the debug observer. |
| **#4196 — D3** | Done on a branch; merge it. |
| **#4194 — D2** | Badger does not run `TestIsolation` — it runs only Database, SubBatch, Prefix and Delete, across all four versions. Add it and see whether it passes. If it does not, the difference is much larger than the test. |
| **#4200 — D4** | Enforce the bcdb read window: `getAt` falls back to `GetDeep` for a **shallow** reader, so no ordinary read ever reports absence. The fallback count is now zero, which is the evidence its own comment asks for. After #4196 — enforcement without the conformance test swaps a measured fallback for an unverified one. |

## Phase 3 — correctness debt

Not urgent, not optional.

**#4197 — E5: the re-evaluation loop.** `stageRuns` is documented as callable
more than once per block and `drainRevealed` runs it up to eight times. The spec
says three groups in sequence, each evaluated once. Establish whether either
stated reason still bites once the run is computed from arrivals *and* the
staged set with anchors executing first; if neither does, the loop and
`maxDrainRounds` go. A correctness question before a performance one: that bound
exists to stop a round that always reports progress from hanging a block, which
guards a condition the design says cannot arise.

**#4198 — E4: anchor authorization into staging.** Signatures route to staging,
staging packs them with the one anchor and evaluates quorum or proof, and the
anchor executes once with no further checking. Removes N−1 executions per anchor
whose only product is a signature, and lets the payload deduplicate.
O(validators) per anchor: small at four, linear as that grows.

**#4199 — D1: record placement.** `route.go` is a second model of the record
model, maintained by hand, wrong twice and both times caught by a soak. Either
derive placement from the record model or make divergence detectable without a
soak.

---

## Order of work

```
E1+H4 ─▶ H5 ─▶ E2 ─▶ H2 ─▶ H3 ─▶ H1      the critical path
E6, D2, D3 ─▶ D4                          parallel, any time
E5, E4, D1                                after
```

An entry leaves [DIFFERENCES.md](DIFFERENCES.md) when the code matches the
spec, not when its issue is filed or closed.
