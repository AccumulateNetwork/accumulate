# Accumulate Database - Key Resource Files

## Critical Files for MCP Implementation

### Public API
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/pkg/database/database.go** - Main database interface
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/pkg/database/keyvalue/store.go** - Key-value interface
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/pkg/types/record/key.go** - Key definition
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/pkg/types/record/key_hash.go** - Key hashing

### Batch & Database Operations
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/internal/database/database.go** - Database init
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/internal/database/batch.go** - Batch implementation
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/internal/database/model.yml** - Data model definition
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/internal/database/model_gen.go** - Generated model code

### Account & Chain Management
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/internal/database/account.go** - Account operations
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/internal/database/account_chains.go** - Chain access
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/internal/database/chain.go** - Chain wrapper
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/internal/database/transaction.go** - Transaction handling

### BPT Implementation
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/pkg/database/bpt/bpt.go** - BPT core
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/pkg/database/bpt/node.go** - BPT nodes
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/pkg/database/bpt/iterate.go** - BPT iteration
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/pkg/database/bpt/model.yml** - BPT model
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/internal/database/bpt.go** - BPT integration

### Merkle Chains
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/pkg/database/merkle/chain.go** - Chain management
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/pkg/database/merkle/hash.go** - Hash operations
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/pkg/database/merkle/receipt.go** - Merkle receipts
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/pkg/database/merkle/model.yml** - Chain model

### Key-Value Backends
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/pkg/database/keyvalue/badger/core.go** - BadgerDB wrapper
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/pkg/database/keyvalue/leveldb/database.go** - LevelDB wrapper
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/pkg/database/keyvalue/memory/database.go** - Memory store
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/pkg/database/keyvalue/block/database.go** - Block store

### Snapshots
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/pkg/database/snapshot/format.go** - Snapshot format
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/pkg/database/snapshot/store.go** - Snapshot store
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/internal/database/snapshot.go** - Snapshot collection
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/pkg/database/snapshot/types.yml** - Snapshot types

### Values & Collections
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/pkg/database/values/value.go** - Value implementation
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/pkg/database/values/list.go** - List implementation
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/pkg/database/values/set.go** - Set implementation
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/pkg/database/values/store.go** - Store wrapper

### Documentation
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/pkg/database/README.md** - Architecture docs
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/internal/database/smt/README.md** - SMT explanation

### Testing Examples
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/internal/database/batch_test.go** - Batch usage examples
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/internal/database/state_test.go** - State access examples
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/internal/database/snapshot_test.go** - Snapshot examples

## Configuration Files

### Build Configuration
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/internal/database/types.yml** - Type generation
- **/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/internal/database/model.yml** - Model generation

## Key Data Structures Summary

### Batch
```
Batch {
    id: string
    writable: bool
    parent: *Batch (for nested transactions)
    store: record.Store
    
    // Cached entities
    bpt: *bpt.BPT
    account: map[accountMapKey]*Account
    message: map[messageMapKey]*Message
    transaction: map[transactionMapKey]*Transaction
    systemData: map[systemDataMapKey]*SystemData
}
```

### Key Types
```
Key = []any                    // Hierarchical composite key
KeyHash = [32]byte            // SHA256 hash of key
```

### Record Key Examples
- Account: Key{"Account", "alice.acme"}
- Chain: Key{"Account", "alice.acme", "MainChain"}
- Message: Key{"Message", [32]byte}
- Transaction: Key{"Transaction", [32]byte}

### Chain State
```
State {
    Count: int64              // Number of entries
    Anchor: []byte            // Current merkle root
    HashList: [][]byte        // Pending hashes
    Pending: [][]byte         // Pending roots
}
```

## Performance Characteristics

### Storage Complexity
- Key lookup: O(1) average, O(32) bytes of hash
- Account lookup via BPT: O(log n)
- Chain entry lookup: O(log n) via index chain
- Chain marks: O(1) on power-of-2 boundaries

### Batch Operations
- View: Read-only, no lock contention
- Update: Writable with single commit
- Nested: Child batches accumulate changes until parent commit

## Integration Points for MCP

1. **Database Initialization**: Use database.Open()
2. **Account Queries**: Use batch.Account(url)
3. **Chain Traversal**: Use batch.Account(url).MainChain()
4. **BPT Access**: Use batch.BPT()
5. **Iteration**: Use batch.ForEachAccount() or bpt.Iterate()
6. **Snapshots**: Use batch.Collect()
7. **Raw KV**: Use db.Store()
