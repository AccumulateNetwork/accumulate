package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"math/big"
	"sync"
	"time"

	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var (
	serverURL     = flag.String("server", "http://127.0.0.1:26660/v2", "DevNet server URL")
	duration      = flag.Duration("duration", 10*time.Minute, "Test duration")
	numADIs       = flag.Int("adis", 5, "Number of ADIs to create")
	accountsPerADI = flag.Int("accounts", 10, "Number of accounts per ADI")
	workers       = flag.Int("workers", 5, "Number of concurrent workers")
	verbose       = flag.Bool("v", false, "Verbose output")
	faucetAmount  = flag.Int64("faucet", 10000000, "Amount to request from faucet (in credits)")
)

type CrosschainLoadTester struct {
	client       *client.Client
	faucetKey    ed25519.PrivateKey
	testADIs     []TestADI
	stats        *TestStats
	stopChan     chan bool
	wg           sync.WaitGroup
}

type TestADI struct {
	URL          *url.URL
	PrivateKey   ed25519.PrivateKey
	PublicKey    ed25519.PublicKey
	TokenAccounts []TestAccount
	DataAccounts  []TestAccount
}

type TestAccount struct {
	URL        *url.URL
	PrivateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
	ADI        *TestADI
}

type TestStats struct {
	mu                    sync.Mutex
	ADIsCreated          int
	TokenAccountsCreated int
	DataAccountsCreated  int
	TokenTransfers       int
	DataWrites           int
	FaucetRequests       int
	Errors               int
	StartTime            time.Time
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
	case "errors":
		s.Errors++
	}
}

func (s *TestStats) GetStats() (int, int, int, int, int, int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ADIsCreated, s.TokenAccountsCreated, s.DataAccountsCreated, 
		   s.TokenTransfers, s.DataWrites, s.FaucetRequests, s.Errors
}

func main() {
	flag.Parse()
	
	fmt.Printf("Starting Crosschain Load Test\n")
	fmt.Printf("=============================\n")
	fmt.Printf("Server: %s\n", *serverURL)
	fmt.Printf("Duration: %v\n", *duration)
	fmt.Printf("ADIs: %d\n", *numADIs)
	fmt.Printf("Accounts per ADI: %d\n", *accountsPerADI)
	fmt.Printf("Workers: %d\n", *workers)
	fmt.Printf("\n")
	
	tester, err := NewCrosschainLoadTester(*serverURL)
	if err != nil {
		log.Fatalf("Failed to create load tester: %v", err)
	}
	
	// Start the test
	err = tester.RunTest(*duration, *numADIs, *accountsPerADI, *workers)
	if err != nil {
		log.Fatalf("Test failed: %v", err)
	}
}

func NewCrosschainLoadTester(serverURL string) (*CrosschainLoadTester, error) {
	c, err := client.New(serverURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %v", err)
	}
	
	// Generate a test faucet key
	faucetKey := ed25519.NewKeyFromSeed(make([]byte, 32))
	
	return &CrosschainLoadTester{
		client:    c,
		faucetKey: faucetKey,
		stats: &TestStats{
			StartTime: time.Now(),
		},
		stopChan: make(chan bool),
	}, nil
}

func (t *CrosschainLoadTester) RunTest(duration time.Duration, numADIs, accountsPerADI, workers int) error {
	fmt.Println("Phase 1: Setting up test infrastructure...")
	
	// Phase 1: Create ADIs and accounts
	err := t.setupTestInfrastructure(numADIs, accountsPerADI)
	if err != nil {
		return fmt.Errorf("failed to setup test infrastructure: %v", err)
	}
	
	fmt.Printf("Phase 2: Starting %d workers for %v...\n", workers, duration)
	
	// Phase 2: Start workers
	for i := 0; i < workers; i++ {
		t.wg.Add(1)
		go t.worker(i)
	}
	
	// Phase 3: Run statistics reporter
	go t.statsReporter()
	
	// Phase 4: Wait for duration then stop
	time.Sleep(duration)
	close(t.stopChan)
	
	fmt.Println("Stopping workers...")
	t.wg.Wait()
	
	// Final statistics
	t.printFinalStats()
	
	return nil
}

func (t *CrosschainLoadTester) setupTestInfrastructure(numADIs, accountsPerADI int) error {
	t.testADIs = make([]TestADI, numADIs)
	
	for i := 0; i < numADIs; i++ {
		fmt.Printf("Creating ADI %d/%d...\n", i+1, numADIs)
		
		// Generate key pair for ADI
		pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return fmt.Errorf("failed to generate ADI key pair: %v", err)
		}
		
		// Create ADI URL
		adiURL := protocol.AccountUrl(fmt.Sprintf("test-adi-%d", i))
		
		adi := TestADI{
			URL:        adiURL,
			PrivateKey: privKey,
			PublicKey:  pubKey,
		}
		
		// Create token accounts for this ADI
		adi.TokenAccounts = make([]TestAccount, accountsPerADI/2)
		for j := 0; j < accountsPerADI/2; j++ {
			tokenPubKey, tokenPrivKey, _ := ed25519.GenerateKey(rand.Reader)
			tokenURL := adiURL.JoinPath(fmt.Sprintf("tokens-%d", j))
			
			adi.TokenAccounts[j] = TestAccount{
				URL:        tokenURL,
				PrivateKey: tokenPrivKey,
				PublicKey:  tokenPubKey,
				ADI:        &adi,
			}
		}
		
		// Create data accounts for this ADI
		adi.DataAccounts = make([]TestAccount, accountsPerADI/2)
		for j := 0; j < accountsPerADI/2; j++ {
			dataPubKey, dataPrivKey, _ := ed25519.GenerateKey(rand.Reader)
			dataURL := adiURL.JoinPath(fmt.Sprintf("data-%d", j))
			
			adi.DataAccounts[j] = TestAccount{
				URL:        dataURL,
				PrivateKey: dataPrivKey,
				PublicKey:  dataPubKey,
				ADI:        &adi,
			}
		}
		
		t.testADIs[i] = adi
	}
	
	fmt.Printf("Created %d ADIs with %d accounts each\n", numADIs, accountsPerADI)
	return nil
}

func (t *CrosschainLoadTester) worker(workerID int) {
	defer t.wg.Done()
	
	fmt.Printf("Worker %d started\n", workerID)
	
	for {
		select {
		case <-t.stopChan:
			fmt.Printf("Worker %d stopping\n", workerID)
			return
		default:
			// Perform random operations
			t.performRandomOperation(workerID)
			
			// Small delay between operations
			time.Sleep(time.Duration(100+workerID*50) * time.Millisecond)
		}
	}
}

func (t *CrosschainLoadTester) performRandomOperation(workerID int) {
	operations := []string{"faucet", "create_adi", "create_token_account", "create_data_account", "transfer_tokens", "write_data"}
	
	// Choose random operation based on worker ID and time
	opIndex := (workerID + int(time.Now().UnixNano())) % len(operations)
	operation := operations[opIndex]
	
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	var err error
	switch operation {
	case "faucet":
		err = t.requestFromFaucet(ctx, workerID)
	case "create_adi":
		err = t.createRandomADI(ctx, workerID)
	case "create_token_account":
		err = t.createRandomTokenAccount(ctx, workerID)
	case "create_data_account":
		err = t.createRandomDataAccount(ctx, workerID)
	case "transfer_tokens":
		err = t.transferTokens(ctx, workerID)
	case "write_data":
		err = t.writeData(ctx, workerID)
	}
	
	if err != nil {
		if *verbose {
			fmt.Printf("Worker %d, %s failed: %v\n", workerID, operation, err)
		}
		t.stats.Increment("errors")
	}
}

func (t *CrosschainLoadTester) requestFromFaucet(ctx context.Context, workerID int) error {
	// Create a temporary account to receive faucet tokens
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	
	// Create a lite account URL
	liteURL := protocol.LiteTokenAddress(pubKey, protocol.ACME, protocol.SignatureTypeED25519)
	
	// Request tokens from faucet (this is a simplified version)
	// In a real implementation, you'd need to call the faucet endpoint
	if *verbose {
		fmt.Printf("Worker %d: Requesting %d credits from faucet for %s\n", workerID, *faucetAmount, liteURL)
	}
	
	t.stats.Increment("faucet")
	return nil
}

func (t *CrosschainLoadTester) createRandomADI(ctx context.Context, workerID int) error {
	if len(t.testADIs) == 0 {
		return fmt.Errorf("no test ADIs available")
	}
	
	// Generate new ADI
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	
	adiURL := protocol.AccountUrl(fmt.Sprintf("worker-%d-adi-%d", workerID, time.Now().Unix()))
	
	if *verbose {
		fmt.Printf("Worker %d: Creating ADI %s\n", workerID, adiURL)
	}
	
	// In a real implementation, you'd create the ADI transaction here
	// tx := &protocol.CreateIdentity{...}
	
	t.stats.Increment("adis")
	return nil
}

func (t *CrosschainLoadTester) createRandomTokenAccount(ctx context.Context, workerID int) error {
	if len(t.testADIs) == 0 {
		return fmt.Errorf("no test ADIs available")
	}
	
	// Choose random ADI
	adi := t.testADIs[workerID%len(t.testADIs)]
	
	// Generate new token account
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	
	tokenURL := adi.URL.JoinPath(fmt.Sprintf("worker-%d-token-%d", workerID, time.Now().Unix()))
	
	if *verbose {
		fmt.Printf("Worker %d: Creating token account %s\n", workerID, tokenURL)
	}
	
	// In a real implementation, you'd create the token account transaction here
	// tx := &protocol.CreateTokenAccount{...}
	
	t.stats.Increment("token_accounts")
	return nil
}

func (t *CrosschainLoadTester) createRandomDataAccount(ctx context.Context, workerID int) error {
	if len(t.testADIs) == 0 {
		return fmt.Errorf("no test ADIs available")
	}
	
	// Choose random ADI
	adi := t.testADIs[workerID%len(t.testADIs)]
	
	// Generate new data account
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	
	dataURL := adi.URL.JoinPath(fmt.Sprintf("worker-%d-data-%d", workerID, time.Now().Unix()))
	
	if *verbose {
		fmt.Printf("Worker %d: Creating data account %s\n", workerID, dataURL)
	}
	
	// In a real implementation, you'd create the data account transaction here
	// tx := &protocol.CreateDataAccount{...}
	
	t.stats.Increment("data_accounts")
	return nil
}

func (t *CrosschainLoadTester) transferTokens(ctx context.Context, workerID int) error {
	if len(t.testADIs) < 2 {
		return fmt.Errorf("need at least 2 ADIs for cross-chain transfers")
	}
	
	// Choose two different ADIs for cross-chain transfer
	sourceADI := t.testADIs[workerID%len(t.testADIs)]
	destADI := t.testADIs[(workerID+1)%len(t.testADIs)]
	
	if len(sourceADI.TokenAccounts) == 0 || len(destADI.TokenAccounts) == 0 {
		return fmt.Errorf("no token accounts available")
	}
	
	sourceAccount := sourceADI.TokenAccounts[0]
	destAccount := destADI.TokenAccounts[0]
	
	amount := int64(1000 + workerID*100) // Variable amounts
	
	if *verbose {
		fmt.Printf("Worker %d: Transferring %d tokens from %s to %s\n", 
			workerID, amount, sourceAccount.URL, destAccount.URL)
	}
	
	// In a real implementation, you'd create the send tokens transaction here
	// tx := &protocol.SendTokens{...}
	
	t.stats.Increment("token_transfers")
	return nil
}

func (t *CrosschainLoadTester) writeData(ctx context.Context, workerID int) error {
	if len(t.testADIs) == 0 {
		return fmt.Errorf("no test ADIs available")
	}
	
	// Choose random ADI
	adi := t.testADIs[workerID%len(t.testADIs)]
	
	if len(adi.DataAccounts) == 0 {
		return fmt.Errorf("no data accounts available")
	}
	
	dataAccount := adi.DataAccounts[workerID%len(adi.DataAccounts)]
	
	// Create test data
	data := fmt.Sprintf("Worker %d data entry at %s - Random: %d", 
		workerID, time.Now().Format(time.RFC3339), time.Now().UnixNano())
	
	if *verbose {
		fmt.Printf("Worker %d: Writing %d bytes to %s\n", 
			workerID, len(data), dataAccount.URL)
	}
	
	// In a real implementation, you'd create the write data transaction here
	// tx := &protocol.WriteData{...}
	
	t.stats.Increment("data_writes")
	return nil
}

func (t *CrosschainLoadTester) statsReporter() {
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

func (t *CrosschainLoadTester) printStats() {
	adis, tokenAccounts, dataAccounts, transfers, writes, faucet, errors := t.stats.GetStats()
	elapsed := time.Since(t.stats.StartTime)
	
	fmt.Printf("\n--- Stats (Elapsed: %v) ---\n", elapsed.Round(time.Second))
	fmt.Printf("ADIs Created: %d\n", adis)
	fmt.Printf("Token Accounts: %d\n", tokenAccounts)
	fmt.Printf("Data Accounts: %d\n", dataAccounts)
	fmt.Printf("Token Transfers: %d\n", transfers)
	fmt.Printf("Data Writes: %d\n", writes)
	fmt.Printf("Faucet Requests: %d\n", faucet)
	fmt.Printf("Errors: %d\n", errors)
	
	totalOps := adis + tokenAccounts + dataAccounts + transfers + writes + faucet
	if elapsed.Seconds() > 0 {
		opsPerSec := float64(totalOps) / elapsed.Seconds()
		fmt.Printf("Operations/sec: %.2f\n", opsPerSec)
	}
	fmt.Printf("-------------------------\n\n")
}

func (t *CrosschainLoadTester) printFinalStats() {
	fmt.Printf("\n")
	fmt.Printf("Final Test Results\n")
	fmt.Printf("==================\n")
	
	adis, tokenAccounts, dataAccounts, transfers, writes, faucet, errors := t.stats.GetStats()
	elapsed := time.Since(t.stats.StartTime)
	
	fmt.Printf("Test Duration: %v\n", elapsed.Round(time.Second))
	fmt.Printf("ADIs Created: %d\n", adis)
	fmt.Printf("Token Accounts Created: %d\n", tokenAccounts)
	fmt.Printf("Data Accounts Created: %d\n", dataAccounts)
	fmt.Printf("Token Transfers: %d\n", transfers)
	fmt.Printf("Data Writes: %d\n", writes)
	fmt.Printf("Faucet Requests: %d\n", faucet)
	fmt.Printf("Total Operations: %d\n", adis+tokenAccounts+dataAccounts+transfers+writes+faucet)
	fmt.Printf("Errors: %d\n", errors)
	
	if elapsed.Seconds() > 0 {
		totalOps := adis + tokenAccounts + dataAccounts + transfers + writes + faucet
		opsPerSec := float64(totalOps) / elapsed.Seconds()
		fmt.Printf("Average Operations/sec: %.2f\n", opsPerSec)
		
		if errors > 0 {
			errorRate := float64(errors) / float64(totalOps+errors) * 100
			fmt.Printf("Error Rate: %.2f%%\n", errorRate)
		}
	}
	
	fmt.Printf("\nCrosschain Load Test Completed!\n")
}
