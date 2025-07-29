// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package liteclient

import (
	"context"
	"fmt"
	"testing"
)

// Test comprehensive account data retrieval - shows what data looks like
func TestAccountDataRetrieval(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}

	client, err := NewLiteClient("https://mainnet.accumulatenetwork.io/v2")
	if err != nil {
		t.Fatalf("Failed to create lite client: %v", err)
	}

	ctx := context.Background()

	// Test many different account types
	testAccounts := []struct {
		url         string
		description string
	}{
		// ADI accounts
		{"acc://RenatoDAP.acme/token", "ADI Token Account"},
		{"acc://RenatoDAP.acme", "ADI Identity"},
		{"acc://RenatoDAP.acme/book", "ADI Key Book"},
		{"acc://RenatoDAP.acme/book/1", "ADI Key Page"},
		{"acc://RenatoDAP.acme/staking", "ADI Staking Account"},

		// Lite accounts
		{"acc://c7b2d77d5beadeb7774ca04106f2f68a9317b75c2f96efee/ACME", "Lite Token Account 1"},
		{"acc://08115f96ebb5e35a9c806de9cffe4c99455a0c5a60942d53/ACME", "Lite Token Account 2"},
		{"acc://e4571e13d3af400ad41a7e70134387d0f9b0bd5a94f4347f/ACME", "Lite Token Account 3"},
		{"acc://3752fc879ff3538e4e436512191aec2b61f8a9374c38f723/ACME", "Lite Token Account 4"},
		{"acc://9fe752486d3f03a607b465c0766947f86a8242de54e0c0c4/ACME", "Lite Token Account 5"},

		// System accounts
		{"acc://dn.acme/anchors", "DN Anchor Ledger"},
		{"acc://bvn0.acme/anchors", "BVN0 Anchor Ledger"},
		{"acc://directory.acme", "Directory Service"},
		{"acc://operators.acme", "Operators Page"},

		// Other known accounts
		{"acc://alice", "Test Alice Account"},
		{"acc://7117c50f04f1254d56b704dc05298912deeb25dbc1d26ef6/ACME", "Database Test Account"},
	}

	for _, testAccount := range testAccounts {
		println("\n=== Testing:", testAccount.description, "===")
		println("URL:", testAccount.url)

		// Get account data and show what it looks like
		accountData, err := client.getAccountData(ctx, testAccount.url)
		if err != nil {
			println("❌ Error:", err)
			continue
		}

		// Show the retrieved data structure
		println("✓ Retrieved Data:")
		println("  URL:", accountData.URL)
		println("  Type:", accountData.TypeName, "(", accountData.Type, ")")
		println("  Data Type:", fmt.Sprintf("%T", accountData.Data))
		println("  Is Token:", accountData.IsTokenAccount())
		println("  Is Identity:", accountData.IsIdentityAccount())
		println("  Is Data:", accountData.IsDataAccount())
		println("  Is Key:", accountData.IsKeyAccount())

		// Show account summary
		summary, err := client.GetAccountSummary(ctx, testAccount.url)
		if err != nil {
			println("  Summary Error:", err)
		} else {
			println("  Summary Category:", summary.Category)
			println("  Summary Balance:", summary.Balance)
			println("  Summary Token URL:", summary.TokenURL)
			println("  Summary Key Book:", summary.KeyBook)
		}
	}

	println("\n=== Account data retrieval test completed ===")
}

// Test account type detection - shows detected types
func TestAccountTypes(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}

	client, err := NewLiteClient("https://mainnet.accumulatenetwork.io/v2")
	if err != nil {
		t.Fatalf("Failed to create lite client: %v", err)
	}

	ctx := context.Background()

	// Test account type detection for different accounts
	testAccounts := []string{
		"acc://RenatoDAP.acme/token",
		"acc://RenatoDAP.acme",
		"acc://RenatoDAP.acme/book",
		"acc://c7b2d77d5beadeb7774ca04106f2f68a9317b75c2f96efee/ACME",
		"acc://dn.acme/anchors",
	}

	for _, url := range testAccounts {
		println("\n--- Account Type Detection ---")
		println("URL:", url)

		accountType, err := client.GetAccountType(ctx, url)
		if err != nil {
			println("❌ Type Error:", err)
			continue
		}

		println("✓ Detected Type:", accountType.String())
		println("✓ Type Number:", int(accountType))
	}

	println("\n=== Account type detection completed ===")
}
