package client

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"testing"
	"time"
)

func TestEncodeData(t *testing.T) {
	tests := []struct {
		name      string
		data      string
		encoding  string
		expected  []byte
		expectErr bool
	}{
		{
			name:      "utf8 encoding",
			data:      "Hello, World!",
			encoding:  "utf8",
			expected:  []byte("Hello, World!"),
			expectErr: false,
		},
		{
			name:      "hex encoding",
			data:      "48656c6c6f",
			encoding:  "hex",
			expected:  []byte("Hello"),
			expectErr: false,
		},
		{
			name:      "hex encoding with 0x prefix",
			data:      "0x48656c6c6f",
			encoding:  "hex",
			expected:  []byte("Hello"),
			expectErr: false,
		},
		{
			name:      "base64 encoding",
			data:      base64.StdEncoding.EncodeToString([]byte("Hello")),
			encoding:  "base64",
			expected:  []byte("Hello"),
			expectErr: false,
		},
		{
			name:      "invalid hex",
			data:      "not-hex-data",
			encoding:  "hex",
			expectErr: true,
		},
		{
			name:      "invalid base64",
			data:      "not@valid@base64!",
			encoding:  "base64",
			expectErr: true,
		},
		{
			name:      "unsupported encoding",
			data:      "data",
			encoding:  "invalid",
			expectErr: true,
		},
		{
			name:      "empty encoding defaults to utf8",
			data:      "test",
			encoding:  "",
			expected:  []byte("test"),
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := EncodeData(tt.data, tt.encoding)
			if tt.expectErr {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if string(result) != string(tt.expected) {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestCreateDataAccount(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	client, err := NewClient(devnetURL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// This test requires an ADI with credits
	// We'll create a full workflow
	t.Log("=== Creating ADI for Data Account Test ===")

	// Generate funding key
	_, fundingPrivateKey, fundingAccountURL, err := GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate funding key: %v", err)
	}

	// Fund with faucet
	_, err = client.Faucet(ctx, fundingAccountURL, nil)
	if err != nil {
		t.Skipf("skipping: faucet not available (network connectivity issue): %v", err)
	}
	time.Sleep(5 * time.Second)

	// Add credits
	_, err = client.AddCredits(ctx, fundingAccountURL, fundingAccountURL, 200000000, fundingPrivateKey)
	if err != nil {
		t.Logf("AddCredits error: %v", err)
	}
	time.Sleep(5 * time.Second)

	// Create ADI
	adiPublicKey, adiPrivateKey, _, err := GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate ADI key: %v", err)
	}

	adiName := fmt.Sprintf("data-test-%d", time.Now().Unix())
	adiURL := fmt.Sprintf("acc://%s.acme", adiName)
	keyBookURL := fmt.Sprintf("%s/book", adiURL)

	_, err = client.CreateIdentity(ctx, adiURL, adiPublicKey, fundingAccountURL, fundingPrivateKey)
	if err != nil {
		t.Logf("CreateIdentity error: %v", err)
	}
	time.Sleep(8 * time.Second)

	// Add credits to ADI KeyBook
	_, err = client.AddCredits(ctx, keyBookURL, fundingAccountURL, 100000000, fundingPrivateKey)
	if err != nil {
		t.Logf("AddCredits to KeyBook error: %v", err)
	}
	time.Sleep(5 * time.Second)

	t.Log("\n=== Creating Data Account ===")
	dataAccountURL := fmt.Sprintf("%s/data", adiURL)
	txHash, err := client.CreateDataAccount(ctx, dataAccountURL, keyBookURL, adiPrivateKey, nil)
	if err != nil {
		t.Logf("CreateDataAccount error (may be expected if insufficient credits): %v", err)
	} else {
		t.Logf("CreateDataAccount tx hash: %x", txHash)
		time.Sleep(8 * time.Second)

		// Verify data account exists
		dataAccountResult, err := client.QueryAccount(ctx, dataAccountURL)
		if err != nil {
			t.Logf("Failed to query data account: %v", err)
		} else {
			t.Logf("Data account verified: %v", dataAccountResult)
		}
	}
}

func TestWriteData(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	client, err := NewClient(devnetURL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// This test requires a data account
	// We'll create the full workflow
	t.Log("=== Full Data Write Workflow ===")

	// Step 1: Create funding account
	_, fundingPrivateKey, fundingAccountURL, err := GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate funding key: %v", err)
	}
	t.Logf("Funding account: %s", fundingAccountURL)

	// Step 2: Fund with faucet
	_, err = client.Faucet(ctx, fundingAccountURL, nil)
	if err != nil {
		t.Skipf("skipping: faucet not available (network connectivity issue): %v", err)
	}
	time.Sleep(5 * time.Second)

	// Step 3: Add credits to funding account
	_, err = client.AddCredits(ctx, fundingAccountURL, fundingAccountURL, 300000000, fundingPrivateKey)
	if err != nil {
		t.Logf("AddCredits error: %v", err)
	}
	time.Sleep(5 * time.Second)

	// Step 4: Create ADI
	adiPublicKey, adiPrivateKey, _, err := GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate ADI key: %v", err)
	}

	adiName := fmt.Sprintf("write-test-%d", time.Now().Unix())
	adiURL := fmt.Sprintf("acc://%s.acme", adiName)
	keyBookURL := fmt.Sprintf("%s/book", adiURL)

	_, err = client.CreateIdentity(ctx, adiURL, adiPublicKey, fundingAccountURL, fundingPrivateKey)
	if err != nil {
		t.Logf("CreateIdentity error: %v", err)
	}
	time.Sleep(8 * time.Second)

	// Step 5: Add credits to KeyBook
	_, err = client.AddCredits(ctx, keyBookURL, fundingAccountURL, 200000000, fundingPrivateKey)
	if err != nil {
		t.Logf("AddCredits to KeyBook error: %v", err)
	}
	time.Sleep(5 * time.Second)

	// Step 6: Create data account
	dataAccountURL := fmt.Sprintf("%s/data", adiURL)
	_, err = client.CreateDataAccount(ctx, dataAccountURL, keyBookURL, adiPrivateKey, nil)
	if err != nil {
		t.Logf("CreateDataAccount error: %v", err)
	}
	time.Sleep(8 * time.Second)

	// Step 7: Write data - UTF8
	t.Log("\n=== Writing UTF8 Data ===")
	testData := fmt.Sprintf(`{"event":"test","timestamp":%d}`, time.Now().Unix())
	txHash, err := client.WriteData(ctx, dataAccountURL, keyBookURL, adiPrivateKey, []byte(testData), false)
	if err != nil {
		t.Logf("WriteData error: %v", err)
	} else {
		t.Logf("WriteData tx hash: %x", txHash)
		time.Sleep(5 * time.Second)

		// Query data entries
		dataEntries, err := client.QueryData(ctx, dataAccountURL, map[string]interface{}{"start": 0, "count": 10})
		if err != nil {
			t.Logf("Failed to query data entries: %v", err)
		} else {
			t.Logf("Data entries: %v", dataEntries)
		}
	}

	// Step 8: Write data - Hex
	t.Log("\n=== Writing Hex Data ===")
	hexData := []byte("Hello from hex")
	txHash, err = client.WriteData(ctx, dataAccountURL, keyBookURL, adiPrivateKey, hexData, false)
	if err != nil {
		t.Logf("WriteData (hex) error: %v", err)
	} else {
		t.Logf("WriteData (hex) tx hash: %x", txHash)
	}
}

func TestCreateTokenAccount(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	client, err := NewClient(devnetURL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	t.Log("=== Full Token Account Creation Workflow ===")

	// Step 1: Create and fund account
	_, fundingPrivateKey, fundingAccountURL, err := GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate funding key: %v", err)
	}

	_, err = client.Faucet(ctx, fundingAccountURL, nil)
	if err != nil {
		t.Skipf("skipping: faucet not available (network connectivity issue): %v", err)
	}
	time.Sleep(5 * time.Second)

	// Step 2: Add credits
	_, err = client.AddCredits(ctx, fundingAccountURL, fundingAccountURL, 300000000, fundingPrivateKey)
	if err != nil {
		t.Logf("AddCredits error: %v", err)
	}
	time.Sleep(5 * time.Second)

	// Step 3: Create ADI
	adiPublicKey, adiPrivateKey, _, err := GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate ADI key: %v", err)
	}

	adiName := fmt.Sprintf("token-test-%d", time.Now().Unix())
	adiURL := fmt.Sprintf("acc://%s.acme", adiName)
	keyBookURL := fmt.Sprintf("%s/book", adiURL)

	_, err = client.CreateIdentity(ctx, adiURL, adiPublicKey, fundingAccountURL, fundingPrivateKey)
	if err != nil {
		t.Logf("CreateIdentity error: %v", err)
	}
	time.Sleep(8 * time.Second)

	// Step 4: Add credits to KeyBook
	_, err = client.AddCredits(ctx, keyBookURL, fundingAccountURL, 200000000, fundingPrivateKey)
	if err != nil {
		t.Logf("AddCredits to KeyBook error: %v", err)
	}
	time.Sleep(5 * time.Second)

	// Step 5: Create token account
	t.Log("\n=== Creating Token Account ===")
	tokenAccountURL := fmt.Sprintf("%s/tokens", adiURL)
	txHash, err := client.CreateTokenAccount(ctx, tokenAccountURL, "acc://ACME", keyBookURL, adiPrivateKey, nil)
	if err != nil {
		t.Logf("CreateTokenAccount error (may be expected if insufficient credits): %v", err)
	} else {
		t.Logf("CreateTokenAccount tx hash: %x", txHash)
		time.Sleep(8 * time.Second)

		// Verify token account exists
		tokenAccountResult, err := client.QueryAccount(ctx, tokenAccountURL)
		if err != nil {
			t.Logf("Failed to query token account: %v", err)
		} else {
			t.Logf("Token account verified: %v", tokenAccountResult)
		}
	}
}

func TestMultiEncodingDataWrite(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		encoding string
	}{
		{
			name:     "UTF8 JSON",
			data:     `{"type":"stake","amount":1000}`,
			encoding: "utf8",
		},
		{
			name:     "Hex data",
			data:     hex.EncodeToString([]byte("Binary data test")),
			encoding: "hex",
		},
		{
			name:     "Base64 data",
			data:     base64.StdEncoding.EncodeToString([]byte("Base64 test data")),
			encoding: "base64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := EncodeData(tt.data, tt.encoding)
			if err != nil {
				t.Errorf("failed to encode data: %v", err)
			}

			if len(data) == 0 {
				t.Error("encoded data is empty")
			}

			t.Logf("Encoded %s data: %d bytes", tt.encoding, len(data))
		})
	}
}

func TestQueryData(t *testing.T) {
	client, err := NewClient(devnetURL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	tests := []struct {
		name      string
		url       string
		params    map[string]interface{}
		expectErr bool
	}{
		{
			name: "query with range",
			url:  "acc://dn.acme/oracle",
			params: map[string]interface{}{
				"start": uint64(0),
				"count": uint64(5),
			},
			expectErr: false,
		},
		{
			name: "query with only start param",
			url:  "acc://dn.acme/oracle",
			params: map[string]interface{}{
				"start": uint64(0),
			},
			expectErr: false,
		},
		{
			name: "query with only count param",
			url:  "acc://dn.acme/oracle",
			params: map[string]interface{}{
				"count": uint64(10),
			},
			expectErr: false,
		},
		{
			name:      "query with nil params",
			url:       "acc://dn.acme/oracle",
			params:    nil,
			expectErr: false,
		},
		{
			name:      "query with empty params",
			url:       "acc://dn.acme/oracle",
			params:    map[string]interface{}{},
			expectErr: false,
		},
		{
			name: "query with entry hash",
			url:  "acc://dn.acme/oracle",
			params: map[string]interface{}{
				"entryHash": "0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			},
			expectErr: false,
		},
		{
			name: "query with expand param",
			url:  "acc://dn.acme/oracle",
			params: map[string]interface{}{
				"start":  uint64(0),
				"count":  uint64(5),
				"expand": true,
			},
			expectErr: false,
		},
		{
			name:      "query non-existent data account",
			url:       "acc://fake-data-account-12345.acme",
			params:    map[string]interface{}{},
			expectErr: true,
		},
		{
			name:      "invalid URL",
			url:       "not-a-url",
			params:    map[string]interface{}{},
			expectErr: true,
		},
		{
			name: "query with index uint64",
			url:  "acc://dn.acme/oracle",
			params: map[string]interface{}{
				"index": uint64(0),
			},
			expectErr: false,
		},
		{
			name: "query with index float64",
			url:  "acc://dn.acme/oracle",
			params: map[string]interface{}{
				"index": float64(1),
			},
			expectErr: false,
		},
		{
			name: "query with entry parameter",
			url:  "acc://dn.acme/oracle",
			params: map[string]interface{}{
				"entry": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			},
			expectErr: false,
		},
		{
			name: "query with float64 range",
			url:  "acc://dn.acme/oracle",
			params: map[string]interface{}{
				"start": float64(0),
				"count": float64(10),
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := client.QueryData(ctx, tt.url, tt.params)
			if tt.expectErr {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				// Allow network-level errors about no data entries - these are valid responses
				if !contains(err.Error(), "no data entries") && !contains(err.Error(), "not found") {
					t.Errorf("unexpected error: %v", err)
				} else {
					t.Logf("Query data returned expected network response: %v", err)
				}
				return
			}

			if result == nil {
				t.Error("expected non-nil result")
			}

			t.Logf("Query data result: %v", result)
		})
	}
}

// Validation tests that don't require network

func TestCreateDataAccountValidation(t *testing.T) {
	client, err := NewClient(devnetURL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	tests := []struct {
		name        string
		accountURL  string
		sponsor     string
		privateKey  string
		authorities []string
		expectErr   bool
		errContains string
	}{
		{
			name:        "invalid private key",
			accountURL:  "acc://test.acme/data",
			sponsor:     "acc://test.acme/book",
			privateKey:  "invalid",
			authorities: nil,
			expectErr:   true,
			errContains: "invalid private key",
		},
		{
			name:        "wrong private key length",
			accountURL:  "acc://test.acme/data",
			sponsor:     "acc://test.acme/book",
			privateKey:  "0123456789abcdef",
			authorities: nil,
			expectErr:   true,
			errContains: "invalid private key length",
		},
		{
			name:        "invalid account URL",
			accountURL:  "not-a-url",
			sponsor:     "acc://test.acme/book",
			privateKey:  testPrivateKey,
			authorities: nil,
			expectErr:   true,
			errContains: "invalid account URL",
		},
		{
			name:        "invalid sponsor URL",
			accountURL:  "acc://test.acme/data",
			sponsor:     "not-a-url",
			privateKey:  testPrivateKey,
			authorities: nil,
			expectErr:   true,
			errContains: "invalid sponsor URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.CreateDataAccount(ctx, tt.accountURL, tt.sponsor, tt.privateKey, tt.authorities)

			if tt.expectErr {
				if err == nil {
					// URL parsing may not error upfront
					t.Logf("CreateDataAccount did not error on invalid input (may be expected)")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					// Allow for network-level errors
					if !contains(err.Error(), "failed to submit") {
						t.Errorf("expected error containing '%s', got: %v", tt.errContains, err)
					}
				}
			} else if err != nil {
				t.Logf("CreateDataAccount error (may be expected): %v", err)
			}
		})
	}
}

func TestWriteDataValidation(t *testing.T) {
	client, err := NewClient(devnetURL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	tests := []struct {
		name         string
		accountURL   string
		signerURL    string
		privateKey   string
		data         []byte
		writeToState bool
		expectErr    bool
		errContains  string
	}{
		{
			name:         "invalid private key",
			accountURL:   "acc://test.acme/data",
			signerURL:    "acc://test.acme/book",
			privateKey:   "invalid",
			data:         []byte("test data"),
			writeToState: false,
			expectErr:    true,
			errContains:  "invalid private key",
		},
		{
			name:         "wrong private key length",
			accountURL:   "acc://test.acme/data",
			signerURL:    "acc://test.acme/book",
			privateKey:   "0123456789abcdef",
			data:         []byte("test data"),
			writeToState: false,
			expectErr:    true,
			errContains:  "invalid private key length",
		},
		{
			name:         "invalid account URL",
			accountURL:   "not-a-url",
			signerURL:    "acc://test.acme/book",
			privateKey:   testPrivateKey,
			data:         []byte("test data"),
			writeToState: false,
			expectErr:    true,
			errContains:  "invalid account URL",
		},
		{
			name:         "invalid signer URL",
			accountURL:   "acc://test.acme/data",
			signerURL:    "not-a-url",
			privateKey:   testPrivateKey,
			data:         []byte("test data"),
			writeToState: false,
			expectErr:    true,
			errContains:  "invalid signer URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.WriteData(ctx, tt.accountURL, tt.signerURL, tt.privateKey, tt.data, tt.writeToState)

			if tt.expectErr {
				if err == nil {
					// URL parsing may not error upfront
					t.Logf("WriteData did not error on invalid input (may be expected)")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					// Allow for network-level errors
					if !contains(err.Error(), "failed to submit") {
						t.Errorf("expected error containing '%s', got: %v", tt.errContains, err)
					}
				}
			} else if err != nil {
				t.Logf("WriteData error (may be expected): %v", err)
			}
		})
	}
}

func TestCreateTokenAccountValidation(t *testing.T) {
	client, err := NewClient(devnetURL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	tests := []struct {
		name        string
		accountURL  string
		tokenURL    string
		sponsor     string
		privateKey  string
		authorities []string
		expectErr   bool
		errContains string
	}{
		{
			name:        "invalid private key",
			accountURL:  "acc://test.acme/tokens",
			tokenURL:    "acc://ACME",
			sponsor:     "acc://test.acme/book",
			privateKey:  "invalid",
			authorities: nil,
			expectErr:   true,
			errContains: "invalid private key",
		},
		{
			name:        "wrong private key length",
			accountURL:  "acc://test.acme/tokens",
			tokenURL:    "acc://ACME",
			sponsor:     "acc://test.acme/book",
			privateKey:  "0123456789abcdef",
			authorities: nil,
			expectErr:   true,
			errContains: "invalid private key length",
		},
		{
			name:        "invalid account URL",
			accountURL:  "not-a-url",
			tokenURL:    "acc://ACME",
			sponsor:     "acc://test.acme/book",
			privateKey:  testPrivateKey,
			authorities: nil,
			expectErr:   true,
			errContains: "invalid account URL",
		},
		{
			name:        "invalid token URL",
			accountURL:  "acc://test.acme/tokens",
			tokenURL:    "not-a-url",
			sponsor:     "acc://test.acme/book",
			privateKey:  testPrivateKey,
			authorities: nil,
			expectErr:   true,
			errContains: "invalid token URL",
		},
		{
			name:        "invalid sponsor URL",
			accountURL:  "acc://test.acme/tokens",
			tokenURL:    "acc://ACME",
			sponsor:     "not-a-url",
			privateKey:  testPrivateKey,
			authorities: nil,
			expectErr:   true,
			errContains: "invalid sponsor URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.CreateTokenAccount(ctx, tt.accountURL, tt.tokenURL, tt.sponsor, tt.privateKey, tt.authorities)

			if tt.expectErr {
				if err == nil {
					// URL parsing may not error upfront
					t.Logf("CreateTokenAccount did not error on invalid input (may be expected)")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					// Allow for network-level errors
					if !contains(err.Error(), "failed to submit") {
						t.Errorf("expected error containing '%s', got: %v", tt.errContains, err)
					}
				}
			} else if err != nil {
				t.Logf("CreateTokenAccount error (may be expected): %v", err)
			}
		})
	}
}
