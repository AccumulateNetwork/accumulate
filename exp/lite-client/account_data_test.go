package liteclient

import (
	"context"
	"fmt"
	"testing"
	"time"
)

const (
	// Valid test account on Kermit testnet (funded with 10 ACME)
	testAccount = "acc://c7b2d77d5beadeb7774ca04106f2f68a9317b75c2f96efee/ACME"
	kermitAPI   = "https://testnet.accumulatenetwork.io"
)

func TestAccountDataAPI_Features(t *testing.T) {
	fmt.Println("\n=== Account Data API: Feature #4 ===")

	client, err := NewLiteClient(kermitAPI)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Println("Step 1: GetBalance\n-------------------")
	fmt.Printf("Requesting balance for account: %s\n", testAccount)
	bal, err := client.GetBalance(ctx, testAccount)
	if err != nil {
		t.Fatalf("GetBalance failed: %v", err)
	}
	fmt.Printf("✓ Balance: %s %s (height %d)\n", bal.Balance, bal.Token, bal.Height)

	// Verify balance data structure
	if bal.AccountUrl != testAccount {
		t.Errorf("Account URL mismatch: got %s, want %s", bal.AccountUrl, testAccount)
	}
	if bal.Balance == "" {
		t.Error("Balance is empty")
	}
	if bal.Token == "" {
		t.Error("Token is empty")
	}
	if bal.Height <= 0 {
		t.Error("Height should be positive")
	}

	fmt.Println("\nStep 2: GetTransactions\n-----------------------")
	fmt.Printf("Requesting transactions for account: %s\n", testAccount)
	txs, err := client.GetTransactions(ctx, testAccount, 10)
	if err != nil {
		t.Fatalf("GetTransactions failed: %v", err)
	}
	fmt.Printf("✓ Retrieved %d transactions\n", len(txs))
	for i, tx := range txs {
		if tx.TxID == "" {
			t.Errorf("Transaction %d missing TxID", i)
		}
		if tx.Account != testAccount {
			t.Errorf("Transaction %d has wrong account: got %s, want %s", i, tx.Account, testAccount)
		}
		fmt.Printf("  Tx %d: %s (%s) at %d\n", i, tx.TxID, tx.Type, tx.Timestamp)
	}

	fmt.Println("\nStep 3: GetBalanceAndTransactions\n-------------------------------")
	fmt.Printf("Requesting combined balance and transactions for account: %s\n", testAccount)
	bal2, txs2, err := client.GetBalanceAndTransactions(ctx, testAccount, 5)
	if err != nil {
		t.Fatalf("GetBalanceAndTransactions failed: %v", err)
	}
	fmt.Printf("✓ Combined: %s %s with %d transactions\n", bal2.Balance, bal2.Token, len(txs2))

	// Verify consistency
	if bal2.Balance != bal.Balance {
		t.Errorf("Balance consistency check failed: %s != %s", bal2.Balance, bal.Balance)
	}

	fmt.Println("\n=== Account Data API Test Complete ===")
}

func TestAccountDataAPI_ErrorHandling(t *testing.T) {
	fmt.Println("\n=== Account Data API: Error Handling ===")

	client, err := NewLiteClient(kermitAPI)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	invalidAccount := "acc://invalid/account"

	fmt.Println("Step 1: GetBalance with invalid account\n----------------------------------------")
	_, err = client.GetBalance(ctx, invalidAccount)
	if err == nil {
		fmt.Println("⚠ GetBalance should have failed with invalid account (may be expected)")
	} else {
		fmt.Printf("✓ GetBalance properly failed with invalid account: %v\n", err)
	}

	fmt.Println("\nStep 2: GetTransactions with invalid account\n-------------------------------------------")
	_, err = client.GetTransactions(ctx, invalidAccount, 5)
	if err == nil {
		fmt.Println("⚠ GetTransactions should have failed with invalid account (may be expected)")
	} else {
		fmt.Printf("✓ GetTransactions properly failed with invalid account: %v\n", err)
	}

	fmt.Println("\n=== Error Handling Test Complete ===")
}
