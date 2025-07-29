// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package liteclient

import (
	"sync"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestNewUnifiedCache tests cache creation with different TTL values
func TestNewUnifiedCache(t *testing.T) {
	t.Run("DefaultTTL", func(t *testing.T) {
		cache := NewUnifiedCache(0) // Should use default 5 minute TTL
		if cache.defaultTTL != 5*time.Minute {
			t.Errorf("Expected default TTL of 5 minutes, got %v", cache.defaultTTL)
		}
	})

	t.Run("CustomTTL", func(t *testing.T) {
		customTTL := 10 * time.Minute
		cache := NewUnifiedCache(customTTL)
		if cache.defaultTTL != customTTL {
			t.Errorf("Expected TTL of %v, got %v", customTTL, cache.defaultTTL)
		}
	})

	t.Run("InitializedMaps", func(t *testing.T) {
		cache := NewUnifiedCache(time.Minute)
		if cache.accountData == nil {
			t.Error("accountData map not initialized")
		}
		if cache.transactions == nil {
			t.Error("transactions map not initialized")
		}
		if cache.balances == nil {
			t.Error("balances map not initialized")
		}
		if cache.identityInfo == nil {
			t.Error("identityInfo map not initialized")
		}
		if cache.dataAccountInfo == nil {
			t.Error("dataAccountInfo map not initialized")
		}
		if cache.accountSummaries == nil {
			t.Error("accountSummaries map not initialized")
		}
	})
}

// TestAccountDataCaching tests account data storage and retrieval
func TestAccountDataCaching(t *testing.T) {
	cache := NewUnifiedCache(time.Minute)
	testURL := "acc://test.acme/token"

	// Create test account data
	testData := &AccountData{
		URL:      testURL,
		Type:     protocol.AccountTypeTokenAccount,
		TypeName: "TokenAccount",
		Data:     map[string]interface{}{"balance": "100.00000000"},
	}

	t.Run("StoreAndRetrieve", func(t *testing.T) {
		// Store data
		cache.StoreAccountData(testURL, testData)

		// Retrieve data
		retrieved, found := cache.GetAccountData(testURL)
		if !found {
			t.Fatal("Account data not found after storing")
		}

		if retrieved.URL != testData.URL {
			t.Errorf("Expected URL %s, got %s", testData.URL, retrieved.URL)
		}
		if retrieved.Type != testData.Type {
			t.Errorf("Expected Type %d, got %d", testData.Type, retrieved.Type)
		}
	})

	t.Run("CustomTTL", func(t *testing.T) {
		shortTTL := 100 * time.Millisecond
		cache.StoreAccountData(testURL+"2", testData, shortTTL)

		// Should be found immediately
		_, found := cache.GetAccountData(testURL + "2")
		if !found {
			t.Error("Data should be found immediately after storing")
		}

		// Wait for expiration
		time.Sleep(150 * time.Millisecond)

		// Should be expired now
		_, found = cache.GetAccountData(testURL + "2")
		if found {
			t.Error("Data should be expired after TTL")
		}
	})

	t.Run("NonExistentAccount", func(t *testing.T) {
		_, found := cache.GetAccountData("acc://nonexistent.acme")
		if found {
			t.Error("Should not find non-existent account")
		}
	})
}

// TestBalanceCaching tests balance storage and retrieval
func TestBalanceCaching(t *testing.T) {
	cache := NewUnifiedCache(time.Minute)
	testURL := "acc://test.acme/token"

	testBalance := &TokenBalanceInfo{
		AccountURL: testURL,
		Balance:    "100.00000000",
		TokenURL:   "acc://ACME.acme",
	}

	t.Run("StoreAndRetrieve", func(t *testing.T) {
		cache.StoreBalance(testURL, testBalance)

		retrieved, found := cache.GetBalance(testURL)
		if !found {
			t.Fatal("Balance not found after storing")
		}

		if retrieved.Balance != testBalance.Balance {
			t.Errorf("Expected balance %s, got %s", testBalance.Balance, retrieved.Balance)
		}
		if retrieved.TokenURL != testBalance.TokenURL {
			t.Errorf("Expected token URL %s, got %s", testBalance.TokenURL, retrieved.TokenURL)
		}
	})

	t.Run("TTLExpiration", func(t *testing.T) {
		shortTTL := 50 * time.Millisecond
		cache.StoreBalance(testURL+"_ttl", testBalance, shortTTL)

		// Wait for expiration
		time.Sleep(100 * time.Millisecond)

		_, found := cache.GetBalance(testURL + "_ttl")
		if found {
			t.Error("Balance should be expired")
		}
	})
}

// TestTransactionCaching tests transaction storage and retrieval
func TestTransactionCaching(t *testing.T) {
	cache := NewUnifiedCache(time.Minute)
	testURL := "acc://test.acme/token"

	testTxs := []*CachedTransaction{
		{
			TxID:      "tx1",
			Type:      "tokenSend",
			Status:    "delivered",
			Amount:    "10.00000000",
			From:      testURL,
			To:        "acc://recipient.acme/token",
			Account:   testURL,
			Height:    12345,
			CachedAt:  time.Now(),
			ExpiresAt: time.Now().Add(time.Minute),
		},
		{
			TxID:      "tx2",
			Type:      "tokenSend",
			Status:    "delivered",
			Amount:    "5.00000000",
			From:      testURL,
			To:        "acc://recipient2.acme/token",
			Account:   testURL,
			Height:    12346,
			CachedAt:  time.Now(),
			ExpiresAt: time.Now().Add(time.Minute),
		},
	}

	t.Run("StoreAndRetrieve", func(t *testing.T) {
		cache.StoreTransactions(testURL, testTxs)

		retrieved, found := cache.GetTransactions(testURL)
		if !found {
			t.Fatal("Transactions not found after storing")
		}

		if len(retrieved) != len(testTxs) {
			t.Errorf("Expected %d transactions, got %d", len(testTxs), len(retrieved))
		}

		for i, tx := range retrieved {
			if tx.TxID != testTxs[i].TxID {
				t.Errorf("Transaction %d: expected TxID %s, got %s", i, testTxs[i].TxID, tx.TxID)
			}
		}
	})

	t.Run("AddSingleTransaction", func(t *testing.T) {
		newTx := &CachedTransaction{
			TxID:      "tx3",
			Type:      "tokenSend",
			Status:    "delivered",
			Amount:    "15.00000000",
			From:      testURL,
			To:        "acc://recipient3.acme/token",
			Account:   testURL,
			Height:    12347,
			CachedAt:  time.Now(),
			ExpiresAt: time.Now().Add(time.Minute),
		}

		cache.AddTransaction(testURL, newTx)

		retrieved, found := cache.GetTransactions(testURL)
		if !found {
			t.Fatal("Transactions not found after adding")
		}

		// Should now have 3 transactions (2 original + 1 added)
		if len(retrieved) != 3 {
			t.Errorf("Expected 3 transactions after adding, got %d", len(retrieved))
		}

		// First transaction should be the newly added one (most recent first)
		firstTx := retrieved[0]
		if firstTx.TxID != newTx.TxID {
			t.Errorf("Expected first transaction TxID %s, got %s", newTx.TxID, firstTx.TxID)
		}
	})
}

// TestIdentityInfoCaching tests identity info storage and retrieval
func TestIdentityInfoCaching(t *testing.T) {
	cache := NewUnifiedCache(time.Minute)
	testURL := "acc://test.acme"

	testIdentity := &IdentityInfo{
		AccountURL:  testURL,
		IdentityURL: testURL,
		KeyBook:     "acc://test.acme/book",
	}

	t.Run("StoreAndRetrieve", func(t *testing.T) {
		cache.StoreIdentityInfo(testURL, testIdentity)

		retrieved, found := cache.GetIdentityInfo(testURL)
		if !found {
			t.Fatal("Identity info not found after storing")
		}

		if retrieved.AccountURL != testIdentity.AccountURL {
			t.Errorf("Expected URL %s, got %s", testIdentity.AccountURL, retrieved.AccountURL)
		}
		if retrieved.KeyBook != testIdentity.KeyBook {
			t.Errorf("Expected KeyBook %s, got %s", testIdentity.KeyBook, retrieved.KeyBook)
		}
	})
}

// TestDataAccountInfoCaching tests data account info storage and retrieval
func TestDataAccountInfoCaching(t *testing.T) {
	cache := NewUnifiedCache(time.Minute)
	testURL := "acc://test.acme/data"

	testDataAccount := &DataAccountInfo{
		AccountURL:  testURL,
		AccountType: "DataAccount",
		DataURL:     testURL,
		KeyBook:     "acc://test.acme/book",
	}

	t.Run("StoreAndRetrieve", func(t *testing.T) {
		cache.StoreDataAccountInfo(testURL, testDataAccount)

		retrieved, found := cache.GetDataAccountInfo(testURL)
		if !found {
			t.Fatal("Data account info not found after storing")
		}

		if retrieved.AccountURL != testDataAccount.AccountURL {
			t.Errorf("Expected URL %s, got %s", testDataAccount.AccountURL, retrieved.AccountURL)
		}
		if retrieved.KeyBook != testDataAccount.KeyBook {
			t.Errorf("Expected KeyBook %s, got %s", testDataAccount.KeyBook, retrieved.KeyBook)
		}
	})
}

// TestAccountSummaryCaching tests account summary storage and retrieval
func TestAccountSummaryCaching(t *testing.T) {
	cache := NewUnifiedCache(time.Minute)
	testURL := "acc://test.acme/token"

	testSummary := &AccountSummary{
		AccountURL:  testURL,
		AccountType: "TokenAccount",
		Category:    "token",
		Balance:     "100.00000000",
		TokenURL:    "acc://ACME.acme",
		KeyBook:     "acc://test.acme/book",
	}

	t.Run("StoreAndRetrieve", func(t *testing.T) {
		cache.StoreAccountSummary(testURL, testSummary)

		retrieved, found := cache.GetAccountSummary(testURL)
		if !found {
			t.Fatal("Account summary not found after storing")
		}

		if retrieved.AccountURL != testSummary.AccountURL {
			t.Errorf("Expected URL %s, got %s", testSummary.AccountURL, retrieved.AccountURL)
		}
		if retrieved.Category != testSummary.Category {
			t.Errorf("Expected Category %s, got %s", testSummary.Category, retrieved.Category)
		}
		if retrieved.Balance != testSummary.Balance {
			t.Errorf("Expected Balance %s, got %s", testSummary.Balance, retrieved.Balance)
		}
	})
}

// TestCacheInvalidation tests cache invalidation functionality
func TestCacheInvalidation(t *testing.T) {
	cache := NewUnifiedCache(time.Minute)
	testURL := "acc://test.acme/token"

	// Store test data in all cache types
	testData := &AccountData{URL: testURL, Type: protocol.AccountTypeTokenAccount, TypeName: "TokenAccount"}
	testBalance := &TokenBalanceInfo{AccountURL: testURL, Balance: "100.00000000", TokenURL: "acc://ACME.acme"}
	testTxs := []*CachedTransaction{{TxID: "tx1", Account: testURL, CachedAt: time.Now(), ExpiresAt: time.Now().Add(time.Minute)}}
	testIdentity := &IdentityInfo{AccountURL: testURL, IdentityURL: testURL, KeyBook: "acc://test.acme/book"}
	testDataAccount := &DataAccountInfo{AccountURL: testURL, AccountType: "DataAccount", DataURL: testURL, KeyBook: "acc://test.acme/book"}
	testSummary := &AccountSummary{AccountURL: testURL, AccountType: "TokenAccount", Category: "token"}

	cache.StoreAccountData(testURL, testData)
	cache.StoreBalance(testURL, testBalance)
	cache.StoreTransactions(testURL, testTxs)
	cache.StoreIdentityInfo(testURL, testIdentity)
	cache.StoreDataAccountInfo(testURL, testDataAccount)
	cache.StoreAccountSummary(testURL, testSummary)

	t.Run("InvalidateAccount", func(t *testing.T) {
		// Verify data exists
		_, found := cache.GetAccountData(testURL)
		if !found {
			t.Fatal("Test data should exist before invalidation")
		}

		// Invalidate account
		cache.InvalidateAccount(testURL)

		// Verify all data for this account is gone
		_, found = cache.GetAccountData(testURL)
		if found {
			t.Error("Account data should be invalidated")
		}
		_, found = cache.GetBalance(testURL)
		if found {
			t.Error("Balance should be invalidated")
		}
		_, found = cache.GetTransactions(testURL)
		if found {
			t.Error("Transactions should be invalidated")
		}
		_, found = cache.GetIdentityInfo(testURL)
		if found {
			t.Error("Identity info should be invalidated")
		}
		_, found = cache.GetDataAccountInfo(testURL)
		if found {
			t.Error("Data account info should be invalidated")
		}
		_, found = cache.GetAccountSummary(testURL)
		if found {
			t.Error("Account summary should be invalidated")
		}
	})

	t.Run("InvalidateAll", func(t *testing.T) {
		// Re-store data
		cache.StoreAccountData(testURL, testData)
		cache.StoreBalance(testURL, testBalance)

		// Verify data exists
		_, found := cache.GetAccountData(testURL)
		if !found {
			t.Fatal("Test data should exist before invalidation")
		}

		// Invalidate all
		cache.InvalidateAll()

		// Verify all data is gone
		_, found = cache.GetAccountData(testURL)
		if found {
			t.Error("All data should be invalidated")
		}
		_, found = cache.GetBalance(testURL)
		if found {
			t.Error("All data should be invalidated")
		}
	})
}

// TestCacheCleanup tests cache cleanup functionality
func TestCacheCleanup(t *testing.T) {
	cache := NewUnifiedCache(time.Minute)
	testURL := "acc://test.acme/token"

	t.Run("CleanupExpired", func(t *testing.T) {
		// Store data with short TTL
		shortTTL := 50 * time.Millisecond
		testData := &AccountData{URL: testURL, Type: protocol.AccountTypeTokenAccount, TypeName: "TokenAccount"}
		cache.StoreAccountData(testURL, testData, shortTTL)

		// Verify data exists
		_, found := cache.GetAccountData(testURL)
		if !found {
			t.Fatal("Data should exist immediately after storing")
		}

		// Wait for expiration
		time.Sleep(100 * time.Millisecond)

		// Run cleanup
		cache.CleanupExpired()

		// Verify expired data is removed
		_, found = cache.GetAccountData(testURL)
		if found {
			t.Error("Expired data should be cleaned up")
		}
	})

	t.Run("PruneOlderThan", func(t *testing.T) {
		// Store data
		testData := &AccountData{URL: testURL, Type: protocol.AccountTypeTokenAccount, TypeName: "TokenAccount"}
		cache.StoreAccountData(testURL, testData)

		// Verify data exists
		_, found := cache.GetAccountData(testURL)
		if !found {
			t.Fatal("Data should exist after storing")
		}

		// Prune data older than now (should remove everything)
		cache.PruneOlderThan(time.Now().Add(time.Second))

		// Verify data is removed
		_, found = cache.GetAccountData(testURL)
		if found {
			t.Error("Data should be pruned")
		}
	})
}

// TestCacheStats tests cache statistics functionality
func TestCacheStats(t *testing.T) {
	cache := NewUnifiedCache(time.Minute)

	// Store various types of data
	testData := &AccountData{URL: "acc://test.acme/token", Type: protocol.AccountTypeTokenAccount, TypeName: "TokenAccount"}
	testBalance := &TokenBalanceInfo{AccountURL: "acc://test.acme/token", Balance: "100.00000000", TokenURL: "acc://ACME.acme"}
	testTxs := []*CachedTransaction{{TxID: "tx1", Account: "acc://test.acme/token", CachedAt: time.Now(), ExpiresAt: time.Now().Add(time.Minute)}}
	testSummary := &AccountSummary{AccountURL: "acc://test.acme/token", AccountType: "TokenAccount", Category: "token"}

	cache.StoreAccountData("acc://test.acme/token", testData)
	cache.StoreBalance("acc://test.acme/token", testBalance)
	cache.StoreTransactions("acc://test.acme/token", testTxs)
	cache.StoreAccountSummary("acc://test.acme/token", testSummary)

	t.Run("GetStats", func(t *testing.T) {
		stats := cache.GetStats()
		if stats == nil {
			t.Fatal("Stats should not be nil")
		}

		if stats.AccountDataEntries != 1 {
			t.Errorf("Expected 1 account data entry, got %d", stats.AccountDataEntries)
		}
		if stats.BalanceEntries != 1 {
			t.Errorf("Expected 1 balance entry, got %d", stats.BalanceEntries)
		}
		if stats.TransactionEntries != 1 {
			t.Errorf("Expected 1 transaction entry, got %d", stats.TransactionEntries)
		}
		if stats.TotalEntries != 4 {
			t.Errorf("Expected 4 total entries, got %d", stats.TotalEntries)
		}
	})

	t.Run("GetCacheStats", func(t *testing.T) {
		statsMap := cache.GetCacheStats()
		if statsMap == nil {
			t.Fatal("Stats map should not be nil")
		}

		if accountData, ok := statsMap["accountData"].(int); !ok || accountData != 1 {
			t.Errorf("Expected 1 account data entry in stats map, got %v", statsMap["accountData"])
		}
	})
}

// TestIsStale tests staleness detection
func TestIsStale(t *testing.T) {
	cache := NewUnifiedCache(time.Minute)
	testURL := "acc://test.acme/token"

	t.Run("FreshData", func(t *testing.T) {
		testData := &AccountData{URL: testURL, Type: 4, TypeName: "TokenAccount"}
		cache.StoreAccountData(testURL, testData)

		if cache.IsStale(testURL, "accountData") {
			t.Error("Fresh data should not be stale")
		}
	})

	t.Run("StaleData", func(t *testing.T) {
		shortTTL := 50 * time.Millisecond
		testData := &AccountData{URL: testURL + "_stale", Type: protocol.AccountTypeTokenAccount, TypeName: "TokenAccount"}
		cache.StoreAccountData(testURL+"_stale", testData, shortTTL)

		// Wait for data to become stale
		time.Sleep(100 * time.Millisecond)

		if !cache.IsStale(testURL+"_stale", "accountData") {
			t.Error("Expired data should be stale")
		}
	})

	t.Run("NonExistentData", func(t *testing.T) {
		if !cache.IsStale("acc://nonexistent.acme", "accountData") {
			t.Error("Non-existent data should be considered stale")
		}
	})
}

// TestGetCachedADIs tests ADI listing functionality
func TestGetCachedADIs(t *testing.T) {
	cache := NewUnifiedCache(time.Minute)

	// Store data for multiple ADIs
	testData1 := &AccountData{URL: "acc://adi1.acme/token", Type: protocol.AccountTypeTokenAccount, TypeName: "TokenAccount"}
	testData2 := &AccountData{URL: "acc://adi2.acme/token", Type: protocol.AccountTypeTokenAccount, TypeName: "TokenAccount"}
	testBalance := &TokenBalanceInfo{AccountURL: "acc://adi1.acme/staking", Balance: "100.00000000", TokenURL: "acc://ACME.acme"}

	cache.StoreAccountData("acc://adi1.acme/token", testData1)
	cache.StoreAccountData("acc://adi2.acme/token", testData2)
	cache.StoreBalance("acc://adi1.acme/staking", testBalance)

	t.Run("ListCachedADIs", func(t *testing.T) {
		adis := cache.GetCachedADIs()

		if len(adis) < 2 {
			t.Errorf("Expected at least 2 ADIs, got %d", len(adis))
		}

		// Check that both ADIs are present
		adiSet := make(map[string]bool)
		for _, adi := range adis {
			adiSet[adi] = true
		}

		if !adiSet["acc://adi1.acme"] {
			t.Error("Expected to find acc://adi1.acme in cached ADIs")
		}
		if !adiSet["acc://adi2.acme"] {
			t.Error("Expected to find acc://adi2.acme in cached ADIs")
		}
	})

	t.Run("GetADIAccounts", func(t *testing.T) {
		accounts := cache.GetADIAccounts("acc://adi1.acme")

		if len(accounts) != 1 {
			t.Errorf("Expected 1 account for adi1.acme, got %d", len(accounts))
		}

		if accounts[0].URL != "acc://adi1.acme/token" {
			t.Errorf("Expected account URL acc://adi1.acme/token, got %s", accounts[0].URL)
		}
	})
}

// TestConcurrentAccess tests thread safety of cache operations
func TestConcurrentAccess(t *testing.T) {
	cache := NewUnifiedCache(time.Minute)
	testURL := "acc://test.acme/token"
	testData := &AccountData{URL: testURL, Type: 4, TypeName: "TokenAccount"}

	t.Run("ConcurrentReadWrite", func(t *testing.T) {
		var wg sync.WaitGroup
		numGoroutines := 10
		numOperations := 100

		// Concurrent writes
		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < numOperations; j++ {
					url := testURL + "_" + string(rune(id)) + "_" + string(rune(j))
					cache.StoreAccountData(url, testData)
				}
			}(i)
		}

		// Concurrent reads
		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < numOperations; j++ {
					url := testURL + "_" + string(rune(id)) + "_" + string(rune(j))
					cache.GetAccountData(url)
				}
			}(i)
		}

		wg.Wait()
		// If we reach here without deadlock or race conditions, the test passes
	})

	t.Run("ConcurrentInvalidation", func(t *testing.T) {
		var wg sync.WaitGroup
		numGoroutines := 5

		// Store initial data
		cache.StoreAccountData(testURL, testData)

		// Concurrent invalidations
		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer wg.Done()
				cache.InvalidateAccount(testURL)
			}()
		}

		wg.Wait()
		// Should not cause any race conditions or panics
	})
}

// TestExtractADIFromURL tests the ADI URL extraction helper function
func TestExtractADIFromURL(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"acc://test.acme/token", "acc://test.acme"},
		{"acc://test.acme/book/1", "acc://test.acme"},
		{"acc://test.acme", "acc://test.acme"},
		{"acc://complex-name.acme/data/entry", "acc://complex-name.acme"},
		{"", ""},
		{"invalid", ""},
		{"http://test.com", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := extractADIFromURL(tc.input)
			if result != tc.expected {
				t.Errorf("extractADIFromURL(%q) = %q, expected %q", tc.input, result, tc.expected)
			}
		})
	}
}
