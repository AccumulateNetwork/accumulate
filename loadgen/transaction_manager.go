package loadgen

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/loadgen/wallet"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// TransactionType represents the different types of transactions
type TransactionType string

const (
	// Infrastructure (30%)
	TxCreateADI          TransactionType = "create_adi"
	TxCreateKeyBook      TransactionType = "create_key_book"
	TxCreateKeyPage      TransactionType = "create_key_page"
	TxUpdateKeyPage      TransactionType = "update_key_page"
	TxCreateTokenAccount TransactionType = "create_token_account"
	TxCreateDataAccount  TransactionType = "create_data_account"
	TxCreateLiteAccount  TransactionType = "create_lite_account"
	TxAddCredits         TransactionType = "add_credits"

	// Value Transfer (50%)
	TxSendTokensADI   TransactionType = "send_tokens_adi"
	TxSendTokensLite  TransactionType = "send_tokens_lite"
	TxSendTokensMixed TransactionType = "send_tokens_mixed"
	TxBurnTokens      TransactionType = "burn_tokens"
	TxLockAccount     TransactionType = "lock_account"

	// Data Operations (15%)
	TxWriteData       TransactionType = "write_data"
	TxWriteDataToLite TransactionType = "write_data_to_lite"
	TxScratchData     TransactionType = "scratch_data"

	// Token Issuance (5%)
	TxCreateToken       TransactionType = "create_token"
	TxIssueTokens       TransactionType = "issue_tokens"
	TxUpdateTokenIssuer TransactionType = "update_token_issuer"
)

// LoadProfile defines the transaction distribution
type LoadProfile string

const (
	ProfileSetup       LoadProfile = "setup"
	ProfileSteadyState LoadProfile = "steady_state"
	ProfileStressTest  LoadProfile = "stress_test"
	ProfileTokenEconomy LoadProfile = "token_economy"
)

// TransactionRequest represents a transaction to be executed
type TransactionRequest struct {
	ID        string
	Type      TransactionType
	Principal *url.URL
	Target    *url.URL
	Payload   interface{}
	CreatedAt time.Time
}

// TransactionResult represents the outcome of a transaction
type TransactionResult struct {
	Request   *TransactionRequest
	Success   bool
	TxID      []byte
	Error     error
	Latency   time.Duration
}

// TransactionMetrics tracks transaction statistics
type TransactionMetrics struct {
	mu        sync.RWMutex
	Attempted map[TransactionType]uint64
	Succeeded map[TransactionType]uint64
	Failed    map[TransactionType]uint64
	StartTime time.Time
}

// TransactionManager orchestrates transaction generation and execution
type TransactionManager struct {
	wallet   *wallet.Wallet
	executor *TransactionExecutor
	profile  LoadProfile
	metrics  *TransactionMetrics
	
	// Distribution weights for each profile
	distributions map[LoadProfile]map[TransactionType]float64
	
	// Control
	ctx    context.Context
	cancel context.CancelFunc
}

// NewTransactionManager creates a new transaction manager
func NewTransactionManager(w *wallet.Wallet, executor *TransactionExecutor, profile LoadProfile) *TransactionManager {
	ctx, cancel := context.WithCancel(context.Background())
	
	tm := &TransactionManager{
		wallet:   w,
		executor: executor,
		profile:  profile,
		metrics: &TransactionMetrics{
			Attempted: make(map[TransactionType]uint64),
			Succeeded: make(map[TransactionType]uint64),
			Failed:    make(map[TransactionType]uint64),
			StartTime: time.Now(),
		},
		ctx:    ctx,
		cancel: cancel,
	}
	
	tm.initDistributions()
	return tm
}

// initDistributions sets up the transaction type weights for each profile
func (tm *TransactionManager) initDistributions() {
	tm.distributions = map[LoadProfile]map[TransactionType]float64{
		ProfileSetup: {
			TxCreateADI:         0.25,
			TxCreateKeyBook:     0.15,
			TxCreateTokenAccount: 0.20,
			TxCreateLiteAccount: 0.20,
			TxAddCredits:        0.10,
			TxSendTokensLite:    0.10,
		},
		ProfileSteadyState: {
			TxSendTokensADI:     0.25,
			TxSendTokensLite:    0.20,
			TxSendTokensMixed:   0.10,
			TxWriteData:         0.12,
			TxCreateTokenAccount: 0.08,
			TxCreateLiteAccount: 0.08,
			TxAddCredits:        0.05,
			TxWriteDataToLite:   0.05,
			TxScratchData:       0.03,
			TxBurnTokens:        0.02,
			TxLockAccount:       0.02,
		},
		ProfileStressTest: {
			TxSendTokensLite:  0.70,
			TxWriteDataToLite: 0.15,
			TxScratchData:     0.15,
		},
		ProfileTokenEconomy: {
			TxSendTokensADI:   0.30,
			TxSendTokensLite:  0.25,
			TxSendTokensMixed: 0.10,
			TxIssueTokens:     0.10,
			TxBurnTokens:      0.10,
			TxCreateToken:     0.05,
			TxLockAccount:     0.05,
			TxUpdateTokenIssuer: 0.05,
		},
	}
}

// SelectTransactionType randomly selects a transaction type based on distribution
func (tm *TransactionManager) SelectTransactionType() TransactionType {
	dist := tm.distributions[tm.profile]
	r := rand.Float64()
	cumulative := 0.0
	
	for txType, weight := range dist {
		cumulative += weight
		if r <= cumulative {
			return txType
		}
	}
	
	// Return first type as fallback
	for txType := range dist {
		return txType
	}
	return TxSendTokensLite
}

// GenerateAndExecute generates and executes a transaction
func (tm *TransactionManager) GenerateAndExecute(ctx context.Context) (*TransactionResult, error) {
	txType := tm.SelectTransactionType()
	
	req := &TransactionRequest{
		ID:        fmt.Sprintf("%s_%d", txType, time.Now().UnixNano()),
		Type:      txType,
		CreatedAt: time.Now(),
	}
	
	// Record attempt
	tm.metrics.recordAttempt(txType)
	
	// Execute transaction
	result, err := tm.executor.Execute(ctx, req)
	if err != nil {
		tm.metrics.recordFailure(txType)
		return nil, err
	}
	
	// Record result
	if result.Success {
		tm.metrics.recordSuccess(txType)
	} else {
		tm.metrics.recordFailure(txType)
	}
	
	return result, nil
}

// GetMetrics returns current metrics
func (tm *TransactionManager) GetMetrics() map[string]interface{} {
	tm.metrics.mu.RLock()
	defer tm.metrics.mu.RUnlock()
	
	elapsed := time.Since(tm.metrics.StartTime).Seconds()
	var totalAttempted, totalSucceeded uint64
	
	for _, count := range tm.metrics.Attempted {
		totalAttempted += count
	}
	for _, count := range tm.metrics.Succeeded {
		totalSucceeded += count
	}
	
	return map[string]interface{}{
		"elapsed_seconds": elapsed,
		"total_attempted": totalAttempted,
		"total_succeeded": totalSucceeded,
		"tps":            float64(totalSucceeded) / elapsed,
		"by_type":        tm.getTypeMetrics(),
	}
}

// getTypeMetrics returns metrics broken down by transaction type
func (tm *TransactionManager) getTypeMetrics() map[TransactionType]map[string]uint64 {
	result := make(map[TransactionType]map[string]uint64)
	
	for txType, attempted := range tm.metrics.Attempted {
		result[txType] = map[string]uint64{
			"attempted": attempted,
			"succeeded": tm.metrics.Succeeded[txType],
			"failed":    tm.metrics.Failed[txType],
		}
	}
	
	return result
}

// Stop gracefully shuts down the transaction manager
func (tm *TransactionManager) Stop() {
	tm.cancel()
}

// Metrics helper methods
func (m *TransactionMetrics) recordAttempt(txType TransactionType) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Attempted[txType]++
}

func (m *TransactionMetrics) recordSuccess(txType TransactionType) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Succeeded[txType]++
}

func (m *TransactionMetrics) recordFailure(txType TransactionType) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Failed[txType]++
}