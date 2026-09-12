// Alice and Bob Fee Examples Script
//
// This script creates two ADIs (alice and bob) with token accounts, data accounts,
// credits, and ACME tokens for testing user fees.
//
// Run: go run alice_bob_fee_examples.go
//
// Configuration: Edit examples/aip-50/testconfig/config.go to change ADI names/version

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"

	cfg "gitlab.com/accumulatenetwork/accumulate/examples/aip-50/testconfig"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var (
	oracleFloat float64
	ctx         context.Context
	client      *jsonrpc.Client
)

func main() {
	fmt.Println("============================================================")
	fmt.Println("Alice & Bob Fee Examples Script")
	fmt.Println("============================================================")
	fmt.Printf("API Endpoint: %s\n", cfg.ACC_API)
	fmt.Printf("Version: %s\n", cfg.Version)
	fmt.Printf("Alice ADI: %s\n", cfg.AliceADIName)
	fmt.Printf("Bob ADI: %s\n\n", cfg.BobADIName)

	ctx = context.Background()
	client = jsonrpc.NewClient(cfg.ACC_API)

	// Load keys from config
	privateKey := cfg.GetAliceKey()
	publicKey := cfg.GetAlicePubKey()
	keyHash := sha256.Sum256(publicKey)
	liteAddr := cfg.GetAliceLiteAddress()
	liteId := liteAddr.RootIdentity()

	fmt.Println("Step 1: Using configured ED25519 keys...")
	fmt.Printf("  Public Key Hash: %s\n", hex.EncodeToString(keyHash[:]))
	fmt.Printf("  Lite Token Addr: %s\n", liteAddr)
	fmt.Println()

	// =========================================================================
	// STEP 2: Call faucet to fund the lite account
	// =========================================================================
	fmt.Println("Step 2: Calling faucet 50 times...")
	for i := 0; i < 50; i++ {
		_, err := client.Faucet(ctx, liteAddr, api.FaucetOptions{})
		if err != nil && (i+1)%10 == 0 {
			fmt.Printf("  Faucet call %d failed: %v\n", i+1, err)
		}
		if (i+1)%10 == 0 {
			fmt.Printf("  Faucet call %d/50 complete\n", i+1)
		}
		time.Sleep(200 * time.Millisecond)
	}
	fmt.Println("  Faucet calls complete! Waiting 15 seconds...")
	time.Sleep(15 * time.Second)
	fmt.Println()

	// Query oracle
	fmt.Println("Step 3: Querying network oracle...")
	networkStatus, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{Partition: "BVN1"})
	check(err, "query network status")
	oracleFloat = float64(networkStatus.Oracle.Price) / protocol.AcmeOraclePrecision
	fmt.Printf("  Oracle price: %.4f\n\n", oracleFloat)

	// =========================================================================
	// STEP 4: Purchase credits to lite identity
	// =========================================================================
	fmt.Println("Step 4: Purchasing 5000 credits to lite identity...")
	env, err := build.Transaction().
		For(liteAddr).
		AddCredits().To(liteId).WithOracle(oracleFloat).Purchase(5000).
		SignWith(liteAddr).Version(1).Timestamp(time.Now().UTC().UnixMicro()).PrivateKey(privateKey).
		Done()
	check(err, "build add credits")
	submitAndWait(env, "add credits to lite identity")
	time.Sleep(15 * time.Second)

	// =========================================================================
	// Create Alice ADI
	// =========================================================================
	fmt.Printf("\nStep 5: Creating Alice ADI (%s)...\n", cfg.AliceADIName)
	env, err = build.Transaction().
		For(liteAddr).
		CreateIdentity(cfg.AliceADI()).
		WithKey(publicKey, protocol.SignatureTypeED25519).
		WithKeyBook(cfg.AliceKeyBook()).
		SignWith(liteAddr).Version(1).Timestamp(time.Now().UTC().UnixMicro()).PrivateKey(privateKey).
		Done()
	check(err, "build create Alice identity")
	submitAndWait(env, "create Alice identity")
	time.Sleep(15 * time.Second)

	// Purchase credits to Alice's key page
	fmt.Printf("\nStep 6: Purchasing 2000 credits to %s...\n", cfg.AliceKeyPage())
	env, err = build.Transaction().
		For(liteAddr).
		AddCredits().To(cfg.AliceKeyPage()).WithOracle(oracleFloat).Purchase(2000).
		SignWith(liteAddr).Version(1).Timestamp(time.Now().UTC().UnixMicro()).PrivateKey(privateKey).
		Done()
	check(err, "build add credits to Alice")
	submitAndWait(env, "add credits to Alice key page")
	time.Sleep(15 * time.Second)

	// Create Alice's token account
	fmt.Printf("\nStep 7: Creating Alice's token account (%s)...\n", cfg.AliceTokens())
	env, err = build.Transaction().
		For(cfg.AliceADI()).
		CreateTokenAccount(cfg.AliceTokens()).ForToken(protocol.AcmeUrl()).WithAuthority(cfg.AliceKeyBook()).
		SignWith(cfg.AliceKeyPage()).Version(1).Timestamp(time.Now().UTC().UnixMicro()).PrivateKey(privateKey).
		Done()
	check(err, "build create Alice token account")
	submitAndWait(env, "create Alice token account")
	time.Sleep(15 * time.Second)

	// Create Alice's data account
	fmt.Printf("\nStep 8: Creating Alice's data account (%s/data)...\n", cfg.AliceADIName)
	aliceDataUrl := cfg.AliceADI().JoinPath("data")
	env, err = build.Transaction().
		For(cfg.AliceADI()).
		CreateDataAccount(aliceDataUrl).WithAuthority(cfg.AliceKeyBook()).
		SignWith(cfg.AliceKeyPage()).Version(1).Timestamp(time.Now().UTC().UnixMicro()).PrivateKey(privateKey).
		Done()
	check(err, "build create Alice data account")
	submitAndWait(env, "create Alice data account")
	time.Sleep(15 * time.Second)

	// Send 200 ACME to Alice
	fmt.Printf("\nStep 9: Sending 200 ACME to %s...\n", cfg.AliceTokens())
	amount := big.NewInt(200 * protocol.AcmePrecision)
	env, err = build.Transaction().
		For(liteAddr).
		SendTokens(amount, protocol.AcmePrecisionPower).To(cfg.AliceTokens()).
		SignWith(liteAddr).Version(1).Timestamp(time.Now().UTC().UnixMicro()).PrivateKey(privateKey).
		Done()
	check(err, "build send tokens to Alice")
	submitAndWait(env, "send tokens to Alice")
	time.Sleep(15 * time.Second)

	// =========================================================================
	// Create Bob ADI
	// =========================================================================
	fmt.Printf("\nStep 10: Creating Bob ADI (%s)...\n", cfg.BobADIName)
	env, err = build.Transaction().
		For(liteAddr).
		CreateIdentity(cfg.BobADI()).
		WithKey(publicKey, protocol.SignatureTypeED25519).
		WithKeyBook(cfg.BobKeyBook()).
		SignWith(liteAddr).Version(1).Timestamp(time.Now().UTC().UnixMicro()).PrivateKey(privateKey).
		Done()
	check(err, "build create Bob identity")
	submitAndWait(env, "create Bob identity")
	time.Sleep(15 * time.Second)

	// Purchase credits to Bob's key page
	fmt.Printf("\nStep 11: Purchasing 2000 credits to %s...\n", cfg.BobKeyPage())
	env, err = build.Transaction().
		For(liteAddr).
		AddCredits().To(cfg.BobKeyPage()).WithOracle(oracleFloat).Purchase(2000).
		SignWith(liteAddr).Version(1).Timestamp(time.Now().UTC().UnixMicro()).PrivateKey(privateKey).
		Done()
	check(err, "build add credits to Bob")
	submitAndWait(env, "add credits to Bob key page")
	time.Sleep(15 * time.Second)

	// Create Bob's token account
	fmt.Printf("\nStep 12: Creating Bob's token account (%s)...\n", cfg.BobTokens())
	env, err = build.Transaction().
		For(cfg.BobADI()).
		CreateTokenAccount(cfg.BobTokens()).ForToken(protocol.AcmeUrl()).WithAuthority(cfg.BobKeyBook()).
		SignWith(cfg.BobKeyPage()).Version(1).Timestamp(time.Now().UTC().UnixMicro()).PrivateKey(privateKey).
		Done()
	check(err, "build create Bob token account")
	submitAndWait(env, "create Bob token account")
	time.Sleep(15 * time.Second)

	// Create Bob's data account
	fmt.Printf("\nStep 13: Creating Bob's data account (%s/data)...\n", cfg.BobADIName)
	bobDataUrl := cfg.BobADI().JoinPath("data")
	env, err = build.Transaction().
		For(cfg.BobADI()).
		CreateDataAccount(bobDataUrl).WithAuthority(cfg.BobKeyBook()).
		SignWith(cfg.BobKeyPage()).Version(1).Timestamp(time.Now().UTC().UnixMicro()).PrivateKey(privateKey).
		Done()
	check(err, "build create Bob data account")
	submitAndWait(env, "create Bob data account")
	time.Sleep(15 * time.Second)

	// Send 200 ACME to Bob
	fmt.Printf("\nStep 14: Sending 200 ACME to %s...\n", cfg.BobTokens())
	env, err = build.Transaction().
		For(liteAddr).
		SendTokens(amount, protocol.AcmePrecisionPower).To(cfg.BobTokens()).
		SignWith(liteAddr).Version(1).Timestamp(time.Now().UTC().UnixMicro()).PrivateKey(privateKey).
		Done()
	check(err, "build send tokens to Bob")
	submitAndWait(env, "send tokens to Bob")

	// =========================================================================
	// Summary
	// =========================================================================
	fmt.Println("\n============================================================")
	fmt.Println("Setup complete!")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Printf("Alice ADI: %s\n", cfg.AliceADI())
	fmt.Printf("  Key Page:      %s\n", cfg.AliceKeyPage())
	fmt.Printf("  Token Account: %s\n", cfg.AliceTokens())
	fmt.Printf("  ACME Balance:  200 ACME\n")
	fmt.Println()
	fmt.Printf("Bob ADI: %s\n", cfg.BobADI())
	fmt.Printf("  Key Page:      %s\n", cfg.BobKeyPage())
	fmt.Printf("  Token Account: %s\n", cfg.BobTokens())
	fmt.Printf("  ACME Balance:  200 ACME\n")
}

func check(err error, msg string) {
	if err != nil {
		panic(fmt.Sprintf("%s: %v", msg, err))
	}
}

func submitAndWait(env *messaging.Envelope, desc string) {
	results, err := client.Submit(ctx, env, api.SubmitOptions{})
	if err != nil {
		fmt.Printf("  Error submitting %s: %v\n", desc, err)
		return
	}
	for _, r := range results {
		if r.Status == nil {
			continue
		}
		fmt.Printf("  %s: %s (code: %v)\n", desc, r.Status.TxID, r.Status.Code)
		if r.Status.Error != nil {
			fmt.Printf("    ERROR: %v\n", r.Status.Error)
		}
		if r.Status.Code.Delivered() && r.Status.Error == nil {
			return
		}
		// Poll for completion
		txid := r.Status.TxID
		for i := 0; i < 15; i++ {
			time.Sleep(1 * time.Second)
			txRecord, err := client.Query(ctx, txid.AsUrl(), nil)
			if err != nil {
				continue
			}
			jsonData, _ := json.Marshal(txRecord)
			jsonStr := string(jsonData)
			if strings.Contains(jsonStr, `"delivered"`) || strings.Contains(jsonStr, `"status":"delivered"`) {
				fmt.Printf("    Transaction delivered!\n")
				return
			}
		}
	}
}
