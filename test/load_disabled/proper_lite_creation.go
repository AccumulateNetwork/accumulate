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
	fmt.Println("🔧 Proper Lite Account Creation Test")
	fmt.Println("Step 1: Create private key and lite address")
	fmt.Println("Step 2: Create lite token account with CreateLiteTokenAccount()")  
	fmt.Println("Step 3: Fund it and test transactions")
	
	// Create API client
	client := jsonrpc.NewClient("http://127.0.0.1:26660/v3")
	ctx := context.Background()
	
	// Step 1: Generate key and derive lite address
	fmt.Println("\n🔑 Step 1: Generating key and lite address...")
	
	seed := make([]byte, 32)
	_, err := rand.Read(seed)
	if err != nil {
		log.Fatalf("Failed to generate seed: %v", err)
	}
	
	key := ed25519.NewKeyFromSeed(seed)
	pubKey := key[32:]
	
	// Create lite token address
	liteAddr, err := protocol.LiteTokenAddress(pubKey, protocol.ACME, protocol.SignatureTypeED25519)
	if err != nil {
		log.Fatalf("Failed to create lite address: %v", err)
	}
	
	fmt.Printf("✅ Generated lite address: %s\n", liteAddr.String()[:50]+"...")
	
	// Step 2: Create the lite token account explicitly
	fmt.Println("\n🏗️  Step 2: Creating lite token account...")
	
	// But wait - who signs the CreateLiteTokenAccount transaction?
	// Looking at the test, it's signed by a signer (either a page or another lite account)
	// We need a funded account to sign the creation transaction
	
	// Let's first create a temporary account using faucet (this will work for simple operations)
	tempSeed := make([]byte, 32)
	_, err = rand.Read(tempSeed)
	if err != nil {
		log.Fatalf("Failed to generate temp seed: %v", err)
	}
	
	tempKey := ed25519.NewKeyFromSeed(tempSeed)
	tempAddr, err := protocol.LiteTokenAddress(tempKey[32:], protocol.ACME, protocol.SignatureTypeED25519)
	if err != nil {
		log.Fatalf("Failed to create temp address: %v", err)
	}
	
	// Fund temp account via faucet
	fmt.Printf("💰 Funding temporary signer account: %s...\n", tempAddr.String()[:40]+"...")
	for i := 0; i < 5; i++ {
		resp, err := http.Post(
			"http://127.0.0.1:26660/faucet",
			"text/plain",
			strings.NewReader(tempAddr.String()),
		)
		if err != nil {
			log.Printf("Temp faucet request %d failed: %v", i+1, err)
		} else {
			_, _ = io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode == 200 {
				fmt.Print(".")
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	fmt.Println(" Done!")
	
	// Wait for temp account to settle
	time.Sleep(3 * time.Second)
	
	// Now try to create the proper lite token account
	fmt.Printf("🏗️  Creating lite token account %s...\n", liteAddr.String()[:40]+"...")
	
	var timestamp uint64
	env, err := build.Transaction().For(liteAddr).
		CreateLiteTokenAccount().
		SignWith(tempAddr).Version(1).Timestamp(&timestamp).PrivateKey(tempKey).
		Done()
	
	if err != nil {
		log.Printf("❌ Failed to build CreateLiteTokenAccount: %v", err)
		return
	}
	
	subs, err := client.Submit(ctx, env, v3api.SubmitOptions{})
	if err != nil {
		log.Printf("❌ CreateLiteTokenAccount submit failed: %v", err)
		return
	}
	
	created := true
	for _, sub := range subs {
		if err := sub.Status.AsError(); err != nil {
			log.Printf("❌ CreateLiteTokenAccount failed: %v", err)
			created = false
		}
	}
	
	if !created {
		fmt.Println("❌ Cannot proceed without account creation")
		return
	}
	
	fmt.Println("✅ Lite token account created successfully!")
	
	// Step 3: Now fund the created account
	fmt.Println("\n💰 Step 3: Funding the created lite account...")
	
	time.Sleep(2 * time.Second) // Let creation settle
	
	for i := 0; i < 3; i++ {
		resp, err := http.Post(
			"http://127.0.0.1:26660/faucet",
			"text/plain",
			strings.NewReader(liteAddr.String()),
		)
		if err != nil {
			log.Printf("Faucet request %d failed: %v", i+1, err)
		} else {
			_, _ = io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode == 200 {
				fmt.Print(".")
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	fmt.Println(" Funded!")
	
	// Wait for funding to settle
	time.Sleep(3 * time.Second)
	
	// Step 4: Test transactions with the properly created account
	fmt.Println("\n🧪 Step 4: Testing transaction with properly created lite account...")
	
	// Create target account for transfer
	targetSeed := make([]byte, 32)
	_, err = rand.Read(targetSeed)
	if err != nil {
		log.Fatalf("Failed to generate target seed: %v", err)
	}
	
	targetKey := ed25519.NewKeyFromSeed(targetSeed)
	targetAddr, err := protocol.LiteTokenAddress(targetKey[32:], protocol.ACME, protocol.SignatureTypeED25519)
	if err != nil {
		log.Fatalf("Failed to create target address: %v", err)
	}
	
	// Try token transfer
	var timestamp2 uint64
	env2, err := build.Transaction().
		For(liteAddr).
		Body(&protocol.SendTokens{
			To: []*protocol.TokenRecipient{{
				Url:    targetAddr,
				Amount: *big.NewInt(1000000), // 1 ACME
			}},
		}).
		SignWith(liteAddr).Version(1).Timestamp(&timestamp2).PrivateKey(key).
		Done()
	
	if err != nil {
		log.Printf("❌ Failed to build test transaction: %v", err)
	} else {
		subs2, err := client.Submit(ctx, env2, v3api.SubmitOptions{})
		if err != nil {
			log.Printf("❌ Test transaction submit failed: %v", err)
		} else {
			success := true
			for _, sub := range subs2 {
				if err := sub.Status.AsError(); err != nil {
					log.Printf("❌ Test transaction failed: %v", err)
					success = false
				}
			}
			if success {
				fmt.Println("✅ Test transaction succeeded!")
				fmt.Println("🎉 Properly created lite account works for transactions!")
			}
		}
	}
	
	fmt.Println("\n🏁 Proper lite account creation test completed!")
}