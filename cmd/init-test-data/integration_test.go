// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"path/filepath"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/test/wallet"
)

// TestInitializeLiteAccountsFlow tests the lite account initialization flow.
func TestInitializeLiteAccountsFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	walletPath := filepath.Join(tmpDir, "test-wallet.json")

	w := wallet.New(walletPath)
	if err := w.GenerateFunder("acc://test-funder.acme/tokens"); err != nil {
		t.Fatalf("Failed to generate funder: %v", err)
	}
	if err := w.GenerateAccounts(5, 0, 0); err != nil {
		t.Fatalf("Failed to generate accounts: %v", err)
	}
	if err := w.Save(); err != nil {
		t.Fatalf("Failed to save wallet: %v", err)
	}

	config := Config{
		WalletPath:   walletPath,
		NodeURL:      "http://localhost:8080/v3",
		BatchSize:    2,
		Concurrency:  2,
		InitialFunds: 1000,
		SkipADIToken: true,
		SkipADIData:  true,
	}

	init, err := NewInitializer(config)
	if err != nil {
		t.Fatalf("NewInitializer() failed: %v", err)
	}

	// Verify account counts
	liteAccounts := init.wallet.GetAccountsByType("lite")
	if len(liteAccounts) != 5 {
		t.Errorf("Expected 5 lite accounts, got %d", len(liteAccounts))
	}

	// Test account structure
	for i, acct := range liteAccounts {
		if acct.URL == "" {
			t.Errorf("Account %d has empty URL", i)
		}
		if acct.PublicKey == "" {
			t.Errorf("Account %d has empty public key", i)
		}
		if acct.PrivateKey == "" {
			t.Errorf("Account %d has empty private key", i)
		}
		if acct.Type != "lite" {
			t.Errorf("Account %d type = %s, want lite", i, acct.Type)
		}

		// Verify keys are valid
		if _, err := acct.GetPublicKey(); err != nil {
			t.Errorf("Account %d has invalid public key: %v", i, err)
		}
		if _, err := acct.GetPrivateKey(); err != nil {
			t.Errorf("Account %d has invalid private key: %v", i, err)
		}
	}
}

// TestInitializeADITokenAccountsFlow tests the ADI token account initialization flow.
func TestInitializeADITokenAccountsFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	walletPath := filepath.Join(tmpDir, "test-wallet.json")

	w := wallet.New(walletPath)
	if err := w.GenerateFunder("acc://test-funder.acme/tokens"); err != nil {
		t.Fatalf("Failed to generate funder: %v", err)
	}
	if err := w.GenerateAccounts(0, 3, 0); err != nil {
		t.Fatalf("Failed to generate accounts: %v", err)
	}
	if err := w.Save(); err != nil {
		t.Fatalf("Failed to save wallet: %v", err)
	}

	config := Config{
		WalletPath:   walletPath,
		NodeURL:      "http://localhost:8080/v3",
		BatchSize:    2,
		Concurrency:  2,
		InitialFunds: 1000,
		SkipLite:     true,
		SkipADIData:  true,
	}

	init, err := NewInitializer(config)
	if err != nil {
		t.Fatalf("NewInitializer() failed: %v", err)
	}

	// Verify account counts
	adiTokenAccounts := init.wallet.GetAccountsByType("adi-token")
	if len(adiTokenAccounts) != 3 {
		t.Errorf("Expected 3 ADI token accounts, got %d", len(adiTokenAccounts))
	}

	// Test account structure
	for i, acct := range adiTokenAccounts {
		if acct.URL == "" {
			t.Errorf("Account %d has empty URL", i)
		}
		if acct.Type != "adi-token" {
			t.Errorf("Account %d type = %s, want adi-token", i, acct.Type)
		}

		// Verify URL format (should be acc://test-a########.acme/tokens)
		parsedURL, err := acct.GetURL()
		if err != nil {
			t.Errorf("Account %d has invalid URL: %v", i, err)
		}
		if parsedURL.Path != "/tokens" {
			t.Errorf("Account %d path = %s, want /tokens", i, parsedURL.Path)
		}
	}
}

// TestInitializeADIDataAccountsFlow tests the ADI data account initialization flow.
func TestInitializeADIDataAccountsFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	walletPath := filepath.Join(tmpDir, "test-wallet.json")

	w := wallet.New(walletPath)
	if err := w.GenerateFunder("acc://test-funder.acme/tokens"); err != nil {
		t.Fatalf("Failed to generate funder: %v", err)
	}
	if err := w.GenerateAccounts(0, 0, 2); err != nil {
		t.Fatalf("Failed to generate accounts: %v", err)
	}
	if err := w.Save(); err != nil {
		t.Fatalf("Failed to save wallet: %v", err)
	}

	config := Config{
		WalletPath:   walletPath,
		NodeURL:      "http://localhost:8080/v3",
		BatchSize:    2,
		Concurrency:  2,
		InitialFunds: 1000,
		SkipLite:     true,
		SkipADIToken: true,
	}

	init, err := NewInitializer(config)
	if err != nil {
		t.Fatalf("NewInitializer() failed: %v", err)
	}

	// Verify account counts
	adiDataAccounts := init.wallet.GetAccountsByType("adi-data")
	if len(adiDataAccounts) != 2 {
		t.Errorf("Expected 2 ADI data accounts, got %d", len(adiDataAccounts))
	}

	// Test account structure
	for i, acct := range adiDataAccounts {
		if acct.URL == "" {
			t.Errorf("Account %d has empty URL", i)
		}
		if acct.Type != "adi-data" {
			t.Errorf("Account %d type = %s, want adi-data", i, acct.Type)
		}

		// Verify URL format (should be acc://test-d########.acme/data)
		parsedURL, err := acct.GetURL()
		if err != nil {
			t.Errorf("Account %d has invalid URL: %v", i, err)
		}
		if parsedURL.Path != "/data" {
			t.Errorf("Account %d path = %s, want /data", i, parsedURL.Path)
		}
	}
}

// TestMixedAccountTypes tests initialization with all account types.
func TestMixedAccountTypes(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	walletPath := filepath.Join(tmpDir, "test-wallet.json")

	w := wallet.New(walletPath)
	if err := w.GenerateFunder("acc://test-funder.acme/tokens"); err != nil {
		t.Fatalf("Failed to generate funder: %v", err)
	}
	if err := w.GenerateAccounts(3, 2, 1); err != nil {
		t.Fatalf("Failed to generate accounts: %v", err)
	}
	if err := w.Save(); err != nil {
		t.Fatalf("Failed to save wallet: %v", err)
	}

	config := Config{
		WalletPath:   walletPath,
		NodeURL:      "http://localhost:8080/v3",
		BatchSize:    2,
		Concurrency:  2,
		InitialFunds: 1000,
	}

	init, err := NewInitializer(config)
	if err != nil {
		t.Fatalf("NewInitializer() failed: %v", err)
	}

	// Verify total account count
	totalAccounts := init.wallet.Count()
	if totalAccounts != 6 {
		t.Errorf("Total accounts = %d, want 6", totalAccounts)
	}

	// Verify counts by type
	if init.wallet.CountByType("lite") != 3 {
		t.Errorf("Lite accounts = %d, want 3", init.wallet.CountByType("lite"))
	}
	if init.wallet.CountByType("adi-token") != 2 {
		t.Errorf("ADI token accounts = %d, want 2", init.wallet.CountByType("adi-token"))
	}
	if init.wallet.CountByType("adi-data") != 1 {
		t.Errorf("ADI data accounts = %d, want 1", init.wallet.CountByType("adi-data"))
	}
}

// TestBatchProcessingWithErrors tests batch processing error recovery.
func TestBatchProcessingWithErrors(t *testing.T) {
	tmpDir := t.TempDir()
	walletPath := filepath.Join(tmpDir, "test-wallet.json")

	w := wallet.New(walletPath)
	if err := w.GenerateFunder("acc://test-funder.acme/tokens"); err != nil {
		t.Fatalf("Failed to generate funder: %v", err)
	}
	if err := w.GenerateAccounts(10, 0, 0); err != nil {
		t.Fatalf("Failed to generate accounts: %v", err)
	}
	if err := w.Save(); err != nil {
		t.Fatalf("Failed to save wallet: %v", err)
	}

	config := Config{
		WalletPath:   walletPath,
		NodeURL:      "http://localhost:8080/v3",
		BatchSize:    3,
		Concurrency:  2,
		InitialFunds: 1000,
	}

	init, err := NewInitializer(config)
	if err != nil {
		t.Fatalf("NewInitializer() failed: %v", err)
	}

	accounts := init.wallet.GetAccountsByType("lite")
	ctx := context.Background()

	// Process batch with a function that simulates some errors
	errorCount := 0
	mockCreateFn := func(ctx context.Context, acct *wallet.Account) error {
		// Simulate error for every 3rd account
		if acct.Index%3 == 0 {
			errorCount++
			return context.DeadlineExceeded
		}
		return nil
	}

	// Process should complete even with errors
	err = init.processBatch(ctx, accounts, mockCreateFn)
	if err != nil {
		t.Errorf("processBatch() failed: %v", err)
	}

	// Verify that errors occurred as expected
	if errorCount == 0 {
		t.Error("Expected some errors, got none")
	}
}

// TestAccountIndexing tests that account indices are correctly assigned.
func TestAccountIndexing(t *testing.T) {
	tmpDir := t.TempDir()
	walletPath := filepath.Join(tmpDir, "test-wallet.json")

	w := wallet.New(walletPath)
	if err := w.GenerateFunder("acc://test-funder.acme/tokens"); err != nil {
		t.Fatalf("Failed to generate funder: %v", err)
	}
	if err := w.GenerateAccounts(5, 3, 2); err != nil {
		t.Fatalf("Failed to generate accounts: %v", err)
	}
	if err := w.Save(); err != nil {
		t.Fatalf("Failed to save wallet: %v", err)
	}

	config := Config{
		WalletPath:   walletPath,
		NodeURL:      "http://localhost:8080/v3",
		BatchSize:    10,
		Concurrency:  5,
		InitialFunds: 1000,
	}

	init, err := NewInitializer(config)
	if err != nil {
		t.Fatalf("NewInitializer() failed: %v", err)
	}

	// Verify indices are sequential
	expectedIndex := 0
	for _, acct := range init.wallet.Accounts {
		if acct.Index != expectedIndex {
			t.Errorf("Account index = %d, want %d", acct.Index, expectedIndex)
		}
		expectedIndex++
	}

	// Verify we can retrieve accounts by index
	for i := 0; i < init.wallet.Count(); i++ {
		acct, err := init.wallet.GetAccount(i)
		if err != nil {
			t.Errorf("GetAccount(%d) failed: %v", i, err)
		}
		if acct.Index != i {
			t.Errorf("Retrieved account index = %d, want %d", acct.Index, i)
		}
	}

	// Test out of bounds access
	_, err = init.wallet.GetAccount(-1)
	if err == nil {
		t.Error("GetAccount(-1) expected error, got nil")
	}

	_, err = init.wallet.GetAccount(init.wallet.Count())
	if err == nil {
		t.Error("GetAccount(count) expected error, got nil")
	}
}

// TestAccountURLParsing tests that account URLs are correctly formatted.
func TestAccountURLParsing(t *testing.T) {
	tmpDir := t.TempDir()
	walletPath := filepath.Join(tmpDir, "test-wallet.json")

	w := wallet.New(walletPath)
	if err := w.GenerateFunder("acc://test-funder.acme/tokens"); err != nil {
		t.Fatalf("Failed to generate funder: %v", err)
	}
	if err := w.GenerateAccounts(2, 2, 2); err != nil {
		t.Fatalf("Failed to generate accounts: %v", err)
	}
	if err := w.Save(); err != nil {
		t.Fatalf("Failed to save wallet: %v", err)
	}

	config := Config{
		WalletPath:   walletPath,
		NodeURL:      "http://localhost:8080/v3",
		BatchSize:    10,
		Concurrency:  5,
		InitialFunds: 1000,
	}

	init, err := NewInitializer(config)
	if err != nil {
		t.Fatalf("NewInitializer() failed: %v", err)
	}

	// Test lite account URLs
	liteAccounts := init.wallet.GetAccountsByType("lite")
	for i, acct := range liteAccounts {
		parsedURL, err := acct.GetURL()
		if err != nil {
			t.Errorf("Lite account %d URL parse error: %v", i, err)
		}
		// Lite accounts should have URLs in format acc://[pubkeyhash]/ACME
		if parsedURL.String() == "" {
			t.Errorf("Lite account %d has empty URL", i)
		}
	}

	// Test ADI token account URLs
	adiTokenAccounts := init.wallet.GetAccountsByType("adi-token")
	for i, acct := range adiTokenAccounts {
		parsedURL, err := acct.GetURL()
		if err != nil {
			t.Errorf("ADI token account %d URL parse error: %v", i, err)
		}
		// Path includes leading slash in Accumulate URLs
		if parsedURL.Path != "/tokens" {
			t.Errorf("ADI token account %d path = %s, want /tokens", i, parsedURL.Path)
		}
	}

	// Test ADI data account URLs
	adiDataAccounts := init.wallet.GetAccountsByType("adi-data")
	for i, acct := range adiDataAccounts {
		parsedURL, err := acct.GetURL()
		if err != nil {
			t.Errorf("ADI data account %d URL parse error: %v", i, err)
		}
		// Path includes leading slash in Accumulate URLs
		if parsedURL.Path != "/data" {
			t.Errorf("ADI data account %d path = %s, want /data", i, parsedURL.Path)
		}
	}
}

// TestFunderAccountStructure tests the funder account properties.
func TestFunderAccountStructure(t *testing.T) {
	tmpDir := t.TempDir()
	walletPath := filepath.Join(tmpDir, "test-wallet.json")

	w := wallet.New(walletPath)
	if err := w.GenerateFunder("acc://test-funder.acme/tokens"); err != nil {
		t.Fatalf("Failed to generate funder: %v", err)
	}
	if err := w.Save(); err != nil {
		t.Fatalf("Failed to save wallet: %v", err)
	}

	config := Config{
		WalletPath:   walletPath,
		NodeURL:      "http://localhost:8080/v3",
		BatchSize:    10,
		Concurrency:  5,
		InitialFunds: 1000,
	}

	init, err := NewInitializer(config)
	if err != nil {
		t.Fatalf("NewInitializer() failed: %v", err)
	}

	// Test funder properties
	funder := init.wallet.Funder
	if funder == nil {
		t.Fatal("Funder is nil")
	}

	if funder.URL != "acc://test-funder.acme/tokens" {
		t.Errorf("Funder URL = %s, want acc://test-funder.acme/tokens", funder.URL)
	}

	if funder.Type != "funder" {
		t.Errorf("Funder type = %s, want funder", funder.Type)
	}

	if funder.Index != -1 {
		t.Errorf("Funder index = %d, want -1", funder.Index)
	}

	// Verify funder keys
	funderPubKey, err := funder.GetPublicKey()
	if err != nil {
		t.Errorf("Failed to get funder public key: %v", err)
	}
	if len(funderPubKey) != 32 {
		t.Errorf("Funder public key length = %d, want 32", len(funderPubKey))
	}

	funderPrivKey, err := funder.GetPrivateKey()
	if err != nil {
		t.Errorf("Failed to get funder private key: %v", err)
	}
	if len(funderPrivKey) != 64 {
		t.Errorf("Funder private key length = %d, want 64", len(funderPrivKey))
	}
}
