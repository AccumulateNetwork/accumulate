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
	fmt.Println("🔄 CLI Credit Pattern Test")
	fmt.Println("Testing: Lite account → sends credits → to key pages")
	fmt.Println("Key insight: Lite accounts send credits TO key pages, not purchase credits FOR themselves")
	
	// Create API client
	client := jsonrpc.NewClient("http://127.0.0.1:26660/v3")
	ctx := context.Background()
	
	// Step 1: Create and fund a lite account (this should work)
	fmt.Println("\n💰 Step 1: Creating and funding lite account...")
	liteAccount, err := createAndFundLiteAccount()
	if err != nil {
		log.Fatalf("Failed to create lite account: %v", err)
	}
	
	fmt.Printf("✅ Lite account: %s (%.2f ACME)\n", 
		liteAccount.URL.String()[:40]+"...", float64(liteAccount.Balance)/1000000)
	
	// Step 2: Create an ADI with a key page (this might work if lite account signing works)
	fmt.Println("\n🆔 Step 2: Creating ADI with key page...")
	
	// Generate key for ADI
	adiSeed := make([]byte, 32)
	_, err = rand.Read(adiSeed)
	if err != nil {
		log.Fatalf("Failed to generate ADI seed: %v", err)
	}
	adiKey := ed25519.NewKeyFromSeed(adiSeed)
	adiPubKey := adiKey[32:]
	
	// Create unique ADI name
	adiName := fmt.Sprintf("test-adi-%d.acme", time.Now().Unix())
	
	// Try to create ADI using lite account as authority
	var ts uint64
	env, err := build.Transaction().For(liteAccount.URL).
		CreateIdentity(adiName).WithKey(adiPubKey, protocol.SignatureTypeED25519).WithAuthority(liteAccount.URL).
		SignWith(liteAccount.URL).Version(1).Timestamp(&ts).PrivateKey(liteAccount.Key).
		Done()
	
	if err != nil {
		log.Printf("❌ Failed to build ADI creation: %v", err)
		return
	}
	
	subs, err := client.Submit(ctx, env, v3api.SubmitOptions{})
	if err != nil {
		log.Printf("❌ ADI creation submit failed: %v", err)
		return
	}
	
	adiCreated := true
	for _, sub := range subs {
		if err := sub.Status.AsError(); err != nil {
			log.Printf("❌ ADI creation failed: %v", err)
			adiCreated = false
		}
	}
	
	if !adiCreated {
		fmt.Println("❌ Cannot proceed without ADI creation")
		return
	}
	
	fmt.Printf("✅ ADI created: %s\n", adiName)
	
	// Step 3: Send credits from lite account to ADI key page
	fmt.Println("\n💳 Step 3: Sending credits from lite account to ADI key page...")
	
	// Wait for ADI to be created
	time.Sleep(3 * time.Second)
	
	// ADI key page URL (typically adi.acme/book/1)
	adiURL := accurl.MustParse(adiName)
	keyPageURL := adiURL.JoinPath("book", "1")
	
	// Get network status for oracle price
	ns, err := client.NetworkStatus(ctx, v3api.NetworkStatusOptions{Partition: protocol.Directory})
	if err != nil {
		log.Printf("❌ Failed to get network status: %v", err)
		return
	}
	
	oracle := float64(ns.Oracle.Price) / protocol.AcmeOraclePrecision
	if oracle == 0 {
		oracle = 0.01 // Test price for DevNet
		fmt.Printf("⚠️  Using test oracle price: %.4f ACME per credit\n", oracle)
	}
	
	// Add credits to key page, funded by lite account (using AddCredits)
	creditAmount := int64(1000000) // 1M credits
	var ts2 uint64
	env2, err := build.Transaction().For(liteAccount.URL).
		AddCredits().To(keyPageURL).WithOracle(oracle).Purchase(float64(creditAmount)).
		SignWith(liteAccount.URL).Version(1).Timestamp(&ts2).PrivateKey(liteAccount.Key).
		Done()
	
	if err != nil {
		log.Printf("❌ Failed to build credit transfer: %v", err)
		return
	}
	
	subs2, err := client.Submit(ctx, env2, v3api.SubmitOptions{})
	if err != nil {
		log.Printf("❌ Credit transfer submit failed: %v", err)
		return
	}
	
	creditsSent := true
	for _, sub := range subs2 {
		if err := sub.Status.AsError(); err != nil {
			log.Printf("❌ Credit transfer failed: %v", err)
			creditsSent = false
		}
	}
	
	if creditsSent {
		fmt.Printf("✅ Credits transferred to key page: %s\n", keyPageURL.String())
	}
	
	// Step 4: Now try to use the key page to perform a transaction
	fmt.Println("\n📝 Step 4: Using key page to perform transaction...")
	
	if creditsSent {
		// Wait for credits to settle
		time.Sleep(3 * time.Second)
		
		// Try to write data using the key page as signer
		testData := fmt.Sprintf("Test data written by key page at %s", time.Now().Format(time.RFC3339))
		
		var ts3 uint64
		env3, err := build.Transaction().For(adiURL.JoinPath("data")).
			WriteData().DoubleHash([]byte(testData)).
			SignWith(keyPageURL).Version(1).Timestamp(&ts3).PrivateKey(adiKey).
			Done()
		
		if err != nil {
			log.Printf("❌ Failed to build data write with key page: %v", err)
		} else {
			subs3, err := client.Submit(ctx, env3, v3api.SubmitOptions{})
			if err != nil {
				log.Printf("❌ Key page data write submit failed: %v", err)
			} else {
				success := true
				for _, sub := range subs3 {
					if err := sub.Status.AsError(); err != nil {
						log.Printf("❌ Key page data write failed: %v", err)
						success = false
					}
				}
				
				if success {
					fmt.Println("✅ Key page transaction succeeded!")
				}
			}
		}
	}
	
	fmt.Println("\n🏁 CLI Credit Pattern Test completed!")
	fmt.Println("Key insight: This tests the proper credit flow pattern used by CLI")
}

type LiteAccount struct {
	URL     *accurl.URL
	Key     ed25519.PrivateKey
	Balance int64
}

func createAndFundLiteAccount() (*LiteAccount, error) {
	// Generate key using correct pattern
	seed := make([]byte, 32)
	_, err := rand.Read(seed)
	if err != nil {
		return nil, fmt.Errorf("failed to generate seed: %v", err)
	}
	
	key := ed25519.NewKeyFromSeed(seed)
	
	// Create lite token account URL
	liteURL, err := protocol.LiteTokenAddress(key[32:], protocol.ACME, protocol.SignatureTypeED25519)
	if err != nil {
		return nil, fmt.Errorf("failed to create lite address: %v", err)
	}
	
	// Fund with multiple faucet requests
	totalBalance := int64(0)
	for i := 0; i < 10; i++ { // 100 ACME total
		resp, err := http.Post(
			"http://127.0.0.1:26660/faucet",
			"text/plain",
			strings.NewReader(liteURL.String()),
		)
		if err != nil {
			log.Printf("⚠️  Faucet request %d failed: %v", i+1, err)
			continue
		}
		
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		
		if resp.StatusCode != http.StatusOK {
			log.Printf("⚠️  Faucet request %d failed (status %d): %s", i+1, resp.StatusCode, string(body))
		} else {
			totalBalance += 10000000 // 10 ACME per request
		}
		
		time.Sleep(200 * time.Millisecond)
	}
	
	// Wait for account to settle
	time.Sleep(3 * time.Second)
	
	return &LiteAccount{
		URL:     liteURL,
		Key:     key,
		Balance: totalBalance,
	}, nil
}