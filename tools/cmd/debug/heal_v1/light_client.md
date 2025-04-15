# Light Client Implementation

This document details the Light client implementation and its usage as infrastructure by the synthetic transaction healing process in the Accumulate network.

## Light Client Overview {#light-client-overview}

The Light client is a critical component that provides a consistent API for accessing and manipulating account data, chain data, and transaction data. It serves as the interface between the healing code and the database, abstracting away the details of data storage and retrieval.

**Important**: The Light client is not a healing approach itself, but rather infrastructure used by the `heal_synth` approach to build Merkle proofs and access chain data.

## Light Client Initialization {#light-client-initialization}

The Light client is initialized in `heal_common.go` when a database path is provided:

```go
// In heal_common.go
func (h *healer) initDB() error {
    if lightDb != "" {
        cv2, err := client.New(accumulate.ResolveWellKnownEndpoint(network, "v2"))
        check(err)
        cv2.DebugRequest = debug

        db, err := bolt.Open(lightDb, bolt.WithPlainKeys)
        check(err)

        h.light, err = light.NewClient(
            light.Store(db, ""),
            light.ClientV2(cv2),
            light.Querier(h.C2),
            light.Router(h.router),
        )
        check(err)
        go func() { <-ctx.Done(); _ = h.light.Close() }()
    }
    return nil
}
```

The Light client is created with:
1. A database instance (Bolt DB in this case)
2. A ClientV2 instance for API calls
3. A Querier for querying the network
4. A Router for routing requests

Note that for the `heal_anchor` approach, the Light client is explicitly disabled by setting `lightDb = ""` in `heal_anchor.go`.

**Related Documentation**:
- [Database Requirements](./database.md#current-database-implementation)
- [Database Interfaces](./database.md#database-interface-requirements)

## Light Client Usage in Synthetic Transaction Healing {#light-client-usage-in-synthetic-healing}

The `heal_synth` approach uses the Light client as infrastructure to build Merkle proofs for synthetic transactions. The Light client is used in several critical phases of the synthetic healing process:

### 1. Data Preparation Phase {#data-preparation-phase}

Before attempting to heal a synthetic transaction, the Light client is used to pull necessary account and chain data:

```go
// In heal_synth.go
func (h *healer) healSingle(h *healer, src, dst *protocol.PartitionInfo, num uint64, txid *url.TxID) {
    srcUrl := protocol.PartitionUrl(src.ID)
    dstUrl := protocol.PartitionUrl(dst.ID)

    // Pull chains
    pullSynthDirChains(h)
    pullSynthSrcChains(h, srcUrl)

    // Pull accounts
    pullSynthLedger(h, srcUrl)
    pullSynthLedger(h, dstUrl)

    // Heal
    healSingleSynth(h, src.ID, dst.ID, num, txid)
}
```

This preparation includes:

1. **Pulling Directory Network Chains**:
   ```go
   // In heal_synth.go
   func pullSynthDirChains(h *healer) {
       check(h.light.PullAccount(ctx, protocol.DnUrl().JoinPath(protocol.Network)))
       check(h.light.PullAccountWithChains(ctx, protocol.DnUrl().JoinPath(protocol.Ledger), includeRootChain))
       check(h.light.IndexAccountChains(ctx, protocol.DnUrl().JoinPath(protocol.Ledger)))
       check(h.light.PullAccountWithChains(ctx, protocol.DnUrl().JoinPath(protocol.AnchorPool), func(c *api.ChainRecord) bool {
           return c.Type == merkle.ChainTypeAnchor || c.IndexOf != nil && c.IndexOf.Type == merkle.ChainTypeAnchor
       }))
       check(h.light.IndexAccountChains(ctx, protocol.DnUrl().JoinPath(protocol.AnchorPool)))
   }
   ```

2. **Pulling Source Partition Chains**:
   ```go
   // In heal_synth.go
   func pullSynthSrcChains(h *healer, part *url.URL) {
       check(h.light.PullAccountWithChains(ctx, part.JoinPath(protocol.Ledger), includeRootChain))
       check(h.light.IndexAccountChains(ctx, part.JoinPath(protocol.Ledger)))
       check(h.light.PullAccountWithChains(ctx, part.JoinPath(protocol.Synthetic), func(c *api.ChainRecord) bool {
           return strings.HasPrefix(c.Name, "synthetic-sequence") || c.Name == "main" || c.Name == "main-index"
       }))
       check(h.light.IndexAccountChains(ctx, part.JoinPath(protocol.Synthetic)))
   }
   ```

3. **Pulling Synthetic Ledgers**:
   ```go
   // In heal_synth.go
   func pullSynthLedger(h *healer, part *url.URL) *protocol.SyntheticLedger {
       check(h.light.PullAccountWithChains(h.ctx, part.JoinPath(protocol.Synthetic), func(cr *api.ChainRecord) bool { return false }))
       batch := h.light.OpenDB(false)
       defer batch.Discard()
       var ledger *protocol.SyntheticLedger
       check(batch.Account(part.JoinPath(protocol.Synthetic)).Main().GetAs(&ledger))
       return ledger
   }
   ```

These preparation steps ensure that all the necessary chain data is available in the local database before attempting to build Merkle proofs.

### 2. Receipt Building Phase {#receipt-building-phase}

The Light client is used extensively during the receipt building phase, which is version-specific:

#### V1 Receipt Building {#v1-receipt-building}

For V1 networks, the receipt building process uses the Light client to:

```go
// In healing/synthetic.go
func (h *Healer) buildSynthReceiptV1(ctx context.Context, args HealSyntheticArgs, si SequencedInfo) (*merkle.Receipt, error) {
    // Open a database batch
    batch := args.Light.OpenDB(false)
    defer batch.Discard()
    
    // Get the anchors index
    anchors := batch.Index().PartitionAnchors(protocol.Directory)
    
    // Find the appropriate anchor
    anchor, err := getAnchorForBlock(batch, anchors, block, args.SkipAnchors)
    if err != nil {
        return nil, err
    }
    
    // Build the receipt
    // ... complex receipt building logic
}
```

The V1 receipt building process:
1. Opens a database batch
2. Gets the partition anchors index
3. Finds the appropriate anchor based on block number
4. Builds a receipt using that anchor

#### V2 Receipt Building {#v2-receipt-building}

For V2 networks, the receipt building process is more complex:

```go
// In healing/synthetic.go
func (h *Healer) buildSynthReceiptV2(_ context.Context, args HealSyntheticArgs, si SequencedInfo) (*merkle.Receipt, error) {
    // Open a database batch
    batch := args.Light.OpenDB(false)
    defer batch.Discard()
    
    // Set up URLs
    uSrc := protocol.PartitionUrl(si.Source)
    uSrcSys := uSrc.JoinPath(protocol.Ledger)
    uSrcSynth := uSrc.JoinPath(protocol.Synthetic)
    uDn := protocol.DnUrl()
    uDnSys := uDn.JoinPath(protocol.Ledger)
    uDnAnchor := uDn.JoinPath(protocol.AnchorPool)

    // Load the synthetic sequence chain entry
    b, err := batch.Account(uSrcSynth).SyntheticSequenceChain(si.Destination).Entry(int64(si.Number) - 1)
    // ... error handling
    
    // Build the synthetic ledger part of the receipt
    receipt, err := batch.Account(uSrcSynth).MainChain().Receipt(seqEntry.Source, mainIndex.Source)
    // ... error handling
    
    // Build the BVN part of the receipt
    bvnReceipt, err := batch.Account(uSrcSys).RootChain().Receipt(mainIndex.Anchor, bvnRootIndex.Source)
    // ... error handling
    receipt, err = receipt.Combine(bvnReceipt)
    
    // If not from DN, build the DN-BVN and DN parts
    if !strings.EqualFold(si.Source, protocol.Directory) {
        // ... more receipt building
    }
    
    return receipt, nil
}
```

The V2 receipt building process:
1. Opens a database batch
2. Sets up URLs for all relevant accounts
3. Loads the synthetic sequence chain entry
4. Builds a multi-part receipt that includes:
   - The synthetic ledger part
   - The BVN part
   - The DN-BVN part (if source is not the DN)
   - The DN part (if source is not the DN)
5. Combines all parts into a complete receipt

This complex receipt building process is only possible with the Light client's ability to access and manipulate chain data efficiently.

### 3. Database Access Patterns {#database-access-patterns}

The Light client provides several key database access patterns used in synthetic transaction healing:

1. **Account Access**:
   ```go
   // Access an account
   account := batch.Account(url)
   
   // Get the main chain
   mainChain := account.MainChain()
   
   // Get a specific chain
   chain := account.SyntheticSequenceChain(destination)
   ```

2. **Chain Access**:
   ```go
   // Get a chain entry
   entry, err := chain.Entry(index)
   
   // Build a receipt between two points in a chain
   receipt, err := chain.Receipt(from, to)
   ```

3. **Index Access**:
   ```go
   // Access the index
   index := batch.Index()
   
   // Get partition anchors
   anchors := index.PartitionAnchors(partition)
   
   // Find an index entry
   entry, err := index.Account(url).Chain(name).SourceIndex().FindIndexEntryAfter(source)
   ```

These access patterns provide a consistent and efficient way to work with the database, abstracting away the details of the underlying storage.

## Light Client Interface {#light-client-interface}

The Light client provides several key interfaces that are used by the healing code:

### 1. Client Interface {#client-interface}

```go
// Client provides access to the light client
type Client interface {
    // PullAccount pulls an account from the network
    PullAccount(ctx context.Context, url *url.URL) error
    
    // PullAccountWithChains pulls an account and its chains from the network
    PullAccountWithChains(ctx context.Context, url *url.URL, filter func(*api.ChainRecord) bool) error
    
    // IndexAccountChains indexes the chains of an account
    IndexAccountChains(ctx context.Context, url *url.URL) error
    
    // OpenDB opens a database batch
    OpenDB(writable bool) *DB
    
    // Close closes the client
    Close() error
}
```

This is the main interface used by the synthetic healing code to interact with the Light client.

### 2. DB Interface {#db-interface}

```go
// DB provides access to the database
type DB interface {
    // Account returns an account
    Account(url *url.URL) Account
    
    // Index returns the index
    Index() Index
    
    // Commit commits the batch
    Commit() error
    
    // Discard discards the batch
    Discard()
}
```

This interface is used to access the database and create transactions.

### 3. Account Interface {#account-interface}

```go
// Account provides access to an account
type Account interface {
    // Main returns the main state of the account
    Main() State
    
    // MainChain returns the main chain of the account
    MainChain() Chain
    
    // RootChain returns the root chain of the account
    RootChain() Chain
    
    // SyntheticSequenceChain returns the synthetic sequence chain for a destination
    SyntheticSequenceChain(destination string) Chain
    
    // AnchorChain returns the anchor chain for a source
    AnchorChain(source string) Chain
}
```

This interface is used to access account data and chains.

### 4. Chain Interface {#chain-interface}

```go
// Chain provides access to a chain
type Chain interface {
    // Name returns the name of the chain
    Name() string
    
    // Root returns the root chain
    Root() Chain
    
    // Entry returns a chain entry
    Entry(index int64) ([]byte, error)
    
    // Receipt builds a receipt between two points in the chain
    Receipt(from, to uint64) (*merkle.Receipt, error)
    
    // IndexOf finds the index of a value in the chain
    IndexOf(value []byte) (int64, error)
}
```

This interface is used to access chain data and build Merkle proofs.

### 5. Index Interface {#index-interface}

```go
// Index provides access to the index
type Index interface {
    // Account returns an account index
    Account(url *url.URL) AccountIndex
    
    // PartitionAnchors returns the partition anchors index
    PartitionAnchors(partition string) *IndexDBPartitionAnchors
}
```

This interface is used to access the index and find relationships between chains.

## Implementation Considerations {#implementation-considerations}

When implementing or modifying the Light client or its usage in synthetic transaction healing, consider these key factors:

### 1. Database Schema {#database-schema}

The Light client relies on a specific database schema for storing and retrieving data:

- **Accounts**: Stored under the `account` prefix with the URL as the key
- **Chains**: Stored under the `chain` prefix with the account URL and chain name as the key
- **Indexes**: Stored under the `index` prefix with various subkeys for different index types

Maintaining compatibility with this schema is critical for the Light client to function correctly.

### 2. URL Handling {#url-handling}

The Light client uses URLs extensively for identifying accounts and chains. Proper URL handling is essential:

- URLs must be normalized before being used as keys
- URL comparison must be case-insensitive
- URL paths must be handled correctly, especially for special accounts like `anchors` and `synthetic`

### 3. Chain Indexing {#chain-indexing}

The Light client relies on chain indexing for efficient access to chain data:

- Chains must be indexed before they can be used for building receipts
- Indexes must be updated when chains are modified
- Index lookups must be efficient for performance

### 4. Receipt Building {#receipt-building}

The receipt building process is complex and version-specific:

- V1 and V2 networks use different receipt building processes
- Multi-part receipts must be combined correctly
- Chain entries must be located efficiently

### 5. Error Handling {#error-handling}

Proper error handling is essential for the Light client to function correctly:

- Database errors must be propagated correctly
- Network errors must be handled gracefully
- Missing data must be detected and handled appropriately

## Testing the Light Client {#testing-light-client}

When testing the Light client or its usage in synthetic transaction healing, focus on:

1. **Database Interactions**: Verify that database operations work correctly
2. **Receipt Building**: Verify that receipts are built correctly for both V1 and V2 networks
3. **Chain Indexing**: Verify that chains are indexed correctly and efficiently
4. **URL Handling**: Verify that URLs are handled correctly in all contexts
5. **Error Handling**: Verify that errors are handled gracefully and appropriately

## Conclusion {#conclusion}

The Light client is a critical component of the synthetic transaction healing process, providing the infrastructure needed to build Merkle proofs and access chain data efficiently. Understanding its implementation and usage is essential for maintaining and improving the healing code.

**Related Documentation**:
- [heal_synth Transaction Creation](./transactions.md#heal_synth-transaction-creation)
- [Database Requirements](./database.md#database-usage-in-healing-approaches)
- [Implementation Guidelines](./implementation.md#bright-line-guidelines)
