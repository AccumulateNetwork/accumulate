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
   **empty** headers — parents and weak links, no batches; rounds continue,
   liveness is kept, nothing new is certified — and refuses user work until
   execution catches up. The bound is a few seconds of traffic and is not a
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
10. **A refusal says why.** Refusing for a full own store (consensus is not
   committing this validator's batches) and refusing for execution lag (commits
   are fine, the executor is behind) are the same `NotReady` to the submitter
   and different facts to the operator. Each is reported separately: a reason
   in the error, and a gauge per reason.

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
pending vote needs); own batches are counted against their own share
(invariant 8). There is no count limit by default: `MaxStoredBatches`
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
`max_execution_lag`) the header takes no batches from
`ConsumeAvailableBatches` — it still carries its parents and weak links so
rounds and the DAG advance, and the batches stay queued for a later header —
and every worker enters refusal with reason `execution-lagging`
(`Worker.SetExecutionLagging`, `ErrExecutionLagging`, `NotReady` to the
submitter). Both clear when the lag falls back under the bound. The reasons
are separate gauges, `accumulate_dagbft_batch_store_refusing{partition,worker,reason}`
with reason `store-full` or `execution-lagging`, and separate transition log
lines; `accumulate_dagbft_execution_lagging{partition}` is the state. The
commit channel's depth is then a consequence of the bound plus the DAG's GC
depth, not a buffer: it never holds more than the bound allows.

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
leader round and its per-author watermarks, and the block index the position
belongs to (`persist.Checkpoint`, `Service.saveCheckpoint`, two files under
the node's `consensus/<partition>/` directory — the position for the block
about to be produced and the one before it). On restart the node first
re-executes from the store's last sealed height through the committed log to
its head — the blocks whose state the seal had not yet made durable — then
restores the checkpoint whose block is the executor's last block
(`Service.seedFromCheckpoint`, `Node.Restore`), so the node participates from
that round and certificate catch-up bridges the gap to the live frontier,
within `DAGGCDepth`. A node with state but no matching checkpoint starts at
round zero, says so, and cannot catch a live network. What a restarted node
still lacks — the state and staging its peers hold, and the certificates
below its checkpoint that a later leader commits — is the sync mechanism
(E11, [DIFFERENCES.md](DIFFERENCES.md)).

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
