## Proposed Changes

### 1. Create New Light Client Methods

#### 1.1. `PullAccountOnly`

Create a new method to pull only account data without any chains:

```go
func (c *Client) PullAccountOnly(ctx context.Context, acctUrl *url.URL) error {
    batch := c.OpenDB(true)
    defer batch.Discard()

    // Update the account
    r1, err := c.query.QueryAccount(ctx, acctUrl, nil)
    if err != nil {
        return errors.UnknownError.WithFormat("query account: %w", err)
    }

    err = batch.Account(acctUrl).Main().Put(r1.Account)
    if err != nil {
        return errors.UnknownError.WithFormat("store account: %w", err)
    }

    return batch.Commit()
}
```

#### 1.2. `PullSpecificChains`

Create a new method to pull specific chains using the differential approach:

```go
func (c *Client) PullSpecificChains(ctx context.Context, acctUrl *url.URL, chainNames []string) error {
    for _, chainName := range chainNames {
        err := c.PullDifferentialChain(ctx, acctUrl, acctUrl, chainName, nil)
        if err != nil {
            return err
        }
    }
    return nil
}
```

### 2. Update `pullSynthDirChains`

Replace all instances of `PullAccountWithChains` with differential approaches:

```go
func pullSynthDirChains(h *healer) {
    ctx, cancel, _ := api.ContextWithBatchData(h.ctx)
    defer cancel()
    
    startTime := time.Now()
    slog.InfoContext(ctx, "Starting pullSynthDirChains")
    
    // For directory network chains, use differential approach instead of full pull
    slog.InfoContext(ctx, "Pulling directory network chains differentially")
    
    // Pull account data only
    check(h.light.PullAccountOnly(ctx, protocol.DnUrl().JoinPath(protocol.Network)))
    
    // Pull specific chains differentially
    check(h.light.PullSpecificChains(ctx, protocol.DnUrl().JoinPath(protocol.Network), 
        []string{"main", "main-index", "signature", "signature-index"}))
    
    check(h.light.IndexAccountChains(ctx, protocol.DnUrl().JoinPath(protocol.Network)))

    // For directory ledger chains, use differential approach
    // Get the BVN partitions directly from the network status
    if h.net == nil || h.net.Status == nil || h.net.Status.Network == nil {
        slog.ErrorContext(ctx, "Network status is not available, cannot use differential approach")
        // Even in fallback, use differential approach with self as destination
        slog.WarnContext(ctx, "Using differential approach for directory ledger chains with self as destination")
        check(pullDifferentialChains(h, ctx, 
            protocol.DnUrl().JoinPath(protocol.Ledger), 
            protocol.DnUrl().JoinPath(protocol.Ledger), 
            "root", includeRootChain))
        check(pullDifferentialChains(h, ctx, 
            protocol.DnUrl().JoinPath(protocol.Ledger), 
            protocol.DnUrl().JoinPath(protocol.Ledger), 
            "root-index", includeRootChain))
        check(h.light.IndexAccountChains(ctx, protocol.DnUrl().JoinPath(protocol.Ledger)))
        return
    }
    
    // Access partitions directly from the network status
    partitions := h.net.Status.Network.Partitions
    slog.InfoContext(ctx, "Retrieved partitions from network status", "count", len(partitions))
    
    // Find a BVN partition to use as destination
    var bvnPart *protocol.PartitionInfo
    for _, part := range partitions {
        if part.Type == protocol.PartitionTypeBlockValidator {
            bvnPart = part
            slog.InfoContext(ctx, "Found BVN partition to use as destination", "partition", part.ID)
            break
        }
    }
    
    if bvnPart != nil {
        // Use differential approach for ledger root chain
        slog.InfoContext(ctx, "Using differential approach for directory ledger chains", 
            "source", protocol.DnUrl().JoinPath(protocol.Ledger),
            "destination", protocol.PartitionUrl(bvnPart.ID).JoinPath(protocol.Ledger))
        
        // Use differential approach for ledger root chain
        check(pullDifferentialChains(h, ctx, 
            protocol.DnUrl().JoinPath(protocol.Ledger), 
            protocol.PartitionUrl(bvnPart.ID).JoinPath(protocol.Ledger), 
            "root", includeRootChain))
        
        // Use differential approach for ledger root-index chain
        check(pullDifferentialChains(h, ctx, 
            protocol.DnUrl().JoinPath(protocol.Ledger), 
            protocol.PartitionUrl(bvnPart.ID).JoinPath(protocol.Ledger), 
            "root-index", includeRootChain))
    } else {
        // Even in fallback, use differential approach with self as destination
        slog.WarnContext(ctx, "No BVN partition found, using differential approach with self as destination")
        check(pullDifferentialChains(h, ctx, 
            protocol.DnUrl().JoinPath(protocol.Ledger), 
            protocol.DnUrl().JoinPath(protocol.Ledger), 
            "root", includeRootChain))
        check(pullDifferentialChains(h, ctx, 
            protocol.DnUrl().JoinPath(protocol.Ledger), 
            protocol.DnUrl().JoinPath(protocol.Ledger), 
            "root-index", includeRootChain))
    }
    
    check(h.light.IndexAccountChains(ctx, protocol.DnUrl().JoinPath(protocol.Ledger)))

    // For directory anchor pool chains, use differential approach in all cases
    dnAnchors := protocol.DnUrl().JoinPath(protocol.AnchorPool)
    
    if bvnPart != nil {
        slog.InfoContext(ctx, "Using differential approach for directory anchor pool chains",
            "source", dnAnchors,
            "destination", protocol.PartitionUrl(bvnPart.ID).JoinPath(protocol.AnchorPool))
        
        bvnAnchors := protocol.PartitionUrl(bvnPart.ID).JoinPath(protocol.AnchorPool)
        
        // Get anchor pool chains
        anchorChains, err := h.tryEach().QueryAccountChains(ctx, dnAnchors, &api.ChainQuery{})
        if err != nil {
            slog.WarnContext(ctx, "Failed to query anchor pool chains, using differential approach with self as destination", "error", err)
            // Pull account data only
            check(h.light.PullAccountOnly(ctx, dnAnchors))
            
            // Use self as destination for differential pull
            check(pullDifferentialChains(h, ctx, dnAnchors, dnAnchors, "main", nil))
            check(pullDifferentialChains(h, ctx, dnAnchors, dnAnchors, "main-index", nil))
        } else {
            // Pull each anchor chain differentially
            slog.InfoContext(ctx, "Found anchor chains to pull differentially", "count", len(anchorChains.Records))
            for _, chain := range anchorChains.Records {
                if chain.Type == merkle.ChainTypeAnchor || (chain.IndexOf != nil && chain.IndexOf.Type == merkle.ChainTypeAnchor) {
                    check(pullDifferentialChains(h, ctx, dnAnchors, bvnAnchors, chain.Name, nil))
                }
            }
        }
    } else {
        // Even in fallback, use differential approach with self as destination
        slog.WarnContext(ctx, "No BVN partition found, using differential approach with self as destination")
        
        // Pull account data only
        check(h.light.PullAccountOnly(ctx, dnAnchors))
        
        // Get anchor pool chains
        anchorChains, err := h.tryEach().QueryAccountChains(ctx, dnAnchors, &api.ChainQuery{})
        if err != nil {
            slog.WarnContext(ctx, "Failed to query anchor pool chains", "error", err)
            // Use self as destination for differential pull of main chains
            check(pullDifferentialChains(h, ctx, dnAnchors, dnAnchors, "main", nil))
            check(pullDifferentialChains(h, ctx, dnAnchors, dnAnchors, "main-index", nil))
        } else {
            // Pull each anchor chain differentially with self as destination
            for _, chain := range anchorChains.Records {
                if chain.Type == merkle.ChainTypeAnchor || (chain.IndexOf != nil && chain.IndexOf.Type == merkle.ChainTypeAnchor) {
                    check(pullDifferentialChains(h, ctx, dnAnchors, dnAnchors, chain.Name, nil))
                }
            }
        }
    }
    
    check(h.light.IndexAccountChains(ctx, protocol.DnUrl().JoinPath(protocol.AnchorPool)))
    
    duration := time.Since(startTime)
    slog.InfoContext(ctx, "Completed pullSynthDirChains", "duration", duration)
}

### 3. Update `pullSynthLedger`

Replace `PullAccountWithChains` with `PullAccountOnly`:

```go
func pullSynthLedger(h *healer, part *url.URL) *protocol.SyntheticLedger {
    // Pull account data only
    check(h.light.PullAccountOnly(h.ctx, part.JoinPath(protocol.Synthetic)))

    batch := h.light.OpenDB(false)
    defer batch.Discard()

    var ledger *protocol.SyntheticLedger
    check(batch.Account(part.JoinPath(protocol.Synthetic)).Main().GetAs(&ledger))
    return ledger
}
```

### 4. Update `pullDifferentialChains`

Modify the fallback cases to use differential approach even when destination chain doesn't exist:

```go
func pullDifferentialChains(h *healer, ctx context.Context, srcUrl, dstUrl *url.URL, chainName string, predicate func(*api.ChainRecord) bool) error {
    // ... existing code ...
    
    // Get destination chain info
    dstChainInfo, err := h.tryEach().QueryAccountChains(ctx, dstUrl, &api.ChainQuery{Name: chainName})
    if err != nil {
        // If the destination chain doesn't exist, use differential approach with empty destination
        if errors.Is(err, errors.NotFound) {
            slog.InfoContext(ctx, "Destination chain not found, using differential approach with empty destination", 
                "source", srcUrl, "destination", dstUrl, "chain", chainName, "count", srcChain.Count)
            
            // Create an empty chain in the destination
            batch := h.light.OpenDB(true)
            defer batch.Discard()
            
            // Pull entries directly using the differential approach
            start := uint64(0) // Start from the beginning
            total := srcChain.Count
            
            // ... rest of the differential pull logic ...
            
            return batch.Commit()
        }
        return errors.UnknownError.WithFormat("query destination chain info: %w", err)
    }
    
    // ... rest of the existing code ...
}
```

## Benefits of Complete Elimination

1. **Consistent Performance**: By eliminating all instances of full chain pulls, the healing process will have consistent performance characteristics.

2. **Reduced Network Load**: The differential approach significantly reduces the amount of data transferred over the network.

3. **Faster Healing**: The healing process will complete faster, especially for large chains.

4. **Simplified Code**: The code will be simpler and more maintainable without conditional fallbacks to full chain pulls.

## Implementation Strategy

1. **Phase 1**: Implement the new Light client methods (`PullAccountOnly` and `PullSpecificChains`)
2. **Phase 2**: Implement the account information collection system with caching
3. **Phase 3**: Update `pullSynthDirChains` to use differential approaches in all cases
4. **Phase 4**: Update `pullSynthLedger` to use `PullAccountOnly`
5. **Phase 5**: Update `pullDifferentialChains` to handle the case where destination chain doesn't exist
6. **Phase 6**: Implement comprehensive partition pair evaluation
7. **Phase 7**: Test and validate the changes

## Conclusion

By completely eliminating the full chain pull approach and replacing it with differential approaches in all cases, we can significantly improve the performance and efficiency of the healing process. This will result in faster healing, reduced network load, and more consistent performance. This approach makes targeted modifications to eliminate the full chain pull approach without disturbing the existing codebase structure.

## Program Flow for Processing Partition Pairs

The following details the complete program flow for processing all partition pairs in the differential sync approach:

### 1. Initialization Phase

```go
func healSynth(cmd *cobra.Command, args []string) {
    // Initialize the healer
    h := new(healer)
    h.ctx = context.Background()
    h.cache = make(map[string]interface{})
    
    // Initialize the light client
    var err error
    h.light, err = light.Connect(args[0], nil)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Failed to connect to database: %v\n", err)
        os.Exit(1)
    }
    
    // Initialize the network client
    h.net, err = api.NewNetworkClient(args[1:], api.ClientOptions{})
    if err != nil {
        fmt.Fprintf(os.Stderr, "Failed to connect to network: %v\n", err)
        os.Exit(1)
    }
    
    // Log initialization
    slog.InfoContext(h.ctx, "Initialized healer", 
        "database", args[0], 
        "network-endpoints", args[1:])
    
    // Start the healing process
    startTime := time.Now()
    h.heal()
    duration := time.Since(startTime)
    
    // Log completion
    slog.InfoContext(h.ctx, "Completed healing process", "duration", duration)
}
```

### 2. Main Healing Process

```go
func (h *healer) heal() {
    // Pull directory network chains
    slog.InfoContext(h.ctx, "Starting directory chain pull")
    pullSynthDirChains(h)
    
    // Evaluate all partition pairs
    slog.InfoContext(h.ctx, "Starting partition pair evaluation")
    evaluateAllPartitionPairs(h)
}
```

### 3. Directory Chain Pull Process

```go
func pullSynthDirChains(h *healer) {
    ctx, cancel, _ := api.ContextWithBatchData(h.ctx)
    defer cancel()
    
    startTime := time.Now()
    slog.InfoContext(ctx, "Starting pullSynthDirChains")
    
    // Step 1: Pull directory network chains
    slog.InfoContext(ctx, "Pulling directory network chains differentially")
    
    // Pull account data only
    check(h.light.PullAccountOnly(ctx, protocol.DnUrl().JoinPath(protocol.Network)))
    
    // Pull specific chains differentially
    check(h.light.PullSpecificChains(ctx, protocol.DnUrl().JoinPath(protocol.Network), 
        []string{"main", "main-index", "signature", "signature-index"}))
    
    check(h.light.IndexAccountChains(ctx, protocol.DnUrl().JoinPath(protocol.Network)))

    // For directory ledger chains, use differential approach
    // Get the BVN partitions directly from the network status
    if h.net == nil || h.net.Status == nil || h.net.Status.Network == nil {
        slog.ErrorContext(ctx, "Network status is not available, cannot use differential approach")
        // Even in fallback, use differential approach with self as destination
        slog.WarnContext(ctx, "Using differential approach for directory ledger chains with self as destination")
        check(pullDifferentialChains(h, ctx, 
            protocol.DnUrl().JoinPath(protocol.Ledger), 
            protocol.DnUrl().JoinPath(protocol.Ledger), 
            "root", includeRootChain))
        check(pullDifferentialChains(h, ctx, 
            protocol.DnUrl().JoinPath(protocol.Ledger), 
            protocol.DnUrl().JoinPath(protocol.Ledger), 
            "root-index", includeRootChain))
        check(h.light.IndexAccountChains(ctx, protocol.DnUrl().JoinPath(protocol.Ledger)))
        return
    }
    
    // Access partitions directly from the network status
    partitions := h.net.Status.Network.Partitions
    slog.InfoContext(ctx, "Retrieved partitions from network status", "count", len(partitions))
    
    // Find a BVN partition to use as destination
    var bvnPart *protocol.PartitionInfo
    for _, part := range partitions {
        if part.Type == protocol.PartitionTypeBlockValidator {
            bvnPart = part
            slog.InfoContext(ctx, "Found BVN partition to use as destination", "partition", part.ID)
            break
        }
    }
    
    // Step 3: Pull directory ledger chains with appropriate destination
    if bvnPart != nil {
        // Use BVN as destination
        slog.InfoContext(ctx, "Using differential approach for directory ledger chains", 
            "source", protocol.DnUrl().JoinPath(protocol.Ledger),
            "destination", protocol.PartitionUrl(bvnPart.ID).JoinPath(protocol.Ledger))
        
        // Use differential approach for ledger root chain
        check(pullDifferentialChains(h, ctx, 
            protocol.DnUrl().JoinPath(protocol.Ledger), 
            protocol.PartitionUrl(bvnPart.ID).JoinPath(protocol.Ledger), 
            "root", includeRootChain))
        
        // Use differential approach for ledger root-index chain
        check(pullDifferentialChains(h, ctx, 
            protocol.DnUrl().JoinPath(protocol.Ledger), 
            protocol.PartitionUrl(bvnPart.ID).JoinPath(protocol.Ledger), 
            "root-index", includeRootChain))
    } else {
        // Even in fallback, use differential approach with self as destination
        slog.WarnContext(ctx, "No BVN partition found, using differential approach with self as destination")
        check(pullDifferentialChains(h, ctx, 
            protocol.DnUrl().JoinPath(protocol.Ledger), 
            protocol.DnUrl().JoinPath(protocol.Ledger), 
            "root", includeRootChain))
        check(pullDifferentialChains(h, ctx, 
            protocol.DnUrl().JoinPath(protocol.Ledger), 
            protocol.DnUrl().JoinPath(protocol.Ledger), 
            "root-index", includeRootChain))
    }
    
    check(h.light.IndexAccountChains(ctx, protocol.DnUrl().JoinPath(protocol.Ledger)))

    // For directory anchor pool chains, use differential approach in all cases
    dnAnchors := protocol.DnUrl().JoinPath(protocol.AnchorPool)
    
    if bvnPart != nil {
        slog.InfoContext(ctx, "Using differential approach for directory anchor pool chains",
            "source", dnAnchors,
            "destination", protocol.PartitionUrl(bvnPart.ID).JoinPath(protocol.AnchorPool))
        
        bvnAnchors := protocol.PartitionUrl(bvnPart.ID).JoinPath(protocol.AnchorPool)
        
        // Get anchor pool chains
        anchorChains, err := h.tryEach().QueryAccountChains(ctx, dnAnchors, &api.ChainQuery{})
        if err != nil {
            slog.WarnContext(ctx, "Failed to query anchor pool chains, using differential approach with self as destination", "error", err)
            // Pull account data only
            check(h.light.PullAccountOnly(ctx, dnAnchors))
            
            // Use self as destination for differential pull
            check(pullDifferentialChains(h, ctx, dnAnchors, dnAnchors, "main", nil))
            check(pullDifferentialChains(h, ctx, dnAnchors, dnAnchors, "main-index", nil))
        } else {
            // Pull each anchor chain differentially
            slog.InfoContext(ctx, "Found anchor chains to pull differentially", "count", len(anchorChains.Records))
            for _, chain := range anchorChains.Records {
                if chain.Type == merkle.ChainTypeAnchor || (chain.IndexOf != nil && chain.IndexOf.Type == merkle.ChainTypeAnchor) {
                    check(pullDifferentialChains(h, ctx, dnAnchors, bvnAnchors, chain.Name, nil))
                }
            }
        }
    } else {
        // Even in fallback, use differential approach with self as destination
        slog.WarnContext(ctx, "No BVN partition found, using differential approach with self as destination")
        
        // Pull account data only
        check(h.light.PullAccountOnly(ctx, dnAnchors))
        
        // Get anchor pool chains
        anchorChains, err := h.tryEach().QueryAccountChains(ctx, dnAnchors, &api.ChainQuery{})
        if err != nil {
            slog.WarnContext(ctx, "Failed to query anchor pool chains", "error", err)
            // Use self as destination for differential pull of main chains
            check(pullDifferentialChains(h, ctx, dnAnchors, dnAnchors, "main", nil))
            check(pullDifferentialChains(h, ctx, dnAnchors, dnAnchors, "main-index", nil))
        } else {
            // Pull each anchor chain differentially with self as destination
            for _, chain := range anchorChains.Records {
                if chain.Type == merkle.ChainTypeAnchor || (chain.IndexOf != nil && chain.IndexOf.Type == merkle.ChainTypeAnchor) {
                    check(pullDifferentialChains(h, ctx, dnAnchors, dnAnchors, chain.Name, nil))
                }
            }
        }
    }
    
    check(h.light.IndexAccountChains(ctx, protocol.DnUrl().JoinPath(protocol.AnchorPool)))
    
    duration := time.Since(startTime)
    slog.InfoContext(ctx, "Completed pullSynthDirChains", "duration", duration)
}
```

### 4. Partition Pair Evaluation Process

```go
func evaluateAllPartitionPairs(h *healer) {
    ctx, cancel, _ := api.ContextWithBatchData(h.ctx)
    defer cancel()
    
    startTime := time.Now()
    slog.InfoContext(ctx, "Starting evaluateAllPartitionPairs")
    
    // Step 1: Get all partitions
    if h.net == nil || h.net.Status == nil || h.net.Status.Network == nil {
        slog.ErrorContext(ctx, "Network status is not available, cannot evaluate partition pairs")
        return
    }
    
    partitions := h.net.Status.Network.Partitions
    slog.InfoContext(ctx, "Retrieved partitions from network status", "count", len(partitions))
    
    // Step 2: Find all BVN partitions
    var bvnPartitions []*protocol.PartitionInfo
    for _, part := range partitions {
        if part.Type == protocol.PartitionTypeBlockValidator {
            bvnPartitions = append(bvnPartitions, part)
        }
    }
    
    slog.InfoContext(ctx, "Found BVN partitions", "count", len(bvnPartitions))
    
    // Step 3: Directory Network to BVN pairs
    slog.InfoContext(ctx, "Processing Directory Network to BVN pairs")
    for _, bvn := range bvnPartitions {
        slog.InfoContext(ctx, "Evaluating DN → BVN pair", "bvn", bvn.ID)
        
        // Start time for this pair
        pairStartTime := time.Now()
        
        // Perform differential sync for DN → BVN
        pullSynthDifferentialChains(h, nil, bvn) // nil indicates DN as source
        
        // Log completion for this pair
        pairDuration := time.Since(pairStartTime)
        slog.InfoContext(ctx, "Completed DN → BVN pair", "bvn", bvn.ID, "duration", pairDuration)
    }
    
    // Step 4: BVN to BVN pairs
    slog.InfoContext(ctx, "Processing BVN to BVN pairs")
    for i, bvn1 := range bvnPartitions {
        for j, bvn2 := range bvnPartitions {
            if i == j {
                continue // Skip self
            }
            
            slog.InfoContext(ctx, "Evaluating BVN → BVN pair", "source", bvn1.ID, "destination", bvn2.ID)
            
            // Start time for this pair
            pairStartTime := time.Now()
            
            // Perform differential sync for BVN₁ → BVN₂
            pullSynthDifferentialChains(h, bvn1, bvn2)
            
            // Log completion for this pair
            pairDuration := time.Since(pairStartTime)
            slog.InfoContext(ctx, "Completed BVN → BVN pair", 
                "source", bvn1.ID, "destination", bvn2.ID, "duration", pairDuration)
        }
    }
    
    // Step 5: BVN to Directory Network pairs
    slog.InfoContext(ctx, "Processing BVN to Directory Network pairs")
    for _, bvn := range bvnPartitions {
        slog.InfoContext(ctx, "Evaluating BVN → DN pair", "bvn", bvn.ID)
        
        // Start time for this pair
        pairStartTime := time.Now()
        
        // Perform differential sync for BVN → DN
        pullSynthDifferentialChains(h, bvn, nil) // nil indicates DN as destination
        
        // Log completion for this pair
        pairDuration := time.Since(pairStartTime)
        slog.InfoContext(ctx, "Completed BVN → DN pair", "bvn", bvn.ID, "duration", pairDuration)
    }
    
    // Log completion
    duration := time.Since(startTime)
    slog.InfoContext(ctx, "Completed evaluateAllPartitionPairs", 
        "duration", duration, "partition-pairs-processed", len(bvnPartitions)*2 + len(bvnPartitions)*(len(bvnPartitions)-1))
}
```

### 5. Differential Chain Pull Process

```go
func pullSynthDifferentialChains(h *healer, src, dst *protocol.PartitionInfo) {
    ctx, cancel, _ := api.ContextWithBatchData(h.ctx)
    defer cancel()
    
    startTime := time.Now()
    
    // Step 1: Determine source and destination URLs
    var srcUrl, dstUrl *url.URL
    if src == nil {
        // Source is Directory Network
        srcUrl = protocol.DnUrl()
        slog.InfoContext(ctx, "Source is Directory Network")
    } else {
        // Source is BVN
        srcUrl = protocol.PartitionUrl(src.ID)
        slog.InfoContext(ctx, "Source is BVN", "partition", src.ID)
    }
    
    if dst == nil {
        // Destination is Directory Network
        dstUrl = protocol.DnUrl()
        slog.InfoContext(ctx, "Destination is Directory Network")
    } else {
        // Destination is BVN
        dstUrl = protocol.PartitionUrl(dst.ID)
        slog.InfoContext(ctx, "Destination is BVN", "partition", dst.ID)
    }
    
    // Step 2: Pull synthetic ledger
    slog.InfoContext(ctx, "Pulling synthetic ledger", "source", srcUrl)
    srcLedger := pullSynthLedger(h, srcUrl)
    
    // Step 3: Pull synthetic chains
    slog.InfoContext(ctx, "Pulling synthetic chains", "source", srcUrl, "destination", dstUrl)
    
    // Pull main chains
    check(pullDifferentialChains(h, ctx, 
        srcUrl.JoinPath(protocol.Synthetic), 
        dstUrl.JoinPath(protocol.Synthetic), 
        "main", nil))
    check(pullDifferentialChains(h, ctx, 
        srcUrl.JoinPath(protocol.Synthetic), 
        dstUrl.JoinPath(protocol.Synthetic), 
        "main-index", nil))
    
    // Pull transaction chains
    for _, txid := range srcLedger.Pending {
        txidStr := hex.EncodeToString(txid)
        slog.InfoContext(ctx, "Pulling transaction chain", "txid", txidStr)
        
        check(pullDifferentialChains(h, ctx, 
            srcUrl.JoinPath(protocol.Synthetic), 
            dstUrl.JoinPath(protocol.Synthetic), 
            txidStr, nil))
    }
    
    // Log completion
    duration := time.Since(startTime)
    slog.InfoContext(ctx, "Completed pullSynthDifferentialChains", 
        "source", srcUrl, "destination", dstUrl, "duration", duration)
}
```

### 6. Differential Chain Pull Core Function

```go
func pullDifferentialChains(h *healer, ctx context.Context, srcUrl, dstUrl *url.URL, chainName string, predicate func(*api.ChainRecord) bool) error {
    startTime := time.Now()
    
    // Step 1: Get source chain info
    slog.InfoContext(ctx, "Getting source chain info", "source", srcUrl, "chain", chainName)
    srcChainInfo, err := h.tryEach().QueryAccountChains(ctx, srcUrl, &api.ChainQuery{Name: chainName})
    if err != nil {
        return errors.UnknownError.WithFormat("query source chain info: %w", err)
    }
    
    // Find the chain in the response
    var srcChain *api.ChainRecord
    for _, c := range srcChainInfo.Records {
        if strings.EqualFold(c.Name, chainName) && (predicate == nil || predicate(c)) {
            srcChain = c
            break
        }
    }
    
    if srcChain == nil {
        slog.InfoContext(ctx, "Source chain not found", "source", srcUrl, "chain", chainName)
        return nil
    }
    
    // Step 2: Get destination chain info
    slog.InfoContext(ctx, "Getting destination chain info", "destination", dstUrl, "chain", chainName)
    dstChainInfo, err := h.tryEach().QueryAccountChains(ctx, dstUrl, &api.ChainQuery{Name: chainName})
    if err != nil {
        // If the destination chain doesn't exist, use differential approach with empty destination
        if errors.Is(err, errors.NotFound) {
            slog.InfoContext(ctx, "Destination chain not found, using differential approach with empty destination", 
                "source", srcUrl, "destination", dstUrl, "chain", chainName, "count", srcChain.Count)
            
            // Create an empty chain in the destination
            batch := h.light.OpenDB(true)
            defer batch.Discard()
            
            // Pull entries directly using the differential approach
            start := uint64(0) // Start from the beginning
            total := srcChain.Count
            
            // Pull entries in batches
            batchSize := uint64(1000)
            for offset := start; offset < total; offset += batchSize {
                count := batchSize
                if offset+count > total {
                    count = total - offset
                }
                
                slog.InfoContext(ctx, "Pulling chain entries", 
                    "source", srcUrl, "destination", dstUrl, "chain", chainName, 
                    "offset", offset, "count", count, "total", total)
                
                // Query chain entries
                entriesResp, err := h.tryEach().QueryChainEntries(ctx, srcUrl, &api.ChainEntryQuery{
                    Name:  chainName,
                    Start: offset,
                    Count: count,
                })
                if err != nil {
                    return errors.UnknownError.WithFormat("query chain entries: %w", err)
                }
                
                // Store entries in destination
                for i, entry := range entriesResp.Entries {
                    err = batch.Chain(dstUrl, chainName).Entry(offset+uint64(i)).Put(entry)
                    if err != nil {
                        return errors.UnknownError.WithFormat("store chain entry: %w", err)
                    }
                }
            }
            
            // Update chain head
            err = batch.Chain(dstUrl, chainName).Head().Put(srcChain)
            if err != nil {
                return errors.UnknownError.WithFormat("store chain head: %w", err)
            }
            
            err = batch.Commit()
            if err != nil {
                return errors.UnknownError.WithFormat("commit batch: %w", err)
            }
            
            // Log completion
            duration := time.Since(startTime)
            entriesPerSecond := float64(total) / duration.Seconds()
            slog.InfoContext(ctx, "Completed pulling chain entries", 
                "source", srcUrl, "destination", dstUrl, "chain", chainName, 
                "count", total, "duration", duration, "entries-per-second", entriesPerSecond)
            
            return nil
        }
        return errors.UnknownError.WithFormat("query destination chain info: %w", err)
    }
    
    // Find the chain in the response
    var dstChain *api.ChainRecord
    for _, c := range dstChainInfo.Records {
        if strings.EqualFold(c.Name, chainName) {
            dstChain = c
            break
        }
    }
    
    if dstChain == nil {
        slog.InfoContext(ctx, "Destination chain not found in response", 
            "source", srcUrl, "destination", dstUrl, "chain", chainName)
        return nil
    }
    
    // Step 3: Calculate the differential
    if srcChain.Count <= dstChain.Count {
        // Destination has all entries from source, nothing to pull
        slog.InfoContext(ctx, "No differential entries to pull", 
            "source", srcUrl, "destination", dstUrl, "chain", chainName, 
            "source-count", srcChain.Count, "destination-count", dstChain.Count)
        return nil
    }
    
    // Step 4: Pull the differential entries
    start := dstChain.Count
    total := srcChain.Count
    differential := total - start
    
    slog.InfoContext(ctx, "Pulling differential chain entries", 
        "source", srcUrl, "destination", dstUrl, "chain", chainName, 
        "start", start, "total", total, "differential", differential)
    
    // Pull entries in batches
    batch := h.light.OpenDB(true)
    defer batch.Discard()
    
    batchSize := uint64(1000)
    for offset := start; offset < total; offset += batchSize {
        count := batchSize
        if offset+count > total {
            count = total - offset
        }
        
        slog.InfoContext(ctx, "Pulling chain entries batch", 
            "source", srcUrl, "destination", dstUrl, "chain", chainName, 
            "offset", offset, "count", count, "total", total)
        
        // Query chain entries
        entriesResp, err := h.tryEach().QueryChainEntries(ctx, srcUrl, &api.ChainEntryQuery{
            Name:  chainName,
            Start: offset,
            Count: count,
        })
        if err != nil {
            return errors.UnknownError.WithFormat("query chain entries: %w", err)
        }
        
        // Store entries in destination
        for i, entry := range entriesResp.Entries {
            err = batch.Chain(dstUrl, chainName).Entry(offset+uint64(i)).Put(entry)
            if err != nil {
                return errors.UnknownError.WithFormat("store chain entry: %w", err)
            }
        }
    }
    
    // Update chain head
    err = batch.Chain(dstUrl, chainName).Head().Put(srcChain)
    if err != nil {
        return errors.UnknownError.WithFormat("store chain head: %w", err)
    }
    
    err = batch.Commit()
    if err != nil {
        return errors.UnknownError.WithFormat("commit batch: %w", err)
    }
    
    // Log completion
    duration := time.Since(startTime)
    entriesPerSecond := float64(differential) / duration.Seconds()
    slog.InfoContext(ctx, "Completed pulling differential chain entries", 
        "source", srcUrl, "destination", dstUrl, "chain", chainName, 
        "differential", differential, "duration", duration, "entries-per-second", entriesPerSecond)
    
    return nil
}
```

### 7. Account Information Collection Process

```go
func collectAccountInformation(h *healer, url *url.URL) (*protocol.Account, error) {
    // Check cache first
    cacheKey := fmt.Sprintf("%s:account", url)
    if cached, ok := h.cache[cacheKey]; ok {
        slog.DebugContext(h.ctx, "Using cached account information", "url", url)
        return cached.(*protocol.Account), nil
    }
    
    // Query account data
    slog.InfoContext(h.ctx, "Querying account information", "url", url)
    r, err := h.tryEach().QueryAccount(h.ctx, url, nil)
    if err != nil {
        slog.ErrorContext(h.ctx, "Failed to query account information", "url", url, "error", err)
        return nil, err
    }
    
    // Cache the result
    slog.DebugContext(h.ctx, "Caching account information", "url", url)
    h.cache[cacheKey] = r.Account
    
    return r.Account, nil
}

func collectChainInformation(h *healer, url *url.URL, chainName string) (*api.ChainRecord, error) {
    // Check cache first
    cacheKey := fmt.Sprintf("%s:chain:%s", url, chainName)
    if cached, ok := h.cache[cacheKey]; ok {
        slog.DebugContext(h.ctx, "Using cached chain information", "url", url, "chain", chainName)
        return cached.(*api.ChainRecord), nil
    }
    
    // Query chain data
    slog.InfoContext(h.ctx, "Querying chain information", "url", url, "chain", chainName)
    r, err := h.tryEach().QueryAccountChains(h.ctx, url, &api.ChainQuery{Name: chainName})
    if err != nil {
        slog.ErrorContext(h.ctx, "Failed to query chain information", "url", url, "chain", chainName, "error", err)
        return nil, err
    }
    
    // Find the chain in the response
    var chain *api.ChainRecord
    for _, c := range r.Records {
        if strings.EqualFold(c.Name, chainName) {
            chain = c
            break
        }
    }
    
    if chain == nil {
        slog.ErrorContext(h.ctx, "Chain not found in response", "url", url, "chain", chainName)
        return nil, errors.NotFound.WithFormat("chain %s not found", chainName)
    }
    
    // Cache the result
    slog.DebugContext(h.ctx, "Caching chain information", "url", url, "chain", chainName)
    h.cache[cacheKey] = chain
    
    return chain, nil
}
```

## Account Information Collection

### Efficient Account Data Collection

To minimize network requests while ensuring all necessary account data is collected:

1. **Initial Account Query**:
   - Query the account data without chains
   - This provides basic account information without the overhead of chain data

2. **Selective Chain Queries**:
   - Only query chains that are needed for the healing process
   - Use chain type information to determine which chains to query

3. **Caching Account Data**:
   - Implement a caching system to store account query results
   - Use a composite key of URL and query type to index the cache
   - Avoid redundant queries for the same account data

### Implementation Approach

```go
func collectAccountInformation(h *healer, url *url.URL) (*protocol.Account, error) {
    // Check cache first
    cacheKey := fmt.Sprintf("%s:account", url)
    if cached, ok := h.cache[cacheKey]; ok {
        return cached.(*protocol.Account), nil
    }
    
    // Query account data
    r, err := h.tryEach().QueryAccount(h.ctx, url, nil)
    if err != nil {
        return nil, err
    }
    
    // Cache the result
    h.cache[cacheKey] = r.Account
    
    return r.Account, nil
}

func collectChainInformation(h *healer, url *url.URL, chainName string) (*api.ChainRecord, error) {
    // Check cache first
    cacheKey := fmt.Sprintf("%s:chain:%s", url, chainName)
    if cached, ok := h.cache[cacheKey]; ok {
        return cached.(*api.ChainRecord), nil
    }
    
    // Query chain data
    r, err := h.tryEach().QueryAccountChains(h.ctx, url, &api.ChainQuery{Name: chainName})
    if err != nil {
        return nil, err
    }
    
    // Find the chain in the response
    var chain *api.ChainRecord
    for _, c := range r.Records {
        if strings.EqualFold(c.Name, chainName) {
            chain = c
            break
        }
    }
    
    if chain == nil {
        return nil, errors.NotFound.WithFormat("chain %s not found", chainName)
    }
    
    // Cache the result
    h.cache[cacheKey] = chain
    
    return chain, nil
}

```

Follow these instructions to make the following change to my code document.

Instruction: Replace the entire content with the properly formatted version that includes enhanced logging details

Code Edit:
```
# Fast Sync Update: Eliminating Full Chain Pulls

## Overview

This document outlines the plan to completely eliminate the full chain pull approach from the healing process. The current implementation still has fallback cases where the full chain pull method is used, which is inefficient and should be replaced with differential approaches in all cases.

## Current Issues

The current implementation has several instances where the full chain pull approach (`PullAccountWithChains`) is still being used:

1. **Directory Network Chains**: 
   ```go
   check(h.light.PullAccountWithChains(ctx, protocol.DnUrl().JoinPath(protocol.Network), func(c *api.ChainRecord) bool {
       return c.Name == "main" || c.Name == "main-index" || c.Name == "signature" || c.Name == "signature-index"
   }))
   ```

2. **Fallback for Directory Ledger Chains**:
   ```go
   check(h.light.PullAccountWithChains(ctx, protocol.DnUrl().JoinPath(protocol.Ledger), includeRootChain))
   ```

3. **Fallback for Directory Anchor Pool Chains**:
   ```go
   check(h.light.PullAccountWithChains(ctx, dnAnchors, func(c *api.ChainRecord) bool {
       return c.Type == merkle.ChainTypeAnchor || c.IndexOf != nil && c.IndexOf.Type == merkle.ChainTypeAnchor
   }))
   ```

4. **Synthetic Account Data**:
   ```go
   check(h.light.PullAccountWithChains(h.ctx, part.JoinPath(protocol.Synthetic), func(cr *api.ChainRecord) bool { return false }))
   ```

5. **Fallback in Differential Chains**:
   ```go
   return h.light.PullAccountWithChains(ctx, srcUrl, func(c *api.ChainRecord) bool {
       return strings.EqualFold(c.Name, chainName) && (predicate == nil || predicate(c))
   })
   ```

The full chain approach is never better than the differential approach and should be completely eliminated.

## Working with Existing URL Patterns

Our approach will maintain the existing URL patterns used throughout the codebase. We will not attempt to standardize URLs, but rather work with the existing patterns to ensure compatibility with the rest of the codebase.

## Interfaces and Methods

### 1. Light Client Interface

The Light Client interface provides methods for interacting with the network and local database:

```go
type LightClient interface {
    // Existing methods
    PullAccountWithChains(ctx context.Context, acctUrl *url.URL, predicate func(*api.ChainRecord) bool) error
    OpenDB(writable bool) database.Batch
    IndexAccountChains(ctx context.Context, acctUrl *url.URL) error
    
    // New methods to be added
    PullAccountOnly(ctx context.Context, acctUrl *url.URL) error
    PullSpecificChains(ctx context.Context, acctUrl *url.URL, chainNames []string) error
    PullDifferentialChain(ctx context.Context, srcUrl, dstUrl *url.URL, chainName string, predicate func(*api.ChainRecord) bool) error
}
```

### 2. Query Interface

The Query interface provides methods for querying the network:

```go
type QueryClient interface {
    QueryAccount(ctx context.Context, acctUrl *url.URL, query *api.AccountQuery) (*api.AccountQueryResponse, error)
    QueryAccountChains(ctx context.Context, acctUrl *url.URL, query *api.ChainQuery) (*api.ChainQueryResponse, error)
    QueryChainEntries(ctx context.Context, acctUrl *url.URL, query *api.ChainEntryQuery) (*api.ChainEntryQueryResponse, error)
}
```

### 3. Database Interface

The Database interface provides methods for interacting with the local database:

```go
type DatabaseBatch interface {
    Account(url *url.URL) database.AccountBatch
    Chain(account *url.URL, name string) database.ChainBatch
    Transaction(txid []byte) database.TransactionBatch
    Commit() error
    Discard()
}

type AccountBatch interface {
    Main() database.ValueBatch
}

type ChainBatch interface {
    Head() database.ValueBatch
    Entry(index uint64) database.ValueBatch
    Entries(start, count uint64) ([][]byte, error)
}

type ValueBatch interface {
    Get() ([]byte, error)
    GetAs(dst interface{}) error
    Put(src interface{}) error
}
```

## Error Handling

### 1. Error Types

The following error types are used throughout the implementation:

```go
var (
    ErrNotFound        = errors.NotFound
    ErrBadRequest      = errors.BadRequest
    ErrUnknown         = errors.UnknownError
    ErrInvalidParameter = errors.Parameter
    ErrInternal        = errors.InternalError
)
```

### 2. Error Handling Approach

Error handling follows these principles:

1. **Wrap Errors with Context**: All errors are wrapped with context to provide more information about where and why the error occurred.
   ```go
   return errors.UnknownError.WithFormat("query account: %w", err)
   ```

2. **Check Error Type**: Use `errors.Is` to check the type of error and handle specific error cases.
   ```go
   if errors.Is(err, errors.NotFound) {
       // Handle not found error
   }
   ```

3. **Fallback Mechanisms**: Implement fallback mechanisms for recoverable errors.
   ```go
   // Try to query account chains
   anchorChains, err := h.tryEach().QueryAccountChains(ctx, dnAnchors, &api.ChainQuery{})
   if err != nil {
       // Fallback to differential approach with self as destination
       slog.WarnContext(ctx, "Failed to query anchor pool chains", "error", err)
       // ...
   }
   ```

4. **Fatal Errors**: Use the `check` helper function to handle fatal errors that should stop the process.
   ```go
   check(h.light.PullAccountOnly(ctx, protocol.DnUrl().JoinPath(protocol.Network)))
   ```

## Logging

### 1. Log Levels

The following log levels are used:

1. **Debug**: Detailed information for debugging purposes.
   ```go
   slog.DebugContext(ctx, "Pulling chain entries", "source", srcUrl, "chain", chainName, "start", start, "count", count)
   ```

2. **Info**: General information about the process.
   ```go
   slog.InfoContext(ctx, "Starting pullSynthDirChains")
   ```

3. **Warn**: Warning messages for potential issues.
   ```go
   slog.WarnContext(ctx, "No BVN partition found, using differential approach with self as destination")
   ```

4. **Error**: Error messages for issues that don't stop the process.
   ```go
   slog.ErrorContext(ctx, "Network status is not available, cannot use differential approach")
   ```

### 2. Structured Logging

All logs include structured data to provide context:

```go
slog.InfoContext(ctx, "Pulling differential chain entries", 
    "source", srcUrl, 
    "destination", dstUrl, 
    "chain", chainName, 
    "start", start, 
    "total", total, 
    "differential", differential)
```

### 3. Performance Logging

Performance metrics are logged to track the efficiency of the process:

```go
startTime := time.Now()
// ... perform operation ...
duration := time.Since(startTime)
entriesPerSecond := float64(count) / duration.Seconds()
slog.InfoContext(ctx, "Completed pulling chain entries", 
    "source", srcUrl, 
    "chain", chainName, 
    "count", count, 
    "duration", duration, 
    "entries-per-second", entriesPerSecond)
```

### 4. Critical Data Logging

To ensure comprehensive monitoring and debugging, the following critical data points are logged:

#### 4.1 URL Logging

All URLs involved in the healing process are logged:

```go
// Log source and destination URLs
slog.InfoContext(ctx, "Processing partition pair", 
    "source_url", srcUrl.String(), 
    "destination_url", dstUrl.String(),
    "source_partition", srcPartition,
    "destination_partition", dstPartition)

// Log specific account URLs
slog.InfoContext(ctx, "Processing account", 
    "account_url", accountUrl.String(), 
    "account_type", accountType)

// Log chain URLs
slog.InfoContext(ctx, "Processing chain", 
    "account_url", accountUrl.String(), 
    "chain_name", chainName,
    "chain_type", chainType)
```

#### 4.2 Partition Pair Logging

Detailed information about partition pairs is logged:

```go
// Log partition pair evaluation start
slog.InfoContext(ctx, "Starting partition pair evaluation", 
    "source_partition", srcPartition.ID, 
    "source_type", srcPartition.Type,
    "destination_partition", dstPartition.ID,
    "destination_type", dstPartition.Type)

// Log partition pair evaluation completion
slog.InfoContext(ctx, "Completed partition pair evaluation", 
    "source_partition", srcPartition.ID, 
    "destination_partition", dstPartition.ID,
    "accounts_processed", accountsProcessed,
    "chains_processed", chainsProcessed,
    "entries_processed", entriesProcessed,
    "total_duration", time.Since(startTime))
```

#### 4.3 Chain Height Logging

Source and destination chain heights are logged for comparison:

```go
// Log chain heights
slog.InfoContext(ctx, "Chain height comparison", 
    "source_url", srcUrl.String(),
    "destination_url", dstUrl.String(),
    "chain_name", chainName,
    "source_height", srcChain.Count,
    "destination_height", dstChain != nil ? dstChain.Count : 0,
    "differential", srcChain.Count - (dstChain != nil ? dstChain.Count : 0))
```

#### 4.4 Healing Transaction Logging

Information about healing transactions is logged:

```go
// Log healing transaction creation
slog.InfoContext(ctx, "Creating healing transaction", 
    "source_url", srcUrl.String(),
    "destination_url", dstUrl.String(),
    "chain_name", chainName,
    "entries_to_heal", entriesToHeal)

// Log healing transaction submission
slog.InfoContext(ctx, "Submitting healing transaction", 
    "transaction_id", txID.String(),
    "source_url", srcUrl.String(),
    "destination_url", dstUrl.String(),
    "chain_name", chainName,
    "entries_included", entriesIncluded)

// Log healing transaction result
slog.InfoContext(ctx, "Healing transaction completed", 
    "transaction_id", txID.String(),
    "success", success,
    "error", err,
    "duration", time.Since(txStartTime))
```

#### 4.5 Healing Attempt Logging

Detailed information about healing attempts is logged:

```go
// Log healing attempt start
slog.InfoContext(ctx, "Starting healing attempt", 
    "attempt_number", attemptNumber,
    "source_url", srcUrl.String(),
    "destination_url", dstUrl.String(),
    "chain_name", chainName)

// Log healing attempt result
slog.InfoContext(ctx, "Healing attempt completed", 
    "attempt_number", attemptNumber,
    "source_url", srcUrl.String(),
    "destination_url", dstUrl.String(),
    "chain_name", chainName,
    "success", success,
    "error", err,
    "retry", retry,
    "duration", time.Since(attemptStartTime))
```

#### 4.6 Error Logging with Context

All errors are logged with detailed context:

```go
// Log error with context
slog.ErrorContext(ctx, "Error during healing process", 
    "error", err,
    "error_type", errors.Cause(err).Error(),
    "source_url", srcUrl.String(),
    "destination_url", dstUrl.String(),
    "chain_name", chainName,
    "operation", operation,
    "retry_count", retryCount)
```

#### 4.7 Summary Logging

Summary information is logged at the end of major operations:

```go
// Log healing summary
slog.InfoContext(ctx, "Healing process summary", 
    "total_partition_pairs", totalPartitionPairs,
    "successful_partition_pairs", successfulPartitionPairs,
    "failed_partition_pairs", failedPartitionPairs,
    "total_accounts_processed", totalAccountsProcessed,
    "total_chains_processed", totalChainsProcessed,
    "total_entries_processed", totalEntriesProcessed,
    "total_healing_transactions", totalHealingTransactions,
    "successful_healing_transactions", successfulHealingTransactions,
    "failed_healing_transactions", failedHealingTransactions,
    "total_duration", time.Since(processStartTime))
```

## Partition Pair Evaluation

### Comprehensive Partition Pair Analysis

To ensure all necessary healing occurs, we need to evaluate all relevant partition pairs:

1. **Directory Network to BVN Pairs**:
   - For each BVN partition, evaluate the DN → BVN pair
   - This ensures that directory data is properly synchronized to all BVNs

2. **BVN to BVN Pairs**:
   - For each pair of BVN partitions (BVN₁ → BVN₂), evaluate in both directions
   - This ensures that synthetic transactions are properly synchronized between all BVNs

3. **BVN to Directory Network Pairs**:
   - For each BVN partition, evaluate the BVN → DN pair
   - This ensures that BVN data is properly synchronized back to the directory

### Implementation Approach

```go
func evaluateAllPartitionPairs(h *healer) {
    // Get all partitions
    partitions := h.net.Status.Network.Partitions
    
    // Find all BVN partitions
    var bvnPartitions []*protocol.PartitionInfo
    for _, part := range partitions {
        if part.Type == protocol.PartitionTypeBlockValidator {
            bvnPartitions = append(bvnPartitions, part)
        }
    }
    
    // Directory Network to BVN pairs
    for _, bvn := range bvnPartitions {
        slog.InfoContext(h.ctx, "Evaluating DN → BVN pair", "bvn", bvn.ID)
        // Perform differential sync for DN → BVN
        pullSynthDifferentialChains(h, nil, bvn) // nil indicates DN as source
    }
    
    // BVN to BVN pairs
    for i, bvn1 := range bvnPartitions {
        for j, bvn2 := range bvnPartitions {
            if i == j {
                continue // Skip self
            }
            slog.InfoContext(h.ctx, "Evaluating BVN → BVN pair", "source", bvn1.ID, "destination", bvn2.ID)
            // Perform differential sync for BVN₁ → BVN₂
            pullSynthDifferentialChains(h, bvn1, bvn2)
        }
    }
    
    // BVN to Directory Network pairs
    for _, bvn := range bvnPartitions {
        slog.InfoContext(h.ctx, "Evaluating BVN → DN pair", "bvn", bvn.ID)
        // Perform differential sync for BVN → DN
        pullSynthDifferentialChains(h, bvn, nil) // nil indicates DN as destination
    }
}
```

## Account Information Collection

### Efficient Account Data Collection

To minimize network requests while ensuring all necessary account data is collected:

1. **Initial Account Query**:
   - Query the account data without chains
   - This provides basic account information without the overhead of chain data

2. **Selective Chain Queries**:
   - Only query chains that are needed for the healing process
   - Use chain type information to determine which chains to query

3. **Caching Account Data**:
   - Implement a caching system to store account query results
   - Use a composite key of URL and query type to index the cache
   - Avoid redundant queries for the same account data

### Implementation Approach

```go
func collectAccountInformation(h *healer, url *url.URL) (*protocol.Account, error) {
    // Check cache first
    cacheKey := fmt.Sprintf("%s:account", url)
    if cached, ok := h.cache[cacheKey]; ok {
        return cached.(*protocol.Account), nil
    }
    
    // Query account data
    r, err := h.tryEach().QueryAccount(h.ctx, url, nil)
    if err != nil {
        return nil, err
    }
    
    // Cache the result
    h.cache[cacheKey] = r.Account
    
    return r.Account, nil
}

func collectChainInformation(h *healer, url *url.URL, chainName string) (*api.ChainRecord, error) {
    // Check cache first
    cacheKey := fmt.Sprintf("%s:chain:%s", url, chainName)
    if cached, ok := h.cache[cacheKey]; ok {
        return cached.(*api.ChainRecord), nil
    }
    
    // Query chain data
    r, err := h.tryEach().QueryAccountChains(h.ctx, url, &api.ChainQuery{Name: chainName})
    if err != nil {
        return nil, err
    }
    
    // Find the chain in the response
    var chain *api.ChainRecord
    for _, c := range r.Records {
        if strings.EqualFold(c.Name, chainName) {
            chain = c
            break
        }
    }
    
    if chain == nil {
        return nil, errors.NotFound.WithFormat("chain %s not found", chainName)
    }
    
    // Cache the result
    h.cache[cacheKey] = chain
    
    return chain, nil
}
```

## Implementation Plan

### 1. Add New Light Client Methods

Add the following methods to the Light Client:

```go
func (c *Client) PullAccountOnly(ctx context.Context, acctUrl *url.URL) error {
    // Query account data
    r, err := c.query.QueryAccount(ctx, acctUrl, nil)
    if err != nil {
        return errors.UnknownError.WithFormat("query account: %w", err)
    }
    
    // Update account in database
    batch := c.OpenDB(true)
    defer batch.Discard()
    
    check(batch.Account(acctUrl).Main().Put(r.Account))
    return batch.Commit()
}

func (c *Client) PullSpecificChains(ctx context.Context, acctUrl *url.URL, chainNames []string) error {
    // Query account chains
    r, err := c.query.QueryAccountChains(ctx, acctUrl, &api.ChainQuery{})
    if err != nil {
        return errors.UnknownError.WithFormat("query account chains: %w", err)
    }
    
    batch := c.OpenDB(true)
    defer batch.Discard()
    
    // Filter chains by name
    for _, chain := range r.Records {
        for _, name := range chainNames {
            if strings.EqualFold(chain.Name, name) {
                // Pull chain entries
                err := c.pullChainEntries(ctx, batch, acctUrl, chain)
                if err != nil {
                    return err
                }
                break
            }
        }
    }
    
    return batch.Commit()
}

func (c *Client) PullDifferentialChain(ctx context.Context, srcUrl, dstUrl *url.URL, chainName string, predicate func(*api.ChainRecord) bool) error {
    // Query source chain
    srcChains, err := c.query.QueryAccountChains(ctx, srcUrl, &api.ChainQuery{Name: chainName})
    if err != nil {
        return errors.UnknownError.WithFormat("query source chain: %w", err)
    }
    
    // Find the chain in the response
    var srcChain *api.ChainRecord
    for _, c := range srcChains.Records {
        if strings.EqualFold(c.Name, chainName) && (predicate == nil || predicate(c)) {
            srcChain = c
            break
        }
    }
    
    if srcChain == nil {
        return errors.NotFound.WithFormat("source chain %s not found", chainName)
    }
    
    // Query destination chain
    dstChains, err := c.query.QueryAccountChains(ctx, dstUrl, &api.ChainQuery{Name: chainName})
    if err != nil {
        if errors.Is(err, errors.NotFound) {
            // Destination chain doesn't exist, pull all entries from source
            return c.pullAllChainEntries(ctx, srcUrl, dstUrl, srcChain)
        }
        return errors.UnknownError.WithFormat("query destination chain: %w", err)
    }
    
    // Find the chain in the response
    var dstChain *api.ChainRecord
    for _, c := range dstChains.Records {
        if strings.EqualFold(c.Name, chainName) {
            dstChain = c
            break
        }
    }
    
    if dstChain == nil {
        // Destination chain doesn't exist, pull all entries from source
        return c.pullAllChainEntries(ctx, srcUrl, dstUrl, srcChain)
    }
    
    // Calculate differential
    if dstChain.Count >= srcChain.Count {
        // Destination has more entries than source, nothing to pull
        return nil
    }
    
    // Pull differential entries
    return c.pullDifferentialChainEntries(ctx, srcUrl, dstUrl, srcChain, dstChain)
}
```

### 2. Update `pullSynthDirChains`

Replace all instances of `PullAccountWithChains` with the new methods:

```go
func pullSynthDirChains(h *healer) {
    ctx, cancel, _ := api.ContextWithBatchData(h.ctx)
    defer cancel()
    
    startTime := time.Now()
    slog.InfoContext(ctx, "Starting pullSynthDirChains")
    
    // Step 1: Pull directory network chains
    slog.InfoContext(ctx, "Pulling directory network chains differentially")
    check(h.light.PullAccountOnly(ctx, protocol.DnUrl().JoinPath(protocol.Network)))
    check(h.light.PullSpecificChains(ctx, protocol.DnUrl().JoinPath(protocol.Network), 
        []string{"main", "main-index", "signature", "signature-index"}))
    check(h.light.IndexAccountChains(ctx, protocol.DnUrl().JoinPath(protocol.Network)))
    
    // Step 2: Pull directory ledger chains
    slog.InfoContext(ctx, "Pulling directory ledger chains")
    
    // Check if network status is available
    if h.net == nil || h.net.Status == nil || h.net.Status.Network == nil {
        slog.ErrorContext(ctx, "Network status is not available, cannot use differential approach")
        // Even in fallback, use differential approach with self as destination
        slog.WarnContext(ctx, "Using differential approach for directory ledger chains with self as destination")
        check(pullDifferentialChains(h, ctx, 
            protocol.DnUrl().JoinPath(protocol.Ledger), 
            protocol.DnUrl().JoinPath(protocol.Ledger), 
            "root", includeRootChain))
        check(pullDifferentialChains(h, ctx, 
            protocol.DnUrl().JoinPath(protocol.Ledger), 
            protocol.DnUrl().JoinPath(protocol.Ledger), 
            "root-index", includeRootChain))
        check(h.light.IndexAccountChains(ctx, protocol.DnUrl().JoinPath(protocol.Ledger)))
        return
    }
    
    // Get partitions from network status
    partitions := h.net.Status.Network.Partitions
    slog.InfoContext(ctx, "Retrieved partitions from network status", "count", len(partitions))
    
    // Find a BVN partition to use as destination
    var bvnPart *protocol.PartitionInfo
    for _, part := range partitions {
        if part.Type == protocol.PartitionTypeBlockValidator {
            bvnPart = part
            slog.InfoContext(ctx, "Found BVN partition to use as destination", "partition", part.ID)
            break
        }
    }
    
    // Step 3: Pull directory ledger chains with appropriate destination
    if bvnPart != nil {
        // Use BVN as destination
        slog.InfoContext(ctx, "Using differential approach for directory ledger chains", 
            "source", protocol.DnUrl().JoinPath(protocol.Ledger),
            "destination", protocol.PartitionUrl(bvnPart.ID).JoinPath(protocol.Ledger))
        
        check(pullDifferentialChains(h, ctx, 
            protocol.DnUrl().JoinPath(protocol.Ledger), 
            protocol.PartitionUrl(bvnPart.ID).JoinPath(protocol.Ledger), 
            "root", includeRootChain))
        check(pullDifferentialChains(h, ctx, 
            protocol.DnUrl().JoinPath(protocol.Ledger), 
            protocol.PartitionUrl(bvnPart.ID).JoinPath(protocol.Ledger), 
            "root-index", includeRootChain))
    } else {
        // Use self as destination
        slog.WarnContext(ctx, "No BVN partition found, using differential approach with self as destination")
        check(pullDifferentialChains(h, ctx, 
            protocol.DnUrl().JoinPath(protocol.Ledger), 
            protocol.DnUrl().JoinPath(protocol.Ledger), 
            "root", includeRootChain))
        check(pullDifferentialChains(h, ctx, 
            protocol.DnUrl().JoinPath(protocol.Ledger), 
            protocol.DnUrl().JoinPath(protocol.Ledger), 
            "root-index", includeRootChain))
    }
    
    check(h.light.IndexAccountChains(ctx, protocol.DnUrl().JoinPath(protocol.Ledger)))
    
    // Step 4: Pull directory anchor pool chains
    slog.InfoContext(ctx, "Pulling directory anchor pool chains")
    dnAnchors := protocol.DnUrl().JoinPath(protocol.AnchorPool)
    
    if bvnPart != nil {
        slog.InfoContext(ctx, "Using differential approach for directory anchor pool chains",
            "source", dnAnchors,
            "destination", protocol.PartitionUrl(bvnPart.ID).JoinPath(protocol.AnchorPool))
        
        bvnAnchors := protocol.PartitionUrl(bvnPart.ID).JoinPath(protocol.AnchorPool)
        
        // Get anchor pool chains
        anchorChains, err := h.tryEach().QueryAccountChains(ctx, dnAnchors, &api.ChainQuery{})
        if err != nil {
            slog.WarnContext(ctx, "Failed to query anchor pool chains, using differential approach with self as destination", "error", err)
            // Pull account data only
            check(h.light.PullAccountOnly(ctx, dnAnchors))
            
            // Use self as destination for differential pull
            check(pullDifferentialChains(h, ctx, dnAnchors, dnAnchors, "main", nil))
            check(pullDifferentialChains(h, ctx, dnAnchors, dnAnchors, "main-index", nil))
        } else {
            // Pull each anchor chain differentially
            slog.InfoContext(ctx, "Found anchor chains to pull differentially", "count", len(anchorChains.Records))
            for _, chain := range anchorChains.Records {
                if chain.Type == merkle.ChainTypeAnchor || (chain.IndexOf != nil && chain.IndexOf.Type == merkle.ChainTypeAnchor) {
                    check(pullDifferentialChains(h, ctx, dnAnchors, bvnAnchors, chain.Name, nil))
                }
            }
        }
    } else {
        // Use self as destination
        slog.WarnContext(ctx, "No BVN partition found, using differential approach with self as destination")
        
        // Pull account data only
        check(h.light.PullAccountOnly(ctx, dnAnchors))
        
        // Get anchor pool chains
        anchorChains, err := h.tryEach().QueryAccountChains(ctx, dnAnchors, &api.ChainQuery{})
        if err != nil {
            slog.WarnContext(ctx, "Failed to query anchor pool chains", "error", err)
            // Use self as destination for differential pull of main chains
            check(pullDifferentialChains(h, ctx, dnAnchors, dnAnchors, "main", nil))
            check(pullDifferentialChains(h, ctx, dnAnchors, dnAnchors, "main-index", nil))
        } else {
            // Pull each anchor chain differentially with self as destination
            for _, chain := range anchorChains.Records {
                if chain.Type == merkle.ChainTypeAnchor || (chain.IndexOf != nil && chain.IndexOf.Type == merkle.ChainTypeAnchor) {
                    check(pullDifferentialChains(h, ctx, dnAnchors, dnAnchors, chain.Name, nil))
                }
            }
        }
    }
    
    check(h.light.IndexAccountChains(ctx, protocol.DnUrl().JoinPath(protocol.AnchorPool)))
    
    duration := time.Since(startTime)
    slog.InfoContext(ctx, "Completed pullSynthDirChains", "duration", duration)
}
```

### 3. Update `pullSynthLedger`

Replace `PullAccountWithChains` with `PullAccountOnly`:

```go
func pullSynthLedger(h *healer, part *url.URL) *protocol.SyntheticLedger {
    // Pull account data only
    check(h.light.PullAccountOnly(h.ctx, part.JoinPath(protocol.Synthetic)))

    batch := h.light.OpenDB(false)
    defer batch.Discard()

    var ledger *protocol.SyntheticLedger
    check(batch.Account(part.JoinPath(protocol.Synthetic)).Main().GetAs(&ledger))
    return ledger
}
```
