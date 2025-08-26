//go:build !testnet
// +build !testnet

package load_test

import (
	"fmt"
	"math/big"
	"testing"
	"time"
)

func verifyBalances(t *testing.T, ctx *LoadTestContext, config LoadConfig, results *LoadResults, timeout time.Duration, verbose bool) bool {
	t.Log("🔍 Starting balance verification...")
	fmt.Printf("🔍 Starting balance verification (timeout: %v)...\n", timeout)
	
	// Track previous balances to detect progress
	prevSenderBalances := make([]*big.Int, len(ctx.KAccounts))
	prevReceiverBalances := make([]*big.Int, len(ctx.AAccounts))

	noProgressTimeout := 1 * time.Minute // Per design: 1 minute timeout that resets on progress
	lastProgressTime := time.Now()
	deadline := time.Now().Add(timeout)
	checkCount := 0

	for time.Now().Before(deadline) {
		checkCount++
		time.Sleep(2 * time.Second)

		progressMade := false
		allCorrect := true
		txPerSender := config.NumTxs / config.NumSenders
		remainder := config.NumTxs % config.NumSenders

		// Check sender balances
		senderIssues := 0
		for i, account := range ctx.KAccounts {
			txCount := txPerSender
			if i < remainder {
				txCount++
			}

			expectedBalance := big.NewInt(config.ACMEPerK - int64(txCount)*config.TxAmount)
			balance, err := ctx.GetBalance(account.URL)
			if err != nil {
				allCorrect = false
				senderIssues++
				continue
			}

			// Check if balance changed
			if prevSenderBalances[i] == nil || balance.Cmp(prevSenderBalances[i]) != 0 {
				progressMade = true
				prevSenderBalances[i] = new(big.Int).Set(balance)
			}

			diff := new(big.Int).Sub(balance, expectedBalance)
			if diff.Abs(diff).Cmp(big.NewInt(1000)) > 0 { // Allow 0.00001 ACME tolerance
				allCorrect = false
				senderIssues++
				if verbose {
					fmt.Printf("  ❌ K%d: expected %v, got %v (diff: %v)\n", 
						i+1, expectedBalance, balance, diff)
				}
			}
		}

		// Check receiver balances using tracked amounts
		receiverIssues := 0
		for i, account := range ctx.AAccounts {
			// Use tracked amount instead of calculated
			expectedBalance := big.NewInt(ctx.AAccountsReceived[account.URL.String()])
			balance, err := ctx.GetBalance(account.URL)
			if err != nil {
				allCorrect = false
				receiverIssues++
				continue
			}

			// Check if balance changed
			if prevReceiverBalances[i] == nil || balance.Cmp(prevReceiverBalances[i]) != 0 {
				progressMade = true
				prevReceiverBalances[i] = new(big.Int).Set(balance)
			}

			diff := new(big.Int).Sub(balance, expectedBalance)
			if diff.Abs(diff).Cmp(big.NewInt(1000)) > 0 { // Allow 0.00001 ACME tolerance
				allCorrect = false
				receiverIssues++
				if verbose {
					fmt.Printf("  ❌ A%d: expected %v (tracked), got %v (diff: %v)\n", 
						i+1, expectedBalance, balance, diff)
				}
			}
		}

		// Print progress every 5 checks
		if checkCount%5 == 0 || allCorrect {
			fmt.Printf("📊 Check #%d: %d/%d senders OK, %d/%d receivers OK\n", 
				checkCount,
				config.NumSenders-senderIssues, config.NumSenders,
				config.NumReceivers-receiverIssues, config.NumReceivers)
		}

		// Reset timeout if progress was made
		if progressMade {
			lastProgressTime = time.Now()
			if verbose {
				fmt.Printf("   ↻ Settlement progress detected, resetting no-progress timer\n")
			}
		}

		// Check if we've been stuck without progress
		if time.Since(lastProgressTime) > noProgressTimeout {
			fmt.Printf("⚠️  No settlement progress for %v, stopping verification\n", noProgressTimeout)
			break
		}

		if allCorrect {
			fmt.Printf("✅ All balances correct after %d checks!\n", checkCount)
			return true
		}
	}

	fmt.Printf("⏱️  Verification timeout after %d checks\n", checkCount)
	return false
}