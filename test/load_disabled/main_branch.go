package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	v3api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type LiteAccount struct {
	PrivateKey  ed25519.PrivateKey
	TokenURL    *url.URL
	IdentityURL *url.URL
	PublicKey   []byte
}

func createLiteAccount() (*LiteAccount, error) {
	seed := make([]byte, 32)
	_, err := rand.Read(seed)
	if err != nil {
		return nil, err
	}

	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := privateKey[32:]

	tokenURL, err := protocol.LiteTokenAddress(publicKey, protocol.ACME, protocol.SignatureTypeED25519)
	if err != nil {
		return nil, err
	}

	identityURL := tokenURL.Identity()

	return &LiteAccount{
		PrivateKey:  privateKey,
		TokenURL:    tokenURL,
		IdentityURL: identityURL,
		PublicKey:   publicKey,
	}, nil
}

func fundAccount(tokenURL *url.URL) error {
	resp, err := http.Post(
		"http://127.0.0.1:27004/faucet",
		"text/plain",
		strings.NewReader(tokenURL.String()),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("faucet failed (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

func addCreditsToAccount(client *jsonrpc.Client, account *LiteAccount) error {
	ctx := context.Background()
	timestamp := uint64(time.Now().UnixMilli())

	// Query network status for oracle price
	ns, err := client.NetworkStatus(ctx, v3api.NetworkStatusOptions{Partition: "Directory"})
	if err != nil {
		return fmt.Errorf("failed to get network status: %v", err)
	}

	// Calculate oracle price
	oracle := float64(ns.Oracle.Price) / 1e8 // AcmeOraclePrecision
	if oracle == 0 {
		oracle = 0.01 // Set test price for DevNet
	}

	// Build add credits transaction
	env, err := build.Transaction().
		For(account.TokenURL).
		Body(&protocol.AddCredits{
			Recipient: account.IdentityURL,
			Amount:    *big.NewInt(100000), // 1 ACME worth of credits
			Oracle:    uint64(oracle * 1e8),
		}).
		SignWith(account.IdentityURL).Version(1).Timestamp(&timestamp).PrivateKey(account.PrivateKey).
		Done()

	if err != nil {
		return fmt.Errorf("build credits transaction failed: %v", err)
	}

	subs, err := client.Submit(ctx, env, v3api.SubmitOptions{})
	if err != nil {
		return fmt.Errorf("submit credits transaction failed: %v", err)
	}

	// Check if any result failed
	for i, sub := range subs {
		if err := sub.Status.AsError(); err != nil {
			return fmt.Errorf("credits result %d failed: %v", i, err)
		}
	}

	return nil
}

func sendTransaction(client *jsonrpc.Client, from, to *LiteAccount, amount int64) error {
	ctx := context.Background()
	timestamp := uint64(time.Now().UnixMilli())

	env, err := build.Transaction().
		For(from.TokenURL).
		Body(&protocol.SendTokens{
			To: []*protocol.TokenRecipient{{
				Url:    to.TokenURL,
				Amount: *big.NewInt(amount),
			}},
		}).
		SignWith(from.IdentityURL).Version(1).Timestamp(&timestamp).PrivateKey(from.PrivateKey).
		Done()

	if err != nil {
		return fmt.Errorf("build failed: %v", err)
	}

	subs, err := client.Submit(ctx, env, v3api.SubmitOptions{})
	if err != nil {
		return fmt.Errorf("submit failed: %v", err)
	}

	// Check if any result failed
	for i, sub := range subs {
		if err := sub.Status.AsError(); err != nil {
			return fmt.Errorf("result %d failed: %v", i, err)
		}
	}

	return nil
}

func main() {
	fmt.Println("🚀 Main Branch Load Test")
	fmt.Println("Testing lite account transactions on main branch")

	client := jsonrpc.NewClient("http://127.0.0.1:27004/v3")

	// Create accounts
	fmt.Println("\n📝 Creating lite accounts...")
	accounts := make([]*LiteAccount, 5)
	for i := 0; i < 5; i++ {
		acc, err := createLiteAccount()
		if err != nil {
			log.Fatalf("Failed to create account %d: %v", i, err)
		}
		accounts[i] = acc
		fmt.Printf("Account %d: %s\n", i, acc.TokenURL.String())
	}

	// Fund accounts
	fmt.Println("\n💰 Funding accounts...")
	for i, acc := range accounts {
		for j := 0; j < 3; j++ { // 3 faucet calls each
			if err := fundAccount(acc.TokenURL); err != nil {
				log.Printf("Failed to fund account %d (attempt %d): %v", i, j+1, err)
			} else {
				fmt.Printf("✅ Funded account %d (attempt %d)\n", i, j+1)
			}
			time.Sleep(500 * time.Millisecond)
		}
	}

	// Wait for accounts to be created
	fmt.Println("\n⏳ Waiting for accounts to be created...")
	time.Sleep(10 * time.Second)

	// Add credits to accounts
	fmt.Println("\n💳 Adding credits to accounts...")
	for i, acc := range accounts {
		if err := addCreditsToAccount(client, acc); err != nil {
			log.Printf("Failed to add credits to account %d: %v", i, err)
		} else {
			fmt.Printf("✅ Added credits to account %d\n", i)
		}
		time.Sleep(1 * time.Second)
	}

	// Wait for credit transactions to settle
	fmt.Println("\n⏳ Waiting for credits to settle...")
	time.Sleep(5 * time.Second)

	// Run load test
	fmt.Println("\n🔥 Starting load test...")

	var wg sync.WaitGroup
	successCount := int64(0)
	errorCount := int64(0)
	var mu sync.Mutex

	startTime := time.Now()

	// Send transactions concurrently
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(txNum int) {
			defer wg.Done()

			// Pick random sender and receiver
			fromIdx := txNum % len(accounts)
			toIdx := (txNum + 1) % len(accounts)

			err := sendTransaction(client, accounts[fromIdx], accounts[toIdx], 100000) // 0.1 ACME

			mu.Lock()
			if err != nil {
				errorCount++
				log.Printf("❌ Transaction %d failed: %v", txNum, err)
			} else {
				successCount++
				fmt.Printf("✅ Transaction %d succeeded\n", txNum)
			}
			mu.Unlock()
		}(i)

		time.Sleep(200 * time.Millisecond) // Stagger starts
	}

	wg.Wait()
	duration := time.Since(startTime)

	// Results
	fmt.Printf("\n📊 Load Test Results:\n")
	fmt.Printf("Duration: %v\n", duration)
	fmt.Printf("Successful transactions: %d\n", successCount)
	fmt.Printf("Failed transactions: %d\n", errorCount)
	fmt.Printf("Total transactions: %d\n", successCount+errorCount)
	if successCount > 0 {
		fmt.Printf("Success rate: %.1f%%\n", float64(successCount)/float64(successCount+errorCount)*100)
		fmt.Printf("TPS: %.2f\n", float64(successCount)/duration.Seconds())
	}
}
