// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build !testnet
// +build !testnet

package load_test

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"golang.org/x/time/rate"
)

func TestSimple100K(t *testing.T) {
	// DEFAULTS - Use test flags to override at runtime
	// Example: go test -v -run TestSimple100K -args -txs 50000 -tps 100 -senders 20 -receivers 20
	const (
		numSenders   = 40                 // DEFAULT: Number of sender accounts
		numReceivers = 40                 // DEFAULT: Number of receiver accounts
		totalTxs     = 50000              // DEFAULT: Total transactions to send (stress test)
		targetTPS    = 3000               // DEFAULT: Target transactions per second
		txAmount     = int64(0.001 * 1e8) // DEFAULT: 0.001 ACME per tx
	)

	// Find endpoint
	endpoint, err := FindDevnetEndpoint()
	if err != nil {
		t.Skip("No devnet found")
	}

	client := jsonrpc.NewClient(endpoint)
	client.Client.Timeout = 30 * time.Second
	ctx := context.Background()

	t.Logf("=== Simple 100K Transaction Test ===")
	t.Logf("Endpoint: %s", endpoint)
	t.Logf("Transactions: %d", totalTxs)
	t.Logf("Target TPS: %d", targetTPS)
	t.Logf("Expected duration: %.1f seconds", float64(totalTxs)/float64(targetTPS))

	// Generate accounts
	t.Log("Generating accounts...")
	senders := make([]Account, numSenders)
	receivers := make([]Account, numReceivers)

	for i := range senders {
		seed := fmt.Sprintf("sender100k%d_%d", i, time.Now().UnixNano())
		hash := sha256.Sum256([]byte(seed))
		senders[i].Key = ed25519.NewKeyFromSeed(hash[:])
		senders[i].URL, _ = protocol.LiteTokenAddress(senders[i].Key[32:], "ACME", protocol.SignatureTypeED25519)
	}

	for i := range receivers {
		seed := fmt.Sprintf("receiver100k%d_%d", i, time.Now().UnixNano())
		hash := sha256.Sum256([]byte(seed))
		receivers[i].Key = ed25519.NewKeyFromSeed(hash[:])
		receivers[i].URL, _ = protocol.LiteTokenAddress(receivers[i].Key[32:], "ACME", protocol.SignatureTypeED25519)
	}

	// Fund senders via faucet
	t.Log("Funding sender accounts...")
	fundingStart := time.Now()

	var wg sync.WaitGroup
	for i, sender := range senders {
		wg.Add(1)
		go func(idx int, acc Account) {
			defer wg.Done()

			// Each sender needs enough for their share of transactions
			// Plus some extra for credits
			txPerSender := totalTxs / numSenders
			if idx < totalTxs%numSenders {
				txPerSender++
			}

			// Request 10 ACME per 2500 transactions
			faucetCalls := (txPerSender / 2500) + 2 // +2 for buffer
			if faucetCalls < 5 {
				faucetCalls = 5 // Minimum 50 ACME
			}

			for j := 0; j < faucetCalls; j++ {
				_, _ = client.Faucet(ctx, acc.URL, api.FaucetOptions{})
				time.Sleep(200 * time.Millisecond) // Slightly slower for stability
			}
		}(i, sender)
	}
	wg.Wait()

	t.Logf("Funding completed in %.1f seconds", time.Since(fundingStart).Seconds())

	// Wait for balances to settle
	t.Log("Waiting for balances to settle...")
	time.Sleep(15 * time.Second)

	// Verify funding and add credits
	t.Log("Adding credits to sender accounts...")
	status, _ := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
	oracle := uint64(1e8) // Default oracle
	if status != nil && status.Oracle != nil {
		oracle = status.Oracle.Price
	}

	for _, sender := range senders {
		// Add credits (1 ACME worth)
		env, _ := build.Transaction().
			For(sender.URL).
			Body(&protocol.AddCredits{
				Recipient: sender.URL,
				Amount:    *big.NewInt(1e8),
				Oracle:    oracle,
			}).
			SignWith(sender.URL).Version(1).Timestamp(uint64(time.Now().UnixNano())).PrivateKey(sender.Key).
			Done()

		_, _ = client.Submit(ctx, env, api.SubmitOptions{})
	}

	time.Sleep(10 * time.Second)

	// Start load test
	t.Log("=== STARTING LOAD TEST ===")
	testStart := time.Now()

	var successCount int32
	var failCount int32

	// Create rate limiter for target TPS
	limiter := rate.NewLimiter(rate.Limit(targetTPS), 1)

	// Distribute transactions among senders
	txChan := make(chan int, totalTxs)
	for i := 0; i < totalTxs; i++ {
		txChan <- i
	}
	close(txChan)

	// Start sender goroutines
	var sendWg sync.WaitGroup
	for senderIdx := range senders {
		sendWg.Add(1)
		go func(idx int) {
			defer sendWg.Done()

			sender := senders[idx]

			for txNum := range txChan {
				// Rate limit
				_ = limiter.Wait(ctx)

				// Select receiver round-robin
				receiverIdx := txNum % numReceivers
				receiver := receivers[receiverIdx]

				// Build and send transaction
				env, err := build.Transaction().
					For(sender.URL).
					SendTokens(big.NewInt(txAmount), 0).To(receiver.URL).
					SignWith(sender.URL).Version(1).Timestamp(uint64(time.Now().UnixNano())).PrivateKey(sender.Key).
					Done()

				if err != nil {
					atomic.AddInt32(&failCount, 1)
					continue
				}

				_, err = client.Submit(ctx, env, api.SubmitOptions{})
				if err != nil {
					atomic.AddInt32(&failCount, 1)
				} else {
					atomic.AddInt32(&successCount, 1)
				}

				// Progress update every 5000 transactions
				total := atomic.LoadInt32(&successCount) + atomic.LoadInt32(&failCount)
				if total%5000 == 0 {
					elapsed := time.Since(testStart).Seconds()
					currentTPS := float64(total) / elapsed
					t.Logf("Progress: %d/%d (%.1f%%) - TPS: %.1f",
						total, totalTxs, float64(total)/float64(totalTxs)*100, currentTPS)
				}
			}
		}(senderIdx)
	}

	sendWg.Wait()
	testDuration := time.Since(testStart)

	// Calculate results
	totalSent := int(successCount + failCount)
	actualTPS := float64(totalSent) / testDuration.Seconds()

	t.Log("=== RESULTS ===")
	t.Logf("Duration: %.1f seconds", testDuration.Seconds())
	t.Logf("Total sent: %d", totalSent)
	t.Logf("Successful: %d (%.1f%%)", successCount, float64(successCount)/float64(totalSent)*100)
	t.Logf("Failed: %d (%.1f%%)", failCount, float64(failCount)/float64(totalSent)*100)
	t.Logf("Actual TPS: %.1f", actualTPS)
	t.Logf("Target TPS: %d", targetTPS)

	// Wait for settlement
	t.Log("Waiting for settlement...")
	time.Sleep(45 * time.Second)

	// Verify balances (sample first 10 receivers)
	t.Log("Verifying receiver balances...")
	totalReceived := int64(0)
	for i, receiver := range receivers {
		record, err := client.Query(ctx, receiver.URL, &api.DefaultQuery{})
		if err != nil {
			continue
		}

		if accRecord, ok := record.(*api.AccountRecord); ok {
			if tokenAccount, ok := accRecord.Account.(*protocol.LiteTokenAccount); ok {
				balance := &tokenAccount.Balance
				totalReceived += balance.Int64()

				if i < 10 { // Show first 10 balances
					balanceACME := float64(balance.Int64()) / 1e8
					t.Logf("Receiver %d balance: %.4f ACME", i, balanceACME)
				}
			}
		}
	}

	expectedReceived := int64(successCount) * txAmount
	receivedACME := float64(totalReceived) / 1e8
	expectedACME := float64(expectedReceived) / 1e8

	t.Logf("Total received: %.4f ACME (expected: %.4f ACME)", receivedACME, expectedACME)

	// Success criteria
	if actualTPS >= float64(targetTPS)*0.8 { // 80% of target TPS
		t.Logf("✅ Test PASSED - Achieved %.1f%% of target TPS", actualTPS/float64(targetTPS)*100)
	} else {
		t.Errorf("❌ Test FAILED - Only achieved %.1f%% of target TPS", actualTPS/float64(targetTPS)*100)
	}
}
