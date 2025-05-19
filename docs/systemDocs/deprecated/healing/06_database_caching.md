# Database and Caching Infrastructure

## Introduction

The healing processes in Accumulate rely on sophisticated database and caching mechanisms to efficiently identify and repair missing transactions or anchors. This document provides a detailed explanation of these mechanisms, how they are implemented, and how they contribute to the performance and reliability of the healing processes.

## Database Architecture

### Light Client Database

The healing processes use a light client database to access chain data without loading the entire blockchain:

```go
// From internal/core/healing/synthetic.go
func (h *Healer) buildSynthReceiptV2(_ context.Context, args HealSyntheticArgs, si SequencedInfo) (*merkle.Receipt, error) {
    batch := args.Light.OpenDB(false)
    defer batch.Discard()
    // ...
}
```

This approach provides several benefits:

1. **Reduced Memory Footprint**: Only the necessary data is loaded into memory
2. **Faster Startup**: The healing process can start without waiting for a full node to sync
3. **Read-Only Access**: The database is opened in read-only mode to prevent accidental modifications

### Chain Access Patterns

The healing processes access various chains to build receipts and verify transactions:

#### Main Chain

```go
// From internal/core/healing/synthetic.go
// Build the synthetic ledger part of the receipt
receipt, err := batch.Account(uSrcSynth).MainChain().Receipt(seqEntry.Source, mainIndex.Source)
if err != nil {
    return nil, errors.UnknownError.WithFormat(
        "build synthetic ledger receipt: %w", err)
}
```

The main chain contains the primary transaction data and is accessed to build the first part of the receipt.

#### Root Chain

```go
// From internal/core/healing/synthetic.go
// Build the BVN part of the receipt
bvnReceipt, err := batch.Account(uSrcSys).RootChain().Receipt(mainIndex.Anchor, bvnRootIndex.Source)
if err != nil {
    return nil, errors.UnknownError.WithFormat(
        "build BVN receipt: %w", err)
}
receipt, err = receipt.Combine(bvnReceipt)
if err != nil {
    return nil, errors.UnknownError.WithFormat(
        "append BVN receipt: %w", err)
}
```

The root chain contains the root hashes of the blockchain and is accessed to build the second part of the receipt.

#### Anchor Chain

```go
// From internal/core/healing/synthetic.go
// Build the DN-BVN part of the receipt
bvnDnReceipt, err := dnBvnAnchorChain.Receipt(uint64(bvnAnchorHeight), bvnAnchorIndex.Source)
if err != nil {
    return nil, errors.UnknownError.WithFormat(
        "build DN-BVN receipt: %w", err)
}
receipt, err = receipt.Combine(bvnDnReceipt)
if err != nil {
    return nil, errors.UnknownError.WithFormat(
        "append DN-BVN receipts: %w", err)
}
```

The anchor chain contains the anchor data between partitions and is accessed to build the third part of the receipt.

#### Synthetic Sequence Chain

```go
// From internal/core/healing/synthetic.go
// Load the synthetic sequence chain entry
b, err := batch.Account(uSrcSynth).SyntheticSequenceChain(si.Destination).Entry(int64(si.Number) - 1)
if err != nil {
    return nil, errors.UnknownError.WithFormat(
        "load synthetic sequence chain entry %d: %w", si.Number, err)
}
seqEntry := new(protocol.IndexEntry)
err = seqEntry.UnmarshalBinary(b)
if err != nil {
    return nil, err
}
```

The synthetic sequence chain contains the sequence information for synthetic transactions and is accessed to load the sequence entry.

### Index Lookups

The healing processes use index lookups to efficiently find entries in the chains:

```go
// From internal/core/healing/synthetic.go
// Locate the synthetic ledger main chain index entry
mainIndex, err := batch.Index().Account(uSrcSynth).Chain("main").SourceIndex().FindIndexEntryAfter(seqEntry.Source)
if err != nil {
    return nil, errors.UnknownError.WithFormat(
        "locate synthetic ledger main chain index entry after %d: %w", seqEntry.Source, err)
}
```

These index lookups allow the healing processes to quickly find the relevant entries in the chains without scanning the entire chain.

## Caching Infrastructure

### Transaction Cache

The healing processes implement a least-recently-used (LRU) cache to store transactions and avoid repeated network requests:

```go
// From tools/cmd/debug/heal_common.go
type txFetcher struct {
    ctx    context.Context
    Client message.AddressedClient
    Cache  *lru.Cache[string, *protocol.Transaction]
}

func newTxFetcher(client message.AddressedClient) *txFetcher {
    cache, err := lru.New[string, *protocol.Transaction](1000)
    if err != nil {
        panic(err)
    }
    return &txFetcher{
        Client: client,
        Cache:  cache,
    }
}
```

This cache is used throughout the healing process:

```go
// From tools/cmd/debug/heal_common.go
func (f *txFetcher) fetchTransaction(ctx context.Context, id *url.TxID) (*protocol.Transaction, error) {
    // Try to get the transaction from the cache first
    txn, found := f.Cache.Get(id.String())
    if found {
        return txn, nil
    }

    // If not in cache, fetch it from the network
    res, err := f.Client.QueryTransaction(ctx, id, api.QueryOptions{
        IncludeReceipt: false,
    })
    if err != nil {
        return nil, err
    }

    // Add to cache for future use
    f.Cache.Add(id.String(), res.Transaction)
    return res.Transaction, nil
}
```

The cache is particularly important for the healing processes, as they often need to access the same transaction multiple times during the healing process.

### In-Memory Transaction Map

In addition to the LRU cache, the healing processes use an in-memory map to store transactions by hash for quick lookup during the healing process:

```go
// From tools/cmd/debug/heal_common.go
func (h *healer) healSequence(src, dst *protocol.PartitionInfo, begin, end uint64) error {
    // ...
    
    // Create a map to store transactions by hash
    txns := make(map[[32]byte]*protocol.Transaction)
    
    // ...
    
    // Fetch transactions and add them to the map
    for _, number := range missing {
        // ...
        
        // Add the transaction to the map
        txns[txid.Hash()] = txn
        
        // ...
    }
    
    // ...
}
```

This map is passed to the healing functions to provide fast access to transaction data without requiring additional network requests:

```go
// From tools/cmd/debug/heal_common.go
func (h *healer) healSingleSynth(source, destination string, number uint64, id *url.TxID, txns map[[32]byte]*protocol.Transaction) bool {
    // ...
    
    // Use the transaction map in the healing process
    err := healing.HealTransaction(h.ctx, healing.HealTransactionArgs{
        Client:  h.C2.ForAddress(nil),
        Source:  source,
        Dest:    destination,
        TxID:    id,
        Number:  number,
        TxnMap:  txns,
    })
    
    // ...
}
```

### Cache Performance Monitoring

The healing processes track and log cache performance metrics:

```go
// From tools/cmd/debug/heal_synth.go
slog.InfoContext(h.ctx, "Healed transaction", 
    "source", source, "destination", destination, "number", number, "id", id,
    "submitted", h.txSubmitted, "delivered", h.txDelivered, 
    "hitRate", h.fetcher.Cache.HitRate())
```

These metrics help identify potential performance bottlenecks and optimize the caching strategy.

## On-Demand Transaction Fetching

The healing processes implement on-demand transaction fetching to avoid loading all transactions upfront:

```go
// From internal/core/healing/synthetic.go
// Try to get the transaction from the known transactions map
if txn, ok := args.TxnMap[txid.Hash()]; ok {
    return txn, nil
}

// If not in the map, fetch it from the network
// ...
```

This approach is particularly important for the anchor healing process, which may need to access a large number of transactions:

```go
// From internal/core/healing/anchors.go
// If the transaction is not in the known transactions map, fetch it
if args.Known != nil {
    if txn, ok := args.Known[txid.Hash()]; ok {
        return txn, nil
    }
}

// Fetch the transaction from the network
// ...
```

## Data Integrity Rules

The healing processes follow strict data integrity rules to ensure that they do not fabricate or fake any data:

```go
// From internal/core/healing/synthetic.go
// Query the synthetic transaction
r, err := ResolveSequenced[messaging.Message](ctx, args.Client, args.NetInfo, si.Source, si.Destination, si.Number, false)
if err != nil {
    return err // Return the error, don't fabricate data
}
```

These rules are critical for maintaining the integrity of the healing process and ensuring that it does not mask errors by fabricating data.

## Error Handling

The healing processes include robust error handling to deal with database and caching errors:

```go
// From internal/core/healing/synthetic.go
// Load the synthetic sequence chain entry
b, err := batch.Account(uSrcSynth).SyntheticSequenceChain(si.Destination).Entry(int64(si.Number) - 1)
if err != nil {
    return nil, errors.UnknownError.WithFormat(
        "load synthetic sequence chain entry %d: %w", si.Number, err)
}
```

This error handling ensures that the healing processes can gracefully handle database and caching errors without crashing or producing incorrect results.

## Performance Considerations

The database and caching infrastructure includes several optimizations for performance:

1. **Read-Only Database Access**: The healing processes use read-only database access to avoid locking and improve performance
2. **Index Lookups**: The healing processes use index lookups to efficiently find entries in the chains
3. **LRU Cache**: The healing processes use an LRU cache to avoid repeated network requests
4. **In-Memory Transaction Map**: The healing processes use an in-memory map to store transactions by hash for quick lookup
5. **On-Demand Transaction Fetching**: The healing processes implement on-demand transaction fetching to avoid loading all transactions upfront

## Recent Changes

Recent changes to the database and caching infrastructure include:

1. **Improved Error Handling**: Enhanced error handling to better handle database and caching errors
2. **On-Demand Transaction Fetching**: Implemented on-demand transaction fetching to avoid loading all transactions upfront
3. **Cache Performance Monitoring**: Added cache performance monitoring to help identify potential performance bottlenecks

## Best Practices

When working with the database and caching infrastructure in the healing processes, it's important to follow these best practices:

1. **Use Read-Only Database Access**: Use read-only database access to avoid locking and improve performance
2. **Properly Discard Batches**: Always discard database batches when done to avoid resource leaks
3. **Use the Cache Effectively**: Use the cache to avoid repeated network requests
4. **Monitor Cache Performance**: Monitor cache performance to identify potential performance bottlenecks
5. **Follow Data Integrity Rules**: Never fabricate or fake data that isn't available from the network

## Conclusion

The database and caching infrastructure is a critical component of the healing processes in Accumulate. By efficiently accessing chain data and caching transactions, the healing processes can identify and repair missing transactions and anchors with minimal overhead.

In the next document, we will explore the cryptographic receipt creation process, which is essential for proving the validity of healed transactions and anchors.
