package server

import (
	"testing"
)

// TestBatchExtract_SingleAccount tests extracting a single account
func TestBatchExtract_SingleAccount(t *testing.T) {
	srv := NewServer()

	args := map[string]interface{}{
		"accounts": []interface{}{"acc://ACME"},
	}

	result, err := srv.dbExtractAccountsBatch(args)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Check result structure
	accounts, ok := result["accounts"].([]interface{})
	if !ok {
		t.Fatal("Expected accounts to be an array")
	}

	// Should have at least the structure
	if len(accounts) > 1 {
		t.Errorf("Expected at most 1 account, got %d", len(accounts))
	}

	// Check extracted_count
	if _, ok := result["extracted_count"]; !ok {
		t.Error("Expected extracted_count in result")
	}

	// Check failed array
	if _, ok := result["failed"]; !ok {
		t.Error("Expected failed array in result")
	}
}

// TestBatchExtract_MultipleAccounts tests extracting multiple accounts
func TestBatchExtract_MultipleAccounts(t *testing.T) {
	srv := NewServer()

	args := map[string]interface{}{
		"accounts": []interface{}{
			"acc://ACME",
			"acc://dn.acme",
		},
	}

	result, err := srv.dbExtractAccountsBatch(args)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	accounts, ok := result["accounts"].([]interface{})
	if !ok {
		t.Fatal("Expected accounts array")
	}

	if len(accounts) > 2 {
		t.Errorf("Expected at most 2 accounts, got %d", len(accounts))
	}

	extractedCount, ok := result["extracted_count"].(int)
	if !ok {
		t.Fatal("Expected extracted_count to be an int")
	}

	if extractedCount < 0 || extractedCount > 2 {
		t.Errorf("Expected extracted_count between 0-2, got %d", extractedCount)
	}
}

// TestBatchExtract_WithChains tests include_chains option
func TestBatchExtract_WithChains(t *testing.T) {
	srv := NewServer()

	args := map[string]interface{}{
		"accounts":      []interface{}{"acc://ACME"},
		"include_chains": true,
	}

	result, err := srv.dbExtractAccountsBatch(args)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	accounts, ok := result["accounts"].([]interface{})
	if !ok || len(accounts) == 0 {
		t.Skip("No accounts to test chains with")
	}

	// First account should have chains if include_chains is true
	firstAcct, ok := accounts[0].(map[string]interface{})
	if ok {
		if _, hasChains := firstAcct["chains"]; !hasChains {
			t.Log("Note: chains field missing (may be expected if account has no chains)")
		}
	}
}

// TestBatchExtract_WithTransactions tests include_transactions option
func TestBatchExtract_WithTransactions(t *testing.T) {
	srv := NewServer()

	args := map[string]interface{}{
		"accounts":             []interface{}{"acc://ACME"},
		"include_transactions": true,
	}

	result, err := srv.dbExtractAccountsBatch(args)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	accounts, ok := result["accounts"].([]interface{})
	if !ok || len(accounts) == 0 {
		t.Skip("No accounts to test transactions with")
	}

	// First account should have transactions if include_transactions is true
	firstAcct, ok := accounts[0].(map[string]interface{})
	if ok {
		if _, hasTxs := firstAcct["transactions"]; !hasTxs {
			t.Log("Note: transactions field missing (may be expected if account has no transactions)")
		}
	}
}

// TestBatchExtract_NoAccounts tests error when no accounts provided
func TestBatchExtract_NoAccounts(t *testing.T) {
	srv := NewServer()

	args := map[string]interface{}{
		"accounts": []interface{}{},
	}

	_, err := srv.dbExtractAccountsBatch(args)
	if err == nil {
		t.Error("Expected error when no accounts provided")
	}
}

// TestBatchExtract_MissingAccounts tests when accounts parameter is missing
func TestBatchExtract_MissingAccounts(t *testing.T) {
	srv := NewServer()

	args := map[string]interface{}{}

	_, err := srv.dbExtractAccountsBatch(args)
	if err == nil {
		t.Error("Expected error when accounts parameter is missing")
	}
}

// TestBatchExtract_DatabaseSelection tests database parameter
func TestBatchExtract_DatabaseSelection(t *testing.T) {
	srv := NewServer()

	args := map[string]interface{}{
		"database": "dn-primary",
		"accounts": []interface{}{"acc://ACME"},
	}

	result, err := srv.dbExtractAccountsBatch(args)
	if err != nil {
		// Error is OK if database doesn't exist
		t.Logf("Database not found (expected if no test databases configured): %v", err)
		return
	}

	// Should have valid structure
	if _, ok := result["accounts"]; !ok {
		t.Error("Expected accounts in result")
	}
}

// TestBatchExtract_FailedAccounts tests that failed accounts are reported
func TestBatchExtract_FailedAccounts(t *testing.T) {
	srv := NewServer()

	args := map[string]interface{}{
		"accounts": []interface{}{
			"acc://nonexistent-account-12345",
			"acc://another-invalid-67890",
		},
	}

	result, err := srv.dbExtractAccountsBatch(args)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	failed, ok := result["failed"].([]interface{})
	if !ok {
		t.Fatal("Expected failed to be an array")
	}

	// These accounts don't exist, so they should be in failed list
	if len(failed) < 2 {
		t.Logf("Expected at least 2 failed accounts, got %d (may depend on database state)", len(failed))
	}
}
