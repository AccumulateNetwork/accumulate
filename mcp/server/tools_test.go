package server

import (
	"strings"
	"testing"
)

// Test Query Account tool
func TestQueryAccount(t *testing.T) {
	server := NewServer()

	tests := []struct {
		name      string
		args      map[string]interface{}
		expectErr bool
	}{
		{
			name: "missing url",
			args: map[string]interface{}{
				"network": "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "invalid network URL",
			args: map[string]interface{}{
				"url":     "acc://test.acme",
				"network": "not-a-url",
			},
			expectErr: true,
		},
		{
			name: "valid parameters",
			args: map[string]interface{}{
				"url":     "acc://dn.acme",
				"network": "http://127.0.0.1:26660/v3",
			},
			expectErr: false, // Will fail with network error, but not validation error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.queryAccount(tt.args)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			}
			// Note: non-validation errors (network, etc.) are expected when testing with fake URLs
		})
	}
}

// Test Query Transaction tool
func TestQueryTransaction(t *testing.T) {
	server := NewServer()

	tests := []struct {
		name      string
		args      map[string]interface{}
		expectErr bool
	}{
		{
			name: "missing tx_hash",
			args: map[string]interface{}{
				"network": "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "valid parameters",
			args: map[string]interface{}{
				"tx_hash": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"network": "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.queryTransaction(tt.args)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			}
		})
	}
}

// Test Send Tokens tool
func TestSendTokens(t *testing.T) {
	server := NewServer()

	tests := []struct {
		name      string
		args      map[string]interface{}
		expectErr bool
		errMsg    string
	}{
		{
			name: "missing from",
			args: map[string]interface{}{
				"to":      "acc://recipient.acme/tokens",
				"amount":  "1000000",
				"key":     "0123456789abcdef",
				"network": "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "missing to",
			args: map[string]interface{}{
				"from":    "acc://sender.acme/tokens",
				"amount":  "1000000",
				"key":     "0123456789abcdef",
				"network": "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "missing amount",
			args: map[string]interface{}{
				"from":    "acc://sender.acme/tokens",
				"to":      "acc://recipient.acme/tokens",
				"key":     "0123456789abcdef",
				"network": "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "missing key",
			args: map[string]interface{}{
				"from":    "acc://sender.acme/tokens",
				"to":      "acc://recipient.acme/tokens",
				"amount":  "1000000",
				"network": "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "valid parameters",
			args: map[string]interface{}{
				"from":    "acc://sender.acme/tokens",
				"to":      "acc://recipient.acme/tokens",
				"amount":  "1000000",
				"key":     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"network": "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.sendTokens(tt.args)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			}
		})
	}
}

// Test Generate Key tool
func TestGenerateKey(t *testing.T) {
	server := NewServer()

	// Generate key should work with no arguments
	result, err := server.generateKey(map[string]interface{}{})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Check result structure
	content, ok := result["content"].([]map[string]interface{})
	if !ok {
		t.Fatal("expected content to be an array")
	}

	if len(content) == 0 {
		t.Fatal("expected non-empty content array")
	}

	text, ok := content[0]["text"].(string)
	if !ok {
		t.Fatal("expected text to be a string")
	}

	// Should contain public key, private key, and lite account URL (in JSON format)
	if !strings.Contains(text, "publicKey") {
		t.Error("expected result to contain 'publicKey'")
	}

	if !strings.Contains(text, "privateKey") {
		t.Error("expected result to contain 'privateKey'")
	}

	if !strings.Contains(text, "liteAccountURL") {
		t.Error("expected result to contain 'liteAccountURL'")
	}
}

// Test Create ADI tool
func TestCreateADI(t *testing.T) {
	server := NewServer()

	tests := []struct {
		name      string
		args      map[string]interface{}
		expectErr bool
	}{
		{
			name: "missing url",
			args: map[string]interface{}{
				"public_key": "0123456789abcdef",
				"key":        "0123456789abcdef",
				"key_book":   "acc://test.acme/book",
				"key_page":   "acc://test.acme/book/1",
				"network":    "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "missing public_key",
			args: map[string]interface{}{
				"url":      "acc://test.acme",
				"key":      "0123456789abcdef",
				"key_book": "acc://test.acme/book",
				"key_page": "acc://test.acme/book/1",
				"network":  "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "valid parameters",
			args: map[string]interface{}{
				"url":        "acc://test.acme",
				"public_key": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"key":        "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"key_book":   "acc://test.acme/book",
				"key_page":   "acc://test.acme/book/1",
				"network":    "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.createADI(tt.args)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			}
		})
	}
}

// Test Add Credits tool
func TestAddCredits(t *testing.T) {
	server := NewServer()

	tests := []struct {
		name      string
		args      map[string]interface{}
		expectErr bool
	}{
		{
			name: "missing recipient",
			args: map[string]interface{}{
				"sender":  "acc://lite-account",
				"amount":  "100000",
				"key":     "0123456789abcdef",
				"network": "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "missing amount",
			args: map[string]interface{}{
				"recipient": "acc://test.acme/book/1",
				"sender":    "acc://lite-account",
				"key":       "0123456789abcdef",
				"network":   "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "valid parameters",
			args: map[string]interface{}{
				"recipient": "acc://test.acme/book/1",
				"sender":    "acc://lite-account",
				"amount":    "100000",
				"key":       "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"network":   "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.addCredits(tt.args)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			}
		})
	}
}

// Test Create Data Account tool
func TestCreateDataAccount(t *testing.T) {
	server := NewServer()

	tests := []struct {
		name      string
		args      map[string]interface{}
		expectErr bool
	}{
		{
			name: "missing url",
			args: map[string]interface{}{
				"signer":             "acc://test.acme/book",
				"signer_private_key": "0123456789abcdef",
				"network":            "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "missing signer",
			args: map[string]interface{}{
				"url":                "acc://test.acme/data",
				"signer_private_key": "0123456789abcdef",
				"network":            "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "valid parameters",
			args: map[string]interface{}{
				"url":                "acc://test.acme/data",
				"signer":             "acc://test.acme/book",
				"signer_private_key": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"network":            "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.createDataAccount(tt.args)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			}
		})
	}
}

// Test Write Data tool
func TestWriteData(t *testing.T) {
	server := NewServer()

	tests := []struct {
		name      string
		args      map[string]interface{}
		expectErr bool
	}{
		{
			name: "missing account_url",
			args: map[string]interface{}{
				"data":               "test data",
				"signer":             "acc://test.acme/book",
				"signer_private_key": "0123456789abcdef",
				"network":            "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "missing data",
			args: map[string]interface{}{
				"account_url":        "acc://test.acme/data",
				"signer":             "acc://test.acme/book",
				"signer_private_key": "0123456789abcdef",
				"network":            "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "valid text data",
			args: map[string]interface{}{
				"account_url":        "acc://test.acme/data",
				"data":               "test data",
				"signer":             "acc://test.acme/book",
				"signer_private_key": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"network":            "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
		{
			name: "valid json data",
			args: map[string]interface{}{
				"account_url":        "acc://test.acme/data",
				"data":               map[string]interface{}{"key": "value"},
				"signer":             "acc://test.acme/book",
				"signer_private_key": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"network":            "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.writeData(tt.args)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			}
		})
	}
}

// Test Create Token Account tool
func TestCreateTokenAccount(t *testing.T) {
	server := NewServer()

	tests := []struct {
		name      string
		args      map[string]interface{}
		expectErr bool
	}{
		{
			name: "missing url",
			args: map[string]interface{}{
				"token_url":          "acc://ACME",
				"signer":             "acc://test.acme/book",
				"signer_private_key": "0123456789abcdef",
				"network":            "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "missing token_url",
			args: map[string]interface{}{
				"url":                "acc://test.acme/tokens",
				"signer":             "acc://test.acme/book",
				"signer_private_key": "0123456789abcdef",
				"network":            "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "valid parameters",
			args: map[string]interface{}{
				"url":                "acc://test.acme/tokens",
				"token_url":          "acc://ACME",
				"signer":             "acc://test.acme/book",
				"signer_private_key": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"network":            "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.createTokenAccount(tt.args)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			}
		})
	}
}

// Test Create KeyPage tool
func TestCreateKeyPage(t *testing.T) {
	server := NewServer()

	tests := []struct {
		name      string
		args      map[string]interface{}
		expectErr bool
	}{
		{
			name: "missing keybook_url",
			args: map[string]interface{}{
				"keys":               []interface{}{"key1", "key2"},
				"signer":             "acc://test.acme/book",
				"signer_private_key": "0123456789abcdef",
				"network":            "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "missing keys",
			args: map[string]interface{}{
				"keybook_url":        "acc://test.acme/book",
				"signer":             "acc://test.acme/book",
				"signer_private_key": "0123456789abcdef",
				"network":            "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "valid parameters",
			args: map[string]interface{}{
				"keybook_url": "acc://test.acme/book",
				"keys": []interface{}{
					"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
					"fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210",
				},
				"signer":             "acc://test.acme/book",
				"signer_private_key": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"network":            "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.createKeyPage(tt.args)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			}
		})
	}
}

// Test Update KeyPage tool
func TestUpdateKeyPage(t *testing.T) {
	server := NewServer()

	tests := []struct {
		name      string
		args      map[string]interface{}
		expectErr bool
	}{
		{
			name: "missing keypage_url",
			args: map[string]interface{}{
				"operation":          "add",
				"key":                "0123456789abcdef",
				"signer":             "acc://test.acme/book",
				"signer_private_key": "0123456789abcdef",
				"network":            "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "missing operation",
			args: map[string]interface{}{
				"keypage_url":        "acc://test.acme/book/1",
				"key":                "0123456789abcdef",
				"signer":             "acc://test.acme/book",
				"signer_private_key": "0123456789abcdef",
				"network":            "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "valid add operation",
			args: map[string]interface{}{
				"keypage_url":        "acc://test.acme/book/1",
				"operation":          "add",
				"key":                "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"signer":             "acc://test.acme/book",
				"signer_private_key": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"network":            "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
		{
			name: "valid set_threshold operation",
			args: map[string]interface{}{
				"keypage_url":        "acc://test.acme/book/1",
				"operation":          "set_threshold",
				"threshold":          float64(2),
				"signer":             "acc://test.acme/book",
				"signer_private_key": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"network":            "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.updateKeyPage(tt.args)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			}
		})
	}
}

// Test Create Token tool
func TestCreateToken(t *testing.T) {
	server := NewServer()

	tests := []struct {
		name      string
		args      map[string]interface{}
		expectErr bool
	}{
		{
			name: "missing url",
			args: map[string]interface{}{
				"symbol":             "MTK",
				"signer":             "acc://test.acme/book",
				"signer_private_key": "0123456789abcdef",
				"network":            "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "missing symbol",
			args: map[string]interface{}{
				"url":                "acc://test.acme/token",
				"signer":             "acc://test.acme/book",
				"signer_private_key": "0123456789abcdef",
				"network":            "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "valid parameters with defaults",
			args: map[string]interface{}{
				"url":                "acc://test.acme/token",
				"symbol":             "MTK",
				"signer":             "acc://test.acme/book",
				"signer_private_key": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"network":            "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
		{
			name: "valid parameters with precision",
			args: map[string]interface{}{
				"url":                "acc://test.acme/token",
				"symbol":             "MTK",
				"precision":          float64(18),
				"signer":             "acc://test.acme/book",
				"signer_private_key": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"network":            "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
		{
			name: "valid parameters with supply limit",
			args: map[string]interface{}{
				"url":                "acc://test.acme/token",
				"symbol":             "MTK",
				"supply_limit":       "1000000000",
				"signer":             "acc://test.acme/book",
				"signer_private_key": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"network":            "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.createToken(tt.args)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			}
		})
	}
}

// Test Issue Tokens tool
func TestIssueTokens(t *testing.T) {
	server := NewServer()

	tests := []struct {
		name      string
		args      map[string]interface{}
		expectErr bool
	}{
		{
			name: "missing token_url",
			args: map[string]interface{}{
				"recipient":          "acc://recipient.acme/tokens",
				"amount":             "1000000",
				"signer":             "acc://test.acme/book",
				"signer_private_key": "0123456789abcdef",
				"network":            "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "missing recipient",
			args: map[string]interface{}{
				"token_url":          "acc://test.acme/token",
				"amount":             "1000000",
				"signer":             "acc://test.acme/book",
				"signer_private_key": "0123456789abcdef",
				"network":            "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "missing amount",
			args: map[string]interface{}{
				"token_url":          "acc://test.acme/token",
				"recipient":          "acc://recipient.acme/tokens",
				"signer":             "acc://test.acme/book",
				"signer_private_key": "0123456789abcdef",
				"network":            "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "valid parameters",
			args: map[string]interface{}{
				"token_url":          "acc://test.acme/token",
				"recipient":          "acc://recipient.acme/tokens",
				"amount":             "1000000",
				"signer":             "acc://test.acme/book",
				"signer_private_key": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"network":            "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.issueTokens(tt.args)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			}
		})
	}
}

// Test Burn Tokens tool
func TestBurnTokens(t *testing.T) {
	server := NewServer()

	tests := []struct {
		name      string
		args      map[string]interface{}
		expectErr bool
	}{
		{
			name: "missing account_url",
			args: map[string]interface{}{
				"amount":             "1000000",
				"signer":             "acc://test.acme/book",
				"signer_private_key": "0123456789abcdef",
				"network":            "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "missing amount",
			args: map[string]interface{}{
				"account_url":        "acc://test.acme/tokens",
				"signer":             "acc://test.acme/book",
				"signer_private_key": "0123456789abcdef",
				"network":            "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "valid parameters",
			args: map[string]interface{}{
				"account_url":        "acc://test.acme/tokens",
				"amount":             "1000000",
				"signer":             "acc://test.acme/book",
				"signer_private_key": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"network":            "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.burnTokens(tt.args)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			}
		})
	}
}

// Test Query Chain tool
func TestQueryChain(t *testing.T) {
	server := NewServer()

	tests := []struct {
		name      string
		args      map[string]interface{}
		expectErr bool
	}{
		{
			name: "missing url",
			args: map[string]interface{}{
				"network": "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "valid parameters",
			args: map[string]interface{}{
				"url":     "acc://dn.acme",
				"network": "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.queryChain(tt.args)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			}
		})
	}
}

// Test Query Data tool
func TestQueryData(t *testing.T) {
	server := NewServer()

	tests := []struct {
		name      string
		args      map[string]interface{}
		expectErr bool
	}{
		{
			name: "missing url",
			args: map[string]interface{}{
				"network": "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "valid parameters",
			args: map[string]interface{}{
				"url":     "acc://test.acme/data",
				"network": "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.queryData(tt.args)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			}
		})
	}
}

// Test Faucet tool
func TestFaucet(t *testing.T) {
	server := NewServer()

	tests := []struct {
		name      string
		args      map[string]interface{}
		expectErr bool
	}{
		{
			name: "missing url",
			args: map[string]interface{}{
				"network": "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "valid parameters",
			args: map[string]interface{}{
				"url":     "acc://lite-account",
				"network": "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.faucet(tt.args)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			}
		})
	}
}

// Test Query Directory tool
func TestQueryDirectory(t *testing.T) {
	server := NewServer()

	tests := []struct {
		name      string
		args      map[string]interface{}
		expectErr bool
	}{
		{
			name: "missing url",
			args: map[string]interface{}{
				"network": "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "valid parameters",
			args: map[string]interface{}{
				"url":     "acc://dn.acme",
				"network": "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
		{
			name: "with pagination",
			args: map[string]interface{}{
				"url":     "acc://dn.acme",
				"start":   float64(0),
				"count":   float64(10),
				"network": "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.queryDirectory(tt.args)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			}
		})
	}
}

// Test Query Pending tool
func TestQueryPending(t *testing.T) {
	server := NewServer()

	tests := []struct {
		name      string
		args      map[string]interface{}
		expectErr bool
	}{
		{
			name: "missing url",
			args: map[string]interface{}{
				"network": "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "valid parameters",
			args: map[string]interface{}{
				"url":     "acc://test.acme",
				"network": "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.queryPending(tt.args)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			}
		})
	}
}

// Test Query Minor Block tool
func TestQueryMinorBlock(t *testing.T) {
	server := NewServer()

	tests := []struct {
		name      string
		args      map[string]interface{}
		expectErr bool
	}{
		{
			name: "missing partition",
			args: map[string]interface{}{
				"network": "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "valid parameters",
			args: map[string]interface{}{
				"partition": "Directory",
				"index":     float64(0),
				"network":   "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.queryMinorBlock(tt.args)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			}
		})
	}
}

// Test Query Major Block tool
func TestQueryMajorBlock(t *testing.T) {
	server := NewServer()

	tests := []struct {
		name      string
		args      map[string]interface{}
		expectErr bool
	}{
		{
			name: "missing partition",
			args: map[string]interface{}{
				"network": "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "valid parameters",
			args: map[string]interface{}{
				"partition": "Directory",
				"index":     float64(0),
				"network":   "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.queryMajorBlock(tt.args)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			}
		})
	}
}

// Test Node Info tool
func TestNodeInfo(t *testing.T) {
	server := NewServer()

	tests := []struct {
		name      string
		args      map[string]interface{}
		expectErr bool
	}{
		{
			name:      "no parameters",
			args:      map[string]interface{}{},
			expectErr: false,
		},
		{
			name: "with network",
			args: map[string]interface{}{
				"network": "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.nodeInfo(tt.args)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			}
		})
	}
}

// Test Network Status tool
func TestNetworkStatus(t *testing.T) {
	server := NewServer()

	tests := []struct {
		name      string
		args      map[string]interface{}
		expectErr bool
	}{
		{
			name:      "no parameters",
			args:      map[string]interface{}{},
			expectErr: false,
		},
		{
			name: "with network",
			args: map[string]interface{}{
				"network": "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.networkStatus(tt.args)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			}
		})
	}
}

// Test Consensus Status tool
func TestConsensusStatus(t *testing.T) {
	server := NewServer()

	tests := []struct {
		name      string
		args      map[string]interface{}
		expectErr bool
	}{
		{
			name:      "no parameters",
			args:      map[string]interface{}{},
			expectErr: false,
		},
		{
			name: "with partition",
			args: map[string]interface{}{
				"partition": "Directory",
				"network":   "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.consensusStatus(tt.args)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			}
		})
	}
}

// Test Metrics tool
func TestMetrics(t *testing.T) {
	server := NewServer()

	tests := []struct {
		name      string
		args      map[string]interface{}
		expectErr bool
	}{
		{
			name:      "no parameters",
			args:      map[string]interface{}{},
			expectErr: false,
		},
		{
			name: "with metric",
			args: map[string]interface{}{
				"metric":  "tps",
				"network": "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.metrics(tt.args)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			}
		})
	}
}

// Test Search Public Key tool
func TestSearchPublicKey(t *testing.T) {
	server := NewServer()

	tests := []struct {
		name      string
		args      map[string]interface{}
		expectErr bool
	}{
		{
			name: "missing public_key",
			args: map[string]interface{}{
				"network": "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "valid parameters",
			args: map[string]interface{}{
				"public_key": "0123456789abcdef",
				"network":    "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.searchPublicKey(tt.args)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			}
		})
	}
}

// Test Search Public Key Hash tool
func TestSearchPublicKeyHash(t *testing.T) {
	server := NewServer()

	tests := []struct {
		name      string
		args      map[string]interface{}
		expectErr bool
	}{
		{
			name: "missing public_key_hash",
			args: map[string]interface{}{
				"network": "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "valid parameters",
			args: map[string]interface{}{
				"public_key_hash": "0123456789abcdef",
				"network":         "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.searchPublicKeyHash(tt.args)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			}
		})
	}
}

// Test Search Anchor tool
func TestSearchAnchor(t *testing.T) {
	server := NewServer()

	tests := []struct {
		name      string
		args      map[string]interface{}
		expectErr bool
	}{
		{
			name: "missing anchor",
			args: map[string]interface{}{
				"network": "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "valid parameters",
			args: map[string]interface{}{
				"anchor":  "0123456789abcdef",
				"network": "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.searchAnchor(tt.args)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			}
		})
	}
}

// Test Query KeyBook tool
func TestQueryKeyBook(t *testing.T) {
	server := NewServer()

	tests := []struct {
		name      string
		args      map[string]interface{}
		expectErr bool
	}{
		{
			name: "missing url",
			args: map[string]interface{}{
				"network": "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "valid parameters",
			args: map[string]interface{}{
				"url":     "acc://test.acme/book",
				"network": "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.queryKeyBook(tt.args)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			}
		})
	}
}

// Test Query KeyPage tool
func TestQueryKeyPage(t *testing.T) {
	server := NewServer()

	tests := []struct {
		name      string
		args      map[string]interface{}
		expectErr bool
	}{
		{
			name: "missing url",
			args: map[string]interface{}{
				"network": "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "valid parameters",
			args: map[string]interface{}{
				"url":     "acc://test.acme/book/1",
				"network": "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.queryKeyPage(tt.args)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			}
		})
	}
}

// Test Create KeyBook tool
func TestCreateKeyBook(t *testing.T) {
	server := NewServer()

	tests := []struct {
		name      string
		args      map[string]interface{}
		expectErr bool
	}{
		{
			name: "missing url",
			args: map[string]interface{}{
				"public_key_hash":    "0123456789abcdef",
				"signer":             "acc://test.acme/book",
				"signer_private_key": "0123456789abcdef",
				"network":            "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "missing public_key_hash",
			args: map[string]interface{}{
				"url":                "acc://test.acme/admin-book",
				"signer":             "acc://test.acme/book",
				"signer_private_key": "0123456789abcdef",
				"network":            "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "valid parameters",
			args: map[string]interface{}{
				"url":                "acc://test.acme/admin-book",
				"public_key_hash":    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"signer":             "acc://test.acme/book",
				"signer_private_key": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"network":            "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.createKeyBook(tt.args)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			}
		})
	}
}

// Test Update Account Auth tool
func TestUpdateAccountAuth(t *testing.T) {
	server := NewServer()

	tests := []struct {
		name      string
		args      map[string]interface{}
		expectErr bool
	}{
		{
			name: "missing account_url",
			args: map[string]interface{}{
				"operations": []interface{}{
					map[string]interface{}{
						"type":      "add",
						"authority": "acc://test.acme/book",
					},
				},
				"signer":             "acc://test.acme/book",
				"signer_private_key": "0123456789abcdef",
				"network":            "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "missing operations",
			args: map[string]interface{}{
				"account_url":        "acc://test.acme/tokens",
				"signer":             "acc://test.acme/book",
				"signer_private_key": "0123456789abcdef",
				"network":            "http://127.0.0.1:26660/v3",
			},
			expectErr: true,
		},
		{
			name: "valid add operation",
			args: map[string]interface{}{
				"account_url": "acc://test.acme/tokens",
				"operations": []interface{}{
					map[string]interface{}{
						"type":      "add",
						"authority": "acc://test.acme/admin-book",
					},
				},
				"signer":             "acc://test.acme/book",
				"signer_private_key": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"network":            "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
		{
			name: "valid multiple operations",
			args: map[string]interface{}{
				"account_url": "acc://test.acme/tokens",
				"operations": []interface{}{
					map[string]interface{}{
						"type":      "add",
						"authority": "acc://test.acme/admin-book",
					},
					map[string]interface{}{
						"type":      "disable",
						"authority": "acc://test.acme/old-book",
					},
				},
				"signer":             "acc://test.acme/book",
				"signer_private_key": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"network":            "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.updateAccountAuth(tt.args)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			}
		})
	}
}

// Test Create Lite Account tool
func TestCreateLiteAccount(t *testing.T) {
	server := NewServer()

	tests := []struct {
		name      string
		args      map[string]interface{}
		expectErr bool
	}{
		{
			name: "valid with public key",
			args: map[string]interface{}{
				"public_key": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"network":    "http://127.0.0.1:26660/v3",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.createLiteAccount(tt.args)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			}
		})
	}
}
