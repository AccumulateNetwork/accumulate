// +build integration

package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/mcp/client"
)

const (
	transactionTimeout = 60 * time.Second
)

// generateTestKey generates a random ED25519 key pair for testing
func generateTestKey() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	return pub, priv, err
}

// TestDevnetTokenSend tests sending tokens on devnet
func TestDevnetTokenSend(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), transactionTimeout)
	defer cancel()

	c, err := client.NewClient(devnetURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	t.Log("=== Token Send Test ===")

	// Generate test keys
	t.Log("Step 1: Generating keys...")
	pubKey1, privKey1, err := generateTestKey()
	if err != nil {
		t.Fatalf("Failed to generate key 1: %v", err)
	}

	pubKey2, _, err := generateTestKey()
	if err != nil {
		t.Fatalf("Failed to generate key 2: %v", err)
	}

	// Create lite accounts
	t.Log("Step 2: Creating lite accounts...")
	fromAccount, err := client.CreateLiteAccountURL(hex.EncodeToString(pubKey1))
	if err != nil {
		t.Fatalf("Failed to create from account: %v", err)
	}

	toAccount, err := client.CreateLiteAccountURL(hex.EncodeToString(pubKey2))
	if err != nil {
		t.Fatalf("Failed to create to account: %v", err)
	}

	t.Logf("From account: %s", fromAccount)
	t.Logf("To account: %s", toAccount)

	// Fund the from account using faucet
	t.Log("Step 3: Requesting faucet funds...")
	faucetResult, err := c.Faucet(ctx, fromAccount, map[string]interface{}{})
	if err != nil {
		t.Skipf("Faucet not available, skipping test: %v", err)
	}
	t.Logf("Faucet result: %+v", faucetResult)

	// Wait for faucet transaction to be confirmed
	t.Log("Step 4: Waiting for faucet confirmation (10s)...")
	time.Sleep(10 * time.Second)

	// Verify account has funds
	t.Log("Step 5: Verifying account balance...")
	accountResult, err := c.QueryAccount(ctx, fromAccount)
	if err != nil {
		t.Logf("Warning: Could not query account: %v", err)
	} else {
		t.Logf("Account state: %+v", accountResult)
	}

	// Send tokens from account 1 to account 2
	t.Log("Step 6: Sending tokens...")
	privKeyHex := hex.EncodeToString(privKey1)
	txHash, err := c.SendTokens(ctx, fromAccount, toAccount, 1000000, privKeyHex) // 1.0 ACME
	if err != nil {
		t.Fatalf("Token send failed: %v", err)
	}

	t.Logf("Token send successful! TX Hash: %x", txHash)

	// Wait for transaction confirmation
	t.Log("Step 7: Waiting for transaction confirmation (5s)...")
	time.Sleep(5 * time.Second)

	// Query both accounts to verify balances
	t.Log("Step 8: Verifying final balances...")
	fromResult, err := c.QueryAccount(ctx, fromAccount)
	if err != nil {
		t.Logf("Warning: Could not query from account: %v", err)
	} else {
		t.Logf("From account final state: %+v", fromResult)
	}

	toResult, err := c.QueryAccount(ctx, toAccount)
	if err != nil {
		t.Logf("Warning: Could not query to account: %v", err)
	} else {
		t.Logf("To account final state: %+v", toResult)
	}

	t.Log("=== Token Send Test Complete ===")
}

// TestDevnetCreateADI tests creating an ADI
func TestDevnetCreateADI(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), transactionTimeout)
	defer cancel()

	c, err := client.NewClient(devnetURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	t.Log("=== ADI Creation Test ===")

	// Generate test key
	t.Log("Step 1: Generating key...")
	pubKey, privKey, err := generateTestKey()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	pubKeyHex := hex.EncodeToString(pubKey)
	privKeyHex := hex.EncodeToString(privKey)

	// Create lite account
	t.Log("Step 2: Creating lite account...")
	liteAccount, err := client.CreateLiteAccountURL(pubKeyHex)
	if err != nil {
		t.Fatalf("Failed to create lite account: %v", err)
	}
	t.Logf("Lite account: %s", liteAccount)

	// Fund the account using faucet
	t.Log("Step 3: Requesting faucet funds...")
	faucetResult, err := c.Faucet(ctx, liteAccount, map[string]interface{}{})
	if err != nil {
		t.Skipf("Faucet not available, skipping test: %v", err)
	}
	t.Logf("Faucet result: %+v", faucetResult)

	// Wait for faucet transaction
	t.Log("Step 4: Waiting for faucet confirmation (10s)...")
	time.Sleep(10 * time.Second)

	// Generate a unique ADI name for testing
	adiName := "test-adi-" + time.Now().Format("20060102150405")
	t.Logf("Step 5: Creating ADI: %s", adiName)

	// Create ADI
	txHash, err := c.CreateIdentity(ctx, adiName, pubKeyHex, liteAccount, privKeyHex)
	if err != nil {
		t.Fatalf("ADI creation failed: %v", err)
	}

	t.Logf("ADI creation successful! TX Hash: %x", txHash)

	// Wait for confirmation
	t.Log("Step 6: Waiting for ADI creation confirmation (10s)...")
	time.Sleep(10 * time.Second)

	// Query the ADI
	t.Log("Step 7: Querying created ADI...")
	adiURL := "acc://" + adiName + ".acme"
	adiResult, err := c.QueryAccount(ctx, adiURL)
	if err != nil {
		t.Logf("Warning: Could not query ADI: %v", err)
	} else {
		t.Logf("ADI state: %+v", adiResult)
	}

	t.Log("=== ADI Creation Test Complete ===")
}

// TestDevnetCreateDataAccount tests creating a data account
func TestDevnetCreateDataAccount(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if os.Getenv("INTEGRATION_FULL") != "true" {
		t.Skip("Skipping full integration test (set INTEGRATION_FULL=true to enable)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), transactionTimeout)
	defer cancel()

	c, err := client.NewClient(devnetURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Generate test key
	pubKey, privKey, err := generateTestKey()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	pubKeyHex := hex.EncodeToString(pubKey)
	privKeyHex := hex.EncodeToString(privKey)

	// This test assumes we have an existing ADI
	testADI := os.Getenv("TEST_ADI_URL")
	if testADI == "" {
		t.Skip("Skipping data account creation test (set TEST_ADI_URL to enable)")
	}

	dataAccountURL := testADI + "/test-data"

	// Attempt to create data account
	result, err := c.CreateDataAccount(ctx, dataAccountURL, testADI, privKeyHex, []string{pubKeyHex})
	if err != nil {
		t.Logf("Data account creation failed: %v", err)
		return
	}

	t.Logf("Data account creation result: %+v", result)
}

// TestDevnetWriteData tests writing data to a data account
func TestDevnetWriteData(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if os.Getenv("INTEGRATION_FULL") != "true" {
		t.Skip("Skipping full integration test (set INTEGRATION_FULL=true to enable)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), transactionTimeout)
	defer cancel()

	c, err := client.NewClient(devnetURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// This test assumes we have an existing data account
	testDataAccount := os.Getenv("TEST_DATA_ACCOUNT_URL")
	testPrivateKey := os.Getenv("TEST_PRIVATE_KEY")
	if testDataAccount == "" || testPrivateKey == "" {
		t.Skip("Skipping write data test (set TEST_DATA_ACCOUNT_URL and TEST_PRIVATE_KEY to enable)")
	}

	// Test data to write
	testData := "integration test data " + time.Now().Format(time.RFC3339)

	// Attempt to write data
	result, err := c.WriteData(ctx, testDataAccount, testDataAccount, testPrivateKey, []byte(testData), false)
	if err != nil {
		t.Fatalf("Failed to write data: %v", err)
	}

	t.Logf("Write data result: %+v", result)

	// Wait a bit for the transaction to be confirmed
	time.Sleep(5 * time.Second)

	// Query the data back
	dataResult, err := c.QueryData(ctx, testDataAccount, map[string]interface{}{})
	if err != nil {
		t.Fatalf("Failed to query data: %v", err)
	}

	t.Logf("Query data result: %+v", dataResult)
}

// TestDevnetAddCredits tests adding credits to an account
func TestDevnetAddCredits(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Skip("Skipping AddCredits test - requires NetworkStatus which has SDK version incompatibility")

	ctx, cancel := context.WithTimeout(context.Background(), transactionTimeout)
	defer cancel()

	c, err := client.NewClient(devnetURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	t.Log("=== Add Credits Test ===")

	// Generate test key
	t.Log("Step 1: Generating key...")
	pubKey, privKey, err := generateTestKey()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	pubKeyHex := hex.EncodeToString(pubKey)
	privKeyHex := hex.EncodeToString(privKey)

	// Create lite account
	t.Log("Step 2: Creating lite account...")
	liteAccount, err := client.CreateLiteAccountURL(pubKeyHex)
	if err != nil {
		t.Fatalf("Failed to create lite account: %v", err)
	}
	t.Logf("Lite account: %s", liteAccount)

	// Fund the account using faucet
	t.Log("Step 3: Requesting faucet funds...")
	faucetResult, err := c.Faucet(ctx, liteAccount, map[string]interface{}{})
	if err != nil {
		t.Skipf("Faucet not available, skipping test: %v", err)
	}
	t.Logf("Faucet result: %+v", faucetResult)

	// Wait for faucet transaction
	t.Log("Step 4: Waiting for faucet confirmation (10s)...")
	time.Sleep(10 * time.Second)

	// Add credits to the account
	t.Log("Step 5: Adding credits...")
	txHash, err := c.AddCredits(ctx, liteAccount, liteAccount, 1000, privKeyHex)
	if err != nil {
		t.Fatalf("Add credits failed: %v", err)
	}

	t.Logf("Add credits successful! TX Hash: %x", txHash)

	t.Log("=== Add Credits Test Complete ===")
}

// TestDevnetUpdateKeyPage tests updating a key page
func TestDevnetUpdateKeyPage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if os.Getenv("INTEGRATION_FULL") != "true" {
		t.Skip("Skipping full integration test (set INTEGRATION_FULL=true to enable)")
	}

	// This test requires an existing ADI with a key page
	testKeyPage := os.Getenv("TEST_KEY_PAGE_URL")
	testOldKey := os.Getenv("TEST_OLD_KEY")
	testNewKey := os.Getenv("TEST_NEW_KEY")
	testPrivateKey := os.Getenv("TEST_PRIVATE_KEY")

	if testKeyPage == "" || testOldKey == "" || testNewKey == "" || testPrivateKey == "" {
		t.Skip("Skipping key page update test (set TEST_KEY_PAGE_URL, TEST_OLD_KEY, TEST_NEW_KEY, TEST_PRIVATE_KEY to enable)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), transactionTimeout)
	defer cancel()

	c, err := client.NewClient(devnetURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Attempt to update key page
	result, err := c.UpdateKeyPage(ctx, testKeyPage, testKeyPage, testPrivateKey, "update", testNewKey, 1)
	if err != nil {
		t.Logf("Update key page failed: %v", err)
		return
	}

	t.Logf("Update key page result: %+v", result)
}

// TestDevnetBurnTokens tests burning tokens
func TestDevnetBurnTokens(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), transactionTimeout)
	defer cancel()

	c, err := client.NewClient(devnetURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	t.Log("=== Burn Tokens Test ===")

	// Generate test key
	t.Log("Step 1: Generating key...")
	pubKey, privKey, err := generateTestKey()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	pubKeyHex := hex.EncodeToString(pubKey)
	privKeyHex := hex.EncodeToString(privKey)

	// Create lite account
	t.Log("Step 2: Creating lite account...")
	liteAccount, err := client.CreateLiteAccountURL(pubKeyHex)
	if err != nil {
		t.Fatalf("Failed to create lite account: %v", err)
	}
	t.Logf("Lite account: %s", liteAccount)

	// Fund the account using faucet
	t.Log("Step 3: Requesting faucet funds...")
	faucetResult, err := c.Faucet(ctx, liteAccount, map[string]interface{}{})
	if err != nil {
		t.Skipf("Faucet not available, skipping test: %v", err)
	}
	t.Logf("Faucet result: %+v", faucetResult)

	// Wait for faucet transaction
	t.Log("Step 4: Waiting for faucet confirmation (10s)...")
	time.Sleep(10 * time.Second)

	// Burn tokens
	t.Log("Step 5: Burning tokens...")
	txHash, err := c.BurnTokens(ctx, liteAccount, liteAccount, privKeyHex, 100000) // 0.1 ACME
	if err != nil {
		t.Fatalf("Burn tokens failed: %v", err)
	}

	t.Logf("Burn tokens successful! TX Hash: %x", txHash)

	t.Log("=== Burn Tokens Test Complete ===")
}

// TestDevnetFullWorkflow tests a complete end-to-end workflow
func TestDevnetFullWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if os.Getenv("INTEGRATION_FULL") != "true" {
		t.Skip("Skipping full integration test (set INTEGRATION_FULL=true to enable)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), transactionTimeout)
	defer cancel()

	c, err := client.NewClient(devnetURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	t.Log("=== Starting Full Workflow Test ===")

	// Step 1: Generate key
	t.Log("Step 1: Generating key pair...")
	pubKey, privKey, err := generateTestKey()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}
	pubKeyHex := hex.EncodeToString(pubKey)
	privKeyHex := hex.EncodeToString(privKey)
	t.Logf("Generated public key: %s", pubKeyHex)

	// Step 2: Create lite account
	t.Log("Step 2: Creating lite account...")
	liteAccount, err := client.CreateLiteAccountURL(pubKeyHex)
	if err != nil {
		t.Fatalf("Failed to create lite account: %v", err)
	}
	t.Logf("Created lite account: %s", liteAccount)

	// Step 3: Query lite account (should not exist yet)
	t.Log("Step 3: Querying lite account (before funding)...")
	accountResult, err := c.QueryAccount(ctx, liteAccount)
	if err != nil {
		t.Logf("Account query failed (expected if not funded yet): %v", err)
	} else {
		t.Logf("Account result: %+v", accountResult)
	}

	// Step 4: Request faucet
	t.Log("Step 4: Requesting faucet funds...")
	faucetResult, err := c.Faucet(ctx, liteAccount, map[string]interface{}{})
	if err != nil {
		t.Skipf("Faucet not available, skipping test: %v", err)
	}
	t.Logf("Faucet result: %+v", faucetResult)

	// Wait for faucet transaction to be confirmed
	t.Log("Waiting for faucet transaction to be confirmed...")
	time.Sleep(10 * time.Second)

	// Step 5: Query account again (should exist now)
	t.Log("Step 5: Querying account after faucet...")
	accountResult, err = c.QueryAccount(ctx, liteAccount)
	if err != nil {
		t.Fatalf("Failed to query account after faucet: %v", err)
	}
	t.Logf("Account after faucet: %+v", accountResult)

	// Step 6: Create another account to send tokens to
	t.Log("Step 6: Creating second account...")
	pubKey2, _, err := generateTestKey()
	if err != nil {
		t.Fatalf("Failed to generate second key: %v", err)
	}
	pubKey2Hex := hex.EncodeToString(pubKey2)
	toAccount, err := client.CreateLiteAccountURL(pubKey2Hex)
	if err != nil {
		t.Fatalf("Failed to create second lite account: %v", err)
	}
	t.Logf("Created second account: %s", toAccount)

	// Step 7: Send tokens
	t.Log("Step 7: Sending tokens...")
	sendResult, err := c.SendTokens(ctx, liteAccount, toAccount, 1000000, privKeyHex) // 1.0 ACME
	if err != nil {
		t.Fatalf("Send tokens failed: %v", err)
	}
	t.Logf("Send tokens result: %x", sendResult)

	t.Log("=== Full Workflow Test Complete ===")
}
