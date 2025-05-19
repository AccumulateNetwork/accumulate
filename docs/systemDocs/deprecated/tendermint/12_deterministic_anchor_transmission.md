---
title: Deterministic Anchor Transmission for Accumulate
description: A design for a deterministic anchor transmission system where all validators can independently generate and sign identical transactions, eliminating single points of failure and improving reliability
tags: [accumulate, anchoring, cross-network, deterministic, reliability, consensus, validators]
created: 2025-05-16
version: 1.0
---

# Deterministic Anchor Transmission for Accumulate

## Introduction

This document proposes a deterministic approach to anchor transmission between Accumulate networks that addresses the reliability challenges in the current leader-based model. By enabling all validators to independently generate and sign identical transactions, this approach eliminates single points of failure and significantly improves the reliability of cross-network communication.

## Current System Limitations

The current anchor transmission system in Accumulate has several limitations:

1. **Single Leader Dependency**: Only the leader validator submits anchors, creating a single point of failure
2. **Detection Challenges**: Lost transactions are difficult to detect
3. **Complex Healing**: Requires complex healing mechanisms to ensure eventual consistency
4. **Asymmetric Knowledge**: Not all validators know about all transactions that should be submitted

## Deterministic Anchor Transmission Design

### Core Principles

1. **Deterministic Transaction Generation**: All validators independently generate identical anchors
2. **Universal Participation**: Every validator can submit signatures for the transaction
3. **Dual-Network Validators**: Validators run both DN and at least one BVN node
4. **Standard RPC Interface**: Uses existing Accumulate RPC for transaction submission
5. **Separate Signature Submission**: Signatures can be submitted separately to the same transaction

### System Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                                                             │
│                  Validator Node (DN + BVN)                  │
│                                                             │
│  ┌───────────────┐          ┌───────────────────────────┐  │
│  │               │          │                           │  │
│  │  DN Instance  │◄────────►│  BVN Instance            │  │
│  │               │          │                           │  │
│  └───────┬───────┘          └───────────┬───────────────┘  │
│          │                              │                   │
│          ▼                              ▼                   │
│  ┌───────────────┐          ┌───────────────────────────┐  │
│  │ Deterministic │          │ Deterministic             │  │
│  │ Anchor        │          │ Anchor                    │  │
│  │ Generator     │          │ Generator                 │  │
│  └───────┬───────┘          └───────────┬───────────────┘  │
│          │                              │                   │
│          ▼                              ▼                   │
│  ┌───────────────┐          ┌───────────────────────────┐  │
│  │ Transaction   │          │ Transaction               │  │
│  │ Signer        │          │ Signer                    │  │
│  └───────┬───────┘          └───────────┬───────────────┘  │
│          │                              │                   │
└──────────┼──────────────────────────────┼───────────────────┘
           │                              │
           ▼                              ▼
    ┌─────────────────┐          ┌─────────────────┐
    │                 │          │                 │
    │  DN Network     │          │  BVN Network    │
    │                 │          │                 │
    └─────────────────┘          └─────────────────┘
```

## Implementation Details

### 1. Deterministic Anchor Generation

All validators must generate identical anchors for the same block. This requires:

```go
// DeterministicAnchorGenerator generates anchors deterministically
// from block state that all validators can access
type DeterministicAnchorGenerator struct {
    // Access to chain state
    database *database.Batch
    
    // Network information
    networkID string
    partitionID string
}

// GenerateAnchor creates an anchor deterministically from the block state
func (g *DeterministicAnchorGenerator) GenerateAnchor(blockIndex uint64) (protocol.AnchorBody, error) {
    // Load system ledger state at the specified block
    var systemLedger *protocol.SystemLedger
    err := g.database.Account(protocol.PartitionUrl(g.partitionID).JoinPath(protocol.Ledger)).
        MainChain().
        Entry(int64(blockIndex)).
        GetAs(&systemLedger)
    if err != nil {
        return nil, err
    }
    
    // Get the root chain state
    rootChain, err := g.database.Account(protocol.PartitionUrl(g.partitionID).JoinPath(protocol.Ledger)).
        RootChain().
        Get()
    if err != nil {
        return nil, err
    }
    
    // Get the state root at this block
    stateRoot, err := g.database.GetBptRootHash(blockIndex)
    if err != nil {
        return nil, err
    }
    
    // Construct the anchor deterministically
    // All validators will produce the same anchor given the same inputs
    anchor := &protocol.PartitionAnchor{
        Source:          protocol.PartitionUrl(g.partitionID),
        RootChainIndex:  blockIndex,
        RootChainAnchor: *(*[32]byte)(rootChain.Anchor()),
        StateTreeAnchor: stateRoot,
        MinorBlockIndex: systemLedger.Index,
        MajorBlockIndex: systemLedger.MajorBlockIndex,
        Timestamp:       systemLedger.Timestamp, // Use block timestamp for determinism
    }
    
    return anchor, nil
}
```

### 2. Deterministic Transaction Construction

Once the anchor is generated, a deterministic transaction must be constructed:

```go
// ConstructAnchorTransaction creates a deterministic transaction from an anchor
func ConstructAnchorTransaction(anchor protocol.AnchorBody, destination *url.URL) *protocol.Transaction {
    // Create a deterministic transaction
    txn := new(protocol.Transaction)
    txn.Header.Principal = destination.JoinPath(protocol.AnchorPool)
    txn.Body = anchor
    
    // Generate a deterministic transaction ID
    // This ensures all validators create the same transaction ID
    txid := url.TxID{
        URL: txn.Header.Principal.Copy(),
        Hash: sha256.Sum256(anchor.GetPartitionAnchor().StateTreeAnchor),
    }
    txn.Header.Initiator = txid.URL
    
    return txn
}
```

### 3. Independent Signature Submission

Each validator independently signs and submits its signature:

```go
// SignAndSubmitAnchorTransaction signs and submits a signature for an anchor transaction
func SignAndSubmitAnchorTransaction(ctx context.Context, txn *protocol.Transaction, validatorKey ed25519.PrivateKey) error {
    // Create a signature builder
    builder := new(signing.Builder).
        SetType(protocol.SignatureTypeED25519).
        SetPrivateKey(validatorKey).
        SetUrl(protocol.DnUrl().JoinPath(protocol.Network)).
        SetTimestampToNow()
    
    // Sign the transaction hash
    txHash := txn.GetHash()
    sig, err := builder.Sign(txHash[:])
    if err != nil {
        return err
    }
    
    // Create a signature message
    sigMsg := &messaging.SignatureMessage{
        TransactionHash: txHash,
        Signature:       sig,
    }
    
    // Create an envelope with just the signature
    env := &messaging.Envelope{
        Messages: []messaging.Message{sigMsg},
    }
    
    // Submit the signature via RPC
    client := api.NewClient(api.ClientOptions{
        URL: "http://localhost:26660/v2", // Use local RPC endpoint
    })
    
    _, err = client.Submit(ctx, env, api.SubmitOptions{
        Wait: api.BoolPtr(true), // Wait for confirmation
    })
    
    return err
}
```

### 4. Transaction Monitoring and Verification

All validators monitor the transaction status to ensure it's properly processed:

```go
// MonitorAnchorTransaction monitors the status of an anchor transaction
func MonitorAnchorTransaction(ctx context.Context, txid *url.TxID) error {
    client := api.NewClient(api.ClientOptions{
        URL: "http://localhost:26660/v2", // Use local RPC endpoint
    })
    
    // Poll for transaction status
    for {
        status, err := client.QueryTransaction(ctx, txid, nil)
        if err != nil {
            if errors.Is(err, errors.NotFound) {
                // Transaction not found yet, continue polling
                time.Sleep(1 * time.Second)
                continue
            }
            return err
        }
        
        // Check if transaction is delivered
        if status.Status.Delivered() {
            return nil
        }
        
        // Check for errors
        if status.Status.Error != nil {
            return status.Status.Error
        }
        
        // Continue polling
        time.Sleep(1 * time.Second)
    }
}
```

### 5. Integration with Block Processing

The deterministic anchor transmission is integrated into the block processing flow:

```go
// ProcessBlockForAnchoring processes a block for anchor transmission
func (node *Node) ProcessBlockForAnchoring(ctx context.Context, blockIndex uint64) error {
    // Generate the anchor deterministically
    generator := &DeterministicAnchorGenerator{
        database:    node.Database,
        networkID:   node.NetworkID,
        partitionID: node.PartitionID,
    }
    
    anchor, err := generator.GenerateAnchor(blockIndex)
    if err != nil {
        return err
    }
    
    // Determine the destination
    var destination *url.URL
    if node.PartitionID == protocol.Directory {
        // DN -> BVN
        // In this case, we would generate multiple transactions, one for each BVN
        for _, bvn := range node.Network.Partitions {
            if bvn.Type == protocol.PartitionTypeBlockValidator {
                destination = protocol.PartitionUrl(bvn.ID)
                
                // Construct the transaction
                txn := ConstructAnchorTransaction(anchor, destination)
                
                // Sign and submit
                err = SignAndSubmitAnchorTransaction(ctx, txn, node.ValidatorKey)
                if err != nil {
                    return err
                }
                
                // Monitor transaction (could be done asynchronously)
                go MonitorAnchorTransaction(ctx, txn.ID())
            }
        }
    } else {
        // BVN -> DN
        destination = protocol.DnUrl()
        
        // Construct the transaction
        txn := ConstructAnchorTransaction(anchor, destination)
        
        // Sign and submit
        err = SignAndSubmitAnchorTransaction(ctx, txn, node.ValidatorKey)
        if err != nil {
            return err
        }
        
        // Monitor transaction (could be done asynchronously)
        go MonitorAnchorTransaction(ctx, txn.ID())
    }
    
    return nil
}
```

## Benefits of Deterministic Anchor Transmission

### 1. Elimination of Single Points of Failure

Since all validators can independently generate and sign the same transaction, there is no dependency on a single leader. If any validator fails, others can still ensure the anchor is transmitted.

### 2. Improved Reliability

The probability of successful anchor transmission increases dramatically as multiple validators attempt to sign and submit the same transaction. With N validators, the system can tolerate up to N-1 validator failures.

### 3. Simplified Detection of Missing Anchors

Since all validators know exactly which anchors should exist, detection of missing anchors becomes trivial. Each validator can verify that anchors for all blocks have been properly transmitted.

### 4. Reduced Need for Complex Healing

With multiple validators submitting signatures, the likelihood of needing healing mechanisms is significantly reduced. Healing can be simplified to focus only on rare edge cases.

### 5. Consistent System State

All validators have a consistent view of which anchors should be transmitted, eliminating asymmetric knowledge problems in the current system.

## Implementation Considerations

### 1. Transaction ID Generation

For this approach to work, all validators must generate the same transaction ID for the same anchor. This requires a deterministic method of generating transaction IDs based on the anchor content.

### 2. Signature Aggregation

To optimize performance, the system could implement signature aggregation, where multiple validator signatures are combined into a single signature before submission.

### 3. Timing Considerations

Validators should implement a small random delay before submitting signatures to avoid network congestion from all validators submitting simultaneously.

### 4. Fallback Mechanisms

While this approach significantly improves reliability, fallback mechanisms should still be implemented for extreme edge cases where all validators might fail.

### 5. Network Partition Handling

The system should handle network partitions gracefully, ensuring that anchors are eventually transmitted once connectivity is restored.

## Migration Path

### Phase 1: Implementation (2 weeks)

1. **Week 1: Core Implementation**
   - Implement deterministic anchor generation
   - Develop deterministic transaction construction
   - Create signature submission mechanism

2. **Week 2: Integration and Testing**
   - Integrate with block processing
   - Develop monitoring and verification
   - Test with multiple validators

### Phase 2: Deployment (2 weeks)

1. **Week 1: Testnet Deployment**
   - Deploy to testnet
   - Monitor performance and reliability
   - Address any issues identified

2. **Week 2: Mainnet Rollout**
   - Gradual rollout to mainnet
   - Parallel operation with existing system
   - Complete transition to deterministic approach

## Conclusion

The deterministic anchor transmission approach offers a simple yet powerful solution to the reliability challenges in Accumulate's cross-network communication. By leveraging the deterministic nature of blockchain state and enabling all validators to participate in anchor transmission, this approach eliminates single points of failure and significantly improves reliability.

This solution builds on Accumulate's existing architecture and requires minimal changes to the core system, making it a pragmatic and efficient approach to enhancing cross-network communication reliability.

## References

1. [Accumulate Cross-Network Communication](../implementation/02_cross_network_communication.md)
2. [Anchor Proofs and Receipts](../implementation/03_anchor_proofs_and_receipts.md)
3. [CometBFT P2P Networking](06_cometbft_p2p_networking.md)
