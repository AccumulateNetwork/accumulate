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
E8 #4217 (done) ─▶ H1 #4193 (DONE) ─▶ E10 (DONE: staging is memory, `execute.Staging`, a block transaction that commits with the block) ─▶ C6 #4215 (DONE: execution lag bounded at 8 blocks; empty headers and refusal by reason past it) ─▶ H8 #4216 (DONE as a pull by span from staging; push and hash set are DIFFERENCES H8) ─▶ acceptance run #7
E12 one chain per pair, one stage per chain (DIFFERENCES E12)   Paul 2026-09-05; steps 1–4 done 2026-09-05 (chains, proofs, cache per destination; stage as two lists; anchors through the stage, push healer deleted); 5 open; proof form of a pulled anchor is H9
C7 cross-partition back-pressure (DIFFERENCES C7)      the death reproduction's finding; a spec decision first
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

DONE 2026-09-05 as a pull: the conductor's requester
(`internal/core/crosschain/requester.go`) decides from `execute.Staging` on
activation blocks, asks the source's `SequenceRange` by span and submits the
bundle itself; the source's sequencer answers from the cache with the span's
proof, the Directory block it is provable under and each entry's companion.
Items 1, 4 and most of 5 below are done as written; 2 and 3 are done in the
pull form (DIFFERENCES H8). Anchors are not pulled (DIFFERENCES H9). Seven
dropped-entry acceptance tests pass; the two anchor-proof tests are skipped.
Still to show: the "done when" soak below.

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

### E11 #4205 — a node joins from the running protocol, and a restart is a join

Spec: executor.md "Sync". Decided by Paul 2026-09-18: a starting node does not
catch up through consensus. It listens and collects into staging, takes
staging from a running validator as of that validator's last committed block,
pulls the state — the Directory's spine, then the BPT by pages and the
accounts the buffered blocks name — verified against the anchored root, and
executes from the first block after the root matches. It serves nothing it
cannot answer until its history is backfilled. The bootstrap-v3 work
(`origin/bootstrap-v3`, `origin/bootstrap-v3-merge-1.4.4`; epic #3985:
`internal/core/bootstrap/{pull,enumerate,tracker,bptproof,nodestate,…}`,
`BptPageQuery`) is the state half, built on the CometBFT line and never on
this one; the staging half is new.

1. **Staging as an API.** A validator serves its staging as of its last
   committed block, per partition: every stream's `Delivered`, sighted mark,
   held entries with companions and collected flags, validated hashes,
   proofs waiting by anchor block, anchor copies held. Served atomically at a
   block boundary. Test: a snapshot taken between two blocks equals what a
   fresh staging fed the same blocks holds.
2. **Collect without executing.** The executor applies a committed block to
   staging only — classify, intake proofs, hold entries and anchor copies —
   and a joining node runs every buffered block through it from `P + 1`.
   Then, at `Q`, releases through each stream's `Delivered` from the pulled
   ledgers and decides proofs against the anchors executed by `Q`. Test: a
   node that collected blocks `P + 1 .. Q` on top of a peer's staging at `P`
   holds exactly what the peer holds at `Q`.
3. **State pull on this line.** Port `pull`, `enumerate`, `tracker`,
   `bptproof` and `BptPageQuery` from bootstrap-v3; the spine first; the
   accounts named by buffered blocks next; verified against the Directory's
   `StateTreeAnchor` for the block. A restart pulls only what changed after
   its last block. Test: a corrupted account is refused and re-pulled from
   another peer; a node with state at `R` reaches the root at `Q` pulling
   only accounts touched in `(R, Q]`.

   *Landed (#4293) with three gaps to close here.* The pages are read and
   nothing of a peer's goes into the local BPT — the stale set is the diff
   between the peer's pages and the node's own leaves — and a pulled account
   replaces what the node held for it and carries its chains' open mark sets,
   so it can be executed from. What is *not* done: the accounts are named by
   re-diffing the peer's whole tree each round rather than by the blocks
   (#4292's `CollectBlock` output is the block-named path, not wired); the
   anchored roots are read from one API with no `BlockAnchor` signature or
   quorum checked, so with one source that source supplies both the root and
   the state hashing into it; and accounts are pulled as if independent,
   though a key page's hash covers its book's pending. DIFFERENCES E11 has
   the list.
4. **Handoff.** At the root match the executor starts at `Q + 1` from the
   buffer; the consensus checkpoint restores only the DAG position; catch-up
   replay of unexecuted blocks is removed, as is the interim rejoin pull
   (`Conductor.Rejoin`) once this is proven. Test: the simulator's
   `RestartNode` takes this path and `TestOneValidatorRestartDoesNotDiverge`
   holds; a Docker chaos run keeps every restarted validator agreeing on the
   anchor body.
5. **Serve last.** Node state `BOOTING → ACTIVE → COMPLETE`, advertised; the
   sequencer and the historical API refuse until `COMPLETE`; the cache fills
   by backfill. Test: a request for missing data routed to a `BOOTING` node
   is refused and answered by a `COMPLETE` one.

Done when: a soak with chaos restarts under load keeps every restarted
validator agreeing on every anchor body, and #4205 closes. Required before
chaos returns to the acceptance run.

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
- **#4205 (E11)** — a restarted validator rejoins by syncing from the running
  protocol (plan above); required before chaos returns.

### E12 — one chain per pair, one stage per chain

Spec: executor.md "One chain per pair, one stage per chain"; healing.md
"Deciding, in staging"; database.md "Chains are logs".

1. **Chains.** DONE 2026-09-05. The synthetic ledger gets a chain per
   destination (`SyntheticChain(partition)`), each anchored into the root
   chain when it changes; `buildSynthTxn` appends to the destination's chain;
   the sequence (index) chains go, because the chain index is the sequence
   number. Test: `test/e2e` `TestSyntheticChainPerDestination` — a block
   sending to two destinations appends to two chains, and each chain's
   entries are that destination's entries in order.
2. **Proofs and the cache.** DONE 2026-09-05. The producer cache keeps a
   segment per destination chain per block; the package proof and the
   sequencer's answer cover one chain. Test (same test): a proof's element
   list equals the destination's entries for the span, and the proof's index
   is the sequence number. Lesson from building it: the cache seed at start
   must read existence from a chain's head — `Chain2.Get` registers the chain
   on the account, and that write in a block's batch moved the state-tree
   root between the anchor the conductor sent and the one the executor
   recorded, so the anchor healer's re-send of it later failed a block.
3. **The stage as two lists.** DONE 2026-09-05. One implementation: entries
   and validated hashes indexed from `Delivered + 1`, the two walks, the run
   where they agree, the two gap spans; hash-keyed maps removed; a collected
   entry a proof contradicts is dropped; numbers beyond `MaxStageSpan` above
   `Delivered` are ignored so a forged number cannot size the lists. Tests:
   `execute` `TestStaging_ProveConflictAndExtension`,
   `TestStaging_DisprovedEntryIsDropped`, `TestStaging_Bounds`; `crosschain`
   `TestDecide_TwoKindsOfGap`, `TestDecide_ValidatedBeyondHeld` (a hole, entries
   beyond the proof, a proof beyond the entries).
4. **Anchors through the stage.** DONE 2026-09-05. An anchor below its quorum
   is held at its number (collected), runs when the signatures reach the
   threshold or a validated hash at its number is its own, is tossed at or
   below `Delivered`; the requester asks for anchor gaps from the source's
   cache and each answer carries the answering validator's signature; the
   source-side re-send (`healAnchors`, `deliveryStalled`,
   `EnableAnchorHealing`) is deleted. Tests: `TestAnchorRangeRecovery`
   un-skipped on the re-attestation form; `TestAnchorThreshold`,
   `TestAnchorPlaceholder`, `TestReuseDirectoryAnchorSignatures` assert "not
   executed" instead of "recorded pending". `TestAnchorQuorumStuckRecovery`
   stays skipped: it needs the proof form (H9, narrowed).
5. **Reproductions.** The store-backed receipt test
   (`TestDirectoryReceiptsPastTheWindow`) and the lagging-destination test
   (`TestRequester_LaggingDestination`) pass on the new layout (2026-09-05);
   the Docker comparison, acceptance run #7 ninth start
   (`runs/20260905T225751Z`), STALLED at minute 5 of load: copies of the
   Directory's anchors were lost between dispatch and the BVN executors, fewer
   than the six-signature threshold arrived, and nothing re-sent them once the
   source-side re-send was deleted in step 4 — the pull's answer carried one
   signature from the same node every time. Found by three short Docker
   diagnostics (`233002Z`, `234434Z`, `235425Z`), not in-process: the
   simulator lands every validator's copy in one block. Fix committed
   (723637686): the Directory's answer carries every signature it holds on
   its own copy. Those five runs also ran on leveldb by omission (the
   harness now takes the backend from `docker-network.yml`), so their memory
   numbers say nothing about the network; the acceptance run on BlockchainDB
   is next.

Fresh installs; no migration of the interleaved chain.

## Memory and execution review (2026-09-06)

[docs/reviews/memory-execution-paths-2026-09-06.md](../reviews/memory-execution-paths-2026-09-06.md)
— what grows and with what, across the executor, staging and the cache, the
API and store, and consensus. Top of the list: the producer cache keeps every
synthetic with its transaction for an hour (2–5 GB per node at 500 tps); the
dispatcher drops the rest of a send cycle on the first transport error (issue
4222's mechanism); a sequencer read view held across blocks makes every
commit take pre-images. Issues: umbrella #4223 with the whole review; per
finding #4224–#4233; the dispatcher is #4222. Done the same day: the cache is
released by the destination's Delivered carried on every dispatch (ccb80592e);
staging release returns a drained backlog (4ff104e6f). The second pass added
#4234–#4246. Commits for all of #4222, #4224 and #4226–#4246 landed on
`issue-4193-producer-cache` (2026-09-06, one `Issue #N:` commit each; #4225
was moot), but a code verification the same evening found only **seven
complete**: #4224, #4226, #4229, #4234, #4240, #4242, #4246. The other
fourteen are PARTIAL, and the remainder of each is recorded on its issue —
some knowingly deferred (E11, H1, BlockchainDB), some silently incomplete
(#4230's worker bound, #4232's three residues, #4235's unbounded map,
#4239's channel wedge, #4244's `set.Add`, #4245's submit-path Join).
"Worked" is not "done": the claim above was made from the implementing
agents' reports, and verifying against the code corrected it. What each fix does not do is in DIFFERENCES: a restarted validator
resumes its round but pulls nothing it missed (E11); the stage's span bound is
constant because no wire path tells a destination the source's produced count
(H1); a stranded stream leaves that state only by sync. Acceptance is the
twelve-hour soak on BlockchainDB at 500 tps, not the tests.

## E13. Durability moves to the committed log; the seal lags

Decided 2026-09-06 (Paul: "If the DAG has a log, use that"; #4259). Spec:
database.md invariant 5 and "The seal lags the commit"; consensus.md "The
committed log", "Restart", "Retention". Steps, each with its reproduction
first:

1. **Store question answered** (BlockchainDB#88): `SealBlock(h)` called for
   h = last sealed + k with writes already in the next tails — confirm the
   height tags and the dyna window age correctly, or add a seal-range call.
   Test in BlockchainDB.
2. **The log** (`internal/node/dagbft`): append the committed group — its
   certificates and batches — to `consensus/<partition>/log` as one record
   per block, one `fsync`, before `processCommittedGroup` executes it. Test:
   the record round-trips and the file is synced before execution begins.
3. **The seal off the block path** (`pkg/database/keyvalue/bcdb`):
   `writeThrough` puts and returns; a sealer goroutine calls `SealBlock` for
   the newest version at or below (committed − MergeLag), records the seal
   watermark, and exports the gap as a gauge. Refusal above a bound (the
   `execution-lagging` machinery, with a `seal-lagging` reason). Test with a
   store whose fsync takes a configurable time: block production continues,
   the gap grows, refusal engages at the bound.
4. **Replay on restart**: from the store's sealed height, re-execute the log
   to its head before seeding consensus (#4238). Test: kill between commit
   and seal; the restarted node's state hash matches its peers'.
5. **Log deletion** at min(seal watermark, retention floor). Test: the log
   never holds a block below the watermark and never drops one above it.
6. Soak on BlockchainDB with an induced fsync stall (chaos hook): no stall
   in block production.

## Throughput and latency review (2026-09-06)

[docs/reviews/throughput-latency-2026-09-06.md](../reviews/throughput-latency-2026-09-06.md)
— why 500 tps does not hold on the acceptance soak and why latency is high.
Consensus makes a block a second; a loaded block executes in ~1.4 s because
the executor waits on ~31 k store lookups per block (dyna-history walks with
blooms freed), runs serially, executes eleven messages and writes 73 records
per user transaction; at eight blocks behind a validator refuses its users
with no hysteresis and stays refusing 52–58% of the time. Cross-partition
latency is two Directory round trips plus lag. Memory is bounded. Umbrella
#4258; P1 #4249/BlockchainDB#88, P2 #4250, P3 #4236/#4251, P4 #4252, P5 #4149,
P6 #4253 (spec decision for Paul), P7 #4254, P8 #4255, P9 #4256, P10 #4257.

## Simulation first

Every failure a soak finds gets an in-process reproduction before its fix, and
the fix is tested there, with every earlier reproduction, in seconds — not in
the next thirty-minute run (Paul, 2026-09-05: "replicate in simulation these
crashes, so we can debug and fix the crashes and test that our fix doesn't
break the past fixes"). The soak is the twelve-hour confirmation, not the
discovery loop. Reproductions so far:

| failure | reproduction | time |
|---|---|---|
| Directory anchors rejected once a mark point is behind the store's window; every stream frozen (runs 032333Z–051008Z) | `test/e2e` `TestDirectoryReceiptsPastTheWindow` on the BlockchainDB-backed store (`simulator.BcdbDbOpener`); `pkg/database/keyvalue/bcdb` `TestReceiptReachesPastTheWindow` | 2 min; 4 s |
| the backlog after a refusal window comes back as one block, lag oscillates on the bound (run 144928Z) | `pkg/consensus/consim` `TestOverload_BacklogComesBackAHeaderAtATime`: user and system load, slow executor, cap off and on | 60 s |
| the network dies: one BVN's dumped blocks double every cycle until it stops, while the other keeps accepting the users that feed it (run 144928Z, "we are dead") | `pkg/consensus/consim` `TestOverload_UncappedHeadersDoubleTheDumpUntilThePartitionStops` / `..._CappedHeadersKeepThePartitionMoving`; from the command line: `go run ./cmd/consim -bvns 2 -vals 4 -workers 4 -round 500ms -batch-timeout 100ms -batch-size 50 -user -skew BVN1=400,BVN2=250,Directory=2 -synth-per-user 1.5 -exec-cost BVN1=2.5ms,BVN2=2.5ms -max-header 1073741824 -duration 300s -height 0 -stall-after 40s` | 5 min |
| Directory anchors never reach a quorum at a BVN when dispatched copies are lost; every stream waits (run 225751Z) | none in-process yet: the simulator delivers every validator's copy in the same block, so a quorum always forms. Needs a simulator hook that drops a validator's dispatched copies; until then `TestAnchorAnswerCarriesQuorum` covers the answer's shape only | — |
| the requester pulls entries sitting in the destination's own backlog; heals with nothing dropped (runs 134346Z–142724Z) | `test/e2e` `TestRequester_LaggingDestination`: a block hook holds the destination's envelopes thirty blocks, the conductor reads the depth as its lag; nothing dropped and one package dropped | 3 s |

The death reproduction is what names the next design item: the header cap
bounds the blocks but not the inflow, and only cross-partition back-pressure
does (DIFFERENCES C7). Each of the others fails on the code before its fix
(checked by reverting the fix), and the
existing dropped-entry tests (`TestMissingSynthTxn`, `TestRangeRecovery`, ...)
stay green with it.

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

