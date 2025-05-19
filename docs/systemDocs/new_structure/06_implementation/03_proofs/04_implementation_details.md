# Implementation Details

This document provides technical details about the implementation of anchor proofs and receipts in Accumulate.

## Code Structure

The anchor proof system is implemented across several packages:

- `pkg/api/v3` - API definitions for proof retrieval
- `pkg/database` - Storage and retrieval of proof data
- `pkg/protocol` - Core data structures for proofs
- `pkg/client` - Client-side verification utilities

## Key Data Structures

```go
// Receipt represents a complete receipt with all information needed for verification
type Receipt struct {
    Anchor         *protocol.AnchorLedger
    Proof          *MerkleProof
    SourceNetwork  string
    DestNetwork    string
    Timestamp      time.Time
}

// MerkleProof contains the path and siblings for verification
type MerkleProof struct {
    RootHash  []byte
    TxHash    []byte
    Path      []byte
    Siblings  [][]byte
}
```

## Algorithms

### Proof Generation

```go
func GenerateProof(txHash []byte, merkleState *MerkleState) (*MerkleProof, error) {
    // Find the leaf node containing the transaction
    leaf, err := merkleState.FindLeaf(txHash)
    if err != nil {
        return nil, err
    }
    
    // Generate the path from leaf to root
    path, siblings, err := merkleState.GeneratePath(leaf)
    if err != nil {
        return nil, err
    }
    
    // Create and return the proof
    return &MerkleProof{
        RootHash: merkleState.Root(),
        TxHash:   txHash,
        Path:     path,
        Siblings: siblings,
    }, nil
}
```

### Proof Verification

```go
func VerifyProof(proof *MerkleProof) (bool, error) {
    // Start with the transaction hash
    currentHash := proof.TxHash
    
    // Apply each step in the path
    for i, direction := range proof.Path {
        sibling := proof.Siblings[i]
        
        // Combine the current hash with its sibling according to the path direction
        if direction == 0 {
            currentHash = hashPair(currentHash, sibling)
        } else {
            currentHash = hashPair(sibling, currentHash)
        }
    }
    
    // Verify the final hash matches the root hash
    return bytes.Equal(currentHash, proof.RootHash), nil
}
```

## Performance Optimizations

Several optimizations have been implemented:

1. **Batch Processing**: Multiple proofs can be generated and verified in a single operation
2. **Caching**: Frequently accessed proofs and intermediate nodes are cached
3. **Parallel Processing**: Proof generation and verification can be parallelized
4. **Compression**: Proof data is compressed to reduce storage and transmission costs
