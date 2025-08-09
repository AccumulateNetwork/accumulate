package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
)

// ConnectionDiagnostics runs diagnostics to identify v3 connection issues
type ConnectionDiagnostics struct {
	serverURL string
}

func NewDiagnostics(serverURL string) *ConnectionDiagnostics {
	return &ConnectionDiagnostics{serverURL: serverURL}
}

func (d *ConnectionDiagnostics) RunAll() {
	fmt.Println("========================================")
	fmt.Println("     V3 CONNECTION DIAGNOSTICS")
	fmt.Println("========================================")
	fmt.Println()

	d.testBasicConnection()
	d.testConnectionPooling()
	d.testConcurrentConnections()
	d.testConnectionLeaks()
	d.testTimeoutBehavior()
	d.printRecommendations()
}

func (d *ConnectionDiagnostics) testBasicConnection() {
	fmt.Println("1. BASIC CONNECTION TEST")
	fmt.Println("------------------------")

	client := jsonrpc.NewClient(d.serverURL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	_, err := client.NodeInfo(ctx, api.NodeInfoOptions{})
	duration := time.Since(start)

	if err != nil {
		fmt.Printf("❌ Connection failed: %v\n", err)
		fmt.Printf("   Duration: %v\n", duration)

		// Check error type
		if netErr, ok := err.(net.Error); ok {
			if netErr.Timeout() {
				fmt.Println("   Error type: Network timeout")
			} else {
				fmt.Println("   Error type: Network error (not timeout)")
			}
		}
	} else {
		fmt.Printf("✓ Connection successful\n")
		fmt.Printf("   Response time: %v\n", duration)
	}
	fmt.Println()
}

func (d *ConnectionDiagnostics) testConnectionPooling() {
	fmt.Println("2. CONNECTION POOLING TEST")
	fmt.Println("--------------------------")

	// Test with default client (no pooling)
	fmt.Println("Testing default client (no connection pooling):")
	defaultTimes := make([]time.Duration, 10)
	for i := 0; i < 10; i++ {
		client := jsonrpc.NewClient(d.serverURL)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		start := time.Now()
		_, err := client.NodeInfo(ctx, api.NodeInfoOptions{})
		defaultTimes[i] = time.Since(start)
		cancel()

		if err != nil {
			fmt.Printf("  Request %d: Failed - %v\n", i+1, err)
		}
	}

	// Test with reused client
	fmt.Println("\nTesting reused client:")
	reuseClient := jsonrpc.NewClient(d.serverURL)
	reuseTimes := make([]time.Duration, 10)
	for i := 0; i < 10; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		start := time.Now()
		_, err := reuseClient.NodeInfo(ctx, api.NodeInfoOptions{})
		reuseTimes[i] = time.Since(start)
		cancel()

		if err != nil {
			fmt.Printf("  Request %d: Failed - %v\n", i+1, err)
		}
	}

	// Compare results
	var defaultAvg, reuseAvg time.Duration
	for i := 0; i < 10; i++ {
		defaultAvg += defaultTimes[i]
		reuseAvg += reuseTimes[i]
	}
	defaultAvg /= 10
	reuseAvg /= 10

	fmt.Printf("\nResults:\n")
	fmt.Printf("  New client each time: avg %v\n", defaultAvg)
	fmt.Printf("  Reused client: avg %v\n", reuseAvg)

	if reuseAvg < defaultAvg {
		improvement := (float64(defaultAvg-reuseAvg) / float64(defaultAvg)) * 100
		fmt.Printf("✓ Connection reuse is %.1f%% faster\n", improvement)
	} else {
		fmt.Printf("⚠ No improvement from connection reuse\n")
	}
	fmt.Println()
}

func (d *ConnectionDiagnostics) testConcurrentConnections() {
	fmt.Println("3. CONCURRENT CONNECTIONS TEST")
	fmt.Println("-------------------------------")

	concurrencyLevels := []int{1, 5, 10, 20, 50}

	for _, level := range concurrencyLevels {
		var successCount int32
		var errorCount int32
		var wg sync.WaitGroup

		start := time.Now()

		for i := 0; i < level; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				client := jsonrpc.NewClient(d.serverURL)
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				_, err := client.NodeInfo(ctx, api.NodeInfoOptions{})
				if err != nil {
					atomic.AddInt32(&errorCount, 1)
				} else {
					atomic.AddInt32(&successCount, 1)
				}
			}()
		}

		wg.Wait()
		duration := time.Since(start)

		fmt.Printf("  Concurrency %d: %d success, %d errors in %v\n",
			level, successCount, errorCount, duration)

		if errorCount > 0 {
			fmt.Printf("    ⚠ Connection errors at concurrency level %d\n", level)
		}
	}
	fmt.Println()
}

func (d *ConnectionDiagnostics) testConnectionLeaks() {
	fmt.Println("4. CONNECTION LEAK TEST")
	fmt.Println("-----------------------")

	// Get initial goroutine count
	initialGoroutines := runtime.NumGoroutine()

	// Create many clients without proper cleanup
	for i := 0; i < 100; i++ {
		client := jsonrpc.NewClient(d.serverURL)
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		client.NodeInfo(ctx, api.NodeInfoOptions{})
		cancel()
	}

	// Force GC
	runtime.GC()
	time.Sleep(1 * time.Second)
	runtime.GC()

	// Check goroutine count
	finalGoroutines := runtime.NumGoroutine()
	leaked := finalGoroutines - initialGoroutines

	fmt.Printf("  Initial goroutines: %d\n", initialGoroutines)
	fmt.Printf("  Final goroutines: %d\n", finalGoroutines)

	if leaked > 10 {
		fmt.Printf("❌ Potential goroutine leak: %d goroutines\n", leaked)
	} else if leaked > 0 {
		fmt.Printf("⚠ Minor goroutine increase: %d\n", leaked)
	} else {
		fmt.Printf("✓ No goroutine leaks detected\n")
	}
	fmt.Println()
}

func (d *ConnectionDiagnostics) testTimeoutBehavior() {
	fmt.Println("5. TIMEOUT BEHAVIOR TEST")
	fmt.Println("------------------------")

	timeouts := []time.Duration{
		100 * time.Millisecond,
		500 * time.Millisecond,
		1 * time.Second,
		5 * time.Second,
	}

	for _, timeout := range timeouts {
		client := jsonrpc.NewClient(d.serverURL)
		ctx, cancel := context.WithTimeout(context.Background(), timeout)

		start := time.Now()
		_, err := client.NodeInfo(ctx, api.NodeInfoOptions{})
		duration := time.Since(start)
		cancel()

		if err != nil {
			fmt.Printf("  Timeout %v: Failed after %v - %v\n", timeout, duration, err)
		} else {
			fmt.Printf("  Timeout %v: Success in %v\n", timeout, duration)
		}
	}
	fmt.Println()
}

func (d *ConnectionDiagnostics) printRecommendations() {
	fmt.Println("========================================")
	fmt.Println("         RECOMMENDATIONS")
	fmt.Println("========================================")
	fmt.Println()

	fmt.Println("IDENTIFIED ISSUES:")
	fmt.Println("1. No connection pooling in default client")
	fmt.Println("2. Hard-coded 15-second timeout in jsonrpc.Client")
	fmt.Println("3. New HTTP client created for each jsonrpc.NewClient()")
	fmt.Println("4. No retry logic for transient failures")
	fmt.Println("5. Default transport settings not optimized for high load")
	fmt.Println()

	fmt.Println("IMMEDIATE WORKAROUNDS:")
	fmt.Println("1. Reuse jsonrpc.Client instances instead of creating new ones")
	fmt.Println("2. Implement retry logic at the application level")
	fmt.Println("3. Use longer timeouts for operations that might be slow")
	fmt.Println("4. Limit concurrent connections to avoid overwhelming the server")
	fmt.Println("5. Monitor and log connection errors to identify patterns")
	fmt.Println()

	fmt.Println("EXAMPLE PATTERN FOR EXISTING CODE:")
	fmt.Println("```go")
	fmt.Println("// Create once and reuse")
	fmt.Println("var globalClient = jsonrpc.NewClient(\"http://127.0.0.1:26660/v3\")")
	fmt.Println()
	fmt.Println("// Add retry logic")
	fmt.Println("func queryWithRetry(ctx context.Context) error {")
	fmt.Println("    for i := 0; i < 3; i++ {")
	fmt.Println("        err := doQuery(ctx)")
	fmt.Println("        if err == nil {")
	fmt.Println("            return nil")
	fmt.Println("        }")
	fmt.Println("        time.Sleep(time.Second * time.Duration(i+1))")
	fmt.Println("    }")
	fmt.Println("    return fmt.Errorf(\"max retries exceeded\")")
	fmt.Println("}")
	fmt.Println("```")
}

func main() {
	diagnostics := NewDiagnostics("http://127.0.0.1:26660/v3")
	diagnostics.RunAll()

	// Also test with custom transport
	fmt.Println("\n========================================")
	fmt.Println("     CUSTOM TRANSPORT TEST")
	fmt.Println("========================================")
	fmt.Println()

	// Create client with custom transport
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		DisableKeepAlives:   false,
	}

	client := jsonrpc.NewClient("http://127.0.0.1:26660/v3")
	client.Client.Transport = transport
	client.Client.Timeout = 30 * time.Second

	fmt.Println("Testing with optimized transport settings:")
	fmt.Printf("  MaxIdleConns: 100\n")
	fmt.Printf("  MaxIdleConnsPerHost: 10\n")
	fmt.Printf("  IdleConnTimeout: 90s\n")
	fmt.Printf("  KeepAlives: Enabled\n")
	fmt.Println()

	// Run stress test with optimized client
	var successCount int32
	var errorCount int32
	var wg sync.WaitGroup

	testCount := 100
	start := time.Now()

	for i := 0; i < testCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			_, err := client.NodeInfo(ctx, api.NodeInfoOptions{})
			if err != nil {
				atomic.AddInt32(&errorCount, 1)
			} else {
				atomic.AddInt32(&successCount, 1)
			}
		}()
	}

	wg.Wait()
	duration := time.Since(start)

	fmt.Printf("Results with optimized transport:\n")
	fmt.Printf("  Requests: %d\n", testCount)
	fmt.Printf("  Success: %d\n", successCount)
	fmt.Printf("  Errors: %d\n", errorCount)
	fmt.Printf("  Duration: %v\n", duration)
	fmt.Printf("  Avg time per request: %v\n", duration/time.Duration(testCount))

	if errorCount == 0 {
		fmt.Println("\n✓ Optimized transport eliminated connection errors!")
	} else {
		fmt.Printf("\n⚠ Still seeing %d errors with optimized transport\n", errorCount)
	}
}
