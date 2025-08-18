package main

import (
	"context"
	"fmt"
	"time"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
)

func main() {
	endpoints := []string{
		"http://127.0.0.1:26660/v3",
		"http://localhost:26660/v3",
	}
	
	for _, endpoint := range endpoints {
		fmt.Printf("Testing endpoint: %s\n", endpoint)
		
		client := jsonrpc.NewClient(endpoint)
		client.Client.Timeout = 2 * time.Second
		
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		
		status, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
		if err != nil {
			fmt.Printf("  ERROR: %v\n", err)
		} else {
			fmt.Printf("  SUCCESS: Oracle price: %d\n", status.Oracle.Price)
		}
	}
}