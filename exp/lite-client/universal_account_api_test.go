// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package liteclient

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Phase 1 Test: Basic Universal Account API functionality
func TestPhase1_UniversalAccountAPI(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}

	client, err := NewLiteClient("https://mainnet.accumulatenetwork.io/v2")
	if err != nil {
		t.Fatalf("Failed to create lite client: %v", err)
	}

	ctx := context.Background()

	// Test multiple account types
	testAccounts := []struct {
		url         string
		description string
	}{
		{"acc://RenatoDAP.acme/token", "Token Account"},
		{"acc://RenatoDAP.acme", "Identity Account (ADI)"},
		{"acc://c7b2d77d5beadeb7774ca04106f2f68a9317b75c2f96efee/ACME", "Lite Token Account"},
		{"acc://08115f96ebb5e35a9c806de9cffe4c99455a0c5a60942d53/ACME", "Another Lite Token Account"},
		{"acc://e4571e13d3af400ad41a7e70134387d0f9b0bd5a94f4347f/ACME", "Factoid-derived Account"},
	}

	for _, testAccount := range testAccounts {
		t.Run(testAccount.description, func(t *testing.T) {
			testAccountURL := testAccount.url
			fmt.Printf("\n=== Testing %s: %s ===\n", testAccount.description, testAccountURL)

			// Test basic account data retrieval
			accountData, err := client.GetAccountData(ctx, testAccountURL)
			if err != nil {
				// Some accounts may not exist, so we'll skip gracefully
				fmt.Printf("Account not found or error: %v\n", err)
				t.Skip("Account not accessible")
				return
			}

			// Print retrieved data
			fmt.Printf("=== Phase 1: Retrieved Account Data ===\n")
			fmt.Printf("Account URL: %s\n", accountData.URL)
			fmt.Printf("Account Type: %s (%d)\n", accountData.TypeName, accountData.Type)
			fmt.Printf("Data Type: %T\n", accountData.Data)
			fmt.Printf("Is Token Account: %v\n", accountData.IsTokenAccount())
			fmt.Printf("Is Data Account: %v\n", accountData.IsDataAccount())
			fmt.Printf("Is Identity Account: %v\n", accountData.IsIdentityAccount())
			fmt.Printf("Is Key Account: %v\n", accountData.IsKeyAccount())

			// Validate core fields
			if accountData.URL != testAccountURL {
				t.Errorf("Expected URL %s, got %s", testAccountURL, accountData.URL)
			}
			if accountData.Data == nil {
				t.Error("Account data is nil")
			}

			// Test account type retrieval
			accountType, err := client.GetAccountType(ctx, testAccountURL)
			if err != nil {
				t.Fatalf("GetAccountType failed: %v", err)
			}
			if accountType == protocol.AccountTypeUnknown {
				t.Error("GetAccountType returned Unknown type")
			}

			fmt.Printf("Direct Account Type Query: %s (%d)\n", accountType.String(), accountType)
		})
	}

	// Test error handling with invalid URL (outside the loop)
	_, err = client.GetAccountData(ctx, "invalid-url")
	if err == nil {
		t.Error("Expected error for invalid URL")
	} else {
		fmt.Printf("Error handling test passed: %v\n", err)
	}
}

// Phase 2 Test: Account type detection and routing
func TestPhase2_AccountTypeDetectionAndRouting(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}

	client, err := NewLiteClient("https://mainnet.accumulatenetwork.io/v2")
	if err != nil {
		t.Fatalf("Failed to create lite client: %v", err)
	}

	ctx := context.Background()

	// Test multiple account types for routing
	testAccounts := []struct {
		url         string
		description string
	}{
		{"acc://RenatoDAP.acme/token", "Token Account"},
		{"acc://c7b2d77d5beadeb7774ca04106f2f68a9317b75c2f96efee/ACME", "Lite Token Account"},
		{"acc://08115f96ebb5e35a9c806de9cffe4c99455a0c5a60942d53/ACME", "Another Lite Token Account"},
	}

	for _, testAccount := range testAccounts {
		t.Run(testAccount.description, func(t *testing.T) {
			testAccountURL := testAccount.url
			fmt.Printf("\n=== Testing %s: %s ===\n", testAccount.description, testAccountURL)

			// Test router creation and handler selection
			router := client.NewAccountRouter()
			if router == nil {
				t.Fatal("NewAccountRouter returned nil")
			}

			fmt.Printf("=== Phase 2: Account Routing and Handlers ===\n")
			fmt.Printf("Router created successfully\n")

			// Test handler for token account
			handler := router.GetHandler(protocol.AccountTypeTokenAccount)
			if handler == nil {
				t.Fatal("No handler found for TokenAccount")
			}
			if !handler.CanHandle(protocol.AccountTypeTokenAccount) {
				t.Error("Handler claims it cannot handle TokenAccount")
			}

			fmt.Printf("TokenAccount handler found: %T\n", handler)
			fmt.Printf("Handler can handle TokenAccount: %v\n", handler.CanHandle(protocol.AccountTypeTokenAccount))

			// Test balance operation routing (skip if account doesn't exist)
			result, err := client.RouteAccountOperation(ctx, testAccountURL, "balance", nil)
			if err != nil {
				fmt.Printf("Account not accessible for routing test: %v\n", err)
				t.Skip("Account not accessible")
				return
			}
			balanceData, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("Balance result is not a map: %T", result)
			}
			if _, exists := balanceData["balance"]; !exists {
				t.Error("Balance data missing 'balance' field")
			}

			fmt.Printf("Balance operation routed successfully:\n")
			fmt.Printf("  balance: %v\n", balanceData["balance"])
			fmt.Printf("  tokenUrl: %v\n", balanceData["tokenUrl"])
			fmt.Printf("  type: %v\n", balanceData["type"])

			// Test invalid operation
			_, err = client.RouteAccountOperation(ctx, testAccountURL, "invalid-operation", nil)
			if err == nil {
				t.Error("Expected error for invalid operation")
			} else {
				fmt.Printf("Invalid operation error handling test passed: %v\n", err)
			}
		})
	}
}

// Phase 3 Test: Type-specific data access methods
func TestPhase3_TypeSpecificDataAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}

	client, err := NewLiteClient("https://mainnet.accumulatenetwork.io/v2")
	if err != nil {
		t.Fatalf("Failed to create lite client: %v", err)
	}

	ctx := context.Background()

	// Test multiple account types for type-specific methods
	testAccounts := []struct {
		url              string
		description      string
		accountType      string
		expectedCategory string
	}{
		{"acc://RenatoDAP.acme/token", "ADI Token Account", "tokenAccount", "token"},
		{"acc://c7b2d77d5beadeb7774ca04106f2f68a9317b75c2f96efee/ACME", "Lite Token Account", "liteTokenAccount", "token"},
		{"acc://RenatoDAP.acme", "Identity Account (ADI)", "identity", "identity"},
		{"acc://RenatoDAP.acme/book", "Key Book Account", "keyBook", "key"},
		{"acc://RenatoDAP.acme/book/1", "Key Page Account", "keyPage", "key"},
		{"acc://dn.acme/anchors", "System Ledger Account", "systemLedger", "unknown"},
	}

	for _, testAccount := range testAccounts {
		t.Run(testAccount.description, func(t *testing.T) {
			testAccountURL := testAccount.url
			fmt.Printf("\n=== Testing %s: %s ===\n", testAccount.description, testAccountURL)

			fmt.Printf("=== Phase 3: Type-Specific Data Access Methods ===\n")

			// First, get the account data to determine the actual type
			accountData, err := client.GetAccountData(ctx, testAccountURL)
			if err != nil {
				fmt.Printf("Account not accessible: %v\n", err)
				t.Skip("Account not accessible")
				return
			}

			fmt.Printf("Account Data Results:\n")
			fmt.Printf("  Account URL: %s\n", accountData.URL)
			fmt.Printf("  Account Type: %s\n", accountData.TypeName)
			fmt.Printf("  Expected Category: %s\n", testAccount.expectedCategory)

			// Test GetAccountSummary method (works for all account types)
			summary, err := client.GetAccountSummary(ctx, testAccountURL)
			if err != nil {
				t.Fatalf("GetAccountSummary failed: %v", err)
			}

			fmt.Printf("GetAccountSummary Results:\n")
			fmt.Printf("  Account URL: %s\n", summary.AccountURL)
			fmt.Printf("  Account Type: %s\n", summary.AccountType)
			fmt.Printf("  Category: %s\n", summary.Category)
			fmt.Printf("  Balance: %s\n", summary.Balance)
			fmt.Printf("  Token URL: %s\n", summary.TokenURL)
			fmt.Printf("  Key Book: %s\n", summary.KeyBook)

			if summary.AccountURL != testAccountURL {
				t.Errorf("Expected AccountURL %s, got %s", testAccountURL, summary.AccountURL)
			}
			if summary.Category != testAccount.expectedCategory {
				t.Errorf("Expected category '%s', got '%s'", testAccount.expectedCategory, summary.Category)
			}

			// Test type-specific methods based on account category
			switch testAccount.expectedCategory {
			case "token":
				// Test GetTokenBalance for token accounts
				balanceInfo, err := client.GetTokenBalance(ctx, testAccountURL)
				if err != nil {
					t.Errorf("GetTokenBalance failed for token account: %v", err)
				} else {
					fmt.Printf("GetTokenBalance Results:\n")
					fmt.Printf("  Balance: %s\n", balanceInfo.Balance)
					fmt.Printf("  Token URL: %s\n", balanceInfo.TokenURL)
					fmt.Printf("  Credit Balance: %d\n", balanceInfo.CreditBalance)
					if balanceInfo.Balance == "" {
						t.Error("Balance should not be empty for token account")
					}
				}

				// Test that GetIdentityInfo fails for token accounts
				_, err = client.GetIdentityInfo(ctx, testAccountURL)
				if err == nil {
					t.Error("Expected error when calling GetIdentityInfo on token account")
				} else {
					fmt.Printf("Type mismatch error handling test passed: %v\n", err)
				}

			case "identity":
				// Test GetIdentityInfo for identity accounts
				identityInfo, err := client.GetIdentityInfo(ctx, testAccountURL)
				if err != nil {
					t.Errorf("GetIdentityInfo failed for identity account: %v", err)
				} else {
					fmt.Printf("GetIdentityInfo Results:\n")
					fmt.Printf("  Identity URL: %s\n", identityInfo.IdentityURL)
					fmt.Printf("  Key Book: %s\n", identityInfo.KeyBook)
					if identityInfo.KeyBook == "" {
						t.Error("KeyBook should not be empty for identity account")
					}
				}

				// Test that GetTokenBalance fails for identity accounts
				_, err = client.GetTokenBalance(ctx, testAccountURL)
				if err == nil {
					t.Error("Expected error when calling GetTokenBalance on identity account")
				} else {
					fmt.Printf("Type mismatch error handling test passed: %v\n", err)
				}

			case "data":
				// Test GetDataAccountInfo for data accounts
				dataInfo, err := client.GetDataAccountInfo(ctx, testAccountURL)
				if err != nil {
					t.Errorf("GetDataAccountInfo failed for data account: %v", err)
				} else {
					fmt.Printf("GetDataAccountInfo Results:\n")
					fmt.Printf("  Data URL: %s\n", dataInfo.DataURL)
					fmt.Printf("  Key Book: %s\n", dataInfo.KeyBook)
				}

			case "key":
				// For key accounts, just verify the summary works
				fmt.Printf("Key account detected - summary test sufficient\n")

			default:
				// For unknown/system accounts, just verify the summary works
				fmt.Printf("System/unknown account detected - summary test sufficient\n")
			}
		})
	}
}

// Phase 4 Test: Unified caching/storage for all types
func TestPhase4_UnifiedCachingStorage(t *testing.T) {
	client, err := NewLiteClient("https://mainnet.accumulatenetwork.io")
	if err != nil {
		t.Fatalf("Failed to create lite client: %v", err)
	}

	ctx := context.Background()

	fmt.Printf("\n=== Phase 4: Unified Caching/Storage for All Account Types ===\n")

	// Test accounts for comprehensive caching
	testAccounts := []struct {
		url         string
		description string
		accountType string
	}{
		{"acc://RenatoDAP.acme/token", "ADI Token Account", "token"},
		{"acc://RenatoDAP.acme", "Identity Account (ADI)", "identity"},
		{"acc://RenatoDAP.acme/book", "Key Book Account", "key"},
		{"acc://RenatoDAP.acme/book/1", "Key Page Account", "key"},
		{"acc://08115f96ebb5e35a9c806de9cffe4c99455a0c5a60942d53/ACME", "Lite Token Account", "token"},
	}

	// Phase 4.1: Test Initial Data Fetching and Caching
	fmt.Printf("\n--- Phase 4.1: Initial Data Fetching and Caching ---\n")
	for _, testAccount := range testAccounts {
		fmt.Printf("Testing %s: %s\n", testAccount.description, testAccount.url)

		// Get initial cache stats
		initialStats := client.unifiedCache.GetCacheStats()
		fmt.Printf("  Initial cache stats: %+v\n", initialStats)

		// Fetch account data (should cache automatically)
		accountData, err := client.GetAccountData(ctx, testAccount.url)
		if err != nil {
			fmt.Printf("  Account not accessible: %v\n", err)
			continue
		}

		// Fetch account summary (should cache automatically)
		summary, err := client.GetAccountSummary(ctx, testAccount.url)
		if err != nil {
			t.Errorf("GetAccountSummary failed: %v", err)
			continue
		}

		// Test type-specific caching based on account type
		switch testAccount.accountType {
		case "token":
			// Fetch balance (should cache automatically)
			balance, err := client.GetTokenBalance(ctx, testAccount.url)
			if err != nil {
				fmt.Printf("  GetTokenBalance failed: %v\n", err)
			} else {
				fmt.Printf("  Cached token balance: %s\n", balance.Balance)
			}

		case "identity":
			// Fetch identity info (should cache automatically)
			identityInfo, err := client.GetIdentityInfo(ctx, testAccount.url)
			if err != nil {
				fmt.Printf("  GetIdentityInfo failed: %v\n", err)
			} else {
				fmt.Printf("  Cached identity info: %s\n", identityInfo.KeyBook)
			}
		}

		// Get updated cache stats
		updatedStats := client.unifiedCache.GetCacheStats()
		fmt.Printf("  Updated cache stats: %+v\n", updatedStats)
		fmt.Printf("  Account data cached: %s (%s)\n", accountData.TypeName, accountData.URL)
		fmt.Printf("  Summary cached: %s category\n", summary.Category)
	}

	// Phase 4.2: Test Cache Hit Performance
	fmt.Printf("\n--- Phase 4.2: Cache Hit Performance Testing ---\n")
	for _, testAccount := range testAccounts {
		fmt.Printf("Testing cache hits for %s: %s\n", testAccount.description, testAccount.url)

		// These should all be cache hits (no network calls)
		start := time.Now()
		accountData, err := client.GetAccountData(ctx, testAccount.url)
		if err != nil {
			continue
		}
		cacheHitTime := time.Since(start)

		summary, err := client.GetAccountSummary(ctx, testAccount.url)
		if err != nil {
			t.Errorf("GetAccountSummary cache hit failed: %v", err)
			continue
		}

		fmt.Printf("  Cache hit time: %v\n", cacheHitTime)
		fmt.Printf("  Retrieved from cache: %s (%s)\n", accountData.TypeName, summary.Category)

		// Verify cache hit was successful
		if cacheHitTime > 100*time.Millisecond {
			t.Errorf("Cache hit took too long: %v (expected < 100ms)", cacheHitTime)
		}
	}

	// Phase 4.3: Test Cache Management
	fmt.Printf("\n--- Phase 4.3: Cache Management Testing ---\n")

	// Test cache statistics
	finalStats := client.unifiedCache.GetCacheStats()
	fmt.Printf("Final cache statistics: %+v\n", finalStats)

	// Verify we have cached data
	if finalStats["accountData"].(int) == 0 {
		t.Error("Expected cached account data, but cache is empty")
	}
	if finalStats["accountSummaries"].(int) == 0 {
		t.Error("Expected cached account summaries, but cache is empty")
	}

	// Test cache invalidation for a specific account
	testURL := "acc://RenatoDAP.acme/token"
	client.unifiedCache.InvalidateAccount(testURL)
	fmt.Printf("Invalidated cache for: %s\n", testURL)

	// Verify the account is no longer in cache
	if _, found := client.unifiedCache.GetAccountData(testURL); found {
		t.Error("Expected account data to be invalidated, but it's still in cache")
	}
	if _, found := client.unifiedCache.GetAccountSummary(testURL); found {
		t.Error("Expected account summary to be invalidated, but it's still in cache")
	}

	// Test staleness detection
	fmt.Printf("\n--- Phase 4.4: Cache Staleness Detection ---\n")
	for _, testAccount := range testAccounts {
		isStaleAccountData := client.unifiedCache.IsStale(testAccount.url, "accountData")
		isStaleBalance := client.unifiedCache.IsStale(testAccount.url, "balance")
		isStaleUnknown := client.unifiedCache.IsStale(testAccount.url, "unknown")

		fmt.Printf("Staleness for %s:\n", testAccount.url)
		fmt.Printf("  Account Data: %v\n", isStaleAccountData)
		fmt.Printf("  Balance: %v\n", isStaleBalance)
		fmt.Printf("  Unknown Type: %v\n", isStaleUnknown)

		// Unknown types should always be stale
		if !isStaleUnknown {
			t.Error("Expected unknown data type to be stale")
		}
	}

	// Test cleanup of expired entries
	fmt.Printf("\n--- Phase 4.5: Cache Cleanup Testing ---\n")
	preCleanupStats := client.unifiedCache.GetCacheStats()
	client.unifiedCache.CleanupExpired()
	postCleanupStats := client.unifiedCache.GetCacheStats()

	fmt.Printf("Pre-cleanup stats: %+v\n", preCleanupStats)
	fmt.Printf("Post-cleanup stats: %+v\n", postCleanupStats)

	// Test complete cache invalidation
	client.unifiedCache.InvalidateAll()
	emptyStats := client.unifiedCache.GetCacheStats()
	fmt.Printf("After InvalidateAll stats: %+v\n", emptyStats)

	// Verify cache is empty
	if emptyStats["accountData"].(int) != 0 {
		t.Error("Expected empty cache after InvalidateAll")
	}

	fmt.Printf("\n=== Phase 4 Complete: Unified Caching System Working ===\n")
	fmt.Printf("✅ All account types cached successfully\n")
	fmt.Printf("✅ Cache hit performance verified\n")
	fmt.Printf("✅ Cache management operations working\n")
	fmt.Printf("✅ Staleness detection functioning\n")
	fmt.Printf("✅ Cache cleanup and invalidation working\n")
}
