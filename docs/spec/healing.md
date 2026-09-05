# Healing — Specification

Cross-partition messages — synthetic transactions and anchors — travel in
sequenced streams and must be delivered in order. When one goes missing, the
destination cannot advance past it. Healing is how the missing message is
obtained. It is the retry mechanism for cross-partition delivery, so it needs no
retry mechanism of its own.

## 1. Architecture — what we are doing

### Gaps

Staging is two stores ([executor.md](executor.md), "Collection"): entries by
stream and index, and collection proofs by the Directory block index of the anchor
each terminates in. Entries and indexes are one to one, and every index is
eventually covered by a proof, so there are exactly two kinds of gap, both by
index:

| gap | meaning | answer |
|---|---|---|
| **proven, missing** | a validated proof covers the index and no entry is held there | the entry, in a bundle |
| **held or expected, unproven** | entries are held (or lower indexes are proven) and no validated proof covers the index | a proof extending the covered range |

Anchors are a third case and are not a healing-cycle matter: anchors are slow
and sequenced, so a later anchor exposes a missing earlier one, and that anchor
is requested at once. An anchor is admitted by validator signature quorum, as
today; a raw past anchor that arrives or is held is validated when a later
anchor's hashes prove it. Healed anchors therefore travel raw, and every
validator keeps re-sending its own signatures on the cadence.

**Nothing is provable before the Directory has anchored it.** A synthetic or
anchor a BVN produces in block N cannot leave, and cannot be proven to anyone,
until the Directory has executed a BVN anchor that covers N — the anchor for N
itself in the normal case, but any later block's anchor works, since the later
root chain contains N's root — and sent the receipt back
([executor.md](executor.md), "Dispatch"). So an index the destination has
not sighted is not a gap until that round trip has had time to complete, and a
source can serve a proof only for spans its Directory receipts already cover;
asked sooner it answers "not yet", which is counted and is not a miss.

**A gap is judged only after staging has finished the block** — intake,
anchors, proofs, drains. A new gap is ignored until the next healing cycle. If
it is still there at the second cycle, it is requested. Two cycles is the
patience, counted in blocks, so every validator judges the same gaps; the
cadence times two must exceed the Directory round trip above, or the healer
asks for what is still on its way.

Nothing at or below `Delivered` is a gap. An index further ahead than about an
hour of the source's production is refused on arrival, not healed: a partition
that far ahead is a fault to be dealt with elsewhere.

A node that restarts syncs first: it replays the committed stream and rebuilds
staging as its peers built it, so when it has caught up it holds what they
hold and has the gaps they have (executor.md, "Sync").

### Who asks, and when

Healing **activates every few blocks**, not every block. A request goes to
another partition and its answer comes back through consensus, which takes
blocks; activating every block would re-request what is already on its way.

**Every validator computes the same request set.** Both staging stores are
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
wanted for proven-missing gaps, and the **index spans** for which a proof is
wanted — nothing else. One request per source per activation, whatever the
number of gaps; the several messages that reveal one gap collapse into one
hash in the set.

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

For hashes, the source answers **entirely from its cache** (below) and nothing
else: no chain walk, no receipt, no signature, no database read. For index
spans it answers with a proof built from the same cache — the span's hashes,
and the receipt to a Directory root the cache keeps for the block those
entries were dispatched under ([Proofs are
extended](#proofs-are-extended-not-replaced)) — and only for
spans the Directory has anchored back to it; a span above that is "not yet".
Nor does it serve what it dispatched within the last few blocks
(`InFlightBlocks`): those entries are on their way, and an answer must never
duplicate a delivery in flight. A span that is partly ready is answered as
far as it is ready; the requester remembers only what it was given.
It packs the entries into a **bundle** — as many anchors and synthetic transactions as fit the
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

In the block it is **intake**, the first group of the sort
([executor.md](executor.md), "Sort, then four groups"): entries go to synthetic
staging at their index, where the proof that named them has already proven
them, and a proof goes to anchor staging under its anchor's Directory block index.
Nothing is evaluated for the envelope and nothing is recorded for it. The runs
the entries complete drain in the same block.

A proof an anchor disproves is discarded and counted. Two proofs for the same
indexes with different hashes are an attack, counted; a validator signature on
proofs is the eventual answer.

When a run executes, its entries and the proven ranges at or below `Delivered`
are released from staging at commit. Staging holds only what is above
`Delivered`.

### Bounds

- **A request carries at most `MaxRequestHashes` (1,024) hashes and
  `MaxRequestSpans` (16) index spans.** What does not fit waits for the next
  activation; the oldest indexes go first, because delivery is in order.
- **An answer is as many bundles as the entries need**, each within the envelope
  budget. A bundle is never held back to grow: the minimum size only coalesces
  requests from the same destination that are pending in the same block, so a
  lone missing entry is answered by the call that asked for it.
- **A request needs no authentication.** It changes nothing at the source, its
  answer is gated by the requesting network's consensus and proven by proofs
  the destination already holds, and its cost to the source is bounded by the
  request bounds and served from the cache. A hash the cache does not hold is
  not served: the miss is counted, and it is a defect.
- **The asked-once record and the per-source back-off are node state, not
  consensus state.** They live in memory beside the healer, keyed by hash and
  by source with the block index of the activation that asked. A restart
  empties them; the cost is at most one duplicate request per gap.
- **The synthetic/anchor cache holds entries in play**, indexed by partition and
  index, cleared as the destination delivers ([The cache](#the-cache)). Its size is decided from measurement of how many
  entries are in play at the target rate. The sanity horizon of about an hour
  bounds what staging will hold, not what the cache keeps.

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
its current list begins. The source reads the hashes out of its cache — no
chain walk, no rebuilding, no signing — and the destination validates the widened list
against the receipt it already holds, so a wrong or dishonest extension fails
to validate and is discarded.

The same request fills interior holes: a counted merkle state binds every element
to an absolute index, so a destination holding fragments of a range knows
exactly which spans are missing and asks for each. Nothing already held is
fetched again. A later proof does not invalidate an earlier one; each verifies
against its own state and receipt.

Whether fragments must outlive the activation that fetched them is decided by
measurement — how far back a destination actually has to reach against the
per-request bound. If they must, they live where staging lives: in memory,
outside anything hashed or written.

### The cache

The synthetic/anchor cache is the **producer's**, and it holds only the entries
**in play**: what this partition has produced that a destination may still ask
for. It is one of two caches and must not be confused with the other
([database.md](database.md), "Caches"): the hash-to-URL mapping cache is a
two-level cycled cache in the dynamic layer and serves reads, not healing.

- **Indexed by partition and index** — the stream and the sequence number —
  and by entry hash, the request's vocabulary.
- **Filled at production**, when the block sequences the entry onto the
  synthetic chain or builds the anchor: the earliest point at which the entry
  is final, one write on a path the block already takes, a mirror of
  production.
- **Read by dispatch and by healing.** The executor builds every package from
  it (executor.md, "Dispatch"); the sequencer builds every bundle from it.
  Neither reads the historical record for anything: **any read of the
  historical record while building a package, a bundle or a proof is a
  failure**, and it is counted.
- **Contents.** The sequenced message and, when it has one, the transaction it
  belongs to; and, per block and per destination, what a proof is built from —
  the block's span on that destination's synthetic chain and the receipt to
  the Directory root the block was dispatched under — so a package's or a bundle's proof is built from the
  cache alone.
- **Cleared as gaps close.** Because it is indexed by partition and index, every
  entry the destination is known to have delivered can be dropped: the cache
  holds the entries in play, not a window of history. It is sized for the
  entries in play, and sizing it is a separate matter decided from measurement.
- **Never served from storage.** Every entry is also persisted to the
  permanent layer through execution; that is the record, not a fallback. A
  request for an entry the cache does not hold is refused and **counted as a
  miss** with its depth — a miss means the cache is undersized or the request
  is stale, and either is a defect worth a number. The historical record is
  not read to build an answer.
- **Nothing is invalidated.** An entry's content cannot change under its index
  or its hash.

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
| proof requests, spans per request, proofs disproved, duplicate proofs | proof loss, and attempts to lie about hashes |
| anchor requests | anchors lost, requested at once |

### Invariants

1. **A depth is healed once.** A hash is requested at most until it lands, and
   a range healed to a depth is not healed to that depth again.
2. **The source already has the answer.** Serving a request is handing back what
   the producer cached at production; it is never rebuilt.
3. **Every validator computes the same requests; a selected pair sends them.**
   Signatures are contributions and are exempt from selection.
4. **A bundle is an envelope, not a transaction.** Nothing is executed or
   recorded for it; its entries execute in their streams.
5. **Bundles land through consensus and go into staging at intake.** Staging is
   the same on every validator at every block.
6. **Staging is released as runs commit.** It holds only what is above
   `Delivered`.
7. **Healing is bounded per activation** in requests, in time, and to one
   activation at a time.
8. **Anchors are requested at once; entries and proofs after two cycles.**

## 2. Specification — how it is implemented

Deciding is part of the block: staging computes the gaps and the request set as
part of executing an activation block, deterministically. Transport is not: the
API call, the bundle submission and the counters live in
`internal/core/crosschain` and run outside consensus.

### Deciding, in staging

On an activation block (`healActivates(index)`, every `healCadence` blocks),
each stage is read as its two lists from `Delivered + 1` — the entries held,
the hashes proofs have validated, aligned by index because a proof covers one
chain (executor.md, "One chain per pair, one stage per chain"). Two walks:
how far the validated hashes reach, and how far the held entries match them.
Fewer entries than validated hashes is a gap of entries; entries beyond the
validated hashes is a gap of proof. Those spans are the request set, coalesced,
oldest first, at most `MaxRequestSpans`; an index asked within the last
`healPatience` activations is not asked again. A stage holding nothing above
`Delivered` is missing every validating hash above it and asks for the span
above `Delivered` whole; the source answers with what it has dispatched, or
that it has produced nothing there yet. Nothing is timed and nothing is
inferred from the source's ledger, and the walks allocate nothing per entry.
Anchors are a stage like any other (executor.md, "One chain per pair, one
stage per chain"): a missing anchor below a validated one is a gap of entries
and is requested the same way. Sender selection is a function
of the previous block's hash over the validator set yielding two indices; a
node compares them against its own position.

### Requesting and answering

The private sequencer service (`internal/api/private`) carries three methods:
one entry by stream and number (anchors), a **proof for index spans** of a
stream, and **entries by hash set** for a destination. Entries and proofs are
answered from the producer cache; nothing is read from the store.

The source packs the entries into bundles under the envelope budget
(`synthPackageBudget`) and above the minimum size, and submits each bundle to
the requesting partition through the dispatcher
(`internal/node/daemon/dispatcher.go`), the same path `sendSyntheticTransactions`
uses. The requester's call is bounded by `HealTimeout`; a transport failure is
retried a few times because routing picks a peer per attempt; a `NotFound` for a
hash is a deterministic answer and is counted as a miss.

### Landing

The block's sort (`exec_stage.go`, `classify`) places every sequenced entry in
synthetic staging at its index and every collection proof in anchor staging
under its anchor's Directory block index, bundles and packages alike, before
the anchor group is evaluated. Nothing is written for an envelope, and nothing
is written for an entry until it executes. `stageRuns` then
computes runs from what is proven and held, and executed entries and the proven
ranges at or below `Delivered` are released when the block commits.

### The cache

`internal/core/synthcache`, filled by the executor as it produces
(`produceSyntheticInto`, `recordAnchor`) through a per-block transaction that
commits after the store commits, so the cache never holds an entry the chain
does not; keyed by hash and by (stream, number); per block, the synthetic
chain's segment (`merkle.Segment`: the state before the block's first element
and its elements, proving byte-identically to the stored chain) and the
receipt from the chain's anchor to the block's root, built at close from the
root chain segment the block appended; the Directory receipt and anchor the
block was dispatched under, recorded at dispatch (`MarkDispatched`).
Dispatch (`sendSyntheticTransactionsForBlock`) and the sequencer service
(`sequencer_cache.go`) read it and nothing else; a miss is refused as
`NotFound` and counted (`accumulate_synthcache_misses_total{kind}`). Trimmed by
a horizon of blocks (`DefaultHorizon`, an hour) until a delivery signal
exists. At the first block an executor opens, the cache is seeded from the
node's own chains by position for the recent blocks whose anchors have not
returned (`seedSynthCache`): genesis produces through another executor, and a
node that starts has produced blocks not yet anchored — a start-up step, not a
runtime path. The store path in the sequencer remains only for the v1
simulator, which has no cache; the node never wires it. Hits, misses, miss depth and construction failures are counters
on the node's metrics endpoint, as are every row of the counting table above.

---

Where the implementation departs from this specification, see
[DIFFERENCES.md](DIFFERENCES.md).
