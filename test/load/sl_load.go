//go:build !testnet
// +build !testnet

package load_test

import (
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (ctx *LoadTestContext) GenerateLoad() (*LoadResults, error) {
	results := &LoadResults{}
	start := time.Now()
	
	var wg sync.WaitGroup
	var successCount int32
	var failCount int32
	
	txPerSender := ctx.Config.NumTxs / ctx.Config.NumSenders
	remainder := ctx.Config.NumTxs % ctx.Config.NumSenders
	
	for senderIdx := 0; senderIdx < ctx.Config.NumSenders; senderIdx++ {
		txCount := txPerSender
		if senderIdx < remainder {
			txCount++
		}
		
		wg.Add(1)
		go func(idx int, count int) {
			defer wg.Done()
			
			for i := 0; i < count; i++ {
				receiverIdx := i % ctx.Config.NumReceivers
				err := ctx.SendTransaction(
					ctx.KAccounts[idx],
					ctx.AAccounts[receiverIdx],
					ctx.Config.TxAmount,
				)
				
				if err == nil {
					atomic.AddInt32(&successCount, 1)
				} else {
					atomic.AddInt32(&failCount, 1)
				}
			}
		}(senderIdx, txCount)
	}
	
	wg.Wait()
	
	results.Duration = time.Since(start)
	results.TotalSent = ctx.Config.NumTxs
	results.TotalSuccess = int(successCount)
	results.TotalFailed = int(failCount)
	results.TPS = ctx.MeasureTPS(start, results.TotalSuccess)
	
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
	
	sub, err := ctx.Client.Submit(ctx.Context, env, api.SubmitOptions{})
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
	var wg sync.WaitGroup
	
	for i, tx := range batch {
		wg.Add(1)
		go func(idx int, transaction Transaction) {
			defer wg.Done()
			errors[idx] = ctx.SendTransaction(transaction.From, transaction.To, transaction.Amount)
		}(i, tx)
	}
	
	wg.Wait()
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