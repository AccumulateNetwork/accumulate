// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build !testnet
// +build !testnet

package load_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// findAccumulatedPortsOld finds the ports that the accumulated process is listening on
// Deprecated: Use FindDevnetEndpoint from devnet_endpoint.go instead
func findAccumulatedPortsOld() []int {
	// First find the accumulated process PID
	output, err := exec.Command("pgrep", "-f", "accumulated run devnet").Output()
	if err != nil {
		return nil
	}
	
	pid := strings.TrimSpace(string(output))
	if pid == "" {
		return nil
	}
	
	// Use lsof to find listening ports for this PID
	cmd := exec.Command("lsof", "-Pan", "-p", pid, "-i")
	output, err = cmd.Output()
	if err != nil {
		// Try alternative method with ss
		cmd = exec.Command("ss", "-tlnp")
		output, err = cmd.Output()
		if err != nil {
			return nil
		}
	}
	
	// Parse the output to find listening ports
	ports := make(map[int]bool)
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		// Look for LISTEN state and extract port
		if strings.Contains(line, "LISTEN") {
			// Try to extract port from various formats
			// lsof format: *:26660 (LISTEN)
			// ss format: *:26660
			parts := strings.Fields(line)
			for _, part := range parts {
				if strings.Contains(part, ":") {
					portStr := strings.Split(part, ":")[len(strings.Split(part, ":"))-1]
					// Remove any trailing characters
					portStr = strings.TrimRight(portStr, " )")
					if port, err := strconv.Atoi(portStr); err == nil && port > 1024 && port < 65536 {
						ports[port] = true
					}
				}
			}
		}
	}
	
	// Convert map to slice
	var result []int
	for port := range ports {
		result = append(result, port)
	}
	
	// Sort ports for consistent ordering
	sort.Ints(result)
	return result
}

// findDevnetEndpoint attempts to discover a running devnet endpoint
func findDevnetEndpoint(t *testing.T) string {
	// Check environment variable first
	if endpoint := os.Getenv("DEVNET_ENDPOINT"); endpoint != "" {
		t.Logf("Using DEVNET_ENDPOINT from environment: %s", endpoint)
		return endpoint
	}

	// Try to find accumulated process and its ports
	t.Log("Looking for accumulated process and its listening ports...")
	accumulatedPorts := findAccumulatedPortsOld()
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	// First try ports from the actual process
	if len(accumulatedPorts) > 0 {
		t.Logf("Found accumulated process listening on ports: %v", accumulatedPorts)
		for _, port := range accumulatedPorts {
			endpoint := fmt.Sprintf("http://localhost:%d/v3", port)
			client := jsonrpc.NewClient(endpoint)
			client.Client.Timeout = 1 * time.Second

			// Try to get network status
			_, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
			if err == nil {
				t.Logf("Found devnet API endpoint at: %s", endpoint)
				return endpoint
			}
		}
	}

	// Fallback to common devnet ports
	commonPorts := []int{
		26660, // BVN0
		26760, // BVN1  
		26860, // BVN2
		26960, // DN
		8080,  // Default local port
		9090,  // Alternative port
	}

	t.Log("Checking common devnet ports...")
	for _, port := range commonPorts {
		// Skip if already checked
		alreadyChecked := false
		for _, p := range accumulatedPorts {
			if p == port {
				alreadyChecked = true
				break
			}
		}
		if alreadyChecked {
			continue
		}
		
		endpoint := fmt.Sprintf("http://localhost:%d/v3", port)
		client := jsonrpc.NewClient(endpoint)
		client.Client.Timeout = 1 * time.Second

		// Try to get network status
		_, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
		if err == nil {
			t.Logf("Found devnet endpoint at: %s", endpoint)
			return endpoint
		}
	}

	// Check if accumulated process is running
	output, err := exec.Command("ps", "aux").Output()
	if err == nil && bytes.Contains(output, []byte("accumulated run devnet")) {
		t.Error("accumulated devnet process is running but API endpoint not accessible")
		if len(accumulatedPorts) > 0 {
			t.Logf("Process is listening on ports: %v but none respond to API calls", accumulatedPorts)
		}
		t.Log("Set DEVNET_ENDPOINT environment variable to specify the correct endpoint")
	} else {
		t.Error("No devnet is running. Start devnet with: ./accumulated run devnet")
	}

	return ""
}

func execCommand(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

// TestDevnetLoadTest replicates the cheap_load bash script functionality
// This test automatically discovers and connects to a running devnet
func TestDevnetLoadTest(t *testing.T) {
	// Skip if explicitly disabled
	if os.Getenv("SKIP_DEVNET_TESTS") == "true" {
		t.Skip("Skipping devnet test (SKIP_DEVNET_TESTS=true)")
	}

	// Find devnet endpoint
	endpoint := findDevnetEndpoint(t)
	if endpoint == "" {
		t.Fatal("Failed to find devnet endpoint. Please ensure devnet is running.")
	}

	// Test configuration
	const (
		numKAccounts   = 10
		numAAccounts   = 10
		targetBalance  = 10 * 1e8  // 10 ACME in fixed point
		creditAmount   = 1000       // Credits to add (1000 credits = 10 ACME with oracle 100:1)
		txCount        = 100        // Number of transactions to send
		txAmount       = 0.001 * 1e8 // 0.001 ACME per transaction in fixed point
	)

	// Create client
	client := jsonrpc.NewClient(endpoint)
	client.Client.Timeout = 30 * time.Second
	ctx := context.Background()

	// Generate keys for k accounts and a accounts
	type Account struct {
		Key         ed25519.PrivateKey
		LiteURL     *url.URL // The token account URL (with /ACME)
		LiteIdentity *url.URL // The lite identity URL (without /ACME)
		Balance     *big.Int
	}

	kAccounts := make([]Account, numKAccounts)
	aAccounts := make([]Account, numAAccounts)

	// Step 1: Create k1-k10 accounts
	t.Log("Step 1: Creating k1-k10 accounts...")
	for i := range kAccounts {
		seed := fmt.Sprintf("k%d test seed", i+1)
		hash := sha256.Sum256([]byte(seed))
		kAccounts[i].Key = ed25519.NewKeyFromSeed(hash[:])
		
		kAccounts[i].LiteURL, _ = protocol.LiteTokenAddress(kAccounts[i].Key[32:], "ACME", protocol.SignatureTypeED25519)
		kAccounts[i].LiteIdentity = kAccounts[i].LiteURL.Identity()
		kAccounts[i].Balance = big.NewInt(0)
		
		t.Logf("k%d: %s", i+1, kAccounts[i].LiteURL)
	}

	// Step 2: Fund k1-k10 accounts via faucet
	t.Log("Step 2: Funding k accounts via faucet...")
	
	// Make all faucet calls concurrently (10 ACME per call, 10 calls per account = 100 ACME total)
	const faucetCallsPerAccount = 10
	const acmePerFaucetCall = 10 * 1e8 // 10 ACME in fixed point
	const targetTotalBalance = 100 * 1e8 // 100 ACME in fixed point
	
	var wg sync.WaitGroup
	for i, account := range kAccounts {
		wg.Add(1)
		go func(idx int, acc Account) {
			defer wg.Done()
			t.Logf("Starting %d faucet calls for k%d...", faucetCallsPerAccount, idx+1)
			
			for call := 0; call < faucetCallsPerAccount; call++ {
				submission, err := client.Faucet(ctx, acc.LiteURL, api.FaucetOptions{})
				if err != nil {
					t.Logf("Faucet error for k%d call %d: %v", idx+1, call+1, err)
				} else if submission.Status != nil && submission.Status.Error != nil {
					t.Logf("Faucet returned error for k%d call %d: %v", idx+1, call+1, submission.Status.Error)
				} else {
					t.Logf("Faucet call %d for k%d submitted", call+1, idx+1)
				}
				// Small delay between calls from same goroutine
				time.Sleep(100 * time.Millisecond)
			}
		}(i, account)
	}
	
	// Wait for all faucet calls to complete
	wg.Wait()
	t.Log("All faucet calls submitted, waiting for balances to accumulate...")
	
	// Wait and verify all accounts reach 100 ACME
	maxWaitTime := 60 * time.Second
	checkInterval := 2 * time.Second
	startTime := time.Now()
	
	for {
		allFunded := true
		for i, account := range kAccounts {
			record, err := client.Query(ctx, account.LiteURL, &api.DefaultQuery{})
			if err != nil {
				t.Logf("Failed to query k%d: %v", i+1, err)
				allFunded = false
				continue
			}
			
			currentBalance := big.NewInt(0)
			if accRecord, ok := record.(*api.AccountRecord); ok {
				if tokenAccount, ok := accRecord.Account.(*protocol.LiteTokenAccount); ok {
					currentBalance = &tokenAccount.Balance
					kAccounts[i].Balance = currentBalance
				}
			}
			
			balanceACME := new(big.Float).Quo(new(big.Float).SetInt(currentBalance), big.NewFloat(1e8))
			if currentBalance.Cmp(big.NewInt(targetTotalBalance)) < 0 {
				t.Logf("k%d balance: %s ACME (waiting for 100 ACME)", i+1, balanceACME.String())
				allFunded = false
			} else {
				t.Logf("k%d balance: %s ACME ✓", i+1, balanceACME.String())
			}
		}
		
		if allFunded {
			t.Log("All k accounts successfully funded with 100 ACME!")
			break
		}
		
		if time.Since(startTime) > maxWaitTime {
			t.Error("Timeout waiting for k accounts to reach 100 ACME")
			break
		}
		
		time.Sleep(checkInterval)
	}

	// Step 2.5: Get oracle and add credits to k accounts concurrently
	t.Log("Step 2.5: Getting oracle price and adding credits to all k accounts...")
	
	// Get the oracle price from the network
	status, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
	if err != nil {
		t.Fatalf("Failed to get oracle: %v", err)
	}
	oracle := status.Oracle.Price
	t.Logf("Oracle price: %d (%.2f USD per ACME)", oracle, float64(oracle)/1e8)
	
	// Submit all AddCredits transactions concurrently
	var creditWg sync.WaitGroup
	for i, account := range kAccounts {
		creditWg.Add(1)
		go func(idx int, acc Account) {
			defer creditWg.Done()
			
			// Build add credits transaction
			// For lite accounts, source and recipient are the same
			env, err := build.Transaction().
				For(acc.LiteURL).  // Source: the token account
				Body(&protocol.AddCredits{
					Recipient: acc.LiteURL,  // Same as source for lite accounts
					Amount:    *big.NewInt(10 * 1e8), // 10 ACME to spend
					Oracle:    oracle, // Use actual oracle from network
				}).
				SignWith(acc.LiteURL).Version(1).Timestamp(uint64(time.Now().UnixNano())).PrivateKey(acc.Key).
				Done()
			
			if err != nil {
				t.Logf("Failed to build add credits transaction for k%d: %v", idx+1, err)
				return
			}

			// Submit transaction
			submissions, err := client.Submit(ctx, env, api.SubmitOptions{})
			if err != nil {
				t.Logf("Failed to add credits to k%d: %v", idx+1, err)
				return
			}
			
			success := true
			for _, sub := range submissions {
				if sub.Status != nil && sub.Status.Error != nil {
					t.Logf("Add credits error for k%d: %v", idx+1, sub.Status.Error)
					success = false
				}
			}
			if success {
				t.Logf("Successfully submitted add credits for k%d (1000 credits for 10 ACME)", idx+1)
			}
		}(i, account)
	}
	
	// Wait for all submissions to complete
	creditWg.Wait()
	t.Log("All AddCredits transactions submitted")

	// Wait and verify credits were added with retries
	t.Log("Waiting for credits to settle and verifying...")
	// Wait a bit first for transactions to be processed
	time.Sleep(10 * time.Second)
	
	maxCreditWait := 30 * time.Second
	creditCheckInterval := 2 * time.Second
	creditStartTime := time.Now()
	
	for {
		creditsVerified := 0
		totalCredits := uint64(0)
		
		for i, account := range kAccounts {
			// Query the lite identity to get credit balance
			record, err := client.Query(ctx, account.LiteIdentity, &api.DefaultQuery{})
			if err != nil {
				t.Logf("Failed to query k%d identity for credits: %v", i+1, err)
				continue
			}
			
			if accRecord, ok := record.(*api.AccountRecord); ok {
				if liteIdentity, ok := accRecord.Account.(*protocol.LiteIdentity); ok {
					if liteIdentity.CreditBalance >= 1000 {
						creditsVerified++
						totalCredits += liteIdentity.CreditBalance
					}
					t.Logf("k%d has %d credits", i+1, liteIdentity.CreditBalance)
				}
			}
		}
		
		if creditsVerified == numKAccounts {
			t.Logf("✓ All %d k accounts have 1000+ credits (total: %d credits)", numKAccounts, totalCredits)
			break
		}
		
		if time.Since(creditStartTime) > maxCreditWait {
			t.Logf("Warning: Timeout waiting for credits to settle (%d/%d accounts have 1000+ credits)", creditsVerified, numKAccounts)
			break
		}
		
		t.Logf("Waiting for credits to settle (%d/%d accounts have 1000+ credits)...", creditsVerified, numKAccounts)
		time.Sleep(creditCheckInterval)
	}

	// Step 3: Create a1-a10 accounts
	t.Log("Step 3: Creating a1-a10 accounts...")
	for i := range aAccounts {
		seed := fmt.Sprintf("a%d test seed", i+1)
		hash := sha256.Sum256([]byte(seed))
		aAccounts[i].Key = ed25519.NewKeyFromSeed(hash[:])
		
		aAccounts[i].LiteURL, _ = protocol.LiteTokenAddress(aAccounts[i].Key[32:], "ACME", protocol.SignatureTypeED25519)
		aAccounts[i].LiteIdentity = aAccounts[i].LiteURL.Identity()
		aAccounts[i].Balance = big.NewInt(0)
		
		t.Logf("a%d: %s", i+1, aAccounts[i].LiteURL)
	}

	// Step 4: Send any existing ACME from a accounts back to k1 (cleanup)
	t.Log("Step 4: Cleaning up a accounts...")
	for i, account := range aAccounts {
		record, err := client.Query(ctx, account.LiteURL, &api.DefaultQuery{})
		if err == nil {
			if accRecord, ok := record.(*api.AccountRecord); ok {
				if tokenAccount, ok := accRecord.Account.(*protocol.LiteTokenAccount); ok {
					if tokenAccount.Balance.Cmp(big.NewInt(0)) > 0 {
						// Send balance back to k1
						sendAmount := new(big.Int).Sub(&tokenAccount.Balance, big.NewInt(int64(0.001*1e8))) // Keep small amount for fees
						if sendAmount.Cmp(big.NewInt(0)) > 0 {
							t.Logf("Sending %s ACME from a%d to k1", 
								new(big.Float).Quo(new(big.Float).SetInt(sendAmount), big.NewFloat(1e8)), i+1)
							
							env, err := build.Transaction().
								For(account.LiteURL).
								SendTokens(sendAmount, 0).To(kAccounts[0].LiteURL).
								SignWith(account.LiteURL).Version(1).Timestamp(uint64(time.Now().UnixNano())).PrivateKey(account.Key).
								Done()
							if err == nil {
								client.Submit(ctx, env, api.SubmitOptions{})
								time.Sleep(500 * time.Millisecond)
							}
						}
					}
				}
			}
		}
	}

	// Step 6: Send transactions in round-robin fashion
	t.Log("Step 6: Sending transactions in round-robin...")
	
	sentToA := make([]float64, numAAccounts)
	successCount := 0
	failCount := 0
	
	for txNum := 0; txNum < txCount; txNum++ {
		kIndex := txNum % numKAccounts
		aIndex := txNum % numAAccounts
		
		fromAccount := kAccounts[kIndex]
		toAccount := aAccounts[aIndex]
		
		// Build transaction
		env, err := build.Transaction().
			For(fromAccount.LiteURL).
			SendTokens(big.NewInt(int64(txAmount)), 0).To(toAccount.LiteURL).
			SignWith(fromAccount.LiteURL).Version(1).Timestamp(uint64(time.Now().UnixNano())).PrivateKey(fromAccount.Key).
			Done()
		
		if err != nil {
			t.Logf("[%d/%d] Failed to build transaction: %v", txNum+1, txCount, err)
			failCount++
			continue
		}
		
		// Submit transaction
		t.Logf("[%d/%d] Sending %.4f ACME: k%d -> a%d", 
			txNum+1, txCount, txAmount/1e8, kIndex+1, aIndex+1)
		
		submissions, err := client.Submit(ctx, env, api.SubmitOptions{})
		if err != nil {
			t.Logf("Submit error: %v", err)
			failCount++
			continue
		}
		
		// Check submission status
		success := true
		for _, sub := range submissions {
			if sub.Status != nil && sub.Status.Error != nil {
				t.Logf("Transaction error: %v", sub.Status.Error)
				success = false
				failCount++
			}
		}
		
		if success {
			sentToA[aIndex] += txAmount / 1e8
			successCount++
		}
		
		// Pause every 10 transactions
		if (txNum+1)%10 == 0 {
			time.Sleep(100 * time.Millisecond)
		}
	}
	
	t.Logf("Transaction summary: %d successful, %d failed out of %d total", 
		successCount, failCount, txCount)

	// Step 7: Verify balances
	t.Log("Step 7: Verifying account balances...")
	t.Log("Expected balances (based on successful transactions):")
	for i := 0; i < numAAccounts; i++ {
		txCountForA := int(sentToA[i] / (txAmount / 1e8))
		t.Logf("  a%d: %.4f ACME (%d transactions)", i+1, sentToA[i], txCountForA)
	}
	
	// Verification with retries
	maxAttempts := 20
	waitTime := 2 * time.Second
	
	t.Logf("Starting balance verification (will retry up to %d times with %v delays)...", 
		maxAttempts, waitTime)
	
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		t.Logf("Verification attempt %d/%d:", attempt, maxAttempts)
		
		allMatch := true
		matchedCount := 0
		
		for i, account := range aAccounts {
			record, err := client.Query(ctx, account.LiteURL, &api.DefaultQuery{})
			if err != nil {
				t.Logf("  ⚠ a%d: Failed to query account: %v", i+1, err)
				allMatch = false
				continue
			}
			
			actualBalance := big.NewInt(0)
			if accRecord, ok := record.(*api.AccountRecord); ok {
				if tokenAccount, ok := accRecord.Account.(*protocol.LiteTokenAccount); ok {
					actualBalance = &tokenAccount.Balance
				}
			}
			
			actualFloat := new(big.Float).Quo(new(big.Float).SetInt(actualBalance), big.NewFloat(1e8))
			expectedFloat := sentToA[i]
			diff := new(big.Float).Sub(actualFloat, big.NewFloat(expectedFloat))
			
			// Allow small tolerance for fees (0.00001 ACME)
			tolerance := 0.00001
			diffFloat, _ := diff.Float64()
			
			if diffFloat < 0 {
				diffFloat = -diffFloat
			}
			
			if diffFloat < tolerance {
				t.Logf("  ✓ a%d: %s ACME (expected: %.4f, diff: %.8f)", 
					i+1, actualFloat.String(), expectedFloat, diffFloat)
				matchedCount++
			} else {
				t.Logf("  ✗ a%d: %s ACME (expected: %.4f, diff: %.8f)", 
					i+1, actualFloat.String(), expectedFloat, diffFloat)
				allMatch = false
			}
		}
		
		t.Logf("  Summary: %d/%d accounts match", matchedCount, numAAccounts)
		
		if allMatch {
			t.Log("✅ SUCCESS: All balances match expected values!")
			return
		}
		
		if attempt < maxAttempts {
			t.Logf("⏳ Balances don't match yet. Waiting %v before retry...", waitTime)
			time.Sleep(waitTime)
		}
	}
	
	t.Error("❌ FAILURE: Balances did not match after maximum attempts")
}

// TestDevnetConcurrentLoad tests concurrent transaction submission
func TestDevnetConcurrentLoad(t *testing.T) {
	// Skip if explicitly disabled
	if os.Getenv("SKIP_DEVNET_TESTS") == "true" {
		t.Skip("Skipping devnet test (SKIP_DEVNET_TESTS=true)")
	}

	// Find devnet endpoint
	endpoint := findDevnetEndpoint(t)
	if endpoint == "" {
		t.Fatal("Failed to find devnet endpoint. Please ensure devnet is running.")
	}

	// Create client
	client := jsonrpc.NewClient(endpoint)
	client.Client.Timeout = 30 * time.Second
	ctx := context.Background()

	// Generate test accounts
	numAccounts := 5
	accounts := make([]struct {
		Key          ed25519.PrivateKey
		LiteURL      *url.URL
		LiteIdentity *url.URL
	}, numAccounts)

	for i := range accounts {
		seed := fmt.Sprintf("concurrent test seed %d", i)
		hash := sha256.Sum256([]byte(seed))
		accounts[i].Key = ed25519.NewKeyFromSeed(hash[:])
		
		accounts[i].LiteURL, _ = protocol.LiteTokenAddress(accounts[i].Key[32:], "ACME", protocol.SignatureTypeED25519)
		accounts[i].LiteIdentity = accounts[i].LiteURL.Identity()
		
		// Fund account via faucet
		t.Logf("Funding account %d: %s", i, accounts[i].LiteURL)
		client.Faucet(ctx, accounts[i].LiteURL, api.FaucetOptions{})
	}

	// Wait for funding
	time.Sleep(5 * time.Second)

	// Add credits
	for _, account := range accounts {
		env, _ := build.Transaction().
			For(account.LiteURL).
			Body(&protocol.AddCredits{
				Recipient: account.LiteURL,
				Amount:    *big.NewInt(1000 * 1e8),
				Oracle:    1e8,
			}).
			SignWith(account.LiteURL).Version(1).Timestamp(uint64(time.Now().UnixNano())).PrivateKey(account.Key).
			Done()
		client.Submit(ctx, env, api.SubmitOptions{})
	}

	time.Sleep(3 * time.Second)

	// Submit transactions concurrently
	const numWorkers = 5
	const txPerWorker = 10
	
	var wg sync.WaitGroup
	successCount := int32(0)
	failCount := int32(0)

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			for i := 0; i < txPerWorker; i++ {
				fromIdx := workerID % numAccounts
				toIdx := (workerID + 1) % numAccounts
				
				from := accounts[fromIdx]
				to := accounts[toIdx]
				
				env, err := build.Transaction().
					For(from.LiteURL).
					SendTokens(big.NewInt(int64(0.001*1e8)), 0).To(to.LiteURL).
					SignWith(from.LiteURL).Version(1).Timestamp(uint64(time.Now().UnixNano())).PrivateKey(from.Key).
					Done()
				
				if err != nil {
					failCount++
					continue
				}
				
				_, err = client.Submit(ctx, env, api.SubmitOptions{})
				if err != nil {
					failCount++
				} else {
					successCount++
				}
				
				time.Sleep(100 * time.Millisecond)
			}
		}(w)
	}

	wg.Wait()
	
	t.Logf("Concurrent test completed: %d successful, %d failed", successCount, failCount)
}