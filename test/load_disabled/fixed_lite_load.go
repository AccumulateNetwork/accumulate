package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	v3api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	accurl "gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func main() {
	fmt.Println("🔧 Fixed Lite Account Load Test")
	fmt.Println("Using corrected key generation and avoiding explicit credit purchases")
	fmt.Println("Key insight: Lite accounts should work without explicit credit management")
	
	// Create API client
	client := jsonrpc.NewClient("http://127.0.0.1:26660/v3")
	
	// Create multiple lite accounts using correct pattern
	numAccounts := 5
	accounts := make([]*LiteAccount, numAccounts)
	
	for i := 0; i < numAccounts; i++ {
		account, err := createLiteAccount(client)
		if err != nil {
			log.Fatalf("Failed to create account %d: %v", i, err)
		}
		accounts[i] = account
		
		fmt.Printf("Account %d: %s (Balance: %.2f ACME)\n", 
			i+1, account.URL.String()[:40]+"...", float64(account.Balance)/1000000)
	}
	
	fmt.Printf("✅ Created %d lite accounts, total funded: %.2f ACME\n", 
		numAccounts, float64(getTotalBalance(accounts))/1000000)
	
	// Test simple transactions between accounts
	fmt.Println("\n🔄 Testing simple transactions between lite accounts...")
	
	ctx := context.Background()
	successCount := 0
	totalAttempts := 0
	
	// Test 1: Simple transfers
	for i := 0; i < 10; i++ {
		fromIdx := i % numAccounts
		toIdx := (i + 1) % numAccounts
		
		from := accounts[fromIdx]
		to := accounts[toIdx]
		
		if from.Balance < 2000000 { // Need at least 2 ACME
			continue
		}
		
		amount := int64(1000000) // 1 ACME
		totalAttempts++
		
		// Build transaction without explicit credit handling
		var ts uint64
		env, err := build.Transaction().For(from.URL).
			SendTokens(amount, 0).To(to.URL).
			SignWith(from.URL).Version(1).Timestamp(&ts).PrivateKey(from.Key).
			Done()
		
		if err != nil {
			log.Printf("❌ Failed to build transfer %d: %v", i+1, err)
			continue
		}
		
		// Submit transaction
		subs, err := client.Submit(ctx, env, v3api.SubmitOptions{})
		if err != nil {
			log.Printf("❌ Transfer %d submit failed: %v", i+1, err)
			continue
		}
		
		success := true
		for _, sub := range subs {
			if err := sub.Status.AsError(); err != nil {
				log.Printf("❌ Transfer %d failed: %v", i+1, err)
				success = false
				break
			}
		}
		
		if success {
			successCount++
			from.Balance -= amount
			to.Balance += amount
			fmt.Printf("✅ Transfer %d: %.2f ACME from Account %d to Account %d\n", 
				i+1, float64(amount)/1000000, fromIdx+1, toIdx+1)
		}
	}
	
	fmt.Printf("\n📊 Transfer Results: %d/%d successful (%.1f%%)\n", 
		successCount, totalAttempts, float64(successCount)/float64(totalAttempts)*100)
	
	// Test 2: Data writes (if transfers work)
	if successCount > 0 {
		fmt.Println("\n📝 Testing data write transactions...")
		
		dataSuccesses := 0
		for i := 0; i < 5; i++ {
			account := accounts[i%numAccounts]
			testData := fmt.Sprintf("Load test data entry %d - timestamp %d", i+1, time.Now().Unix())
			
			var ts uint64
			env, err := build.Transaction().For(account.URL).
				WriteData().DoubleHash([]byte(testData)).Scratch().
				SignWith(account.URL).Version(1).Timestamp(&ts).PrivateKey(account.Key).
				Done()
			
			if err != nil {
				log.Printf("❌ Failed to build data write %d: %v", i+1, err)
				continue
			}
			
			subs, err := client.Submit(ctx, env, v3api.SubmitOptions{})
			if err != nil {
				log.Printf("❌ Data write %d submit failed: %v", i+1, err)
				continue
			}
			
			success := true
			for _, sub := range subs {
				if err := sub.Status.AsError(); err != nil {
					log.Printf("❌ Data write %d failed: %v", i+1, err)
					success = false
					break
				}
			}
			
			if success {
				dataSuccesses++
				fmt.Printf("✅ Data write %d successful\n", i+1)
			}
		}
		
		fmt.Printf("📊 Data Write Results: %d/5 successful\n", dataSuccesses)
	}
	
	fmt.Println("\n🏁 Fixed lite account load test completed!")
	fmt.Printf("Key findings: %d transfer successes, total balance: %.2f ACME\n", 
		successCount, float64(getTotalBalance(accounts))/1000000)
}

type LiteAccount struct {
	URL     *accurl.URL
	Key     ed25519.PrivateKey
	Balance int64
}

func createLiteAccount(client *jsonrpc.Client) (*LiteAccount, error) {
	// Generate key using the correct pattern for lite accounts
	seed := make([]byte, 32)
	_, err := rand.Read(seed)
	if err != nil {
		return nil, fmt.Errorf("failed to generate seed: %v", err)
	}
	
	key := ed25519.NewKeyFromSeed(seed)
	
	// Create lite token account URL using public key portion (key[32:])
	liteURL, err := protocol.LiteTokenAddress(key[32:], protocol.ACME, protocol.SignatureTypeED25519)
	if err != nil {
		return nil, fmt.Errorf("failed to create lite address: %v", err)
	}
	
	// Fund account with multiple faucet requests
	totalBalance := int64(0)
	for i := 0; i < 5; i++ { // 50 ACME total
		amount, err := requestFromFaucet(liteURL)
		if err != nil {
			log.Printf("⚠️  Faucet request %d failed: %v", i+1, err)
		} else {
			totalBalance += amount
		}
		time.Sleep(200 * time.Millisecond)
	}
	
	// Wait for account to be created
	time.Sleep(2 * time.Second)
	
	return &LiteAccount{
		URL:     liteURL,
		Key:     key,
		Balance: totalBalance,
	}, nil
}

func requestFromFaucet(accountURL *accurl.URL) (int64, error) {
	resp, err := http.Post(
		"http://127.0.0.1:26660/faucet",
		"text/plain",
		strings.NewReader(accountURL.String()),
	)
	if err != nil {
		return 0, fmt.Errorf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read response: %v", err)
	}
	
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("faucet request failed (status %d): %s", resp.StatusCode, string(body))
	}
	
	// Assume 10 ACME per successful request
	return 10000000, nil
}

func getTotalBalance(accounts []*LiteAccount) int64 {
	total := int64(0)
	for _, account := range accounts {
		total += account.Balance
	}
	return total
}