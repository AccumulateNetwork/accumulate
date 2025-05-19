---
title: Implementation Plan for List Proofs for Anchors
description: A detailed analysis of what would be required to implement list proofs for anchors in Accumulate
tags: [accumulate, anchors, merkle-proofs, list-proofs, implementation-plan]
created: 2025-05-16
version: 1.0
---

# Implementation Plan for List Proofs for Anchors

## 1. Introduction

This document outlines what would be required to implement list proofs specifically for anchors in Accumulate. Anchors are critical for cross-network communication, and using list proofs for anchors would provide significant benefits in terms of efficiency, reliability, and security. This implementation plan builds on Accumulate's existing Stateful Merkle Trees and ReceiptList mechanisms.

## 2. Current Anchor Implementation

### 2.1 Anchor Structure and Processing

Currently, anchors in Accumulate:

1. Are stored in the `AnchorSequenceChain` for each destination
2. Contain a sequence number for ordering
3. Are processed individually by the receiving network
4. Require individual signature verification

```go
// Current anchor sequence chain access
func (c *Account) AnchorSequenceChain(destination string) *Chain2 {
    return values.GetOrCreate(c, &c.anchorSequenceChain, (*Account).newAnchorSequenceChain)
}
```

### 2.2 Anchor Transmission

The current anchor transmission process:

1. Each validator generates and signs an anchor
2. The anchor is sent to the destination network
3. The destination network verifies the signature
4. The anchor is processed if valid

This approach requires individual signature verification for each anchor, which can be inefficient when processing multiple anchors.

## 3. Requirements for List Proofs for Anchors

### 3.1 Core Requirements

To implement list proofs for anchors, we would need:

1. **Anchor Collection**: A mechanism to collect anchors in sequence
2. **Proof Generation**: A function to generate list proofs for a range of anchors
3. **Proof Distribution**: A method to distribute the proofs to destination networks
4. **Proof Verification**: A process for verifying and processing anchor list proofs
5. **Integration with Existing Code**: Updates to the anchor processing pipeline

### 3.2 Technical Requirements

From a technical perspective, the implementation would require:

1. **Data Structures**: Define structures for anchor list proofs
2. **API Extensions**: Extend the API to support anchor list proofs
3. **Database Updates**: Ensure the database can store and retrieve anchor list proofs
4. **Consensus Updates**: Modify the consensus mechanism to support anchor list proofs

## 4. Implementation Plan

### 4.1 Phase 1: Data Structures and Core Functions

#### 4.1.1 Anchor List Proof Structure

```go
// AnchorListProof represents a proof of ordered anchors
type AnchorListProof struct {
    // Source network
    Source *url.URL
    // Destination network
    Destination *url.URL
    // Starting sequence number
    StartSequence uint64
    // Ending sequence number
    EndSequence uint64
    // Receipt list proving the order of anchors
    ReceiptList *merkle.ReceiptList
    // Chain state at the time of proof generation
    ChainState *merkle.State
    // Block height at the time of proof generation
    BlockHeight int64
}

// SignedAnchorListProof represents a signed proof of ordered anchors
type SignedAnchorListProof struct {
    // The anchor list proof
    Proof *AnchorListProof
    // Signature of the proof
    Signature protocol.Signature
    // Signer identity
    SignerUrl *url.URL
}
```

#### 4.1.2 Anchor List Proof Generation

```go
func GenerateAnchorListProof(batch *database.Batch, source, destination *url.URL, startSeq, endSeq uint64) (*protocol.AnchorListProof, error) {
    // Get the anchor sequence chain for the destination
    sequenceChain := batch.Account(protocol.AnchorPool).AnchorSequenceChain(destination.String())
    
    // Get the chain head to verify sequence range is valid
    head, err := sequenceChain.Head().Get()
    if err != nil {
        return nil, err
    }
    
    if endSeq >= uint64(head.Count) {
        return nil, errors.New("end sequence exceeds chain length")
    }
    
    // Generate a receipt list for the sequence range
    receiptList, err := merkle.GetReceiptList(sequenceChain, int64(startSeq), int64(endSeq))
    if err != nil {
        return nil, err
    }
    
    // Get the current block height
    blockHeight, err := batch.Account(protocol.Network).MainChain().Height()
    if err != nil {
        return nil, err
    }
    
    // Create the proof
    proof := &protocol.AnchorListProof{
        Source:         source,
        Destination:    destination,
        StartSequence:  startSeq,
        EndSequence:    endSeq,
        ReceiptList:    receiptList,
        ChainState:     head.Copy(),
        BlockHeight:    blockHeight,
    }
    
    return proof, nil
}
```

#### 4.1.3 Anchor List Proof Signing

```go
func SignAnchorListProof(proof *protocol.AnchorListProof, signer protocol.Signer, signerUrl *url.URL) (*protocol.SignedAnchorListProof, error) {
    // Serialize the proof
    data, err := proof.MarshalBinary()
    if err != nil {
        return nil, err
    }
    
    // Sign the proof
    signature, err := signer.Sign(data)
    if err != nil {
        return nil, err
    }
    
    // Create signed proof
    signedProof := &protocol.SignedAnchorListProof{
        Proof:     proof,
        Signature: signature,
        SignerUrl: signerUrl,
    }
    
    return signedProof, nil
}
```

### 4.2 Phase 2: Integration with Anchor Processing

#### 4.2.1 Anchor Collection and Proof Generation

```go
func CollectAndProveAnchors(batch *database.Batch, executor *execute.Executor, source, destination *url.URL, maxCount int) (*protocol.SignedAnchorListProof, error) {
    // Get the anchor ledger
    ledger := batch.Account(protocol.AnchorPool).AnchorLedger()
    partLedger := ledger.Anchor(destination)
    
    // Determine the sequence range
    startSeq := partLedger.Delivered + 1
    endSeq := min(startSeq+uint64(maxCount-1), partLedger.Received)
    
    // If no anchors to prove, return nil
    if endSeq < startSeq {
        return nil, nil
    }
    
    // Generate the proof
    proof, err := GenerateAnchorListProof(batch, source, destination, startSeq, endSeq)
    if err != nil {
        return nil, err
    }
    
    // Get the validator's signer
    signer, signerUrl, err := executor.GetValidatorSigner()
    if err != nil {
        return nil, err
    }
    
    // Sign the proof
    signedProof, err := SignAnchorListProof(proof, signer, signerUrl)
    if err != nil {
        return nil, err
    }
    
    return signedProof, nil
}
```

#### 4.2.2 Anchor List Proof Verification and Processing

```go
func VerifyAndProcessAnchorListProof(batch *database.Batch, executor *execute.Executor, signedProof *protocol.SignedAnchorListProof) error {
    // Verify the signature
    data, err := signedProof.Proof.MarshalBinary()
    if err != nil {
        return err
    }
    
    // Verify the signer is a valid validator
    if !executor.IsValidator(signedProof.SignerUrl) {
        return errors.New("signer is not a validator")
    }
    
    // Verify the signature
    if !signedProof.Signature.Verify(data) {
        return errors.New("invalid signature")
    }
    
    // Verify the receipt list
    if !signedProof.Proof.ReceiptList.Validate(nil) {
        return errors.New("invalid receipt list")
    }
    
    // Check sequence numbers against local ledger
    ledger := batch.Account(protocol.AnchorPool).AnchorLedger()
    partLedger := ledger.Anchor(signedProof.Proof.Source)
    
    if signedProof.Proof.StartSequence <= partLedger.Delivered {
        return errors.New("proof contains already delivered anchors")
    }
    
    if signedProof.Proof.StartSequence > partLedger.Delivered+1 {
        // Store for later processing when the gap is filled
        return storeForLaterProcessing(batch, signedProof)
    }
    
    // Process anchors in order
    for i, element := range signedProof.Proof.ReceiptList.Elements {
        seq := signedProof.Proof.StartSequence + uint64(i)
        
        // Parse the element to get the anchor
        var anchor protocol.Anchor
        err = anchor.UnmarshalBinary(element)
        if err != nil {
            return err
        }
        
        // Process the anchor
        err = processAnchor(batch, executor, &anchor, seq)
        if err != nil {
            return err
        }
        
        // Update the ledger
        partLedger.Add(true, seq, anchor.TxID)
    }
    
    // Commit the batch
    return batch.Commit()
}
```

### 4.3 Phase 3: API and Network Protocol Updates

#### 4.3.1 API Extensions

```go
// Add API endpoints for anchor list proofs
func RegisterAnchorListProofAPI(router *mux.Router) {
    router.HandleFunc("/v2/anchor-list-proofs", GetAnchorListProofs).Methods("GET")
    router.HandleFunc("/v2/anchor-list-proofs", SubmitAnchorListProof).Methods("POST")
}

// Get anchor list proofs
func GetAnchorListProofs(w http.ResponseWriter, r *http.Request) {
    // Implementation
}

// Submit an anchor list proof
func SubmitAnchorListProof(w http.ResponseWriter, r *http.Request) {
    // Implementation
}
```

#### 4.3.2 Network Protocol Updates

```go
// Register anchor list proof message types
func RegisterAnchorListProofMessages() {
    messaging.RegisterMessageType(messaging.MessageTypeAnchorListProof, func() messaging.Message {
        return new(messaging.AnchorListProofMessage)
    })
}

// AnchorListProofMessage represents a message containing an anchor list proof
type AnchorListProofMessage struct {
    messaging.MessageBase
    Proof *protocol.SignedAnchorListProof
}
```

## 5. Implementation Considerations

### 5.1 Performance Considerations

1. **Proof Size**: Anchor list proofs can become large for long sequences
2. **Verification Overhead**: Verifying a list proof is more complex than verifying a single anchor
3. **Storage Requirements**: Storing proofs requires additional database space

To address these concerns:

1. **Optimize Proof Size**: Limit the number of anchors in a single proof
2. **Batch Processing**: Process multiple proofs in parallel
3. **Pruning**: Remove old proofs after they are processed

### 5.2 Security Considerations

1. **Validator Authentication**: Ensure only valid validators can generate proofs
2. **Proof Integrity**: Ensure proofs cannot be tampered with
3. **Replay Protection**: Prevent replay attacks using sequence numbers

To address these concerns:

1. **Validator Registry**: Maintain a registry of valid validators
2. **Cryptographic Verification**: Use strong cryptographic verification
3. **Sequence Tracking**: Track processed sequence numbers

### 5.3 Compatibility Considerations

1. **Backward Compatibility**: Ensure compatibility with existing anchor processing
2. **Forward Compatibility**: Design for future extensions
3. **Network Upgrades**: Coordinate network upgrades to support anchor list proofs

To address these concerns:

1. **Dual-Mode Operation**: Support both individual anchors and anchor list proofs
2. **Versioning**: Include version information in proofs
3. **Phased Rollout**: Roll out support gradually across the network

## 6. Benefits of List Proofs for Anchors

### 6.1 Efficiency Improvements

1. **Reduced Signature Verification**: One signature verification for multiple anchors
2. **Reduced Network Traffic**: Fewer messages for the same number of anchors
3. **Batch Processing**: Process multiple anchors in a single operation

### 6.2 Security Improvements

1. **Guaranteed Ordering**: Anchors are cryptographically proven to be in sequence
2. **Reduced Attack Surface**: Fewer messages means fewer opportunities for attacks
3. **Improved Validation**: More comprehensive validation of anchor sequences

### 6.3 Reliability Improvements

1. **Simplified Healing**: Easier to detect and request missing anchors
2. **Consistent Processing**: Anchors are processed in the correct order
3. **Reduced Failure Modes**: Fewer points of failure in the anchor processing pipeline

## 7. Implementation Timeline

### 7.1 Phase 1: Design and Prototyping (2 weeks)

1. Finalize data structures and interfaces
2. Develop prototype implementations
3. Create test cases and validation criteria

### 7.2 Phase 2: Core Implementation (3 weeks)

1. Implement anchor list proof generation
2. Implement anchor list proof verification
3. Implement anchor list proof processing

### 7.3 Phase 3: Integration and Testing (3 weeks)

1. Integrate with existing anchor processing
2. Develop comprehensive tests
3. Perform performance and security testing

### 7.4 Phase 4: Deployment and Monitoring (2 weeks)

1. Deploy to test networks
2. Monitor performance and security
3. Address any issues identified

Total estimated timeline: 10 weeks

## 8. Conclusion

Implementing list proofs for anchors in Accumulate would provide significant benefits in terms of efficiency, security, and reliability. By leveraging the existing Stateful Merkle Trees and ReceiptList mechanisms, this implementation can be achieved with moderate effort and minimal disruption to existing functionality.

The key components required are:
1. Data structures for anchor list proofs
2. Functions for generating and verifying proofs
3. Integration with the existing anchor processing pipeline
4. API and network protocol updates

With these components in place, Accumulate would have a more efficient and reliable mechanism for cross-network communication through anchors.

## 9. References

1. [Stateful Merkle Trees in Accumulate](../implementation/04_stateful_merkle_trees.md)
2. [Enhanced List Proofs for Sequenced Transactions](23_enhanced_list_proofs_for_sequenced_transactions.md)
3. [Deterministic Anchor Transmission](12_deterministic_anchor_transmission.md)
4. [Merkle Chain Implementation](../../pkg/database/merkle/chain.go)
5. [Receipt List Implementation](../../pkg/database/merkle/receipt_list.go)
