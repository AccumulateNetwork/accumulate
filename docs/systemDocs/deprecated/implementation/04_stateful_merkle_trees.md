---
title: Stateful Merkle Trees in Accumulate
description: An in-depth analysis of Stateful Merkle Tree implementation and how they support chains and proofs
tags: [accumulate, merkle-trees, smt, chains, proofs, implementation]
created: 2025-05-16
version: 1.0
---

# Stateful Merkle Trees in Accumulate

## 1. Introduction

Stateful Merkle Trees (SMTs) are a fundamental data structure in Accumulate that provide the foundation for building and maintaining information in Merkle Trees across multiple blocks. Unlike traditional Merkle Trees used in many blockchains, SMTs in Accumulate allow for maintaining Merkle Trees that span many blocks and even enable the creation of Merkle Trees of Merkle Trees.

This document explores the implementation of SMTs in Accumulate, focusing on how they support chains and proofs, which are critical for cross-network communication and transaction validation.

## 2. Core Concepts

### 2.1 Stateful Merkle Trees

A Stateful Merkle Tree maintains its state across multiple blocks, allowing for:

1. **Continuous Growth**: The tree can grow indefinitely, with new entries being added over time
2. **Historical State Access**: The state of the tree at any point in its history can be reconstructed
3. **Efficient Proofs**: Proofs can be generated for any entry or range of entries in the tree

### 2.2 Mark Points

SMTs in Accumulate use a concept called "mark points" to efficiently store and retrieve historical states:

```go
c.markPower = markPower                             // # levels in Merkle Tree to be indexed
c.markFreq = int64(math.Pow(2, float64(markPower))) // The number of elements between indexes
c.markMask = c.markFreq - 1                         // Mask to index of next mark (0 if at a mark)
```

Mark points are stored at regular intervals (powers of 2) and contain the complete state of the Merkle tree at that point. This allows for efficient reconstruction of any historical state without storing every intermediate state.

### 2.3 Chains as Merkle Trees

In Accumulate, chains are implemented as Merkle trees. Each chain maintains:

1. **Ordered Entries**: Entries are added to the chain in sequence
2. **Merkle Root**: The root hash of the Merkle tree representing the current state
3. **Historical States**: States at mark points for historical reconstruction

This implementation allows chains to provide cryptographic proofs of inclusion and ordering for their entries.

## 3. Implementation Details

### 3.1 Chain Structure

The core implementation of chains in Accumulate is found in `pkg/database/merkle/chain.go`. A Chain is a Merkle tree with additional functionality for maintaining state:

```go
type Chain struct {
    logger   values.LazyValue[log.Logger]
    store    record.Store
    key      *record.Key
    name     string
    typ      ChainType
    markPower int64
    markFreq  int64
    markMask  int64
}
```

Key components:
- **markPower**: Determines how frequently states are stored (2^markPower)
- **markFreq**: The number of elements between mark points
- **markMask**: Bit mask for determining if an index is at a mark point

### 3.2 Adding Entries to a Chain

When an entry is added to a chain, it's incorporated into the Merkle tree:

```go
func (m *Chain) AddEntry(hash []byte, unique bool) error {
    head, err := m.Head().Get() // Get the current state
    if err != nil {
        return err
    }

    // ... (index the hash)

    switch (head.Count + 1) & m.markMask {
    case 0: // Is this the end of the Mark set, i.e. 0, ..., markFreq-1
        head.AddEntry(hash)                                     // Add the hash to the Merkle Tree
        err = m.States(uint64(head.Count) - 1).Put(head.Copy()) // Save Merkle State at n*MarkFreq-1
        if err != nil {
            return err
        }
    case 1: //                              After MarkFreq elements are written
        head.HashList = head.HashList[:0] // then clear the HashList
        fallthrough                       // then fall through as normal
    default:
        head.AddEntry(hash) // 0 to markFeq-2, always add to the merkle tree
    }

    // ... (update head)
    return nil
}
```

Key operations:
1. Get the current head state
2. Add the hash to the Merkle tree
3. If at a mark point, save the complete state
4. Update the head state

### 3.3 Merkle State

The state of a Merkle tree is represented by the `State` structure:

```go
type State struct {
    Count    int64    // Number of elements in the Merkle Tree
    HashList [][]byte // The most recent hashes added to the Merkle Tree
    Pending  [][]byte // The intermediate hashes in the Merkle Tree
}
```

- **Count**: The number of elements in the tree
- **HashList**: Recent hashes added to the tree (up to markFreq)
- **Pending**: Intermediate hashes in the tree (internal nodes)

### 3.4 Retrieving Historical States

The chain implementation allows for retrieving the state at any point in its history:

```go
func (m *Chain) StateAt(element int64) (ms *State, err error) {
    // ... (validation)
    
    // Find the nearest mark point before the requested element
    MINext := (element+1)&^m.markMask - 1
    if MINext < 0 {
        MINext = 0
    }
    
    // Get the state at the mark point
    NMark, err := m.States(uint64(MINext)).Get()
    if errors.Is(err, errors.NotFound) {
        // ... (handle not found)
    } else if err != nil {
        return nil, err
    }
    
    // Clone the state
    cState := NMark.Copy()
    
    // Replay entries from the mark point to the requested element
    for i := MINext + 1; i <= element; i++ {
        hash, err := m.Entry(i)
        if err != nil {
            return nil, err
        }
        cState.AddEntry(hash)
    }
    
    return cState, nil
}
```

This function:
1. Finds the nearest mark point before the requested element
2. Retrieves the state at that mark point
3. Replays entries from the mark point to the requested element
4. Returns the reconstructed state

## 4. Proofs in Accumulate

### 4.1 Receipt Structure

Proofs in Accumulate are implemented as "receipts" that prove the inclusion of an element in a Merkle tree:

```go
type Receipt struct {
    Start      []byte         // The starting hash (usually the hash being proven)
    StartIndex int64          // The index of the starting hash
    End        []byte         // The ending hash (usually the anchor)
    EndIndex   int64          // The index of the ending hash
    Anchor     []byte         // The anchor hash (Merkle root)
    Entries    []*ReceiptEntry // The path from Start to Anchor
}

type ReceiptEntry struct {
    Hash  []byte // The hash to combine with
    Right bool   // Whether the hash is on the right
}
```

A receipt contains:
- The hash being proven (Start)
- The Merkle root (Anchor)
- The path from the hash to the root (Entries)

### 4.2 Receipt Validation

Receipts can be validated to verify the inclusion of an element:

```go
func (r *Receipt) Validate(opts *ValidateOptions) bool {
    MDRoot := r.Start // To begin with, we start with the object as the MDRoot
    // Now apply all the path hashes to the MDRoot
    for _, node := range r.Entries {
        if !opts.allowEntry(node) {
            return false
        }
        MDRoot = node.apply(MDRoot)
    }
    // In the end, MDRoot should be the same hash the receipt expects.
    return bytes.Equal(MDRoot, r.Anchor)
}
```

This function:
1. Starts with the hash being proven
2. Applies each entry in the path
3. Verifies that the result matches the expected Merkle root

### 4.3 Receipt Lists

For proving multiple elements in sequence, Accumulate uses "receipt lists":

```go
type ReceiptList struct {
    // MerkleState at the beginning of the list
    MerkleState *State
    // Elements in the list, in order
    Elements [][]byte
    // Receipt proving the last element in Elements
    Receipt *Receipt
    // Optional receipt that proves the anchor of Receipt
    ContinuedReceipt *Receipt
}
```

A receipt list proves:
1. The inclusion of multiple elements in a Merkle tree
2. The order of those elements
3. The connection to a specific Merkle root

### 4.4 Generating Receipt Lists

Receipt lists are generated from a chain using the `GetReceiptList` function:

```go
func GetReceiptList(manager *Chain, Start int64, End int64) (r *ReceiptList, err error) {
    // ... (validation)
    
    // Allocate the ReceiptList, add all the elements to the ReceiptList
    r = NewReceiptList()
    for i := Start; i <= End; i++ { // Get all the elements for the list
        h, err := manager.Entry(i)
        if err != nil {
            return nil, err
        }
        r.Elements = append(r.Elements, copyHash(h))
    }
    
    r.MerkleState, err = manager.StateAt(Start - 1)
    if err != nil {
        return nil, err
    }
    lastElement := append([]byte{}, r.Elements[len(r.Elements)-1]...)
    r.Receipt, err = getReceipt(manager, lastElement, lastElement)
    if err != nil {
        return nil, err
    }
    
    return r, nil
}
```

This function:
1. Collects all elements from Start to End
2. Gets the Merkle state at Start-1
3. Generates a receipt for the last element
4. Returns the complete receipt list

### 4.5 Validating Receipt Lists

Receipt lists can be validated to verify the inclusion and order of multiple elements:

```go
func (r *ReceiptList) Validate(opts *ValidateOptions) bool {
    // Make sure the ReceiptList isn't empty
    if r.MerkleState == nil ||
        r.Receipt == nil ||
        r.Elements == nil || len(r.Elements) == 0 {
        return false
    }
    
    // Start with the initial Merkle state and add each element
    MS := r.MerkleState.Copy()
    for _, h := range r.Elements {
        if h == nil || len(h) != 32 {
            return false
        }
        MS.AddEntry(append([]byte{}, h...))
    }
    
    // Compute the anchor at this point
    anchor := MS.Anchor()
    if len(anchor) == 0 {
        return false
    }
    
    // Verify the last element matches the receipt's start
    lastElement := r.Elements[len(r.Elements)-1]
    if !bytes.Equal(lastElement, r.Receipt.Start) {
        return false
    }
    
    // Verify the anchor matches the receipt's anchor
    if !bytes.Equal(r.Receipt.Anchor, anchor) {
        return false
    }
    
    // Validate the receipt
    if !r.Receipt.Validate(opts) {
        return false
    }
    
    // ... (validate continued receipt if present)
    
    return true
}
```

This validation ensures:
1. All elements are valid
2. The elements produce the expected Merkle state
3. The receipt correctly proves the last element
4. The receipt anchors to the expected Merkle root

## 5. Applications in Accumulate

### 5.1 Sequence Chains

Sequence chains in Accumulate are implemented as chains (Merkle trees) that store ordered sequences of transactions:

```go
func (c *Account) AnchorSequenceChain() *Chain2 {
    return values.GetOrCreate(c, &c.anchorSequenceChain, (*Account).newAnchorSequenceChain)
}

func (c *Account) SyntheticSequenceChain(partition string) *Chain2 {
    return c.getSyntheticSequenceChain(strings.ToLower(partition))
}
```

These chains maintain:
1. The order of transactions
2. The ability to prove inclusion and ordering
3. The ability to reconstruct historical states

### 5.2 Cross-Network Communication

For cross-network communication, Accumulate can use receipt lists to prove the order of transactions:

1. A source network adds transactions to its sequence chain
2. A receipt list is generated for a range of transactions
3. The receipt list is signed and sent to the destination network
4. The destination network validates the receipt list
5. The destination network processes the transactions in the proven order

This approach ensures:
1. Transactions are processed in the correct order
2. Only valid transactions are accepted
3. The source network cannot repudiate the transactions

### 5.3 Transaction Validation

For transaction validation, Accumulate can use receipts to prove the inclusion of a transaction in a chain:

1. A transaction is added to a chain
2. A receipt is generated for the transaction
3. The receipt is used to prove the transaction's inclusion
4. The receipt can be combined with other receipts to prove anchoring

This approach allows for:
1. Efficient validation of individual transactions
2. Proof of inclusion in the blockchain
3. Verification of transaction ordering

## 6. Optimizations and Considerations

### 6.1 Mark Point Optimization

The frequency of mark points involves a trade-off:

1. **More Frequent Mark Points**:
   - Faster historical state reconstruction
   - More storage space required

2. **Less Frequent Mark Points**:
   - Less storage space required
   - Slower historical state reconstruction

Accumulate uses a configurable mark power to allow for tuning this trade-off.

### 6.2 Receipt Size Optimization

Receipt size can be optimized by:

1. **Minimizing Path Length**: Shorter paths result in smaller receipts
2. **Batch Proofs**: Using receipt lists for multiple elements
3. **Compression**: Compressing receipts for transmission

### 6.3 Performance Considerations

Key performance considerations include:

1. **State Reconstruction**: Reconstructing historical states can be expensive
2. **Receipt Generation**: Generating receipts requires traversing the Merkle tree
3. **Receipt Validation**: Validating receipts is computationally efficient

## 7. Conclusion

Stateful Merkle Trees in Accumulate provide a powerful foundation for chains and proofs. By maintaining the state of Merkle trees across multiple blocks and implementing efficient mechanisms for historical state reconstruction, Accumulate enables robust cross-network communication and transaction validation.

The combination of chains as Merkle trees and receipt lists for proving multiple elements in sequence makes Accumulate particularly well-suited for deterministic anchor transmission and BVN-to-BVN communication.

## 8. References

1. [Merkle Chain Implementation](../../pkg/database/merkle/chain.go)
2. [Receipt Implementation](../../pkg/database/merkle/receipt.go)
3. [Receipt List Implementation](../../pkg/database/merkle/receipt_list.go)
4. [BPT Implementation](../../pkg/database/bpt/bpt.go)
5. [Stateful Merkle Tree README](../../internal/database/smt/README.md)
6. [Sequence Number Management](../tendermint/17_sequence_numbers_processing_flow.md)
7. [Leveraging Sequence Chains for Ordered Transaction Proofs](../tendermint/22_leveraging_sequence_chains_for_ordered_transactions.md)
