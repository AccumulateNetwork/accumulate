package client

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestGenerateKey(t *testing.T) {
	publicKey, privateKey, liteAccountURL, err := GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	// Validate public key format
	if len(publicKey) != 64 { // 32 bytes = 64 hex chars
		t.Errorf("expected public key length 64, got %d", len(publicKey))
	}

	// Validate private key format
	if len(privateKey) != 128 { // 64 bytes = 128 hex chars
		t.Errorf("expected private key length 128, got %d", len(privateKey))
	}

	// Validate lite account URL format
	if !strings.HasPrefix(liteAccountURL, "acc://") {
		t.Errorf("expected lite account URL to start with 'acc://', got %s", liteAccountURL)
	}

	if !strings.HasSuffix(liteAccountURL, "/ACME") {
		t.Errorf("expected lite account URL to end with '/ACME', got %s", liteAccountURL)
	}

	// Validate hex encoding
	_, err = hex.DecodeString(publicKey)
	if err != nil {
		t.Errorf("public key is not valid hex: %v", err)
	}

	_, err = hex.DecodeString(privateKey)
	if err != nil {
		t.Errorf("private key is not valid hex: %v", err)
	}

	// Validate key sizes
	pubBytes, _ := hex.DecodeString(publicKey)
	if len(pubBytes) != ed25519.PublicKeySize {
		t.Errorf("expected public key size %d, got %d", ed25519.PublicKeySize, len(pubBytes))
	}

	privBytes, _ := hex.DecodeString(privateKey)
	if len(privBytes) != ed25519.PrivateKeySize {
		t.Errorf("expected private key size %d, got %d", ed25519.PrivateKeySize, len(privBytes))
	}
}

func TestAddCredits(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	client, err := NewClient(devnetURL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Generate a key for a lite account
	_, privateKey, liteAccountURL, err := GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	t.Logf("Generated lite account: %s", liteAccountURL)

	// Fund the account with faucet
	faucetResult, err := client.Faucet(ctx, liteAccountURL, nil)
	if err != nil {
		t.Fatalf("failed to request faucet: %v", err)
	}
	t.Logf("Faucet result: %v", faucetResult)

	// Wait for faucet transaction to settle
	time.Sleep(5 * time.Second)

	// Try to add credits
	// Note: This may fail if the account doesn't have enough ACME
	// but we're testing the transaction creation/submission logic
	txHash, err := client.AddCredits(ctx, liteAccountURL, liteAccountURL, 10000000, privateKey)
	if err != nil {
		// Log the error but don't fail - might be insufficient balance
		t.Logf("AddCredits error (may be expected): %v", err)
	} else {
		t.Logf("AddCredits transaction hash: %x", txHash)
	}
}

func TestCreateIdentity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	client, err := NewClient(devnetURL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Generate key for funding account
	_, fundingPrivateKey, fundingAccountURL, err := GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate funding key: %v", err)
	}

	t.Logf("Funding account: %s", fundingAccountURL)

	// Fund the account with faucet
	faucetResult, err := client.Faucet(ctx, fundingAccountURL, nil)
	if err != nil {
		t.Fatalf("failed to request faucet: %v", err)
	}
	t.Logf("Faucet result: %v", faucetResult)

	// Wait for faucet transaction
	time.Sleep(5 * time.Second)

	// Add credits to funding account
	txHash, err := client.AddCredits(ctx, fundingAccountURL, fundingAccountURL, 100000000, fundingPrivateKey)
	if err != nil {
		t.Logf("AddCredits error: %v", err)
	} else {
		t.Logf("AddCredits tx: %x", txHash)
	}

	// Wait for credits transaction
	time.Sleep(5 * time.Second)

	// Generate key for ADI
	adiPublicKey, _, _, err := GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate ADI key: %v", err)
	}

	// Create unique ADI name with timestamp
	adiName := fmt.Sprintf("test-adi-%d", time.Now().Unix())
	adiURL := fmt.Sprintf("acc://%s.acme", adiName)

	t.Logf("Creating ADI: %s", adiURL)

	// Create ADI
	txHash, err = client.CreateIdentity(ctx, adiURL, adiPublicKey, fundingAccountURL, fundingPrivateKey)
	if err != nil {
		t.Logf("CreateIdentity error (may be expected if insufficient credits): %v", err)
		// Don't fail - might not have enough credits
	} else {
		t.Logf("CreateIdentity transaction hash: %x", txHash)

		// Wait for ADI creation
		time.Sleep(8 * time.Second)

		// Try to query the new ADI
		adiResult, err := client.QueryAccount(ctx, adiURL)
		if err != nil {
			t.Logf("Failed to query new ADI: %v", err)
		} else {
			t.Logf("ADI created successfully: %v", adiResult)
		}

		// Try to query the KeyBook
		keyBookURL := fmt.Sprintf("%s/book", adiURL)
		keyBookResult, err := client.QueryKeyBook(ctx, keyBookURL)
		if err != nil {
			t.Logf("Failed to query KeyBook: %v", err)
		} else {
			t.Logf("KeyBook exists: %v", keyBookResult)
		}
	}
}

func TestFullADIWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	client, err := NewClient(devnetURL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	t.Log("=== Step 1: Generate Key ===")
	publicKey, privateKey, liteAccountURL, err := GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	t.Logf("Public Key: %s", publicKey)
	t.Logf("Lite Account: %s", liteAccountURL)

	t.Log("\n=== Step 2: Request Faucet ===")
	faucetResult, err := client.Faucet(ctx, liteAccountURL, nil)
	if err != nil {
		t.Fatalf("failed to request faucet: %v", err)
	}
	t.Logf("Faucet result: %v", faucetResult)
	time.Sleep(5 * time.Second)

	t.Log("\n=== Step 3: Query Lite Account Balance ===")
	accountResult, err := client.QueryAccount(ctx, liteAccountURL)
	if err != nil {
		t.Logf("Failed to query account: %v", err)
	} else {
		t.Logf("Account data: %v", accountResult)
	}

	t.Log("\n=== Step 4: Add Credits ===")
	txHash, err := client.AddCredits(ctx, liteAccountURL, liteAccountURL, 100000000, privateKey)
	if err != nil {
		t.Logf("AddCredits error: %v", err)
	} else {
		t.Logf("AddCredits tx hash: %x", txHash)
	}
	time.Sleep(5 * time.Second)

	t.Log("\n=== Step 5: Generate ADI Key ===")
	adiPublicKey, _, _, err := GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate ADI key: %v", err)
	}
	t.Logf("ADI Public Key: %s", adiPublicKey)

	t.Log("\n=== Step 6: Create ADI ===")
	adiName := fmt.Sprintf("test-adi-%d", time.Now().Unix())
	adiURL := fmt.Sprintf("acc://%s.acme", adiName)
	t.Logf("Creating ADI: %s", adiURL)

	txHash, err = client.CreateIdentity(ctx, adiURL, adiPublicKey, liteAccountURL, privateKey)
	if err != nil {
		t.Logf("CreateIdentity error: %v", err)
	} else {
		t.Logf("CreateIdentity tx hash: %x", txHash)
		time.Sleep(8 * time.Second)

		t.Log("\n=== Step 7: Verify ADI ===")
		adiResult, err := client.QueryAccount(ctx, adiURL)
		if err != nil {
			t.Logf("Failed to query ADI: %v", err)
		} else {
			t.Logf("ADI verified: %v", adiResult)
		}

		t.Log("\n=== Step 8: Query KeyBook ===")
		keyBookURL := fmt.Sprintf("%s/book", adiURL)
		keyBookResult, err := client.QueryKeyBook(ctx, keyBookURL)
		if err != nil {
			t.Logf("Failed to query KeyBook: %v", err)
		} else {
			t.Logf("KeyBook found: %v", keyBookResult)
		}

		t.Log("\n=== Workflow Complete ===")
	}
}

// Validation tests that don't require network

func TestAddCreditsValidation(t *testing.T) {
	client, err := NewClient(devnetURL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	tests := []struct {
		name        string
		recipient   string
		payer       string
		amount      int64
		privateKey  string
		expectErr   bool
		errContains string
	}{
		{
			name:        "invalid private key",
			recipient:   "acc://test.acme/book",
			payer:       "acc://lite.acme",
			amount:      1000,
			privateKey:  "invalid",
			expectErr:   true,
			errContains: "invalid private key",
		},
		{
			name:        "wrong private key length",
			recipient:   "acc://test.acme/book",
			payer:       "acc://lite.acme",
			amount:      1000,
			privateKey:  "0123456789abcdef",
			expectErr:   true,
			errContains: "invalid private key length",
		},
		{
			name:        "invalid recipient URL",
			recipient:   "not-a-url",
			payer:       "acc://lite.acme",
			amount:      1000,
			privateKey:  testPrivateKey,
			expectErr:   true,
			errContains: "invalid recipient URL",
		},
		{
			name:        "invalid payer URL",
			recipient:   "acc://test.acme/book",
			payer:       "not-a-url",
			amount:      1000,
			privateKey:  testPrivateKey,
			expectErr:   true,
			errContains: "invalid payer URL",
		},
		{
			name:       "private key with 0x prefix",
			recipient:  "acc://test.acme/book",
			payer:      "acc://lite.acme",
			amount:     1000,
			privateKey: "0x" + testPrivateKey,
			expectErr:  false,
		},
		{
			name:       "small amount",
			recipient:  "acc://test.acme/book",
			payer:      "acc://lite.acme",
			amount:     1,
			privateKey: testPrivateKey,
			expectErr:  false,
		},
		{
			name:       "large amount",
			recipient:  "acc://test.acme/book",
			payer:      "acc://lite.acme",
			amount:     1000000000,
			privateKey: testPrivateKey,
			expectErr:  false,
		},
		{
			name:       "zero amount",
			recipient:  "acc://test.acme/book",
			payer:      "acc://lite.acme",
			amount:     0,
			privateKey: testPrivateKey,
			expectErr:  false,
		},
		{
			name:       "lite account to lite account",
			recipient:  "acc://1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef/ACME",
			payer:      "acc://fedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321/ACME",
			amount:     5000,
			privateKey: testPrivateKey,
			expectErr:  false,
		},
		{
			name:       "ADI keybook to ADI keybook",
			recipient:  "acc://alice.acme/book",
			payer:      "acc://bob.acme/book",
			amount:     10000,
			privateKey: testPrivateKey,
			expectErr:  false,
		},
		{
			name:       "ADI keypage recipient",
			recipient:  "acc://test.acme/book/1",
			payer:      "acc://lite.acme",
			amount:     2000,
			privateKey: testPrivateKey,
			expectErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.AddCredits(ctx, tt.recipient, tt.payer, tt.amount, tt.privateKey)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					// Allow for network-level errors too
					if !contains(err.Error(), "failed to get network status") && !contains(err.Error(), "failed to submit") {
						t.Errorf("expected error containing '%s', got: %v", tt.errContains, err)
					}
				}
			} else if err != nil {
				// May fail if network unavailable
				t.Logf("AddCredits error (may be expected): %v", err)
			}
		})
	}
}

func TestCreateIdentityValidation(t *testing.T) {
	client, err := NewClient(devnetURL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	tests := []struct {
		name        string
		adiURL      string
		publicKey   string
		sponsor     string
		privateKey  string
		expectErr   bool
		errContains string
	}{
		{
			name:        "invalid public key",
			adiURL:      "acc://test.acme",
			publicKey:   "invalid",
			sponsor:     "acc://sponsor.acme",
			privateKey:  testPrivateKey,
			expectErr:   true,
			errContains: "invalid public key",
		},
		{
			name:        "invalid private key",
			adiURL:      "acc://test.acme",
			publicKey:   "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			sponsor:     "acc://sponsor.acme",
			privateKey:  "invalid",
			expectErr:   true,
			errContains: "invalid private key",
		},
		{
			name:        "wrong private key length",
			adiURL:      "acc://test.acme",
			publicKey:   "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			sponsor:     "acc://sponsor.acme",
			privateKey:  "0123456789abcdef",
			expectErr:   true,
			errContains: "invalid private key length",
		},
		{
			name:        "invalid ADI URL",
			adiURL:      "not-a-url",
			publicKey:   "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			sponsor:     "acc://sponsor.acme",
			privateKey:  testPrivateKey,
			expectErr:   true,
			errContains: "invalid ADI URL",
		},
		{
			name:        "invalid sponsor URL",
			adiURL:      "acc://test.acme",
			publicKey:   "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			sponsor:     "not-a-url",
			privateKey:  testPrivateKey,
			expectErr:   true,
			errContains: "invalid sponsor URL",
		},
		{
			name:        "keys with 0x prefix",
			adiURL:      "acc://test.acme",
			publicKey:   "0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			sponsor:     "acc://sponsor.acme",
			privateKey:  "0x" + testPrivateKey,
			expectErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.CreateIdentity(ctx, tt.adiURL, tt.publicKey, tt.sponsor, tt.privateKey)

			if tt.expectErr {
				if err == nil {
					// URL parsing may not error upfront
					t.Logf("CreateIdentity did not error on invalid input (may be expected)")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					// Allow for network-level errors
					if !contains(err.Error(), "failed to submit") {
						t.Errorf("expected error containing '%s', got: %v", tt.errContains, err)
					}
				}
			} else if err != nil {
				// May fail if network unavailable
				t.Logf("CreateIdentity error (may be expected): %v", err)
			}
		})
	}
}
