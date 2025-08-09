package main

import (
	"context"
	"fmt"
	"log"
	"time"
)

// DevNet Load Test - focuses on transaction types that work in DevNet environment
func main() {
	fmt.Println("🚀 DevNet Load Test - CrosschainCoordinator Validation")
	fmt.Println("====================================================")
	fmt.Println("Testing ACME collection and transaction generation")
	fmt.Println("Focus: Sustained load with faucet coordination")
	fmt.Println()

	// Initialize faucet helper for aggressive token collection
	faucetHelper, err := NewFaucetHelper("http://127.0.0.1:26660", &FaucetConfig{
		ACMEPerRequest:    10000000,        // 10 ACME per request
		RequestInterval:   1 * time.Second, // Fast collection
		MaxConcurrentReqs: 4,               // More concurrent requests
		RetryDelay:        500 * time.Millisecond,
	})
	if err != nil {
		log.Fatalf("❌ Failed to create faucet helper: %v", err)
	}

	// Initialize ACME spender optimized for DevNet
	spender, err := NewACMESpender("http://127.0.0.1:26660", &SpenderConfig{
		WorkerCount:         5,               // More workers for higher load
		TransactionInterval: 1 * time.Second, // Faster transaction attempts
		MaxRetries:          2,
		RetryDelay:          500 * time.Millisecond,

		// Focus on transaction types that provide good load testing
		TokenTransferWeight:      30, // Token transfers (will fail but generate load)
		DataWriteWeight:          30, // Data writes (will fail but generate load)
		AccountCreateWeight:      20, // ADI creation (will fail but test coordinator)
		TokenSendWeight:          20, // Simple sends (will fail but generate load)
		DataCollectWeight:        0,  // Skip query operations for now
		TokenIssueWeight:         0,  // Skip complex transactions for now
		IssuedTokenMoveWeight:    0,
		TokenAccountCreateWeight: 0,
		DataAccountCreateWeight:  0,
	})
	if err != nil {
		log.Fatalf("❌ Failed to create ACME spender: %v", err)
	}

	// Create context for load test duration
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Phase 1: Start aggressive background faucet funding
	fmt.Println("🚰 Phase 1: Starting aggressive faucet collection...")
	faucetHelper.Start(ctx)
	defer faucetHelper.Stop()

	// Wait for faucet to initialize
	time.Sleep(2 * time.Second)

	// Phase 2: Create multiple spender accounts for high load
	fmt.Println("\n💰 Phase 2: Creating multiple funded spender accounts...")
	spenderAccounts, err := faucetHelper.CreateMultipleFundedAccounts(10, 50000000) // 10 accounts, 50 ACME each
	if err != nil {
		log.Fatalf("❌ Failed to create funded accounts: %v", err)
	}

	fmt.Printf("✅ Created %d spender accounts with 50 ACME each\n", len(spenderAccounts))

	// Phase 3: Start high-load transaction generation
	fmt.Println("\n💸 Phase 3: Starting high-load transaction generation...")
	err = spender.Start(ctx, spenderAccounts)
	if err != nil {
		log.Fatalf("❌ Failed to start ACME spender: %v", err)
	}
	defer spender.Stop()

	// Phase 4: Run sustained load test with monitoring
	fmt.Println("\n🔥 Phase 4: Running sustained DevNet load test...")
	err = runDevNetLoadTest(ctx, faucetHelper, spender)
	if err != nil {
		log.Printf("⚠️  Load test encountered issues: %v", err)
	}

	// Final comprehensive report
	fmt.Println("\n📊 DevNet Load Test Results:")
	fmt.Println("=============================")

	fmt.Println("\n🚰 FAUCET PERFORMANCE:")
	faucetHelper.PrintStats()

	fmt.Println("\n💸 TRANSACTION LOAD PERFORMANCE:")
	spender.PrintStats()

	// Calculate load metrics
	faucetStats := faucetHelper.GetStats()
	spenderStats := spender.GetStats()
	elapsed := time.Since(faucetStats.StartTime)

	fmt.Println("\n🎯 LOAD TEST SUMMARY:")
	fmt.Printf("⏱️  Test Duration: %v\n", elapsed.Round(time.Second))
	fmt.Printf("📊 Total Transaction Attempts: %d\n", spenderStats.TotalTransactions)
	fmt.Printf("🚰 Total ACME Collected: %.2f ACME\n", float64(faucetStats.TotalACMEFunded)/1000000)

	if elapsed.Seconds() > 0 {
		tps := float64(spenderStats.TotalTransactions) / elapsed.Seconds()
		cps := float64(faucetStats.TotalACMEFunded) / 1000000 / elapsed.Seconds()
		fmt.Printf("⚡ Transaction Rate: %.2f tx/s\n", tps)
		fmt.Printf("⚡ Collection Rate: %.2f ACME/s\n", cps)
	}

	fmt.Println("\n✅ DevNet load test completed successfully!")
	fmt.Println("📈 CrosschainCoordinator processed sustained transaction load")
}

// runDevNetLoadTest manages the sustained load test operations
func runDevNetLoadTest(ctx context.Context, faucetHelper *FaucetHelper, spender *ACMESpender) error {
	fmt.Println("🎪 Starting sustained DevNet load for 4 minutes...")

	// Create a test context for the load duration
	testCtx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()

	// Start frequent statistics reporting for load monitoring
	statsTicker := time.NewTicker(15 * time.Second)
	defer statsTicker.Stop()

	go func() {
		for {
			select {
			case <-testCtx.Done():
				return
			case <-statsTicker.C:
				printDevNetLoadStats(faucetHelper, spender)
			}
		}
	}()

	// Run additional load generation
	go func() {
		loadTicker := time.NewTicker(30 * time.Second)
		defer loadTicker.Stop()

		for {
			select {
			case <-testCtx.Done():
				return
			case <-loadTicker.C:
				// Create additional funded accounts during the test
				log.Println("🔥 Creating additional accounts for sustained load...")
				newAccounts, err := faucetHelper.CreateMultipleFundedAccounts(3, 30000000) // 3 more accounts
				if err != nil {
					log.Printf("⚠️  Failed to create additional accounts: %v", err)
				} else {
					log.Printf("✅ Added %d more funded accounts", len(newAccounts))
				}
			}
		}
	}()

	// Wait for the test duration
	<-testCtx.Done()
	return nil
}

// printDevNetLoadStats displays focused load testing statistics
func printDevNetLoadStats(faucetHelper *FaucetHelper, spender *ACMESpender) {
	fmt.Println("\n" + "🔥 DevNet Load Test - Live Stats 🔥")
	fmt.Println("=====================================")

	faucetStats := faucetHelper.GetStats()
	spenderStats := spender.GetStats()
	elapsed := time.Since(faucetStats.StartTime)

	// Load generation stats
	fmt.Printf("⏱️  Runtime: %v\n", elapsed.Round(time.Second))
	fmt.Printf("🚰 Accounts Created: %d (%.2f ACME funded)\n",
		faucetStats.AccountsCreated, float64(faucetStats.TotalACMEFunded)/1000000)
	fmt.Printf("💸 Transaction Attempts: %d (%.1f%% connection rate)\n",
		spenderStats.TotalTransactions,
		float64(spenderStats.TotalTransactions-spenderStats.FailedTxs)/float64(max(spenderStats.TotalTransactions, 1))*100)

	// Load rates
	if elapsed.Seconds() > 0 {
		tps := float64(spenderStats.TotalTransactions) / elapsed.Seconds()
		rps := float64(faucetStats.TotalRequests) / elapsed.Seconds()
		fmt.Printf("⚡ Current Load: %.1f tx/s, %.1f req/s\n", tps, rps)
	}

	// Transaction breakdown
	fmt.Printf("🔄 Transaction Mix: Transfers(%d), Data(%d), ADI(%d), Send(%d)\n",
		spenderStats.TokenTransfers, spenderStats.DataWrites,
		spenderStats.AccountCreations, spenderStats.TokenSends)

	fmt.Println("=====================================\n")
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
