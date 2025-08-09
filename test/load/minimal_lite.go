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
	fmt.Println("🧪 Minimal Lite Account Transaction Test")
	fmt.Println("Testing if lite accounts can transact without explicit credit purchase")
	
	// Create API client
	client := jsonrpc.NewClient("http://127.0.0.1:26660/v3")
	
	// Generate key pairs using the same pattern as the build test
	seed1 := make([]byte, 32)
	_, err := rand.Read(seed1)
	if err != nil {
		log.Fatalf("Failed to generate seed 1: %v", err)
	}
	
	seed2 := make([]byte, 32)  
	_, err = rand.Read(seed2)
	if err != nil {
		log.Fatalf("Failed to generate seed 2: %v", err)
	}
	
	key1 := ed25519.NewKeyFromSeed(seed1)
	key2 := ed25519.NewKeyFromSeed(seed2)
	
	// Create lite token account URLs using public key portion (key[32:])
	liteURL1, err := protocol.LiteTokenAddress(key1[32:], protocol.ACME, protocol.SignatureTypeED25519)
	if err != nil {
		log.Fatalf("Failed to create lite address 1: %v", err)
	}
	
	liteURL2, err := protocol.LiteTokenAddress(key2[32:], protocol.ACME, protocol.SignatureTypeED25519)
	if err != nil {
		log.Fatalf("Failed to create lite address 2: %v", err)
	}
	
	fmt.Printf("Account 1: %s\n", liteURL1.String()[:40]+"...")
	fmt.Printf("Account 2: %s\n", liteURL2.String()[:40]+"...")
	
	// Fund Account 1 with significant ACME
	fmt.Println("\n💰 Funding Account 1 with ACME...")
	for i := 0; i < 10; i++ {
		err = fundFromFaucet(liteURL1)
		if err != nil {
			log.Printf("Faucet request %d failed: %v", i+1, err)
		} else {
			fmt.Printf(".")
		}
		time.Sleep(500 * time.Millisecond)
	}
	fmt.Println(" Done!")
	
	// Wait for account to be created and settle
	fmt.Println("⏰ Waiting for account to settle...")
	time.Sleep(5 * time.Second)
	
	// Test 1: Simple token transfer (lite to lite)
	fmt.Println("\n💸 Test 1: Simple token transfer (5 ACME from Account 1 to Account 2)...")
	ctx := context.Background()
	
	var ts uint64
	env, err := build.Transaction().For(liteURL1).
		SendTokens(5000000, 0).To(liteURL2).  // 5 ACME
		SignWith(liteURL1).Version(1).Timestamp(&ts).PrivateKey(key1).
		Done()
	
	if err != nil {
		log.Printf("❌ Failed to build token transfer: %v", err)
	} else {
		subs, err := client.Submit(ctx, env, v3api.SubmitOptions{})
		if err != nil {
			log.Printf("❌ Token transfer submit failed: %v", err)
		} else {
			success := true
			for _, sub := range subs {
				if err := sub.Status.AsError(); err != nil {
					log.Printf("❌ Token transfer failed: %v", err)
					success = false
				}
			}
			if success {
				fmt.Println("✅ Token transfer succeeded!")
			}
		}
	}
	
	// Test 2: Data write transaction
	fmt.Println("\n📝 Test 2: Data write transaction...")
	var ts2 uint64
	env2, err := build.Transaction().For(liteURL1).
		WriteData().DoubleHash([]byte("minimal test data")).Scratch().
		SignWith(liteURL1).Version(1).Timestamp(&ts2).PrivateKey(key1).
		Done()
	
	if err != nil {
		log.Printf("❌ Failed to build data write: %v", err)
	} else {
		subs2, err := client.Submit(ctx, env2, v3api.SubmitOptions{})
		if err != nil {
			log.Printf("❌ Data write submit failed: %v", err)
		} else {
			success := true
			for _, sub := range subs2 {
				if err := sub.Status.AsError(); err != nil {
					log.Printf("❌ Data write failed: %v", err)
					success = false
				}
			}
			if success {
				fmt.Println("✅ Data write succeeded!")
			}
		}
	}
	
	// Test 3: Try smaller token transfer from Account 2 back to Account 1
	fmt.Println("\n🔄 Test 3: Return transfer (1 ACME from Account 2 to Account 1)...")
	time.Sleep(2 * time.Second)  // Give time for first transfer to settle
	
	var ts3 uint64
	env3, err := build.Transaction().For(liteURL2).
		SendTokens(1000000, 0).To(liteURL1).  // 1 ACME
		SignWith(liteURL2).Version(1).Timestamp(&ts3).PrivateKey(key2).
		Done()
	
	if err != nil {
		log.Printf("❌ Failed to build return transfer: %v", err)
	} else {
		subs3, err := client.Submit(ctx, env3, v3api.SubmitOptions{})
		if err != nil {
			log.Printf("❌ Return transfer submit failed: %v", err)
		} else {
			success := true
			for _, sub := range subs3 {
				if err := sub.Status.AsError(); err != nil {
					log.Printf("❌ Return transfer failed: %v", err)
					success = false
				}
			}
			if success {
				fmt.Println("✅ Return transfer succeeded!")
			}
		}
	}
	
	fmt.Println("\n🏁 Minimal lite account test completed!")
	fmt.Println("Key insight: Testing if lite accounts work WITHOUT explicit credit purchases")
}

func fundFromFaucet(accountURL *accurl.URL) error {
	resp, err := http.Post(
		"http://127.0.0.1:26660/faucet",
		"text/plain",
		strings.NewReader(accountURL.String()),
	)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %v", err)
	}
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("faucet request failed (status %d): %s", resp.StatusCode, string(body))
	}
	
	return nil
}