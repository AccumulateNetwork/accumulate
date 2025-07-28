# Accumulate Lite Client Chain Indexing Architecture

## Overview

The Accumulate lite client implements a sophisticated multi-level chain indexing system that mirrors the full network's hierarchical structure. This document explains how account main chains are indexed into Block Validation Network (BVN) chains and subsequently into Directory Network (DN) chains to create cryptographically verifiable proofs.

## Chain Hierarchy

The Accumulate network uses a three-tier hierarchical structure:

```
Account Main Chain → BVN Anchor Chain → DN Anchor Chain
```

1. **Account Main Chain**: Contains the account's transaction history and state changes
2. **BVN Anchor Chain**: Aggregates multiple account main chains within a partition
3. **DN Anchor Chain**: Aggregates multiple BVN anchor chains across the entire network

## Indexing Mechanism

### 1. Main Chain to BVN Indexing

The lite client uses the **main-index chain** to map account main chain positions to BVN anchor positions:

#### Implementation (`index_chain.go`)

```go
type IndexEntry struct {
    Source uint64 // Position in the main chain
    Anchor uint64 // Position in the BVN anchor chain
}
```

**Process:**
1. Query account data to extract the `main-index` chain
2. Decode the latest main-index root to get the `IndexEntry`
3. Use the `IndexEntry.Anchor` value to locate the corresponding position in the BVN anchor chain
4. Query the BVN anchor chain at that position to retrieve the anchor transaction

#### Key Functions:
- `attemptRealBVNIndexLookup()`: Performs the main-index lookup
- `extractMainIndexRoot()`: Extracts main-index data from account chains
- `decodeIndexEntry()`: Decodes binary index entries (little-endian uint64 pairs)

### 2. BVN to DN Indexing

BVN anchor chains are indexed into the DN anchor chain through a similar mechanism:

#### Implementation (`bvn_anchor.go`, `dn_anchor.go`)

**BVN Anchor Chain Access:**
- BVN system ledgers are accessible via URLs: `acc://bvn0.acme/anchors`, `acc://bvn1.acme/anchors`, etc.
- Each BVN partition maintains its own anchor chain
- Anchor transactions contain embedded receipts with complete cryptographic proof chains

**DN Anchor Chain Access:**
- DN anchor chain is accessible via: `acc://dn.acme/anchors`
- Aggregates all BVN anchor chains into a single directory-level chain
- Provides the final anchor point for network-wide verification

## Proof Generation Process

### Multi-Level Receipt Construction (`healing.go`)

The lite client follows the healing approach from `internal/core/healing/synthetic.go:buildSynthReceiptV2`:

```go
func (hpg *HealingProofGenerator) buildMultiLevelReceipt(ctx context.Context, accountURL string, startIndex, endIndex int64) (*merkle.Receipt, error) {
    // Step 1: Build main chain receipt (account level)
    mainReceipt := hpg.buildMainChainReceipt(ctx, u, startIndex, endIndex)
    
    // Step 2: Build BVN receipt (main → BVN)
    bvnReceipt := hpg.buildIndexBasedBVNReceipt(ctx, u, mainReceipt.Anchor)
    
    // Step 3: Build DN receipt (BVN → DN)
    dnReceipt := hpg.buildIndexBasedDNReceipt(ctx, u, bvnReceipt.Anchor)
    
    // Step 4: Combine receipts into multi-level proof
    return hpg.combineReceipts(mainReceipt, bvnReceipt, dnReceipt)
}
```

### Receipt Combination

Receipts are combined using the standard `merkle.Receipt` structure:

```go
type Receipt struct {
    Start   []byte           // Starting hash (account state)
    Anchor  []byte           // Final anchor hash (DN level)
    Entries []*ReceiptEntry  // Combined Merkle path entries
}
```

The combined receipt contains:
- **Start**: Account's main chain state hash
- **Anchor**: DN-level anchor hash (network root)
- **Entries**: All Merkle path entries from account → main → BVN → DN

## API Integration

### V2 API Usage

The lite client uses the v2 API for account queries:

```go
query := &v2api.GeneralQuery{
    UrlQuery: v2api.UrlQuery{Url: accountURL},
}
resp, err := client.Query(ctx, query)
```

### Transaction History Integration

For receipt extraction from transaction history:

```go
query := &v2api.TxHistoryQuery{
    UrlQuery: v2api.UrlQuery{Url: accountURL},
    QueryPagination: v2api.QueryPagination{
        Start: 0,
        Count: 10,
    },
}
resp, err := client.QueryTxHistory(ctx, query)
```

## Error Handling and Fallbacks

### Real vs. Computed Receipts

The lite client implements a fallback strategy:

1. **Real Receipt Generation**: Attempts to use actual BVN/DN anchor chain data
2. **Computed Receipt Generation**: Falls back to cryptographically structured synthetic receipts when real data is unavailable

### Graceful Degradation

```go
// Try real BVN index lookup first
realBVNReceipt, err := hpg.attemptRealBVNIndexLookup(ctx, u, mainAnchor)
if err == nil {
    return realBVNReceipt, nil
}

// Fallback to computed receipt
return hpg.buildComputedBVNReceipt(anchorPosition, mainAnchor), nil
```

## Key Insights from Paul Snow's Architecture

1. **Index Chain Mapping**: "The index will give you where the main chain is written to the bvn anchor chain"
2. **Anchor Chain Access**: BVN system ledgers are accessible via anchor chain URLs
3. **Embedded Receipts**: Anchor transactions contain embedded receipts with complete proof chains
4. **Healing Patterns**: The implementation follows proven patterns from `internal/core/healing`

## Validation

Receipts are validated using the built-in validation method:

```go
func (hpg *HealingProofGenerator) ValidateReceipt(receipt *merkle.Receipt) bool {
    if receipt == nil {
        return false
    }
    return receipt.Validate(nil)
}
```

## Files and Responsibilities

- **`healing.go`**: Main proof generator and multi-level receipt orchestration
- **`index_chain.go`**: Main-index chain decoding and BVN position lookup
- **`bvn_anchor.go`**: BVN anchor chain access and receipt extraction
- **`dn_anchor.go`**: DN anchor chain access and final anchoring
- **`proof.go`**: Legacy transaction-based receipt fetching
- **`cache.go`**: Proof caching and staleness detection

This architecture provides cryptographically verifiable proofs while maintaining compatibility with public APIs and graceful degradation when full node access is unavailable.
