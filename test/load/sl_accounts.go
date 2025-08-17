//go:build !testnet
// +build !testnet

package load_test

import (
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"math/big"
	"time"

	api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

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
	ctx.FundingAcct = CreateLiteAccount(ctx.Seed, 0, "funding")
	
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
	// Use the client's Faucet method directly
	sub, err := ctx.Client.Faucet(ctx.Context, account.URL, api.FaucetOptions{})
	if err != nil {
		return fmt.Errorf("failed to faucet account: %w", err)
	}
	
	if sub == nil || sub.Status.TxID == nil {
		return fmt.Errorf("faucet transaction returned no ID")
	}
	
	time.Sleep(FaucetDelay)
	return nil
}

func (ctx *LoadTestContext) FundFundingAccount(totalACME int64) error {
	numCalls := (totalACME + 9*1e8) / (10 * 1e8)
	
	for i := int64(0); i < numCalls; i++ {
		if err := ctx.FundAccount(ctx.FundingAcct, 10*1e8); err != nil {
			return err
		}
	}
	
	time.Sleep(GetSettlementWait())
	
	var lastBalance *big.Int
	var lastErr error
	for retry := 0; retry < GetMaxRetries(); retry++ {
		balance, err := ctx.GetBalance(ctx.FundingAcct.URL)
		if err != nil {
			// Account might not exist yet, keep trying
			lastErr = err
			time.Sleep(1 * time.Second)
			continue
		}
		lastBalance = balance
		if balance != nil && balance.Cmp(big.NewInt(totalACME)) >= 0 {
			ctx.FundingAcct.Balance = balance
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	
	if lastBalance != nil {
		return fmt.Errorf("funding account did not receive expected balance: wanted %d, got %s", totalACME/1e8, lastBalance.String())
	}
	return fmt.Errorf("funding account did not receive expected balance: %v", lastErr)
}

func (ctx *LoadTestContext) DistributeToK(amountPerAccount int64) error {
	// Send sequentially with small delays to avoid overwhelming the network
	for i, account := range ctx.KAccounts {
		if err := ctx.sendACME(ctx.FundingAcct, account, amountPerAccount); err != nil {
			return fmt.Errorf("failed to send to k%d: %w", i+1, err)
		}
		// Small delay between sends to allow processing
		if i < len(ctx.KAccounts)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}
	
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
	
	sub, err := ctx.Client.Submit(ctx.Context, env, api.SubmitOptions{})
	if err != nil {
		return fmt.Errorf("failed to send ACME: %w", err)
	}
	
	if len(sub) == 0 || sub[0].Status.TxID == nil {
		return fmt.Errorf("send transaction returned no ID")
	}
	
	return nil
}