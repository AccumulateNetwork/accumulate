package server

import (
	"strings"
	"testing"
)

// Test Submit Batch tool
func TestSubmitBatch(t *testing.T) {
	server := NewServer()

	tests := []struct {
		name      string
		args      map[string]interface{}
		expectErr bool
		errMsg    string
	}{
		{
			name: "missing transactions",
			args: map[string]interface{}{
				"private_key": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"network":     "mainnet",
			},
			expectErr: true,
			errMsg:    "missing or empty transactions array",
		},
		{
			name: "empty transactions array",
			args: map[string]interface{}{
				"transactions": []interface{}{},
				"private_key":  "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"network":      "mainnet",
			},
			expectErr: true,
			errMsg:    "missing or empty transactions array",
		},
		{
			name: "missing private_key",
			args: map[string]interface{}{
				"transactions": []interface{}{
					map[string]interface{}{
						"type": "sendTokens",
						"params": map[string]interface{}{
							"from":   "acc://sender.acme/tokens",
							"to":     "acc://recipient.acme/tokens",
							"amount": "1000000",
						},
					},
				},
				"network": "mainnet",
			},
			expectErr: true,
			errMsg:    "missing required parameter: private_key",
		},
		{
			name: "invalid private_key",
			args: map[string]interface{}{
				"transactions": []interface{}{
					map[string]interface{}{
						"type": "sendTokens",
						"params": map[string]interface{}{
							"from":   "acc://sender.acme/tokens",
							"to":     "acc://recipient.acme/tokens",
							"amount": "1000000",
						},
					},
				},
				"private_key": "invalid-key",
				"network":     "mainnet",
			},
			expectErr: true,
			errMsg:    "invalid private key",
		},
		{
			name: "transaction not an object",
			args: map[string]interface{}{
				"transactions": []interface{}{
					"not-an-object",
				},
				"private_key": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"network":     "mainnet",
			},
			expectErr: true,
			errMsg:    "transaction 0 is not a valid object",
		},
		{
			name: "transaction missing type",
			args: map[string]interface{}{
				"transactions": []interface{}{
					map[string]interface{}{
						"params": map[string]interface{}{
							"from":   "acc://sender.acme/tokens",
							"to":     "acc://recipient.acme/tokens",
							"amount": "1000000",
						},
					},
				},
				"private_key": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"network":     "mainnet",
			},
			expectErr: true,
			errMsg:    "transaction 0 missing type",
		},
		{
			name: "transaction missing params",
			args: map[string]interface{}{
				"transactions": []interface{}{
					map[string]interface{}{
						"type": "sendTokens",
					},
				},
				"private_key": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"network":     "mainnet",
			},
			expectErr: true,
			errMsg:    "transaction 0 missing params",
		},
		{
			name: "unsupported transaction type",
			args: map[string]interface{}{
				"transactions": []interface{}{
					map[string]interface{}{
						"type": "unsupportedType",
						"params": map[string]interface{}{
							"foo": "bar",
						},
					},
				},
				"private_key": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"network":     "mainnet",
			},
			expectErr: true,
			errMsg:    "unsupported transaction type",
		},
		{
			name: "sendTokens with missing from",
			args: map[string]interface{}{
				"transactions": []interface{}{
					map[string]interface{}{
						"type": "sendTokens",
						"params": map[string]interface{}{
							"to":     "acc://recipient.acme/tokens",
							"amount": "1000000",
						},
					},
				},
				"private_key": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"network":     "mainnet",
			},
			expectErr: true,
			errMsg:    "missing required parameter: from",
		},
		{
			name: "valid single transaction",
			args: map[string]interface{}{
				"transactions": []interface{}{
					map[string]interface{}{
						"type": "sendTokens",
						"params": map[string]interface{}{
							"from":   "acc://sender.acme/tokens",
							"to":     "acc://recipient.acme/tokens",
							"amount": "1000000",
						},
					},
				},
				"private_key": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"network":     "mainnet",
			},
			expectErr: false, // Will fail with network error, but validation should pass
		},
		{
			name: "valid multiple transactions",
			args: map[string]interface{}{
				"transactions": []interface{}{
					map[string]interface{}{
						"type": "sendTokens",
						"params": map[string]interface{}{
							"from":   "acc://sender.acme/tokens",
							"to":     "acc://alice.acme/tokens",
							"amount": "1000000",
						},
					},
					map[string]interface{}{
						"type": "sendTokens",
						"params": map[string]interface{}{
							"from":   "acc://sender.acme/tokens",
							"to":     "acc://bob.acme/tokens",
							"amount": "2000000",
						},
					},
					map[string]interface{}{
						"type": "writeData",
						"params": map[string]interface{}{
							"account": "acc://sender.acme/data",
							"data":    "test data",
						},
					},
				},
				"private_key": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"network":     "mainnet",
			},
			expectErr: false, // Will fail with network error, but validation should pass
		},
		{
			name: "valid with 0x prefix on private key",
			args: map[string]interface{}{
				"transactions": []interface{}{
					map[string]interface{}{
						"type": "sendTokens",
						"params": map[string]interface{}{
							"from":   "acc://sender.acme/tokens",
							"to":     "acc://recipient.acme/tokens",
							"amount": "1000000",
						},
					},
				},
				"private_key": "0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"network":     "mainnet",
			},
			expectErr: false, // Will fail with network error, but validation should pass
		},
		{
			name: "valid createDataAccount",
			args: map[string]interface{}{
				"transactions": []interface{}{
					map[string]interface{}{
						"type": "createDataAccount",
						"params": map[string]interface{}{
							"url":       "acc://myadi.acme/data",
							"principal": "acc://myadi.acme",
						},
					},
				},
				"private_key": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"network":     "mainnet",
			},
			expectErr: false,
		},
		{
			name: "valid createTokenAccount",
			args: map[string]interface{}{
				"transactions": []interface{}{
					map[string]interface{}{
						"type": "createTokenAccount",
						"params": map[string]interface{}{
							"url":       "acc://myadi.acme/tokens",
							"principal": "acc://myadi.acme",
							"token_url": "acc://ACME",
						},
					},
				},
				"private_key": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"network":     "mainnet",
			},
			expectErr: false,
		},
		{
			name: "valid createIdentity",
			args: map[string]interface{}{
				"transactions": []interface{}{
					map[string]interface{}{
						"type": "createIdentity",
						"params": map[string]interface{}{
							"url":        "acc://myadi.acme",
							"sponsor":    "acc://lite-account",
							"public_key": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
						},
					},
				},
				"private_key": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"network":     "mainnet",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := server.submitBatch(tt.args)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error message to contain %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				// For valid parameters, we expect either success or network errors
				// (not validation errors)
				if err != nil {
					// Check if it's a network error (which is OK for these tests)
					if !strings.Contains(err.Error(), "failed to create client") &&
						!strings.Contains(err.Error(), "failed to submit batch") &&
						!strings.Contains(err.Error(), "network") {
						t.Errorf("unexpected validation error: %v", err)
					}
				}
			}

			// Check result structure for successful validation
			if !tt.expectErr && result != nil {
				content, ok := result["content"].([]map[string]interface{})
				if !ok {
					t.Error("expected content to be an array")
				} else if len(content) > 0 {
					text, ok := content[0]["text"].(string)
					if !ok {
						t.Error("expected text to be a string")
					} else {
						// Verify warnings are present
						if !strings.Contains(text, "⚠️") {
							t.Error("expected warnings in response")
						}
						if !strings.Contains(text, "no fee savings") || !strings.Contains(text, "partial failures") {
							t.Error("expected warnings about no fee savings and partial failures")
						}
					}
				}
			}
		})
	}
}

// Test buildTransaction helper
func TestBuildTransaction(t *testing.T) {
	tests := []struct {
		name      string
		txnType   string
		params    map[string]interface{}
		expectErr bool
		errMsg    string
	}{
		{
			name:    "sendTokens - valid",
			txnType: "sendTokens",
			params: map[string]interface{}{
				"from":   "acc://sender.acme/tokens",
				"to":     "acc://recipient.acme/tokens",
				"amount": "1000000",
			},
			expectErr: false,
		},
		{
			name:    "sendTokens - missing from",
			txnType: "sendTokens",
			params: map[string]interface{}{
				"to":     "acc://recipient.acme/tokens",
				"amount": "1000000",
			},
			expectErr: true,
			errMsg:    "missing required parameter: from",
		},
		{
			name:    "createDataAccount - valid",
			txnType: "createDataAccount",
			params: map[string]interface{}{
				"url":       "acc://myadi.acme/data",
				"principal": "acc://myadi.acme",
			},
			expectErr: false,
		},
		{
			name:    "writeData - valid",
			txnType: "writeData",
			params: map[string]interface{}{
				"account": "acc://myadi.acme/data",
				"data":    "test data",
			},
			expectErr: false,
		},
		{
			name:    "createTokenAccount - valid",
			txnType: "createTokenAccount",
			params: map[string]interface{}{
				"url":       "acc://myadi.acme/tokens",
				"principal": "acc://myadi.acme",
				"token_url": "acc://ACME",
			},
			expectErr: false,
		},
		{
			name:    "createIdentity - valid",
			txnType: "createIdentity",
			params: map[string]interface{}{
				"url":        "acc://myadi.acme",
				"sponsor":    "acc://sponsor.acme",
				"public_key": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			},
			expectErr: false,
		},
		{
			name:    "unsupported type",
			txnType: "unsupportedType",
			params: map[string]interface{}{
				"foo": "bar",
			},
			expectErr: true,
			errMsg:    "unsupported transaction type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			txn, err := buildTransaction(tt.txnType, tt.params)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error message to contain %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if txn == nil {
					t.Error("expected non-nil transaction")
				}
			}
		})
	}
}
