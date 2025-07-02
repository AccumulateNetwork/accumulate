# How the Lite Client Extracts Info from Major Blocks

This document explains, at a high level, how the lite client extracts and tracks information from each major block in the Accumulate blockchain. The focus is on authority set extraction and tracking, which is foundational for block and signature validation.

## 1. Overview

- **Major blocks** are the top-level blocks in Accumulate, each grouping many minor blocks and recording the validator set (authority set) and its signatures.
- The lite client must extract the authority set from each major block to validate signatures and track changes in network governance.

## 2. Step-by-Step Extraction Process

1. **Query Major Blocks**
   - The client queries major blocks sequentially from genesis (index 0) to the latest.
2. **Extract Authority Sets**
   - For each major block, the client extracts the authority set (keys, threshold, and index).
3. **Build Authority Tracker**
   - The client builds an `AuthorityTracker` to map block indices to their respective authority sets, capturing all changes over time.

## 3. Reference Code Snippets

### Querying and Extracting Authority Sets
```go
var authoritySets []*signatures.AuthoritySet
for i := uint64(0); ; i++ {
    majorBlocks, err := blocks.QueryMajorBlocks(ctx, c.v2, i, 1)
    if err != nil {
        return fmt.Errorf("failed to query major block %d: %w", i, err)
    }
    if len(majorBlocks) == 0 {
        // No more blocks
        break
    }
    authSet, err := blocks.ExtractAuthoritySet(majorBlocks[0])
    if err != nil {
        return fmt.Errorf("failed to extract AuthoritySet for block %d: %w", i, err)
    }
    authoritySets = append(authoritySets, authSet)
}
```

### Building the Authority Tracker
```go
authorityTracker, err := blocks.BuildAuthorityTracker(authoritySets)
if err != nil {
    return fmt.Errorf("failed to build authority tracker: %w", err)
}
```

## 4. Key Data Structures

```go
type AuthoritySet struct {
    Keys      [][]byte // Validator public keys
    Threshold uint64   // Required signatures
    Index     uint64   // Block index
}

type AuthorityTracker struct {
    history map[uint64]*AuthoritySet // Maps block index to authority set
}
```

## 5. Summary

- The lite client walks through all major blocks, extracting each authority set.
- It builds a tracker to map when and how the authority set changes.
- This enables the lite client to validate signatures and authority transitions efficiently and securely.

For more details, see the implementation in `blocks/block_major.go` and `signatures/authority.go`.
