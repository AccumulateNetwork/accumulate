// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package client_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/client"
)

// TestMainnetConnectivity tests connection to mainnet
func TestMainnetConnectivity(t *testing.T) {
	// Create mainnet client
	c, err := client.NewMainnet()
	if err != nil {
		t.Fatalf("Failed to create mainnet client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Try to get network status
	fmt.Println("Testing Mainnet (https://mainnet.accumulatenetwork.io/v3)...")
	status, err := c.GetNetworkStatus(ctx)
	if err != nil {
		fmt.Printf("❌ Mainnet connection failed: %v\n", err)
		t.Logf("Mainnet connection failed: %v", err)
	} else {
		fmt.Printf("✅ Mainnet connected! Network: %v, Directory Height: %d\n",
			status.Network, status.DirectoryHeight)
		t.Logf("Mainnet connected successfully")
	}

	// Try to query ACME token
	account, err := c.GetAccount(ctx, "acc://ACME")
	if err != nil {
		fmt.Printf("   Failed to query ACME: %v\n", err)
	} else if account != nil && account.Account != nil {
		fmt.Printf("   ACME token found: %s\n", account.Account.GetUrl())
	}
}

// TestTestnetConnectivity tests connection to testnet (Kermit)
func TestTestnetConnectivity(t *testing.T) {
	// Create testnet client
	c, err := client.NewTestnet()
	if err != nil {
		t.Fatalf("Failed to create testnet client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Try to get network status
	fmt.Println("\nTesting Testnet/Kermit (https://kermit.accumulatenetwork.io/v3)...")
	status, err := c.GetNetworkStatus(ctx)
	if err != nil {
		fmt.Printf("❌ Testnet connection failed: %v\n", err)
		t.Logf("Testnet connection failed: %v", err)
	} else {
		fmt.Printf("✅ Testnet connected! Network: %v, Directory Height: %d\n",
			status.Network, status.DirectoryHeight)
		t.Logf("Testnet connected successfully")
	}

	// Try to query ACME token
	account, err := c.GetAccount(ctx, "acc://ACME")
	if err != nil {
		fmt.Printf("   Failed to query ACME: %v\n", err)
	} else if account != nil && account.Account != nil {
		fmt.Printf("   ACME token found: %s\n", account.Account.GetUrl())
	}
}

// TestLocalDevnetConnectivity tests connection to local devnet
func TestLocalDevnetConnectivity(t *testing.T) {
	// Try default local endpoint
	c, err := client.NewLocal("")
	if err != nil {
		t.Fatalf("Failed to create local client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fmt.Println("\nTesting Local Devnet (http://localhost:8080/v3)...")
	status, err := c.GetNetworkStatus(ctx)
	if err != nil {
		fmt.Printf("❌ Local devnet not running: %v\n", err)
		t.Logf("Local devnet not available (expected): %v", err)
	} else {
		fmt.Printf("✅ Local devnet connected! Network: %v, Directory Height: %d\n",
			status.Network, status.DirectoryHeight)
		t.Logf("Local devnet connected")
	}
}

// TestCustomEndpoint tests connection with a custom endpoint
func TestCustomEndpoint(t *testing.T) {
	// Test with a custom endpoint (using testnet as example)
	c, err := client.New(&client.Config{
		Endpoint: "https://testnet.accumulatenetwork.io/v3",
		Network:  client.NetworkCustom,
		Timeout:  10 * time.Second,
	})
	if err != nil {
		t.Fatalf("Failed to create custom client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fmt.Println("\nTesting Custom Endpoint (https://testnet.accumulatenetwork.io/v3)...")
	status, err := c.GetNetworkStatus(ctx)
	if err != nil {
		fmt.Printf("❌ Custom endpoint connection failed: %v\n", err)
		t.Logf("Custom endpoint connection failed: %v", err)
	} else {
		fmt.Printf("✅ Custom endpoint connected! Network: %v, Directory Height: %d\n",
			status.Network, status.DirectoryHeight)
		t.Logf("Custom endpoint connected successfully")
	}
}

// TestAllEndpoints runs connectivity tests for all configured endpoints
func TestAllEndpoints(t *testing.T) {
	t.Run("Mainnet", TestMainnetConnectivity)
	t.Run("Testnet", TestTestnetConnectivity)
	t.Run("LocalDevnet", TestLocalDevnetConnectivity)
	t.Run("CustomEndpoint", TestCustomEndpoint)
}
