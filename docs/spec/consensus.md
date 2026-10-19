# Consensus — Specification

**Scope of this part, for now.** The consensus part of the specification is
not yet written in full: rounds, headers, votes, certificates and the DAG are
still a gap (see [SPEC.md](SPEC.md)). This document covers one thing that
could not wait for the rest — **the memory the batch plane may hold, and what
happens when it is full** — because three soaks in one day ended on it. It is
written in the same two sections as the other parts and will be folded into
the full consensus part when that is written.

## 1. Architecture — what we are doing

### What a batch is

A transaction submitted to a validator is validated, then held by one of the
validator's **workers** until the worker **seals** it with others into a
**batch**. The batch is what consensus orders: a header names the batches a
validator proposes, votes and certificates commit to headers, and a block
executes the batches of its committed certificates in certificate order. A
batch is broadcast to every validator of the partition when sealed, and any
validator that lacks a batch its committed certificates name must fetch it
from a peer before it can execute the block.

A node that is not in the partition's committee has no worker for it: what it
accepts for that partition it relays to a validator that does (executor.md,
"Sync" step 6), and its own batches, headers and votes for that partition are
nothing to consensus.

So a validator holds batches in four places, for four reasons:

| store | holds | until |
|---|---|---|
| **pending** | accepted transactions not yet sealed | the worker seals them |
| **inbound queue** | batches received from peers, not yet stored | the worker stores them |
| **active store** | batches this validator may still need to execute — its own until committed, peers' until executed | the certificate naming them is executed |
| **retention** | batches already executed, kept so a lagging peer can still fetch them | the retention window passes |

### The invariants

1. **Every store is bounded in bytes, and the bound is per node.** A budget is
   a number of bytes a node may spend on one purpose, whatever the number of
   partitions, workers or peers. Workers divide a budget; they do not each
   get one. A count is not a bound: a batch is anything from one transaction
   to 500 KB, so a count says nothing about memory.
2. **A batch is sealed by size or by fullness, not by the clock alone.** A
   worker seals when a batch is full (`BatchSize` transactions or
   `MaxBatchBytes`) or when a timeout passes with something pending; the
   timeout exists so a quiet validator does not delay a lone transaction, not
   so a busy one emits a batch per transaction. Under load, batches are full.
   The number of batches a partition emits per second is therefore bounded by
   its throughput divided by the batch size, and every count-shaped structure
   downstream — stores, retention, headers — is sized in seconds of traffic
   by that number, not by the clock.
3. **What a vote needs is never evicted.** The active store evicts least
   recently used batches to stay within budget, but never a batch named by a
   header this validator has not yet voted on or a certificate it has not yet
   executed. Evicting such a batch only causes it to be fetched again.
4. **Own uncommitted batches are bounded by refusing new work, not by
   growing.** A validator's own batches cannot be evicted — it is responsible
   for them reaching a certificate — so when they fill the budget the
   validator stops accepting submissions and says so (`NotReady`), and the
   submitter backs off. The store never exceeds its budget for its own
   batches; the submitter waits instead. A partition whose commits lag its
   offered load fills its budget, refuses, and stays live; it does not fill
   its memory.
5. **A full store is reported once, not once per submission.** The condition
   is a state, and a state is logged when it changes and counted while it
   holds. A warning per refused submission at 500 tps is 500 warnings a
   second, which is itself a resource. The same rule sets log levels:
   lifecycle events and state transitions are `Info`; anything that happens
   per header, vote, certificate, sync request or transaction is `Debug`, and
   the arguments of such a line are not built unless the level is enabled.
6. **Retention is a window in seconds of traffic, bounded in bytes.** A peer
   that is further behind than the window cannot recover by fetching batches
   and must resync (E11; the mechanism is not snapshots — Paul, 2026-09-06).
   The window is not made longer to cover that case.
7. **A batch is proposed once.** Re-proposal exists for a batch no header has
   certified — the author's own recovery from a lost broadcast. A batch that
   is in a certified header is never proposed again, whatever the executor
   has since done with it; a second certificate naming the same batch is the
   defect that makes the executor need a batch it has already retired.
8. **Own batches and the peer cache do not share a budget.** Own batches are
   bounded by refusal (invariant 4); the peer cache is bounded by eviction.
   A full own store must not empty the cache of peers' batches, because
   those are what the next header's vote needs (invariant 3).
9. **Consensus does not outrun execution.** A certificate the executor has
   not executed is memory — its batches, and the certificate itself in the
   commit queue — and an own batch is "uncommitted" until its block is
   executed, not until it is certified. If proposal continues while execution
   lags, that memory grows without bound and the own store's refusal
   (invariant 4) reports a backlog that no amount of waiting by the
   submitter can drain. So a validator whose executor is more than
   **`MaxExecutionLag` = 8 blocks** behind the DAG's last commit proposes
   headers with **no user work** — parents, weak links and batches of system
   traffic alone; rounds continue, liveness is kept, no user transaction is
   certified — and refuses user work until execution catches up. System
   traffic still goes through, because a lagging executor is often waiting on
   it: a Directory anchor below quorum is completed only by copies that are
   ordered (#4517; "Execution lag"). The bound is a few seconds of traffic and is not a
   buffer to be made bigger. When execution catches up, the batches that
   built up meanwhile come back **a header at a time**: a header carries at
   most `MaxHeaderBytes` of batches and the rest wait for the next. Draining
   the backlog into one header made one block the executor took ten to
   seventeen seconds over, which re-crossed the bound and refused user work
   again — an oscillation, not a recovery (run `20260905T144928Z`).
   **The bound is per block, not per header.** A block executes every
   validator's header over two rounds, so a per-header cap alone lets N
   validators carry N × 2 × `MaxHeaderBytes` in one block — 4 MiB for a
   Directory of eight. Each header therefore gets its share of a per-block
   budget, `MaxBlockBytes / (2 × validators)`, capped by `MaxHeaderBytes`; a
   block is bounded whatever the number of validators.
   **The lag counts commits the executor has not executed, and a joining
   node's commits are executed by its handoff or by its state.** While a
   node joins it executes nothing, so it reads as lagging and proposes no
   user work (#4292). At the handoff the groups it collected are either
   produced — each reported executed as it is — or already held by the
   pulled state, executed by the peers whose state it is: those count as
   executed at the handoff too. Dropped uncounted, every validator restarted
   on a loaded partition read as behind by them for ever and refused user
   work for good — 125 blocks on BVN1, 50 on the Directory, run
   `20261008T170913Z` (#4553).
10. **A refusal says why.** Refusing for a full own store (consensus is not
   committing this validator's batches) and refusing for execution lag (commits
   are fine, the executor is behind) are the same `NotReady` to the submitter
   and different facts to the operator. Each is reported separately: a reason
   in the error, and a gauge per reason.

11. **A vote deferred is a vote asked for.** A validator votes on a header only
    when it holds everything that header names: every parent certificate and
    every batch. When something is missing it must ASK for it, not merely
    wait, and it asks for everything it lacks at once: a header authored
    after a restart names every batch its node accumulated while it was
    down, hundreds of them, and a voter that asked for one per rebroadcast
    took minutes to certify it (#4503). The author rebroadcasts its header about once a second and each
    rebroadcast re-tests the same condition, so anything broadcast once and
    never re-sent — a certificate, a batch — leaves a condition that cannot
    become true on its own. The cost of getting this wrong is a whole
    partition: every validator sits on the round's headers refusing to vote,
    no header reaches the quorum the round needs, the round never advances,
    and the partition stops while its peers run on. Both halves have now cost
    exactly that — batches in #4159, parent certificates in #4182.

## 2. Specification — how it is implemented

`pkg/consensus/worker` (the worker, its pending list, active store and
retention), `pkg/consensus/gossip/batch_queue.go` (the inbound queue),
`pkg/consensus/consensus.go` (how budgets are divided among workers).

### Budgets

Per partition, per node:

| store | bytes | how divided |
|---|---|---|
| active store | `DefaultMaxStoredBatchBytes` = 32 MB | `perWorkerBytes`: budget / workers, floor 2 × `MaxBatchBytes` |
| retention | `DefaultMaxRetainedBatchBytes` = 32 MB | the same |
| inbound queue | `DefaultMaxInboundBatchBytes` = 32 MB | one queue per partition |
| pending | `MaxPendingSize` = 10 MB, `MaxPendingCount` = 10,000 | per worker |
| a block's batches | `DefaultMaxBlockBytes` = 1 MiB | `headerBudget`: budget / (2 × validators), capped by `DefaultMaxHeaderBytes` = 256 KiB, floor one batch |

A node running a Directory and a BVN validator holds two of each. The
per-worker share must include the wire buffer a stored batch aliases
(`types.UnmarshalBatch` takes ownership of the pubsub message), because that
is the memory actually resident.

### Sealing

`Worker.Submit` appends to pending and signals a seal when pending reaches
`BatchSize` (500) or `MaxBatchBytes` (500 KB); `batchLoop` also seals on a
ticker of `BatchTimeout`, which is the latency floor for a quiet worker,
one second (`DefaultBatchTimeout`, the block interval). Generated node
configurations run one worker per node (`cmd_init_network.go`): at four, a
partition ran sixteen seal timers and emitted ~160 one-transaction batches a
second at 250 tps; at one, four workers at ~60 tps each seal batches of tens
of transactions a few times a second.

### The active store and eviction

`StoreBatch` adds a batch and, when the peer cache exceeds its byte budget,
runs an LRU eviction over peers' batches that skips pinned ones (those a
pending vote needs) and those named by a certificate consensus has committed
and the executor has not yet executed — marked when the group is handed to
the executor, released when `PruneCommitted` retires them (or the executor
skips a re-delivered certificate); own batches are counted against their own
share (invariant 8). A follower casts no vote, so the second mark is the
only thing that keeps its backlog: without it, a follower 300 blocks behind
evicted a committed batch the validators had already retired and waited on
it for good (Docker run `20261007T050444Z`, bvn2-fol1). A node so far behind
that its backlog outgrows memory is past the retention window and resyncs
(invariant 6). There is no count limit by default: `MaxStoredBatches`
and `MaxRetainedBatches` are zero, and only a test sets them. A negative
`MaxRetainedBatches` turns retention off.

### Refusal and back-pressure

A worker has two entry points. `Submit` is for the system's own traffic —
synthetics, anchors, the healer's re-submissions — and never refuses for
lack of room, because that traffic is what drains the store (#4165).
`SubmitUser` is for a user's transaction from the API: while own uncommitted
batches plus pending transactions exceed the worker's own share
(`maxOwnBytes`, separate from the peer cache's `maxStoredBytes`, invariant 8)
it returns `ErrStoreFull`, which `SubmitterService.Submit` returns as `NotReady`
(invariant 4). The API decides which by the envelope: a synthetic, sequenced,
anchor, network-update or proof message anywhere in it makes it system
traffic (`isUserEnvelope`). The load generator honours `NotReady` with a
back-off as it honours the query gate. The condition is
`accumulate_dagbft_batch_store_refusing{partition,worker}`, the store's own
and peer bytes are `accumulate_dagbft_batch_store_bytes{kind}`, and the
over-limit state is logged on transition only (invariant 5); the eviction
summary is logged at most once a second per worker.

The inbound queue already applies back-pressure at its byte budget (it drops
the newest batch and lets the author re-broadcast); that is the model.

**A validation refusal is answered as the refusal.** An envelope the
executor's pre-batch validation refuses (`ErrValidationFailed`) is not
back-pressure and not `Pending`: `SubmitterService.Submit` answers it with
`Success: false` and a status carrying the executor's code and message, so
`Status.AsError()` is the refusal. An error with no code is not a verdict on
the envelope but the node failing to reach one (the executor's own `Validate`
error), and is answered `InternalError`: the dispatcher retries it, and no
sender is told a good envelope is bad. A sender that reads the status sees it;
the dispatcher settles it as refused, not sent (executor.md, "Dispatch").
What an accepted submission returns is unchanged. Answered as `Pending` with
no error, a refused heal was counted sent and believed landed (#4426).

### Re-proposal

`ReproposeAfter` re-broadcasts an own batch that has waited without a
certificate. It asks the DAG, not the executor: `Config.Certified` is wired to
`DAG.HasCertifiedBatch`, which indexes every batch digest a certified header
names (pruned with the rounds), and `staleOwnBatches` skips any batch it
reports (invariant 7). The executor's `PruneCommitted` is the wrong signal,
because execution can lag certification by minutes when blocks are slow.

### Execution lag

The node counts the leader groups Bullshark hands to the executor's channel
and the blocks the executor has produced from them (`Node.ReportExecuted`,
called by the block production loop after each block); their difference is
the **execution lag** in blocks (`Node.ExecutionLag`,
`accumulate_dagbft_execution_lag_blocks{partition}`). The header builder asks
before every header (`Primary.executionLagging`); when the lag exceeds
`MaxExecutionLag` (8 blocks, `primary.DefaultMaxExecutionLag`, config
`max_execution_lag`) the header takes no batch that holds a user's
transaction — it still carries its parents and weak links so rounds and the
DAG advance, those batches stay queued for a later header, and batches of
system traffic alone are still proposed (below) —
and every worker enters refusal with reason `execution-lagging`
(`Worker.SetExecutionLagging`, `ErrExecutionLagging`, `NotReady` to the
submitter). Both clear when the lag falls back under the bound. The reasons
are separate gauges, `accumulate_dagbft_batch_store_refusing{partition,worker,reason}`
with reason `store-full` or `execution-lagging`, and separate transition log
lines; `accumulate_dagbft_execution_lagging{partition}` is the state. The
commit channel's depth is then a consequence of the bound plus the DAG's GC
depth, not a buffer: it never holds more than the bound allows.

**System traffic is proposed while execution lags** (#4517). A lagging
partition's executor is often waiting on exactly that traffic: a Directory
anchor below quorum holds every collection proof and every receipt behind
it, and its quorum is counted from executed copies. Proposing nothing while
lagging therefore kept the copies that would end the lag from ever being
ordered. In run `20261003T232435Z` every Directory validator sent its copy of
each anchor after 1,980 to BVN2, BVN2 lagged 13 to 75 blocks, and no anchor
after 1,980 collected a quorum. So a worker marks each batch it seals by
whether it holds a user's transaction (one taken by `SubmitUser`). While
lagging, the header takes batches with no user transaction, within its byte
budget, and leaves the others queued. User work is refused at the worker
meanwhile, so the batches sealed during the lag are all system traffic.
System traffic is bounded by what the other partitions produce, not by
users, so the bound on how far consensus outruns execution still holds for
the work it exists to hold back.

### The DAG facts the rest of the specification relies on

The rounds, votes and certificates of DAG-BFT are still to be written. Until
they are, these are the facts the executor and healing parts depend on:

- **The leader of a block** is the author of the leader certificate whose
  causal history the block executes. Exactly one validator is the leader of a
  block, and duties assigned to "the leader" — dispatching the block's
  synthetics — are that validator's alone.
- **Canonical payload order.** A block executes the batches of its committed
  certificates in the certificates' canonical order and each batch's
  transactions in order. Any node-local order diverges state.
- **A committed certificate's batches are fetched, never skipped.** A validator
  that lacks a batch a committed certificate names fetches it from a peer and
  waits; after `BatchCollectTimeout` the node halts rather than execute a
  block its peers executed differently. Retention is what lets a peer serve
  it (invariant 6).
- **Re-delivery is idempotent.** A certificate delivered to the executor twice
  executes once; the second delivery is recognised and skipped, and its
  batches are not pruned a second time.
- **Weak links.** A header references the certificates of the previous round
  and, as weak links, recent older certificates it has not yet referenced, so
  a certificate that arrived late still enters a committed leader's causal
  history. A certificate no header ever references is never executed and its
  batches are lost; the weak-link window is what makes that impossible in
  practice, and the count of orphaned certificates is a metric.

### DAG retention

The DAG keeps `DAGGCDepth` (2,000) rounds of certificates behind **whichever
is further ahead, the last commit or the latest round**. Collection runs from
the commit path, as before, and from the round advance: when a certificate
for a new round is inserted, rounds more than `DAGGCDepth` behind it are
collected. A node whose executor has halted therefore does not grow its DAG
while the network goes on — the rounds it never committed are collected once
the frontier is more than the depth ahead of its last commit, counted
(`accumulate_dagbft_dag_uncommitted_rounds_dropped_total`) and reported once,
because such a node is stranded, not lagging: the batches those rounds name
are outside every peer's retention, so it could not have executed them
(invariant 6, E11). A healthy node commits within a few rounds of the
frontier, and the round advance collects nothing the commit path would not.
Collection walks only the rounds the cutoff moved over, never the whole DAG.

### Certificate catch-up

Certificates are broadcast once. A node that missed some — one that fell
behind, or one that joined and was seeded at a round below the frontier
(Restart, `Node.Rejoin`) — pulls them from its peers over the partition's
certificate-sync topic. There are two kinds of request, and both are
broadcast:

- **By round.** A header more than one round ahead of the node's round asks
  for every certificate of the rounds from one below the node's round
  upward, at most 64 rounds, at most once a second
  (`requestRoundCatchUp`). A node whose round has not advanced for 5 s
  re-shares the certificates of its current and previous round and asks for
  the rounds either side of its own, at most every 2 s (`recoverFromStall`).
- **By digest.** A certificate whose parents the node lacks asks for those
  parents, and so does a header it will not vote on for the same reason.
  Digest requests are batched, deduplicated for 5 s and retried up to ten
  times; the parents of every certificate held waiting are asked for again
  every 15 s. One request names at most 100 digests — what one response
  carries — and a longer list goes as several requests: a peer leaves what
  does not fit unanswered, and it would wait out the 30 s retry (#4469).

A peer answers after a random delay of up to 200 ms with at most 100
certificates, and does not answer a request it has already seen answered.
A request is named by its requester's key and its ID together, and a
response carries both: every node numbers its own requests, so an ID alone
named a request of every node at once, and a peer that had seen node A's
request *k* answered skipped node B's request *k*. A node numbers its
requests from a random starting point, so one restarted under the same key
does not reuse the IDs its peers saw answered before it stopped (#4470).
What arrives is verified against the committee and inserted. A
certificate whose parents are missing is held (up to `2 × 10 × committee`
certificates) until they arrive, and its parents are asked for.

**A held certificate is released whenever its parents are in the DAG,
whoever inserted them** (#4515). A copy of a held certificate is reported
held without being retried, so a release that never comes is permanent:
nothing above that certificate is inserted again. Three goroutines insert
into the DAG: the gossip loop, the sync-response goroutine, and the one that
inserts each certificate this node forms. So:

- **Inserting this node's own certificate releases** what waits for it. Its
  peers' certificates of the next round name it and often arrive first.
- **A copy of a certificate already in the DAG releases** what waits for it.
- **A certificate just held releases itself** if one of the parents it was
  held for was inserted between its failed insert and being held.
- **Once a second, every held certificate whose parents are in the DAG is
  released**, the backstop that makes any missed release temporary.

Before this, a Directory validator restarted alone after a 6 s stop held
every certificate it was sent for eleven minutes (`verified=0`) while its
peers answered every request (run `20261003T191744Z`). consim
`TestRestartOneDirectoryValidatorOfTwelve` wedged 3 times in 29 with 240
held, one waiting only on parents already in the DAG and the rest waiting on
it. It ran 45 times without a wedge after the fix. A wedged Directory
validator does not vote. Two restored Directory validators that did not vote
took the Directory below its quorum in run `20261002T145702Z`, probably by
this defect (#4511, not confirmed from that run's logs).

**A certificate the buffer will refuse is refused before it is verified.**
Verification is an ed25519 check per signature, and a certificate already
in the DAG is recognised before it. A certificate that is held, or that the
full buffer refuses, is never in the DAG, so before this check every copy of
it was verified again. Copies are many: gossip delivers each certificate,
every peer that answers a sync request sends its own response, and every
node receives every response, not just answers to its own requests.
Responses wait in a queue of 500 per partition (`gossip.go`), read one at a
time; when it is full, a new response is dropped. A joining node with its
buffer full of frontier certificates spent its time verifying refused
copies. Its queue filled and the responses carrying the parents it needed
were dropped, though peers had answered every request. Under `-race`, consim
`TestFollowerJoinCatchesUp` failed 10 runs in 10 this way (#4469). So a
certificate the buffer already holds is reported held, and one with
missing parents that the full buffer would refuse, above the round after the
node's own, is reported dropped, both without verification. A certificate
the node needs (at or below the round after its own) still takes the full
path: it is verified, and its parents are asked for. Nothing enters the DAG
or the buffer unverified. `accumulate_dagbft_cert_verifications_total` and
the `verified` field of the `Certificate catch-up` line count the
verifications.

**A certificate the node needs to advance is never refused without asking
for its parents.** When the buffer is full, a certificate at or below the
round after the node's own whose parents are missing is not held, but its
parents are still asked for; catch-up re-delivers it, and it is inserted
once they are in. Certificates further ahead are left to round catch-up.
This is the common case for a node seeded by `Rejoin` at a round `P` (the
seed, `S` in "Restart"), not a corner:
the DAG accepts certificates at `P` and `P + 1` without their parents, but a
header names as weak links every certificate of the rounds from eight below
it to two below it, so the certificates at `P + 2` name those of
`P − 6 … P − 1`, which no round request asks for (round catch-up starts one
round below the node's own). And a joining node's buffer is already full:
until `Rejoin` its floor is zero, so every live certificate gossip delivered
while it pulled its state waits for parents, and none is pruned, being above
its round. Refusing without asking left a follower's Directory at `P + 2`
for four minutes, released only when a rescan happened to ask for the right
parents (Docker run `20260929T152146Z`; consim `TestFollowerJoinCatchesUp`,
#4469).

What catch-up did is reported, per partition, in
`accumulate_dagbft_cert_sync_requests_sent_total{kind}` (round, digest),
`…_requests_served_total{outcome}` (answered, suppressed, empty),
`…_responses_received_total` and `…_certificates_received_total{outcome}`
(inserted, duplicate, buffered, dropped, invalid, equivocation, failed),
and in one Info line, `Certificate catch-up`, at most every 10 s and only
when something crossed the path: round requests and the span they asked
for, digest requests, responses, certificates by outcome and the last
refusal, and the requests this node served or suppressed. The line is what
tells "nobody answered" (requests, no responses) from "everybody answered
and the node refused" (responses, certificates dropped) — a follower's
Directory that sat at its seed round for four minutes could not tell the
two apart with every line at Debug and the counters never incremented
(#4469).

### The committed log

The DAG's committed leader groups are the network's ordered record of what
to execute: each group is the certificates Bullshark committed and the
batches they name, in execution order, and execution is deterministic. **That
record, on disk, is the durability point** (database.md, invariant 5). Before
a committed group is handed to the executor, the node appends it to a
per-partition log file — one record per block, the certificates and the
batches' transactions as they were committed — and syncs the file once. The
commit of the block's state to the store then returns without waiting for the
store's seal.

The log is deleted from the front, never rewritten: an entry is removed once
the store has sealed the block it produced (the seal watermark) **and** the
block is older than what a peer may still ask this node for (the batch
retention below). It therefore grows only when the seal falls behind, and
its length is the replay a restart needs. It carries nothing the DAG does not
already hold in memory; it is the same data, made durable.

### Restart

A validator's consensus position is **checkpointed per block, before the
block is produced**: the primary's round and epoch, Bullshark's last committed
leader round, its per-author watermarks and its committed-digest set (the
certificates within the rescue window already committed — without it the
first leader after a restart re-commits every ancestor in the window and the
node executes a block its peers did not, #4290), and the block index the
position belongs to (`persist.Checkpoint`, `Service.saveCheckpoint`, two files under
the node's `consensus/<partition>/` directory — the position for the block
about to be produced and the one before it). On restart the node restores the
checkpoint whose block is the executor's last block
(`Service.seedFromCheckpoint`, `Node.Restore`), so it participates in
consensus from that round rather than from zero.

**The executor keeps no empty block**, so its last block is the last one that
carried anything, and a partition whose last blocks were empty restarts with
both checkpoints above it (Docker, 2026-09-28: every BVN validator with its
executor at 12214 and checkpoints for 12216 and 12217, while the busy
Directory matched; #4448). The older checkpoint is then the position to
resume: the newer one was written only after the older one's block was
produced, so every block through the older one's was produced and those above
the executor's last block carried nothing. The node restores it and numbers
its next block after that checkpoint's block, as every peer that did not
restart numbers it (`persist.ResumeFrom`). The newer checkpoint is never
resumed from while its block is above the executor's: that block may not have
been produced, and resuming there would skip its group. This rests on a
produced block being durable once production returns, which the exact match
already assumes; a store that loses a non-empty block that was produced is
resumed past it (DIFFERENCES, C10).

**A store at genesis resumes from no checkpoint** (#4484). The empty-blocks
resume reads the blocks between the executor's last block and the checkpoint
as blocks this store kept nothing of. A store at the genesis block has kept
no block of its own, so the blocks below a checkpoint above it are blocks it
never had: its store was replaced — wiped, or created fresh beside an old
`consensus/` directory — and the checkpoint is the old store's position
(Docker run `20260929T152146Z`, three followers with `lastBlock=1` resumed
after block 107048). Resumed, the node would number its next block after the
checkpoint's, and the whole-restart exit (`Service.Resume`, #4447), which
refuses only a node with no restored position, would execute that block on
the genesis state. So `seedFromCheckpoint` restores nothing when the
executor's last block is at or below `GenesisBlock` and no checkpoint is for
that block, and says so: the node is in the position of one that has no
checkpoint. A joining node orders nothing until its join names a round (below);
one that does not join starts at round zero, as the first node of a network
does (executor.md, "Sync", step 5). A store restored from a backup older than
the checkpoint is not told apart from a run of empty blocks (DIFFERENCES,
C11).

The position is the one **of the block being produced**, not Bullshark's
position now (`Node.CheckpointAt(leaderRound)`): groups Bullshark has ordered
since are still in the executor's queue, so the checkpoint's last commit is
that block's leader round and its committed-digest set leaves out the
certificates of the groups ordered after it. A restart from Bullshark's
position now would never be handed those groups.

**A restart reloads the DAG it stopped with** (#4448). The DAG lives in
memory, and a header needs a quorum of the previous round's certificates as
parents: a partition restarted as a whole otherwise holds no parent anywhere
and no node authors again (Docker, 12 validators restored at block 40060,
round 80129, `certificates_created_total` 0). So beside the checkpoint, in
`consensus-dag/` (`persist.DAGStore`, one file per certificate or batch,
named by digest, written once):

- **With each checkpoint**, every certificate in the DAG from `RescueWindow`
  below the checkpoint's last commit up to the frontier, and the batches they
  name that the node holds. The window is what ordering walks
  (`orderDag`), so the restored node orders the certificates above the floor
  exactly as its peers do; the batches are what those groups execute, and
  after a whole restart no memory holds them. The checkpoint lists them
  (`Checkpoint.Tail`), with the batches of the header this node authored
  last: authored after the rounds above and not yet certified, no
  certificate names them, and a restart that handed the header back without
  them left a round no vote could certify and its author could not author
  again (#4502). Files neither this checkpoint nor the previous one lists
  are removed, except those written after the checkpoint began. The write
  cost per block is the certificates and batches the block adds.
- **Before each header this node authors is broadcast**, the header itself
  and the DAG from four rounds below it up, with the batches held. The
  executor, and so the checkpoint, can be many rounds behind consensus when
  the node stops; those rounds are on disk only because authoring wrote them.

On restart, before the primary starts, `Node.Restore` loads every persisted
certificate into the DAG without parent checks (`DAG.Restore`: each was
quorum-verified when first taken in, and the ones below the window are
committed history), every persisted batch into the batch store — the ones
named under this node's key, by its certificates in the tail or by its last
authored header, as its **own** (invariant 4: never evicted; the tail is a
whole rescue window and can be larger than the active store, and only the
author can serve what its header names, #4545), the rest as peer copies —
and runs the certificates through Bullshark lowest round first. The last authored header
goes back to the primary (`Primary.RestoreAuthored`): the primary never
authors that round or a lower one again, its round is raised to that
header's if it had got further than the checkpoint, and if the header's
certificate is not in the DAG the header rejoins vote collection so the
ordinary rebroadcast sends **the same header** again — a second header for a
round is self-equivocation (#4159 stall 3). The authored header is read back
on **every** start, checkpoint or not (`NewNode`): a start that resumes no
checkpoint — none for the executor's last block, or a store at genesis —
otherwise knew nothing of the round and signed a second header for it
(#4592). Its file and its directory are synced before the header is
broadcast, so a power loss after the broadcast cannot bring the previous
header back; the certificates and batches written beside it are not, since
losing them costs a fetch, not an equivocation. A checkpoint written before
#4448 has no tail and no authored header, and restores the position alone
(DIFFERENCES, C9).

**A joining node with no checkpoint orders nothing until the join names a
round** (#4405). Round zero is not a position: a network past `DAGGCDepth`
has collected those certificates, and every peer retired their batches as it
executed them, so a node ordering from zero waits for the first group for
ever (Docker run `20260925T011332Z`, `fol2-stall`: a fresh follower waiting on
the batches of a round-4 certificate while the network was past block 640).
Such a node starts with ordering held (`Node.HoldOrdering`): certificates
reach its primary and none is ordered. The join's first proven state names
the round (`Service.StageThrough` reads the leader round `P` its system
ledger records), and consensus is seeded **thirty-four rounds below it**, at
`S = P − (RescueWindow + 2)` (`Node.Rejoin(S)`, `seedBelow`; #4530):
Bullshark orders nothing at or below `S`, the DAG accepts certificates at `S`
and `S + 1` without their parents, and the primary participates from `S`.
`P` is the round of a leader the network committed, recorded in a state the
join itself pulled seconds earlier, so `S` is near the frontier: the
certificates above it are in every peer's DAG and their batches are still
served. A peer's current round or the first certificates to arrive would not
be — certificate catch-up delivers low rounds first.

`S` need not be a committed leader's round, and usually is not. The leader
chain above a seed is fixed by the certificates above it, whatever kind of
round the seed is (`bullshark`,
`TestSeededBelowAStateOrdersThePeersGroupsAboveIt`: six DAGs built as the
primary builds headers, with missed rounds, late certificates and skipped
leaders, every committed leader's round as `P`, seeds 33 to 52 rounds below).
What a seed lacks is the committed-digest set: the first leaders ordered
after it walk the rescue window below it and may take in certificates the
peers committed before it. So no state at a round at or below
`S + RescueWindow` (32) is handed off at (`Conflict`).

**The first proven state is two rounds above that floor, and is the state
the node hands off at.** Until #4530 the seed was `P` itself, the first state
was at its own floor, its handoff was always refused, and every join pulled
another round to a state above it: 16 to 55 s for each partition of each of
three followers in Docker run `20261006T044611Z`. Seeded below `P`, the node
is refused at `P` only as not having collected that far yet (`NotReady`), for
the seconds consensus takes to order thirty-four rounds; the join asks about
the same state again and does not pull on (executor.md, "Sync", step 5).

**A seed forgets what the primary held before it** (`Primary.ForgetHeld`).
Until the seed the DAG's floor is zero, so every certificate gossip delivered
while the node pulled was held for want of parents. The seed makes those at
`S` and `S + 1` acceptable without parents; left held, each stayed waiting
for parents nobody would send, and every later copy of it was refused as
already held. Seeded at `P` a node never met one, because what it held was
older than the seed; seeded below `P` on a join that took seconds, the node
held exactly the rounds it was seeded at, its DAG stayed empty and it ordered
nothing (the libp2p join test, three runs of four). Forgotten, each is judged
again when catch-up next delivers it.

Two networks are seeded at `P` itself, as before, and pay the refused first
handoff: one younger than thirty-four rounds, and one whose `DAGGCDepth` is
below `seedBelowDepth` (eight times the distance, 272 rounds). The first
proven state is itself behind the frontier — up to 190 rounds in Docker run
`20261006T062709Z` — and a seed below it must still be inside what a peer
holds, or no peer can serve the rounds above it. A node restarted with a checkpoint more than
`DAGGCDepth` below the frontier is not seeded this way (DIFFERENCES,
"Seeding").

**A restored or rejoining node catches up before it takes part** (Paul,
2026-10-03: "What I think we need to do on boot is to slow down the whole
process where we focus on serializing steps that in a fully running
protocol does not require serialization"). After `Node.Restore` or
`Node.Rejoin` the primary is in **catch-up**, not live:

- **It authors nothing and votes on nothing.** Its last authored round is
  still never authored again (`RestoreAuthored`).
- **Gossiped certificates above the round after its own are not held.** A
  frontier certificate only tells the node how far the frontier is. A node
  far behind could otherwise fill its pending buffer with certificates it
  cannot prune, since they are above its round, and refuse the ones it needs.
- **One goroutine catches up in round order.** It asks for whole rounds from
  the highest round at which its DAG holds a quorum of certificates, inserts
  each round's certificates parents-first, asks by digest for any parent a
  round still lacks, and moves to the next round only when the one before is
  in. No other path inserts into the DAG while it runs.
- **It goes live within one round of the frontier.** That is the highest
  round it has seen certified. From then on it is the protocol above, with
  gossip, authoring and voting. A frontier more than `DAGGCDepth` rounds
  ahead cannot be caught up from peers, and the node rejoins from a pulled
  state as a node with no checkpoint does.

A running node a round or two behind the frontier needs none of this. The
round catch-up above serves it. Catch-up removes the burst a restart meets: a restored Directory
node dropped 197 votes in its first second (run `20261003T191744Z`). It also
removes the frontier filling the buffer. It was not what fixed #4515, which
was a release missed in the live protocol too (Certificate catch-up). It is
built only if the Docker proof of that fix (PLAN R8-1) still shows restart
catch-up slow or bursty (DIFFERENCES, C15).

**What it does not do is execute from there.** A restart is a join
(executor.md, "Sync"): the node keeps every committed block in a buffer
instead of executing it, pulls the state a signed anchor proves, takes the
buffered blocks through the one after that state's block into its own
staging, and produces the buffered blocks after the state from there; the
blocks after the first reach staging only by being executed (#4398). No peer
is asked for its staging. Which buffered group is the block after the state is
decided by the leader round the pulled system ledger records for that block
(`SystemLedger.LeaderRound`), never by counting from where the node stood
(#4362). The checkpoint decides where the node stands in the DAG; the join
decides what it executes. The round it restores is also what the join's
handoff measures the pulled state against: consensus delivers the groups
committed after it, so a state committed at a round below it would need
groups the node will never be given, and the join pulls a newer one
(executor.md, "Sync", step 5). A handoff whose block cannot be produced
returns the node to collecting and the join tries again, counted (#4401). A
node that executed the blocks between its last one and the network's — by
replaying the committed log, or by catching up certificate by certificate —
would execute them from a staging its peers do not have, and the root chain
is a Merkle root over the history of block roots, so it would never match
again (#4290).

There is no fallback to executing from the node's own last block. A node
that can find no validator of its partition keeps collecting and executes
nothing: it cannot know what its peers executed, and executing from its own
state is the divergence the join exists to prevent (#4296). A node that has
executed no block — genesis is not an execution — does not join at all
(executor.md, "Sync", step 5; #4304).

### Retention

`retain` keeps executed batches up to `DefaultMaxRetainedBatchBytes` and
`RetainCommittedFor`; there is no count. A peer that asks for a batch outside
the window is told so (`absence=no-record`) and must resync (#4205, E11).
The committed log keeps a block's batches at least until the store has
sealed that block, whatever the byte budget says: retention may forget a
batch a peer wants, never one this node still needs to replay.

---

Where the implementation departs from this specification, see
[DIFFERENCES.md](DIFFERENCES.md).

**A joiner collects no votes for decided rounds (Paul, 2026-10-02; #4476).**
"We can't expect a joining node to collect all the votes for blocks that have
already met consensus." Every block records the leader round it was committed
at (the system ledger), so a node whose state is proven at block V, committed
at round R, holds the result of every round at or below R: a certificate for
such a round is dropped on arrival, unverified, and is never requested. A
joiner keeps applying block-ledger records until its proven block is the
newest, and only then seeds consensus, thirty-four rounds below that block's
round (Restart; #4530), so the rounds it must collect are the frontier's last
few and those thirty-four. The pending buffer's size is not the
question ("240 is nothing, it is just we don't need even that"); it stays as
is.
