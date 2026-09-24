# Executor — Specification

## 1. Architecture — what we are doing

The executor turns an ordered stream of messages from consensus into blocks of
executed state. It is the only thing that writes protocol state, and it is
deterministic: every validator given the same messages in the same order
produces the same block and the same state hash.

### The path

```
consensus ──▶ sort ──▶ ┌─ bundles:    hold in staging               ─┐
              (streams)│  anchors:    evaluate ▸ drain ▸ execute    │─▶ batch ─▶ commit
                       │  synthetics: evaluate ▸ drain ▸ execute    │   (one, at close)
                       └─ user:       evaluate ▸ drain ▸ execute   ─┘
```

A message must satisfy each of the following before it executes. Validity is
decided on the message alone; everything after it is decided *by staging*:

1. **Consensus** decides which messages exist and in what order. It is the only
   input. Batches are executed in the certificate's canonical payload order —
   any node-local order diverges chain entries and BPT roots across validators.
2. **Validity** is settled first, on the message alone. A proof that does not
   hash to its own claimed anchor is refused outright — `BadRequest`, never
   staged. An index further ahead than the source could have produced in about
   an hour is refused the same way: a partition that far ahead is a fault, not
   something staging waits for.
3. **Collection.** Staging is two stores. **Synthetic staging** holds entries by
   stream and index; an entry whose index no validated proof covers yet is
   *collected* and waits. **Anchor staging** holds collection proofs by the
   Directory block index of the anchor each terminates in; a proof whose anchor has
   not executed yet is collected and waits. Entries and indexes are one to one:
   every index will be covered by some later proof, because the source's chain
   grows and every later anchor covers everything before it, so a collected
   entry is never a category of its own — only an entry whose proof has not
   arrived yet.
4. **Proof.** When an anchor executes, every proof waiting on its sequence
   number is validated against it. A validated proof moves to synthetic staging
   and marks its index range **proven**; the collected entries whose hashes sit
   at those indexes are proven with it, and a later proof extends the proven
   range. A proof the anchor disproves is discarded and counted — the entries
   it claimed are simply not proven by it. Two proofs claiming the same indexes
   with different hashes are an attack, counted; squashing them with a
   validator signature is future work.
5. **Readiness** is whether the message is *next* on its stream. Sequenced
   streams execute in order with no gaps; a proven entry whose predecessor is
   missing waits. Once index *i* has executed, any entry at or below *i* is
   **tossed** — it is already processed and nothing consults it; any entry
   above the last validated index is **held**, waiting for validation, as
   long as it is within the horizon. A user transaction is on no stream and
   is always ready.
6. **Execution** runs the message. Nothing before this point changes protocol
   state, and nothing is ever recorded as pending outside staging: an entry the
   block cannot execute is in staging, or it was refused.
7. **The database write is a side effect of execution**, not a stage of its own.
   Executors write into the block's batch as they run; the batch is committed
   once, when the block closes.

The four groups run in sequence, each finished before the next is evaluated.
Executing one group changes the state the next is evaluated against — anchors
extend the chain synthetics are judged by — and that is the sequence doing its
job, not a feedback loop. Within a group nothing is re-asked.

What must never feed back is the block's persisted output. Staging reads the
streams' positions and the anchor chain as the block builds them; it does not
read the ledger record the block writes at its close.

### Validation is a separate, earlier thing

`Executor.Validate` is the pre-consensus check — CheckTx's equivalent. It runs
against a read-only batch that is always discarded, so it writes nothing and
changes nothing. It exists so a node can refuse a malformed or unpayable
envelope before it is gossiped and sequenced, not to decide anything about
execution. The batch is **unisolated**: validation judges against the latest
committed state, so it pins no version and no block commit takes pre-images
on its account ([database.md](database.md), "Isolation has a price").

A message that reaches execution is validated again on the path above, by the
gates that can only be evaluated with block state in hand.

### Draining

A stream's messages arrive out of order. One that is not next cannot execute, so
staging holds it — and when the message it was waiting for arrives, everything
held behind it becomes executable at once.

**Draining a stream is executing that backlog**: the contiguous run of staged
messages starting at the delivery point, in order, until the next gap. It is the
only way a stream advances by more than one message, and it is why a single
missing message stalls a stream and a single arrival can release thousands.

The word is used for one other thing in this document, and they are unrelated:
the **delivery queues** drained at `Begin` hold locally produced messages, not
staged stream messages. Where the distinction matters the text says which.

### Sort, then four groups in turn

Everything consensus delivers is **sorted once**, in a single pass. Each message
is asked which stream it belongs to; if it belongs to one it is recorded as an
arrival on that stream, keyed by its sequence number, and the first sighting of
a number wins — the same message may appear twice in a block and applies at most
once. A message belonging to no stream is a user transaction.

So anchors and synthetics are sorted up front. There is no later step that
discovers more of them.

Then **four groups, each finished before the next begins**. A group is
evaluated, drained and executed; only then is the next group evaluated.

0. **Intake.** Every arriving entry and proof — dispatched packages and
   healing bundles alike — is written into staging before anything is
   evaluated: entries into synthetic staging at their index, proofs into
   anchor staging at their anchor's Directory block index. A healing bundle is not a
   transaction and is never sent to the executor: it is an envelope carrying
   missing entries or a proof, there is no message type or executor for it,
   and nothing is recorded for the envelope. Intake is first so that what the
   groups below execute is decided against everything the block brought.
1. **Anchors.** Evaluated — each is admissible or not, by quorum or proof —
   drained, and executed.
2. **Synthetics.** Evaluated *after* the anchors have executed, so every proof
   waiting on one of this block's anchors has been validated and its indexes
   proven before an entry is judged. That frees as much of the synthetic
   backlog as can be freed. Drained, and executed.
3. **User transactions.** Drained and executed last, so a deposit has landed
   before a transaction spends it. A send that would fail on a stale balance
   succeeds instead: strictly more permissive, and deterministic either way.

Within a group, streams run in canonical source order — the directory first,
then partitions by ID. Any fixed rule would do; what matters is that every node
uses the same one.

**Each group is evaluated once**, and the sequence is what makes that
sufficient:

- an entry is proven by a proof, and a proof by its anchor; the anchors that
  validate this block's waiting proofs have executed by the time synthetics
  are evaluated;
- a stream's run is computed from its arrivals **and what is already staged**,
  so a message arriving this block that unblocks a backlog from earlier blocks
  is part of that stream's run when it is computed — nothing about it becomes
  true later;
- intake writes every entry and proof into staging before anything is
  evaluated, so the runs they complete are seen by the anchor and synthetic
  evaluations, and executed entries are released from staging at commit;
- a user transaction is on no stream and cannot unblock one. What it produces
  for this partition goes on the delivery queue and executes next block, so it
  cannot free a synthetic within this block either.

Nothing is re-asked, because nothing that could change an answer happens after
the answer is given.

`maxRunPerBlock` (1,024) bounds how far one evaluation may carry a single stream
in a block, so no block inherits an unbounded run.

The predecessor of this design re-read the ledger after every single delivery,
which is where its O(n²) came from. One evaluation per stream per block reads
the position once.

### One chain per pair, one stage per chain

Every stream between two partitions is **its own chain at the source and its
own stage at the destination**, and nothing is shared between streams.

- A partition keeps a **synthetic chain per destination**: BVN1 has a chain of
  what it sends BVN2, another of what it sends the Directory, and so on, each
  anchored into BVN1's root chain when it changes. The Directory keeps an
  **anchor chain per BVN** and each BVN keeps one for the Directory, as they do
  today. One interleaved chain for every destination, which is what exists
  now, makes a proof carry other destinations' hashes, and a destination
  cannot tell which hashes are its own without already holding the entries.
- A **collection proof covers a span of one chain**, so its hashes are exactly
  the destination's entries in sequence order: the proof's index *is* the
  sequence number.
- The destination keeps **one stage per chain**: an indexed list of the entries
  it holds, from `Delivered + 1`, and beside it the list of hashes proofs have
  validated at the same indexes. Anything at or below `Delivered` is dropped.
  Aligning the two lists is a walk, not a lookup, and allocates nothing per
  entry.
- Two walks from `Delivered + 1`: how far the validated hashes reach, and how
  far the held entries match them. Where the two agree, that run executes, in
  order. Fewer entries than validated hashes is a **gap of entries**; entries
  beyond the validated hashes is a **gap of proof**. Those two spans are what
  healing requests, on its cadence, and nothing else is.
- **Anchors go through the same stage.** An anchor is an entry at its sequence
  number in the anchor chain's stage; it is validated by a collection proof
  over that chain or by a validator signature quorum, and executes once, in
  order, when validated. A missing anchor is a gap of entries like any other.
  There is no separate anchor mechanism.

The stage is one implementation. It does not know which chain it serves.

### Sync

**Staging is a state that builds up after a node syncs with the protocol.** It
is memory: what the node has received and not yet executed, the proofs waiting
for their anchors, and the index ranges those proofs have proven. Nothing in
it is written to the database. What is written is what executes, at the
block's single commit, and `Delivered` — how far each stream has executed —
is block output and is hashed with the rest.

Staging decides what executes: a block delivers the contiguous run starting at
`Delivered + 1`, taken from this block's arrivals and from what is held. Two
nodes holding different things execute different runs from the same block, so
staging must be the same everywhere.

**A node that joins does not catch up through consensus, and neither does a
node that restarts.** Consensus is a stream of blocks to execute; a node that
executes them from a state and a staging that differ from its peers' by even
one entry executes a different block, and the root chain is a Merkle root
over the history of block roots, so it never matches again (#4290). **A
restart is a join.** The consensus checkpoint restores the DAG position
(consensus.md, "Restart"); what the node executes is decided by the join, not
by the position.

A join asks one question — *what is the state of the protocol at a height, and
why should this node believe it* — and answers it in four steps: validate the
spine, pull the state that spine's root commits to, collect consensus until
what it collected and what it pulled line up, execute from the next block.
Nothing in it rests on a peer's word, and nothing in it asks a peer what it
holds.

#### 1. The spine is the trust root, and it is validated first

**The spine is the network definition and the anchors its validators sign.**
An anchor carries the source partition's `StateTreeAnchor` for one of its
blocks, and it is signed by a quorum of that partition's validators. The set
those signatures are counted against is the **network definition**
(`dn.acme/network`: each partition's validators and its threshold — the
protocol's own anchor authority, `core.AnchorSigner`, `ValidatorThreshold`,
the same set the executor checks an anchor against when it arrives), **not
the operators' key page**: genesis writes every node of every partition into
every partition's operator page and takes the threshold over all of them, so
a BVN's own four validators can never reach it (#4301, measured). The
operators' book is not gone, it is relocated: governance still terminates
there — the definition changes only by a write to `dn.acme/network` that
`dn.acme/operators` authorizes — but a joining node never reads it; it reads
the definition, which that book last authorized. One anchor, verified, is the
proof of the state of the entire protocol at that height: every account under
that root — the spine's own accounts included — is a leaf check from there.

**A signature is verified against a key, not against a root.** That is what
makes the spine establishable before anything else exists. An earlier version
of this document said the spine "cannot be verified before it is there", and
the code repeated it (`join/state.go:625`): the four spine accounts were
pulled in `ModeFullSpine` and settled with `Keep`, unverified, because they
were "what the verifier reads from". The circularity was never real, and what
it cost is #4301 — the pull's chain of trust terminated in one unauthenticated
peer, which supplied both the root and the state that hashes into it, so the
whole scheme proved only that the peer agreed with itself.

The node has a set to start from in every case. A restart holds the network
definition it executed with; a node starting from genesis holds the genesis
definition. **Churn is a leaf, not a walk** (#4301 statement (c), through the
protocol 2026-09-21): an operator change on this line is not carried by any
anchor — the anchor of the block that changes the set is signed by the *new*
set, and nothing signs the change in the old set's name — so a node that
trusts an older set cannot follow the change signature by signature. It
follows it the way the executor does: an anchor is judged by membership in
the set the node trusts, to that set's threshold, with the anchor's declared
version a *floor* (older is refused, newer is not a selector); and the trusted
set moves only when `<partition>/network` is pulled as a leaf under a root a
quorum of the trusted set signed and its version is greater. The stated
limit, the executor's own: a change that turns over more of the set than the
old threshold can bridge cannot be crossed from the old set, and a node that
trusts only the old set must be re-seeded from a definition it can verify.
The spine is pulled in `ModeFullSpine` — head, secondary state and **every
chain entry replayed** — not for verification, which the definition and its
signatures give, but so that the spine's every-block chains are comparable
entry for entry with what the node executes from there.

An anchor is accepted when valid signatures from **distinct members of the
producing partition's validator set** — the network definition's, not a key
page's — reach that set's threshold. Copies from one signer do not
accumulate; a second copy from a validator is no second signature.

**Anchors are routed by producer.** To verify partition P's root, the node
needs an anchor *produced by* P, signed by P's validators, and a produced
anchor lives on the **receiving** partition's anchor pool. A BVN's root is
read from `dn.acme/anchors`. The Directory anchors to every partition
*including itself*, so its own root is in `dn.acme/anchors` too, with real
signatures — measured on this line (#4301); a BVN's pool holds the same
Directory anchors and is a second, independent place to read them. An
earlier version of this paragraph said the Directory's root could never be
obtained from `dn.acme/anchors`; that was inferred from the routing rule and
not from a run, and it was wrong. The defect was never which pool was read;
it was that no signature was checked.

**Until the spine validates, nothing is kept and no root is handed on.** A
root that has not been verified against the trusted set is not a root; it is a
number a peer sent. What one peer can still do is withhold, or serve only old
anchors that a real quorum once signed — a slower join, never a fork — and a
second peer is what closes that: the join draws its anchors from more than one
peer where it has them, so that agreement rather than availability decides
it. On this line the anchor source keeps ONE cursor while the peer rotates
beneath it on every call, so a lagging peer's chain count rewinds the cursor
a full window and the entries are re-verified — reading several peers is the
right answer to withholding and #4379 is what makes it cheap, not what makes
it safe. A literal cross-check of two pools before any root is trusted is not
built (#4301, stated); the quorum's signatures are the mechanism and
withholding is its limit.

#### 2. Everything else is a leaf check against a proven root

**The accounts a peer serves to a join are current.** The join asks for an
account as of the block the peer is on, and the answer carries a receipt
running from the account's state hash to that peer's BPT root. The join asks
no peer for an account as of a past block: a BPT is a tree of current state,
and a node does not retain chain heads, directory lists or pending lists per
block to rebuild an old leaf from (Paul, 2026-09-21: "The signed anchor has
the anchor and BPT root for a block + the history. All the accounts for an
anchor are current. So the anchor + the history is everything for the current
block every block.").

**A root is proven by two things, and nothing else: it equals the
`StateTreeAnchor` of a signed anchor, or the bpt chain's history from one
such root to it hashes into the root chain anchor a later signed anchor
carries.** A verified anchor carries, under a quorum's signatures, the BPT
root of the block that sent it — the root that block committed, which is the
root a peer's state is current at while its ledger names that block
(`TestAnAnchorsStateTreeAnchorIsTheRootOfItsBlock`) — and the root chain's
anchor and height as of that block. The next non-empty block records that
root on the ledger's bpt chain, and the bpt chain anchors into the root
chain, so the roots between two anchors are the bpt entries between their
`StateTreeAnchor`s (Paul, 2026-09-21: "the anchor + the history is
everything for the current block every block"). A root a pass was served at
is proven when it equals that value on an anchor this node verified. Failing
that, with the latest verified anchor L and the latest verified anchor B
below the served block, the join reads the bpt entries from B's root to the
served root from the producer's peers and holds them to L
(`anchorsrc.ProveRoot`): each entry's receipt is asked for at L's signed root
chain height, and that height alone decides which step of the receipt is an
entry of the root chain — the index the peer reports shapes the rebuild and
the range asked for, and enters no conclusion; B's root is the last bpt entry
at its own anchoring, so the steps of its receipt below that entry rebuild
the bpt chain's merkle state there; the entries between are appended, the
served root must be the last, and the rebuilt anchor must be the hash the
served root's own receipt enters the root chain at. Every hash in that chain
of reasoning is a node on a receipt to a signed anchor, so a peer that names
another index, serves other entries, or serves a transaction's true receipt
as a root's produces a hash the root chain does not hold
(`TestATransactionsReceiptIsRefusedThoughThePeerForgesTheBptIndex`,
`TestAReceiptThatIsNotTheBptChainsIsRefused`). Nothing under a root proven
neither way is trusted (Paul, 2026-09-22: "How can anything in the BPT not
be proven? The BPT root is part of the signed anchor?"). Everything under a
proven root is proven by BPT receipt to it. What is open: an interior node of
the bpt chain, whose leaves are true roots, can pass for an entry when the
peer chooses the range (DIFFERENCES.md E11).

An account is kept only if three things hold: its receipt is valid; it ends at
a root proven as above; and it passes through the leaf the pulled state hashes
to locally. The third is what makes the pull safe, because a peer can serve a
true receipt for an account and a false body for it.

**A leaf is pulled whether or not the account has a body** (#4397). The leaf
hashes the main state, the directory, the chains and the pending list, and any
of them can be there without the others: an authority signature recorded on a
principal that does not exist leaves a leaf with `signature` chains and no
body, and a failed deposit leaves an empty account's leaf. Asked for an
account with a receipt, a peer whose tree holds a leaf for it answers with no
body and the receipt for that leaf; the join pulls the rest of the account as
for any other and keeps it by the same three checks, the missing body hashing
as the zero hash. A peer that answers "no body" with the receipt of an
account that has one fails the third. The check proves less here than for a
body, and that is stated rather than hidden: the tree hashes a leaf's value
and not its key, and a leaf with no body carries no URL, so the receipt of
one such leaf passes for any account whose pulled state hashes to the same
value — every empty account's leaf is one hash. What refuses a leaf placed
under the wrong name is the whole-root match, so the exposure is a join that
does not finish, not a node that executes from a wrong state (DIFFERENCES.md
E11). `NotFound` means the peer's tree holds no leaf: a name every
source answers that way is dropped rather than asked again, and the page diff
names it again if a leaf ever appears; a name some source failed to answer is
asked again.

**One pass is one root, and it is written whole.** What a round fetches is
held, unwritten, until the root its receipts end at is proven, and nothing new
is fetched while it is held. The peers move while a pass is fetched, so its
accounts can end at different roots, each of them true; written together they
are a state no block ever had. The root most of the pass ends at is the pass,
and the rest — with any account served with no receipt — are fetched again. A
spine account that leaves a pass this way fails the spine for that pass: the
rest of it is written, and the spine is asked for again, whole.

**A pass is held only while waiting can end.** There is one wait: no
verified anchor reaches the root yet, and the next one may — a root served
at block N is recorded on the bpt chain in the block after it, so the pass
is proven once any anchor of a block after N is verified, whether or not
block N sent one. A root the history has *passed* — an anchor of a later
block is verified and the bpt chain does not record this root — is not a
wait, because no anchor to come changes it: the pass is dropped and fetched
again, from the next peer in rotation. **No count of rounds is involved**; a
bound on rounds discards exactly the accounts that change every block, which
is every account a restarted node lacks (#4352, #4353). **Which blocks send
an anchor is the executor's rule, not the join's**: a block that changed any
account beyond the ledger and the anchor pool, or produced a synthetic
transaction, or received a partition anchor, sends one; a block that only
received a directory anchor sends one on the heartbeat, at most every fourth
block (`shouldSendAnchor`, `anchorHeartbeatSkip`). Under load every block
anchors and a pass is proven on the next anchor by equality; idle, the
passes served between heartbeats are proven by the history on the next
heartbeat's anchor. **The join settles at the root it was served, and does
not wait for a fetch to land on a block that anchored.**

**The join converges by repetition.** Every round pulls what the block ledger
says changed since the last root it settled at, plus the page diff on its
cadence, and settles the pass when its root is proven; it is done when the
local BPT root equals a verified anchor's `StateTreeAnchor` and the block is
read from the ledger in that state (§5, `tracker.Check`).

**The node hashes to the leaf or it does not, and no ordering question arises
in the check.** There is no "am I ahead of this peer", no level case, no
chain-height comparison in deciding whether an account is kept; that apparatus
carried a hole of its own, because an account's body can move with all of its
chains standing still (#4350). The block the node's state *is* comes from the
state itself: when the local root equals the root a pass proved, the block is
read from the ledger account in that state, which hashes into the proven root,
and never from a block number in a peer's answer.

**A peer can also answer as of an anchored block, and the join does not ask
it to** (#4361). Asked with a height, a peer serves an account's body as of
that block with a receipt that terminates at that block's `StateTreeAnchor` —
the body and the receipt coherent or the answer a refusal — and a BPT page as
of that block; a block outside what it retains is refused as such. A refusal
is never disguised as a fact about the record: `IncompleteChain` names the
window the peer retains (1024 minor blocks by default, configured per node) or
a leaf the peer could not rebuild for that block, and the peer never answers a
historical ask with its current state; `NotFound` says only that this peer's
index has no record of the account at that height, and a peer that cannot
read its own index — a joined node holds a chain from its open mark, not from
element 0 — answers from the BPT rather than calling its own gap an absence,
because a requester reads `NotFound` from every peer as the network's answer
and would drop an account they all hold. What such a receipt proves is the
body under that root and nothing beside it: the components as of that block
are pulled, never read off the receipt. It is a reader's capability. It is not
what a join stands on, because the leaf of an account whose chains, directory
or pending list have moved since that block cannot be rebuilt for it, and
those are the accounts a restarted node lacks.

**A body served with a proof is the stored body, byte for byte.** Nothing
derived may be filled into an account on the way out of the API, because the
receipt served in the same call is built from what is stored: a body with one
synthesised field in it does not hash to the leaf its own receipt proves, so
the check above refuses it from every peer, forever. `Received` on a sequence
ledger is derived from staging and was filled in on read, and it left every
restarting node unable to pull `<partition>/anchors` from anybody (#4295). A
derived value travels **beside** the body, in its own field, and a reader
merges it after it has checked the proof.

**The answer carries everything the leaf hashes, and the pull writes all of
it** (#4399). Besides the body, the directory, the pending list and the
chains, a partition's `synthetic` leaf hashes its two delivery queues and its
`ledger` leaf the root of its scheduled events. A current answer with a
receipt carries them in its `Leaf` — the queues, and the events themselves
rather than their root, since a root cannot be written — read from the batch
the receipt is built from; the pull replaces what the node held with them,
and a queue or an event set served empty clears the node's. A queued local
delivery executes from its stored message at the next block, so the pull also
fetches each queued message and keeps it only if its hash is the queued ID.

**BPT pages are read, never written.** A leaf enters the local tree only as
the hash of state this node holds and has verified, because the local root is
what the node matches against a proven root; a leaf taken from a peer's
word would make that root the peer's and the match would say nothing. Pages
name accounts and say what the peer's leaves are; the difference from the
node's own is the set to pull. A page carries no proof, so a peer can omit a
leaf, and the root failing to match is the only detector (#4301) — a mismatch
must therefore name what it could not account for.

#### 3. The state pull

**Every read is addressed at a named peer, and never at this node.** A node's
own client answers locally for any service the node provides, and a joining
node provides the querier for every partition it serves — so a pull given that
client reads the un-executed store the pull exists to fill, and is refused by
it forever (#4303: 18,313 refusals, not one of them a verification failure).
The joining node routes each account to a partition, looks that partition's
query service up under its network's key, **drops its own peer ID**, and asks
one named peer at a time. A peer that cannot serve an account is that peer's
condition and the next peer is asked.

**The partition's own spine first**, then the accounts the blocks changed. The
Directory's anchors are read from a Directory peer and are **not written into
this partition's store**: a partition's state tree holds no account of another
partition, so a leaf no peer of this partition has puts the local root beyond
every root ever anchored for it, however perfectly everything else is pulled.
A node runs the Directory alongside its BVN, and the Directory's own join
pulls the Directory's spine into the Directory's store, where those accounts
belong.

**The set of accounts is the block ledger's, not the block's envelopes'.**
Every block records `(account, chain, index)` for every chain its execution
changed (see "The block ledger"), that record is a chain on the partition's
ledger account, and the chain's anchor is part of the account's hash — so the
state root commits to what each block changed and a receipt from the chain
proves it. A joining node asks for the block ledger records covering
`(R, Q]` — `Q` being the block the peers are on — verifies each against a
proven root the way it verifies an account, and pulls the union of their
accounts. That set includes the accounts
a block changed as a side effect and the system accounts every block touches —
`<partition>/ledger` and `<partition>/synthetic` — which a block's envelopes
never name, and it contains no unroutable name, which envelopes do.

**The page diff is the backstop and it must stay reachable.** It runs on the
first round, because a node that has just started does not know whether the
store it holds is the state of `R`; on a cadence after that, counted in rounds
that fetch; and instead of the walk whenever `(R, Q]` is wider than a walk is
worth. Running it only when the ledger named nothing makes it unreachable,
because one name that can never be satisfied keeps the set non-empty for the
life of the process (#4306). So does a cadence counted in every round: a round
that is still settling an earlier pass fetches nothing and decides nothing, and
a pass that settles in a fixed number of rounds can make the fetching rounds
miss every multiple of the cadence for ever (#4395). It is
also what covers an account whose body moved with no chain of its own moving,
which the block ledger cannot name.

**`R` is the block this node's EXECUTOR last executed, and it is read once.**
It is not `<partition>/ledger`'s `Index` read again each round: that ledger is
an account, and it is one of the accounts the pull overwrites, so after the
first round the store answers the PEER's block. The executor writes the block
it commits into `SystemData(partition).ExecutedBlock`, in the block's own
batch, so it commits exactly when the block does; the record is not an account
and is not in the state tree, so nothing a peer serves can reach it (#4344).
Reading `R` from the store instead made a node 853 blocks behind believe it was
17 behind, which kept the span inside the walk's limit, so the page diff never
ran as the primary and the walk covered seventeen blocks of the wrong end of
the history (#4295).

**What is pulled is written into the state tree, not only into the store.**
Committing an account does not move the root by itself; the root is what the
node matches against the proven root, so a pull that does not update the tree
can fetch everything the network has and never move (#4305).

**What is pulled is what the node executes from.** The pulled state replaces
what the node holds for that account rather than joining with it, or a restart
keeps entries the peer has dropped and the account never hashes into a
proven root again. A chain is taken with the entries of its open mark set —
the entries since its last mark point — because an append rebuilds the chain's
tail from them, and a node that cannot append to its chains cannot execute
block `Q + 1`.

**Syncing and bootstrapping are one walk at two depths.** The spine's chains
are taken entry by entry, and the node fills them back from the account's head
until it **meets data it already has**. A bootstrapping node never meets any
and collects the whole chain; a restarted node meets its own at once and
collects nothing. Same walk, different stopping point — which is why a defect
at the meeting point is invisible to every bootstrap test and fatal to every
restart.

**An account's pending list is part of its leaf**, and the material behind it —
validator signatures, payments, votes, signatures — must be pulled with it or
the account's hash cannot be computed to match. An account with a non-empty
pending list that is pulled without it diverges, and the node never converges
(#3999, #4298).

#### 4. Staging is what was collected, minus what the state says executed

A stage holds two lists per stream, indexed from `Delivered + 1`: the entries
received, and the hashes proofs have validated ("Staging is one structure").
A joining node fills both from consensus alone, and reads `Delivered` out of
the state it pulled.

**Synced to `B`, the next block is everything not yet executed.** The pulled
state at `B` says, per stream, exactly how far execution reached. Block
`B + 1`'s transactions are by definition ones not executed as of `B`. If the
run each stream can deliver from `Delivered + 1` is contiguous — no sequence
number missing between `Delivered` and what `B + 1` carries, no proof naming
a hash the node does not hold — then nothing any peer is holding is missing
here, and the node executes `B + 1` as its peers did. **If there are no gaps
there is nothing in staging to worry about.**

**A gap is an entry that arrived before the node was listening.** `B + 1`
delivers #105 while the node's `Delivered` is 103 and #104 is not in `B + 1`:
the peers held #104 from a block before `B`, and executing `B + 1` without it
is the #4290 divergence. The node does not execute. It takes `B + 1`'s block
ledger, pulls the accounts it names at anchored `B + 1` — whose `Delivered`
now says what the peers actually ran — keeps `B + 1`'s transactions in
staging, and asks the same question of `B + 2`. **It advances the sync one
block at a time until a block has no gap, then executes.**

That loop ends, and quickly. The node has collected every committed block
since it started, so an entry a peer holds that *arrived after that point*
the node holds too; a gap can only be an entry from before. Held sets are
small — a handful of entries at 100 tps — and clear within a few blocks, so
within a few rounds the last pre-listen entry has been executed by the
network and is in the pulled state, and every stream's run is contiguous.

**The root is the check that does not depend on the sequence numbers.** After
executing any block, the local BPT root equals that block's proven root or
it does not. A mismatch is a gap the sequence check missed — the node
re-syncs at that block and continues — so a wrong run is caught at the block
it happens in, never carried forward.

**The bodies are content-addressed, so they come from anybody.** An entry the
node needs and did not receive — a validated hash with no body behind it — is
fetched by hash from any peer and checked against the hash the proof already
fixed. There is nothing to trust in the source: a wrong body fails its own
hash. **The run is a function of the validated frontier, not of what the node
happens to hold**: on a validated index whose body is missing, the block waits
and the body is fetched; it does not deliver a shorter run, because a run
that stops at what a node holds differs per node.

**An anchor below its quorum needs no quorum recovered.** It is held in the
anchor stream's stage at its number, runnable once the signatures reach the
threshold **or a validated hash at its number is its own**; and a collection
proof under a known directory root authorizes it outright, because the proof
depends only on the current directory root, which every synced node has
(#4056). A historical quorum is never re-gathered.

**No peer is ever asked what it holds.** The earlier design's step 2 — a
validator serving its staging as of its last committed block — answered a
question the node can answer for itself, and answered it from one
unauthenticated peer with nothing to check it against (#4322). The block
ledger, the accounts and the anchors are verified; the transactions come from
consensus; `Delivered` is hashed state. Staging is what was collected minus
what the state says executed, and that is all it ever needs to be.

#### 5. Converge, then execute

When the local BPT root equals the root a pass proved (§2), `Q` is the block
the ledger in that state names, and staging is brought to `Q`: everything collected through `Q` held,
everything at or below each stream's `Delivered` at `Q` — read from the pulled
ledgers — released, and proofs decided against the anchors executed by `Q`. `Q`
is checked against the state, not taken on trust: staging settled against
another block than the state it is paired with executes a different block than
the peers, which is the failure the join exists to prevent.

The node then executes block `Q + 1` from the buffer as any node executes a
block, and it is a validator or a follower from there. A follower differs from
a validator in what it does with the blocks it processes — it does not vote or
propose — not in how it gets there; what it does with a transaction it cannot
propose is step 6's rule: it relays it, and never drops it.

While all of this runs the node **listens**: it subscribes to consensus and
takes every committed block from then on into a buffer, and into staging —
collected, not executed. A collected block has no index; the node executes
nothing, so it does not report execution and its primary proposes no batches
(consensus.md, invariant 9).

**A node that has executed no block does not join at all**, and that is the
decision to enter the join rather than a flag inside it. **Genesis is not an
execution.** Loading the genesis snapshot writes the system ledger at block 1,
so "has executed no block" is `lastBlock <= GenesisBlock`; reading `lastBlock >
0` as "this node has been running" sent every node of a fresh network into the
join to ask the others for a state none of them had (#4304). The two cases this
cannot tell apart are the first node of a **new network** and a node added to a
**running partition** holding nothing but genesis: both read block 1, and there
is no local fact that separates them — the distinction is whether the partition
has moved on, which is a network fact (#4340).

#### 6. Serve last

**Fully synced is a verified state, not a backfilled history.** A node is
fully synced when its state is the state a signed anchor commits to and it
executes every block from there at the network's cadence — the only proof
this protocol has of its own state is the signed anchor at a height ("We only
need the signed anchor at the current height to prove the state of the entire
protocol at that height"; Paul, 2026-09-18/19, the spine decision), and this
line has no backfill of
history: the producer cache fills by execution alone, a join pulls state and
not chain entries, and nothing under this section fetches entries a node did
not execute — that is phase 3's conversion of history, or phase 2's database
node, not a syncing node's work. So the node states are two: **`BOOTING`**,
from the start of a join until the local root matches a verified anchored
root; **`ACTIVE`** from that block on, and from its first block for a node
that took nothing from a peer — a node that never joined has no state
machine at all and serves as `ACTIVE` (#4368). `COMPLETE` and `WAITING`, which
named a backfilled history, are retired: nothing reached them and nothing
could. What a joined node cannot answer *for a block it did not execute* —
an entry the sequencer is asked for from before it joined — it refuses per
request with `NotReady` naming the block it joined at, never `NotFound`,
which a requester counts as a miss (#4295, DIFFERENCES E11). The node's state
is a gauge and is advertised, but advertising is
not what keeps a request away: a peer finds any installed handler by libp2p
identify ahead of the DHT (`connectedPeersDiscoverer`), so what protects a
caller is the node's answer — `NotReady` for a read it cannot make, a relay
for a transaction it cannot propose — never the absence of a record.

**A read needs local state; a relay needs none. That is the whole rule.**
Everything a node is asked divides on it, and the two halves have opposite
answers.

**A read is refused unless the node can answer it from what it holds.** A
joining node does not answer the two reads another node's **pull** takes — a
BPT page, and an account with a receipt — because its leaves and its root are
the half-filled ones its own pull is building, and a second joining node would
otherwise take its spine from the first (#4297). Nor does it answer for
missing data: not the sequencer, not healing. **In this phase a syncing node
refuses every read** and answers once it is fully synced (Paul, 2026-09-19):
`BOOTING` refuses with `NotReady`, `ACTIVE` serves. "Every read" is the rule;
the code gates two query kinds (`servingFor`: a BPT page, an account with a
receipt) and a joining node still answers ordinary account reads from a
half-filled store — a code change under #4295, not a narrowing of this
sentence. Tracking
which nodes are not synced, so a *reader* can be sent to one that can answer,
is the next phase's work and nothing here anticipates it.

**A transaction is relayed, never dropped, whether the node is following or
syncing** (Paul, 2026-09-19: "Followers can relay txs. And should."). A node
that cannot propose a transaction — because it holds no committee key for the
partition, or because it has not caught up — hands it to a node that can, and
that costs it nothing it does not have: **a relay reads no account, verifies
no signature and needs no state**, which is exactly why the sync rule above
does not reach it. The failure this replaces is not a node answering when it
should not have; it is a node **accepting a transaction and then dropping it**,
which is what gate 0 measured — 3,929 entries healed into one partition, and a
user transaction stranded with no healer at all, because a synthetic has one
and a user's has none (#4366, run `20260919T191634Z`).

**It does not validate what it relays.** Validating against a store the pull
has half filled fails on an account the node does not have yet and tells the
sender its transaction is bad when it is not (#4307) — so a node that relays
passes the submission on **not validated, decoded only to route**: routing
reads the envelope's principals to pick the destination partition, against
the routing table the node holds as of its last executed block (the globals
it was seeded with, then `WillChangeGlobals`), and reads nothing else; the
node that will propose it is the node that validates it. A relay is therefore
not a weaker `Submit`; it is a different operation, and a node that cannot
propose must not answer as though it had. **`Validate` is a read**: a syncing
node refuses it (#4307), and a node that is synced answers it from its own
state whatever its committee, because a validation judges against the latest
committed state and promises nothing about proposal.

What a follower is owed by this, and what it owes: a follower is a full
participant in carrying traffic and in no committee, so it relays every
transaction it is given — from a client at its API and from a peer at its
submit service alike, which puts one relay hop on the share of cross-partition
dispatch that lands on it — and proposes none. **A relay goes to a node that
can propose, never to the relaying node itself, never to another node that
would only relay it again, and never twice for one submission**: "never
dropped" with no bound would make a loop between two followers, or a node and
itself through a local-first dial, conform to this text. The mechanism is the
build's; the bound is the rule's. A node in no committee of a
partition still never *proposes* for it, and still never authors or dispatches
an anchor for it (#4367) — relaying a transaction and producing consensus
output are different things, and the first is allowed precisely because it
produces nothing.

Open when this was written, and where each stands now (2026-09-19/21): what a
relaying node does when the target refuses or is unreachable — *decided by
the lead for the build (#4366 note_3869841847, named as the lead's, not
Paul's): a validator's refusal is passed back unchanged; unreachable or
`NotReady` is tried once per committee member, then answered as such* — and
whether it answers its caller on the relay's result or accepts and forwards —
*decided the same way: synchronously; the harness's stranded arithmetic
depends on it (REPORTING-SPEC §3)*; whether it relays only for the partitions
it runs — *any partition it is asked for*; what a node that holds no
committee to choose a target from does — *`NotReady`, counted `not-ready`*;
what "fully synced" is — *settled above, #4368*. Still open, decided by
nobody: whether a node that cannot propose still advertises
`submit:<partition>` on the DHT (a record, distinct from the installed
handler above; the build left advertising as it was and #4300/#4336 hold it).
Each lead decision is overturnable on #4366 with evidence; none is Paul's
word.

What a restart therefore never does is replay committed blocks it did not
execute, or rebuild staging from a source's cache: the first executes with the
wrong staging, the second holds what the source produced rather than what the
peers had received (run 20260918T023054Z: an entry still in flight to the
peers, held from the cache, executed a block early).

Nothing derived from staging is written into hashed state unless it is derived
through execution. `Delivered` qualifies. A copy of how far a stream has been
sighted does not: it is per-node and transient.

### A block does not begin with an empty slate

Opening a block finishes the previous one. Before any of this block's messages
are seen, `Begin`:

- captures the previous block's BPT root, and where the directory anchor chain
  stood before this block applies anything to it;
- **finalizes the previous block** — records its anchor if it has not been
  recorded, and dispatches the synthetic messages whose receipts the previous
  block brought back (see Dispatch). These are independent duties: skipping
  the anchor must not skip the synthetics;
- resets the ledger's transient values and refuses to move backwards — a block
  index that does not increase is a panic, not an error;
- records the previous block's votes and evidence, unless that block was empty;
- **drains the delivery queues** — everything the previous block queued, local
  synthetics and locally produced messages, is delivered before any of this
  block's own. These are the delivery queues, not staged streams.

So "the executor's input is consensus" is true of *messages*; the block's work
also includes the tail of the block before it.

### Parallel execution

A block may execute independent user envelopes in parallel across shards. What
may be parallelised is decided by identity, and the rule is adversarial:

**Classification never trusts a submitter's claim.** A remote stub's principal
and a signature's TxID account are claims. The executor loads the real
transaction by hash and writes *its* principal's records, so classifying by the
claim would let a crafted envelope execute another identity's writes on the
wrong shard. Claims are resolved to the real transaction, and anything
unresolvable is serial.

Serial by construction: any non-user message, any signature that is not a user
key signature, any system or partition identity, ACME, any held transaction,
and any envelope spanning more than one identity. Sequenced messages are serial
too — they belong to streams, and streams are settled by staging — so only user
transactions are ever candidates for a shard.

### The block ledger

Every block leaves a record of which chains it changed: for block *N*, the
list of (account, chain, index) entries the block's execution touched. This is
the **block ledger**. It is the only place the block-to-chains direction exists
— the root chain commits to every changed chain's anchor, but an anchor is a
hash, not a name — and it is what the block query, the block event stream and
the metrics service answer from.

The block ledger is a **chain on the partition's system ledger account**, with
one entry per non-empty block, and the block's entry list stored once, keyed by
block index. Two things follow, and both are the point:

- **A snapshot carries every block's ledger record.** They are what a restored
  node rebuilds its account indices from (`repair-indices`), as it could from
  the per-block accounts they replace; a snapshot without them leaves a
  restored node with no way back to its indices. A snapshot carries a chain's
  entries and not its hash index, and restore rebuilds every chain's index
  from its entries before the node runs.
- **Closing a block costs the block, not the chain.** Recording block *N*
  writes one record the size of block *N*'s entry list and appends one hash to
  a chain. Nothing already written is read back or written again. A node at
  block 35,000,000 pays the same to record a block as a node at block 100.
- **The block ledger is consensus state.** The chain's anchor is part of the
  ledger account's hash, so the state root commits to what every block changed,
  and a receipt from the chain proves it.

It is not an account per block — that puts a BPT entry into the state tree for
every block forever, which is the tree's size doubling for no consensus
purpose. It is not a paged log that re-writes its head page on every append —
that makes the cost of a block grow with the height of the chain, which is the
one thing a per-block record must never do. An empty block has no entry.

### The invariants

1. **A stream executes in order, with no gaps.** There is no skip.
2. **An invalid proof is refused; an unproven one waits in anchor staging until
   its anchor decides it.** A proof that does not verify is a `BadRequest` and
   never enters staging. A proof whose anchor has not executed yet is collected;
   the anchor validates or disproves it. Nothing is ever recorded pending
   outside staging.
3. **Everything received is held until it can be processed.** Staging is
   bounded only by the sanity horizon: about an hour of the source's
   production ahead of `Delivered`. Within it a message that cannot be dropped
   and cannot yet execute is kept.
4. **A message that reaches staging is held in memory until it executes or
   is tossed.** Nothing is written until it executes: staging is the state
   before any persistence.
5. **Block state never feeds back into staging.** The only thing the executor
   reads from a stream's ledger is `Delivered` — what has been processed.
   Nothing else about an inbound stream lives there.
6. **Staging is identical on every node.** It is fed only by consensus, so it
   is a deterministic function of the same input everywhere. A node that joins
   or restarts syncs first — it replays the committed stream from its last
   executed block and rebuilds staging as it goes — and executes nothing until
   it has caught up. A node whose staging differs from its peers' will execute
   a different run and produce a different block hash.
7. **State changes only as a side effect of execution**, and become durable only
   at the block's single commit.
8. **A ready message executes; a not-ready message executes nothing.**
9. **The work of closing a block is bounded by the block's contents.** No
   step at block end re-reads or re-writes a record whose size grows with the
   height of the chain. A per-block record is written once and never touched
   again.
10. **What a block records about its execution is recorded in execution
    order.** Messages run in the order staging released them and the
    chains are appended in that order; the per-message state that says what
    was appended folds into the block in the same order. Nothing between
    execution and the block reorders it, and nothing after has to put the
    order back. A structure that loses the order — a map — is not used to
    carry it.
11. **Every state-tree root the network produces can be proven to the
    directory.** A root that an anchor never carried is still provable, because
    the root is an entry on the partition's bpt chain and that chain is anchored
    into the root chain. There is no root an account proof cannot be completed
    against, and therefore no "not yet" that means "never".

### Versioning

Behaviour that changes what a block produces is gated on an `ExecutorVersion` so
nodes at different versions do not disagree about the same block — `V2Baikonur`
(6), `V2Jiuquan` (8), `V2Kourou` (10, collection proofs). A gate protects a
network that is running; it is not ceremony for code that has never been
deployed.

**Ungated, and deliberately so: the order a bundle folds its states in.**
Folding in execution order rather than transaction-hash order changes the
order of `State.ReceivedAnchors`, which is the order of the receipts in the
`DirectoryAnchor` the block builds. Under DagBFT nothing votes on a block
hash — consensus is on certificates, and the state hash is stamped on the
certificate after execution for comparison, not agreement — so the
consequence is not a block the partition cannot agree on. It is two things:
the anchor body is what every Directory validator signs, and validators
that build it with receipts in different orders sign different
transactions, so the BVN never gathers a quorum for it (the #4054 failure
mode); and the body is stored on the system ledger, so the state trees
diverge, which the next anchor's BPT hash exposes. It is ungated because
`V2Kourou` has never run a network that outlives a run: every soak starts
from genesis, and no deployed network executes this path. **If Kourou is
ever activated on a live network before this lands, this needs a gate.**

## 2. Specification — how it is implemented

### Opening a block

`block_begin.go`, `Executor.Begin`:

1. Opens the block's writable batch, discarded if anything below fails.
2. Publishes `WillBeginBlock`.
3. Reads the previous BPT root into `State.PreviousStateHash`, and
   `dnAnchorsAtStart` — the directory anchor chain's height before this block
   applies anchors (#4169 step 0c), which staging uses to tell an anchor applied
   this block from one applied earlier.
4. `finalizeBlock` for the previous block: records its anchor if unrecorded, and
   sends the synthetics it produced. The anchor is recorded for the last
   *non-empty* block rather than strictly the previous one — under CometBFT at a
   block a second the two nearly always coincide, but DAG-BFT produces a block
   per committed certificate, dozens a second, and a one-block window that is
   missed stalls the anchor sequence (#4054: 4 anchors recorded of 55 anchored
   blocks). That changes when anchors are recorded, which is state, so it is
   version-gated to preserve replay of pre-Kourou history.
5. Loads the ledger and **panics** if the index does not increase.
6. Resets transient ledger values: index, timestamp, pending updates, ACME
   burnt, anchor.
7. Captures votes and evidence as data entries, unless the previous block was
   empty.
8. `drainDeliveryQueues` — delivers what the previous block queued (#4146).

### Entry from consensus

`pkg/consensus/adapter/executor_bridge.go`, `ProduceBlock`:

1. Every batch named by the committed certificate is checked to be in hand
   **before the block is opened**. A nil batch is fatal: `CollectBatches`
   guarantees a complete set, and executing a certificate without one of its
   batches silently diverges state (#4116/#4119). Checking after `Begin` left
   an opened block behind (#4279).
2. `executor.Begin(BlockParams)` opens the block.
3. The batches are walked **in payload order**, each transaction unmarshalled
   into an envelope and processed — `ProcessAll` when the block supports
   parallel execution (#4145), otherwise `Process` per envelope.
4. `block.Close()` produces the block state; `state.Hash()` then
   `state.Commit()`. **A `Close` or a `Commit` that fails releases the
   block** — its batch, its cache view and its staging view — before
   returning the error. The caller holds an `execute.Block` and then an
   `execute.BlockState`, neither of which offers a way to release it, and a
   block left open pins a version of the store for the life of the process:
   the Directory held one open for ten hours on every node, every commit
   since keeping its pre-images for a reader that would never read (#4279,
   run `20260915T042428Z`). `Commit` has two such paths of its own — the
   pre-commit event publish, and a `Conflict` from the batch, which returns
   before the change set is committed and so before the store's view is
   released.

Accounting is emitted per non-empty block — arrived, executed, unmarshalFailed,
processFailed, statusFailed, sharded, serial, shardsUsed — because 95 of 100
submitted transactions once vanished between acceptance and execution with no
log line anywhere (#4132). This is the seam where consensus hands to execution,
so it is where "lost in consensus" and "lost in execution" separate.

### Validation

`exec_validate.go`, `Executor.Validate`: begins a read-only batch, normalizes
the envelope, rejects unsigned transactions, and calls each message's validator
through a bundle whose block is a shell. The batch is discarded unconditionally.

### Execution

`exec_process.go`:

- `Block.Process(envelope)` normalizes, calls `processEnvelope`, then merges the
  resulting bundles into block state. The merge is the caller's so that under
  parallel execution every touch of shared block state happens serially, in a
  deterministic order.
- **A bundle folds its messages' states in execution order** (`bundleStates`,
  a slice in the order the states were recorded), and the bundles fold into
  the block in the order they ran. That order is deterministic on every node
  — messages run in envelope order, bundles fold serially — so nothing is
  sorted and nothing downstream reconstructs it. It was a map sorted by
  message hash: every node folded in the same order, but not the order the
  chains were appended in, and the block's segment bookkeeping (below,
  "Dispatch") lost the span of one of three anchors from one partition
  (#4279). Order that execution already has is kept as a slice; it is never
  put through a map and sorted back.
- `processMessages` runs the messages and every pass of additional messages they
  cascade into.
- `bundle.callMessageExecutor` finds the executor registered for the message
  type and calls `Process`. Internal message types are refused on the first
  pass.
- Statuses returned to consensus are **cleaned**: the result and a success code,
  or a generic error code. Error messages are porcelain and differing text
  across nodes would be a consensus failure; the API reads real status from the
  database instead.

### Message executors

`msg_*.go`, registered at init into `messageExecutors`:

- `registerSimpleExec[T]` — always available.
- `registerConditionalExec[T]` — available only when a predicate holds, which is
  how version gating is expressed. `SyntheticProof` registers only under
  `V2KourouEnabled`, because what a node is willing to accept is consensus
  critical.

### Validity — the proof itself

`msg_synthetic.go`, `SyntheticMessage.check`, before anything else:

- A collection proof (`ReceiptList`) is refused unless `V2Kourou` is active, and
  a proof may carry a receipt or a receipt list, never both.
- `ReceiptList.Validate` replays `MerkleState` through `Elements`, requires the
  last element to be the receipt's start and the recomputed anchor to equal the
  receipt's anchor, then validates the receipt. So a list proves every element
  it carries, in any order, without needing any other element — and it also
  proves each element's absolute index, because the state is counted.
- A list is validated **once per envelope**, not per member: a package's members
  share one proof, and rehashing it per member is a CheckTx denial of service.
- `MaxReceiptListElements` (4,096) bounds how many elements a proof may carry.
  Unlike the other bounds in this document, a receipt list is untrusted input
  and verification hashes every element before it can know the proof is junk, so
  the bound limits what an attacker can make a validator do. It binds in three
  places and all three must agree: the sender will not build a package whose
  span exceeds it (`packageSpanFits`), the sequencer refuses a range request
  larger than it, and the receiver rejects a proof carrying more.
- Failing any of these is `BadRequest`. The message does not reach admission or
  staging.

### Anchor staging — proofs wait for their anchor

A collection proof names the directory anchor it terminates in:
`AnnotatedReceipt.Anchor.SourceBlock` is the Directory block whose anchor
carries the proof's root (`directoryAnchorMetadata`, filled on both dispatch
paths), the same block index the destination records on each entry of its
Directory anchor chain. On intake (`Block.intakeProof`, from `classify`) the
proof is held in anchor staging, in memory, under its source and that block.
`DirectoryAnchorBlock` on the anchor pool is the newest Directory anchor
executed here, written as a `DirectoryAnchor` executes — execution output,
like `Delivered`; a proof naming a block at or below it that the chain does
not carry is disproved at intake. `validateStagedProofs` runs after
the anchor group, over the Directory anchors the block executed. A validated
proof's hashes go into the stream's stage as the **validated hashes at their
numbers**: the proof covers one chain, so element i of a proof starting at
chain index s is sequence number s+i+1 ("One chain per pair, one stage per
chain"). A proof that contradicts a hash already validated at a number is
refused (`errors.Conflict`); a collected entry a proof contradicts is dropped,
so its number is a hole healing asks for again. A proof below what is
validated fills in behind, so what it proves is validated wherever it lands
and a later contradiction there is still a conflict. Outcomes are
`accumulate_exec_staged_proofs_total{outcome}`: staged, validated, disproved,
conflict, invalid.

**What bounds anchor staging, and in what currency.** A source's waiting
proofs are bounded by what they cost in bytes (`maxStagedProofBytes`), not by
how many distinct Directory blocks they wait on. The difference matters
because the number of blocks a destination waits on is a measure of how far
behind it has fallen, not of what it is holding, and a bound in that currency
tightens exactly when the proofs become most valuable. A proof is one receipt
list covering a whole package, while the entries it proves are held with no
byte bound at all — so refusing the proof saves almost nothing and forfeits
everything it would have proved.

The flood such a bound exists to stop is already prevented, and still is: a
proof must cover a message from its source in the same envelope, it may not
name a Directory block more than `maxAnchorAhead` past the newest executed,
and each list is capped at `MaxReceiptListElements` and must validate. Proof
volume is therefore already proportional to traffic the destination agreed to
accept; the byte budget bounds what remains.

**A package and its proof share a fate.** When the budget does bind, the
entries that travelled with the refused proof are refused too, rather than
collected. A collected entry is recorded as received, which leaves no gap —
and nothing re-sends a proof, so an entry collected without one waits for a
proof that already arrived and was discarded. Refused together, what is left
is an ordinary hole: the source still holds those entries as undelivered, and
the destination asks for the span again once it has caught up and has budget.
This is the rule the batch-bytes defect taught (#4159, #4282): a message with
no recovery path must not be the one that is dropped.

### Collection — an unproven entry is held, never parked

`SyntheticMessage.process`: an entry whose proof's anchor is not here yet is
collected (`collect`): the message and the transaction it belongs to are held
in staging at the entry's number (first sighting wins), marked collected until
the hash validated at its number is its own — **or until the anchor named by
the proof it arrived with has executed here**, which is the same question its
arrival asked, asked again against the chain as it now stands. Its own proof
is re-checked in full when it runs, so that decides only when the entry is
offered. Without it an entry held for want of an anchor waits for a package
proof over its number that may never come: the proof it arrived with is not
staged for its anchor, and nothing re-offers it. Nothing is written. The run
builder never takes a collected number until then (`streamPosition.runnable`),
and staging judges an arriving proof-less entry the same way
(`syntheticIsProven`); an entry held by the sequenced layer carries no
`Collected` mark because it passed its proof when it was held. When a
collected entry is validated, `MessageIsReady` loads it and `check` accepts it
on the validated hash alone, signature or not. Should one be run before that — it cannot be, by
construction — `check` answers "not yet proven" and nothing is recorded.
An entry at or below the delivered point is tossed on arrival
(`errors.Delivered`, nothing stored); an entry whose number is later taken by
a proven arrival is superseded — the arrival executes, the stream advances
past the collected mark, and the collected entry is never consulted again.

Staging is testable in isolation: `staging_sim_test.go` drives both halves
event by event — a package arrives, a Directory anchor executes, a block runs
— against a source chain of real sequenced messages, with the sequenced layer
replaced by a fake that only moves the stream position. Each rule above is one
simulation there. An
entry numbered more than `maxSequenceAhead` past the delivery point is refused
(`BadRequest`), not collected.

**An entry is collected only on a source validator's word.** Every copy's
signer is checked — the signature over the sequenced message must verify and
the key must be in the source partition's current validator set
(`signerIsSourceValidator`). Whether that decides anything depends on the
proof. A copy proven by a validated proof, or by a collection proof whose
anchor is here, executes on the proof alone, whoever signed it: the proof
authenticates the sequenced message, and requiring a current validator there
wedged recovery of historical ranges after validator churn (#4056). A copy
whose proof is NOT yet anchored proves nothing yet, and the number it would be
held at sizes the stream's stage — a self-consistent receipt list over a
re-wrapped message with a forged number is cheap to build — so it is
collected only when its signer is a current validator of its source; anyone
else's copy is refused (`BadRequest`, counted `refused`), and nothing is held
(#4243). The destination does not learn how many entries the source has
produced on any wire path, so `maxSequenceAhead` is a constant bound on what
a source validator can make it hold, not the count. Counted as
`accumulate_exec_synthetic_anchor_total{applied}`: proven, unproven,
collected, refused. When
that anchor executes, every proof waiting on it is validated against the
anchor's root: a match marks the proof's index range proven in synthetic
staging; a mismatch discards the proof and increments a counter. A proof whose
anchor has already executed is validated at intake. Nothing about a proof is
decided by the block that receives it except where it waits.

An anchor's own gate is a **validator signature quorum**: each copy of an
anchor carries one validator's signature, recorded as it arrives, and the
anchor executes once the signatures reach the threshold. Below it the anchor
is an entry held in its stream's stage, collected, like a synthetic without
its proof; it is never recorded pending. An anchor at or below `Delivered` is
tossed on arrival. There is one other way an anchor is validated: a raw past
anchor that arrives or is already held is validated when a **proof over the
source's anchor chain covers it** — the chain itself is the proof. A missing
anchor is a gap of entries and is requested on the cadence; a held anchor
below its quorum is an unvalidated entry and is requested the same way, each
answer carrying the answering validator's signature. Nothing is re-sent from
the source on its own.

### Staging — the ordering gate

`exec_stage.go`, `exec_stage_run.go`, `stream*.go`. Before anything executes,
each stream's work for the block is settled:

```go
type streamRun struct {
    stream stream
    run    []runEntry   // executes this block, in order
    stage  []*arrival   // held for a later block
}
```

`executionOrder` composes the runs in the group order above. The decision is
made once per stream per block, not per message: the executor previously read
the ledger inside every message's child batch, and because a child does not
share its parent's value each read deep-copied the whole ledger, making a drain
of n messages cost O(n²) (`TestSequenceLedgerCostIsPerRead`).

`streamPosition` is the block's working copy of one stream: `delivered`, plus a
reference to staging for what is held. It is built once per stream per block
from the ledger's `Delivered` and advanced in place, and at close **only
`Delivered` is written back**. It holds a reference rather than a copy of the
held set — a copy is a moment, and a moment of what the node holds
disagreeing with what the node holds is the whole defect.

### Staging is one structure, and this is what it answers

Staging is one in-memory structure per node, shared by everything that needs
it. It is not the block's, and it is not the healer's: both ask the same
structure, because two views of what the node holds is exactly the
disagreement that livelocked the network.

A stream is one chain: the source's synthetic chain to this partition, or an
anchor chain between the two (architecture, "One chain per pair, one stage per
chain"). Anchors and synthetics between the same pair of partitions are
separate chains and separate stages — anchors tracked by the anchor pool,
synthetics by the synthetic account — so conflating them would let an anchor's
position gate a synthetic's. A stage holds two lists indexed from `Delivered +
1`: the entries received, and the hashes proofs have validated; because a proof
covers one chain, its element at position *i* is the entry at index *i*.

| question | asked by | answer |
|---|---|---|
| hold this entry at index *n* | intake | held; the first sighting of an index wins |
| hold this proof for anchor *a* | intake | held; validated when *a* executes, or now if it has |
| how far do the validated hashes reach, and how far do the held entries match them | the executor, building a run | the run is where both agree, from `Delivered + 1`, in order |
| fewer entries than validated hashes; entries beyond the validated hashes | healing, on its cadence | a gap of entries; a gap of proof — the two spans it requests |
| release through *n* | the block, on commit | every entry at or below *n* is dropped; proven ranges below *n* are dropped |

What it holds is visible: per stream, the entries held and their encoded
bytes are gauges (`accumulate_staging_held_entries`,
`accumulate_staging_held_bytes`, labelled by ledger and source), updated when a
block commits, and a stream holding more than several blocks' worth
(`heldAlarmEntries`) is reported once and its clearing once (#4233). Staging
has no byte budget of its own — it holds what consensus accepted and execution
has not run, and what bounds it is the Directory's anchor latency, which is
what a growing gauge points at.

Four rules govern it, and each of them is a defect that has actually happened:

**The first sighting of a number wins.** A number can be offered twice — a block
discarded and re-executed, a healed message racing the original — and both carry
the same message, because the number identifies it. Keeping the first means the
same input always produces the same staging.

**The set of streams is staging's, not the ledger's.** A stream that has only
staged has delivered nothing, so it has no ledger entry to be found by. A
stream staging holds nothing for is current: an index it has never seen is
covered by the next proof the source's next anchor brings.

**The proven range is what says an index is missing.** Releasing what was
delivered drops the entries and the proven ranges at or below `Delivered`; what
remains proven above it and unheld is a gap.

**Release happens on COMMIT, not at flush.** Until the batch commits the
delivery has not happened. Dropping a staged message for a block that is then
discarded makes the node fetch back across the network something it still holds,
which is the failure this whole change removes, reintroduced from the other end.

### What the stream ledger is for

Exactly one field, in the inbound direction: **`Delivered`** — what has been
processed. It is read to place the stream and written when the block closes.

Nothing else about an inbound stream lives there. There is no pending array,
because the held set is staging's; there is no received mark, because how far a
stream has been sighted is staging's and writing it back would put a per-node
value into hashed state (see Restart). `Produced` remains, but it belongs to the
other direction — what this partition has produced FOR that one.

And a message at or below `Delivered` requires nothing at all. It is not healed,
not re-recorded, and not counted: it has been processed, and that is the end of
it.

**The ledger's `Delivered` is the one that counts**, and closing a block
releases every stream it touched at that value, not only the streams it
delivered into. Staging is memory: after a restart its own copy starts at
zero while the ledger's does not, and a stage that said zero would report
entries as held that the node executed blocks ago — which is exactly what a
joining node would then take from it (healing.md, "Staging snapshot").

**`Received` is answered, not stored -- BESIDE the body, never in it.** Removing
the field from the record does not remove the question, and the question is the
one every operator surface asks: how far is this stream behind. The API answers
it from staging's sighted mark, computed on read and never written, so the
account on disk carries no trace of it and nothing about consensus depends on
the answer. It travels as `AccountRecord.Sighted`, a value beside the body, and
a reader that wants it merges on its own side after checking whatever proof came
with the record. Filling it INTO the body on the way out was the mistake: the
receipt is built from the stored state, so a synthesised body does not hash to
the leaf its own receipt proves, and no anchor ledger could ever be pulled
(#4295). See "Sync", step 3.

Writing it back instead would be the mistake. A value derived from staging,
placed in an account, makes a staging discrepancy a divergent block hash rather
than a wrong number on a dashboard. And simply dropping it is the other
mistake: every reader then sees zero, which does not read as "no data" — it
reads as "nothing ever arrived", and paints a healthy stream as stalled.

`msg_sequenced.go`, `SequencedMessage`: `isReady` asks the block's position
whether the message is next. Ready messages execute; not-ready messages record
pending and execute nothing. `Process` records the message and its status, then
advances the stream — the advance is deferred so it lands only once everything
the message records has, and never on a path that discards.

### Closing a block

`block_end.go`, in order — the order is part of the contract, because each step
depends on the last:

1. Write each stream's advances to its ledger, once per stream (#4169 step 7).
2. Decide whether this completes a major block.
3. Process events: expiring transactions and signature sets, against the major
   block height just decided.
4. Settle the block's produced messages, `produceBlockMessages`. This must run
   before the anchor decision, because whether an anchor is needed depends on
   whether anything was *sequenced*, which is only known after the split below.
5. Decide whether an anchor must be sent.
6. **If the block is empty, stop.** Nothing below runs.
7. Record the previous block's state hash on the BPT chain.
8. Record pending transactions; process chain updates.
9. Add each synthetic chain that changed to the root chain, index the root
   chain, name each entry that chain gained in this block's entry list, update
   the transaction-chain index — from the hashes the block appended, kept with
   the record of each append (`ChainUpdates.Hashes`); only a chain appended to
   outside that record (the signature chain, the BPT chain) is read back for
   its hash (#4245).
10. Record the block ledger (below). It is written LAST of the things that
    change the entry list, because step 9 adds to that list: the synthetic
    chains are not anchored by the chain-update loop, so the only record that
    they changed — and of which entries they gained — is the one step 9 makes.
    A block ledger written before step 9 never names the partition's synthetic
    account at all.
11. Update major index chains if this is a major block.
12. Execute post-update actions.
13. **Update the BPT**, and only then active globals.

### Anchor emission, and the heartbeat

Step 5 above decides whether the block sends an anchor. It does, if any of:

- the block completes a major block;
- the block produced synthetic messages;
- the block updated an account other than the system ledger and the anchor
  pool — the system ledger changes every block, so counting it would make the
  test vacuous;
- the block received an anchor from another partition;
- **the block received a directory anchor and the heartbeat is due.**

The last is the heartbeat, and it exists because the first four are not enough
to keep a proof answerable. Every block moves the state tree root, including a
block whose only content is a received directory anchor. A reader who queries
at that moment gets a receipt terminating at a root no anchor is going to
carry, and before Kourou that reader waited forever: the directory-anchor
cascade died out, the network stopped, and nothing would ever put that root
within reach. Measured on an idle network: the reader's root never bound.

So from Kourou a partition anchors such a block anyway, which keeps the
cascade alive and every root reachable. It is **not a setting**. It was
`AnchorEmptyBlocks`, a network global defaulting to false, and a proof that
only works on a busy network is not a proof anyone can rely on — an external
reader does not control whether the network is busy.

It is rate limited. `AnchorLedger.LastAnchorBlock` records where the last
anchor went out, and the heartbeat fires only when the current block is more
than `anchorHeartbeatSkip` (3) beyond it — at most one anchor every fourth
block, rather than one on every block. Measured over 200 idle steps on one BVN:
67 anchors uncapped, 34 at skip 3, 23 at skip 7. The cap governs anchors, not
blocks: block production on an idle multi-BVN network is driven by the cascade
as a whole and stays at roughly one per block interval either way.

**The consequence, stated plainly: a Kourou network does not quiesce.** An idle
network keeps producing blocks and anchoring at the heartbeat rate, forever.
That was a deliberate property before Kourou (#3453, #3520) and is deliberately
given up, because a proof that cannot be completed is worth less than an idle
network that stops.

### What a stream logs

A stall on a stream must be readable from the node's own log, after the
fact, without a dashboard: when delivery stopped, what the stream was
waiting on, what the source had produced by then, and whether any number
ever went backwards. Nothing recorded that on run `20260915T042428Z`, so
"delivered stopped at 32,469" was a reading off a board, with no record of
when (#4279). Every block therefore writes, at Info, `module=stream`:

- **`Stream position`**, per stream, after `flushStreams` has written
  Delivered: `block`, `ledger` (synthetic or anchors), `source`,
  `delivered`, `advanced` (by this block), `sighted` (the highest number
  ever held), `reach` (how far validated hashes stand), `held` (entries in
  staging), `waiting` (the first number above Delivered nothing is held
  for, 0 when none).

  **A line is written when something about the stream changed, and
  otherwise no more often than `StreamLogEvery` blocks.** A stream in
  trouble is worth a line a block; a caught-up stream ticking along is
  worth one a minute; and a frozen stall must not write the same line
  every block for as long as it lasts, which is how per-message logging
  became the problem #4182 fixed. The cadence is also the liveness
  evidence: a stream that stops logging altogether means the partition
  stopped closing blocks, which is a different failure from a stream that
  logs the same position forever.

  `waiting` is the first hole within `StatusScan` numbers above Delivered.
  Finding it walks the stage, and a stage may hold an hour of the source's
  production, so the walk is bounded rather than growing with the backlog
  it reports. Zero with a non-zero `held` is a backlog with no gap in the
  window, not an empty stream.
- **`Stream produced`**, once per destination the block sequenced
  synthetics for: `block`, `destination`, `from`, `to`, `count`. One block's
  `to` and the next block's `from` are contiguous; a gap or a step backwards
  in the producer's own log is a defect in the producer, not in delivery.

`test/docker/soak/streamlog.py` reads those lines back out of a run's node
log and reports, per node and stream, where it stands, when it last
advanced, how long it has waited and on what, every value that went
backwards, and every gap in a producer's numbering.

### The block ledger

Step 10 of closing a block records the block ledger. The records live on the
partition's system ledger account, `<partition>.acme/ledger`, which is the only
account permitted to hold them (`Account.Commit` rejects a dirty block ledger
on any other account).

**Two records per block.**

1. `Account(ledger).BlockLedger(index)` — a state record keyed by the block
   index, holding `database.BlockLedger{Index, Time, Entries}`. `Entries` is
   `block.State.ChainUpdates.Entries` as collected before this step: the
   `BlockEntry` list of every (account, chain, index) the block changed. The
   block-ledger chain's own append happens after that list is collected and is
   not registered as a chain update, so the record never lists itself. The list
   is also how an append learns the block already holds an entry for its chain
   (a transaction appends to a chain once); it is indexed by (account, chain)
   for that, and the index is never consulted blind — the list is the record,
   and the index follows it.
2. `Account(ledger).BlockLedgerChain()` — a chain named `block-ledger`. The
   block appends one entry: the hash of the marshaled record above.

**The synthetic chains are named the same way as everything else.** A
partition's synthetic chains are anchored by `anchorSynthChains`, not by the
chain-update loop, so nothing else adds them to the entry list. That step adds
them, and it adds **one entry per appended chain entry, carrying that entry's
index** — the same (account, chain, index) contract every other chain gets,
because every consumer reads the chain AT the index it is given
(`loadBlockEntry`, and through it `queryMinorBlock` and the block event
stream). One entry per chain per block would name index 0 in every block, so a
block query would answer block *N* with the first block's synthetic
transaction, and a reader reconstructing from the block ledger would recover
one of the *n* entries a block appended. The entries are emitted by sorted
destination and ascending index: the record is hashed, so its order is
consensus state, and a map's order is not.

Both are written once. The record is never rewritten, which is what a layered
backend's permanent layer holds ([database.md](database.md), "Backends"); the
chain's element, element-index and mark-state records already qualify. Only the
chain head and the tail chunk of its open mark set, a few hundred bytes
together, are rewritten ([database.md](database.md), "The head is Count and
Pending").

**Commitment.** The ledger's chains are not added to the root chain in the
chain-update loop (the root chain and the BPT chain live on the same account and
would anchor themselves). The block-ledger chain is committed the same way they
are: the anchor of every chain on an account is folded into the account's hash
(`observer_prod.hashChains`), the ledger account's hash is in the BPT, and the
BPT root is the state root. A receipt from the block-ledger chain to the state
root therefore proves what a block changed.

The chain's height is not a block number: empty blocks write nothing, so
entry *i* of the chain is the *i*-th non-empty block. Lookups never go through
the chain; they go to the keyed record.

**Reads.** `LoadBlockLedger(index)` reads the keyed record. If it is absent it
reads the pre-activation account `<partition>.acme/ledger/<index>`; if that is
absent too the block is not found. One read for any block, at any height.
`queryMinorBlock`, the event service and the metrics service all go through it.

**Cost.** Per non-empty block: one record of size O(entries in the block), four
small chain records (element, element-index, head, and a mark state every 256
entries). Nothing is read back from earlier blocks. This is invariant 9.

**Activation and history.** Recording the block ledger this way changes the
ledger account's hash, so it is gated on an `ExecutorVersion` like any change to
what a block produces. Nothing is migrated at activation: blocks recorded before
it stay readable through the fall-through above, and their BPT entries are sunk
cost that the activation block does not touch. If they are ever removed it is
paced — a bounded number per block — and gated on its own, never a walk of the
whole chain inside one block. A chain that has only ever run this version has
nothing to fall through to.

**TBD — the existing network.** Mainnet holds one block-ledger account, and one
BPT entry, for every block since 2022: 35 million on the Directory. Bringing
that history into this form — or out of the state tree at all — is a database
reorganization on a scale nothing in this specification covers, and it is a
separate problem with its own design, not a step of this one. What is specified
here is what a block writes from activation on. Test networks are fresh installs
and have no history to reorganize, so they run this form from genesis.

### Signatures

Signatures are dispatched twice, through two registries.

- `messageExecutors` handles `SignatureMessage`, which checks the wrapper — a
  signature and a transaction ID must both be present — and then calls the
  signature executor for the signature's *type*.
- `signatureExecutors` holds the type-specific executors: `UserSignature` (the
  ordinary key signatures, plus a conditional variant), `AuthoritySignature`,
  and `EthereumDataSignature` under its own condition.

So a user signature is not special: it is an ordinary message carrying a
signature, and the second dispatch exists only because the signature's type
decides how it is verified.

### Anchor signatures are not that path

A block anchor is a **message** executor, `BlockAnchor` in `messageExecutors`,
and never reaches `signatureExecutors`. It is authorized in one of two ways:

- **A validator signature**, counted towards a quorum. The signer must be a
  validator of the anchor's *source* partition, and the source must parse as a
  partition URL.
- **A collection proof under a known directory root** (#4056). This authorizes a
  *healed* anchor without re-gathering a quorum, because a historical quorum may
  be impossible to re-gather after validator churn while the proof depends only
  on the current directory root, which every synced node has.

The proof is checked the same way a synthetic's is — receipt list only, never
both forms, bounded by `MaxReceiptListElements`, and `Validate`d — and the same
bound applies. An anchor with neither a signature nor a proof is `BadRequest`.

The anchor itself must be a sequenced anchor transaction whose destination's
root identity matches the transaction's principal, and a remote placeholder is
resolved to the real transaction by hash before the type is checked.

So the difference is not the signature algorithm. It is **when** authorization
is decided:

- **A user signature is checked as part of execution.** Authority, thresholds
  and delegation are state-dependent, and evaluating them *is* execution work.
- **An anchor is authorized before execution.** Quorum or proof is a
  precondition, not an effect, so it is a staging decision.

### Anchor authorization belongs to staging

Signatures for an anchor route to **staging**, not through execution. They are
inputs to a decision, not state changes.

Staging holds the one anchor and collects the signatures that arrive for it,
packs them together, and evaluates the quorum — or the collection proof, which
needs no accumulation at all. Once it answers yes the anchor is **authorized**,
and it executes once, in one block, **with no further checking**: everything an
executor would re-verify has already been verified, so execution applies the
anchor rather than re-deciding whether it may.

The payload deduplicates naturally under this shape. Copies of an anchor from
different validators are identical, so they collapse to one; today they cannot,
because each `BlockAnchor` embeds a different signature and therefore hashes
differently, and `checkStatus` never short-circuits.

Determinism is free. Signatures arrive through consensus like everything else —
there is no out-of-band path — so every node accumulates the same set by the
same block and authorizes at the same block.

This also removes an asymmetry. An anchor authorized by proof executes on first
arrival; one authorized by quorum takes as many executions as it takes
signatures. Under this shape both are a staging decision followed by one
execution.

**What a copy costs.** Until the copies collapse in staging, each one is a
message the executor sees, and what it writes is bounded by what the copy adds
(#4224):

- The anchor transaction is stored **once**, under its own hash, by the first
  copy to arrive. Every copy is stored as its signature over a reference to the
  transaction by hash ("The database write"), never as another body.
- The signature chain receives one entry **per distinct signer** — that is the
  chain's purpose. A second copy from the same validator, under a different
  hash, adds no signature and no entry.
- The validator signature set is read once per anchor per block and written
  **once**: with the execution it authorizes when the quorum is reached, or at
  the block's close for an anchor still below it. The quorum is counted from
  the block's view of the set, so copies arriving in the same block reach it in
  that block (`anchorSignatures`, `Block.anchorIsAdmissible`).
- A copy is one Debug line; the anchor's execution is the one Info line.

### There is no cascade

A message does not queue further messages into a later pass of the running
bundle. Staging decides the whole run before anything executes, so a successor
does not need to be discovered while its predecessor is running.

What replaced it is the run entry. Staging places the messages that will execute
this block, and each is entered through an internal `MessageIsReady` naming the
staged message; the executor loads it and calls its executor.

**A run entry enters at pass 1, not pass 0**, and the number is load-bearing.
Internal message types cannot be marshalled, so one arriving in a submitted
envelope would have to be forged — the executor therefore refuses internal types
at pass 0. A run entry is internally generated in exactly the same sense as the
old mechanism's queued message, which was handed to a *later* pass, so it must
enter at a later pass too.

Getting it wrong is silent. The guard returns an error *status* rather than an
error, so a staged entry looks like it ran: every staged entry fails, every run
stops at its first one, and only freshly arrived messages are ever delivered.
The symptom is a backlog that cannot close — 40 delivered per block against 40
arriving — and it is invisible in the statuses. It is found by asking the ledger
whether it moved.

The one thing still carried to a later pass is a **network update**, produced
when a directory anchor brings one. That is a genuine consequence of executing
the anchor, not a deferred delivery.

### Parallel execution

`exec_parallel.go`. `classify` sorts every envelope once: each message is asked
for its stream through `streamOf`, recorded as an arrival keyed by sequence
number with first sighting winning, and an envelope whose messages belong to no
stream is a user transaction. `stageRuns` then decides one kind of stream's runs.
`ProcessAll` runs the user transactions across `ExecutionShards`; at a shard
count of one or less that is exactly a loop over `Process`.

`envelopeIdentity` returns the one identity an envelope's messages belong to, or
`(nil, false)` meaning serial. Serial covers any non-user message, any signature
that is not a user key signature, any system or partition identity, ACME, any
held (`HoldUntil`) transaction, any transaction that cannot be resolved to its
real content, and anything spanning more than one identity. Network accounts are
serial even though they are user-writable, because they mutate shared executor
state.

Claims are resolved rather than trusted: a remote stub's principal and a
signature's TxID account are claims, and classifying by them would let a crafted
envelope execute another identity's writes on the wrong shard (#4149).

**What proves it.** Shard count is a local parallelism choice that cannot
change the result, and the proof is not an argument but a gate: one simulated
network whose nodes run at *different* shard counts, executing the same blocks
from the same inputs, with the simulator's consensus comparing every node's
deliver and commit results on every block. If shard count could change any
result, the step fails on the block where it does. Four gates run it
(`test/e2e/sharded_*_test.go`): identity-local and cross-partition transfers
(`TestShardCountDoesNotChangeBlockHash`), the signature shapes the classifier
must reason about -- a multisig completed by a later signature-only envelope,
cross-ADI delegation, a held transaction (`TestShardEquivalence_MixedSignatureShapes`),
a synthetic-heavy block (`TestShardEquivalence_SyntheticHeavy`), and randomized
mixed traffic at 1/8/64 shards over many rounds
(`TestShardEquivalence_Randomized`). Each asserts its own coverage -- the
parallel lane must actually have run -- because a gate that compares serial
to serial proves nothing, which is what the first one did until #4149. CI
runs them under the race detector (`go test race (sharded)`); the data race
on `Batch.nextChildId` was invisible without it.

`ExecutionShards` defaults to 1. It is raised on a network only with those
gates green under `-race`, and only through the network definition
(`executionShards` in the network file), which `init network` writes into
every node's configuration: what a node runs is what its network was defined
with, frozen with the run. There is no environment path -- one used to exist
for shard sweeps, and a knob the environment can change is a run that can
silently differ from its recorded config (BlockchainDB spec 1.10).

A block's production time is exported by partition
(`accumulate_dagbft_block_production_seconds{partition}`) and each phase of it
on its own -- begin, unmarshal, process, close, hash, commit
(`accumulate_dagbft_block_phase_seconds{partition,phase}`) -- so a review can
say where a block's second goes without sampling goroutines (#4257). One
histogram per process for both partitions' blocks said nothing about which
executor was behind.

Timing is booked as serial versus parallel share, so a run can say whether
sharding helped or whether nothing was shardable.

### Dispatch — when a block's synthetics leave, and who sends them

**Every proof a block sends is built from memory, never from the stored tree.**
The block keeps the span of the root chain it appends and, for every anchor
chain it appends to, that chain's state before the block and the hashes it
added (`merkle.Segment`); the Directory's receipts for the partition anchors it
received are the anchor-chain segment from the received anchor to its new head
joined to the root segment from where that head landed. The synthetic proofs
come from the producer cache's segments the same way. Nothing reads a chain
back to prove it (database.md, "Duplicates are caught at entry").

A chain's segment is assembled from the spans each message appended, folded in
execution order (invariant 10), so every span continues the one before it. A
span that does not is not tolerated and not dropped: it is recorded against
**that chain** (`ChainUpdates.SegmentError(key)`) and the receipt built over
that chain's segment is refused, naming the span. The fault is per chain, not
per block: a block carries segments for chains no receipt is built from, and
failing the whole block on any of them turns one unbuildable receipt into a
partition that cannot close a block at all — which is how #4279 killed the
Directory. A segment with a hole would otherwise produce a receipt built over
the wrong span, which is what the hash-order fold did.

A block's synthetic messages do not leave when the block closes. They leave
when a **Directory receipt covering that block comes back**: the block's
anchor goes to the Directory, the Directory anchors it and sends back a
`DirectoryAnchor` carrying a receipt for that block, and the block that
executes that `DirectoryAnchor` is the one whose `Begin` dispatches the
synthetics of every block the receipts cover. Only then can a proof be built
that terminates in a Directory root the destination will hold. The receipt for
the block itself is the normal case; a receipt for any later block also covers
it, because the later root chain contains the earlier root, and that is what a
healed proof is built under.

**Everything a package needs comes from the cache** ([healing.md](healing.md),
"The cache"): the bodies, the transactions they belong to, and the positions
the proof is built from. Nothing is read from the historical record to build a
package or its proof. Such a read is a failure, and it is counted.

**The leader sends.** Every validator builds the packages; only the block's
leader (consensus.md, "The DAG facts") submits them, through the dispatcher,
to the destination partition's submit service. A dispatch changes no state
here, so it is not part of what the block produces and nothing waits on it.

**The package.** For a destination, the block's synthetics are grouped into as
many envelopes as fit `synthPackageBudget`. Each envelope is one
`SyntheticProof` — a collection proof over the span of **that destination's
synthetic chain** from the package's first member to the block's last element,
continued through the root chain to the Directory root, and carrying that
Directory anchor's **sequence number** — followed by the members as proof-less
sequenced messages, each with the transaction it belongs to. The proof and
every member also carry the sender's **`Delivered`** on the destination's
stream to the sender: the latest of the destination's synthetics the sender
has executed, which tells the destination what it may drop from its own
producer cache (healing.md, "The cache"). The proof's hashes
are the destination's entries in order, so the proof's index is the sequence
number and the stage aligns the two by position. A group of one is sent with an
individual receipt instead. **Each anchor copy carries the same word for the
anchor stream**: `BlockAnchor.Delivered` is the sender's `Delivered` on the
destination's anchor stream to it, set beside the signature (the signature is
over the sequenced anchor, not the copy) and taken at the destination where
the copy's validator signature is recorded, so the destination's cache drops
the anchors it produced that the sender has executed (healing.md, "The
cache"). At the destination the members go to synthetic
staging by index and the proof to anchor staging by its anchor's sequence
number (Collection), whichever arrives first.

**The dispatcher is isolated from the data it dispatches.** The block hands
its envelopes to the dispatcher (`Submit`) and is done; nothing in the block
waits on a send, and the block's own `Send` only marks the block boundary. The
dispatcher owns **one outbound queue per destination partition**, drained by
its own goroutine on its own stream with its own per-attempt deadline, so one
unreachable partition never delays another and one failed dial never loses the
envelopes queued behind it. **A write that fails is retried**: an attempt that
fails at the transport, or that the destination answers "not now" (worker
back-pressure, a full store, any server-side error), keeps the envelope queued
and tries again after a back-off, until the envelope's retry deadline — a few
blocks' worth — passes. A destination that refuses an envelope as invalid
settles it: it is counted and not retried. The queue is bounded in **blocks**:
envelopes from more than `dispatchQueueBlocks` blocks ago are dropped, oldest
first, when a new block begins. Every outcome is a counter per destination —
queued, sent, retried, refused, dropped (by reason: deadline or queue-full) —
so that what left the leader can be compared with what arrived
(`accumulate_dispatcher_*`, `internal/node/daemon/dispatcher.go`).

**Failure has one fallback.** A dispatch the leader never makes — its executor
behind, the receipt lost — or that the dispatcher drops after its retries is
not retried by the executor. The destination sees the gap and healing fills it
([healing.md](healing.md)). The executor counts packages built, packages
dispatched and submission errors so that the two can be compared.

### Executed height

After each block commits, the executor reports the block's leader round to the
node. Consensus uses it to keep the DAG within `MaxExecutionLag` of execution
(consensus.md, invariant 9).

### Produced messages, and the local/remote split

`exec_local_queue.go`. A message a block produces is either **remote** — its
destination routes to another partition — or **local**, routing back to the
partition that produced it.

- **Remote** messages are sequenced and dispatched: they take a sequence number,
  a position on the synthetic chain **to their destination**, a proof continued
  to a DN-anchored root, and anchor-pool validation on arrival.
- **Local** messages take none of that. They go on a persisted queue and execute
  at the start of the *next* block as ordinary messages with their own
  principal — no sequence number, no synthetic-chain position (so they consume
  no collection-proof span), no dispatch, and no anchoring dependency (#4146).

Next block rather than the same block, and that falls out of the geometry: the
queues are drained at the *start* of a block and written at the *end* of one, so
nothing queued while draining can execute before the next block. A local
synthetic that produces further locals therefore creates ordered work in the
following block — no cascade, and no termination bound to reason about.

The input must already be in canonical order (#4144); the queue preserves it,
which is what makes the drain deterministic. Each entry is
`destination.WithTxID(msgHash)`, so the drain re-checks routing without
re-deriving the destination from the message type.

### The database write

Executors write into the block's `*database.Batch` as they run. A message
executor takes a sub-batch and commits it into its parent on success or discards
it on failure, so a failed message leaves nothing. The block's batch reaches
disk exactly once, at `state.Commit()` in `ProduceBlock`. `state.Hash()` is
taken before the commit, and a failure to hash discards rather than commits.

**One body per transaction.** A transaction reaches the executor inside
wrappers — a `SequencedMessage`, a `SyntheticMessage`, a `BlockAnchor` copy —
and each wrapper is a message of its own, recorded under its own hash. The
transaction's own executor stores the transaction under the transaction's
hash; once it has, every wrapper recorded in the same bundle is stored
referring to it by hash — a `RemoteTransaction` placeholder where the body
was, as an envelope carries one — so the store holds one body per transaction
rather than one per wrapper (`storedForm`, #4236). A wrapper whose transaction
was not stored — the transaction was refused before it was recorded — keeps
the body, because it is the only copy. The record's key is the wrapper's hash
as it arrived; a reader that recomputes the hash of what it loads gets the
reference's, and resolves the transaction as it would a placeholder
(`getTransaction`).

**One status per outcome.** A status is written for every message that has an
outcome of its own — the transaction, the signature, the payment, and each
wrapper — because the status is what stops a message that is delivered twice
([database.md](database.md), "Duplicates are caught at entry"): a sequenced
message re-run from staging, a signature resubmitted, an anchor copy landing
in two blocks are each caught by their own status. What is not written is a
status nothing reads: the source keeps no status for the sequenced messages it
produces (the chain entry is their record; the destination's status is the
outcome), and nothing records a status for a message it did not execute.

---

Where the implementation departs from this specification, see
[DIFFERENCES.md](DIFFERENCES.md).
