# Accumulate Database Implementation Guide
## Complete Reference for MCP Server Design

### 1. DATABASE ARCHITECTURE OVERVIEW

#### 1.1 High-Level Architecture
Accumulate uses a hierarchical, record-based data model with multiple storage backends:

**Key Components:**
- **Batch**: Transaction-like interface for atomic reads/writes
- **BPT (Binary Patricia Tree)**: Merkle tree indexing all accounts by content hash
- **Chains**: Merkle trees for transaction sequences (Main, Signature, Root, Anchor, etc.)
- **Key-Value Store**: Underlying persistence layer (BadgerDB, LevelDB, or in-memory)
- **Record Model**: Hierarchical data structure with lazy loading and caching

#### 1.2 Storage Backends
Located in: `/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/pkg/database/keyvalue/`

- **BadgerDB** (`badger/`): Default, high-performance embedded key-value store
  - Multiple versions supported (v1-v4)
  - File path: User configurable via config
  - Features: MVCC, transactions, compression
  
- **LevelDB** (`leveldb/`): Alternative embedded key-value store
  - Simpler, more stable than BadgerDB
  - File path: User configurable via config
  
- **Memory** (`memory/`): In-memory storage for testing
  - Used for development and testing
  
- **Block** (`block/`): Custom block-based storage format
  - Used for read-only database access
  - Compressed block format for efficiency

#### 1.3 Key-Value Interface
```go
type Store interface {
    Get(*record.Key) ([]byte, error)
    Put(*record.Key, []byte) error
    Delete(*record.Key) error
    ForEach(func(*record.Key, []byte) error) error
}
```

---

### 2. KEY-VALUE STRUCTURE AND PREFIXES

#### 2.1 Key Design
Located in: `/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/pkg/types/record/`

**Key Structure:**
- Hierarchical composite keys: `Key{part1, part2, part3, ...}`
- Each key part can be: string, URL, hash, int64, uint64, bytes, time.Time
- Keys are converted to 32-byte SHA256 hashes for storage: `KeyHash`

**Examples:**
```
Key{"Account", "alice.acme"}                    // Account record
Key{"Account", "alice.acme", "Main"}           // Account main state
Key{"Account", "alice.acme", "MainChain"}      // Account main chain
Key{"Account", "alice.acme", "SignatureChain"} // Signature chain
Key{"Account", "alice.acme", "Transaction", <txn-hash>}  // Transaction
Key{"Account", "alice.acme", "Data", "Entry"}  // Data entries
Key{"Message", <msg-hash>}                      // Message record
Key{"Transaction", <txn-hash>}                  // Transaction details
Key{"SystemData", "DN"}                         // System data
```

#### 2.2 Key Hashing Process
```go
type KeyHash [32]byte

// Keys are hashed by iteratively applying SHA256:
// hash = SHA256(previous_hash || key_part_bytes)

func (k KeyHash) Append(key ...interface{}) KeyHash {
    for _, part := range key {
        bytes := convertKeyPart(part)
        combined := append(k[:], bytes...)
        k = sha256.Sum256(combined)
    }
    return k
}
```

#### 2.3 Database Prefixes
The database stores all records with their key hash as the storage key. The database itself uses different types of records:

- **Account records**: Key["Account", url]
- **Message records**: Key["Message", [32]byte hash]
- **Transaction records**: Key["Transaction", [32]byte hash]
- **BPT entries**: Stored as Key["BPT"] with entries indexed by account key hash
- **Chain elements**: Key["Account", url, "MainChain", "Element", index]
- **Chain marks**: Key["Account", url, "MainChain", "States", index]

---

### 3. DATABASE BATCH OPERATIONS

Located in: `/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/internal/database/`

#### 3.1 Batch Interface
```go
type Batch struct {
    id          string
    writable    bool
    parent      *Batch
    store       record.Store
    
    // Cached entities
    bpt         *bpt.BPT
    account     map[accountMapKey]*Account
    message     map[messageMapKey]*Message
    transaction map[transactionMapKey]*Transaction
    systemData  map[systemDataMapKey]*SystemData
}

// Methods
func (db *Database) Begin(writable bool) *Batch
func (batch *Batch) Commit() error
func (batch *Batch) Discard() error
func (batch *Batch) Begin(writable bool) *Batch // Nested batch
```

#### 3.2 Batch Lifecycle
```go
// Read-only batch
batch := db.Begin(false)
defer batch.Discard()
account, err := batch.Account(url).Main().Get()

// Writable batch with commit
batch := db.Begin(true)
defer batch.Discard()
err := batch.Account(url).Main().Put(account)
if err == nil {
    err = batch.Commit()
}

// Nested batch
parentBatch := db.Begin(true)
childBatch := parentBatch.Begin(true)
// Changes propagate to parent on commit
childBatch.Commit() // Commits to parent
parentBatch.Commit() // Commits to database
```

#### 3.3 Database Views and Updates
```go
// Read-only view
err := db.View(func(batch *Batch) error {
    account, err := batch.Account(url).Main().Get()
    return err
})

// Writable update
err := db.Update(func(batch *Batch) error {
    return batch.Account(url).Main().Put(account)
})
```

---

### 4. ACCOUNT STORAGE FORMAT

Located in: `/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/internal/database/`

#### 4.1 Account Structure
```yaml
# From model.yml
- name: Account
  type: entity
  parameters:
    - name: Url
      type: url
  attributes:
    # Account state record
    - name: Main
      type: state
      dataType: protocol.Account
    
    # Pending transactions
    - name: Pending
      type: state
      dataType: txid
      collection: set
    
    # Chains
    - name: MainChain
      type: other
      dataType: Chain2
      hasChains: true
    
    - name: SignatureChain
      type: other
      dataType: Chain2
      hasChains: true
    
    - name: ScratchChain
      type: other
      dataType: Chain2
      hasChains: true
    
    - name: RootChain
      type: other
      dataType: Chain2
      hasChains: true
```

#### 4.2 Account Access Patterns
```go
// Get account
account := batch.Account(url)

// Get/set main account state
var acc protocol.Account
err := batch.Account(url).Main().GetAs(&acc)
err = batch.Account(url).Main().Put(acc)

// Access chains
mainChain, err := batch.Account(url).MainChain().Get()
sigChain, err := batch.Account(url).SignatureChain().Get()

// Access transactions within account
tx, err := batch.Account(url).Transaction(txnHash).Main().Get()

// Access pending transactions
pending, err := batch.Account(url).Pending().Get()

// Access data
data, err := batch.Account(url).Data().Entry().Get()
```

#### 4.3 Account Storage Keys
```
Account/{url}                       // Account key
Account/{url}/Main                  // Account main state
Account/{url}/MainChain             // Merkle chain for transactions
Account/{url}/SignatureChain        // Merkle chain for signatures
Account/{url}/ScratchChain          // Scratch/temporary chain
Account/{url}/RootChain             // Anchor/root chain
Account/{url}/BptChain              // BPT validation chain
Account/{url}/AnchorSequenceChain   // Anchor sequence
Account/{url}/MajorBlockChain       // Major block chain
Account/{url}/SyntheticSequenceChain/{partition}  // Synthetic tx chain
Account/{url}/AnchorChain({partition})/Root      // Anchor root chain
Account/{url}/AnchorChain({partition})/BPT       // Anchor BPT chain
Account/{url}/Transaction/{hash}    // Transaction record
Account/{url}/Chains                // Chains metadata index
Account/{url}/Data/Entry            // Data entry index
Account/{url}/Data/Transaction      // Data transaction mapping
```

---

### 5. CHAIN STORAGE FORMAT

Located in: `/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/pkg/database/merkle/`

#### 5.1 Chain Types
```go
type ChainType uint64
const (
    ChainTypeTransaction = 0  // Regular transaction chains
    ChainTypeAnchor      = 1  // Anchor/root chains
    ChainTypeIndex       = 2  // Index chains (for fast lookup)
)
```

#### 5.2 Chain Structure
Each chain maintains:
- **Head State**: Current merkle state (count, hash, pending roots)
- **Elements**: Individual entries in the chain (usually 32-byte hashes)
- **Marks**: Periodic snapshots at power-of-2 boundaries
- **Index Chain**: Fast lookup index for finding entries by hash

```go
type Chain struct {
    key          *record.Key
    store        record.Store
    typ          ChainType
    name         string
    markPower    int64  // Usually 8 (256 element marks)
    markFreq     int64  // 2^markPower
    markMask     int64  // markFreq - 1
}

type State struct {
    Count      int64    // Number of entries
    Anchor     []byte   // Current merkle root
    HashList   [][]byte // Pending hashes for next anchor
    Pending    [][]byte // Pending roots
}
```

#### 5.3 Chain Storage Keys
```
Account/{url}/MainChain/Head              // Current chain state
Account/{url}/MainChain/Element/{index}   // Entry at index
Account/{url}/MainChain/States/{index}    // State at mark point
Account/{url}/MainChain/Index/{hash}      // Entry index lookup
Account/{url}/MainChain/Index/Element/{hash}  // For the index chain

// Index chain (for main chain lookups)
Account/{url}/MainChain/Index/Head
Account/{url}/MainChain/Index/Element/{index}
Account/{url}/MainChain/Index/States/{index}
Account/{url}/MainChain/Index/Index/{hash}
```

#### 5.4 Chain Access Examples
```go
// Get chain
chain, err := batch.Account(url).MainChain().Get()

// Get current state
state := chain.CurrentState()
height := chain.Height()

// Get entry at height
entry, err := chain.Entry(height)

// Get state at height
state, err := chain.State(height)

// Get anchor (merkle root)
anchor := chain.Anchor()
anchorAt, err := chain.AnchorAt(height)

// Get receipt from one index to another
receipt, err := chain.Receipt(from, to)

// Add entry to chain
err := chain.AddEntry(entryHash, unique)

// Get all entries in range
entries, err := chain.Entries(start, end)
```

---

### 6. BPT (BINARY PATRICIA TREE) STRUCTURE

Located in: `/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/pkg/database/bpt/`

#### 6.1 BPT Purpose
The BPT is a Merkle tree that indexes all accounts by their key hash. It enables:
- Fast verification of account state (O(log n))
- Generation of merkle proofs for any account
- Efficient iteration over all accounts
- State root hash for consensus

#### 6.2 BPT Node Structure
```go
// Internal node types
type node interface {
    Type() nodeType
    IsDirty() bool
    getHash() ([32]byte, bool)
    writeTo(io.Writer) error
    readFrom(*bytes.Buffer, marshalOpts) error
}

type emptyNode struct{
    parent *branch
}

type leaf struct {
    parent *branch
    Key    *record.Key  // Account key
    Value  []byte       // 32-byte account hash
}

type branch struct {
    parent *branch
    Height uint64       // Depth in tree (0-254)
    Key    [32]byte     // Node key (deterministic based on height)
    Left   node
    Right  node
    status branchStatus // clean, unhashed, uncommitted
}
```

#### 6.3 BPT Operations
```go
// Insert/update account
bpt := batch.BPT()
err := bpt.Insert(accountKey, accountHash)

// Get account hash
hash, err := bpt.Get(accountKey)

// Get root hash
rootHash, err := bpt.GetRootHash()

// Iterate all entries
iterator := bpt.Iterate(1000)
for iterator.Next() {
    for _, entry := range iterator.Value() {
        key := entry.Key      // record.Key
        hash := entry.Value   // [32]byte
    }
}

// Get merkle receipt for account
receipt, err := bpt.GetReceipt(accountKey)
```

#### 6.4 BPT Storage Format
```
BPT/Root          // Root node
BPT/{nodeKey}     // Internal nodes
BPT/AccountUrl/{hash}  // URL mapping for long URLs
BPT/AccountUrl/{hash}/Url  // Actual URL value
```

---

### 7. MESSAGE AND TRANSACTION STORAGE

#### 7.1 Message Structure
```yaml
- name: Message
  type: entity
  parameters:
    - name: Hash
      type: hash
  attributes:
    - name: Main
      type: state
      dataType: messaging.Message
      union: true
    - name: Cause
      type: index
      dataType: txid
      collection: set
    - name: Produced
      type: index
      dataType: txid
      collection: set
    - name: Signers
      type: index
      dataType: url
      collection: set
```

#### 7.2 Transaction Structure
```yaml
- name: Transaction
  type: entity
  parameters:
    - name: Hash
      type: hash
  attributes:
    - name: Main
      type: state
      dataType: SigOrTxn
    - name: Status
      type: state
      dataType: protocol.TransactionStatus
    - name: Produced
      type: state
      dataType: txid
      collection: set
    - name: Signatures
      type: state
      dataType: sigSetData
      parameters:
        - name: Signer
          type: url
      collection: set
```

#### 7.3 Access Patterns
```go
// Get message by hash
msg, err := batch.Message(msgHash).Main().Get()

// Get message status
status, err := batch.Message(msgHash).Status().Get()

// Get transaction by hash
txn, err := batch.Account(url).Transaction(txnHash).Main().Get()

// Get transaction status
status, err := batch.Account(url).Transaction(txnHash).Status().Get()

// Get signatures for transaction
sigs, err := batch.Account(url).Transaction(txnHash).Signatures(signerUrl)

// Add signature
batch.Account(url).Transaction(txnHash).ensureSigner(signer)
```

---

### 8. SNAPSHOT FORMATS

Located in: `/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/pkg/database/snapshot/`

#### 8.1 Snapshot File Format
Snapshots use a segmented file format with multiple section types:

```
[Header Section]
  - Metadata about the snapshot
  - Version information
  - Network identifier

[Records Section]
  - Key-value pairs of database records
  - Serialized as length-prefixed entries

[Record Index Section]
  - Fast lookup index
  - 32-byte key hash + 32-byte value hash pairs
  - Sorted by key hash for binary search

[BPT Section]
  - BPT entries
  - Key hash + value hash for account entries

[Consensus Section]
  - Protocol-specific consensus parameters
  - Implementation-specific format
```

#### 8.2 Snapshot Collection
```go
// Collect snapshot
opts := &CollectOptions{
    BuildIndex: true,
    Predicate: func(record database.Record) (bool, error) {
        // Filter records if needed
        return true, nil
    },
}

w, err := batch.Collect(file, partition, opts)

// The snapshot writer provides progress information
err = w.Close()
```

#### 8.3 Snapshot Restoration
```go
// Restore from snapshot
err := batch.Restore(snapshotFile, options)
```

---

### 9. OPENING AND READING DATABASE FILES

#### 9.1 Database Opening
```go
// From config
db, err := database.Open(cfg, logger)
defer db.Close()

// From filepath
db, err := database.OpenBadger("/path/to/db", logger)
db, err := database.OpenLevelDB("/path/to/db", logger)

// In-memory
db := database.OpenInMemory(logger)
```

#### 9.2 Getting Underlying Key-Value Store
```go
kvStore, err := db.Store()

// Raw key-value access
value, err := kvStore.Get(record.NewKey(...))
```

#### 9.3 Reading Snapshots
```go
// Open snapshot file
file, err := os.Open("snapshot.acc")
defer file.Close()

// Get version
version, err := snapshot.GetVersion(file)

// Read records with index
store := snapshot.NewStore(file, useIndex)
value, err := store.GetValue(key, valueRecord)

// Iterate snapshot records
reader := snapshot.NewReader(file)
for reader.Next() {
    key, value, err := reader.Entry()
}
```

---

### 10. HISTORICAL DATABASE ACCESS

#### 10.1 Block-Based Read-Only Access
Located in: `/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/pkg/database/keyvalue/block/`

```go
// Open block-based database (compressed archive format)
db, err := block.Open("/path/to/blocks")
defer db.Close()

// Access as normal key-value store
batch := db.Begin(nil, false)
value, err := batch.Get(key)
```

#### 10.2 Time-Based Queries
Accumulate doesn't have direct time-based queries, but you can:

1. Query by block height via block ledger
2. Query chain at specific index
3. Get state at specific anchor

```go
// Get account state at chain index
chain, err := batch.Account(url).MainChain().Get()
state, err := chain.State(index)

// Get entry at specific height
entry, err := chain.Entry(height)

// Get receipt proving state at height
receipt, err := chain.Receipt(from, to)
```

#### 10.3 Batch Snapshots
```go
// Create snapshot for later analysis
err := db.Update(func(batch *Batch) error {
    // Perform operations
    return batch.Commit()
})

// Collect snapshot
batch := db.Begin(false)
defer batch.Discard()
w, err := batch.Collect(file, partition, nil)
```

---

### 11. MAIN INTERFACES FOR DATABASE ACCESS

#### 11.1 Record Interface
```go
type Record interface {
    Key() *Key
    Resolve(key *Key) (Record, *Key, error)
    IsDirty() bool
    Commit() error
    Walk(opts WalkOptions, fn WalkFunc) error
}
```

#### 11.2 Value Interface
```go
type Value interface {
    Record
    GetValue() (value encoding.BinaryValue, version int, err error)
    LoadValue(value Value, put bool) error
    LoadBytes(data []byte, put bool) error
}
```

#### 11.3 Store Interface
```go
type Store interface {
    GetValue(key *record.Key, value Value) error
    PutValue(key *record.Key, value Value) error
}
```

#### 11.4 Beginner Interface
```go
type Beginner interface {
    Updater
    Begin(bool) *Batch
    SetObserver(Observer)
}

type Updater interface {
    Viewer
    Update(func(batch *Batch) error) error
}

type Viewer interface {
    View(func(batch *Batch) error) error
}
```

---

### 12. CODE EXAMPLES FOR DATABASE QUERIES

#### 12.1 Basic Account Query
```go
db := database.OpenInMemory(logger)
defer db.Close()

// Read account
var account protocol.Account
err := db.View(func(batch *Batch) error {
    return batch.Account(url).Main().GetAs(&account)
})
if err != nil {
    log.Fatal(err)
}
```

#### 12.2 Transaction Query
```go
// Get transaction from account's main chain
var txn *messaging.TransactionMessage
err := db.View(func(batch *Batch) error {
    // Get the transaction entry from main chain
    chain, err := batch.Account(url).MainChain().Get()
    if err != nil {
        return err
    }
    
    // Get entry hash at index
    entry, err := chain.Entry(index)
    if err != nil {
        return err
    }
    
    // Get the message
    return batch.Message(*(*[32]byte)(entry)).Main().GetAs(&txn)
})
```

#### 12.3 Iterating Accounts
```go
err := db.View(func(batch *Batch) error {
    return batch.ForEachAccount(func(account *Account, hash [32]byte) error {
        url := account.Url()
        fmt.Printf("Account: %s, Hash: %X\n", url, hash)
        return nil
    })
})
```

#### 12.4 Getting BPT Root
```go
var rootHash [32]byte
err := db.View(func(batch *Batch) error {
    var err error
    rootHash, err = batch.GetBptRootHash()
    return err
})
```

#### 12.5 Signature Chain Query
```go
// Get all signatures for a transaction
err := db.View(func(batch *Batch) error {
    chain, err := batch.Account(url).SignatureChain().Get()
    if err != nil {
        return err
    }
    
    // Iterate all entries
    entries, err := chain.Entries(0, chain.Height())
    if err != nil {
        return err
    }
    
    for i, entry := range entries {
        fmt.Printf("Signature %d: %X\n", i, entry)
    }
    
    return nil
})
```

#### 12.6 Creating Merkle Receipts
```go
// Create receipt proving inclusion
err := db.View(func(batch *Batch) error {
    chain, err := batch.Account(url).MainChain().Get()
    if err != nil {
        return err
    }
    
    receipt, err := chain.Receipt(fromIndex, toIndex)
    if err != nil {
        return err
    }
    
    fmt.Printf("Receipt: %v\n", receipt)
    return nil
})
```

---

### 13. FILE LOCATIONS FOR DATABASE IMPLEMENTATION

#### 13.1 Public Database Package
```
pkg/database/
├── database.go                 # Database main interface
├── not_found.go               # NotFound error handling
├── README.md                  # Architecture documentation
├── bpt/                       # Binary Patricia Tree
│   ├── bpt.go                # Main BPT implementation
│   ├── node.go               # Node types (leaf, branch, empty)
│   ├── iterate.go            # Iterator for BPT traversal
│   ├── marshal.go            # Serialization
│   └── model.yml             # BPT model definition
├── keyvalue/                 # Key-value store backends
│   ├── store.go              # Store interface
│   ├── badger/               # BadgerDB implementation
│   ├── leveldb/              # LevelDB implementation
│   ├── memory/               # In-memory implementation
│   ├── block/                # Block-based read-only
│   ├── overlay/              # Overlay for nested batches
│   └── remote/               # Remote RPC access
├── merkle/                   # Merkle chain implementation
│   ├── chain.go              # Chain management
│   ├── hash.go               # Hash operations
│   ├── receipt.go            # Merkle receipts
│   └── model.yml             # Merkle model
├── values/                   # Value types and collections
│   ├── value.go              # Base value implementation
│   ├── list.go               # List implementation
│   ├── set.go                # Set implementation
│   └── store.go              # Store wrapper
├── snapshot/                 # Snapshot format
│   ├── format.go             # Snapshot file format
│   ├── collect.go            # Collection logic
│   ├── store.go              # Snapshot store access
│   └── types.yml             # Snapshot types
└── indexing/                 # Indexing support
    ├── search.go             # Search interface
    ├── log.go                # Log implementation
    └── model.yml             # Indexing model
```

#### 13.2 Internal Database Package
```
internal/database/
├── database.go               # Database initialization
├── batch.go                  # Batch implementation
├── account.go                # Account handling
├── account_chains.go         # Chain management
├── chain.go                  # Chain wrapper
├── transaction.go            # Transaction handling
├── message.go                # Message handling
├── model.yml                 # Data model definition
├── model_gen.go              # Generated model code
├── types.yml                 # Type definitions
├── types_gen.go              # Generated types
├── bpt.go                    # BPT integration
├── bpt_account.go            # Account-BPT integration
├── hash.go                   # Hash computation
├── snapshot.go               # Snapshot collection
├── snapshot/                 # Snapshot utilities
│   ├── collect.go
│   ├── restore.go
│   ├── merkle_snapshot.go
│   └── records.go
├── indexing/                 # Index support
│   ├── account.go
│   ├── chain.go
│   ├── transaction.go
│   └── receipts.go
└── smt/                      # Sparse Merkle Tree
    └── storage/              # SMT storage backend
```

#### 13.3 Type Definition Files
```
pkg/types/record/
├── key.go                    # Key type definition
├── key_hash.go               # KeyHash implementation
├── key_part.go               # Key part handling

internal/database/record/
└── types.go                  # Record type aliases
```

---

### 14. DATABASE DESIGN PATTERNS FOR MCP TOOLS

#### 14.1 Read-Only Query Tool
```go
func (mcp *MCPServer) QueryDatabase(params QueryParams) (Result, error) {
    db := mcp.database
    
    var result Result
    err := db.View(func(batch *Batch) error {
        // Perform queries
        account, err := batch.Account(url).Main().Get()
        if err != nil {
            return err
        }
        result.Account = account
        return nil
    })
    
    return result, err
}
```

#### 14.2 Direct Key-Value Access
```go
func (mcp *MCPServer) GetRawValue(key string) ([]byte, error) {
    db := mcp.database
    kvStore, err := db.Store()
    if err != nil {
        return nil, err
    }
    
    recordKey := record.NewKey(...) // parse key string
    return kvStore.Get(recordKey)
}
```

#### 14.3 Iterating Over All Accounts
```go
func (mcp *MCPServer) ListAccounts() ([]*AccountInfo, error) {
    var accounts []*AccountInfo
    
    err := mcp.database.View(func(batch *Batch) error {
        return batch.ForEachAccount(func(acc *Account, hash [32]byte) error {
            main, err := acc.Main().Get()
            if err != nil {
                return err
            }
            
            accounts = append(accounts, &AccountInfo{
                URL: main.GetUrl(),
                Hash: hash,
            })
            return nil
        })
    })
    
    return accounts, err
}
```

#### 14.4 Chain Analysis
```go
func (mcp *MCPServer) AnalyzeChain(url *url.URL, chainName string) (ChainStats, error) {
    var stats ChainStats
    
    err := mcp.database.View(func(batch *Batch) error {
        chain, err := batch.Account(url).GetChainByName(chainName)
        if err != nil {
            return err
        }
        
        stats.Height = chain.Height()
        stats.Anchor = chain.Anchor()
        
        entries, err := chain.Entries(0, chain.Height())
        if err != nil {
            return err
        }
        stats.EntryCount = len(entries)
        
        return nil
    })
    
    return stats, err
}
```

---

### 15. KEY TAKEAWAYS FOR MCP IMPLEMENTATION

1. **Batch-Based Access**: All database operations go through batches (read-only or writable)
2. **Key Hashing**: Keys are SHA256 hashed for storage, enabling content-addressed lookups
3. **Hierarchical Records**: The model is hierarchical with lazy loading and caching
4. **Multiple Storage Backends**: Support BadgerDB, LevelDB, or in-memory for flexibility
5. **Merkle Verification**: BPT and Chains provide cryptographic proofs
6. **No Time-Based Queries**: Use chain indices instead of timestamps
7. **Snapshot Format**: Use segmented format for export/import
8. **Observer Pattern**: Database observer tracks account changes for hash calculation
9. **Iterators**: Use BPT and Chain iterators for bulk operations
10. **Transaction Atomicity**: Batches provide ACID-like semantics

