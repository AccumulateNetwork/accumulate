package client

import (
	"context"
	"testing"
)

func TestCreateTokenValidation(t *testing.T) {
	client, err := NewClient(devnetURL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	tests := []struct {
		name        string
		tokenURL    string
		signerURL   string
		privateKey  string
		symbol      string
		precision   uint64
		properties  map[string]interface{}
		expectErr   bool
		errContains string
	}{
		{
			name:        "invalid private key",
			tokenURL:    "acc://test.acme/mytoken",
			signerURL:   "acc://test.acme/book",
			privateKey:  "invalid",
			symbol:      "MTK",
			precision:   8,
			expectErr:   true,
			errContains: "invalid private key",
		},
		{
			name:        "wrong private key length",
			tokenURL:    "acc://test.acme/mytoken",
			signerURL:   "acc://test.acme/book",
			privateKey:  "0123456789abcdef",
			symbol:      "MTK",
			precision:   8,
			expectErr:   true,
			errContains: "invalid private key length",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.CreateToken(ctx, tt.tokenURL, tt.signerURL, tt.privateKey, tt.symbol, tt.precision, tt.properties)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing '%s', got: %v", tt.errContains, err)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestIssueTokensValidation(t *testing.T) {
	client, err := NewClient(devnetURL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	tests := []struct {
		name         string
		tokenURL     string
		recipientURL string
		signerURL    string
		privateKey   string
		amount       int64
		expectErr    bool
		errContains  string
	}{
		{
			name:         "invalid private key",
			tokenURL:     "acc://test.acme/mytoken",
			recipientURL: "acc://recipient.acme/tokens",
			signerURL:    "acc://test.acme/book",
			privateKey:   "invalid",
			amount:       1000000,
			expectErr:    true,
			errContains:  "invalid private key",
		},
		{
			name:         "wrong private key length",
			tokenURL:     "acc://test.acme/mytoken",
			recipientURL: "acc://recipient.acme/tokens",
			signerURL:    "acc://test.acme/book",
			privateKey:   "0123456789abcdef",
			amount:       1000000,
			expectErr:    true,
			errContains:  "invalid private key length",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.IssueTokens(ctx, tt.tokenURL, tt.recipientURL, tt.signerURL, tt.privateKey, tt.amount)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing '%s', got: %v", tt.errContains, err)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestBurnTokensValidation(t *testing.T) {
	client, err := NewClient(devnetURL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	tests := []struct {
		name        string
		accountURL  string
		signerURL   string
		privateKey  string
		amount      int64
		expectErr   bool
		errContains string
	}{
		{
			name:        "invalid private key",
			accountURL:  "acc://test.acme/tokens",
			signerURL:   "acc://test.acme/book",
			privateKey:  "invalid",
			amount:      1000000,
			expectErr:   true,
			errContains: "invalid private key",
		},
		{
			name:        "wrong private key length",
			accountURL:  "acc://test.acme/tokens",
			signerURL:   "acc://test.acme/book",
			privateKey:  "0123456789abcdef",
			amount:      1000000,
			expectErr:   true,
			errContains: "invalid private key length",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.BurnTokens(ctx, tt.accountURL, tt.signerURL, tt.privateKey, tt.amount)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing '%s', got: %v", tt.errContains, err)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// Test hex prefix handling across all token functions
func TestTokensHexPrefixHandling(t *testing.T) {
	client, err := NewClient(devnetURL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Use a valid private key with 0x prefix
	privateKeyWith0x := "0x" + testPrivateKey

	t.Run("CreateToken with 0x prefix", func(t *testing.T) {
		_, err := client.CreateToken(ctx, "acc://test.acme/token", "acc://test.acme/book", privateKeyWith0x, "MTK", 8, nil)
		// Should pass private key validation (will fail later at network level)
		if err != nil && contains(err.Error(), "invalid private key") && !contains(err.Error(), "invalid private key length") {
			t.Error("0x prefix should be handled correctly for private key")
		}
	})

	t.Run("IssueTokens with 0x prefix", func(t *testing.T) {
		_, err := client.IssueTokens(ctx, "acc://test.acme/token", "acc://recipient.acme/tokens", "acc://test.acme/book", privateKeyWith0x, 1000000)
		// Should pass private key validation (will fail later at network level)
		if err != nil && contains(err.Error(), "invalid private key") && !contains(err.Error(), "invalid private key length") {
			t.Error("0x prefix should be handled correctly for private key")
		}
	})

	t.Run("BurnTokens with 0x prefix", func(t *testing.T) {
		_, err := client.BurnTokens(ctx, "acc://test.acme/tokens", "acc://test.acme/book", privateKeyWith0x, 1000000)
		// Should pass private key validation (will fail later at network level)
		if err != nil && contains(err.Error(), "invalid private key") && !contains(err.Error(), "invalid private key length") {
			t.Error("0x prefix should be handled correctly for private key")
		}
	})
}
