# Healing — Specification

## 1. Architecture — what we are doing

Cross-partition messages — synthetic transactions and anchors — are sequenced
and must be delivered in order. When one goes missing, the destination cannot
proceed: the stream stops at the hole. Healing is what fills it.

Healing is a **destination-side pull**. The destination notices a gap in a
stream it receives, fetches the missing message from the source partition, and
submits it to itself. The source is not asked to track who is behind and does
not retry on anyone's behalf.

### Finding the gaps

A gap is a sequence number the node needs and does not hold. Three facts define
it, and each has exactly one owner:

| fact | owner | meaning |
|---|---|---|
| `Delivered` | the ledger | the highest number this stream has executed. Block output, durable because the block is |
| what is **held** | the executor's staging | numbers received and not yet executed |
| `Produced` | the source | how far the source has gone |

**The gaps are the numbers above `Delivered`, up to what the source produced,
that staging does not hold.** Healing asks staging directly; it does not infer
what the node has from anything the block wrote.

The ledger is opened for **`Delivered` and nothing else** — what has been
processed. A number at or below it needs nothing done about it: it is not a gap,
it is not re-requested, and it is not counted. Everything above it that the node
is not holding is a gap, and everything above it that the node IS holding is
already in hand.

**Which streams to consider comes from staging, not from the ledger.** A stream
that has only staged has delivered nothing and therefore has no ledger entry —
so a scan driven by the ledger's entries would never look at the stream most
likely to be stuck. A stream staging holds nothing for has nothing for this scan
to find: it is current, or it has lost a tail that no local evidence can reveal,
which is the reconcile path's job and is answered by the source's `Produced`.

That is the division between the two, and it is worth stating plainly because
they look like the same mechanism:

- **The gap scan** fills holes BELOW what the node has sighted. Its evidence is
  local: holding a number proves the source produced everything under it.
- **Reconcile** fills the tail ABOVE it. It has no local evidence at all — a
  stream that lost its first messages, or its last, looks exactly like a stream
  with nothing to do — so it asks the source what it produced.

That is the whole of it, and it is why staging must be askable. A healer that
cannot see what the executor holds has only two options, and both are wrong:
fetch everything above the watermark, which re-fetches what the node already
has, or trust a record the block wrote, which is the coupling that made the
executor read its own output.

It also gives H2's rule for free — skipping what is already staged is not a
separate optimisation, it is what "gap" means.

**A restart changes nothing here.** Staging is durable ([executor.md](executor.md),
Restart), so a restarted node holds what it held and has the same gaps it had.
That is not a convenience: staging decides what a block executes, so a node that
came back holding less than its peers would execute a shorter run and produce a
different block hash. Healing is not what covers a restart, and could not be —
it is asynchronous, and the divergence would be immediate.

### Generating requests

**Requests are computed from staging at a block boundary**, on the blocks where
healing activates.

A boundary is chosen because it is the moment the gap set is final: the anchors
have executed, the synthetics have been judged against the chain those anchors
extended, and every stream's run has been drained. What is still missing then is
what is genuinely missing.

Since staging is durable state ([executor.md](executor.md), Restart), the
boundary can be read rather than intercepted: opening the committed state as a
block begins sees exactly what the previous block finished with. So the
computation does not have to live inside block execution, and it should not —
sending a request is a network act, and the executor's business is what
executes. What matters is not WHERE the set is computed but that it is a
function of AGREED STATE rather than of local timing, and committed state is
agreed by construction.

**Every validator therefore generates the same requests.** Staging is a
deterministic function of consensus input and is durable, so every node reaches
that boundary with the same streams, the same held set and the same watermarks,
and computes the same gaps. Nothing is random, no node needs to know what another node is doing,
and there is no coordination to get wrong.

That is worth contrasting with the alternative, because it is the one a healer
reaches for: if requests are generated outside staging, each node decides on its
own and the design has to stop N validators asking for the same thing at once —
usually with a random per-node delay and a back-off, tuned so the first answer
lands before the others fire. That is a heuristic standing in for agreement the
system already has. Generating in staging uses it instead of approximating it.

**One request per gap.** Several things can reveal the same gap in one block —
most obviously an anchor, where each signature independently reveals the same
missing message. Computing the set once, from the gaps themselves rather than
from whatever noticed them, makes the number of requests the number of gaps.
There is nothing to deduplicate because nothing is counted twice.

**Two validators actually send, chosen by the previous block's hash** — which
is the state hash the previous block committed, and is therefore available to
every node as the next block begins, without being distributed or agreed
separately.
Every validator computes the same gaps; that does not mean every validator
should ask for them. The previous block's hash selects the pair, so each node
knows whether it is one of the two without being told and without negotiating
anything.

The hash rather than the clock, for three reasons. It is already agreed —
consensus settled it, and every node has it before this block begins, so there
is nothing new to distribute or to disagree about. It changes every block, where
a clock need not: a partition producing several blocks a second would keep
selecting the same pair while its stream fell further behind. And it is not
anyone's to choose — a validator can nudge its own clock, and the node picking
the senders should not be the node deciding who they are.

Two rather than one, because one is a single point of failure: if the chosen
validator is down, or cannot reach the source, the gap goes unrequested for that
activation and healing waits on a node that is not going to answer. Two covers
that at a cost of two requests per gap rather than N.

Two rather than all, because N validators asking for the same message is N−1
wasted round trips at exactly the moment a stream is already behind — and the
extra answers are discarded anyway, since the block's sort keeps the first
sighting of a sequence number.

The pair rotates with every activation, because the hash does, so a validator
that cannot reach a source stops being asked at the next activation and the load
spreads instead of settling on whoever was picked first.

The selection is over which node *sends*, not over what is requested. Every
validator computes the same request set, so one that is not selected has already
done the work and is ready to be selected at the next activation without
discovering anything new.

**Selection applies to a pull, never to a push.** The two look alike — both are
a node acting to close a gap — and treating them alike breaks the network.

A request is **fungible**. Whoever asks, the answer comes back through consensus
and heals every validator at once, so the other N−2 askers add nothing but load.
That is the whole argument for choosing a pair.

A signature is **not fungible**. Only validator N can produce validator N's
signature, so a node re-sending its own anchor signature is contributing
something no other node can contribute. Selecting a pair there does not save
duplicate work, it WITHHOLDS the rest of the quorum — and a destination that
lost an anchor then waits for signatures that are never coming.

So the anchor push runs on the cadence for every validator, while every pull —
a missing synthetic, an anchor range, a reconcile against what a source says it
produced — runs on the cadence for the selected pair. The test for which one a
piece of healing is: **does another node's action make mine unnecessary?** If
yes it is a pull and a pair is enough; if no it is a contribution and everyone
owes theirs.

A request is still not consensus: the node asks the source's sequencer, and the
answer is submitted back and re-enters through consensus, sorted and staged and
executed like any other message. What is deterministic is *which* requests are
made, not the transport that carries them.

### Cadence

Healing activates **every few blocks**, not every block. The number is small and
not magic — two may be enough.

The reason is the round trip. A request goes to another partition and its answer
comes back through consensus, which takes blocks. Activating every block would
re-request gaps whose answers are still in flight, so a stream that is behind
would generate requests at the block rate for messages already on their way.
Waiting a few blocks lets an answer arrive before the same gap is considered
again.

That is the whole of the rate control. There is no back-off, no jitter and no
per-source scheduling, because there is nothing to control: two requests a gap
per activation is not a load worth managing. The earlier design spread requests
in time to protect a source from N validators; with two senders and a cadence,
the protection is already there and the machinery would only be a way to get it
wrong.

### Sending

**A request is sent immediately.** It is a submission to another partition and
changes nothing here, so it does not wait for the block to commit and is not
part of what the block produces. Nothing in this partition's state depends on
whether it was sent, when, or whether it succeeded.

That is also why a lost request costs nothing. If it never goes out, or goes out
and is never answered, the gap is still a gap at the next activation and is
requested again. Healing does not need delivery guarantees because it is already
the retry mechanism.

Two things still matter when a request fails:

- **A failure must not stop the batch.** Delivery is ordered, so failing to pull
  one hole must not stop the others being pulled. A stream must never wedge
  because one request failed.
- **A request is bounded in time.** A hung call must not pin the goroutine, or
  its read batch, indefinitely — and by the time the bound expires the next
  activation is due anyway.

### Order

Two different things are fetched to close a gap, and only one of them has an
order.

**A receipt only needs the hashes.** So the proof is one fetch: ask for whatever
hashes the proof requires, however far back they reach, and validate them
against the newest receipt already held. There is no ordering question here
because there is no sequence to advance — a proof is complete or it is not.

Every receipt is a collection proof, and every receipt goes through staging, so
a receipt that was dropped means the hashes for its range were never held. That
is why the proof is never chased backwards through past receipts: drop twenty of
them and the newest still proves everything behind it, once the hashes in
between are supplied. The source achieves that by adding hashes to the next
receipt it was going to send anyway — no per-destination bookkeeping, no special
range, and no request path to serve. [Extending a proof](#extending-a-proof-rather-than-replacing-it)
is the mechanism.

**The entries are fetched from the oldest gap to the newest.** Delivery is in
order, so the stream advances the moment the oldest run fills and keeps
advancing as each next one lands. A fetch is bounded per activation, and that
bound is exactly why the direction matters: filling the holes furthest from the
watermark first would consume the budget on messages that unblock nothing, and a
stream deep enough behind would never advance at all. In the run this work comes
from the gap was 8,556 and a scan carries a few hundred.

The two used to be conflated, and conflating them is what made "heal newest
first" look necessary: if a proof could only come from the receipt that
originally covered a range, healing the oldest hole would depend on the one
receipt most likely to be missing. It cannot, because a receipt only needs the
hashes.

### Extending a proof rather than replacing it

How far back a single proof reaches is bounded. `MaxReceiptListElements` (4,096)
caps the elements a collection proof may carry, and it binds at three points
that must agree: the sender will not build a package whose span exceeds it
(`packageSpanFits`), the sequencer refuses a range request larger than it, and
the receiver rejects a proof carrying more.

The bound exists because a receipt list is untrusted input, and verifying one
hashes every element before it can be known to be junk — unlike staging, which
holds only what consensus already accepted. It limits what an attacker can make
a validator do.

It does **not** limit how far back a destination can prove, because a proof can
be extended rather than replaced. A collection proof is a merkle state at the
*start* of its list, the elements, and a receipt anchoring the *last* element to
a root. Widening the range backwards means an earlier merkle state and the
elements in between: the replay still ends at the same anchor, so **the same
receipt keeps working**. Hashes can be added to a collection proof without
another receipt.

This is the hash fetch the [order](#order) names — the half of closing a gap
that has no ordering, because a proof is complete or it is not. It moves hashes
and nothing else: no message bodies, no signature, no new anchor.

So a destination that needs to reach further back asks for an **extension**, not
a new proof. The request carries:

- the **last hash of the proof it already holds** — where the extension
  attaches, and what lets the source confirm the two are continuous;
- that hash's **index**, which the existing proof already establishes, because a
  receipt list's merkle state is counted and therefore binds each element to an
  absolute position;
- **how far back is wanted**, or the maximum a single request may carry.

The source answers with the earlier merkle state and the intervening elements —
raw hashes, no signature, no new anchor. The destination prepends them and
validates the wider list against the receipt it already had.

The same request fills holes, not only the tail. A receipt list must be
contiguous to validate — the replay runs from the merkle state through every
element to the anchor — but a destination may hold **fragments** of a range:
proofs that arrived, some spans dropped in between. Because a counted merkle
state binds each element to an absolute index, a fragment's position is
unambiguous, so what is missing is a set of index spans and each can be asked
for on its own. The destination assembles the pieces it has with the hashes it
receives, and once the list is contiguous from the earliest element it needs to
the receipt's start, it validates.

So a proof is built up rather than obtained: nothing already held is fetched
again, and no fragment has to be discarded because it does not reach far enough
by itself.

This keeps every property the order relies on. A single message stays bounded,
so the attacker's cost is unchanged. Reach becomes unbounded in increments, so
an arbitrarily lagged destination converges. Only what is genuinely absent moves
on the wire. And the expensive part of a proof — the anchored receipt — is
transferred once and reused, rather than re-sent with every widening.

A later collection proof does not invalidate an earlier one. Each verifies
against its own merkle state and receipt, so work already done against the proof
in hand stays valid when new proofs arrive — which is what lets a drain converge
under load rather than restarting each time a proof lands.

### The cache

Healing caches what it fetches, and **only healing uses that cache**.

The reason is what each reader does. The executor reads a record once per block
and its reads do not repeat; a cache serves it nothing. Healing fetches the same
message from the source over and over while a stream is behind — in soak
`20260902T132651Z`, 53,011 fetches for 8,556 distinct sequence numbers, some
41 times each. Those repeat, so they can be cached.

The cache is therefore:

- **In Accumulate, with the healer.** It caches what healing fetched, which is a
  property of healing, not of any storage backend. It works the same whichever
  database is configured, and no storage backend knows it exists.
- **Keyed by what identifies a fetch** — source, destination and sequence
  number.
- **Two generations.** A lookup tries the hot map, then the cold one, and a hit
  in cold is promoted. When hot fills it becomes cold and a new hot starts, so
  entries leave by being unused for a whole generation and nothing is evicted
  one at a time.
- **Never invalidated.** A sequenced message is named by its position in a
  stream and its content cannot change under that name. If it could, the
  protocol would be broken and a stale cache entry would be the smallest
  consequence.

## 2. Specification — how it is implemented

Healing has two halves and they live in different places. **Deciding** is part
of the block: staging computes the gaps and the request set, deterministically,
as part of executing the block. **Fetching** is not: the transport lives in
`internal/core/crosschain` and runs outside consensus, because a request changes
no state here.

### Deciding, in staging

On an activation block, after the anchor and synthetic groups have been
evaluated, drained and executed, staging computes for each stream:

- `Delivered`, read from the ledger — the highest number executed;
- the **held** set, read from staging itself — numbers received, not executed;
- `Produced`, the source's high-water mark as carried by the stream.

The gaps are the numbers in `(Delivered, Produced]` that staging does not hold.
Requests are made one per gap: several askers of the same gap — most often the
signatures of one anchor — collapse to a single request.

Selection of senders is a function of the previous block's hash over the
validator set, yielding two indices. A node compares them against its own
position; no message is exchanged to establish this.

### Fetching, outside the block

A selected node sends its requests immediately, without waiting for the block to
commit, because nothing in this partition's state depends on the send.

`requestSyntheticFrom(ctx, source, num)` pulls one missing message from the
source partition's sequencer: `c.Sequencer.Sequence(source, destination, num)`.

- It retries a transient failure up to three times, because routing picks a peer
  per attempt and one transient "no live peers" once wedged a stream
  permanently (#4067).
- A `NotFound` is a deterministic answer about the source's state and is **not**
  retried (#4086, #4115).
- The call is bounded in time, so a hung source cannot pin the goroutine or its
  read batch. The bound need not be generous: the next activation is due
  regardless, and an expired request is simply a gap that is still a gap.
- A failure moves to the next hole rather than abandoning the batch. Delivery is
  ordered, so one unfillable gap must not stop the others being pulled.

`buildSyntheticSubmission` assembles the envelope from the sequencer's response
— the sequenced message, the proof, and the source's signature.

- A non-nil proof from the response overrides the per-message one: that is the
  collection proof covering a whole range, which every record of the range
  shares.
- For a message belonging to a transaction, the transaction is bundled with it,
  exactly as the normal outbound path does. Without it a healed message fails on
  "load transaction" and the stream stays stuck (#4066).

The envelope is submitted with `c.submit` and re-enters through consensus, where
it is sorted, staged and executed like any other message — which is why the
duplicate answer from the second sender costs nothing: the block's sort keeps
the first sighting of a sequence number.

The log line "Requested missing synthetic transaction" is emitted **after** the
submit succeeds, so it records a completed heal, not an attempt.

### Extension requests

An extension is served from the source's **chain**, not from its outbox. That is
the whole reason it is cheap: the source is not rebuilding a proof, signing
anything, or tracking what any destination holds — it is reading hashes out of a
merkle chain it already has.

#### What a proof requires, and therefore what an extension is

`ReceiptList.Validate` replays `MerkleState` through `Elements` and requires
that the last element equals `Receipt.Start` and that the resulting anchor
equals `Receipt.Anchor`. So a list must be **contiguous** from its merkle state
to its receipt, and its reach backwards is decided entirely by where its merkle
state sits.

Widening backwards is therefore: an **earlier merkle state**, plus the
**elements between** it and the state currently held. Replay then runs from the
earlier state through the new elements into the existing ones and arrives at the
same anchor — so the receipt is untouched and keeps working. This is why the
expensive part of a proof is transferred once.

#### The request

A destination holding a list whose merkle state sits at count `c`, and needing
to reach index `f` below it, asks the source for **the merkle state at `f` and
the elements `[f, c)`** of the chain that carries this stream.

That is the whole request: a stream, and two indices. It does not carry the hash
it is attaching to, and does not need to — **the destination validates the
widened list against the receipt it already holds**, so an extension that is
wrong, stale, or dishonest fails to validate and is discarded. Continuity is
self-checking, which is better than a continuity field the source could satisfy
while being wrong about everything else.

A request is bounded by `MaxReceiptListElements`, the same bound a proof carries,
and for the same reason: it is untrusted input that must be hashed before it can
be known to be junk. Reach is unbounded in increments rather than in one message.

#### Fragments

The same request fills interior holes, not only the tail. A destination may hold
several disjoint pieces of one range — proofs that arrived, spans dropped in
between — and because a counted merkle state binds element `j` to absolute index
`Count + j`, every piece knows exactly where it sits. What is missing is a set of
index spans, each of which is one request.

**A fragment is worth keeping only if it outlives the activation that fetched
it.** If assembly always completes within one activation there is nothing to
store and no state to reason about; if it does not, fragments need somewhere to
live, and that somewhere has the same two properties staging needed — durable,
because losing them silently re-fetches, and unhashed, because they are a
deterministic function of what the source served and prove themselves on
validation. **This is the open question in this part of the design**, and it is
answered by measurement rather than argument: how far back a real destination
has to reach, against `MaxReceiptListElements` per request.

#### Order, again

None of this is ordered. The hashes are one fetch, complete or not; the entries
are fetched oldest-first because delivery is. An extension moves hashes only —
no message bodies, no signature, no anchor.

---

Where the implementation departs from this specification, see
[DIFFERENCES.md](DIFFERENCES.md).
