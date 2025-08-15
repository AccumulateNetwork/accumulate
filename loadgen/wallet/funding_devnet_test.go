package wallet

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

const (
	// Default devnet endpoints
	devnetAPI = "http://localhost:27004/v3"
	devnetTimeout = 30 * time.Second
)

// TestAcmeCollector_RealDevnet_CollectAcme tests ACME collection against real devnet
func TestAcmeCollector_RealDevnet_CollectAcme(t *testing.T) {
	// Skip if not running against devnet
	if testing.Short() {
		t.Skip("Skipping devnet test in short mode")
	}

	// Create real client
	client := jsonrpc.NewClient(devnetAPI)
	client.Client.Timeout = devnetTimeout

	// Create a test lite account
	privKey, pubKey, _ := ed25519.GenerateKey(nil)
	keyHash := sha256.Sum256(pubKey)
	liteURL := protocol.LiteAuthorityForKey(pubKey, protocol.SignatureTypeED25519).JoinPath("/ACME")

	t.Logf("Test account: %s", liteURL)
	t.Logf("Public key: %x", pubKey)

	// Create collector with real client
	collector := NewAcmeCollector(client, 0, 60*time.Second)

	// Test 1: First collection should succeed
	ctx := context.Background()
	err := collector.CollectAcme(ctx, liteURL)
	
	// Note: This might fail if faucet is empty or devnet is not running
	// In production, we'd check devnet status first
	if err != nil {
		t.Logf("Faucet request failed (expected if devnet not running): %v", err)
		t.Skip("Devnet not available or faucet empty")
	}

	// Check metrics
	successful, failed := collector.GetMetrics()
	assert.Equal(t, uint64(1), successful)
	assert.Equal(t, uint64(0), failed)

	// Test 2: Second immediate request should be skipped (cooldown)
	err = collector.CollectAcme(ctx, liteURL)
	assert.NoError(t, err) // No error, but request skipped

	// Metrics should not change
	successful2, failed2 := collector.GetMetrics()
	assert.Equal(t, successful, successful2)
	assert.Equal(t, failed, failed2)

	// Test 3: Wait a bit for the transaction to process
	time.Sleep(2 * time.Second)
	
	// Check the account actually received ACME
	query := &api.DefaultQuery{}
	resp, err := client.Query(ctx, liteURL, query)
	
	// Account might not exist yet if this is first time
	if err != nil {
		t.Logf("Account query error (may be creating): %v", err)
	} else if accRecord, ok := resp.(*api.AccountRecord); ok {
		if tokenAccount, ok := accRecord.Account.(*protocol.LiteTokenAccount); ok {
			t.Logf("Account balance: %v ACME", tokenAccount.Balance)
			assert.Greater(t, tokenAccount.Balance.Uint64(), uint64(0), "Should have received ACME")
		}
	}

	// Store test account for cleanup or future tests
	testAccount := &LiteIdentity{
		URL:           liteURL.RootIdentity(),
		Key:           &Key{
			Type:          protocol.SignatureTypeED25519,
			PublicKey:     pubKey,
			PrivateKey:    privKey,
			PublicKeyHash: keyHash[:],
		},
		PublicKeyHash: keyHash[:20],
		Created:       true,
		LastUpdated:   time.Now(),
	}
	_ = testAccount
}

// TestAcmeCollector_RealDevnet_Cooldown tests cooldown enforcement with real devnet
func TestAcmeCollector_RealDevnet_Cooldown(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping devnet test in short mode")
	}

	client := jsonrpc.NewClient(devnetAPI)
	client.Client.Timeout = devnetTimeout

	// Create test account
	_, pubKey, _ := ed25519.GenerateKey(nil)
	liteURL := protocol.LiteAuthorityForKey(pubKey, protocol.SignatureTypeED25519).JoinPath("/ACME")

	// Create collector with short cooldown for testing
	collector := NewAcmeCollector(client, 0, 5*time.Second)

	ctx := context.Background()

	// First request
	startTime := time.Now()
	err := collector.CollectAcme(ctx, liteURL)
	if err != nil {
		t.Skip("Devnet not available")
	}

	// Should not be able to collect immediately
	assert.False(t, collector.CanCollect())

	// Try again immediately - should skip
	err = collector.CollectAcme(ctx, liteURL)
	assert.NoError(t, err)

	// Wait for cooldown
	time.Sleep(5 * time.Second)

	// Should be able to collect now
	assert.True(t, collector.CanCollect())

	// Second request after cooldown
	err = collector.CollectAcme(ctx, liteURL)
	elapsed := time.Since(startTime)
	
	// Should have taken at least 5 seconds
	assert.GreaterOrEqual(t, elapsed, 5*time.Second)

	// Check metrics
	successful, _ := collector.GetMetrics()
	assert.Equal(t, uint64(2), successful, "Should have 2 successful requests")
}

// TestAcmeCollector_RealDevnet_MultipleAccounts tests collecting for multiple accounts
func TestAcmeCollector_RealDevnet_MultipleAccounts(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping devnet test in short mode")
	}

	client := jsonrpc.NewClient(devnetAPI)
	client.Client.Timeout = devnetTimeout

	// Create multiple test accounts
	accounts := make([]*url.URL, 3)
	for i := 0; i < 3; i++ {
		_, pubKey, _ := ed25519.GenerateKey(nil)
		accounts[i] = protocol.LiteAuthorityForKey(pubKey, protocol.SignatureTypeED25519).JoinPath("/ACME")
		t.Logf("Account %d: %s", i, accounts[i])
	}

	// Create collector
	collector := NewAcmeCollector(client, 0, 1*time.Second) // Short cooldown for test

	ctx := context.Background()

	// Request ACME for each account
	for i, account := range accounts {
		err := collector.CollectAcme(ctx, account)
		if err != nil && i == 0 {
			t.Skip("Devnet not available")
		}
		
		// Wait for cooldown between requests
		if i < len(accounts)-1 {
			time.Sleep(1100 * time.Millisecond)
		}
	}

	// Check metrics
	successful, failed := collector.GetMetrics()
	t.Logf("Successful: %d, Failed: %d", successful, failed)
	assert.GreaterOrEqual(t, successful, uint64(1), "Should have at least 1 successful request")
}

// TestFundingManager_RealDevnet_Integration tests the full funding manager against devnet
func TestFundingManager_RealDevnet_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping devnet test in short mode")
	}

	// Create wallet
	wallet := NewWallet()

	// Create funding account
	privKey, pubKey, _ := ed25519.GenerateKey(nil)
	keyHash := sha256.Sum256(pubKey)
	fundingAccount := &LiteIdentity{
		URL: protocol.LiteAuthorityForKey(pubKey, protocol.SignatureTypeED25519),
		Key: &Key{
			Type:          protocol.SignatureTypeED25519,
			PublicKey:     pubKey,
			PrivateKey:    privKey,
			PublicKeyHash: keyHash[:],
		},
		PublicKeyHash: keyHash[:20],
	}
	wallet.SetFundingAccount(fundingAccount)

	// Create some test lite accounts
	for i := 0; i < 3; i++ {
		_, pubKey, _ := ed25519.GenerateKey(nil)
		keyHash := sha256.Sum256(pubKey)
		lite := &LiteIdentity{
			URL: protocol.LiteAuthorityForKey(pubKey, protocol.SignatureTypeED25519),
			Key: &Key{
				Type:          protocol.SignatureTypeED25519,
				PublicKey:     pubKey,
				PublicKeyHash: keyHash[:],
			},
			PublicKeyHash: keyHash[:20],
		}
		wallet.StoreLiteIdentity(lite)
	}

	// Start funding manager
	config := &FundingConfig{
		ServerURL:        devnetAPI,
		TargetCredits:    1000,
		MaxFaucetRequest: 0, // Use default
		FaucetCooldown:   5 * time.Second,
		CheckInterval:    2 * time.Second,
	}
	
	wallet.StartFunding(config)
	defer wallet.StopFunding()

	// Let it run for a bit
	time.Sleep(10 * time.Second)

	// Check metrics
	metrics := wallet.GetFundingMetrics()
	if metrics != nil {
		t.Logf("Funding metrics - Successful: %d, Failed: %d",
			metrics.SuccessfulRequests, metrics.FailedRequests)
			// metrics.AccountsTopped, metrics.CreditsDistributed)
		
		// Should have at least tried to get ACME
		assert.GreaterOrEqual(t, metrics.SuccessfulRequests+metrics.FailedRequests, uint64(1),
			"Should have attempted at least one faucet request")
	}
}

// TestAcmeCollector_RealDevnet_Balance verifies actual balance changes
func TestAcmeCollector_RealDevnet_Balance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping devnet test in short mode")
	}

	client := jsonrpc.NewClient(devnetAPI)
	client.Client.Timeout = devnetTimeout
	ctx := context.Background()

	// Create test account
	_, pubKey, _ := ed25519.GenerateKey(nil)
	liteURL := protocol.LiteAuthorityForKey(pubKey, protocol.SignatureTypeED25519).JoinPath("/ACME")

	// Request ACME from faucet first
	collector := NewAcmeCollector(client, 0, 60*time.Second)
	err := collector.CollectAcme(ctx, liteURL)
	if err != nil {
		t.Skip("Devnet not available")
	}

	// Wait longer for transaction to fully settle and account to be created
	t.Log("Waiting for transaction to settle...")
	time.Sleep(20 * time.Second)

	// Now check the balance - account should exist after faucet
	t.Logf("Querying account: %s", liteURL)
	querier := api.Querier2{Querier: client}
	accRecord, err := querier.QueryAccount(ctx, liteURL, nil)
	if err != nil {
		// Try one more time after additional wait
		t.Logf("First query failed: %v, waiting and retrying...", err)
		time.Sleep(10 * time.Second)
		accRecord, err = querier.QueryAccount(ctx, liteURL, nil)
		if err != nil {
			t.Fatalf("Failed to query account after faucet (2 attempts): %v", err)
		}
	}

	var balance uint64
	if tokenAccount, ok := accRecord.Account.(*protocol.LiteTokenAccount); ok {
		balance = tokenAccount.Balance.Uint64()
		t.Logf("Account balance after faucet: %d", balance)
	} else {
		t.Fatalf("Account is not a LiteTokenAccount: %T", accRecord.Account)
	}

	// Balance should be greater than 0 after faucet
	assert.Greater(t, balance, uint64(0), "Balance should be greater than 0 after faucet")
}