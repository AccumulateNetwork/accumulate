---
title: Deterministic BVN-to-BVN Synthetic Transactions - Design Options
description: Analysis of design options for implementing deterministic synthetic transactions between Block Validator Networks
tags: [accumulate, synthetic-transactions, cross-network, deterministic, bvn-to-bvn]
created: 2025-05-16
version: 1.0
---

# Deterministic BVN-to-BVN Synthetic Transactions - Design Options

## Introduction

While the deterministic anchor transmission design addresses communication between the Directory Network (DN) and Block Validator Networks (BVNs), this document explores options for extending the deterministic approach to synthetic transactions between different BVNs.

The BVN-to-BVN communication pattern presents unique challenges compared to DN-BVN communication, as there is no natural hierarchy between BVNs. This document outlines several potential approaches, their advantages, and their limitations.

## Current Synthetic Transaction Flow

In the current Accumulate architecture:

1. **DN → BVN**: Directory Network sends anchors to Block Validator Networks
2. **BVN → DN**: Block Validator Networks send anchors to Directory Network
3. **BVN → BVN**: Block Validator Networks communicate directly with each other for cross-BVN transactions

The deterministic approach we've designed works well for the first two cases because there's a clear hierarchy and relationship. For BVN-to-BVN, we need to consider additional factors.

## Design Options for BVN-to-BVN Deterministic Transactions

### Option 1: Direct Deterministic Transmission

Extend the same deterministic approach where all validators in the source BVN independently generate and sign identical transactions targeting the destination BVN.

#### Implementation Approach

```go
// File: internal/core/crosschain/deterministic.go

// GenerateBvnToBvnTransaction generates a deterministic synthetic transaction for BVN-to-BVN communication
func (g *DeterministicTxGenerator) GenerateBvnToBvnTransaction(ctx context.Context, sourceChain, destChain *url.URL, message protocol.TransactionBody) (*protocol.Transaction, error) {
    // Generate deterministic transaction ID based on source, destination, and message content
    txid := g.generateDeterministicTxID(sourceChain, destChain, message)
    
    // Build transaction with the deterministic ID
    txn := &protocol.Transaction{
        Header: protocol.TransactionHeader{
            Principal: destChain,
        },
        Body: message,
    }
    
    // Set the deterministic ID
    txn.Header.SetTxID(txid)
    
    return txn, nil
}
```

#### Advantages

1. **Consistent Approach**: Uses the same deterministic methodology as DN-BVN communication
2. **No Single Point of Failure**: Multiple validators ensure reliability
3. **Leverages Existing Code**: Can reuse much of the deterministic anchor transmission code

#### Limitations

1. **Network Traffic**: Could lead to network flooding if many validators are involved
2. **Cross-BVN Coordination**: Requires coordination between different BVN validator sets

### Option 2: Validator-Only Transmission

Restrict synthetic transaction transmission to validators only, reducing network traffic while maintaining deterministic properties.

#### Implementation Approach

```go
// File: internal/core/crosschain/bvn_transmitter.go (new file to create)

// BvnToBvnTransmitter handles deterministic transmission between BVNs
type BvnToBvnTransmitter struct {
    config TransmitterConfig
    generator *DeterministicTxGenerator
    sigManager *SignatureManager
    validatorSet *ValidatorSetManager
}

// Transmit sends a synthetic transaction to another BVN
func (t *BvnToBvnTransmitter) Transmit(ctx context.Context, source, destination *url.URL, message protocol.TransactionBody) error {
    // Check if this node is a validator
    isValidator, err := t.validatorSet.IsValidator(t.config.ValidatorKey.Public())
    if err != nil {
        return err
    }
    
    // Only validators participate in cross-BVN transmission
    if !isValidator {
        t.config.Logger.Debug("Node is not a validator, skipping BVN-to-BVN transmission")
        return nil
    }
    
    // Generate deterministic transaction
    txn, err := t.generator.GenerateBvnToBvnTransaction(ctx, source, destination, message)
    if err != nil {
        return err
    }
    
    // Check if transaction already has sufficient signatures
    sufficient, err := t.sigManager.HasSufficientSignatures(ctx, txn.ID())
    if err != nil && !errors.Is(err, errors.NotFound) {
        return err
    }
    
    if sufficient {
        t.config.Logger.Debug("Transaction already has sufficient signatures", "txid", txn.ID())
        return nil
    }
    
    // Sign and submit
    return t.sigManager.SignAndSubmit(ctx, txn)
}
```

#### Advantages

1. **Restricted Participation**: Only validators participate, reducing network traffic
2. **Deterministic Generation**: Maintains deterministic transaction generation
3. **Simplified Implementation**: No need for complex routing or bridge nodes

#### Limitations

1. **Validator Set Knowledge**: Each BVN needs to know the validator set of other BVNs
2. **Cross-BVN Authentication**: Validators need to authenticate across BVN boundaries

### Option 3: BVN Bridge Nodes

Designate specific "bridge nodes" in each BVN that are responsible for cross-BVN communication.

#### Implementation Approach

```go
// File: internal/core/crosschain/bridge.go (new file to create)

// BvnBridgeNode handles cross-BVN communication
type BvnBridgeNode struct {
    config BridgeConfig
    generator *DeterministicTxGenerator
    sigManager *SignatureManager
}

// IsBridgeNode determines if this node is a designated bridge node
func (b *BvnBridgeNode) IsBridgeNode() bool {
    // Deterministic selection based on validator key and BVN ID
    seed := append(b.config.ValidatorKey.Public(), []byte(b.config.BvnID)...)
    hash := sha256.Sum256(seed)
    
    // Select bridge nodes based on hash value
    // For example, nodes with hash < threshold become bridge nodes
    threshold := big.NewInt(0).Exp(big.NewInt(2), big.NewInt(255), nil)
    threshold = threshold.Div(threshold, big.NewInt(int64(b.config.BridgeNodeCount)))
    
    hashInt := big.NewInt(0).SetBytes(hash[:])
    return hashInt.Cmp(threshold) < 0
}

// TransmitCrossBvn sends a synthetic transaction to another BVN
func (b *BvnBridgeNode) TransmitCrossBvn(ctx context.Context, source, destination *url.URL, message protocol.TransactionBody) error {
    // Only bridge nodes participate in cross-BVN transmission
    if !b.IsBridgeNode() {
        b.config.Logger.Debug("Node is not a bridge node, skipping BVN-to-BVN transmission")
        return nil
    }
    
    // Generate deterministic transaction
    txn, err := b.generator.GenerateBvnToBvnTransaction(ctx, source, destination, message)
    if err != nil {
        return err
    }
    
    // Sign and submit
    return b.sigManager.SignAndSubmit(ctx, txn)
}
```

#### Advantages

1. **Reduced Network Traffic**: Fewer nodes involved in cross-BVN communication
2. **Deterministic Selection**: Bridge nodes can be selected deterministically
3. **Clear Responsibility**: Specific nodes have clear cross-BVN responsibilities

#### Limitations

1. **Potential Points of Failure**: If bridge nodes fail, communication is disrupted
2. **More Complex Implementation**: Requires bridge node selection and management
3. **Monitoring Overhead**: Need to monitor bridge node health and performance

### Option 4: DN-Mediated Communication

Route all BVN-to-BVN communication through the Directory Network.

#### Implementation Approach

```go
// File: internal/core/crosschain/dn_mediated.go (new file to create)

// DnMediatedTransmitter routes BVN-to-BVN communication through the DN
type DnMediatedTransmitter struct {
    config TransmitterConfig
    generator *DeterministicTxGenerator
    sigManager *SignatureManager
}

// Transmit sends a synthetic transaction to another BVN via the DN
func (t *DnMediatedTransmitter) Transmit(ctx context.Context, source, destination *url.URL, message protocol.TransactionBody) error {
    // First, create a transaction to the DN
    dnUrl := protocol.DnUrl()
    
    // Create a wrapper message that contains the ultimate destination
    wrapper := &protocol.BvnToBvnMessage{
        Source:      source,
        Destination: destination,
        Message:     message,
    }
    
    // Generate deterministic transaction to the DN
    txn, err := t.generator.GenerateBvnToBvnTransaction(ctx, source, dnUrl, wrapper)
    if err != nil {
        return err
    }
    
    // Sign and submit to the DN
    err = t.sigManager.SignAndSubmit(ctx, txn)
    if err != nil {
        return err
    }
    
    // The DN will forward the message to the destination BVN
    t.config.Logger.Debug("BVN-to-BVN message sent via DN", 
        "source", source, 
        "destination", destination,
        "txid", txn.ID())
    
    return nil
}
```

#### Advantages

1. **Leverages Existing Paths**: Uses established DN-BVN communication channels
2. **Simplified Routing**: DN provides a natural coordination point
3. **Consistent Security Model**: Uses the same security model as DN-BVN communication

#### Limitations

1. **Increased DN Load**: Adds additional load to the Directory Network
2. **Additional Latency**: Adds an extra hop to BVN-to-BVN communication
3. **DN Dependency**: Makes BVN-to-BVN communication dependent on DN availability

### Option 5: Deterministic Routing with Threshold Signatures

Use deterministic routing combined with threshold signatures to optimize network traffic.

#### Implementation Approach

```go
// File: internal/core/crosschain/threshold.go (new file to create)

// ThresholdTransmitter uses threshold signatures for BVN-to-BVN communication
type ThresholdTransmitter struct {
    config TransmitterConfig
    generator *DeterministicTxGenerator
    thresholdSigner *ThresholdSigner
    validatorSet *ValidatorSetManager
}

// Transmit sends a synthetic transaction with threshold signatures
func (t *ThresholdTransmitter) Transmit(ctx context.Context, source, destination *url.URL, message protocol.TransactionBody) error {
    // Check if this node is a validator
    isValidator, err := t.validatorSet.IsValidator(t.config.ValidatorKey.Public())
    if err != nil {
        return err
    }
    
    // Only validators participate in threshold signing
    if !isValidator {
        t.config.Logger.Debug("Node is not a validator, skipping threshold signing")
        return nil
    }
    
    // Generate deterministic transaction
    txn, err := t.generator.GenerateBvnToBvnTransaction(ctx, source, destination, message)
    if err != nil {
        return err
    }
    
    // Participate in threshold signing
    err = t.thresholdSigner.SignAndSubmitShare(ctx, txn)
    if err != nil {
        return err
    }
    
    t.config.Logger.Debug("Threshold signature share submitted", 
        "source", source, 
        "destination", destination,
        "txid", txn.ID())
    
    return nil
}
```

#### Advantages

1. **Optimized Network Traffic**: Threshold signatures reduce the number of signatures needed
2. **Strong Security**: Maintains security through threshold cryptography
3. **Deterministic Generation**: Preserves deterministic transaction generation

#### Limitations

1. **Complex Cryptography**: Requires implementation of threshold signature schemes
2. **Setup Complexity**: Threshold schemes require more complex setup
3. **Key Management**: More complex key management requirements

## Comparison of Approaches

| Approach | Complexity | Network Efficiency | Reliability | Implementation Effort |
|----------|------------|-------------------|-------------|----------------------|
| Direct Deterministic | Low | Low | High | Low |
| Validator-Only | Low | Medium | High | Low |
| BVN Bridge Nodes | Medium | High | Medium | Medium |
| DN-Mediated | Low | Medium | Medium | Low |
| Threshold Signatures | High | High | High | High |

## Recommendation

Based on the analysis of the different options, the **Validator-Only Transmission** approach offers the best balance of simplicity, reliability, and network efficiency. It maintains the deterministic properties we want while limiting the number of nodes involved in cross-network communication.

This approach:
1. Restricts participation to validators only
2. Uses the same deterministic transaction generation methodology
3. Leverages Accumulate's existing signature validation mechanism
4. Requires minimal changes to the existing deterministic anchor transmission design

## Next Steps

1. Finalize the design choice for BVN-to-BVN synthetic transactions
2. Integrate the chosen approach with the deterministic anchor transmission system
3. Implement and test the BVN-to-BVN communication mechanism
4. Update the healing process to handle BVN-to-BVN synthetic transactions

## References

1. [Deterministic Anchor Transmission](12_deterministic_anchor_transmission.md)
2. [Deterministic Anchor Detailed Design](13_deterministic_anchor_detailed_design.md)
3. [Deterministic Anchor Abstraction](14_deterministic_anchor_abstraction.md)
4. [Cross-Network Communication](../implementation/02_cross_network_communication.md)
