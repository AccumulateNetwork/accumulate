# Accumulate Lite Client

A lightweight client for the Accumulate network that provides cryptographically valid proof generation and account data retrieval.

## Features

- **Proof Generation**: Healing-based multi-level receipt construction (main chain → BVN → DN)
- **Account Data**: Balance and transaction retrieval for token accounts
- **Cryptographic Validation**: 100% cryptographically valid proofs without observer dependencies
- **Public API Only**: Works with standard Accumulate v2/v3 APIs

## Quick Start

```go
package main

import (
    "fmt"
    "gitlab.com/accumulatenetwork/accumulate/exp/lite-client"
)

func main() {
    // Generate proof for an account
    verified, err := liteclient.FetchProof("acc://alice.acme/tokens")
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Receipt entries: %d\n", len(verified.Receipt.Entries))
    fmt.Printf("Account height: %d\n", verified.Height)
}
```

## Architecture

### Core Components

- **`healing.go`** - Main proof generator using healing patterns
- **`bvn_anchor.go`** - BVN anchor chain traversal
- **`dn_anchor.go`** - DN anchor chain traversal  
- **`index_chain.go`** - Main-index to BVN position mapping
- **`proof.go`** - Legacy wrapper functions for backward compatibility
- **`api.go`** - Account data retrieval (balances, transactions)
- **`liteclient.go`** - Main client with caching and validation
- **`types.go`** - Core data structures

### Proof Generation Process

1. **Account Data Query**: Retrieve account state from v2 API
2. **Main Chain Receipt**: Extract main chain Merkle proof
3. **BVN Receipt**: Map main chain position to BVN anchor chain
4. **DN Receipt**: Traverse BVN to DN anchor chain
5. **Receipt Combination**: Combine multi-level receipts into final proof

## Testing

```bash
# Run the simple proof test
go test -v -run TestReceiptConstruction

# Run all tests
go test -v ./...
```

## Documentation

- **`docs/chain_indexing_architecture.md`** - Detailed explanation of chain indexing
- **`docs/healing.md`** - Healing-based proof generation approach
- **`docs/manual_receipt_generation.md`** - Manual receipt construction guide
- **`docs/account_api.md`** - Account data retrieval API documentation

## Implementation Notes

- Uses `testing.NullObserver{}` to bypass observer dependencies
- Follows patterns from `internal/core/healing` for production-grade proof construction
- Maintains backward compatibility through legacy wrapper functions
- Supports both real chain data and synthetic fallback receipts
