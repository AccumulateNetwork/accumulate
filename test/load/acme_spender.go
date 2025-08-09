package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"log"
	"sync"
	"time"

	v3api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ACMESpender provides various transaction types that spend ACME tokens
type ACMESpender struct {
	client      *jsonrpc.Client
	serverURL   string
	running     bool
	stopChan    chan struct{}
	wg          sync.WaitGroup
	mu          sync.RWMutex
	
	// Statistics
	stats       SpenderStats
	
	// Configuration  
	config      SpenderConfig
	
	// Test accounts for spending
	spenderAccounts []*FundedAccount
}

type SpenderConfig struct {
	WorkerCount          int           // Number of concurrent spending workers
	TransactionInterval  time.Duration // Time between transactions
	MaxRetries          int           // Maximum retry attempts
	RetryDelay          time.Duration // Delay between retries
	
	// Transaction type weights (probability distribution)
	TokenTransferWeight     int // Cross-partition token transfers
	DataWriteWeight        int // Data account writes  
	AccountCreateWeight    int // ADI/account creation (ADIs, token accounts, data accounts)
	TokenSendWeight        int // Simple token sends
	DataCollectWeight      int // Data collection/querying
	TokenIssueWeight       int // Token issuing operations
	IssuedTokenMoveWeight  int // Moving issued tokens
	TokenAccountCreateWeight int // Creating ADI token accounts
	DataAccountCreateWeight  int // Creating ADI data accounts
}

type SpenderStats struct {
	mu                   sync.RWMutex
	StartTime           time.Time
	
	// Transaction counts
	TotalTransactions   int64
	SuccessfulTxs      int64
	FailedTxs          int64
	
	// By transaction type
	TokenTransfers      int64
	DataWrites         int64
	AccountCreations   int64
	TokenSends         int64
	DataCollections    int64
	TokenIssues        int64
	IssuedTokenMoves   int64
	TokenAccountCreates int64
	DataAccountCreates  int64
	
	// ACME tracking
	TotalACMESpent     int64
	FeesLaid          int64
	
	// Performance
	LastTxTime        time.Time
}

type TransactionResult struct {
	TxType    string
	Success   bool
	ACMESpent int64
	Error     error
	Duration  time.Duration
}

// NewACMESpender creates a new ACME spending test suite
func NewACMESpender(serverURL string, config *SpenderConfig) (*ACMESpender, error) {
	c := jsonrpc.NewClient(serverURL + "/v3")
	
	// Set default configuration
	if config == nil {
		config = &SpenderConfig{
			WorkerCount:         3,
			TransactionInterval: 2 * time.Second,
			MaxRetries:         3,
			RetryDelay:         1 * time.Second,
			TokenTransferWeight:     15, // 15% cross-partition transfers
			DataWriteWeight:        15, // 15% data writes
			AccountCreateWeight:    15, // 15% ADI creation
			TokenSendWeight:        10, // 10% simple sends
			DataCollectWeight:      10, // 10% data collection
			TokenIssueWeight:       10, // 10% token issuing
			IssuedTokenMoveWeight:  10, // 10% issued token moves
			TokenAccountCreateWeight: 8, // 8% ADI token account creation
			DataAccountCreateWeight:  7, // 7% ADI data account creation
		}
	}
	
	return &ACMESpender{
		client:    c,
		serverURL: serverURL,
		stopChan:  make(chan struct{}),
		config:    *config,
		stats: SpenderStats{
			StartTime: time.Now(),
		},
	}, nil
}

// Start begins the ACME spending operations with funded accounts
func (s *ACMESpender) Start(ctx context.Context, spenderAccounts []*FundedAccount) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("spender is already running")
	}
	s.running = true
	s.spenderAccounts = spenderAccounts
	s.mu.Unlock()
	
	if len(spenderAccounts) == 0 {
		return fmt.Errorf("need at least one funded account to start spending")
	}
	
	log.Printf("💸 Starting ACME spender with %d funded accounts and %d workers", 
		len(spenderAccounts), s.config.WorkerCount)
	
	// Start spending workers
	for i := 0; i < s.config.WorkerCount; i++ {
		s.wg.Add(1)
		go s.spendingWorker(ctx, i)
	}
	
	return nil
}

// Stop gracefully shuts down the ACME spender
func (s *ACMESpender) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()
	
	close(s.stopChan)
	s.wg.Wait()
	
	stats := s.GetStats()
	log.Printf("🛑 ACME spender stopped after %d transactions (%.2f ACME spent)", 
		stats.TotalTransactions, float64(stats.TotalACMESpent)/1000000)
}

// GetStats returns current spending statistics
func (s *ACMESpender) GetStats() SpenderStats {
	s.stats.mu.RLock()
	defer s.stats.mu.RUnlock()
	return s.stats
}

// PrintStats displays current spending statistics
func (s *ACMESpender) PrintStats() {
	stats := s.GetStats()
	elapsed := time.Since(stats.StartTime)
	
	fmt.Printf("\n💸 ACME Spender Statistics (Running: %v)\n", elapsed.Round(time.Second))
	fmt.Printf("📊 Total Transactions: %d (Success: %d, Failed: %d)\n", 
		stats.TotalTransactions, stats.SuccessfulTxs, stats.FailedTxs)
	
	if stats.TotalTransactions > 0 {
		successRate := float64(stats.SuccessfulTxs) / float64(stats.TotalTransactions) * 100
		fmt.Printf("✅ Success Rate: %.1f%%\n", successRate)
	}
	
	fmt.Printf("💰 ACME Spent: %.2f ACME (Fees: %.2f ACME)\n", 
		float64(stats.TotalACMESpent)/1000000, float64(stats.FeesLaid)/1000000)
	
	fmt.Printf("🔄 Transaction Types:\n")
	fmt.Printf("  💸 Token Transfers: %d\n", stats.TokenTransfers)
	fmt.Printf("  📝 Data Writes: %d\n", stats.DataWrites) 
	fmt.Printf("  🆕 ADI Creates: %d\n", stats.AccountCreations)
	fmt.Printf("  📤 Token Sends: %d\n", stats.TokenSends)
	fmt.Printf("  📊 Data Collections: %d\n", stats.DataCollections)
	fmt.Printf("  🪙 Token Issues: %d\n", stats.TokenIssues)
	fmt.Printf("  🔄 Issued Token Moves: %d\n", stats.IssuedTokenMoves)
	fmt.Printf("  💳 Token Account Creates: %d\n", stats.TokenAccountCreates)
	fmt.Printf("  📂 Data Account Creates: %d\n", stats.DataAccountCreates)
	
	if elapsed.Seconds() > 0 {
		tps := float64(stats.TotalTransactions) / elapsed.Seconds()
		aps := float64(stats.TotalACMESpent) / 1000000 / elapsed.Seconds()
		fmt.Printf("⚡ Rates: %.2f tx/s, %.2f ACME/s\n", tps, aps)
	}
	fmt.Println()
}

// spendingWorker runs continuous transaction generation
func (s *ACMESpender) spendingWorker(ctx context.Context, workerID int) {
	defer s.wg.Done()
	
	ticker := time.NewTicker(s.config.TransactionInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-s.stopChan:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			result := s.generateRandomTransaction(workerID)
			s.recordTransactionResult(result)
			
			if result.Success {
				log.Printf("Worker %d: %s ✅ (%.2f ACME, %v)", 
					workerID, result.TxType, float64(result.ACMESpent)/1000000, result.Duration.Round(time.Millisecond))
			} else {
				log.Printf("Worker %d: %s ❌ (%v)", 
					workerID, result.TxType, result.Error)
			}
		}
	}
}

// generateRandomTransaction creates a random transaction based on configured weights
func (s *ACMESpender) generateRandomTransaction(workerID int) TransactionResult {
	start := time.Now()
	
	// Calculate total weight for all transaction types
	totalWeight := s.config.TokenTransferWeight + s.config.DataWriteWeight + 
	               s.config.AccountCreateWeight + s.config.TokenSendWeight +
	               s.config.DataCollectWeight + s.config.TokenIssueWeight +
	               s.config.IssuedTokenMoveWeight + s.config.TokenAccountCreateWeight +
	               s.config.DataAccountCreateWeight
	
	if totalWeight == 0 {
		return TransactionResult{
			TxType: "unknown",
			Success: false,
			Error: fmt.Errorf("no transaction types configured"),
			Duration: time.Since(start),
		}
	}
	
	randNum := int(time.Now().UnixNano()) % totalWeight
	
	var txType string
	var acmeSpent int64
	var err error
	
	// Cumulative weight selection
	cumulative := 0
	
	switch {
	case randNum < cumulative + s.config.TokenTransferWeight:
		txType = "crosschain-transfer"
		acmeSpent, err = s.performCrosschainTransfer(workerID)
		
	case randNum < (cumulative + s.config.TokenTransferWeight + s.config.DataWriteWeight):
		txType = "data-write"
		acmeSpent, err = s.performDataWrite(workerID)
		
	case randNum < (cumulative + s.config.TokenTransferWeight + s.config.DataWriteWeight + s.config.AccountCreateWeight):
		txType = "adi-create"
		acmeSpent, err = s.performADICreation(workerID)
		
	case randNum < (cumulative + s.config.TokenTransferWeight + s.config.DataWriteWeight + s.config.AccountCreateWeight + s.config.TokenSendWeight):
		txType = "token-send"
		acmeSpent, err = s.performTokenSend(workerID)
		
	case randNum < (cumulative + s.config.TokenTransferWeight + s.config.DataWriteWeight + s.config.AccountCreateWeight + s.config.TokenSendWeight + s.config.DataCollectWeight):
		txType = "data-collect"
		acmeSpent, err = s.performDataCollection(workerID)
		
	case randNum < (cumulative + s.config.TokenTransferWeight + s.config.DataWriteWeight + s.config.AccountCreateWeight + s.config.TokenSendWeight + s.config.DataCollectWeight + s.config.TokenIssueWeight):
		txType = "token-issue"
		acmeSpent, err = s.performTokenIssue(workerID)
		
	case randNum < (cumulative + s.config.TokenTransferWeight + s.config.DataWriteWeight + s.config.AccountCreateWeight + s.config.TokenSendWeight + s.config.DataCollectWeight + s.config.TokenIssueWeight + s.config.IssuedTokenMoveWeight):
		txType = "issued-token-move"
		acmeSpent, err = s.performIssuedTokenMove(workerID)
		
	case randNum < (cumulative + s.config.TokenTransferWeight + s.config.DataWriteWeight + s.config.AccountCreateWeight + s.config.TokenSendWeight + s.config.DataCollectWeight + s.config.TokenIssueWeight + s.config.IssuedTokenMoveWeight + s.config.TokenAccountCreateWeight):
		txType = "token-account-create"
		acmeSpent, err = s.performTokenAccountCreation(workerID)
		
	default:
		txType = "data-account-create"
		acmeSpent, err = s.performDataAccountCreation(workerID)
	}
	
	return TransactionResult{
		TxType:    txType,
		Success:   err == nil,
		ACMESpent: acmeSpent,
		Error:     err,
		Duration:  time.Since(start),
	}
}

// performCrosschainTransfer creates a cross-partition token transfer
func (s *ACMESpender) performCrosschainTransfer(workerID int) (int64, error) {
	if len(s.spenderAccounts) < 2 {
		return 0, fmt.Errorf("need at least 2 accounts for crosschain transfer")
	}
	
	// Select source and destination accounts from different partitions (simulated)
	sourceAccount := s.spenderAccounts[workerID%len(s.spenderAccounts)]
	destAccount := s.spenderAccounts[(workerID+1)%len(s.spenderAccounts)]
	
	transferAmount := int64(1000000) // 1 ACME
	
	// Build transaction using build package
	var ts uint64
	env, err := build.Transaction().For(sourceAccount.URL).
		SendTokens(transferAmount, 0).To(destAccount.URL).
		SignWith(sourceAccount.URL).Version(1).Timestamp(&ts).PrivateKey(sourceAccount.PrivateKey).
		Done()
	if err != nil {
		return 0, fmt.Errorf("failed to build transaction: %v", err)
	}
	
	// Submit transaction
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	subs, err := s.client.Submit(ctx, env, v3api.SubmitOptions{})
	if err != nil {
		return 0, fmt.Errorf("crosschain transfer failed: %v", err)
	}
	
	for _, sub := range subs {
		if err := sub.Status.AsError(); err != nil {
			return 0, fmt.Errorf("crosschain transfer failed: %v", err)
		}
	}
	
	// Estimate spent amount (transfer + fees)
	return transferAmount + 10000, nil // 1 ACME + ~0.01 ACME fees
}

// performDataWrite writes data to a data account
func (s *ACMESpender) performDataWrite(workerID int) (int64, error) {
	sourceAccount := s.spenderAccounts[workerID%len(s.spenderAccounts)]
	
	// Generate test data
	testData := fmt.Sprintf("Load test data from worker %d at %s - Random: %d", 
		workerID, time.Now().Format(time.RFC3339), time.Now().UnixNano())
	
	// Build transaction using build package (write to scratch space)
	var ts uint64
	env, err := build.Transaction().For(sourceAccount.URL).
		WriteData().DoubleHash([]byte(testData)).Scratch().
		SignWith(sourceAccount.URL).Version(1).Timestamp(&ts).PrivateKey(sourceAccount.PrivateKey).
		Done()
	if err != nil {
		return 0, fmt.Errorf("failed to build transaction: %v", err)
	}
	
	// Submit transaction  
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	subs, err := s.client.Submit(ctx, env, v3api.SubmitOptions{})
	if err != nil {
		return 0, fmt.Errorf("data write failed: %v", err)
	}
	
	for _, sub := range subs {
		if err := sub.Status.AsError(); err != nil {
			return 0, fmt.Errorf("data write failed: %v", err)
		}
	}
	
	// Estimate spent amount (data storage fees)
	return 5000, nil // ~0.005 ACME for data write
}

// performADICreation creates a new ADI (Accumulate Digital Identifier)
func (s *ACMESpender) performADICreation(workerID int) (int64, error) {
	sourceAccount := s.spenderAccounts[workerID%len(s.spenderAccounts)]
	
	// Generate new identity
	pubKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return 0, fmt.Errorf("failed to generate key pair: %v", err)
	}
	
	// Create unique ADI name (must end with .acme)
	adiName := fmt.Sprintf("test-worker-%d-%d.acme", workerID, time.Now().Unix())
	
	// Build transaction using build package with authority
	var ts uint64
	env, err := build.Transaction().For(sourceAccount.URL).
		CreateIdentity(adiName).WithKey(pubKey, protocol.SignatureTypeED25519).WithAuthority(sourceAccount.URL).
		SignWith(sourceAccount.URL).Version(1).Timestamp(&ts).PrivateKey(sourceAccount.PrivateKey).
		Done()
	if err != nil {
		return 0, fmt.Errorf("failed to build transaction: %v", err)
	}
	
	// Submit transaction
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	subs, err := s.client.Submit(ctx, env, v3api.SubmitOptions{})
	if err != nil {
		return 0, fmt.Errorf("account creation failed: %v", err)
	}
	
	for _, sub := range subs {
		if err := sub.Status.AsError(); err != nil {
			return 0, fmt.Errorf("account creation failed: %v", err)
		}
	}
	
	// ADI creation costs vary by name length (simplified)
	return 500000, nil // ~0.5 ACME for ADI creation
}

// performTokenSend performs a simple token send within the same partition
func (s *ACMESpender) performTokenSend(workerID int) (int64, error) {
	if len(s.spenderAccounts) < 2 {
		return 0, fmt.Errorf("need at least 2 accounts for token send")
	}
	
	sourceAccount := s.spenderAccounts[workerID%len(s.spenderAccounts)]
	destAccount := s.spenderAccounts[(workerID+1)%len(s.spenderAccounts)]
	
	sendAmount := int64(500000) // 0.5 ACME
	
	// Build transaction using build package
	var ts uint64
	env, err := build.Transaction().For(sourceAccount.URL).
		SendTokens(sendAmount, 0).To(destAccount.URL).
		SignWith(sourceAccount.URL).Version(1).Timestamp(&ts).PrivateKey(sourceAccount.PrivateKey).
		Done()
	if err != nil {
		return 0, fmt.Errorf("failed to build transaction: %v", err)
	}
	
	// Submit transaction
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	subs, err := s.client.Submit(ctx, env, v3api.SubmitOptions{})
	if err != nil {
		return 0, fmt.Errorf("token send failed: %v", err)
	}
	
	for _, sub := range subs {
		if err := sub.Status.AsError(); err != nil {
			return 0, fmt.Errorf("token send failed: %v", err)
		}
	}
	
	// Estimate spent amount (send + fees)
	return sendAmount + 5000, nil // 0.5 ACME + ~0.005 ACME fees
}

// performDataCollection queries and collects data from data accounts
func (s *ACMESpender) performDataCollection(workerID int) (int64, error) {
	sourceAccount := s.spenderAccounts[workerID%len(s.spenderAccounts)]
	
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	
	// Query the account data using the API client
	_, err := s.client.Query(ctx, sourceAccount.URL, nil)
	if err != nil {
		return 0, fmt.Errorf("data collection failed: %v", err)
	}
	
	// Data collection is typically free, but we simulate a small processing cost
	return 1000, nil // ~0.001 ACME for data collection processing
}

// performTokenIssue creates a new token issuer (simplified for testing)
func (s *ACMESpender) performTokenIssue(workerID int) (int64, error) {
	// For now, simulate token issuing with a data write transaction
	// This is a simplified implementation for load testing purposes
	return s.performDataWrite(workerID)
}

// performIssuedTokenMove transfers issued tokens between accounts (simplified)
func (s *ACMESpender) performIssuedTokenMove(workerID int) (int64, error) {
	// For now, simulate with a regular token send for testing purposes
	return s.performTokenSend(workerID)
}

// performTokenAccountCreation creates a token account within an ADI (simplified)
func (s *ACMESpender) performTokenAccountCreation(workerID int) (int64, error) {
	// For now, simulate with ADI creation for testing purposes
	return s.performADICreation(workerID)
}

// performDataAccountCreation creates a data account within an ADI (simplified)
func (s *ACMESpender) performDataAccountCreation(workerID int) (int64, error) {
	// For now, simulate with ADI creation for testing purposes
	return s.performADICreation(workerID)
}

// recordTransactionResult updates statistics with transaction result
func (s *ACMESpender) recordTransactionResult(result TransactionResult) {
	s.stats.mu.Lock()
	defer s.stats.mu.Unlock()
	
	s.stats.TotalTransactions++
	s.stats.LastTxTime = time.Now()
	
	if result.Success {
		s.stats.SuccessfulTxs++
		s.stats.TotalACMESpent += result.ACMESpent
		
		// Update transaction type counters
		switch result.TxType {
		case "crosschain-transfer":
			s.stats.TokenTransfers++
		case "data-write":
			s.stats.DataWrites++
		case "adi-create":
			s.stats.AccountCreations++
		case "token-send":
			s.stats.TokenSends++
		case "data-collect":
			s.stats.DataCollections++
		case "token-issue":
			s.stats.TokenIssues++
		case "issued-token-move":
			s.stats.IssuedTokenMoves++
		case "token-account-create":
			s.stats.TokenAccountCreates++
		case "data-account-create":
			s.stats.DataAccountCreates++
		}
	} else {
		s.stats.FailedTxs++
	}
}