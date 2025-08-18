//go:build !testnet
// +build !testnet

package load_test

import (
	"fmt"
	"math/big"
	"time"

	api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (ctx *LoadTestContext) WaitForACME(accounts []LiteAccount, expected int64) error {
	var lastErr error
	var lastBalance *big.Int
	for retry := 0; retry < GetMaxRetries(); retry++ {
		allHaveBalance := true
		
		for i, account := range accounts {
			balance, err := ctx.GetBalance(account.URL)
			if err != nil {
				// Account might not exist yet
				lastErr = err
				allHaveBalance = false
				continue // Check other accounts
			}
			lastBalance = balance
			if balance.Cmp(big.NewInt(expected)) < 0 {
				allHaveBalance = false
			}
			
			if i < len(ctx.KAccounts) {
				ctx.KAccounts[i].Balance = balance
			}
		}
		
		if allHaveBalance {
			return nil
		}
		
		time.Sleep(3 * time.Second) // Give more time for accounts to be created
	}
	
	if lastErr != nil {
		return fmt.Errorf("accounts did not receive expected ACME (wanted %d): %v", expected/1e8, lastErr)
	}
	if lastBalance != nil {
		return fmt.Errorf("accounts did not receive expected ACME: wanted %d ACME, got %s", expected/1e8, lastBalance.String())
	}
	return fmt.Errorf("accounts did not receive expected ACME")
}

func (ctx *LoadTestContext) VerifyBalances(accounts []LiteAccount, expected []int64) bool {
	if len(accounts) != len(expected) {
		return false
	}
	
	for i, account := range accounts {
		balance, err := ctx.GetBalance(account.URL)
		if err != nil {
			return false
		}
		
		expectedBig := big.NewInt(expected[i])
		tolerance := big.NewInt(1e4)
		
		diff := new(big.Int).Sub(balance, expectedBig)
		diff.Abs(diff)
		
		if diff.Cmp(tolerance) > 0 {
			return false
		}
	}
	
	return true
}

func (ctx *LoadTestContext) GetBalance(account *url.URL) (*big.Int, error) {
	resp, err := ctx.Client.Query(ctx.Context, account, nil)
	if err != nil {
		return nil, err
	}
	
	// Check if response is an AccountRecord
	if accRec, ok := resp.(*api.AccountRecord); ok {
		if acc, ok := accRec.Account.(*protocol.LiteTokenAccount); ok {
			return &acc.Balance, nil
		}
	}
	return nil, fmt.Errorf("unexpected account type: %T", resp)
}

func (ctx *LoadTestContext) CheckSettlement(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	
	for time.Now().Before(deadline) {
		allSettled := true
		
		for i, account := range ctx.KAccounts {
			balance, err := ctx.GetBalance(account.URL)
			if err != nil {
				allSettled = false
				break
			}
			ctx.KAccounts[i].Balance = balance
		}
		
		for i, account := range ctx.AAccounts {
			balance, err := ctx.GetBalance(account.URL)
			if err != nil {
				allSettled = false
				break
			}
			ctx.AAccounts[i].Balance = balance
		}
		
		if allSettled {
			return true
		}
		
		time.Sleep(1 * time.Second)
	}
	
	return false
}

func (ctx *LoadTestContext) WaitForTransaction(txHash []byte) error {
	for retry := 0; retry < GetMaxRetries(); retry++ {
		// Query transaction status using the txHash
		// This is a simplified version - you may need to adjust based on actual API
		time.Sleep(1 * time.Second)
	}
	
	return fmt.Errorf("transaction did not complete")
}