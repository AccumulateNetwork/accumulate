package main

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

const (
	// 12 nodes × 3 workers per node = 36 total generators
	workersPerNode = 3
	totalNodes     = 12
	targetTPS      = 10000
	// Each worker targets: 10,000 TPS / 36 workers = 278 TPS
	tpsPerWorker = targetTPS / (totalNodes * workersPerNode)
	// Accounts per worker (10 each)
	accountsPerWorker = 10
)

func main() {
	// 12 validator API endpoints (one per node)
	nodes := []string{
		"http://localhost:26660/v3", // BVN1-val1
		"http://localhost:26661/v3", // BVN1-val2
		"http://localhost:26662/v3", // BVN1-val3
		"http://localhost:26663/v3", // BVN1-val4
		"http://localhost:26664/v3", // BVN2-val1
		"http://localhost:26665/v3", // BVN2-val2
		"http://localhost:26666/v3", // BVN2-val3
		"http://localhost:26667/v3", // BVN2-val4
		"http://localhost:26668/v3", // BVN3-val1
		"http://localhost:26669/v3", // BVN3-val2
		"http://localhost:26670/v3", // BVN3-val3
		"http://localhost:26671/v3", // BVN3-val4
	}

	totalWorkers := len(nodes) * workersPerNode
	fmt.Printf("Starting parallel load test with %d workers (%d nodes × %d workers/node)\n",
		totalWorkers, len(nodes), workersPerNode)
	fmt.Printf("Target: %d TPS total (~%d TPS per worker)\n\n", targetTPS, tpsPerWorker)

	// Create root context with cancellation for clean shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Global counters
	var submitted, succeeded, failed atomic.Uint64
	var wg sync.WaitGroup

	// Progress reporter with context cancellation
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	start := time.Now()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				elapsed := time.Since(start)
				s := submitted.Load()
				tps := float64(s) / elapsed.Seconds()
				fmt.Printf("Progress: submitted=%d success=%d failure=%d elapsed=%v actual_tps=%.2f\n",
					s, succeeded.Load(), failed.Load(),
					elapsed.Round(time.Second), tps)
			}
		}
	}()

	fmt.Println("Starting load generators...")

	// Configure shared HTTP client with optimized connection pooling
	// For 36 workers, we need sufficient connection capacity
	httpClient := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        200, // Increased for more workers
			IdleConnTimeout:     90 * time.Second,
			MaxIdleConnsPerHost: 20,  // Increased for parallel workers per host
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
		},
		Timeout: 30 * time.Second,
	}

	// Start workers per node
	for nodeIdx, nodeURL := range nodes {
		// Spawn multiple workers for this node
		for workerIdx := 0; workerIdx < workersPerNode; workerIdx++ {
			wg.Add(1)
			workerID := nodeIdx*workersPerNode + workerIdx + 1
			go func(workerID int, nodeURL string) {
				defer wg.Done()
				runWorker(ctx, workerID, nodeURL, httpClient, &submitted, &succeeded, &failed)
			}(workerID, nodeURL)
		}
	}

	fmt.Printf("All %d workers started. Running for 5 minutes...\n\n", totalWorkers)

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Create timer for 5 minute timeout
	timer := time.NewTimer(5 * time.Minute)
	defer timer.Stop()

	// Wait for either timeout or interrupt signal
	select {
	case <-timer.C:
		fmt.Println("\n5 minute test duration reached. Shutting down gracefully...")
	case sig := <-sigChan:
		fmt.Printf("\nReceived signal %v. Shutting down gracefully...\n", sig)
	}

	// Cancel context to stop all workers
	cancel()

	// Wait for all goroutines to finish with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		fmt.Println("All workers stopped cleanly.")
	case <-time.After(10 * time.Second):
		fmt.Println("Warning: Some workers did not stop within timeout.")
	}

	// Close HTTP client connections
	httpClient.CloseIdleConnections()

	// Final report
	elapsed := time.Since(start)
	s := submitted.Load()
	tps := float64(s) / elapsed.Seconds()
	fmt.Printf("\nFinal Results:\n")
	fmt.Printf("  Duration: %v\n", elapsed.Round(time.Second))
	fmt.Printf("  Submitted: %d\n", s)
	fmt.Printf("  Success: %d\n", succeeded.Load())
	fmt.Printf("  Failed: %d\n", failed.Load())
	fmt.Printf("  Average TPS: %.2f\n", tps)

	fmt.Println("\nTest complete!")
}

func runWorker(ctx context.Context, workerID int, nodeURL string, httpClient *http.Client, submitted, succeeded, failed *atomic.Uint64) {
	// Create JSON-RPC client for this worker
	client := jsonrpc.NewClient(nodeURL)

	// Pre-allocate accounts for this worker
	accounts := make([]ed25519.PrivateKey, accountsPerWorker)
	urls := make([]*url.URL, accountsPerWorker)

	for i := range accounts {
		_, sk, _ := ed25519.GenerateKey(nil)
		accounts[i] = sk
		urls[i], _ = protocol.LiteTokenAddress(sk[32:], "ACME", protocol.SignatureTypeED25519)
	}

	// Calculate target interval for this worker
	// 278 TPS = 1 transaction every ~3.6ms
	targetInterval := time.Second / time.Duration(tpsPerWorker)
	nonce := uint64(time.Now().UnixMilli()) + uint64(workerID*1000000)

	for {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			return
		default:
		}

		loopStart := time.Now()

		// Pick sender and receiver from worker's accounts
		sender := int(time.Now().UnixNano() % int64(len(accounts)))
		receiver := (sender + 1) % len(accounts)

		// Build transaction
		env, err := build.Transaction().
			For(urls[sender]).
			SendTokens(1, 0).To(urls[receiver]).
			SignWith(urls[sender]).
			Version(1).
			Timestamp(&nonce).
			PrivateKey(accounts[sender]).
			Done()

		if err != nil {
			failed.Add(1)
			continue
		}

		// Submit transaction with context
		submitted.Add(1)

		// Create timeout context for this request
		reqCtx, reqCancel := context.WithTimeout(ctx, 10*time.Second)
		_, err = client.Submit(reqCtx, env, api.SubmitOptions{})
		reqCancel() // Always cancel to release resources

		if err != nil {
			// Check if error is due to context cancellation (shutdown)
			if ctx.Err() != nil {
				return
			}
			failed.Add(1)
		} else {
			succeeded.Add(1)
		}

		// Rate limit with context awareness
		elapsed := time.Since(loopStart)
		if elapsed < targetInterval {
			sleepDuration := targetInterval - elapsed
			select {
			case <-ctx.Done():
				return
			case <-time.After(sleepDuration):
				// Continue to next iteration
			}
		}
	}
}
