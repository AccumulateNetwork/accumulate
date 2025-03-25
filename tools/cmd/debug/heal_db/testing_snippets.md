# Testing the Documentation Code Snippets

This document provides guidance on how to test the code snippets from the in-memory database documentation.

## Prerequisites

Before testing the code snippets, ensure you have:

1. A working Go environment
2. Access to the Accumulate codebase
3. Proper import paths configured

## Testing Approaches

### 1. Integration Tests

The most thorough way to test the code snippets is to create integration tests that verify the behavior described in the documentation.

Create a test file in the appropriate package:

```go
package database_test

import (
	"context"
	"testing"

	"gitlab.com/AccumulateNetwork/accumulate/internal/database"
	"gitlab.com/AccumulateNetwork/accumulate/pkg/url"
	"gitlab.com/AccumulateNetwork/accumulate/protocol"
)

func TestAccountChains(t *testing.T) {
	ctx := context.Background()
	
	// Create an in-memory database for testing
	db := database.OpenInMemory(nil)
	defer db.Close()
	
	// Parse the account URL
	accountUrl, err := url.Parse("acc://redwagon/token")
	if err != nil {
		t.Fatalf("parsing URL: %v", err)
	}
	
	// Get a database batch
	batch := database.Begin(ctx, db)
	defer batch.Discard()
	
	// Create the account if it doesn't exist
	account := batch.Account(accountUrl)
	if err := account.Put(new(protocol.TokenAccount)); err != nil {
		t.Fatalf("creating account: %v", err)
	}
	
	// Access standard chains
	mainChain := account.MainChain()
	if mainChain == nil {
		t.Fatal("main chain is nil")
	}
	
	// Test other chains and operations as needed
}
```

### 2. Manual Testing in the REPL

You can test individual snippets using the Go REPL:

```bash
# Start the Go REPL
cd /path/to/accumulate
go run github.com/traefik/yaegi/cmd/yaegi
```

Then in the REPL:

```go
import (
	"context"
	"gitlab.com/AccumulateNetwork/accumulate/internal/database"
	"gitlab.com/AccumulateNetwork/accumulate/pkg/url"
)

ctx := context.Background()
db := database.OpenInMemory(nil)
accountUrl, _ := url.Parse("acc://redwagon/token")
batch := database.Begin(ctx, db)
account := batch.Account(accountUrl)
// Continue with other operations
```

### 3. Testing API Snippets

For API-related snippets, you can create a small program that connects to the testnet:

```go
package main

import (
	"context"
	"fmt"
	"os"

	"gitlab.com/AccumulateNetwork/accumulate/pkg/api/v3"
	"gitlab.com/AccumulateNetwork/accumulate/pkg/url"
)

func main() {
	ctx := context.Background()
	
	// Create a client for testnet
	client := api.NewClient("https://testnet.accumulatenetwork.io/v3")
	
	// Parse the account URL - use a known account on testnet
	accountUrl, _ := url.Parse("acc://testnet")
	
	// Query a chain
	chainQuery := &api.ChainQuery{
		ChainID: "main",
	}
	
	chain, err := client.QueryChain(ctx, accountUrl, chainQuery)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error querying chain: %v\n", err)
		os.Exit(1)
	}
	
	fmt.Printf("Chain %s has %d entries\n", chain.Name, chain.Count)
}
```

## Validating Documentation Accuracy

To ensure the documentation is accurate:

1. **Verify imports**: Make sure all import paths are correct
2. **Check method signatures**: Ensure method names and parameters match the actual code
3. **Test with real accounts**: When possible, test with real accounts on testnet
4. **Validate error handling**: Test error cases to ensure error handling examples are correct

## Common Issues and Solutions

### Import Path Issues

If you encounter import path issues, ensure you're using the correct import paths:

```go
// Correct import paths
import (
	"gitlab.com/AccumulateNetwork/accumulate/internal/database"
	"gitlab.com/AccumulateNetwork/accumulate/pkg/api/v3"
	"gitlab.com/AccumulateNetwork/accumulate/pkg/url"
	"gitlab.com/AccumulateNetwork/accumulate/protocol"
)
```

### API Connection Issues

If you have trouble connecting to the API:

1. Ensure the endpoint URL is correct
2. Try using a known account that exists on the network
3. Check network connectivity
4. Verify that the testnet or mainnet is operational

### Database Operations

When testing database operations:

1. Always create the account before trying to access its chains
2. Remember to commit batches if you want changes to persist
3. Use proper error handling for all operations
