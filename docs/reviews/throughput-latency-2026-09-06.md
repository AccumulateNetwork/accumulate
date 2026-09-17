# Why 500 tps does not hold, and why latency is high

Review of 2026-09-06, against soak run `test/docker/soak/runs/20260906T134054Z`
(issue-4193-producer-cache @ 9390f3c42; 2 BVNs × 4 validators, each container
one BVN node and one DN node in one process; 1 s blocks; 500 tps target;
BlockchainDB; chaos off). Paul's question: "seems we have fixed stalls, fixed
out of memories. However we can't maintain 500 tps, and latency is high. Do a
deep review; why?"

Measured means read from the run (metrics, profiles, ledgers). Inferred means
read from the code. Each finding says which.

## What the run shows (measured)

| time into run | DN height | heal entries (all nodes) | BVN execution lag (blocks) | load achieved | top node RSS |
|---|---|---|---|---|---|
| 17 min | 942 | 123 | 0–4 | 499.7 tps | 755 MiB |
| 31 min | 1654 | 7 493 | 3–7 | 494 | 760 |
| 51 min | 2883 | 75 936 | 12–17 | 460 | 862 |
| 1 h 21 | 4630 | 235 874 | 5–12 | 419 | 885 |
| 1 h 51 | 6410 | 378 947 | — | 392 | 906 |
| 2 h 15 | — | — | — | — | 990 |

- Block rate holds at ~0.96 blocks/s per partition throughout. Consensus is
  not the bottleneck; execution is behind consensus.
- Dispatcher drops: 0 on every node for the whole run. Nothing is lost; the
  heals are duplicates of late leader dispatch (#4248).
- Load generator: generated 2.95 M, **skipped 3.8 M** (submissions refused with
  "worker refusing user submissions: execution is lagging consensus"), rejected
  20. The network served ~390–460 tps of a 500 tps offer and turned the rest
  away.
- Synthetic backlog (source produced − destination delivered) at 2 h 15:
  BVN2→BVN1 1 637 of 369 755; BVN1→BVN2 1 911 of 163 472. BVN2 produces 2.3×
  BVN1's synthetics: the load generator's 25 614 lite accounts hash unevenly,
  so BVN2 lags first and hardest.
- Read probe (API reads): p50 2.4–3.2 ms, p95 7–28 ms. Reads are not the
  latency.

### Block execution wall time (measured, acc-bvn2-val4, both partitions)

`accumulate_dagbft_block_production_seconds`: 15 590 blocks, sum 8 586 s, mean
0.55 s. Distribution: ~2 500 blocks under 13 ms (empty), ~7 000 in 0.05–0.2 s,
~1 000 in 0.2–0.4 s, ~1 300 in 0.4–0.8 s, **3 426 blocks over 0.82 s (22%)**.
At a 1 s interval, every block over 1 s puts execution one block further
behind; the tail is the lag.

`accumulate_exec_phase_seconds_total{phase="serial"}` = 3 857 s,
`{phase="parallel"}` = **0**, `accumulate_exec_flushes_total` = **0**. Message
processing is entirely serial, and it is 45% of block wall time; the other 55%
is begin/close/commit.

### CPU (measured, 30 s profile of the whole process, both partitions)

22.4 s of samples in 30.1 s: **0.74 of one core** for a process that is falling
behind. The executor is not CPU-bound; a block's wall time is waiting and
serialization. Within the samples, cumulative:

| where | share of samples |
|---|---|
| block production loop (ProduceBlock) | 46% |
| message processing (ProcessAll) | 28% |
| store reads (`RecordStore.GetValue`) | 18.5% |
| BlockchainDB `SegmentStore.Get` | 14% |
| of which `lookupHistory` | 11% |
| `PseudoSynthetic.Process` | 16% |
| `SignatureMessage.Process` | 11% |
| `TransactionMessage.Process` | 11% |
| `Batch.Commit` | 10% |
| user submit path (`Submitter.submit` → `Validate`) | 5% |
| ED25519 verify | 4.6% |
| BPT update | 4.7% |
| syscalls (flat) | 17% |
| GC (flat) | 13% |
| crypto (flat) | 11% |

### Memory (measured)

Not a leak. On acc-bvn2-val4 (mem.csv, 20-minute samples) RSS went 760 →
842 → 1023 → 832 → 934 → 929 MiB while heap in use went 635 → 696 → 641 → 623
→ 826 → 773 MiB and next-GC 550–750 MiB: the runtime is using the headroom
GOMEMLIMIT (1700 MiB) allows, and both series oscillate rather than climb.
RSS is 95% anonymous (RssAnon 890 MB, RssFile 48 MB), so it is Go heap, not
mmapped store files. Two heap profiles 11 minutes apart show in-use *falling*
(371 → 342 MB); the largest holders are `bcdb.immutableCache.put` 79 MB, libp2p
pubsub `Message.Unmarshal` 53 MB, consensus `UnmarshalCertificate` 48 MB and
`UnmarshalHeader` 36 MB. Memory is bounded at this load; it is not the
problem this review is about.

### Correction on the heals

The monitor's "heals" column is the sum of `accumulate_conductor_heal_entries_total`
over the eight nodes: 580 592 at 2 h 28 (per node 19 k–160 k; bvn1-val1 and
bvn1-val2 160 k and 153 k against 391 k entries delivered to BVN1, i.e. **41% of
BVN1's delivered volume arrived as heal answers on those nodes**). Every node
runs its own requester, so one gap is asked for by up to eight requesters, each
answer is signed per entry by the source and submitted into the destination's
consensus, and the destination drops the late copy at classification for the
price of a hash. Executor cost of the duplicates: under 3% (measured on
bvn2-val4). Cost that is real: source-side signing and proof building while
lagging, DAG bytes (a 1 MiB/s block budget), and gossip verification on four
validators. #4248 stands, as a bytes-and-noise amplifier, not as the cause of
the lag.

## Findings

Ordered by what they cost. "Measured" is from the run; "from the code" is a
reading with file references.

### F1. The executor waits on the store: ~31 000 lookups per loaded block, most of them `pread` (measured)

BlockchainDB's own stats.json on bvn2-val4 (bvnn, 5 600 commits): perm
LookupTotal 95.7 M, dyna LookupTotal 78.9 M → **31 k store lookups per
committed BVN block, ~85 per message**; perm FilterAbsent 94.6 M (99% of perm
lookups are absence probes: `Transaction.Status` 11.5 M, `Produced` 5.4 M,
`Message.Cause` 4.9 M, …); dyna FilterWalked 4.7 M history walks. Process-wide
`/proc/1/io`: 77.5 k read syscalls/s. Sampling the BVN2 block goroutine 36
times: 24 in `pread` (bloom/index probes in `segment.lookup`,
`SegmentStore.lookupHistory`, `readValue`), 4 in the seal's fsyncs, 8
computing. History segments do not keep their blooms resident:
`segstore.go:289–313` issues K one-byte `ReadAt`s per history segment per
lookup and then binary-searches the index by `ReadAt` (:347–357); `loadBloom`
runs only for the active segment (`seal.go:447`). Shard 0000 has 43 perm + 7
dyna segments, so one absence probe is hundreds of tiny reads. The CPU profile
agrees: `SegmentStore.Get` 14% cum, `lookupHistory` 11%, syscalls 17% flat,
while the whole process uses 0.74 of a core. **This is the block-time tail.**

Why a *current* read walks history (measured + code, store slice): it is not
pre-image pinning (`stagedCommits` 0, 177 pre-image reads per commit) and not
the immutable cache. `KV2.Get` (`kv_2.go:281–289`) asks the **dynamic layer
first**, and the mutable store never short-circuits a miss:
`segstore.go:1791–1795` walks `lookupHistory` for anything outside the live
window, and `handoffBelowWindow` (`segstore.go:1085`) **frees the bloom of every
history segment** (BlockchainDB #64 did this for perm history, which grows;
dyna history does not). So every read of a permanent record (`Message.Main`,
chain `Element`), every absence check, and every dyna record untouched for more
than 40 blocks (chain heads and tails, BPT nodes) pays a dyna-history walk of
K=3 one-byte `pread`s per segment plus a ~14-`pread` binary search on "maybe".
Per user transaction: **≈56 dyna lookups, ≈45 history walks, 36 NotFound**
(11 152 filter-absent → full walks per commit). Among the misses:
`Transaction.(h).Main` **2.37 M misses and zero writes** in the run (a dead
probe in `batch.getTransaction`, `batch.go:190–194`); `Status` 8.5 per tx
(`checkStatus` on messages the block itself produced); `MainChain.ElementIndex`
1.8 per tx (the `unique` probe, `chain.go:236`, misses for every new entry).
Store side: BlockchainDB issue (resident dyna-history blooms or a key filter;
fsync duration metric). Accumulate side: route write-once shapes to the
permanent layer first (`getCurrent`, `database.go:935`), a negative cache per
block batch (a NotFound propagates through 3–7 nested batches to the store on
every re-ask), drop the dead probe, skip `checkStatus` for block-produced
messages, `unique` only where duplicates are possible.

Writes (bvnn stats ÷ ~250 user tx per commit — a per-transaction *share of
everything a block writes*, not the cost of one transaction; the block's
anchors, chains, index chains and BPT nodes are all in the numerator):
**≈73 records per unit of user work**. Counted directly instead, a
cross-partition transfer commits **six** records keyed by its own hash at the
destination (`TestUserTransactionWrites`: one body, one status, one cause, one
chains entry, one payments and one votes clear), against 337 records committed
in that destination window altogether. The gap between six and seventy-three
is chains, index chains and BPT nodes, which is where the writes actually
are — bodies 8.3 (`Message.(h).Main` for every wrapper), statuses
7.7, sets 21 (Produced 4.1, Cause 3.8, History 3.2, Transaction.Chains 2.4,
Signatures 2, Payments 2, Votes 2, Signers 1.3), chains 30 (SignatureChain
16.3, MainChain 9.2, RootChain 4.1), state 3. The #4236 remainder. Dyna puts
(10.4 k per commit) also set how fast dyna segments seal and so how many
history segments every probe above walks.

### F2. The seal: ~42 fsyncs per commit in ~5 serialized waves, both partitions on one disk (from the code, partly measured)

`bcdb/database.go:1017–1123 → writeThrough 592–639`: `persistExceptions`
(fsync when pending), `putRouted` per entry, `KVShard.SealBlock`
(`kv_shard.go:512–553`) over 8 shards: perm data ‖ index fsync, manifest tmp
fsync + dir fsync, dyna tail fsync, then `writeBlockHeight` two more. bvnn has
43 853 perm `PutConflict` (writes to `Message.(hash).Main` routed to the wrong
layer, logged to the exceptions file → `persistExceptions` fsyncs on most
commits; `accumulate_bcdb_dyna_exceptions{bvnn}` 43 889). Eight containers,
sixteen stores, one NVMe under LUKS: host `/proc/pressure/io` "some" 25%,
"full" 20%, device 60% busy. fsync latency is not measured (BlockchainDB counts
fsyncs, not their duration). `GetBptRootHash` runs `UpdateBPT` a second time
(`internal/database/bpt.go:27–33`; 0.29 s of 1.05 s in the profile).

### F3. Message processing is serial by default (measured + from the code)

`cmd/accumulated/run/dagbft.go:110` `setDefaultPtr(&s.ExecutionShards, 1)`;
soak.conf leaves `ACC_EXECUTION_SHARDS` empty → `exec_parallel.go:265` serial
path; `exec_phase_seconds_total{parallel}` = 0, `exec_flushes_total` = 0.
ProcessAll is 45% of block wall, and inside it the reads are RLock-only
(`KV2.Get`, `SegmentStore.Get`), so shards would overlap the F1 waits.
Anchors, synthetics and sequenced messages stay serial in any case
(`exec_parallel.go:239–262`). Not verified: determinism under shards (#4149
hazards).

### F4. Back-pressure is bang-bang, per validator, and turns users away 52–58% of the time (measured)

`MaxExecutionLag` = 8 blocks (`primary.go:132`), evaluated per header with no
hysteresis (`primary.go:278–299`); `SubmitUser` refuses with
`ErrExecutionLagging` → `NotReady` to the client (`worker.go:468–473`,
`dagbft/api.go:248–251`); system `Submit` never refuses (`worker.go:404–409`).
From `node-logs-live.txt` 13:43–16:01Z: each node was refusing **52–58% of
wall time** (bvn1-val4 76%, one episode 2 833 s), ~240 episodes per node,
median 17 s, period ≈ 28 s. Load generator: skipped/generated = 1.29, achieved
370–460 tps. The oscillation (lag 2 → 17 → 4) is the control loop: when the
threshold trips, 8 full blocks are already queued and 1–3 rounds pass before
empty headers reach the queue (overshoot to 17–19); non-lagging validators'
headers keep carrying batches, so the lagging node's blocks stay full while its
own clients are refused; on resume the backlog is metered at 128 KiB/header
(`header_builder.go:108–122`) and re-trips it. A refusal costs the client the
whole tick: the load generator re-picks nodes up to 4× with 50–150 ms sleeps
and counts a skip; there is no server-side queue. **This is why 500 tps does
not hold**: the network executes ~260 msgs/s per BVN (7 700 s of block time
over 5 400 loaded blocks ≈ 1.4 s per loaded block) and refuses the rest.

### F5. Cross-partition latency is two Directory round trips plus lag (from the code, backlog measured)

Same-partition minimum ≈ 2.3 s (batch ticker 0–1 s, round interval 0–0.5 s,
commit 0.5–1.5 s, execute 0.55 s); observed 2.3 + L for lag L = 10–21 s.
Cross-partition BVN2→BVN1: execute at N; anchor sent at begin(N+1)
(`block_begin.go:88, 247`); DN batch/header/commit 2–3 s, executes at the 6th
of 8 copies so paced by the third-slowest BVN validator's lag; DN anchor with
the receipt leaves at DN begin(M+1); BVN2 commits it after L; the package is
dispatched at begin(N′+1) **by the leader only** (`block_begin.go:278, 404–416,
449`); BVN1 commits after L′. Minimum 9–12 s; observed 35–60 s. Little's law on
the ledgers: BVN2→BVN1 backlog 1 637 at 45.6/s ≈ 36 s; BVN1→BVN2 1 911 at 20/s
≈ 95 s. There is **no path that sends a synthetic before the DN receipt**
(`sendSynthWithOwnProof` is the single-message receipt form, built from the
same DN `blockReceipt`, `block_begin.go:495–606`). The destination already
stages proofs against a named DN block (`anchor_staging.go:64–85`), so sending
at begin(N+1) with the source-root proof and letting the destination complete
it from the DN anchor it executes anyway would remove steps 4–6 (≈ 3–5 s + L +
L_leader) and the whole #4248 exposure. Spec change (executor.md "Dispatch",
"Anchor staging").

### F6. Eleven executor invocations per user transaction (from the code, profile-weighted)

For one lite→lite `SendTokens`: at the source `TransactionMessage.Process` ×3
(envelope pass: status read, put, no execution; via `CreditPayment`: pending
add; via `AuthoritySignature`: the execution), `UserSignature.Process` (second
ED25519 verify of a signature already verified at `Validate`), the produced
`CreditPayment` and `AuthoritySignature` wrapped as `PseudoSynthetic` and run
in the next pass (`exec_process.go:307–319`); at the destination
`SyntheticProof`, `SyntheticMessage` (verifies the member signature twice:
`check` and `noteRemoteDelivered`, `msg_synthetic.go:205, 267, 475–484`),
`SequencedMessage`, `TransactionMessage` ×1. Totals: 11 invocations, 4
`TransactionMessage.Process`, ~5 status reads, ~9 message/status puts, pending
add + remove, 4 ED25519 verifies per node. `PseudoSynthetic.Process` at 16% of
samples **is the user transaction's own execution** through this cascade
(`TransactionMessage` 2.16 s, `CreditPayment` 1.24 s, `AuthoritySignature`
0.92 s, `SignatureRequest` 0.69 s for ADI principals), not anchors
(`BlockAnchor.Process` 1.2%). ED25519 is 10.8% of samples overall: 41% user
signatures, 33% libp2p transport, 12% DAG certificates, 10% votes.

### F7. Anchors: 24 copies per block pair on one process, 83% never execute (measured)

Every BVN validator sends its own `BlockAnchor` to the DN (4 per BVN block);
the DN sends to every partition **including itself** (`conductor.go:246–250`,
8 copies per DN block); the destination executes the copy that reaches
threshold and records the rest (`anchor-collected` 13 310 + `anchor-tossed`
5 705 vs executed ≈ 3 800 on one node). #4224 made a copy cheap (one body, one
set write), so this is message count, verifies and one block of latency at the
6-of-8 threshold, not CPU.

### F8. Load that produces nothing (measured)

`accumulate_exec_transaction_failed_total` on bvn2-val4: 285 708 failed
executions in 2 h 25 (sendTokens 199 944, updateAccountAuth 29 411,
createIdentity 14 929, writeData 14 859, burnTokens 14 774,
syntheticDepositTokens 9 357). The load generator's log is full of
"insufficient credits: have 0.00, want 0.01": its accounts run dry and it keeps
sending. bvnn also shows 394 476 perm `PutDuplicate` (the same value written
twice). A failed transaction costs the full cascade of F6 and the store writes
of F1; the network is spending perhaps a sixth of its execution on
transactions that were never going to succeed. That is the load generator's
problem to fix (fund before sending, stop on dry accounts) and, separately, a
question of whether validation should have refused them at submit.

### F9. Memory is not the problem (measured)

See "Memory" above: bounded, oscillating with GC headroom, in-use falling
between two profiles.

### Addendum at 3 h 50 (measured): the laggards diverge

Execution lag per node at 17:32Z: bvn1-val1 **202** blocks, bvn1-val4 96,
bvn2-val1 39, bvn2-val2 29, bvn2-val4 45, the other three 3–4. Same partition,
same load, same disk: two BVN1 validators are minutes behind while two keep up.
The 202-block node's block goroutine, sampled, is in `pread` inside
`SegmentStore.lookupHistory` → `segment.readValue` (F1, live). Its RSS is
1.43 GiB of 2 GiB (heap in use ~950 MiB; the consensus worker holds 92 MB of
its own unexecuted batches, `batch_store_bytes{kind="own"}`), so a laggard
also converts lag into memory. Load 331 tps. Why two nodes diverge from their
peers is not determined here (not the heal volume: val1 healed 160 k, val4
33 k); the per-partition block timer and seal-duration histogram of P10 are
what would show it. Relevance to the plan: lag compounds — a node that falls
behind keeps refusing its users (P2) and probes more history (P1) — so the
fixes are not optional for steady state.

### The run's end at 4 h 26 (measured): a stall, in the seal

At 18:03Z BVN2 stopped producing blocks; the stall guard stopped the run at
18:06:50Z after 241 s (BVN1 had also stalled 120 s at 18:04, wedge capture
`wedge-20260906T180450Z`). Every node's block goroutine was inside
`closedBlock.Commit → bcdb.commit → writeThrough → KVShard.SealBlock`, in
`sync.WaitGroup.Wait` for the shards' `KV2.Seal` goroutines, which were all in
`syscall.Fsync` (`pendingSync.finish`, `pendingSeal.promote → writeIndexTmp`):
16 fsync goroutines on bvn2-val1, 24 on bvn1-val1, 27 on bvn1-val2, 12 on
bvn1-val4 — of the order of a hundred concurrent fsyncs against one LUKS NVMe,
none returning for minutes. Consensus kept going (round counters identical on
all four BVN2 nodes, 61 588; "proposing headers without batches") and lag was
small on three of them (2–6 blocks): this was not the executor and not
consensus, it was the store's commit barrier. No kernel message, no trim job
(fstrim next due Monday), no host process on the disk at that time; BlockchainDB
stats show every store's commit rate dipping together only in the 18:04
interval. What is missing is exactly P10's seal-duration histogram: the seal's
fsync wall is not measured anywhere, so whether the disk degraded under
sixteen stores' seals or a single seal stalled the rest cannot be separated
here. Streams at the kill: BVN2→BVN1 2 453 undelivered, BVN1→BVN2 1 213,
BVN2→DN 420 — the in-flight backlog, nothing older.

Consequence for the plan: **F2 is not a 10–15% item.** A commit that waits on
~40 fsyncs per node per block, across sixteen stores on one device, is a stall
waiting to happen once the disk queue is deep enough; the block-time tail of F1
and the seal of F2 are the same wall from two sides. The seal must either
leave the block path (durability acknowledged after the state hash is
broadcast — a spec decision against database.md invariant 5) or cost one
barrier, not five waves. Filed as #4259 here and on BlockchainDB#88.

Also seen at the end: laggards asking sources for anchors they have not
executed yet ("Source cannot serve missing anchors: 12220 is not in the
cache"): a node hundreds of blocks behind consensus asks for entries that sit
in its own committed, unexecuted backlog, and the source has released them on
the partition's Delivered claim. The requester should not ask while execution
lags consensus (#4260).

## Why, in one paragraph

Consensus makes a block every second. Executing a loaded block takes about
1.4 s on average because the executor spends two thirds of its wall time in
tiny `pread`s against BlockchainDB (every miss walks dyna history with its
blooms freed; ~31 k lookups per block, 36 of them NotFound per user
transaction), runs every message serially, executes eleven messages for each
user transaction, writes some seventy records per unit of user work (six of
them the transaction's own; the rest chains, index chains and BPT nodes), and
seals with ~40 fsyncs on a disk
shared by sixteen stores. Execution therefore falls behind consensus; at eight
blocks behind a validator refuses its users outright with no hysteresis, and
because its peers' headers keep the blocks full, it stays behind and refuses
52–58% of the time. What it does execute includes a sixth of transactions that
fail for lack of credits and, on some nodes, a 41% duplicate stream of healed
synthetics. Cross-partition latency is two Directory round trips plus the lag
at each end (35–60 s), because a synthetic is never sent before the Directory
receipt and only the leader sends it. Memory is bounded and is not part of
this.

## Plan (ranked by expected gain; each says what it breaks)

Issues: umbrella #4258 (this review in full). P1 #4249 + BlockchainDB#88; P2 #4250;
P3 #4236 + #4251; P4 #4252; P5 #4149; P6 #4253 (spec decision); P7 #4254; P8 #4255;
P9 #4256; P10 #4257; P11 #4248 (corrected).

| # | change | where | expected gain | breaks / needs |
|---|---|---|---|---|
| P1 | **Store misses stop walking history.** BlockchainDB: keep dyna-history blooms (or a resident key filter) so a miss is memory-only; add an fsync-duration histogram. Accumulate: `getCurrent` routes write-once shapes to perm first; a per-block negative cache; drop the dead `Transaction.Main` probe; no `checkStatus` for block-produced messages; `unique` probe only where a duplicate is possible; `GetBptRootHash` must not run `UpdateBPT` twice. | BlockchainDB (issue there) + `internal/database`, `pkg/database/keyvalue/bcdb`, `internal/core/execute/v2/block` | −2.2 of 3.2 s store-read CPU (≈ −20% executor CPU), −35–55 k syscalls/s; the block-time tail is mostly this | store memory for blooms (~19 KB per dyna segment); routing must match the key classifier exactly or reads go stale |
| P2 | **Back-pressure with hysteresis and pacing, per partition.** Trip at 8, resume at ≤ 4; while lagging, accept users into pending and let the synchronous seal block the submitter instead of refusing; the empty-header bound stays. | `pkg/consensus/primary`, `pkg/consensus/worker`, spec consensus.md invariant 9 | recovers the 25–50% of offered load now refused; user latency becomes queueing (2.3 s + L) instead of a lost tick | clients see slower submits instead of `NotReady`; the load generator's skip accounting changes |
| P3 | **Records per transaction 73 → under 30** (#4236 remainder): one body, one status, Produced once, no History/Signers/Transaction.Chains where nothing reads them; fix the `Message.Main` stored-form conflict (43 853 refused writes → exceptions fsync on most commits). | executor, `internal/database`, spec database.md | fewer dyna puts → slower dyna sealing → fewer history segments for P1 to probe; −1 fsync/commit; less marshal/GC | every reader of a dropped record must be found first (API, `MessageIsReady`, dedup) |
| P4 | **Single-signer fast path**: when the initiator's signature alone satisfies the authority (lite identity; single-key page, threshold 1), execute inside `UserSignature.Process` instead of producing `CreditPayment` + `AuthoritySignature` wrapped as `PseudoSynthetic`. | `sig_user.go`, `exec_process.go`, spec executor.md "Signatures" | −7 executor invocations, −2 `TransactionMessage.Process`, −pending add/remove, −5 puts, −3 status reads per tx; ~10–12% of CPU samples | a new spec section; the payment/vote records disappear for that path |
| P5 | **Execution shards on by default (4–8)** once #4149 determinism is proven. | `cmd/accumulated/run/dagbft.go`, `exec_parallel.go` | overlaps store IO and ED25519 across shards: ~20–25% of block wall on user-heavy blocks | parent-batch mutex contention; anchors/synthetics stay serial; must be validated in consim first |
| P6 | **Send synthetics at begin(N+1) with the source-root proof; the destination completes the proof from the DN anchor it executes anyway.** | `block_begin.go` dispatch, `anchor_staging.go`, spec executor.md "Dispatch"/"Anchor staging", healing.md | cross-partition minimum 9–12 s → 5–6 s; removes the leader's lag from the path and the whole #4248 exposure | a spec decision (Paul): the destination trusts a proof it completes itself; #4248's in-flight rule becomes moot |
| P7 | **Anchors: one execution per (source, block)**; do not record tossed/collected copies; the DN's anchor to itself is 8 copies per block — send one, or none if nothing reads it. | `msg_block_anchor.go`, `conductor.go:246–250`, spec executor.md "Dispatch" | −20 messages, −20 verifies, −60 puts per block pair; one block of latency at the 6-of-8 threshold | leader-only anchor send or an aggregated signature changes the spec |
| P8 | **Verify each signature once**: carry the Validate result to Process; verify a package member once in `SyntheticMessage`; let the collection proof be the member's authority (no per-member sign at the leader, none in heal answers). | `sig_user.go`, `msg_synthetic.go`, `block_begin.go:744`, spec executor.md, healing.md | ≈2–2.5% CPU, −1 sign per member at the leader and per healed entry at the source | dropping member signatures is a spec change |
| P9 | **The load generator stops sending what cannot succeed**: fund before sending, stop on dry accounts, no timestamp replays; report offered vs accepted honestly. | `tools/cmd/loadgen` | a sixth of executed transactions currently fail; that execution comes back | none |
| P10 | **Observability to settle what this review could not**: `block_production_seconds` labelled by partition; Begin/ProcessAll/Close/Commit timers exported; a seal-duration histogram; a per-`Get` syscall counter in BlockchainDB. | `internal/node/dagbft`, `block_end.go`, BlockchainDB | none directly; the next review measures instead of samples goroutines | — |
| P11 | #4248 as filed (every validator dispatches, or in-flight measured from the send) — superseded by P6 if P6 is chosen. | `block_begin.go`, `sequencer_cache.go` | −41% duplicate volume on the worst nodes | 4× dispatch bandwidth if every validator sends |

P1–P3 are the throughput; P2 alone turns "refuse half the time" into "run at
capacity". P6 is the latency. P4/P5/P7/P8 are the multipliers that raise
capacity itself. Everything goes through an in-process reproduction first
(simulation first): a slow-store simulator that counts `pread`s and NotFound
reads per transaction, and a consim run with one lagging executor that asserts
the refusal duty cycle.
