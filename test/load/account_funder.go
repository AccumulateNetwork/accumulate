// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package load_test

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// FundingConfig contains configuration for funding accounts
type FundingConfig struct {
	TargetBalance int64 // Target balance in ACME (will be multiplied by 1e8)
	CreditAmount  int64 // Credits to add to each account
	MaxAttempts   int   // Maximum faucet attempts per account
	RetryDelay    time.Duration
}

// DefaultFundingConfig returns default funding configuration
func DefaultFundingConfig() FundingConfig {
	return FundingConfig{
		TargetBalance: 100,               // 100 ACME
		CreditAmount:  1000,              // 1000 credits
		MaxAttempts:   5,                 // Try faucet up to 5 times
		RetryDelay:    2 * time.Second,   // Wait 2 seconds between attempts
	}
}

// FundAccounts funds the given accounts with ACME tokens via faucet
func FundAccounts(ctx context.Context, client *jsonrpc.Client, accounts []TestAccount, config FundingConfig) error {
	targetBalanceFixed := big.NewInt(config.TargetBalance * 1e8)
	
	for i, account := range accounts {
		// Check current balance
		currentBalance := big.NewInt(0)
		record, err := client.Query(ctx, account.LiteURL, &api.DefaultQuery{})
		if err == nil {
			if accRecord, ok := record.(*api.AccountRecord); ok {
				if tokenAccount, ok := accRecord.Account.(*protocol.LiteTokenAccount); ok {
					currentBalance = &tokenAccount.Balance
					accounts[i].Balance = currentBalance
				}
			}
		}
		
		// Check if we need to fund
		if currentBalance.Cmp(targetBalanceFixed) >= 0 {
			balanceFloat := new(big.Float).Quo(new(big.Float).SetInt(currentBalance), big.NewFloat(1e8))
			fmt.Printf("Account %d already has sufficient balance: %s ACME\n", i+1, balanceFloat.String())
			continue
		}
		
		// Try to fund via faucet
		funded := false
		for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
			fmt.Printf("Funding account %d (attempt %d/%d)...\n", i+1, attempt, config.MaxAttempts)
			
			submission, err := client.Faucet(ctx, account.LiteURL, api.FaucetOptions{})
			if err != nil {
				fmt.Printf("Faucet error: %v\n", err)
				time.Sleep(config.RetryDelay)
				continue
			}
			
			if submission.Status != nil && submission.Status.Error != nil {
				fmt.Printf("Faucet returned error: %v\n", submission.Status.Error)
				time.Sleep(config.RetryDelay)
				continue
			}
			
			funded = true
			fmt.Printf("Faucet successful for account %d\n", i+1)
			time.Sleep(config.RetryDelay) // Wait for transaction to process
			
			// Check balance again
			record, err := client.Query(ctx, account.LiteURL, &api.DefaultQuery{})
			if err == nil {
				if accRecord, ok := record.(*api.AccountRecord); ok {
					if tokenAccount, ok := accRecord.Account.(*protocol.LiteTokenAccount); ok {
						accounts[i].Balance = &tokenAccount.Balance
						if tokenAccount.Balance.Cmp(targetBalanceFixed) >= 0 {
							break
						}
					}
				}
			}
		}
		
		if !funded {
			return fmt.Errorf("failed to fund account %d after %d attempts", i+1, config.MaxAttempts)
		}
	}
	
	return nil
}

// AddCredits adds credits to the given accounts
func AddCredits(ctx context.Context, client *jsonrpc.Client, accounts []TestAccount, creditAmount int64) error {
	for i, account := range accounts {
		// Build add credits transaction
		env, err := build.Transaction().
			For(account.LiteURL).
			Body(&protocol.AddCredits{
				Recipient: account.LiteURL,
				Amount:    *big.NewInt(creditAmount * 1e8), // Convert to fixed point
				Oracle:    1e8, // 1 credit per ACME
			}).
			SignWith(account.LiteIdentity).Version(1).Timestamp(uint64(time.Now().Unix())).PrivateKey(account.Key).
			Done()
		
		if err != nil {
			return fmt.Errorf("failed to build add credits transaction for account %d: %w", i+1, err)
		}

		// Submit transaction
		submissions, err := client.Submit(ctx, env, api.SubmitOptions{})
		if err != nil {
			fmt.Printf("Warning: Failed to add credits to account %d: %v\n", i+1, err)
			continue
		}
		
		for _, sub := range submissions {
			if sub.Status != nil && sub.Status.Error != nil {
				fmt.Printf("Warning: Add credits error for account %d: %v\n", i+1, sub.Status.Error)
			} else {
				fmt.Printf("Added %d credits to account %d\n", creditAmount, i+1)
			}
		}
		
		time.Sleep(500 * time.Millisecond)
	}
	
	return nil
}

// FundAndPrepareAccounts funds accounts and adds credits in one operation
func FundAndPrepareAccounts(ctx context.Context, endpoint string, accounts []TestAccount, config FundingConfig) error {
	// Create client
	client := jsonrpc.NewClient(endpoint)
	client.Client.Timeout = 30 * time.Second
	
	// Fund accounts
	fmt.Println("Funding accounts with ACME...")
	if err := FundAccounts(ctx, client, accounts, config); err != nil {
		return fmt.Errorf("failed to fund accounts: %w", err)
	}
	
	// Wait for funding to settle
	fmt.Println("Waiting for funding to process...")
	time.Sleep(3 * time.Second)
	
	// Add credits
	fmt.Println("Adding credits to accounts...")
	if err := AddCredits(ctx, client, accounts, config.CreditAmount); err != nil {
		return fmt.Errorf("failed to add credits: %w", err)
	}
	
	// Wait for credits to process
	fmt.Println("Waiting for credits to process...")
	time.Sleep(3 * time.Second)
	
	return nil
}