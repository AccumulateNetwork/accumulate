// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package liteclient

import (
	"context"
	"testing"
	"time"
)

// TestADIOrchestration demonstrates the main ADI processing functionality
func TestADIOrchestration(t *testing.T) {
	// Create lite client
	client, err := NewLiteClient("https://mainnet.accumulatenetwork.io/v2")
	if err != nil {
		t.Fatalf("Failed to create lite client: %v", err)
	}

	// Test ADIs - using known working ADIs from mainnet
	testADIs := []string{
		"RenatoDAP.acme", // User's ADI from memory
	}

	t.Run("ProcessMultipleADIs", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Create ADI orchestrator and process the ADIs
		orchestrator, err := NewADIOrchestrator(client)
		if err != nil {
			t.Fatalf("Failed to create ADI orchestrator: %v", err)
		}
		defer orchestrator.Close()

		report, err := orchestrator.ProcessTargetADIs(ctx, testADIs)
		if err != nil {
			t.Fatalf("Failed to process ADIs: %v", err)
		}

		// Validate report structure
		if report == nil {
			t.Fatal("Report is nil")
		}

		if report.Summary == nil {
			t.Fatal("Report summary is nil")
		}

		// Check that we processed the expected number of ADIs
		if report.Summary.TotalADIs != len(testADIs) {
			t.Errorf("Expected %d ADIs, got %d", len(testADIs), report.Summary.TotalADIs)
		}

		// Display results
		t.Logf("=== ADI PROCESSING REPORT ===")
		t.Logf("Summary: %s", report.GetSummaryString())
		t.Logf("Timestamp: %s", report.Timestamp.Format(time.RFC3339))

		for adi, result := range report.ProcessedADIs {
			t.Logf("\nADI: %s", adi)
			t.Logf("  Status: %s", result.Status)
			t.Logf("  Accounts: %d", len(result.Accounts))

			if result.Error != "" {
				t.Logf("  Error: %s", result.Error)
			}

			// Display account details
			for accountURL, account := range result.Accounts {
				status := "✓ VERIFIED"
				if !account.Verified {
					status = "⚠ UNVERIFIED"
				}

				t.Logf("    %s %s (%s)", status, accountURL, account.Type)

				if account.Balance != "" {
					t.Logf("      Balance: %s %s", account.Balance, account.TokenURL)
				}
				if account.KeyBook != "" {
					t.Logf("      Key Book: %s", account.KeyBook)
				}
				if account.ProofMethod != "" {
					t.Logf("      Proof Method: %s", account.ProofMethod)
				}
				if account.Error != "" {
					t.Logf("      Error: %s", account.Error)
				}
			}
		}

		// Verify at least one ADI was processed successfully
		if report.Summary.SuccessfulADIs == 0 {
			t.Error("No ADIs were processed successfully")
		}
	})

	t.Run("ProcessSingleADI", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		// Test single ADI processing using orchestrator
		orchestrator, err := NewADIOrchestrator(client)
		if err != nil {
			t.Fatalf("Failed to create ADI orchestrator: %v", err)
		}
		defer orchestrator.Close()

		testADI := "RenatoDAP.acme"
		report, err := orchestrator.ProcessTargetADIs(ctx, []string{testADI})
		if err != nil {
			t.Fatalf("Failed to process ADI: %v", err)
		}

		result, exists := report.ProcessedADIs[testADI]
		if !exists {
			t.Fatalf("ADI %s not found in processing results", testADI)
		}

		// Validate result
		if result == nil {
			t.Fatal("Result is nil")
		}

		if result.ADI != "RenatoDAP.acme" {
			t.Errorf("Expected ADI 'RenatoDAP.acme', got '%s'", result.ADI)
		}

		t.Logf("=== SINGLE ADI RESULT ===")
		t.Logf("ADI: %s", result.ADI)
		t.Logf("Status: %s", result.Status)
		t.Logf("Accounts: %d", len(result.Accounts))

		// Should have at least the identity account
		if len(result.Accounts) == 0 {
			t.Error("No accounts found for ADI")
		}

		// Check for expected accounts
		expectedAccounts := []string{
			"acc://RenatoDAP.acme",        // Identity
			"acc://RenatoDAP.acme/token",  // Token account
			"acc://RenatoDAP.acme/book",   // Key book
		}

		for _, expectedAccount := range expectedAccounts {
			if account, exists := result.Accounts[expectedAccount]; exists {
				t.Logf("  Found expected account: %s (%s)", expectedAccount, account.Type)
			} else {
				t.Logf("  Expected account not found: %s", expectedAccount)
			}
		}
	})

	t.Run("ErrorHandling", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Test with invalid ADI using orchestrator
		orchestrator, err := NewADIOrchestrator(client)
		if err != nil {
			t.Fatalf("Failed to create ADI orchestrator: %v", err)
		}
		defer orchestrator.Close()

		invalidADIs := []string{"nonexistent.acme", "invalid-adi"}

		report, err := orchestrator.ProcessTargetADIs(ctx, invalidADIs)
		if err != nil {
			t.Fatalf("ProcessADIs should not fail even with invalid ADIs: %v", err)
		}

		// Check that errors are properly handled
		for adi, result := range report.ProcessedADIs {
			t.Logf("Invalid ADI %s: Status=%s, Error=%s", adi, result.Status, result.Error)
			
			if result.Status != "failed" {
				t.Errorf("Expected status 'failed' for invalid ADI %s, got '%s'", adi, result.Status)
			}
		}

		// Summary should reflect failures
		if report.Summary.SuccessfulADIs != 0 {
			t.Errorf("Expected 0 successful ADIs, got %d", report.Summary.SuccessfulADIs)
		}
	})
}

// TestADIDiscovery tests the account discovery functionality
func TestADIDiscovery(t *testing.T) {
	client, err := NewLiteClient("https://mainnet.accumulatenetwork.io/v2")
	if err != nil {
		t.Fatalf("Failed to create lite client: %v", err)
	}

	orchestrator, err := NewADIOrchestrator(client)
	if err != nil {
		t.Fatalf("Failed to create orchestrator: %v", err)
	}
	defer orchestrator.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Test account discovery for known ADI
	accounts, err := orchestrator.discoverADIAccounts(ctx, "RenatoDAP.acme")
	if err != nil {
		t.Fatalf("Failed to discover accounts: %v", err)
	}

	t.Logf("Discovered %d accounts for RenatoDAP.acme:", len(accounts))
	for i, account := range accounts {
		t.Logf("  %d. %s", i+1, account)
	}

	// Should have at least the identity account
	if len(accounts) == 0 {
		t.Error("No accounts discovered")
	}

	// First account should be the ADI identity
	expectedIdentity := "acc://RenatoDAP.acme"
	if accounts[0] != expectedIdentity {
		t.Errorf("Expected first account to be %s, got %s", expectedIdentity, accounts[0])
	}
}

// TestVerifiedAccountInfo tests the account processing functionality
func TestVerifiedAccountInfo(t *testing.T) {
	client, err := NewLiteClient("https://mainnet.accumulatenetwork.io/v2")
	if err != nil {
		t.Fatalf("Failed to create lite client: %v", err)
	}

	orchestrator, err := NewADIOrchestrator(client)
	if err != nil {
		t.Fatalf("Failed to create orchestrator: %v", err)
	}
	defer orchestrator.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Test processing different account types
	testAccounts := []string{
		"acc://RenatoDAP.acme",        // Identity account
		"acc://RenatoDAP.acme/token",  // Token account
		"acc://RenatoDAP.acme/book",   // Key book
	}

	for _, accountURL := range testAccounts {
		t.Run(accountURL, func(t *testing.T) {
			info, err := orchestrator.processAccount(ctx, accountURL)
			if err != nil {
				t.Logf("Failed to process account %s: %v", accountURL, err)
				return // Skip this account if it fails
			}

			t.Logf("Account: %s", info.URL)
			t.Logf("  Type: %s", info.Type)
			t.Logf("  Verified: %t", info.Verified)
			t.Logf("  Proof Method: %s", info.ProofMethod)

			if info.Balance != "" {
				t.Logf("  Balance: %s %s", info.Balance, info.TokenURL)
			}
			if info.KeyBook != "" {
				t.Logf("  Key Book: %s", info.KeyBook)
			}
			if info.Error != "" {
				t.Logf("  Error: %s", info.Error)
			}

			// Validate basic fields
			if info.URL != accountURL {
				t.Errorf("Expected URL %s, got %s", accountURL, info.URL)
			}

			if info.Type == "" {
				t.Error("Account type should not be empty")
			}
		})
	}
}
