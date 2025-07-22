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
		fmt.Println("Usage: light-client <server-url>")
		fmt.Println("Example: light-client https://mainnet.accumulatenetwork.io")
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

	// Collect operators key book and pages
	fmt.Println("=== Accumulate Light Client ===")
	fmt.Printf("Server: %s\n", serverURL)
	fmt.Println("\nCollecting operators keybook...")

	operators, err := client.GetOperators(ctx)
	if err != nil {
		log.Fatalf("Failed to collect operators: %v", err)
	}

	// Display results
	fmt.Println("\n=== Operators KeyBook Summary ===")
	fmt.Printf("KeyBook URL: %s\n", operators.KeyBook.URL)
	fmt.Printf("KeyBook Type: %s\n", operators.KeyBook.Type)
	fmt.Printf("KeyBook Threshold: %d\n", operators.KeyBook.Threshold)
	fmt.Printf("Number of Key Pages: %d\n", len(operators.KeyPages))
	fmt.Printf("Total Keys: %d\n", len(operators.AllKeys))

	fmt.Println("\n=== Key Pages ===")
	for i, page := range operators.KeyPages {
		fmt.Printf("\nPage %d: %s\n", i+1, page.URL)
		fmt.Printf("  Type: %s\n", page.Type)
		fmt.Printf("  Threshold: %d\n", page.Threshold)
		fmt.Printf("  Keys (%d):\n", len(page.Keys))
		for j, key := range page.Keys {
			fmt.Printf("    %d. %s\n", j+1, key)
		}
	}

	fmt.Println("\n=== All Keys (Flattened) ===")
	for i, key := range operators.AllKeys {
		fmt.Printf("  %d. %s\n", i+1, key)
	}

	fmt.Println("\nSuccessfully collected operators keybook and all key pages!")
}
