// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestAccountTypeDetection tests the account type detection logic
func TestAccountTypeDetection(t *testing.T) {
	// Test URL-based detection
	testCases := []struct {
		name     string
		urlStr   string
		expected string
	}{
		{"KeyBook", "acc://example.acme/keybook", "KeyBook"},
		{"KeyPage", "acc://example.acme/keybook/keypage/1", "KeyPage"},
		{"TokenAccount", "acc://example.acme/tokens/acme", "TokenAccount"},
		{"LiteTokenAccount", "acc://lite/tokens/acme", "LiteTokenAccount"},
		{"DataAccount", "acc://example.acme/data/mydata", "DataAccount"},
		{"LiteDataAccount", "acc://lite/data/mydata", "LiteDataAccount"},
		{"SystemLedger", "acc://system/ledger", "SystemLedger"},
		{"AnchorLedger", "acc://system/anchor", "AnchorLedger"},
		{"Identity", "acc://example.acme", "Identity"},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualType := determineAccountTypeFromURL(tc.urlStr)
			assert.Equal(t, tc.expected, actualType)
		})
	}

	// Test protocol-based detection using a mock
	t.Run("Protocol-based detection", func(t *testing.T) {
		// Skip this test for now as it requires more complex setup
		// In a real implementation, we would need to properly marshal an account
		t.Skip("Skipping protocol-based detection test as it requires proper account marshaling")
	})
}

// TestSnapshotReport tests the snapshot report collection and generation
func TestSnapshotReport(t *testing.T) {
	// Create a new report
	report, err := OpenReport()
	if err != nil {
		t.Fatalf("Failed to open report: %v", err)
	}
	defer report.Close()

	// Add some test accounts
	testAccounts := map[string]string{
		"acc://example.acme/account1": "TokenAccount",
		"acc://example.acme/account2": "TokenAccount",
		"acc://example.acme":          "Identity",
		"acc://user1.acme":            "Identity",
		"acc://user2.acme":            "Identity",
		"acc://user1.acme/tokens":     "TokenAccount",
		"acc://user2.acme/tokens":     "TokenAccount",
		"acc://user1.acme/page":       "DataAccount",
		"acc://user2.acme/page":       "DataAccount",
	}

	for url, accountType := range testAccounts {
		err := report.AddAccount(url, accountType)
		if err != nil {
			t.Fatalf("Failed to add account %s: %v", url, err)
		}
	}

	// Add some test chains
	testChains := map[string][]string{
		"acc://example.acme/account1": {"main", "secondary"},
		"acc://example.acme/account2": {"main"},
		"acc://example.acme":          {"directory", "registry", "main"},
		"acc://user1.acme":            {"directory", "registry"},
		"acc://user1.acme/tokens":     {"main"},
		"acc://user1.acme/page":       {"main", "data"},
	}

	for url, chains := range testChains {
		for _, chainID := range chains {
			// Infer chain type from the chain ID for testing purposes
			chainType := "Unknown"
			if chainID == "main" {
				chainType = "Main"
			} else if strings.Contains(chainID, "data") {
				chainType = "Data"
			} else if strings.Contains(chainID, "directory") {
				chainType = "Directory"
			} else if strings.Contains(chainID, "registry") {
				chainType = "Registry"
			} else if strings.Contains(chainID, "secondary") {
				chainType = "Secondary"
			}
			
			err := report.AddChain(url, chainID, chainType)
			if err != nil {
				t.Fatalf("Failed to add chain %s to account %s: %v", chainID, url, err)
			}
		}
	}

	// Add some test messages and transactions
	for i := 0; i < 10; i++ {
		err := report.AddMessage(fmt.Sprintf("message%d", i))
		if err != nil {
			t.Fatalf("Failed to add message: %v", err)
		}

		err = report.AddTransaction(fmt.Sprintf("transaction%d", i))
		if err != nil {
			t.Fatalf("Failed to add transaction: %v", err)
		}
	}

	// Commit the changes
	err = report.Commit()
	if err != nil {
		t.Fatalf("Failed to commit changes: %v", err)
	}

	// Generate the report
	reportText := report.GenerateReport()
	
	// Print the report content for inspection
	fmt.Println("\n--- REPORT CONTENT ---")
	fmt.Println(reportText)
	fmt.Println("--- END REPORT CONTENT ---")

	// Verify the report contains expected data
	expectedStrings := []string{
		"Snapshot Report",
		"Accounts:",
		"Account Types:",
		"TokenAccount:",
		"Identity:",
		"DataAccount:",
		"Total chains found:",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(reportText, expected) {
			t.Errorf("Report does not contain expected string: %s", expected)
		}
	}

	// Verify account counts
	if report.AccountCount != len(testAccounts) {
		t.Errorf("Expected %d accounts, got %d", len(testAccounts), report.AccountCount)
	}

	// Verify chain counts
	expectedChainCount := 0
	for _, chains := range testChains {
		expectedChainCount += len(chains)
	}
	if report.ChainCount != expectedChainCount {
		t.Errorf("Expected %d chains, got %d", expectedChainCount, report.ChainCount)
	}

	// Verify message and transaction counts
	if report.MessageCount != 10 {
		t.Errorf("Expected 10 messages, got %d", report.MessageCount)
	}
	if report.TransactionCount != 10 {
		t.Errorf("Expected 10 transactions, got %d", report.TransactionCount)
	}
}

// TestSnapshotDB tests the BlockchainDB adapter directly
func TestSnapshotDB(t *testing.T) {
	// Create a new database
	db, err := OpenSnapshotDB()
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Add some test accounts
	testAccounts := map[string]string{
		"acc://example.acme/account1": "TokenAccount",
		"acc://example.acme":          "Identity",
	}

	for url, accountType := range testAccounts {
		err := db.AddAccount(url, accountType)
		if err != nil {
			t.Fatalf("Failed to add account %s: %v", url, err)
		}
	}

	// Add some test chains
	testChains := map[string][]string{
		"acc://example.acme/account1": {"main", "secondary"},
		"acc://example.acme":          {"directory", "registry"},
	}

	for url, chains := range testChains {
		for _, chainID := range chains {
			err := db.AddChain(url, chainID)
			if err != nil {
				t.Fatalf("Failed to add chain %s to account %s: %v", chainID, url, err)
			}
		}
	}

	// Test key hashing
	key := hashKey("test")
	if len(key) != 32 {
		t.Errorf("Expected hash key length of 32, got %d", len(key))
	}

	// Test database compression
	db.Compress()
}

// TestEmptyReport tests handling of an empty report
func TestEmptyReport(t *testing.T) {
	// Create a new report
	report, err := OpenReport()
	if err != nil {
		t.Fatalf("Failed to open report: %v", err)
	}
	defer report.Close()

	// Generate the report without adding any data
	reportText := report.GenerateReport()

	// Verify the report contains expected data for an empty report
	expectedStrings := []string{
		"Snapshot Report",
		"Accounts:",
		"Account Types:",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(reportText, expected) {
			t.Errorf("Empty report does not contain expected string: %s", expected)
		}
	}

	// Verify counts are zero
	if report.AccountCount != 0 {
		t.Errorf("Expected 0 accounts, got %d", report.AccountCount)
	}
	if report.ChainCount != 0 {
		t.Errorf("Expected 0 chains, got %d", report.ChainCount)
	}
	if report.MessageCount != 0 {
		t.Errorf("Expected 0 messages, got %d", report.MessageCount)
	}
	if report.TransactionCount != 0 {
		t.Errorf("Expected 0 transactions, got %d", report.TransactionCount)
	}
}

// TestErrorHandling tests error handling in the report
func TestErrorHandling(t *testing.T) {
	// Create a new report
	report, err := OpenReport()
	if err != nil {
		t.Fatalf("Failed to open report: %v", err)
	}
	defer report.Close()

	// Test adding invalid accounts
	err = report.AddAccount("", "TokenAccount")
	if err == nil {
		t.Error("Expected error when adding account with empty URL, got nil")
	}

	// Test adding invalid chains
	err = report.AddChain("", "main", "Main")
	if err == nil {
		t.Error("Expected error when adding chain with empty account URL, got nil")
	}

	err = report.AddChain("acc://example.acme", "", "Main")
	if err == nil {
		t.Error("Expected error when adding chain with empty chain ID, got nil")
	}

	// Test adding invalid messages
	err = report.AddMessage("")
	if err == nil {
		t.Error("Expected error when adding message with empty hash, got nil")
	}

	// Test adding invalid transactions
	err = report.AddTransaction("")
	if err == nil {
		t.Error("Expected error when adding transaction with empty hash, got nil")
	}
}
