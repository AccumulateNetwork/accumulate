# Test reporting specification

What a test run MUST report, and what the words on a dashboard are allowed to
mean. This exists because every clause in it has been violated, at cost:
panels rendered `0` for instruments that do not exist, a load generator
reported `success` for transactions the node rejected, containers reported
`healthy` for a chain that had stopped, and a 10,500 TPS result was published
with no chain-side evidence whatsoever.

Normative language: MUST / MUST NOT are requirements. A run that violates one
is not evidence of anything.

## 1. Displayed means measured

A dashboard panel MUST show a value only if the instrument behind it exists
and was read. An absent instrument MUST render as **`— not measured`**, never
as `0`. Zero is a measurement; absence is not.

## 1a. No impossible states, and bounds over blanks

A panel MUST NOT display a value that another value on the same panel
disproves. `received: 23, sent: 0` is not a missing measurement — it is an
assertion of P and ¬P, and it teaches the reader to distrust the whole board.

Where chain truth proves a bound, the display MUST show the bound rather than
an absent instrument's zero: 23 received proves `sent ≥ 23`, so render
`≥ 23 (inferred)`. An inconsistency between an instrument and an inference
MUST itself be surfaced as an alarm — it means an instrument is broken, which
is a finding, not a rendering choice.

Self-streams (a partition's deliveries to itself) are real sequenced streams
that carry real traffic and can wedge like any other — measured: BVN2→BVN2
`produced=1 delivered=1` during the #4103 bisection. The flow matrix MUST
display the diagonal, not delete it as "bookkeeping".

"No stream entry yet", "zero traffic", and "instrument absent" are three
different facts and MUST render distinguishably.

## 1b. One node is one point of stale truth

A validator answers a query from its own store, and its own store is only as
current as its executor. A reading of a partition's ledger MUST be taken from
every node that answers and reported as the **max across answers, per
field**: every field of a sequence ledger is monotone on every node, so a
lagging node can only under-report, and the max is what the network holds.
Reading one node mixed a BVN1 ledger 348 blocks stale with a current
Directory ledger from the same process and displayed BVN1 → Directory as
produced 100,804 against received 102,177 (run `20260906T134054Z`) — the
destination had received more than the source had sent. On a live board the
same mixing shows as `sent` and `delivered` flickering between a lower and a
higher reading as the router answers from different peers.

A sequence number never decreases. A field that reads LOWER than its previous
sample, after the max across nodes, is not lag — it is a value that went
backwards in a store, and it MUST be surfaced as an alarm naming the stream
and both readings, never absorbed by a high-water mark. The flow matrix's
history MUST carry every cell's `sent`/`received`/`delivered` per sample, so
"it was higher earlier" is a query, not a recollection (#4279).

### A partition's height, and each node's own (#4404, #4345)

A partition's height MUST NOT be read from one node. `monitor.csv`'s
Directory column was the Directory ledger's `index` as host port 26680 —
acc-bvn1-val1 — answered it, and on run `20260924T052134Z` that was the node
chaos restarted first: the column sat at 207 from 05:26:19 to 05:27:24 while
the other eleven Directory nodes went 215 → 323.

- **A partition's height is the highest block any of its validators that
  answered the sample executed** (their `accumulate_node_executed_block` for
  the partition), recorded with **how many answered**. An executor cannot
  pass what its partition certified, so the highest answer is where the
  partition is, and stuck nodes cannot drag it down however many there are.
  A majority of the answering set could (review F2): a 4-validator BVN with
  one node stuck at 214, one paused and one missing a scrape answers
  [1000, 214], whose "majority" is 214, and the stuck node read caught up
  against its own height. The max falls only if the leading validators all
  miss a sample, which the answered count shows. Followers are not in it. A
  node executing a divergent fork past its partition would raise it; anchor
  agreement, not height, catches that node.
- **Each node's own executed block is its own column**, per partition. A node
  that did not answer the scrape is an empty cell, never 0.

`monitor.csv` (heights.py): `time,dnHeightMax,heals,cpuPct,followerHeals,
dnValidatorsAnswered,exec.<container>.<partition>…` — one `exec.` column per partition
of every validator and of every follower the run has (never a declared
follower the run does not start, #4389). The column was named `dnHeight` and
held the one-node ledger index until #4404; the two are different quantities
and MUST NOT be compared across that change. The executed block runs ahead of
the ledger index on an idle network, because an empty block is executed and
never written.

## 2. The chain is the source of truth

Every claim about network behaviour MUST be derived from chain state or node
telemetry, never from the tool that generated the load:

| claim | authoritative source |
|---|---|
| blocks produced | system ledger `index` per partition |
| a transaction happened | its effect on chain state (account exists, balance moved) |
| delivery state per stream | synthetic/anchor ledger `produced`/`received`/`delivered`, per src→dst |
| recovery activity | heal / reconcile counters exported by the node |
| induced faults | container logs (`dropping … envelope`) |

## 3. Required node exports

Nodes MUST export the following Prometheus families (namespace `accumulate`).
This is the contract the soak monitor is written against:

| family | kind | labels | meaning |
|---|---|---|---|
| `conductor_heal_requests_total` | counter | source, destination, outcome={answered,not-yet,miss,failed} | span requests the receiver made, by what the source answered |
| `conductor_heal_entries_total` | counter | — | entries received in answer to span requests |
| `exec_staged_proofs_total` | counter | outcome={staged,validated,disproved,conflict,invalid,unbound,refused,duplicate} | collection proofs by anchor-staging outcome |
| `exec_synthetic_anchor_total` | counter | applied={proven,unproven,collected,tossed,anchor-collected,anchor-tossed,this_block,earlier,missing} | synthetics as staging judged them |
| `staging_held_entries`, `staging_held_bytes` | gauge | ledger, source | what a stream holds in staging |
| `dispatcher_drops_total` | counter | destination, reason={deadline,queue-full} | envelopes dropped undelivered |
| `bcdb_staged_commits`, `bcdb_oldest_view_age_seconds` | gauge | database | store isolation cost, as of the last commit or release |
| `dagbft_execution_lag_blocks` | gauge | partition | committed groups the executor has not produced a block from |
| `dagbft_submissions_total` | counter | partition, outcome={accepted,rejected} | `accepted` = **the node took responsibility** — the submission entered this node's worker or this node's relay, counted once at that first entry, never per attempt. `rejected` = refused without relaying |
| `dagbft_certified_own_transactions_total` | counter | partition | transactions from this node's OWN batches that reached a CERTIFIED header of this node, each counted at most once |
| `dagbft_relayed_total` | counter | partition, outcome={taken,refused,not-ready,unreachable} | submissions this node handed to a node that can propose them, counted once per submission at its **final** answer |
| `node_state` | gauge | partition (lower case) | this node's state for the partition: 0 `BOOTING`, 2 `ACTIVE`; 1 and 3 are the retired `WAITING`/`COMPLETE` |

Exported: the first two, on both branches. **Missing: the remaining nine — which
is why the flow matrix and wedge panels have never shown a true value** (#4095),
and why no run can say whether a submission was accepted and never proposed.

The last three are the set a follower makes necessary (#4364, for #4366/#4369).
The quantity is

> **accepted, neither certified here, taken on relay, nor refused
> (#, whole run)**
> = `accepted - certified - relayed{taken} - relayed{refused}`,
> per (node, partition), floored at 0.

On a validator it sits at the in-flight window — the rounds not yet certified.
On a **working** relaying node it is ~0, because the hand-off discharges the
duty. On one that strands it is everything the network dialled to it and lost.
What is left in it had **no answer of any kind**: `not-ready`, `unreachable`,
or still in flight.

**`relayed{refused}` MUST be subtracted.** A validator validated the submission
and declined, and that answer went back to the caller unchanged — the caller
was told "no", nothing is in flight and nothing is lost. Left in, the figure is
driven by whoever sends the node garbage (a client, a peer dialling junk at
`submit:P`, the load generator's own invalid submissions) while the same
envelope sent straight to a validator is `rejected` and costs nothing: a
working follower's row goes red and the acceptance gate fails on traffic it did
not create (threat-reviewer F4 on #4366). Garbage at a follower is `rejected`
if the node refuses it without relaying and `relayed{refused}` if a validator
declines it; neither is stranded, and both stay visible on their own rows.

This holds **because the relay is synchronous and the refusal is passed back
unchanged**. Under accept-and-forward the caller has already been told yes, so
a later refusal IS a loss and MUST return to the figure. A change to that
decision changes this subtraction with it.

**`accepted` MUST mean "the node took responsibility"** — the submission
entered this node's worker, or this node's relay — and `rejected` MUST mean the
node refused it **without relaying**: a malformed envelope, the worker's own
refusal, or a node that cannot propose and has no relay. A submission that
enters the relay is `accepted` however the relay ends; where it ends is
`relayed{outcome}`. It is the node's ledger of what it owes, settled when the
submission is taken.

Two narrower readings are both wrong, and each was tried:

- *"the envelope entered this node's worker batch"* — under a synchronous relay
  it never does, so this exports `accepted = 0` beside `relayed = n` and raises
  the `relayed > accepted` alarm on a node working perfectly.
- *"`Submit` returned success to the caller"* — a relay ending `refused`,
  `not-ready` or `unreachable` returned no success, so one unreachable relay
  anywhere makes `sum(relayed) > accepted` and fires the **instrument-fault**
  alarm on a correct run; and a node whose relays never land reads `accepted 0,
  stranded 0`, so **a node dropping everything looks perfect**. The two rules
  beside it in this section — `relayed <= accepted`, and a relay that gives up
  lands in the stranded figure — require the opposite (#4366 note_3869869239,
  decided at note_3869919047).

Whether the caller is answered on the relay's result or accepts-and-forwards is
a separate decision and does not move this counter.

**`accepted` MUST be counted once per submission, at its first entry into the
node's worker or its relay — never per attempt.** One submission is one
responsibility however many targets the node tries for it, and the mirror of
this rule is already stated for `relayed`: once per submission at its final
answer. Counted per attempt, `accepted - certified - relayed{taken}` becomes
the number of not-taken **attempts** on monotone counters, so chaos restarting
one validator — the follower's next N relays to it end `unreachable` and land
on the retry through another validator — leaves the follower's row red for the
remaining hours of a twelve-hour run, after a transient the network recovered
from perfectly (reviewer M2 on #4366). A fresh `Submit` from the **client** is
a new submission and a new `accepted`; the rule governs the node's own attempts
inside one call.

**Across a disturbance**, which is what a long chaos run is made of: these are
monotone counters and the stranded figure never clears, so it is a cumulative
loss and not a level. A target restarting is not a loss — the retry lands and
the row does not move. A submission the node gave up on **is** a loss and stays
counted for the rest of the run, so one restart that loses one transaction
leaves a permanent 1. The acceptance reading is therefore not "0 forever" but
**the figure does not climb between disturbances, and every step is
attributable to one of them**; the step per disturbance is a subtraction over
`submissions.csv` against `chaos.log`, and the manifest's trend clause answers
the tail.

**The rule is read against the LAST sample, and stated with its trend.** In
flight and stranded are the same number at any one sample: a relay not yet
answered, and on a validator the rounds not yet certified. The monitor MUST
write a final row when it is stopped — which is after the load generator's
grace drain and any idle tail — and the manifest MUST state that value **and
its movement over the final samples**: falling with no new accepts is draining,
flat or rising is stranded. The requirement is therefore **0 at the last sample
after the drain, or the residue and its trend** — never "a small number is
fine", which teaches a reader to excuse a slow strand.

**The relay leg is not optional arithmetic.** Paul, 2026-09-19: *"Followers can
relay txs. And should."* `accepted - certified` on a node in no committee is
everything it took, by construction, so without the third term a follower
relaying perfectly would render the largest red number on the board under a
label meaning failure — a clause-1 false negative's mirror image, and on the one
node a follower run exists to watch. `relayed_total` MUST be counted **once per
submission, at the relay's answer**, not per attempt: a submission refused by two
targets and taken by a third is one `accepted`, and a retry count is a different
measurement wanting its own family.

**It composes across nodes.** `relayed{taken}` says a node that can propose
took it, not that it certified it; the next leg is that node's own
`accepted`/`certified` pair in the same families. Summing the quantity over the
fleet therefore gives the network's true stranded count with no node claiming
credit for another's work. For that to hold, **a relayed submission MUST NOT
also be proposed by the relaying node** — relay or propose, never both.
Otherwise a node promoted mid-run certifies what it also relayed, which
double-counts across the fleet and fires the alarm below on a real event.

**`not-ready` is a fact about the network, not about the submission.** A target
that answers `NotReady` is a joining node (executor.md step 6, #4307); the
protocol's meaning is "ask someone else", so a `NotReady` is a retry and MUST
NOT be recorded as an outcome. A submission that is then taken is one `taken`.
Only when the relaying node stops trying does the submission take an outcome,
and if every answer it ever received was `NotReady` that outcome is
`not-ready`, distinct from `refused` — which means a target validated it and
declined, a final answer and a statement about the submission itself.

**Relaying is not gated on being synced.** Paul, 2026-09-19: *"update specs and
development plan with relaying txs if following or syncing"*. A read needs
local state and a relay needs none (executor.md step 6), so a syncing node
refuses every read and still relays every transaction. A syncing node's relay
counters are live.

**An outcome label the reader does not know MUST be surfaced under its own
name**, never folded into a known one and never dropped: four questions about
the relay's behaviour are open (lead's note on #4366) and the answer may add a
label; folding an unread outcome into `accepted` would move a failure into the
success column.

**It MUST be certification and not proposal.** A node in no committee authors
and broadcasts a header carrying its own batches exactly as a validator does
(`pkg/consensus/primary/header_builder.go:35-76`, no committee gate on that
path); what it never obtains is 2f+1 votes, because validators drop its header
at `vote_handler.go:277-284`, so `tryCreateCertificateLocked` never fires for
it. A counter of transactions *proposed* would therefore tick for everything a
follower accepted, the difference would read **0**, and the board would render
an un-red zero under a label asserting nothing stranded — on the one node where
everything does. That is a clause-1 false negative produced by a build that
followed the contract exactly (reviewer H1 on #4364), and it is why the
exporter's hook is the node's own certificate and not its own header.

Each transaction MUST be counted at most once, at the first certified header
carrying its batch: a header that never certifies is requeued and its batches
re-proposed, so a per-header count double-counts and drives the difference
negative. Three impossible states (clause 1a) MUST be surfaced as instrument
alarms and never floored silently: `certified + relayed{taken} +
relayed{refused} > accepted` — the same three terms the quantity subtracts, so
a violation the check does not name is hidden by the floor —
(a per-header certified count, a per-attempt relay count, or a node promoted
mid-run that kept its own copy of what it relayed — a real event, not a broken
counter); `sum(relayed) > accepted`, where the sum includes outcomes the
reader does not know; and **any relay at all beside no `accepted` series**,
since a relayed submission is `accepted` by definition — the first two are
read against a reported `accepted`, so without that check a build exporting
`relayed_total` and no `submissions_total{outcome="accepted"}` slips past
both, the stranded count floors to 0, and a node relaying everything or
losing everything reads clean. Until the families exist the harness renders
`— not measured` on the board, in `submissions.csv` (a header and no rows, and
an empty field in a row that does exist) and in the manifest — never 0, because
0 asserts that nothing stranded, which is the one thing run `20260919T191634Z`
could not establish.

### The node-state row (#4364)

`node_state` is read **per node AND per partition**, every node including the
follower: one process runs the Directory beside its BVN and one can boot while
the other serves, so a row per container would fold a booting BVN under an
active Directory.

- **ACTIVE is value 2, and is the predicate.** Nothing else reads as active —
  not a retired state (shown by name, `WAITING (retired)`), not an unknown
  value, and not a missing gauge.
- **BOOTING is shown by name and is not an alarm by itself**: a node just
  restarted, or a follower just added, is booting, and that is the join
  working.
- **BOOTING longer than `BOOTING_BOUND_S` (600 s) after its container started
  is an alarm.** The disturbance is dated by the container's
  `State.StartedAt`: a restart is what makes a node boot, a pause is not.
  Where the start cannot be read, BOOTING is shown and not judged.
- **A node with no gauge is one row, `— not measured`, never ACTIVE** (clause
  1). The fleet reads all-active only when every row is measured and 2.

Board: *ACTIVE, now (node × partition rows)* as `n / rows`, *BOOTING, now
(rows)*, and every row that is not ACTIVE named with its state and its time
since its container started.

**Container start → ACTIVE (s)** is the number the verdict wants on a restart
and on an add-follower. `nodestate.csv` (`time,node,role,partition,
containerStarted,state,startToActiveS,kind`) holds one row per start, per
partition, as it reaches ACTIVE: `reached` when the start was seen not ACTIVE
first — a measurement, to the 5 s scrape interval — and `already` when it was
ACTIVE at the first sample after the start, an upper bound only. On its way
out the monitor writes a `final` row for every start that never reached
ACTIVE. `/data` carries the same under `nodeState.starts`. The manifest's
row, one for the validators and one for the follower, states the worst
`reached` figure and names every start that never reached ACTIVE; `already`
figures are counted and kept out of the worst.

**Rejoined, not ACTIVE (#4404).** The gauge cannot say a node rejoined: it
goes ACTIVE at the join's first root match, mid-join
(`internal/node/join/state.go:857` → `tracker.go:194`), and is never demoted.
Run `20260924T052134Z`'s row read three failed starts as reaching ACTIVE —
acc-bvn1-val1 bvn1 at 12.1 s, stuck at block 214 while its peers reached
1376; acc-bvn3-val1 and acc-bvn2-val2 on the Directory 81 s and 262 s before
handoffs that failed. A start of a node, per partition, is **rejoined** only
when all three hold:

1. the gauge read ACTIVE after the start;
2. at some sample it was ACTIVE with its executed block within
   `REJOIN_MAX_BEHIND` (soak.conf, blocks) of its partition's height, and it
   was still within that bound at the start's last reading;
3. every anchor it stated for that partition after its start agrees with its
   peers': the same (root, BPT) as the other validators at the same block, and
   the same (block, root, BPT) under the same sequence number to the same
   destination. Peers vote once each per value, never once per line — a node
   re-sending one anchor 304 times is one peer. By block alone this misses a
   node that signs another body under a sequence number at a block its peers
   never anchored: acc-bvn2-val2 signed seq 1154 as block 1300 (root
   `f5b4979b`) where its peers' seq 1154 is block 1301 (root `de98b6c8`), and
   no peer anchored block 1300.

A failing reading makes the start **NOT rejoined**; a reading that could not
be made (no executed gauge, no final row, no anchor line after the start)
makes it **not established**, and says which. It is never called rejoined on
the gauge's word. The time quoted is container start to the first sample that
was ACTIVE and within the bound. **Only the launch goes unjudged**: a
container started before the first sample in `nodestate.csv`. Every later
start is judged, however it was first seen — a start the monitor first sees
already ACTIVE (a join inside one scrape interval, or a restart that spans a
monitor restart, whose new process sees every node `already`) is judged on
height and anchors like any other, with its gauge time given as an upper
bound and "boot time not measured". A row with no start after the launch
says so and claims nothing about starts.

`nodestate.csv` carries, beside the columns above, `executedBlock`,
`partitionHeight` (the highest block any answering validator of the partition
executed), `startToCaughtUpS` and `validatorsAnswered`, read at each
row's sample, and two more kinds: `caught-up`, the first sample ACTIVE and
within the bound, and `superseded`, a start's last reading when its container
started again. At exit the monitor writes a `final` row for **every** start,
not only those never ACTIVE: that row is the start's last reading. A start
that never reached ACTIVE is one with no `reached` or `already` row, as
before. The board lists a row the gauge calls ACTIVE when it is more than the
bound behind its partition. The manifest's row is `rejoin.py`, reading
`nodestate.csv` and `node-logs-live.txt`.

**A follower of the run is one the run has (#4389).** A follower in the
compose's late-follower profile is declared in docker-network.yml, so init
writes its key, and `compose up` never starts it. It is a follower of the run
only while it is up — from its `add-follower` line in `chaos.log` to its
`remove-follower` — and the manifest counts it only when the add-follower walk
is on, as the follower "started only by the add-follower disturbance". Outside
that it has no `follower.csv` row, no probe read and no `exec.` column, and the
follower report's behind figures are read from its own follower's rows only.

### The add-follower and remove-follower verdicts (#4364)

The manifest's follower section carries one row per `add-follower` and one
per `remove-follower` in `chaos.log`, in time order, written by soak.sh's
`follower_verdict_rows` from the run's captured files — never from a live
network. An add is read over **that container's life only**, from its
`add-follower` line to its `remove-follower`, matched by container name; a
second add of the same container never borrows the first one's numbers.

Per add-follower:

- **container start → ACTIVE (s)**, the worst partition, from the `reached`
  rows of `nodestate.csv` for the start inside the life (the chaos log's own
  seconds-after-the-add only when soakmon wrote none); `NEVER ACTIVE` with
  the chaos log's reason when the life ended first;
- **blocks behind at hand-off**, per partition, from the `follower.csv`
  sample nearest the moment the last partition went ACTIVE — not the lag
  before it;
- **first root match**, the block and the validator, from `chaos.log`;
- **NotReady before ACTIVE**, the reads refused before the hand-off and the
  partition and service that refused them, from `readprobe-follower.csv`;
- **accepted, relayed-taken (as a share of accepted), stranded** on that
  follower at the last `submissions.csv` sample inside its life.

Per remove-follower: `followerchaos.unaffected` over
`follower-removal-N-{before,at,after}.json`, the readings
`FOLLOWER_WINDOW_SECS` either side of removal N — `unaffected`, or
`AFFECTED` naming the partition whose cadence fell or the stream whose
delivered count stopped or went backwards.

Every quantity a life did not record reads `not measured`, never a pass.

`readprobe-follower.csv` (`time,follower,partition,service,outcome,reads`)
is the read probe's record of every follower it asks, from the follower's own
port: one row per round, follower, partition and outcome, `reads` the count.
`service` is the API service that answered (`query`); `outcome` is
`answered`, `not-ready` (the node refused with `NotReady`, JSON-RPC code
-33504 — the join working, not a failed read), `gated`, `timeout` or
`error`. A follower not yet added answers nothing and reads `error`.

**On the board** a follower added mid-run is the same *follower* row group as
the one launched with the network (#4365), not a second one. Every reading
in it names its follower — `acc-bvn3-fol2 BVN3 800/823`, never a bare
`BVN3 800/823` — and the caption says how each came to be there: launched
with the network, added at a time (and which add), removed at a time, or not
added yet, from the last `add-follower` / `remove-follower` line of
`chaos.log`. A removed or not-yet-added follower is not asked and reads
`— not measured` with that reason, never "did not answer". The stream
matrix is unchanged: source to destination, partitions only, read from the
validators.

The consensus-status API MUST additionally report `syntheticHeals` and
`anchorHeals` (#4075) — the coarse monitor's CSV reads them.

**`accepted` and `rejected` partition Submit calls; `rejected` is not in the
stranded figure (#4404).** Every road out of `SubmitterService.Submit`
(`internal/node/dagbft/api.go`) moves exactly one of
`submissions_total{outcome="accepted"}` and `{outcome="rejected"}` by one: a
submission that enters this node's worker or its relay is `accepted` however
the relay ends; one refused before either — undecodable, no relay, or the
worker's own refusal (store full, execution lagging, validation) — is
`rejected`. `TestSubmitter_EverySubmitIsAcceptedOrRejectedNeverBoth` drives
each road and checks the sum. The two are per CALL, not per transaction: a
client that retries a rejected transaction and is then accepted appears in
both, once each, and only the `accepted` one enters the figure. Equal values
are therefore two independent series crossing — acc-bvn1-val1/BVN1 read
4,086 and 4,086 at 05:46:10 on run `20260924T052134Z`, then 4,216 and 4,322 —
and a refused submission never inflates the stranded count.

## 4. Health means liveness

A container healthcheck MUST fail when the node's partition ledger stops
advancing beyond a threshold. `13/13 healthy` over a chain that wrote nothing
for 12 minutes (#4103) is a false report, and it is the one report everyone
checks first.

## 5. Load generators are witnesses, not referees

A load generator MUST report, distinctly:
- **requested vs achieved rate** — silently substituting another rate (#4102) is a defect
- **accepted vs rejected**, with rejection reasons — counting HTTP acceptance as success (#4104) is a defect
- **followed-to-delivery outcomes** — a bounded number of followed transactions
  that never landed MUST fail the run (`-max-stranded`)

A generator that cannot produce fee-valid work (#4107) MUST refuse to start
rather than submit doomed transactions.

**A read a node will not answer is asked of another, and counted (#4404).** A
joining node answers every read `NotReady` ("… is joining and cannot answer for
state it has not executed"), which is the protocol's "ask someone else". The
generator's query pool rotates to the next endpoint on a `NotReady` exactly as
on a transport error; run `20260924T052134Z` lost 52 reads to one joining node
before it did. `loadgen-stats.json`'s `queries` object counts
`notReadyRetriedElsewhere` and `transportErrorRetriedElsewhere` (answers
handed to another endpoint) and `notReadyAtEveryEndpoint` and
`transportErrorAtEveryEndpoint` (queries returned to the caller because no
endpoint answered). Submissions are not rotated on `NotReady`: they are pinned
to an endpoint by signer for ordering, and a submission's `NotReady` is also
the store-full back-pressure answer.

## 6. Provenance

Every run MUST record before load starts: commit, `git describe`, branch,
uncommitted-file count and patch, image ID, executor version, topology,
settings (duration, rate, drops, healing), and the config files as run.
Results MUST be appended to the same manifest. (Implemented — `soak.sh`.)

## 7. Observation

The monitor MUST be running and verified before load starts, and the run MUST
abort if it is not (implemented — the soakmon gate in `soak.sh`). The
dashboard MUST be visible to the operator, opened for them, before load
starts (implemented — `run-remote.sh`).

## 8. Summaries in, raw data out

Test results are reported as summaries — in the issue the run served, and in
the run's `manifest.md` / `runs/INDEX.md`. Raw test data MUST NOT be
committed: no logs (`*.log`), no heap/CPU profiles (`*.pb.gz`), no goroutine
dumps, no node-log captures. They stay on the machine that ran the test
(enforced — `runs/.gitignore`); anything a reader needs from them goes into
the summary. On 2026-08-24 the full history of every branch and release tag
was rewritten to purge previously committed raw data — do not reintroduce it.

## Current compliance

| clause | state |
|---|---|
| 1 displayed=measured | met for healing and wedges as of #4279 — a family no node reports renders `— not measured`; the monitor had read `crosschain_*` families that no longer existed and shown 0 for every one of them |
| 1a no impossible states | **violated** — anchor flow shows `sent 0` beside `18/18 received`; diagonal deleted (#4093, #4095) |
| 1b one node is stale truth | met, as of #4279 — soakmon and streams.py read every node and keep the max; a decrease is logged as `SEQUENCE REGRESSION` and the cell goes red; the history carries the matrix |
| 2 chain as truth | met by soak.sh/loadgen; violated by parallel-loadtest's recorded results |
| 3 node exports | table above rewritten to the families the node exports (#4279); consensus-status fields missing on DI (#4075) |
| 4 health=liveness | **violated** (#4108) |
| 5 generator honesty | met by tools/cmd/loadgen; parallel-loadtest fails all three (#4102, #4104, #4107) |
| 6 provenance | met |
| 7 observation | met, as of this branch |
| 3 node-state row | met by the harness (#4364): board, `nodestate.csv`, manifest; a node without the gauge reads `— not measured`. Since #4404 the manifest judges rejoined (gauge, executed height, anchor agreement), not ACTIVE |
| 1b partition height | met, as of #4404: `monitor.csv`'s Directory height is the max over the validators that answered, with the count that answered, and every node's executed block has its own column |
