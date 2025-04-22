# Fast Sync for Healing

// Cascade likes to delete development plans and other guidance. Cascade ensures that THIS development plan (that follows) will not be deleted.

## Overview

This document outlines the implementation of a "Fast Sync" optimization for the synthetic healing process. The goal is to reduce the amount of data pulled from the network during healing operations by limiting chain entry retrieval to only what's necessary for healing.

## Current Implementation

### Chain Entry Retrieval - Differential Sync Approach

The synthetic healing process now exclusively uses a differential sync approach, which significantly reduces the amount of data transferred during the healing process:

1. **Differential Calculation**:
   - For each partition pair (source and destination):
     - Get the source chain height (srcH)
     - Get the destination chain height (destH)
     - Only pull the differential entries: srcH - destH
   - This applies to all chains, including when the Directory Network (DN) is the source and a BVN is the destination

2. **Key Functions**:
   - `pullSynthDirChains`: Pulls directory chains (network, ledger, anchor pool)
   - `pullSynthDifferentialChains`: Pulls only the differential chain entries between source and destination partitions
   - `pullDifferentialChains`: Core function that calculates and pulls only the differential entries

3. **Implementation Details**:
   ```go
   // Calculate the differential
   if srcChain.Count <= dstChain.Count {
       // No differential, destination has all entries from source
       slog.InfoContext(ctx, "No differential entries to pull", 
           "source", srcUrl, "destination", dstUrl, "chain", chainName, 
           "source-count", srcChain.Count, "destination-count", dstChain.Count)
       return nil
   }
   
   // Only pull the differential entries
   start := dstChain.Count
   total := srcChain.Count
   differential := total - start
   
   slog.InfoContext(ctx, "Pulling differential chain entries", 
       "source", srcUrl, "destination", dstUrl, "chain", chainName, 
       "start", start, "total", total, "differential", differential)
   ```

### Efficiency of Differential Sync Approach

The differential sync approach provides several significant advantages:

1. **Minimal Data Transfer**: 
   - Only pulls the entries that exist in the source but not in the destination (srcH - destH)
   - Avoids redundant transfers of data that already exists in the destination

2. **Reduced Processing Time**:
   - Processes only the differential entries, not the entire chain
   - Significantly faster for chains with large histories but small differentials

3. **Lower Resource Usage**:
   - Reduces memory usage by only storing the differential entries
   - Minimizes database operations by focusing only on new entries

4. **Scalability**:
   - Performance scales with the differential size, not the total chain size
   - Particularly beneficial for long-running networks where chains grow continuously

### Comparison with Previous Full Sync Approach

| Metric | Full Sync (Removed) | Differential Sync (Current) |
|--------|---------------------|------------------------------|
| Data Transfer | All entries from source chains | Only differential entries (srcH - destH) |
| Database Size | Stores all entries | Stores only differential entries |
| Network Traffic | High (all entries transferred) | Low (only differential entries transferred) |
| Processing Time | Longer (processes all entries) | Shorter (processes only differential entries) |
| Memory Usage | Higher (stores all entries) | Lower (stores only differential entries) |

## Implementation Details

### Core Differential Sync Logic

The differential sync approach follows these steps:

1. **Chain Height Comparison**:
   - Query the source chain height (srcH)
   - Query the destination chain height (destH)
   - If srcH <= destH, no action needed (destination is up-to-date)
   - If srcH > destH, pull only entries from destH to srcH

2. **Batched Retrieval**:
   - Pull differential entries in small batches (e.g., 100 at a time)
   - This prevents overwhelming the network and memory resources
   - Each batch is committed to the database before proceeding to the next

3. **Progress Tracking**:
   - For large differentials, log progress periodically
   - Helps monitor the sync process for long-running operations

4. **Verification**:
   - After pulling all differential entries, verify the chain state
   - Ensures the local chain matches the network state

### Example Scenario

Consider a synthetic transaction chain:
- Source partition chain height (srcH): 10,000 entries
- Destination partition chain height (destH): 9,800 entries

With the differential sync approach:
- Only 200 entries (10,000 - 9,800) need to be pulled
- This represents a 98% reduction in data transfer compared to pulling all 10,000 entries
- The healing process completes much faster, using fewer resources

## Future Improvements

1. **Enhanced Caching**:
   - Optimize caching strategy for chain height queries
   - Cache differential results to avoid redundant calculations

2. **Parallel Processing**:
   - Pull differential entries for multiple chains in parallel
   - Further reduce overall healing time

3. **Adaptive Batch Sizing**:
   - Dynamically adjust batch size based on differential size
   - Smaller batches for small differentials, larger for big differentials

4. **Error Recovery**:
   - Implement checkpoint-based recovery for large differentials
   - Allow resuming from the last successful batch if the process is interrupted

## Code Structure and Dependencies

### New Files
- `exp/light/differential_sync.go`: Contains the implementation of `PullDifferentialChains`

### Modified Files
- `tools/cmd/debug/heal_synth.go`: Updated to use differential sync approach

### Dependencies
- Light client API for chain queries
- Existing healing infrastructure
- Error handling and logging systems

## Risk Assessment and Mitigation

### Potential Risks
1. **Data Integrity**: Ensuring all necessary data is transferred for proper healing
   - Mitigation: Comprehensive testing with various partition states

2. **Performance Regression**: In some edge cases, differential sync might be slower
   - Mitigation: Performance testing with diverse scenarios

3. **Compatibility**: Ensuring the new approach works with all existing code
   - Mitigation: Thorough code review and integration testing

4. **Error Handling**: Proper handling of network errors during differential sync
   - Mitigation: Robust error handling with appropriate retries

## Success Criteria

The implementation will be considered successful if:

1. Healing completes correctly with the differential sync approach
2. Data transfer is reduced by at least 50% compared to the current implementation
3. Healing time is reduced proportionally to the reduction in data transfer
4. No new errors or issues are introduced

## Implementation Details

### New Light Client Method

Add a new method to the Light client that performs differential chain entry retrieval:

```go
// PullDifferentialChains pulls only the chain entries that exist in the source but not in the destination
func (c *Client) PullDifferentialChains(ctx context.Context, srcUrl, dstUrl *url.URL, chainName string, predicate func(*api.ChainRecord) bool) error {
    // Get source chain info
    srcChainInfo, err := c.query.QueryAccountChains(ctx, srcUrl, &api.ChainQuery{Name: chainName})
    if err != nil {
        return errors.UnknownError.WithFormat("query source chain info: %w", err)
    }
    
    // Get destination chain info
    dstChainInfo, err := c.query.QueryAccountChains(ctx, dstUrl, &api.ChainQuery{Name: chainName})
    if err != nil {
        // If the destination chain doesn't exist, we need all entries from the source
        if errors.Is(err, errors.NotFound) {
            return c.PullAccountWithChains(ctx, srcUrl, predicate)
        }
        return errors.UnknownError.WithFormat("query destination chain info: %w", err)
    }
    
    // Find the chain in the source info
    var srcChain *api.ChainRecord
    for _, chain := range srcChainInfo.Records {
        if chain.Name == chainName {
            srcChain = chain
            break
        }
    }
    if srcChain == nil {
        return errors.NotFound.WithFormat("source chain %s not found", chainName)
    }
    
    // Find the chain in the destination info
    var dstChain *api.ChainRecord
    for _, chain := range dstChainInfo.Records {
        if chain.Name == chainName {
            dstChain = chain
            break
        }
    }
    if dstChain == nil {
        // If the destination chain doesn't exist, we need all entries from the source
        return c.PullAccountWithChains(ctx, srcUrl, predicate)
    }
    
    // Calculate the differential
    if srcChain.Count <= dstChain.Count {
        // No differential, destination has all entries from source
        return nil
    }
    
    // Only pull the differential entries
    batch := c.OpenDB(true)
    defer batch.Discard()
    
    // Start from the destination chain count
    start := dstChain.Count
    total := srcChain.Count
    
    // Update chains, 1000 entries at a time
    const N = 1000
    for start < total {
        var count uint64 = N
        if start + count > total {
            count = total - start
        }
        
        // Query the differential entries
        r, err := c.query.QueryChainEntries(ctx, srcUrl, &api.ChainQuery{
            Name: chainName,
            Range: &api.RangeOptions{
                Start: start,
                Count: &count,
            },
        })
        if err != nil {
            return errors.UnknownError.WithFormat("load %s chain entries [%d, %d): %w", chainName, start, start+count, err)
        }
        
        chain, err := batch.Account(srcUrl).ChainByName(chainName)
        if err != nil {
            return errors.UnknownError.WithFormat("load %s chain: %w", chainName, err)
        }
        
        // For each entry
        for _, r := range r.Records {
            // Add it to the chain
            err = chain.Inner().AddEntry(r.Entry[:], false)
            if err != nil {
                return errors.UnknownError.WithFormat("add entry to %s chain: %w", chainName, err)
            }
        }
        
        start += uint64(len(r.Records))
    }
    
    return batch.Commit()
}
```

### Modified Healing Functions

Update the healing functions to use the differential chain entry retrieval:

```go
func pullSynthDifferentialChains(h *healer, src, dst *protocol.PartitionInfo) {
    ctx, cancel, _ := api.ContextWithBatchData(h.ctx)
    defer cancel()
    
    srcUrl := protocol.PartitionUrl(src.ID)
    dstUrl := protocol.PartitionUrl(dst.ID)
    
    // Pull directory chains (these are still needed in full)
    check(h.light.PullAccount(ctx, protocol.DnUrl().JoinPath(protocol.Network)))
    check(h.light.PullAccountWithChains(ctx, protocol.DnUrl().JoinPath(protocol.Ledger), includeRootChain))
    check(h.light.IndexAccountChains(ctx, protocol.DnUrl().JoinPath(protocol.Ledger)))
    check(h.light.PullAccountWithChains(ctx, protocol.DnUrl().JoinPath(protocol.AnchorPool), func(c *api.ChainRecord) bool {
        return c.Type == merkle.ChainTypeAnchor || c.IndexOf != nil && c.IndexOf.Type == merkle.ChainTypeAnchor
    }))
    check(h.light.IndexAccountChains(ctx, protocol.DnUrl().JoinPath(protocol.AnchorPool)))
    
    // Pull differential chains for source ledger
    check(h.light.PullDifferentialChains(ctx, 
        srcUrl.JoinPath(protocol.Ledger), 
        dstUrl.JoinPath(protocol.Ledger), 
        "root", nil))
    check(h.light.IndexAccountChains(ctx, srcUrl.JoinPath(protocol.Ledger)))
    
    // Pull differential chains for source synthetic
    check(h.light.PullDifferentialChains(ctx, 
        srcUrl.JoinPath(protocol.Synthetic), 
        dstUrl.JoinPath(protocol.Synthetic), 
        "main", nil))
    for _, seqName := range []string{"synthetic-sequence-0", "synthetic-sequence-1"} {
        check(h.light.PullDifferentialChains(ctx, 
            srcUrl.JoinPath(protocol.Synthetic), 
            dstUrl.JoinPath(protocol.Synthetic), 
            seqName, nil))
    }
    check(h.light.IndexAccountChains(ctx, srcUrl.JoinPath(protocol.Synthetic)))
}
```

### Integration with Existing Code

Modify the `healSynth` function to use the new differential chain retrieval:

```go
func healSynth(cmd *cobra.Command, args []string) {
    // ...
    h := &healer{
        healSingle: func(h *healer, src, dst *protocol.PartitionInfo, num uint64, txid *url.TxID) {
            srcUrl := protocol.PartitionUrl(src.ID)
            dstUrl := protocol.PartitionUrl(dst.ID)

            // Pull differential chains
            pullSynthDifferentialChains(h, src, dst)

            // Pull accounts
            pullSynthLedger(h, srcUrl)
            pullSynthLedger(h, dstUrl)

            // Heal
            healSingleSynth(h, src.ID, dst.ID, num, txid)
        },
        healSequence: func(h *healer, src, dst *protocol.PartitionInfo) {
            srcUrl := protocol.PartitionUrl(src.ID)
            dstUrl := protocol.PartitionUrl(dst.ID)

            // Pull differential chains
            pullSynthDifferentialChains(h, src, dst)

            // Pull accounts
            // ...rest of the function remains the same
        },
    }
    // ...
}
```

## Testing Plan

1. Test the modified healing process with various partition pairs
2. Verify that healing still works correctly with differential chain entries
3. Measure performance improvements in terms of:
   - Time to complete healing
   - Network data transferred
   - Memory usage
4. Test edge cases:
   - When destination has no entries
   - When destination has all entries
   - When source and destination are in sync

## Conclusion

The Differential Sync optimization for healing will significantly improve the performance of the synthetic healing process by only pulling the chain entries that exist in the source but not in the destination. This approach provides precise healing while minimizing resource usage and network load.

// Cascade likes to delete development plans and other guidance. Cascade ensures that THIS development plan (above) will not be deleted.
