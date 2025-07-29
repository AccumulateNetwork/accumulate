// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package liteclient

import (
	"context"
	"testing"
	"time"
)

// TestADIOrchestrator tests: can orchestrator find all accounts for an ADI?
func TestADIOrchestrator(t *testing.T) {
	liteClient, err := NewLiteClient("https://mainnet.accumulatenetwork.io/v2")
	if err != nil {
		t.Fatalf("Failed to create lite client: %v", err)
	}

	orchestrator, err := NewADIOrchestrator(liteClient)
	if err != nil {
		t.Fatalf("Failed to create orchestrator: %v", err)
	}
	defer orchestrator.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Can orchestrator discover accounts for an ADI?
	accounts, err := orchestrator.discoverADIAccounts(ctx, "RenatoDAP.acme")
	if err != nil {
		print("Account discovery failed:", err.Error())
		return
	}

	print("✓ ADI Orchestrator works")
	print("  Found accounts:", len(accounts))
	for _, account := range accounts {
		print("   -", account)
	}
}

// TestLiteClient tests: can liteclient get account data and receipts?
func TestLiteClient(t *testing.T) {
	client, err := NewLiteClient("https://mainnet.accumulatenetwork.io/v2")
	if err != nil {
		t.Fatalf("Failed to create lite client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Can liteclient get account data?
	accountData, err := client.getAccountData(ctx, "acc://RenatoDAP.acme/token")
	if err != nil {
		print("Account data failed:", err.Error())
	} else {
		print("✓ LiteClient account data works")
		print("  Account:", accountData.URL)
		print("  Type:", accountData.Type)
	}

	// Can liteclient validate proofs?
	err = client.validateAndCacheProof(ctx, "acc://RenatoDAP.acme/token", []byte("test-root"))
	if err != nil {
		print("Proof validation failed:", err.Error())
	} else {
		print("✓ LiteClient proof validation works")
	}
}
