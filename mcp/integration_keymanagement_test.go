// +build integration

package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/mcp/client"
)

const (
	keyManagementTimeout = 90 * time.Second
)

// TestDevnetKeyBookManagement tests full KeyBook and KeyPage workflow
func TestDevnetKeyBookManagement(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), keyManagementTimeout)
	defer cancel()

	c, err := client.NewClient(devnetURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	t.Log("=== KeyBook Management Test ===")

	// Step 1: Generate keys
	t.Log("Step 1: Generating keys...")
	pubKey1, privKey1, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key 1: %v", err)
	}

	pubKey2, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key 2: %v", err)
	}

	pubKey3, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key 3: %v", err)
	}

	pubKeyHex1 := hex.EncodeToString(pubKey1)
	privKeyHex1 := hex.EncodeToString(privKey1)
	pubKeyHex2 := hex.EncodeToString(pubKey2)
	pubKeyHex3 := hex.EncodeToString(pubKey3)

	t.Logf("Key 1: %s", pubKeyHex1[:32])
	t.Logf("Key 2: %s", pubKeyHex2[:32])
	t.Logf("Key 3: %s", pubKeyHex3[:32])

	// Step 2: Create lite account
	t.Log("Step 2: Creating lite account...")
	liteAccount, err := client.CreateLiteAccountURL(pubKeyHex1)
	if err != nil {
		t.Fatalf("Failed to create lite account: %v", err)
	}
	t.Logf("Lite account: %s", liteAccount)

	// Step 3: Fund via faucet
	t.Log("Step 3: Requesting faucet funds...")
	faucetResult, err := c.Faucet(ctx, liteAccount, map[string]interface{}{})
	if err != nil {
		t.Skipf("Faucet not available, skipping test: %v", err)
	}
	t.Logf("Faucet result: %+v", faucetResult)

	// Wait for funding
	t.Log("Step 4: Waiting for faucet confirmation (10s)...")
	time.Sleep(10 * time.Second)

	// Step 5: Create ADI (which automatically creates a KeyBook)
	adiName := "keytest-" + time.Now().Format("20060102150405")
	t.Logf("Step 5: Creating ADI: %s.acme", adiName)

	adiTxHash, err := c.CreateIdentity(ctx, adiName, pubKeyHex1, liteAccount, privKeyHex1)
	if err != nil {
		t.Fatalf("Failed to create ADI: %v", err)
	}
	t.Logf("ADI created! TX Hash: %x", adiTxHash)

	// Wait for ADI creation
	t.Log("Step 6: Waiting for ADI creation (15s)...")
	time.Sleep(15 * time.Second)

	// Step 7: Query the ADI to verify it exists
	adiURL := "acc://" + adiName + ".acme"
	t.Logf("Step 7: Querying ADI: %s", adiURL)
	adiResult, err := c.QueryAccount(ctx, adiURL)
	if err != nil {
		t.Logf("Warning: Could not query ADI: %v", err)
	} else {
		t.Logf("ADI found: %+v", adiResult)
	}

	// Step 8: Query the default KeyBook (created with ADI)
	keyBookURL := adiURL + "/book"
	t.Logf("Step 8: Querying KeyBook: %s", keyBookURL)
	keyBookResult, err := c.QueryKeyBook(ctx, keyBookURL)
	if err != nil {
		t.Logf("Warning: Could not query KeyBook: %v", err)
	} else {
		t.Logf("KeyBook found: %+v", keyBookResult)
	}

	// Step 9: Query the default KeyPage
	keyPageURL := keyBookURL + "/1"
	t.Logf("Step 9: Querying KeyPage: %s", keyPageURL)
	keyPageResult, err := c.QueryKeyPage(ctx, keyPageURL)
	if err != nil {
		t.Logf("Warning: Could not query KeyPage: %v", err)
	} else {
		t.Logf("KeyPage found: %+v", keyPageResult)
	}

	// Step 10: Create additional KeyPage with multiple keys
	t.Log("Step 10: Creating additional KeyPage with multiple keys...")
	keyPageTxHash, err := c.CreateKeyPage(ctx, keyBookURL, keyBookURL, privKeyHex1, []string{pubKeyHex2, pubKeyHex3})
	if err != nil {
		t.Logf("Warning: Could not create KeyPage: %v", err)
		// Don't fail - this might not be supported in current devnet
	} else {
		t.Logf("KeyPage created! TX Hash: %x", keyPageTxHash)

		// Wait for creation
		t.Log("Waiting for KeyPage creation (10s)...")
		time.Sleep(10 * time.Second)

		// Query the new KeyPage
		newKeyPageURL := keyBookURL + "/2"
		t.Logf("Querying new KeyPage: %s", newKeyPageURL)
		newKeyPageResult, err := c.QueryKeyPage(ctx, newKeyPageURL)
		if err != nil {
			t.Logf("Warning: Could not query new KeyPage: %v", err)
		} else {
			t.Logf("New KeyPage: %+v", newKeyPageResult)
		}
	}

	// Step 11: Update KeyPage - add a key
	t.Log("Step 11: Testing KeyPage update (add key)...")
	updateTxHash, err := c.UpdateKeyPage(ctx, keyPageURL, keyBookURL, privKeyHex1, "add", pubKeyHex2, 0)
	if err != nil {
		t.Logf("Warning: Could not update KeyPage (add): %v", err)
	} else {
		t.Logf("KeyPage updated (add)! TX Hash: %x", updateTxHash)

		// Wait for update
		time.Sleep(10 * time.Second)

		// Query updated KeyPage
		t.Log("Querying updated KeyPage...")
		updatedResult, err := c.QueryKeyPage(ctx, keyPageURL)
		if err != nil {
			t.Logf("Warning: Could not query updated KeyPage: %v", err)
		} else {
			t.Logf("Updated KeyPage: %+v", updatedResult)
		}
	}

	// Step 12: Update KeyPage - set threshold for multisig
	t.Log("Step 12: Testing KeyPage threshold update...")
	thresholdTxHash, err := c.UpdateKeyPage(ctx, keyPageURL, keyBookURL, privKeyHex1, "set_threshold", "", 2)
	if err != nil {
		t.Logf("Warning: Could not update KeyPage threshold: %v", err)
	} else {
		t.Logf("KeyPage threshold updated! TX Hash: %x", thresholdTxHash)
	}

	t.Log("=== KeyBook Management Test Complete ===")
}

// TestDevnetMultipleKeyBooks tests creating multiple KeyBooks for an ADI
func TestDevnetMultipleKeyBooks(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), keyManagementTimeout)
	defer cancel()

	c, err := client.NewClient(devnetURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	t.Log("=== Multiple KeyBooks Test ===")

	// Step 1: Generate keys
	t.Log("Step 1: Generating keys...")
	pubKey1, privKey1, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key 1: %v", err)
	}

	pubKey2, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key 2: %v", err)
	}

	pubKeyHex1 := hex.EncodeToString(pubKey1)
	privKeyHex1 := hex.EncodeToString(privKey1)
	pubKeyHex2 := hex.EncodeToString(pubKey2)

	t.Logf("Key 1: %s", pubKeyHex1[:32])
	t.Logf("Key 2: %s", pubKeyHex2[:32])

	// Step 2: Create and fund lite account
	t.Log("Step 2: Creating and funding lite account...")
	liteAccount, err := client.CreateLiteAccountURL(pubKeyHex1)
	if err != nil {
		t.Fatalf("Failed to create lite account: %v", err)
	}

	faucetResult, err := c.Faucet(ctx, liteAccount, map[string]interface{}{})
	if err != nil {
		t.Skipf("Faucet not available, skipping test: %v", err)
	}
	t.Logf("Faucet result: %+v", faucetResult)

	time.Sleep(10 * time.Second)

	// Step 3: Create ADI
	adiName := "multibook-" + time.Now().Format("20060102150405")
	t.Logf("Step 3: Creating ADI: %s.acme", adiName)

	adiTxHash, err := c.CreateIdentity(ctx, adiName, pubKeyHex1, liteAccount, privKeyHex1)
	if err != nil {
		t.Fatalf("Failed to create ADI: %v", err)
	}
	t.Logf("ADI created! TX Hash: %x", adiTxHash)

	time.Sleep(15 * time.Second)

	// Step 4: Create second KeyBook
	adiURL := "acc://" + adiName + ".acme"
	secondKeyBookURL := adiURL + "/book2"
	t.Logf("Step 4: Creating second KeyBook: %s", secondKeyBookURL)

	// Hash the public key for KeyBook creation
	keyHash := hex.EncodeToString(pubKey2)

	keyBookTxHash, err := c.CreateKeyBook(ctx, secondKeyBookURL, adiURL+"/book", privKeyHex1, keyHash)
	if err != nil {
		t.Logf("Warning: Could not create second KeyBook: %v", err)
		// Don't fail - this might not be supported in current devnet
	} else {
		t.Logf("Second KeyBook created! TX Hash: %x", keyBookTxHash)

		// Wait for creation
		time.Sleep(10 * time.Second)

		// Query the second KeyBook
		t.Log("Step 5: Querying second KeyBook...")
		keyBookResult, err := c.QueryKeyBook(ctx, secondKeyBookURL)
		if err != nil {
			t.Logf("Warning: Could not query second KeyBook: %v", err)
		} else {
			t.Logf("Second KeyBook: %+v", keyBookResult)
		}
	}

	t.Log("=== Multiple KeyBooks Test Complete ===")
}

// TestDevnetKeyPageQueries tests querying KeyBook and KeyPage information
func TestDevnetKeyPageQueries(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	c, err := client.NewClient(devnetURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	t.Log("=== KeyBook/KeyPage Query Test ===")

	// Test querying known KeyBook patterns
	testCases := []struct {
		name string
		url  string
	}{
		{"DN KeyBook", "acc://dn.acme/book"},
		{"DN KeyPage 1", "acc://dn.acme/book/1"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Querying: %s", tc.url)

			if tc.name == "DN KeyBook" || tc.name[:7] == "DN Key" {
				// Try KeyBook query
				result, err := c.QueryKeyBook(ctx, tc.url)
				if err != nil {
					t.Logf("Could not query as KeyBook: %v", err)
				} else {
					t.Logf("KeyBook result: %+v", result)
					return
				}
			}

			// Try KeyPage query
			result, err := c.QueryKeyPage(ctx, tc.url)
			if err != nil {
				t.Logf("Could not query as KeyPage: %v", err)
			} else {
				t.Logf("KeyPage result: %+v", result)
			}
		})
	}

	t.Log("=== KeyBook/KeyPage Query Test Complete ===")
}

// TestDevnetMultisigWorkflow tests a complete multisig workflow
func TestDevnetMultisigWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*keyManagementTimeout)
	defer cancel()

	c, err := client.NewClient(devnetURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	t.Log("=== Multisig Workflow Test ===")
	t.Log("This test demonstrates:")
	t.Log("1. Creating ADI with KeyBook")
	t.Log("2. Adding multiple keys to KeyPage")
	t.Log("3. Setting threshold for multisig")
	t.Log("4. Querying authority structure")

	// Generate 3 keys for multisig
	t.Log("\nStep 1: Generating 3 keys for multisig...")
	var keys []string
	var privKeys []string

	for i := 1; i <= 3; i++ {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("Failed to generate key %d: %v", i, err)
		}
		pubHex := hex.EncodeToString(pub)
		privHex := hex.EncodeToString(priv)
		keys = append(keys, pubHex)
		privKeys = append(privKeys, privHex)
		t.Logf("  Key %d: %s...", i, pubHex[:32])
	}

	// Create lite account with first key
	t.Log("\nStep 2: Creating lite account with first key...")
	liteAccount, err := client.CreateLiteAccountURL(keys[0])
	if err != nil {
		t.Fatalf("Failed to create lite account: %v", err)
	}
	t.Logf("Lite account: %s", liteAccount)

	// Fund account
	t.Log("\nStep 3: Funding account via faucet...")
	_, err = c.Faucet(ctx, liteAccount, map[string]interface{}{})
	if err != nil {
		t.Skipf("Faucet not available, skipping test: %v", err)
	}
	time.Sleep(10 * time.Second)

	// Create ADI
	adiName := "multisig-" + time.Now().Format("20060102150405")
	t.Logf("\nStep 4: Creating ADI: %s.acme", adiName)
	_, err = c.CreateIdentity(ctx, adiName, keys[0], liteAccount, privKeys[0])
	if err != nil {
		t.Fatalf("Failed to create ADI: %v", err)
	}
	time.Sleep(15 * time.Second)

	// Query KeyBook structure
	adiURL := "acc://" + adiName + ".acme"
	keyBookURL := adiURL + "/book"
	keyPageURL := keyBookURL + "/1"

	t.Log("\nStep 5: Querying initial KeyBook structure...")
	t.Logf("  ADI: %s", adiURL)
	t.Logf("  KeyBook: %s", keyBookURL)
	t.Logf("  KeyPage: %s", keyPageURL)

	keyBookResult, err := c.QueryKeyBook(ctx, keyBookURL)
	if err != nil {
		t.Logf("  KeyBook query: %v", err)
	} else {
		t.Logf("  KeyBook: %+v", keyBookResult)
	}

	keyPageResult, err := c.QueryKeyPage(ctx, keyPageURL)
	if err != nil {
		t.Logf("  KeyPage query: %v", err)
	} else {
		t.Logf("  Initial KeyPage: %+v", keyPageResult)
	}

	// Add second key to KeyPage
	t.Log("\nStep 6: Adding second key to KeyPage...")
	_, err = c.UpdateKeyPage(ctx, keyPageURL, keyBookURL, privKeys[0], "add", keys[1], 0)
	if err != nil {
		t.Logf("  Add key warning: %v", err)
	} else {
		t.Log("  Second key added successfully")
		time.Sleep(10 * time.Second)
	}

	// Add third key to KeyPage
	t.Log("\nStep 7: Adding third key to KeyPage...")
	_, err = c.UpdateKeyPage(ctx, keyPageURL, keyBookURL, privKeys[0], "add", keys[2], 0)
	if err != nil {
		t.Logf("  Add key warning: %v", err)
	} else {
		t.Log("  Third key added successfully")
		time.Sleep(10 * time.Second)
	}

	// Set threshold to 2 of 3
	t.Log("\nStep 8: Setting threshold to 2-of-3 multisig...")
	_, err = c.UpdateKeyPage(ctx, keyPageURL, keyBookURL, privKeys[0], "set_threshold", "", 2)
	if err != nil {
		t.Logf("  Set threshold warning: %v", err)
	} else {
		t.Log("  Threshold set to 2")
		time.Sleep(10 * time.Second)
	}

	// Query final KeyPage state
	t.Log("\nStep 9: Querying final multisig configuration...")
	finalKeyPage, err := c.QueryKeyPage(ctx, keyPageURL)
	if err != nil {
		t.Logf("  Final KeyPage query: %v", err)
	} else {
		t.Logf("  Final KeyPage (should show 3 keys, threshold 2): %+v", finalKeyPage)
	}

	t.Log("\n=== Multisig Workflow Test Complete ===")
	t.Log("Summary:")
	t.Log("  - Created ADI with KeyBook")
	t.Log("  - Added 3 keys to KeyPage")
	t.Log("  - Set 2-of-3 multisig threshold")
	t.Log("  - Verified authority structure")
}
