//go:build !testnet
// +build !testnet

package load_test

import (
	"fmt"
	"math/big"
	"time"

	api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (ctx *LoadTestContext) AddCredits(from, to LiteAccount, amount int64) error {
	credits := CalculateCredits(amount, ctx.Oracle)
	
	txn := build.Transaction().
		For(from.URL).
		Body(&protocol.AddCredits{
			Recipient: to.URL,
			Amount:    *big.NewInt(amount),
			Oracle:    ctx.Oracle,
		}).
		SignWith(from.URL).Version(1).Timestamp(uint64(time.Now().UnixNano())).PrivateKey(from.PrivateKey)
	
	env, err := txn.Done()
	if err != nil {
		return fmt.Errorf("failed to build transaction: %w", err)
	}
	
	sub, err := ctx.Client.Submit(ctx.Context, env, api.SubmitOptions{})
	if err != nil {
		return fmt.Errorf("failed to add credits: %w", err)
	}
	
	if len(sub) == 0 || sub[0].Status.TxID == nil {
		return fmt.Errorf("add credits transaction returned no ID")
	}
	
	to.Credits += credits
	return nil
}

func (ctx *LoadTestContext) AddCreditsToK(amountPerAccount int64) error {
	// Send credits sequentially to avoid overwhelming the network
	for i, account := range ctx.KAccounts {
		if err := ctx.AddCredits(ctx.FundingAcct, account, amountPerAccount); err != nil {
			return fmt.Errorf("failed to add credits to k%d: %w", i+1, err)
		}
		// Small delay between credit additions
		if i < len(ctx.KAccounts)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}
	
	return nil
}

func (ctx *LoadTestContext) WaitForCredits(accounts []LiteAccount, expected uint64) error {
	for retry := 0; retry < GetMaxRetries(); retry++ {
		allHaveCredits := true
		
		for i, account := range accounts {
			credits := ctx.GetCreditsBalance(account.URL)
			if credits < expected {
				allHaveCredits = false
				break
			}
			ctx.KAccounts[i].Credits = credits
		}
		
		if allHaveCredits {
			return nil
		}
		
		time.Sleep(1 * time.Second)
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
	
	resp, err := ctx.Client.Query(ctx.Context, principalUrl, nil)
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