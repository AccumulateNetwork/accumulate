//go:build integration
// +build integration

package server

import (
	"context"
	"os"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/mcp/client"
)

// Integration tests that hit real networks
// Run with: go test -tags=integration -v ./...
// Set ACCUMULATE_TEST_NETWORK=devnet to test against local devnet

func getTestNetwork() string {
	if network := os.Getenv("ACCUMULATE_TEST_NETWORK"); network != "" {
		return network
	}
	return "mainnet" // Default to mainnet for read-only tests
}

func newTestState(network string) *State {
	cfg := LoadConfig()
	cfg.Network = network
	cfg.APITimeout = 30 * time.Second
	return NewState(cfg)
}

func TestIntegration_ClientQueryAccount(t *testing.T) {
	network := getTestNetwork()
	t.Logf("Testing against network: %s", network)

	c, err := client.NewClient(network)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Query a well-known account that exists on mainnet
	// acc://dn.acme is the Directory Network system account
	result, err := c.QueryAccount(ctx, "acc://dn.acme")
	if err != nil {
		t.Fatalf("Failed to query account: %v", err)
	}

	if result == nil {
		t.Fatal("QueryAccount returned nil result")
	}

	t.Logf("Successfully queried acc://dn.acme")
}

func TestIntegration_ClientQueryAccountNotFound(t *testing.T) {
	network := getTestNetwork()
	c, err := client.NewClient(network)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Query a non-existent account - should return an error
	_, err = c.QueryAccount(ctx, "acc://this-account-definitely-does-not-exist-12345.acme")
	if err == nil {
		t.Fatal("Expected error for non-existent account, got nil")
	}

	t.Logf("Correctly got error for non-existent account: %v", err)
}

func TestIntegration_WithClientRetry_Success(t *testing.T) {
	network := getTestNetwork()
	state := newTestState(network)
	helper := NewClientHelper(state)

	result, err := helper.WithClientRetry(network, nil, func(ctx context.Context, c *client.Client) (map[string]interface{}, error) {
		record, err := c.QueryAccount(ctx, "acc://dn.acme")
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"result": record}, nil
	})

	if err != nil {
		t.Fatalf("WithClientRetry failed: %v", err)
	}

	if result == nil {
		t.Fatal("WithClientRetry returned nil result")
	}

	if result["result"] == nil {
		t.Fatal("Result does not contain 'result' key")
	}

	t.Log("WithClientRetry successfully queried account with retry logic")
}

func TestIntegration_WithClientRetry_NonRetryableError(t *testing.T) {
	network := getTestNetwork()
	state := newTestState(network)
	helper := NewClientHelper(state)

	// Query non-existent account - this is a non-retryable error (not found)
	// Should fail immediately without retrying
	start := time.Now()
	_, err := helper.WithClientRetry(network, nil, func(ctx context.Context, c *client.Client) (map[string]interface{}, error) {
		_, queryErr := c.QueryAccount(ctx, "acc://nonexistent-account-xyz123.acme")
		return nil, queryErr
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Expected error for non-existent account")
	}

	// Non-retryable errors should fail quickly (< 5 seconds)
	// Retryable errors with default config would take ~3.5s (500ms + 1s + 2s backoffs)
	if elapsed > 10*time.Second {
		t.Errorf("Non-retryable error took too long (%v), suggests unnecessary retries", elapsed)
	}

	t.Logf("Non-retryable error handled correctly in %v: %v", elapsed, err)
}

func TestIntegration_WithClientRetry_Timeout(t *testing.T) {
	// This test verifies timeout handling in the retry logic
	// Note: The client has its own internal timeout (15s), so we test
	// that very short context timeouts are properly detected and retried
	network := getTestNetwork()
	cfg := LoadConfig()
	cfg.Network = network
	// Set a reasonable timeout - we'll test context cancellation behavior
	cfg.APITimeout = 100 * time.Millisecond
	state := NewState(cfg)
	helper := NewClientHelper(state)

	// Use minimal retries for this test
	retryConfig := &RetryConfig{
		MaxRetries:     1,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     50 * time.Millisecond,
		BackoffFactor:  2.0,
	}

	// Test that the retry logic properly handles context deadline exceeded
	_, err := helper.WithClientRetry(network, retryConfig, func(ctx context.Context, c *client.Client) (map[string]interface{}, error) {
		// Simulate a slow operation that would timeout
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
			// This simulates an operation that takes longer than the timeout
			return map[string]interface{}{"result": "should not reach here"}, nil
		}
	})

	if err == nil {
		t.Fatal("Expected timeout error")
	}

	t.Logf("Timeout handled correctly: %v", err)
}

func TestIntegration_ServerQueryAccount(t *testing.T) {
	network := getTestNetwork()

	// Create a real server instance
	srv := NewServer()
	srv.state.Config.Network = network
	srv.state.Config.APITimeout = 30 * time.Second

	// Call the actual queryAccount method
	result, err := srv.queryAccount(map[string]interface{}{
		"url":     "acc://dn.acme",
		"network": network,
	})

	if err != nil {
		t.Fatalf("queryAccount failed: %v", err)
	}

	if result == nil {
		t.Fatal("queryAccount returned nil")
	}

	t.Log("Server.queryAccount successfully queried real account")
}

func TestIntegration_ServerQueryChain(t *testing.T) {
	network := getTestNetwork()

	srv := NewServer()
	srv.state.Config.Network = network
	srv.state.Config.APITimeout = 30 * time.Second

	// Query the main chain of a known account
	result, err := srv.queryChain(map[string]interface{}{
		"url":     "acc://dn.acme",
		"network": network,
	})

	if err != nil {
		t.Fatalf("queryChain failed: %v", err)
	}

	if result == nil {
		t.Fatal("queryChain returned nil")
	}

	t.Log("Server.queryChain successfully queried real chain")
}

func TestIntegration_ServerQueryDirectory(t *testing.T) {
	network := getTestNetwork()

	srv := NewServer()
	srv.state.Config.Network = network
	srv.state.Config.APITimeout = 30 * time.Second

	// Query directory of a known ADI with proper range parameters
	result, err := srv.queryDirectory(map[string]interface{}{
		"url":     "acc://dn.acme",
		"network": network,
		"start":   float64(0),
		"count":   float64(10),
	})

	if err != nil {
		t.Fatalf("queryDirectory failed: %v", err)
	}

	if result == nil {
		t.Fatal("queryDirectory returned nil")
	}

	t.Log("Server.queryDirectory successfully queried real directory")
}

// Devnet-only tests (require local devnet running)

func TestIntegration_Devnet_QueryAccount(t *testing.T) {
	if os.Getenv("ACCUMULATE_TEST_NETWORK") != "devnet" {
		t.Skip("Skipping devnet test - set ACCUMULATE_TEST_NETWORK=devnet to run")
	}

	c, err := client.NewClient("devnet")
	if err != nil {
		t.Fatalf("Failed to create devnet client: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Query system account on devnet
	result, err := c.QueryAccount(ctx, "acc://dn.acme")
	if err != nil {
		t.Fatalf("Failed to query devnet account: %v", err)
	}

	if result == nil {
		t.Fatal("QueryAccount returned nil on devnet")
	}

	t.Log("Successfully queried devnet")
}

// Benchmark tests

func BenchmarkIntegration_QueryAccount(b *testing.B) {
	network := getTestNetwork()
	c, err := client.NewClient(network)
	if err != nil {
		b.Fatalf("Failed to create client: %v", err)
	}
	defer c.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := c.QueryAccount(ctx, "acc://dn.acme")
		if err != nil {
			b.Fatalf("Query failed: %v", err)
		}
	}
}

func BenchmarkIntegration_WithClientRetry(b *testing.B) {
	network := getTestNetwork()
	state := newTestState(network)
	helper := NewClientHelper(state)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := helper.WithClientRetry(network, nil, func(ctx context.Context, c *client.Client) (map[string]interface{}, error) {
			record, err := c.QueryAccount(ctx, "acc://dn.acme")
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{"result": record}, nil
		})
		if err != nil {
			b.Fatalf("WithClientRetry failed: %v", err)
		}
	}
}
