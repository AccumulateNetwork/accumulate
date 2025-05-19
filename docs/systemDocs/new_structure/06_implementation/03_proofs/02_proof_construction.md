# Proof Construction

This document details how proofs are constructed in the Accumulate blockchain system.

## Merkle Proof Construction

Accumulate uses Merkle trees to construct proofs of transaction inclusion. The process involves:

1. Hashing the transaction data
2. Placing the hash in a Merkle tree
3. Constructing a path from the transaction hash to the Merkle root
4. Including sibling hashes needed for verification

## Anchor Chain Inclusion

Once a Merkle root is calculated for a block of transactions:

1. The root is included in an anchor transaction
2. The anchor transaction is submitted to the target chain
3. The anchor transaction creates a link between the source and target chains
4. This link can be cryptographically verified

## Proof Data Structure

A proof in Accumulate consists of:

```
Proof {
    AnchorHash      []byte
    RootHash        []byte
    TxHash          []byte
    Path            [][]byte
    Siblings        [][]byte
    SourceNetwork   string
    DestNetwork     string
    BlockHeight     uint64
    BlockTime       time.Time
}
```

## Optimizations

Several optimizations are employed to make proofs more efficient:

1. Path compression to reduce proof size
2. Batch processing of proofs
3. Caching of intermediate nodes
4. Parallel verification of multiple proofs
