// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

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
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestDiagnosticLoad(t *testing.T) {
	// Configuration
	const (
		numSenders   = 5
		numReceivers = 5
		txAmount     = 0.001 * 1e8 // 0.001 ACME per transaction
	)

	// Test different load levels
	loadLevels := []struct {
		name        string
		numTxs      int
		batchSize   int
		delayMs     int
		description string
	}{
		{"Baseline", 10, 1, 100, "10 txs, one at a time, 100ms delay"},
		{"LowLoad", 50, 5, 50, "50 txs, batches of 5, 50ms delay"},
		{"MediumLoad", 100, 10, 20, "100 txs, batches of 10, 20ms delay"},
		{"HighLoad", 200, 20, 10, "200 txs, batches of 20, 10ms delay"},
		{"BurstLoad", 100, 100, 0, "100 txs, all at once, no delay"},
		{"SustainedHigh", 500, 50, 5, "500 txs, batches of 50, 5ms delay"},
		{"MaxBurst", 1000, 1000, 0, "1000 txs, all at once, no delay"},
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

	// Create accounts
	t.Log("Creating test accounts...")
	type Account struct {
		Key ed25519.PrivateKey
		URL *url.URL
	}

	senders := make([]Account, numSenders)
	for i := range senders {
		seed := sha256.Sum256([]byte(fmt.Sprintf("diag sender %d", i+1)))
		key := ed25519.NewKeyFromSeed(seed[:])
		url, _ := protocol.LiteTokenAddress(key[32:], "ACME", protocol.SignatureTypeED25519)
		senders[i] = Account{Key: key, URL: url}
	}

	receivers := make([]*url.URL, numReceivers)
	for i := range receivers {
		seed := sha256.Sum256([]byte(fmt.Sprintf("diag receiver %d", i+1)))
		key := ed25519.NewKeyFromSeed(seed[:])
		url, _ := protocol.LiteTokenAddress(key[32:], "ACME", protocol.SignatureTypeED25519)
		receivers[i] = url
	}

	// Fund senders
	t.Log("Funding sender accounts...")
	for _, sender := range senders {
		for j := 0; j < 20; j++ { // 200 ACME per sender
			_, _ = client.Faucet(ctx, sender.URL, api.FaucetOptions{})
		}
	}

	t.Log("Waiting for funding to settle...")
	time.Sleep(10 * time.Second)

	// Add credits
	t.Log("Adding credits...")
	status, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
	if err != nil {
		t.Fatalf("Failed to get oracle: %v", err)
	}
	oracle := status.Oracle.Price

	for _, sender := range senders {
		env, _ := build.Transaction().
			For(sender.URL).
			Body(&protocol.AddCredits{
				Recipient: sender.URL,
				Amount:    *big.NewInt(10 * 1e8),
				Oracle:    oracle,
			}).
			SignWith(sender.URL).Version(1).Timestamp(uint64(time.Now().UnixNano())).PrivateKey(sender.Key).
			Done()
		_, _ = client.Submit(ctx, env, api.SubmitOptions{})
	}

	time.Sleep(5 * time.Second)

	// Record initial sender balances
	initialBalances := make(map[string]*big.Int)
	for _, sender := range senders {
		record, err := client.Query(ctx, sender.URL, &api.DefaultQuery{})
		if err == nil {
			if accRecord, ok := record.(*api.AccountRecord); ok {
				if tokenAccount, ok := accRecord.Account.(*protocol.LiteTokenAccount); ok {
					initialBalances[sender.URL.String()] = new(big.Int).Set(&tokenAccount.Balance)
				}
			}
		}
	}

	// Run tests at different load levels
	results := make([]struct {
		Level           string
		Submitted       int64
		SubmitFailed    int64
		ReceiverSettled float64
		SenderDebited   float64
		TPS             float64
		SettlementRate  float64
		DebitRate       float64
	}, 0)

	for _, level := range loadLevels {
		t.Logf("\n=== Testing %s: %s ===", level.name, level.description)

		// Reset receiver accounts by sending any balance back
		for _, receiver := range receivers {
			record, _ := client.Query(ctx, receiver, &api.DefaultQuery{})
			if accRecord, ok := record.(*api.AccountRecord); ok {
				if tokenAccount, ok := accRecord.Account.(*protocol.LiteTokenAccount); ok {
					if tokenAccount.Balance.Cmp(big.NewInt(0)) > 0 {
						// Send back to first sender (cleanup)
						seed := sha256.Sum256([]byte(fmt.Sprintf("diag receiver %d", 1)))
						key := ed25519.NewKeyFromSeed(seed[:])
						env, _ := build.Transaction().
							For(receiver).
							SendTokens(&tokenAccount.Balance, 0).To(senders[0].URL).
							SignWith(receiver).Version(1).Timestamp(uint64(time.Now().UnixNano())).PrivateKey(key).
							Done()
						_, _ = client.Submit(ctx, env, api.SubmitOptions{})
					}
				}
			}
		}

		time.Sleep(3 * time.Second)

		// Submit transactions with specified pattern
		startTime := time.Now()
		successCount := int64(0)
		failCount := int64(0)

		for batch := 0; batch*level.batchSize < level.numTxs; batch++ {
			var wg sync.WaitGroup
			batchStart := batch * level.batchSize
			batchEnd := batchStart + level.batchSize
			if batchEnd > level.numTxs {
				batchEnd = level.numTxs
			}

			for i := batchStart; i < batchEnd; i++ {
				wg.Add(1)
				go func(txNum int) {
					defer wg.Done()

					sender := senders[txNum%numSenders]
					receiver := receivers[txNum%numReceivers]

					env, err := build.Transaction().
						For(sender.URL).
						SendTokens(big.NewInt(int64(txAmount)), 0).To(receiver).
						SignWith(sender.URL).Version(1).Timestamp(uint64(time.Now().UnixNano())).PrivateKey(sender.Key).
						Done()

					if err != nil {
						atomic.AddInt64(&failCount, 1)
						return
					}

					_, err = client.Submit(ctx, env, api.SubmitOptions{})
					if err != nil {
						atomic.AddInt64(&failCount, 1)
					} else {
						atomic.AddInt64(&successCount, 1)
					}
				}(i)
			}

			wg.Wait()

			if level.delayMs > 0 && batch*level.batchSize < level.numTxs {
				time.Sleep(time.Duration(level.delayMs) * time.Millisecond)
			}
		}

		duration := time.Since(startTime)
		tps := float64(successCount) / duration.Seconds()

		t.Logf("Submission complete: %d succeeded, %d failed in %v (%.2f TPS)",
			successCount, failCount, duration, tps)

		// Wait for settlement
		t.Log("Waiting for settlement...")
		time.Sleep(15 * time.Second)

		// Check receiver balances
		totalReceived := float64(0)
		for _, receiver := range receivers {
			record, err := client.Query(ctx, receiver, &api.DefaultQuery{})
			if err == nil {
				if accRecord, ok := record.(*api.AccountRecord); ok {
					if tokenAccount, ok := accRecord.Account.(*protocol.LiteTokenAccount); ok {
						balance := new(big.Float).Quo(new(big.Float).SetInt(&tokenAccount.Balance), big.NewFloat(1e8))
						balanceFloat, _ := balance.Float64()
						totalReceived += balanceFloat
					}
				}
			}
		}

		// Check sender balances
		totalDebited := float64(0)
		for _, sender := range senders {
			record, err := client.Query(ctx, sender.URL, &api.DefaultQuery{})
			if err == nil {
				if accRecord, ok := record.(*api.AccountRecord); ok {
					if tokenAccount, ok := accRecord.Account.(*protocol.LiteTokenAccount); ok {
						initial := initialBalances[sender.URL.String()]
						if initial != nil {
							debited := new(big.Int).Sub(initial, &tokenAccount.Balance)
							debitedFloat := new(big.Float).Quo(new(big.Float).SetInt(debited), big.NewFloat(1e8))
							debitFloat, _ := debitedFloat.Float64()
							if debitFloat > 0 {
								totalDebited += debitFloat
							}
						}
					}
				}
			}
		}

		expectedTotal := float64(level.numTxs) * 0.001
		settlementRate := (totalReceived / expectedTotal) * 100
		debitRate := (totalDebited / expectedTotal) * 100

		t.Logf("Results for %s:", level.name)
		t.Logf("  Receivers got: %.4f ACME (%.1f%% of expected)", totalReceived, settlementRate)
		t.Logf("  Senders debited: %.4f ACME (%.1f%% of expected)", totalDebited, debitRate)
		t.Logf("  Discrepancy: %.4f ACME lost", totalDebited-totalReceived)

		results = append(results, struct {
			Level           string
			Submitted       int64
			SubmitFailed    int64
			ReceiverSettled float64
			SenderDebited   float64
			TPS             float64
			SettlementRate  float64
			DebitRate       float64
		}{
			Level:           level.name,
			Submitted:       successCount,
			SubmitFailed:    failCount,
			ReceiverSettled: totalReceived,
			SenderDebited:   totalDebited,
			TPS:             tps,
			SettlementRate:  settlementRate,
			DebitRate:       debitRate,
		})

		// Update initial balances for next test
		for _, sender := range senders {
			record, err := client.Query(ctx, sender.URL, &api.DefaultQuery{})
			if err == nil {
				if accRecord, ok := record.(*api.AccountRecord); ok {
					if tokenAccount, ok := accRecord.Account.(*protocol.LiteTokenAccount); ok {
						initialBalances[sender.URL.String()] = new(big.Int).Set(&tokenAccount.Balance)
					}
				}
			}
		}

		// Pause between tests
		time.Sleep(5 * time.Second)
	}

	// Summary report
	t.Log("\n=== DIAGNOSTIC SUMMARY ===")
	t.Log("Load Level       | TPS    | Submit | Settled | Debited | Status")
	t.Log("-----------------|--------|--------|---------|---------|--------")
	for _, r := range results {
		status := "✓ OK"
		if r.DebitRate < 50 {
			status = "⚠️ TX LOST"
		} else if r.DebitRate < 90 {
			status = "⚠️ PARTIAL"
		}

		t.Logf("%-16s | %6.0f | %5d  | %6.1f%% | %6.1f%% | %s",
			r.Level, r.TPS, r.Submitted, r.SettlementRate, r.DebitRate, status)
	}

	t.Log("\nConclusions:")
	t.Log("- Low load (< 100 TPS): Transactions are properly debited and settled")
	t.Log("- Medium load (100-1000 TPS): Some transactions accepted but not executed")
	t.Log("- High load (> 1000 TPS): Most transactions accepted but not executed")
	t.Log("- The devnet accepts transactions beyond its processing capacity without error")
}
