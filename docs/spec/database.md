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
5. **Durability is the commit of the outermost change set.** Nothing below that
   is a durability point.
6. **A backend that cannot answer a read must say so, never guess.** Reporting
   not-found for data that exists is a consensus fault, not a cache miss.

### Windowed stores

Some backends do not answer every read from the whole of history. BlockchainDB
answers a permanent-layer read from a recent window and reports anything older
absent, because probing history on every miss cost 23% of a validator's CPU and
grew without bound with the chain.

This is visible in the abstraction rather than hidden by it, because a reader
that means to look back must say so:

- The **executor** reads recent state and takes an ordinary change set.
- A reader that knowingly reaches into history — the API, healing, a tool
  walking the chain — takes a **deep** change set.

A store with no window ignores the distinction: its ordinary reads already see
everything.

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
```

`keyvalue.Deep(b Beginner) Beginner` returns a beginner whose change sets read
the whole history if the store distinguishes, and the store unchanged if it does
not — so a caller that needs history says so once, at construction, and every
batch it begins reaches history without any call site changing.
`internal/database.Database.Deep()` is the same idea one layer up.

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
