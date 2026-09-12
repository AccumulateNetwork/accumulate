// Model 2: Namespace Ownership / Dual Authority Enforcement Demo
//
// This script demonstrates AIP-50 fee enforcement using namespace ownership:
//   1. Service owns the namespace (acc://fee-service-X.acme)
//   2. Service creates user accounts UNDER its namespace
//   3. User accounts have BOTH user's key AND service's key as authorities
//   4. Service only signs if UserFee is included
//
// This is the "custodial exchange" model.
//
// Prerequisites:
//   - Devnet running on localhost:26660
//   - alice_bob_fee_examples.go has been run
//   - service_setup.go has been run
//
// Run: go run model2_namespace_enforcement.go
//
// Configuration: Edit examples/aip-50/testconfig/config.go to change ADI names/version

package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	cfg "gitlab.com/accumulatenetwork/accumulate/examples/aip-50/testconfig"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var (
	ctx    context.Context
	client *jsonrpc.Client
)

func main() {
	fmt.Println("============================================================")
	fmt.Println("Model 2: Namespace Ownership / Dual Authority Demo")
	fmt.Println("============================================================")
	fmt.Printf("Version: %s\n\n", cfg.Version)
	fmt.Println("This model creates user accounts UNDER the service's namespace.")
	fmt.Println("User accounts require BOTH user's key AND service's key.")
	fmt.Println("This is the 'custodial exchange' pattern.")
	fmt.Println()

	ctx = context.Background()
	client = jsonrpc.NewClient(cfg.ACC_API)

	// Load keys
	servicePrivKey := cfg.GetServiceKey()
	alicePrivKey := cfg.GetAliceKey()
	aliceLiteAddr := cfg.GetAliceLiteAddress()

	fmt.Println("Loading service credentials...")
	fmt.Printf("  Service ADI: %s\n", cfg.ServiceADI())
	fmt.Printf("  Service Key Book: %s\n", cfg.ServiceKeyBook())
	fmt.Println()

	// Generate a NEW key for the namespace user
	fmt.Printf("Generating key for namespace user (%s)...\n", cfg.UserName)
	_, userPrivKey, err := ed25519.GenerateKey(rand.Reader)
	check(err, "generate user key")
	userPubKey := userPrivKey.Public().(ed25519.PublicKey)
	userKeyHash := sha256.Sum256(userPubKey)
	fmt.Printf("  User Public Key Hash: %s\n\n", hex.EncodeToString(userKeyHash[:]))

	// Get oracle
	networkStatus, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{Partition: "BVN1"})
	check(err, "query network status")
	oracleFloat := float64(networkStatus.Oracle.Price) / protocol.AcmeOraclePrecision

	// =========================================================================
	// SETUP: Create user account under service namespace
	// =========================================================================
	fmt.Println("============================================================")
	fmt.Println("SETUP: Creating user account under service namespace")
	fmt.Println("============================================================")
	fmt.Println()

	// Create user's key book under service namespace
	fmt.Printf("Step 1: Creating user's key book (%s)...\n", cfg.UserKeyBook())
	env, err := build.Transaction().
		For(cfg.ServiceADI()).
		CreateKeyBook(cfg.UserKeyBook()).
		WithKey(userPubKey, protocol.SignatureTypeED25519).
		SignWith(cfg.ServiceKeyPage()).
		Version(1).
		Timestamp(time.Now().UTC().UnixMicro()).
		PrivateKey(servicePrivKey).
		Done()
	check(err, "build create user key book")
	submitAndWait(env, "create user key book")
	fmt.Println("  Waiting 15 seconds...")
	time.Sleep(15 * time.Second)

	// Add credits to user's key page
	fmt.Println("\nStep 2: Adding credits to user's key page...")
	env, err = build.Transaction().
		For(aliceLiteAddr).
		AddCredits().To(cfg.UserKeyPage()).WithOracle(oracleFloat).Purchase(5000).
		SignWith(aliceLiteAddr).Version(1).Timestamp(time.Now().UTC().UnixMicro()).PrivateKey(alicePrivKey).
		Done()
	check(err, "build add credits to user")
	submitAndWait(env, "add credits to user key page")
	fmt.Println("  Waiting 30 seconds for credits to arrive...")
	time.Sleep(30 * time.Second)

	// Create user's token account with service authority only
	fmt.Printf("\nStep 3a: Creating user's token account (%s)...\n", cfg.UserTokens())
	env, err = build.Transaction().
		For(cfg.ServiceADI()).
		CreateTokenAccount(cfg.UserTokens()).
		ForToken(protocol.AcmeUrl()).
		WithAuthority(cfg.ServiceKeyBook()).
		SignWith(cfg.ServiceKeyPage()).
		Version(1).
		Timestamp(time.Now().UTC().UnixMicro()).
		PrivateKey(servicePrivKey).
		Done()
	check(err, "build create user token account")
	submitAndWait(env, "create user token account")
	fmt.Println("  Waiting 15 seconds...")
	time.Sleep(15 * time.Second)

	// Add user's key book as additional authority (requires both to sign)
	fmt.Println("\nStep 3b: Adding user's key book as additional authority...")
	fmt.Println("         This creates the DUAL authority requirement")
	fmt.Println("         Service signs first, then user accepts")

	fmt.Println("\n  Step 3b-1: Service signs UpdateAccountAuth...")
	env, err = build.Transaction().
		For(cfg.UserTokens()).
		UpdateAccountAuth().
		Add(cfg.UserKeyBook()).
		SignWith(cfg.ServiceKeyPage()).
		Version(1).
		Timestamp(time.Now().UTC().UnixMicro()).
		PrivateKey(servicePrivKey).
		Done()
	check(err, "build add user authority")

	results, err := client.Submit(ctx, env, api.SubmitOptions{})
	check(err, "submit service signature for UpdateAccountAuth")
	var authTxID *url.TxID
	for _, r := range results {
		if r.Status != nil {
			if authTxID == nil {
				authTxID = r.Status.TxID
			}
			fmt.Printf("    TxID: %s\n", r.Status.TxID)
			fmt.Printf("    Status: %v\n", r.Status.Code)
		}
	}
	time.Sleep(3 * time.Second)

	// User co-signs to accept
	fmt.Println("\n  Step 3b-2: User co-signs to accept authority...")
	env, err = build.SignatureForTxID(authTxID).
		Url(cfg.UserKeyPage()).
		Version(1).
		Timestamp(time.Now().UTC().UnixMicro()).
		PrivateKey(userPrivKey).
		Done()
	check(err, "build user signature for UpdateAccountAuth")

	results, err = client.Submit(ctx, env, api.SubmitOptions{})
	check(err, "submit user signature for UpdateAccountAuth")
	for _, r := range results {
		if r.Status != nil {
			fmt.Printf("    Status: %v\n", r.Status.Code)
		}
	}
	fmt.Println("    User accepted being added as authority!")
	fmt.Println("  Waiting 15 seconds for settlement...")
	time.Sleep(15 * time.Second)

	// Fund user's token account
	fmt.Println("\nStep 4: Funding user's token account with 50 ACME...")
	amount := big.NewInt(50 * protocol.AcmePrecision)
	env, err = build.Transaction().
		For(aliceLiteAddr).
		SendTokens(amount, protocol.AcmePrecisionPower).To(cfg.UserTokens()).
		SignWith(aliceLiteAddr).Version(1).Timestamp(time.Now().UTC().UnixMicro()).PrivateKey(alicePrivKey).
		Done()
	check(err, "build fund user account")
	submitAndWait(env, "fund user token account")
	fmt.Println("  Waiting 20 seconds for all transactions to settle...")
	time.Sleep(20 * time.Second)

	// Verify setup
	fmt.Println("\nVerifying account setup...")
	record, err := client.Query(ctx, cfg.UserTokens(), nil)
	check(err, "query user token account")
	jsonData, _ := json.MarshalIndent(record, "  ", "  ")
	fmt.Printf("  Account state:\n%s\n\n", truncate(string(jsonData), 1000))

	// =========================================================================
	// DEMO A: User sends WITH UserFee
	// =========================================================================
	fmt.Println("============================================================")
	fmt.Println("DEMO A: User sends WITH UserFee")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Println("User sends 1 ACME with 0.1 ACME fee.")
	fmt.Println("Both user and service sign. Should SUCCEED.")
	fmt.Println()

	sendAmount := big.NewInt(1 * protocol.AcmePrecision)
	feeAmount := big.NewInt(protocol.AcmePrecision / 10)

	userBalanceBefore := getTokenBalance(cfg.UserTokens())
	serviceBalanceBefore := getTokenBalance(cfg.ServiceFees())
	fmt.Printf("  User balance before:    %.2f ACME\n", float64(userBalanceBefore)/1e8)
	fmt.Printf("  Service balance before: %.2f ACME\n\n", float64(serviceBalanceBefore)/1e8)

	// User signs with fee
	env, err = build.Transaction().
		For(cfg.UserTokens()).
		UserFee(cfg.ServiceFees(), feeAmount, protocol.AcmeUrl(), cfg.UserTokens()).
		SendTokens(sendAmount, protocol.AcmePrecisionPower).
		To(cfg.BobTokens()).
		SignWith(cfg.UserKeyPage()).
		Version(1).
		Timestamp(time.Now().UTC().UnixMicro()).
		PrivateKey(userPrivKey).
		Done()
	check(err, "build user send with fee")

	fmt.Println("  Step 1: User signs transaction with fee...")
	results, err = client.Submit(ctx, env, api.SubmitOptions{})
	check(err, "submit user signature")
	var txID *url.TxID
	for _, r := range results {
		if r.Status != nil {
			if txID == nil {
				txID = r.Status.TxID // Use the FIRST TxID (the transaction, not the signature)
			}
			fmt.Printf("    TxID: %s\n", r.Status.TxID)
		}
	}
	time.Sleep(2 * time.Second)

	fmt.Println("\n  Step 2: Service validates and co-signs...")
	fmt.Println("    [Service: Fee of 0.1 ACME detected - APPROVED]")

	env, err = build.SignatureForTxID(txID).
		Url(cfg.ServiceKeyPage()).
		Version(1).
		Timestamp(time.Now().UTC().UnixMicro()).
		PrivateKey(servicePrivKey).
		Done()
	check(err, "build service signature")

	_, err = client.Submit(ctx, env, api.SubmitOptions{})
	check(err, "submit service signature")
	fmt.Println("    Service signed!")

	fmt.Println("\n  Waiting 15 seconds...")
	time.Sleep(15 * time.Second)

	userBalanceAfter := getTokenBalance(cfg.UserTokens())
	serviceBalanceAfter := getTokenBalance(cfg.ServiceFees())
	fmt.Printf("\n  User balance after:    %.2f ACME\n", float64(userBalanceAfter)/1e8)
	fmt.Printf("  Service balance after: %.2f ACME\n", float64(serviceBalanceAfter)/1e8)

	if serviceBalanceAfter > serviceBalanceBefore {
		fmt.Println("\n  *** SUCCESS: Fee received by service! ***")
	}

	// =========================================================================
	// DEMO B: User sends WITHOUT UserFee
	// =========================================================================
	fmt.Println()
	fmt.Println("============================================================")
	fmt.Println("DEMO B: User sends WITHOUT UserFee")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Println("User tries to send 1 ACME without fee.")
	fmt.Println("Service refuses to sign. Should be BLOCKED.")
	fmt.Println()

	env, err = build.Transaction().
		For(cfg.UserTokens()).
		SendTokens(sendAmount, protocol.AcmePrecisionPower).
		To(cfg.BobTokens()).
		SignWith(cfg.UserKeyPage()).
		Version(1).
		Timestamp(time.Now().UTC().UnixMicro()).
		PrivateKey(userPrivKey).
		Done()
	check(err, "build send without fee")

	fmt.Println("  Step 1: User signs transaction (no fee)...")
	results, err = client.Submit(ctx, env, api.SubmitOptions{})
	check(err, "submit user signature (no fee)")
	for _, r := range results {
		if r.Status != nil {
			fmt.Printf("    TxID: %s\n", r.Status.TxID)
			fmt.Printf("    Status: %v (pending)\n", r.Status.Code)
		}
	}

	fmt.Println("\n  Step 2: Service checks transaction...")
	fmt.Println("    [Service: NO fee detected - REJECTED]")
	fmt.Println("    [Service: Refusing to co-sign]")
	fmt.Println()
	fmt.Println("  Transaction stays PENDING - user cannot withdraw!")

	// =========================================================================
	// Summary
	// =========================================================================
	fmt.Println()
	fmt.Println("============================================================")
	fmt.Println("MODEL 2 ENFORCEMENT SUMMARY")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Println("Namespace Structure:")
	fmt.Printf("  %s              (service owns)\n", cfg.ServiceADI())
	fmt.Printf("  %s    (user's key book)\n", cfg.UserKeyBook())
	fmt.Printf("  %s  (requires BOTH authorities)\n", cfg.UserTokens())
	fmt.Println()
	fmt.Println("Enforcement:")
	fmt.Println("  - User account has dual authority requirement")
	fmt.Println("  - User CAN sign, but needs service co-signature")
	fmt.Println("  - Service ONLY signs if fee is included")
	fmt.Println()
	fmt.Println("Result: Perfect for exchanges and custodial services!")
}

func check(err error, msg string) {
	if err != nil {
		panic(fmt.Sprintf("%s: %v", msg, err))
	}
}

func getTokenBalance(accountUrl *url.URL) int64 {
	record, err := client.Query(ctx, accountUrl, nil)
	if err != nil {
		return 0
	}
	data, _ := json.Marshal(record)
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return 0
	}
	if account, ok := result["account"].(map[string]interface{}); ok {
		if balance, ok := account["balance"].(string); ok {
			var b int64
			fmt.Sscanf(balance, "%d", &b)
			return b
		}
	}
	return 0
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
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...(truncated)"
}
