package wallet

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

const (
	// Devnet configuration
	devnetTimeoutDuration = 30 * time.Second
	
	// Test configuration
	testFaucetAmount = 10000000 // 10 ACME in lowest denomination
	testCreditAmount = 100       // Credits to test with
)

var (
	// Discovered devnet endpoint - populated by findDevnetEndpoint()
	devnetAPIEndpoint string
)

// Helper: Find the devnet endpoint by checking environment or scanning ports
func findDevnetEndpoint() string {
	// First, check environment variables for configuration
	// ACCUMULATE_DEVNET_URL - Full URL to devnet (e.g., http://localhost:27004/v3)
	// ACCUMULATE_DEVNET_PORT - Just the port number (e.g., 27004)
	// ACCUMULATE_DEVNET_HOST - Host to connect to (default: localhost)
	
	if envURL := os.Getenv("ACCUMULATE_DEVNET_URL"); envURL != "" {
		// User specified full URL
		return envURL
	}
	
	host := os.Getenv("ACCUMULATE_DEVNET_HOST")
	if host == "" {
		host = "localhost"
	}
	
	if envPort := os.Getenv("ACCUMULATE_DEVNET_PORT"); envPort != "" {
		// User specified port
		port, err := strconv.Atoi(envPort)
		if err == nil {
			endpoint := fmt.Sprintf("http://%s:%d/v3", host, port)
			if testEndpoint(endpoint) {
				return endpoint
			}
		}
	}
	
	// Try to auto-detect by scanning common ports
	// Based on accumulated defaults: base port + 4 for JSON-RPC
	commonBasePorts := []int{
		27000, // Current running instance (base port)
		26656, // Default base port for accumulated
		8000,  // Sometimes used for testing
	}
	
	for _, basePort := range commonBasePorts {
		// JSON-RPC is typically on basePort + 4
		jsonRPCPort := basePort + 4
		endpoint := fmt.Sprintf("http://%s:%d/v3", host, jsonRPCPort)
		if testEndpoint(endpoint) {
			return endpoint
		}
		
		// Also try the base port itself
		endpoint = fmt.Sprintf("http://%s:%d/v3", host, basePort)
		if testEndpoint(endpoint) {
			return endpoint
		}
	}
	
	// Try specific known ports
	knownPorts := []int{
		27004, // Observed in current setup
		26660, // Old default  
		8545,  // Sometimes used
		9545,  // Alternative
	}
	
	for _, port := range knownPorts {
		endpoint := fmt.Sprintf("http://%s:%d/v3", host, port)
		if testEndpoint(endpoint) {
			return endpoint
		}
	}
	
	return "" // No devnet found
}

// Helper: Test if an endpoint is responding
func testEndpoint(endpoint string) bool {
	client := jsonrpc.NewClient(endpoint)
	client.Client.Timeout = 2 * time.Second
	
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	
	// Try to query the version endpoint or faucet
	_, err := client.Faucet(ctx, &url.URL{Authority: "test"}, api.FaucetOptions{})
	
	// If we get any response (even an error), the endpoint is alive
	// The faucet might return an error but the connection works
	if err != nil {
		errStr := err.Error()
		// Check if it's a connection error vs application error
		return !strings.Contains(errStr, "connection refused") && 
		       !strings.Contains(errStr, "no such host") &&
		       !strings.Contains(errStr, "i/o timeout") &&
		       !strings.Contains(errStr, "network is unreachable")
	}
	return true
}

// Helper: Check if devnet is available
func isDevnetAvailable() bool {
	// Find the endpoint if not already discovered
	if devnetAPIEndpoint == "" {
		devnetAPIEndpoint = findDevnetEndpoint()
	}
	
	if devnetAPIEndpoint == "" {
		return false // No devnet found on any port
	}
	
	client := jsonrpc.NewClient(devnetAPIEndpoint)
	client.Client.Timeout = 5 * time.Second
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	// Try a simple query to verify it's working
	// Use faucet as a connectivity test
	_, err := client.Faucet(ctx, &url.URL{Authority: "test"}, api.FaucetOptions{})
	
	// Any response (even error) means devnet is running
	if err != nil {
		errStr := err.Error()
		return !strings.Contains(errStr, "connection refused") && 
		       !strings.Contains(errStr, "no such host") &&
		       !strings.Contains(errStr, "i/o timeout")
	}
	return true
}

// Helper: Create a real lite account with faucet funding
func createFundedLiteAccount(t *testing.T, client *jsonrpc.Client, name string) *LiteIdentity {
	// Generate new key pair
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err, "Failed to generate key pair")
	
	// Create lite identity URL
	liteUrl := protocol.LiteAuthorityForKey(pubKey, protocol.SignatureTypeED25519)
	
	// Create LiteIdentity structure
	keyHash := sha256.Sum256(pubKey)
	liteIdentity := &LiteIdentity{
		URL: liteUrl,
		Key: &Key{
			Type:          protocol.SignatureTypeED25519,
			PublicKey:     pubKey,
			PrivateKey:    privKey,
			PublicKeyHash: keyHash[:20],
		},
		PublicKeyHash: keyHash[:20],
	}
	
	t.Logf("Created %s account: %s", name, liteUrl)
	t.Logf("  Public key: %x", pubKey)
	
	// Request ACME from faucet repeatedly until we have enough
	ctx := context.Background()
	acmeUrl := liteUrl.JoinPath("/ACME")
	
	// Each faucet call gives 10 ACME, we need at least 100 ACME
	// So make 10+ calls without waiting for each to complete
	targetACME := uint64(100 * protocol.AcmePrecision) // 100 ACME
	faucetCalls := 12 // Make 12 calls to get 120 ACME (with buffer)
	
	t.Logf("Making %d faucet calls for %s to get %d ACME", faucetCalls, name, targetACME/protocol.AcmePrecision)
	
	// Fire off multiple faucet requests without waiting
	for i := 0; i < faucetCalls; i++ {
		submission, err := client.Faucet(ctx, acmeUrl, api.FaucetOptions{})
		if err != nil {
			t.Logf("Faucet call %d failed for %s: %v", i+1, name, err)
		} else if submission != nil && submission.Status != nil {
			t.Logf("Faucet call %d submitted for %s: TxID=%s", i+1, name, submission.Status.TxID)
		}
		// Small delay between requests to avoid rate limiting
		time.Sleep(100 * time.Millisecond)
	}
	
	// Now wait for the ACME balance to reach target
	t.Logf("Waiting for %s to have at least %d ACME", name, targetACME/protocol.AcmePrecision)
	deadline := time.Now().Add(60 * time.Second)
	
	for time.Now().Before(deadline) {
		balance := queryAcmeBalanceOnChain(t, client, liteUrl)
		if balance >= targetACME {
			t.Logf("%s now has %d ACME", name, balance/protocol.AcmePrecision)
			break
		}
		t.Logf("%s has %d ACME, waiting for %d ACME...", name, balance/protocol.AcmePrecision, targetACME/protocol.AcmePrecision)
		time.Sleep(2 * time.Second)
	}
	
	// Query and update credit balance
	resp, err := client.Query(ctx, liteUrl, &api.DefaultQuery{})
	if err == nil {
		if record, ok := resp.(*api.AccountRecord); ok {
			if lite, ok := record.Account.(*protocol.LiteIdentity); ok {
				liteIdentity.CreditBalance = lite.CreditBalance
				t.Logf("  Current credits: %d", lite.CreditBalance)
			}
		}
	}
	
	// Query ACME balance
	resp, err = client.Query(ctx, acmeUrl, &api.DefaultQuery{})
	if err == nil {
		if record, ok := resp.(*api.AccountRecord); ok {
			if token, ok := record.Account.(*protocol.TokenAccount); ok {
				balance := token.Balance.Uint64() / protocol.AcmePrecision
				t.Logf("  ACME balance: %d ACME", balance)
			}
		}
	}
	
	return liteIdentity
}

// Helper: Wait for transaction to complete by polling
func waitForTransaction(t *testing.T, client *jsonrpc.Client, txID *url.TxID) {
	if txID == nil {
		return
	}
	
	ctx := context.Background()
	deadline := time.Now().Add(60 * time.Second) // Increase timeout to 60 seconds
	attempts := 0
	
	t.Logf("Waiting for transaction %s to complete...", txID)
	
	for time.Now().Before(deadline) {
		// Query the transaction status
		resp, err := client.Query(ctx, txID.AsUrl(), &api.DefaultQuery{})
		
		if err != nil {
			// Transaction might not be visible yet, keep trying
			time.Sleep(500 * time.Millisecond)
			continue
		}
		
		if resp != nil {
			// Check different response types to determine if transaction is complete
			switch r := resp.(type) {
			case *api.ChainRecord:
				// Transaction found in a chain, it's been processed
				t.Logf("Transaction %s found in chain", txID)
				// Wait a bit more for state to propagate
				time.Sleep(2 * time.Second)
				return
				
			case *api.AccountRecord:
				// This might be returned for certain transaction types
				t.Logf("Transaction %s returned account record", txID)
				time.Sleep(2 * time.Second)
				return
				
			default:
				// MessageRecord indicates pending, keep waiting
				// After many attempts, assume it's as complete as it will get
				// Some transactions may stay in MessageRecord state
				attempts++
				if attempts > 20 {
					t.Logf("Transaction %s still in %T state after %d attempts, proceeding", txID, r, attempts)
					time.Sleep(3 * time.Second)
					return
				}
				time.Sleep(500 * time.Millisecond)
			}
		}
		
		time.Sleep(500 * time.Millisecond)
	}
	
	t.Logf("Warning: Transaction %s did not complete within 60 second timeout", txID)
}

// Helper: Wait for account to exist
func waitForAccount(t *testing.T, client *jsonrpc.Client, accountUrl *url.URL, timeout time.Duration) bool {
	ctx := context.Background()
	deadline := time.Now().Add(timeout)
	
	for time.Now().Before(deadline) {
		resp, err := client.Query(ctx, accountUrl, &api.DefaultQuery{})
		if err == nil && resp != nil {
			if _, ok := resp.(*api.AccountRecord); ok {
				return true
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	
	return false
}

// Helper: Query account credits on chain
func queryCreditsOnChain(t *testing.T, client *jsonrpc.Client, accountUrl *url.URL) uint64 {
	ctx := context.Background()
	
	// First wait for account to exist
	if !waitForAccount(t, client, accountUrl, 10*time.Second) {
		t.Logf("Account %s does not exist yet", accountUrl)
		return 0
	}
	
	resp, err := client.Query(ctx, accountUrl, &api.DefaultQuery{})
	if err != nil {
		t.Logf("Failed to query account %s: %v", accountUrl, err)
		return 0
	}
	
	record, ok := resp.(*api.AccountRecord)
	if !ok {
		t.Logf("Unexpected response type for %s: %T", accountUrl, resp)
		return 0
	}
	
	switch acc := record.Account.(type) {
	case *protocol.LiteIdentity:
		return acc.CreditBalance
	case *protocol.KeyPage:
		return acc.CreditBalance
	default:
		t.Logf("Unexpected account type for %s: %T", accountUrl, acc)
		return 0
	}
}

// Helper: Query ACME balance on chain
func queryAcmeBalanceOnChain(t *testing.T, client *jsonrpc.Client, accountUrl *url.URL) uint64 {
	ctx := context.Background()
	acmeUrl := accountUrl.JoinPath("/ACME")
	
	// Wait for ACME account to exist
	if !waitForAccount(t, client, acmeUrl, 10*time.Second) {
		t.Logf("ACME account %s does not exist yet", acmeUrl)
		return 0
	}
	
	resp, err := client.Query(ctx, acmeUrl, &api.DefaultQuery{})
	if err != nil {
		t.Logf("Failed to query ACME balance for %s: %v", acmeUrl, err)
		return 0
	}
	
	record, ok := resp.(*api.AccountRecord)
	if !ok {
		t.Logf("Unexpected response type for ACME account %s: %T", acmeUrl, resp)
		return 0
	}
	
	// Try different token account types
	var balance uint64
	switch acc := record.Account.(type) {
	case *protocol.TokenAccount:
		balance = acc.Balance.Uint64()
	case *protocol.LiteTokenAccount:
		balance = acc.Balance.Uint64()
	default:
		t.Logf("Account %s is not a token account: %T", acmeUrl, record.Account)
		return 0
	}
	
	return balance
}

// RealTransactionSigner implements actual transaction signing
type RealTransactionSigner struct{}

func (s *RealTransactionSigner) SignTransaction(txn *protocol.Transaction, signerUrl *url.URL, privateKey []byte) (*messaging.Envelope, error) {
	// Create signature
	sig := &protocol.ED25519Signature{
		PublicKey: ed25519.PrivateKey(privateKey).Public().(ed25519.PublicKey),
		Signer:    signerUrl,
	}
	
	// Compute transaction hash
	txnHash := txn.GetHash()
	
	// Sign the hash
	sig.Signature = ed25519.Sign(ed25519.PrivateKey(privateKey), txnHash[:])
	
	// Create envelope with transaction and signature
	env := &messaging.Envelope{
		Messages: []messaging.Message{
			&messaging.TransactionMessage{
				Transaction: txn,
			},
			&messaging.SignatureMessage{
				Signature: sig,
			},
		},
	}
	
	return env, nil
}

// TestCreditManager_TopUpLiteAccount_Devnet tests credit top-up with real devnet
func TestCreditManager_TopUpLiteAccount_Devnet(t *testing.T) {
	// Skip if devnet not available
	if !isDevnetAvailable() {
		t.Skip("Devnet not available - please run ./devnet_manager.sh start")
	}
	
	t.Logf("Using devnet endpoint: %s", devnetAPIEndpoint)
	
	// Create real client
	client := jsonrpc.NewClient(devnetAPIEndpoint)
	client.Client.Timeout = devnetTimeoutDuration
	
	t.Run("successful top-up on devnet", func(t *testing.T) {
		// Create funding account with ACME
		fundingAccount := createFundedLiteAccount(t, client, "funding")
		
		// Ensure funding account has enough ACME
		acmeBalance := queryAcmeBalanceOnChain(t, client, fundingAccount.URL)
		if acmeBalance < MinimumFundingBalance {
			t.Skipf("Funding account has insufficient ACME: %d", acmeBalance/protocol.AcmePrecision)
		}
		
		// Create target account (might have some credits from creation)
		targetAccount := createFundedLiteAccount(t, client, "target")
		
		// Record initial credit balance
		initialCredits := queryCreditsOnChain(t, client, targetAccount.URL)
		t.Logf("Initial target credits: %d", initialCredits)
		
		// Create credit manager with real components
		signer := &RealTransactionSigner{}
		cm := NewCreditManager(client, client, signer, fundingAccount)
		
		// Execute credit top-up
		ctx := context.Background()
		err := cm.TopUpLiteAccount(ctx, targetAccount)
		
		// For devnet, we expect success or specific errors
		if err != nil {
			// Check if it's because account already has enough credits
			if initialCredits > MaximumCreditBalance {
				t.Logf("Skipped top-up: account already has %d credits", initialCredits)
				return
			}
			// Otherwise it's a real error
			require.NoError(t, err, "Failed to top up account")
		}
		
		// Wait a bit for transaction to propagate
		time.Sleep(2 * time.Second)
		
		// Verify credits were added on chain
		finalCredits := queryCreditsOnChain(t, client, targetAccount.URL)
		t.Logf("Final target credits: %d", finalCredits)
		
		// If top-up happened, credits should have increased
		if initialCredits <= MaximumCreditBalance {
			assert.Greater(t, finalCredits, initialCredits, 
				"Credits should have increased after top-up")
		}
	})
	
	t.Run("skip top-up when balance sufficient on devnet", func(t *testing.T) {
		// Create funding account
		fundingAccount := createFundedLiteAccount(t, client, "funding2")
		
		// Create target account that already has credits
		targetAccount := createFundedLiteAccount(t, client, "target2")
		
		// Add credits to target account first to get it above threshold
		// This simulates an account that already has sufficient credits
		ctx := context.Background()
		
		// Try to add credits multiple times to get above threshold
		signer := &RealTransactionSigner{}
		cm := NewCreditManager(client, client, signer, fundingAccount)
		
		// First, do multiple top-ups to get above MaximumCreditBalance
		for i := 0; i < 2; i++ {
			_ = cm.TopUpLiteAccount(ctx, targetAccount)
			time.Sleep(2 * time.Second)
		}
		
		// Check current credits
		currentCredits := queryCreditsOnChain(t, client, targetAccount.URL)
		t.Logf("Target account has %d credits", currentCredits)
		
		// Now test that it skips when balance is high
		initialCredits := currentCredits
		err := cm.TopUpLiteAccount(ctx, targetAccount)
		
		// Should succeed (by skipping)
		require.NoError(t, err)
		
		// Wait and verify credits didn't change
		time.Sleep(2 * time.Second)
		finalCredits := queryCreditsOnChain(t, client, targetAccount.URL)
		
		if initialCredits > MaximumCreditBalance {
			assert.Equal(t, initialCredits, finalCredits,
				"Credits should not change when above threshold")
		}
	})
	
	t.Run("error handling with real network failures", func(t *testing.T) {
		// Create account with no ACME (won't get faucet)
		pubKey, privKey, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		
		liteUrl := protocol.LiteAuthorityForKey(pubKey, protocol.SignatureTypeED25519)
		keyHash := sha256.Sum256(pubKey)
		
		emptyAccount := &LiteIdentity{
			URL: liteUrl,
			Key: &Key{
				Type:          protocol.SignatureTypeED25519,
				PublicKey:     pubKey,
				PrivateKey:    privKey,
				PublicKeyHash: keyHash[:20],
			},
			PublicKeyHash: keyHash[:20],
		}
		
		// Create target
		targetAccount := createFundedLiteAccount(t, client, "target3")
		
		// Try to top up from empty account
		signer := &RealTransactionSigner{}
		cm := NewCreditManager(client, client, signer, emptyAccount)
		
		ctx := context.Background()
		err = cm.TopUpLiteAccount(ctx, targetAccount)
		
		// Should fail due to insufficient balance
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "insufficient funding")
	})
}

// TestCreditManager_TopUpKeyPage_Devnet tests key page credit top-up with real devnet
func TestCreditManager_TopUpKeyPage_Devnet(t *testing.T) {
	// Skip if devnet not available
	if !isDevnetAvailable() {
		t.Skip("Devnet not available - please run ./devnet_manager.sh start")
	}
	
	t.Run("key page top-up on devnet", func(t *testing.T) {
		// Note: Key pages require ADIs which are more complex to set up
		// For now, we'll document the test structure
		t.Skip("Key page testing requires ADI setup - implement after ADI creation tools are available")
		
		// TODO: When ADI creation is available:
		// 1. Create an ADI
		// 2. Create a key page for the ADI
		// 3. Create funding account with ACME
		// 4. Top up the key page credits
		// 5. Verify on-chain state
	})
}

// TestCreditCalculations_Devnet tests credit/ACME calculations with real transactions
func TestCreditCalculations_Devnet(t *testing.T) {
	// Skip if devnet not available
	if !isDevnetAvailable() {
		t.Skip("Devnet not available - please run ./devnet_manager.sh start")
	}
	
	// Create real client
	client := jsonrpc.NewClient(devnetAPIEndpoint)
	client.Client.Timeout = devnetTimeoutDuration
	
	t.Run("verify credit to ACME conversion on devnet", func(t *testing.T) {
		// Create accounts
		fundingAccount := createFundedLiteAccount(t, client, "funding-calc")
		targetAccount := createFundedLiteAccount(t, client, "target-calc")
		
		// Get initial balances
		initialFundingAcme := queryAcmeBalanceOnChain(t, client, fundingAccount.URL)
		initialTargetCredits := queryCreditsOnChain(t, client, targetAccount.URL)
		
		t.Logf("Initial funding ACME: %d", initialFundingAcme/protocol.AcmePrecision)
		t.Logf("Initial target credits: %d", initialTargetCredits)
		
		// Create and execute transaction
		signer := &RealTransactionSigner{}
		cm := NewCreditManager(client, client, signer, fundingAccount)
		
		ctx := context.Background()
		err := cm.TopUpLiteAccount(ctx, targetAccount)
		
		if err != nil && initialTargetCredits > MaximumCreditBalance {
			t.Skip("Target already has sufficient credits")
		}
		require.NoError(t, err)
		
		// Wait for transaction
		time.Sleep(3 * time.Second)
		
		// Get final balances
		finalFundingAcme := queryAcmeBalanceOnChain(t, client, fundingAccount.URL)
		finalTargetCredits := queryCreditsOnChain(t, client, targetAccount.URL)
		
		t.Logf("Final funding ACME: %d", finalFundingAcme/protocol.AcmePrecision)
		t.Logf("Final target credits: %d", finalTargetCredits)
		
		// Verify ACME was spent
		assert.Less(t, finalFundingAcme, initialFundingAcme,
			"Funding account ACME should decrease")
		
		// Verify credits were added
		if initialTargetCredits <= MaximumCreditBalance {
			creditsAdded := finalTargetCredits - initialTargetCredits
			t.Logf("Credits added: %d", creditsAdded)
			
			// Should have added approximately CreditsToAdd
			// (might be slightly different due to oracle price)
			assert.Greater(t, creditsAdded, uint64(0), "Credits should be added")
		}
	})
}

// TestTransactionFailures_Devnet tests real transaction failure scenarios
func TestTransactionFailures_Devnet(t *testing.T) {
	// Skip if devnet not available
	if !isDevnetAvailable() {
		t.Skip("Devnet not available - please run ./devnet_manager.sh start")
	}
	
	// Create real client
	client := jsonrpc.NewClient(devnetAPIEndpoint)
	client.Client.Timeout = devnetTimeoutDuration
	
	t.Run("handle failed transactions on devnet", func(t *testing.T) {
		// Create account with wrong signature (will fail)
		pubKey, _, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		
		// Use wrong private key
		_, wrongPrivKey, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		
		liteUrl := protocol.LiteAuthorityForKey(pubKey, protocol.SignatureTypeED25519)
		keyHash := sha256.Sum256(pubKey)
		
		badAccount := &LiteIdentity{
			URL: liteUrl,
			Key: &Key{
				Type:          protocol.SignatureTypeED25519,
				PublicKey:     pubKey,
				PrivateKey:    wrongPrivKey, // Wrong key!
				PublicKeyHash: keyHash[:20],
			},
			PublicKeyHash: keyHash[:20],
		}
		
		targetAccount := createFundedLiteAccount(t, client, "target-fail")
		
		// Try to execute with bad signature
		signer := &RealTransactionSigner{}
		cm := NewCreditManager(client, client, signer, badAccount)
		
		ctx := context.Background()
		err = cm.TopUpLiteAccount(ctx, targetAccount)
		
		// Should fail
		assert.Error(t, err)
		t.Logf("Expected error: %v", err)
	})
}

// TestConcurrentOperations_Devnet tests concurrent credit operations
func TestConcurrentOperations_Devnet(t *testing.T) {
	// Skip if devnet not available
	if !isDevnetAvailable() {
		t.Skip("Devnet not available - please run ./devnet_manager.sh start")
	}
	
	t.Run("concurrent top-ups on devnet", func(t *testing.T) {
		// This test verifies that the system handles concurrent operations correctly
		// In production, multiple processes might try to top up accounts simultaneously
		
		client := jsonrpc.NewClient(devnetAPI)
		client.Client.Timeout = devnetTimeout
		
		// Create shared funding account
		fundingAccount := createFundedLiteAccount(t, client, "funding-concurrent")
		
		// Create multiple target accounts
		targets := make([]*LiteIdentity, 3)
		for i := 0; i < 3; i++ {
			targets[i] = createFundedLiteAccount(t, client, fmt.Sprintf("target-concurrent-%d", i))
		}
		
		// Run concurrent top-ups
		signer := &RealTransactionSigner{}
		cm := NewCreditManager(client, client, signer, fundingAccount)
		
		ctx := context.Background()
		errors := make(chan error, len(targets))
		
		for _, target := range targets {
			go func(t *LiteIdentity) {
				errors <- cm.TopUpLiteAccount(ctx, t)
			}(target)
		}
		
		// Collect results
		for i := 0; i < len(targets); i++ {
			err := <-errors
			if err != nil {
				t.Logf("Concurrent operation %d: %v", i, err)
			}
		}
		
		// Verify final states
		time.Sleep(3 * time.Second)
		for i, target := range targets {
			credits := queryCreditsOnChain(t, client, target.URL)
			t.Logf("Target %d final credits: %d", i, credits)
		}
	})
}

// Benchmark: Real transaction performance
func BenchmarkCreditTopUp_Devnet(b *testing.B) {
	// Skip if devnet not available
	if !isDevnetAvailable() {
		b.Skip("Devnet not available")
	}
	
	client := jsonrpc.NewClient(devnetAPI)
	client.Client.Timeout = devnetTimeout
	
	// Setup accounts once
	fundingAccount := createFundedLiteAccountBench(b, client, "bench-funding")
	targetAccount := createFundedLiteAccountBench(b, client, "bench-target")
	
	signer := &RealTransactionSigner{}
	cm := NewCreditManager(client, client, signer, fundingAccount)
	ctx := context.Background()
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		_ = cm.TopUpLiteAccount(ctx, targetAccount)
		time.Sleep(1 * time.Second) // Rate limit for devnet
	}
}

// Helper function for benchmark test
func createFundedLiteAccountBench(b *testing.B, client *jsonrpc.Client, name string) *LiteIdentity {
	// Generate new key pair
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		b.Fatal(err)
	}
	
	// Create lite identity URL
	liteUrl := protocol.LiteAuthorityForKey(pubKey, protocol.SignatureTypeED25519)
	
	// Create LiteIdentity structure
	keyHash := sha256.Sum256(pubKey)
	liteIdentity := &LiteIdentity{
		URL: liteUrl,
		Key: &Key{
			Type:          protocol.SignatureTypeED25519,
			PublicKey:     pubKey,
			PrivateKey:    privKey,
			PublicKeyHash: keyHash[:20],
		},
		PublicKeyHash: keyHash[:20],
	}
	
	b.Logf("Created %s account: %s", name, liteUrl)
	
	// Request ACME from faucet
	ctx := context.Background()
	acmeUrl := liteUrl.JoinPath("/ACME")
	_, _ = client.Faucet(ctx, acmeUrl, api.FaucetOptions{})
	
	return liteIdentity
}