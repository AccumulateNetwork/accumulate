// Verify UserFee Implementation Script
//
// This script tests if the AIP-50 UserFee implementation is active in the running devnet.
//
// Run: go run verify_userfee.go

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
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func main() {
	fmt.Println("============================================================")
	fmt.Println("AIP-50 UserFee Implementation Verification")
	fmt.Println("============================================================")
	fmt.Printf("API Endpoint: %s\n", cfg.ACC_API)
	fmt.Printf("Version: %s\n\n", cfg.Version)

	ctx := context.Background()
	client := jsonrpc.NewClient(cfg.ACC_API)

	// Test 1: Check network status
	fmt.Println("Test 1: Checking network status...")
	networkStatus, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{Partition: "BVN1"})
	if err != nil {
		fmt.Printf("  ERROR: Cannot connect to devnet: %v\n", err)
		return
	}
	fmt.Printf("  Network: %s\n", networkStatus.Network.NetworkName)
	fmt.Printf("  Executor Version: %s\n", networkStatus.ExecutorVersion)
	fmt.Println("  OK - Devnet is running\n")

	// Test 2: Query Alice's lite account to see if it has tokens
	aliceLite := cfg.GetAliceLiteAddress()
	fmt.Printf("Test 2: Checking Alice's lite account (%s)...\n", aliceLite)
	record, err := client.Query(ctx, aliceLite, nil)
	if err != nil {
		fmt.Printf("  ERROR: Cannot query account: %v\n", err)
		fmt.Println("  You may need to run alice_bob_fee_examples.go first.\n")
	} else {
		data, _ := json.MarshalIndent(record, "  ", "  ")
		fmt.Printf("  Account state:\n%s\n\n", truncate(string(data), 500))
	}

	// Test 3: Try to build a transaction with UserFee (this doesn't require submission)
	fmt.Println("Test 3: Building transaction with UserFee...")
	feeAmount := big.NewInt(protocol.AcmePrecision / 10)
	sendAmount := big.NewInt(1 * protocol.AcmePrecision)

	txn, err := build.Transaction().
		For(aliceLite).
		UserFee(cfg.ServiceFees(), feeAmount, protocol.AcmeUrl(), aliceLite).
		SendTokens(sendAmount, protocol.AcmePrecisionPower).
		To(cfg.BobTokens()).
		Done()
	if err != nil {
		fmt.Printf("  ERROR: Cannot build transaction with UserFee: %v\n", err)
		fmt.Println("  This may indicate the UserFee builder is missing from the codebase.\n")
		return
	}

	fmt.Printf("  Transaction built successfully!\n")
	fmt.Printf("  Has UserFees in header: %v\n", len(txn.Header.UserFees) > 0)
	if len(txn.Header.UserFees) > 0 {
		for i, fee := range txn.Header.UserFees {
			fmt.Printf("    Fee %d: %s to %s\n", i,
				protocol.FormatBigAmount(&fee.Amount, protocol.AcmePrecisionPower),
				fee.Recipient)
		}
	}
	fmt.Println()

	// Test 4: Try to submit and see what happens (if alice has funds)
	fmt.Println("Test 4: Attempting to submit transaction with UserFee...")
	privateKey := cfg.GetAliceKey()

	env, err := build.Transaction().
		For(aliceLite).
		UserFee(cfg.ServiceFees(), feeAmount, protocol.AcmeUrl(), aliceLite).
		SendTokens(sendAmount, protocol.AcmePrecisionPower).
		To(cfg.BobTokens()).
		SignWith(aliceLite).
		Version(1).
		Timestamp(time.Now().UTC().UnixMicro()).
		PrivateKey(privateKey).
		Done()
	if err != nil {
		fmt.Printf("  ERROR: Cannot build signed transaction: %v\n", err)
		return
	}

	results, err := client.Submit(ctx, env, api.SubmitOptions{})
	if err != nil {
		fmt.Printf("  Submit returned error: %v\n", err)
		// Check if this is a "UserFees field unknown" type error
		errStr := err.Error()
		if contains(errStr, "UserFees") || contains(errStr, "unknown field") {
			fmt.Println("\n  *** FAILURE: The devnet does NOT have AIP-50 UserFee support! ***")
			fmt.Println("  The devnet was likely built WITHOUT the AIP-50 code.")
			fmt.Println("  Please rebuild with: docker-compose build --no-cache")
		} else if contains(errStr, "insufficient") {
			fmt.Println("\n  *** UserFee implementation is PRESENT but account has insufficient funds ***")
			fmt.Println("  Run alice_bob_fee_examples.go to fund the account first.")
		}
		return
	}

	fmt.Println("  Transaction submitted successfully!")
	for _, r := range results {
		if r.Status != nil {
			fmt.Printf("    TxID: %s\n", r.Status.TxID)
			fmt.Printf("    Status: %v\n", r.Status.Code)
			if r.Status.Error != nil {
				fmt.Printf("    Error: %v\n", r.Status.Error)
				errStr := r.Status.Error.Error()
				if contains(errStr, "UserFees") || contains(errStr, "unknown") {
					fmt.Println("\n  *** FAILURE: The devnet does NOT have AIP-50 UserFee support! ***")
				}
			}
		}
	}

	fmt.Println()
	fmt.Println("============================================================")
	fmt.Println("VERIFICATION RESULT")
	fmt.Println("============================================================")
	fmt.Println("If you see the transaction was submitted without 'UserFees unknown' errors,")
	fmt.Println("then the AIP-50 UserFee implementation IS running in the container.")
	fmt.Println()
	fmt.Println("If you see errors about 'unknown field' or 'UserFees', the devnet was")
	fmt.Println("built without AIP-50 code. Rebuild with: docker-compose build --no-cache")
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...(truncated)"
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
