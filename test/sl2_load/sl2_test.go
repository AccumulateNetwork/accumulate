package sl2_load_test

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"strconv"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Account struct {
	PrivateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
	LiteURL    *url.URL
}

type SL2Test struct {
	// Core components
	fundingAccount Account
	testAccounts   []Account
	client         *jsonrpc.Client
	
	// Test configuration parameters
	// Note: Design document mentions these but doesn't specify how they're set
	
	// Performance metrics tracking
	successCount   int32
	failureCount   int32
	startTime      time.Time
	
	// Test metadata
	seed           string
	timestamp      int64
}

func TestSL2Load(t *testing.T) {
	// Parse first argument for number of faucet calls (default 1)
	numFaucetCalls := 1
	args := flag.Args()
	if len(args) > 0 {
		if n, err := strconv.Atoi(args[0]); err == nil && n > 0 {
			numFaucetCalls = n
		}
	}
	
	test := &SL2Test{}
	
	// Test Initialization Flow (as per design section 4)
	// Step 1: Generate funding account from hardcoded seed
	fundingPrivKey := sha256.Sum256([]byte("sl2_load"))
	test.fundingAccount = createAccount(fundingPrivKey[:])
	
	// Step 2: Create timestamp-based seed for test accounts
	currentTime := time.Now()
	test.timestamp = currentTime.UnixNano()
	timestampBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(timestampBytes, uint64(test.timestamp))
	seedHash := sha256.Sum256(timestampBytes)
	test.seed = hex.EncodeToString(seedHash[:])
	
	// Step 3: Print configuration (time, seed, funding details)
	fmt.Println("=== SL2 Load Test Configuration ===")
	fmt.Printf("Current Time: %s\n", currentTime.Format(time.RFC3339Nano))
	fmt.Printf("Timestamp (ns): %d\n", test.timestamp)
	fmt.Printf("Seed: %s\n", test.seed)
	fmt.Printf("Faucet Calls: %d\n", numFaucetCalls)
	fmt.Println("\n=== Funding Account ===")
	fmt.Printf("Private Key: %x\n", test.fundingAccount.PrivateKey)
	fmt.Printf("Public Key: %x\n", test.fundingAccount.PublicKey)
	fmt.Printf("Lite URL: %s\n", test.fundingAccount.LiteURL)
	
	// Step 4: Generate 100 lite accounts deterministically
	test.testAccounts = make([]Account, 100)
	fmt.Println("\n=== Test Accounts ===")
	for i := 0; i < 100; i++ {
		// Account[i] key: SHA256(seed || 8_bytes_of_i)
		accountSeed := make([]byte, 40) // 32 bytes seed + 8 bytes index
		copy(accountSeed[:32], seedHash[:])
		binary.BigEndian.PutUint64(accountSeed[32:], uint64(i))
		accountKeyHash := sha256.Sum256(accountSeed)
		test.testAccounts[i] = createAccount(accountKeyHash[:])
	}
	
	// Step 5: Display account URLs for verification
	fmt.Printf("Created %d accounts\n", len(test.testAccounts))
	for i := 0; i < 3; i++ {
		fmt.Printf("Account %3d: %s\n", i, test.testAccounts[i].LiteURL)
	}
	fmt.Println("...")
	fmt.Printf("Account %3d: %s\n", 99, test.testAccounts[99].LiteURL)
	
	// Step 6: Initialize lite client (lazy initialization on first use)
	// This happens in the faucet module when needed
	
	// Phase 1: Setup (as per design)
	t.Log("\n=== Phase 1: Setup ===")
	
	// Multiple faucet funding with settlement verification
	balance, err := fundWithMultipleCalls(test, test.fundingAccount.LiteURL, numFaucetCalls, t)
	if err != nil {
		t.Fatalf("Failed to fund account: %v", err)
	}
	t.Logf("Funding account final balance: %d ACME", balance)
	
	// Phase 2: Load Generation - NOT IMPLEMENTED
	// Design says: "Pending: Transaction execution"
	t.Log("\n=== Phase 2: Load Generation ===")
	t.Log("Transaction execution not yet implemented (marked as pending in design)")
	
	// Phase 3: Verification - NOT IMPLEMENTED  
	// Design says: "Pending: Performance monitoring, Report generation"
	t.Log("\n=== Phase 3: Verification ===")
	t.Log("Performance monitoring and report generation not yet implemented (marked as pending in design)")
}

func createAccount(seed []byte) Account {
	// Use seed to generate ed25519 key pair
	privKey := ed25519.NewKeyFromSeed(seed[:32])
	pubKey := privKey.Public().(ed25519.PublicKey)
	
	// Create lite token account URL
	liteURL, _ := protocol.LiteTokenAddress(pubKey, protocol.ACME, protocol.SignatureTypeED25519)
	
	return Account{
		PrivateKey: privKey,
		PublicKey:  pubKey,
		LiteURL:    liteURL,
	}
}

func fundWithMultipleCalls(test *SL2Test, accountURL *url.URL, numCalls int, t *testing.T) (int64, error) {
	// Ensure client is initialized
	if test.client == nil {
		client, err := InitializeClient()
		if err != nil {
			return 0, fmt.Errorf("failed to initialize client: %w", err)
		}
		test.client = client
		t.Log("Client initialized for devnet")
	}
	
	// Get starting balance (may be non-zero from previous runs)
	startingBalance, _ := queryBalance(test.client, accountURL)
	if startingBalance > 0 {
		t.Logf("Starting balance: %d ACME", startingBalance)
	} else {
		t.Log("Starting balance: 0 ACME (account may not exist yet)")
		startingBalance = 0
	}
	
	// Expected final balance = starting balance + (N * 10 ACME)
	expectedBalance := startingBalance + int64(numCalls*10)
	t.Logf("Expected final balance after %d faucet calls: %d ACME", numCalls, expectedBalance)
	
	// Call faucet N times
	t.Logf("Calling faucet %d times...", numCalls)
	for i := 0; i < numCalls; i++ {
		err := FundAccount(test, accountURL)
		if err != nil {
			t.Logf("Warning: Faucet call %d failed: %v", i+1, err)
		} else {
			t.Logf("Faucet call %d completed", i+1)
		}
	}
	
	// Settlement verification
	t.Log("Waiting for balance settlement...")
	var lastBalance int64 = startingBalance
	var lastChangeTime = time.Now()
	deadline := time.Now().Add(1 * time.Minute)
	pollInterval := 2 * time.Second
	attemptCount := 0
	
	for {
		attemptCount++
		currentBalance, err := queryBalance(test.client, accountURL)
		if err != nil {
			// Account might not exist yet
			currentBalance = 0
		}
		
		// Check if balance changed
		if currentBalance != lastBalance {
			t.Logf("Attempt %d: Balance changed from %d to %d ACME", 
				attemptCount, lastBalance, currentBalance)
			lastBalance = currentBalance
			lastChangeTime = time.Now()
			// Reset deadline on balance change
			deadline = time.Now().Add(1 * time.Minute)
		}
		
		// End conditions (whichever comes first)
		// a) Balance reaches expected total
		if currentBalance >= expectedBalance {
			t.Logf("✓ Balance reached expected total: %d ACME", currentBalance)
			return currentBalance, nil
		}
		
		// b) 1 minute passes with no balance change
		if time.Since(lastChangeTime) > 1*time.Minute {
			if currentBalance > startingBalance {
				t.Logf("⏱ Settlement timeout - balance stabilized at %d ACME (expected %d)", 
					currentBalance, expectedBalance)
				return currentBalance, nil
			} else {
				return currentBalance, fmt.Errorf("no balance increase after 1 minute")
			}
		}
		
		// Check overall deadline
		if time.Now().After(deadline) {
			t.Logf("⏱ Overall timeout reached - final balance: %d ACME (expected %d)", 
				currentBalance, expectedBalance)
			return currentBalance, nil
		}
		
		// Log progress periodically
		if attemptCount%10 == 0 {
			t.Logf("Attempt %d: Current balance %d ACME, waiting for %d ACME", 
				attemptCount, currentBalance, expectedBalance)
		}
		
		time.Sleep(pollInterval)
	}
}

func verifyBalanceWithSettlement(test *SL2Test, accountURL *url.URL, t *testing.T) (int64, error) {
	// Ensure client is initialized
	if test.client == nil {
		client, err := InitializeClient()
		if err != nil {
			return 0, fmt.Errorf("failed to initialize client: %w", err)
		}
		test.client = client
	}
	
	// Poll for up to 1 minute for account creation and balance
	// If balance changes or account appears, reset the 1-minute deadline
	var lastBalance int64 = -1
	var lastError error
	deadline := time.Now().Add(1 * time.Minute)
	pollInterval := 2 * time.Second
	
	t.Log("Waiting for account settlement (up to 1 minute)...")
	
	attemptCount := 0
	for time.Now().Before(deadline) {
		attemptCount++
		balance, err := queryBalance(test.client, accountURL)
		
		// Check if something changed
		changed := false
		if err == nil && lastError != nil {
			// Account appeared!
			t.Logf("Attempt %d: Account created with balance: %d ACME (deadline reset)", attemptCount, balance)
			changed = true
		} else if err == nil && balance != lastBalance {
			// Balance changed
			t.Logf("Attempt %d: Balance changed from %d to %d ACME (deadline reset)", attemptCount, lastBalance, balance)
			changed = true
		} else if err != nil && lastError == nil {
			// Account disappeared? This is unexpected
			t.Logf("Attempt %d: Account query error: %v (deadline reset)", attemptCount, err)
			changed = true
		} else if err != nil && lastError != nil && err.Error() != lastError.Error() {
			// Different error
			t.Logf("Attempt %d: Error changed: %v (deadline reset)", attemptCount, err)
			changed = true
		}
		
		if changed {
			// Something changed - reset deadline
			deadline = time.Now().Add(1 * time.Minute)
			lastBalance = balance
			lastError = err
		}
		
		// If we have a balance and it hasn't changed in the last iteration, we're done
		if err == nil && balance == lastBalance && attemptCount > 1 {
			t.Logf("Balance stabilized at %d ACME", balance)
			return balance, nil
		}
		
		// Log periodically even if nothing changed
		if attemptCount % 10 == 0 && err != nil {
			t.Logf("Attempt %d: Still waiting, account not found", attemptCount)
		}
		
		time.Sleep(pollInterval)
	}
	
	// Timeout reached
	if lastError != nil {
		return 0, fmt.Errorf("account not created after 1 minute: %w", lastError)
	}
	return lastBalance, nil
}

func queryBalance(client *jsonrpc.Client, accountURL *url.URL) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	// Query the account
	resp, err := client.Query(ctx, accountURL, nil)
	if err != nil {
		return 0, err
	}
	
	// Cast to AccountRecord
	accRecord, ok := resp.(*api.AccountRecord)
	if !ok {
		return 0, fmt.Errorf("unexpected response type: %T", resp)
	}
	
	// Check if it's a lite token account
	liteAccount, ok := accRecord.Account.(*protocol.LiteTokenAccount)
	if !ok {
		return 0, fmt.Errorf("account is not a lite token account: %T", accRecord.Account)
	}
	
	// Return balance in ACME tokens (1 ACME = 1e8 units)
	return int64(liteAccount.Balance.Int64() / 1e8), nil
}