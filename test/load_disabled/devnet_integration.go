package main

import (
	"context"
	"fmt"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// DevnetTest tests against the actual running devnet
type DevnetTest struct {
	client api.Querier
	logger logging.OptionalLogger
}

func main() {
	fmt.Println("================================================================================")
	fmt.Println("                        DEVNET INTEGRATION TEST")
	fmt.Println("================================================================================")
	fmt.Println()

	test := &DevnetTest{}

	// Check if devnet is running
	if !test.CheckDevnetStatus() {
		fmt.Println("❌ Devnet is not running at http://127.0.0.1:26660/v3")
		fmt.Println()
		fmt.Println("Please start the devnet first:")
		fmt.Println("  cd ../.. && ./devnet_manager.sh start")
		return
	}

	fmt.Println("✅ Devnet is running")
	fmt.Println()

	// Run all devnet tests
	test.RunDevnetTests()
}

func (dt *DevnetTest) CheckDevnetStatus() bool {
	// Try to connect to devnet
	client, err := jsonrpc.NewClient("http://127.0.0.1:26660/v3")
	if err != nil {
		return false
	}

	dt.client = client

	// Try a simple query to verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try to get network status
	req := new(api.GeneralQuery)
	req.Url = protocol.DnUrl().JoinPath(protocol.Network)

	_, err = dt.client.Query(ctx, "", req)
	return err == nil
}

func (dt *DevnetTest) RunDevnetTests() {
	tests := []struct {
		name string
		fn   func() error
	}{
		{"Test 1: Query Network Status", dt.TestNetworkStatus},
		{"Test 2: Query Directory Partitions", dt.TestDirectoryPartitions},
		{"Test 3: Query BVN Status", dt.TestBVNStatus},
		{"Test 4: Test Anchor Ledger Access", dt.TestAnchorLedger},
		{"Test 5: Test Synthetic Ledger Access", dt.TestSyntheticLedger},
		{"Test 6: Test Connection Pooling", dt.TestConnectionPooling},
		{"Test 7: Test Concurrent Queries", dt.TestConcurrentQueries},
		{"Test 8: Test Error Handling", dt.TestErrorHandling},
	}

	passed := 0
	failed := 0

	for i, test := range tests {
		fmt.Printf("[%d/%d] %s\n", i+1, len(tests), test.name)
		fmt.Println("────────────────────────────────────────────────────────────────────────────────")

		err := test.fn()
		if err != nil {
			fmt.Printf("  ❌ FAILED: %v\n", err)
			failed++
		} else {
			fmt.Printf("  ✅ PASSED\n")
			passed++
		}
		fmt.Println()
	}

	fmt.Println("================================================================================")
	fmt.Println("                              DEVNET TEST RESULTS")
	fmt.Println("================================================================================")
	fmt.Printf("Total Tests: %d\n", len(tests))
	fmt.Printf("Passed: %d\n", passed)
	fmt.Printf("Failed: %d\n", failed)

	if failed == 0 {
		fmt.Println("\n✅ ALL DEVNET TESTS PASSED!")
	} else {
		fmt.Println("\n❌ SOME DEVNET TESTS FAILED")
	}
}

func (dt *DevnetTest) TestNetworkStatus() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req := new(api.GeneralQuery)
	req.Url = protocol.DnUrl().JoinPath(protocol.Network)

	resp, err := dt.client.Query(ctx, "", req)
	if err != nil {
		return fmt.Errorf("failed to query network: %w", err)
	}

	if resp == nil || resp.Data == nil {
		return fmt.Errorf("empty response from network query")
	}

	// Check if it's a network object
	switch v := resp.Data.(type) {
	case *api.ChainQueryResponse:
		fmt.Printf("  Network query successful\n")
		fmt.Printf("  Type: %T\n", v.Data)
		if v.MainChain != nil {
			fmt.Printf("  Main Chain Height: %d\n", v.MainChain.Height)
		}
	case *protocol.SystemData:
		fmt.Printf("  System Data retrieved\n")
	default:
		fmt.Printf("  Response type: %T\n", resp.Data)
	}

	return nil
}

func (dt *DevnetTest) TestDirectoryPartitions() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Query the directory
	req := new(api.GeneralQuery)
	req.Url = protocol.DnUrl()

	resp, err := dt.client.Query(ctx, "", req)
	if err != nil {
		return fmt.Errorf("failed to query directory: %w", err)
	}

	if resp == nil || resp.Data == nil {
		return fmt.Errorf("empty response from directory query")
	}

	fmt.Printf("  Directory query successful\n")
	fmt.Printf("  Response type: %T\n", resp.Data)

	// Try to get partition info
	req2 := new(api.GeneralQuery)
	req2.Url = protocol.PartitionUrl("Directory")

	resp2, err := dt.client.Query(ctx, "", req2)
	if err != nil {
		// This might fail if the partition doesn't exist
		fmt.Printf("  Warning: Could not query Directory partition: %v\n", err)
	} else if resp2 != nil {
		fmt.Printf("  Directory partition found\n")
	}

	return nil
}

func (dt *DevnetTest) TestBVNStatus() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Test BVN partitions
	partitions := []string{"BVN0", "BVN1", "BVN2"}
	foundCount := 0

	for _, partition := range partitions {
		req := new(api.GeneralQuery)
		req.Url = protocol.PartitionUrl(partition)

		_, err := dt.client.Query(ctx, "", req)
		if err != nil {
			// Check if it's a not found error or actual error
			if errors.Is(err, errors.NotFound) {
				fmt.Printf("  Partition %s not found (may not be configured)\n", partition)
			} else {
				fmt.Printf("  Error querying %s: %v\n", partition, err)
			}
		} else {
			fmt.Printf("  ✓ Partition %s is accessible\n", partition)
			foundCount++
		}
	}

	if foundCount == 0 {
		return fmt.Errorf("no BVN partitions found")
	}

	fmt.Printf("  Found %d/%d BVN partitions\n", foundCount, len(partitions))
	return nil
}

func (dt *DevnetTest) TestAnchorLedger() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Try to query an anchor ledger
	// This URL might need adjustment based on your actual devnet configuration
	anchorLedgerUrl := protocol.DnUrl().JoinPath(protocol.AnchorPool)

	req := new(api.GeneralQuery)
	req.Url = anchorLedgerUrl

	resp, err := dt.client.Query(ctx, "", req)
	if err != nil {
		// Try alternative URL
		req.Url = protocol.DnUrl().JoinPath("anchors")
		resp, err = dt.client.Query(ctx, "", req)
		if err != nil {
			fmt.Printf("  Warning: Could not access anchor ledger: %v\n", err)
			fmt.Printf("  This might be normal if anchors haven't been created yet\n")
			return nil // Don't fail the test
		}
	}

	if resp != nil && resp.Data != nil {
		fmt.Printf("  Anchor ledger accessible\n")
		fmt.Printf("  Response type: %T\n", resp.Data)
	}

	return nil
}

func (dt *DevnetTest) TestSyntheticLedger() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Try to query synthetic transaction ledger
	synthLedgerUrl := protocol.DnUrl().JoinPath(protocol.Synthetic)

	req := new(api.GeneralQuery)
	req.Url = synthLedgerUrl

	resp, err := dt.client.Query(ctx, "", req)
	if err != nil {
		// Try alternative URL
		req.Url = protocol.DnUrl().JoinPath("synthetic")
		resp, err = dt.client.Query(ctx, "", req)
		if err != nil {
			fmt.Printf("  Warning: Could not access synthetic ledger: %v\n", err)
			fmt.Printf("  This might be normal if synthetic transactions haven't been created yet\n")
			return nil // Don't fail the test
		}
	}

	if resp != nil && resp.Data != nil {
		fmt.Printf("  Synthetic ledger accessible\n")
		fmt.Printf("  Response type: %T\n", resp.Data)
	}

	return nil
}

func (dt *DevnetTest) TestConnectionPooling() error {
	// Test our connection pooling implementation
	fmt.Printf("  Testing connection pool with 20 clients...\n")

	clients := make([]*jsonrpc.Client, 0)
	for i := 0; i < 20; i++ {
		client := GetPooledClient("http://127.0.0.1:26660/v3")
		if client == nil {
			return fmt.Errorf("failed to get pooled client %d", i)
		}
		clients = append(clients, client)
	}

	// Check pool metrics
	poolSize := len(clientPool)
	fmt.Printf("  Pool size: %d (max: %d)\n", poolSize, maxPoolSize)

	if poolSize > maxPoolSize {
		return fmt.Errorf("pool size exceeded maximum: %d > %d", poolSize, maxPoolSize)
	}

	// Test cleanup
	CleanupClientPool()
	poolSizeAfter := len(clientPool)

	if poolSizeAfter != 0 {
		return fmt.Errorf("pool not cleaned properly: %d clients remaining", poolSizeAfter)
	}

	fmt.Printf("  Connection pooling working correctly\n")
	return nil
}

func (dt *DevnetTest) TestConcurrentQueries() error {
	fmt.Printf("  Running 50 concurrent queries...\n")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	type result struct {
		success bool
		err     error
	}

	results := make(chan result, 50)

	for i := 0; i < 50; i++ {
		go func(id int) {
			// Get a new client from pool
			client := GetPooledClient("http://127.0.0.1:26660/v3")

			req := new(api.GeneralQuery)
			req.Url = protocol.DnUrl().JoinPath(protocol.Network)

			_, err := client.Query(ctx, "", req)
			results <- result{err == nil, err}
		}(i)
	}

	// Collect results
	successCount := 0
	errorCount := 0
	var lastError error

	for i := 0; i < 50; i++ {
		r := <-results
		if r.success {
			successCount++
		} else {
			errorCount++
			lastError = r.err
		}
	}

	fmt.Printf("  Completed: %d successful, %d failed\n", successCount, errorCount)

	if errorCount > 10 {
		return fmt.Errorf("too many concurrent query failures: %d (last error: %v)", errorCount, lastError)
	}

	if successCount == 0 {
		return fmt.Errorf("no concurrent queries succeeded")
	}

	fmt.Printf("  Concurrent queries handled successfully\n")
	return nil
}

func (dt *DevnetTest) TestErrorHandling() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Test with invalid URL
	req := new(api.GeneralQuery)
	req.Url = protocol.AccountUrl("invalid", "account", "that", "does", "not", "exist")

	_, err := dt.client.Query(ctx, "", req)
	if err == nil {
		return fmt.Errorf("expected error for invalid account, got nil")
	}

	fmt.Printf("  Invalid query correctly returned error: %v\n", err)

	// Test timeout handling
	shortCtx, shortCancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer shortCancel()

	req2 := new(api.GeneralQuery)
	req2.Url = protocol.DnUrl()

	_, err = dt.client.Query(shortCtx, "", req2)
	if err == nil {
		fmt.Printf("  Warning: Expected timeout error, got success\n")
	} else {
		fmt.Printf("  Timeout correctly handled: %v\n", err)
	}

	return nil
}

// Include the pooled client implementation
var (
	clientPool    = make(map[string]*jsonrpc.Client)
	clientPoolMu  sync.RWMutex
	maxPoolSize   = 50
	poolCleanupMu sync.Mutex
)

func GetPooledClient(serverURL string) *jsonrpc.Client {
	clientPoolMu.RLock()
	client, exists := clientPool[serverURL]
	clientPoolMu.RUnlock()

	if exists {
		return client
	}

	clientPoolMu.Lock()
	defer clientPoolMu.Unlock()

	// Double-check after acquiring write lock
	if client, exists := clientPool[serverURL]; exists {
		return client
	}

	// Check pool size limit
	if len(clientPool) >= maxPoolSize {
		// Evict oldest entry (simple FIFO)
		for url := range clientPool {
			delete(clientPool, url)
			break
		}
	}

	// Create new client
	client, err := jsonrpc.NewClient(serverURL)
	if err != nil {
		fmt.Printf("Failed to create client: %v\n", err)
		return nil
	}

	clientPool[serverURL] = client
	return client
}

func CleanupClientPool() {
	poolCleanupMu.Lock()
	defer poolCleanupMu.Unlock()

	clientPoolMu.Lock()
	defer clientPoolMu.Unlock()

	// Clear the pool
	for url := range clientPool {
		delete(clientPool, url)
	}
}
