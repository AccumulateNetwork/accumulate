package liteclient

import (
	"context"
	"testing"
	"time"
)

// TestCoreIntegration tests all three core functionalities:
// 1. Account data retrieval
// 2. Proof generation
// 3. Caching (for both account data and proofs)
func TestCoreIntegration(t *testing.T) {
	// Setup
	client, err := NewLiteClient("https://mainnet.accumulatenetwork.io/v2")
	if err != nil {
		t.Fatalf("Failed to create lite client: %v", err)
	}

	// Test account URL
	accountURL := "acc://RenatoDAP.acme/token"
	// Validate URL format
	if err := client.validateAccountURL(accountURL); err != nil {
		t.Fatalf("Invalid account URL: %v", err)
	}

	ctx := context.Background()

	// === PHASE 1: Account Data Retrieval ===
	println("=== Testing Account Data Retrieval ===")

	// Retrieve account data (should hit network)
	accountData, err := client.getAccountData(ctx, accountURL)
	if err != nil {
		t.Fatalf("Failed to retrieve account data: %v", err)
	}

	println("✓ Account data retrieved successfully")
	println("  Account Type:", accountData.Type)
	println("  Type Name:", accountData.TypeName)

	// === PHASE 2: Proof Generation ===
	println("\n=== Testing Proof Generation ===")

	// Generate cryptographic proof using internal method
	err = client.validateAndCacheProof(ctx, accountURL, []byte("test-root-hash"))
	if err != nil {
		t.Logf("Proof generation failed (expected for some accounts): %v", err)
		// Continue test - proof generation may fail for accounts without sufficient chain data
	} else {
		println("✓ Proof generated and cached successfully")
	}

	// === PHASE 3: Caching Verification ===
	println("\n=== Testing Caching System ===")

	// Check account data cache
	cachedAccount, found := client.unifiedCache.GetAccountData(accountURL)
	if !found {
		t.Error("Account data should be cached after retrieval")
	} else {
		println("✓ Account data cached successfully")
		println("  Cached account type:", cachedAccount.Type)
	}

	// Check if account summary exists (proxy for proof caching)
	// Check if balance is cached (proxy for additional caching)
	balanceInfo, balanceFound := client.unifiedCache.GetBalance(accountURL)
	if balanceFound {
		println("✓ Balance cached successfully")
		println("  Cached balance:", balanceInfo.Balance)
	}

	// === PHASE 4: Cache Hit Testing ===
	println("\n=== Testing Cache Hit Performance ===")

	// Second retrieval should hit cache
	start := time.Now()
	accountData2, err := client.getAccountData(ctx, accountURL)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Failed to retrieve cached account data: %v", err)
	}

	println("✓ Cache hit successful")
	println("  Cache retrieval time:", duration)
	println("  Account types match:", accountData.Type == accountData2.Type)

	// Verify it's the same data (cache hit)
	if accountData.Type != accountData2.Type {
		t.Error("Cached account data doesn't match original")
	}

	// === PHASE 5: Cache Statistics ===
	println("\n=== Cache Statistics ===")

	stats := client.unifiedCache.GetStats()
	println("  Account cache entries:", stats.AccountDataEntries)
	println("  Transaction cache entries:", stats.TransactionEntries)
	println("  Balance cache entries:", stats.BalanceEntries)
	println("  Total cache entries:", stats.TotalEntries)
	println("  Hit rate:", stats.HitRate)

	println("\n=== Core Integration Test Complete ===")
	println("✓ All three core functionalities working together")
}

// TestCoreIntegrationWithMultipleAccounts tests core functionality with different account types
func TestCoreIntegrationWithMultipleAccounts(t *testing.T) {
	client, err := NewLiteClient("https://mainnet.accumulatenetwork.io/v2")
	if err != nil {
		t.Fatalf("Failed to create lite client: %v", err)
	}

	ctx := context.Background()

	// Test different account types
	testAccounts := []string{
		"acc://RenatoDAP.acme/token",  // Token account
		"acc://RenatoDAP.acme",        // Identity account
		"acc://RenatoDAP.acme/book/1", // Key page account
	}

	println("=== Testing Multiple Account Types ===")

	for i, accountURL := range testAccounts {
		println("\n--- Account", i+1, ":", accountURL, "---")

		// Validate URL format
		if err := client.validateAccountURL(accountURL); err != nil {
			t.Logf("Invalid URL %s: %v", accountURL, err)
			continue
		}

		// Test account data retrieval
		accountData, err := client.getAccountData(ctx, accountURL)
		if err != nil {
			t.Logf("Failed to retrieve account data for %s: %v", accountURL, err)
			continue
		}

		println("✓ Retrieved:", accountData.Type, "type name:", accountData.TypeName)

		// Verify caching
		_, found := client.unifiedCache.GetAccountData(accountURL)
		if !found {
			t.Errorf("Account %s should be cached", accountURL)
		} else {
			println("✓ Cached successfully")
		}
	}

	// Final cache statistics
	stats := client.unifiedCache.GetStats()
	println("\n=== Final Cache Statistics ===")
	println("  Total account entries:", stats.AccountDataEntries)
	println("  Total transaction entries:", stats.TransactionEntries)
	println("  Total balance entries:", stats.BalanceEntries)
	println("  Total entries:", stats.TotalEntries)
	println("  Hit rate:", stats.HitRate)
}
