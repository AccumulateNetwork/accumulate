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

   *Landed (#4291).* Paged by stream and by sequence number, bounded in bytes
   as well as span, with `More` saying whether there is another page and the
   block index on every one. A failed `Load` leaves staging empty and
   retryable. The page's block is the consensus index of the last block the
   executor processed, which is not necessarily a block whose state was
   written — DIFFERENCES E11 says why that is the safe direction.
2. **Collect without executing.** The executor applies a committed block to
   staging only — classify, intake proofs, hold entries and anchor copies —
   and a joining node runs every buffered block through it from `P + 1`.
   Then, at `Q`, releases through each stream's `Delivered` from the pulled
   ledgers and decides proofs against the anchors executed by `Q`. Test: a
   node that collected blocks `P + 1 .. Q` on top of a peer's staging at `P`
   holds exactly what the peer holds at `Q`.

   *Landed (#4292).* Collecting runs each arrival through its own executor's
   check, so it holds what a block holds and refuses what a block refuses —
   including absorbing an anchored collection proof into the stream's replica
   — and writes nothing. The DAG service buffers committed groups while a node
   is joining and applies them to the peer's staging once it has it, in order.
   The collecting node in the tests has its own store, so the equality it
   proves is not equality given identical state.
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

   *Landed (#4294), except the Docker proof.* The handoff is served by the
   block production loop, which is the only thing that produces blocks.
   `Conductor.Rejoin`, its metric and `Executor.Collect` are gone. A node that
   has executed a block before enters collecting mode before consensus starts
   and joins; a node that finds no peer with staging to give — every validator
   restarted — executes from its own state instead of waiting forever.
   Runnability was made a question about the entry and the state rather than
   about when the entry was held, or a joining node would hold collected what
   its peers hold runnable and its stream would stop where theirs moved.
5. **Serve last.** Node state `BOOTING → ACTIVE → COMPLETE`, advertised; the
   sequencer and the historical API refuse until `COMPLETE`; the cache fills
   by backfill. Test: a request for missing data routed to a `BOOTING` node
   is refused and answered by a `COMPLETE` one.

   *Landed (#4295) with the departures in DIFFERENCES E11.* Every private
   call the sequencer serves, and the staging snapshot, refuse while a node is
   joining, counted per call, with the state as a gauge. A node that joined
   answers `NotReady` — not `NotFound`, which strands a stream — for the
   blocks it did not execute. Serving resumes when the node EXECUTES, not when
   its history is backfilled: there is no backfill on this line, so `COMPLETE`
   would mean a node that joined never answered again. The state is not
   persisted and not advertised, and the v3 querier is not gated.

Order and gates: 1 and 3 in parallel (they share nothing); 2 on 1; 4 on all
three, gated on `TestOneValidatorRestartDoesNotDiverge` with the interim pull
removed and no Docker chaos run before it passes; 5 after 4; then **#4296**,
a gate before any chaos run: a joining node must be able to find a validator
to ask, and must refuse to execute when it found none — *delivered and
closed*. Then the defects that keep a join from completing at all, which the
debug agent found in run `20260918T131713Z` and which are the same kind of
gate for the same reason — the chaos gate cannot be met on a build where no
join completes: **#4303** (a joining node pulls from itself, so nothing is
ever pulled), **#4305** (the pull never updates the BPT, so the root cannot
move), **#4306** (the ledger and synthetic accounts are named by no block and
pulled once, so the set is structurally incomplete), **#4298** (and those two
accounts cannot be verified anyway), **#4309** (a BVN's join wrote four
`dn.acme` accounts into its own store, putting its root permanently beyond
every anchored root — independent of the others and fatal on its own),
**#4304** (a fresh network cannot start: `Fresh` is dead code), with
**#4297** and **#4307** beside them as the un-gated services that made the
first possible. All but #4298 and #4304 are fixed on
`issue-4303-join-pulls-from-peers`, **merged 2026-09-18** (`05221528b` into
`c2b0e9d2f`), the full gate green including consim at 717s; the fix introduces
one knowing contradiction with this spec — the block ledger is taken on the
peer's word — which is **#4310**, and the reviewer judges it.

**Merged with known exceptions, agreed by Paul.** #4313 and #4314 remain open
and are *not* fixed by that merge. **#4313** is an executed exploit on the
path the join drives — `RestoreHead` skips its discharge at absorbed
boundaries, letting one peer write 256 chosen hashes into the anchor pool; its
fix commits `Count` into the BPT leaf, which changes the leaf and therefore
needs an activation height. **#4314** is that a collection proof does not bind
absolute index: `Validate` never reads `Count`, and restated counts verify.
Both pre-date the branch. Neither is a builder's call to start: #4313 is
blocked on the activation height, which is Paul's.

**Ahead of any design work on #4298, one measurement** (lead, 2026-09-18):
how often `<partition>/ledger`'s scheduled-events BPT and
`<partition>/synthetic`'s delivery queues are actually non-empty at the block
a peer serves. Both are skipped when empty
(`internal/database/observer_prod.go:49-77`) and the local delivery queue
drains at the next block's `Begin`, so if they are rarely non-empty then
#4298 is a retry rather than a blocker. It has never been measured; it is a
sample on a running soak, not a design. Then
`30m-100tps-chaos.conf`; then #4299, provisionally, which that run may
promote ahead of itself; then the 24-hour run.

**#4319 reframes everything below it, and Paul has now decided it.** A
debugger agent proved by execution on 2026-09-18 (in-process
simulator only — no Docker, no soak) that a restarted node **never rejoins at
all**: it stays BOOTING, executes nothing, and every settle batch expires.
`pulled=0` on all 59–60 rounds; the settle histogram 366/359/352/345/338 is
every batch exhausting `maxSettleRounds = 4` (`internal/node/join/state.go:66`,
window at `:431`) and being discarded. The cause is structural: a restarted
node already holds every cold account, so it differs from its peers **only in
the accounts that change every block** — and those are served at the peer's
*current* block, which the Directory has not yet anchored, so
`settleBatch`'s `anchors.AnchoredRoot` (`state.go:410`) never resolves inside
the window. The lag grows rather than closing: 88−38=50 at round 0, 238−63=175
at round 50. A node joining from **empty** converges (pulled=3,3,19, matched
round 7) because its cold accounts have old, already-anchored served blocks.
**That is why `test/e2e/join_pull_test.go` passes from `emptyDb()` and the
real restart case cannot** — the join is tested in the one configuration where
it works. This is not #4290's divergence; the node never returns to service.
It also names the cause of #4316: the same `pullSpine` write turned a node
holding correct, peer-identical, anchored block-22 state into a root no node
ever held and the Directory never anchored.

**DECIDED by Paul, 2026-09-18: a restarted node catches up by REPLAYING THE
COMMITTED LOG, not by pulling state.** It already holds every cold account;
give it blocks `R+1..N` from the DAG log and let it execute forward. His
rationale: *the committed log is the durability point, and execution produces
anchored state by construction, so the anchor treadmill that #4319 documents
cannot arise — the node never needs to be served state at a block it cannot
verify.* Replay removes the premise rather than tuning the gap.

Two alternatives were considered and **rejected**, recorded so they are not
relitigated. **(a) Make `settleBatch` wait on the anchor instead of discarding
after `maxSettleRounds = 4`** — rejected; #4319's own evidence is why it could
not have worked, since the lag *grows* and no finite window closes it.
**(b) Have peers serve state as of a caller-specified anchored block** —
rejected, partly because it requires historical reads, which is a standing
position and not a new one.

**The consequence that bounds the decision: #4238 is now THE constraint on the
chosen mechanism, not an alternative to it.** Replay works only as far back as
the log is retained, so **the retention horizon is the maximum outage a node
can survive**. #4238 is where that limit lives — a restarted validator that
cannot rejoin past a DAG GC horizon, with `RequestStateSync` a stub. **Its
figures must not be quoted as current**: the 2000-round depth and the ~8-minute
conclusion were written 2026-09-06 against a different build, and a debugger is
establishing the real horizon by execution. Until then the honest statement is
that replay is bounded by log retention and the size of that bound is being
measured. What happens to a node down *longer* than the horizon is a follow-on
the decision does not settle; #4319's evidence that a node joining from empty
does converge (pulled=3,3,19, matched round 7) makes "wipe and start from
empty" a real candidate rather than a counsel of despair.

The decision does not by itself delete the state-pull path — it says a restart
does not take it. What becomes of `PulledState`, `pullSpine` and the settle
window is downstream design work, and **none of #4316, #4318, #4301, #4310,
#4298 or #4306 should be closed on the strength of it**. A decision is not a
delivery; until replay is built and proven, the pull path is still the code
that runs.

**Seven items filed 2026-09-18 after the merge, and where they are proposed to
go — a proposal, not a change to the approved order.** The approved order
above stands until Paul says otherwise; this paragraph is the issue manager's
reasoning about where these belong, recorded so nothing lives only in a chat
message. Its twin is the note of the same date on #4205.

Three findings were relayed as "recorded on #4205" and were **recorded
nowhere** — not in any of that issue's seven notes. They are now filed with
their evidence, and two of the three have since been confirmed by execution
rather than reading:

- **#4316** — `localBlock` reads the ledger record `pullSpine` just
  overwrote (`internal/node/join/state.go:320` reads what `:575`/`:588`/`:596`
  wrote from the peer via `pull.go:685` and `:418`), so the block-ledger walk
  measures the node against the peer rather than against itself. Today the
  page-diff backstop covers for the mis-measured walk — but the *write* is a
  live corruption, not a cost: #4319 shows it turned a node holding correct,
  peer-identical, anchored block-22 state into a root no node ever held.
- **#4317** — **three** of the five #4303 fixes have no test that fails when
  broken, established by a test-auditor running the reverts: *pull addressed at
  a named peer*, *the set comes from the block ledger*, *joining node must not
  serve*. `705da0ab5`'s claim that `TestJoinPullsFromPeersAndPromotes` "fails
  on each of the five taken alone" is **refuted — it fails on two.** (Filed as
  "two", from a reading of test bodies that asked a different question —
  *targeted* test versus *any* test that goes red. The audit's answer is the
  one that counts.)
- **#4318** — the join's verification rests on one call site
  (`internal/node/join/state.go:413`); substituting `Keep()` for `Settle()`
  makes it verify nothing. **Executed: the suite stayed green**, so
  `internal/core/bootstrap/pull/verify.go` is entirely dead to its caller. This
  is why #4303's "zero verification failures in 22 MB of logs" read as success
  rather than as the verifier never running — and it means the safety property
  #4310 and #4301 lean on is untested.
- **#4320** — the layer beneath both: the daemon's join wiring has no test at
  all. Setting `joining := false` at `cmd/accumulated/run/dagbft.go:458` leaves
  the whole suite green, `./cmd/accumulated/run` included, as does dropping the
  `NodeState` gates (`run/api.go:83`, `dagbft.go:648`, `:660`, and a third at
  `:674`) or `self = inst.p2p.ID()` at `:476`. `PulledState.Executing` has
  exactly one caller in the tree, `dagbft.go:542`, and no test reaches it. So
  the switch that decides whether a node joins at all can be turned off and
  nothing notices — and so can the fixes #4297, #4303 and #4307 cost a run
  each to find.

And two that follow from the block ledger:

- **#4315** — anchor the block-ledger chain into the root chain. Verified in
  the tree: `recordBlockLedger` at `block_end.go:245` appends at `:968`, after
  `enumerateModifiedChains` at `:120`, and no `addChainAnchor` names
  `BlockLedgerChain()` (the three that exist are `:226`, `:277` for the bpt
  chain per #4272, and `:551`). The same ordering accident #4272 fixed, and
  the mechanical reason #4310 exists. **Blocked on an activation height —
  Paul's call**, as #4272's was.
- **#4312** — unadjudicated, and Paul's. A review disproved its "entries are
  lost" premise, and the two code claims check out: `Entry(i)` returns
  whatever sits at that position unchecked (`merkle/chain.go:494`) and
  `AddEntry(hash, unique)` returns `nil` without appending on a stale
  `ElementIndex` hit (`:317-324`). That is consensus divergence, not a storage
  leak, and it inverts the proposed disposal step. **Nothing on #4312 should
  be built until that is answered.**

*Proposed placement, for Paul.* What a 30-minute chaos run can be a gate for
is the question, and today the answer is nothing about the join. It would pass
or fail for reasons unrelated to it:

- **#4304** — a fresh network cannot start; `Fresh` is dead code, evaluated
  inside `if joining` at `dagbft.go:513` and therefore always false. The soak
  starts a fresh network. Run `20260918T131713Z` ended 11 of 12 nodes
  collecting, one executing, the Directory stuck at block 1. Until this is
  fixed the run does not begin, let alone gate.
- **#4319** — on the current build a restarted node never rejoins at all, so a
  chaos run's restarts produce a node that stays BOOTING. The run cannot
  distinguish that from any other failure. **Now decided, and therefore work**:
  replay `R+1..N` from the committed log. Nothing is built yet.
- **#4318** — the run cannot distinguish a join that verified every account
  from one that verified none. **Executed: the substitution survives a green
  suite.** One test fixes that.
- **#4317** — **three** of the fixes the run would rest on have no test that
  fails when broken, and `705da0ab5`'s coverage claim is refuted by execution.
- **#4320** — the run rests on the daemon wiring that decides whether a node
  joins at all, and that wiring can be switched off with the suite green.

So the order that makes the gate mean something is: **#4319's replay**, then
#4304, then #4318, #4317 and #4320 (all three cheap, all three about being able
to read the run's verdict), then the run. **#4238** sits beside #4319 as its
bound rather than in the sequence — the retention horizon is now the maximum
outage a node can survive, and its size is being measured by execution; its old
~8-minute figure must not be quoted. **#4316** is a live corruption rather than
the masked cost it was filed as — see #4319's second defect — and belongs with
#4319 because the same write causes both; it is not separately gating.
**#4315** and **#4313** do not gate the run at all, but both are stopped dead
until their activation heights are named, so they should be asked about now
rather than when they become urgent. **#4312** is a fork, not a tweak, and
nothing on it should be built until it is answered.

**Still open for Paul: the activation heights (#4313, #4315) and the #4312
adjudication.** The #4319 decision is made and recorded above; the rest of this
placement remains a proposal with its reasoning, which is what the plan asks
for when the order would change.

**The tests that passed do not exercise the mechanism.** This has to be said
next to the order, because the order was built on them.
`TestOneValidatorRestartDoesNotDiverge` replaces steps 3 and 4 with a store
copy (`test/simulator/partition.go:96-116`, `memory.Database.Export`/`Import`);
`TestPullReachesTheAnchoredRoot` sources from `api.Querier2{Querier:
sim.S.Services()}` (`test/e2e/state_pull_test.go:128`), so there is no p2p, no
routing and no self-dial to find, and it calls `batch.UpdateBPT()` by hand at
`:161` and `:187` — the exact step the production pull omits. A test that
performs by hand the step its production caller must perform proves the
library and not the caller. Steps 3 and 4
show a plan before code (a port across a five-month database-API gap; a
rewiring of consensus start-up). The observations behind the order — three
causes each sufficient alone, why a source's cache and consensus replay are
both wrong, how to find the next divergence in minutes — are on #4205
(2026-09-18).

Why #4296 is a gate and not a fix in passing: the chaos gate cannot be met on
a build where the join never engages, and run `20260918T124530Z` is a build
that was exactly that — every lookup for a validator to ask returned an empty
list, on every node and both partitions, and the restarted node executed from
its own stage at Directory block 186 and BVN1 block 181. **Both halves of
#4296 are in the gate.** The lookup fix removes the symptom; the half that
counts the validators found, and refuses to execute on a count of zero unless
the node has executed no block, is what makes the refusal sound. The
fallback's safety rested on the clause "every validator of the partition is
asked for its staging" (DIFFERENCES E11), which was never true on a live
network, so "asked nobody" was being read as "nobody holds anything". Any
later failure that empties the peer list reaches the same wrong conclusion by
the same route, so the count is part of the gate and not an optimisation.

#4298 (an account carrying pending signature material, scheduled events or a
delivery queue cannot be verified, so a partition holding one cannot be
joined) and #4299 (a message whose remote stub the store cannot resolve is
dropped by collect) are the two holes DIFFERENCES E11 calls the ones that will
meet the chaos run first. Run `20260918T131713Z` did not promote either and
did not rule either out: it stalled before the state in which either would
bite, on #4303 and #4304. It did change what #4298 is — the accounts that
cannot be verified are `<partition>/ledger` and `<partition>/synthetic`, which
are in every join's required set, so it is not "a partition holding one cannot
be joined" but "no partition can be joined" — which is why #4298 now sits with
the blockers and #4299 alone stays provisional. They are placed after the
30-minute run deliberately: both are expensive to design for in the abstract
and cheap to observe, and a live join that stalls on an unverifiable account
says so in minutes with evidence no amount of reasoning produces. The
30-minute run decides whether either moves ahead of it; a stall on one is a
promotion with evidence, which is the only kind to make.

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

