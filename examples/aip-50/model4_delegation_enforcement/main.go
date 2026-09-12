// Model 4: Delegated Signature Enforcement Demo
//
// This script demonstrates AIP-50 fee enforcement using delegation:
//   1. User adds service as a DELEGATE on their key page
//   2. Service can sign transactions on behalf of the user
//   3. Service ALWAYS includes UserFee when creating transactions
//
// Note: Pure delegation alone doesn't PREVENT user from bypassing.
// For full enforcement, combine with Model 1 (multi-sig).
//
// Prerequisites:
//   - Devnet running on localhost:26660
//   - alice_bob_fee_examples.go has been run
//   - service_setup.go has been run
//
// Run: go run model4_delegation_enforcement.go
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
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var (
	ctx    context.Context
	client *jsonrpc.Client
)

func main() {
	fmt.Println("============================================================")
	fmt.Println("Model 4: Delegated Signature Enforcement Demo")
	fmt.Println("============================================================")
	fmt.Printf("Version: %s\n\n", cfg.Version)
	fmt.Println("In this model, the user DELEGATES signing authority to the service.")
	fmt.Println("The service creates and signs transactions on the user's behalf,")
	fmt.Println("ALWAYS including UserFee in every transaction.")
	fmt.Println()

	ctx = context.Background()
	client = jsonrpc.NewClient(cfg.ACC_API)

	// Load keys
	alicePrivKey := cfg.GetAliceKey()
	servicePrivKey := cfg.GetServiceKey()

	fmt.Println("Loading credentials...")
	fmt.Printf("  User (Bob) Token Account: %s\n", cfg.BobTokens())
	fmt.Printf("  User (Bob) Key Page:      %s\n", cfg.BobKeyPage())
	fmt.Printf("  Service Key Book:         %s\n", cfg.ServiceKeyBook())
	fmt.Printf("  Service Fee Account:      %s\n", cfg.ServiceFees())
	fmt.Println()

	// =========================================================================
	// SETUP: Add service as delegate on Bob's key page
	// =========================================================================
	fmt.Println("============================================================")
	fmt.Println("SETUP: Adding service as delegate on Bob's key page")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Println("Using UpdateKeyPage to add service's key as a delegate.")
	fmt.Println("This allows the service to sign on behalf of Bob.")
	fmt.Println()
	fmt.Println("IMPORTANT: The service must sign to ACCEPT being added as delegate!")
	fmt.Println()

	// Step 1: Bob initiates UpdateKeyPage to add service as delegate
	// NOTE: Delegate entry points to service's key BOOK (any page in that book can sign)
	// This creates an entry: { "delegate": "acc://fee-service-X.acme/book" }
	fmt.Println("  Step 1: Bob signs UpdateKeyPage to add service as delegate...")
	env, err := build.Transaction().
		For(cfg.BobKeyPage()).
		UpdateKeyPage().
		Add().
		Entry().
		Owner(cfg.ServiceKeyBook()). // Delegate entry points to service's key BOOK
		FinishEntry().
		FinishOperation().
		SignWith(cfg.BobKeyPage()).
		Version(1).
		Timestamp(time.Now().UTC().UnixMicro()).
		PrivateKey(alicePrivKey). // Bob uses same key as Alice in our setup
		Done()
	check(err, "build UpdateKeyPage")

	results, err := client.Submit(ctx, env, api.SubmitOptions{})
	check(err, "submit UpdateKeyPage")
	var updateKeyTxID *url.TxID
	for _, r := range results {
		if r.Status != nil {
			if updateKeyTxID == nil {
				updateKeyTxID = r.Status.TxID
			}
			fmt.Printf("    TxID: %s\n", r.Status.TxID)
			fmt.Printf("    Status: %v\n", r.Status.Code)
		}
	}
	time.Sleep(3 * time.Second)

	// Step 2: Service signs to accept being added as delegate
	fmt.Println("\n  Step 2: Service signs to ACCEPT being added as delegate...")
	fmt.Printf("    Signing TxID: %s\n", updateKeyTxID)
	fmt.Printf("    Service signer: %s\n", cfg.ServiceKeyPage())
	env, err = build.SignatureForTxID(updateKeyTxID).
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
			fmt.Printf("    Acceptance TxID: %s\n", r.Status.TxID)
			fmt.Printf("    Status: %v\n", r.Status.Code)
		}
	}

	fmt.Println("\n  Waiting 10 seconds for settlement...")
	time.Sleep(10 * time.Second)

	// Query the UpdateKeyPage tx to see its final status
	fmt.Println("\n  Checking UpdateKeyPage transaction status...")
	record, err := client.Query(ctx, updateKeyTxID.AsUrl(), nil)
	if err != nil {
		fmt.Printf("    Error querying tx: %v\n", err)
	} else {
		data, _ := json.Marshal(record)
		var result map[string]interface{}
		json.Unmarshal(data, &result)
		if status, ok := result["status"].(map[string]interface{}); ok {
			fmt.Printf("    UpdateKeyPage status: %v\n", status["code"])
			if signers, ok := status["signers"].([]interface{}); ok {
				fmt.Printf("    Number of signers: %d\n", len(signers))
			}
		}
	}

	fmt.Println("\n  Waiting 5 more seconds...")
	time.Sleep(5 * time.Second)

	// Bob's key page version is 2 after UpdateKeyPage adds the delegate entry
	// (version increments from 1 to 2 when the key page is modified)
	bobKeyPageVersion := uint64(2)
	fmt.Printf("  Bob's key page version: %d (after UpdateKeyPage)\n\n", bobKeyPageVersion)

	sendAmount := big.NewInt(1 * protocol.AcmePrecision)
	feeAmount := big.NewInt(protocol.AcmePrecision / 10)

	// =========================================================================
	// DEMO A: Service signs as delegate WITH fee
	// =========================================================================
	fmt.Println("============================================================")
	fmt.Println("DEMO A: Service creates transaction on behalf of user")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Println("User requests a transfer through the service's API.")
	fmt.Println("Service builds the transaction WITH fee and signs as delegate.")
	fmt.Println()

	bobBalanceBefore := getTokenBalance(cfg.BobTokens())
	aliceBalanceBefore := getTokenBalance(cfg.AliceTokens())
	serviceBalanceBefore := getTokenBalance(cfg.ServiceFees())

	fmt.Printf("  Balances BEFORE:\n")
	fmt.Printf("    Bob (sender):  %.2f ACME\n", float64(bobBalanceBefore)/1e8)
	fmt.Printf("    Alice (recv):  %.2f ACME\n", float64(aliceBalanceBefore)/1e8)
	fmt.Printf("    Service (fee): %.2f ACME\n\n", float64(serviceBalanceBefore)/1e8)

	fmt.Println("  [Service API receives request: 'Send 1 ACME to Alice']")
	fmt.Println("  [Service builds transaction with 0.1 ACME fee]")
	fmt.Println("  [Service signs with DelegatedSignature on behalf of Bob's authority]")
	fmt.Println()

	fmt.Printf("  Delegator = %s (Bob's key BOOK)\n", cfg.BobKeyBook())
	fmt.Printf("  Signer    = %s (service's key PAGE)\n\n", cfg.ServiceKeyPage())

	// Service creates a DelegatedSignature on behalf of Bob's authority
	// - Delegator = Bob's key BOOK (the authority that delegated)
	// - Signer = Service's key PAGE (the actual signer)
	// Version(1) = service's key page version (the signer)
	env, err = build.Transaction().
		For(cfg.BobTokens()).
		UserFee(cfg.ServiceFees(), feeAmount, protocol.AcmeUrl(), cfg.BobTokens()).
		SendTokens(sendAmount, protocol.AcmePrecisionPower).
		To(cfg.AliceTokens()).
		SignWith(cfg.ServiceKeyPage()).
		Delegator(cfg.BobKeyBook()). // Bob's authority (key BOOK) delegated to service
		Version(1).                  // Service's key page version (the signer)
		Timestamp(time.Now().UTC().UnixMicro()).
		PrivateKey(servicePrivKey).
		Done()
	check(err, "build delegated transaction with fee")

	fmt.Println("  Submitting delegated transaction...")
	results, err = client.Submit(ctx, env, api.SubmitOptions{})
	check(err, "submit delegated transaction")
	for _, r := range results {
		if r.Status != nil {
			fmt.Printf("    TxID: %s\n", r.Status.TxID)
			fmt.Printf("    Status: %v\n", r.Status.Code)
		}
	}

	fmt.Println("\n  Waiting 15 seconds for completion...")
	time.Sleep(15 * time.Second)

	bobBalanceAfter := getTokenBalance(cfg.BobTokens())
	aliceBalanceAfter := getTokenBalance(cfg.AliceTokens())
	serviceBalanceAfter := getTokenBalance(cfg.ServiceFees())

	fmt.Printf("\n  Balances AFTER:\n")
	fmt.Printf("    Bob (sender):  %.2f ACME (sent 1.0 + 0.1 fee)\n", float64(bobBalanceAfter)/1e8)
	fmt.Printf("    Alice (recv):  %.2f ACME (received 1.0)\n", float64(aliceBalanceAfter)/1e8)
	fmt.Printf("    Service (fee): %.2f ACME (received 0.1 fee)\n", float64(serviceBalanceAfter)/1e8)

	demoASuccess := aliceBalanceAfter > aliceBalanceBefore
	if demoASuccess {
		fmt.Println("\n  *** SUCCESS: Service fee collected via delegated signature! ***")
	} else {
		fmt.Println("\n  *** PENDING: Delegated transaction did NOT complete ***")
	}

	// =========================================================================
	// DEMO B: Limitation - User CAN bypass (pure delegation doesn't prevent)
	// =========================================================================
	fmt.Println()
	fmt.Println("============================================================")
	fmt.Println("DEMO B: Limitation - User CAN sign directly")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Println("IMPORTANT: Pure delegation alone doesn't PREVENT user bypass!")
	fmt.Println("User still has their own key and can sign without fee.")
	fmt.Println()
	fmt.Println("For FULL enforcement, combine delegation with Model 1:")
	fmt.Println("  1. Add service as delegate (convenience)")
	fmt.Println("  2. Add service as authority (enforcement)")
	fmt.Println()

	// User signs directly without fee
	// Use Bob's current key page version (incremented after UpdateKeyPage)
	fmt.Println("  Demonstrating: Bob signs directly without fee...")
	env, err = build.Transaction().
		For(cfg.BobTokens()).
		SendTokens(sendAmount, protocol.AcmePrecisionPower).
		To(cfg.AliceTokens()).
		SignWith(cfg.BobKeyPage()).
		Version(bobKeyPageVersion). // Use Bob's current key page version
		Timestamp(time.Now().UTC().UnixMicro()).
		PrivateKey(alicePrivKey). // Bob's key (same as Alice in our setup)
		Done()
	check(err, "build direct transaction without fee")

	results, err = client.Submit(ctx, env, api.SubmitOptions{})
	check(err, "submit direct transaction")
	for _, r := range results {
		if r.Status != nil {
			fmt.Printf("    TxID: %s\n", r.Status.TxID)
			fmt.Printf("    Status: %v\n", r.Status.Code)
			if r.Status.Code.Delivered() {
				fmt.Println("\n  WARNING: Transaction succeeded without fee!")
				fmt.Println("  This shows why pure delegation needs Model 1 for enforcement.")
			}
		}
	}

	// =========================================================================
	// Summary
	// =========================================================================
	fmt.Println()
	fmt.Println("============================================================")
	fmt.Println("MODEL 4 DELEGATION SUMMARY")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Println("Delegation Setup:")
	fmt.Println("  - Bob's key page has delegate entry: { delegate: service/book }")
	fmt.Println("  - Service signed to ACCEPT being added (required!)")
	fmt.Println("  - Service uses DelegatedSignature with Delegator=Bob's key BOOK")
	fmt.Println()
	fmt.Println("Demo A (Delegated + Fee):")
	fmt.Printf("  - Result: %s\n", map[bool]string{true: "SUCCESS", false: "PENDING"}[demoASuccess])
	fmt.Println("  - Service signed as delegate WITH UserFee")
	fmt.Println()
	fmt.Println("Demo B (User Bypass):")
	fmt.Println("  - User signed directly WITHOUT fee")
	fmt.Println("  - Transaction EXECUTED (no enforcement!)")
	fmt.Println()
	fmt.Println("RECOMMENDATION:")
	fmt.Println("  For bulletproof enforcement, combine BOTH patterns:")
	fmt.Println("  1. Delegation -> Better UX (service signs for user)")
	fmt.Println("  2. Authority -> Enforcement (service must co-sign)")
	fmt.Println()
	fmt.Println("  See: model1_multisig_enforcement.go")
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
