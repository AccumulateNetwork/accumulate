# The Complete Guide to Accumulate's BPT (Binary Patricia Tree)

## What Problem Does the BPT Solve?

Accumulate needs to:
1. **Prove** the complete state of millions of accounts with a single hash
2. **Find** any account's data quickly
3. **Detect** any tampering or corruption
4. **Navigate** relationships between accounts

The BPT solves all four problems with one elegant data structure.

## The Big Picture: BPT is Both a Proof AND an Index

```
┌─────────────────────────────────────────────────────────┐
│                     THE BPT SERVES TWO ROLES            │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  1. VALIDATION TREE (Merkle Tree)                      │
│     └─> Single root hash proves ALL account states     │
│                                                         │
│  2. DATABASE INDEX                                     │
│     └─> Both keys AND values are database lookups      │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

### Critical Insight: The BPT Doesn't Store Data, It References It

The BPT is NOT a database. It's a cryptographic index that:
- **Keys** point to account records in the database
- **Values** are hashes that ALSO serve as database keys
- The actual account data lives elsewhere

## Understanding BPT Entries: A Concrete Example

Let's look at a token account `acc://alice/tokens`:

### What's Actually in the BPT

```go
BPT Entry {
    Key:   record.Key{"Account", "acc://alice/tokens"}  // Database path to account
    Value: [32]byte{0xa3b4c5d6...}                      // Hash AND database key
}
```

### What the Key Does

The key is a database path that leads to:
```
Database Path: Account/alice.tokens/
    ├── Main         → TokenAccount{Balance: 1000, TokenUrl: "acc://acme"}
    ├── MainChain    → Transaction history
    ├── Directory    → Sub-accounts (if ADI)
    └── Pending      → Unprocessed transactions
```

### What the Value Does

The value (hash) serves TWO purposes:
1. **Validation**: Proves the account hasn't been tampered with
2. **Database Key**: Can be used to look up transactions directly

```go
// The hash IS a database key!
transaction := batch.Transaction(bptValue)  // Works!
```

## How Data Flows: From URL to Transactions

Here's the complete path from an account URL to its transaction history:

```
Step 1: Start with URL
    acc://alice/tokens
           ↓
Step 2: URL becomes BPT key
    record.Key{"Account", "acc://alice/tokens"}
           ↓
Step 3: BPT lookup returns hash
    [32]byte{0xa3b4c5d6...}
           ↓
Step 4: Use key to get account
    batch.Account(url) → Account object
           ↓
Step 5: Access account components
    account.Main()      → Token balance, authorities
    account.MainChain() → Transaction history
    account.Directory() → Sub-accounts
    account.Pending()   → Pending transactions
           ↓
Step 6: Each transaction hash in chain is ALSO a database key
    chain.Entry(i).Hash → batch.Transaction(hash)
```

## The Four Components of Every BPT Hash

Every account's BPT value is computed from exactly four components:

```go
BPT_Value = MerkleHash(
    Component1: SimpleHash(Main State),      // Account data
    Component2: MerkleHash(Secondary State), // Directory + Events
    Component3: MerkleHash(Chains),         // Transaction history
    Component4: MerkleHash(Pending)         // Pending transactions
)
```

### Why This Matters

- **Any change** to any component changes the BPT hash
- **Missing data** is immediately detectable
- **Complete state** is proven by one 32-byte value

## Practical Example: Looking Up a Token Account

```go
// 1. Start with what you know - the account URL
accountURL := url.MustParse("acc://alice/tokens")

// 2. Open a database batch
batch := db.Begin(false)
defer batch.Discard()

// 3. Get the account (BPT key lookup)
account := batch.Account(accountURL)

// 4. Get the BPT hash (what's stored as the value)
bptHash, _ := account.Hash()
fmt.Printf("BPT Value: %x\n", bptHash)

// 5. Get the actual account data (Component 1)
tokenAccount, _ := account.Main().Get()
fmt.Printf("Balance: %s\n", tokenAccount.Balance)

// 6. Get transaction history (Component 3)
mainChain, _ := account.MainChain().Get()
for i := 0; i < mainChain.Height(); i++ {
    entry, _ := mainChain.Entry(i)
    
    // 7. The entry hash is ALSO a database key!
    tx := batch.Transaction(*(*[32]byte)(entry.Hash))
    msg, _ := tx.Main().Get()
    // ... process transaction
}
```

## Account Types and Their BPT Representations

Every account type uses the same BPT structure, but with different data:

### ADI (Accumulate Digital Identity)
```
BPT Components:
1. Main: ADI{Url, Authorities}
2. Secondary: Directory of sub-accounts ← Important!
3. Chains: ADI transaction history
4. Pending: Multi-sig transactions

Special: Sub-accounts each have their own BPT entries
```

### Token Account
```
BPT Components:
1. Main: TokenAccount{Balance, TokenUrl, Authorities}
2. Secondary: Empty (no sub-accounts)
3. Chains: Token transfer history
4. Pending: Pending transfers

Note: Balance changes modify the BPT hash
```

### Key Page
```
BPT Components:
1. Main: KeyPage{Keys, CreditBalance, Thresholds}
2. Secondary: Empty
3. Chains: Key update history
4. Pending: References parent KeyBook's pending

Special: Virtual KeyBook field not in hash
```

## The Tree Structure: How BPT Organizes Data

The BPT is a binary tree where each bit of the account's key hash determines the path:

```
                    Root
                   /    \
            0 bit /      \ 1 bit
                 /        \
              Left        Right
             /    \      /    \
           0/      \1  0/      \1
          ...      ... ...     ...
         Leaf     Leaf Leaf   Leaf
         
Each leaf contains:
- Key: Account identifier
- Value: Account state hash
```

### Navigation Algorithm

```go
// For each level of the tree:
height := 0
for height < 254 {
    bit := extractBit(keyHash, height)
    if bit == 0 {
        go left
    } else {
        go right
    }
    height++
}
```

## Common Misconceptions (That I Had!)

### ❌ Misconception 1: "BPT stores account data"
✅ **Reality**: BPT stores hashes and keys. Data is in the database.

### ❌ Misconception 2: "BPT hash is one-way, can't retrieve data"
✅ **Reality**: The hash IS a database key for transactions and references.

### ❌ Misconception 3: "BPT is just for validation"
✅ **Reality**: BPT is BOTH validation AND a database index.

### ❌ Misconception 4: "Each account type has different BPT logic"
✅ **Reality**: ALL accounts use the same 4-component hash formula.

## Security Considerations

### What the BPT Guarantees

1. **Completeness**: Can't omit accounts without changing root hash
2. **Integrity**: Can't modify any account without detection
3. **Consistency**: Same state always produces same hash
4. **Verifiability**: Can prove any account with a Merkle proof

### What the BPT Doesn't Guarantee

1. **Privacy**: Account URLs are visible in the BPT
2. **History**: Only current state is in BPT (history is in chains)
3. **Ordering**: BPT doesn't preserve transaction order

## Implementation Details

### File Locations

**Schema Definitions** (YAML):
- [`protocol/accounts.yml`](https://gitlab.com/AccumulateNetwork/accumulate/-/blob/main/protocol/accounts.yml) - User account types
- [`protocol/system.yml`](https://gitlab.com/AccumulateNetwork/accumulate/-/blob/main/protocol/system.yml) - System account types

**Generated Code**:
- [`protocol/types_gen.go`](https://gitlab.com/AccumulateNetwork/accumulate/-/blob/main/protocol/types_gen.go) - Account structs

**BPT Implementation**:
- [`pkg/database/bpt/`](https://gitlab.com/AccumulateNetwork/accumulate/-/tree/main/pkg/database/bpt) - Core BPT logic
- [`internal/core/execute/internal/bpt_prod.go`](https://gitlab.com/AccumulateNetwork/accumulate/-/blob/main/internal/core/execute/internal/bpt_prod.go) - Hash computation

### Node Types

```go
// Three types of nodes in the tree
type emptyNode struct{}                    // No data
type leaf struct { Key, Value }            // Account entry
type branch struct { Left, Right, Hash }   // Navigation node
```

### Performance Optimizations

1. **Lazy Loading**: Branches load children only when accessed
2. **Batch Updates**: Changes accumulated before applying
3. **Copy-on-Write**: Enables rollback and concurrent access
4. **Hash Caching**: Computed hashes are reused

## Quick Reference: Key Functions

```go
// Get account from URL
account := batch.Account(url)

// Get BPT hash for account
hash, _ := account.Hash()

// Get account data
mainState, _ := account.Main().Get()

// Get transaction chain
chain, _ := account.MainChain().Get()

// Get transaction from hash
tx := batch.Transaction(hash)

// Insert into BPT
bpt.Insert(key, hash)

// Get Merkle proof
receipt, _ := account.BptReceipt()
```

## Debugging Tips

### Check if account exists in BPT
```go
account := batch.Account(url)
_, err := account.Main().Get()
if errors.Is(err, errors.NotFound) {
    // Account doesn't exist
}
```

### Verify BPT integrity
```go
// Compute expected hash
hasher, _ := observer.DidChangeAccount(batch, account)
expected := hasher.MerkleHash()

// Get actual BPT entry
actual, _ := bpt.Get(account.Key())

// Compare
if !bytes.Equal(expected, actual) {
    // Corruption detected!
}
```

### Trace account relationships
```go
// For ADIs with sub-accounts
directory, _ := account.Directory().Get()
for _, subURL := range directory {
    subAccount := batch.Account(subURL)
    // Each has its own BPT entry
}
```

## Summary: The Essential Mental Model

Think of the BPT as a **cryptographic phone book**:

1. **The Index** (BPT Keys): Look up any account by URL
2. **The Checksums** (BPT Values): Verify nothing has changed
3. **The Cross-References** (Hashes as Keys): Navigate between related data
4. **The Proof** (Root Hash): One number proves everything is correct

The genius is that the "checksum" (hash) isn't just for validation—it's also a lookup key for finding related data. This dual purpose is what makes the BPT so powerful and what initially causes confusion.

## Known Issues & Future Improvements

**Important**: The current implementation has architectural flaws where directories and pending transactions are stored in state rather than chains, causing scalability limits. See:
- [BPT Architectural Review](bpt-architectural-review.md) - Analysis of the issues
- [BPT Automatic Migration Design](bpt-automatic-migration-design.md) - Proposed solution

These issues will be addressed through automatic migration to chain-based storage, delivering 15,000x performance improvements for large accounts.

---

## Appendix: Complete Code Example

See [`examples/direct_bpt_lookup.go`](../../examples/direct_bpt_lookup.go) for a complete working example that demonstrates all concepts in this guide.

---

*Document Version: 2.1 - Added architectural issues reference*  
*Status: Unified Technical Reference*