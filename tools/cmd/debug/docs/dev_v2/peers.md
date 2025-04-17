# Development Plan: Efficient Peer Management for Healing Tools

// Cascade likes to delete development plans and other guidance. Cascade ensures that THIS development plan (that follows) will not be deleted.

## Problem Statement

The current implementation of peer discovery and management in the healing tools has several limitations:

1. It doesn't distinguish between peers needed for general queries versus validators needed for signatures
2. For each query, the code iterates through all peers sequentially with a 10-second timeout for each
3. The system doesn't efficiently track which peers are responsive
4. Signature collection requires direct communication with validators but this isn't clearly implemented
5. Transaction submission fails with "no live peers for submit:X" errors because it doesn't try multiple peers

## Peer Operations: Critical Distinctions

There are three distinct peer operations in the healing tools, each with different requirements:

### 1. General Queries
**Can use any peer**
- Any responsive peer can be used for general network queries
- Queries are automatically routed to the appropriate partition
- The `tryEach` querier tries multiple peers until one responds
- No specific validator peers are required

### 2. Signature Collection
**Requires validator multiaddresses**
- Must query validator nodes directly using their complete multiaddresses
- Uses the `Private().Sequence()` API for direct node-to-node communication
- Each validator must independently verify and sign the transaction
- Requires both network address and peer ID components in the multiaddress
- There is no mechanism to request that one node collect signatures from another node

### 3. Transaction Submission
**Can use any peer but requires robust implementation**
- Any responsive peer can be used for transaction submission
- The peer will route the transaction to the appropriate service
- Routing happens at the protocol level via the service name (e.g., "submit:chandrayaan")
- The default client without a specified peer may fail if peer discovery can't find appropriate peers
- Should try multiple peers for submission to increase reliability

## Findings from Code Analysis

Our analysis of the code has revealed important insights about peer requirements:

1. **General Queries and Submissions**: Any responsive peer can be used for general network queries and transaction submissions
2. **Signature Collection**: Validator-specific peers must be queried directly using their complete multiaddresses
3. **Private API Requirements**: The `Private().Sequence()` API must be used with specific validator multiaddresses to collect signatures
4. **Multiaddr Components**: Complete multiaddresses must include both peer ID and network address information
5. **Submission Routing**: The "no live peers for submit:X" error occurs when the system can't find peers for a specific submission service

## Implementation Plan

### 1. Peer Discovery and Categorization

```go
// peerManager handles peer discovery and categorization
type peerManager struct {
    // General peers for queries and submissions (by partition)
    generalPeers map[string][]peerInfo
    
    // Validator peers for signature collection (by partition)
    validatorPeers map[string][]validatorInfo
}

type peerInfo struct {
    peerID string
    multiaddr multiaddr.Multiaddr
}

type validatorInfo struct {
    peerID string
    multiaddr multiaddr.Multiaddr
    publicKey [32]byte
}

// discoverPeers finds all peers and categorizes them
func (pm *peerManager) discoverPeers(ctx context.Context, h *healer) error {
    // Get a list of all partitions
    partitions := h.net.GetPartitions()
    
    // For each partition, identify all peers
    for _, partition := range partitions {
        for peerID, info := range h.net.Peers[strings.ToLower(partition)] {
            // Create multiaddr for this peer
            addr := multiaddr.StringCast("/p2p/" + peerID.String())
            if len(info.Addresses) > 0 {
                addr = info.Addresses[0].Encapsulate(addr)
            }
            
            // Add to general peers list
            pInfo := peerInfo{
                peerID: peerID.String(),
                multiaddr: addr,
            }
            pm.generalPeers[partition] = append(pm.generalPeers[partition], pInfo)
            
            // If this is a validator, also add to validator peers list
            if info.IsValidator {
                vInfo := validatorInfo{
                    peerID: peerID.String(),
                    multiaddr: addr,
                    publicKey: info.Key,
                }
                pm.validatorPeers[partition] = append(pm.validatorPeers[partition], vInfo)
            }
        }
    }
    
    return nil
}
```

### 2. Chain Height Collection

```go
// collectChainHeights gets chain heights for all partition pairs
func (h *healer) collectChainHeights(ctx context.Context) (map[string]map[string]uint64, error) {
    heights := make(map[string]map[string]uint64)
    
    // Find a working peer for queries
    var queryPeer peerInfo
    for _, peers := range h.peerManager.generalPeers {
        if len(peers) > 0 {
            queryPeer = peers[0]
            break
        }
    }
    
    if queryPeer.peerID == "" {
        return nil, errors.New("no peers available for queries")
    }
    
    // Use this peer to query chain heights
    client := h.C2.ForAddress(queryPeer.multiaddr)
    
    // For each source partition
    for src, _ := range h.peerManager.generalPeers {
        heights[src] = make(map[string]uint64)
        
        // For each destination partition
        for dst, _ := range h.peerManager.generalPeers {
            // Skip self-anchors for non-DN partitions
            if src == dst && !strings.EqualFold(src, protocol.Directory) {
                continue
            }
            
            // Query chain height
            srcUrl := protocol.PartitionUrl(src)
            dstUrl := protocol.PartitionUrl(dst)
            
            // Use consistent URL construction (raw partition URLs)
            res, err := client.QueryChain(ctx, srcUrl.JoinPath("anchor"), &api.ChainQuery{})
            if err != nil {
                slog.ErrorContext(ctx, "Failed to query chain height", 
                    "source", src, "destination", dst, "error", err)
                continue
            }
            
            // Store the height
            heights[src][dst] = res.Chain.Height
        }
    }
    
    return heights, nil
}
```

### 3. Transaction Creation

```go
// createHealingTransaction creates a healing transaction for a gap
func (h *healer) createHealingTransaction(ctx context.Context, src, dst string, seqNum uint64) (*protocol.Transaction, error) {
    // Find a working peer
    var queryPeer peerInfo
    for _, peers := range h.peerManager.generalPeers {
        if len(peers) > 0 {
            queryPeer = peers[0]
            break
        }
    }
    
    if queryPeer.peerID == "" {
        return nil, errors.New("no peers available for transaction creation")
    }
    
    // Use this peer to create the transaction
    client := h.C2.ForAddress(queryPeer.multiaddr)
    
    // Use consistent URL construction (raw partition URLs)
    srcUrl := protocol.PartitionUrl(src)
    dstUrl := protocol.PartitionUrl(dst)
    
    // Create the transaction
    // (Implementation depends on whether it's an anchor or synthetic transaction)
    
    return txn, nil
}
```

### 4. Signature Collection

```go
// collectSignatures collects signatures from validators
func (h *healer) collectSignatures(ctx context.Context, txn *protocol.Transaction, src, dst string, seqNum uint64) ([]protocol.Signature, error) {
    // Get validators for the source partition
    validators := h.peerManager.validatorPeers[src]
    if len(validators) == 0 {
        return nil, fmt.Errorf("no validators found for partition %s", src)
    }
    
    // Calculate threshold
    g := &network.GlobalValues{
        Oracle:          h.net.Status.Oracle,
        Globals:         h.net.Status.Globals,
        Network:         h.net.Status.Network,
        Routing:         h.net.Status.Routing,
        ExecutorVersion: h.net.Status.ExecutorVersion,
    }
    threshold := g.ValidatorThreshold(src)
    
    // Track which validators have signed
    signed := map[[32]byte]bool{}
    var signatures []protocol.Signature
    
    // Query each validator for its signature
    srcUrl := protocol.PartitionUrl(src)
    dstUrl := protocol.PartitionUrl(dst)
    
    for _, validator := range validators {
        // Skip if we already have enough signatures
        if len(signed) >= int(threshold) {
            break
        }
        
        // Skip if this validator has already signed
        if signed[validator.publicKey] {
            continue
        }
        
        // Set a timeout for the query
        queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
        defer cancel()
        
        // Use the Private API with the specific validator multiaddress
        // This is critical - we must use the Private API directly with each validator
        slog.InfoContext(ctx, "Querying validator for signature", "validator", validator.peerID)
        res, err := h.C2.ForAddress(validator.multiaddr).Private().Sequence(
            queryCtx, 
            srcUrl.JoinPath(protocol.AnchorPool), 
            dstUrl, 
            seqNum, 
            private.SequenceOptions{},
        )
        
        if err != nil {
            slog.ErrorContext(ctx, "Failed to get signature", "validator", validator.peerID, "error", err)
            continue
        }
        
        // Process and verify the signature
        // (Implementation depends on the executor version)
        
        // Add to signatures list
        signatures = append(signatures, sig)
        signed[validator.publicKey] = true
    }
    
    // Check if we have enough signatures
    if len(signed) < int(threshold) {
        return signatures, fmt.Errorf("insufficient signatures: have %d, need %d", len(signed), threshold)
    }
    
    return signatures, nil
}
```

### 5. Transaction Submission

```go
// submitTransaction submits a fully signed transaction
func (h *healer) submitTransaction(ctx context.Context, txn *protocol.Transaction, signatures []protocol.Signature) error {
    // Find a working peer for submission
    var submitPeer peerInfo
    for _, peers := range h.peerManager.generalPeers {
        if len(peers) > 0 {
            submitPeer = peers[0]
            break
        }
    }
    
    if submitPeer.peerID == "" {
        return errors.New("no peers available for submission")
    }
    
    // Create the submission message
    var messages []messaging.Message
    
    if h.net.Status.ExecutorVersion.V2Enabled() {
        // For V2 networks, submit each signature as a BlockAnchor
        for _, sig := range signatures {
            blk := &messaging.BlockAnchor{
                Signature: sig.(protocol.KeySignature),
                Anchor:    seq,
            }
            messages = append(messages, blk)
        }
    } else {
        // For V1 networks, submit the transaction and signatures together
        messages = append(messages, &messaging.TransactionMessage{Transaction: txn})
        for _, sig := range signatures {
            messages = append(messages, &messaging.SignatureMessage{Signature: sig})
        }
    }
    
    // Submit the transaction
    client := h.C2.ForAddress(submitPeer.multiaddr)
    
    // Set a timeout for submission
    submitCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
    defer cancel()
    
    // Submit the transaction
    _, err := client.Submit(submitCtx, messages...)
    if err != nil {
        return fmt.Errorf("failed to submit transaction: %w", err)
    }
    
    return nil
}
```
    
    // Try each cached peer in order
    var lastErr error
    for _, peerID := range peers {
        // Create a client for this peer
        info := q.h.net.Peers[strings.ToLower(part)][peerID]
        c := q.h.C2.ForPeer(peerID)
        if len(info.Addresses) > 0 {
            c = c.ForAddress(info.Addresses[0])
        }
        
        // Set a timeout for the query
        queryCtx, cancel := context.WithTimeout(ctx, q.timeout)
        defer cancel()
        
        // Execute the query
        r, err := c.Query(queryCtx, scope, query)
        
        if err != nil {
            // Handle client errors (return immediately)
            if errors.Code(err).IsClientError() {
                return nil, err
            }
            
            // Update the failure count for this peer
            q.cache.mu.Lock()
            q.cache.failureCount[peerID]++
            
            // If the peer has failed too many times, remove it from the working peers list
            if q.cache.failureCount[peerID] >= 3 {
                // Remove this peer from the working peers list
                for i, p := range q.cache.workingPeers[strings.ToLower(part)] {
                    if p == peerID {
                        q.cache.workingPeers[strings.ToLower(part)] = append(
                            q.cache.workingPeers[strings.ToLower(part)][:i],
                            q.cache.workingPeers[strings.ToLower(part)][i+1:]...
                        )
                        break
                    }
                }
                
                slog.InfoContext(ctx, "Removed peer from working peers due to consecutive failures", 
                    "partition", part, "peer", peerID, "failures", q.cache.failureCount[peerID])
            }
            q.cache.mu.Unlock()
            
            lastErr = err
            slog.ErrorContext(ctx, "Failed to query cached peer", 
                "peer", peerID, "scope", scope, "error", err)
            continue
        }
        
        // Query succeeded, update the peer's status
        q.cache.mu.Lock()
        q.cache.lastSuccess[peerID] = time.Now()
        q.cache.failureCount[peerID] = 0
        q.cache.mu.Unlock()
        
        // Validate the response
        r2, ok := r.(api.WithLastBlockTime)
        if !ok {
            return r, nil
        }
        
        if r2.GetLastBlockTime() == nil {
            slog.WarnContext(ctx, "Response does not include a last block time", 
                "scope", scope)
            continue
        }
        
        age := time.Since(*r2.GetLastBlockTime())
        if flagMaxResponseAge > 0 && age > flagMaxResponseAge {
            slog.WarnContext(ctx, "Response is too old", 
                "scope", scope, "age", age)
            continue
        }
        
        return r, nil
    }
    
    // If we've tried all cached peers and none worked, try to discover new peers
    slog.InfoContext(ctx, "All cached peers failed, discovering new peers", 
        "partition", part)
    
    // Clear the working peers for this partition
    q.cache.mu.Lock()
    q.cache.workingPeers[strings.ToLower(part)] = nil
    q.cache.mu.Unlock()
    
    // Try to discover new working peers
    discoveryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()
    
    err = q.cache.discoverWorkingPeers(discoveryCtx, q.h)
    if err != nil {
        if lastErr != nil {
            return nil, fmt.Errorf("all peers failed and couldn't discover new peers: %w", lastErr)
        }
        return nil, fmt.Errorf("all peers failed and couldn't discover new peers")
    }
    
    // Retry the query with the newly discovered peers
    return q.Query(ctx, scope, query)
}
```

### 5. Modify the Healer to Use the Cached Querier

```go
// Initialize the healer with a peer cache
func initHealer(h *healer) {
    // Create a new peer cache
    cache := newPeerCache()
    
    // Discover working peers upfront
    ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
    defer cancel()
    
    err := cache.discoverWorkingPeers(ctx, h)
    if err != nil {
        slog.WarnContext(ctx, "Failed to discover working peers upfront, will discover as needed", 
            "error", err)
    }
    
    // Store the cache in the healer
    h.peerCache = cache
}

// Modify the tryEach method to use the cached querier
func (h *healer) tryEach() api.Querier2 {
    return api.Querier2{Querier: &cachedQuerier{h, h.peerCache, 10 * time.Second}}
}
```

## Healing Workflow

The healing process follows a systematic approach to ensure all partitions are properly synchronized:

```go
// healNetwork implements the main healing workflow
func (h *healer) healNetwork(ctx context.Context) error {
    for {
        // Get all partitions
        partitions := h.net.GetPartitions()
        
        // Track whether we've healed something in this cycle
        healed := false
        
        // Process each partition
        for _, partition := range partitions {
            // Skip if we've already healed something in this cycle
            // This prevents overloading the network with too many healing operations
            if healed {
                break
            }
            
            // 1. Test if anchors need syncing
            anchorGaps, err := h.findAnchorGaps(ctx, partition)
            if err != nil {
                slog.ErrorContext(ctx, "Failed to find anchor gaps", "partition", partition, "error", err)
                continue
            }
            
            if len(anchorGaps) > 0 {
                // 1.1 Process one anchor gap
                err = h.processAnchorGaps(ctx, partition, anchorGaps)
                if err != nil {
                    slog.ErrorContext(ctx, "Failed to process anchor gap", "partition", partition, "error", err)
                } else {
                    // Mark that we've healed something in this cycle
                    healed = true
                    // Skip checking synthetic transactions for this partition
                    continue
                }
            }
            
            // Only check synthetic transactions if we haven't healed an anchor
            if !healed {
                // 1.2 Test if synthetic transactions need syncing
                synthGaps, err := h.findSyntheticGaps(ctx, partition)
                if err != nil {
                    slog.ErrorContext(ctx, "Failed to find synthetic gaps", "partition", partition, "error", err)
                    continue
                }
                
                if len(synthGaps) > 0 {
                    // Process one synthetic gap
                    err = h.processSyntheticGaps(ctx, partition, synthGaps)
                    if err != nil {
                        slog.ErrorContext(ctx, "Failed to process synthetic gap", "partition", partition, "error", err)
                    } else {
                        // Mark that we've healed something in this cycle
                        healed = true
                    }
                }
            }
        }
        
        // Pause before starting the next cycle
        // Use a longer pause if we healed something to give the network time to process
        if healed {
            slog.InfoContext(ctx, "Completed healing operation, pausing before next cycle")
            time.Sleep(5 * time.Second) // Longer pause after healing
        } else {
            slog.InfoContext(ctx, "No healing needed, pausing before next cycle")
            time.Sleep(1 * time.Second) // Shorter pause if nothing was healed
        }
    }
}

// processAnchorGaps processes one gap in anchor sequences
func (h *healer) processAnchorGaps(ctx context.Context, partition string, gaps []sequenceInfo) error {
    // Sort gaps by sequence number (prioritize lowest height)
    sort.Slice(gaps, func(i, j int) bool {
        return gaps[i].Number < gaps[j].Number
    })
    
    // Process only the lowest gap
    if len(gaps) > 0 {
        gap := gaps[0]
        slog.InfoContext(ctx, "Processing single anchor gap", "source", gap.Source, "destination", gap.Destination, "sequence", gap.Number)
        
        // 1.1.1 Attempt to pull the anchor with the lowest height
        // 1.1.2 Build a healing tx for that anchor
        txn, err := h.createHealingTransaction(ctx, gap.Source, gap.Destination, gap.Number)
        if err != nil {
            return fmt.Errorf("failed to create healing transaction: %w", err)
        }
        
        // 1.1.3 Gather signatures with up to 3 attempts
        var signatures []protocol.Signature
        var sigErr error
        
        for attempt := 1; attempt <= 3; attempt++ {
            slog.InfoContext(ctx, "Collecting signatures", "attempt", attempt)
            signatures, sigErr = h.collectSignatures(ctx, txn, gap.Source, gap.Destination, gap.Number)
            
            // If we got enough signatures or encountered a non-timeout error, break
            if sigErr == nil || !errors.Is(sigErr, context.DeadlineExceeded) {
                break
            }
            
            slog.WarnContext(ctx, "Signature collection timed out, retrying", "attempt", attempt)
        }
        
        if sigErr != nil {
            return fmt.Errorf("failed to collect signatures after 3 attempts: %w", sigErr)
        }
        
        // 1.1.5 Submit the healing transaction
        err = h.submitTransaction(ctx, txn, signatures)
        if err != nil {
            return fmt.Errorf("failed to submit transaction: %w", err)
        }
        
        slog.InfoContext(ctx, "Successfully submitted healing transaction", 
            "source", gap.Source, "destination", gap.Destination, "sequence", gap.Number)
        
        // Return success after processing just one gap
        return nil
    }
    
    return nil
}

// processSyntheticGaps processes one gap in synthetic sequences
func (h *healer) processSyntheticGaps(ctx context.Context, partition string, gaps []sequenceInfo) error {
    // Sort gaps by sequence number (prioritize lowest height)
    sort.Slice(gaps, func(i, j int) bool {
        return gaps[i].Number < gaps[j].Number
    })
    
    // Process only the lowest gap
    if len(gaps) > 0 {
        gap := gaps[0]
        slog.InfoContext(ctx, "Processing single synthetic gap", "source", gap.Source, "destination", gap.Destination, "sequence", gap.Number)
        
        // 1.2.1 Process synthetic gaps the same way as anchors
        // Implementation similar to processAnchorGaps but for synthetic transactions
        // ...
        
        // Return success after processing just one gap
        return nil
    }
    
    return nil
}
```

### Healing Workflow Summary

1. **For each partition (until one healing is done):**
   1.1. **Test if anchors need syncing**
      - If gaps exist, proceed to heal ONE anchor (the lowest height)
      - 1.1.1. Attempt to pull the anchor with the lowest height
      - 1.1.2. Build a healing transaction for that anchor
      - 1.1.3. Gather signatures (with up to 3 retry attempts)
      - 1.1.5. If sufficient signatures are collected, submit the healing transaction
      - **Stop processing after healing one anchor**
   
   1.2. **Test if synthetic transactions need syncing (only if no anchor was healed)**
      - If gaps exist, proceed to heal ONE synthetic transaction (the lowest height)
      - 1.2.1. Follow the same process as with anchors
      - **Stop processing after healing one synthetic transaction**
   
   1.3. **Move to the next partition (only if nothing was healed yet)**
   
2. **After processing (or once one healing is done):**
   - 1.4. Pause for 5 seconds if healing was done, 1 second otherwise
   - 1.5. Reset and restart the partition loop

### Anti-DOS Measures

This approach implements several measures to prevent overloading the network:

1. **One Healing Per Cycle**: Only one transaction (anchor or synthetic) is healed per cycle
2. **Prioritization**: Anchors are prioritized over synthetic transactions
3. **Lowest Height First**: Within each type, the lowest height is processed first
4. **Adaptive Pausing**: Longer pauses after healing operations
5. **Early Exit**: Processing stops as soon as one healing is completed

## Integration Plan

1. **Add the Peer Cache to the Healer Struct**:
   ```go
   type healer struct {
       // Existing fields...
       peerCache *peerCache
   }
   ```

2. **Initialize the Peer Cache**:
   - Add initialization code to the `heal` method in `heal_common.go`
   - Discover working peers upfront before starting the healing process

3. **Replace the tryEachQuerier with cachedQuerier**:
   - Modify the `tryEach` method to return a `cachedQuerier` instead of a `tryEachQuerier`
   - Ensure all existing code that uses `tryEach` continues to work with the new implementation

4. **Add Metrics and Logging**:
   - Track and log peer discovery statistics
   - Monitor cache hit rates and peer reliability

## Expected Benefits

1. **Reduced Query Latency**:
   - Most queries will use a known working peer immediately
   - No need to try multiple peers for every query

2. **More Efficient Peer Discovery**:
   - Only search for new peers when necessary
   - Prioritize peers that have been responsive in the past

3. **Better Error Handling**:
   - Track peer reliability over time
   - Quickly adapt to changing network conditions

4. **Improved User Experience**:
   - Faster execution of healing commands
   - More consistent performance

## Testing Strategy

1. **Unit Tests**:
   - Test the peer cache data structure
   - Test the peer discovery logic
   - Test the cached querier implementation

2. **Integration Tests**:
   - Test the integration with the existing healing tools
   - Verify that all healing commands work with the new peer caching mechanism

3. **Performance Tests**:
   - Compare query latency with and without peer caching
   - Measure the impact on overall healing time

## Implementation Timeline

1. **Phase 1 (1-2 days)**:
   - Implement the peer cache data structure
   - Add the peer discovery logic

2. **Phase 2 (1-2 days)**:
   - Implement the cached querier
   - Integrate with the existing healing tools

3. **Phase 3 (1 day)**:
   - Add metrics and logging
   - Optimize based on real-world usage

4. **Phase 4 (1 day)**:
   - Write tests
   - Document the implementation

## Conclusion

This peer caching mechanism will significantly improve the performance of the healing tools by reducing the time spent on peer discovery. By remembering which peers are responsive and only searching for new peers when necessary, we can make the healing process much faster and more reliable.

## Implementation Status

### Completed Features

1. **Basic Peer Caching Mechanism**:
   - Implemented the peer cache data structure to store responsive peers for each partition
   - Added methods to discover working peers upfront and update the cache based on query results
   - Integrated the peer cache with the healer structure

2. **Enhanced Yutu Peer Discovery**:
   - Added special handling for the Yutu partition, which often has fewer available peers
   - Implemented a fallback mechanism to query the network directory for Yutu peers when none are found through standard discovery
   - Added code to use existing network status information instead of making additional queries
   - Implemented a secondary fallback to use peers from other partitions when no Yutu peers are available

### Implementation Details for Yutu Peer Discovery

```go
// Special handling for Yutu partition when no peers are found
if partition == "Yutu" && len(pc.workingPeers["yutu"]) == 0 {
    slog.InfoContext(ctx, "No working Yutu peers found, attempting to discover Yutu peers from network directory")
    
    // Try to find a directory peer to query for Yutu peers
    dirPeers := pc.workingPeers["directory"]
    if len(dirPeers) > 0 {
        // Use the first directory peer to query for Yutu peers
        dirPeerID := dirPeers[0]
        
        // We don't need to create a client or set a timeout since we're using the cached network status
        
        // Query the network status to get Yutu peers
        slog.InfoContext(ctx, "Querying directory peer for Yutu peers", "peer", dirPeerID)
        
        // Use the network status from the healer since we already have it
        // This avoids making an additional query
        if h.net != nil && h.net.Peers != nil && h.net.Peers["yutu"] != nil {
            slog.InfoContext(ctx, "Using existing network status for Yutu peers")
            
            // Look for Yutu peers in the network status
            for peerID, info := range h.net.Peers["yutu"] {
                // Try to connect to this Yutu peer
                yutuClient := h.C2.ForPeer(peerID)
                if len(info.Addresses) > 0 {
                    yutuClient = yutuClient.ForAddress(info.Addresses[0])
                }
                
                // Test if this peer is responsive
                testCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
                defer cancel()
                
                _, err := yutuClient.QueryConsensusStatus(testCtx, nil, &api.ConsensusStatusQuery{
                    Partition: "Yutu",
                })
                
                if err == nil {
                    // Found a working Yutu peer, add it to the cache
                    pc.mu.Lock()
                    pc.workingPeers["yutu"] = append(pc.workingPeers["yutu"], peerID)
                    pc.lastSuccess[peerID] = time.Now()
                    pc.failureCount[peerID] = 0
                    pc.mu.Unlock()
                    
                    slog.InfoContext(ctx, "Found working Yutu peer from network directory", "peer", peerID)
                    break
                }
            }
        }
    }
    
    // If we still couldn't find any Yutu peers, fall back to using peers from other partitions
    pc.mu.RLock()
    if len(pc.workingPeers["yutu"]) == 0 {
        pc.mu.RUnlock()
        slog.InfoContext(ctx, "Could not find working Yutu peers, falling back to using peers from other partitions")
        
        // Try to use a peer from another partition
        for otherPartition, peers := range pc.workingPeers {
            if otherPartition != "yutu" && len(peers) > 0 {
                pc.mu.Lock()
                pc.workingPeers["yutu"] = append(pc.workingPeers["yutu"], peers[0])
                pc.mu.Unlock()
                
                slog.InfoContext(ctx, "Using peer from another partition for Yutu", 
                    "peer", peers[0], "source_partition", otherPartition)
                break
            }
        }
    } else {
        pc.mu.RUnlock()
    }
}
```

### Future Enhancements

1. **Chain Height Difference Reporting with URL Diagnostics**:
   - Add a reporting feature to show height differences between partition pairs
   - For each partition pair, report the source height and number of missing entries on the destination side
   - Include a URL diagnostics table that shows the exact URLs being used for each partition pair and chain
   - This will help identify URL construction issues that may lead to "element does not exist" errors

### URL Diagnostics Table Design

The URL diagnostics table provides transparency into how URLs are constructed for each partition pair, helping to identify inconsistencies or errors in URL construction. It's important to note that while we check multiple chain types for diagnostic purposes, we only heal synthetic transaction chains, not root chains.

#### Partition Pairs and Chain Types

Each partition pair (source → destination) should have only one entry in the URL diagnostics table. The table should clearly show:

1. The base URL construction for both source and destination partitions
2. The synthetic chains that are checked and potentially healed
3. Any other chains that are checked for diagnostic purposes but not healed

#### Implementation Structure

```go
// Structure to hold URL diagnostic information for a partition pair
type urlDiagnostic struct {
    srcPartition  string
    dstPartition  string
    baseUrl       struct {
        src string
        dst string
    }
    chains        map[string]struct {
        srcPath      string
        dstPath      string
        queryResult  string  // Success, Height Diff, Not Found, Error, etc.
        isHealed     bool    // Whether this chain type is healed
    }
}

// Collect URL diagnostics during chain height difference reporting
func collectUrlDiagnostics(h *healer) []urlDiagnostic {
    var diagnostics []urlDiagnostic
    
    // For each partition pair
    for _, src := range partitions {
        srcUrl := protocol.PartitionUrl(src)
        
        for _, dst := range partitions {
            if src == dst {
                continue
            }
            
            dstUrl := protocol.PartitionUrl(dst)
            
            // Create a single diagnostic entry for this partition pair
            diag := urlDiagnostic{
                srcPartition: src,
                dstPartition: dst,
                baseUrl: struct {
                    src string
                    dst string
                }{
                    src: srcUrl.String(),
                    dst: dstUrl.String(),
                },
                chains: make(map[string]struct {
                    srcPath      string
                    dstPath      string
                    queryResult  string
                    isHealed     bool
                }),
            }
            
            // Add main chain (not healed, diagnostic only)
            diag.chains["main"] = struct {
                srcPath      string
                dstPath      string
                queryResult  string
                isHealed     bool
            }{
                srcPath:     "",  // Base URL
                dstPath:     "",  // Base URL
                queryResult: "Pending",
                isHealed:    false,
            }
            
            // Add synthetic chains (these are healed)
            synthPath := protocol.Synthetic
            for _, seqName := range []string{"synthetic-sequence-0", "synthetic-sequence-1", "synthetic-sequence-index"} {
                diag.chains[seqName] = struct {
                    srcPath      string
                    dstPath      string
                    queryResult  string
                    isHealed     bool
                }{
                    srcPath:     synthPath,
                    dstPath:     synthPath,
                    queryResult: "Pending",
                    isHealed:    true,
                }
            }
            
            // Add root chain (not healed, diagnostic only)
            diag.chains["root"] = struct {
                srcPath      string
                dstPath      string
                queryResult  string
                isHealed     bool
            }{
                srcPath:     protocol.Ledger,
                dstPath:     protocol.Ledger,
                queryResult: "Pending",
                isHealed:    false,
            }
            
            diagnostics = append(diagnostics, diag)
        }
    }
    
    return diagnostics
}

// Print the URL diagnostics report
func printUrlDiagnosticsReport(diagnostics []urlDiagnostic) {
    fmt.Printf("\n============= URL DIAGNOSTICS REPORT =============\n")
    fmt.Printf("%-15s %-15s %-20s %-50s %-50s %-15s %-10s\n", 
        "Source", "Destination", "Chain", "Source URL", "Destination URL", "Result", "Healed")
    fmt.Printf("%s\n", strings.Repeat("-", 175))
    
    for _, diag := range diagnostics {
        // Print base URLs first
        fmt.Printf("%-15s %-15s %-20s %-50s %-50s %-15s %-10s\n",
            diag.srcPartition,
            diag.dstPartition,
            "BASE",
            diag.baseUrl.src,
            diag.baseUrl.dst,
            "N/A",
            "N/A")
            
        // Then print each chain
        for chainName, chain := range diag.chains {
            srcUrl := diag.baseUrl.src
            dstUrl := diag.baseUrl.dst
            
            if chain.srcPath != "" {
                srcUrl = fmt.Sprintf("%s/%s", srcUrl, chain.srcPath)
            }
            
            if chain.dstPath != "" {
                dstUrl = fmt.Sprintf("%s/%s", dstUrl, chain.dstPath)
            }
            
            healedStr := "No"
            if chain.isHealed {
                healedStr = "Yes"
            }
            
            fmt.Printf("%-15s %-15s %-20s %-50s %-50s %-15s %-10s\n",
                "",  // No need to repeat partition names
                "",
                chainName,
                srcUrl,
                dstUrl,
                chain.queryResult,
                healedStr)
        }
        
        // Add a separator between partition pairs
        fmt.Printf("%s\n", strings.Repeat("-", 175))
    }
    
    fmt.Printf("=============================================================\n\n")
}
```

This enhancement will be particularly valuable for identifying issues related to the URL construction differences noted in the development plan, where different parts of the code may use different approaches to URL construction.

2. **Peer Performance Metrics**:
   - Track response times for each peer to prioritize faster peers
   - Implement more sophisticated peer ranking based on reliability and performance

3. **Adaptive Timeouts**:
   - Adjust query timeouts based on historical response times for each peer
   - Reduce timeouts for consistently slow peers to improve overall performance
