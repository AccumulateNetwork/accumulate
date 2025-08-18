//go:build !testnet
// +build !testnet

package load_test

import (
	"context"
	"flag"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// Command-line flags for complete test configuration
var (
	flagTxs     = flag.Int("txs", 1000, "Number of transactions to send")
	flagK       = flag.Int("k", 10, "Number of sender (K) accounts")
	flagA       = flag.Int("a", 10, "Number of receiver (A) accounts")
	flagTPS     = flag.Int("tps", 0, "Target transactions per second (0 = unlimited)")
	flagTimeout = flag.Duration("timeout", 0, "Settlement timeout (0 = auto-calculated)")
	flagVerbose = flag.Bool("verbose", false, "Enable verbose logging")
)

// TestStreamlinedLoad is the single entry point for all load testing
// All behavior is controlled via command-line flags
func TestStreamlinedLoad(t *testing.T) {
	// Parse flags if not already parsed
	if !flag.Parsed() {
		flag.Parse()
	}

	// Calculate required ACME per K account
	txsPerK := *flagTxs / *flagK
	if *flagTxs%*flagK != 0 {
		txsPerK++ // Round up for remainder
	}

	// Calculate ACME needed per K account (txs * 0.001 + buffer)
	acmePerK := int64(txsPerK)*int64(0.001*1e8) + int64(0.5*1e8) // 0.5 ACME buffer
	if acmePerK < 1*1e8 {
		acmePerK = 1 * 1e8 // Minimum 1 ACME per K account
	}

	// Calculate credits per K based on transaction count
	var creditsPerK int64
	switch {
	case txsPerK >= 5000:
		creditsPerK = 1 * 1e8 // 1 ACME worth
	case txsPerK >= 1000:
		creditsPerK = int64(0.5 * 1e8) // 0.5 ACME worth
	default:
		creditsPerK = int64(0.1 * 1e8) // 0.1 ACME worth
	}

	config := LoadConfig{
		NumSenders:   *flagK,
		NumReceivers: *flagA,
		NumTxs:       *flagTxs,
		TxAmount:     int64(0.001 * 1e8), // Fixed at 0.001 ACME per tx
		ACMEPerK:     acmePerK,
		CreditsPerK:  creditsPerK,
	}

	// Calculate timeout if not specified
	timeout := *flagTimeout
	if timeout == 0 {
		switch {
		case config.NumTxs >= 10000:
			timeout = 10 * time.Minute
		case config.NumTxs >= 1000:
			timeout = 5 * time.Minute
		default:
			timeout = 3 * time.Minute
		}
	}

	// Log configuration
	t.Logf("=== CONFIGURATION ===")
	t.Logf("Transactions: %d", *flagTxs)
	t.Logf("Senders: %d, Receivers: %d", *flagK, *flagA)
	if *flagTPS > 0 {
		t.Logf("Target TPS: %d (expected duration: %.1f seconds)", *flagTPS, float64(*flagTxs)/float64(*flagTPS))
	} else {
		t.Logf("Target TPS: unlimited (maximum speed)")
	}
	t.Logf("Timeout: %v", timeout)
	t.Logf("Per sender: %d txs, %.2f ACME, %.2f ACME credits", txsPerK, float64(acmePerK)/1e8, float64(creditsPerK)/1e8)

	// Run the test
	runLoadTest(t, config, *flagTPS, timeout, *flagVerbose)
}

func runLoadTest(t *testing.T, config LoadConfig, targetTPS int, timeout time.Duration, verbose bool) {
	ctx := NewLoadTestContext(config)
	if ctx == nil {
		t.Skip("Could not initialize test context")
	}

	t.Log("=== SETUP PHASE ===")

	// Create accounts
	ctx.CreateAllAccounts()
	if verbose {
		t.Logf("Created %d sender accounts and %d receiver accounts", config.NumSenders, config.NumReceivers)
	}

	// Fund the funding account
	totalACME := GetRequiredFunding(config)
	if verbose {
		t.Logf("Funding account with %.2f ACME", float64(totalACME)/1e8)
	}

	if err := ctx.FundFundingAccount(totalACME); err != nil {
		t.Fatalf("Failed to fund funding account: %v", err)
	}

	// Add credits to funding account
	if verbose {
		t.Log("Adding credits to funding account")
	}
	creditAmount := int64(1 * 1e8) // 1 ACME worth of credits
	if err := ctx.AddCredits(ctx.FundingAcct, ctx.FundingAcct, creditAmount); err != nil {
		t.Fatalf("Failed to add credits to funding account: %v", err)
	}

	time.Sleep(GetSettlementWait())

	// Verify credits were added
	credits := ctx.GetCreditsBalance(ctx.FundingAcct.URL.WithQuery("",).Identity())
	if verbose {
		t.Logf("Funding account credits: %d credits", credits)
	}
	if credits == 0 {
		t.Fatal("Funding account has no credits")
	}

	// Distribute ACME to senders
	if verbose {
		t.Logf("Distributing %.2f ACME to each K account", float64(config.ACMEPerK)/1e8)
	}
	if err := ctx.DistributeToK(config.ACMEPerK); err != nil {
		t.Fatalf("Failed to distribute ACME to K accounts: %v", err)
	}

	time.Sleep(GetSettlementWait())

	// Wait for K accounts to receive ACME
	if verbose {
		t.Log("Waiting for K accounts to receive ACME")
	}
	if err := ctx.WaitForACME(ctx.KAccounts, config.ACMEPerK); err != nil {
		t.Fatalf("K accounts did not receive ACME: %v", err)
	}

	// Add credits to K accounts
	if verbose {
		t.Logf("Adding %.4f ACME worth of credits to K accounts", float64(config.CreditsPerK)/1e8)
	}
	if err := ctx.AddCreditsToK(config.CreditsPerK); err != nil {
		t.Fatalf("Failed to add credits to K accounts: %v", err)
	}

	// Wait for credits to settle
	if verbose {
		t.Log("Waiting for credits to settle")
	}
	expectedCredits := CalculateCredits(config.CreditsPerK, ctx.Oracle)
	minCredits := expectedCredits / 2
	if err := ctx.WaitForCredits(ctx.KAccounts, uint64(minCredits)); err != nil {
		t.Fatalf("K accounts did not receive credits: %v", err)
	}

	t.Log("=== LOAD TEST PHASE ===")
	t.Logf("Sending %d transactions", config.NumTxs)

	// Start timing for end-to-end TPS
	endToEndStart := time.Now()

	// Generate load with optional rate limiting
	var results *LoadResults
	var err error
	if targetTPS > 0 {
		results, err = generateLoadWithRateLimit(ctx, targetTPS, verbose)
	} else {
		results, err = ctx.GenerateLoad()
	}

	if err != nil {
		t.Fatalf("Failed to generate load: %v", err)
	}

	t.Logf("Sent %d transactions in %v (%.2f send TPS)", results.TotalSent, results.Duration, results.TPS)
	t.Logf("Success: %d, Failed: %d", results.TotalSuccess, results.TotalFailed)

	t.Log("=== VERIFICATION PHASE ===")
	t.Log("Waiting for settlement...")

	// Verify balances with timeout
	verified := verifyBalances(t, ctx, config, timeout, verbose)

	// Calculate end-to-end TPS
	endToEndDuration := time.Since(endToEndStart)
	endToEndTPS := float64(results.TotalSuccess) / endToEndDuration.Seconds()

	t.Logf("\n=== TIMING SUMMARY ===")
	t.Logf("Send phase: %v (%.2f TPS)", results.Duration, results.TPS)
	t.Logf("Settlement phase: %v", endToEndDuration-results.Duration)
	t.Logf("Total end-to-end: %v (%.2f TPS including settlement)", endToEndDuration, endToEndTPS)

	// Generate report
	report := ctx.GenerateReport(results)
	t.Log(report)

	if !verified {
		issues := ctx.DetectIssues()
		t.Log("\n=== ISSUES DETECTED ===")
		for _, issue := range issues {
			t.Logf("%s: %s - %s", issue.Account, issue.Type, issue.Description)
		}
		t.Fatal("Load test verification failed")
	}

	t.Log("✅ Load test completed successfully")
}

func generateLoadWithRateLimit(ctx *LoadTestContext, targetTPS int, verbose bool) (*LoadResults, error) {
	results := &LoadResults{}
	start := time.Now()

	// Create rate limiter
	limiter := rate.NewLimiter(rate.Limit(targetTPS), targetTPS)

	var successCount int32
	var failCount int32

	txPerSender := ctx.Config.NumTxs / ctx.Config.NumSenders
	remainder := ctx.Config.NumTxs % ctx.Config.NumSenders

	// Use goroutines for concurrent sending with rate limiting
	var wg sync.WaitGroup
	txChan := make(chan struct {
		sender   LiteAccount
		receiver LiteAccount
	}, ctx.Config.NumTxs)

	// Fill the channel with transactions to send
	totalTxs := 0
	for senderIdx := 0; senderIdx < ctx.Config.NumSenders; senderIdx++ {
		txCount := txPerSender
		if senderIdx < remainder {
			txCount++
		}

		for i := 0; i < txCount; i++ {
			receiverIdx := totalTxs % ctx.Config.NumReceivers
			txChan <- struct {
				sender   LiteAccount
				receiver LiteAccount
			}{
				sender:   ctx.KAccounts[senderIdx],
				receiver: ctx.AAccounts[receiverIdx],
			}
			totalTxs++
		}
	}
	close(txChan)

	// Start worker goroutines
	numWorkers := 10
	if targetTPS < 10 {
		numWorkers = 1
	}

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for tx := range txChan {
				// Wait for rate limiter token
				if err := limiter.Wait(context.Background()); err != nil {
					atomic.AddInt32(&failCount, 1)
					if verbose {
						fmt.Printf("Rate limiter error: %v\n", err)
					}
					continue
				}

				err := ctx.SendTransaction(
					tx.sender,
					tx.receiver,
					ctx.Config.TxAmount,
				)

				if err == nil {
					atomic.AddInt32(&successCount, 1)
				} else {
					atomic.AddInt32(&failCount, 1)
					if verbose {
						fmt.Printf("Transaction failed: %v\n", err)
					}
				}
			}
		}()
	}

	wg.Wait()

	results.Duration = time.Since(start)
	results.TotalSent = ctx.Config.NumTxs
	results.TotalSuccess = int(successCount)
	results.TotalFailed = int(failCount)
	results.TPS = float64(results.TotalSuccess) / results.Duration.Seconds()

	return results, nil
}

func verifyBalances(t *testing.T, ctx *LoadTestContext, config LoadConfig, timeout time.Duration, verbose bool) bool {
	// Track previous balances to detect progress
	prevSenderBalances := make([]*big.Int, len(ctx.KAccounts))
	prevReceiverBalances := make([]*big.Int, len(ctx.AAccounts))

	noProgressTimeout := 10 * time.Second
	lastProgressTime := time.Now()
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)

		progressMade := false
		allCorrect := true
		txPerSender := config.NumTxs / config.NumSenders
		remainder := config.NumTxs % config.NumSenders

		// Check sender balances
		for i, account := range ctx.KAccounts {
			txCount := txPerSender
			if i < remainder {
				txCount++
			}

			expectedBalance := big.NewInt(config.ACMEPerK - int64(txCount)*config.TxAmount)
			balance, err := ctx.GetBalance(account.URL)
			if err != nil {
				allCorrect = false
				continue
			}

			// Check if balance changed
			if prevSenderBalances[i] == nil || balance.Cmp(prevSenderBalances[i]) != 0 {
				progressMade = true
				prevSenderBalances[i] = new(big.Int).Set(balance)
			}

			diff := new(big.Int).Sub(balance, expectedBalance)
			diff.Abs(diff)

			if diff.Cmp(big.NewInt(1e4)) > 0 {
				allCorrect = false
			}
		}

		// Check receiver balances
		txPerReceiver := make([]int, config.NumReceivers)
		for i := 0; i < config.NumTxs; i++ {
			receiverIdx := i % config.NumReceivers
			txPerReceiver[receiverIdx]++
		}

		for i, account := range ctx.AAccounts {
			expectedBalance := big.NewInt(int64(txPerReceiver[i]) * config.TxAmount)
			balance, err := ctx.GetBalance(account.URL)
			if err != nil {
				// Account might not exist yet
				if expectedBalance.Cmp(big.NewInt(0)) > 0 {
					allCorrect = false
				}
				continue
			}

			// Check if balance changed
			if prevReceiverBalances[i] == nil || balance.Cmp(prevReceiverBalances[i]) != 0 {
				progressMade = true
				prevReceiverBalances[i] = new(big.Int).Set(balance)
			}

			diff := new(big.Int).Sub(balance, expectedBalance)
			diff.Abs(diff)

			if diff.Cmp(big.NewInt(1e4)) > 0 {
				allCorrect = false
			}
		}

		// Reset timeout if progress was made
		if progressMade {
			lastProgressTime = time.Now()
			if verbose {
				t.Log("Settlement progress detected, resetting timeout")
			}
		}

		// Check if we've been stuck without progress
		if time.Since(lastProgressTime) > noProgressTimeout {
			if verbose {
				t.Logf("No settlement progress for %v, stopping verification", noProgressTimeout)
			}
			break
		}

		if allCorrect {
			return true
		}
	}

	return false
}