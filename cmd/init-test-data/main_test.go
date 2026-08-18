// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/test/wallet"
)

// createTestWallet creates a temporary test wallet with predefined accounts.
func createTestWallet(t *testing.T, numLite, numADIToken, numADIData int) (*wallet.Wallet, string) {
	t.Helper()

	tmpDir := t.TempDir()
	walletPath := filepath.Join(tmpDir, "test-wallet.json")

	w := wallet.New(walletPath)
	if err := w.GenerateFunder("acc://test-funder.acme/tokens"); err != nil {
		t.Fatalf("Failed to generate funder: %v", err)
	}

	if err := w.GenerateAccounts(numLite, numADIToken, numADIData); err != nil {
		t.Fatalf("Failed to generate accounts: %v", err)
	}

	if err := w.Save(); err != nil {
		t.Fatalf("Failed to save wallet: %v", err)
	}

	return w, walletPath
}

// TestNewInitializer tests the NewInitializer function.
func TestNewInitializer(t *testing.T) {
	tests := []struct {
		name        string
		setupWallet func(*testing.T) string
		wantErr     bool
		errContains string
	}{
		{
			name: "Valid wallet with funder",
			setupWallet: func(t *testing.T) string {
				_, path := createTestWallet(t, 5, 3, 2)
				return path
			},
			wantErr: false,
		},
		{
			name: "Wallet without funder",
			setupWallet: func(t *testing.T) string {
				tmpDir := t.TempDir()
				walletPath := filepath.Join(tmpDir, "no-funder-wallet.json")
				w := wallet.New(walletPath)
				if err := w.Save(); err != nil {
					t.Fatalf("Failed to save wallet: %v", err)
				}
				return walletPath
			},
			wantErr:     true,
			errContains: "no funder account",
		},
		{
			name: "Non-existent wallet",
			setupWallet: func(t *testing.T) string {
				return "/nonexistent/wallet.json"
			},
			wantErr:     true,
			errContains: "load wallet",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			walletPath := tt.setupWallet(t)

			config := Config{
				WalletPath:   walletPath,
				NodeURL:      "http://localhost:8080/v3",
				BatchSize:    10,
				Concurrency:  5,
				InitialFunds: 1000,
			}

			init, err := NewInitializer(config)

			if tt.wantErr {
				if err == nil {
					t.Errorf("NewInitializer() expected error containing %q, got nil", tt.errContains)
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("NewInitializer() error = %v, want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("NewInitializer() unexpected error: %v", err)
				}
				if init == nil {
					t.Error("NewInitializer() returned nil initializer")
				}
				if init != nil {
					if init.wallet == nil {
						t.Error("Initializer has nil wallet")
					}
					if init.client == nil {
						t.Error("Initializer has nil client")
					}
					if init.stats == nil {
						t.Error("Initializer has nil stats")
					}
				}
			}
		})
	}
}

// TestDryRunMode tests the dry-run functionality.
func TestDryRunMode(t *testing.T) {
	_, walletPath := createTestWallet(t, 10, 5, 3)

	config := Config{
		WalletPath:   walletPath,
		NodeURL:      "http://localhost:8080/v3",
		BatchSize:    10,
		Concurrency:  5,
		InitialFunds: 1000,
		DryRun:       true,
	}

	init, err := NewInitializer(config)
	if err != nil {
		t.Fatalf("NewInitializer() failed: %v", err)
	}

	// Run should complete without errors in dry-run mode
	ctx := context.Background()
	if err := init.Run(ctx); err != nil {
		t.Errorf("Run() in dry-run mode failed: %v", err)
	}

	// Verify no accounts were created (stats should be zero)
	if init.stats.LiteCreated.Load() != 0 {
		t.Errorf("Dry run created %d lite accounts, expected 0", init.stats.LiteCreated.Load())
	}
	if init.stats.ADITokenCreated.Load() != 0 {
		t.Errorf("Dry run created %d ADI token accounts, expected 0", init.stats.ADITokenCreated.Load())
	}
	if init.stats.ADIDataCreated.Load() != 0 {
		t.Errorf("Dry run created %d ADI data accounts, expected 0", init.stats.ADIDataCreated.Load())
	}

	// Verify wallet still has expected accounts
	if init.wallet.CountByType("lite") != 10 {
		t.Errorf("Expected 10 lite accounts in wallet, got %d", init.wallet.CountByType("lite"))
	}
}

// TestSkipFlags tests the skip flags functionality.
func TestSkipFlags(t *testing.T) {
	tests := []struct {
		name         string
		skipLite     bool
		skipADIToken bool
		skipADIData  bool
	}{
		{"Skip lite", true, false, false},
		{"Skip ADI token", false, true, false},
		{"Skip ADI data", false, false, true},
		{"Skip all", true, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, walletPath := createTestWallet(t, 5, 3, 2)

			config := Config{
				WalletPath:   walletPath,
				NodeURL:      "http://localhost:8080/v3",
				BatchSize:    10,
				Concurrency:  5,
				InitialFunds: 1000,
				SkipLite:     tt.skipLite,
				SkipADIToken: tt.skipADIToken,
				SkipADIData:  tt.skipADIData,
				DryRun:       true, // Use dry-run to avoid actual network calls
			}

			init, err := NewInitializer(config)
			if err != nil {
				t.Fatalf("NewInitializer() failed: %v", err)
			}

			ctx := context.Background()
			if err := init.Run(ctx); err != nil {
				t.Errorf("Run() failed: %v", err)
			}
		})
	}
}

// TestStatsTracking tests the statistics tracking functionality.
func TestStatsTracking(t *testing.T) {
	stats := &Stats{
		StartTime: time.Now(),
		Errors:    make([]string, 0),
	}

	// Test atomic counters
	stats.LiteCreated.Add(5)
	stats.LiteFailed.Add(2)
	stats.ADITokenCreated.Add(3)
	stats.ADITokenFailed.Add(1)
	stats.ADIDataCreated.Add(4)
	stats.ADIDataFailed.Add(0)

	if stats.LiteCreated.Load() != 5 {
		t.Errorf("LiteCreated = %d, want 5", stats.LiteCreated.Load())
	}
	if stats.LiteFailed.Load() != 2 {
		t.Errorf("LiteFailed = %d, want 2", stats.LiteFailed.Load())
	}
	if stats.ADITokenCreated.Load() != 3 {
		t.Errorf("ADITokenCreated = %d, want 3", stats.ADITokenCreated.Load())
	}
	if stats.ADITokenFailed.Load() != 1 {
		t.Errorf("ADITokenFailed = %d, want 1", stats.ADITokenFailed.Load())
	}
	if stats.ADIDataCreated.Load() != 4 {
		t.Errorf("ADIDataCreated = %d, want 4", stats.ADIDataCreated.Load())
	}
	if stats.ADIDataFailed.Load() != 0 {
		t.Errorf("ADIDataFailed = %d, want 0", stats.ADIDataFailed.Load())
	}
}

// TestConfigValidation tests various configuration combinations.
func TestConfigValidation(t *testing.T) {
	_, walletPath := createTestWallet(t, 5, 3, 2)

	tests := []struct {
		name   string
		config Config
	}{
		{
			name: "Default config",
			config: Config{
				WalletPath:   walletPath,
				NodeURL:      "http://localhost:8080/v3",
				BatchSize:    10,
				Concurrency:  5,
				InitialFunds: 1000,
			},
		},
		{
			name: "Small batch size",
			config: Config{
				WalletPath:   walletPath,
				NodeURL:      "http://localhost:8080/v3",
				BatchSize:    1,
				Concurrency:  1,
				InitialFunds: 100,
			},
		},
		{
			name: "Large batch size",
			config: Config{
				WalletPath:   walletPath,
				NodeURL:      "http://localhost:8080/v3",
				BatchSize:    100,
				Concurrency:  20,
				InitialFunds: 5000,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			init, err := NewInitializer(tt.config)
			if err != nil {
				t.Fatalf("NewInitializer() failed: %v", err)
			}

			if init.config.BatchSize != tt.config.BatchSize {
				t.Errorf("BatchSize = %d, want %d", init.config.BatchSize, tt.config.BatchSize)
			}
			if init.config.Concurrency != tt.config.Concurrency {
				t.Errorf("Concurrency = %d, want %d", init.config.Concurrency, tt.config.Concurrency)
			}
			if init.config.InitialFunds != tt.config.InitialFunds {
				t.Errorf("InitialFunds = %d, want %d", init.config.InitialFunds, tt.config.InitialFunds)
			}
		})
	}
}

// TestPrintPlan tests the plan printing functionality.
func TestPrintPlan(t *testing.T) {
	_, walletPath := createTestWallet(t, 10, 5, 3)

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

	// This should not panic
	init.printPlan()
}

// TestPrintSummary tests the summary printing functionality.
func TestPrintSummary(t *testing.T) {
	_, walletPath := createTestWallet(t, 5, 3, 2)

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

	// Set some stats
	init.stats.LiteCreated.Add(5)
	init.stats.LiteFailed.Add(1)
	init.stats.ADITokenCreated.Add(3)
	init.stats.ADIDataCreated.Add(2)

	// This should not panic and should create a summary file
	init.printSummary()

	// Verify summary file was created
	summaryFile := "/tmp/init-test-data-summary.json"
	if _, err := os.Stat(summaryFile); os.IsNotExist(err) {
		t.Error("Summary file was not created")
	} else {
		// Read and verify summary contents
		data, err := os.ReadFile(summaryFile)
		if err != nil {
			t.Fatalf("Failed to read summary file: %v", err)
		}

		var summary map[string]interface{}
		if err := json.Unmarshal(data, &summary); err != nil {
			t.Fatalf("Failed to unmarshal summary: %v", err)
		}

		// Verify key fields
		if summary["node"] != config.NodeURL {
			t.Errorf("Summary node = %v, want %v", summary["node"], config.NodeURL)
		}
		if summary["batch_size"] != float64(config.BatchSize) {
			t.Errorf("Summary batch_size = %v, want %v", summary["batch_size"], config.BatchSize)
		}

		// Clean up
		os.Remove(summaryFile)
	}
}

// TestAccountTypeFiltering tests filtering accounts by type.
func TestAccountTypeFiltering(t *testing.T) {
	_, walletPath := createTestWallet(t, 10, 7, 5)

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

	// Test account type filtering
	liteAccounts := init.wallet.GetAccountsByType("lite")
	if len(liteAccounts) != 10 {
		t.Errorf("GetAccountsByType(lite) = %d accounts, want 10", len(liteAccounts))
	}

	adiTokenAccounts := init.wallet.GetAccountsByType("adi-token")
	if len(adiTokenAccounts) != 7 {
		t.Errorf("GetAccountsByType(adi-token) = %d accounts, want 7", len(adiTokenAccounts))
	}

	adiDataAccounts := init.wallet.GetAccountsByType("adi-data")
	if len(adiDataAccounts) != 5 {
		t.Errorf("GetAccountsByType(adi-data) = %d accounts, want 5", len(adiDataAccounts))
	}

	// Test non-existent type
	otherAccounts := init.wallet.GetAccountsByType("other")
	if len(otherAccounts) != 0 {
		t.Errorf("GetAccountsByType(other) = %d accounts, want 0", len(otherAccounts))
	}

	// Test count by type
	if init.wallet.CountByType("lite") != 10 {
		t.Errorf("CountByType(lite) = %d, want 10", init.wallet.CountByType("lite"))
	}
	if init.wallet.CountByType("adi-token") != 7 {
		t.Errorf("CountByType(adi-token) = %d, want 7", init.wallet.CountByType("adi-token"))
	}
	if init.wallet.CountByType("adi-data") != 5 {
		t.Errorf("CountByType(adi-data) = %d, want 5", init.wallet.CountByType("adi-data"))
	}
}

// TestBatchProcessing tests the batch processing logic.
func TestBatchProcessing(t *testing.T) {
	_, walletPath := createTestWallet(t, 25, 0, 0)

	tests := []struct {
		name        string
		batchSize   int
		wantBatches int
	}{
		{"Small batches", 5, 5},
		{"Medium batches", 10, 3},
		{"Large batches", 30, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := Config{
				WalletPath:   walletPath,
				NodeURL:      "http://localhost:8080/v3",
				BatchSize:    tt.batchSize,
				Concurrency:  5,
				InitialFunds: 1000,
			}

			init, err := NewInitializer(config)
			if err != nil {
				t.Fatalf("NewInitializer() failed: %v", err)
			}

			accounts := init.wallet.GetAccountsByType("lite")
			if len(accounts) != 25 {
				t.Fatalf("Expected 25 lite accounts, got %d", len(accounts))
			}

			// Count batches
			batches := 0
			for idx := 0; idx < len(accounts); idx += init.config.BatchSize {
				batches++
			}

			if batches != tt.wantBatches {
				t.Errorf("Number of batches = %d, want %d", batches, tt.wantBatches)
			}
		})
	}
}

// TestProcessBatchConcurrency tests concurrent batch processing.
func TestProcessBatchConcurrency(t *testing.T) {
	_, walletPath := createTestWallet(t, 10, 0, 0)

	config := Config{
		WalletPath:   walletPath,
		NodeURL:      "http://localhost:8080/v3",
		BatchSize:    10,
		Concurrency:  3,
		InitialFunds: 1000,
	}

	init, err := NewInitializer(config)
	if err != nil {
		t.Fatalf("NewInitializer() failed: %v", err)
	}

	accounts := init.wallet.GetAccountsByType("lite")
	ctx := context.Background()

	// Mock function that tracks execution
	processed := make(map[string]bool)
	var mu sync.Mutex

	mockCreateFn := func(ctx context.Context, acct *wallet.Account) error {
		mu.Lock()
		processed[acct.URL] = true
		mu.Unlock()
		time.Sleep(10 * time.Millisecond) // Simulate work
		return nil
	}

	// Process batch
	err = init.processBatch(ctx, accounts, mockCreateFn)
	if err != nil {
		t.Errorf("processBatch() failed: %v", err)
	}

	// Verify all accounts were processed
	if len(processed) != len(accounts) {
		t.Errorf("Processed %d accounts, want %d", len(processed), len(accounts))
	}
}

// TestConcurrentStatsUpdates tests thread-safe stats updates.
func TestConcurrentStatsUpdates(t *testing.T) {
	stats := &Stats{
		StartTime: time.Now(),
		Errors:    make([]string, 0),
	}

	// Simulate concurrent updates
	var wg sync.WaitGroup
	iterations := 100

	for i := 0; i < iterations; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			stats.LiteCreated.Add(1)
		}()
		go func() {
			defer wg.Done()
			stats.ADITokenCreated.Add(1)
		}()
		go func() {
			defer wg.Done()
			stats.ADIDataCreated.Add(1)
		}()
	}

	wg.Wait()

	if stats.LiteCreated.Load() != uint64(iterations) {
		t.Errorf("LiteCreated = %d, want %d", stats.LiteCreated.Load(), iterations)
	}
	if stats.ADITokenCreated.Load() != uint64(iterations) {
		t.Errorf("ADITokenCreated = %d, want %d", stats.ADITokenCreated.Load(), iterations)
	}
	if stats.ADIDataCreated.Load() != uint64(iterations) {
		t.Errorf("ADIDataCreated = %d, want %d", stats.ADIDataCreated.Load(), iterations)
	}
}

// TestWalletIntegration tests wallet loading and key extraction.
func TestWalletIntegration(t *testing.T) {
	_, walletPath := createTestWallet(t, 5, 3, 2)

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

	// Test funder account
	if init.wallet.Funder == nil {
		t.Fatal("Wallet has no funder")
	}

	funderPrivKey, err := init.wallet.Funder.GetPrivateKey()
	if err != nil {
		t.Errorf("Failed to get funder private key: %v", err)
	}
	if len(funderPrivKey) != 64 {
		t.Errorf("Funder private key length = %d, want 64", len(funderPrivKey))
	}

	funderPubKey, err := init.wallet.Funder.GetPublicKey()
	if err != nil {
		t.Errorf("Failed to get funder public key: %v", err)
	}
	if len(funderPubKey) != 32 {
		t.Errorf("Funder public key length = %d, want 32", len(funderPubKey))
	}

	// Test lite account keys
	liteAccounts := init.wallet.GetAccountsByType("lite")
	if len(liteAccounts) > 0 {
		acct := liteAccounts[0]
		privKey, err := acct.GetPrivateKey()
		if err != nil {
			t.Errorf("Failed to get lite account private key: %v", err)
		}
		if len(privKey) != 64 {
			t.Errorf("Lite account private key length = %d, want 64", len(privKey))
		}

		pubKey, err := acct.GetPublicKey()
		if err != nil {
			t.Errorf("Failed to get lite account public key: %v", err)
		}
		if len(pubKey) != 32 {
			t.Errorf("Lite account public key length = %d, want 32", len(pubKey))
		}
	}
}

// Helper function for string contains check.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
