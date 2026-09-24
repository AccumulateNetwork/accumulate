# Database Abstraction — Specification

## 1. Architecture — what we are doing

Accumulate stores all state as key-value pairs and does not depend on any
particular store to do it. LevelDB, Badger, Bolt, BlockchainDB and an in-memory
map are interchangeable; a node chooses one at initialization and the protocol
above is unaware of the choice.

### What a store is

A store maps a **record key** to an opaque **value**. It does not interpret
values, and it does not need to preserve key order — iteration is unordered and
yields keys as hashes.

Four operations, and no more: get, put, delete, iterate.

### The objects

- **`record.Key`** — a structured key (`Account(url).MainChain.Element(3)`),
  reduced to a 32-byte hash for storage. The hash is what a store holds; the
  path is what the model above uses.
- **`Store`** — the four operations.
- **`ChangeSet`** — a store with `Commit` and `Discard`, which can itself begin
  nested change sets.
- **`Beginner`** — anything that can begin a change set.

### The invariants

1. **A zero-length value does not exist.** A store may hold an empty value; a
   read reports it as not-found. Deletion and "never written" are
   indistinguishable, deliberately, because the model above already treats them
   the same.
2. **A change set is isolated.** Changes are invisible to anyone else until
   `Commit`, and a discarded change set leaves nothing behind.
3. **A nested change set is atomic within its parent.** Committing it moves its
   changes to the parent, not to disk.
4. **A prefix scopes a change set.** Keys are relative to it.
5. **Durability is the committed log; the state store seals behind it.** The
   commit of the outermost change set makes a block's state visible and
   hands it to the store; what makes the block survive a crash is its entry in
   the consensus log (consensus.md, "The committed log"), which is on disk
   before the commit returns. The store seals the block's writes later, on
   its own goroutine, and a block's log entry may be deleted only once the
   store has sealed it. Nothing below the outermost commit is a visibility
   point, and nothing but the log is a durability point.
6. **A backend that cannot answer a read must say so, never guess.** Reporting
   not-found for data that exists is a consensus fault, not a cache miss.

### The seal lags the commit

A block's writes reach the store when its outermost change set commits; the
store's *seal* — the barriers that make a block's segments durable and mark
the height — runs behind, at the merge watermark (`MergeLag`, 20 blocks) or
later if the disk is slow. Three consequences the rest of the system relies
on:

- **A restart replays.** On open the store holds every sealed block and
  whatever of the unsealed tail reached disk; the node re-executes from the
  last sealed height through the committed log to the log's head. Execution
  is deterministic, so the state it rebuilds is the state it lost.
- **The gap is bounded by refusal, not by a constant.** How far the seal may
  lag is an operational bound: past it the node refuses user work, because a
  store that cannot keep up with the network is the condition back-pressure
  exists to detect. A slow disk is a growing gap, not a stopped partition.
- **The log's retention is the gap.** The committed log deletes an entry when
  the seal passes its block, so the log is exactly as long as the replay a
  restart would need, plus whatever peers may still ask for
  (consensus.md, "Retention").

Why (2026-09-06, soak 20260906T134054Z, #4259): with the seal on the block
path every commit waited for ~40 fsyncs per node, and with sixteen stores on
one disk the run stalled for four minutes in those barriers while consensus
went on. An append to a log is one sequential write and one sync; that is
what LevelDB's write-ahead log does, and the committed log already exists.

### Windowed stores

Some backends do not answer every read from the whole of history. BlockchainDB
answers a permanent-layer read from a recent window and reports anything older
absent, because probing history on every miss cost 23% of a validator's CPU and
grew without bound with the chain.

This is visible in the abstraction rather than hidden by it, because a reader
that means to look back must say so:

- The **executor** reads recent state and takes an ordinary change set.
- A reader that knowingly reaches into history — the API, a tool walking the
  chain — takes a **deep** change set. Dispatch and healing are not such
  readers: they read the producer's cache (healing.md, "The cache"), and a
  read of history by either is a failure.

A store with no window ignores the distinction: its ordinary reads already see
everything.

### Isolation has a price, and one reader declines it

A windowed store has no versions of its own. It keeps invariant 2 for a
change set by remembering, for every commit made while the change set is
open, what each rewritten key held before — one store read per dynamic
entry per commit, held in memory until the last older reader closes. That is
paid only while a reader is open, so **who holds a reader while blocks commit
decides what every commit costs.**

One reader wants the opposite of isolation: **validation** (CheckTx) judges a
submission against the latest committed state, and a snapshot from when it
began is only staler. It takes an **unisolated** change set — reads see the
store as it stands at each read, the change set pins no version, and a commit
made while it is open takes no pre-images on its account. A read may fall
inside a commit's write-through and see part of it; validation tolerates that
because the block re-executes what it admits. Nothing else takes one: a
reader that must see a consistent state takes an ordinary change set and pays
for it.

A store with no window ignores this too: its readers cost nothing to isolate.

### Duplicates are caught at entry

There is no rule that a hash appears once in the store. What there is, is a
place where each kind of repeat is stopped, and it is always **recent,
mutable state**:

| a repeat of | is stopped by | which is |
|---|---|---|
| a signature | the key entry's spent timestamps | the key page's state |
| a transaction or message | its status, already Delivered | the message's status record |
| a sequenced entry (synthetic, anchor) | staging's delivered index for the stream | the ledger's stream position |
| an anchor signature | the anchor message's status | as above |

Every one of those is a record the executor rewrites, so it lives in the
dynamic layer, whose own short history answers the question. By the time a
hash reaches a chain append or a first record write it has passed those
checks and is new. **The write does not ask again.** A read whose only purpose
is to learn that a key is absent before writing it is a defect: on a windowed
store it is a search of all history to confirm what recent state already
settled.

**A record is written once per thing it records.** The rows above are the
records a repeat consults, and they are why nothing else needs to be written
more than once. A transaction's body is stored once, under the transaction's
hash; a wrapper that carried it — sequenced, synthetic, an anchor copy — is
stored referring to it by hash ([executor.md](executor.md), "The database
write"). An anchor's validator signature set is written once per block, from
what the block's copies brought, not once per copy. What a message produced
is one set, `Transaction(hash).Produced`, written as it is produced; `Cause`
is its inverse and is kept because the API answers from it. A status is
written for a message that has an outcome — it is the dedup record — and for
nothing else.

**Chains are logs.** A chain is an append-only sequence of hashes. Its element
index maps a hash to the position it was **last written** at, live and after
a restore alike, and no reader relies on which occurrence it names: a receipt,
a query by hash, a proof check and the proven set each need *an* index of the
hash, and any serves. Writing the index is therefore a write, not a read
followed by a write.
An index is the hash's only while the position it names holds that hash. A
chain replaced whole — the join's retake of a chain grown wrongly
([executor.md](executor.md), "Sync", "Two mismatches" 2) — rewrites every
position and re-indexes the entries it now holds, and leaves the index of a
hash only the replaced chain held naming a position that holds another, or
none below the head. A deduplicated append that meets such an index reads the
element it names and appends when the position is past the head or holds
another hash (#4444); an element it cannot read refutes nothing.
Repeats do occur, by construction, and are appended: a root chain receives
equal anchors from equal chains (genesis, one transaction creating several
accounts); a signature chain records one cause per signer and every signature
message as it arrived. Every other chain — index chains, the synthetic chains
(one per destination), the anchor sequence, block ledger and BPT chains, account
main and scratch chains, anchor root and BPT chains — receives each hash once
because the writer appends it once, from one place. A duplicate reaching one of
them is the writer's bug, not the chain's to absorb.

**What the code may assume.**

- A writer that knows a key is new writes it without reading. The conflict
  check between concurrent children of a batch compares versions of records in
  memory; the store holds no version, so a first write never reads the store
  to learn one.
- A mutable record is answered by the dynamic layer alone. It is routed there
  without exception, so a miss there is the answer, and the permanent history
  is never searched for it.
- **The protocol never proves from the stored tree.** A chain is a sequence
  of hashes to everything above the merkle library; receipts and receipt lists
  are built from a *segment* — the chain's state before a span and the hashes
  of the span, kept in memory by whoever appended them — and validated by the
  library. The producer cache holds the synthetic chain's segment per block;
  the block holds its root chain segment and a segment for every anchor chain
  it appended to, and the Directory's anchor receipts are built from those. A
  receipt built by reading the stored tree depends on what the store still
  answers, and a windowed store turned that into rejected anchors for every
  run from `20260905T032333Z` to `051008Z`. Where the store keeps the tree's
  own records (mark points, elements, element indexes) is the store's
  business; mark points are in the dynamic layer because queries reach them
  at any age, and a missing one is an error, never an empty state.
- **A record is written when it changes.** An index that already holds what
  a block would add to it is not rewritten to hold it again: an account's
  `Chains` index lists a chain once, when the chain is first appended to,
  and a block that appends to chains the index lists writes no `Chains`
  record. The same holds for any collection a commit "ensures" — the check
  is a read of the record in hand, and the write happens only on a change.
- **The head is Count and Pending; the open mark set is chunked.** A chain's
  head is rewritten on every append, so it holds only what every append
  changes: the count and the pending roots (log₂ n hashes). The hashes of
  the *open* mark set — the entries since the last mark point, which a
  receipt, a state or a range inside that set replays — are kept in
  `Tail(k)` records of at most eight hashes, reused every set and identified
  by the index of their first hash, so an append rewrites one chunk and not
  the set; the mark point that closes the set is assembled from them and
  holds the whole set, as it always has. The tail is mutable state in the
  dynamic layer for the same reason mark points are: the open set of a slow
  chain (an anchor chain at one entry a block, the major-block chain) is
  older than the permanent window, and its readers — the executor's last
  major block, a state or range for the API — must find it at any age. A
  head written before the tail records carries the set itself and is read
  as such until its next append moves the set over.
- **A layer exception is for a permanent shape only.** The dynamic layer is
  read first, so a tombstone there shadows a later write of the same key to
  the permanent layer; a windowed store therefore remembers a deleted
  permanent key as an exception — in memory for the process lifetime and in
  a file read whole at open — and sends its later writes to the dynamic
  layer. A mutable shape needs no such memory: it is routed to the dynamic
  layer with or without a tombstone. Clearing a set is a tombstone on a
  mutable shape, three per delivered transaction, and adds nothing to the
  exception set. The set's size is published
  (`accumulate_bcdb_dyna_exceptions`, `dynaExceptions` in `stats.json`); a
  count that grows with the transaction rate is a misclassified shape.
- A permanent record is answered from the window. The one reader that
  legitimately reaches further — a signature or reference arriving for a
  pending transaction whose body is older than the window — takes a deep
  reader ("Windowed stores"). Dispatch and healing never touch the store for
  an entry or a proof; the cache holds what they need. Nothing else reaches
  into history, and nothing reaches into it to prove an absence.

**How it is tested.** The chain records every append of a hash it already
holds, in test builds, and the executor suites assert that the only ones are
the permitted repeats; a duplicate on any other chain fails the suite. The
adapter counts every shallow miss by record shape and every history walk it
made for one; a test proves a mutable miss never walks, and a soak proves the
permanent shapes' misses are zero once their readers are deep. The replay
tests prove the entry checks themselves.

### Proofs are read, not searched

A Merkle proof runs from an entry to the peaks of its tree. Which sibling it
needs at each level follows from the entry's index, so a proof is arithmetic
over addresses plus one read per address. Nothing about it requires a search,
and nothing it needs has to be recomputed: every sibling is a subtree root
that was calculated once, when the cascade that formed it ran, and every
element is stored individually.

Three positions are involved in a chain receipt, and each must be a read:

1. **Entry hash to entry index.** A stored map, written on every append
   (`ElementIndex`). One read.
2. **Entry index to the positions that anchor it** — which index-chain entry
   covers the entry, and which root index entry covers that. Both are recorded
   when the entry is written (`TransactionChainEntry.ChainIndex` and
   `.AnchorIndex`). Read them. A receipt that searches an index chain for
   something already written down is a defect, however fast the search: the
   linear form of it hung the v3 API past a three-minute client timeout on
   entries with old anchors (#4263).
3. **The sibling hashes themselves.** Stored, addressable, and chosen by
   arithmetic on the index. A chain records each `(left, right)` pair as its
   cascade combines it, keyed by the element's index and the height, so a
   proof reads what was computed once rather than rebuilding the Merkle state
   that held it. Where the cascade never reached a height there is nothing to
   read and nothing to rebuild: adding element *e* carries through one level
   per set bit at the bottom of *e*, so the heights that exist are 1 through
   `trailingOnes(e)`, and a request above that is answered by arithmetic. The
   cost is at most one record per element added: adding N entries performs
   N − popcount(N) combines in total, one per carry, which is N−1 only when N
   is a power of two — the open mark set holds one uncombined hash per set
   bit. Reading is one lookup per level and no state replay, so a proof costs
   about log2(N) reads whatever the mark frequency, which governed only the
   replay that is now gone.

A receipt asked for a *height other than the entry's own anchor* is the one
case position 2 cannot answer from the record, because the root entry wanted
is deliberately not the one the entry was anchored at. That case searches.

The cost this buys is the criterion: **a proof for any entry in a chain of N
costs on the order of log2(N) reads** — 32 for four billion — and no scan of
any length appears in it.

The same principle governs the account state tree: the BPT stores subtree
blocks of `Power` levels, so a proof reads one block per `Power` levels rather
than one record per level, and the blocks it reads already contain every
sibling the upward walk needs.

### Caches

Two caches sit in front of the store, for two different readers, and they are
not the same cache:

| cache | serves | shape | layer behind it |
|---|---|---|---|
| **hash-to-URL mapping** | reads that resolve a hash to the account it belongs to | two-level, cycled: a lookup tries the hot level then the cold one and promotes a hit; when hot fills it becomes cold and a new hot starts | dynamic |
| **synthetic/anchor entries** | dispatch, and healing requests for entries a destination lacks ([healing.md](healing.md), "The cache") | indexed by partition and index and by hash; holds only the entries in play; cleared as the destination delivers | permanent — every entry is persisted through execution |

Hash-to-URL mappings live in the **dynamic** layer: they are read constantly and
churn with the working set. Synthetic and anchor entries are persisted to the
**permanent** layer through execution and never change, so their cache is a hot
front for entries in play and a miss falls through to permanent storage, counted.
Sizing either cache is decided from measurement, not here.

## 2. Specification — how it is implemented

### Interfaces

`pkg/database/keyvalue/store.go`:

```go
type Store interface {
    Get(*record.Key) ([]byte, error)
    Put(*record.Key, []byte) error
    Delete(*record.Key) error
    ForEach(func(*record.Key, []byte) error) error
}
```

`pkg/database/keyvalue/atomic.go`:

```go
type ChangeSet interface {
    Store
    Beginner
    Commit() error
    Discard()
}

type Beginner interface {
    Begin(prefix *database.Key, writable bool) ChangeSet
}

type DeepBeginner interface {   // optional; only a windowed store implements it
    Beginner
    BeginDeep(prefix *database.Key, writable bool) ChangeSet
}

type UnisolatedBeginner interface {   // optional; only a store that isolates by pre-image
    Beginner
    BeginUnisolated(prefix *database.Key, writable bool) ChangeSet
}
```

`keyvalue.Deep(b Beginner) Beginner` returns a beginner whose change sets read
the whole history if the store distinguishes, and the store unchanged if it does
not — so a caller that needs history says so once, at construction, and every
batch it begins reaches history without any call site changing.
`internal/database.Database.Deep()` is the same idea one layer up.
`keyvalue.Unisolated` and `internal/database.Database.Unisolated()` are the
same shape for the reader that declines isolation; the executor's `Validate`
begins its batch through it. The store publishes what isolation cost —
`preImageReads` in `stats.json`, the reads commits made for pinned readers —
so a reader that should have been unisolated shows in the numbers. Two gauges
say who is holding readers now: `accumulate_bcdb_staged_commits` (commit
overlays kept for readers begun before them) and
`accumulate_bcdb_oldest_view_age_seconds`, and a commit made while the oldest
reader is older than `ViewWarnAfter` logs the function that began it. All
three are computed when a commit or a reader's release happens, not at
scrape: a store that has stopped committing reports the age its last commit
saw. A reader held across many commits is not a store problem; it names a
caller that opened a change set and lost it (#4279).

### Adapting to the record model

`keyvalue.RecordStore` adapts a `Store` to `database.Store`, which is what the
record model reads and writes through. It is where invariant 1 is enforced:

```go
if len(b) == 0 {
    return (*database.NotFoundError)(key)
}
```

### Backends

`pkg/database/keyvalue/`:

| Backend | Notes |
|---|---|
| `leveldb` | Default (`run.DefaultStorageType`) |
| `badger` | Versions 1–4, selected by configuration |
| `bolt` | |
| `memory` | Tests, simulation, genesis construction |
| `bcdb` | BlockchainDB: two layers, permanent and dynamic; **windowed** |
| `block` | Experimental block-oriented store |
| `overlay` | Composes two stores, reads falling through |
| `remote` | Serves a store over a connection |

### Conformance

`pkg/database/keyvalue/kvtest` is the contract. A backend is correct when it
passes:

| Test | Establishes |
|---|---|
| `TestDatabase` | Writes are readable, survive reopening, and `ForEach` yields exactly what was written |
| `TestIsolation` | Uncommitted changes are invisible to another change set |
| `TestSubBatch` | A nested change set commits into its parent, not to disk |
| `TestPrefix` | Keys are scoped by the prefix a change set was begun with |
| `TestDelete` | A deleted key reads as not-found |

A backend that does not run `kvtest` is unspecified, whatever else it is tested
for.

### Choosing a backend

`cmd/accumulated/run/storage.go`:

- Configuration names the backend; `DefaultStorageType` is LevelDB for
  hand-written configurations. Init flows record the choice explicitly.
- `detectStorageDir` identifies an existing database by its on-disk markers —
  BlockchainDB by `perm/segments.json`, Badger by `KEYREGISTRY` or `*.vlog`,
  LevelDB by `CURRENT`.
- `checkStorageDir` refuses to open a database with the wrong backend. Opening
  one with another backend would at best fail obscurely and at worst come up
  empty, so the mismatch is a fatal error naming both the found and configured
  types.

### Record keys

`pkg/types/record`. A `Key` is a sequence of components — strings, URLs,
hashes, integers. `Key.Hash()` reduces it to 32 bytes, which is what a store is
keyed by. A store therefore cannot reconstruct the path from the key, which is
why `ForEach` yields `record.KeyFromHash`.

---

Where the implementation departs from this specification, see
[DIFFERENCES.md](DIFFERENCES.md).
