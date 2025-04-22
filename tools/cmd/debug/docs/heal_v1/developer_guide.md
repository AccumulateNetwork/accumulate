# Developer's Guide to Healing Implementation

This guide is designed for developers who need to understand, modify, or build upon the healing code in the Accumulate Network. It provides a practical, code-focused approach to navigating the healing implementation.

## Quick Start for Developers

### Key Files and Their Purposes

| File | Purpose | Key Functions |
|------|---------|--------------|
| `heal_anchor.go` | Main implementation of anchor healing | `healSingleAnchor()`, `HealAnchor()` |
| `heal_synth.go` | Main implementation of synthetic healing | `healSingleSynth()`, `HealSynthetic()` |
| `heal_common.go` | Shared code for both approaches | `initDB()`, `processSubmissions()` |
| `healing/anchors.go` | Core functionality for anchor healing | `HealAnchor()`, `healAnchorV1()`, `healAnchorV2()` |
| `healing/synthetic.go` | Core functionality for synthetic healing | `HealSynthetic()`, `buildSynthReceiptV1()`, `buildSynthReceiptV2()` |
| `healing/sequenced.go` | Shared code for transaction identification | `ResolveSequenced()`, `SequencedInfo` struct |
| `healing/scan.go` | Network scanning and information gathering | `NetworkInfo` struct, `ScanNetwork()` |

### Common Debugging Scenarios

1. **"Element does not exist" errors**:
   - Check URL construction in your code ([URL Construction Guide](./implementation.md#url-construction))
   - Verify the caching system is working correctly ([Caching System](./implementation.md#caching))
   - Ensure you're querying the correct partition

2. **Transaction submission failures**:
   - Check signature collection for anchor healing ([Signature Collection](./transactions.md#signature-collection))
   - Verify receipt building for synthetic healing ([Receipt Building](./light_client.md#receipt-building))
   - Ensure version-specific code is being used correctly ([Version Handling](./implementation.md#version-handling))

3. **Database-related issues**:
   - For `heal_synth`: Verify database initialization ([Database Initialization](./database.md#current-database-implementation))
   - Check transaction management ([Database Transaction Management](./implementation.md#database-transaction-management))
   - Ensure proper error handling for database operations

## Code Generation Templates

### Anchor Healing Implementation

```go
// Template for implementing anchor healing
func HealAnchor(ctx context.Context, args HealAnchorArgs, si SequencedInfo) error {
    // 1. Resolve the transaction
    record, err := ResolveSequenced[messaging.BlockAnchor](ctx, args.Client, args.NetInfo, 
        si.Source, si.Destination, si.Number, true)
    if err != nil {
        return err
    }

    // 2. Check if the transaction is already known
    // ... check args.Known map

    // 3. Version-specific healing
    if args.NetInfo.Status.ExecutorVersion.V2Enabled() {
        if strings.EqualFold(si.Source, protocol.Directory) && 
            !strings.EqualFold(si.Destination, protocol.Directory) {
            return healDnAnchorV2(ctx, args, si)
        }
        return healAnchorV2(ctx, args, si)
    }
    return healAnchorV1(ctx, args, si)
}
```

### Synthetic Healing Implementation

```go
// Template for implementing synthetic healing
func HealSynthetic(ctx context.Context, args HealSyntheticArgs, si SequencedInfo) error {
    // 1. Resolve the transaction
    record, err := ResolveSequenced[messaging.TransactionMessage](ctx, args.Client, args.NetInfo, 
        si.Source, si.Destination, si.Number, false)
    if err != nil {
        return err
    }

    // 2. Build a receipt
    var receipt *merkle.Receipt
    if args.NetInfo.Status.ExecutorVersion.V2Enabled() {
        receipt, err = buildSynthReceiptV2(ctx, args, si)
    } else {
        receipt, err = buildSynthReceiptV1(ctx, args, si)
    }
    if err != nil {
        return err
    }

    // 3. Create and submit the synthetic message
    // ... create message with receipt
    // ... submit message
    return nil
}
```

## Critical Implementation Details

### URL Construction

There are two different URL construction methods used in the codebase:

```go
// Method 1: Used in sequence.go
srcUrl := protocol.PartitionUrl(src.ID)  // e.g., acc://bvn-Apollo.acme

// Method 2: Used in heal_anchor.go
srcUrl := protocol.DnUrl().JoinPath(protocol.AnchorPool).JoinPath(src.ID)  // e.g., acc://dn.acme/anchors/Apollo
```

**Best Practice**: Standardize on Method 1 for new code and be explicit about which method you're using.

### Database Interaction

For `heal_synth`, proper database interaction is critical:

```go
// Opening a database batch
batch := args.Light.OpenDB(false)  // false = read-only
defer batch.Discard()  // Always defer discard to prevent resource leaks

// Accessing account data
account := batch.Account(url)
chain := account.Chain(name)

// Building receipts
receipt, err := chain.Receipt(from, to)
```

### Caching Implementation

The caching system reduces redundant network requests:

```go
// Cache key structure
type cacheKey struct {
    url  string
    typ  string
}

// Thread-safe cache access
func (c *cache) Get(url, typ string) (interface{}, bool) {
    c.mutex.RLock()
    defer c.mutex.RUnlock()
    value, ok := c.cache[cacheKey{url, typ}]
    return value, ok
}

func (c *cache) Set(url, typ string, value interface{}) {
    c.mutex.Lock()
    defer c.mutex.Unlock()
    c.cache[cacheKey{url, typ}] = value
}
```

## Version-Specific Considerations

### V1 Networks

- Uses `BatchTransaction` for anchor healing
- Simpler receipt building for synthetic healing
- Different URL construction patterns

### V2 Networks

- Uses individual `BlockAnchor` messages for anchor healing
- More complex, multi-part receipt building for synthetic healing
- Special handling for DN→BVN anchors

### V2 Baikonur Networks

- Uses `SyntheticMessage` instead of `BadSyntheticMessage` (pre-Baikonur networks used `BadSyntheticMessage`)
- The name "Bad" doesn't indicate a fault - it refers to the original implementation that was later improved upon in the Baikonur release
- The code checks `args.NetInfo.Status.ExecutorVersion.V2BaikonurEnabled()` to determine which message type to use
- Additional validation requirements for synthetic transactions
- Different signature verification process

## Testing Your Implementation

1. **Unit Testing**:
   - Test URL construction with different partition IDs
   - Test database operations with mock database
   - Test version detection and handling

2. **Integration Testing**:
   - Test with different network configurations
   - Test error cases and recovery
   - Test with different transaction types

3. **Performance Testing**:
   - Test caching effectiveness
   - Test with large transaction volumes
   - Test retry logic under load

## See Also

- [Detailed Implementation Guidelines](./implementation.md)
- [Transaction Creation Details](./transactions.md)
- [Database Requirements](./database.md)
- [Light Client Implementation](./light_client.md)
- [Common Issues and Solutions](./issues.md)
