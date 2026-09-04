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

```
E8 #4217 ─▶ H8 #4216 (with H1 #4193, H6 #4212) ─▶ C6 #4215 ─▶ #4214 check ─▶ acceptance run #7
S4 #4211, S5, S2 follow-up, S7, BlockchainDB#86      cost, after run #7 shows the healer gone
E5 #4197, E4 #4198, E6, D1 #4199, D2, D3 ─▶ D4       correctness debt, parallel or after
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
2. **Anchor staging.** A store of proofs keyed by (source, anchor block).
   Intake writes every arriving proof there; executing a Directory anchor
   validates or discards every proof waiting on its number; a proof whose
   anchor already executed is validated at intake. Counters: proofs validated,
   disproved, duplicate-for-same-indexes. Test: a proof arriving one block
   before its anchor is validated when the anchor executes; a disproved proof
   is gone and counted.
3. **Proven ranges by index.** Per stream, the union of validated proofs'
   index ranges, above `Delivered`, durable and unhashed, in the snapshot.
   The BPT-hashed replica is deleted. Test: two overlapping proofs give one
   range; release at commit drops ranges at or below `Delivered`.
4. **Collection.** Synthetic staging holds every arriving entry by index,
   proven or not; an entry more than the sanity horizon ahead is refused.
   `buildRun` executes an entry only when proven and next. `SyntheticMessage`
   never returns `Pending`: the pending-outside-staging path is deleted. Test:
   a package arriving before its anchor is held, executes the block after the
   anchor lands, and no status is recorded in between.
5. **Intake as group 0.** `classify` writes entries and proofs into both
   stores before the anchor group is evaluated, for packages and bundles
   alike. Test: an envelope's entries complete a run that drains in the same
   block.
6. **Snapshot.** Both stores and the proven ranges are collected and restored.
   Test: a node restored from a snapshot holds what the source held and
   executes the same run.
7. **Gaps.** Staging answers "proven and missing" and "held or expected and
   unproven" by index, and "anchors missing below the newest held". The
   reconcile-by-`Produced` path is deleted. Test: a lost package produces the
   first kind, a lost proof the second, and neither is reported before staging
   has finished the block.

Done when: the e2e suite delivers packages with anchors arriving in either
order without healing; `exec_synthetic_anchor_total{applied="missing"}` no
longer exists because nothing is judged that way; a 30-minute soak at 500 tps
shows heals only for injected drops.

### H8 #4216 — healing by hash set, from the producer cache

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
on healing. Closes H1 #4193 and H6 #4212 with it.

### C6 #4215 — consensus does not outrun execution

Spec: consensus.md invariants 9 and 10, "Execution lag".

1. The bridge reports the executed leader round to the node after each commit.
2. The header builder takes no batches while the lag exceeds `MaxExecutionLag`
   (8); the worker refuses with reason `execution-lagging`.
3. `batch_store_refusing{reason}` and separate transition log lines; a lag gauge
   and commit queue depth gauge.

Test: an executor that executes one block in three keeps the DAG within the
bound, the own store within its share, and the reason says lag. Done when a
soak with an artificially slow executor never exceeds the bound.

### #4214 — the dispatch leg, verified

After E8 the "missing anchor" leg is gone by construction. Verify the rest:
dispatch counters (packages built, dispatched, refused) against the
destination's proven-missing gaps over a 30-minute soak. Any remaining loss is
a new difference to record before run #7.

### Acceptance run #7

Twelve hours at 500 tps, 1 s blocks, chaos off, then the same with chaos once
#4205 is closed. Judged by the criteria below; nothing shorter is a claim.

## Cost, after the healer is gone

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
- **D2, D3 ─▶ D4** — isolation verified for every backend; the window part of
  the backend contract; absence reported.
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

