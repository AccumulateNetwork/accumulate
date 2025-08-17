//go:build !testnet
// +build !testnet

package load_test

import (
	"flag"
	"fmt"
	"math/big"
	"testing"
	"time"
)

// formatCredits formats credits with thousands separators
func formatCredits(credits uint64) string {
	if credits < 1000 {
		return fmt.Sprintf("%d credits", credits)
	}
	// Add thousands separators
	str := fmt.Sprintf("%d", credits)
	var result []byte
	for i, digit := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(digit))
	}
	return fmt.Sprintf("%s credits", result)
}

// Command-line flags for test configuration
var (
	flagTxs        = flag.Int("txs", 1000, "Number of transactions to send")
	flagK          = flag.Int("k", 10, "Number of sender (K) accounts")
	flagA          = flag.Int("a", 10, "Number of receiver (A) accounts")
	flagBatchDelay = flag.Duration("batch-delay", 0, "Delay after every 1000 transactions (e.g., 100ms)")
)

func TestStreamlinedLoadWithFlags(t *testing.T) {
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
	acmePerK := int64(txsPerK) * int64(0.001*1e8) + int64(0.5*1e8) // 0.5 ACME buffer
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
	
	t.Logf("Configuration: %d txs, %d senders, %d receivers", *flagTxs, *flagK, *flagA)
	t.Logf("Per sender: %d txs, %.2f ACME, %.2f ACME credits", txsPerK, float64(acmePerK)/1e8, float64(creditsPerK)/1e8)
	
	// Store batch delay in context for use during load generation
	if *flagBatchDelay > 0 {
		t.Logf("Batch delay: %v after every 1000 transactions", *flagBatchDelay)
	}
	
	runFullLoadTestWithBatchDelay(t, config, *flagBatchDelay)
}

func runFullLoadTestWithBatchDelay(t *testing.T, config LoadConfig, batchDelay time.Duration) {
	ctx := NewLoadTestContext(config)
	if ctx == nil {
		t.Skip("Could not initialize test context")
	}
	
	// Store batch delay in context (we'll need to add this field)
	// For now, we'll pass it through to the load generation
	
	t.Log("=== SETUP PHASE ===")
	
	ctx.CreateAllAccounts()
	t.Logf("Created %d sender accounts and %d receiver accounts", config.NumSenders, config.NumReceivers)
	
	totalACME := GetRequiredFunding(config)
	t.Logf("Funding account with %.2f ACME", float64(totalACME)/1e8)
	
	if err := ctx.FundFundingAccount(totalACME); err != nil {
		t.Fatalf("Failed to fund funding account: %v", err)
	}
	
	t.Log("Adding credits to funding account")
	// Always use a reasonable amount for funding account credits
	creditAmount := int64(1 * 1e8) // 1 ACME worth of credits for funding account
	if err := ctx.AddCredits(ctx.FundingAcct, ctx.FundingAcct, creditAmount); err != nil {
		t.Fatalf("Failed to add credits to funding account: %v", err)
	}
	
	time.Sleep(GetSettlementWait())
	
	// Verify credits were added
	credits := ctx.GetCreditsBalance(ctx.FundingAcct.URL.WithQuery("",).Identity())
	t.Logf("Funding account credits: %s", formatCredits(credits))
	if credits == 0 {
		t.Fatal("Funding account has no credits")
	}
	
	t.Logf("Distributing %.2f ACME to each K account", float64(config.ACMEPerK)/1e8)
	if err := ctx.DistributeToK(config.ACMEPerK); err != nil {
		t.Fatalf("Failed to distribute ACME to K accounts: %v", err)
	}
	
	// Wait for transactions to settle before checking balances
	time.Sleep(GetSettlementWait())
	
	// Check funding account balance to see if it was debited
	fundingBalance, _ := ctx.GetBalance(ctx.FundingAcct.URL)
	if fundingBalance != nil {
		t.Logf("Funding account balance after distribution: %.2f ACME", float64(fundingBalance.Int64())/1e8)
	}
	
	t.Log("Waiting for K accounts to receive ACME")
	if err := ctx.WaitForACME(ctx.KAccounts, config.ACMEPerK); err != nil {
		t.Fatalf("K accounts did not receive ACME: %v", err)
	}
	
	t.Logf("Adding %.4f ACME worth of credits to K accounts", float64(config.CreditsPerK)/1e8)
	if err := ctx.AddCreditsToK(config.CreditsPerK); err != nil {
		t.Fatalf("Failed to add credits to K accounts: %v", err)
	}
	
	t.Log("Waiting for credits to settle")
	expectedCredits := CalculateCredits(config.CreditsPerK, ctx.Oracle)
	// In debug mode, just check for any credits
	minCredits := expectedCredits / 2
	if DEBUG_MODE {
		minCredits = 1 // Just check for any credits
	}
	if err := ctx.WaitForCredits(ctx.KAccounts, uint64(minCredits)); err != nil {
		t.Fatalf("K accounts did not receive credits: %v", err)
	}
	
	t.Log("=== LOAD TEST PHASE ===")
	t.Logf("Sending %d transactions", config.NumTxs)
	
	// Start timing for end-to-end TPS (including settlement)
	endToEndStart := time.Now()
	
	// Generate load with batch delay support
	results, err := GenerateLoadWithBatchDelay(ctx, batchDelay)
	if err != nil {
		t.Fatalf("Failed to generate load: %v", err)
	}
	
	t.Logf("Sent %d transactions in %v (%.2f send TPS)", results.TotalSent, results.Duration, results.TPS)
	t.Logf("Success: %d, Failed: %d", results.TotalSuccess, results.TotalFailed)
	
	t.Log("=== VERIFICATION PHASE ===")
	t.Log("Waiting for settlement...")
	
	verified := false
	
	// Determine max time based on transaction count
	var maxTotalTime time.Duration
	switch {
	case config.NumTxs >= 10000:
		maxTotalTime = 10 * time.Minute
	case config.NumTxs >= 1000:
		maxTotalTime = 5 * time.Minute
	default:
		maxTotalTime = 3 * time.Minute
	}
	
	// Track previous balances to detect progress
	prevSenderBalances := make([]*big.Int, len(ctx.KAccounts))
	prevReceiverBalances := make([]*big.Int, len(ctx.AAccounts))
	
	// Initial timeout and retry settings
	noProgressTimeout := 10 // seconds without progress before giving up
	lastProgressTime := time.Now()
	deadline := time.Now().Add(maxTotalTime)
	
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
			t.Log("Settlement progress detected, resetting timeout")
		}
		
		// Check if we've been stuck without progress
		if time.Since(lastProgressTime) > time.Duration(noProgressTimeout)*time.Second {
			t.Logf("No settlement progress for %d seconds, stopping verification", noProgressTimeout)
			break
		}
		
		if allCorrect {
			verified = true
			break
		}
	}
	
	// Calculate end-to-end TPS including settlement time
	endToEndDuration := time.Since(endToEndStart)
	endToEndTPS := float64(results.TotalSuccess) / endToEndDuration.Seconds()
	
	t.Logf("\n=== TIMING SUMMARY ===")
	t.Logf("Send phase: %v (%.2f TPS)", results.Duration, results.TPS)
	t.Logf("Settlement phase: %v", endToEndDuration - results.Duration)
	t.Logf("Total end-to-end: %v (%.2f TPS including settlement)", endToEndDuration, endToEndTPS)
	
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

// GenerateLoadWithBatchDelay generates load with optional batch delays
func GenerateLoadWithBatchDelay(ctx *LoadTestContext, batchDelay time.Duration) (*LoadResults, error) {
	results := &LoadResults{}
	start := time.Now()
	
	var successCount int32
	var failCount int32
	
	txPerSender := ctx.Config.NumTxs / ctx.Config.NumSenders
	remainder := ctx.Config.NumTxs % ctx.Config.NumSenders
	
	// Track total transactions sent for batch delay
	totalSent := 0
	
	for senderIdx := 0; senderIdx < ctx.Config.NumSenders; senderIdx++ {
		txCount := txPerSender
		if senderIdx < remainder {
			txCount++
		}
		
		for i := 0; i < txCount; i++ {
			receiverIdx := (totalSent) % ctx.Config.NumReceivers
			err := ctx.SendTransaction(
				ctx.KAccounts[senderIdx],
				ctx.AAccounts[receiverIdx],
				ctx.Config.TxAmount,
			)
			
			if err == nil {
				successCount++
			} else {
				failCount++
			}
			
			totalSent++
			
			// Apply batch delay if configured
			if batchDelay > 0 && totalSent%1000 == 0 && totalSent < ctx.Config.NumTxs {
				time.Sleep(batchDelay)
			}
		}
	}
	
	results.Duration = time.Since(start)
	results.TotalSent = ctx.Config.NumTxs
	results.TotalSuccess = int(successCount)
	results.TotalFailed = int(failCount)
	results.TPS = float64(results.TotalSuccess) / results.Duration.Seconds()
	
	return results, nil
}