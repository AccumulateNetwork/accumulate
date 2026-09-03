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
   second, which is itself a resource.
6. **Retention is a window in seconds of traffic, bounded in bytes.** A peer
   that is further behind than the window cannot recover by fetching batches
   and must recover by snapshot (see the healing and fast-sync parts). The
   window is not made longer to cover that case.

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

A node running a Directory and a BVN validator holds two of each. The
per-worker share must include the wire buffer a stored batch aliases
(`types.UnmarshalBatch` takes ownership of the pubsub message), because that
is the memory actually resident.

### Sealing

`Worker.Submit` appends to pending and signals a seal when pending reaches
`BatchSize` (500) or `MaxBatchBytes` (500 KB); `batchLoop` also seals on a
ticker of `BatchTimeout`. Invariant 2 requires the timeout to be a floor on
latency for a quiet worker, not the normal seal trigger under load: with
`BatchTimeout` at 100 ms and sixteen workers in a partition at 250 tps, a
batch holds one or two transactions and the partition emits ~160 batches a
second. The timeout is to be set so that a worker at its share of the target
rate fills a batch before it fires, and the number of workers is a
parallelism decision that must not multiply the batch rate.

### The active store and eviction

`StoreBatch` adds a batch and, when the store exceeds its byte budget or
`MaxStoredBatches`, runs an LRU eviction that skips own uncommitted batches
and pinned batches (those a pending vote needs). Invariant 3 says the count
limit goes: with byte budgets and full batches the count is implied, and a
count that is small relative to the batch rate evicts what the next header
needs.

### Refusal and back-pressure

When own uncommitted batches alone exceed the worker's share, `Submit`
returns `NotReady` (invariant 4). The API's submit path returns it to the
client as the retry-later it already is for every internal caller, and the
load generator honours it as it honours the query gate. The condition is
exposed as a gauge per worker and logged on transition (invariant 5); the
eviction summary is logged at most once a second per worker.

The inbound queue already applies back-pressure at its byte budget (it drops
the newest batch and lets the author re-broadcast); that is the model.

### Retention

`retain` keeps executed batches up to `DefaultMaxRetainedBatchBytes`, and
`DefaultMaxRetainedBatches` (4,096) goes with the other count. A peer that
asks for a batch outside the window is told so (`absence=no-record`) and
must snapshot-sync (#4205).

---

Where the implementation departs from this specification, see
[DIFFERENCES.md](DIFFERENCES.md), entries C1–C3.
