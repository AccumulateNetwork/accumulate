package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/pkg/lightclient"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: staking-client <server-url>")
		fmt.Println("Example: staking-client https://mainnet.accumulatenetwork.io")
		fmt.Println("")
		fmt.Println("Available server shortcuts:")
		fmt.Println("  local    - http://127.0.1.1:26660")
		fmt.Println("  testnet  - https://testnet.accumulatenetwork.io")
		fmt.Println("  beta     - https://beta.testnet.accumulatenetwork.io")
		fmt.Println("  canary   - https://canary.testnet.accumulatenetwork.io")
		fmt.Println("  mainnet  - http://apollo-mainnet.accumulate.defidevs.io:16595")
		fmt.Println("  mainnet-ssl - https://mainnet.accumulatenetwork.io")
		os.Exit(1)
	}

	serverURL := os.Args[1]

	// Create light client
	client, err := lightclient.NewClient(serverURL)
	if err != nil {
		log.Fatalf("Failed to create light client: %v", err)
	}

	ctx := context.Background()

	// Collect staking registry and accounts
	fmt.Println("=== Accumulate Staking Client ===")
	fmt.Printf("Server: %s\n", serverURL)
	fmt.Println("\nCollecting staking registry...")

	stakingURLs, stakingAccounts, err := client.GetStakingAccounts(ctx)
	if err != nil {
		log.Fatalf("Failed to collect staking accounts: %v", err)
	}

	// Display results
	fmt.Println("\n=== Staking Registry Summary ===")
	fmt.Printf("Registry URL: acc://staking.acme/registered\n")
	fmt.Printf("Number of registered staking accounts: %d\n", len(stakingURLs))
	fmt.Printf("Successfully retrieved: %d\n", len(stakingAccounts))

	fmt.Println("\n=== Registered Staking Account URLs ===")
	for i, stakingURL := range stakingURLs {
		fmt.Printf("  %d. %s\n", i+1, stakingURL)
	}

	fmt.Println("\n=== Staking Account Details ===")
	totalStaked := int64(0)
	for i, account := range stakingAccounts {
		fmt.Printf("\nAccount %d: %s\n", i+1, account.URL)
		fmt.Printf("  Type: %s\n", account.Type)
		fmt.Printf("  Balance: %d\n", account.Balance)
		fmt.Printf("  Token URL: %s\n", account.TokenURL)
		fmt.Printf("  Authorities (%d):\n", len(account.Authorities))
		for j, auth := range account.Authorities {
			fmt.Printf("    %d. %s\n", j+1, auth)
		}
		totalStaked += account.Balance
	}

	fmt.Println("\n=== Summary ===")
	fmt.Printf("Total registered staking accounts: %d\n", len(stakingURLs))
	fmt.Printf("Successfully retrieved accounts: %d\n", len(stakingAccounts))
	fmt.Printf("Total staked tokens: %d\n", totalStaked)

	fmt.Println("\nSuccessfully collected staking registry and account details!")
}
