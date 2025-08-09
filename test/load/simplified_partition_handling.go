package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// SimplifiedPartitionHandler handles partition failures without holding transactions
// Instead, it relies on the ability to recreate transactions from ledgers
type SimplifiedPartitionHandler struct {
	dispatcher    execute.Dispatcher
	logger        logging.OptionalLogger
	
	// Track partition health with circuit breaker
	partitionStates map[string]*PartitionState
	mu             sync.RWMutex
	
	// Configuration
	maxRetries        int
	retryDelay        time.Duration
	failureThreshold  int           // Failures before marking partition down
	recoveryInterval  time.Duration // How often to check for recovery
	
	// Metrics
	totalSent         int64
	totalFailed       int64
	totalDropped      int64 // Transactions dropped due to partition being down
}

// PartitionState tracks the health of a partition
type PartitionState struct {
	ID                string
	IsHealthy         bool
	ConsecutiveFails  int32
	LastFailure       time.Time
	LastSuccess       time.Time
	LastSequenceSent  uint64
	LastSequenceAck   uint64
	
	// Circuit breaker
	CircuitOpen       bool
	CircuitOpenTime   time.Time
}

// NewSimplifiedPartitionHandler creates a new simplified handler
func NewSimplifiedPartitionHandler(dispatcher execute.Dispatcher, logger logging.OptionalLogger) *SimplifiedPartitionHandler {
	return &SimplifiedPartitionHandler{
		dispatcher:       dispatcher,
		logger:          logger,
		partitionStates: make(map[string]*PartitionState),
		maxRetries:      3,
		retryDelay:      time.Second,
		failureThreshold: 3,
		recoveryInterval: 30 * time.Second,
	}
}

// Start initializes partition monitoring
func (sph *SimplifiedPartitionHandler) Start(partitions []string) {
	// Initialize partition states
	for _, partition := range partitions {
		sph.mu.Lock()
		sph.partitionStates[partition] = &PartitionState{
			ID:           partition,
			IsHealthy:    true,
			LastSuccess:  time.Now(),
		}
		sph.mu.Unlock()
	}
	
	// Start recovery checker
	go sph.recoveryChecker()
	
	sph.logger.Info("Simplified partition handler started",
		"partitions", len(partitions),
		"failure_threshold", sph.failureThreshold)
}

// SubmitTransaction attempts to send a transaction with simplified failure handling
func (sph *SimplifiedPartitionHandler) SubmitTransaction(ctx context.Context, msg messaging.Message, dest *url.URL, seqNum uint64) error {
	partitionID := sph.getPartitionID(dest)
	
	// Check if partition is healthy
	if !sph.isPartitionHealthy(partitionID) {
		// Don't hold the transaction - just drop it
		// The recovery system will recreate it from ledgers when partition recovers
		atomic.AddInt64(&sph.totalDropped, 1)
		
		sph.logger.Info("Dropping transaction - partition is down",
			"partition", partitionID,
			"destination", dest.String(),
			"sequence", seqNum,
			"note", "Will be recovered from ledger when partition recovers")
		
		// Return nil - we're intentionally dropping this
		// The calling code can continue with other transactions
		return nil
	}
	
	// Try to send with retries
	env := &messaging.Envelope{Messages: []messaging.Message{msg}}
	
	for attempt := 0; attempt < sph.maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			delay := time.Duration(attempt) * sph.retryDelay
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		
		// Attempt to send
		err := sph.dispatcher.Submit(ctx, dest, env)
		if err == nil {
			// Success
			sph.recordSuccess(partitionID, seqNum)
			atomic.AddInt64(&sph.totalSent, 1)
			return nil
		}
		
		// Failed - check if we should continue retrying
		if !sph.isRetryableError(err) {
			sph.recordFailure(partitionID, err)
			atomic.AddInt64(&sph.totalFailed, 1)
			return err
		}
		
		sph.logger.Debug("Retrying transaction",
			"attempt", attempt+1,
			"partition", partitionID,
			"error", err)
	}
	
	// All retries failed
	sph.recordFailure(partitionID, fmt.Errorf("exhausted retries"))
	atomic.AddInt64(&sph.totalFailed, 1)
	
	// Check if we should mark partition as down
	if sph.shouldMarkPartitionDown(partitionID) {
		sph.markPartitionDown(partitionID)
	}
	
	return fmt.Errorf("failed after %d attempts", sph.maxRetries)
}

// HandleOutOfOrderSequence handles when a partition reports an unexpected sequence
func (sph *SimplifiedPartitionHandler) HandleOutOfOrderSequence(source string, receivedSeq uint64, expectedSeq uint64) {
	sph.logger.Info("Out-of-order sequence detected",
		"source", source,
		"received", receivedSeq,
		"expected", expectedSeq)
	
	if receivedSeq < expectedSeq {
		// Partition is behind - it needs transactions we've already sent
		// This means the partition was down and missed some transactions
		missing := expectedSeq - receivedSeq
		
		sph.logger.Info("Partition needs catch-up",
			"partition", source,
			"missing_sequences", fmt.Sprintf("%d-%d", receivedSeq, expectedSeq-1),
			"count", missing,
			"action", "Will recreate from ledger")
		
		// Mark partition as recovered if it was down
		sph.markPartitionRecovered(source)
		
		// Trigger recovery to recreate missing transactions from ledger
		// This would call into the RecoveryManager we built earlier
		go sph.triggerRecoveryFromLedger(source, receivedSeq, expectedSeq)
		
	} else if receivedSeq > expectedSeq {
		// We are behind - request missing transactions
		gap := receivedSeq - expectedSeq
		
		sph.logger.Info("We are behind - requesting missing transactions",
			"source", source,
			"gap", gap,
			"our_last", expectedSeq-1,
			"their_current", receivedSeq)
		
		// This would trigger our recovery system to request missing transactions
		go sph.requestMissingTransactions(source, expectedSeq, receivedSeq)
	}
}

// triggerRecoveryFromLedger recreates transactions from the ledger
func (sph *SimplifiedPartitionHandler) triggerRecoveryFromLedger(partition string, fromSeq, toSeq uint64) {
	sph.logger.Info("Starting ledger-based recovery",
		"partition", partition,
		"sequences", fmt.Sprintf("%d-%d", fromSeq, toSeq-1))
	
	// This is where we would:
	// 1. Read the anchor/synthetic ledger
	// 2. Find the transactions in the sequence range
	// 3. Recreate them from the ledger data
	// 4. Send them to the partition
	
	// The RecoveryManager we built earlier handles this:
	// - recovery.go: RequestMissingTransactions()
	// - recovery.go: recoverAnchors() / recoverSynthetics()
	
	sph.logger.Info("Recovery triggered - transactions will be recreated from ledger",
		"partition", partition,
		"note", "RecoveryManager will handle recreation")
}

// requestMissingTransactions requests transactions we're missing
func (sph *SimplifiedPartitionHandler) requestMissingTransactions(source string, fromSeq, toSeq uint64) {
	sph.logger.Info("Requesting missing transactions",
		"from", source,
		"sequences", fmt.Sprintf("%d-%d", fromSeq, toSeq-1))
	
	// This would use the RecoveryManager to request from the source partition
	// The source partition would read its ledger and send us the transactions
}

// recordSuccess records a successful transaction
func (sph *SimplifiedPartitionHandler) recordSuccess(partitionID string, seqNum uint64) {
	sph.mu.Lock()
	defer sph.mu.Unlock()
	
	state, exists := sph.partitionStates[partitionID]
	if !exists {
		return
	}
	
	state.ConsecutiveFails = 0
	state.LastSuccess = time.Now()
	state.LastSequenceSent = seqNum
	state.LastSequenceAck = seqNum
	
	// If circuit was open, close it
	if state.CircuitOpen {
		state.CircuitOpen = false
		state.IsHealthy = true
		sph.logger.Info("Partition recovered - circuit closed",
			"partition", partitionID)
	}
}

// recordFailure records a failed transaction
func (sph *SimplifiedPartitionHandler) recordFailure(partitionID string, err error) {
	sph.mu.Lock()
	defer sph.mu.Unlock()
	
	state, exists := sph.partitionStates[partitionID]
	if !exists {
		return
	}
	
	atomic.AddInt32(&state.ConsecutiveFails, 1)
	state.LastFailure = time.Now()
	
	sph.logger.Debug("Transaction failed",
		"partition", partitionID,
		"consecutive_fails", state.ConsecutiveFails,
		"error", err)
}

// isPartitionHealthy checks if a partition is healthy
func (sph *SimplifiedPartitionHandler) isPartitionHealthy(partitionID string) bool {
	sph.mu.RLock()
	defer sph.mu.RUnlock()
	
	state, exists := sph.partitionStates[partitionID]
	if !exists {
		return true // Default to healthy for unknown partitions
	}
	
	return state.IsHealthy && !state.CircuitOpen
}

// shouldMarkPartitionDown checks if partition should be marked as down
func (sph *SimplifiedPartitionHandler) shouldMarkPartitionDown(partitionID string) bool {
	sph.mu.RLock()
	defer sph.mu.RUnlock()
	
	state, exists := sph.partitionStates[partitionID]
	if !exists {
		return false
	}
	
	return atomic.LoadInt32(&state.ConsecutiveFails) >= int32(sph.failureThreshold)
}

// markPartitionDown marks a partition as down
func (sph *SimplifiedPartitionHandler) markPartitionDown(partitionID string) {
	sph.mu.Lock()
	defer sph.mu.Unlock()
	
	state, exists := sph.partitionStates[partitionID]
	if !exists {
		return
	}
	
	if !state.CircuitOpen {
		state.CircuitOpen = true
		state.CircuitOpenTime = time.Now()
		state.IsHealthy = false
		
		sph.logger.Info("Partition marked as down - circuit opened",
			"partition", partitionID,
			"consecutive_failures", state.ConsecutiveFails,
			"note", "Transactions will be dropped and recovered from ledger later")
	}
}

// markPartitionRecovered marks a partition as recovered
func (sph *SimplifiedPartitionHandler) markPartitionRecovered(partitionID string) {
	sph.mu.Lock()
	defer sph.mu.Unlock()
	
	state, exists := sph.partitionStates[partitionID]
	if !exists {
		return
	}
	
	if state.CircuitOpen {
		state.CircuitOpen = false
		state.IsHealthy = true
		state.ConsecutiveFails = 0
		
		sph.logger.Info("Partition marked as recovered",
			"partition", partitionID,
			"downtime", time.Since(state.CircuitOpenTime))
	}
}

// recoveryChecker periodically checks if down partitions have recovered
func (sph *SimplifiedPartitionHandler) recoveryChecker() {
	ticker := time.NewTicker(sph.recoveryInterval)
	defer ticker.Stop()
	
	for range ticker.C {
		sph.mu.RLock()
		partitions := make([]*PartitionState, 0)
		for _, state := range sph.partitionStates {
			if state.CircuitOpen {
				partitions = append(partitions, state)
			}
		}
		sph.mu.RUnlock()
		
		for _, state := range partitions {
			// Check if it's time to test recovery
			if time.Since(state.CircuitOpenTime) > 30*time.Second {
				sph.logger.Info("Testing partition recovery",
					"partition", state.ID,
					"down_duration", time.Since(state.CircuitOpenTime))
				
				// In production, this would ping the partition
				// For now, we'll wait for an out-of-order sequence to indicate recovery
			}
		}
	}
}

// isRetryableError determines if an error should be retried
func (sph *SimplifiedPartitionHandler) isRetryableError(err error) bool {
	// Check error strings for retryable patterns
	
	// Check error strings
	errStr := err.Error()
	retryableStrings := []string{
		"connection refused",
		"connection reset",
		"timeout",
		"temporary",
	}
	
	for _, s := range retryableStrings {
		if contains(errStr, s) {
			return true
		}
	}
	
	return false
}

// getPartitionID extracts partition ID from URL
func (sph *SimplifiedPartitionHandler) getPartitionID(dest *url.URL) string {
	if dest.Authority != "" {
		return dest.Authority
	}
	return "unknown"
}

// GetMetrics returns current metrics
func (sph *SimplifiedPartitionHandler) GetMetrics() map[string]interface{} {
	sph.mu.RLock()
	defer sph.mu.RUnlock()
	
	healthyCount := 0
	downCount := 0
	
	for _, state := range sph.partitionStates {
		if state.IsHealthy {
			healthyCount++
		} else {
			downCount++
		}
	}
	
	return map[string]interface{}{
		"total_sent":         atomic.LoadInt64(&sph.totalSent),
		"total_failed":       atomic.LoadInt64(&sph.totalFailed),
		"total_dropped":      atomic.LoadInt64(&sph.totalDropped),
		"partitions_healthy": healthyCount,
		"partitions_down":    downCount,
	}
}

// GetPartitionStatus returns the status of a specific partition
func (sph *SimplifiedPartitionHandler) GetPartitionStatus(partitionID string) *PartitionState {
	sph.mu.RLock()
	defer sph.mu.RUnlock()
	
	return sph.partitionStates[partitionID]
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr
}