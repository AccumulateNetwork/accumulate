//go:build !testnet
// +build !testnet

package load_test

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/time/rate"
)

func generateLoadWithRateLimit(ctx *LoadTestContext, targetTPS int, verbose bool) (*LoadResults, error) {
	fmt.Printf("\n📊 Starting rate-limited transaction sending at %d TPS...\n", targetTPS)
	
	results := &LoadResults{}
	start := time.Now()

	// Create rate limiter
	limiter := rate.NewLimiter(rate.Limit(targetTPS), targetTPS)

	successCount := 0
	failCount := 0
	sentCount := 0

	txPerSender := ctx.Config.NumTxs / ctx.Config.NumSenders
	remainder := ctx.Config.NumTxs % ctx.Config.NumSenders

	// Initialize A account tracking
	ctx.AAccountsReceived = make(map[string]int64)
	for _, account := range ctx.AAccounts {
		ctx.AAccountsReceived[account.URL.String()] = 0
	}
	
	// Build transaction list
	fmt.Printf("📝 Building transaction list: %d transactions across %d senders to %d receivers\n", 
		ctx.Config.NumTxs, ctx.Config.NumSenders, ctx.Config.NumReceivers)
	
	type txPair struct {
		sender   LiteAccount
		receiver LiteAccount
	}
	
	transactions := make([]txPair, 0, ctx.Config.NumTxs)
	for senderIdx := 0; senderIdx < ctx.Config.NumSenders; senderIdx++ {
		txCount := txPerSender
		if senderIdx < remainder {
			txCount++
		}

		for i := 0; i < txCount; i++ {
			receiverIdx := len(transactions) % ctx.Config.NumReceivers
			transactions = append(transactions, txPair{
				sender:   ctx.KAccounts[senderIdx],
				receiver: ctx.AAccounts[receiverIdx],
			})
		}
	}

	fmt.Printf("✅ Transaction list ready: %d transactions\n", len(transactions))
	fmt.Printf("🚀 Starting to send transactions...\n\n")

	// Progress tracking (per design: every 10% OR 30 seconds)
	lastPrintTime := time.Now()
	progressInterval := 30 * time.Second
	nextPercentageMilestone := 10

	// Send transactions with rate limiting
	for i, tx := range transactions {
		// Rate limit
		limiter.Wait(context.Background())
		
		sentCount++
		
		// Send transaction
		err := ctx.SendTransaction(
			tx.sender,
			tx.receiver,
			ctx.Config.TxAmount,
		)

		if err == nil {
			successCount++
			// Track amount sent to this A account
			ctx.AAccountsReceived[tx.receiver.URL.String()] += ctx.Config.TxAmount
		} else {
			failCount++
			if verbose {
				fmt.Printf("❌ Transaction %d failed: %v\n", i+1, err)
			}
		}

		// Print progress: every 10% OR every 30 seconds (whichever comes first)
		currentPercentage := (sentCount * 100) / ctx.Config.NumTxs
		shouldPrintPercentage := currentPercentage >= nextPercentageMilestone
		shouldPrintTime := time.Since(lastPrintTime) >= progressInterval
		
		if shouldPrintPercentage || shouldPrintTime {
			elapsed := time.Since(start)
			currentTPS := float64(sentCount) / elapsed.Seconds()
			successRate := float64(successCount) * 100 / float64(sentCount)
			
			// Calculate ETA if rate-limited
			var etaStr string
			if targetTPS > 0 {
				remainingTxs := ctx.Config.NumTxs - sentCount
				remainingSeconds := float64(remainingTxs) / float64(targetTPS)
				eta := time.Duration(remainingSeconds) * time.Second
				etaStr = fmt.Sprintf(", ETA: %v", eta.Round(time.Second))
			}
			
			fmt.Printf("📈 Progress: %d/%d sent (%.1f%%), %d success (%.1f%%), %d failed, %.1f TPS, elapsed: %v%s\n",
				sentCount, ctx.Config.NumTxs, 
				float64(sentCount)*100/float64(ctx.Config.NumTxs),
				successCount, successRate, failCount,
				currentTPS, elapsed.Round(time.Second), etaStr)
			
			if shouldPrintPercentage {
				nextPercentageMilestone += 10
			}
			lastPrintTime = time.Now()
		}
	}

	// Final summary
	results.Duration = time.Since(start)
	results.TotalSent = sentCount
	results.TotalSuccess = successCount
	results.TotalFailed = failCount
	results.TPS = float64(results.TotalSuccess) / results.Duration.Seconds()

	fmt.Printf("\n✅ Sending complete: %d sent, %d success, %d failed in %v (%.2f TPS)\n\n",
		sentCount, successCount, failCount, results.Duration, results.TPS)

	return results, nil
}