// +build integration

package server

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestAllDatabases_Corruption checks corruption status across all 5 databases
func TestAllDatabases_Corruption(t *testing.T) {
	server := NewServer()

	// Get all databases
	listResult, err := server.dbList(map[string]interface{}{})
	if err != nil {
		t.Fatalf("dbList failed: %v", err)
	}

	content := listResult["content"].([]map[string]interface{})
	text := content[0]["text"].(string)

	var databases []DatabaseInfo
	json.Unmarshal([]byte(text), &databases)

	// Test each database
	for _, dbInfo := range databases {
		if !dbInfo.Exists {
			t.Logf("⏭️  Skipping %s (not available)", dbInfo.Name)
			continue
		}

		t.Run(dbInfo.Name, func(t *testing.T) {
			t.Logf("Testing database: %s (%s)", dbInfo.Name, dbInfo.SizeGB)

			// Test 1: BPT Walking
			args := map[string]interface{}{
				"database": dbInfo.Name,
				"limit":    100,
			}

			result, err := server.dbListAccounts(args)
			if err != nil {
				t.Errorf("❌ BPT walk failed completely: %v", err)
				return
			}

			content := result["content"].([]map[string]interface{})
			text := content[0]["text"].(string)

			var accountList map[string]interface{}
			json.Unmarshal([]byte(text), &accountList)

			count := int(accountList["count"].(float64))
			isPartial := accountList["partial"] != nil && accountList["partial"].(bool)

			if isPartial {
				warning := accountList["warning"].(string)
				t.Logf("⚠️  BPT walk partial: retrieved %d accounts before corruption", count)
				t.Logf("    Warning: %s", warning)
			} else {
				t.Logf("✓ BPT walk successful: %d accounts (no corruption detected)", count)
			}

			// Test 2: Known account queries
			testAccounts := []string{
				"acc://ACME",
				"acc://dn.acme",
			}

			for _, accountURL := range testAccounts {
				queryArgs := map[string]interface{}{
					"database": dbInfo.Name,
					"url":      accountURL,
				}

				result, err := server.dbQueryAccount(queryArgs)
				if err != nil {
					if strings.Contains(err.Error(), "not found") {
						t.Logf("  ⏭️  %s: not in this partition (expected)", accountURL)
					} else {
						t.Logf("  ❌ %s: query failed: %v", accountURL, err)
					}
					continue
				}

				content := result["content"].([]map[string]interface{})
				text := content[0]["text"].(string)

				var account map[string]interface{}
				if err := json.Unmarshal([]byte(text), &account); err == nil {
					t.Logf("  ✓ %s: accessible (type: %s)", accountURL, account["type"])
				}
			}

			// Test 3: Chain integrity (if we have accessible accounts)
			if count > 0 {
				accounts := accountList["accounts"].([]interface{})
				testAccount := accounts[0].(string)

				chainArgs := map[string]interface{}{
					"database":   dbInfo.Name,
					"url":        testAccount,
					"chain_name": "main",
					"start":      0,
					"count":      10,
				}

				result, err := server.dbQueryChain(chainArgs)
				if err != nil {
					if !strings.Contains(err.Error(), "not found") {
						t.Logf("  ⚠️  Chain query failed for %s: %v", testAccount, err)
					}
				} else {
					content := result["content"].([]map[string]interface{})
					text := content[0]["text"].(string)

					var chainData map[string]interface{}
					if err := json.Unmarshal([]byte(text), &chainData); err == nil {
						height := int(chainData["height"].(float64))
						t.Logf("  ✓ Chain accessible: %s/main (height: %d)", testAccount, height)
					}
				}
			}
		})
	}
}

// TestAllDatabases_Summary provides a summary of all databases
func TestAllDatabases_Summary(t *testing.T) {
	server := NewServer()

	listResult, err := server.dbList(map[string]interface{}{})
	if err != nil {
		t.Fatalf("dbList failed: %v", err)
	}

	content := listResult["content"].([]map[string]interface{})
	text := content[0]["text"].(string)

	var databases []DatabaseInfo
	json.Unmarshal([]byte(text), &databases)

	t.Log("\n=== Database Summary ===")
	totalSize := int64(0)
	availableCount := 0

	for _, db := range databases {
		if db.Exists {
			t.Logf("✓ %-15s: %10s at %s", db.Name, db.SizeGB, db.Path)
			totalSize += db.SizeBytes
			availableCount++
		} else {
			t.Logf("✗ %-15s: Not available", db.Name)
		}
	}

	t.Logf("\nTotal: %d available databases, %.2f GB", availableCount, float64(totalSize)/1e9)
}
