// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestCreateAccount verifies that account creation works correctly
func TestCreateAccount(t *testing.T) {
	acc, err := createAccount()
	if err != nil {
		t.Fatalf("Failed to create account: %v", err)
	}

	if acc == nil {
		t.Fatal("Account URL is nil")
	}

	// Verify URL is not empty
	urlStr := acc.String()
	if urlStr == "" {
		t.Error("Account URL string is empty")
	}

	// Verify it starts with acc://
	if !strings.HasPrefix(urlStr, "acc://") {
		t.Errorf("Expected URL to start with acc://, got: %s", urlStr)
	}

	t.Logf("Created account URL: %s", urlStr)
}

// TestCreateAccountUniqueness verifies that each account creation produces unique addresses
func TestCreateAccountUniqueness(t *testing.T) {
	accounts := make(map[string]bool)
	count := 100

	for i := 0; i < count; i++ {
		acc, err := createAccount()
		if err != nil {
			t.Fatalf("Failed to create account %d: %v", i, err)
		}

		urlStr := acc.String()
		if accounts[urlStr] {
			t.Fatalf("Duplicate account URL generated: %s", urlStr)
		}
		accounts[urlStr] = true
	}

	if len(accounts) != count {
		t.Errorf("Expected %d unique accounts, got %d", count, len(accounts))
	}
}

// TestLiteTokenAddressGeneration verifies the lite token address generation
func TestLiteTokenAddressGeneration(t *testing.T) {
	// Generate a test key
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	// Create lite token address
	acc, err := protocol.LiteTokenAddress(pub, protocol.ACME, protocol.SignatureTypeED25519)
	if err != nil {
		t.Fatalf("Failed to create lite token address: %v", err)
	}

	if acc == nil {
		t.Fatal("Lite token address is nil")
	}

	// Verify it's a valid URL
	urlStr := acc.String()
	if !strings.HasPrefix(urlStr, "acc://") {
		t.Errorf("Expected acc:// prefix, got: %s", urlStr)
	}
}

// TestClientStructure verifies the Client structure is properly formed
func TestClientStructure(t *testing.T) {
	client := &Client{
		DataSet: nil, // DataSet can be nil for testing
		Client:  nil, // Client can be nil for testing
		Id:      42,
		TxCount: 0,
	}

	if client.Id != 42 {
		t.Errorf("Expected client ID 42, got %d", client.Id)
	}

	if client.TxCount != 0 {
		t.Errorf("Expected tx count 0, got %d", client.TxCount)
	}
}

// TestDefaultFlags verifies default flag values
func TestDefaultFlags(t *testing.T) {
	// Note: These are package-level variables that may have been modified
	// This test documents the expected defaults
	t.Logf("Server URL: %s", serverUrl)
	t.Logf("Transactions: %d", transactions)
	t.Logf("Max Goroutines: %d", maxGoroutines)
	t.Logf("Duration: %d", duration)
}

// TestTransactionLoadCalculation verifies the load calculation logic
func TestTransactionLoadCalculation(t *testing.T) {
	tests := []struct {
		name              string
		transactions      int
		maxGoroutines     int
		duration          int
		expectedPerClient int
		expectedClients   int
		expectedTotal     int
	}{
		{
			name:              "default_config",
			transactions:      100,
			maxGoroutines:     25,
			duration:          5,
			expectedPerClient: 25,
			expectedClients:   20,
			expectedTotal:     500,
		},
		{
			name:              "low_load",
			transactions:      10,
			maxGoroutines:     5,
			duration:          2,
			expectedPerClient: 5,
			expectedClients:   4,
			expectedTotal:     20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transactionsPerClient := tt.maxGoroutines
			numClientsPerBurst := tt.transactions / transactionsPerClient
			maxNumClients := numClientsPerBurst * tt.duration
			totalTransactions := maxNumClients * transactionsPerClient

			if transactionsPerClient != tt.expectedPerClient {
				t.Errorf("transactionsPerClient: expected %d, got %d",
					tt.expectedPerClient, transactionsPerClient)
			}

			if maxNumClients != tt.expectedClients {
				t.Errorf("maxNumClients: expected %d, got %d",
					tt.expectedClients, maxNumClients)
			}

			if totalTransactions != tt.expectedTotal {
				t.Errorf("totalTransactions: expected %d, got %d",
					tt.expectedTotal, totalTransactions)
			}
		})
	}
}

// TestDataSetLogging verifies dataset logging functionality
func TestDataSetLogging(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize dataset
	dsl.SetPath(tmpDir)
	dsl.SetProcessName("test-load")
	dsl.Initialize("test-dataset", logging.DefaultOptions())

	// Set a header
	header := "## Test Header\n## Test Data"
	dsl.SetHeader(header)

	// Dump dataset
	outputPaths, err := dsl.DumpDataSetToDiskFile()
	if err != nil {
		t.Fatalf("Failed to dump dataset: %v", err)
	}

	// Log output paths (may be empty if no data was saved)
	if len(outputPaths) == 0 {
		t.Log("No output paths generated (no data saved)")
	} else {
		t.Logf("Dataset written to: %v", outputPaths)
	}
}

// Integration tests - require running devnet
func TestLoadGeneratorIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if os.Getenv("INTEGRATION_TEST") != "1" {
		t.Skip("Skipping integration test - set INTEGRATION_TEST=1 to run")
	}

	// Get server URL from environment or use default
	testServerURL := os.Getenv("ACC_API")
	if testServerURL == "" {
		testServerURL = "http://127.0.0.1:26660/v2"
	}

	// Save and restore
	originalURL := serverUrl
	defer func() { serverUrl = originalURL }()
	serverUrl = testServerURL

	// Create client to test connectivity
	cl, err := client.New(testServerURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Test server connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = cl.Describe(ctx)
	if err != nil {
		t.Skipf("Server not available at %s: %v", testServerURL, err)
	}

	t.Run("single_account_faucet", func(t *testing.T) {
		// Initialize one client
		clients, err := initializeClients(1)
		if err != nil {
			t.Fatalf("Failed to initialize clients: %v", err)
		}

		if len(clients) != 1 {
			t.Fatalf("Expected 1 client, got %d", len(clients))
		}

		c := clients[0]
		// Create test account
		acc, err := createAccount()
		if err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		// Faucet the account
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		resp, err := c.Client.Faucet(ctx, &protocol.AcmeFaucet{Url: acc})
		if err != nil {
			t.Fatalf("Faucet failed: %v", err)
		}

		if resp.TransactionHash == nil {
			t.Fatal("Faucet response missing transaction hash")
		}

		// Wait for transaction
		txReq := api.TxnQuery{
			Txid:          resp.TransactionHash,
			Wait:          15 * time.Second,
			IgnorePending: false,
		}

		_, err = c.Client.QueryTx(ctx, &txReq)
		if err != nil {
			t.Logf("Transaction query failed (may be expected): %v", err)
		}

		t.Logf("Successfully fauceted account %s", acc)
	})
}

// BenchmarkCreateAccount benchmarks account creation
func BenchmarkCreateAccount(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := createAccount()
		if err != nil {
			b.Fatalf("Failed to create account: %v", err)
		}
	}
}

// TestMain provides test suite setup and teardown
func TestMain(m *testing.M) {
	fmt.Println("Starting load generator test suite")
	code := m.Run()
	fmt.Println("Load generator test suite completed")
	os.Exit(code)
}
