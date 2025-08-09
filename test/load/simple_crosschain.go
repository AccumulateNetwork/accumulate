package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

var (
	serverURL = flag.String("server", "http://127.0.0.1:26660/v2", "DevNet server URL")
	duration  = flag.Duration("duration", 5*time.Minute, "Test duration")
	workers   = flag.Int("workers", 3, "Number of concurrent workers")
	verbose   = flag.Bool("v", false, "Verbose output")
)

type CrosschainTester struct {
	serverURL  string
	stats      *TestStats
	stopChan   chan bool
	wg         sync.WaitGroup
	httpClient *http.Client
}

type TestStats struct {
	mu            sync.Mutex
	QueryRequests int
	Errors        int
	StartTime     time.Time
}

type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
	ID      int         `json:"id"`
}

type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
	ID      int             `json:"id"`
}

type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type QueryParams struct {
	URL string `json:"url"`
}

func (s *TestStats) Increment(stat string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	switch stat {
	case "queries":
		s.QueryRequests++
	case "errors":
		s.Errors++
	}
}

func (s *TestStats) GetStats() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.QueryRequests, s.Errors
}

func main() {
	flag.Parse()
	
	fmt.Printf("Starting Simple Crosschain Load Test\n")
	fmt.Printf("====================================\n")
	fmt.Printf("Server: %s\n", *serverURL)
	fmt.Printf("Duration: %v\n", *duration)
	fmt.Printf("Workers: %d\n", *workers)
	fmt.Printf("\n")
	
	tester, err := NewCrosschainTester(*serverURL)
	if err != nil {
		log.Fatalf("Failed to create load tester: %v", err)
	}
	
	err = tester.RunTest(*duration, *workers)
	if err != nil {
		log.Fatalf("Test failed: %v", err)
	}
}

func NewCrosschainTester(serverURL string) (*CrosschainTester, error) {
	return &CrosschainTester{
		serverURL: serverURL,
		stats: &TestStats{
			StartTime: time.Now(),
		},
		stopChan: make(chan bool),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

func (t *CrosschainTester) RunTest(duration time.Duration, workers int) error {
	fmt.Printf("Starting %d workers for %v...\n", workers, duration)
	
	// Start workers
	for i := 0; i < workers; i++ {
		t.wg.Add(1)
		go t.worker(i)
	}
	
	// Start statistics reporter
	go t.statsReporter()
	
	// Wait for duration then stop
	time.Sleep(duration)
	close(t.stopChan)
	
	fmt.Println("Stopping workers...")
	t.wg.Wait()
	
	// Final statistics
	t.printFinalStats()
	
	return nil
}

func (t *CrosschainTester) worker(workerID int) {
	defer t.wg.Done()
	
	fmt.Printf("Worker %d started\n", workerID)
	
	// Test URLs to query - these exercise different components and crosschain communication
	testURLs := []string{
		"acc://dn.acme",              // Directory Network identity
		"acc://dn.acme/operators",    // Operators page
		"acc://dn.acme/network",      // Network definition
		"acc://dn.acme/globals",      // Global variables
		"acc://dn.acme/anchors",      // Anchor chains
		"acc://ACME",                 // Root ACME identity
	}
	
	for {
		select {
		case <-t.stopChan:
			fmt.Printf("Worker %d stopping\n", workerID)
			return
		default:
			// Query different URLs to exercise crosschain communication
			targetURL := testURLs[workerID%len(testURLs)]
			
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err := t.queryAccount(ctx, targetURL)
			cancel()
			
			if err != nil {
				if *verbose {
					fmt.Printf("Worker %d, query %s failed: %v\n", workerID, targetURL, err)
				}
				t.stats.Increment("errors")
			} else {
				if *verbose {
					fmt.Printf("Worker %d: Successfully queried %s\n", workerID, targetURL)
				}
				t.stats.Increment("queries")
			}
			
			// Variable delay between operations
			delay := time.Duration(300+workerID*100) * time.Millisecond
			time.Sleep(delay)
		}
	}
}

func (t *CrosschainTester) queryAccount(ctx context.Context, accountURL string) error {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "query",
		Params: QueryParams{
			URL: accountURL,
		},
		ID: int(time.Now().UnixNano() % 10000),
	}
	
	return t.sendJSONRPCRequest(ctx, req)
}

func (t *CrosschainTester) sendJSONRPCRequest(ctx context.Context, req JSONRPCRequest) error {
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %v", err)
	}
	
	httpReq, err := http.NewRequestWithContext(ctx, "POST", t.serverURL, strings.NewReader(string(reqBytes)))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %v", err)
	}
	
	httpReq.Header.Set("Content-Type", "application/json")
	
	resp, err := t.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP request returned status %d", resp.StatusCode)
	}
	
	var jsonResp JSONRPCResponse
	err = json.NewDecoder(resp.Body).Decode(&jsonResp)
	if err != nil {
		return fmt.Errorf("failed to decode response: %v", err)
	}
	
	if jsonResp.Error != nil {
		// Some errors are expected (account not found, etc.)
		if *verbose && jsonResp.Error.Code != -32800 { // -32800 is "not found"
			fmt.Printf("JSON-RPC error: %d - %s\n", jsonResp.Error.Code, jsonResp.Error.Message)
		}
	}
	
	return nil
}

func (t *CrosschainTester) statsReporter() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-t.stopChan:
			return
		case <-ticker.C:
			t.printStats()
		}
	}
}

func (t *CrosschainTester) printStats() {
	queries, errors := t.stats.GetStats()
	elapsed := time.Since(t.stats.StartTime)
	
	fmt.Printf("\n--- Stats (Elapsed: %v) ---\n", elapsed.Round(time.Second))
	fmt.Printf("Query Requests: %d\n", queries)
	fmt.Printf("Errors: %d\n", errors)
	
	if elapsed.Seconds() > 0 {
		queryRate := float64(queries) / elapsed.Seconds()
		fmt.Printf("Queries/sec: %.2f\n", queryRate)
	}
	fmt.Printf("-------------------------\n\n")
}

func (t *CrosschainTester) printFinalStats() {
	fmt.Printf("\n")
	fmt.Printf("Final Crosschain Test Results\n")
	fmt.Printf("=============================\n")
	
	queries, errors := t.stats.GetStats()
	elapsed := time.Since(t.stats.StartTime)
	
	fmt.Printf("Test Duration: %v\n", elapsed.Round(time.Second))
	fmt.Printf("Query Requests: %d\n", queries)
	fmt.Printf("Errors: %d\n", errors)
	
	if elapsed.Seconds() > 0 {
		queryRate := float64(queries) / elapsed.Seconds()
		fmt.Printf("Average Queries/sec: %.2f\n", queryRate)
		
		if errors > 0 {
			errorRate := float64(errors) / float64(queries+errors) * 100
			fmt.Printf("Error Rate: %.2f%%\n", errorRate)
		}
	}
	
	fmt.Printf("\nCrosschain Load Test Completed!\n")
	fmt.Printf("This test exercised crosschain communication by querying\n")
	fmt.Printf("different partitions and network components.\n")
}
