package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func main() {
	endpoint := "http://127.0.0.1:26660/v3"
	client := jsonrpc.NewClient(endpoint)
	client.Client.Timeout = 10 * time.Second
	
	testAddr := "acc://test1234567890abcdef1234567890abcdef12345678/ACME"
	u, err := url.Parse(testAddr)
	if err != nil {
		log.Fatal("Failed to parse URL:", err)
	}
	
	ctx := context.Background()
	
	fmt.Println("Starting rapid faucet test (like CLI does)...")
	
	// Submit 100 faucet requests rapidly like the CLI does
	start := time.Now()
	for i := 0; i < 100; i++ {
		sub, err := client.Faucet(ctx, u, api.FaucetOptions{})
		if err != nil {
			fmt.Printf("Faucet %d failed: %v\n", i+1, err)
			continue
		}
		if sub != nil && sub.Status != nil && sub.Status.TxID != nil {
			// Just print progress, don't wait
			if i%10 == 0 {
				fmt.Printf("Submitted %d faucet requests...\n", i+1)
			}
		}
	}
	
	elapsed := time.Since(start)
	fmt.Printf("\nSubmitted 100 faucet requests in %v\n", elapsed)
	fmt.Printf("Rate: %.2f requests/second\n", 100.0/elapsed.Seconds())
	
	// Now wait and check balance
	fmt.Println("\nWaiting 15 seconds for transactions to settle...")
	time.Sleep(15 * time.Second)
	
	// Check balance
	fmt.Println("Checking balance...")
	query, err := client.Query(ctx, u, &api.DefaultQuery{})
	if err != nil {
		fmt.Printf("Query failed: %v\n", err)
	} else {
		fmt.Printf("Account state: %+v\n", query)
	}
}