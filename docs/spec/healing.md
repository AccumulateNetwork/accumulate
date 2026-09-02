# Healing — Specification

## 1. Architecture — what we are doing

Cross-partition messages — synthetic transactions and anchors — are sequenced
and must be delivered in order. When one goes missing, the destination cannot
proceed: the stream stops at the hole. Healing is what fills it.

Healing is a **destination-side pull**. The destination notices a gap in a
stream it receives, fetches the missing message from the source partition, and
submits it to itself. The source is not asked to track who is behind and does
not retry on anyone's behalf.

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

This keeps every property the order relies on. A single message stays bounded,
so the attacker's cost is unchanged. Reach becomes unbounded in increments, so
an arbitrarily lagged destination converges. And the expensive part of a proof —
the anchored receipt — is transferred once and reused, rather than re-sent with
every widening.

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

- `requestSyntheticFrom(ctx, source, num)` pulls one missing synthetic from the
  source partition's sequencer: `c.Sequencer.Sequence(source, destination, num)`.
  It retries up to three times, because routing picks a peer per attempt and one
  transient "no live peers" once wedged a stream permanently (#4067). A
  `NotFound` is a deterministic answer about the source's state and is not
  retried (#4086, #4115).
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

## Known gaps

- **The cache described above does not exist yet.** A read cache was built in
  the BlockchainDB adapter instead (accumulatenetwork/accumulate#4186) and
  removed: it sat on the storage read path where it answered 0.40% of lookups,
  because it was caching the executor's reads rather than the healer's fetches.
- **Healing is not ordered.** It does not heal newest-first, and it does not
  skip numbers already staged locally, so it re-fetches messages the node
  already holds (accumulatenetwork/accumulate#4187).
- **Proof extension does not exist.** There is no extension request and no way
  to widen a held proof; a destination further behind than
  `MaxReceiptListElements` currently cannot be covered at all — the sender will
  not build the package, the sequencer will not serve the range, and the
  receiver would reject the proof. The gap in soak `20260902T132651Z` was 8,556,
  more than twice the cap.
