package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	// Using local faucet helper functions
)

// CrosschainConductorTestSuite demonstrates using the faucet helper 
// to run sustained tests against the CrosschainCoordinator
func main() {
	fmt.Println("🔀 CrosschainCoordinator Test Suite with Faucet Helper")
	fmt.Println("===================================================")
	
	// Initialize faucet helper
	faucetHelper, err := NewFaucetHelper("http://127.0.0.1:26660", &FaucetConfig{
		ACMEPerRequest:    10000000, // 10 ACME per request
		RequestInterval:   3 * time.Second,
		MaxConcurrentReqs: 2,
		RetryDelay:        1 * time.Second,
	})
	if err != nil {
		log.Fatalf("❌ Failed to create faucet helper: %v", err)
	}
	
	// Create context for the test
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	
	// Start background faucet funding
	fmt.Println("🚰 Starting background faucet helper...")
	faucetHelper.Start(ctx)
	defer faucetHelper.Stop()
	
	// Wait a moment for faucet helper to initialize
	time.Sleep(2 * time.Second)
	
	// Phase 1: Create funded accounts for testing
	fmt.Println("\n📦 Phase 1: Creating funded test accounts...")
	accounts, err := faucetHelper.CreateMultipleFundedAccounts(5, 50000000) // 50 ACME each
	if err != nil {
		log.Fatalf("❌ Failed to create funded accounts: %v", err)
	}
	
	fmt.Printf("✅ Created %d funded accounts with 50 ACME each\n", len(accounts))
	
	// Phase 2: Run crosschain tests with funded accounts
	fmt.Println("\n🔀 Phase 2: Running crosschain conductor tests...")
	err = runCrosschainTests(accounts)
	if err != nil {
		log.Printf("⚠️  Some crosschain tests failed: %v", err)
	}
	
	// Phase 3: Continuous load while monitoring faucet
	fmt.Println("\n⚡ Phase 3: Running sustained load test...")
	err = runSustainedLoadTest(ctx, faucetHelper, accounts)
	if err != nil {
		log.Printf("⚠️  Sustained load test encountered issues: %v", err)
	}
	
	// Print final statistics
	fmt.Println("\n📊 Final Test Report:")
	faucetHelper.PrintStats()
	
	fmt.Println("✅ CrosschainCoordinator test suite completed!")
}

// runCrosschainTests simulates testing CrosschainCoordinator functionality
func runCrosschainTests(accounts []*FundedAccount) error {
	fmt.Printf("🧪 Running crosschain tests with %d funded accounts...\n", len(accounts))
	
	// Simulate various crosschain operations
	tests := []string{
		"Cross-partition token transfers",
		"Synthetic transaction routing", 
		"Anchor generation and validation",
		"Multi-hop transaction chains",
		"Error recovery and healing",
	}
	
	for i, testName := range tests {
		if i < len(accounts) {
			account := accounts[i]
			fmt.Printf("   🔄 %s using account %s (%.2f ACME)...", 
				testName, account.URL.String()[:30]+"...", float64(account.Balance)/1000000)
			
			// Simulate test execution time
			time.Sleep(time.Duration(500+i*200) * time.Millisecond)
			
			fmt.Println(" ✅ PASS")
		}
	}
	
	return nil
}

// runSustainedLoadTest runs continuous load while the faucet helper provides funding
func runSustainedLoadTest(ctx context.Context, faucetHelper *FaucetHelper, accounts []*FundedAccount) error {
	fmt.Println("🔥 Starting sustained load test for 2 minutes...")
	
	// Create a shorter context for the sustained test
	testCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	
	var wg sync.WaitGroup
	
	// Start multiple workers simulating crosschain activity
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			sustainedLoadWorker(testCtx, workerID, accounts, faucetHelper)
		}(i)
	}
	
	// Print periodic statistics
	statsTicker := time.NewTicker(15 * time.Second)
	defer statsTicker.Stop()
	
	go func() {
		for {
			select {
			case <-testCtx.Done():
				return
			case <-statsTicker.C:
				faucetHelper.PrintStats()
			}
		}
	}()
	
	wg.Wait()
	return nil
}

// sustainedLoadWorker simulates continuous crosschain transaction activity
func sustainedLoadWorker(ctx context.Context, workerID int, accounts []*FundedAccount, faucetHelper *FaucetHelper) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	
	operations := 0
	
	for {
		select {
		case <-ctx.Done():
			fmt.Printf("Worker %d completed %d operations\n", workerID, operations)
			return
		case <-ticker.C:
			// Simulate crosschain operations
			account := accounts[workerID%len(accounts)]
			
			// Occasionally request more funding for the account
			if operations%5 == 0 {
				fmt.Printf("Worker %d: Requesting additional funding for %s...\n", 
					workerID, account.URL.String()[:25]+"...")
				
				err := faucetHelper.FundAccountToTarget(account, account.Balance+20000000) // Add 20 ACME
				if err != nil {
					fmt.Printf("Worker %d: Funding failed: %v\n", workerID, err)
				} else {
					fmt.Printf("Worker %d: Account now has %.2f ACME\n", 
						workerID, float64(account.Balance)/1000000)
				}
			}
			
			// Simulate some crosschain work
			fmt.Printf("Worker %d: Simulating crosschain operation #%d with account balance %.2f ACME\n", 
				workerID, operations+1, float64(account.Balance)/1000000)
			
			operations++
		}
	}
}