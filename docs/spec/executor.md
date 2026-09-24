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

#### The algorithm (Paul, 2026-09-25)

This is the rule; the numbered sections below say how each part is done, and
where they disagree with it, this wins.

1. **Pull every account the state tree holds, and every block-ledger record
   from the start of the pull to the present, in order.** The pull starts at
   the peer's block S. The node walks the whole BPT, page by page, and pulls
   every account its leaves name; alongside it, it takes the block-ledger
   record of every block after S, in block order, and pulls again every
   account each record names (invariant 14). The walk may take many blocks;
   that does not matter. An account pulled during the walk is either
   unchanged since, or named by a later record and pulled again, so once the
   walk is done and the records are processed through the present, every
   account is current and the local BPT is the partition's. **The walk never
   overwrites a value the records wrote:** an account a block-ledger record
   has already brought current is skipped when the walk reaches it, because
   the walk's page may be older than that record. The other direction needs
   no rule: when the walk's page is newer than the records processed so far,
   the account it wrote was changed by a block the records have not reached
   yet, and that block's record names it, so it is pulled again when the
   records get there. Either way every account ends at the state of the last
   record processed.
2. **Keep processing the records.** Until the match, every new block's record
   is processed as it comes, in order, so the local tree stays current.
3. **The match is the proof.** Only when every account is present and current
   does the local root equal a root the network signed. That equality with a
   verified signed anchor's `StateTreeAnchor` (§1) proves the whole state at
   that block B. Nothing before the match is proven, and nothing before it
   needs to be. A lying peer can delay the match; it cannot fake it. **The
   anchor is the partition's own** (Paul, 2026-09-25): a BVN's state is proven
   by the BVN's anchor for block B, not by the Directory's copy of it, which
   reaches the Directory's pool only after the Directory executes it — far too
   late to sync against a partition that moves every block. Each validator of
   the partition signs its own copy of the anchor as B closes, and a
   partition's pool does not hold its own anchors with their signatures
   (measured: a BVN's pool holds none of its own; only the Directory, which
   anchors to itself, does). So the joining node collects the quorum itself:
   it asks the partition's validators for the anchor of B — the sequencer
   answers an anchor by number signed by the validator that answers (#4424:
   committee members only) — and the anchor is verified when distinct members
   reaching the partition's threshold have signed it.
4. **The synthetic ledgers give the staging floor.** In the state at B, each
   stream's `Delivered` says every synthetic transaction at or below it has
   been received and processed. Staging never needs any of them.
5. **Only then does the node stage.** It collects synthetic transactions and
   anchors from consensus into staging, above the floor.
6. **Then execute, and request what staging lacks.** The state at B
   records, for every stream, the highest number its partition had received
   — the synthetic ledger's `Received`, and the anchor ledger's for anchor
   streams — written by every block as hashed state (Paul, 2026-09-25,
   #4412). What the node lacks between `Delivered + 1` and `Received` it
   requests from the source by number, **and the answers reach staging
   through consensus, exactly as healing answers do — never straight into
   this node's staging.** A number that is a hole network-wide is filled in
   the same block on every node, this one included (Paul, 2026-09-25). The
   node does not wait for them: it executes from B + 1, user transactions
   included, with whatever staging holds, and an entry that arrives after
   the block its peers executed it in is an anchor mismatch, repaired
   ("Two mismatches", 2). Staging need not be exact (Paul).

**Two mismatches, handled differently** (Paul). They are not the same
failure, and the join treats them apart:

1. **The BPT root does not match: re-pull.** Until the pull is proven, the
   node executes nothing. It walks and processes block-ledger records (steps
   1–2) and compares the root it built with the partition's own signed anchor
   at every block that sent one; an idle partition's next anchor is its
   heartbeat, and the records carry the tree there. A root that does not
   match means the pull is wrong, and the only thing that fixes a wrong pull
   is to pull again: the whole walk, every leaf, while records keep being
   processed. The BPT has every leaf; no pull skips one.

   **Every block is checked, and a miss is located, not re-pulled whole**
   (Paul). After the records of a block are applied, the node's root should
   equal the partition's BPT root for that block. Every block has one: the
   peers keep the BPT's node history by height (`bpt.NodeAt`), so a peer can
   serve the root, and the interior hashes, as of block B even after it has
   moved on. Only a block that sent an anchor proves anything; the others'
   roots are a peer's word and only localize. When the roots differ, the node locates the difference along the
   tree's own storage: the BPT is stored in blocks of eight levels, so a
   peer serves, as of B, the 256 hashes eight levels down (one stored
   block), the node compares them with its own, and for each that differs
   asks for the 256 hashes eight levels below it — at a million accounts
   that isolates about sixteen accounts, at about 8 KiB an answer (Paul
   asked for a cut a few levels above the leaves; this is the cut the
   storage makes exact). The node pulls again, whole, only the accounts under
   the branches that differ. **It deletes nothing on this comparison:** a
   root that is not a signed anchor's is a peer's word, so a leaf the peer's
   branch lacks is marked, and removed only when the comparison was at a
   signed block and the match that follows confirms it. **A record is taken
   with a receipt** to a signed root it sits under wherever the node holds
   one, so a peer cannot drop or add accounts in it; and a source whose
   answers were part of a re-pull that did not bring the match is moved to
   the back of the order. A repair then costs what is
   wrong, not the size of the tree.
2. **The anchor does not match: repair, and move to the next block.** Once
   the root has matched, the node stages and executes — synthetic and user
   transactions, with whatever staging holds. A staging difference can make
   the node's execution differ from its peers' in some accounts, so the
   anchor it produces for a block may not match the partition's signed one.
   That does not break the BPT the pull proved: the block ledger names every
   account the block changed (invariant 14), so the node repairs those
   accounts from the peers, whole — main state, every chain with its entries
   and the messages behind them, the pending list, the directory; a chain it
   grew wrongly is replaced, not appended to — and moves on to the next
   block. Until repair pulls accounts as of the block that mismatched and
   the executor reloads what it holds in memory across a repair (the
   globals, the producer cache, staging's settlement), the node stops
   executing to repair and hands off again at the repaired block, as the
   join does (#4440). This is the same for every node: joining, restarted or running
   (#4440). No node's staging has to be exact for the network to stay
   correct.

Past the match, every account the walk took by its chain heads alone is
filled in whole the same way, so the node ends holding every account's chains
and entries, not only a root that matches.

Invariants 13–15 are what make steps 2 and 3 sound: no leaf exists for an
account that holds nothing, the record names every account a block changes,
and a rejected transaction changes nothing but its record.

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
chain entry replayed**, with the message behind each entry of a transaction
chain (§3) — not for verification, which the definition and its
signatures give, but so that the spine's every-block chains are comparable
entry for entry with what the node executes from there, and so that the
node can read what they record when it executes again.

An anchor is accepted when valid signatures from **distinct members of the
producing partition's validator set** — the network definition's, not a key
page's — reach that set's threshold. Copies from one signer do not
accumulate; a second copy from a validator is no second signature.

**The anchor is collected from the partition's own validators** (step 3 of
the algorithm; #4438). To verify partition P's root at block B the node
needs P's own anchor for B, signed by P's validators, and it needs it as B
closes. The copy in `dn.acme/anchors` reaches that pool only after the
Directory executes it, which is far too late for a partition that moves every
block, and a partition's own pool does not hold its own anchors with their
signatures (a BVN's holds none of them). So the join asks each of P's
validators — found by the partition's sequencer service, this node dropped
(#4303) — for anchor number n (`Sequence(<P>/anchors → dn.acme, n)`), and each
answers the anchor it produced, signed with its own key (#4424: committee
members only; a validator that also holds a quorum of the partition's own
copy adds it). Answers are grouped by the sequenced message they carry and
signatures are counted within a group, never across, by the rule above; an
answer under another number than the one asked is refused. The first read
starts at the newest anchor P's anchor ledger names (`MinorBlockSequenceNumber`,
read from a peer: it positions the read and decides nothing) and every read
goes forward from where the last stopped, to the first number no validator
has produced yet. The Directory's own anchors are collected from the
Directory's validators the same way (`anchorsrc.Collector`). The pool reader
(`anchorsrc.Source`, reading `dn.acme/anchors` or a BVN's pool) is no longer
what the join reads.

**Until an anchor is signed by a quorum, no root is handed on.** A root that
has not been verified against the trusted set is not a root; it is a number a
peer sent. What a validator can still do is withhold its signature — a slower
join, never a fork. An anchor that some validator produced and answered, and
that no quorum of the set signed this read, is where the collector is held,
and it is asked of every validator again on the next read. A read held there
is a **stall**, and it is said: the join reports the anchor's sequence number
on `accumulate_join_spine_stalled_entry` (−1 when not held) and logs it, with
the validators asked, once a minute (#4419).

#### 2. Nothing is proven before the match, and the match is the proof

**The accounts a peer serves to a join are as of a block** (decided after
the plan review on #4438, note_3901212756; supersedes Paul's 2026-09-21
"current" rule). An account pulled for a block's record is pulled as the
peer held it at that block, and every page of the walk is as of one block:
the peer's BPT history (`BptPageQuery.ForHeight`) and the historical account
proof (`HistoricalAccountStateProof`, whose retained leaf carries each
chain's `Count` and pending set, so the node can append) serve it. A pull of
current state straddles blocks — the tree it builds is a mixture no block
ever held, and equals a block's root only when the partition is quiet
(#4411) — so after applying block B's record the local root is the root of
B, and can be compared at every block. The bound is the peers' retention
(`BPTHistoryDepth`, 1024 blocks by default): a walk or record the peers can
no longer serve as of its block moves the start forward to a block they can,
and the walk is taken again from there. Pulls run in parallel; a join that
pulls one account at a time cannot keep up with a partition at 100 tps.

**What the pull takes, it writes, and nothing is proven account by account**
(the algorithm, step 3; #4438). There is one proof: the whole local root equal
to the `StateTreeAnchor` of the partition's own anchor for a block, signed by
a quorum of its validators (§1). A peer that serves a false body writes a
false account, and the local root then equals no signed root, so the node
never matches and never hands off on it
(`TestJoinDoesNotMatchOnStateThatDoesNotHashIntoTheAnchoredRoot`): a lying peer
can delay the match, it cannot fake it. The pull still asks for each account
with its receipt, because the answer that carries one is the answer that
carries the rest of the leaf and whose `NotFound` means no leaf (below), and it
still refuses what is malformed: a body served under another name than the one
asked for (#4408), an answer with no body (below), an entry with no message
behind it or a message that is not the entry's (§3, #4400).

**The trusted validator sets move only at the match.** Until the match the
store holds what peers served, and a network definition read out of it would
be a peer's word — and the anchors the match is judged by would be verified
against that peer's validators. So the sets are taken from the node's own
store when the join starts, and again when the local root has matched; a
change to the sets during a join is crossed at the match, by anchors the old
set can still judge (§1's stated limit).

**The node executes only from a pulled state that matched** ("Two
mismatches"). Every account is pulled as it is on the peer now, so the pulled
state is the partition's only at a block the records have carried it to.
The node keeps processing records block by block and compares its root at
every block that sent an anchor; an idle partition anchors on its heartbeat,
so the records carry the tree to the next heartbeat and the comparison is
made there. A root that does not match is a wrong pull, and the accounts are
pulled again — every leaf — while the records go on. The first root that
equals the partition's own signed anchor is the match, step 3; only then does
the node stage and execute (steps 4–6).

**After the match, an anchor mismatch is repaired from the block ledger**
(`PulledState.RepairFrom`). The anchor a block produces that is not the
partition's signed one means execution differed, not that the pull was wrong.
The node takes the partition's record of that block and its own, and pulls
every account they name again, **whole**: main state, every chain with every
entry and the message behind each, the pending list and the directory. A
chain is taken again from its first entry when the node's is not the peers'
prefix, or is longer than the peers' (a chain the node grew wrongly is
replaced, not appended to); the account's chain index becomes the peers';
and the entries below each chain's open mark set are brought in at once
(`pull.Backfill`), so a repaired account is held entire. Then it moves on to
the next block.

**Past the match, every account taken by its heads is brought in whole.** The
walk and the records take an account by its chain heads and open mark set
(§3), which reproduce its leaf and let the node append. Once the node is
`ACTIVE`, the root watch backfills those accounts, 64 a check, while the node
executes (`pull.Backfill`): the entries below each chain's open mark set, the
mark points that close their sets, and the message behind every entry. It
writes nothing at or above a chain's head, so it does not race the blocks the
node executes; the entries are held to the node's own head, replayed from the
first, so a peer serving another chain is refused and the next is asked. The
node ends holding every account's chains and entries
(`TestARestartFarBehindJoinsAPartitionThatMovesEveryBlock`).

**A message is proven by its entry.** The leaf covers a chain's head, and the
head covers its entries, but an entry of a transaction chain is the hash of a
message and the message is not under the root. So a message the pull takes
beside an entry is kept only if it hashes to that entry, and nothing about the
peer that served it is believed (§3, #4400).

**Every leaf has a body.** The state tree holds a leaf only for an account
with main state (invariant 13), so an answer that carries a receipt and no
body describes a leaf that does not exist: it is refused as a failure of the
source that gave it, and the next source is asked. It never counts as the
account having no leaf, and it never drops the name. Before #4437 such leaves
did exist — a failed transaction left one for its missing principal — and
the join had to pull them and take their existence on peers' word (#4397,
#4406); that is gone with them. `NotFound` means the peer's tree holds no leaf:
a name every source answers that way is dropped rather than asked again, and a
record names it again if a block ever gives it one; a name some source failed
to answer is asked again next round.

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
and would drop an account they all hold. It is a reader's capability. It is
not what a join stands on.

**A body served with a proof is the stored body, byte for byte.** Nothing
derived may be filled into an account on the way out of the API, because the
receipt served in the same call is built from what is stored, and the local
leaf the pulled body hashes to must be the peer's leaf or the local root never
matches. `Received` on a sequence ledger was derived from staging and filled
in on read, and it left every restarting node unable to pull
`<partition>/anchors` from anybody (#4295); it is stored now (#4412), and
served as stored. A derived value travels **beside** the body, in its own
field, and a reader merges it after it has checked the proof.

**The answer carries what the leaf hashes, and the pull writes what it
carries** (#4399). Besides the body, the directory, the pending list and the
chains, a partition's `synthetic` leaf hashes its two delivery queues and its
`ledger` leaf the root of its scheduled events. A current answer with a
receipt carries them in its `Leaf` — the queues, and the events themselves
rather than their root, since a root cannot be written — read from the batch
the receipt is built from; the pull replaces what the node held with them,
and a queue or an event set served empty clears the node's. The block lists
the executor finds the events by are not in the events tree, so no leaf check
covers them: the pull derives them from the event sets and never takes them
from the answer. **What is not carried** is the signature material of a
pending transaction (its validator signatures, payments, votes and
signatures, which `hashPendingV2` hashes): an account holding a pending
transaction whose sets are not empty still does not hash to the peer's leaf
(#4298, DIFFERENCES.md E11). A queued local delivery executes from its stored
message at the next block, so the pull also fetches each queued message and
keeps it only if its hash is the queued ID.

**BPT pages are read, never written.** A leaf enters the local tree only as
the hash of state this node holds, because the local root is what the node
matches against a signed root; a leaf taken from a peer's page would make that
root the peer's and the match would say nothing. Pages name accounts and say
what the peer's leaves are; the difference from the node's own is what the
walk pulls. A page carries no proof, so a peer can omit a leaf, and the root
failing to match is the only detector (#4301).

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

**The pull is the whole tree and every record from its start, in order**
(the algorithm, steps 1–2; #4438). The pull starts at `S`, the block the
peer's ledger names when the join starts, and keeps two cursors: the walk's
place in the peer's BPT, and `L`, the last block whose block-ledger record has
been processed, which starts at `S`. Each round, in this order:

1. **The records.** The block-ledger record of every block after `L` through
   the peer's block, in block order (`QueryMinorBlock` with the entries not
   expanded; a block with no record is an empty block and changed nothing).
   Every account a record names is marked current and pulled; `L` moves to
   the last block read. An account several records name is pulled once for
   them all: every pull of the round is of the peer's state now, which is at
   or after every block read. A record that cannot be read stops the round's
   records there, and the next round goes on from it.
2. **What is owed.** An account a pull could not take — no peer answered it
   whole — is asked for again, once a round, whoever named it.
3. **The walk.** The next pages of the peer's BPT (`enumerate.ReadPage`,
   eight pages of 256 leaves a round). An account a record has named is
   skipped: the page may be older than that record, and **the walk never
   overwrites a value the records wrote**. Every other leaf this node does not
   hold, or holds and does not agree with, is pulled and written, with no block
   compared: a page newer than the records wrote an account a later block
   changed, and that block's record names it and pulls it again, so every
   account ends at the state of the last record processed
   (`TestTheWalkNeverOverwritesWhatARecordWrote`,
   `TestAWalkPageNewerThanTheRecordsIsCaughtUpByTheRecord`).

Once the walk has covered the tree and nothing is owed, the node compares its
root with the partition's signed anchor at every block that sent one, and it
executes only from a root that matched (§2, "Two mismatches", 1). The set of
accounts is the block
ledger's, not the block's envelopes': every block records `(account, chain,
index)` for every chain its execution changed (see "The block ledger"), and it
names every account whose leaf the block changes (invariant 14), including the
accounts a block changed as a side effect and the system accounts every block
touches; envelopes name neither all of that nor only names that can be routed.

**After the handoff, a mismatch is an anchor mismatch** ("Two mismatches",
2): the node repairs the accounts the block ledger names for that block —
the partition's record and its own — and moves to the next block; it does
not re-pull the tree, which the match proved. **The BPT has every leaf; no
pull skips one.** The only thing that decides a leaf does not exist is
execution: a transaction whose principal does not exist is rejected before it
changes anything, leaves no leaf for that principal (invariants 13, 15), and
its refund is a synthetic transaction logged on the rejecting partition's
synthetic chain back to the sender. The block this node's executor last
executed is read from `SystemData(partition).ExecutedBlock` — not from
`<partition>/ledger`, which is an account the pull overwrites (#4295, #4344)
— and it bounds the node's own records the repair reads.

**What is pulled is written into the state tree, not only into the store.**
Committing an account does not move the root by itself; the root is what the
node matches against a signed root, so a pull that does not update the tree
can fetch everything the network has and never move (#4305).

**What is pulled is what the node executes from.** The pulled state replaces
what the node holds for that account rather than joining with it, or a restart
keeps entries the peer has dropped and the account never hashes into a
signed root again. A chain is taken with the entries of its open mark set —
the entries since its last mark point — because an append rebuilds the chain's
tail from them, and a node that cannot append to its chains cannot execute
block `Q + 1`.

**Syncing and bootstrapping are one walk at two depths.** The spine's chains
are taken entry by entry, and the node fills them back from the account's head
until it **meets data it already has**. A bootstrapping node never meets any
and collects the whole chain; a restarted node meets its own at once and
collects nothing. Same walk, different stopping point — which is why a defect
at the meeting point is invisible to every bootstrap test and fatal to every
restart. A spine account is taken this way **whenever** it is pulled, not
only when the join takes the spine first: every block's record names
`<partition>/anchors`, since every block writes the pool, and a pull that took
it as a head and an open mark set left its new entries with no message behind
them (#4421). What the node already has is an entry
**and the message behind it**: an entry held without its message — a store an
earlier join wrote that way — is not met but fetched, once per process, for
the newest entries the first block's reads can reach. A chain that is not
the peer's once the peer's entries are appended to it — a node that executed
from a wrong state appended entries of its own — is taken again whole, from
its first entry, with its messages: the node's history is not compared with
the peer's to find where they part, since at an anchored height there is one
correct chain. A peer that serves fewer entries than the node holds is
behind, and is asked again — except in a repair, where the node's longer
chain is its own wrong growth and is replaced by the peers', taken whole from
the first entry (Paul, "Two mismatches": a chain the node grew wrongly is
replaced by the peers', not appended to; this settles #4403's
provisional rule). The retake tracks no orphaned entries.

**A pulled transaction chain carries the messages behind its entries.** The
entries are hashes, and the executor reads what they name: the first block a
new process opens seeds its producer cache by walking the anchor pool's main
and anchor sequence chains and loading the message behind each entry, and
whether that block records an anchor is decided from the message behind the
newest anchor sequence entry (`lastAnchoredBlock`) — which is consensus
output, so the message is needed, not a fallback. A chain taken as hashes
alone left every restarted validator that fell one anchored block behind
unable to open its first block (#4400, run `20260924T052134Z`). So each entry
of a spine transaction chain the pull replays comes with its message (asked
for expanded), **each message is checked against its entry hash**, and it is
written with the entries and discarded with them. What a peer
stores under an entry is the message as it arrived, or — for a wrapper whose
transaction is stored under its own hash (an anchor, a sequenced or synthetic
message; the executor's stored form, #4236) — the wrapper referring to that
transaction by hash. Such a message is checked by putting the transaction
back, itself checked against its own hash, and hashing the result; it is kept
in the stored form with the transaction beside it, as the peer keeps it — a
stored form the node builds from the checked message, the transaction
referred to by hash under the principal it names, and not the one served:
the reference is replaced whole when the message is checked, so its header is
under no hash (#4416 review F3). A
transaction served as a remote stub is not a transaction's body: its hash is
whatever the stub says. **A peer that serves an entry with no message behind
it, or a message that is not the entry's, has not served the chain** — a peer
that itself joined holds the blocks it did not execute that way — and the
next peer is asked; a hash is never kept without its message.
**A pulled anchor pool carries each anchor's signature history.** The query
API reads an anchor's signatures from the pool's per-transaction history
index into its signature chain, and the sequence they cover from the
transaction's cause (`loadMessage`); executing an anchor writes both beside
the signature chain entry (`RecordHistory`, `recordMessageAndStatus`), and
neither is on a chain. A node that took the chains without them held every
anchor appended by a block it did not execute with no signatures, and could
serve none of its pulled range (#4413, #4416). Executing a copy also adds
its signature to the anchor's validator signature set, which is what the
executor counts the anchor's quorum from (`anchorSignaturesFor`,
`anchorIsAdmissible`); it is under the account hash only while the anchor is
pending, and an anchor below its quorum is not recorded pending, so a pulled
account verifies without it. A node that joined holding no set for an anchor
its peers held one signature of counted the next copy as one where they
counted two, held the anchor they executed, and its state diverged on a block
after a good handoff (#4416 review F1). So for each entry of a pulled
signature chain that is a validator's signature of an anchor, the pull writes
what executing it wrote — the history index and the signer; the entry's
signature added to the anchor's validator signature set, sorted by public key
and one per validator, to what the node already held (execution records a
history entry exactly when it adds a new signer to the set, so the set is the
signatures of the transaction's history entries, whether the anchor executed
or is still below its quorum); and, for an anchor that executed — its
transaction an entry of the pool's main chain — the sequenced message, in its
stored form, and the cause — **rebuilt from the entry, never taken from a
peer's word about it**, and **each signature is checked to be a signature of the
anchor it carries**, in the anchor's own form or the Directory's reused one
(the executor's `checkSignature`); an entry whose signature does not verify
has not served the chain, and the next peer is asked. Which validators may
sign is not checked by the pull: the chain is under the root the node's state
matches, which a quorum signed, and the pull does not hold the
validator set of every block it takes — checked against today's set, the
honest history of every anchor signed before a change to it would be refused
and the pool never pulled. Membership is checked by each reader, against the
set it trusts, when it is served the anchor (`anchorsrc.verify`). The
accounts pulled state-only carry no messages: their chains are taken as a head and an
open mark set, for appending (§3 above), and a block that reads a message
behind one of those entries reads it only for a block the node executed (§6).

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
is the #4290 divergence. **The node executes anyway** ("The algorithm", step
6; "Two mismatches", 2): it requests the missing number from the source, and
a block executed without an entry it needed is wrong only in the accounts its
record names; the anchor check finds it at the next block that anchors, and
the repair brings those accounts — the synthetic ledger's `Delivered` among
them — to what the peers ran (§2). The gap is said and put on
the gauge (below); it no longer holds the handoff. Before, the node advanced
the sync one block at a time until a block had no gap (#4362).

**A restart is a join, and it finds a gap when it stopped holding an
unexecuted entry.** Staging is memory: whatever the node held unexecuted when
it stopped — a synthetic waiting on the anchor that proves it — is lost, it
arrived before the node was listening again, and the first check at `Q + 1`
finds a gap at those numbers once a later number on that stream has been
sighted; with nothing later collected the check sees no gap, and the hole is
an anchor mismatch after the handoff ("Two mismatches", 2). A node restarted with nothing held loses nothing
and finds none. So whether a restart meets a gap is the traffic's in-flight
state at the moment it stopped, not the code: an entry is held only while
the anchor that proves it has not arrived
(`TestRestartGapAtQPlusOneIsAnEntryHeldBeforeTheRestart` gaps only in its arm
where the anchors lag the synthetics, at exactly the numbers the node held
before it stopped). Run `20260924T111811Z` gapped at `Q + 1` on 3 of 3 BVN
restarts and runs 2 and 3 on 0 of 3; what each restarted node held when it
stopped is not in those runs' logs, and the gap line below is what will say
(#4432). The gap check reads staging and the pulled ledgers'
`Delivered`, and no message's status: what a message records when it is held
(#4423) does not enter it.

**The gap line names what it found** (`join.StreamGap`, #4432). For every
stream with a gap, `The next block has a gap; executing anyway, and repairing on
a mismatch` carries, under
`gaps`, the stream as `source->ledger`, its `Delivered`, the first run of
numbers nothing is held for (`missing=<first>-<last>`), the highest number held
and the highest a validated hash stands at. The same first missing number is
on the gauge `accumulate_join_gap_first_missing{partition,stream}`; each check
replaces the partition's series, so a stream is on it only while its gap
stands.

**The root is the check that does not depend on the sequence numbers.** After
executing any block, the local BPT root equals that block's proven root or
it does not. A mismatch — a gap in staging, a pulled state that was a
mixture, an account only this node's execution touched — is repaired from the
block ledger (§2) and the node goes on, so a wrong run is caught at the next
block that anchors, never carried forward. **A mismatch demotes the node to
`BOOTING`** (§6): from the mismatch until an executed block's root matches
again it refuses every read, serves nothing and relays every submission, and
the match makes it `ACTIVE` again. A node hands off only from a state that
matched ("Two mismatches", 1) and is `ACTIVE` at that handoff. What a
`BOOTING` node emits besides is §6's rule. A node whose state is known wrong is not one that
answers for it (#4385: run `20260924T074702Z`, a Directory node frozen at
block 661 with its gauge reading `ACTIVE` served 693 pulls at that block, and
2,944 of the run's 2,973 stranded submissions were deliveries handed to such a
node, which accepted them and never certified or relayed them).

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

When the local BPT root equals a signed anchor's root (§2), `Q` is the block
the ledger in that state names, and staging is brought to `Q`: everything
collected through `Q + 1` held, everything at or below each stream's
`Delivered` at `Q` — read from the pulled ledgers — released, and proofs
decided against the anchors executed by `Q`. `Q` is checked against the
state, not taken on trust: staging settled against another block than the
state it is paired with executes a different block than the peers, which is
the failure the join exists to prevent.

**At the handoff, staging holds everything collected through `Q + 1` and
nothing collected after it.** The buffered groups up to and including the one
that is `Q + 1` — by leader round, below — are taken into staging in the order
consensus committed them, before the gap check, because the gap check asks
what `Q + 1` carries (§4). The groups after `Q + 1` reach staging only by
being executed, as they reach a peer's: a peer executing `Q + 1` holds nothing
that arrived in `Q + 2`, and a block delivers the contiguous run from what is
held, so a node whose staging already held the arrivals of `Q + 2`, `Q + 3`, …
executes a longer run than its peers and a different block (#4398: run
`20260924T052134Z`, a BVN joined at 203 with eight groups buffered, and its
block 204 delivered Directory anchors and produced synthetics its peers only
received in 205–211). Staging only grows as the sync advances — `Q` only
moves forward — and a group that is `Q + 1` but has not been collected yet is
waited for (`NotReady`): a gap check without it would not see the gap it
carries (`Service.StageThrough`, called by the join after the match and before
the gap check).

Its line, `Staged the buffered groups through the block after the state`, says
how many groups it staged, how many are `after` the block — not staged, because
they reach staging only by being produced — and, under `streams`, per stream,
how many of the staged groups' arrivals were held and why each of the rest was
not: `delivered` (at or below the store's `Delivered`), `horizon` (past the
sanity horizon), `duplicate` (its number already held; first sighting wins),
`refused` (its own executor refuses it) or `unattested` (not proven here and
not signed by a validator of its source, #4243). A proof dropped for want of
budget takes nothing with it (#4439), so it is not a reason. A gap is an
entry the node did not hold; the reason says whether its peers held it either
(#4432).

The node then executes block `Q + 1` from the buffer as any node executes a
block, and it is a validator or a follower from there. **The handoff that
succeeds is what makes it `ACTIVE`** (§6), not the match: a node whose
root matches `Q` and has not handed off executes nothing, and if it served
from there it would answer for a block it is not executing past (#4413: run
`20260924T074702Z`, a node that read `ACTIVE` from its first match, never
handed off, and served stale anchors to the next joiner).

**Which buffered group is `Q + 1` is decided by the leader round, read from
the state.** A collected group has no block number; it has the leader round
consensus committed it at, and consensus commits leaders in one order on
every node. From v2-kourou the system ledger records, for each block, the
leader round that committed it — `SystemLedger.LeaderRound`, field 10,
written by the executor from the round the DAG service produced the block
with; before v2-kourou the field is never written and the ledger encodes as
it did. The pulled ledger hashes into the signed root, so the handoff reads
`Q` and its round `P` from the same record, and a ledger whose `Index` is
not `Q` is not the state `Q` is. The groups at or below `P` are in the
state; the groups above it, in the order consensus committed them, are
`Q + 1`, `Q + 2`, … (`Service.performHandoffAt`). Nothing is counted: the
first design numbered the buffer from the block the node stood at when it
started collecting, which depends on when collecting started relative to the
service learning its block (#4351) and produces a second time, under numbers
that are not theirs, any group consensus delivers again after a restart.
The handoff does not happen, and the buffer is left as it is, when:

- `P` is above every round the node has executed or collected: the groups up
  to it are still arriving. The join waits (`NotReady`).
- `P` is below the round the node's consensus stood at — the round of the
  last block it produced, or the checkpoint's committed round it resumed
  from (consensus.md, "Restart"): the groups between were committed before
  the node listened, so they are in neither the buffer nor the state. The
  join pulls again, to a newer state (`Conflict`).
- No group this node's consensus committed is at `P`, or the ledger records
  no round at all (a block written before v2-kourou). The first is a state
  this node's consensus did not produce (`Conflict`); the second says
  nothing about which group is next, and the join waits for a state that
  records one (`NotReady`) rather than fall back to counting.

**A handoff that cannot be performed sends the node back to syncing; it does
not end the join.** A buffered group the executor cannot produce — it refuses
to open or to execute the block — stops the handoff at that group. The blocks
before it were produced and stand: the node stands at the last block it
produced and returns to collecting mode holding the groups it did not produce,
in order, with what it had already staged still staged. The join then syncs
again and tries again from the state it reaches, as it does after a root
mismatch ("Two mismatches"); nothing is dropped, and the node is never left neither
collecting nor executing (#4401: run `20260924T052134Z`, a Directory node
whose first produced block failed sat for the rest of the run with its buffer
discarded, refusing every later handoff as "not joining" and dropping every
committed group). **A failed handoff demotes the node to `BOOTING`**, as a
re-sync does ("Two mismatches"): it matched a root, but it is not executing from it, and
it serves nothing until a handoff succeeds (#4385). A handoff that is retried
is `BOOTING` throughout — the match does not promote — so a failure that
recurs on every attempt never flips the node's state. The retry has no bound, because a node that stops trying
executes nothing and collects nothing; so **every failed attempt is counted**
— `accumulate_join_handoff_failures_total{partition}` — and logged as an error
with its attempt number, and a failure that recurs on every attempt is seen
as a climbing count rather than as silence.

A follower differs from
a validator in what it does with the blocks it processes — it does not vote or
propose — not in how it gets there; what it does with a transaction it cannot
propose is §6's rule: it relays it, and never drops it.

While all of this runs the node **listens**: it subscribes to consensus and
takes every committed block from then on into a buffer — collected, not
executed. Staging is filled from that buffer only through the block after the
state the join proves (above), never as a group arrives. A collected block has no index; the node executes
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
history. The producer cache fills by execution, and once more at the first
block a process opens, when it is seeded from the store by position (#4241):
the anchors this partition produced, from the anchor sequence chain; the
Directory's receipts of its blocks, from the anchor pool's main chain; and the
synthetics of its own blocks still in flight, from `<partition>/synthetic`.
The first two read the spine's chains and the messages behind them, which the
join carries (§3), and an anchor is the partition's whichever node executed
the block that produced it. The third skips **only the blocks the node did not
execute**: it produced none of their synthetics, holds none of their messages,
and answers for none of them (below). **The store says which they are**, not a
memory of the join: a block whose synthetic entries the store holds with no
message behind them — or whose entries it does not hold at all — is a block a
join carried the node past, because the join pulls `<partition>/synthetic` as a
head and an open mark set, and a node that executes a block writes its entries
and their messages in the one batch. Every other block in the window is the
node's own and is rebuilt, so a restart that fell nothing behind skips nothing,
and a restart after an earlier join skips that join's blocks however many
processes ago it was (#4400). A seed that fails is not a seed: the
next block tries again, and no block opens on a cache nothing filled. The
spine is the one place a join takes chain entries and the messages behind
them; nothing under this section fetches the history of any other account a
node did not execute — that is phase 3's conversion of history, or phase 2's
database node, not a syncing node's work. So the node states are two, and
one rule divides them: **`ACTIVE` serves while the node is executing in
agreement; `BOOTING` refuses and relays at every other time** (#4385).
**`BOOTING`** is from the start of a join until its handoff succeeds (§5)
— a root that matches is not enough, because a node that has matched and not
handed off executes nothing — and again after any demotion: a re-sync after
a root mismatch ("Two mismatches") and a handoff that fails (§5) each return the
node to `BOOTING`. **`ACTIVE`** is from a handoff that succeeds until the
next demotion, and from its first block for a node that took nothing from a
peer — a node that never joined has no state machine at all and serves as
`ACTIVE` (#4368). `BOOTING` means everything below: reads refused with
`NotReady`, submissions relayed unexamined, and the gauge reading `BOOTING`
until that handoff succeeds. **Two things do not wait for `ACTIVE`**
(decided after the plan review, note_3901212756). *The sequencer answers for
every anchor the node itself executed and signed, whatever its state:* the
anchor collector gathers a partition's anchors from its validators, and a
validator that answers only while `ACTIVE` makes a partition with more than
N − threshold validators booting at once unable to prove anything to
anyone, forever. *A node signs and dispatches block N's anchor and
synthetic transactions only if the newest root it could check matched*
(lag one): a node whose last checked root did not match signs nothing,
dispatches nothing, and repairs. A wrongly executing node then stalls
rather than forms a wrong quorum with others like it, and a partition whose
validators are all booting still anchors. *What `BOOTING` emits today:* every
block a node executes has its anchor signed and sent and its synthetic
transactions dispatched, whatever the node's state — the conductor's gate is
committee membership alone and dispatch reads no node state — until #4443
builds the lag-one gate (DIFFERENCES.md E11). `COMPLETE` and `WAITING`, which
named a backfilled history, are retired: nothing reached them and nothing
could. What a joined node cannot answer *for a block it did not execute* —
an entry the sequencer is asked for from before it joined — it refuses per
request with `NotReady` naming the block it joined at, never `NotFound`,
which a requester counts as a miss (#4295, DIFFERENCES E11).

**A node serves only what it can serve signed.** An anchor is worth the
quorum that signed it and nothing else, so a node that holds an anchor
without its signatures does not serve it: an expanded read of an anchor pool's
main chain whose entry is an anchor carrying no signatures is refused with
`NotReady` (`servedSigned`, `queryChainEntry`), and the reader asks a node
that executed it. A node that joined by a pull from before #4416 holds exactly
that for its pulled range until the pull brought the signature history an
anchor's signatures are read from: served bare, every reader that checked
them refused them as unsigned, 22 in run `20260924T074702Z` (#4413). An
`ACTIVE` node refuses an anchor it holds unsigned rather than serving it
(#4413), and the pull now brings the history (§3), so a node that joined
serves its pulled range signed (#4416). The pool's anchor sequence chain is
not refused: it holds the anchors the partition *sent*, whose signatures the
receivers hold and the producer never does. On the reading side, **an anchor
served with no signatures, or a pool entry served without its body (an error
record in its place, or nothing), is the serving peer's gap, not a fact about
the anchor**: the anchor source does not move its cursor past it, and the
next page asks the next peer for it (#4413, #4418); an anchor that carries
signatures and fails is refused once and passed, because every peer serves
the same signatures. The node's state
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
`BOOTING` refuses with `NotReady`, `ACTIVE` serves. "Every read" is the rule
and, since #4368, what the code does: `servingFor` refuses every query while
the node is `BOOTING`, counted per call. Tracking
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
Nor does it sign an anchor's healing answer for that partition (#4424):
asked for an anchor, it answers with only the validators' signatures it
already holds, and "not yet" when it holds none. A synthetic it serves like
any node that holds the entry — with its collection proof, which is what the
destination checks, under its own signature, which the destination requires
on the wire and does not require to be a validator's. The requester keeps
only the anchor signatures whose keys are active on the source partition in
its own globals before it builds the envelope, because the destination
refuses a whole envelope at its first anchor signature by a key outside the
committee — the quorum's good copies with it ([healing.md](healing.md), "The
answer").

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

Every block leaves a record of what it changed: for block *N*, the list of
(account, chain, index) entries the block's execution appended, and an entry
naming no chain for every other account whose state the block changed. This is
the **block ledger**. It names every account whose state-tree leaf the block
changes — the system ledger, which the record is written into, and the
receiving side's synthetic ledger, which moves its delivered positions without
an append, included — and no account that holds nothing (invariant 14). It is the only place the block-to-chains direction exists
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
   reads from a stream's ledger to place the stream is `Delivered` — what has
   been processed. The ledger also records `Received`, how far the stream
   has arrived (#4412); the block writes it from its own arrivals and nothing
   in staging reads it.
6. **Staging is identical on every node.** It is fed only by consensus, so it
   is a deterministic function of the same input everywhere. A node that joins
   or restarts pulls the state and collects consensus from the moment it
   listens ("Sync", "The algorithm"); what it lacks between `Delivered + 1`
   and `Received` it requests by number, and the answers reach its staging
   through consensus. A node whose staging differs from its peers' will
   execute a different run and produce a different block hash, and that is
   an anchor mismatch, repaired ("Two mismatches", 2).
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
12. **A copy of an entry that is held is not executed and nothing is recorded
    for it: a message's status says Delivered only once its stream has moved
    past its number.** This is invariants 4 and 8 applied to a message that
    wraps another. A synthetic copy whose inner sequenced message is only held
    — not next on its stream — records no status of its own, exactly as a
    collected one does. A status is keyed by the message's hash, so a
    Delivered written for a held copy is also the status of every
    byte-identical copy, including the one staging holds, and that one then
    answers "already delivered" when its number comes up and executes
    nothing: a stream frozen with every number held and nothing missing
    (#4423).
13. **The state tree holds a leaf only for an account that exists.** An
    account exists when it has main state. A write that touches only an
    account's bookkeeping — the votes and payments recorded against a
    transaction — does not create an account, and the block inserts no leaf
    for an account that holds nothing. Clearing a record that is already
    empty is not a write, and a missing account keeps no history: a
    signature, payment or request for a transaction whose principal does not
    exist is not appended to that principal's signature chain. An account
    with no main state that does hold something — a chain, a pending
    transaction, a directory entry — is a defect, and it is never hidden: it
    keeps its leaf and is counted
    (`accumulate_database_stateless_account_leaves_total`) and logged. Before this
    rule, every transaction that failed against a missing principal left a
    leaf hashing to nothing (`db56114e…`), identical for every such account:
    state no peer could serve with a body, which the join then had to pull
    and trust on peers' word (#4397, #4406, #4437).
14. **The block ledger names every account whose leaf the block changes, and
    no account that holds nothing.** A joining node follows the partition
    block by block from the block ledger alone, so an account the record
    leaves out is one the join never pulls again, and an account it names
    that holds nothing is a leaf no peer can serve (#4437).
15. **A transaction is checked before it changes anything.** A synthetic
    transaction is never refused at validation — refusing it would stop its
    stream — so it is checked when it executes, and a rejected one changes
    nothing but its own record: the message and its failed status, the
    stream's delivered position, and the refund it produces. Its missing or
    wrong principal is not touched (#4437;
    `TestARejectedDepositChangesNothingButTheRecord`).

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

**Ungated, likewise: a held copy records nothing (invariant 12, #4423).** Before
it, a synthetic copy whose inner message was only held recorded its outer
message, a Delivered status and its signer's signature at arrival; now it
records none of them, and neither does the copy when its sequenced message
later runs from staging, since staging runs the sequenced message and not the
copy. That changes the state hash of any block that held a synthetic out of
order. It is ungated under the same fresh-install rule: this line runs no
network that outlives a run (DIFFERENCES.md, E15).

**Ungated, likewise: every block writes `Received` (#4412).** It was never
written on this line; now every block that holds an entry or delivers one
writes it, and a block that only held something commits rather than being
discarded as empty. Both change state hashes. Ungated under the same
fresh-install rule (DIFFERENCES.md, E16).

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
conflict, invalid, unbound, duplicate, refused (past `maxAnchorAhead`),
dropped (over the byte budget, below).

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

**The budget bounds memory and decides nothing else** (Paul, 2026-09-24,
#4439). It is read from staging, and staging is memory: a restarted or
joining node's staged proofs are not its peers', so whatever the budget
decides differs between nodes. When it binds, the proof is dropped
(`accumulate_exec_staged_proofs_total{dropped}`) and that is all. The entries
that travelled with it are held and counted exactly as they would have been
with the proof staged — they are consensus input, and `Received`, which is
hashed, counts them ("What the stream ledger is for"). A dropped proof is not
lost for good: an entry held with no validated hash and no staged proof is a
**gap of proof**, and once its stream stops at it the requester asks the
source for the span, whose answer — the span's proof with its entries —
reaches staging through consensus like any other (healing.md, "Deciding, in
staging"). So nothing is stranded waiting for a proof that arrived and was
dropped, which is what the budget did when it was counted in Directory blocks
(#4282: 80,552 entries held with no gap for healing to find). Until #4439 the
entries were refused with their proof instead, which kept them a hole the
source still held; that made whether a node counted an entry depend on its
memory, and `Received` with it.

What the budget must also not decide is *when* an entry executes, and today it
does: a node that dropped a proof its peers staged executes the package when
the fetched proof lands, blocks after its peers executed it on theirs, and
from that block its state is not theirs (DIFFERENCES.md, E17).

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
from the ledger's `Delivered` and advanced in place, and at close
**`Delivered` and `Received` are written back** ("What the stream ledger is
for"). It holds a reference rather than a copy of the
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
same input always produces the same staging. A later copy is not only kept out
of staging: **a later copy writes nothing.** It records no status, no message and
no signature, so it cannot change what the held entry does when it runs
(invariant 12). A dispatcher resends an envelope the destination already took
when the answer is lost after the send, and the requester submits its heal
answers through a dispatcher too, so a byte-identical second copy is the normal
case, not a fault. (Two separate heal answers are not byte-identical: the
source signs each when it gives it.)

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

Two fields, in the inbound direction, both written when the block closes
(`flushStreams`, step 1 of "Closing a block"):

- **`Delivered`** — what has been processed. It is read to place the stream.
- **`Received`** — the highest number that has entered staging from consensus
  by the end of the block (Paul, 2026-09-25, #4412; "Sync", "The algorithm",
  step 6). It is the ledger's previous `Received`, raised to the highest
  number this block's own execution held on the stream, and to `Delivered`.
  It never decreases and is never below `Delivered`.

`Received` is counted where the block holds an entry (`Block.hold`: an entry
held behind the next number, a synthetic collected without an anchored proof,
an anchor copy below its quorum), from what the block's consensus messages
carried — never read back from staging. Staging is memory, and whether it
takes an entry also depends on what this node already holds and has
validated, which after a restart is not what its peers hold; `Received` is
hashed, so it may depend only on the state and on the block. For the same
reason no hold depends on the anchor-staging budget: an entry whose package's
proof was dropped for want of budget is held and counted like any other
(#4439, "Anchor staging"). A number at or
below `Delivered` is not an arrival, and one more than `MaxStageSpan` above
it is held nowhere, so neither counts. A heal answer counts exactly as any
other arrival does, because it reaches the block through consensus like any
other; nothing a node fetches or learns outside consensus raises it.

**A block that raises `Received` commits.** An empty block's batch is
discarded, so a raise in a block judged empty would be lost, and carrying it
to a later block in memory would put a per-node value into hashed state. What
decides it today is the held message itself: `SyntheticMessage.Process` and
`BlockAnchor.Process` set a transaction state for every message they process,
held or not, and merging it counts the block as having delivered something
(`BlockState.MergeTransaction`), which `Empty` checks. `BlockState.ReceivedRaised`
is also checked, so the rule does not rest on that bookkeeping; no block
reaches it today.

What `Received` is for: a node that joins takes the state at the matched
block B, and its staging is consistent when it holds every number from
`Delivered + 1` to `Received` as of B ("Sync", "The algorithm", step 6). An
entry its peers held before it began staging, on a stream that has gone
quiet behind a hole every node shares, is otherwise invisible to it, and
executing without it diverges (run 20260924T074702Z, Directory block 658).

Nothing else about an inbound stream lives there. There is no pending array,
because the held set is staging's. `Produced` remains, but it belongs to the
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

**The API serves `Received` as stored**, in the body. Until #4412 it was not
stored: the API answered it from the serving node's staging, beside the body
(`AccountRecord.Sighted`), because filling a derived value into the body
makes the body stop hashing to the leaf its own receipt proves (#4295). That
derivation is gone; `Sighted` is no longer filled. Staging's own sighted mark
remains what the `Stream position` log line reports as `sighted` — this
node's memory, which after a restart can be below the ledger's `Received`.

`msg_sequenced.go`, `SequencedMessage`: `isReady` asks the block's position
whether the message is next. Ready messages execute; not-ready messages record
pending and execute nothing. `Process` records the message and its status, then
advances the stream — the advance is deferred so it lands only once everything
the message records has, and never on a path that discards.

### Closing a block

`block_end.go`, in order — the order is part of the contract, because each step
depends on the last:

1. Write each stream's advances to its ledger, once per stream (#4169 step 7):
   `Delivered`, and `Received` ("What the stream ledger is for").
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
  `delivered` (the ledger's, or staging's if higher), `advanced` (by this
  block), `received` (the ledger's `Received`, #4412), `sighted` (the
  highest number this node's staging ever held), `reach` (how far validated
  hashes stand), `held` (entries in staging), `waiting` (the first number
  above Delivered nothing is held for, 0 when none). `received` and
  `sighted` are logged side by side because they can disagree: on a node
  that rejoined behind a hole its peers hold entries behind, the state says
  N arrived and this node's staging has sighted less, which is #4412's
  failure read straight off the line. A stream whose ledger `Received` is
  above `Delivered` is behind, and is logged as such even when staging
  holds nothing for it.

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
- **`Stream stopped`**, when an entry staging offered as runnable ran and
  did not move its stream: `block`, `ledger`, `source`, `number`, `reason`
  (`not-delivered`, or `error` with the error), and `status` — what the
  entry's execution said of itself. Counted every time
  (`accumulate_exec_run_stopped_total{stream,reason}`), and logged once per
  stream and number, and again no more often than once a minute of block time
  while the stream stays stopped there. Nothing stops a run legitimately: an
  anchor below its quorum is never offered, because it is not runnable, and a
  synthetic entry offered as runnable must move its stream — any non-zero
  count, under either label, is a defect. The run stops at such an entry
  every block, and before this line nothing said so: #4423 froze a
  stream with `waiting=0` and the entry at the head read `delivered` for as
  long as the run lasted.
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
   `block.State.ChainUpdates.Entries` — every (account, chain, index) the block
   appended — plus one `BlockEntry{Account}` naming no chain for every other
   account the block wrote that has main state, and the system ledger itself
   (`blockLedgerEntries`), sorted. The chain entries are also what the root
   chain anchors; the account entries go into the record only. It is written
   after every other write the block makes to an account and immediately
   before the BPT update, so no leaf changes after the record is built
   (#4437; it used to be written before the major block, whose append to the
   anchor pool then went unnamed). The
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
settles it: it is logged (`Destination refused a dispatched envelope`),
counted and not retried, and handed back to whoever registered for refusals
— the conductor, whose requester treats a refused heal as a failed one
(healing.md, "A refused heal is a failed heal"; #4426). The queue is bounded in **blocks**:
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
