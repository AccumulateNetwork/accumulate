package main

import (
	"context"
	"fmt"
	"time"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
)

func main() {
	endpoint := "http://127.0.0.1:26660/v3"
	fmt.Printf("Testing endpoint: %s\n", endpoint)
	
	client := jsonrpc.NewClient(endpoint)
	client.Client.Timeout = 5 * time.Second
	
	ctx := context.Background()
	
	// Test network status
	status, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
	if err != nil {
		fmt.Printf("ERROR getting network status: %v\n", err)
		return
	}
	
	fmt.Printf("SUCCESS: Oracle price: %d\n", status.Oracle.Price)
	
	// Test faucet
	fmt.Println("\nTesting faucet...")
	faucetResp, err := client.Faucet(ctx, nil, api.FaucetOptions{})
	if err != nil {
		fmt.Printf("ERROR calling faucet: %v\n", err)
		return
	}
	
	if faucetResp != nil {
		fmt.Printf("Faucet response: %+v\n", faucetResp)
		fmt.Printf("Faucet submission success: %v\n", faucetResp.Success)
	}
}