package wallet

import (
	"context"
	"log"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// FaucetClient interface for faucet operations (testable)
type FaucetClient interface {
	Faucet(ctx context.Context, account *url.URL, opts api.FaucetOptions) (*api.Submission, error)
}


// AcmeCollector handles ACME acquisition from faucet
type AcmeCollector struct {
	client         FaucetClient
	maxRequest     uint64
	cooldown       time.Duration
	lastFaucetTime time.Time
	mu             sync.RWMutex
	
	// Metrics
	successfulRequests uint64
	failedRequests     uint64
}


// FundingManager manages the funding account and token distribution
type FundingManager struct {
	wallet           *Wallet
	acmeCollector    *AcmeCollector
	creditManager     *CreditManager
	checkInterval    time.Duration

	// Control
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// FundingConfig holds configuration for the funding manager
type FundingConfig struct {
	ServerURL        string // API server URL (e.g., "http://localhost:26660/v3")
	TargetCredits    uint64
	MaxFaucetRequest uint64
	FaucetCooldown   time.Duration
	CheckInterval    time.Duration
}

// NewAcmeCollector creates a new ACME collector
func NewAcmeCollector(client FaucetClient, maxRequest uint64, cooldown time.Duration) *AcmeCollector {
	if maxRequest == 0 {
		maxRequest = 10000000 // 10M ACME default
	}
	if cooldown == 0 {
		cooldown = 60 * time.Second
	}
	
	return &AcmeCollector{
		client:     client,
		maxRequest: maxRequest,
		cooldown:   cooldown,
	}
}

// CollectAcme requests ACME from the faucet for the given account
func (ac *AcmeCollector) CollectAcme(ctx context.Context, account *url.URL) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	
	// Check cooldown
	if time.Since(ac.lastFaucetTime) < ac.cooldown {
		return nil // Skip, still in cooldown
	}
	
	// Request ACME from faucet (faucet gives fixed amount)
	opts := api.FaucetOptions{
		Token: protocol.AcmeUrl(),
	}
	
	_, err := ac.client.Faucet(ctx, account, opts)
	if err != nil {
		ac.failedRequests++
		return err
	}
	
	ac.successfulRequests++
	ac.lastFaucetTime = time.Now()
	return nil
}

// GetMetrics returns collector metrics
func (ac *AcmeCollector) GetMetrics() (successful, failed uint64) {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	return ac.successfulRequests, ac.failedRequests
}

// CanCollect checks if we can collect (not in cooldown)
func (ac *AcmeCollector) CanCollect() bool {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	return time.Since(ac.lastFaucetTime) >= ac.cooldown
}

// NewFundingManager creates a new funding manager
func NewFundingManager(wallet *Wallet, config *FundingConfig) *FundingManager {
	// Set defaults if not provided
	if config.TargetCredits == 0 {
		config.TargetCredits = 1000
	}
	if config.MaxFaucetRequest == 0 {
		config.MaxFaucetRequest = 10000000 // 10M ACME
	}
	if config.FaucetCooldown == 0 {
		config.FaucetCooldown = 60 * time.Second
	}
	if config.CheckInterval == 0 {
		config.CheckInterval = 5 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Create v3 JSONRPC client
	client := jsonrpc.NewClient(config.ServerURL)

	// Create collectors
	acmeCollector := NewAcmeCollector(client, config.MaxFaucetRequest, config.FaucetCooldown)
	
	// Get funding account for credit manager
	fundingAccount := wallet.GetFundingAccount()
	if fundingAccount == nil {
		log.Println("Warning: No funding account configured")
	}
	
	// Create credit manager
	signer := &DefaultTransactionSigner{}
	creditManager := NewCreditManager(client, client, signer, fundingAccount)

	return &FundingManager{
		wallet:            wallet,
		acmeCollector:     acmeCollector,
		creditManager:     creditManager,
		checkInterval:     config.CheckInterval,
		ctx:               ctx,
		cancel:            cancel,
	}
}

// Start begins the funding manager goroutine
func (fm *FundingManager) Start() {
	fm.wg.Add(1)
	go fm.run()
}

// Stop gracefully stops the funding manager
func (fm *FundingManager) Stop() {
	fm.cancel()
	fm.wg.Wait()
}

// run is the main funding loop
func (fm *FundingManager) run() {
	defer fm.wg.Done()

	log.Println("FundingManager: Starting funding loop")
	ticker := time.NewTicker(fm.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-fm.ctx.Done():
			log.Println("FundingManager: Stopping funding loop")
			return
		case <-ticker.C:
			// Request ACME from faucet
			fm.collectAcme()

			// Top up all accounts with credits
			fm.distributeCredits()
		}
	}
}

// collectAcme requests ACME from the faucet for the funding account
func (fm *FundingManager) collectAcme() {
	fundingAccount := fm.wallet.GetFundingAccount()
	if fundingAccount == nil {
		log.Println("FundingManager: No funding account configured")
		return
	}

	err := fm.acmeCollector.CollectAcme(fm.ctx, fundingAccount.URL)
	if err != nil {
		log.Printf("FundingManager: Failed to collect ACME: %v", err)
	} else if fm.acmeCollector.CanCollect() {
		// Log only if we actually made a request (not in cooldown)
		successful, failed := fm.acmeCollector.GetMetrics()
		log.Printf("FundingManager: Collected ACME (total: %d successful, %d failed)", successful, failed)
	}
}

// distributeCredits tops up credits for all accounts
func (fm *FundingManager) distributeCredits() {
	if fm.creditManager == nil || fm.creditManager.fundingAccount == nil {
		return
	}
	
	// Top up all lite accounts
	fm.topUpLiteAccounts()

	// Top up all key pages
	fm.topUpKeyPages()
}

// topUpLiteAccounts ensures all lite accounts have target credits
func (fm *FundingManager) topUpLiteAccounts() {
	liteAccounts := fm.wallet.GetAllLiteIdentities()
	
	for _, lite := range liteAccounts {
		err := fm.creditManager.TopUpLiteAccount(fm.ctx, lite)
		if err != nil {
			log.Printf("FundingManager: Failed to top up lite account %s: %v", lite.URL, err)
		}
	}
}

// topUpKeyPages ensures all key pages have target credits
func (fm *FundingManager) topUpKeyPages() {
	keyPages := fm.wallet.GetAllKeyPages()
	
	for _, page := range keyPages {
		err := fm.creditManager.TopUpKeyPage(fm.ctx, page)
		if err != nil {
			log.Printf("FundingManager: Failed to top up key page %s: %v", page.URL, err)
		}
	}
}


// GetMetrics returns current funding metrics
func (fm *FundingManager) GetMetrics() FundingMetrics {
	successful, failed := fm.acmeCollector.GetMetrics()
	
	return FundingMetrics{
		SuccessfulRequests: successful,
		FailedRequests:     failed,
	}
}

// FundingMetrics holds funding statistics
type FundingMetrics struct {
	SuccessfulRequests uint64
	FailedRequests     uint64
}