package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// EnhancedCrossChainConductor extends the conductor with partition failure handling
type EnhancedCrossChainConductor struct {
	dispatcher      execute.Dispatcher
	logger          logging.OptionalLogger
	healthMonitor   *PartitionHealthMonitor
	
	// Original conductor fields
	destinationQueues map[DestinationKey]*DestinationQueue
	queueMu          sync.RWMutex
	
	// Configuration
	maxRetries       int
	retryDelay       time.Duration
	stopCh           chan struct{}
	wg               sync.WaitGroup
	
	// Metrics
	totalSent        int64
	totalFailed      int64
	totalQueued      int64
	partitionsDown   int32
}

// NewEnhancedCrossChainConductor creates a conductor with partition failure handling
func NewEnhancedCrossChainConductor(dispatcher execute.Dispatcher, logger logging.OptionalLogger) *EnhancedCrossChainConductor {
	healthMonitor := NewPartitionHealthMonitor(logger)
	
	return &EnhancedCrossChainConductor{
		dispatcher:        dispatcher,
		logger:           logger.With("module", "enhanced-conductor"),
		healthMonitor:    healthMonitor,
		destinationQueues: make(map[DestinationKey]*DestinationQueue),
		maxRetries:       3,
		retryDelay:       time.Second,
		stopCh:           make(chan struct{}),
	}
}

// Start begins the enhanced conductor
func (ecc *EnhancedCrossChainConductor) Start(partitions []string) {
	// Start health monitoring
	ecc.healthMonitor.Start(partitions)
	
	// Start processing loops
	ecc.wg.Add(2)
	go ecc.processTransactions()
	go ecc.handlePartitionRecovery()
	
	ecc.logger.Info("Enhanced conductor started",
		"partitions", len(partitions),
		"max_retries", ecc.maxRetries)
}

// Stop gracefully stops the conductor
func (ecc *EnhancedCrossChainConductor) Stop() {
	close(ecc.stopCh)
	ecc.wg.Wait()
	
	ecc.logger.Info("Enhanced conductor stopped",
		"total_sent", ecc.totalSent,
		"total_failed", ecc.totalFailed,
		"total_queued", ecc.totalQueued)
}

// SubmitTransaction submits a transaction with partition failure handling
func (ecc *EnhancedCrossChainConductor) SubmitTransaction(ctx context.Context, msg messaging.Message, dest *url.URL, seqNum uint64) error {
	partitionID := getPartitionID(dest)
	
	// Check if partition is healthy
	canSend, err := ecc.healthMonitor.CanSendToPartition(partitionID)
	if err != nil {
		return errors.InternalError.Wrap(err)
	}
	
	if !canSend {
		// Partition is down, queue the transaction
		ecc.logger.Info("Partition is down, queuing transaction",
			"partition", partitionID,
			"destination", dest.String(),
			"sequence", seqNum)
		
		tx := &PendingTransaction{
			ID:          fmt.Sprintf("%s-%d", dest.String(), seqNum),
			Type:        getMessageType(msg),
			Message:     msg,
			Destination: dest,
			SequenceNum: seqNum,
			Timestamp:   time.Now(),
		}
		
		err = ecc.healthMonitor.QueueTransaction(partitionID, tx)
		if err != nil {
			ecc.totalFailed++
			return errors.Unavailable.Wrap(err)
		}
		
		ecc.totalQueued++
		return nil // Transaction queued, not failed
	}
	
	// Try to send the transaction
	err = ecc.sendWithRetry(ctx, msg, dest, seqNum, partitionID)
	if err != nil {
		// Record failure and potentially queue
		ecc.healthMonitor.RecordFailure(partitionID, err)
		
		// Check if we should queue for later
		if isRetryableError(err) {
			tx := &PendingTransaction{
				ID:          fmt.Sprintf("%s-%d", dest.String(), seqNum),
				Type:        getMessageType(msg),
				Message:     msg,
				Destination: dest,
				SequenceNum: seqNum,
				Timestamp:   time.Now(),
			}
			
			queueErr := ecc.healthMonitor.QueueTransaction(partitionID, tx)
			if queueErr != nil {
				ecc.totalFailed++
				return errors.Unavailable.Wrap(queueErr)
			}
			
			ecc.totalQueued++
			return nil // Queued for retry
		}
		
		ecc.totalFailed++
		return err
	}
	
	// Success
	ecc.healthMonitor.RecordSuccess(partitionID, seqNum)
	ecc.totalSent++
	return nil
}

// sendWithRetry attempts to send with retry logic
func (ecc *EnhancedCrossChainConductor) sendWithRetry(ctx context.Context, msg messaging.Message, dest *url.URL, seqNum uint64, partitionID string) error {
	env := &messaging.Envelope{Messages: []messaging.Message{msg}}
	
	for attempt := 0; attempt < ecc.maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			delay := time.Duration(attempt) * ecc.retryDelay
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		
		// Check if we can still send (circuit breaker might have opened)
		canSend, err := ecc.healthMonitor.CanSendToPartition(partitionID)
		if !canSend {
			return errors.Unavailable.WithFormat("partition %s became unavailable", partitionID)
		}
		
		// Attempt to send
		err = ecc.dispatcher.Submit(ctx, dest, env)
		if err == nil {
			return nil // Success
		}
		
		// Check if error is retryable
		if !isRetryableError(err) {
			return err // Non-retryable error
		}
		
		ecc.logger.Debug("Retrying transaction",
			"attempt", attempt+1,
			"destination", dest.String(),
			"error", err)
	}
	
	return errors.MaxRetries.WithFormat("failed after %d attempts", ecc.maxRetries)
}

// HandleOutOfOrderSequence handles when a partition sends an out-of-order sequence
func (ecc *EnhancedCrossChainConductor) HandleOutOfOrderSequence(source string, receivedSeq uint64, expectedSeq uint64) error {
	ecc.logger.Info("Out-of-order sequence detected",
		"source", source,
		"received", receivedSeq,
		"expected", expectedSeq)
	
	if receivedSeq < expectedSeq {
		// Partition is behind - it might have been down
		// Request missing transactions from it
		missing := expectedSeq - receivedSeq
		ecc.logger.Info("Partition is behind, needs catch-up",
			"source", source,
			"missing_count", missing)
		
		// Get pending transactions for this partition
		pending, err := ecc.healthMonitor.HandleOutOfOrderRequest(source, receivedSeq)
		if err != nil {
			return err
		}
		
		// Re-send the pending transactions
		go ecc.resendPendingTransactions(pending)
		
		return nil
	}
	
	// Partition is ahead - we might have missed something
	// This triggers our recovery system to request missing transactions
	ecc.logger.Warn("Partition is ahead, we may have missed transactions",
		"source", source,
		"gap", receivedSeq-expectedSeq)
	
	// Trigger recovery for our missing transactions
	// This would integrate with the RecoveryManager
	
	return nil
}

// processTransactions main processing loop
func (ecc *EnhancedCrossChainConductor) processTransactions() {
	defer ecc.wg.Done()
	
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ecc.stopCh:
			return
			
		case <-ticker.C:
			// Process queues for healthy partitions
			ecc.processHealthyPartitions()
		}
	}
}

// processHealthyPartitions processes transactions for healthy partitions
func (ecc *EnhancedCrossChainConductor) processHealthyPartitions() {
	statuses := ecc.healthMonitor.GetAllPartitionStatuses()
	
	for partitionID, status := range statuses {
		if status.State == PartitionHealthy || status.State == PartitionRecovering {
			// Process any pending transactions for this partition
			go ecc.processPendingForPartition(partitionID)
		}
	}
}

// processPendingForPartition processes pending transactions for a partition
func (ecc *EnhancedCrossChainConductor) processPendingForPartition(partitionID string) {
	status, err := ecc.healthMonitor.GetPartitionStatus(partitionID)
	if err != nil || len(status.PendingQueue) == 0 {
		return
	}
	
	ecc.logger.Debug("Processing pending transactions",
		"partition", partitionID,
		"count", len(status.PendingQueue))
	
	// Process pending transactions
	for _, tx := range status.PendingQueue {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := ecc.sendWithRetry(ctx, tx.Message, tx.Destination, tx.SequenceNum, partitionID)
		cancel()
		
		if err != nil {
			ecc.logger.Warn("Failed to send pending transaction",
				"partition", partitionID,
				"transaction", tx.ID,
				"error", err)
			break // Stop processing for this partition
		}
	}
}

// handlePartitionRecovery handles partition recovery events
func (ecc *EnhancedCrossChainConductor) handlePartitionRecovery() {
	defer ecc.wg.Done()
	
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ecc.stopCh:
			return
			
		case <-ticker.C:
			// Check for recovered partitions
			ecc.checkRecoveredPartitions()
		}
	}
}

// checkRecoveredPartitions checks for partitions that have recovered
func (ecc *EnhancedCrossChainConductor) checkRecoveredPartitions() {
	statuses := ecc.healthMonitor.GetAllPartitionStatuses()
	
	for partitionID, status := range statuses {
		if status.State == PartitionRecovering && status.CircuitState == CircuitClosed {
			ecc.logger.Info("Partition has recovered, draining queue",
				"partition", partitionID,
				"pending", len(status.PendingQueue))
			
			// Drain the pending queue for this partition
			go ecc.drainPartitionQueue(partitionID)
		}
	}
}

// drainPartitionQueue drains all pending transactions for a partition
func (ecc *EnhancedCrossChainConductor) drainPartitionQueue(partitionID string) {
	status, err := ecc.healthMonitor.GetPartitionStatus(partitionID)
	if err != nil {
		return
	}
	
	ecc.logger.Info("Draining partition queue",
		"partition", partitionID,
		"queue_size", len(status.PendingQueue))
	
	successCount := 0
	failCount := 0
	
	for _, tx := range status.PendingQueue {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := ecc.sendWithRetry(ctx, tx.Message, tx.Destination, tx.SequenceNum, partitionID)
		cancel()
		
		if err != nil {
			failCount++
			ecc.logger.Warn("Failed to drain transaction",
				"partition", partitionID,
				"transaction", tx.ID,
				"error", err)
		} else {
			successCount++
		}
	}
	
	ecc.logger.Info("Queue drain complete",
		"partition", partitionID,
		"success", successCount,
		"failed", failCount)
}

// resendPendingTransactions resends a batch of pending transactions
func (ecc *EnhancedCrossChainConductor) resendPendingTransactions(transactions []*PendingTransaction) {
	ecc.logger.Info("Resending pending transactions", "count", len(transactions))
	
	for _, tx := range transactions {
		partitionID := getPartitionID(tx.Destination)
		
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := ecc.sendWithRetry(ctx, tx.Message, tx.Destination, tx.SequenceNum, partitionID)
		cancel()
		
		if err != nil {
			ecc.logger.Warn("Failed to resend transaction",
				"transaction", tx.ID,
				"error", err)
			// Transaction will remain in queue for next attempt
		}
	}
}

// GetMetrics returns current metrics
func (ecc *EnhancedCrossChainConductor) GetMetrics() map[string]interface{} {
	statuses := ecc.healthMonitor.GetAllPartitionStatuses()
	
	healthyCount := 0
	downCount := 0
	totalPending := 0
	
	for _, status := range statuses {
		if status.State == PartitionHealthy {
			healthyCount++
		} else if status.State == PartitionDown {
			downCount++
		}
		totalPending += len(status.PendingQueue)
	}
	
	return map[string]interface{}{
		"total_sent":        ecc.totalSent,
		"total_failed":      ecc.totalFailed,
		"total_queued":      ecc.totalQueued,
		"partitions_healthy": healthyCount,
		"partitions_down":   downCount,
		"total_pending":     totalPending,
	}
}

// Helper functions

func getPartitionID(dest *url.URL) string {
	// Extract partition ID from URL
	// This is simplified - real implementation would parse properly
	if dest.Authority != "" {
		return dest.Authority
	}
	return "unknown"
}

func getMessageType(msg messaging.Message) MessageType {
	switch msg.Type() {
	case messaging.MessageTypeBlockAnchor:
		return MessageTypeAnchor
	case messaging.MessageTypeSynthetic:
		return MessageTypeSynthetic
	default:
		return MessageTypeUnknown
	}
}

func isRetryableError(err error) bool {
	// Check if error is retryable
	if errors.Is(err, errors.Timeout) ||
		errors.Is(err, errors.NetworkError) ||
		errors.Is(err, errors.Unavailable) {
		return true
	}
	
	// Check for specific error strings
	errStr := err.Error()
	retryableStrings := []string{
		"connection refused",
		"connection reset",
		"timeout",
		"temporarily unavailable",
	}
	
	for _, s := range retryableStrings {
		if contains(errStr, s) {
			return true
		}
	}
	
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr
}