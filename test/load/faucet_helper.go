package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	accurl "gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

const (
	DefaultACMEPerRequest = 10000000 // 10 ACME in credits
	FaucetEndpoint        = "/faucet"
)

// FaucetHelper provides continuous token funding for test accounts
type FaucetHelper struct {
	client      *client.Client
	serverURL   string
	running     bool
	stopChan    chan struct{}
	wg          sync.WaitGroup
	mu          sync.RWMutex
	
	// Statistics
	stats       FaucetStats
	
	// Configuration
	config      FaucetConfig
}

type FaucetConfig struct {
	ACMEPerRequest    int64         // Amount to request per faucet call
	RequestInterval   time.Duration // Time between faucet requests
	MaxConcurrentReqs int           // Maximum concurrent faucet requests
	RetryDelay        time.Duration // Delay before retrying failed requests
}

type FaucetStats struct {
	mu                sync.RWMutex
	StartTime         time.Time
	TotalRequests     int64
	SuccessfulReqs    int64
	FailedReqs        int64
	TotalACMEFunded   int64
	AccountsCreated   int64
	LastRequestTime   time.Time
}

type FundedAccount struct {
	URL        *accurl.URL
	PrivateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
	Balance    int64
	CreatedAt  time.Time
}

// NewFaucetHelper creates a new faucet helper process
func NewFaucetHelper(serverURL string, config *FaucetConfig) (*FaucetHelper, error) {
	c, err := client.New(serverURL + "/v2")
	if err != nil {
		return nil, fmt.Errorf("failed to create API client: %v", err)
	}
	
	// Set default configuration if not provided
	if config == nil {
		config = &FaucetConfig{
			ACMEPerRequest:    DefaultACMEPerRequest,
			RequestInterval:   2 * time.Second,
			MaxConcurrentReqs: 3,
			RetryDelay:        1 * time.Second,
		}
	}
	
	return &FaucetHelper{
		client:    c,
		serverURL: serverURL,
		stopChan:  make(chan struct{}),
		config:    *config,
		stats: FaucetStats{
			StartTime: time.Now(),
		},
	}, nil
}

// Start begins the background faucet funding process
func (fh *FaucetHelper) Start(ctx context.Context) {
	fh.mu.Lock()
	if fh.running {
		fh.mu.Unlock()
		return
	}
	fh.running = true
	fh.mu.Unlock()
	
	log.Printf("🚰 Starting faucet helper with %d ACME per request, %v interval", 
		fh.config.ACMEPerRequest/1000000, fh.config.RequestInterval)
	
	// Start background funding process
	for i := 0; i < fh.config.MaxConcurrentReqs; i++ {
		fh.wg.Add(1)
		go fh.fundingWorker(ctx, i)
	}
}

// Stop gracefully shuts down the faucet helper
func (fh *FaucetHelper) Stop() {
	fh.mu.Lock()
	if !fh.running {
		fh.mu.Unlock()
		return
	}
	fh.running = false
	fh.mu.Unlock()
	
	close(fh.stopChan)
	fh.wg.Wait()
	
	log.Printf("🛑 Faucet helper stopped after funding %.2f ACME across %d accounts", 
		float64(fh.GetStats().TotalACMEFunded)/1000000, fh.GetStats().AccountsCreated)
}

// CreateFundedAccount creates a new lite account and funds it with ACME
func (fh *FaucetHelper) CreateFundedAccount(targetAmount int64) (*FundedAccount, error) {
	// Generate key pair
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key pair: %v", err)
	}
	
	// Create lite token account URL
	liteURL, err := protocol.LiteTokenAddress(pubKey, protocol.ACME, protocol.SignatureTypeED25519)
	if err != nil {
		return nil, fmt.Errorf("failed to create lite address: %v", err)
	}
	
	account := &FundedAccount{
		URL:        liteURL,
		PrivateKey: privKey,
		PublicKey:  pubKey,
		CreatedAt:  time.Now(),
	}
	
	// Fund the account to the target amount
	err = fh.FundAccountToTarget(account, targetAmount)
	if err != nil {
		return nil, fmt.Errorf("failed to fund account: %v", err)
	}
	
	fh.stats.mu.Lock()
	fh.stats.AccountsCreated++
	fh.stats.mu.Unlock()
	
	return account, nil
}

// FundAccountToTarget funds an account until it reaches the target amount
func (fh *FaucetHelper) FundAccountToTarget(account *FundedAccount, targetAmount int64) error {
	requestsNeeded := (targetAmount + fh.config.ACMEPerRequest - 1) / fh.config.ACMEPerRequest
	
	log.Printf("💰 Funding account %s with %.2f ACME (%d requests)", 
		account.URL.String()[:20]+"...", float64(targetAmount)/1000000, requestsNeeded)
	
	for account.Balance < targetAmount {
		amount, err := fh.requestFromFaucet(account.URL)
		if err != nil {
			log.Printf("❌ Faucet request failed for %s: %v", account.URL, err)
			time.Sleep(fh.config.RetryDelay)
			continue
		}
		
		account.Balance += amount
		
		// Short delay between requests to avoid overwhelming faucet
		time.Sleep(500 * time.Millisecond)
	}
	
	log.Printf("✅ Account funded: %.2f ACME", float64(account.Balance)/1000000)
	return nil
}

// CreateMultipleFundedAccounts creates multiple funded accounts concurrently
func (fh *FaucetHelper) CreateMultipleFundedAccounts(count int, amountPerAccount int64) ([]*FundedAccount, error) {
	accounts := make([]*FundedAccount, count)
	errors := make([]error, count)
	
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			account, err := fh.CreateFundedAccount(amountPerAccount)
			accounts[index] = account
			errors[index] = err
		}(i)
	}
	
	wg.Wait()
	
	// Check for errors
	for i, err := range errors {
		if err != nil {
			return nil, fmt.Errorf("failed to create account %d: %v", i, err)
		}
	}
	
	return accounts, nil
}

// GetStats returns current faucet helper statistics
func (fh *FaucetHelper) GetStats() FaucetStats {
	fh.stats.mu.RLock()
	defer fh.stats.mu.RUnlock()
	return fh.stats
}

// PrintStats prints current statistics to the console
func (fh *FaucetHelper) PrintStats() {
	stats := fh.GetStats()
	elapsed := time.Since(stats.StartTime)
	
	fmt.Printf("\n🚰 Faucet Helper Statistics (Running: %v)\n", elapsed.Round(time.Second))
	fmt.Printf("📡 Total Requests: %d (Success: %d, Failed: %d)\n", 
		stats.TotalRequests, stats.SuccessfulReqs, stats.FailedReqs)
	fmt.Printf("💰 Total ACME Funded: %.2f ACME\n", float64(stats.TotalACMEFunded)/1000000)
	fmt.Printf("🏦 Accounts Created: %d\n", stats.AccountsCreated)
	
	if stats.TotalRequests > 0 {
		successRate := float64(stats.SuccessfulReqs) / float64(stats.TotalRequests) * 100
		fmt.Printf("📊 Success Rate: %.1f%%\n", successRate)
	}
	
	if elapsed.Seconds() > 0 {
		rps := float64(stats.TotalRequests) / elapsed.Seconds()
		aps := float64(stats.TotalACMEFunded) / 1000000 / elapsed.Seconds()
		fmt.Printf("⚡ Rates: %.2f req/s, %.2f ACME/s\n", rps, aps)
	}
	fmt.Println()
}

// fundingWorker runs continuous background funding
func (fh *FaucetHelper) fundingWorker(ctx context.Context, workerID int) {
	defer fh.wg.Done()
	
	ticker := time.NewTicker(fh.config.RequestInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-fh.stopChan:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Create and fund a random account for general testing
			fh.createBackgroundFundedAccount(workerID)
		}
	}
}

// createBackgroundFundedAccount creates accounts in the background for general use
func (fh *FaucetHelper) createBackgroundFundedAccount(workerID int) {
	account, err := fh.CreateFundedAccount(fh.config.ACMEPerRequest * 5) // Fund with 50 ACME
	if err != nil {
		log.Printf("Worker %d: Failed to create background account: %v", workerID, err)
		return
	}
	
	log.Printf("Worker %d: Created background account with %.2f ACME: %s", 
		workerID, float64(account.Balance)/1000000, account.URL.String()[:30]+"...")
}

// requestFromFaucet makes a single faucet request
func (fh *FaucetHelper) requestFromFaucet(accountURL *accurl.URL) (int64, error) {
	fh.stats.mu.Lock()
	fh.stats.TotalRequests++
	fh.stats.LastRequestTime = time.Now()
	fh.stats.mu.Unlock()
	
	// Make HTTP request to faucet
	resp, err := http.Post(
		fh.serverURL+FaucetEndpoint,
		"text/plain",
		strings.NewReader(accountURL.String()),
	)
	if err != nil {
		fh.stats.mu.Lock()
		fh.stats.FailedReqs++
		fh.stats.mu.Unlock()
		return 0, fmt.Errorf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()
	
	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fh.stats.mu.Lock()
		fh.stats.FailedReqs++
		fh.stats.mu.Unlock()
		return 0, fmt.Errorf("failed to read response: %v", err)
	}
	
	if resp.StatusCode != http.StatusOK {
		fh.stats.mu.Lock()
		fh.stats.FailedReqs++
		fh.stats.mu.Unlock()
		return 0, fmt.Errorf("faucet request failed (status %d): %s", resp.StatusCode, string(body))
	}
	
	// Parse response if it's JSON, otherwise assume success
	var faucetResp struct {
		TransactionHash string `json:"txid"`
		Amount         int64  `json:"amount"`
	}
	
	amount := fh.config.ACMEPerRequest // Default amount
	if json.Unmarshal(body, &faucetResp) == nil && faucetResp.Amount > 0 {
		amount = faucetResp.Amount
	}
	
	fh.stats.mu.Lock()
	fh.stats.SuccessfulReqs++
	fh.stats.TotalACMEFunded += amount
	fh.stats.mu.Unlock()
	
	return amount, nil
}