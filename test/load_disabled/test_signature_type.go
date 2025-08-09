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
	"time"

	v3api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func main() {
	fmt.Println("🔬 Testing Signature Type Approach")
	fmt.Println("Based on working E2E test patterns")
	
	// Create API client
	client := jsonrpc.NewClient("http://127.0.0.1:26660/v3")
	ctx := context.Background()
	
	// Create lite account using correct pattern
	seed := make([]byte, 32)
	_, err := rand.Read(seed)
	if err != nil {
		log.Fatalf("Failed to generate seed: %v", err)
	}
	
	alice := ed25519.NewKeyFromSeed(seed)
	aliceUrl, err := protocol.LiteTokenAddress(alice[32:], protocol.ACME, protocol.SignatureTypeED25519)
	if err != nil {
		log.Fatalf("Failed to create lite address: %v", err)
	}
	
	// Fund account
	fmt.Println("💰 Funding account...")
	for i := 0; i < 5; i++ {
		resp, err := http.Post(
			"http://127.0.0.1:26660/faucet",
			"text/plain",
			strings.NewReader(aliceUrl.String()),
		)
		if err != nil {
			log.Printf("Faucet request %d failed: %v", i+1, err)
		} else {
			_, _ = io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode == 200 {
				fmt.Print(".")
			} else {
				fmt.Printf("X")
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	fmt.Println(" Done!")
	
	// Wait for settlement
	time.Sleep(3 * time.Second)
	
	// Create second account for transfer target
	seed2 := make([]byte, 32)
	_, err = rand.Read(seed2)
	if err != nil {
		log.Fatalf("Failed to generate seed2: %v", err)
	}
	
	bob := ed25519.NewKeyFromSeed(seed2)
	bobUrl, err := protocol.LiteTokenAddress(bob[32:], protocol.ACME, protocol.SignatureTypeED25519)
	if err != nil {
		log.Fatalf("Failed to create bob address: %v", err)
	}
	
	fmt.Printf("Alice: %s\n", aliceUrl.String()[:40]+"...")
	fmt.Printf("Bob:   %s\n", bobUrl.String()[:40]+"...")
	
	// Test 1: Using E2E test pattern with Body() and SignatureTypeLegacyED25519
	fmt.Println("\n🧪 Test 1: E2E pattern with LegacyED25519...")
	
	var timestamp uint64
	env1, err := build.Transaction().
		For(aliceUrl).
		Body(&protocol.SendTokens{
			To: []*protocol.TokenRecipient{{
				Url:    bobUrl,
				Amount: *big.NewInt(1000000), // 1 ACME
			}},
		}).
		SignWith(aliceUrl).Version(1).Timestamp(&timestamp).PrivateKey(alice).Type(protocol.SignatureTypeLegacyED25519).
		Done()
	
	if err != nil {
		log.Printf("❌ Failed to build E2E pattern transaction: %v", err)
	} else {
		subs1, err := client.Submit(ctx, env1, v3api.SubmitOptions{})
		if err != nil {
			log.Printf("❌ E2E pattern submit failed: %v", err)
		} else {
			success := true
			for _, sub := range subs1 {
				if err := sub.Status.AsError(); err != nil {
					log.Printf("❌ E2E pattern failed: %v", err)
					success = false
				}
			}
			if success {
				fmt.Println("✅ E2E pattern succeeded!")
			}
		}
	}
	
	// Test 2: Try without SignatureTypeLegacyED25519
	fmt.Println("\n🧪 Test 2: E2E pattern without LegacyED25519...")
	
	var timestamp2 uint64
	env2, err := build.Transaction().
		For(aliceUrl).
		Body(&protocol.SendTokens{
			To: []*protocol.TokenRecipient{{
				Url:    bobUrl,
				Amount: *big.NewInt(500000), // 0.5 ACME
			}},
		}).
		SignWith(aliceUrl).Version(1).Timestamp(&timestamp2).PrivateKey(alice).
		Done()
	
	if err != nil {
		log.Printf("❌ Failed to build pattern 2 transaction: %v", err)
	} else {
		subs2, err := client.Submit(ctx, env2, v3api.SubmitOptions{})
		if err != nil {
			log.Printf("❌ Pattern 2 submit failed: %v", err)
		} else {
			success := true
			for _, sub := range subs2 {
				if err := sub.Status.AsError(); err != nil {
					log.Printf("❌ Pattern 2 failed: %v", err)
					success = false
				}
			}
			if success {
				fmt.Println("✅ Pattern 2 succeeded!")
			}
		}
	}
	
	// Test 3: Try our original builder pattern for comparison
	fmt.Println("\n🧪 Test 3: Original builder pattern...")
	
	var timestamp3 uint64
	env3, err := build.Transaction().For(aliceUrl).
		SendTokens(250000, 0).To(bobUrl). // 0.25 ACME
		SignWith(aliceUrl).Version(1).Timestamp(&timestamp3).PrivateKey(alice).
		Done()
	
	if err != nil {
		log.Printf("❌ Failed to build original pattern transaction: %v", err)
	} else {
		subs3, err := client.Submit(ctx, env3, v3api.SubmitOptions{})
		if err != nil {
			log.Printf("❌ Original pattern submit failed: %v", err)
		} else {
			success := true
			for _, sub := range subs3 {
				if err := sub.Status.AsError(); err != nil {
					log.Printf("❌ Original pattern failed: %v", err)
					success = false
				}
			}
			if success {
				fmt.Println("✅ Original pattern succeeded!")
			}
		}
	}
	
	fmt.Println("\n🏁 Signature type test completed!")
}