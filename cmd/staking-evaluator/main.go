package main

import (
	"context"
	"fmt"
	"log"

	"gitlab.com/accumulatenetwork/accumulate/exp/light"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func main() {
	ctx := context.Background()

	// Create mainnet connections
	fmt.Println("Connecting to Accumulate mainnet...")
	
	// Create v2 client for mainnet
	v2Client, err := client.New("mainnet")
	if err != nil {
		log.Fatalf("Failed to create v2 client: %v", err)
	}

	// Create v3 JSON-RPC client for mainnet  
	v3Client := jsonrpc.NewClient("https://mainnet.accumulatenetwork.io/v3")

	// Create experimental light client
	lightClient, err := light.NewClient(
		light.Store(memory.New(nil), "staking-evaluator"),
		light.ClientV2(v2Client),
		light.Querier(v3Client),
	)
	if err != nil {
		log.Fatalf("Failed to create light client: %v", err)
	}

	// Target account: staking.acme/registered
	stakingURL, err := url.Parse("staking.acme/registered")
	if err != nil {
		log.Fatalf("Failed to parse staking URL: %v", err)
	}

	fmt.Printf("Evaluating data account: %s\n", stakingURL)
	fmt.Println("=" + string(make([]byte, 50)) + "=")

	// Pull the staking account data
	fmt.Println("Pulling staking account data...")
	err = lightClient.PullAccount(ctx, stakingURL)
	if err != nil {
		log.Fatalf("Failed to pull staking account: %v", err)
	}

	// Query the account to get its current state
	fmt.Println("Querying account state...")
	record, err := api.Querier2{Querier: v3Client}.QueryAccount(ctx, stakingURL, nil)
	if err != nil {
		log.Fatalf("Failed to query account: %v", err)
	}

	// Check if it's a data account
	dataAccount, ok := record.Account.(*protocol.DataAccount)
	if !ok {
		log.Fatalf("Account is not a data account, got: %T", record.Account)
	}

	fmt.Printf("Data Account: %s\n", dataAccount.Url)
	fmt.Printf("Data Account Type: %s\n", dataAccount.Type())
	fmt.Println()

	// Print the data entries from the account
	if dataAccount.Entry != nil {
		fmt.Printf("Data account entry type: %T\n", dataAccount.Entry)
		
		// Check if it's an AccumulateDataEntry
		if accumEntry, ok := dataAccount.Entry.(*protocol.AccumulateDataEntry); ok {
			fmt.Printf("Found AccumulateDataEntry with %d data fields:\n", len(accumEntry.Data))
			
			for i, data := range accumEntry.Data {
				fmt.Printf("Entry %d: %s\n", i, string(data))
			}
		} else {
			// For other data entry types, use the GetData method
			dataFields := dataAccount.Entry.GetData()
			fmt.Printf("Found %d data fields:\n", len(dataFields))
			
			for i, data := range dataFields {
				fmt.Printf("Entry %d: %s\n", i, string(data))
			}
		}
	} else {
		fmt.Println("No data entry found in the account")
	}

	// Summary
	fmt.Println("\n" + "=" + string(make([]byte, 80)) + "=")
	fmt.Printf("Summary: Processed data account %s\n", stakingURL)
	
	// Try to get additional account information
	fmt.Println("\nAccount Details:")
	fmt.Printf("  URL: %s\n", dataAccount.Url)
	fmt.Printf("  Authority: %v\n", dataAccount.Authorities)
	// Entry details are shown above
	
	if len(dataAccount.Authorities) > 0 {
		fmt.Printf("  Authorities:\n")
		for _, auth := range dataAccount.Authorities {
			fmt.Printf("    - %s\n", auth)
		}
	}

	fmt.Println("\nEvaluation complete!")
}

// isPrintable checks if data contains only printable ASCII characters
func isPrintable(data []byte) bool {
	for _, b := range data {
		if b < 32 || b > 126 {
			return false
		}
	}
	return true
}
