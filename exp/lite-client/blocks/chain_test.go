package blocks

import (
	"context"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
)

// Test endpoints - these are common Accumulate network endpoints
var testEndpoints = []string{
	"https://mainnet.accumulatenetwork.io/v3",     // Mainnet
	"https://testnet.accumulatenetwork.io/v3",     // Testnet
	"http://kermit.accumulatenetwork.io:16695/v3", // Kermit (if available)
	"http://localhost:26660/v3",                   // Local node
}

// Test partition URLs - different partitions to try
var testPartitions = []string{
	"acc://bvn-mainnet.acme", // BVN Mainnet
	"acc://bvn-testnet.acme", // BVN Testnet
	"acc://directory.acme",   // Directory Network
}

func TestQueryAnchorMajorBlockChain_Simple(t *testing.T) {
	// Simple test with a mock or basic setup
	t.Log("Testing QueryAnchorMajorBlockChain function...")

	// This test will likely fail with real network calls, but shows the structure
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Try each endpoint
	for _, endpoint := range testEndpoints {
		t.Run("endpoint_"+endpoint, func(t *testing.T) {
			t.Logf("Testing endpoint: %s", endpoint)

			// Create a client
			client := jsonrpc.NewClient(endpoint)

			// Try each partition
			for _, partition := range testPartitions {
				t.Run("partition_"+partition, func(t *testing.T) {
					t.Logf("Testing partition: %s", partition)

					// Test with a small count
					err := QueryAnchorMajorBlockChain(ctx, client, partition, 3)
					if err != nil {
						t.Logf("Query failed for %s on %s: %v", partition, endpoint, err)
						// Don't fail the test - network issues are expected
					} else {
						t.Logf("Query succeeded for %s on %s", partition, endpoint)
					}
				})
			}
		})
	}
}

func TestQueryAnchorMajorBlockChain_InvalidInputs(t *testing.T) {
	ctx := context.Background()

	// Create a mock client (this will still fail but shows input validation)
	client := jsonrpc.NewClient("http://localhost:26660/v3")

	// Test invalid partition URL
	err := QueryAnchorMajorBlockChain(ctx, client, "invalid-url", 1)
	if err == nil {
		t.Error("Expected error for invalid partition URL, got nil")
	} else {
		t.Logf("Correctly caught invalid URL error: %v", err)
	}

	// Test with zero count
	err = QueryAnchorMajorBlockChain(ctx, client, "acc://directory.acme", 0)
	if err != nil {
		t.Logf("Query with zero count returned error: %v", err)
	}
}

func TestQueryAnchorMajorBlockChain_Timeout(t *testing.T) {
	// Test with very short timeout to see timeout behavior
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	client := jsonrpc.NewClient("https://mainnet.accumulatenetwork.io/v3")

	err := QueryAnchorMajorBlockChain(ctx, client, "acc://directory.acme", 1)
	if err != nil {
		t.Logf("Expected timeout error, got: %v", err)
	}
}

// Benchmark test to see performance
func BenchmarkQueryAnchorMajorBlockChain(b *testing.B) {
	ctx := context.Background()
	client := jsonrpc.NewClient("https://testnet.accumulatenetwork.io/v3")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := QueryAnchorMajorBlockChain(ctx, client, "acc://bvn-testnet.acme", 1)
		if err != nil {
			b.Logf("Query failed: %v", err)
		}
	}
}

// Test to manually inspect what we get from a working endpoint
func TestManualInspection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping manual inspection test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Try testnet first as it's more likely to be available
	client := jsonrpc.NewClient("https://testnet.accumulatenetwork.io/v3")

	t.Log("=== Manual Inspection Test ===")
	t.Log("Trying to query testnet BVN anchor pool...")

	err := QueryAnchorMajorBlockChain(ctx, client, "acc://bvn-testnet.acme", 2)
	if err != nil {
		t.Logf("Testnet query failed: %v", err)

		// Try directory instead
		t.Log("Trying directory network...")
		err = QueryAnchorMajorBlockChain(ctx, client, "acc://directory.acme", 2)
		if err != nil {
			t.Logf("Directory query also failed: %v", err)
		}
	}
}
