package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ExampleV3Fix shows how to fix v3 connection issues in existing code
func main() {
	fmt.Println("========================================")
	fmt.Println("   V3 CONNECTION FIX EXAMPLE")
	fmt.Println("========================================")
	fmt.Println()
	
	// BEFORE: This pattern causes connection issues
	fmt.Println("BAD PATTERN (causes connection exhaustion):")
	fmt.Println("--------------------------------------------")
	showBadPattern()
	
	fmt.Println("\nGOOD PATTERN (with connection pooling):")
	fmt.Println("----------------------------------------")
	showGoodPattern()
	
	fmt.Println("\nBEST PATTERN (with retry logic):")
	fmt.Println("---------------------------------")
	showBestPattern()
}

func showBadPattern() {
	// BAD: Creates new client for each operation
	for i := 0; i < 3; i++ {
		// This creates a new HTTP client each time!
		client := jsonrpc.NewClient("http://127.0.0.1:26660/v3")
		
		ctx := context.Background()
		resp, err := client.NodeInfo(ctx, api.NodeInfoOptions{})
		if err != nil {
			fmt.Printf("  Request %d failed: %v\n", i+1, err)
		} else {
			fmt.Printf("  Request %d succeeded: %s\n", i+1, resp.Network)
		}
	}
}

func showGoodPattern() {
	// GOOD: Use pooled client that reuses connections
	client := GetPooledClient("http://127.0.0.1:26660/v3")
	
	for i := 0; i < 3; i++ {
		ctx, cancel := CreateContextWithTimeout(30 * time.Second)
		resp, err := client.NodeInfo(ctx, api.NodeInfoOptions{})
		cancel()
		
		if err != nil {
			fmt.Printf("  Request %d failed: %v\n", i+1, err)
		} else {
			fmt.Printf("  Request %d succeeded: %s\n", i+1, resp.Network)
		}
	}
}

func showBestPattern() {
	// BEST: Use pooled client with retry logic
	client := GetPooledClient("http://127.0.0.1:26660/v3")
	
	for i := 0; i < 3; i++ {
		var resp *api.NodeInfo
		
		err := SafeQuery(client, func(ctx context.Context) error {
			var err error
			resp, err = client.NodeInfo(ctx, api.NodeInfoOptions{})
			return err
		})
		
		if err != nil {
			fmt.Printf("  Request %d failed after retries: %v\n", i+1, err)
		} else {
			fmt.Printf("  Request %d succeeded: %s\n", i+1, resp.Network)
		}
	}
}

// Example of how to fix the recovery code
func fixedRecoveryExample() {
	fmt.Println("\nExample: Fixed Recovery Code")
	fmt.Println("-----------------------------")
	
	// Use pooled client instead of creating new one
	client := GetPooledClient("http://127.0.0.1:26660/v3")
	Q := api.Querier2{Querier: client}
	
	// Query with proper timeout and retry
	var ledger *protocol.AnchorLedger
	err := SafeQuery(client, func(ctx context.Context) error {
		partUrl := protocol.PartitionUrl("BVN1")
		anchorUrl := partUrl.JoinPath(protocol.AnchorPool)
		
		resp, err := Q.QueryAccount(ctx, anchorUrl, nil)
		if err != nil {
			return err
		}
		
		var ok bool
		ledger, ok = resp.Account.(*protocol.AnchorLedger)
		if !ok {
			return fmt.Errorf("not an anchor ledger")
		}
		return nil
	})
	
	if err != nil {
		log.Printf("Failed to query ledger: %v", err)
	} else {
		fmt.Printf("Successfully queried ledger (type: %s)\n", ledger.Type())
	}
}

// Template for updating existing test files:
//
// 1. Replace:
//    client := jsonrpc.NewClient("http://127.0.0.1:26660/v3")
// With:
//    client := GetPooledClient("http://127.0.0.1:26660/v3")
//
// 2. Replace:
//    ctx := context.Background()
// With:
//    ctx, cancel := CreateContextWithTimeout(30 * time.Second)
//    defer cancel()
//
// 3. For critical operations, wrap with retry:
//    err := SafeQuery(client, func(ctx context.Context) error {
//        // your query here
//        return err
//    })