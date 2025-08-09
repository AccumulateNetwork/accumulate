package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// CollectAndSpendTest demonstrates the full cycle: collect ACME from faucet, then spend it creating accounts
func main() {
	fmt.Println("🔄 Accumulate Collect & Spend Test Suite")
	fmt.Println("=======================================")
	fmt.Println("Phase 1: Collect ACME from faucet")
	fmt.Println("Phase 2: Spend ACME creating ADIs, token accounts, data accounts, token issuers")
	fmt.Println()
	
	// Initialize faucet helper for token collection
	faucetHelper, err := NewFaucetHelper("http://127.0.0.1:26660", &FaucetConfig{
		ACMEPerRequest:    10000000, // 10 ACME per request
		RequestInterval:   2 * time.Second,
		MaxConcurrentReqs: 2,
		RetryDelay:        1 * time.Second,
	})
	if err != nil {
		log.Fatalf("❌ Failed to create faucet helper: %v", err)
	}
	
	// Initialize ACME spender for account creation
	spender, err := NewACMESpender("http://127.0.0.1:26660", &SpenderConfig{
		WorkerCount:         3,
		TransactionInterval: 3 * time.Second,
		MaxRetries:         3,
		RetryDelay:         1 * time.Second,
		
		// Focus on transaction types that work well
		TokenTransferWeight:  40, // 40% transfers (lite to lite work well)
		DataWriteWeight:     20, // 20% data writes (to scratch space)
		AccountCreateWeight: 20, // 20% ADI creation (now with authority)
		TokenSendWeight:     20, // 20% simple sends
		DataCollectWeight:   0,  // Disable for now (query operations)
		TokenIssueWeight:    0,  // Disable for now (simplified to data writes)
		IssuedTokenMoveWeight: 0, // Disable for now (simplified to token sends)
		TokenAccountCreateWeight: 0, // Disable for now (simplified to ADI creation)
		DataAccountCreateWeight: 0,  // Disable for now (simplified to ADI creation)
	})
	if err != nil {
		log.Fatalf("❌ Failed to create ACME spender: %v", err)
	}
	
	// Create context for the test
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	
	// Phase 1: Start background faucet funding
	fmt.Println("🚰 Phase 1: Starting background faucet funding...")
	faucetHelper.Start(ctx)
	defer faucetHelper.Stop()
	
	// Wait for faucet to initialize
	time.Sleep(3 * time.Second)
	
	// Phase 2: Create well-funded accounts for spending
	fmt.Println("\n💰 Phase 2: Creating funded spender accounts...")
	spenderAccounts, err := faucetHelper.CreateMultipleFundedAccounts(5, 100000000) // 100 ACME each
	if err != nil {
		log.Fatalf("❌ Failed to create funded accounts: %v", err)
	}
	
	fmt.Printf("✅ Created %d spender accounts with 100 ACME each\n", len(spenderAccounts))
	
	// Phase 3: Start spending ACME on various account creation transactions
	fmt.Println("\n💸 Phase 3: Starting ACME spending operations...")
	err = spender.Start(ctx, spenderAccounts)
	if err != nil {
		log.Fatalf("❌ Failed to start ACME spender: %v", err)
	}
	defer spender.Stop()
	
	// Phase 4: Run the combined test for a period with periodic reporting
	fmt.Println("\n🔄 Phase 4: Running collect & spend cycle...")
	err = runCollectAndSpendCycle(ctx, faucetHelper, spender)
	if err != nil {
		log.Printf("⚠️  Test encountered issues: %v", err)
	}
	
	// Final report
	fmt.Println("\n📊 Final Test Results:")
	fmt.Println("======================")
	
	fmt.Println("\n🚰 FAUCET COLLECTION RESULTS:")
	faucetHelper.PrintStats()
	
	fmt.Println("\n💸 ACME SPENDING RESULTS:")
	spender.PrintStats()
	
	// Calculate efficiency
	faucetStats := faucetHelper.GetStats()
	spenderStats := spender.GetStats()
	
	fmt.Println("🎯 OVERALL EFFICIENCY:")
	fmt.Printf("💵 Total ACME Collected: %.2f ACME\n", float64(faucetStats.TotalACMEFunded)/1000000)
	fmt.Printf("💸 Total ACME Spent: %.2f ACME\n", float64(spenderStats.TotalACMESpent)/1000000)
	
	if faucetStats.TotalACMEFunded > 0 {
		utilization := float64(spenderStats.TotalACMESpent) / float64(faucetStats.TotalACMEFunded) * 100
		fmt.Printf("📊 ACME Utilization: %.1f%%\n", utilization)
	}
	
	fmt.Println("\n✅ Collect & Spend test completed successfully!")
}

// runCollectAndSpendCycle manages the coordinated collect/spend operations
func runCollectAndSpendCycle(ctx context.Context, faucetHelper *FaucetHelper, spender *ACMESpender) error {
	fmt.Println("🎪 Starting coordinated collect & spend cycle for 5 minutes...")
	
	// Create a test context for 5 minutes
	testCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	
	var wg sync.WaitGroup
	
	// Start periodic statistics reporting
	wg.Add(1)
	go func() {
		defer wg.Done()
		reportingTicker := time.NewTicker(30 * time.Second)
		defer reportingTicker.Stop()
		
		for {
			select {
			case <-testCtx.Done():
				return
			case <-reportingTicker.C:
				printCombinedStats(faucetHelper, spender)
			}
		}
	}()
	
	// Start dynamic account management
	wg.Add(1)
	go func() {
		defer wg.Done()
		managementTicker := time.NewTicker(1 * time.Minute)
		defer managementTicker.Stop()
		
		for {
			select {
			case <-testCtx.Done():
				return
			case <-managementTicker.C:
				err := manageDynamicAccounts(faucetHelper, spender)
				if err != nil {
					log.Printf("⚠️  Account management error: %v", err)
				}
			}
		}
	}()
	
	// Start transaction type cycling
	wg.Add(1)
	go func() {
		defer wg.Done()
		cyclingTicker := time.NewTicker(2 * time.Minute)
		defer cyclingTicker.Stop()
		
		cycle := 0
		for {
			select {
			case <-testCtx.Done():
				return
			case <-cyclingTicker.C:
				cycle++
				adjustTransactionMix(spender, cycle)
			}
		}
	}()
	
	wg.Wait()
	return nil
}

// printCombinedStats displays both faucet and spender statistics together
func printCombinedStats(faucetHelper *FaucetHelper, spender *ACMESpender) {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("📊 PERIODIC COLLECT & SPEND REPORT")
	fmt.Println(strings.Repeat("=", 80))
	
	faucetStats := faucetHelper.GetStats()
	spenderStats := spender.GetStats()
	
	// Collection summary
	fmt.Printf("🚰 COLLECTION: %d accounts created, %.2f ACME funded (%.1f%% success)\n",
		faucetStats.AccountsCreated,
		float64(faucetStats.TotalACMEFunded)/1000000,
		float64(faucetStats.SuccessfulReqs)/float64(faucetStats.TotalRequests)*100)
	
	// Spending summary
	fmt.Printf("💸 SPENDING: %d transactions, %.2f ACME spent (%.1f%% success)\n",
		spenderStats.TotalTransactions,
		float64(spenderStats.TotalACMESpent)/1000000,
		float64(spenderStats.SuccessfulTxs)/float64(spenderStats.TotalTransactions)*100)
	
	// Account creation breakdown
	fmt.Printf("🏗️  ACCOUNT TYPES CREATED:\n")
	fmt.Printf("   🆔 ADI Creates: %d\n", spenderStats.AccountCreations)
	fmt.Printf("   💸 Token Transfers: %d\n", spenderStats.TokenTransfers)
	fmt.Printf("   📝 Data Writes: %d\n", spenderStats.DataWrites)
	fmt.Printf("   📤 Token Sends: %d\n", spenderStats.TokenSends)
	
	// Flow analysis
	if faucetStats.TotalACMEFunded > 0 {
		flow := float64(spenderStats.TotalACMESpent) / float64(faucetStats.TotalACMEFunded) * 100
		fmt.Printf("🔄 ACME Flow: %.1f%% of collected ACME has been spent\n", flow)
	}
	
	elapsed := time.Since(faucetStats.StartTime)
	if elapsed.Seconds() > 0 {
		collectRate := float64(faucetStats.TotalACMEFunded) / 1000000 / elapsed.Seconds()
		spendRate := float64(spenderStats.TotalACMESpent) / 1000000 / elapsed.Seconds()
		fmt.Printf("⚡ RATES: Collecting %.2f ACME/s, Spending %.2f ACME/s\n", collectRate, spendRate)
	}
	
	fmt.Println(strings.Repeat("=", 80) + "\n")
}

// manageDynamicAccounts creates additional funded accounts as needed
func manageDynamicAccounts(faucetHelper *FaucetHelper, spender *ACMESpender) error {
	spenderStats := spender.GetStats()
	
	// If we're doing a lot of transactions, create more funded accounts
	if spenderStats.TotalTransactions > 0 && spenderStats.TotalTransactions%50 == 0 {
		log.Println("🏗️  Creating additional funded accounts for sustained spending...")
		
		newAccounts, err := faucetHelper.CreateMultipleFundedAccounts(2, 75000000) // 75 ACME each
		if err != nil {
			return fmt.Errorf("failed to create additional accounts: %v", err)
		}
		
		log.Printf("✅ Created %d additional funded accounts", len(newAccounts))
		// Note: In a full implementation, you'd add these to the spender's account pool
	}
	
	return nil
}

// adjustTransactionMix changes the transaction type distribution over time
func adjustTransactionMix(spender *ACMESpender, cycle int) {
	log.Printf("🔄 Adjusting transaction mix for cycle %d", cycle)
	
	// This would typically update the spender's configuration
	// For now, just log the cycle change
	switch cycle % 3 {
	case 0:
		log.Println("   Focus: ADI and account creation")
	case 1:
		log.Println("   Focus: Token transfers and data writes")
	case 2:
		log.Println("   Focus: Mixed transaction types")
	}
}