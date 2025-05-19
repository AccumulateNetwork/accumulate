# Anchor Healing

## Introduction

Anchor healing is a critical process in Accumulate that ensures cryptographic commitments between partitions are properly maintained. This document provides a detailed explanation of the anchor healing process, its implementation, and how it works.

## What are Anchors?

Anchors in Accumulate are cryptographic commitments that link blocks across different partitions. They serve several important purposes:

1. **Cross-Chain Validation**: Allow one partition to validate the state of another partition
2. **Data Integrity**: Ensure that data cannot be tampered with without detection
3. **Consistency**: Maintain a consistent view of the distributed ledger across partitions

## Valid Anchor Paths

A key aspect of anchor healing is understanding which partition pairs are valid for anchoring:

1. **Directory Network to BVN (DN→BVN)**: Anchors from the Directory Network to a Blockchain Validation Network
2. **BVN to Directory Network (BVN→DN)**: Anchors from a Blockchain Validation Network to the Directory Network

Importantly, anchors do not flow directly between BVNs (no BVN→BVN anchors).

```go
// From internal/core/execute/v1/chain/anchor.go
// This code shows how anchors are only processed between DN and BVNs

func (x *Executor) ProcessAnchor(batch *database.Batch, envHash [32]byte, txn *protocol.Transaction) error {
    // ...
    
    // Verify the source and destination are valid
    if src.Type != protocol.PartitionTypeDirectory && dst.Type != protocol.PartitionTypeDirectory {
        return errors.BadRequest.WithFormat("anchor source or destination must be the directory")
    }
    
    // ...
}
```

## Anchor Healing Process Flow

The anchor healing process follows these steps:

1. **Initialization**: Set up the healer structure and identify valid partition pairs (DN→BVN and BVN→DN only)
2. **Anchor Discovery**: Find missing anchors between partitions
3. **Signature Collection**: Collect signatures from network nodes to validate the anchor
4. **Anchor Healing**: Submit anchors with signatures to the destination partition
5. **Status Tracking**: Monitor and report on healing progress

## Core Implementation

The core implementation of anchor healing is in the `internal/core/healing/anchors.go` file:

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

The implementation includes version-specific logic to handle different network protocol versions, with specialized handling for DN-to-BVN anchors in Vandenberg-enabled networks.

## Version-Specific Implementations

### Vandenberg DN-to-BVN Anchor Healing

```go
// From internal/core/healing/anchors.go
func healDnAnchorV2(ctx context.Context, args HealAnchorArgs, si SequencedInfo) error {
    if args.Querier == nil {
        args.Querier = args.Client
    }

    // Resolve the anchor sent to the BVN
    rBVN, err := ResolveSequenced[*messaging.TransactionMessage](ctx, args.Client, args.NetInfo, protocol.Directory, si.Destination, si.Number, true)
    if err != nil {
        return err
    }

    // Resolve the anchor the DN sent to itself
    rDN, err := ResolveSequenced[*messaging.TransactionMessage](ctx, args.Client, args.NetInfo, protocol.Directory, protocol.Directory, si.Number, true)
    if err != nil {
        return err
    }

    // Fetch the transaction and signatures
    var signatures []messaging.Message
    Q := api.Querier2{Querier: args.Querier}
    res, err := Q.QueryMessage(ctx, rDN.ID, nil)
    switch {
    case err == nil:
        // If the DN self-anchor has not been delivered, fall back to version 1
        if !res.Status.Delivered() {
            slog.InfoContext(ctx, "DN self anchor has not been delivered, falling back", "id", rDN.ID, "source", si.Source, "destination", si.Destination, "number", si.Number)
            return healAnchorV1(ctx, args, si)
        }

        for _, set := range res.Signatures.Records {
            for _, sig := range set.Signatures.Records {
                blk, ok := sig.Message.(*messaging.BlockAnchor)
                if !ok {
                    continue
                }

                // Use the DN -> DN signature but the DN -> BVN sequenced message
                signatures = append(signatures, &messaging.BlockAnchor{
                    Anchor:    rBVN.Sequence,
                    Signature: blk.Signature,
                })
            }
        }

    case !errors.Is(err, errors.NotFound):
        return err
    }
    if args.Pretend {
        return nil
    }

    slog.InfoContext(ctx, "Submitting signatures from the DN", "count", len(signatures))
    err = args.Submit(signatures...)
    if err != nil {
        return err
    }

    if args.Wait {
        return waitFor(ctx, Q.Querier, si.ID)
    }
    return nil
}
```

This implementation specifically handles DN-to-BVN anchors in Vandenberg-enabled networks, using signatures from the DN self-anchor.

### Pre-Vandenberg Anchor Healing

```go
// From internal/core/healing/anchors.go
func healAnchorV1(ctx context.Context, args HealAnchorArgs, si SequencedInfo) error {
    srcUrl := protocol.PartitionUrl(si.Source)
    dstUrl := protocol.PartitionUrl(si.Destination)

    if args.Querier == nil {
        args.Querier = args.Client
    }

    // If the message ID is not known, resolve it
    var theAnchorTxn *protocol.Transaction
    if si.ID == nil {
        r, err := ResolveSequenced[*messaging.TransactionMessage](ctx, args.Client, args.NetInfo, si.Source, si.Destination, si.Number, true)
        if err != nil {
            return err
        }
        si.ID = r.ID
        theAnchorTxn = r.Message.Transaction
    }

    // Fetch the transaction and signatures
    var sigSets []*api.SignatureSetRecord
    Q := api.Querier2{Querier: args.Querier}
    res, err := Q.QueryMessage(ctx, si.ID, nil)
    switch {
    case err == nil:
        if res.Status.Delivered() {
            slog.InfoContext(ctx, "Anchor has been delivered", "id", si.ID, "source", si.Source, "destination", si.Destination, "number", si.Number)
            return errors.Delivered
        }
        sigSets = res.Signatures.Records
        theAnchorTxn = res.Message.(*messaging.TransactionMessage).Transaction

    case !errors.Is(err, errors.NotFound):
        return err
    }

    // Signature collection and submission
    // ...
}
```

This implementation handles anchor healing for pre-Vandenberg networks or for cases where the Vandenberg-specific approach cannot be used.

## Signature Collection

A critical aspect of anchor healing is collecting signatures from network nodes to validate the anchor:

```go
// From internal/core/healing/anchors.go
// Keep track of which validators have already signed
signed := map[string]bool{}
for _, sigs := range sigSets {
    for _, sig := range sigs.Signatures.Records {
        msg, ok := sig.Message.(*messaging.SignatureMessage)
        if !ok {
            continue
        }
        
        ks, ok := msg.Signature.(protocol.KeySignature)
        if !ok {
            continue
        }
        
        signed[ks.GetSigner().String()] = true
    }
}

// Get a signature from each node that hasn't signed
var gotPartSig bool
var signatures []protocol.Signature
for peer, info := range args.NetInfo.Peers[strings.ToLower(si.Source)] {
    if signed[info.Key] {
        continue
    }
    
    addr := multiaddr.StringCast("/p2p/" + peer.String())
    if len(info.Addresses) > 0 {
        addr = info.Addresses[0].Encapsulate(addr)
    }
    
    ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()
    
    slog.InfoContext(ctx, "Querying node for its signature", "id", peer)
    res, err := args.Client.ForAddress(addr).Private().Sequence(ctx, srcUrl.JoinPath(protocol.AnchorPool), dstUrl, si.Number, private.SequenceOptions{})
    if err != nil {
        slog.ErrorContext(ctx, "Query failed", "error", err)
        continue
    }
    
    // Process the signature
    // ...
}
```

This code tracks which validators have already signed the anchor and collects signatures from nodes that haven't signed yet.

## Signature Validation

Each signature is cryptographically verified before being accepted:

```go
// From internal/core/healing/anchors.go
// Filter out bad signatures
if !sig.Verify(nil, seq) {
    slog.ErrorContext(ctx, "Node gave us an invalid signature", "id", info)
    continue
}
```

This validation ensures that only valid signatures are included in the healed anchor.

## CLI Implementation

The anchor healing process is exposed through a command-line interface in `tools/cmd/debug/heal_anchor.go`:

```go
// From tools/cmd/debug/heal_anchor.go
func healAnchor(cmd *cobra.Command, args []string) {
    // Parse command-line arguments
    // ...

    // Create a healer instance
    h := &healer{
        ctx:      ctx,
        C1:       client,
        C2:       client,
        net:      networkInfo,
        pairStats: make(map[string]*PartitionPairStats),
        startTime: time.Now(),
    }

    // Initialize valid partition pairs (DN→BVN and BVN→DN only)
    for _, src := range h.net.Status.Network.Partitions {
        for _, dst := range h.net.Status.Network.Partitions {
            // Skip self pairs
            if src.ID == dst.ID {
                continue
            }
            
            // Skip BVN to BVN pairs (not valid for anchoring)
            if src.Type != protocol.PartitionTypeDirectory && dst.Type != protocol.PartitionTypeDirectory {
                continue
            }
            
            // Create a key for this partition pair
            pairKey := fmt.Sprintf("%s:%s", src.ID, dst.ID)
            
            // If this pair doesn't exist in stats yet, add it
            if _, exists := h.pairStats[pairKey]; !exists {
                h.pairStats[pairKey] = &PartitionPairStats{
                    Source:      src.ID,
                    Destination: dst.ID,
                    LastUpdated: time.Now(),
                    IsUpToDate:  false, // Assume not up to date until verified
                }
            }
        }
    }

    // Set up periodic reporting
    // ...

    // Run the healing process
    h.heal(args)
    
    // Print a final report
    // ...
}
```

This CLI implementation provides a user-friendly interface for running the anchor healing process, with special attention to initializing only valid partition pairs (DN→BVN and BVN→DN).

## Healing a Single Anchor

The process of healing a single anchor is implemented in the `healSingleAnchor` function:

```go
// From tools/cmd/debug/heal_anchor.go
func (h *healer) healSingleAnchor(srcId, dstId string, seqNum uint64, txid *url.TxID, txns map[[32]byte]*protocol.Transaction) bool {
    var count int
    
    // Increment the transaction submitted counter
    h.txSubmitted++
    
    // Update pair statistics
    pairKey := fmt.Sprintf("%s:%s", srcId, dstId)
    if pairStat, ok := h.pairStats[pairKey]; ok {
        pairStat.TxSubmitted++
        pairStat.LastUpdated = time.Now()
    }
    
retry:
    err := healing.HealAnchor(h.ctx, healing.HealAnchorArgs{
        Client:  h.C2.ForAddress(nil),
        Source:  srcId,
        Dest:    dstId,
        Number:  seqNum,
        TxnMap:  txns,
        ID:      txid,
    })
    if err == nil {
        // Successfully healed the anchor
        h.txDelivered++
        h.totalHealed++
        
        // Update pair statistics
        if pairStat, ok := h.pairStats[pairKey]; ok {
            pairStat.TxDelivered++
            pairStat.CurrentDelivered = seqNum
            pairStat.LastUpdated = time.Now()
        }
        
        slog.InfoContext(h.ctx, "Successfully healed anchor", 
                "source", srcId, "destination", dstId, "number", seqNum, "id", txid,
                "submitted", h.txSubmitted, "delivered", h.txDelivered)
        return false
    }
    
    // Handle various error conditions
    // ...
    
    count++
    if count >= 10 {
        // Failed after multiple attempts
        h.totalFailed++
        
        // Update pair statistics
        if pairStat, ok := h.pairStats[pairKey]; ok {
            pairStat.TxFailed++
            pairStat.LastUpdated = time.Now()
        }
        
        slog.ErrorContext(h.ctx, "Anchor still pending after multiple attempts, skipping", 
                "source", srcId, "destination", dstId, "number", seqNum, "id", txid,
                "attempts", count, "submitted", h.txSubmitted, "delivered", h.txDelivered)
        return false
    }
    slog.WarnContext(h.ctx, "Anchor still pending, retrying", 
            "source", srcId, "destination", dstId, "number", seqNum, "id", txid,
            "attempt", count, "submitted", h.txSubmitted, "delivered", h.txDelivered)
    goto retry
}
```

This function attempts to heal a single anchor and includes retry logic for handling temporary failures.

## Reporting

The anchor healing process includes detailed reporting to track progress:

```go
// From tools/cmd/debug/heal_anchor.go
func (h *healer) printAnchorHealingReport() {
    // Print a header for the report
    fmt.Println()
    color.New(color.FgHiCyan).Add(color.Bold).Println("+-----------------------------------------------------------+")
    color.New(color.FgHiCyan).Add(color.Bold).Println("|                ANCHOR HEALING REPORT                      |")
    color.New(color.FgHiCyan).Add(color.Bold).Println("+-----------------------------------------------------------+")
    
    // Calculate and print the total runtime
    runtime := time.Since(h.startTime)
    color.New(color.FgHiWhite).Add(color.Bold).Print("* Total Runtime: ")
    color.New(color.FgHiYellow).Println(runtime.Round(time.Second))
    
    // Print delivery statistics
    color.New(color.FgHiWhite).Add(color.Bold).Println("\n* DELIVERY STATUS:")
    color.New(color.FgHiWhite).Print("   Transactions Submitted: ")
    color.New(color.FgHiYellow).Printf("%d\n", h.txSubmitted)
    color.New(color.FgHiWhite).Print("   Transactions Delivered: ")
    color.New(color.FgHiYellow).Printf("%d\n", h.txDelivered)
    color.New(color.FgHiWhite).Print("   Transactions Failed: ")
    color.New(color.FgHiYellow).Printf("%d\n", h.totalFailed)
    
    // Print partition pair statistics
    // ...
}
```

This reporting provides detailed information about the anchor healing process, including transaction counts and partition pair statistics.

## Recent Changes

Recent changes to the anchor healing process include:

1. **Improved Reporting**: Enhanced reporting to provide more detailed information about the healing process
2. **ASCII Formatting**: Updated report formatting to use ASCII characters for better terminal compatibility
3. **Timestamp Formatting**: Improved timestamp formatting for readability
4. **Valid Partition Pair Tracking**: Enhanced initialization to ensure only valid partition pairs (DN→BVN and BVN→DN) are included in reports

## Comparison with Synthetic Healing

While both synthetic transaction healing and anchor healing aim to ensure data consistency across partitions, there are some key differences:

1. **Valid Partition Pairs**: Anchor healing only operates on DN→BVN and BVN→DN pairs, while synthetic transaction healing can operate on any partition pair
2. **Signature Collection**: Anchor healing requires collecting signatures from network nodes, while synthetic transaction healing does not
3. **Version-Specific Logic**: Anchor healing has more complex version-specific logic, especially for Vandenberg-enabled networks

## Conclusion

Anchor healing is a critical process in Accumulate that ensures cryptographic commitments between partitions are properly maintained. By understanding this process, developers and AI systems can better work with the healing code and contribute to its improvement.

In the next document, we will explore the API layers that the healing processes interact with, including Tendermint APIs, original Accumulate APIs, v2 APIs, and v3 APIs.
