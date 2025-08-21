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
	// Test different endpoints
	endpoints := []string{
		"http://127.0.0.1:26660/v3",
		"http://localhost:26660/v3",
		"http://127.0.0.1:26660",
	}

	testAddr := "acc://bd821c8b1badf253ee97ba84bf210d16aee1c6829baf84ed/ACME"
	u, err := url.Parse(testAddr)
	if err != nil {
		log.Fatal("Failed to parse URL:", err)
	}

	for _, endpoint := range endpoints {
		fmt.Printf("\n=== Testing endpoint: %s ===\n", endpoint)
		
		client := jsonrpc.NewClient(endpoint)
		client.Client.Timeout = 10 * time.Second
		
		ctx := context.Background()
		
		// Test network status
		fmt.Println("Testing network status...")
		_, err = client.NetworkStatus(ctx, api.NetworkStatusOptions{})
		if err != nil {
			fmt.Printf("Network status failed: %v\n", err)
			continue
		}
		fmt.Printf("Network status OK\n")
		
		// Test faucet
		fmt.Println("Testing faucet...")
		sub, err := client.Faucet(ctx, u, api.FaucetOptions{})
		if err != nil {
			fmt.Printf("Faucet failed: %v\n", err)
			continue
		}
		
		if sub != nil && sub.Status != nil {
			fmt.Printf("Faucet submission: %+v\n", sub.Status)
			if sub.Status.TxID != nil {
				fmt.Printf("Transaction ID: %v\n", sub.Status.TxID)
			}
		}
		
		fmt.Println("SUCCESS!")
		break
	}
}