# Consensus — Specification

**Scope of this part.** Section 1 is the batch plane: the memory it may
hold and what happens when it is full, written first because three soaks in
one day ended on it. Section 2 adds, under "The protocol", the DAG-BFT rules
themselves — rounds, headers, votes, certificates, insertion, leader
election, commit, order, epochs, retention, persistence and who may send —
each citing the code that implements it and the paper section it follows
or departs from (#4600). The parts of section 2 written earlier (catch-up,
retention, the committed log, restart) are the mechanisms those rules run
on.

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
    took minutes to certify it (#4503). **A deferred vote is re-tested when
    what it asked for lands**, not when the author next sends the header:
    the voter records, per deferred header, the batches and parent
    certificates it lacks, and the moment the last of them is stored or
    inserted it tests the header again (#4611). Waiting for the author cost
    every such header at least one rebroadcast after its batches had
    arrived, and the headers after a restart carry the backlog a header at
    a time: a partition went 7–20 s without a block after
    `TestRestartAfterTwoNodesStopFirst`'s restart, unloaded included — and
    what remained of those seconds once the vote was re-tested on arrival
    was the simulator's gossip, not the vote (Restart, below). The
    author still rebroadcasts its header about once a second, and each
    rebroadcast re-tests the same condition: that is the backstop for a
    re-test that never fires (a batch evicted again, a notification dropped),
    and it is why anything broadcast once and never re-sent — a certificate,
    a batch — leaves a condition that cannot become true on its own. The cost of getting this wrong is a whole
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

### The protocol: rounds, headers, votes, certificates, commit

This section is the DAG-BFT protocol as this network runs it: Narwhal's
mempool (arXiv 2105.11827) for availability and Bullshark's partially
synchronous commit rule (arXiv 2201.05677) for order, with the departures
this network makes from each and why. Every rule names the code that
implements it at `7d9458e10` (paths under `pkg/consensus/`, line numbers of
that commit) or the issue that will, and the paper section it follows or
departs from. `7d9458e10` is the one pin for every line number in this
section: the branch sits above it on dagbft-integration, and what the base
has changed since (#4604, #4610, #4611) is named by issue where it touches a
rule, never by line. Paper sections are numbered as the review that found
the gaps numbers them (#4601): Narwhal §3 is the model, §4 the core design (§4.1 the
mempool protocol, §4.3 garbage collection), §6 reconfiguration; Bullshark §4
is the DAG construction and its safety argument, §5.2 the ordering of the
DAG, §6 the eventually synchronous protocol, §7 garbage collection. What
the code does not yet do is in [DIFFERENCES.md](DIFFERENCES.md), C18 to C23.

**The objects.** A **header** (`types.Header`, `types/header.go:40-73`) is
one validator's proposal for one round: its author's key, the round, the
epoch, the author's clock, the batches it proposes (`Payload`), the
certificates it builds on (`Parents`), and the author's signature over the
digest of all of that. A **vote** (`types/vote.go:19-30`) is one validator's
signature over a header's digest, round and epoch. A **certificate**
(`types/certificate.go:46-62`) is a header with a stake quorum of votes: the
proof that a quorum of validators hold the header and every batch it names
(Narwhal §4.1, the certificate of availability). The **DAG** is the
certificates a node holds, by round and author (`dag/dag.go:45-74`). A
**committee** is the validators of a partition with their stakes, in a fixed
order, for one **epoch** (`types/committee.go:26-48`).

**The thresholds.** All thresholds are in **stake**, never in count. The
**quorum** is at least ⌈2S/3⌉ of the committee's total stake S
(`Committee.QuorumThreshold`, `types/committee.go:76-85`, `HasQuorum`
:130-133); the **validity threshold** is more than a third, ⌊S/3⌋+1
(`ValidityThreshold`, :87-93). Two quorums intersect in at least a third of
the stake, more than the faulty share can be, so they share an honest
validator: that is what every rule below rests on. `QuorumCount`
(:203-216) is a count that assumes equal stakes and is not a threshold of
the protocol; where the code still uses one, DIFFERENCES C19 says so.

#### Rounds

- **Round 0 is genesis.** Every validator of the genesis committee has a
  round-0 certificate: a header with no payload, no parents and timestamp
  zero, signed by its author and voted by every genesis validator
  (`genesis/genesis.go:39-45`, `:103-137`). Genesis certificates enter the
  DAG by `InsertGenesis`, which takes no parents and refuses a second
  certificate for an author (`dag/dag.go:207-241`). A network with no
  genesis certificates lets round 1 be authored with no parents
  (`primary/header_builder.go:149-154`); that path is a bootstrap for tests,
  not a rule a running network uses. The genesis epoch is 1 (`genesis.go:69`).
- **A node is at round r until its DAG holds a stake quorum of round-r
  certificates; then it is at r+1** (`Primary.tryAdvanceRound`,
  `primary/certificate_handler.go:272-341`; the quorum is `DAG.HasQuorum`,
  `dag/dag.go:286-303`, stake of the committee's members with a certificate
  in the round). Narwhal §4.1: a validator moves to round r+1 once it has
  2f+1 certificates of round r. The check runs when a certificate is
  inserted and on a 50 ms tick (`primary/primary.go:32-33`, `:444-445`).
- **Pacing.** At the frontier a node advances at most once per
  `MinRoundInterval`, 100 ms (`primary/primary.go:41-43`,
  `certificate_handler.go:298-318`), so consensus does not outrun execution
  (invariant 9 bounds it in blocks; this bounds it in time). Before it
  authors, a node waits up to 80 ms for the certificates of every validator
  of the previous round, not only a quorum (`DefaultParentWaitTimeout`,
  `primary/header_builder.go:18-22`, `:200-245`): a certificate no header
  names in its round can only be reached by a weak link later, and the
  commit rate depends on leaders being named promptly.
- **Catch-up.** When the DAG already holds certificates above the node's
  round, the node is behind and the pacing does not apply: it advances
  round by round as far as the quorums already in its DAG go, and authors
  no header for the rounds it passes through, since a header for a round
  the network has finished gathers no votes (`certificate_handler.go:298-340`).
  A header more than one round ahead of the node makes it ask its peers
  for the missing rounds ("Certificate catch-up"); a header more than
  `DAGGCDepth` rounds ahead means the node is stranded and needs a join
  ("Retention" below; `primary/vote_handler.go:339-353`, `:581-602`). A
  node that restores a checkpoint or is seeded by a join is in a catch-up
  mode of its own until it is within a round of the frontier ("Restart",
  "A restored or rejoining node catches up before it takes part";
  DIFFERENCES C15).

#### Header validity

A header is valid, and a validator may vote for it, when all of these hold.
Narwhal §4.1 for the first two and Bullshark §4 for the third; the rest
are this network's.

- **Its author is a member of the committee of its epoch**, and the
  signature is that member's over the header digest (`Header.Verify`,
  `types/header.go:227-243`). For a header the signature is checked first
  and then membership (`primary/vote_handler.go:278`, `:286`); membership
  comes first only for votes (`:27-47`). !1266 (#4596) moves membership
  before the signature for headers too, inside the topic validator ("Who
  may send consensus messages"). The epoch a header names is the epoch of
  the committee that orders its round ("Epochs" below).
- **Its strong parents are distinct certificates of round r−1 whose
  authors' stake is a quorum of the committee of round r−1.** The parent
  list is a flat list of certificate digests (`types/header.go:62-63`); the
  entries of round r−1 are the strong parents. Narwhal §4.1: a header
  references at least 2f+1 certificates of the previous round; Bullshark
  §4: every vertex has 2f+1 strong edges to the round before, which is
  what gives every later leader a path to a committed one. The builder
  names **every** round r−1 certificate it holds (`header_builder.go:166-173`),
  not a bare quorum, so late certificates are named in their own round
  where they can. The rule is one check, `DAG.CheckParents`, applied by
  the voter, the insert and the builder (#4591, !1272; "DAG insertion"; the
  code at `7d9458e10` checks presence only and the builder's minimum is a
  count — DIFFERENCES C19).
- **Weak links are allowed and not counted.** After the strong parents, a
  header of round r ≥ 3 names every certificate of rounds r−2 down to
  r−8 that it holds (`weakLinkWindow` = 8, `header_builder.go:175-195`).
  They are parents for the ancestry walks (ordering, support) and nothing
  else: they count toward no quorum, and a certificate at round r−k is
  checked only to be a certificate (one the node holds or can fetch, as
  any parent). *Departure.* Bullshark's weak edges name only vertices the
  author has no path to (§4, `v.weakEdges`); this network names every
  certificate of the window whether or not a path exists. The reason is
  cost: a path check per candidate on every header is a graph search the
  builder can skip, and naming a certificate twice is harmless, because
  every walk over the DAG deduplicates by digest (`dag/traversal.go:89-104`,
  `bullshark/ordering.go:36-46`). The price is header size, at most seven
  rounds × the committee in 32-byte digests, which the parent cap
  (`types/header.go:367`) and the gossip message limit bound.
- **Its payload is in strictly ascending digest order, with no
  duplicate.** The builder sorts and deduplicates (`types.NewHeader`,
  `header.go:81-117`); the decoder refuses anything else
  (`UnmarshalHeader`, `header.go:352-357`), so a header with a repeated or
  misordered batch is unparseable to every peer and never voted. The order
  is the execution order ("The DAG facts", canonical payload order).
- **Size caps.** A header's batches fit `headerBudget`: its share of
  `MaxBlockBytes` among the committee's headers of one block, capped by
  `MaxHeaderBytes` (`header_builder.go:105-130`; "Budgets"). The decoder
  caps the counts, 100,000 payload entries, 10,000 parents, a 1,024-byte
  signature (`header.go:338`, `:367`, `:387`), and after #4599 checks each
  count against the bytes that remain before it allocates (the section
  !1256 adds, "Decoding is bounded by the message").
- **Timestamp.** The header carries its author's wall clock in Unix
  nanoseconds, covered by the digest and so by the signature
  (`header.go:47-54`, set at `header_builder.go:92-95`). No voter checks
  it: not against its own clock, not against the parents'. It matters
  once: the **leader's** header timestamp is the block time of the block
  its group produces, clamped only forward, to be strictly after the
  previous block's time (`internal/node/dagbft/service.go:1015-1026`), so
  every validator derives the same block time and a bad clock cannot move
  time backwards. Block time is executed state: a per-validator clock
  diverged the state trees on the first block, which is why the leader's
  timestamp is used at all (#4054). Nothing bounds how far forward it
  goes: a faulty leader moves block time arbitrarily far ahead, and every
  later block is clamped above it. The bound is undecided (DIFFERENCES
  C23). Genesis headers carry zero.

#### The vote rule

- **A validator votes for at most one header per (author, round), and
  records that choice before the vote is signed** (#4590, !1271). Narwhal
  §4.1: a validator signs a header only if it is the first it has received
  from that author for that round. The record is (author, round) → header
  digest. It is appended to the DAG store's `voted` file before the vote
  is signed (`DAGStore.PutVoted`), and a record that cannot be written is
  not voted, the author's rebroadcast tries it again; it is read back on
  every start, checkpoint or not (`Primary.RestoreVoted`), because a vote
  the node has forgotten it cast is a vote it can cast again for a
  different header; it is kept ten rounds in memory, which covers every
  round a header can still be voted in; each checkpoint drops the records
  below the lower of its tail floor and the previous checkpoint's, since
  either checkpoint may be the one resumed; and it is **not fsynced**, as
  nothing in the store is (DIFFERENCES C9), so a power loss inside the
  filesystem's journal window can lose a record a process restart would
  not. A second digest for the same pair is refused, and nothing is asked
  for on its behalf; the recorded header is voted for again whenever its
  author rebroadcasts it. This is the rule that makes a certificate unique
  per author and round, which the commit rule assumes (Bullshark §4). At
  `7d9458e10` the record is by header digest and in memory
  (`primary/vote_handler.go:361-377`, `:471-476`; `primary.go:351`):
  DIFFERENCES C18.
- **What a voter must hold.** Every parent the header names must be in
  the voter's DAG, and every batch it names must be in the voter's store
  (invariant 11). A header missing either is not refused: the vote is
  deferred, the missing parents are asked for by digest and the missing
  batches are fetched, and the header is judged again at the author's
  next rebroadcast (`vote_handler.go:379-458`) and, since #4611, the
  moment the last of what it lacked lands, the rebroadcast being the
  backstop (section 1, invariant 11). The batches a deferred
  header names are pinned meanwhile so eviction cannot take the ones the
  voter already holds (`primary.go:749-794`; invariant 3). Narwhal §4.1:
  a validator votes only once it has the batches, which is what a
  certificate proves.
- **The accepted window.** A voter at round c votes on headers of rounds
  c−1 to c+1 (`vote_handler.go:317-326`). A header below the window tells
  the voter the author is behind, and the voter pushes it the
  certificates of that round and the next (`:354-357`, `:635-675`); a
  header above it tells the voter it is itself behind, and it asks for the
  rounds between (`:339-353`). A validator does not vote for its own
  header (`:297-301`); it signs a self-vote when it authors
  (`primary.go:613-620`), which counts toward its certificate as any
  member's vote does.
- **Re-sending.** Votes are sent once. The author rebroadcasts a header
  about once a second until it has a certificate for the round
  (`primary.go:444-457`, `:826-883`), and a voter that sees a header it has
  already voted for re-sends the same stored vote (`vote_handler.go:364-376`),
  because a vote lost in the mesh otherwise stalls the round. An author
  drops a header that is two or more rounds behind its own round and
  returns its batches to the workers (`primary.go:840-854`): the window
  above means no one will vote for it.
  The author accepts at most 2 × `QuorumCount` votes for one header, so a
  flood of votes cannot exhaust it (`primary.go:45-49`,
  `vote_handler.go:93-105`); duplicates from one author are dropped before
  that count (`:81-91`).

#### Certificate validity

A certificate is valid when all of these hold (`Certificate.Verify`,
`types/certificate.go:141-230`). Narwhal §4.1.

- **The header is valid as above**, its signature checks, and its author
  is a member of the committee of the certificate's epoch (`:163-171`).
- **The signers are distinct members of that same committee, named by
  index, and their stake is a quorum** (`:173-227`): a repeated index is
  refused (`:198-202`), an index outside the committee is refused, and the
  epoch the certificate names must be the committee's (`:158-161`). The
  committee is the one that orders the certificate's round ("Epochs").
- **The signed bytes are the header digest, the round and the epoch**, 32 +
  8 + 8 bytes big-endian (`types/vote.go:42-50`; re-derived at
  `certificate.go:185-190`), so a vote cannot be replayed for another round
  or epoch of a header with the same digest.
- **The parent rule is applied again at insertion.** `Certificate.Verify`
  refuses only a parentless or duplicate-parent header; the stake-quorum
  check on the round r−1 parents is `DAG.CheckParents`, the one check the
  voter, the insert and the builder share (#4591, !1272; "DAG insertion").
  A certificate is proof that a quorum voted, and a quorum of honest
  voters would not have voted for a header without one, but a node takes
  certificates from any peer and must not rely on that. At `7d9458e10`
  neither `Verify` nor the insert checks it (DIFFERENCES C19).
- **`StateHash` is carried but not signed.** The wire format ends with a
  32-byte state hash (`certificate.go:289-291`, `:382-387`) that is not in
  the header digest and so not under any signature (`:58-61`). Nothing
  reads it for consensus; it is zero on every certificate the network
  forms. A node must not treat it as attested.

The author forms the certificate itself, from the votes it collected plus
its self-vote, as soon as their stake is a quorum (`tryCreateCertificateLocked`,
`primary/vote_handler.go:121-234`); it inserts the certificate into its own
DAG, and only then broadcasts it once and hands it to ordering (`:185-228`).
A receiver recognises a certificate already in its DAG before verifying it,
verifies the rest (one ed25519 check per signature), and inserts or holds
them ("Certificate catch-up"; `primary/certificate_handler.go:27-87`).

#### DAG insertion

- **A certificate enters the DAG only when every parent it names is
  there and its parents pass `DAG.CheckParents`** (`DAG.Insert`,
  `dag/dag.go:90-96`, `:145-161`; #4591, !1272; Narwhal §4: a
  certificate's causal history is available before it is used). A
  certificate whose parents are missing is held and its parents fetched;
  it is inserted when they arrive, and dropped if they do not certify it
  ("Certificate catch-up"). The exact exceptions, each a case where the
  parents cannot be there and no ordering walk will reach below the
  certificate:
  - **genesis**, round 0 (`dag.go:154`, `:207-241`);
  - **a seed**: a node seeded by a join at `S` takes certificates at `S`
    and `S + 1` without their parents, since it orders nothing at or
    below `S` ("Restart"; `consensus.go:936-943`);
  - **a restored tail**: `DAG.Restore` inserts the certificates a
    checkpoint persisted, lowest round first, without the check
    (`dag.go:98-111`); each was verified when the node first took it in,
    the tail starts a rescue window below the commit floor, and what is
    below the tail is committed history. The tail must be closed under
    parents down to that floor, which is #4595's second half.
  - **a certificate taken in without its parents at all**: at a seed, as
    above, and at `7d9458e10` any certificate at or below the commit floor
    plus one once there is a floor (`dag.go:153`), which in normal running
    lets a late certificate in without a parent the node never received,
    and the walk below then differs between nodes (#4595, !1259;
    DIFFERENCES C22). !1259 narrows the waiver to the seed and the
    weak-link window above the collection floor.
- **One certificate per (author, round).** A second copy of the same
  certificate is an idempotent no-op (`dag.go:135-139`). A **different**
  certificate for an author and round the DAG already holds is refused,
  the incumbent is kept, the refusal is counted
  (`accumulate_dagbft_cert_sync_certificates_received_total{outcome="equivocation"}`,
  `DAG.Equivocations`) and logged
  at Error (`dag.go:24-30`, `:140-142`; `certificate_handler.go:151-164`).
  *Departure.* Narwhal has no such case: with one vote per (author,
  round) and quorum intersection, two certificates for one pair need votes
  from more than f faulty validators. Keeping whichever arrived first is
  therefore a tripwire for a fault the protocol's assumptions exclude, not
  an ordering rule, and nothing downstream depends on which one a node
  kept. It is safe to keep the incumbent only because of the vote lock
  (#4590): until that lock is per (author, round), honest voters can
  certify two headers of one author and nodes do split on arrival order
  (DIFFERENCES C18).
- **Insertion is a collection point.** A certificate above every round
  the DAG has seen moves the frontier, and rounds more than `DAGGCDepth`
  below it are collected (`dag.go:182-191`; "DAG retention").

#### Leader election

- **Leaders are elected at even rounds from 2** (`bullshark/bullshark.go:142-155`;
  Bullshark §6, the eventually synchronous protocol, one leader every two
  rounds). Round 0 has none.
- **The function** is `sha256` of the round as 8 big-endian bytes; the
  first 8 bytes of the hash, as a big-endian integer, modulo the
  committee's total stake, choose a point in the stake line; the leader is
  the validator whose cumulative stake, in committee order, first exceeds
  that point (`electLeader`, `selectByStake`, `bullshark/leader.go:16-81`;
  `ComputeLeaderSeed`, `:115-120`). The leader of a round is a validator;
  the round has a leader certificate only if that validator's certificate
  for the round is in the DAG (`:36-44`).
- **The committee is the committee of the leader's round** ("Epochs";
  #4593). At `7d9458e10` it is whatever committee Bullshark holds at the
  moment of the check (`leader.go:31`), which differs between nodes around
  a change (DIFFERENCES C20).
- *Departure.* Bullshark §6 names leaders round-robin ("a predetermined
  leader"). This network weights the choice by stake, so that a
  validator's share of leadership, and of the latency and the dispatch
  duty a leader carries ("The DAG facts"), tracks its share of stake. The
  choice is as predictable in advance as round-robin: both are a function
  of the round alone, and the protocol's safety does not depend on the
  leader being unknown, only on honest nodes electing the same one (§5.2).
  The function changes only with the committee.

#### The commit rule

- **Direct commit.** The leader of round L is committed when the
  certificates of round L+1 whose causal history includes the leader's
  certificate carry stake at or above the **validity threshold** of the
  committee of round L+1, more than a third (`hasSupport`,
  `bullshark/leader.go:83-102`; `DAG.IsAncestor`, `dag/traversal.go:15-71`).
  Bullshark §6: f+1 votes in the next round commit the leader, because
  every round L+2 vertex has 2f+1 strong edges to round L+1 and so a path
  to one of those f+1, so every later leader has a path to this one.
- **When it is evaluated.** Every certificate entering the DAG, one
  released from the held buffer included, triggers the evaluation of the
  leader round L for which it is a round L+1 certificate, and no trigger
  may be lost. A leader that arrives after its supporters needs no trigger
  of its own: a round L+1 certificate names the leader as a parent, so the
  insert holds it until the leader is in (`dag/dag.go:154-160`; "DAG
  insertion"), and its release signals as any insertion does
  (`certificate_handler.go:265`). At `7d9458e10` ordering runs once per
  signalled certificate (`Node.order`, `restart.go:27-83`;
  `processBullshark`, `consensus.go:1001-1043`), `ProcessCertificate`
  returns at once for an even-round certificate (`bullshark.go:140-155`),
  and two triggers are lost: a notification dropped when `newCerts` is
  full (`primary.go:818-822`), and a certificate more than a round ahead
  dropped when the held buffer is full (`certificate_handler.go:59-66`;
  #4162 item 1). Neither has a timer or sweep behind it (#4162 item 6),
  so a leader whose last needed supporter's trigger was lost is committed
  only by a later leader's walk (DIFFERENCES C21); safety is unaffected,
  the latency of that group is what differs. A leader round at or below
  the last commit is not evaluated again (`:157-164`).
- **The linked-leader walk.** Committing leader L also commits, oldest
  first, every uncommitted leader below it that L has a path to: the walk
  steps back two rounds at a time from L to the last committed leader
  round, elects each round's leader, skips a round whose leader
  certificate is absent or that the current leader has no path to, and
  continues the walk from each leader it does find
  (`orderLeaders`, `bullshark/ordering.go:88-123`; Bullshark §5.2, §6). A
  skipped leader is never committed: the certificates in its history
  reach the committed order through a later leader that does have a path
  to them. Each committed leader's sub-DAG is ordered in turn
  (`commitLeaderChain`, `:16-86`), and the last commit round moves to each
  leader as it is done (`:63-65`). *Departure.* "Path", here and in the
  support count above, runs over every parent edge, strong and weak
  (`DAG.IsAncestor` follows the whole parent list, `dag/traversal.go:62`),
  while Bullshark's commit rule and its leader walk use strong edges only
  (§5.2, §6). Following weak links is safe once the parent quorum is
  enforced (#4591, !1272): every path starts from a certified vertex and
  runs over certificates every node holds with their parents, so the walk
  is identical on every node; a weak link can only add a path to a leader,
  never remove one, and the extra paths let a leader that a straggler
  named be committed rather than skipped.

#### Sub-DAG order

- **Which certificates.** For each committed leader, every certificate
  reachable from it by parent links, strong or weak, down to
  **`RescueWindow` = 32 rounds below the last committed leader round**,
  that is not in the **committed-digest set** (`orderDag`,
  `bullshark/ordering.go:140-174`; `GetAncestors`, `dag/traversal.go:79-121`).
  The set records every certificate ever committed and keeps an entry for
  2 × `RescueWindow` = 64 rounds below the last commit (`:67-74`);
  the walk never goes below the window, so nothing older can be emitted
  twice. Bullshark §5.2 orders the leader's whole not-yet-ordered causal
  history. *Departure.* Bullshark's walk has no floor, because its garbage
  collection is agreed (§7); here the walk has a floor, and the digest set
  rather than a per-author watermark says what is already ordered,
  because a straggler rescued by a weak link reaches the committed
  history below the frontier and below that author's later certificates
  (#4111). The window is 32 because a straggler at round x is named by a
  weak link from a header no further than 8 rounds above it, that header's
  certificate is in a leader's history within a round or two of that, and
  the walk from that leader begins at a last commit below the leader; the
  window must exceed the weak-link window plus that slack ("Retention").
  The walk is deterministic only if every certificate in it has its
  parents in the DAG (#4595, above): `GetAncestors` skips a parent it does
  not find (`traversal.go:111-117`), which is what parent-complete
  insertion makes harmless.
- **Sort order.** The certificates of one leader's sub-DAG are sorted by
  round ascending, then by author key bytes ascending
  (`ordering.go:165-171`); within a certificate, batches in payload order
  and transactions in batch order ("The DAG facts", canonical payload
  order). Two certificates of one round and author cannot both be in the
  DAG, so the order is total.
- **One executor block per leader.** The certificates of one committed
  leader are one group, handed to the executor as one block whatever the
  call that committed them (`ConsensusOutput.Leader`,
  `bullshark/bullshark.go:45-60`; `Node.order`, `restart.go:37-58`); a
  group spans the leader's round and the one before it in the common case
  (`roundsPerBlock`, `primary/primary.go:111-114`). The block's leader is
  the group's leader, its time the leader's timestamp ("Header validity"),
  and its position in the committed sequence is what a checkpoint records
  ("Restart"). The group is handed over before the next certificate is
  ordered, blocking rather than dropping (`restart.go:65-81`).

#### Epochs

- **An epoch is the version of the network definition**, and a
  partition's committee for an epoch is the validators of that definition
  active on the partition, with their stakes, in a fixed order
  (`internal/node/dagbft/service.go:223-226`, `:1154-1207`;
  `types/committee.go:245-311` keeps the order deterministic). Every
  header, vote and certificate names its epoch, and is checked against
  that epoch's committee ("Header validity", "Certificate validity").
- **A committee change takes effect at a DAG round fixed by the committed
  sequence, not when a node executes the block** (#4593; Narwhal §6,
  reconfiguration at a point every validator agrees on; Bullshark §5.2,
  honest parties order the same leaders, which needs the same committee
  for a round). What #4593 states: the block that activates a new
  definition is committed at leader round `L`, the change takes effect at
  a DAG round `A` fixed in the committed sequence, and every round below
  `A` is ordered and verified with the committee before it, every round
  from `A` with the new one, on every node and after any restart. So a
  header, vote or certificate is checked against the committee of its own
  round, with that committee's epoch, and a round's parent quorum is
  counted in the committee of the round before; a leader is elected, and
  its support counted, in the committee of the leader's round. *Proposed
  under #4593, not decided:* `A = L + 256`; a node does not order, author
  or vote above the leader round of the newest committed state it holds
  plus that margin, since a change committed above that state could take
  effect there; and a node that was down across two changes does not know
  the committee of the rounds between and rejoins as a node with no
  checkpoint does. The constant, the restart and join cases are #4593's to
  decide. At `7d9458e10` a change applies to
  every component the moment the node executes the block
  (`consensus.go:1154-1179`; `primary/certificate_handler.go:411-457`),
  a header of any other epoch than the node's current one is refused
  (`vote_handler.go:309-315`), and a certificate verifies only against the
  node's current committee (`certificate.go:158-161`): DIFFERENCES C20.

#### Retention

The protocol's windows, as constants, with the relation they must keep:

| constant | value | what it bounds | where |
|---|---|---|---|
| weak-link window | 8 rounds | how far below its round a header names certificates | `primary/header_builder.go:184` |
| `RescueWindow` | 32 rounds | how far below the last commit an ordering walk reaches; the seed is 34 below the first proven state and no handoff is taken within 32 of a seed ("Restart") | `bullshark/ordering.go:125-138` |
| committed-digest retention | 64 rounds | how long a committed certificate is remembered as committed | `bullshark/ordering.go:70-74` |
| held-certificate buffer | 2 × 10 × committee certificates, pruned 10 rounds below the node's round | certificates waiting for parents | `primary/primary.go:292-293`, `:336-338`; `pending_certs.go:138-173` |
| round request | 64 rounds, 100 certificates per response | one catch-up request | `gossip/cert_sync.go:32-33`, `:326` |
| `DAGGCDepth` | 2,000 rounds | how many rounds a node keeps below the further of its last commit and its frontier; how far behind a node can be and still catch up from peers | `consensus.go:37-49`; `dag/dag.go:332-392` |

Proposed under #4593 and not decided: a committee activation margin, how far
consensus may run ahead of the newest state a node holds, 256 rounds in the
proposal ("Epochs"); it would sit between the seed distance and `DAGGCDepth`.

The relation: **weak-link window < `RescueWindow` ≤ ½ committed-digest
retention < seed distance (34) < `DAGGCDepth`**, and
`DAGGCDepth` is also larger than any catch-up a round request spans. The
weak-link window must be inside the rescue window or a straggler would be
named and never ordered; the digest set must cover the whole walk or a
certificate would be ordered twice; the seed must be below the first state
by more than the walk, or the first leaders after it would take in
certificates the peers committed before the seed; and the GC depth must
be above all of them, because collecting a round inside any of these
windows removes something the protocol still reads. Narwhal §4.3 and
Bullshark §7 garbage-collect by agreement; this network collects by depth
behind the frontier, so the relation is what keeps collection out of the
protocol's way.

**When history a node needs is gone**, the node does not improvise:

- a certificate whose parent is collected everywhere cannot be inserted
  and stays held until it is pruned; a node more than `DAGGCDepth` behind
  the frontier is **stranded**, says so once a minute
  (`primary/vote_handler.go:581-602`), and rejoins from a pulled state
  ("Restart");
- an ordering walk that would reach a collected round **stops ordering**
  rather than order a group its peers did not (#4595, !1259; "DAG
  retention"), and the node needs a join;
- a batch outside every peer's retention is not fetched; the node resyncs
  (invariant 6, E11; "Retention" under the batch plane).

#### Persistence and restart

What is durable, and when, so that a restart cannot make a validator
contradict itself. The mechanism is in "The committed log" and "Restart";
these are the rules.

- **Before a header this node authored is broadcast**, the header is on
  disk (`Primary.onAuthored`, `primary/primary.go:584-594`;
  `Node.persistAuthored`, `restart.go:211-231`; `persist.DAGStore.PutAuthored`),
  so that a restarted node rebroadcasts the same header and never authors
  the round again, a second header being self-equivocation (#4159 stall 3;
  "Restart"). It is read back on every start, checkpoint or not (#4592,
  !1258). With it go the certificates and batches the DAG holds, so that
  the restored DAG has the header's parents: at `7d9458e10` from four
  rounds below the header (`restart.go:217-222`), after !1259 every
  certificate as it is inserted (#4595). The write is not synced
  (DIFFERENCES C9).
- **Before a vote is signed**, the (author, round) → digest record is
  appended to the `voted` file, unsynced like the rest of the store
  (#4590, !1271; "The vote rule"; DIFFERENCES C9). At `7d9458e10` nothing
  is written (DIFFERENCES C18).
- **A checkpoint**, written before each block is produced, carries the
  consensus position of that block: the primary's round and epoch,
  Bullshark's last committed leader round, its per-author watermarks and
  its committed-digest set as they were when that block's group was
  ordered, the block index, and the names of the DAG tail and batches
  persisted beside it (`persist.Checkpoint`, `persist/persist.go:31-81`;
  `Node.CheckpointAt`, `restart.go:85-125`; after #4593 the committee
  schedule too). The two most recent are kept (`persist/resume.go:9-14`)
  and the one to resume from is chosen by the executor's last block
  (`ResumeFrom`, `:32-58`; "Restart"). The dedup set is pruned against the
  oldest group handed to the executor and not yet produced, not against
  the frontier, so the set the checkpoint copies is still there (#4594,
  !1261).
- **Which groups a restored or seeded node orders.** A node restored from
  a checkpoint orders the certificates of its tail above the checkpoint's
  commit floor, lowest round first, then goes on live
  (`processBullshark`, `consensus.go:1001-1017`; `DAG.Restore`): the first
  leaders it commits are the ones its peers committed after that block. A
  node seeded by a join at `S` orders nothing at or below `S`
  (`Node.Rejoin`, `consensus.go:936-943`) and holds ordering until the join
  names `S` (`HoldOrdering`, `:945-958`); the groups it then commits
  stand only above `S + RescueWindow` ("Restart"). Neither executes from
  its own last block: the join decides what is executed, consensus only
  where the node stands in the DAG ("Restart", "What it does not do").

#### Who may send consensus messages

Only the committee speaks for consensus: a header, vote or certificate
carries its author's signature, and a node acts on it, and passes it on,
only when its author is a member of the committee of the epoch it names
and its signatures are that member's, for a certificate a quorum of
members'. That is #4596 (!1266), which adds invariant 12 and the section
"Who may send consensus messages" with the per-topic table; what it leaves
undecided, which **peers** a validator exchanges the topics with, is
#4596 part 2 (!1266 records it; !1269 decides it; Narwhal §3 assumes
authenticated links among the validators). At `7d9458e10` the checks are
in the handlers, after the router has already relayed the message to the
node's peers: a vote is dropped unless its author is a member and then
unless its signature checks, membership first because it is the cheap
check (`primary/vote_handler.go:27-47`); a header is checked the other
way round, signature first and then membership (`:278`, `:286`), and
!1266 moves membership before the signature for headers too, inside the
topic validator; a certificate is verified
against the committee before it is inserted or held, after the cheap
duplicate and held-buffer checks (`primary/certificate_handler.go:32-83`);
a batch, a sync request or a sync response is decoded and queued from any
peer (`gossip/gossip.go:347-364`, `:456-508`). Votes are rate-limited per
**relaying** peer, 1,000 a second with a burst of 2,000, a 30 s ban after
five violations, forgiven after 30 s of good behaviour
(`gossip/gossip.go:573-600`; `rate-limiter.go:27-44`); the limits sit well
above a committee's recovery bursts, because banning a committee member
is a liveness hazard (#4054). A bound keyed on the relaying peer silences
everyone relayed through it (#4162 item 18); #4596 part 2 is where that is
decided with the rest.

### The DAG facts the rest of the specification relies on

These are the facts the executor and healing parts depend on; the rules
they follow are in "The protocol" above. The subsections of "The protocol"
are normative, and this list summarizes them:

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

**A node handles one complete answer to a request.** Suppression works
only if the first answer reaches a peer before that peer's own delay runs
out, and under load it does not. On a loaded runner, a joining follower's
requests were each answered by 8 of its 12 peers at the median. Every node
receives every answer and handles them one at a time. So the follower's
own answers waited 24 s at the median behind copies of answers it already
had, the response queue (500, below) overflowed and dropped the next ones,
and in CI its Directory sat at its seed round + 2 for the whole of
`TestFollowerJoinCatchesUp` (#4610). Now a request (its requester's key
and its ID, as for suppression) that is answered with nothing reported
missing is done. Later answers to it, whoever asked, are dropped before
their certificates are read. A partial answer leaves the request open for
the next one. The `Certificate catch-up` line counts the dropped copies as
`repeatedAnswers`.

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

**A restarted partition resumes in seconds, and the simulator's gossip is the
node's** (#4610). consim restarts a node in place: the same host and gossipsub
router leave every topic and join it again, and gossipsub answers an
unsubscribe with a 10 s backoff on both sides, so no mesh forms for a while
(22 s in a loaded run). The node publishes with flood publish
(`p2p.startServiceDiscovery`), to every peer on the topic, mesh or none; a
bare router publishes to its mesh alone, and with none it sends each header,
vote and certificate to nobody — they reached the peers by IHAVE/IWANT at the
heartbeat, one second a hop, a round every 4–8 s, a block every 10–16 s, and
`TestRestartAfterTwoNodesStopFirst` stalled at the 20 s mark on a 2-vCPU
runner (2 of 8 runs; 0 of 24 on a fast machine, where the block fell inside
the window). consim now builds its pubsub as the node does, a restarted
partition never goes more than about a second without a block once its
nodes return (`Result.ResumeGap`, the longest such gap in the 30 s after),
and the restart tests refuse a gap over 5 s. A node
restarts as a new process whose peers drop its connection with no backoff;
production never saw that mesh.

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
round is self-equivocation (#4159 stall 3). A checkpoint written before #4448
has no tail and no authored header, and restores the position alone
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
