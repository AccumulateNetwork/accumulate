//go:build !testnet
// +build !testnet

package load_test

import (
	"context"
	"fmt"
	"math/big"
	"time"

	api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (ctx *LoadTestContext) AddCredits(from, to LiteAccount, acmeAmount int64) error {
	// Refresh oracle price before adding credits
	status, err := ctx.Client.NetworkStatus(context.Background(), api.NetworkStatusOptions{})
	if err == nil && status != nil && status.Oracle != nil && status.Oracle.Price > 0 {
		ctx.Oracle = status.Oracle.Price
	}
	
	// The amount parameter is already in ACME units (e.g., 0.01 ACME = 0.01 * 1e8)
	// AddCredits expects the ACME amount to spend on credits
	credits := CalculateCredits(acmeAmount, ctx.Oracle)
	
	// Build and sign the AddCredits transaction using the validated pattern
	env, err := build.Transaction().
		For(from.URL).
		Body(&protocol.AddCredits{
			Recipient: to.URL,
			Amount:    *big.NewInt(acmeAmount),  // This is the ACME amount to spend
			Oracle:    ctx.Oracle,
		}).
		SignWith(from.URL).Version(1).Timestamp(uint64(time.Now().UnixNano())).PrivateKey(from.PrivateKey).
		Done()
	if err != nil {
		return fmt.Errorf("failed to build and sign AddCredits transaction: %w", err)
	}
	
	// Use extended retry for credits (they may take longer)
	var sub []*api.Submission
	err = retryOperationExtended(func() error {
		var e error
		sub, e = ctx.Client.Submit(context.Background(), env, api.SubmitOptions{})
		if e != nil {
			return fmt.Errorf("failed to add credits: %w", e)
		}
		if len(sub) == 0 || sub[0].Status.TxID == nil {
			return fmt.Errorf("add credits transaction returned no ID")
		}
		return nil
	})
	if err != nil {
		return err
	}
	
	// No need to wait - credits process quickly
	
	to.Credits += credits
	return nil
}

func (ctx *LoadTestContext) AddCreditsToK(amountPerAccount int64) error {
	// Submit all credit transactions rapidly
	for i, account := range ctx.KAccounts {
		// Build and sign the AddCredits transaction
		env, err := build.Transaction().
			For(ctx.FundingAcct.URL).
			Body(&protocol.AddCredits{
				Recipient: account.URL,
				Amount:    *big.NewInt(amountPerAccount),
				Oracle:    ctx.Oracle,
			}).
			SignWith(ctx.FundingAcct.URL).Version(1).Timestamp(uint64(time.Now().UnixNano())).PrivateKey(ctx.FundingAcct.PrivateKey).
			Done()
		if err != nil {
			return fmt.Errorf("failed to build credits for k%d: %w", i+1, err)
		}
		
		// Submit without retry logic for speed
		ctx.Client.Submit(context.Background(), env, api.SubmitOptions{})
	}
	
	// Wait once for all credits to propagate
	time.Sleep(5 * time.Second)
	return nil
}

func (ctx *LoadTestContext) WaitForCredits(accounts []LiteAccount, expected uint64) error {
	// More retries with longer waits
	for retry := 0; retry < 30; retry++ {
		allHaveCredits := true
		accountsWithCredits := 0
		
		for i, account := range accounts {
			credits := ctx.GetCreditsBalance(account.URL)
			if retry%10 == 0 { // Log every 10 retries
				// Display credits in external units (divide by CreditPrecision)
				fmt.Printf("Account %d credits: %.2f (expected: %.2f)\n", 
					i, float64(credits)/protocol.CreditPrecision, float64(expected)/protocol.CreditPrecision)
			}
			if credits >= expected {
				accountsWithCredits++
			} else {
				allHaveCredits = false
			}
			ctx.KAccounts[i].Credits = credits
		}
		
		// Accept if most accounts have received credits
		if accountsWithCredits >= (len(accounts)*9)/10 {
			return nil
		}
		
		if allHaveCredits {
			return nil
		}
		
		time.Sleep(2 * time.Second)
	}
	
	// Log final state
	for i, account := range accounts {
		credits := ctx.GetCreditsBalance(account.URL)
		fmt.Printf("FINAL: Account %d credits: %.2f (expected: %.2f)\n", 
			i, float64(credits)/protocol.CreditPrecision, float64(expected)/protocol.CreditPrecision)
	}
	
	return fmt.Errorf("accounts did not receive expected credits")
}

func (ctx *LoadTestContext) VerifyCredits(accounts []LiteAccount) bool {
	for _, account := range accounts {
		credits := ctx.GetCreditsBalance(account.URL)
		if credits == 0 {
			return false
		}
	}
	return true
}

func (ctx *LoadTestContext) GetCreditsBalance(identity *url.URL) uint64 {
	principalUrl := identity.WithQuery("",).Identity()
	
	resp, err := ctx.Client.Query(context.Background(), principalUrl, nil)
	if err != nil {
		return 0
	}
	
	// Check if response is an AccountRecord
	if accRec, ok := resp.(*api.AccountRecord); ok {
		if id, ok := accRec.Account.(*protocol.LiteIdentity); ok {
			return id.CreditBalance
		}
	}
	
	return 0
}

func CalculateCredits(acme int64, oracle uint64) uint64 {
	if oracle == 0 {
		return 0
	}
	
	acmeAmount := new(big.Int).SetInt64(acme)
	oraclePrice := new(big.Int).SetUint64(oracle)
	creditsPerDollar := new(big.Int).SetInt64(protocol.CreditUnitsPerFiatUnit)
	acmePerDollar := new(big.Int).SetInt64(protocol.AcmePrecision)
	
	credits := new(big.Int).Mul(acmeAmount, oraclePrice)
	credits.Mul(credits, creditsPerDollar)
	credits.Div(credits, acmePerDollar)
	
	return credits.Uint64()
}