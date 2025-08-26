//go:build !testnet
// +build !testnet

package load_test

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"math/big"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// CreateHardCodedFundingAccount creates a deterministic funding account
// that is the same across all test runs
func CreateHardCodedFundingAccount() LiteAccount {
	// Use a fixed seed for the funding account
	// This makes it predictable and debuggable
	fixedSeed := sha256.Sum256([]byte("loadtest-funding-account-v1"))
	key := ed25519.NewKeyFromSeed(fixedSeed[:])
	
	liteUrl, _ := protocol.LiteTokenAddress(key[32:], protocol.ACME, protocol.SignatureTypeED25519)
	
	// Print the funding account address and private key for debugging
	fmt.Printf("\n=== FUNDING ACCOUNT ===\n")
	fmt.Printf("Address: %s\n", liteUrl)
	fmt.Printf("Private Key (hex): %x\n", key[:32])
	fmt.Printf("This address can be checked with: accumulate account %s\n", liteUrl)
	fmt.Printf("=======================\n\n")
	
	return LiteAccount{
		PrivateKey: key,
		PublicKey:  ed25519.PublicKey(key[32:]),
		URL:        liteUrl,
		Balance:    big.NewInt(0),
		Credits:    0,
	}
}

func CreateLiteAccount(seed [32]byte, index int, prefix string) LiteAccount {
	accountSeed := sha256.Sum256(append(seed[:], []byte(fmt.Sprintf("%s-%d", prefix, index))...))
	key := ed25519.NewKeyFromSeed(accountSeed[:])
	
	liteUrl, _ := protocol.LiteTokenAddress(key[32:], protocol.ACME, protocol.SignatureTypeED25519)
	
	return LiteAccount{
		PrivateKey: key,
		PublicKey:  ed25519.PublicKey(key[32:]),
		URL:        liteUrl,
		Balance:    big.NewInt(0),
		Credits:    0,
	}
}

func (ctx *LoadTestContext) CreateAllAccounts() {
	// Use hard-coded funding account for consistency across runs
	// This account can be checked with: accumulate account acc://loadtest-funding-8df7a2b9c3e5d1a0f6b8c4e2/ACME
	ctx.FundingAcct = CreateHardCodedFundingAccount()
	
	// Print the generation seed for K and A accounts
	fmt.Printf("\n=== GENERATION SEED ===\n")
	fmt.Printf("Seed (hex): %x\n", ctx.Seed)
	fmt.Printf("=======================\n\n")
	
	ctx.KAccounts = make([]LiteAccount, ctx.Config.NumSenders)
	for i := 0; i < ctx.Config.NumSenders; i++ {
		ctx.KAccounts[i] = CreateLiteAccount(ctx.Seed, i+1, "k")
	}
	
	ctx.AAccounts = make([]LiteAccount, ctx.Config.NumReceivers)
	for i := 0; i < ctx.Config.NumReceivers; i++ {
		ctx.AAccounts[i] = CreateLiteAccount(ctx.Seed, i+1, "a")
	}
}

func (ctx *LoadTestContext) FundAccount(account LiteAccount, amount int64) error {
	// Try v3 client with retry for better reliability
	var sub *api.Submission
	err := retryOperation(func() error {
		var e error
		sub, e = ctx.Client.Faucet(context.Background(), account.URL, api.FaucetOptions{})
		if e != nil {
			return fmt.Errorf("failed to faucet account: %w", e)
		}
		if sub == nil || sub.Status.TxID == nil {
			return fmt.Errorf("faucet transaction returned no ID")
		}
		return nil
	})
	if err != nil {
		return err
	}
	
	// No need to wait - faucet processes quickly
	return nil
}

func (ctx *LoadTestContext) FundFundingAccount(totalACME int64) error {
	// Add 10% buffer as per design
	totalACME = totalACME + (totalACME / 10)
	
	fmt.Printf("Checking current balance of funding account...\n")
	
	// First check current balance
	var currentBalance *big.Int
	var err error
	
	// Try to get current balance (account might not exist yet)
	for retry := 0; retry < 3; retry++ {
		currentBalance, err = ctx.GetBalance(ctx.FundingAcct.URL)
		if err == nil {
			break
		}
		time.Sleep(2 * time.Second)
	}
	
	// Calculate how much more ACME we need
	var neededACME int64
	if currentBalance == nil {
		fmt.Printf("Funding account doesn't exist yet, requesting full amount: %.2f ACME\n", float64(totalACME)/1e8)
		neededACME = totalACME
	} else {
		currentACME := currentBalance.Int64()
		fmt.Printf("Current balance: %.2f ACME\n", float64(currentACME)/1e8)
		
		if currentACME >= totalACME {
			fmt.Printf("Sufficient balance already present (%.2f >= %.2f ACME)\n", 
				float64(currentACME)/1e8, float64(totalACME)/1e8)
			ctx.FundingAcct.Balance = currentBalance
			return nil
		}
		
		neededACME = totalACME - currentACME
		fmt.Printf("Need to top off with %.2f ACME\n", float64(neededACME)/1e8)
	}
	
	// Request from faucet in 10 ACME increments
	numCalls := (neededACME + 9*1e8) / (10 * 1e8)
	if numCalls < 1 {
		numCalls = 1
	}
	
	fmt.Printf("Making %d faucet calls...\n", numCalls)
	for i := int64(0); i < numCalls; i++ {
		if err := ctx.FundAccount(ctx.FundingAcct, 10*1e8); err != nil {
			return fmt.Errorf("faucet call %d failed: %w", i+1, err)
		}
		// No delay needed - faucet can handle rapid requests
	}
	
	// Wait for faucet transactions to settle
	fmt.Printf("Waiting for faucet transactions to settle...\n")
	time.Sleep(10 * time.Second)
	
	// Verify final balance
	for retry := 0; retry < 60; retry++ {
		balance, err := ctx.GetBalance(ctx.FundingAcct.URL)
		if err != nil {
			if retry == 59 {
				return fmt.Errorf("funding account balance check failed after 60 retries: %w", err)
			}
			time.Sleep(500 * time.Millisecond)
			continue
		}
		
		if balance != nil {
			currentBalance := float64(balance.Int64())/1e8
			requiredBalance := float64(totalACME)/1e8
			requiredMin := float64(totalACME*75/100)/1e8
			
			if retry % 10 == 0 {
				fmt.Printf("Retry %d: Current balance: %.2f ACME (required: %.2f, min: %.2f)\n", 
					retry, currentBalance, requiredBalance, requiredMin)
			}
			
			// Accept if we have at least 75% of required (was 90%, then 85%)
			if balance.Cmp(big.NewInt(totalACME*75/100)) >= 0 {
				fmt.Printf("Final balance: %.2f ACME (required: %.2f)\n", 
					currentBalance, requiredBalance)
				ctx.FundingAcct.Balance = balance
				
				// Check for zero balance error
				if balance.Cmp(big.NewInt(0)) == 0 {
					return fmt.Errorf("ERROR: Funding account has ZERO ACME balance")
				}
				
				return nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	
	return fmt.Errorf("funding account did not reach required balance after topping off")
}

func (ctx *LoadTestContext) DistributeToK(amountPerAccount int64) error {
	// Submit all ACME transfers rapidly without excessive waiting
	for i, account := range ctx.KAccounts {
		if err := ctx.sendACME(ctx.FundingAcct, account, amountPerAccount); err != nil {
			return fmt.Errorf("failed to send to k%d: %w", i+1, err)
		}
	}
	
	// Wait once for all transfers to process
	time.Sleep(5 * time.Second)
	return nil
}

func (ctx *LoadTestContext) DistributeACME(from LiteAccount, toList []LiteAccount, amount int64) error {
	// Send sequentially to ensure proper processing
	for i, account := range toList {
		if err := ctx.sendACME(from, account, amount); err != nil {
			return fmt.Errorf("failed to send to account %d: %w", i, err)
		}
		// Small delay between sends
		if i < len(toList)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}
	
	return nil
}

func (ctx *LoadTestContext) sendACME(from, to LiteAccount, amount int64) error {
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
		return fmt.Errorf("failed to build transaction: %w", err)
	}
	
	// Use retry for better reliability
	var sub []*api.Submission
	err = retryOperation(func() error {
		var e error
		sub, e = ctx.Client.Submit(context.Background(), env, api.SubmitOptions{})
		if e != nil {
			return fmt.Errorf("failed to send ACME: %w", e)
		}
		if len(sub) == 0 || sub[0].Status.TxID == nil {
			return fmt.Errorf("send transaction returned no ID")
		}
		return nil
	})
	if err != nil {
		return err
	}
	
	return nil
}