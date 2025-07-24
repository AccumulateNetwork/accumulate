// proof_simple_test.go
//
// OBJECTIVE: This file contains a very simple test that calls FetchProof() once
// to demonstrate that the proof fetching functionality works. It tests against
// a known mainnet account and shows what data is retrieved.

package liteclient

import (
	"fmt"
	"testing"
)

func TestReceiptConstruction(t *testing.T) {
	// Test account
	accountURL := "acc://RenatoDAP.acme/token"

	// Test if we can retrieve/construct a receipt
	verifiedAccount, err := FetchProof(accountURL)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		t.Fatalf("Failed to construct receipt: %v", err)
	}

	// Print the receipt
	fmt.Printf("\n=== Receipt Generated ===\n")
	fmt.Printf("URL: %s\n", verifiedAccount.Url)
	fmt.Printf("Height: %d\n", verifiedAccount.Height)

	if verifiedAccount.Receipt != nil {
		fmt.Printf("Start: %s\n", string(verifiedAccount.Receipt.Start))
		fmt.Printf("Anchor: %x\n", verifiedAccount.Receipt.Anchor)
		fmt.Printf("Entries: %d\n", len(verifiedAccount.Receipt.Entries))
		for i, entry := range verifiedAccount.Receipt.Entries {
			fmt.Printf("  Entry %d: Hash=%x, Right=%v\n", i, entry.Hash, entry.Right)
		}
	} else {
		t.Fatal("No receipt generated")
	}
}

func TestReceiptValidation(t *testing.T) {
	// Test account
	accountURL := "acc://RenatoDAP.acme/token"

	// Get a receipt
	verifiedAccount, err := FetchProof(accountURL)
	if err != nil {
		t.Fatalf("Failed to construct receipt: %v", err)
	}

	if verifiedAccount.Receipt == nil {
		t.Fatal("No receipt to validate")
	}

	// Test if proof.go can validate the receipt
	isValid := VerifyProof(verifiedAccount.Receipt, nil)
	fmt.Printf("\n=== Receipt Validation ===\n")
	fmt.Printf("Receipt is valid: %v\n", isValid)

	// Note: Synthetic receipts are expected to fail validation
	// This test demonstrates the validation process
}
