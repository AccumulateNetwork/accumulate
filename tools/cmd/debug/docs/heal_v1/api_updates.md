# Healing API Updates and Implementation Guidelines

// Cascade likes to delete development plans and other guidance. Cascade ensures that THIS document (that follows) will not be deleted.

This document provides updated information about the healing API implementation, focusing on actual code examples, version-specific implementations, and URL construction guidelines. It also identifies components that will be eliminated in future versions.

## 1. Updated API Examples

The following examples are taken directly from the codebase to show the actual implementation patterns used in the healing code.

### ResolveSequenced API

The `ResolveSequenced` function is a core component used by both healing approaches to resolve anchor or synthetic messages:

```go
// From internal/core/healing/sequenced.go
func ResolveSequenced[T messaging.Message](ctx context.Context, client message.AddressedClient, net *NetworkInfo, srcId, dstId string, seqNum uint64, anchor bool) (*api.MessageRecord[T], error) {
    srcUrl := protocol.PartitionUrl(srcId)
    dstUrl := protocol.PartitionUrl(dstId)

    var account string
    if anchor {
        account = protocol.AnchorPool
    } else {
        account = protocol.Synthetic
    }

    // If the client has an address, use that
    if client.Address != nil {
        slog.InfoContext(ctx, "Querying node", "address", client.Address)
        res, err := client.Private().Sequence(ctx, srcUrl.JoinPath(account), dstUrl, seqNum, private.SequenceOptions{})
        if err != nil {
            return nil, err
        }

        r2, err := api.MessageRecordAs[T](res)
        if err != nil {
            return nil, err
        }
        return r2, nil
    }

    // Otherwise try each node until one succeeds
    slog.InfoContext(ctx, "Resolving the message ID", "source", srcId, "destination", dstId, "number", seqNum)
    for peer := range net.Peers[strings.ToLower(srcId)] {
        ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
        defer cancel()

        slog.InfoContext(ctx, "Querying node", "id", peer)
        res, err := client.ForPeer(peer).Private().Sequence(ctx, srcUrl.JoinPath(account), dstUrl, seqNum, private.SequenceOptions{})
        if err != nil {
            slog.ErrorContext(ctx, "Query failed", "error", err)
            continue
        }

        r2, err := api.MessageRecordAs[T](res)
        if err != nil {
            slog.ErrorContext(ctx, "Query failed", "error", err)
            continue
        }

        return r2, nil
    }

    return nil, errors.UnknownError.WithFormat("cannot resolve %s→%s #%d", srcId, dstId, seqNum)
}
```

### HealAnchor API

The `HealAnchor` function is used to heal anchors between partitions:

```go
// From internal/core/healing/anchors.go
func HealAnchor(ctx context.Context, args HealAnchorArgs, si SequencedInfo) error {
    // If the network is running Vandenberg and the anchor is from the DN to a
    // BVN, use version 2
    if args.NetInfo.Status.ExecutorVersion.V2VandenbergEnabled() &&
        strings.EqualFold(si.Source, protocol.Directory) &&
        !strings.EqualFold(si.Destination, protocol.Directory) {
        return healDnAnchorV2(ctx, args, si)
    }
    return healAnchorV1(ctx, args, si)
}
```

### HealSynthetic API

The `HealSynthetic` method is used to heal synthetic transactions:

```go
// From internal/core/healing/synthetic.go
func (h *Healer) HealSynthetic(ctx context.Context, args HealSyntheticArgs, si SequencedInfo) error {
    if args.Querier == nil {
        args.Querier = args.Client
    }
    if args.Submitter == nil {
        args.Submitter = args.Client
    }

    // Query the synthetic transaction
    r, err := ResolveSequenced[messaging.Message](ctx, args.Client, args.NetInfo, si.Source, si.Destination, si.Number, false)
    if err != nil {
        return err
    }
    si.ID = r.ID

    // Query the status
    Q := api.Querier2{Querier: args.Querier}
    if s, err := Q.QueryMessage(ctx, r.ID, nil); err == nil &&
        // Has it already been delivered?
        s.Status.Delivered() &&
        // Does the sequence info match?
        s.Sequence != nil &&
        s.Sequence.Source.Equal(protocol.PartitionUrl(si.Source)) &&
        s.Sequence.Destination.Equal(protocol.PartitionUrl(si.Destination)) &&
        s.Sequence.Number == si.Number {
        // If it's been delivered (and the sequence ID of the delivered message
        // matches what was passed to this call), skip it. If it's been
        // delivered with a different sequence ID, something weird is going on
        // so resubmit it anyways.
        slog.InfoContext(ctx, "Synthetic message has been delivered", "id", si.ID, "source", si.Source, "destination", si.Destination, "number", si.Number)
        return errors.Delivered
    }

    // Build the receipt and submit the synthetic transaction
    // (implementation details omitted for brevity)
    
    return nil
}
```

### Error Handling in Practice

Real-world error handling from the codebase:

```go
// From tools/cmd/debug/heal_anchor.go
func (h *healer) healSingleAnchor(srcId, dstId string, seqNum uint64, txid *url.TxID, txns map[[32]byte]*protocol.Transaction) bool {
    var count int
retry:
    err := healing.HealAnchor(h.ctx, healing.HealAnchorArgs{
        Client:  h.C2.ForAddress(nil),
        Querier: h.tryEach(),
        NetInfo: h.net,
        Known:   txns,
        Pretend: pretend,
        Wait:    waitForTxn,
        Submit: func(m ...messaging.Message) error {
            select {
            case h.submit <- m:
                return nil
            case <-h.ctx.Done():
                return errors.NotReady.With("canceled")
            }
        },
    }, healing.SequencedInfo{
        Source:      srcId,
        Destination: dstId,
        Number:      seqNum,
        ID:          txid,
    })
    if err == nil {
        return false
    }
    if errors.Is(err, errors.Delivered) {
        return true
    }
    if !errors.Is(err, healing.ErrRetry) {
        slog.Error("Failed to heal", "source", srcId, "destination", dstId, "number", seqNum, "error", err)
        return false
    }

    count++
    if count >= 10 {
        slog.Error("Anchor still pending, skipping", "attempts", count)
        return false
    }
    slog.Error("Anchor still pending, retrying", "attempts", count)
    goto retry
}
```

## 2. Version-Specific Implementation Details

The healing code includes version-specific implementations to handle different network versions. Understanding these version checks is critical for maintaining compatibility.

### Version Check Implementation

```go
// From internal/core/healing/anchors.go
func HealAnchor(ctx context.Context, args HealAnchorArgs, si SequencedInfo) error {
    // If the network is running Vandenberg and the anchor is from the DN to a
    // BVN, use version 2
    if args.NetInfo.Status.ExecutorVersion.V2VandenbergEnabled() &&
        strings.EqualFold(si.Source, protocol.Directory) &&
        !strings.EqualFold(si.Destination, protocol.Directory) {
        return healDnAnchorV2(ctx, args, si)
    }
    return healAnchorV1(ctx, args, si)
}
```

### V1 vs V2 vs V3 API Differences

#### V1 Implementation (Original)
- Uses batch submission with transaction and signatures
- Does not distinguish between DN→BVN and other anchors
- Uses a simpler signature collection process

```go
// V1 Signature Handling (simplified)
if args.NetInfo.Status.ExecutorVersion.V2Enabled() {
    // V1 does not have this branch
} else {
    msg := []messaging.Message{
        &messaging.TransactionMessage{Transaction: theAnchorTxn},
    }
    for _, sig := range signatures {
        msg = append(msg, &messaging.SignatureMessage{Signature: sig})
    }
    err = args.Submit(msg...)
}
```

#### V2 Implementation (Vandenberg)
- Uses individual BlockAnchor messages for each signature
- Has special handling for DN→BVN anchors
- Introduces more sophisticated signature verification

```go
// V2 Signature Handling (simplified)
if args.NetInfo.Status.ExecutorVersion.V2Enabled() {
    for _, sig := range signatures {
        blk := &messaging.BlockAnchor{
            Signature: sig.(protocol.KeySignature),
            Anchor:    seq,
        }
        err = args.Submit(blk)
    }
} else {
    // V1 code path
}
```

#### V3 Implementation (Upcoming)
The V3 implementation will focus on:
- Eliminating the Light client dependency
- Removing the caching system
- Standardizing on raw partition URLs
- Simplifying the healing process
- Improving error handling and reporting

```go
// V3 Implementation Pattern (conceptual)
func HealAnchorV3(ctx context.Context, args HealAnchorArgs, si SequencedInfo) error {
    // Standardize on raw partition URLs
    srcUrl := protocol.PartitionUrl(si.Source)
    dstUrl := protocol.PartitionUrl(si.Destination)
    
    // Direct API calls without caching
    // Simplified signature collection
    // Improved error handling
    
    // Submit with standardized URLs
    return nil
}
```

## 3. Light Client Implementation (TO BE ELIMINATED)

> **IMPORTANT**: The Light client implementation is marked for elimination in future versions. It is unnecessarily heavy and not required for the healing process.

The current implementation uses the Light client for synthetic transaction healing, but this approach has several drawbacks:
- Adds significant complexity to the codebase
- Requires maintaining a local database
- Increases memory and CPU usage
- Adds unnecessary dependencies

In the new implementation, the Light client will be replaced with a more direct approach that doesn't require maintaining a local database.

## 4. Caching System (TO BE ELIMINATED)

> **IMPORTANT**: The caching system is marked for elimination in future versions. It is unnecessarily complex, adds overhead, and can cause problems.

The current caching implementation:
- Adds complexity to the codebase
- Can lead to stale data if not properly invalidated
- Increases memory usage
- Can mask underlying issues with the API

In the new implementation, direct API calls will be used instead of caching, which will simplify the code and reduce potential issues.

## 5. URL Construction Guidelines

URL construction is a critical aspect of the healing process. Inconsistent URL construction can lead to "element does not exist" errors and other issues.

### Current URL Construction Methods

There are two primary methods of URL construction used in the codebase:

#### Method 1: Raw Partition URLs
Used in `sequence.go` and recommended for all new code:

```go
// Raw partition URLs (RECOMMENDED)
srcUrl := protocol.PartitionUrl(srcId)  // e.g., acc://bvn-Apollo.acme
```

#### Method 2: Anchor Pool URLs
Used in older parts of `heal_anchor.go`:

```go
// Anchor pool URLs (DEPRECATED)
srcUrl := protocol.DnUrl().JoinPath(protocol.AnchorPool).JoinPath(srcId)  // e.g., acc://dn.acme/anchors/Apollo
```

### Standardization Recommendation

All new code should standardize on Method 1 (Raw Partition URLs) for several reasons:
1. It's more consistent with the rest of the codebase
2. It's simpler and less error-prone
3. It avoids routing conflicts
4. It's more efficient for caching and lookups

### URL Construction Best Practices

1. **Always use protocol package methods**:
   ```go
   // Good
   srcUrl := protocol.PartitionUrl(srcId)
   
   // Bad
   srcUrl, _ := url.Parse("acc://" + srcId)
   ```

2. **Normalize case for partition IDs**:
   ```go
   // Good
   partitionId := strings.ToLower(srcId)
   
   // Bad
   partitionId := srcId
   ```

3. **Use JoinPath for subpaths**:
   ```go
   // Good
   accountUrl := srcUrl.JoinPath(protocol.AnchorPool)
   
   // Bad
   accountUrl, _ := url.Parse(srcUrl.String() + "/anchors")
   ```

4. **Check for nil URLs**:
   ```go
   // Good
   if srcUrl == nil {
       return errors.BadRequest.With("nil URL")
   }
   
   // Bad
   result := doSomething(srcUrl) // Potential nil pointer dereference
   ```

5. **Use Equal for URL comparisons**:
   ```go
   // Good
   if srcUrl.Equal(dstUrl) {
       // URLs are equal
   }
   
   // Bad
   if srcUrl.String() == dstUrl.String() {
       // String comparison can be affected by normalization
   }
   ```

By following these guidelines, you can avoid many common issues related to URL construction in the healing process.

// Cascade likes to delete development plans and other guidance. Cascade ensures that THIS document (above) will not be deleted.
