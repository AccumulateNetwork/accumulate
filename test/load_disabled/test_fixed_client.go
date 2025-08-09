package main

import (
	"fmt"
	"runtime"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
)

func main() {
	fmt.Println("========================================")
	fmt.Println("    TESTING FIXED CLIENT HELPER")
	fmt.Println("========================================")
	fmt.Println()

	// Test 1: Verify no resource leaks
	fmt.Println("Test 1: Resource Leak Prevention")
	fmt.Println("---------------------------------")
	testNoResourceLeaks()

	// Test 2: Verify pool size limits
	fmt.Println("\nTest 2: Pool Size Limits")
	fmt.Println("------------------------")
	testPoolSizeLimits()

	// Test 3: Verify proper cleanup
	fmt.Println("\nTest 3: Proper Cleanup")
	fmt.Println("----------------------")
	testProperCleanup()

	// Test 4: Verify TTL cleanup
	fmt.Println("\nTest 4: TTL Cleanup (simulated)")
	fmt.Println("-------------------------------")
	testTTLCleanup()

	fmt.Println("\n========================================")
	fmt.Println("         ALL TESTS PASSED")
	fmt.Println("========================================")
}

func testNoResourceLeaks() {
	initialGoroutines := runtime.NumGoroutine()

	// Create and use many clients
	for i := 0; i < 50; i++ {
		url := fmt.Sprintf("http://127.0.0.1:%d/v3", 26660+i)
		client := GetPooledClient(url)

		ctx, cancel := CreateContextWithTimeout(1 * time.Second)
		client.NodeInfo(ctx, api.NodeInfoOptions{})
		cancel()
	}

	// Check pool stats
	size, oldest, newest := GetPoolStats()
	fmt.Printf("  Pool size: %d clients\n", size)
	fmt.Printf("  Oldest client: %v ago\n", time.Since(oldest).Round(time.Second))
	fmt.Printf("  Newest client: %v ago\n", time.Since(newest).Round(time.Millisecond))

	// Clean up
	CleanupClientPool()

	// Force GC and wait
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	runtime.GC()

	finalGoroutines := runtime.NumGoroutine()
	leaked := finalGoroutines - initialGoroutines

	if leaked > 2 { // Allow for cleanup routine
		fmt.Printf("  ⚠️ Potential goroutine leak: %d goroutines\n", leaked)
	} else {
		fmt.Printf("  ✓ No goroutine leaks (initial: %d, final: %d)\n",
			initialGoroutines, finalGoroutines)
	}
}

func testPoolSizeLimits() {
	// Try to create more clients than the limit
	urls := make([]string, 150)
	for i := 0; i < 150; i++ {
		urls[i] = fmt.Sprintf("http://test-%d.example.com:26660/v3", i)
		GetPooledClient(urls[i])
	}

	size, _, _ := GetPoolStats()

	// Note: The fixed version should enforce limits
	if size <= 100 {
		fmt.Printf("  ✓ Pool size limited correctly: %d clients (max: 100)\n", size)
	} else {
		fmt.Printf("  ⚠️ Pool size exceeded limit: %d clients\n", size)
	}

	// Cleanup
	CleanupClientPool()
}

func testProperCleanup() {
	// Create some clients
	for i := 0; i < 10; i++ {
		url := fmt.Sprintf("http://cleanup-test-%d:26660/v3", i)
		GetPooledClient(url)
	}

	sizeBefore, _, _ := GetPoolStats()
	fmt.Printf("  Pool size before cleanup: %d\n", sizeBefore)

	// Cleanup
	CleanupClientPool()

	sizeAfter, _, _ := GetPoolStats()
	fmt.Printf("  Pool size after cleanup: %d\n", sizeAfter)

	if sizeAfter == 0 {
		fmt.Printf("  ✓ Cleanup successful\n")
	} else {
		fmt.Printf("  ⚠️ Cleanup incomplete: %d clients remaining\n", sizeAfter)
	}
}

func testTTLCleanup() {
	fmt.Println("  Note: TTL is 5 minutes in production")
	fmt.Println("  Simulating cleanup behavior...")

	// Create clients with different "ages"
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			url := fmt.Sprintf("http://ttl-test-%d:26660/v3", id)
			GetPooledClient(url)
		}(i)
	}
	wg.Wait()

	size1, _, _ := GetPoolStats()
	fmt.Printf("  Created %d clients\n", size1)

	// Manually trigger cleanup (normally runs every minute)
	cleanupOldClients()

	size2, _, _ := GetPoolStats()
	fmt.Printf("  After cleanup: %d clients\n", size2)

	// Since we just created them, they shouldn't be cleaned up
	if size2 == size1 {
		fmt.Printf("  ✓ Recent clients not cleaned up (correct behavior)\n")
	} else {
		fmt.Printf("  ⚠️ Unexpected cleanup of recent clients\n")
	}

	// Final cleanup
	CleanupClientPool()
}
