// +build integration

package server

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// TestHistoricalDB_ChainIntegrity checks for missing/corrupted chain entries
func TestHistoricalDB_ChainIntegrity(t *testing.T) {
	server := NewServer()

	// Get DN database
	listResult, err := server.dbList(map[string]interface{}{})
	if err != nil {
		t.Fatalf("dbList failed: %v", err)
	}

	content := listResult["content"].([]map[string]interface{})
	text := content[0]["text"].(string)

	var databases []DatabaseInfo
	json.Unmarshal([]byte(text), &databases)

	var dnDB string
	for _, db := range databases {
		if db.Exists && strings.Contains(db.Name, "dn") {
			dnDB = db.Name
			break
		}
	}

	if dnDB == "" {
		t.Skip("DN database not available")
	}

	// Test multiple chains from ACME token
	testCases := []struct {
		chainName string
		account   string
	}{
		{"main", "acc://ACME"},
		{"signature", "acc://ACME"},
		{"index", "acc://ACME"},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s_%s", tc.account, tc.chainName), func(t *testing.T) {
			// Query the full chain in chunks
			chunkSize := 100
			args := map[string]interface{}{
				"database":   dnDB,
				"url":        tc.account,
				"chain_name": tc.chainName,
				"start":      0,
				"count":      chunkSize,
			}

			result, err := server.dbQueryChain(args)
			if err != nil {
				t.Logf("⚠️  Chain %s not accessible: %v", tc.chainName, err)
				return
			}

			content := result["content"].([]map[string]interface{})
			text := content[0]["text"].(string)

			var chainData map[string]interface{}
			json.Unmarshal([]byte(text), &chainData)

			height := int(chainData["height"].(float64))
			entries := chainData["entries"].([]interface{})

			t.Logf("Chain: %s/%s - Height: %d", tc.account, tc.chainName, height)

			// Check for gaps in indices
			missingIndices := []int{}
			for i := 0; i < len(entries); i++ {
				entry := entries[i].(map[string]interface{})
				expectedIndex := i
				actualIndex := int(entry["index"].(float64))

				if actualIndex != expectedIndex {
					missingIndices = append(missingIndices, expectedIndex)
				}

				// Check if hash exists and is valid
				hash := entry["hash"].(string)
				if len(hash) != 64 {
					t.Errorf("Invalid hash length at index %d: %d chars", i, len(hash))
				}
			}

			if len(missingIndices) > 0 {
				t.Errorf("❌ Found %d missing indices in first %d entries: %v",
					len(missingIndices), chunkSize, missingIndices)
			} else {
				t.Logf("✓ First %d entries are sequential with no gaps", len(entries))
			}

			// Sample check the middle of the chain
			if height > 1000 {
				midStart := height / 2
				args["start"] = midStart
				args["count"] = 50

				result, err := server.dbQueryChain(args)
				if err != nil {
					t.Errorf("❌ Failed to query middle of chain at index %d: %v", midStart, err)
					return
				}

				content := result["content"].([]map[string]interface{})
				text := content[0]["text"].(string)
				json.Unmarshal([]byte(text), &chainData)

				entries := chainData["entries"].([]interface{})
				if len(entries) == 0 {
					t.Errorf("❌ No entries found in middle section (indices %d-%d)", midStart, midStart+50)
				} else {
					// Verify sequential
					firstEntry := entries[0].(map[string]interface{})
					firstIndex := int(firstEntry["index"].(float64))
					if firstIndex != midStart {
						t.Errorf("❌ Middle section starts at wrong index: expected %d, got %d", midStart, firstIndex)
					} else {
						t.Logf("✓ Middle section (indices %d-%d) is accessible", midStart, midStart+len(entries))
					}
				}
			}
		})
	}
}

// TestHistoricalDB_CorruptedTableImpact checks if corrupted tables affect specific accounts
func TestHistoricalDB_CorruptedTableImpact(t *testing.T) {
	server := NewServer()

	// Query several known accounts to see if any are affected by corruption
	testAccounts := []string{
		"acc://ACME",
		"acc://dn.acme",
		"acc://staking.acme",
	}

	for _, accountURL := range testAccounts {
		t.Run(accountURL, func(t *testing.T) {
			// Try to query the account
			args := map[string]interface{}{
				"url": accountURL,
				// Use auto-routing
			}

			result, err := server.dbQueryAccount(args)
			if err != nil {
				t.Errorf("❌ Failed to query %s: %v", accountURL, err)
				return
			}

			content := result["content"].([]map[string]interface{})
			text := content[0]["text"].(string)

			var account map[string]interface{}
			err = json.Unmarshal([]byte(text), &account)
			if err != nil {
				t.Errorf("❌ Failed to parse account %s: %v", accountURL, err)
				return
			}

			// Check if account has expected fields
			if accountType, ok := account["type"]; ok {
				t.Logf("✓ Account %s accessible - type: %s", accountURL, accountType)
			} else {
				t.Errorf("❌ Account %s missing type field", accountURL)
			}
		})
	}
}

// TestHistoricalDB_BPTCompleteness checks if BPT walking returns consistent results
func TestHistoricalDB_BPTCompleteness(t *testing.T) {
	server := NewServer()

	// List accounts multiple times to check consistency
	runs := 3
	accountSets := make([]map[string]bool, runs)

	for i := 0; i < runs; i++ {
		args := map[string]interface{}{
			"limit": 100,
		}

		result, err := server.dbListAccounts(args)
		if err != nil {
			t.Fatalf("Run %d failed: %v", i+1, err)
		}

		content := result["content"].([]map[string]interface{})
		text := content[0]["text"].(string)

		var accountList map[string]interface{}
		json.Unmarshal([]byte(text), &accountList)

		accounts := accountList["accounts"].([]interface{})
		accountSet := make(map[string]bool)
		for _, acc := range accounts {
			accountSet[acc.(string)] = true
		}

		accountSets[i] = accountSet
		t.Logf("Run %d: Found %d accounts", i+1, len(accountSet))
	}

	// Compare all runs for consistency
	if len(accountSets[0]) != len(accountSets[1]) || len(accountSets[0]) != len(accountSets[2]) {
		t.Errorf("❌ Inconsistent account counts across runs: %d, %d, %d",
			len(accountSets[0]), len(accountSets[1]), len(accountSets[2]))
	} else {
		t.Logf("✓ Consistent account count across %d runs: %d accounts", runs, len(accountSets[0]))
	}

	// Check if accounts are identical
	for account := range accountSets[0] {
		if !accountSets[1][account] || !accountSets[2][account] {
			t.Errorf("❌ Account %s not consistent across runs", account)
		}
	}
}
