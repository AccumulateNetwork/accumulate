# Healing — Specification

Cross-partition messages — synthetic transactions and anchors — travel in
sequenced streams and must be delivered in order. When one goes missing, the
destination cannot advance past it. Healing is how the missing message is
obtained. It is the retry mechanism for cross-partition delivery, so it needs no
retry mechanism of its own.

## 1. Architecture — what we are doing

### Gaps

A gap is an entry the node needs and does not hold. Four facts define it, each
with one owner:

| fact | owner | meaning |
|---|---|---|
| `Delivered` | the ledger | the highest number this stream has executed |
| **held** | staging | entries received and not yet executed |
| **proven** | the replica | the hashes every accepted collection proof covers |
| `Produced` | the source | how far the source has gone |

The gaps a destination can see are the **proven hashes above `Delivered` that
staging does not hold**. Holding a proof for a range means the source produced
every entry in it; not holding an entry means it never arrived. Nothing at or
below `Delivered` is a gap, is requested, or is counted.

A stream that lost its **tail** shows no gap: there is no proof for what was
never sighted. That case is found by asking the source what it has produced —
the reconcile path — and is requested by stream and range rather than by hash.
A gap must persist for a grace period before reconcile acts on it, so entries
merely in flight are not requested.

Which streams to consider comes from staging and the replica, not from the
ledger: a stream that has only staged has delivered nothing and has no ledger
entry, yet is the stream most likely to be stuck.

A restart changes nothing. Staging is durable ([executor.md](executor.md),
Restart), so a restarted node holds what it held and has the gaps it had.

### Who asks, and when

Healing **activates every few blocks**, not every block. A request goes to
another partition and its answer comes back through consensus, which takes
blocks; activating every block would re-request what is already on its way.

**Every validator computes the same request set.** Staging and the replica are
deterministic functions of consensus input, so every node reaches an activation
with the same gaps. Nothing is random and nothing is negotiated.

**Two validators send**, selected by the previous block's hash over the
validator set. The hash is already agreed, changes every block, and is nobody's
to choose. Two rather than one so a dead or unreachable validator does not cost
an activation; two rather than all because a request is fungible — whoever asks,
the answer heals every validator — so further askers are only load. The pair
rotates with every activation.

Selection applies to **pulls only**. An anchor **signature** is a contribution
only its validator can make, so every validator re-sends its own signatures on
the cadence; selecting a pair there would withhold the quorum. The test: does
another node's action make mine unnecessary? If yes it is a pull and a pair is
enough; if no, everyone owes theirs.

### The request

A request is an API call from a selected validator to a validator of the source
partition. It carries the **destination partition and the set of hashes**
wanted — nothing else: no sequence numbers, no proofs, no receipts. One request
per source per activation, whatever the number of gaps; the several messages
that reveal one gap collapse into one hash in the set.

A request is bounded in time and is sent at once, without waiting for the block
to commit, because nothing in this partition's state depends on it. A lost
request costs nothing: the gap is still a gap at the next activation. An
activation ends when its time is up; the next activation is skipped while one
is still running; a source whose requests all failed is asked less, not more,
until it answers again.

**A hash is asked for once.** The activation that asked is remembered with the
hash, and the hash is not asked again while the answer can still arrive. It is
asked again only when that many activations have passed without it landing.
Asking twice for a hash is asking about an entry already held or already on its
way — a bookkeeping defect, not traffic the stream requires.

### The answer

The source answers **entirely from its cache** (below) and nothing else: no
chain walk, no receipt, no signature, no database read. It packs the entries
into a **bundle** — as many anchors and synthetic transactions as fit the
envelope budget, whatever their streams, each with the transaction it belongs to
when it has one, and with no proof of its own — and **submits the bundle into
the requesting network** through the same path a dispatch uses. A bundle below
the minimum size waits for the next request to the same destination unless
nothing else is pending.

A bundle is **not a transaction** and is never sent to the executor. It is the
envelope that provides the missing synthetic and anchor messages. There is no
healing message type, no executor for one, and nothing is recorded for the
bundle itself. Its entries execute as what they are, in their streams.

### Where it lands

A bundle arrives through the requesting network's consensus, never by a side
door: staging decides what a block executes, and every validator must hold the
same staging at the same block ([executor.md](executor.md), Restart).

In the block, bundles are the **first group** of the sort
([executor.md](executor.md), "Sort, then four groups"). The block opens the
envelope and writes its entries to staging as held before any anchor or
synthetic is evaluated, so the runs they complete drain in the same block. Every
entry is already proven by a receipt the destination accepted, so no
admissibility question is asked of it.

When a run executes, its entries are **truncated** from staging. Staging holds
only what is above `Delivered`; it is a buffer, not a store.

### Proofs are extended, not replaced

A collection proof is a merkle state at the start of its list, the elements, and
a receipt anchoring the last element to a root. Its element count is bounded
(`MaxReceiptListElements`) because a list is untrusted input that must be hashed
before it can be known to be junk; the bound binds identically at the sender,
the sequencer and the receiver.

The bound does not limit how far back a destination can prove. Widening a proof
backwards means an earlier merkle state and the elements in between; the replay
ends at the same anchor, so **the same receipt keeps working**. A destination
that needs to reach further back asks the source for **the merkle state at
index `f` and the elements `[f, c)`** of the stream's chain, where `c` is where
its current list begins. The source reads hashes out of a chain it already has —
no rebuilding, no signing — and the destination validates the widened list
against the receipt it already holds, so a wrong or dishonest extension fails
to validate and is discarded.

The same request fills interior holes: a counted merkle state binds every element
to an absolute index, so a destination holding fragments of a range knows
exactly which spans are missing and asks for each. Nothing already held is
fetched again. A later proof does not invalidate an earlier one; each verifies
against its own state and receipt.

Whether fragments must outlive the activation that fetched them is decided by
measurement — how far back a destination actually has to reach against the
per-request bound. If they must, they live where staging lives: durable and
outside the account hash.

### The cache

The cache is the **producer's**. A partition keeps every synthetic message and
every anchor it produced over the healing window and serves every request from
it. There is no destination-side cache; nothing is fetched twice.

- **When.** An entry enters the cache when it is **produced** — when the block
  sequences it onto the synthetic chain or builds the anchor — not when it is
  dispatched and not when a destination executes it. That is the earliest
  point at which the entry is final, it is one write on a path the block
  already takes, and it makes the cache a mirror of production.
- **Contents.** The sequenced message and, when it has one, the transaction it
  belongs to. No proofs: bundles carry none, and the extension and range paths
  read proofs from the chains.
- **Keys.** By entry hash, the request's vocabulary; and by stream and
  sequence number, for the reconcile and range paths.
- **Window.** Bounded in blocks and in bytes. What leaves the cache has also
  left every destination's gap scan: older than the window means healed to
  depth or in need of a snapshot. Nothing is invalidated; an entry's content
  cannot change under its hash.
- **A miss is a defect.** The cache is populated at production, so a request
  inside the window that misses means the window or the cache is wrong. It is
  counted, with its depth.

### Counting

Per node and per stream, so healing is judged from data:

| count | what it says |
|---|---|
| requests issued, hashes per request | how much is missing and how often we ask |
| requests per hash | monotonicity; anything above one is a defect |
| bundles received, entries per bundle | that answers are bulk, not per message |
| cache hits, misses, miss depth, construction failures | at the source; a miss is a defect |
| request to landing, in blocks | whether the cadence gives an answer time to arrive |
| staging depth, entries truncated | that staging is a buffer, not a store |
| reconcile requests, ranges, entries recovered | how often a tail is lost outright |

### Invariants

1. **A depth is healed once.** A hash is requested at most until it lands, and
   a range healed to a depth is not healed to that depth again.
2. **The source already has the answer.** Serving a request is handing back what
   the producer cached at production; it is never rebuilt.
3. **Every validator computes the same requests; a selected pair sends them.**
   Signatures are contributions and are exempt from selection.
4. **A bundle is an envelope, not a transaction.** Nothing is executed or
   recorded for it; its entries execute in their streams.
5. **Bundles land through consensus and are applied to staging first.** Staging
   is the same on every validator at every block.
6. **Staging is truncated as runs execute.** It holds only what is above
   `Delivered`.
7. **Healing is bounded per activation** in requests, in time, and to one
   activation at a time.

## 2. Specification — how it is implemented

Deciding is part of the block: staging computes the gaps and the request set as
part of executing an activation block, deterministically. Transport is not: the
API call, the bundle submission and the counters live in
`internal/core/crosschain` and run outside consensus.

### Deciding, in staging

On an activation block (`healActivates(index)`, every `healCadence` blocks),
after the four groups have executed, staging computes per source: the proven
hashes above `Delivered` not held, minus hashes asked for within the last
`healPatience` activations. That is the request set. Sender selection is a
function of the previous block's hash over the validator set yielding two
indices; a node compares them against its own position.

The reconcile path runs on the same activations for the selected pair: it asks
each source for its `Produced` toward this partition and, for a tail above the
sighted high-water mark that has been overdue for `reconcileGraceBlocks`,
requests the range by stream and indices.

### Requesting and answering

The private sequencer service (`internal/api/private`) carries three methods:
one entry by stream and number, a range by stream and indices, and **entries by
hash set** for a destination. The first two serve the reconcile and extension
paths; the third serves healing. All three are answered from the producer cache
where it holds the entry; a range or extension that reaches below the cache is
read from the chains.

The source packs the entries into bundles under the envelope budget
(`synthPackageBudget`) and above the minimum size, and submits each bundle to
the requesting partition through the dispatcher
(`internal/node/daemon/dispatcher.go`), the same path `sendSyntheticTransactions`
uses. The requester's call is bounded by `HealTimeout`; a transport failure is
retried a few times because routing picks a peer per attempt; a `NotFound` for a
hash is a deterministic answer and is counted as a miss.

### Landing

The block's sort (`exec_stage.go`, `classify`) recognises a bundle by its shape
— sequenced entries with no proof whose hashes the replica already contains —
and records each entry as an arrival on its stream before the anchor group is
evaluated. Nothing is recorded for the envelope. `stageRuns` then computes runs
with those entries held, and executed entries are removed from staging when the
stream's position is written back at close.

### The cache

`internal/core/crosschain/cache.go`, filled by the executor at production
(`produceSynthetic`, `prepareAnchor`) through a hook the block calls once per
entry; keyed by hash and by (stream, number); bounded by `HealWindowBlocks` and
`HealWindowBytes`; read by the sequencer service. Hits, misses, miss depth and
construction failures are counters on the node's metrics endpoint, as are every
row of the counting table above.

---

Where the implementation departs from this specification, see
[DIFFERENCES.md](DIFFERENCES.md).
