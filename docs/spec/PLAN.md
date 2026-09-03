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

**With E1 closed, the next run died on E7.** Soak `20260903T121819Z` — the
first at 1 s blocks on the staging change — never livelocked. It ran out of
memory: the block ledger's log page grows with height and is rewritten every
block, every node reached GOMEMLIMIT in ten minutes, GC took six cores, and the
partitions stalled at 0.26 h. The batch-store eviction storms and deferred votes
in that run are downstream of the executor lagging. Nothing about a 12-hour run
can be claimed until E7 is closed.

---

## Where we are

| | |
|---|---|
| **Spec** | Executor, database and healing written. Healing was rewritten after the first pass: gaps come from staging, requests are generated in staging, two senders are chosen by the previous block's hash, and healing activates on a cadence. |
| **E1 + H4 + E2 + E3** | **Done**, on `issue-4189-staging-out-of-account`. Removed from the differences: the code matches the spec. |
| **H5** | **Done**, same branch. Cadence and sender selection replace the jitter, back-off and breakers. |
| **H2** | **Done** — nothing to write. Oldest-first entries and "skip what is staged" both arrived with the staging change. |
| **D3** | **Implemented, not merged.** `TestDeep` on branch `issue-4196-kvtest-deep` (`f3339116e`). |
| **E7** | **Done**, on `issue-4202-block-ledger-chain`: a `block-ledger` chain and one keyed record per block; `indexing.Log` and the #4147 walk deleted; the invariant-9 cost test is green. Removed from the differences. Not yet soaked. |
| **D5** | **Done**, on `issue-4203-bcdb-durable-commit` (#4203): every commit is written through and sealed at commit; readers are isolated by per-version pre-image overlays that are dropped as they close; the oldest view's opener is named in a warning; the event service loads one block at a time with a bounded queue and opens its view only while loading. Removed from the differences. Not yet soaked. |
| **Steady state** | S0, S1, S2 done; S8a delivered (BlockchainDB `dcce242`). **Run #2 (`20260903T202621Z`) held 500.0 tps for twelve minutes with block production 0.38–0.46 s and every resource flat** — the first time on bcdb — until chaos restarted one validator: it could not rejoin (#4205, the restart form of #4162's laggard trap) and BVN2 stalled with the S3 storm. **Run #3 (`20260903T213153Z`) runs with chaos off** to answer the 12-hour resource question on its own; chaos returns when #4205 is closed. Reviews in the run directories. |
| **Everything else** | Not started. |

**What E3's answer turned out to be, because it changed the shape of E1.** The
first answer was that staging is rebuilt on restart, not restored. That is
wrong: staging decides what a block executes, so a node holding less than its
peers executes a shorter run and produces a different block hash. The old design
was right that the held set must be AGREED and wrong that it therefore had to be
HASHED — which is what put it in an account, and what made it need a bound.
Durable and unhashed keeps the agreement and drops the bound.

**Not yet validated by a soak.** The livelock's mechanism is removed and the
tests that pin it pass, but the claim that soaks stop dying is a claim about a
12-hour run, not a test suite.

## Phase 2 — the livelock

In order. Each depends on the one before.

**#4189 — E1 + H4 + E2: move staging out of the account model, and have healing
ask it. DONE.** One change, not three.

Staging becomes durable, unhashed records on the stream's ledger account, fed
only by consensus. `Pending` and `Received` leave `PartitionSyntheticLedger`;
`Delivered` stays, because what a block delivered is its output, and becomes the
only thing the executor reads from it. `MaxPendingSequenced` and the refusal at
`stream_position.go:174` go with them: everything received is held until it can
be processed, and a message that reaches staging is recorded.

H4 is inseparable. `missingRuns` finds gaps by walking `Pending` for `nil`
entries, so the moment `Pending` leaves the ledger it is empty and healing goes
from re-fetching what the node holds to re-fetching *everything*. A gap becomes
a number in `(Delivered, Produced]` that staging does not hold — which means
staging must be askable.

**#4201 — H5: compute requests from staging, on a cadence. DONE.** Requests are
computed at a block boundary on an activation block, and pulled by two
validators chosen from the previous block's hash. The `Conductor`'s per-node
jitter, back-off windows, failure breaker and per-gap scheduling are deleted:
with an activation every few blocks there is no rate to manage, and a lost
request costs nothing because the gap is still a gap next time.

The pair applies to **pulls only**. A request is fungible — whoever asks, the
answer returns through consensus and heals everyone. A signature is not, so the
anchor push runs on the cadence for every validator; selecting a pair there
withholds the rest of the quorum.

**#4190 — E2: `isReady` stops reading block state. DONE**, with #4189. It reads
a position built from the ledger's `Delivered` and staging's own records, so
nothing the block writes decides what the executor believes it holds.

**#4191 — H2: order healing. DONE**, and it turned out to be nothing to write.
"Skip what is already staged" arrived with H4, since a staged number is not a
gap. The order arrived with it too: entries are fetched oldest-first, which is
what advances delivery.

What the entry had wrong was the direction. "Newest to lowest" conflated the two
halves of closing a gap — a receipt only needs the HASHES, so the proof is a
separate fetch with no order at all, and the entries go oldest-first because
delivery is in order. Fetching entries newest-first under a bounded budget would
spend it on messages that unblock nothing.

**#4192 — H3: proof extension. DESIGNED, deliberately not built.** The reason
this was urgent turned out to be false: a collection proof spans from the
requested range to the block boundary covering it, not to the chain head, so its
length does not grow with how far behind a destination is. The soak's own
evidence says so — 44,206 heals, errors 0. The real trigger is a single block
producing more than ~4,096 synthetics to one destination, which is a throughput
condition and has never been observed. Build it when a run shows a proof-length
rejection.

**#4193 — H1: the healing cache.** Now the next item, and small. Keyed by source, destination
and sequence number, in Accumulate, used only by healing. It turns 53,011
fetches into 8,556 — worth having, but it optimises a loop E1 and H2 have
already stopped.

**#4202 — E7: the block ledger as a chain. DONE, on its branch; not yet soaked.** A `block-ledger` chain on the
system ledger account and one keyed record per block, written once; reads go to
the keyed record and fall through to the pre-activation account; no migration,
no walk of history. Deletes `indexing.Log`. Supersedes the mechanism in #4147:
mainnet's Jiuquan activation becomes this form, and the in-band walk of
35 million blocks does not happen. Also close the two things the same run
exposed alongside it, because the next run must be able to tell them apart:
export bcdb's staged-commit depth and the age of the oldest open view (18
commits were staged on every BVN database at the end), and rate-limit the
batch-store over-limit warnings (25,000 a minute per node).

## Steady state — RAM and CPU under load

The soaks now fail on resources, not on livelocks: memory climbs to the limit,
GC takes the CPU, blocks slow, and the stall follows. This section is the plan
to reach a node whose memory and CPU are flat for as long as it runs, at the
target rate, and to be able to *show* that they are. It is ordered by what the
evidence says is largest, and every item traces to a spec statement or names the
spec it needs first — nothing here is tuning.

### What steady state means

Measured over a **12-hour run at 500 tps, 1 s blocks** (a shorter run proves
nothing about growth). Warm-up is the first hour; the numbers are taken from
hour 1 to hour 12.

| property | criterion | spec |
|---|---|---|
| Memory is flat | RSS grows less than 1% per hour after warm-up; live heap stays under 70% of `GOMEMLIMIT`; `GOMEMLIMIT` is under 85% of the container limit | executor inv. 9; database inv. 5 |
| GC is not the workload | GC cycles per second flat, not rising with height; GC CPU share under 10% | — (runtime) |
| CPU is flat | seconds per block at the block interval, and process CPU per committed transaction the same at hour 12 as at hour 1 | executor inv. 9 |
| Block work is bounded | bytes allocated per block does not correlate with height; no allocation site's share grows across the run | executor inv. 9 |
| Commits are durable | `stagedCommits` ≤ 1 at every snapshot; oldest open view younger than one block | database inv. 5 |
| Reads are bounded | read-probe p99 flat; `deepFallbacks` zero; no shallow read walks history | database, windows |
| Caches are bounded | every cache is bounded in **bytes** and reports its size | database — *gap, see S6* |
| Consensus memory is bounded | batch stores and queues hold at most their budget, per node, whatever the executor is doing | consensus — *gap, see S4* |
| Logging is bounded | no message exceeds a fixed rate per node; log volume flat | — |

The heap profile at hour 12 must have the same top ten as the profile at hour 1.

### The work, in order

**S0 — be able to measure it.** Nothing above can be judged from today's
harness: memory is sampled every five minutes, `stats.json` is rewritten every
50 commits so its history is lost, the dagbft block-time and round histograms
emit nothing on any node, and the manifest prints the script's default memory
budget rather than what compose ran. Sample RSS, heap, GC cycles and GC CPU
fraction per node every 10–30 s into soakmon's history; snapshot `stats.json`
on the same cadence; make the histograms emit; export `stagedCommits` and
oldest-view age as metrics; record the effective `GOMEMLIMIT` and `mem_limit`.
Take heap and CPU profiles at fixed hours, not only at the wedge. *Parallel
with S1; done before the acceptance run.* **DONE on `issue-4204-soak-instrumentation`
(#4204):** `mem.csv` and `storage-stats.csv` per run, hourly captures with a
30 s CPU profile, the effective memory budget in the manifest, the two
histograms observed, GC cycles and GC CPU exported, and
`accumulate_bcdb_staged_commits` / `oldest_view_age_seconds` gauges.

**S1 — #4202, E7: the block ledger as a chain. DONE on its branch.** The dominant term: 41–46% of the live
heap and the largest allocation site in our code, growing with height. Chain on
the ledger, one keyed record per block, `indexing.Log` deleted, no migration.
*Done when*: `indexing.(*Block).MarshalBinary` is absent from the profile and
bytes allocated per block are flat across the run. Spec: executor, "The block
ledger", invariant 9. Mainnet's history is the TBD in the spec, not this item.

**S2 — #4203, D5: commits reach the store at commit. DONE on its branch.** The retention term: with E7
fixed the pages are small, but a reader that pins a version still holds every
commit since it, and a crash loses them. Restore durability at commit (write
through unconditionally; isolate open views by overlay or by versioned reads),
export the depth and the oldest view's age, tag views with their opener, add
the `kvtest` case. Then find and fix whatever held eighteen blocks on every
BVN. Prime suspect: the API event service, which opens a batch on every commit
and loads the whole block ledger in an unbounded goroutine — bound it to one in
flight and skip the load when nothing is subscribed. *Done when*:
`stagedCommits` ≤ 1 at every snapshot for twelve hours, and no view outlives a
block. Spec: database invariants 2 and 5.

**S3 — write the consensus memory section of the spec, then hold it. SPEC
WRITTEN 2026-09-03 (`consensus.md`, partial part; differences C1–C3 = #4206, #4207, #4208). Run #3
(`20260903T213153Z`, no chaos) reproduced it alone: batches of one or two
transactions from a 100 ms seal timeout, a count-bounded store holding six
seconds of them, evictions of what the next header needed, and the storm at
minute 16 with no fault anywhere else.** There is
no consensus part of the spec, so the batch store has no specified budget and
behaves as it happens to: 32 MB active, 32 MB retained, 32 MB inbound queue
*per partition*, and own uncommitted batches exempt from eviction — so when the
executor lags, the store exceeds its budget without bound, evicts peers'
batches, defers votes, refetches, and warns 25,000 times a minute. Specify:
one byte budget per node; what a full store does (back-pressure the submitter,
never evict what a vote needs, never exceed the budget for own batches); and
that the wire buffer a batch aliases counts against it. Then make the code
match and rate-limit the warnings. *Done when*: `pubsub/pb.(*Message).Unmarshal`
in-use is within the budget at every profile, and the over-limit warning is
gone from a healthy run. Spec: **to be written** (SPEC.md lists consensus as a
missing part).

**S4 — allocation churn that sets the GC cadence.** None of these retain
memory; together they are why the collector runs eight times a second once the
heap is at the limit. From the hour-16 profile of `acc-bvn2-val1` (197 GB
allocated):

| site | allocated | fix |
|---|---|---|
| `encoding.Hash` of `SyntheticMessage` | 17 GB | hashing re-marshals the message with its receipts every time; cache the hash on the message |
| `api/v3/message.Handler` serving synthetic pulls | 18 GB | H1, the healing cache, cuts the pulls; the sequencer's per-request receipt walk is the rest |
| `merkle.(*State).Copy` via `CopyAsInterface` | 9 GB | every `Get` of a chain state deep-copies it; give readers a read-only view |
| `Batch.UpdateBPT` | 22 GB cumulative | measure per block; it should be O(accounts touched) |

*Done when*: bytes allocated per committed transaction halve, and GC cycles per
second under 500 tps are flat and low. Spec: healing, "The cache"; executor,
"The database write".

**S5 — bound every cache in bytes.** The database spec says nothing about
caches; the sub-1 GB work found that caches, not storage layout, set the
footprint, and that ~350 MB of the heap is GC headroom the limit authorizes.
bcdb's two immutable caches are bounded by entry count (200,000 per generation,
two generations, two caches), not bytes. Add to database.md: a cache is bounded
in bytes, reports hits, misses and bytes, and holds only records that cannot
change. Then make the caches match. *Done when*: cache bytes are a metric and
the sum of them is a chosen fraction of the memory budget. Spec: **database.md
addition**.

**S6 — GC and memory limits as configuration, not folklore.** `GOMEMLIMIT` must
sit below the container limit with headroom (a limit above it is an OOM kill
waiting for a spike); the manifest must record the effective values; the
runtime's GC CPU fraction must be exported so the "GC is not the workload"
criterion is measurable. Small, and part of S0's harness work. *The measurement
half is done with S0; the limit policy is a compose change still to make.*

**S7 — logging is a resource.** Rate-limit every warning that can fire per
message or per submit (the two batch-store warnings first), and add log lines
per minute per node to the acceptance table. A node at 50,000 lines a minute is
spending CPU on the symptom.

**S8 — the store's own growth (BlockchainDB, cross-repo).** The dynamic layer's
live-tail rewrite (BlockchainDB#60) and maintenance pauses scale with the
dynamic layer's size, which E7 and S5 shrink but do not remove. Filed there,
not edited here. Any store-side term that survives S1–S5 in the hour-12 profile
becomes an issue in that repository.

**S8a — the seal is the throughput wall (BlockchainDB#84).** Acceptance run #1
(`20260903T173742Z`) had flat memory and idle CPU and delivered ~80 user tps of
500 offered. Block production averaged 0.88 s of every 1 s block with 65 ms of
execution in it; every node's block producer was sampled inside `fsync` in the
store's per-block seal, which issues about forty serialized fsyncs across the
eight shards (17 ms each on this host) while holding each shard's write lock, so
the read that validates a submission waits the seal out. Sixteen submitters at
~190 ms each is ~84 tps. The pre-sharding store reached 497 tps on the same
soak. Fix is the store's: seal shards concurrently, one fsync per layer per
block, no read lock across fsync. **Delivered 2026-09-03 as BlockchainDB PR #85
(`dcce242`, "Seal off the lock, and seal the shards together")**: the cut under
the lock with no barrier, fsyncs with the lock released, layers side by side,
shards concurrent; their measurement a block boundary 253 → 50 ms mean and the
worst Get during a seal 56 → 1.8 ms. Pulled into accumulate on the #4203
branch; acceptance run #2 runs on it.

### Order

```
S0 (measure) ─┬─▶ S1 (E7) ─▶ S2 (D5 + view holders) ─▶ acceptance run #1
              └─▶ S6, S7                         parallel, any time
S3 (consensus spec, then batch store) ─▶ S4 (churn) ─▶ S5 (caches) ─▶ acceptance run #2
S8 as issues, whenever a store term shows in a profile
```

**Revised after run #1 (2026-09-03).** Run #1 showed the order above is wrong
about what comes before the second run. With the database terms gone, the heap
that grew was the batch store and gossip buffers, because the store's seal
holds every block to ~0.9 s and nothing tells the submitter to slow down:

```
S8a (BlockchainDB#84, store) ─┐
S3  (consensus memory spec + back-pressure) ─┴─▶ acceptance run #2
S7  (logging) alongside; S2 follow-up: captureProvableView holds a view for
    minutes by design — capture it only when a snapshot is about to be pinned
#4205 (restart recovery) before chaos returns to an acceptance run
```

Acceptance run #1 answers whether memory is flat with E7 and D5 closed. It will
not yet meet the GC or churn criteria; that is what run #2 is for. Neither run
is a claim until it has lasted twelve hours.

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
E1+H4+E2 ─▶ H5 ─▶ H2 ─▶ E7 ─▶ D5 ─▶ H3 ─▶ H1   the critical path (through H2 DONE; E7 next)
S0..S8                                       steady state — see its own order above
E6, D2, D3 ─▶ D4                          parallel, any time
E5, E4, D1                                after
```

An entry leaves [DIFFERENCES.md](DIFFERENCES.md) when the code matches the
spec, not when its issue is filed or closed.
