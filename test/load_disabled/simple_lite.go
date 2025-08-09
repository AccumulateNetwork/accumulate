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
	fmt.Println("🧪 Simple Lite Account Test")
	fmt.Println("Testing basic lite account transactions")
	
	// Create API client
	client := jsonrpc.NewClient("http://127.0.0.1:26660/v3")
	
	// Generate key pair
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		log.Fatalf("Failed to generate key pair: %v", err)
	}
	
	// Create lite token account URL
	liteURL, err := protocol.LiteTokenAddress(pubKey, protocol.ACME, protocol.SignatureTypeED25519)
	if err != nil {
		log.Fatalf("Failed to create lite address: %v", err)
	}
	
	fmt.Printf("Created lite account: %s\n", liteURL.String()[:40]+"...")
	
	// Test 1: Fund account using faucet
	fmt.Println("\n🚰 Test 1: Funding account from faucet...")
	err = fundFromFaucet(liteURL)
	if err != nil {
		log.Fatalf("Failed to fund from faucet: %v", err)
	}
	
	// Wait for account to be created
	time.Sleep(3 * time.Second)
	
	// Test 2: Check account exists by querying it
	fmt.Println("\n🔍 Test 2: Querying account...")
	ctx := context.Background()
	account, err := client.Query(ctx, liteURL, nil)
	if err != nil {
		log.Printf("Query failed (expected for new account): %v", err)
	} else {
		fmt.Printf("Account found: %+v\n", account)
	}
	
	// Test 3: Try a simple data write transaction - Version A (original)
	fmt.Println("\n📝 Test 3A: Simple data write (with SignWith)...")
	var ts uint64
	env, err := build.Transaction().For(liteURL).
		WriteData().DoubleHash([]byte("test data")).Scratch().
		SignWith(liteURL).Version(1).Timestamp(&ts).PrivateKey(privKey).
		Done()
	if err != nil {
		log.Printf("Failed to build data write: %v", err)
	} else {
		subs, err := client.Submit(ctx, env, v3api.SubmitOptions{})
		if err != nil {
			log.Printf("Data write submit failed: %v", err)
		} else {
			for _, sub := range subs {
				if err := sub.Status.AsError(); err != nil {
					log.Printf("Data write transaction failed: %v", err)
				} else {
					fmt.Println("✅ Data write succeeded!")
				}
			}
		}
	}

	// Test 3B: Skip for now - try a different approach later
	
	// Test 4: Try credit purchase with oracle price
	fmt.Println("\n💳 Test 4: Credit purchase...")
	
	// Get network status for oracle
	ns, err := client.NetworkStatus(ctx, v3api.NetworkStatusOptions{Partition: protocol.Directory})
	if err != nil {
		log.Printf("Failed to get network status: %v", err)
		return
	}
	
	oracle := float64(ns.Oracle.Price) / protocol.AcmeOraclePrecision
	if oracle == 0 {
		oracle = 0.01 // Test price for DevNet
		fmt.Printf("Using test oracle price: %.4f ACME per credit\n", oracle)
	} else {
		fmt.Printf("Using network oracle price: %.4f ACME per credit\n", oracle)
	}
	
	var ts2 uint64
	env2, err := build.Transaction().For(liteURL).
		AddCredits().To(liteURL).WithOracle(oracle).Purchase(float64(1000)).
		SignWith(liteURL).Version(1).Timestamp(&ts2).PrivateKey(privKey).
		Done()
	if err != nil {
		log.Printf("Failed to build credit purchase: %v", err)
	} else {
		subs2, err := client.Submit(ctx, env2, v3api.SubmitOptions{})
		if err != nil {
			log.Printf("Credit purchase submit failed: %v", err)
		} else {
			for _, sub := range subs2 {
				if err := sub.Status.AsError(); err != nil {
					log.Printf("Credit purchase transaction failed: %v", err)
				} else {
					fmt.Println("✅ Credit purchase succeeded!")
				}
			}
		}
	}
	
	fmt.Println("\n🏁 Simple lite account test completed!")
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
	
	fmt.Printf("✅ Faucet funding successful\n")
	return nil
}