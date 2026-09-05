# Development plan

The plan to bring the code to the specification. Ordered from
[DIFFERENCES.md](DIFFERENCES.md) by dependency and by what stops the network,
not by size. Every item names the spec section it implements, what changes,
the tests that come first, and what "done" means. An entry leaves DIFFERENCES
when the code matches the spec, not when its issue is filed or closed.

## Status

E1, E2, E7, D5, S0–S3, C1–C5, H2, H4, H5, H7 and S8a are done. Memory and CPU
hold flat under load. Throughput does not: dispatched packages that arrive
before their anchor are parked outside staging (E8), the healer fills the holes
one message at a time with no cache (H8, H1), the executor spends itself on
that, consensus runs ahead of it without bound (C6), and refusal holds user
throughput near zero. The chain of work below follows that chain of causes.

## Order

The first criterion of every no-drop run is **heals == 0**. The old conductor
healer is gone; a lost entry now shows as a stalled stream, and that gap is
what the next item hunts.

```
E8 #4217 (done) ─▶ H1 #4193 (DONE) ─▶ E10 (DONE: staging is memory, `execute.Staging`, a block transaction that commits with the block) ─▶ C6 #4215 (DONE: execution lag bounded at 8 blocks; empty headers and refusal by reason past it) ─▶ H8 #4216 ─▶ acceptance run #7
R #4219 ─▶ S4 #4211, S5, S2 follow-up, S7, BlockchainDB#86   cost: first the reads that prove an absence, then the rest
E5 #4197, E4 #4198, E6, D1 #4199, D2, D3               correctness debt, parallel or after
H3 #4192                                              when measurement says proofs must reach further back
#4205 restart recovery                                before chaos returns to a soak
```

Each item is its own issue branch from the previous item's tip.

## The critical path

### E8 #4217 — staging as two stores

Spec: executor.md "Collection", "Proof", "Anchor staging", "Sort, then four
groups", "Staging in a snapshot"; healing.md "Gaps".

Steps, each test-first:

1. **The anchor's block on the proof. DONE.** `AnnotatedReceipt.Anchor.SourceBlock`
   is the Directory block whose anchor proves the package; both dispatch paths
   fill it (`directoryAnchorMetadata`). Refusing a proof without it lands with
   step 2, when anchor staging reads it. Test: `synth_proof_anchor_test.go`.
2. **Anchor staging. DONE.** Proofs held in memory by source and anchor
   block (`StagingTxn.StageProof`), `DirectoryAnchorBlock` as execution output; intake from `classify`,
   validation after the anchor group; a proof is bound to its source by
   covering a sequenced sibling from that source; bounded by `maxAnchorAhead`
   and `maxStagedProofBlocks`; every Directory anchor re-evaluates every held
   stream so what it proves drains in the same block;
   `staged_proofs_total{staged,validated,disproved,conflict,invalid,unbound,refused}`.
   Not done: refusing a proof that does not name its anchor (deferred to H8,
   whose paths produce such proofs). Test: `anchor_staging_test.go`.
3. **Proven ranges by index. DONE except release.** The proven set is the
   per-stream proven set in memory (`StagingTxn.Prove`), index to hash, outside
   anything hashed or written, and a proof
   that contradicts a proven index is refused as `Conflict` and counted
   (`staged_proofs_total{outcome="conflict"}`). Tests: `proven_set_test.go`.
   **Deferred:** releasing proven indexes at or below the delivered point.
   The mirror is a chain, whose prefix cannot be dropped without rebasing
   its merkle state, and the delivered point is a sequence number while the
   proven set is by main-chain index; the mapping is only known from the
   proofs of executed entries. Done after step 4, when execution has the
   proof in hand and can record the executed index per stream.
4. **Collection. DONE.** An unproven synthetic is collected — stored under its
   hash with its transaction and held at its number (`SyntheticMessage.collect`)
   — never recorded pending outside staging; staging judges a proof-less
   entry by the proven set (`syntheticIsProven`); a held entry executes
   without a signature once proven; a number beyond `maxSequenceAhead` is
   refused; the run builder never takes a collected entry until proven
   (a held entry marked collected carries its hash), and should one be run
   early it stays collected (never a terminal status). Test: `test/e2e/collection_test.go` — a two-deposit
   package kept ahead of its anchor, with the healer's copies dropped, is
   sighted and not delivered, then delivered by the collected entries when
   the anchor lands; `staging_sim_test.go` — staging in isolation: package
   ahead of / after its anchor, toss at or below delivered, hold above the
   last validated within the horizon, disproved proof leaves entries
   waiting, conflicting proof tossed. Counters:
   `synthetic_anchor_total{proven,unproven,collected}`.
5. **Intake as group 0. DONE in effect.** `classify` records every entry as an
   arrival and hands every proof to anchor staging before the anchor group
   is evaluated; an entry becomes a durable held record the moment it cannot
   execute (collection), which is within the same block. The remaining
   difference from the spec's wording — arrivals are not written durably at
   intake when they are going to execute this block — has no observable
   effect and is not worth a write per entry.
6. **Snapshot. DONE.** Anchor staging's records are `state` with key
   enumerators, so collection walks them; the proven set is an account chain.
   Found and fixed on the way: a restored chain had no hash index, so
   `IndexOf` — admissibility and the proven set — failed on every restored
   node; restore now rebuilds every chain's index. Tests:
   `snapshot_anchor_staging_test.go`, `snapshot_chain_index_test.go`.
7. **Gaps.** Moved to H8, whose request set is the consumer: staging answers
   "proven and missing" and "held or expected and unproven" by index, and
   "anchors missing below the newest held"; the reconcile-by-`Produced` path is
   deleted with the conductor's pull paths.

Done when: the e2e suite delivers packages with anchors arriving in either
order without healing (done); a 30-minute soak at 500 tps shows heals only
for injected drops. **Check run `20260904T140000Z`**: the BVN↔BVN leg is
closed (87,908 proven, 20,074 collected, streams with no backlog), but heals
did not fall — the healer pulls every one-block-late hole immediately (H8),
and the Directory's range recovery proves under source roots that no
destination accepts (a range path since deleted), so the Directory spiralled and the run stalled at
17 minutes. **Check #2 (`20260904T163512Z`, with the review fixes)** ran to its
30-minute deadline: 1.0 s blocks on every partition, heap flat at ~500 MiB,
no refusal, no Directory spiral, streams with no backlog — and found one more
defect (a delivered copy ahead of its anchor failed its envelope; 11,248
envelopes; fixed `cc8c06366`). The heals criterion is H8's to meet; E8's code
is complete except release of the proven set.

### H8 #4216 — healing by hash set, from the producer cache (for dropped entries)

Spec: healing.md throughout; database.md "Caches".

1. **Producer cache.** `internal/core/crosschain/cache.go`: entries in play by
   (partition, index) and by hash, filled from `produceSynthetic` and
   `prepareAnchor`, dropped as the destination's `Delivered` is learned,
   `HealCacheEntries` from measurement, misses served from the permanent layer
   and counted with depth. Test: a produced entry is in the cache before the
   block closes; a request for it never touches the chain.
2. **The request.** A third sequencer method: entries by hash set and proofs
   by index spans for a destination, bounded by `MaxRequestHashes` and
   `MaxRequestSpans`. Test: a request for a thousand hashes is one call and
   one answer.
3. **The answer.** The source packs bundles under `synthPackageBudget` and
   submits them to the requesting partition through the dispatcher. A bundle
   is an envelope of entries or a proof, no message type of its own. Test: a
   destination's staging holds every requested entry one block after the
   answer, with nothing recorded for the envelope.
4. **Deciding in staging.** On activation blocks staging computes the request
   set after the block: gaps first seen two activations ago, not asked within
   `healPatience`, anchors at once. The conductor's `requestMissingSynthetics`
   and reconcile are deleted; the pair selection is kept. Test: the same
   request set on every validator; a gap seen once is not asked.
5. **Counters.** Every row of healing.md's counting table on the metrics
   endpoint.

Done when: a soak with 5% of packages dropped shows heals equal to distinct
gaps, one request per gap, zero cache misses, and executors spending under 5%
on healing. Closes H1 #4193 with it.

### C6 #4215 — consensus does not outrun execution

DONE. The node counts committed leader groups against executed blocks; past
`MaxExecutionLag` (8) the primary proposes headers without batches and every
worker refuses user work with reason `execution-lagging`, apart from
`store-full`; both clear when execution catches up. Tests: the worker refuses
user work and passes system traffic while lagging; the primary's header carries
no batches past the bound, the batches wait, and proposal resumes when the
lag falls. Proof outstanding: a soak in which BVN2's lag stays under the bound
and its anchor leg stays near its floor.

### #4214 — resolved: there was no loss

The no-healer run (`20260904T180918Z`) delivered every stream to received ==
delivered with zero heals and nothing dropped. The 5,974 entries the old
healer "delivered" in check #2 were in flight behind a slowing Directory round
trip, pulled by a healer with no patience. What remains is latency, and it is
the cost work below plus one executor term: entries judged unproven because
their proof arrived after a later proof had seeded the stream's proven set
(28,220 on one node in 21 minutes), each taking the envelope-loop-and-hold
path. Fix: a proof for an earlier span writes its element and index records
below the proven set's origin, so it proves what it covers wherever it lands.
The disorder simulator stays useful for H8 and C6 but is no longer the hunt.

### Acceptance run #7

Twelve hours at 500 tps, 1 s blocks, chaos off, then the same with chaos once
#4205 is closed. Judged by the criteria below; nothing shorter is a claim.

## Cost, after the healer is gone

### R #4219 — reads that prove an absence

Spec: database "Duplicates are caught at entry". A duplicate is stopped by
recent, mutable state; a write below that never asks history. At 500 tps a
BVN executor's segment-store reads were ~95% such asks.

- **R0 — the assertions.** DONE: `merkle.OnDuplicate` records every duplicate
  append in test builds and the e2e suite fails on any outside the permitted
  repeats (root and signature chains, and the E9 double append until it has
  one site). The adapter counts shallow misses by shape and the history walks
  it made for them.
- **R1 — mutable shapes never walk.** DONE: a mutable miss is the dynamic
  layer's answer; tests prove a mutable miss makes no walk, a permanent miss
  still does and is counted, a deep reader's miss is neither.
- **R2 — the first-write reads.** Run 20260904T221627Z: 113.8 M history
  walks on the BVN stores, 99.2% proving a key absent before its first write
  (~6,300 a block a node). The version pre-read is gone (D7, DONE: version-only
  fetch, proven by the counting, conflict and differential tests). Next the
  chain's element-index check (D8) and the dead `Transaction.Main` read of a
  v1 shape v2 never writes. Proof: the e2e duplicate assertion, the A/B golden
  run, `fallbackWalks` near zero.
- **R3 — no fallback.** DONE. Dispatch and healing read the cache (H1); the
  one deep reader left — a transaction a signature or remote copy refers to,
  pending for longer than the window — takes a deep reader; the adapter
  reports absence for every other shallow miss and never walks permanent
  history. D4 and D6 closed with it. `historyReads` in `stats.json` now
  attributes the deep reads, and `shallowMisses` by shape shows any reader
  that should have been deep. Proof outstanding: a soak with `historyReads`
  near zero and BVN2's block time flat.

Then, as cost work rather than correctness (each still proven by the suite's
duplicate assertion and an A/B golden run — same envelope stream under two
builds, compared per block on state root, block ledger, every touched
account's chain heights and anchors, and all element-index records):

- DONE: the transaction hash appended to a chain once per transaction,
  settled from the transaction's own chain-update record (E9), `ErrNotFound`
  on the success path an error, `AddChainEntry2` honouring its argument;
  proven per transaction type by `single_append_test.go`;
- the observer's v1 `Transaction.Main` read gated, an account hashed once per
  block, `clearActiveSignatures` touching only signers that signed;
- the element index restored as first occurrence (D9).


- **S4 #4211** — hash a message once; read-only chain state without deep copy.
  Spec: executor invariant 9.
- **S5** — both caches sized in bytes and reporting their size. Spec: database
  "Caches".
- **S2 follow-up** — capture the provable view only when a snapshot is about
  to be pinned. Spec: database invariant 5.
- **S7** — every warning that can fire per message is rate-limited; log
  volume is a metric.
- **BlockchainDB#86** — history lookups indexed rather than bloom-walked.
  Store-side.

## Correctness debt

- **E5 #4197** — one evaluation per stream per block; the re-evaluation loop
  goes.
- **E4 #4198** — anchor signature quorum assembled in staging, not by
  execution.
- **E6** — `CascadeDeliveryQueue` deleted from the hashed state.
- **D1 #4199** — record placement derived from the record model, or divergence
  detectable without a soak.
- **D2, D3** — isolation verified for every backend; the window part of the
  backend contract (absence is now reported, D4).
- **H3 #4192** — proof extension, when measurement shows a destination must
  reach further back than one proof.
- **#4205** — a restarted validator rejoins from retention or a snapshot;
  required before chaos returns.

## Acceptance criteria

Measured over a **12-hour run at 500 tps, 1 s blocks**. Warm-up is the first
hour; the numbers are taken from hour 1 to hour 12.

| property | criterion | spec |
|---|---|---|
| Memory is flat | RSS grows less than 1% per hour after warm-up; live heap stays under 70% of `GOMEMLIMIT`; `GOMEMLIMIT` is under 85% of the container limit | executor inv. 9; database inv. 5 |
| GC is not the workload | GC cycles per second flat, not rising with height; GC CPU share under 10% | — (runtime) |
| CPU is flat | seconds per block at the block interval, and process CPU per committed transaction the same at hour 12 as at hour 1 | executor inv. 9 |
| Block work is bounded | bytes allocated per block does not correlate with height; no allocation site's share grows across the run | executor inv. 9 |
| Commits are durable | `stagedCommits` ≤ 1 at every snapshot; oldest open view younger than one block | database inv. 5 |
| Reads are bounded | read-probe p99 flat; `deepFallbacks` zero; no shallow read walks history | database, windows |
| Caches are bounded | both caches bounded in **bytes** and reporting their size | database "Caches" |
| Consensus memory is bounded | batch stores and queues hold at most their budget, per node, whatever the executor is doing | consensus — *gap, see S4* |
| Logging is bounded | no message exceeds a fixed rate per node; log volume flat | — |

The heap profile at hour 12 must have the same top ten as the profile at hour 1.


### Metrics behind the criteria


| criterion | metric | exists |
|---|---|---|
| memory flat | process RSS, `go_memstats_heap_alloc_bytes`, `GOMEMLIMIT` | yes |
| GC is not the workload | `go_gc_cycles`, `/cpu/classes/gc` | yes |
| CPU flat | `accumulate_dagbft_block_production_seconds`, process CPU | yes |
| block work bounded | allocation profile per hour | capture |
| commits durable | `accumulate_bcdb_staged_commits`, `accumulate_bcdb_oldest_view_age_seconds` | yes |
| consensus memory bounded | `accumulate_dagbft_batch_store_bytes{kind}`, `batch_store_refusing{reason}` | reason label: no |
| consensus within execution | executed-round lag gauge, commit queue depth | no |
| healing small | the healing counting table (healing.md) | partly (`heals_total`, `reconcile_pulls_total`) |
| staging bounded | staging entries and proofs held, proven range size | no |
| logging bounded | log lines per minute per node | harness only |

