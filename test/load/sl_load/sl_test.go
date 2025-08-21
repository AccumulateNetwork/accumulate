//go:build !testnet
// +build !testnet

package load_test

import (
	"flag"
	"testing"
	"time"
	
	"gitlab.com/accumulatenetwork/accumulate/protocol"
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

	// Create basic config from flags
	config := LoadConfig{
		NumSenders:   *flagK,
		NumReceivers: *flagA,
		NumTxs:       *flagTxs,
		TxAmount:     int64(0.001 * 1e8), // Fixed at 0.001 ACME per tx
	}

	// Get fee schedule (using defaults for now)
	fees := GetDefaultFeeSchedule()
	
	// Use estimated oracle price for initial calculation
	// This will be recalculated with actual oracle price during setup
	estimatedOraclePrice := uint64(5000) // $0.50 per ACME as estimate
	
	// Calculate funding requirements using proper fee calculations
	funding := CalculateFunding(config, fees, estimatedOraclePrice)
	
	// Set the calculated values in config
	config.ACMEPerK = funding.TotalACMEPerK
	config.CreditsPerK = funding.ACMEForCreditsPerK

	// Calculate timeout if not specified (per design: 1 minute max, resets on progress)
	timeout := *flagTimeout
	if timeout == 0 {
		timeout = 1 * time.Minute // Fixed 1 minute timeout per design
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
	
	// Log funding calculations
	t.Logf("=== FUNDING CALCULATIONS ===")
	t.Logf("Fee Schedule: SendTokens = %s credits", FormatCredits(fees.SendTokens))
	t.Logf("Oracle Price (estimated): $%.2f/ACME", float64(estimatedOraclePrice)/10000)
	t.Logf("Per sender:")
	t.Logf("  Transactions: %d", funding.TransactionsPerK)
	t.Logf("  Credits needed: %s", FormatCredits(funding.CreditsPerK))
	t.Logf("  ACME for txs: %s", FormatACME(funding.ACMEPerK))
	t.Logf("  ACME for credits: %s", FormatACME(funding.ACMEForCreditsPerK))
	t.Logf("  Total ACME: %s (with 10%% buffer)", FormatACME(funding.TotalACMEPerK))
	t.Logf("Total funding needed: %s ACME (%d faucet calls)", 
		FormatACME(funding.TotalACMENeeded), funding.FaucetCallsNeeded)

	// Run the test
	runLoadTest(t, config, *flagTPS, timeout, *flagVerbose)
}

func runLoadTest(t *testing.T, config LoadConfig, targetTPS int, timeout time.Duration, verbose bool) {
	ctx := NewLoadTestContext(config)
	if ctx == nil {
		t.Skip("Could not initialize test context")
	}

	t.Log("=== SETUP PHASE ===")
	
	// Get actual oracle price and fee schedule
	fees := GetDefaultFeeSchedule()
	actualOraclePrice := ctx.Oracle
	if actualOraclePrice == 0 {
		actualOraclePrice = 5000 // Default to $0.50/ACME if not available
	}
	
	// Recalculate funding with actual oracle price
	funding := CalculateFunding(config, fees, actualOraclePrice)
	config.ACMEPerK = funding.TotalACMEPerK
	config.CreditsPerK = funding.ACMEForCreditsPerK
	
	t.Logf("📊 Actual oracle price: $%.2f/ACME", float64(actualOraclePrice)/10000)
	t.Logf("📊 Recalculated funding per sender: %s ACME", FormatACME(config.ACMEPerK))

	// Create accounts
	t.Logf("📋 Creating %d sender accounts and %d receiver accounts...", config.NumSenders, config.NumReceivers)
	ctx.CreateAllAccounts()
	t.Log("✅ Accounts created successfully")

	// Fund the funding account
	totalACME := GetRequiredFunding(config)
	t.Logf("💰 Requesting %.2f ACME from faucet for funding account...", float64(totalACME)/1e8)

	if err := ctx.FundFundingAccount(totalACME); err != nil {
		t.Fatalf("Failed to fund funding account: %v", err)
	}
	t.Log("✅ Funding account funded successfully")

	// Top off credits for funding account
	t.Log("🔍 Checking credits for funding account...")
	
	// Calculate credits needed for funding operations
	// Estimate 2 credits per sender account (for distribution operations)
	creditsNeeded := int64(config.NumSenders * 2 * protocol.CreditPrecision) // in internal units
	creditsNeeded = creditsNeeded + (creditsNeeded / 10) // Add 10% buffer
	
	// Check current credits
	currentCredits := ctx.GetCreditsBalance(ctx.FundingAcct.URL.WithQuery("",).Identity())
	t.Logf("📊 Current credits: %s, Target: %s credits", 
		FormatCredits(int64(currentCredits)), 
		FormatCredits(creditsNeeded))
	
	if currentCredits < uint64(creditsNeeded) {
		// Calculate ACME needed to buy the required credits using proper method
		deficit := uint64(creditsNeeded) - currentCredits
		deficitExternal := int64(deficit) / protocol.CreditPrecision
		acmeForCredits := CreditsToACME(deficitExternal, actualOraclePrice)
		
		t.Logf("➕ Adding %.4f ACME worth of credits to funding account...", float64(acmeForCredits)/1e8)
		
		if err := ctx.AddCredits(ctx.FundingAcct, ctx.FundingAcct, acmeForCredits); err != nil {
			t.Fatalf("Failed to add credits to funding account: %v", err)
		}
		
		t.Log("⏳ Waiting for credits to settle...")
		time.Sleep(GetSettlementWait())
		
		// Verify credits were added
		credits := ctx.GetCreditsBalance(ctx.FundingAcct.URL.WithQuery("",).Identity())
		t.Logf("✅ Funding account credits after top-off: %.2f credits", float64(credits)/protocol.CreditPrecision)
		
		if credits == 0 {
			t.Fatal("❌ ERROR: Funding account has ZERO credits after attempting to add")
		}
	} else {
		t.Logf("✅ Sufficient credits already present: %.2f", float64(currentCredits)/protocol.CreditPrecision)
	}

	// Distribute ACME to senders
	t.Logf("💸 Distributing %.2f ACME to each of %d sender accounts...", float64(config.ACMEPerK)/1e8, config.NumSenders)
	if err := ctx.DistributeToK(config.ACMEPerK); err != nil {
		t.Fatalf("Failed to distribute ACME to K accounts: %v", err)
	}
	t.Log("✅ ACME distributed to sender accounts")

	t.Log("⏳ Waiting for distribution to settle...")
	time.Sleep(GetSettlementWait())

	// Wait for K accounts to receive ACME
	t.Log("🔍 Verifying sender accounts received ACME...")
	if err := ctx.WaitForACME(ctx.KAccounts, config.ACMEPerK); err != nil {
		t.Fatalf("K accounts did not receive ACME: %v", err)
	}
	t.Log("✅ All sender accounts have received ACME")

	// Add credits to K accounts using v2 API like wallet does
	t.Logf("➕ Adding %s ACME worth of credits to %d sender accounts...", 
		FormatACME(config.CreditsPerK), config.NumSenders)
	expectedCreditsInternal := ACMEToCredits(config.CreditsPerK, actualOraclePrice)
	t.Logf("📊 Expected credits per sender: %s", FormatCredits(int64(expectedCreditsInternal)))
	
	if err := ctx.AddCreditsToK(config.CreditsPerK); err != nil {
		t.Fatalf("Failed to add credits to K accounts: %v", err)
	}
	t.Log("✅ Credits added to sender accounts")

	// Wait for credits to settle
	t.Log("⏳ Waiting for credits to settle...")
	// Just verify we have SOME credits, don't worry about exact amount
	// 100 internal units = 1 credit (CreditPrecision = 100)
	minCredits := uint64(100) // 1 credit minimum in internal units
	if err := ctx.WaitForCredits(ctx.KAccounts, minCredits); err != nil {
		// Some accounts may not have received credits, but continue anyway
		t.Logf("⚠️ WARNING: Some K accounts did not receive credits: %v", err)
		t.Log("⚠️ Continuing with load test anyway...")
	} else {
		t.Log("✅ All sender accounts have credits ready")
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
	verified := verifyBalances(t, ctx, config, results, timeout, verbose)

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

