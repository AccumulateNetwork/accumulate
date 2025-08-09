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
	fmt.Println("🎯 Simplest Lite Account Test")
	fmt.Println("Just cryptographic creation + funding + signing")

	// Step 1: Generate private key and derive address (pure crypto)
	seed := make([]byte, 32)
	_, err := rand.Read(seed)
	if err != nil {
		log.Fatalf("Failed to generate seed: %v", err)
	}

	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := privateKey[32:]

	// Derive lite account address and identity
	liteAddr, err := protocol.LiteTokenAddress(publicKey, protocol.ACME, protocol.SignatureTypeED25519)
	if err != nil {
		log.Fatalf("Failed to derive lite address: %v", err)
	}

	// Get the lite identity (authority for signing)
	liteIdentity := liteAddr.Identity()

	fmt.Printf("Generated lite token account: %s\n", liteAddr.String())
	fmt.Printf("Lite identity (signing authority): %s\n", liteIdentity.String())
	fmt.Printf("Public key: %x\n", publicKey)

	// Step 2: Fund the address (this creates the account on-chain)
	fmt.Println("\n💰 Funding the lite account...")
	for i := 0; i < 3; i++ {
		resp, err := http.Post(
			"http://127.0.0.1:26660/faucet",
			"text/plain",
			strings.NewReader(liteAddr.String()),
		)
		if err != nil {
			log.Printf("Faucet request %d failed: %v", i+1, err)
		} else {
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				log.Printf("Failed to read response: %v", err)
			} else if resp.StatusCode == 200 {
				fmt.Printf("✅ Funded successfully (response: %s)\n", string(body)[:50]+"...")
			} else {
				fmt.Printf("❌ Funding failed (status %d): %s\n", resp.StatusCode, string(body))
			}
		}
		time.Sleep(1 * time.Second)
	}

	// Step 3: Wait and query account to verify it exists
	fmt.Println("\n🔍 Waiting and checking if account exists...")
	time.Sleep(5 * time.Second)

	client := jsonrpc.NewClient("http://127.0.0.1:26660/v3")
	ctx := context.Background()

	account, err := client.Query(ctx, liteAddr, nil)
	if err != nil {
		fmt.Printf("⚠️  Query failed: %v\n", err)
		fmt.Println("Account might not exist yet, but let's try signing anyway...")
	} else {
		fmt.Printf("✅ Account exists: %+v\n", account)
	}

	// Step 4: Try the simplest possible transaction (query the account itself)
	fmt.Println("\n📝 Testing simplest possible transaction...")

	// Create target address for a simple transfer
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

	// Build the simplest transaction: send 1 ACME
	var timestamp uint64 = uint64(time.Now().UnixMilli())

	env, err := build.Transaction().
		For(liteAddr).
		Body(&protocol.SendTokens{
			To: []*protocol.TokenRecipient{{
				Url:    targetAddr,
				Amount: *big.NewInt(1000000), // 1 ACME
			}},
		}).
		SignWith(liteIdentity).Version(1).Timestamp(&timestamp).PrivateKey(privateKey).
		Done()

	if err != nil {
		log.Printf("❌ Failed to build transaction: %v", err)
		return
	}

	fmt.Printf("✅ Transaction built successfully\n")
	fmt.Printf("Transaction ID: %x\n", env.Transaction[0].ID().Hash())
	fmt.Printf("Signer: %s\n", liteIdentity.String())

	// Submit transaction
	fmt.Println("📤 Submitting transaction...")
	subs, err := client.Submit(ctx, env, v3api.SubmitOptions{})
	if err != nil {
		log.Printf("❌ Submit failed: %v", err)
		return
	}

	fmt.Printf("✅ Submit returned %d results\n", len(subs))

	// Check results
	allSuccess := true
	for i, sub := range subs {
		if err := sub.Status.AsError(); err != nil {
			log.Printf("❌ Result %d failed: %v", i, err)
			allSuccess = false
		} else {
			fmt.Printf("✅ Result %d succeeded\n", i)
		}
	}

	if allSuccess {
		fmt.Println("🎉 SUCCESS! Lite account signing works!")
	} else {
		fmt.Println("❌ Transaction failed despite proper setup")
	}
}
