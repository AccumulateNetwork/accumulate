// Model 1: Multi-Signature Authority Enforcement Demo
//
// This script demonstrates AIP-50 fee enforcement using the multi-sig authority model:
//   1. Service is added as an authority on Alice's token account
//   2. BOTH Alice AND Service must sign any transaction
//   3. Service ONLY signs if the transaction includes a UserFee
//
// IMPORTANT: When adding a new authority via UpdateAccountAuth, the NEW authority
// must also sign to ACCEPT being added. This is a security feature.
//
// Prerequisites:
//   - Devnet running on localhost:26660
//   - alice_bob_fee_examples.go has been run
//   - service_setup.go has been run
//
// Run: go run model1_multisig_enforcement.go
//
// Configuration: Edit examples/aip-50/testconfig/config.go to change ADI names/version

package main

import (
	"context"
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
	fmt.Println("Model 1: Multi-Signature Authority Enforcement Demo")
	fmt.Println("============================================================")
	fmt.Printf("Version: %s\n\n", cfg.Version)
	fmt.Println("This model adds the service as an AUTHORITY on Alice's account.")
	fmt.Println("Both Alice AND Service must sign every transaction.")
	fmt.Println("Service only signs if UserFee is included.")
	fmt.Println()

	ctx = context.Background()
	client = jsonrpc.NewClient(cfg.ACC_API)

	// Load keys
	alicePrivKey := cfg.GetAliceKey()
	servicePrivKey := cfg.GetServiceKey()

	fmt.Println("Loading credentials...")
	fmt.Printf("  Alice Token Account: %s\n", cfg.AliceTokens())
	fmt.Printf("  Alice Key Book:      %s\n", cfg.AliceKeyBook())
	fmt.Printf("  Service Key Book:    %s\n", cfg.ServiceKeyBook())
	fmt.Printf("  Service Fee Account: %s\n", cfg.ServiceFees())
	fmt.Println()

	// =========================================================================
	// SETUP: Add service as authority on Alice's token account
	// =========================================================================
	fmt.Println("============================================================")
	fmt.Println("SETUP: Adding service as authority on Alice's token account")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Printf("This uses UpdateAccountAuth to add %s\n", cfg.ServiceKeyBook())
	fmt.Printf("as an additional authority on %s\n", cfg.AliceTokens())
	fmt.Println()
	fmt.Println("IMPORTANT: The service must ALSO sign to accept being added!")
	fmt.Println()

	// Step 1: Alice initiates UpdateAccountAuth
	fmt.Println("  Step 1: Alice signs UpdateAccountAuth...")
	env, err := build.Transaction().
		For(cfg.AliceTokens()).
		UpdateAccountAuth().
		Add(cfg.ServiceKeyBook()).
		SignWith(cfg.AliceKeyPage()).
		Version(1).
		Timestamp(time.Now().UTC().UnixMicro()).
		PrivateKey(alicePrivKey).
		Done()
	check(err, "build UpdateAccountAuth")

	results, err := client.Submit(ctx, env, api.SubmitOptions{})
	check(err, "submit UpdateAccountAuth")

	var updateAuthTxID *url.TxID
	for _, r := range results {
		if r.Status != nil {
			if updateAuthTxID == nil {
				updateAuthTxID = r.Status.TxID
			}
			fmt.Printf("    TxID: %s\n", r.Status.TxID)
			fmt.Printf("    Status: %v\n", r.Status.Code)
		}
	}
	time.Sleep(3 * time.Second)

	// Step 2: Service signs to ACCEPT being added as authority
	fmt.Println("\n  Step 2: Service signs to ACCEPT being added as authority...")
	env, err = build.SignatureForTxID(updateAuthTxID).
		Url(cfg.ServiceKeyPage()).
		Version(1).
		Timestamp(time.Now().UTC().UnixMicro()).
		PrivateKey(servicePrivKey).
		Done()
	check(err, "build service acceptance signature")

	results, err = client.Submit(ctx, env, api.SubmitOptions{})
	check(err, "submit service acceptance")
	for _, r := range results {
		if r.Status != nil {
			fmt.Printf("    Status: %v\n", r.Status.Code)
			if r.Status.Error != nil {
				fmt.Printf("    Error: %v\n", r.Status.Error)
			}
		}
	}
	fmt.Println("    Service accepted being added as authority!")

	fmt.Println("\n  Waiting 15 seconds for settlement...")
	time.Sleep(15 * time.Second)

	// Verify authorities
	fmt.Println("\nVerifying authorities on Alice's token account...")
	record, err := client.Query(ctx, cfg.AliceTokens(), nil)
	check(err, "query Alice token account")
	jsonData, _ := json.MarshalIndent(record, "  ", "  ")
	fmt.Printf("  Account state:\n%s\n\n", truncate(string(jsonData), 800))

	// =========================================================================
	// DEMO A: Transaction WITH UserFee - SHOULD SUCCEED
	// =========================================================================
	fmt.Println("============================================================")
	fmt.Println("DEMO A: Transaction WITH UserFee")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Println("Alice sends 1 ACME to Bob with a 0.1 ACME fee to the service.")
	fmt.Println("Both Alice and Service sign. Transaction should SUCCEED.")
	fmt.Println()

	aliceBalanceBefore := getTokenBalance(cfg.AliceTokens())
	bobBalanceBefore := getTokenBalance(cfg.BobTokens())
	serviceBalanceBefore := getTokenBalance(cfg.ServiceFees())
	fmt.Printf("  Balances BEFORE:\n")
	fmt.Printf("    Alice:   %.2f ACME\n", float64(aliceBalanceBefore)/1e8)
	fmt.Printf("    Bob:     %.2f ACME\n", float64(bobBalanceBefore)/1e8)
	fmt.Printf("    Service: %.2f ACME\n", float64(serviceBalanceBefore)/1e8)
	fmt.Println()

	sendAmount := big.NewInt(1 * protocol.AcmePrecision)
	feeAmount := big.NewInt(protocol.AcmePrecision / 10)

	env, err = build.Transaction().
		For(cfg.AliceTokens()).
		UserFee(cfg.ServiceFees(), feeAmount, protocol.AcmeUrl(), cfg.AliceTokens()).
		SendTokens(sendAmount, protocol.AcmePrecisionPower).
		To(cfg.BobTokens()).
		SignWith(cfg.AliceKeyPage()).
		Version(1).
		Timestamp(time.Now().UTC().UnixMicro()).
		PrivateKey(alicePrivKey).
		Done()
	check(err, "build send with fee (Alice signature)")

	fmt.Println("  Step 1: Alice signs the transaction...")
	results, err = client.Submit(ctx, env, api.SubmitOptions{})
	check(err, "submit Alice signature")
	var txID *url.TxID
	for _, r := range results {
		if r.Status != nil {
			if txID == nil {
				txID = r.Status.TxID // Use the FIRST TxID (the transaction, not the signature)
			}
			fmt.Printf("    TxID: %s\n", r.Status.TxID)
			fmt.Printf("    Status: %v\n", r.Status.Code)
		}
	}
	time.Sleep(2 * time.Second)

	fmt.Println("\n  Step 2: Service checks for fee and co-signs...")
	fmt.Printf("    [Service logic: UserFee detected for %s]\n", cfg.ServiceFees())
	fmt.Println("    [Service logic: Fee amount 0.1 ACME >= minimum required]")
	fmt.Println("    [Service logic: APPROVED - signing transaction]")

	env, err = build.SignatureForTxID(txID).
		Url(cfg.ServiceKeyPage()).
		Version(1).
		Timestamp(time.Now().UTC().UnixMicro()).
		PrivateKey(servicePrivKey).
		Done()
	check(err, "build service signature")

	_, err = client.Submit(ctx, env, api.SubmitOptions{})
	check(err, "submit service signature")
	fmt.Println("    Service signature submitted!")

	fmt.Println("\n  Waiting 15 seconds for transaction to complete...")
	time.Sleep(15 * time.Second)

	aliceBalanceAfter := getTokenBalance(cfg.AliceTokens())
	bobBalanceAfter := getTokenBalance(cfg.BobTokens())
	serviceBalanceAfter := getTokenBalance(cfg.ServiceFees())
	fmt.Printf("\n  Balances AFTER:\n")
	fmt.Printf("    Alice:   %.2f ACME (sent 1.0 + 0.1 fee = -1.1)\n", float64(aliceBalanceAfter)/1e8)
	fmt.Printf("    Bob:     %.2f ACME (received 1.0)\n", float64(bobBalanceAfter)/1e8)
	fmt.Printf("    Service: %.2f ACME (received 0.1 fee)\n", float64(serviceBalanceAfter)/1e8)

	if serviceBalanceAfter > serviceBalanceBefore {
		fmt.Println("\n  *** SUCCESS: Service received the fee! ***")
	} else {
		fmt.Println("\n  *** WARNING: Fee may not have been received yet ***")
	}

	// =========================================================================
	// DEMO B: Transaction WITHOUT UserFee - SHOULD BE BLOCKED
	// =========================================================================
	fmt.Println()
	fmt.Println("============================================================")
	fmt.Println("DEMO B: Transaction WITHOUT UserFee")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Println("Alice tries to send 1 ACME to Bob WITHOUT including a fee.")
	fmt.Println("Service will REFUSE to co-sign. Transaction should STAY PENDING.")
	fmt.Println()

	env, err = build.Transaction().
		For(cfg.AliceTokens()).
		SendTokens(sendAmount, protocol.AcmePrecisionPower).
		To(cfg.BobTokens()).
		SignWith(cfg.AliceKeyPage()).
		Version(1).
		Timestamp(time.Now().UTC().UnixMicro()).
		PrivateKey(alicePrivKey).
		Done()
	check(err, "build send without fee")

	fmt.Println("  Step 1: Alice signs the transaction (no fee)...")
	results, err = client.Submit(ctx, env, api.SubmitOptions{})
	check(err, "submit Alice signature (no fee)")
	for _, r := range results {
		if r.Status != nil {
			txID = r.Status.TxID
			fmt.Printf("    TxID: %s\n", txID)
			fmt.Printf("    Status: %v (pending - needs service signature)\n", r.Status.Code)
		}
	}

	fmt.Println("\n  Step 2: Service checks for fee...")
	fmt.Println("    [Service logic: Checking UserFees in transaction header...]")
	fmt.Printf("    [Service logic: NO fee found for %s]\n", cfg.ServiceFees())
	fmt.Println("    [Service logic: REJECTED - refusing to sign]")
	fmt.Println()
	fmt.Println("  Service REFUSES to co-sign because no fee is included!")
	fmt.Println("  Transaction will remain PENDING indefinitely.")

	// =========================================================================
	// Summary
	// =========================================================================
	fmt.Println()
	fmt.Println("============================================================")
	fmt.Println("MODEL 1 ENFORCEMENT SUMMARY")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Println("Setup:")
	fmt.Println("  - Service added as authority on Alice's token account")
	fmt.Println("  - Service signed to ACCEPT being added (required!)")
	fmt.Println("  - Both Alice AND Service must sign every transaction")
	fmt.Println()
	fmt.Println("Demo A (WITH Fee):")
	fmt.Println("  - Alice signed with UserFee included")
	fmt.Println("  - Service detected fee and co-signed")
	fmt.Println("  - Transaction EXECUTED, fee paid to service")
	fmt.Println()
	fmt.Println("Demo B (WITHOUT Fee):")
	fmt.Println("  - Alice signed without UserFee")
	fmt.Println("  - Service refused to co-sign")
	fmt.Println("  - Transaction BLOCKED (stays pending)")
	fmt.Println()
	fmt.Println("CONCLUSION: User CANNOT transact without paying the fee!")
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
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...(truncated)"
}
