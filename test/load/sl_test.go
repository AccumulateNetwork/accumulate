//go:build !testnet
// +build !testnet

package load_test

import (
	"math/big"
	"testing"
	"time"
)

func TestStreamlinedLoad(t *testing.T) {
	var config LoadConfig
	if DEBUG_MODE {
		// Debug configuration with multiple senders for round-robin testing
		config = LoadConfig{
			NumSenders:   10,           // 10 K accounts for round-robin
			NumReceivers: 5,            // 5 A accounts as receivers
			NumTxs:       20000,        // 20,000 transactions total
			TxAmount:     0.001 * 1e8,  // 0.001 ACME per transaction
			ACMEPerK:     5 * 1e8,      // 5 ACME per sender (enough for 2000 txs per sender with buffer)
			CreditsPerK:  1 * 1e8,      // 1 ACME worth of credits per sender for many txs
		}
	} else {
		config = LoadConfig{
			NumSenders:   3,
			NumReceivers: 3,
			NumTxs:       100,
			TxAmount:     0.001 * 1e8,
			ACMEPerK:     100 * 1e8,
			CreditsPerK:  10 * 1e8,
		}
	}
	
	runFullLoadTest(t, config)
}

func TestCreditsFlow(t *testing.T) {
	config := LoadConfig{
		NumSenders:   1,
		NumReceivers: 1,
		NumTxs:       10,
		TxAmount:     0.001 * 1e8,
		ACMEPerK:     100 * 1e8,
		CreditsPerK:  10 * 1e8,
	}
	
	runCreditsTest(t, config)
}

func TestSimpleLoad(t *testing.T) {
	config := LoadConfig{
		NumSenders:   2,
		NumReceivers: 2,
		NumTxs:       20,
		TxAmount:     0.001 * 1e8,
		ACMEPerK:     100 * 1e8,
		CreditsPerK:  10 * 1e8,
	}
	
	runSimpleTest(t, config)
}

func TestLoadWithFailures(t *testing.T) {
	config := LoadConfig{
		NumSenders:   5,
		NumReceivers: 5,
		NumTxs:       1000,
		TxAmount:     0.001 * 1e8,
		ACMEPerK:     100 * 1e8,
		CreditsPerK:  10 * 1e8,
	}
	
	runFullLoadTest(t, config)
}

func runFullLoadTest(t *testing.T, config LoadConfig) {
	ctx := NewLoadTestContext(config)
	if ctx == nil {
		t.Skip("Could not initialize test context")
	}
	
	t.Log("=== SETUP PHASE ===")
	
	ctx.CreateAllAccounts()
	t.Logf("Created %d sender accounts and %d receiver accounts", config.NumSenders, config.NumReceivers)
	
	totalACME := GetRequiredFunding(config)
	t.Logf("Funding account with %.2f ACME", float64(totalACME)/1e8)
	
	if err := ctx.FundFundingAccount(totalACME); err != nil {
		t.Fatalf("Failed to fund funding account: %v", err)
	}
	
	t.Log("Adding credits to funding account")
	creditAmount := int64(DEBUG_CREDITS_AMOUNT)
	if !DEBUG_MODE {
		creditAmount = 10 * 1e8
	}
	if err := ctx.AddCredits(ctx.FundingAcct, ctx.FundingAcct, creditAmount); err != nil {
		t.Fatalf("Failed to add credits to funding account: %v", err)
	}
	
	time.Sleep(GetSettlementWait())
	
	// Verify credits were added
	credits := ctx.GetCreditsBalance(ctx.FundingAcct.URL.WithQuery("",).Identity())
	t.Logf("Funding account credits: %d credits", credits)
	if credits == 0 {
		t.Fatal("Funding account has no credits")
	}
	
	t.Logf("Distributing %d ACME to each K account", config.ACMEPerK/1e8)
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
	
	results, err := ctx.GenerateLoad()
	if err != nil {
		t.Fatalf("Failed to generate load: %v", err)
	}
	
	t.Logf("Sent %d transactions in %v (%.2f TPS)", results.TotalSent, results.Duration, results.TPS)
	t.Logf("Success: %d, Failed: %d", results.TotalSuccess, results.TotalFailed)
	
	t.Log("=== VERIFICATION PHASE ===")
	t.Log("Waiting for settlement...")
	
	verified := false
	// Track previous balances to detect progress
	prevSenderBalances := make([]*big.Int, len(ctx.KAccounts))
	prevReceiverBalances := make([]*big.Int, len(ctx.AAccounts))
	
	// Initial timeout and retry settings
	noProgressTimeout := 10 // seconds without progress before giving up
	lastProgressTime := time.Now()
	maxTotalTime := 3 * time.Minute
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

func runCreditsTest(t *testing.T, config LoadConfig) {
	ctx := NewLoadTestContext(config)
	if ctx == nil {
		t.Skip("Could not initialize test context")
	}
	
	ctx.CreateAllAccounts()
	
	if err := ctx.FundFundingAccount(200*1e8); err != nil {
		t.Fatalf("Failed to fund account: %v", err)
	}
	
	initialCredits := ctx.GetCreditsBalance(ctx.FundingAcct.URL.WithQuery("",).Identity())
	t.Logf("Initial credits: %d credits", initialCredits)
	
	if err := ctx.AddCredits(ctx.FundingAcct, ctx.FundingAcct, 10*1e8); err != nil {
		t.Fatalf("Failed to add credits: %v", err)
	}
	
	time.Sleep(GetSettlementWait())
	
	finalCredits := ctx.GetCreditsBalance(ctx.FundingAcct.URL.WithQuery("",).Identity())
	t.Logf("Final credits: %d credits", finalCredits)
	
	if finalCredits <= initialCredits {
		t.Fatal("Credits were not added")
	}
	
	expectedCredits := CalculateCredits(10*1e8, ctx.Oracle)
	t.Logf("Expected credits from 10 ACME: %d credits", expectedCredits)
	
	if finalCredits < initialCredits+expectedCredits/2 {
		t.Fatal("Credits calculation appears incorrect")
	}
}

func runSimpleTest(t *testing.T, config LoadConfig) {
	ctx := NewLoadTestContext(config)
	if ctx == nil {
		t.Skip("Could not initialize test context")
	}
	
	ctx.CreateAllAccounts()
	
	totalACME := GetRequiredFunding(config)
	if err := ctx.FundFundingAccount(totalACME); err != nil {
		t.Fatalf("Failed to fund: %v", err)
	}
	
	if err := ctx.DistributeToK(config.ACMEPerK); err != nil {
		t.Fatalf("Failed to distribute: %v", err)
	}
	
	if err := ctx.WaitForACME(ctx.KAccounts, config.ACMEPerK); err != nil {
		t.Fatalf("Failed to verify K accounts: %v", err)
	}
	
	if err := ctx.AddCreditsToK(config.CreditsPerK); err != nil {
		t.Fatalf("Failed to add credits: %v", err)
	}
	
	time.Sleep(GetSettlementWait())
	
	for i := 0; i < config.NumTxs; i++ {
		senderIdx := i % config.NumSenders
		receiverIdx := i % config.NumReceivers
		
		err := ctx.SendTransaction(
			ctx.KAccounts[senderIdx],
			ctx.AAccounts[receiverIdx],
			config.TxAmount,
		)
		
		if err != nil {
			t.Logf("Transaction %d failed: %v", i, err)
		}
	}
	
	time.Sleep(GetSettlementWait())
	
	for i, account := range ctx.AAccounts {
		balance, _ := ctx.GetBalance(account.URL)
		t.Logf("Receiver a%d balance: %.4f ACME", i+1, float64(balance.Int64())/1e8)
	}
}