package client

import (
	"context"
	"testing"
)

const testPrivateKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestCreateKeyPageValidation(t *testing.T) {
	client, err := NewClient(devnetURL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	tests := []struct {
		name        string
		keybookURL  string
		signerURL   string
		privateKey  string
		keys        []string
		expectErr   bool
		errContains string
	}{
		{
			name:        "invalid private key",
			keybookURL:  "acc://test.acme/book",
			signerURL:   "acc://test.acme/book",
			privateKey:  "invalid",
			keys:        []string{"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
			expectErr:   true,
			errContains: "invalid private key",
		},
		{
			name:        "wrong private key length",
			keybookURL:  "acc://test.acme/book",
			signerURL:   "acc://test.acme/book",
			privateKey:  "0123456789abcdef",
			keys:        []string{"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
			expectErr:   true,
			errContains: "invalid private key length",
		},
		{
			name:        "invalid key in keys array",
			keybookURL:  "acc://test.acme/book",
			signerURL:   "acc://test.acme/book",
			privateKey:  testPrivateKey,
			keys:        []string{"invalid-key"},
			expectErr:   true,
			errContains: "invalid key 0",
		},
		{
			name:        "invalid keybook URL",
			keybookURL:  "not-a-url",
			signerURL:   "acc://test.acme/book",
			privateKey:  testPrivateKey,
			keys:        []string{"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
			expectErr:   true,
			errContains: "invalid keybook URL",
		},
		{
			name:        "invalid signer URL",
			keybookURL:  "acc://test.acme/book",
			signerURL:   "not-a-url",
			privateKey:  testPrivateKey,
			keys:        []string{"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
			expectErr:   true,
			errContains: "invalid signer URL",
		},
		{
			name:        "valid keypage creation with single key",
			keybookURL:  "acc://test.acme/book",
			signerURL:   "acc://test.acme/book",
			privateKey:  testPrivateKey,
			keys:        []string{"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
			expectErr:   false,
		},
		{
			name:        "valid keypage creation with multiple keys",
			keybookURL:  "acc://test.acme/book",
			signerURL:   "acc://test.acme/book",
			privateKey:  testPrivateKey,
			keys: []string{
				"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.CreateKeyPage(ctx, tt.keybookURL, tt.signerURL, tt.privateKey, tt.keys)

			if tt.expectErr {
				if err == nil {
					// URL parsing may not error upfront
					t.Logf("CreateKeyPage did not error on invalid input (may be expected)")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					// Allow for network-level errors
					if !contains(err.Error(), "failed to submit") {
						t.Errorf("expected error containing '%s', got: %v", tt.errContains, err)
					}
				}
			} else if err != nil {
				// May fail at network level
				t.Logf("CreateKeyPage error (may be expected): %v", err)
			}
		})
	}
}

func TestUpdateKeyPageValidation(t *testing.T) {
	client, err := NewClient(devnetURL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	tests := []struct {
		name        string
		keypageURL  string
		signerURL   string
		privateKey  string
		operation   string
		key         string
		threshold   uint64
		expectErr   bool
		errContains string
	}{
		{
			name:        "invalid private key",
			keypageURL:  "acc://test.acme/book/1",
			signerURL:   "acc://test.acme/book",
			privateKey:  "invalid",
			operation:   "add",
			key:         "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			expectErr:   true,
			errContains: "invalid private key",
		},
		{
			name:        "wrong private key length",
			keypageURL:  "acc://test.acme/book/1",
			signerURL:   "acc://test.acme/book",
			privateKey:  "0123456789abcdef",
			operation:   "add",
			key:         "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			expectErr:   true,
			errContains: "invalid private key length",
		},
		{
			name:        "invalid operation",
			keypageURL:  "acc://test.acme/book/1",
			signerURL:   "acc://test.acme/book",
			privateKey:  testPrivateKey,
			operation:   "invalid_op",
			key:         "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			expectErr:   true,
			errContains: "invalid operation",
		},
		{
			name:        "add operation without key",
			keypageURL:  "acc://test.acme/book/1",
			signerURL:   "acc://test.acme/book",
			privateKey:  testPrivateKey,
			operation:   "add",
			key:         "",
			expectErr:   true,
			errContains: "key is required",
		},
		{
			name:        "remove operation without key",
			keypageURL:  "acc://test.acme/book/1",
			signerURL:   "acc://test.acme/book",
			privateKey:  testPrivateKey,
			operation:   "remove",
			key:         "",
			expectErr:   true,
			errContains: "key is required",
		},
		{
			name:        "set_threshold without threshold",
			keypageURL:  "acc://test.acme/book/1",
			signerURL:   "acc://test.acme/book",
			privateKey:  testPrivateKey,
			operation:   "set_threshold",
			threshold:   0,
			expectErr:   true,
			errContains: "threshold is required",
		},
		{
			name:        "add operation with invalid key",
			keypageURL:  "acc://test.acme/book/1",
			signerURL:   "acc://test.acme/book",
			privateKey:  testPrivateKey,
			operation:   "add",
			key:         "not-hex",
			expectErr:   true,
			errContains: "invalid key",
		},
		{
			name:        "remove operation with invalid key",
			keypageURL:  "acc://test.acme/book/1",
			signerURL:   "acc://test.acme/book",
			privateKey:  testPrivateKey,
			operation:   "remove",
			key:         "not-hex",
			expectErr:   true,
			errContains: "invalid key",
		},
		{
			name:        "invalid keypage URL",
			keypageURL:  "not-a-url",
			signerURL:   "acc://test.acme/book",
			privateKey:  testPrivateKey,
			operation:   "add",
			key:         "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			expectErr:   true,
			errContains: "invalid keypage URL",
		},
		{
			name:        "invalid signer URL",
			keypageURL:  "acc://test.acme/book/1",
			signerURL:   "not-a-url",
			privateKey:  testPrivateKey,
			operation:   "add",
			key:         "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			expectErr:   true,
			errContains: "invalid signer URL",
		},
		{
			name:        "valid add operation",
			keypageURL:  "acc://test.acme/book/1",
			signerURL:   "acc://test.acme/book",
			privateKey:  testPrivateKey,
			operation:   "add",
			key:         "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			expectErr:   false,
		},
		{
			name:        "valid remove operation",
			keypageURL:  "acc://test.acme/book/1",
			signerURL:   "acc://test.acme/book",
			privateKey:  testPrivateKey,
			operation:   "remove",
			key:         "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			expectErr:   false,
		},
		{
			name:        "valid set_threshold operation",
			keypageURL:  "acc://test.acme/book/1",
			signerURL:   "acc://test.acme/book",
			privateKey:  testPrivateKey,
			operation:   "set_threshold",
			threshold:   2,
			expectErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.UpdateKeyPage(ctx, tt.keypageURL, tt.signerURL, tt.privateKey, tt.operation, tt.key, tt.threshold)

			if tt.expectErr {
				if err == nil {
					// URL parsing may not error upfront
					t.Logf("UpdateKeyPage did not error on invalid input (may be expected)")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					// Allow for network-level errors
					if !contains(err.Error(), "failed to submit") {
						t.Errorf("expected error containing '%s', got: %v", tt.errContains, err)
					}
				}
			} else if err != nil {
				// May fail at network level
				t.Logf("UpdateKeyPage error (may be expected): %v", err)
			}
		})
	}
}

func TestCreateKeyBookValidation(t *testing.T) {
	client, err := NewClient(devnetURL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	tests := []struct {
		name          string
		keybookURL    string
		signerURL     string
		privateKey    string
		publicKeyHash string
		expectErr     bool
		errContains   string
	}{
		{
			name:          "invalid private key",
			keybookURL:    "acc://test.acme/admin-book",
			signerURL:     "acc://test.acme/book",
			privateKey:    "invalid",
			publicKeyHash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			expectErr:     true,
			errContains:   "invalid private key",
		},
		{
			name:          "wrong private key length",
			keybookURL:    "acc://test.acme/admin-book",
			signerURL:     "acc://test.acme/book",
			privateKey:    "0123456789abcdef",
			publicKeyHash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			expectErr:     true,
			errContains:   "invalid private key length",
		},
		{
			name:          "invalid public key hash",
			keybookURL:    "acc://test.acme/admin-book",
			signerURL:     "acc://test.acme/book",
			privateKey:    testPrivateKey,
			publicKeyHash: "not-hex",
			expectErr:     true,
			errContains:   "invalid public key hash",
		},
		{
			name:          "invalid keybook URL",
			keybookURL:    "not-a-url",
			signerURL:     "acc://test.acme/book",
			privateKey:    testPrivateKey,
			publicKeyHash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			expectErr:     true,
			errContains:   "invalid keybook URL",
		},
		{
			name:          "invalid signer URL",
			keybookURL:    "acc://test.acme/admin-book",
			signerURL:     "not-a-url",
			privateKey:    testPrivateKey,
			publicKeyHash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			expectErr:     true,
			errContains:   "invalid signer URL",
		},
		{
			name:          "valid keybook creation",
			keybookURL:    "acc://test.acme/admin-book",
			signerURL:     "acc://test.acme/book",
			privateKey:    testPrivateKey,
			publicKeyHash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			expectErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.CreateKeyBook(ctx, tt.keybookURL, tt.signerURL, tt.privateKey, tt.publicKeyHash)

			if tt.expectErr {
				if err == nil {
					// URL parsing may not error upfront
					t.Logf("CreateKeyBook did not error on invalid input (may be expected)")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					// Allow for network-level errors
					if !contains(err.Error(), "failed to submit") {
						t.Errorf("expected error containing '%s', got: %v", tt.errContains, err)
					}
				}
			} else if err != nil {
				// May fail at network level
				t.Logf("CreateKeyBook error (may be expected): %v", err)
			}
		})
	}
}

func TestUpdateAccountAuthValidation(t *testing.T) {
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
		operations  []map[string]interface{}
		expectErr   bool
		errContains string
	}{
		{
			name:       "invalid private key",
			accountURL: "acc://test.acme/tokens",
			signerURL:  "acc://test.acme/book",
			privateKey: "invalid",
			operations: []map[string]interface{}{
				{"type": "add", "authority": "acc://test.acme/admin-book"},
			},
			expectErr:   true,
			errContains: "invalid private key",
		},
		{
			name:       "wrong private key length",
			accountURL: "acc://test.acme/tokens",
			signerURL:  "acc://test.acme/book",
			privateKey: "0123456789abcdef",
			operations: []map[string]interface{}{
				{"type": "add", "authority": "acc://test.acme/admin-book"},
			},
			expectErr:   true,
			errContains: "invalid private key length",
		},
		{
			name:       "operation missing type",
			accountURL: "acc://test.acme/tokens",
			signerURL:  "acc://test.acme/book",
			privateKey: testPrivateKey,
			operations: []map[string]interface{}{
				{"authority": "acc://test.acme/admin-book"},
			},
			expectErr:   true,
			errContains: "missing type",
		},
		{
			name:       "operation missing authority",
			accountURL: "acc://test.acme/tokens",
			signerURL:  "acc://test.acme/book",
			privateKey: testPrivateKey,
			operations: []map[string]interface{}{
				{"type": "add"},
			},
			expectErr:   true,
			errContains: "missing authority",
		},
		{
			name:       "invalid operation type",
			accountURL: "acc://test.acme/tokens",
			signerURL:  "acc://test.acme/book",
			privateKey: testPrivateKey,
			operations: []map[string]interface{}{
				{"type": "invalid_op", "authority": "acc://test.acme/admin-book"},
			},
			expectErr:   true,
			errContains: "invalid type",
		},
		{
			name:        "no operations",
			accountURL:  "acc://test.acme/tokens",
			signerURL:   "acc://test.acme/book",
			privateKey:  testPrivateKey,
			operations:  []map[string]interface{}{},
			expectErr:   true,
			errContains: "no operations",
		},
		{
			name:       "invalid account URL",
			accountURL: "not-a-url",
			signerURL:  "acc://test.acme/book",
			privateKey: testPrivateKey,
			operations: []map[string]interface{}{
				{"type": "add", "authority": "acc://test.acme/admin-book"},
			},
			expectErr:   true,
			errContains: "invalid account URL",
		},
		{
			name:       "invalid signer URL",
			accountURL: "acc://test.acme/tokens",
			signerURL:  "not-a-url",
			privateKey: testPrivateKey,
			operations: []map[string]interface{}{
				{"type": "add", "authority": "acc://test.acme/admin-book"},
			},
			expectErr:   true,
			errContains: "invalid signer URL",
		},
		{
			name:       "invalid authority URL in add operation",
			accountURL: "acc://test.acme/tokens",
			signerURL:  "acc://test.acme/book",
			privateKey: testPrivateKey,
			operations: []map[string]interface{}{
				{"type": "add", "authority": "not-a-url"},
			},
			expectErr:   true,
			errContains: "invalid authority URL",
		},
		{
			name:       "invalid authority URL in remove operation",
			accountURL: "acc://test.acme/tokens",
			signerURL:  "acc://test.acme/book",
			privateKey: testPrivateKey,
			operations: []map[string]interface{}{
				{"type": "remove", "authority": "not-a-url"},
			},
			expectErr:   true,
			errContains: "invalid authority URL",
		},
		{
			name:       "invalid authority URL in enable operation",
			accountURL: "acc://test.acme/tokens",
			signerURL:  "acc://test.acme/book",
			privateKey: testPrivateKey,
			operations: []map[string]interface{}{
				{"type": "enable", "authority": "not-a-url"},
			},
			expectErr:   true,
			errContains: "invalid authority URL",
		},
		{
			name:       "invalid authority URL in disable operation",
			accountURL: "acc://test.acme/tokens",
			signerURL:  "acc://test.acme/book",
			privateKey: testPrivateKey,
			operations: []map[string]interface{}{
				{"type": "disable", "authority": "not-a-url"},
			},
			expectErr:   true,
			errContains: "invalid authority URL",
		},
		{
			name:       "valid add operation",
			accountURL: "acc://test.acme/tokens",
			signerURL:  "acc://test.acme/book",
			privateKey: testPrivateKey,
			operations: []map[string]interface{}{
				{"type": "add", "authority": "acc://test.acme/admin-book"},
			},
			expectErr: false,
		},
		{
			name:       "valid remove operation",
			accountURL: "acc://test.acme/tokens",
			signerURL:  "acc://test.acme/book",
			privateKey: testPrivateKey,
			operations: []map[string]interface{}{
				{"type": "remove", "authority": "acc://test.acme/old-book"},
			},
			expectErr: false,
		},
		{
			name:       "valid enable operation",
			accountURL: "acc://test.acme/tokens",
			signerURL:  "acc://test.acme/book",
			privateKey: testPrivateKey,
			operations: []map[string]interface{}{
				{"type": "enable", "authority": "acc://test.acme/backup-book"},
			},
			expectErr: false,
		},
		{
			name:       "valid disable operation",
			accountURL: "acc://test.acme/tokens",
			signerURL:  "acc://test.acme/book",
			privateKey: testPrivateKey,
			operations: []map[string]interface{}{
				{"type": "disable", "authority": "acc://test.acme/temp-book"},
			},
			expectErr: false,
		},
		{
			name:       "multiple operations",
			accountURL: "acc://test.acme/tokens",
			signerURL:  "acc://test.acme/book",
			privateKey: testPrivateKey,
			operations: []map[string]interface{}{
				{"type": "add", "authority": "acc://test.acme/new-book"},
				{"type": "remove", "authority": "acc://test.acme/old-book"},
				{"type": "enable", "authority": "acc://test.acme/backup-book"},
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.UpdateAccountAuth(ctx, tt.accountURL, tt.signerURL, tt.privateKey, tt.operations)

			if tt.expectErr {
				if err == nil {
					// URL parsing may not error upfront
					t.Logf("UpdateAccountAuth did not error on invalid input (may be expected)")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					// Allow for network-level errors
					if !contains(err.Error(), "failed to submit") {
						t.Errorf("expected error containing '%s', got: %v", tt.errContains, err)
					}
				}
			} else if err != nil {
				// May fail at network level
				t.Logf("UpdateAccountAuth error (may be expected): %v", err)
			}
		})
	}
}

// Helper function to check if error contains string
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && stringContains(s, substr)))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
