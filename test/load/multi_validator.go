package main

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"log"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func main() {
	fmt.Println("🚀 Multi-Validator CrossChainConductor Load Test")
	fmt.Println("Testing 3 validators per partition with CrossChainConductor routing")
	fmt.Println()

	// Connect to multi-validator DevNet on port 26660
	apiURL := "http://127.0.0.1:26660"
	client := &api.Client{Server: apiURL}

	// Create 5 lite accounts for testing
	fmt.Println("📝 Creating lite accounts...")
	accounts := make([]*TestAccount, 5)
	for i := range accounts {
		accounts[i] = createLiteAccount()
		fmt.Printf("Account %d: %s\n", i, accounts[i].liteAddr)
	}
	fmt.Println()

	// Fund the accounts
	fmt.Println("💰 Funding accounts...")
	if !fundAccounts(client, accounts) {
		log.Fatal("Failed to fund accounts")
	}

	// Wait for accounts to be created across validators
	fmt.Println("⏳ Waiting for accounts to propagate across validators...")
	time.Sleep(5 * time.Second)

	// Add credits
	fmt.Println("💳 Adding credits to accounts...")
	if !addCreditsToAccounts(client, accounts) {
		log.Fatal("Failed to add credits")
	}

	// Wait for credits to settle across validators
	fmt.Println("⏳ Waiting for credits to settle across validators...")
	time.Sleep(5 * time.Second)

	// Run load test with multi-validator coordination
	fmt.Println("🔥 Starting multi-validator load test...")
	start := time.Now()
	
	var wg sync.WaitGroup
	var successful, failed int64
	var mu sync.Mutex

	// Send transactions concurrently from each account
	for i, account := range accounts {
		wg.Add(1)
		go func(accountIndex int, acc *TestAccount) {
			defer wg.Done()
			
			// Each account sends 4 transactions (20 total)
			for j := 0; j < 4; j++ {
				success := sendTransaction(client, acc, fmt.Sprintf("Multi-validator tx %d-%d", accountIndex, j))
				
				mu.Lock()
				if success {
					successful++
					fmt.Printf("✅ Transaction %d-%d succeeded\n", accountIndex, j)
				} else {
					failed++
					fmt.Printf("❌ Transaction %d-%d failed\n", accountIndex, j)
				}
				mu.Unlock()
				
				// Small delay between transactions
				time.Sleep(100 * time.Millisecond)
			}
		}(i, account)
	}

	wg.Wait()
	duration := time.Since(start)

	// Report results
	fmt.Println()
	fmt.Println("📊 Multi-Validator Load Test Results:")
	fmt.Printf("Duration: %v\n", duration)
	fmt.Printf("Successful transactions: %d\n", successful)
	fmt.Printf("Failed transactions: %d\n", failed)
	fmt.Printf("Total transactions: %d\n", successful+failed)
	fmt.Printf("Success rate: %.1f%%\n", float64(successful)/float64(successful+failed)*100)
	fmt.Printf("TPS: %.2f\n", float64(successful+failed)/duration.Seconds())
	
	if successful == 20 && failed == 0 {
		fmt.Println("🎉 Multi-validator CrossChainConductor test passed!")
	} else {
		fmt.Println("⚠️  Some transactions failed - check validator coordination")
	}
}

type TestAccount struct {
	privateKey ed25519.PrivateKey
	liteAddr   *url.URL
}

func createLiteAccount() *TestAccount {
	// Generate key pair
	_, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		log.Fatal("Failed to generate key:", err)
	}

	// Create lite token address
	liteAddr, err := protocol.LiteTokenAddress(privKey[32:], "ACME", protocol.SignatureTypeED25519)
	if err != nil {
		log.Fatal("Failed to create lite address:", err)
	}

	return &TestAccount{
		privateKey: privKey,
		liteAddr:   liteAddr,
	}
}

func fundAccounts(client *api.Client, accounts []*TestAccount) bool {
	ctx := context.Background()
	success := true

	for i, account := range accounts {
		// Fund 3 times to get ~30 ACME per account
		for j := 0; j < 3; j++ {
			faucetReq := &api.Faucet{Account: account.liteAddr}
			_, err := client.Faucet(ctx, faucetReq)
			if err != nil {
				fmt.Printf("❌ Failed to fund account %d (attempt %d): %v\n", i, j+1, err)
				success = false
			} else {
				fmt.Printf("✅ Funded account %d (attempt %d)\n", i, j+1)
			}
			time.Sleep(200 * time.Millisecond)
		}
	}

	return success
}

func addCreditsToAccounts(client *api.Client, accounts []*TestAccount) bool {
	ctx := context.Background()
	success := true

	for i, account := range accounts {
		// Add credits using the lite account identity as authority
		body := &protocol.AddCredits{
			Recipient: account.liteAddr,
			Amount:    protocol.AcmePrecisionPower, // 1.0 ACME worth of credits
			Oracle:    protocol.AcmeOraclePrice,   // Use oracle price
		}

		envelope := &messaging.Envelope{
			Messages: []messaging.Message{
				&messaging.TransactionMessage{
					Transaction: &protocol.Transaction{
						Header: &protocol.TransactionHeader{
							Principal: account.liteAddr,
						},
						Body: body,
					},
				},
			},
		}

		// Sign with lite account key (authority is the lite identity)
		sigMsg := &messaging.SignatureMessage{
			Signature: &protocol.ED25519Signature{
				Signer:    account.liteAddr.Identity(),
				Timestamp: uint64(time.Now().Unix()),
			},
		}

		// Sign the transaction hash
		hash := envelope.Messages[0].Hash()
		sigMsg.Signature.(*protocol.ED25519Signature).Signature = ed25519.Sign(account.privateKey, hash[:])
		envelope.Messages = append(envelope.Messages, sigMsg)

		// Submit transaction
		_, err := client.Submit(ctx, envelope, api.SubmitOptions{})
		if err != nil {
			fmt.Printf("❌ Failed to add credits to account %d: %v\n", i, err)
			success = false
		} else {
			fmt.Printf("✅ Added credits to account %d\n", i)
		}

		time.Sleep(200 * time.Millisecond)
	}

	return success
}

func sendTransaction(client *api.Client, account *TestAccount, memo string) bool {
	ctx := context.Background()

	// Send 0.1 ACME to same account (simple transaction)
	body := &protocol.SendTokens{
		To: []*protocol.TokenRecipient{
			{
				Url:    account.liteAddr,
				Amount: protocol.AcmePrecision / 10, // 0.1 ACME
			},
		},
		Meta: []byte(memo),
	}

	envelope := &messaging.Envelope{
		Messages: []messaging.Message{
			&messaging.TransactionMessage{
				Transaction: &protocol.Transaction{
					Header: &protocol.TransactionHeader{
						Principal: account.liteAddr,
					},
					Body: body,
				},
			},
		},
	}

	// Sign with lite account key
	sigMsg := &messaging.SignatureMessage{
		Signature: &protocol.ED25519Signature{
			Signer:    account.liteAddr.Identity(),
			Timestamp: uint64(time.Now().Unix()),
		},
	}

	// Sign the transaction hash
	hash := envelope.Messages[0].Hash()
	sigMsg.Signature.(*protocol.ED25519Signature).Signature = ed25519.Sign(account.privateKey, hash[:])
	envelope.Messages = append(envelope.Messages, sigMsg)

	// Submit transaction (this will go through CrossChainConductor routing)
	_, err := client.Submit(ctx, envelope, api.SubmitOptions{})
	if err != nil {
		return false
	}

	return true
}