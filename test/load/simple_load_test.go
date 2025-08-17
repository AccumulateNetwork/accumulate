//go:build !testnet
// +build !testnet

package load_test

import (
	"flag"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

var (
	tpsFlag = flag.Int("tps", 100, "Target transactions per second")
	transactionsFlag = flag.Int("transactions", 100000, "Total number of transactions to send")
)

// TestSimpleLoadWithRetry is a single configurable load test with retry logic
func TestSimpleLoadWithRetry(t *testing.T) {
	// Parse flags if not already parsed
	if !flag.Parsed() {
		flag.Parse()
	}
	
	targetTPS := *tpsFlag
	numTxs := *transactionsFlag
	
	// Calculate optimal sender/receiver counts based on transaction count
	numSenders := 100
	if numTxs < 10000 {
		numSenders = 10
	} else if numTxs < 50000 {
		numSenders = 50
	}
	
	numReceivers := numSenders / 2
	if numReceivers < 5 {
		numReceivers = 5
	}
	
	// Calculate funding needs
	txPerSender := (numTxs / numSenders) + 1
	acmePerK := int64(txPerSender) * int64(0.001 * 1e8) + 5*1e8 // tx amount + 5 ACME buffer
	
	config := LoadConfig{
		NumSenders:   numSenders,
		NumReceivers: numReceivers,
		NumTxs:       numTxs,
		TxAmount:     0.001 * 1e8,
		ACMEPerK:     acmePerK,
		CreditsPerK:  2 * 1e8,
	}
	
	ctx := NewLoadTestContext(config)
	if ctx == nil {
		t.Log("Could not initialize test context - continuing without restart")
		return
	}
	
	// Setup phase
	t.Log("=== SETUP PHASE ===")
	t.Logf("Target TPS: %d", targetTPS)
	t.Logf("Total transactions: %d", numTxs)
	t.Logf("Expected duration: %.1f seconds", float64(numTxs)/float64(targetTPS))
	
	ctx.CreateAllAccounts()
	t.Logf("Created %d sender accounts and %d receiver accounts", numSenders, numReceivers)
	
	// Fund accounts
	totalACME := GetRequiredFunding(config)
	if err := ctx.FundFundingAccount(totalACME); err != nil {
		t.Logf("Failed to fund funding account: %v - continuing without restart", err)
		return
	}
	
	// Add credits
	if err := ctx.AddCredits(ctx.FundingAcct, ctx.FundingAcct, 10*1e8); err != nil {
		t.Logf("Failed to add credits: %v - continuing", err)
	}
	
	time.Sleep(GetSettlementWait())
	
	// Distribute to senders
	if err := ctx.DistributeToK(config.ACMEPerK); err != nil {
		t.Logf("Failed to distribute: %v - continuing", err)
	}
	
	time.Sleep(GetSettlementWait())
	
	// Add credits to senders
	if err := ctx.AddCreditsToK(config.CreditsPerK); err != nil {
		t.Logf("Failed to add credits to K: %v - continuing", err)
	}
	
	time.Sleep(GetSettlementWait())
	
	// Load test phase with rate limiting and retry logic
	t.Log("=== LOAD TEST PHASE ===")
	results := runLoadWithRetries(ctx, targetTPS, t)
	
	// Report results
	t.Log("=== SimpleLoadTest Results ===")
	t.Logf("Target TPS: %d", targetTPS)
	t.Logf("Actual TPS: %.1f", results.ActualTPS)
	t.Logf("Total Sent: %d", results.TotalSent)
	t.Logf("Successful: %d", results.Successful)
	t.Logf("Retries: %d", results.Retries)
	t.Logf("Failed: %d", results.Failed)
	t.Logf("Success Rate: %.2f%%", results.SuccessRate)
	t.Logf("Duration: %v", results.Duration)
	
	// Check if test met expectations
	if results.ActualTPS < float64(targetTPS)*0.9 || results.ActualTPS > float64(targetTPS)*1.1 {
		t.Logf("Warning: Actual TPS (%.1f) deviated from target TPS (%d) by more than 10%%", 
			results.ActualTPS, targetTPS)
	}
	
	if results.SuccessRate < 95.0 {
		t.Logf("Warning: Success rate (%.2f%%) is below 95%%", results.SuccessRate)
	}
	
	t.Log("Test completed - no restart on failure as requested")
}

// LoadTestResults holds the results of the load test
type LoadTestResults struct {
	TotalSent    int
	Successful   int
	Retries      int
	Failed       int
	ActualTPS    float64
	SuccessRate  float64
	Duration     time.Duration
}

// runLoadWithRetries executes the load test with retry logic
func runLoadWithRetries(ctx *LoadTestContext, targetTPS int, t *testing.T) *LoadTestResults {
	results := &LoadTestResults{}
	start := time.Now()
	
	// Create rate limiter
	limiter := rate.NewLimiter(rate.Limit(targetTPS), targetTPS)
	
	var wg sync.WaitGroup
	var successCount int32
	var retryCount int32
	var failCount int32
	
	// Channel to coordinate transaction submission
	type txJob struct {
		senderIdx   int
		receiverIdx int
		amount      int64
	}
	
	txChan := make(chan txJob, 1000)
	
	// Start worker goroutines
	numWorkers := 10
	if targetTPS < 10 {
		numWorkers = 1
	}
	
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range txChan {
				// Wait for rate limiter token
				err := limiter.Wait(ctx.Context)
				if err != nil {
					atomic.AddInt32(&failCount, 1)
					continue
				}
				
				// Try to send transaction with retries
				success := false
				for attempt := 0; attempt < 3; attempt++ {
					if attempt > 0 {
						// Pause 1 second before retry
						time.Sleep(1 * time.Second)
						atomic.AddInt32(&retryCount, 1)
					}
					
					err := ctx.SendTransaction(
						ctx.KAccounts[job.senderIdx],
						ctx.AAccounts[job.receiverIdx],
						job.amount,
					)
					
					if err == nil {
						success = true
						break
					}
				}
				
				if success {
					atomic.AddInt32(&successCount, 1)
				} else {
					atomic.AddInt32(&failCount, 1)
				}
			}
		}()
	}
	
	// Generate transaction jobs
	go func() {
		defer close(txChan)
		
		for i := 0; i < ctx.Config.NumTxs; i++ {
			senderIdx := i % ctx.Config.NumSenders
			receiverIdx := i % ctx.Config.NumReceivers
			
			txChan <- txJob{
				senderIdx:   senderIdx,
				receiverIdx: receiverIdx,
				amount:      ctx.Config.TxAmount,
			}
			
			// Progress report every 1000 transactions
			if (i+1)%1000 == 0 {
				elapsed := time.Since(start)
				actualTPS := float64(i+1) / elapsed.Seconds()
				t.Logf("Progress: %d/%d sent (%.1f%%), TPS: %.1f",
					i+1, ctx.Config.NumTxs, float64(i+1)*100/float64(ctx.Config.NumTxs),
					actualTPS)
			}
		}
	}()
	
	// Wait for all workers to complete
	wg.Wait()
	
	// Calculate results
	results.Duration = time.Since(start)
	results.TotalSent = ctx.Config.NumTxs
	results.Successful = int(successCount)
	results.Retries = int(retryCount)
	results.Failed = int(failCount)
	results.ActualTPS = float64(results.TotalSent) / results.Duration.Seconds()
	
	if results.TotalSent > 0 {
		results.SuccessRate = float64(results.Successful) * 100 / float64(results.TotalSent)
	}
	
	return results
}