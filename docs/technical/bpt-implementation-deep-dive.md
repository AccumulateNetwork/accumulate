# BPT (Binary Patricia Tree) Implementation Deep Dive

## Table of Contents
1. [Executive Summary](#executive-summary)
2. [Core BPT Architecture](#core-bpt-architecture)
3. [Account Representation in BPT](#account-representation-in-bpt)
4. [BPT Operations](#bpt-operations)
5. [Integration with Database Layer](#integration-with-database-layer)
6. [Security Model](#security-model)
7. [Performance Optimizations](#performance-optimizations)
8. [Snapshot and Restoration](#snapshot-and-restoration)
9. [Key Design Patterns](#key-design-patterns)
10. [Critical Implementation Details](#critical-implementation-details)

## Executive Summary

The BPT (Binary Patricia Tree) is Accumulate's core data structure for maintaining cryptographic proof of all account states. It provides:
- **Complete state validation** through a single 32-byte root hash
- **Efficient Merkle proof generation** for individual account verification
- **Tamper-proof account storage** with comprehensive state hashing
- **Optimized performance** through lazy loading and deferred updates

### Key Insight
The BPT is not just a simple key-value store - it's a sophisticated Merkle tree that captures the complete state of every account including balances, transaction history, pending operations, and sub-accounts, all proven by a single root hash.

## Core BPT Architecture

### Node Structure

The BPT uses three types of nodes defined in `/pkg/database/bpt/node.go`:

#### 1. Empty Node
```go
type emptyNode struct {
    parent *branch
}
```
- Represents absence of data at a tree position
- Always returns zero hash
- Replaced when data is inserted

#### 2. Leaf Node
```go
type leaf struct {
    parent *branch
    Key    *record.Key  // Account identifier
    Value  []byte       // 32-byte hash of account state
}
```
- Contains actual account data
- Key identifies the account
- Value is the Merkle hash of complete account state

#### 3. Branch Node
```go
type branch struct {
    bpt    *BPT
    parent *branch
    status branchStatus
    Height uint64       // Position in tree (0 = root)
    Key    [32]byte    // Node identifier
    Hash   [32]byte    // SHA256 of children
    Left   node
    Right  node
}
```
- Internal navigation node
- Height determines bit position for routing
- Hash combines children's hashes

### Binary Navigation System

The tree uses an elegant bit-manipulation system for navigation (`node.go:132-148`):

```go
func (e *branch) getAt(key [32]byte) (*node, error) {
    BIdx := byte(e.Height >> 3)    // Calculate byte index (height/8)
    bitIdx := e.Height & 7          // Bit index within byte (height%8)
    bit := byte(0x80) >> bitIdx    // Create mask for specific bit
    
    n := &e.Left                   // Default to left
    if bit&key[BIdx] != 0 {        // Check if bit is set
        n = &e.Right               // If set, go right
    }
    
    return n, nil
}
```

**Key Properties:**
- Each level examines one bit of the key hash
- Left path for 0, right path for 1
- Maximum depth: 254 levels (prevents infinite recursion)
- Deterministic routing based on key hash

### State Data Structure

The BPT maintains its configuration in `stateData` (`types_gen.go:183`):

```go
type stateData struct {
    RootHash  [32]byte    // Current root hash
    MaxHeight uint64      // Deepest branch level
    Parameters            // Configuration options
}
```

## Account Representation in BPT

### Critical Understanding: BPT is a Dual-Key System

The BPT stores two pieces of information that BOTH function as database keys:

1. **BPT Key**: A database key (`*record.Key`) containing the account URL (or KeyHash for long URLs)
2. **BPT Value**: A 32-byte hash that:
   - Verifies the account's complete state
   - **ALSO serves as a database key** for accessing related data structures

### The Dual-Key Architecture

Both the BPT key AND value are used to retrieve data:

1. **BPT Key** → Locates account records:
   - `Key.Append("Main")` → Account state
   - `Key.Append("MainChain")` → Transaction chain
   - `Key.Append("Directory")` → Sub-accounts

2. **BPT Value (Hash)** → Also a database key:
   - Used to retrieve transaction records: `Transaction(hash)`
   - Used to link related structures across the database
   - Enables navigation between interconnected accounts

### How Database Keys Work

For account URLs, there are two cases:

1. **Short URLs** (directly stored):
   ```go
   // URL: acc://alice.acme
   Key: record.NewKey("Account", "alice.acme")
   ```

2. **Long URLs** (hashed for efficiency):
   ```go
   // URL: acc://very-long-account-name-that-would-be-inefficient/tokens
   KeyHash: sha256(URL) // e.g., 0x1234...
   Key: record.NewKey(KeyHash(0x1234...))
   
   // The database stores a mapping:
   // KeyHash(0x1234...) + "Url" → "acc://very-long-account-name..."
   ```

### How to Get from BPT Entry to Account Transactions

The BPT provides TWO ways to access data:

1. **BPT Entry Contains**:
   ```go
   Key:   *record.Key{"Account", "alice.acme"}  // Database key for account
   Value: [32]byte{0xa3, 0xb4, ...}             // Hash that is ALSO a database key
   ```

2. **Data Access Via BPT Key**:
   ```go
   // Using the BPT key to access account structures:
   account := batch.Account(url)  // Uses BPT key
   
   // Access account components:
   mainState := account.Main().Get()           // Protocol account object
   mainChain := account.MainChain().Get()      // Transaction chain
   directory := account.Directory().Get()      // Sub-account URLs
   pending := account.Pending().Get()          // Pending transactions
   
   // Access transactions using hash as key:
   txRecord := account.Transaction(hash)       // Hash is a database key!
   ```

3. **Data Access Via BPT Value (Hash)**:
   ```go
   // The BPT value (hash) is ALSO used as a database key:
   
   // At the batch level:
   transaction := batch.Transaction(hash)      // Direct access via hash
   
   // The hash creates a database path:
   // "Transaction" + hash → Transaction record
   
   // This enables cross-references:
   // - Account A's transaction hash can be used to find the transaction
   // - Transaction can reference other accounts
   // - Sub-accounts in Directory have their own BPT entries
   ```

4. **Complete Navigation Example**:
   ```go
   // Start with an ADI account
   adi := batch.Account("acc://alice")
   
   // Get sub-accounts from directory
   directory := adi.Directory().Get()  // Returns []*url.URL
   
   // Each sub-account has its own BPT entry
   for _, subUrl := range directory {
       subAccount := batch.Account(subUrl)
       subHash, _ := subAccount.Hash()  // This hash is in the BPT
       
       // The sub-account's hash can be used as a key
       // to access its data from other contexts
   }
   
   // Transaction chains reference transaction hashes
   chain := adi.MainChain().Get()
   for i := 0; i < chain.Height(); i++ {
       entry := chain.Entry(i)
       txHash := entry.Hash
       
       // Use the hash as a database key
       tx := batch.Transaction(txHash)
   }
   ```

### Account Storage Format in Database

The actual account data is stored in the database with this structure:
- **Key Path**: `Account/{URL}/...`
- **Sub-keys**:
  - `Account/{URL}/Main` → Protocol account object
  - `Account/{URL}/MainChain` → Transaction chain
  - `Account/{URL}/Pending` → Pending transactions
  - `Account/{URL}/Directory` → Sub-accounts (for ADIs)

### Comprehensive Account Hash Calculation

The account hash (`bpt_prod.go:29-36`) combines multiple components:

```go
func (a *observedAccount) hashState() (hash.Hasher, error) {
    var hasher hash.Hasher
    
    // 1. Main State - Protocol-defined account data
    hashState(&hasher, a.Main().Get)
    
    // 2. Secondary State - Directory of sub-accounts
    hashState(&hasher, a.hashSecondaryState)
    
    // 3. Chains - Transaction history
    hashState(&hasher, a.hashChains)
    
    // 4. Pending - Unprocessed transactions
    hashState(&hasher, a.hashPending)
    
    return hasher
}
```

#### Component 1: Main State
- Account type-specific data (balances, settings, permissions)
- Serialized protocol buffer of the account object
- Forms the base of the account hash

#### Component 2: Secondary State (`bpt_prod.go:39-61`)
```go
func (a *observedAccount) hashSecondaryState() (hash.Hasher, error) {
    var hasher hash.Hasher
    
    // Directory list for ADI accounts
    for _, u := range a.Directory().Get() {
        dirHasher.AddUrl(u)
    }
    hasher.AddValue(dirHasher)
    
    // Scheduled events for system accounts
    if isSystemAccount {
        hash := a.Events().BPT().GetRootHash()
        hasher.AddHash2(hash)
    }
    
    return hasher
}
```

#### Component 3: Chain Hashing (`bpt_prod.go:65-81`)
```go
func (a *observedAccount) hashChains() (hash.Hasher, error) {
    var hasher hash.Hasher
    
    // Hash each chain's DAG root in alphabetical order
    for _, chainMeta := range a.Chains().Get() {
        chain := a.GetChainByName(chainMeta.Name)
        
        if chain.CurrentState().Count == 0 {
            hasher.AddHash(new([32]byte))  // Empty chain
        } else {
            hasher.AddHash(chain.CurrentState().Anchor())
        }
    }
    
    return hasher
}
```

#### Component 4: Pending Transaction Hashing (`bpt_prod.go:86-117`)
```go
func (a *observedAccount) hashPending() (hash.Hasher, error) {
    var hasher hash.Hasher
    
    for _, txid := range a.Pending().Get() {
        // Add transaction ID
        hasher.AddTxID(txid)
        
        // Add validator signatures
        for _, sig := range txn.ValidatorSignatures().Get() {
            hasher.AddHash(sig.Hash())
        }
        
        // Add credit payments
        for _, hash := range txn.Payments().Get() {
            hasher.AddHash2(hash)
        }
    }
    
    return hasher
}
```

## BPT Operations

### Insert Operation (`mutate.go:79-143`)

The insert operation handles three distinct cases:

```go
func (e *branch) insert(l *leaf) (updated bool, err error) {
    f, err := e.getAt(l.Key.Hash())  // Navigate to position
    
    switch g := (*f).(type) {
    case *emptyNode:
        // Case 1: Empty position - simply insert
        *f = l
        e.status = branchUnhashed
        return true, nil
        
    case *leaf:
        if g.Key.Hash() == l.Key.Hash() {
            // Case 2a: Same key - update value
            g.Value = l.Value
            e.status = branchUnhashed
            return true, nil
        }
        
        // Case 2b: Different key - split into new branch
        br := e.newBranch(g.Key.Hash())
        br.insert(g)  // Insert existing leaf
        br.insert(l)  // Insert new leaf
        *f = br       // Replace leaf with branch
        e.status = branchUnhashed
        return true, nil
        
    case *branch:
        // Case 3: Branch exists - recurse
        updated, err = g.insert(l)
        if updated {
            e.status = branchUnhashed
        }
        return updated, err
    }
}
```

### Hash Calculation (`node.go:105-127`)

Branch nodes compute their hash by combining children:

```go
func (e *branch) getHash() ([32]byte, bool) {
    if e.status != branchUnhashed {
        return e.Hash, true  // Already computed
    }
    
    l, lok := e.Left.getHash()
    r, rok := e.Right.getHash()
    
    switch {
    case lok && rok:
        // Both children exist - combine hashes
        var b [64]byte
        copy(b[:32], l[:])
        copy(b[32:], r[:])
        e.Hash = sha256.Sum256(b[:])
        
    case lok:
        e.Hash = l  // Only left child
        
    case rok:
        e.Hash = r  // Only right child
        
    default:
        e.Hash = [32]byte{}  // No children
    }
    
    e.status = branchUncommitted
    return e.Hash, lok || rok
}
```

### Delete Operation (`mutate.go:145-192`)

Deletion includes branch collapsing optimization:

```go
func (e *branch) delete(key *record.Key) (updated bool, err error) {
    f, err := e.getAt(key.Hash())
    
    switch g := (*f).(type) {
    case *leaf:
        // Replace leaf with empty node
        *f = &emptyNode{parent: e}
        e.status = branchUnhashed
        return true, nil
        
    case *branch:
        updated, err = g.delete(key)  // Recurse
        
        // Collapse branch if possible
        lt, rt := g.Left.Type(), g.Right.Type()
        if lt == nodeTypeEmpty && rt != nodeTypeBranch {
            *f = g.Right  // Collapse to right
        } else if rt == nodeTypeEmpty && lt != nodeTypeBranch {
            *f = g.Left   // Collapse to left
        }
        
        e.status = branchUnhashed
        return true, nil
    }
}
```

## Integration with Database Layer

### Update Flow (`account.go:162-183`)

The BPT update process during batch commit:

```go
func (b *Batch) UpdateBPT() error {
    // Update parent batch first (nested transactions)
    if b.parent != nil {
        err := b.parent.UpdateBPT()
        if err != nil {
            return err
        }
    }
    
    // Process all dirty accounts
    for _, a := range b.account {
        if a.IsDirty() {
            // Commit account changes
            err := a.Commit()
            if err != nil {
                return errors.UnknownError.WithFormat("commit %v: %w", a.Url(), err)
            }
            
            // Update BPT entry
            err = a.putBpt()
            if err != nil {
                return errors.UnknownError.WithFormat("update BPT entry for %v: %w", a.Url(), err)
            }
        }
    }
    return nil
}
```

### Account BPT Update (`bpt_account.go:29-50`)

```go
func (a *Account) putBpt() error {
    // Ensure URL state exists
    _, err := a.getUrl().Get()
    if errors.Is(err, errors.NotFound) {
        err = a.getUrl().Put(a.Url())
    }
    
    // Get account hash through observer
    hasher, err := a.parent.observer.DidChangeAccount(a.parent, a)
    if err != nil {
        return err
    }
    
    // Insert into BPT
    return a.parent.BPT().Insert(a.key, hasher.MerkleHash())
}
```

### Observer Pattern

The `DatabaseObserver` interface ensures consistent hashing:

```go
type Observer interface {
    DidChangeAccount(batch *Batch, account *Account) (hash.Hasher, error)
}
```

This abstraction allows different hashing strategies while maintaining consistency.

## Security Model

### Core Security Properties

1. **Tamper-Proof**: Any modification to any account changes the root hash
2. **Complete Coverage**: Root hash captures entire account set state
3. **Verifiable**: Merkle proofs enable selective verification without full tree
4. **Deterministic**: Same accounts always produce identical root hash

### Security Analysis (from `snapshot-bpt-security-analysis.md`)

#### Why BPT Sections Should Be Ignored During Restoration

**The Problem with BPT Sections:**
1. **Missing Entry Attack**: BPT sections can omit entries while still validating
2. **Incomplete Validation**: Only checks included entries, not completeness
3. **Additional Attack Surface**: More parsing code = more vulnerabilities

**The Solution:**
- Always rebuild BPT from accounts during restoration
- Validate only through root hash comparison
- Accounts are the authoritative source of truth

### Root Hash Validation Strategy

Three validation scenarios:

1. **Non-Zero Root Hash Match**: Full validation successful
   - High security - detects any tampering
   - Used for genesis and full network snapshots

2. **Zero Root Hash**: Skip validation with warning
   - Reduced security - no integrity verification
   - Normal for partition snapshots

3. **Hash Mismatch**: Critical error
   - Indicates tampering or corruption
   - Restoration must abort

## Performance Optimizations

### 1. Deferred Updates

Updates are batched in memory before applying to tree:

```go
type BPT struct {
    pending map[[32]byte]*mutation  // Staged changes
    // ...
}

type mutation struct {
    applied   bool
    committed bool
    delete    bool
    key       *record.Key
    value     []byte
}
```

Benefits:
- Reduces tree manipulation overhead
- Enables atomic batch updates
- Improves cache locality

### 2. Lazy Loading

Branches load children only when accessed:

```go
func (e *branch) load() error {
    if e.Left != nil {
        return nil  // Already loaded
    }
    
    // Load from storage only when needed
    err := e.bpt.store.GetValue(e.bpt.key.Append(e.Key), nodeRecord{e})
    if errors.Is(err, errors.NotFound) {
        // Initialize empty children
        e.Left = &emptyNode{parent: e}
        e.Right = &emptyNode{parent: e}
    }
    return err
}
```

### 3. Copy-on-Write Semantics

The `copyWith()` method enables immutable operations:

```go
func (e *branch) copyWith(s *stateData, p *branch, clean bool) node {
    f := &branch{
        bpt:    p.bpt,
        parent: p,
        Height: e.Height,
        Key:    e.Key,
        Hash:   e.Hash,
    }
    
    // Boundary optimization - don't recurse at boundaries
    if e.Height&s.Mask == 0 {
        return f
    }
    
    // Recursive copy
    f.Left = e.Left.copyWith(s, f, clean)
    f.Right = e.Right.copyWith(s, f, clean)
    return f
}
```

### 4. Branch Status Tracking

Efficient hash caching through status management:

```go
type branchStatus int

const (
    branchClean       // No changes
    branchUnhashed    // Changes made, hash outdated
    branchUncommitted // Hash updated, not yet persisted
)
```

## Snapshot and Restoration

### Collection Process (`snapshot.go:403-484`)

BPT entries are collected in batches for efficiency:

```go
it := batch.BPT().Iterate(1000)  // Process 1000 entries at a time
for it.Next() {
    for _, entry := range it.Value() {
        // Get account key and hash
        key := batch.resolveAccountKey(entry.Key)
        
        // Write to snapshot
        err = wr.WriteValue(&snapshot.RecordEntry{
            Key:   key,
            Value: entry.Value[:],  // 32-byte hash
        })
        
        // Track account types for statistics
        if account, err := batch.Account(url).Main().Get(); err == nil {
            accountTypeCounters[account.Type()]++
        }
    }
}
```

### Restoration Best Practices

Based on security analysis, the recommended approach:

1. **Always ignore BPT sections** from snapshots
2. **Rebuild BPT from all accounts** in the snapshot
3. **Validate only via root hash** comparison
4. **Handle zero root hash** gracefully (common for partitions)

```go
func RestoreBPT(db *Database, snapshot *Snapshot) error {
    batch := db.Begin(true)
    defer batch.Discard()
    
    // Restore all accounts
    for _, account := range snapshot.Accounts {
        batch.RestoreAccount(account)
    }
    
    // Rebuild BPT from accounts
    err := batch.UpdateBPT()
    if err != nil {
        return err
    }
    
    // Validate root hash
    actualHash, _ := batch.GetBptRootHash()
    expectedHash := snapshot.Header.RootHash
    
    if expectedHash == [32]byte{} {
        log.Warn("Zero root hash - cannot validate")
        return batch.Commit()
    }
    
    if actualHash != expectedHash {
        return errors.InvalidRecord.WithFormat(
            "BPT mismatch: expected %x, got %x",
            expectedHash, actualHash)
    }
    
    return batch.Commit()
}
```

## Key Design Patterns

### 1. Observer Pattern
Decouples account hashing from BPT operations, allowing flexible hashing strategies.

### 2. Lazy Evaluation
Defers expensive operations (loading, hashing) until actually needed.

### 3. Immutable Data Structures
Copy-on-write semantics enable safe concurrent access and transaction rollback.

### 4. Batch Processing
Groups operations for improved performance and atomicity.

### 5. Merkle Tree Structure
Provides cryptographic proof with logarithmic verification complexity.

## Critical Implementation Details

### 1. Account Hash Comprehensiveness
The account hash includes not just balances and settings, but:
- Complete transaction history via chain hashes
- All pending operations
- Sub-account directory for ADIs
- Validator signatures and payments

### 2. Bit-Level Navigation
The elegant use of bit manipulation for tree traversal:
- No need to store explicit paths
- Deterministic routing based on key hash
- Efficient single-bit examination per level

### 3. Branch Collapsing
Automatic optimization during deletion:
- Branches with one child collapse to that child
- Maintains minimal tree depth
- Preserves tree balance properties

### 4. Height-Based Key Generation
Node keys encode their position in the tree:
```go
func nodeKeyAt(height uint64, key [32]byte) (nodeKey [32]byte, ok bool) {
    byteCnt := height >> 3          // height/8
    bitCnt := height & 7            // height%8
    copy(nodeKey[:], key[:byteCnt])
    
    lastByte := key[byteCnt]
    lastByte >>= 7 - bitCnt
    lastByte |= 1                   // End marker
    lastByte <<= 7 - bitCnt
    nodeKey[byteCnt] = lastByte
    
    return nodeKey, true
}
```

### 5. Arbitrary Value Support
Configuration option for storing either:
- 32-byte hashes (default, efficient)
- Arbitrary-length values (flexible but less efficient)

## Summary: Understanding the BPT's Role

### What the BPT Is
- **A cryptographic verification layer** that proves account integrity
- **A Merkle tree of hashes** that enables efficient proofs
- **A tamper-detection mechanism** where any change modifies the root

### What the BPT Is NOT
- **Not a primary data store** - actual account data and transactions are in the database
- **Not independent** - it works in conjunction with the database storage layer

### The Complete Picture

When you have a BPT entry for `acc://alice/tokens`:

1. **BPT provides**: 
   - The database key to locate account data
   - A hash to verify data integrity

2. **Database provides**: 
   - The actual account data, transactions, chains, etc.
   - Storage for both short URLs and KeyHash mappings

3. **Data Retrieval Process**:
   - Use BPT key to query database
   - Retrieve account state, chains, pending transactions
   - Verify by hashing retrieved data and comparing to BPT value

### Key Insight for Developers

**BOTH the BPT key AND value are database keys:**

1. **BPT Key** (account identifier):
   - Maps to account records (Main, Chains, Directory, etc.)
   - For long URLs, uses KeyHash indirection
   - Enables account-centric data access

2. **BPT Value** (32-byte hash):
   - Serves dual purpose: verification AND data access
   - Used as database key for transactions and cross-references
   - Enables hash-based lookups across the system
   - NOT cryptographically reversible, but IS a database lookup key

3. **The Complete Picture**:
   - `BPT Entry` = `Database Key (URL)` + `Database Key (Hash)`
   - Both components enable different access patterns
   - Hash values link related structures (transactions, sub-accounts)
   - Creates a web of interconnected data accessible via multiple paths

The genius of this design is that the hash serves both as:
- A cryptographic proof of integrity
- A database key for efficient lookups

This dual-key system enables both hierarchical navigation (via URLs) and direct access (via hashes).

## Conclusion

The BPT is a sophisticated, security-critical component of Accumulate that provides:

1. **Cryptographic integrity** for the entire account database
2. **Efficient verification** through Merkle proofs  
3. **Separation of concerns** between storage (database) and verification (BPT)
4. **Optimized performance** via lazy loading and batching
5. **Robust security model** resistant to tampering

The implementation demonstrates excellent engineering with its elegant bit-manipulation navigation, comprehensive account hashing, and careful optimization strategies. Understanding that the BPT is purely a verification layer—not a storage system—is crucial for working with Accumulate's architecture.

---

**Document Version**: 1.0  
**Last Updated**: 2025-01-12  
**Status**: Technical Reference