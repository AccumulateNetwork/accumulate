//go:build !testnet
// +build !testnet

package load_test

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (ctx *LoadTestContext) GenerateLoad() (*LoadResults, error) {
	fmt.Printf("\n🚀 Starting unlimited speed transaction sending...\n")
	fmt.Printf("📝 Configuration: %d transactions, %d senders, %d receivers\n", 
		ctx.Config.NumTxs, ctx.Config.NumSenders, ctx.Config.NumReceivers)
	
	results := &LoadResults{}
	start := time.Now()
	
	successCount := 0
	failCount := 0
	sentCount := 0
	
	txPerSender := ctx.Config.NumTxs / ctx.Config.NumSenders
	remainder := ctx.Config.NumTxs % ctx.Config.NumSenders
	
	// Progress tracking (per design: every 10% OR 30 seconds)
	lastPrintTime := time.Now()
	progressInterval := 30 * time.Second
	nextPercentageMilestone := 10
	
	fmt.Printf("🔄 Sending transactions at maximum speed...\n\n")
	
	// Sequential sending for simplicity
	totalTxIndex := 0
	for senderIdx := 0; senderIdx < ctx.Config.NumSenders; senderIdx++ {
		txCount := txPerSender
		if senderIdx < remainder {
			txCount++
		}
		
		for i := 0; i < txCount; i++ {
			receiverIdx := totalTxIndex % ctx.Config.NumReceivers
			
			sentCount++
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
			
			totalTxIndex++
			
			// Print progress: every 10% OR every 30 seconds (whichever comes first)
			currentPercentage := (sentCount * 100) / ctx.Config.NumTxs
			shouldPrintPercentage := currentPercentage >= nextPercentageMilestone
			shouldPrintTime := time.Since(lastPrintTime) >= progressInterval
			
			if shouldPrintPercentage || shouldPrintTime {
				elapsed := time.Since(start)
				currentTPS := float64(sentCount) / elapsed.Seconds()
				successRate := float64(successCount) * 100 / float64(sentCount)
				
				fmt.Printf("📈 Progress: %d/%d sent (%.1f%%), %d success (%.1f%%), %d failed, %.1f TPS, elapsed: %v\n",
					sentCount, ctx.Config.NumTxs, 
					float64(sentCount)*100/float64(ctx.Config.NumTxs),
					successCount, successRate, failCount,
					currentTPS, elapsed.Round(time.Second))
				
				if shouldPrintPercentage {
					nextPercentageMilestone += 10
				}
				lastPrintTime = time.Now()
			}
		}
	}
	
	// Final progress print
	elapsed := time.Since(start)
	fmt.Printf("\n✅ Sending complete: %d sent, %d success, %d failed in %v\n",
		sentCount, successCount, failCount, elapsed)
	
	results.Duration = elapsed
	results.TotalSent = ctx.Config.NumTxs
	results.TotalSuccess = successCount
	results.TotalFailed = failCount
	results.TPS = ctx.MeasureTPS(start, results.TotalSuccess)
	
	fmt.Printf("📊 Effective TPS: %.2f\n\n", results.TPS)
	
	return results, nil
}

func (ctx *LoadTestContext) SendTransaction(from, to LiteAccount, amount int64) error {
	txn := build.Transaction().
		For(from.URL).
		Body(&protocol.SendTokens{
			To: []*protocol.TokenRecipient{{
				Url:    to.URL,
				Amount: *big.NewInt(amount),
			}},
		}).
		SignWith(from.URL).Version(1).Timestamp(uint64(time.Now().UnixNano())).PrivateKey(from.PrivateKey)
	
	env, err := txn.Done()
	if err != nil {
		return err
	}
	
	sub, err := ctx.Client.Submit(context.Background(), env, api.SubmitOptions{})
	if err != nil {
		return err
	}
	
	if len(sub) == 0 || sub[0].Status.TxID == nil {
		return fmt.Errorf("transaction returned no ID")
	}
	
	return nil
}

func (ctx *LoadTestContext) SendBatch(batch []Transaction) []error {
	errors := make([]error, len(batch))
	
	// Process transactions sequentially as per design requirement
	for i, tx := range batch {
		errors[i] = ctx.SendTransaction(tx.From, tx.To, tx.Amount)
	}
	
	return errors
}

func (ctx *LoadTestContext) TrackTransactions(txList []Transaction) *LoadResults {
	results := &LoadResults{
		TotalSent: len(txList),
	}
	
	for _, tx := range txList {
		if tx.Status == "success" {
			results.TotalSuccess++
		} else {
			results.TotalFailed++
		}
	}
	
	return results
}

func (ctx *LoadTestContext) MeasureTPS(start time.Time, count int) float64 {
	duration := time.Since(start).Seconds()
	if duration == 0 {
		return 0
	}
	return float64(count) / duration
}