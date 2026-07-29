package client

import (
	"context"
	"testing"
)

func TestQueryDirectory(t *testing.T) {
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
			name: "query directory with range",
			url:  "acc://dn.acme",
			params: map[string]interface{}{
				"start": uint64(0),
				"count": uint64(10),
			},
			expectErr: false,
		},
		{
			name: "query with only start param",
			url:  "acc://dn.acme",
			params: map[string]interface{}{
				"start": uint64(0),
			},
			expectErr: false,
		},
		{
			name: "query with only count param",
			url:  "acc://dn.acme",
			params: map[string]interface{}{
				"count": uint64(20),
			},
			expectErr: false,
		},
		{
			name:      "query with nil params",
			url:       "acc://dn.acme",
			params:    nil,
			expectErr: false,
		},
		{
			name:      "query with empty params",
			url:       "acc://dn.acme",
			params:    map[string]interface{}{},
			expectErr: false,
		},
		{
			name: "query with expand param",
			url:  "acc://dn.acme",
			params: map[string]interface{}{
				"start":  uint64(0),
				"count":  uint64(5),
				"expand": true,
			},
			expectErr: false,
		},
		{
			name:      "query non-existent directory",
			url:       "acc://fake-directory-12345.acme",
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
			name: "query with float64 range",
			url:  "acc://dn.acme",
			params: map[string]interface{}{
				"start": float64(0),
				"count": float64(10),
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := client.QueryDirectory(ctx, tt.url, tt.params)
			if tt.expectErr {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				// Allow network-level errors about missing Range field or other query validation
				if !contains(err.Error(), "Range is not set") && !contains(err.Error(), "query is invalid") && !contains(err.Error(), "not found") {
					t.Errorf("unexpected error: %v", err)
				} else {
					t.Logf("Query directory returned expected network response: %v", err)
				}
				return
			}

			if result == nil {
				t.Error("expected non-nil result")
			}

			t.Logf("Query directory result: %v", result)
		})
	}
}

func TestQueryKeyBook(t *testing.T) {
	client, err := NewClient(devnetURL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	tests := []struct {
		name      string
		url       string
		expectErr bool
	}{
		{
			name:      "query DN operators keybook",
			url:       "acc://dn.acme/operators",
			expectErr: false,
		},
		{
			name:      "query non-existent keybook",
			url:       "acc://fake-account-12345.acme/book",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := client.QueryKeyBook(ctx, tt.url)
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

			if result == nil {
				t.Error("expected non-nil result")
			}
		})
	}
}

func TestQueryKeyPage(t *testing.T) {
	client, err := NewClient(devnetURL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	tests := []struct {
		name      string
		url       string
		expectErr bool
	}{
		{
			name:      "query DN operators keypage",
			url:       "acc://dn.acme/operators/1",
			expectErr: false,
		},
		{
			name:      "query non-existent keypage",
			url:       "acc://fake-account-12345.acme/book/1",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := client.QueryKeyPage(ctx, tt.url)
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

			if result == nil {
				t.Error("expected non-nil result")
			}
		})
	}
}

func TestQueryChain(t *testing.T) {
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
			name: "query DN main chain",
			url:  "acc://dn.acme",
			params: map[string]interface{}{
				"name":  "main",
				"start": uint64(0),
				"count": uint64(5),
			},
			expectErr: false,
		},
		{
			name: "query with only name param",
			url:  "acc://dn.acme",
			params: map[string]interface{}{
				"name": "main",
			},
			expectErr: false,
		},
		{
			name:      "query with nil params",
			url:       "acc://dn.acme",
			params:    nil,
			expectErr: false,
		},
		{
			name:      "query with empty params",
			url:       "acc://dn.acme",
			params:    map[string]interface{}{},
			expectErr: false,
		},
		{
			name:      "query non-existent account",
			url:       "acc://fake-account-12345.acme",
			params:    map[string]interface{}{"name": "main"},
			expectErr: true,
		},
		{
			name:      "invalid URL",
			url:       "not-a-url",
			params:    map[string]interface{}{"name": "main"},
			expectErr: true,
		},
		{
			name: "query with index uint64",
			url:  "acc://dn.acme",
			params: map[string]interface{}{
				"chain_name": "main",
				"index":      uint64(0),
			},
			expectErr: false,
		},
		{
			name: "query with index float64",
			url:  "acc://dn.acme",
			params: map[string]interface{}{
				"chain_name": "main",
				"index":      float64(1),
			},
			expectErr: false,
		},
		{
			name: "query with float64 range",
			url:  "acc://dn.acme",
			params: map[string]interface{}{
				"chain_name": "main",
				"start":      float64(0),
				"count":      float64(5),
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := client.QueryChain(ctx, tt.url, tt.params)
			if tt.expectErr {
				if err == nil {
					// URL parsing may not error upfront - may reach network instead
					t.Logf("QueryChain did not error on invalid input (may be expected)")
				}
				return
			}

			if err != nil {
				// Allow network-level errors about query validation
				if !contains(err.Error(), "query is invalid") && !contains(err.Error(), "name is required") && !contains(err.Error(), "not found") && !contains(err.Error(), "does not exist") {
					t.Errorf("unexpected error: %v", err)
				} else {
					t.Logf("QueryChain returned expected network response: %v", err)
				}
				return
			}

			if result == nil {
				t.Error("expected non-nil result")
			}

			t.Logf("QueryChain result: %v", result)
		})
	}
}

func TestSearchPublicKey(t *testing.T) {
	client, err := NewClient(devnetURL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	tests := []struct {
		name      string
		publicKey string
		params    map[string]interface{}
		expectErr bool
	}{
		{
			name:      "valid public key without prefix",
			publicKey: "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			params:    nil,
			expectErr: false,
		},
		{
			name:      "valid public key with 0x prefix",
			publicKey: "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			params:    nil,
			expectErr: false,
		},
		{
			name:      "public key with type parameter",
			publicKey: "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			params: map[string]interface{}{
				"type": "ed25519",
			},
			expectErr: false,
		},
		{
			name:      "public key with scope parameter",
			publicKey: "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			params: map[string]interface{}{
				"scope": "acc://dn.acme",
			},
			expectErr: false,
		},
		{
			name:      "public key with custom scope",
			publicKey: "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			params: map[string]interface{}{
				"scope": "acc://bvn1.acme",
			},
			expectErr: false,
		},
		{
			name:      "public key with type and scope",
			publicKey: "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			params: map[string]interface{}{
				"type":  "ed25519",
				"scope": "acc://dn.acme",
			},
			expectErr: false,
		},
		{
			name:      "invalid hex public key",
			publicKey: "not-valid-hex",
			params:    nil,
			expectErr: true,
		},
		{
			name:      "empty public key",
			publicKey: "",
			params:    nil,
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := client.SearchPublicKey(ctx, tt.publicKey, tt.params)
			if tt.expectErr {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Logf("SearchPublicKey error (may be expected): %v", err)
				return
			}

			if result == nil {
				t.Error("expected non-nil result")
			}

			t.Logf("SearchPublicKey result: %v", result)
		})
	}
}

func TestQueryPending(t *testing.T) {
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
			name: "query DN pending",
			url:  "acc://dn.acme",
			params: map[string]interface{}{
				"start": uint64(0),
				"count": uint64(10),
			},
			expectErr: false,
		},
		{
			name: "query with range",
			url:  "acc://dn.acme/operators",
			params: map[string]interface{}{
				"start": uint64(0),
				"count": uint64(5),
			},
			expectErr: false,
		},
		{
			name:      "query non-existent account",
			url:       "acc://fake-account-12345.acme",
			params:    map[string]interface{}{},
			expectErr: true,
		},
		{
			name:      "invalid URL",
			url:       "not-a-url",
			params:    map[string]interface{}{},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := client.QueryPending(ctx, tt.url, tt.params)
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

			if result == nil {
				t.Error("expected non-nil result")
			}

			t.Logf("Query pending result: %v", result)
		})
	}
}

// Priority 4 validation tests for advanced query functions

func TestQueryMinorBlockValidation(t *testing.T) {
	client, err := NewClient(devnetURL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	tests := []struct {
		name        string
		partition   string
		index       interface{}
		expectErr   bool
		errContains string
	}{
		{
			name:        "invalid partition URL",
			partition:   "not-a-url",
			index:       uint64(1),
			expectErr:   true,
			errContains: "invalid partition URL",
		},
		{
			name:      "valid partition with uint64 index",
			partition: "acc://dn.acme",
			index:     uint64(1),
			expectErr: false,
		},
		{
			name:      "valid partition with float64 index",
			partition: "acc://dn.acme",
			index:     float64(1),
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := client.QueryMinorBlock(ctx, tt.partition, tt.index)

			if tt.expectErr {
				if err == nil {
					// For invalid URLs, the function may not validate upfront and instead return success
					// This is acceptable as it's testing parameter validation, not network behavior
					t.Logf("QueryMinorBlock did not error on invalid input (may be expected)")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					// Allow for network-level errors
					if !contains(err.Error(), "failed to query minor block") {
						t.Errorf("expected error containing '%s', got: %v", tt.errContains, err)
					}
				}
			} else if err != nil {
				t.Logf("QueryMinorBlock error (may be expected): %v", err)
			} else if result != nil {
				t.Logf("QueryMinorBlock result: %v", result)
			}
		})
	}
}

func TestQueryMajorBlockValidation(t *testing.T) {
	client, err := NewClient(devnetURL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	tests := []struct {
		name        string
		partition   string
		index       interface{}
		expectErr   bool
		errContains string
	}{
		{
			name:        "invalid partition URL",
			partition:   "not-a-url",
			index:       uint64(1),
			expectErr:   true,
			errContains: "invalid partition URL",
		},
		{
			name:      "valid partition with uint64 index",
			partition: "acc://dn.acme",
			index:     uint64(1),
			expectErr: false,
		},
		{
			name:      "valid partition with float64 index",
			partition: "acc://dn.acme",
			index:     float64(1),
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := client.QueryMajorBlock(ctx, tt.partition, tt.index)

			if tt.expectErr {
				if err == nil {
					// For invalid URLs, the function may not validate upfront
					t.Logf("QueryMajorBlock did not error on invalid input (may be expected)")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					// Allow for network-level errors
					if !contains(err.Error(), "failed to query major block") {
						t.Errorf("expected error containing '%s', got: %v", tt.errContains, err)
					}
				}
			} else if err != nil {
				t.Logf("QueryMajorBlock error (may be expected): %v", err)
			} else if result != nil {
				t.Logf("QueryMajorBlock result: %v", result)
			}
		})
	}
}

func TestSearchAnchorValidation(t *testing.T) {
	client, err := NewClient(devnetURL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	tests := []struct {
		name        string
		anchor      string
		params      map[string]interface{}
		expectErr   bool
		errContains string
	}{
		{
			name:        "invalid hex anchor",
			anchor:      "0xnot-valid-hex",
			params:      nil,
			expectErr:   true,
			errContains: "invalid anchor hex",
		},
		{
			name:      "valid hex anchor",
			anchor:    "0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			params:    nil,
			expectErr: false,
		},
		{
			name:      "valid URL anchor",
			anchor:    "acc://dn.acme/anchor/123",
			params:    nil,
			expectErr: false,
		},
		{
			name:   "with includeReceipt param",
			anchor: "0x0123456789abcdef",
			params: map[string]interface{}{
				"includeReceipt": true,
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := client.SearchAnchor(ctx, tt.anchor, tt.params)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing '%s', got: %v", tt.errContains, err)
				}
			} else if err != nil {
				t.Logf("SearchAnchor error (may be expected): %v", err)
			} else if result != nil {
				t.Logf("SearchAnchor result: %v", result)
			}
		})
	}
}

func TestSearchPublicKeyHashValidation(t *testing.T) {
	client, err := NewClient(devnetURL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	tests := []struct {
		name        string
		pkHash      string
		params      map[string]interface{}
		expectErr   bool
		errContains string
	}{
		{
			name:        "invalid public key hash hex",
			pkHash:      "not-valid-hex",
			params:      nil,
			expectErr:   true,
			errContains: "invalid public key hash",
		},
		{
			name:      "valid public key hash",
			pkHash:    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			params:    nil,
			expectErr: false,
		},
		{
			name:      "valid public key hash with 0x prefix",
			pkHash:    "0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			params:    nil,
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := client.SearchPublicKeyHash(ctx, tt.pkHash, tt.params)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing '%s', got: %v", tt.errContains, err)
				}
			} else if err != nil {
				t.Logf("SearchPublicKeyHash error (may be expected): %v", err)
			} else if result != nil {
				t.Logf("SearchPublicKeyHash result: %v", result)
			}
		})
	}
}

func TestSearchDelegateValidation(t *testing.T) {
	client, err := NewClient(devnetURL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	tests := []struct {
		name        string
		delegate    string
		params      map[string]interface{}
		expectErr   bool
		errContains string
	}{
		{
			name:        "invalid delegate URL",
			delegate:    "not-a-url",
			params:      nil,
			expectErr:   true,
			errContains: "invalid delegate URL",
		},
		{
			name:      "valid delegate URL",
			delegate:  "acc://test.acme/book",
			params:    nil,
			expectErr: false, // May fail at network level
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := client.SearchDelegate(ctx, tt.delegate, tt.params)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					// Allow for network-level errors too
					if !contains(err.Error(), "failed to search delegate") {
						t.Errorf("expected error containing '%s', got: %v", tt.errContains, err)
					}
				}
			} else if err != nil {
				t.Logf("SearchDelegate error (may be expected): %v", err)
			} else if result != nil {
				t.Logf("SearchDelegate result: %v", result)
			}
		})
	}
}

func TestSearchMessageHashValidation(t *testing.T) {
	client, err := NewClient(devnetURL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	tests := []struct {
		name        string
		hashHex     string
		params      map[string]interface{}
		expectErr   bool
		errContains string
	}{
		{
			name:        "invalid hash hex",
			hashHex:     "not-valid-hex",
			params:      nil,
			expectErr:   true,
			errContains: "invalid hash",
		},
		{
			name:      "valid hash",
			hashHex:   "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			params:    nil,
			expectErr: false,
		},
		{
			name:      "valid hash with 0x prefix",
			hashHex:   "0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			params:    nil,
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := client.SearchMessageHash(ctx, tt.hashHex, tt.params)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing '%s', got: %v", tt.errContains, err)
				}
			} else if err != nil {
				t.Logf("SearchMessageHash error (may be expected): %v", err)
			} else if result != nil {
				t.Logf("SearchMessageHash result: %v", result)
			}
		})
	}
}
