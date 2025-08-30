// Test program to verify network connectivity
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/client"
)

func main() {
	fmt.Println("=== Accumulate Network Connectivity Test ===\n")

	// Test Mainnet
	fmt.Println("1. MAINNET (https://mainnet.accumulatenetwork.io/v3)")
	fmt.Println("-----------------------------------------------")
	testNetwork(client.NewMainnet)

	// Test Testnet (Kermit)
	fmt.Println("\n2. TESTNET/KERMIT (https://kermit.accumulatenetwork.io/v3)")
	fmt.Println("-----------------------------------------------------------")
	testNetwork(client.NewTestnet)

	// Test Local Devnet
	fmt.Println("\n3. LOCAL DEVNET (http://localhost:8080/v3)")
	fmt.Println("------------------------------------------")
	testNetwork(func() (*client.Client, error) {
		return client.NewLocal("")
	})

	// Test custom devnet from environment
	if devnetEndpoint := os.Getenv("DEVNET_ENDPOINT"); devnetEndpoint != "" {
		fmt.Printf("\n4. CUSTOM DEVNET (%s)\n", devnetEndpoint)
		fmt.Println("------------------------------------------")
		testNetwork(func() (*client.Client, error) {
			return client.NewDevnet(devnetEndpoint)
		})
	}

	// Show how to connect to a custom endpoint
	fmt.Println("\n5. CUSTOM ENDPOINT EXAMPLE")
	fmt.Println("---------------------------")
	fmt.Println("To connect to a custom endpoint:")
	fmt.Println(`
	c, err := client.New(&client.Config{
		Endpoint: "https://your-node.example.com/v3",
		Network:  client.NetworkCustom,
		Timeout:  30 * time.Second,
		Debug:    true,  // Optional: enable debug logging
	})`)
}

func testNetwork(createClient func() (*client.Client, error)) {
	c, err := createClient()
	if err != nil {
		fmt.Printf("❌ Failed to create client: %v\n", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Test 1: Network Status
	fmt.Print("• Network Status: ")
	status, err := c.GetNetworkStatus(ctx)
	if err != nil {
		fmt.Printf("❌ %v\n", err)
		return
	}
	fmt.Printf("✅ Connected\n")
	
	if status.Network != nil {
		fmt.Printf("  - Network Name: %s\n", status.Network.NetworkName)
		fmt.Printf("  - Partitions: %d\n", len(status.Network.Partitions))
	}
	fmt.Printf("  - Directory Height: %d\n", status.DirectoryHeight)
	fmt.Printf("  - Major Block Height: %d\n", status.MajorBlockHeight)

	// Test 2: Node Info
	fmt.Print("• Node Info: ")
	nodeInfo, err := c.GetNodeInfo(ctx)
	if err != nil {
		fmt.Printf("❌ %v\n", err)
	} else {
		fmt.Printf("✅\n")
		fmt.Printf("  - Network: %s\n", nodeInfo.Network)
		fmt.Printf("  - Version: %s\n", nodeInfo.Version)
		fmt.Printf("  - Peer ID: %s\n", nodeInfo.PeerID)
	}

	// Test 3: Query ACME Token
	fmt.Print("• ACME Token: ")
	account, err := c.GetAccount(ctx, "acc://ACME")
	if err != nil {
		fmt.Printf("❌ %v\n", err)
	} else if account != nil && account.Account != nil {
		fmt.Printf("✅ Found at %s\n", account.Account.GetUrl())
	} else {
		fmt.Printf("⚠️ Not found\n")
	}

	// Test 4: Metrics (if available)
	fmt.Print("• Metrics: ")
	metrics, err := c.GetMetrics(ctx, "Directory")
	if err != nil {
		fmt.Printf("⚠️ Not available (%v)\n", err)
	} else {
		fmt.Printf("✅ TPS: %.2f\n", metrics.TPS)
	}
}