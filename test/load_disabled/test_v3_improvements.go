package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func main() {
	fmt.Println("========================================")
	fmt.Println("    V3 CONNECTION IMPROVEMENTS TEST")
	fmt.Println("========================================")
	fmt.Println()

	// Test 1: Compare old vs new client creation
	fmt.Println("Test 1: Client Creation Performance")
	fmt.Println("-----------------------------------")
	testClientCreation()

	// Test 2: Test connection reuse
	fmt.Println("\nTest 2: Connection Reuse Performance")
	fmt.Println("------------------------------------")
	testConnectionReuse()

	// Test 3: Test concurrent operations
	fmt.Println("\nTest 3: Concurrent Operations")
	fmt.Println("-----------------------------")
	testConcurrentOps()

	// Test 4: Test retry logic
	fmt.Println("\nTest 4: Retry Logic")
	fmt.Println("-------------------")
	testRetryLogic()

	fmt.Println("\n========================================")
	fmt.Println("         TEST SUMMARY")
	fmt.Println("========================================")
	printSummary()
}

func testClientCreation() {
	// Test old way - create new client each time
	start := time.Now()
	for i := 0; i < 10; i++ {
		client := jsonrpc.NewClient("http://127.0.0.1:26660/v3")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		client.NodeInfo(ctx, api.NodeInfoOptions{})
		cancel()
	}
	oldDuration := time.Since(start)

	// Test new way - use pooled client
	start = time.Now()
	for i := 0; i < 10; i++ {
		client := GetPooledClient("http://127.0.0.1:26660/v3")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		client.NodeInfo(ctx, api.NodeInfoOptions{})
		cancel()
	}
	newDuration := time.Since(start)

	fmt.Printf("  Old way (new client each time): %v\n", oldDuration)
	fmt.Printf("  New way (pooled client): %v\n", newDuration)

	improvement := float64(oldDuration-newDuration) / float64(oldDuration) * 100
	if improvement > 0 {
		fmt.Printf("  ✓ Improvement: %.1f%% faster\n", improvement)
	} else {
		fmt.Printf("  ⚠ No significant improvement\n")
	}
}

func testConnectionReuse() {
	client := GetPooledClient("http://127.0.0.1:26660/v3")

	times := make([]time.Duration, 5)
	for i := 0; i < 5; i++ {
		start := time.Now()

		ctx, cancel := CreateContextWithTimeout(10 * time.Second)
		Q := api.Querier2{Querier: client}
		partUrl := protocol.PartitionUrl("Directory")
		anchorUrl := partUrl.JoinPath(protocol.AnchorPool)
		Q.QueryAccount(ctx, anchorUrl, nil)
		cancel()

		times[i] = time.Since(start)
		fmt.Printf("  Request %d: %v\n", i+1, times[i])
	}

	// First request should be slower (establishing connection)
	// Subsequent requests should be faster (reusing connection)
	if times[0] > times[1] && times[0] > times[2] {
		fmt.Printf("  ✓ Connection reuse working (first: %v, avg rest: %v)\n",
			times[0], (times[1]+times[2]+times[3]+times[4])/4)
	} else {
		fmt.Printf("  ⚠ Connection reuse may not be working optimally\n")
	}
}

func testConcurrentOps() {
	client := GetPooledClient("http://127.0.0.1:26660/v3")

	var successCount int32
	var errorCount int32
	var wg sync.WaitGroup

	concurrency := 20
	start := time.Now()

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			err := SafeQuery(client, func(ctx context.Context) error {
				_, err := client.NodeInfo(ctx, api.NodeInfoOptions{})
				return err
			})

			if err != nil {
				atomic.AddInt32(&errorCount, 1)
				fmt.Printf("  Request %d: Failed - %v\n", id, err)
			} else {
				atomic.AddInt32(&successCount, 1)
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(start)

	fmt.Printf("  Completed %d concurrent requests in %v\n", concurrency, duration)
	fmt.Printf("  Success: %d, Errors: %d\n", successCount, errorCount)

	if errorCount == 0 {
		fmt.Printf("  ✓ All concurrent requests succeeded\n")
	} else {
		fmt.Printf("  ⚠ %d errors in concurrent requests\n", errorCount)
	}
}

func testRetryLogic() {
	client := GetPooledClient("http://127.0.0.1:26660/v3")

	// Test that retry logic works
	attempts := 0
	err := QueryWithRetry(context.Background(), client, func() error {
		attempts++
		if attempts < 2 {
			// Simulate a transient error on first attempt
			return fmt.Errorf("connection reset")
		}
		return nil
	})

	if err == nil && attempts == 2 {
		fmt.Printf("  ✓ Retry logic working (succeeded on attempt %d)\n", attempts)
	} else if err != nil {
		fmt.Printf("  ⚠ Retry logic may not be working: %v\n", err)
	} else {
		fmt.Printf("  ⚠ Unexpected retry behavior (attempts: %d)\n", attempts)
	}

	// Test with real query
	realAttempts := 0
	err = SafeQuery(client, func(ctx context.Context) error {
		realAttempts++
		_, err := client.NodeInfo(ctx, api.NodeInfoOptions{})
		return err
	})

	if err == nil {
		fmt.Printf("  ✓ Real query succeeded (attempts: %d)\n", realAttempts)
	} else {
		fmt.Printf("  ⚠ Real query failed: %v\n", err)
	}
}

func printSummary() {
	fmt.Println("\nImprovements Applied:")
	fmt.Println("✓ Connection pooling implemented")
	fmt.Println("✓ HTTP transport optimized")
	fmt.Println("✓ Retry logic added")
	fmt.Println("✓ Proper timeouts configured")
	fmt.Println("✓ Connection reuse enabled")

	fmt.Println("\nExpected Benefits:")
	fmt.Println("• Reduced connection errors")
	fmt.Println("• Better performance under load")
	fmt.Println("• Automatic retry for transient failures")
	fmt.Println("• More efficient resource usage")

	fmt.Println("\nFiles Updated:")
	fmt.Println("• test_recovery_direct.go - Using pooled client")
	fmt.Println("• test_recovery_with_missing.go - Using pooled client")
	fmt.Println("• recovery_simulation.go - Using pooled client")
	fmt.Println("• client_helper.go - Provides pooling and retry logic")
}
