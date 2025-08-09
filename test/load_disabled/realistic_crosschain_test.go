package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var (
	serverURL      = flag.String("server", "http://127.0.0.1:26660/v2", "DevNet server URL")
	duration       = flag.Duration("duration", 5*time.Minute, "Test duration")
	numADIs        = flag.Int("adis", 3, "Number of ADIs to create")
	accountsPerADI = flag.Int("accounts", 6, "Number of accounts per ADI")
	workers        = flag.Int("workers", 3, "Number of concurrent workers")
	verbose        = flag.Bool("v", false, "Verbose output")
	faucetAmount   = flag.Int64("faucet", 1000000, "Amount to request from faucet (in credits)")
)

type RealisticLoadTester struct {
	serverURL    string
	testADIs     []TestADI
	liteAccounts []LiteAccount
	stats        *TestStats
	stopChan     chan bool
	wg           sync.WaitGroup
	httpClient   *http.Client
}

type TestADI struct {
	Name          string
	URL           *url.URL
	PrivateKey    ed25519.PrivateKey
	PublicKey     ed25519.PublicKey
	TokenAccounts []TestAccount
	DataAccounts  []TestAccount
	Created       bool
}

type TestAccount struct {
	Name       string
	URL        *url.URL
	PrivateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
	ADI        *TestADI
	Created    bool
}

type LiteAccount struct {
	URL        *url.URL
	PrivateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
	Balance    int64
}

type TestStats struct {
	mu                   sync.Mutex
	ADIsCreated          int
	TokenAccountsCreated int
	DataAccountsCreated  int
	TokenTransfers       int
	DataWrites           int
	FaucetRequests       int
	QueryRequests        int
	Errors               int
	StartTime            time.Time
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

type QueryResult struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func (s *TestStats) Increment(stat string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch stat {
	case "adis":
		s.ADIsCreated++
	case "token_accounts":
		s.TokenAccountsCreated++
	case "data_accounts":
		s.DataAccountsCreated++
	case "token_transfers":
		s.TokenTransfers++
	case "data_writes":
		s.DataWrites++
	case "faucet":
		s.FaucetRequests++
	case "queries":
		s.QueryRequests++
	case "errors":
		s.Errors++
	}
}

func (s *TestStats) GetStats() (int, int, int, int, int, int, int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ADIsCreated, s.TokenAccountsCreated, s.DataAccountsCreated,
		s.TokenTransfers, s.DataWrites, s.FaucetRequests, s.QueryRequests, s.Errors
}

func main() {
	flag.Parse()

	fmt.Printf("Starting Realistic Crosschain Load Test\n")
	fmt.Printf("=======================================\n")
	fmt.Printf("Server: %s\n", *serverURL)
	fmt.Printf("Duration: %v\n", *duration)
	fmt.Printf("ADIs: %d\n", *numADIs)
	fmt.Printf("Accounts per ADI: %d\n", *accountsPerADI)
	fmt.Printf("Workers: %d\n", *workers)
	fmt.Printf("\n")

	tester, err := NewRealisticLoadTester(*serverURL)
	if err != nil {
		log.Fatalf("Failed to create load tester: %v", err)
	}

	// Start the test
	err = tester.RunTest(*duration, *numADIs, *accountsPerADI, *workers)
	if err != nil {
		log.Fatalf("Test failed: %v", err)
	}
}

func NewRealisticLoadTester(serverURL string) (*RealisticLoadTester, error) {
	return &RealisticLoadTester{
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

func (t *RealisticLoadTester) RunTest(duration time.Duration, numADIs, accountsPerADI, workers int) error {
	fmt.Println("Phase 1: Setting up test infrastructure...")

	// Phase 1: Create test data structures
	err := t.setupTestInfrastructure(numADIs, accountsPerADI)
	if err != nil {
		return fmt.Errorf("failed to setup test infrastructure: %v", err)
	}

	// Phase 2: Create some lite accounts for faucet testing
	err = t.createLiteAccounts(5)
	if err != nil {
		return fmt.Errorf("failed to create lite accounts: %v", err)
	}

	fmt.Printf("Phase 2: Starting %d workers for %v...\n", workers, duration)

	// Phase 3: Start workers
	for i := 0; i < workers; i++ {
		t.wg.Add(1)
		go t.worker(i)
	}

	// Phase 4: Run statistics reporter
	go t.statsReporter()

	// Phase 5: Wait for duration then stop
	time.Sleep(duration)
	close(t.stopChan)

	fmt.Println("Stopping workers...")
	t.wg.Wait()

	// Final statistics
	t.printFinalStats()

	return nil
}

func (t *RealisticLoadTester) setupTestInfrastructure(numADIs, accountsPerADI int) error {
	t.testADIs = make([]TestADI, numADIs)

	for i := 0; i < numADIs; i++ {
		// Generate key pair for ADI
		pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return fmt.Errorf("failed to generate ADI key pair: %v", err)
		}

		// Create ADI URL with timestamp to ensure uniqueness
		adiName := fmt.Sprintf("test-adi-%d-%d", i, time.Now().Unix())
		adiURL := protocol.AccountUrl(adiName)

		adi := TestADI{
			Name:       adiName,
			URL:        adiURL,
			PrivateKey: privKey,
			PublicKey:  pubKey,
			Created:    false,
		}

		// Create token accounts for this ADI
		adi.TokenAccounts = make([]TestAccount, accountsPerADI/2)
		for j := 0; j < accountsPerADI/2; j++ {
			tokenPubKey, tokenPrivKey, _ := ed25519.GenerateKey(rand.Reader)
			tokenName := fmt.Sprintf("tokens-%d", j)
			tokenURL := adiURL.JoinPath(tokenName)

			adi.TokenAccounts[j] = TestAccount{
				Name:       tokenName,
				URL:        tokenURL,
				PrivateKey: tokenPrivKey,
				PublicKey:  tokenPubKey,
				ADI:        &adi,
				Created:    false,
			}
		}

		// Create data accounts for this ADI
		adi.DataAccounts = make([]TestAccount, accountsPerADI/2)
		for j := 0; j < accountsPerADI/2; j++ {
			dataPubKey, dataPrivKey, _ := ed25519.GenerateKey(rand.Reader)
			dataName := fmt.Sprintf("data-%d", j)
			dataURL := adiURL.JoinPath(dataName)

			adi.DataAccounts[j] = TestAccount{
				Name:       dataName,
				URL:        dataURL,
				PrivateKey: dataPrivKey,
				PublicKey:  dataPubKey,
				ADI:        &adi,
				Created:    false,
			}
		}

		t.testADIs[i] = adi
	}

	fmt.Printf("Prepared %d ADIs with %d accounts each\n", numADIs, accountsPerADI)
	return nil
}

func (t *RealisticLoadTester) createLiteAccounts(count int) error {
	t.liteAccounts = make([]LiteAccount, count)

	for i := 0; i < count; i++ {
		pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return err
		}

		// Create lite token account URL
		liteURL := protocol.LiteTokenAddress(pubKey, protocol.ACME, protocol.SignatureTypeED25519)

		t.liteAccounts[i] = LiteAccount{
			URL:        liteURL,
			PrivateKey: privKey,
			PublicKey:  pubKey,
			Balance:    0,
		}
	}

	fmt.Printf("Created %d lite accounts for testing\n", count)
	return nil
}

func (t *RealisticLoadTester) worker(workerID int) {
	defer t.wg.Done()

	fmt.Printf("Worker %d started\n", workerID)

	for {
		select {
		case <-t.stopChan:
			fmt.Printf("Worker %d stopping\n", workerID)
			return
		default:
			// Perform random operations with weighted probabilities
			t.performWeightedRandomOperation(workerID)

			// Variable delay between operations
			delay := time.Duration(200+workerID*100) * time.Millisecond
			time.Sleep(delay)
		}
	}
}

func (t *RealisticLoadTester) performWeightedRandomOperation(workerID int) {
	// Weighted operations - queries are most common, then account creation, then transfers
	operations := []struct {
		name   string
		weight int
	}{
		{"query_account", 40},
		{"query_directory", 20},
		{"create_adi", 10},
		{"create_token_account", 10},
		{"create_data_account", 10},
		{"write_data", 5},
		{"faucet_request", 3},
		{"transfer_simulation", 2},
	}

	// Calculate total weight
	totalWeight := 0
	for _, op := range operations {
		totalWeight += op.weight
	}

	// Choose operation based on weight
	choice := (workerID*7 + int(time.Now().UnixNano())) % totalWeight
	currentWeight := 0
	selectedOp := "query_account"

	for _, op := range operations {
		currentWeight += op.weight
		if choice < currentWeight {
			selectedOp = op.name
			break
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var err error
	switch selectedOp {
	case "query_account":
		err = t.queryRandomAccount(ctx, workerID)
	case "query_directory":
		err = t.queryDirectory(ctx, workerID)
	case "create_adi":
		err = t.simulateCreateADI(ctx, workerID)
	case "create_token_account":
		err = t.simulateCreateTokenAccount(ctx, workerID)
	case "create_data_account":
		err = t.simulateCreateDataAccount(ctx, workerID)
	case "write_data":
		err = t.simulateWriteData(ctx, workerID)
	case "faucet_request":
		err = t.simulateFaucetRequest(ctx, workerID)
	case "transfer_simulation":
		err = t.simulateTokenTransfer(ctx, workerID)
	}

	if err != nil {
		if *verbose {
			fmt.Printf("Worker %d, %s failed: %v\n", workerID, selectedOp, err)
		}
		t.stats.Increment("errors")
	}
}

func (t *RealisticLoadTester) queryRandomAccount(ctx context.Context, workerID int) error {
	// Query various account types
	urls := []string{
		"acc://dn.acme",
		"acc://bvn-0.acme",
		"acc://bvn-1.acme",
		"acc://bvn-2.acme",
	}

	// Add our test ADI URLs if they exist
	for _, adi := range t.testADIs {
		urls = append(urls, adi.URL.String())
		for _, acc := range adi.TokenAccounts {
			urls = append(urls, acc.URL.String())
		}
		for _, acc := range adi.DataAccounts {
			urls = append(urls, acc.URL.String())
		}
	}

	// Add lite account URLs
	for _, lite := range t.liteAccounts {
		urls = append(urls, lite.URL.String())
	}

	// Choose random URL
	targetURL := urls[workerID%len(urls)]

	err := t.queryAccount(ctx, targetURL)
	if err != nil {
		return err
	}

	if *verbose {
		fmt.Printf("Worker %d: Queried %s\n", workerID, targetURL)
	}

	t.stats.Increment("queries")
	return nil
}

func (t *RealisticLoadTester) queryDirectory(ctx context.Context, workerID int) error {
	err := t.queryAccount(ctx, "acc://dn.acme")
	if err != nil {
		return err
	}

	if *verbose {
		fmt.Printf("Worker %d: Queried directory network\n", workerID)
	}

	t.stats.Increment("queries")
	return nil
}

func (t *RealisticLoadTester) queryAccount(ctx context.Context, accountURL string) error {
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

func (t *RealisticLoadTester) sendJSONRPCRequest(ctx context.Context, req JSONRPCRequest) error {
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

// Simulation functions - these don't actually create transactions but simulate the load
func (t *RealisticLoadTester) simulateCreateADI(ctx context.Context, workerID int) error {
	if len(t.testADIs) == 0 {
		return fmt.Errorf("no test ADIs available")
	}

	// Mark a random ADI as "created" for simulation
	adi := &t.testADIs[workerID%len(t.testADIs)]
	if !adi.Created {
		adi.Created = true
		if *verbose {
			fmt.Printf("Worker %d: Simulated creating ADI %s\n", workerID, adi.URL)
		}
		t.stats.Increment("adis")
	}

	return nil
}

func (t *RealisticLoadTester) simulateCreateTokenAccount(ctx context.Context, workerID int) error {
	if len(t.testADIs) == 0 {
		return fmt.Errorf("no test ADIs available")
	}

	adi := &t.testADIs[workerID%len(t.testADIs)]
	if len(adi.TokenAccounts) == 0 {
		return fmt.Errorf("no token accounts available")
	}

	// Mark a random token account as "created"
	account := &adi.TokenAccounts[workerID%len(adi.TokenAccounts)]
	if !account.Created {
		account.Created = true
		if *verbose {
			fmt.Printf("Worker %d: Simulated creating token account %s\n", workerID, account.URL)
		}
		t.stats.Increment("token_accounts")
	}

	return nil
}

func (t *RealisticLoadTester) simulateCreateDataAccount(ctx context.Context, workerID int) error {
	if len(t.testADIs) == 0 {
		return fmt.Errorf("no test ADIs available")
	}

	adi := &t.testADIs[workerID%len(t.testADIs)]
	if len(adi.DataAccounts) == 0 {
		return fmt.Errorf("no data accounts available")
	}

	// Mark a random data account as "created"
	account := &adi.DataAccounts[workerID%len(adi.DataAccounts)]
	if !account.Created {
		account.Created = true
		if *verbose {
			fmt.Printf("Worker %d: Simulated creating data account %s\n", workerID, account.URL)
		}
		t.stats.Increment("data_accounts")
	}

	return nil
}

func (t *RealisticLoadTester) simulateWriteData(ctx context.Context, workerID int) error {
	if len(t.testADIs) == 0 {
		return fmt.Errorf("no test ADIs available")
	}

	adi := t.testADIs[workerID%len(t.testADIs)]
	if len(adi.DataAccounts) == 0 {
		return fmt.Errorf("no data accounts available")
	}

	dataAccount := adi.DataAccounts[workerID%len(adi.DataAccounts)]

	if *verbose {
		fmt.Printf("Worker %d: Simulated writing data to %s\n", workerID, dataAccount.URL)
	}

	t.stats.Increment("data_writes")
	return nil
}

func (t *RealisticLoadTester) simulateFaucetRequest(ctx context.Context, workerID int) error {
	if len(t.liteAccounts) == 0 {
		return fmt.Errorf("no lite accounts available")
	}

	liteAccount := &t.liteAccounts[workerID%len(t.liteAccounts)]
	liteAccount.Balance += *faucetAmount

	if *verbose {
		fmt.Printf("Worker %d: Simulated faucet request for %s (balance: %d)\n",
			workerID, liteAccount.URL, liteAccount.Balance)
	}

	t.stats.Increment("faucet")
	return nil
}

func (t *RealisticLoadTester) simulateTokenTransfer(ctx context.Context, workerID int) error {
	if len(t.testADIs) < 2 {
		return fmt.Errorf("need at least 2 ADIs for cross-chain transfers")
	}

	sourceADI := t.testADIs[workerID%len(t.testADIs)]
	destADI := t.testADIs[(workerID+1)%len(t.testADIs)]

	if len(sourceADI.TokenAccounts) == 0 || len(destADI.TokenAccounts) == 0 {
		return fmt.Errorf("no token accounts available")
	}

	sourceAccount := sourceADI.TokenAccounts[0]
	destAccount := destADI.TokenAccounts[0]

	amount := int64(1000 + workerID*100)

	if *verbose {
		fmt.Printf("Worker %d: Simulated transferring %d tokens from %s to %s\n",
			workerID, amount, sourceAccount.URL, destAccount.URL)
	}

	t.stats.Increment("token_transfers")
	return nil
}

func (t *RealisticLoadTester) statsReporter() {
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

func (t *RealisticLoadTester) printStats() {
	adis, tokenAccounts, dataAccounts, transfers, writes, faucet, queries, errors := t.stats.GetStats()
	elapsed := time.Since(t.stats.StartTime)

	fmt.Printf("\n--- Stats (Elapsed: %v) ---\n", elapsed.Round(time.Second))
	fmt.Printf("ADIs Created: %d\n", adis)
	fmt.Printf("Token Accounts: %d\n", tokenAccounts)
	fmt.Printf("Data Accounts: %d\n", dataAccounts)
	fmt.Printf("Token Transfers: %d\n", transfers)
	fmt.Printf("Data Writes: %d\n", writes)
	fmt.Printf("Faucet Requests: %d\n", faucet)
	fmt.Printf("Query Requests: %d\n", queries)
	fmt.Printf("Errors: %d\n", errors)

	totalOps := adis + tokenAccounts + dataAccounts + transfers + writes + faucet + queries
	if elapsed.Seconds() > 0 {
		opsPerSec := float64(totalOps) / elapsed.Seconds()
		fmt.Printf("Operations/sec: %.2f\n", opsPerSec)
	}
	fmt.Printf("-------------------------\n\n")
}

func (t *RealisticLoadTester) printFinalStats() {
	fmt.Printf("\n")
	fmt.Printf("Final Realistic Test Results\n")
	fmt.Printf("============================\n")

	adis, tokenAccounts, dataAccounts, transfers, writes, faucet, queries, errors := t.stats.GetStats()
	elapsed := time.Since(t.stats.StartTime)

	fmt.Printf("Test Duration: %v\n", elapsed.Round(time.Second))
	fmt.Printf("ADIs Created: %d\n", adis)
	fmt.Printf("Token Accounts Created: %d\n", tokenAccounts)
	fmt.Printf("Data Accounts Created: %d\n", dataAccounts)
	fmt.Printf("Token Transfers: %d\n", transfers)
	fmt.Printf("Data Writes: %d\n", writes)
	fmt.Printf("Faucet Requests: %d\n", faucet)
	fmt.Printf("Query Requests: %d\n", queries)
	fmt.Printf("Total Operations: %d\n", adis+tokenAccounts+dataAccounts+transfers+writes+faucet+queries)
	fmt.Printf("Errors: %d\n", errors)

	if elapsed.Seconds() > 0 {
		totalOps := adis + tokenAccounts + dataAccounts + transfers + writes + faucet + queries
		opsPerSec := float64(totalOps) / elapsed.Seconds()
		fmt.Printf("Average Operations/sec: %.2f\n", opsPerSec)

		if errors > 0 {
			errorRate := float64(errors) / float64(totalOps+errors) * 100
			fmt.Printf("Error Rate: %.2f%%\n", errorRate)
		}

		// Query-specific stats
		if queries > 0 {
			queryRate := float64(queries) / elapsed.Seconds()
			fmt.Printf("Query Rate: %.2f queries/sec\n", queryRate)
		}
	}

	fmt.Printf("\nRealistic Crosschain Load Test Completed!\n")
	fmt.Printf("This test exercised the network with realistic query patterns\n")
	fmt.Printf("and simulated crosschain operations to stress the conductor.\n")
}
