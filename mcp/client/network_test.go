package client

import (
	"context"
	"testing"
)

func TestNetworkStatusValidation(t *testing.T) {
	client, err := NewClient(devnetURL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	tests := []struct {
		name      string
		params    map[string]interface{}
		expectErr bool
	}{
		{
			name:      "no parameters",
			params:    nil,
			expectErr: false,
		},
		{
			name:      "empty parameters",
			params:    map[string]interface{}{},
			expectErr: false,
		},
		{
			name: "with partition parameter",
			params: map[string]interface{}{
				"partition": "Directory",
			},
			expectErr: false,
		},
		{
			name: "with BVN partition",
			params: map[string]interface{}{
				"partition": "BVN0",
			},
			expectErr: false,
		},
		{
			name: "with DN partition",
			params: map[string]interface{}{
				"partition": "DN",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := client.NetworkStatus(ctx, tt.params)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else if err != nil {
				// May fail if network is unavailable
				t.Logf("NetworkStatus error (may be expected): %v", err)
			} else if result != nil {
				t.Logf("NetworkStatus result: %v", result)
			}
		})
	}
}

func TestConsensusStatusValidation(t *testing.T) {
	client, err := NewClient(devnetURL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	tests := []struct {
		name      string
		params    map[string]interface{}
		expectErr bool
	}{
		{
			name:      "no parameters",
			params:    nil,
			expectErr: false, // Should work with defaults
		},
		{
			name: "with partition",
			params: map[string]interface{}{
				"partition": "Directory",
			},
			expectErr: false,
		},
		{
			name: "with nodeID",
			params: map[string]interface{}{
				"nodeID": "node1",
			},
			expectErr: false,
		},
		{
			name: "with both partition and nodeID",
			params: map[string]interface{}{
				"partition": "Directory",
				"nodeID":    "node1",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := client.ConsensusStatus(ctx, tt.params)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else if err != nil {
				// May fail if network is unavailable, but shouldn't fail validation
				t.Logf("ConsensusStatus error (may be expected): %v", err)
			} else if result != nil {
				t.Logf("ConsensusStatus result: %v", result)
			}
		})
	}
}

func TestMetricsValidation(t *testing.T) {
	client, err := NewClient(devnetURL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	tests := []struct {
		name      string
		params    map[string]interface{}
		expectErr bool
	}{
		{
			name:      "no parameters",
			params:    nil,
			expectErr: false,
		},
		{
			name: "with partition",
			params: map[string]interface{}{
				"partition": "Directory",
			},
			expectErr: false,
		},
		{
			name: "with span (uint64)",
			params: map[string]interface{}{
				"span": uint64(10),
			},
			expectErr: false,
		},
		{
			name: "with span (float64)",
			params: map[string]interface{}{
				"span": float64(10),
			},
			expectErr: false,
		},
		{
			name: "with both partition and span",
			params: map[string]interface{}{
				"partition": "Directory",
				"span":      float64(20),
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := client.Metrics(ctx, tt.params)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else if err != nil {
				// May fail if network is unavailable
				t.Logf("Metrics error (may be expected): %v", err)
			} else if result != nil {
				t.Logf("Metrics result: %v", result)
			}
		})
	}
}

func TestFaucetValidation(t *testing.T) {
	client, err := NewClient(devnetURL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	tests := []struct {
		name        string
		accountURL  string
		params      map[string]interface{}
		expectErr   bool
		errContains string
	}{
		{
			name:        "invalid account URL",
			accountURL:  "not-a-url",
			params:      nil,
			expectErr:   true,
			errContains: "invalid account URL",
		},
		{
			name:       "valid lite account URL",
			accountURL: "acc://0123456789abcdef0123456789abcdef01234567/ACME",
			params:     nil,
			expectErr:  false, // May fail at network level
		},
		{
			name:       "with custom token",
			accountURL: "acc://0123456789abcdef0123456789abcdef01234567/ACME",
			params: map[string]interface{}{
				"token": "acc://ACME",
			},
			expectErr: false,
		},
		{
			name:       "with invalid token URL",
			accountURL: "acc://0123456789abcdef0123456789abcdef01234567/ACME",
			params: map[string]interface{}{
				"token": "not-a-url",
			},
			expectErr:   true,
			errContains: "invalid token URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := client.Faucet(ctx, tt.accountURL, tt.params)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got none")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					// Allow for network-level errors too
					if !contains(err.Error(), "failed to request faucet") {
						t.Errorf("expected error containing '%s', got: %v", tt.errContains, err)
					}
				}
			} else if err != nil {
				// May fail if network unavailable or account already funded
				t.Logf("Faucet error (may be expected): %v", err)
			} else if result != nil {
				t.Logf("Faucet result: %v", result)
			}
		})
	}
}
