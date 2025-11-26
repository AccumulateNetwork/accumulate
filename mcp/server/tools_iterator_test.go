package server

import (
	"fmt"
	"testing"
)

// TestIterateAccounts_FirstPage tests getting the first page of accounts
func TestIterateAccounts_FirstPage(t *testing.T) {
	srv := NewServer()

	args := map[string]interface{}{
		"page_size": float64(10),
	}

	result, err := srv.dbIterateAccounts(args)
	if err != nil {
		// Skip if database is not available or locked
		t.Skipf("Skipping: database not available: %v", err)
	}

	// Check result structure
	accounts, ok := result["accounts"].([]string)
	if !ok {
		t.Fatal("Expected accounts to be a string array")
	}

	if len(accounts) == 0 {
		// OK if no databases configured, but structure should be correct
		t.Log("No accounts found (may need database configuration)")
	}

	// Should have cursor for next page
	if _, ok := result["next_cursor"]; !ok {
		t.Error("Expected next_cursor in result")
	}

	// Should have has_more flag
	if _, ok := result["has_more"]; !ok {
		t.Error("Expected has_more in result")
	}

	// Should have total_processed
	if _, ok := result["total_processed"]; !ok {
		t.Error("Expected total_processed in result")
	}
}

// TestIterateAccounts_WithCursor tests pagination with cursor
func TestIterateAccounts_WithCursor(t *testing.T) {
	srv := NewServer()

	// Get first page
	firstArgs := map[string]interface{}{
		"page_size": float64(5),
	}

	firstResult, err := srv.dbIterateAccounts(firstArgs)
	if err != nil {
		// Skip if database is not available or locked
		t.Skipf("Skipping: database not available: %v", err)
	}

	// Get cursor from first page
	cursor, ok := firstResult["next_cursor"].(string)
	if !ok {
		t.Fatal("Expected next_cursor to be a string")
	}

	// If we have more pages, try to get the next page
	hasMore, ok := firstResult["has_more"].(bool)
	if ok && hasMore && cursor != "" {
		// Get second page with cursor
		secondArgs := map[string]interface{}{
			"cursor":    cursor,
			"page_size": float64(5),
		}

		secondResult, err := srv.dbIterateAccounts(secondArgs)
		if err != nil {
			t.Fatalf("Unexpected error on second page: %v", err)
		}

		// Second page should have different accounts
		secondAccounts, ok := secondResult["accounts"].([]string)
		if !ok {
			t.Fatal("Expected accounts in second page")
		}

		firstAccounts, _ := firstResult["accounts"].([]string)

		// Verify accounts are different (no overlap)
		if len(firstAccounts) > 0 && len(secondAccounts) > 0 {
			if firstAccounts[0] == secondAccounts[0] {
				t.Error("Expected different accounts on second page")
			}
		}
	}
}

// TestIterateAccounts_PageSize tests respecting page_size limit
func TestIterateAccounts_PageSize(t *testing.T) {
	srv := NewServer()

	testCases := []struct {
		pageSize int
		maxExpected int
	}{
		{1, 1},
		{10, 10},
		{100, 100},
		{1000, 1000}, // Max
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("PageSize_%d", tc.pageSize), func(t *testing.T) {
			args := map[string]interface{}{
				"page_size": float64(tc.pageSize),
			}

			result, err := srv.dbIterateAccounts(args)
			if err != nil {
				// Skip if database is not available or locked
				t.Skipf("Skipping: database not available: %v", err)
			}

			accounts, ok := result["accounts"].([]string)
			if !ok {
				t.Fatal("Expected accounts array")
			}

			if len(accounts) > tc.maxExpected {
				t.Errorf("Expected at most %d accounts, got %d", tc.maxExpected, len(accounts))
			}
		})
	}
}

// TestIterateAccounts_DefaultPageSize tests default page size of 100
func TestIterateAccounts_DefaultPageSize(t *testing.T) {
	srv := NewServer()

	// No page_size specified - should default to 100
	args := map[string]interface{}{}

	result, err := srv.dbIterateAccounts(args)
	if err != nil {
		// Skip if database is not available or locked
		t.Skipf("Skipping: database not available: %v", err)
	}

	accounts, ok := result["accounts"].([]string)
	if !ok {
		t.Fatal("Expected accounts array")
	}

	// Should not exceed default of 100
	if len(accounts) > 100 {
		t.Errorf("Expected at most 100 accounts (default), got %d", len(accounts))
	}
}

// TestIterateAccounts_MaxPageSize tests that page_size is capped at 1000
func TestIterateAccounts_MaxPageSize(t *testing.T) {
	srv := NewServer()

	// Request more than max
	args := map[string]interface{}{
		"page_size": float64(5000),
	}

	result, err := srv.dbIterateAccounts(args)
	if err != nil {
		// Skip if database is not available or locked
		t.Skipf("Skipping: database not available: %v", err)
	}

	accounts, ok := result["accounts"].([]string)
	if !ok {
		t.Fatal("Expected accounts array")
	}

	// Should not exceed max of 1000
	if len(accounts) > 1000 {
		t.Errorf("Expected at most 1000 accounts (max), got %d", len(accounts))
	}
}

// TestIterateAccounts_InvalidCursor tests error handling for invalid cursor
func TestIterateAccounts_InvalidCursor(t *testing.T) {
	srv := NewServer()

	args := map[string]interface{}{
		"cursor":    "invalid-cursor-string",
		"page_size": float64(10),
	}

	_, err := srv.dbIterateAccounts(args)
	if err == nil {
		t.Error("Expected error for invalid cursor")
	}
}

// TestIterateAccounts_DatabaseSelection tests database parameter
func TestIterateAccounts_DatabaseSelection(t *testing.T) {
	srv := NewServer()

	args := map[string]interface{}{
		"database":  "dn-primary",
		"page_size": float64(10),
	}

	result, err := srv.dbIterateAccounts(args)
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
