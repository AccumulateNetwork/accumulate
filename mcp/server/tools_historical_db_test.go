// +build integration

package server

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestHistoricalDB_List tests listing available historical databases
func TestHistoricalDB_List(t *testing.T) {
	server := NewServer()

	result, err := server.dbList(map[string]interface{}{})
	if err != nil {
		t.Fatalf("dbList failed: %v", err)
	}

	content := result["content"].([]map[string]interface{})
	text := content[0]["text"].(string)

	t.Logf("Database list:\n%s", text)

	// Parse JSON response
	var databases []DatabaseInfo
	err = json.Unmarshal([]byte(text), &databases)
	if err != nil {
		t.Fatalf("Failed to parse database list: %v", err)
	}

	// Check we have databases listed
	if len(databases) == 0 {
		t.Fatal("No databases listed")
	}

	// Find an available database for further testing
	var availableDB string
	for _, db := range databases {
		if db.Exists {
			availableDB = db.Name
			t.Logf("✓ Found available database: %s (%.2f GB)", db.Name, float64(db.SizeBytes)/1e9)
		} else {
			t.Logf("✗ Database not available: %s at %s", db.Name, db.Path)
		}
	}

	if availableDB == "" {
		t.Skip("No historical databases available for testing")
	}
}

// TestHistoricalDB_QueryAccount tests querying a known account from historical database
func TestHistoricalDB_QueryAccount(t *testing.T) {
	// First check if we have any available databases
	server := NewServer()
	listResult, err := server.dbList(map[string]interface{}{})
	if err != nil {
		t.Fatalf("dbList failed: %v", err)
	}

	content := listResult["content"].([]map[string]interface{})
	text := content[0]["text"].(string)

	var databases []DatabaseInfo
	err = json.Unmarshal([]byte(text), &databases)
	if err != nil {
		t.Fatalf("Failed to parse database list: %v", err)
	}

	// Find DN database
	var dnDB string
	for _, db := range databases {
		if db.Exists && strings.Contains(db.Name, "dn") {
			dnDB = db.Name
			break
		}
	}

	if dnDB == "" {
		t.Skip("DN database not available for testing")
	}

	// Test querying ACME token (should exist in all databases)
	t.Run("query_acme_token", func(t *testing.T) {
		args := map[string]interface{}{
			"database": dnDB,
			"url":      "acc://ACME",
		}

		result, err := server.dbQueryAccount(args)
		if err != nil {
			t.Fatalf("Failed to query ACME token: %v", err)
		}

		content := result["content"].([]map[string]interface{})
		text := content[0]["text"].(string)

		if !strings.Contains(text, "tokenIssuer") {
			t.Error("Expected ACME to be a token issuer")
		}

		t.Logf("✓ Successfully queried ACME token")
		t.Logf("Response (first 500 chars): %s", text[:min(500, len(text))])
	})

	// Test querying DN ADI
	t.Run("query_dn_adi", func(t *testing.T) {
		args := map[string]interface{}{
			"database": dnDB,
			"url":      "acc://dn.acme",
		}

		result, err := server.dbQueryAccount(args)
		if err != nil {
			t.Fatalf("Failed to query dn.acme: %v", err)
		}

		content := result["content"].([]map[string]interface{})
		text := content[0]["text"].(string)

		if !strings.Contains(text, "dn.acme") {
			t.Error("Expected dn.acme URL in response")
		}

		t.Logf("✓ Successfully queried dn.acme ADI")
		t.Logf("Response (first 500 chars): %s", text[:min(500, len(text))])
	})
}

// TestHistoricalDB_ListAccounts tests listing accounts from BPT
func TestHistoricalDB_ListAccounts(t *testing.T) {
	server := NewServer()

	// Find an available database
	listResult, err := server.dbList(map[string]interface{}{})
	if err != nil {
		t.Fatalf("dbList failed: %v", err)
	}

	content := listResult["content"].([]map[string]interface{})
	text := content[0]["text"].(string)

	var databases []DatabaseInfo
	err = json.Unmarshal([]byte(text), &databases)
	if err != nil {
		t.Fatalf("Failed to parse database list: %v", err)
	}

	var dnDB string
	for _, db := range databases {
		if db.Exists && strings.Contains(db.Name, "dn") {
			dnDB = db.Name
			break
		}
	}

	if dnDB == "" {
		t.Skip("DN database not available for testing")
	}

	// List first 20 accounts
	args := map[string]interface{}{
		"database": dnDB,
		"limit":    20,
	}

	result, err := server.dbListAccounts(args)
	if err != nil {
		t.Fatalf("Failed to list accounts: %v", err)
	}

	content = result["content"].([]map[string]interface{})
	text = content[0]["text"].(string)

	t.Logf("Account list response:\n%s", text)

	// Parse the response
	var accountList map[string]interface{}
	err = json.Unmarshal([]byte(text), &accountList)
	if err != nil {
		t.Fatalf("Failed to parse account list: %v", err)
	}

	count := int(accountList["count"].(float64))
	if count == 0 {
		t.Error("Expected to find accounts in BPT")
	}

	t.Logf("✓ Successfully listed %d accounts from BPT", count)
}

// TestHistoricalDB_GetBPTHash tests getting BPT root hash
func TestHistoricalDB_GetBPTHash(t *testing.T) {
	server := NewServer()

	// Find an available database
	listResult, err := server.dbList(map[string]interface{}{})
	if err != nil {
		t.Fatalf("dbList failed: %v", err)
	}

	content := listResult["content"].([]map[string]interface{})
	text := content[0]["text"].(string)

	var databases []DatabaseInfo
	err = json.Unmarshal([]byte(text), &databases)
	if err != nil {
		t.Fatalf("Failed to parse database list: %v", err)
	}

	var dnDB string
	for _, db := range databases {
		if db.Exists && strings.Contains(db.Name, "dn") {
			dnDB = db.Name
			break
		}
	}

	if dnDB == "" {
		t.Skip("DN database not available for testing")
	}

	args := map[string]interface{}{
		"database": dnDB,
	}

	result, err := server.dbGetBPTHash(args)
	if err != nil {
		t.Fatalf("Failed to get BPT hash: %v", err)
	}

	content = result["content"].([]map[string]interface{})
	text = content[0]["text"].(string)

	t.Logf("BPT hash response:\n%s", text)

	// Parse the response
	var hashData map[string]interface{}
	err = json.Unmarshal([]byte(text), &hashData)
	if err != nil {
		t.Fatalf("Failed to parse BPT hash: %v", err)
	}

	bptHash := hashData["bptRootHash"].(string)
	if len(bptHash) != 64 {
		t.Errorf("Expected 64-character hex hash, got %d characters", len(bptHash))
	}

	t.Logf("✓ BPT Root Hash: %s", bptHash)
}

// TestHistoricalDB_QueryChain tests querying chain entries to get transaction hashes
func TestHistoricalDB_QueryChain(t *testing.T) {
	server := NewServer()

	// Find an available database
	listResult, err := server.dbList(map[string]interface{}{})
	if err != nil {
		t.Fatalf("dbList failed: %v", err)
	}

	content := listResult["content"].([]map[string]interface{})
	text := content[0]["text"].(string)

	var databases []DatabaseInfo
	err = json.Unmarshal([]byte(text), &databases)
	if err != nil {
		t.Fatalf("Failed to parse database list: %v", err)
	}

	var dnDB string
	for _, db := range databases {
		if db.Exists && strings.Contains(db.Name, "dn") {
			dnDB = db.Name
			break
		}
	}

	if dnDB == "" {
		t.Skip("DN database not available for testing")
	}

	// Query the main chain of ACME token (should have entries)
	args := map[string]interface{}{
		"database":   dnDB,
		"url":        "acc://ACME",
		"chain_name": "main",
		"start":      0,
		"count":      5,
	}

	result, err := server.dbQueryChain(args)
	if err != nil {
		t.Fatalf("Failed to query chain: %v", err)
	}

	content = result["content"].([]map[string]interface{})
	text = content[0]["text"].(string)

	t.Logf("Chain response:\n%s", text)

	// Parse the response
	var chainData map[string]interface{}
	err = json.Unmarshal([]byte(text), &chainData)
	if err != nil {
		t.Fatalf("Failed to parse chain data: %v", err)
	}

	// Verify we got the expected fields
	if _, ok := chainData["name"]; !ok {
		t.Error("Expected 'name' field in chain data")
	}
	if _, ok := chainData["type"]; !ok {
		t.Error("Expected 'type' field in chain data")
	}
	if _, ok := chainData["height"]; !ok {
		t.Error("Expected 'height' field in chain data")
	}
	if _, ok := chainData["entries"]; !ok {
		t.Error("Expected 'entries' field in chain data")
	}

	entries := chainData["entries"].([]interface{})
	if len(entries) == 0 {
		t.Error("Expected at least one entry in ACME main chain")
	}

	// Verify first entry has index and hash
	if len(entries) > 0 {
		entry := entries[0].(map[string]interface{})
		if _, ok := entry["index"]; !ok {
			t.Error("Expected 'index' in entry")
		}
		if _, ok := entry["hash"]; !ok {
			t.Error("Expected 'hash' in entry")
		}
		t.Logf("✓ Successfully queried chain with %d entries", len(entries))
		t.Logf("✓ First entry hash: %s", entry["hash"])
	}
}

// TestHistoricalDB_AutoRouting tests automatic database routing
func TestHistoricalDB_AutoRouting(t *testing.T) {
	server := NewServer()

	// Test querying ACME without specifying database (should auto-route to DN)
	args := map[string]interface{}{
		"url": "acc://ACME",
		// No database parameter - should auto-route
	}

	result, err := server.dbQueryAccount(args)
	if err != nil {
		t.Fatalf("Failed to query with auto-routing: %v", err)
	}

	content := result["content"].([]map[string]interface{})
	text := content[0]["text"].(string)

	if !strings.Contains(text, "tokenIssuer") {
		t.Error("Expected ACME to be a token issuer")
	}

	t.Logf("✓ Auto-routing successfully queried ACME from DN database")
	t.Logf("Response (first 200 chars): %s", text[:min(200, len(text))])
}

// TestHistoricalDB_ListAccountsAutoRoute tests listing accounts with auto-routing (defaults to DN)
func TestHistoricalDB_ListAccountsAutoRoute(t *testing.T) {
	server := NewServer()

	// List accounts without specifying database (should default to DN)
	args := map[string]interface{}{
		"limit": 10,
		// No database parameter - should use DN by default
	}

	result, err := server.dbListAccounts(args)
	if err != nil {
		t.Fatalf("Failed to list accounts with auto-routing: %v", err)
	}

	content := result["content"].([]map[string]interface{})
	text := content[0]["text"].(string)

	// Parse the response
	var accountList map[string]interface{}
	err = json.Unmarshal([]byte(text), &accountList)
	if err != nil {
		t.Fatalf("Failed to parse account list: %v", err)
	}

	// Verify database field is present and set to dn-primary
	database := accountList["database"].(string)
	if database != "dn-primary" {
		t.Errorf("Expected database to be 'dn-primary', got '%s'", database)
	}

	count := int(accountList["count"].(float64))
	if count == 0 {
		t.Error("Expected to find accounts in BPT")
	}

	t.Logf("✓ Successfully listed %d accounts from %s database (auto-routed)", count, database)
	t.Logf("Sample accounts: %v", accountList["accounts"].([]interface{})[:min(3, count)])
}

// TestHistoricalDB_QueryTransaction tests querying a transaction
// This test is skipped by default as we need a known transaction ID
func TestHistoricalDB_QueryTransaction(t *testing.T) {
	t.Skip("Requires a known historical transaction ID - run manually with specific TXID")

	// Example (replace with actual transaction ID from your database):
	// args := map[string]interface{}{
	// 	"database": "dn-primary",
	// 	"txid":     "acc://example.acme@1234567890abcdef...",
	// }
	//
	// result, err := server.dbQueryTransaction(args)
	// if err != nil {
	// 	t.Fatalf("Failed to query transaction: %v", err)
	// }
	//
	// content := result["content"].([]map[string]interface{})
	// text := content[0]["text"].(string)
	// t.Logf("Transaction:\n%s", text)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
