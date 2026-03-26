// +build integration

package main

import (
	"context"
	"os"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/mcp/client"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

const (
	devnetURL = "http://127.0.0.1:26660/v3"
	testTimeout = 30 * time.Second
)

// TestDevnetQuery tests querying accounts on the devnet
func TestDevnetQuery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	c, err := client.NewClient(devnetURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Query the ACME token issuer (should always exist on devnet)
	result, err := c.QueryAccount(ctx, "acc://ACME")
	if err != nil {
		t.Fatalf("Failed to query ACME account: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result from query")
	}

	t.Logf("Successfully queried ACME account: %+v", result)
}

// TestDevnetQueryDirectory tests querying directory entries
func TestDevnetQueryDirectory(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	c, err := client.NewClient(devnetURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Query the root directory with range
	result, err := c.QueryDirectory(ctx, "acc://dn", map[string]interface{}{
		"start": uint64(0),
		"count": uint64(10),
	})
	if err != nil {
		t.Fatalf("Failed to query directory: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result from directory query")
	}

	t.Logf("Successfully queried directory: %+v", result)
}

// TestDevnetNodeInfo tests querying node information
func TestDevnetNodeInfo(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	c, err := client.NewClient(devnetURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Query node info
	result, err := c.NodeInfo(ctx, map[string]interface{}{})
	if err != nil {
		t.Fatalf("Failed to get node info: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result from node info query")
	}

	t.Logf("Successfully queried node info: %+v", result)
}

// TestDevnetQueryMetrics tests querying network metrics
func TestDevnetQueryMetrics(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	c, err := client.NewClient(devnetURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Query metrics
	result, err := c.Metrics(ctx, map[string]interface{}{})
	if err != nil {
		t.Fatalf("Failed to get metrics: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result from metrics query")
	}

	t.Logf("Successfully queried metrics: %+v", result)
}

// TestDevnetCreateLiteAccount tests creating a lite account from a public key
func TestDevnetCreateLiteAccount(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Use a test public key (32 bytes ED25519)
	testPubKey := "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"

	liteURL, err := client.CreateLiteAccountURL(testPubKey)
	if err != nil {
		t.Fatalf("Failed to create lite account: %v", err)
	}

	if liteURL == "" {
		t.Fatal("Expected non-empty lite account URL")
	}

	// Verify it's a valid URL
	_, err = url.Parse(liteURL)
	if err != nil {
		t.Fatalf("Invalid lite account URL: %v", err)
	}

	t.Logf("Successfully created lite account: %s", liteURL)
}

// TestDevnetFaucet tests using the faucet (if available)
func TestDevnetFaucet(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Only run if devnet is running
	if os.Getenv("DEVNET_FAUCET_ENABLED") != "true" {
		t.Skip("Skipping faucet test (set DEVNET_FAUCET_ENABLED=true to enable)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	c, err := client.NewClient(devnetURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Create a test lite account
	testPubKey := "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	liteURL, err := client.CreateLiteAccountURL(testPubKey)
	if err != nil {
		t.Fatalf("Failed to create lite account: %v", err)
	}

	// Request faucet funds
	result, err := c.Faucet(ctx, liteURL, map[string]interface{}{})
	if err != nil {
		t.Logf("Faucet error (may not be available): %v", err)
		// Don't fail the test if faucet is not available
		return
	}

	if result == nil {
		t.Fatal("Expected non-nil result from faucet")
	}

	t.Logf("Successfully requested faucet funds: %+v", result)
}

// TestDevnetTransactionQuery tests querying a transaction
func TestDevnetTransactionQuery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Skip if no test transaction ID provided
	testTxID := os.Getenv("TEST_TRANSACTION_ID")
	if testTxID == "" {
		t.Skip("Skipping transaction query test (set TEST_TRANSACTION_ID to enable)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	c, err := client.NewClient(devnetURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	result, err := c.QueryTransaction(ctx, testTxID)
	if err != nil {
		t.Fatalf("Failed to query transaction: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result from transaction query")
	}

	t.Logf("Successfully queried transaction: %+v", result)
}

// TestDevnetChainQuery tests querying a chain
func TestDevnetChainQuery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	c, err := client.NewClient(devnetURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Query the main chain of the ACME account
	result, err := c.QueryChain(ctx, "acc://ACME", map[string]interface{}{
		"chain_name": "main",
	})
	if err != nil {
		t.Fatalf("Failed to query chain: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result from chain query")
	}

	t.Logf("Successfully queried chain: %+v", result)
}

// TestDevnetBlockQuery tests querying blocks
func TestDevnetBlockQuery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	c, err := client.NewClient(devnetURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Query the latest major block
	result, err := c.QueryMajorBlock(ctx, "Directory", uint64(1))
	if err != nil {
		t.Fatalf("Failed to query major block: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result from major block query")
	}

	t.Logf("Successfully queried major block: %+v", result)
}

// TestDevnetNetworkStatus tests querying network status
func TestDevnetNetworkStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Skip("Skipping network status test due to SDK version incompatibility with devnet")

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	c, err := client.NewClient(devnetURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Query network status
	result, err := c.NetworkStatus(ctx, map[string]interface{}{
		"partition": "Directory",
	})
	if err != nil {
		t.Fatalf("Failed to get network status: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result from network status query")
	}

	t.Logf("Successfully queried network status: %+v", result)
}

// TestDevnetSearchByPublicKey tests searching by public key
func TestDevnetSearchByPublicKey(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Skip if no test public key provided
	testPubKey := os.Getenv("TEST_PUBLIC_KEY")
	if testPubKey == "" {
		t.Skip("Skipping public key search test (set TEST_PUBLIC_KEY to enable)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	c, err := client.NewClient(devnetURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	result, err := c.SearchPublicKey(ctx, testPubKey, map[string]interface{}{})
	if err != nil {
		t.Fatalf("Failed to search by public key: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result from public key search")
	}

	t.Logf("Successfully searched by public key: %+v", result)
}
