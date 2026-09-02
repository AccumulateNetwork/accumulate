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

That is the whole of it, and it is why staging must be askable. A healer that
cannot see what the executor holds has only two options, and both are wrong:
fetch everything above the watermark, which re-fetches what the node already
has, or trust a record the block wrote, which is the coupling that made the
executor read its own output.

It also gives H2's rule for free — skipping what is already staged is not a
separate optimisation, it is what "gap" means.

**After a restart, staging is empty**, so every number above `Delivered` is a
gap and healing refetches it. That is the cost of not persisting staging, it is
bounded by how far behind the node was, and it is paid here rather than by the
executor keeping state it would have to write every block. A node that was
current stages nothing and so loses nothing.

### Generating requests

**Requests are generated in staging, at the end of processing the anchor and
synthetic groups.**

That point is chosen because it is the first moment the gap set is final for the
block: the anchors have executed, the synthetics have been judged against the
chain those anchors extended, and every stream's run has been drained. What is
still missing then is what is genuinely missing.

**Every validator therefore generates the same requests.** Staging is a
deterministic function of consensus input, so every node reaches that point with
the same streams, the same held set and the same watermarks, and computes the
same gaps. Nothing is random, no node needs to know what another node is doing,
and there is no coordination to get wrong.

That is worth contrasting with the alternative, because it is the one a healer
reaches for: if requests are generated outside staging, each node decides on its
own and the design has to stop N validators asking for the same thing at once —
usually with a random per-node delay and a back-off, tuned so the first answer
lands before the others fire. That is a heuristic standing in for agreement the
system already has. Generating in staging uses it instead of approximating it.

**Staging dedupes.** Several things can ask for the same gap in one block — most
obviously an anchor, where each signature can independently reveal the same
missing message. Staging holds one request per gap, so the number of requests is
the number of gaps, not the number of things that noticed them.

**Two validators per block actually send, chosen by the clock.** Every validator
computes the same gaps; that does not mean every validator should ask for them.
The block's time selects the pair deterministically, so each node knows whether
it is one of the two without being told and without negotiating anything.

Two rather than one, because one is a single point of failure: if the chosen
validator is down, or cannot reach the source, the gap goes unrequested for that
block and healing waits on a node that is not going to answer. Two covers that
at a cost of two requests per gap per block rather than N.

Two rather than all, because N validators asking for the same message is N−1
wasted round trips at exactly the moment a stream is already behind — and the
extra answers are discarded anyway, since the block's sort keeps the first
sighting of a sequence number.

The pair rotates with the clock, so a validator that cannot reach a source stops
being asked within a block, and the load spreads instead of settling on whoever
was picked first.

The selection is over which node *sends*, not over what is requested. Every
validator computes the same request set, so one that is not selected has already
done the work and is ready to be selected next block without discovering
anything new.

A request is still not consensus: the node asks the source's sequencer, and the
answer is submitted back and re-enters through consensus, sorted and staged and
executed like any other message. What is deterministic is *which* requests are
made, not the transport that carries them.

### Managing requests

Four rules, each answering a way a healer can make things worse:

- **A failure must not stop the batch.** Delivery is ordered, so failing to pull
  one hole must not stop the others being pulled. A stream must never wedge
  because one request failed.
- **A source that keeps failing is skipped, not hammered.** Consecutive failures
  trip a breaker and the source is left alone for a back-off. Retrying a
  partition that cannot answer converts one node's problem into everyone's.
- **A deterministic answer is not retried.** "Not found" is the source telling
  the truth about its own state, not a transport hiccup; retrying it
  milliseconds later multiplies the request rate for no possible gain. A
  transport failure is retried, and routing picks a different peer each attempt,
  so one transient "no live peers" cannot wedge a stream permanently.
- **A request is bounded in time.** A hung call must not pin the goroutine, or
  its read batch, indefinitely.

### Order

Heal from the **newest gap to the lowest gap**.

Every receipt is a collection proof, and every receipt goes through staging. A
receipt that was dropped means the hashes for its range were never held — so
healing lowest-first would need the specific receipt covering the lowest hole,
and if that is the receipt that went missing it cannot proceed.

Newest-first works either way, and it never requires asking for a past receipt:
drop twenty of them and the next receipt still proves everything behind it. The
source achieves this by adding hashes to the next receipt it was going to send
anyway — no per-destination bookkeeping, no special range, and no request path
to serve.

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

`internal/core/crosschain`. The `Conductor` scans the streams it receives,
identifies gaps, fetches, and submits.

Gaps are found by `missingRuns`, which walks `PartitionSyntheticLedger.Pending`
— the positional array in the ledger account — treating a `nil` entry as a hole.
That is the coupling the section above replaces: it reads what the block wrote
rather than asking the executor what it holds, and it is why the two changes
cannot be made separately.

- `claimSyntheticRequest` decides whether this node asks now. On first sight of
  a gap it schedules a jittered fire time and returns false; afterwards it fires
  once per back-off window. The window is `syntheticHealWindow`, 10 s by
  default.
- `requestSyntheticFrom(ctx, source, num)` pulls one missing synthetic from the
  source partition's sequencer: `c.Sequencer.Sequence(source, destination, num)`.
  It retries up to three times, because routing picks a peer per attempt and one
  transient "no live peers" once wedged a stream permanently (#4067). A
  `NotFound` is a deterministic answer about the source's state and is not
  retried (#4086, #4115). The call is bounded by the heal window, since by the
  time it expires the back-off would allow a fresh attempt anyway (#4066).
- A failed pull is classified into a per-source breaker (`classifyRemoteError`,
  `remoteAllowed`) and the scan continues to the next hole rather than
  abandoning the batch.
- `buildSyntheticSubmission` assembles the envelope from the sequencer's
  response — the sequenced message, the proof, and the source's signature. A
  non-nil proof overrides the per-message one: that is the collection proof
  covering a whole range, which every record of the range shares.
- For a message that belongs to a transaction, the transaction is bundled with
  it, exactly as the normal outbound path does. Without it a healed message
  fails on "load transaction" and the stream stays stuck (#4066).
- The message is submitted with `c.submit`. The log line "Requested missing
  synthetic transaction" is emitted **after** the submit succeeds, so it records
  a completed heal, not a request.

---

Where the implementation departs from this specification, see
[DIFFERENCES.md](DIFFERENCES.md).
