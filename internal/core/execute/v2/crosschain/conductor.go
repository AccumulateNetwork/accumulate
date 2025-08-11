// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ConductorMessageType represents the type of cross-partition message
// Note: This is different from the unified transport's MessageType
type ConductorMessageType int

const (
	ConductorMessageTypeAnchor ConductorMessageType = iota
	ConductorMessageTypeSynthetic
	ConductorMessageTypeOther
)

// DestinationKey uniquely identifies a message type + destination combination
type DestinationKey struct {
	Type        ConductorMessageType
	Destination string // URL string for efficient map key
}

// PendingTransmission tracks a transmission awaiting error feedback
type PendingTransmission struct {
	ID          string
	Messages    []messaging.Message
	Destination *url.URL
	DestKey     DestinationKey
	Context     context.Context
	AttemptNum  int
	SubmittedAt time.Time
	RetryAfter  time.Time
	Callback    chan error
}

// DestinationQueue manages transmission state for a specific destination+type combination
type DestinationQueue struct {
	Key            DestinationKey
	IsBlocked      bool
	BlockedSince   time.Time
	PendingTx      map[string]*PendingTransmission
	QueuedRequests []*SyntheticRequest
	LastSuccess    time.Time
	FailureCount   int64
	SuccessCount   int64
	RetryCount     int64
	mu             sync.RWMutex
}

// CrossChainConductor handles async processing of cross-partition transactions
type CrossChainConductor struct {
	// Infrastructure
	dispatcher execute.Dispatcher
	logger     logging.OptionalLogger

	// Async processing
	syntheticChan chan *SyntheticRequest
	retryChan     chan *PendingTransmission
	stopChan      chan struct{}
	wg            sync.WaitGroup

	// Per-destination blocking and tracking
	destinationQueues map[DestinationKey]*DestinationQueue
	queuesMutex       sync.RWMutex
	maxRetries        int
	retryDelay        time.Duration
	txIDCounter       int64

	// Global metrics
	syntheticsSent     int64
	syntheticsErrors   int64
	syntheticsRetried  int64
	transmissionErrors int64

	// Recovery manager for missing transactions
	recoveryManager *RecoveryManager

	// Batch proof recovery manager for efficient collection proofs
	batchProofManager *BatchProofRecoveryManager

	// Centralized proof service for construction and validation
	proofService *ProofService
	
	// Unified transport for all crosschain messages
	unifiedTransport *UnifiedTransport
	
	// Block integration for the block executor
	blockIntegration *BlockIntegration
}

// NewCrossChainConductor creates and starts the conductor
func NewCrossChainConductor(dispatcher execute.Dispatcher, logger logging.OptionalLogger) *CrossChainConductor {
	cc := &CrossChainConductor{
		dispatcher:        dispatcher,
		logger:            logger.With("module", "crosschain-conductor").(logging.OptionalLogger),
		syntheticChan:     make(chan *SyntheticRequest, 100),   // Buffered channel for async processing
		retryChan:         make(chan *PendingTransmission, 50), // Retry queue
		stopChan:          make(chan struct{}),
		destinationQueues: make(map[DestinationKey]*DestinationQueue),
		maxRetries:        3,               // Retry failed transmissions up to 3 times
		retryDelay:        2 * time.Second, // Wait 2 seconds between retries
	}

	// Initialize centralized proof service (NO CACHING for easier testing)
	cc.proofService = NewProofService(logger)
	cc.proofService.SetDebugMode(true) // Enable debug mode for testing
	
	// Initialize unified transport
	cc.unifiedTransport = NewUnifiedTransport(cc.proofService, cc, logger)
	cc.unifiedTransport.SetDebugMode(true) // Enable debug mode for testing
	
	// Initialize block integration
	cc.blockIntegration = NewBlockIntegration(cc)

	// Initialize batch proof recovery manager
	cc.batchProofManager = NewBatchProofRecoveryManager(cc, logger)
	cc.batchProofManager.Start()

	// Start async processors
	cc.wg.Add(3)
	go cc.processSynthetics()
	go cc.monitorTransmissionErrors()
	go cc.processRetries()

	return cc
}

// getMessageType determines the message type for blocking purposes
func (cc *CrossChainConductor) getMessageType(messages []messaging.Message) ConductorMessageType {
	// Check the first message to determine type - in practice, envelopes should be homogeneous
	if len(messages) == 0 {
		return ConductorMessageTypeOther
	}

	switch messages[0].Type() {
	case messaging.MessageTypeBlockAnchor:
		return ConductorMessageTypeAnchor
	case messaging.MessageTypeSynthetic, messaging.MessageTypeBadSynthetic:
		return ConductorMessageTypeSynthetic
	default:
		return ConductorMessageTypeOther
	}
}

// createDestinationKey creates a key for destination+type combination
func (cc *CrossChainConductor) createDestinationKey(msgType ConductorMessageType, destination *url.URL) DestinationKey {
	return DestinationKey{
		Type:        msgType,
		Destination: destination.String(),
	}
}

// getOrCreateDestinationQueue gets or creates a destination queue
func (cc *CrossChainConductor) getOrCreateDestinationQueue(key DestinationKey) *DestinationQueue {
	cc.queuesMutex.Lock()
	defer cc.queuesMutex.Unlock()

	queue, exists := cc.destinationQueues[key]
	if !exists {
		queue = &DestinationQueue{
			Key:            key,
			IsBlocked:      false,
			PendingTx:      make(map[string]*PendingTransmission),
			QueuedRequests: make([]*SyntheticRequest, 0),
			LastSuccess:    time.Now(),
		}
		cc.destinationQueues[key] = queue
		cc.logger.Debug("Created new destination queue", "type", key.Type, "destination", key.Destination)
	}
	return queue
}

// ProcessInbound processes inbound cross-partition messages through the conductor
func (cc *CrossChainConductor) ProcessInbound(ctx context.Context, messages []messaging.Message) []messaging.Message {
	// Phase 1: Direct pass-through for all messages (zero behavior change)
	// Future phases can add conductor logic here

	// Count and log cross-partition messages
	var crossPartitionCount int
	for _, msg := range messages {
		if cc.isCrossPartitionMessage(msg) {
			crossPartitionCount++
		}
	}

	if crossPartitionCount > 0 {
		cc.logger.Debug("Processing inbound cross-partition messages", "count", crossPartitionCount, "total_messages", len(messages))
	}

	// For now, return all messages unchanged
	return messages
}

// isCrossPartitionMessage determines if a message is a cross-partition anchor or synthetic transaction
func (cc *CrossChainConductor) isCrossPartitionMessage(msg messaging.Message) bool {
	switch msg.Type() {
	case messaging.MessageTypeSynthetic, messaging.MessageTypeBadSynthetic:
		return true
	case messaging.MessageTypeBlockAnchor:
		return true
	default:
		return false
	}
}

// SubmitSynthetic submits synthetic transactions for async processing
func (cc *CrossChainConductor) SubmitSynthetic(ctx context.Context, messages []messaging.Message, destination *url.URL) error {
	responseChan := make(chan error, 1)
	req := &SyntheticRequest{
		Messages:     messages,
		Destination:  destination,
		Context:      ctx,
		ResponseChan: responseChan,
	}

	select {
	case cc.syntheticChan <- req:
		// Wait for async processing to complete
		return <-responseChan
	case <-ctx.Done():
		return ctx.Err()
	case <-cc.stopChan:
		return errors.InternalError.With("conductor stopped")
	}
}

// processSynthetics is the main async processing loop
func (cc *CrossChainConductor) processSynthetics() {
	defer cc.wg.Done()
	cc.logger.Info("CrossChainConductor started")

	for {
		select {
		case req := <-cc.syntheticChan:
			cc.processSyntheticRequest(req)

		case <-cc.stopChan:
			cc.logger.Info("CrossChainConductor stopping")
			// Drain remaining requests
			for {
				select {
				case req := <-cc.syntheticChan:
					req.ResponseChan <- errors.InternalError.With("conductor stopping")
				default:
					return
				}
			}
		}
	}
}

// generateTxID creates a unique transaction ID for tracking
func (cc *CrossChainConductor) generateTxID() string {
	id := atomic.AddInt64(&cc.txIDCounter, 1)
	return fmt.Sprintf("ccc-%d-%d", time.Now().UnixNano(), id)
}

// processSyntheticRequest processes a single synthetic transaction request with per-destination blocking
func (cc *CrossChainConductor) processSyntheticRequest(req *SyntheticRequest) {
	// Determine message type and destination key
	msgType := cc.getMessageType(req.Messages)
	destKey := cc.createDestinationKey(msgType, req.Destination)

	// Get the destination queue for this type+destination combination
	queue := cc.getOrCreateDestinationQueue(destKey)

	queue.mu.Lock()
	defer queue.mu.Unlock()

	// Check if this destination+type combination is currently blocked
	if queue.IsBlocked {
		// Queue the request for later processing
		queue.QueuedRequests = append(queue.QueuedRequests, req)
		cc.logger.Debug("Request queued - destination blocked",
			"type", msgType, "destination", req.Destination,
			"blocked_since", queue.BlockedSince, "queue_depth", len(queue.QueuedRequests))
		return
	}

	// Not blocked - process immediately
	cc.processRequestImmediately(req, queue, destKey)
}

// processRequestImmediately processes a request without queueing
func (cc *CrossChainConductor) processRequestImmediately(req *SyntheticRequest, queue *DestinationQueue, destKey DestinationKey) {
	// Create pending transmission for error tracking
	txID := cc.generateTxID()
	pending := &PendingTransmission{
		ID:          txID,
		Messages:    req.Messages,
		Destination: req.Destination,
		DestKey:     destKey,
		Context:     req.Context,
		AttemptNum:  1,
		SubmittedAt: time.Now(),
		Callback:    req.ResponseChan,
	}

	// Store pending transmission in destination-specific queue
	queue.PendingTx[txID] = pending

	// Submit to dispatcher
	env := &messaging.Envelope{Messages: req.Messages}
	err := cc.dispatcher.Submit(req.Context, req.Destination, env)

	if err != nil {
		// Immediate submission error - remove from pending and report
		delete(queue.PendingTx, txID)
		atomic.AddInt64(&cc.syntheticsErrors, 1)
		queue.FailureCount++

		cc.logger.Error("Synthetic transaction submission failed",
			"destination", req.Destination, "error", err, "tx_id", txID, "type", destKey.Type)
		req.ResponseChan <- err
		return
	}

	// Success - block this destination+type until we get transmission confirmation
	queue.IsBlocked = true
	queue.BlockedSince = time.Now()

	atomic.AddInt64(&cc.syntheticsSent, 1)
	cc.logger.Debug("Synthetic transaction submitted - destination now blocked",
		"destination", req.Destination, "tx_id", txID, "type", destKey.Type)

	// Return success immediately - transmission monitoring will handle errors/retries
	req.ResponseChan <- nil
}

// monitorTransmissionErrors monitors the dispatcher's error channel for transmission failures
func (cc *CrossChainConductor) monitorTransmissionErrors() {
	defer cc.wg.Done()
	cc.logger.Info("Transmission error monitor started")

	for {
		select {
		case <-cc.stopChan:
			cc.logger.Info("Transmission error monitor stopping")
			return

		default:
			// Call dispatcher.Send() and monitor the error channel
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			errorChan := cc.dispatcher.Send(ctx)

			for err := range errorChan {
				if err != nil {
					atomic.AddInt64(&cc.transmissionErrors, 1)
					cc.logger.Error("Transmission error detected", "error", err)

					// Handle transmission error - we'll need to implement error->transaction mapping
					cc.handleTransmissionError(err)
				}
			}
			cancel()

			// Brief pause before next monitoring cycle
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// handleTransmissionError processes transmission errors with per-destination handling
func (cc *CrossChainConductor) handleTransmissionError(err error) {
	// Process each destination queue to find and handle pending transmissions
	cc.queuesMutex.RLock()
	destinationQueues := make([]*DestinationQueue, 0, len(cc.destinationQueues))
	for _, queue := range cc.destinationQueues {
		destinationQueues = append(destinationQueues, queue)
	}
	cc.queuesMutex.RUnlock()

	// Handle errors for each destination queue independently
	for _, queue := range destinationQueues {
		cc.handleQueueTransmissionError(queue, err)
	}
}

// handleQueueTransmissionError handles transmission errors for a specific destination queue
func (cc *CrossChainConductor) handleQueueTransmissionError(queue *DestinationQueue, err error) {
	queue.mu.Lock()
	defer queue.mu.Unlock()

	// Find the oldest pending transmission in this queue to retry
	var oldestTxID string
	var oldestPending *PendingTransmission
	var oldestTime time.Time = time.Now()

	for txID, pending := range queue.PendingTx {
		if pending.SubmittedAt.Before(oldestTime) {
			oldestTime = pending.SubmittedAt
			oldestTxID = txID
			oldestPending = pending
		}
	}

	if oldestPending == nil {
		// No pending transactions in this queue
		return
	}

	if oldestPending.AttemptNum >= cc.maxRetries {
		// Max retries reached - fail the transaction and unblock the destination
		cc.logger.Error("Transaction failed after max retries",
			"tx_id", oldestTxID, "attempts", oldestPending.AttemptNum,
			"type", queue.Key.Type, "destination", queue.Key.Destination)

		delete(queue.PendingTx, oldestTxID)
		queue.FailureCount++

		// Unblock this destination+type and process queued requests
		cc.unblockDestinationQueue(queue)

		// Notify the original caller of the failure (if callback still exists)
		if oldestPending.Callback != nil {
			select {
			case oldestPending.Callback <- errors.InternalError.WithFormat("transmission failed after %d attempts: %v", oldestPending.AttemptNum, err):
			default:
				// Callback channel might be closed, that's okay
			}
		}
		return
	}

	// Queue for retry - increment attempt but don't unblock yet
	oldestPending.AttemptNum++
	oldestPending.RetryAfter = time.Now().Add(cc.retryDelay)
	queue.RetryCount++

	select {
	case cc.retryChan <- oldestPending:
		cc.logger.Info("Transaction queued for retry",
			"tx_id", oldestTxID, "attempt", oldestPending.AttemptNum,
			"type", queue.Key.Type, "destination", queue.Key.Destination)
	default:
		// Retry queue full - fail the transaction and unblock
		cc.logger.Error("Retry queue full, failing transaction",
			"tx_id", oldestTxID, "type", queue.Key.Type, "destination", queue.Key.Destination)

		delete(queue.PendingTx, oldestTxID)
		queue.FailureCount++
		cc.unblockDestinationQueue(queue)

		if oldestPending.Callback != nil {
			select {
			case oldestPending.Callback <- errors.InternalError.With("retry queue full"):
			default:
				// Callback channel might be closed, that's okay
			}
		}
	}
}

// unblockDestinationQueue unblocks a destination queue and processes any queued requests
func (cc *CrossChainConductor) unblockDestinationQueue(queue *DestinationQueue) {
	// This function assumes queue.mu is already locked by the caller

	queue.IsBlocked = false
	queue.LastSuccess = time.Now() // Update success time even if this was a failure

	cc.logger.Debug("Unblocked destination queue",
		"type", queue.Key.Type, "destination", queue.Key.Destination,
		"queued_requests", len(queue.QueuedRequests))

	// Process the first queued request if any exist
	if len(queue.QueuedRequests) > 0 {
		nextReq := queue.QueuedRequests[0]
		queue.QueuedRequests = queue.QueuedRequests[1:] // Remove first element

		// Process the next request immediately (this will re-block if submission succeeds)
		cc.processRequestImmediately(nextReq, queue, queue.Key)
	}
}

// processRetries handles retry attempts for failed transmissions
func (cc *CrossChainConductor) processRetries() {
	defer cc.wg.Done()
	cc.logger.Info("Retry processor started")

	ticker := time.NewTicker(1 * time.Second) // Check for retries every second
	defer ticker.Stop()

	for {
		select {
		case <-cc.stopChan:
			cc.logger.Info("Retry processor stopping")
			// Fail all remaining retries
			for {
				select {
				case pending := <-cc.retryChan:
					pending.Callback <- errors.InternalError.With("conductor stopping")
				default:
					return
				}
			}

		case pending := <-cc.retryChan:
			// Check if it's time to retry
			if time.Now().Before(pending.RetryAfter) {
				// Not ready yet - put it back
				select {
				case cc.retryChan <- pending:
				default:
					// Queue full - fail the transaction
					cc.logger.Error("Cannot requeue retry, failing transaction", "tx_id", pending.ID)
					// Note: pending transaction will be handled by the queue cleanup logic
					pending.Callback <- errors.InternalError.With("retry queue full")
				}
				continue
			}

			// Retry the transmission
			cc.retryTransmission(pending)

		case <-ticker.C:
			// Periodic cleanup of old pending transactions
			cc.cleanupOldTransmissions()
		}
	}
}

// retryTransmission attempts to retransmit a failed transaction with per-destination handling
func (cc *CrossChainConductor) retryTransmission(pending *PendingTransmission) {
	// Get the destination queue for this transmission
	queue := cc.getOrCreateDestinationQueue(pending.DestKey)

	queue.mu.Lock()
	defer queue.mu.Unlock()

	// Verify the pending transmission still exists in the queue
	if _, exists := queue.PendingTx[pending.ID]; !exists {
		cc.logger.Info("Retry attempted for non-existent transaction",
			"tx_id", pending.ID, "type", pending.DestKey.Type, "destination", pending.DestKey.Destination)
		return
	}

	env := &messaging.Envelope{Messages: pending.Messages}
	err := cc.dispatcher.Submit(pending.Context, pending.Destination, env)

	if err != nil {
		// Retry submission failed
		cc.logger.Error("Retry submission failed",
			"tx_id", pending.ID, "attempt", pending.AttemptNum, "error", err,
			"type", pending.DestKey.Type, "destination", pending.DestKey.Destination)

		if pending.AttemptNum >= cc.maxRetries {
			// Max retries reached - fail and unblock
			delete(queue.PendingTx, pending.ID)
			queue.FailureCount++

			cc.unblockDestinationQueue(queue)

			if pending.Callback != nil {
				select {
				case pending.Callback <- errors.InternalError.WithFormat("retry failed after %d attempts: %v", pending.AttemptNum, err):
				default:
					// Callback channel might be closed, that's okay
				}
			}
		} else {
			// Queue for another retry
			pending.AttemptNum++
			pending.RetryAfter = time.Now().Add(cc.retryDelay)
			queue.RetryCount++

			select {
			case cc.retryChan <- pending:
				cc.logger.Debug("Transaction requeued for retry",
					"tx_id", pending.ID, "attempt", pending.AttemptNum,
					"type", pending.DestKey.Type, "destination", pending.DestKey.Destination)
			default:
				// Queue full - fail and unblock
				delete(queue.PendingTx, pending.ID)
				queue.FailureCount++
				cc.unblockDestinationQueue(queue)

				if pending.Callback != nil {
					select {
					case pending.Callback <- errors.InternalError.With("retry queue full"):
					default:
						// Callback channel might be closed, that's okay
					}
				}
			}
		}
		return
	}

	// Retry submission successful - destination remains blocked until transmission confirmation
	atomic.AddInt64(&cc.syntheticsRetried, 1)
	cc.logger.Info("Transaction retry submitted successfully",
		"tx_id", pending.ID, "attempt", pending.AttemptNum,
		"type", pending.DestKey.Type, "destination", pending.DestKey.Destination)

	// Update pending transmission timestamp
	pending.SubmittedAt = time.Now()
}

// cleanupOldTransmissions removes transactions that have been pending too long across all destination queues
func (cc *CrossChainConductor) cleanupOldTransmissions() {
	cutoff := time.Now().Add(-5 * time.Minute) // Timeout after 5 minutes

	cc.queuesMutex.RLock()
	destinationQueues := make([]*DestinationQueue, 0, len(cc.destinationQueues))
	for _, queue := range cc.destinationQueues {
		destinationQueues = append(destinationQueues, queue)
	}
	cc.queuesMutex.RUnlock()

	// Clean up each destination queue independently
	for _, queue := range destinationQueues {
		cc.cleanupQueueTransmissions(queue, cutoff)
	}
}

// cleanupQueueTransmissions cleans up old transmissions in a specific queue
func (cc *CrossChainConductor) cleanupQueueTransmissions(queue *DestinationQueue, cutoff time.Time) {
	queue.mu.Lock()
	defer queue.mu.Unlock()

	var staleTransmissions []*PendingTransmission

	// Find stale transmissions
	for txID, pending := range queue.PendingTx {
		if pending.SubmittedAt.Before(cutoff) {
			staleTransmissions = append(staleTransmissions, pending)
			delete(queue.PendingTx, txID)
		}
	}

	// If we removed transmissions, potentially unblock the queue
	if len(staleTransmissions) > 0 {
		cc.logger.Info("Cleaning up stale pending transmissions",
			"count", len(staleTransmissions),
			"type", queue.Key.Type, "destination", queue.Key.Destination)

		// Unblock the destination if it was blocked
		if queue.IsBlocked {
			cc.unblockDestinationQueue(queue)
		}

		// Notify callbacks of timeout
		for _, pending := range staleTransmissions {
			if pending.Callback != nil {
				select {
				case pending.Callback <- errors.InternalError.With("transaction timeout"):
				default:
					// Callback channel might be closed, that's okay
				}
			}
		}
	}
}

// Stop gracefully stops the conductor
func (cc *CrossChainConductor) Stop() {
	close(cc.stopChan)
	cc.wg.Wait()

	// Clean up any remaining pending transactions across all destination queues
	cc.queuesMutex.Lock()
	for _, queue := range cc.destinationQueues {
		queue.mu.Lock()

		// Fail all pending transmissions
		for txID, pending := range queue.PendingTx {
			if pending.Callback != nil {
				select {
				case pending.Callback <- errors.InternalError.With("conductor stopped"):
				default:
					// Callback channel might be closed, that's okay
				}
			}
			delete(queue.PendingTx, txID)
		}

		// Fail all queued requests
		for _, req := range queue.QueuedRequests {
			if req.ResponseChan != nil {
				select {
				case req.ResponseChan <- errors.InternalError.With("conductor stopped"):
				default:
					// Channel might be closed, that's okay
				}
			}
		}
		queue.QueuedRequests = nil

		queue.mu.Unlock()
	}
	cc.queuesMutex.Unlock()

	cc.logger.Info("CrossChainConductor stopped")
}

// GetMetrics returns current processing metrics
func (cc *CrossChainConductor) GetMetrics() (sent, errors, retried, transmissionErrors int64) {
	return atomic.LoadInt64(&cc.syntheticsSent),
		atomic.LoadInt64(&cc.syntheticsErrors),
		atomic.LoadInt64(&cc.syntheticsRetried),
		atomic.LoadInt64(&cc.transmissionErrors)
}

// InitRecoveryManager initializes the recovery manager with database and client
func (cc *CrossChainConductor) InitRecoveryManager(db database.Beginner, client api.Querier) {
	if cc.recoveryManager != nil {
		cc.logger.Info("Recovery manager already initialized")
		return
	}

	cc.recoveryManager = NewRecoveryManager(cc, db, client)
	cc.recoveryManager.Start()
	cc.logger.Info("Recovery manager initialized and started")
}

// RequestMissingTransactions requests missing anchors or synthetic transactions
func (cc *CrossChainConductor) RequestMissingTransactions(
	msgType ConductorMessageType,
	source, destination string,
	fromNum, toNum uint64,
) (*RecoveryResponse, error) {
	if cc.recoveryManager == nil {
		return nil, errors.NotReady.With("recovery manager not initialized")
	}

	req := &RecoveryRequest{
		Type:        msgType,
		Source:      source,
		Destination: destination,
		FromNumber:  fromNum,
		ToNumber:    toNum,
		Requester:   destination,
		Priority:    1,
	}

	return cc.recoveryManager.RequestMissingTransactions(req)
}

// RequestMissingTransactionsWithBatchProof requests missing transactions using collection proofs for efficiency
func (cc *CrossChainConductor) RequestMissingTransactionsWithBatchProof(
	partitionID string,
	msgType ConductorMessageType,
	missingSequences []uint64,
	chainURL *url.URL,
) error {
	if cc.batchProofManager == nil {
		return errors.NotReady.With("batch proof recovery manager not initialized")
	}

	// Convert MessageType to RecoveryType
	var recoveryType RecoveryType
	switch msgType {
	case ConductorMessageTypeAnchor:
		recoveryType = RecoveryTypeAnchor
	case ConductorMessageTypeSynthetic:
		recoveryType = RecoveryTypeSynthetic
	default:
		return errors.BadRequest.WithFormat("unsupported message type for batch recovery: %d", msgType)
	}

	cc.logger.Info("Requesting batch proof recovery",
		"partition", partitionID,
		"type", recoveryType.String(),
		"sequences", len(missingSequences),
		"chain", chainURL)

	// Create batch recovery request
	req := &BatchRecoveryRequest{
		PartitionID:      partitionID,
		Type:             recoveryType,
		MissingSequences: missingSequences,
		ChainURL:         chainURL,
		RequestTime:      time.Now(),
		Callback: func(response *BatchRecoveryResponse) {
			cc.handleBatchRecoveryResponse(response)
		},
	}

	// Send to batch proof manager
	cc.batchProofManager.RequestBatchRecovery(req)
	return nil
}

// handleBatchRecoveryResponse processes the response from batch proof recovery
func (cc *CrossChainConductor) handleBatchRecoveryResponse(response *BatchRecoveryResponse) {
	if response.Error != nil {
		cc.logger.Error("Batch recovery failed",
			"partition", response.PartitionID,
			"type", response.Type,
			"error", response.Error)
		return
	}

	cc.logger.Info("Batch recovery successful",
		"partition", response.PartitionID,
		"type", response.Type,
		"batch_size", response.BatchSize,
		"proof_savings", response.ProofSavings,
		"transactions", len(response.Transactions))

	// Process recovered transactions
	for _, tx := range response.Transactions {
		cc.logger.Debug("Processing recovered transaction",
			"sequence", tx.SequenceNum,
			"hash", fmt.Sprintf("%x", tx.Hash[:8]),
			"type", tx.Type)

		// Here you would submit the recovered transaction back to the destination partition
		// This would integrate with the existing message processing pipeline
	}

	// Log collection proof efficiency metrics
	if response.CollectionProof != nil {
		cc.logger.Info("Collection proof metrics",
			"partition", response.PartitionID,
			"proof_elements", len(response.CollectionProof.Elements),
			"individual_proofs_saved", response.ProofSavings,
			"generation_time", response.ProofGenerated.Sub(time.Now().Add(-time.Since(response.ProofGenerated))))
	}
}

// HandleRecoveryRequest processes an incoming recovery request from another partition
func (cc *CrossChainConductor) HandleRecoveryRequest(req *RecoveryRequest) error {
	if cc.recoveryManager == nil {
		return errors.NotReady.With("recovery manager not initialized")
	}

	cc.logger.Info("Received recovery request",
		"type", cc.getMessageTypeName(req.Type),
		"source", req.Source,
		"destination", req.Destination,
		"range", fmt.Sprintf("%d-%d", req.FromNumber, req.ToNumber),
		"requester", req.Requester)

	// Process the recovery request asynchronously
	go func() {
		resp, err := cc.recoveryManager.RequestMissingTransactions(req)
		if err != nil {
			cc.logger.Error("Failed to process recovery request", "error", err)
			return
		}

		// Send recovered transactions to the requester
		if len(resp.Transactions) > 0 {
			err = cc.recoveryManager.ProvideRecoveredTransactions(resp.Transactions, req.Requester)
			if err != nil {
				cc.logger.Error("Failed to provide recovered transactions", "error", err)
			} else {
				cc.logger.Info("Provided recovered transactions",
					"count", len(resp.Transactions),
					"to", req.Requester)
			}
		}
	}()

	return nil
}

// SubmitAnchor submits an anchor for transmission
func (cc *CrossChainConductor) SubmitAnchor(req *AnchorRequest) error {
	destKey := cc.createDestinationKey(ConductorMessageTypeAnchor, req.Destination)

	// Get or create destination queue
	queue := cc.getOrCreateDestinationQueue(destKey)

	// Create synthetic request wrapper
	synthReq := &SyntheticRequest{
		Messages:    []messaging.Message{req.Anchor},
		Destination: req.Destination,
		SequenceNum: req.SequenceNum,
	}

	// Queue or send based on blocking state
	queue.mu.Lock()
	if queue.IsBlocked {
		queue.QueuedRequests = append(queue.QueuedRequests, synthReq)
		queue.mu.Unlock()
		cc.logger.Debug("Anchor queued (destination blocked)",
			"destination", req.Destination.String(),
			"sequence", req.SequenceNum)
	} else {
		queue.mu.Unlock()
		// Send for immediate processing
		select {
		case cc.syntheticChan <- synthReq:
			cc.logger.Debug("Anchor submitted for transmission",
				"destination", req.Destination.String(),
				"sequence", req.SequenceNum)
		default:
			cc.logger.Info("Synthetic channel full, queueing anchor")
			queue.mu.Lock()
			queue.QueuedRequests = append(queue.QueuedRequests, synthReq)
			queue.mu.Unlock()
		}
	}

	return nil
}

// CheckPartitionHealth checks and reports health of partition synchronization
func (cc *CrossChainConductor) CheckPartitionHealth() map[string]interface{} {
	health := make(map[string]interface{})

	cc.queuesMutex.RLock()
	defer cc.queuesMutex.RUnlock()

	var totalQueued, totalPending, blockedQueues int
	missingByDestination := make(map[string]int)

	for key, queue := range cc.destinationQueues {
		queue.mu.RLock()
		queued := len(queue.QueuedRequests)
		pending := len(queue.PendingTx)
		blocked := queue.IsBlocked
		queue.mu.RUnlock()

		totalQueued += queued
		totalPending += pending
		if blocked {
			blockedQueues++
		}

		if queued > 10 || pending > 10 {
			missingByDestination[key.Destination] = queued + pending
		}
	}

	health["total_queued"] = totalQueued
	health["total_pending"] = totalPending
	health["blocked_queues"] = blockedQueues
	health["destinations_with_backlog"] = missingByDestination

	// Check recovery manager health if available
	if cc.recoveryManager != nil {
		cc.recoveryManager.mu.RLock()
		activeRecovery := len(cc.recoveryManager.activeRecovery)
		cc.recoveryManager.mu.RUnlock()
		health["active_recovery_sessions"] = activeRecovery
	}

	return health
}

// Helper function to get message type name
func (cc *CrossChainConductor) getMessageTypeName(t ConductorMessageType) string {
	switch t {
	case ConductorMessageTypeAnchor:
		return "anchor"
	case ConductorMessageTypeSynthetic:
		return "synthetic"
	default:
		return "unknown"
	}
}

// Batch Proof Recovery Types (inline to avoid import cycles)

// RecoveryType represents the type of recovery needed
type RecoveryType int

const (
	RecoveryTypeAnchor RecoveryType = iota
	RecoveryTypeSynthetic
)

func (rt RecoveryType) String() string {
	switch rt {
	case RecoveryTypeAnchor:
		return "anchor"
	case RecoveryTypeSynthetic:
		return "synthetic"
	default:
		return "unknown"
	}
}

// BatchRecoveryRequest represents a request for batch recovery using collection proofs
type BatchRecoveryRequest struct {
	PartitionID      string
	Type             RecoveryType
	MissingSequences []uint64
	ChainURL         *url.URL
	RequestTime      time.Time
	Callback         func(*BatchRecoveryResponse)
}

// BatchRecoveryResponse contains the batch proof and transactions
type BatchRecoveryResponse struct {
	PartitionID string
	Type        RecoveryType

	// Collection proof data
	CollectionProof   *merkle.ReceiptList // Single proof for all transactions
	TransactionHashes [][]byte            // Hashes in the collection proof

	// Transaction data (sent separately without individual proofs)
	Transactions []*RecoveredTransaction

	// Metadata
	ProofGenerated time.Time
	BatchSize      int
	ProofSavings   int // How many individual proofs we avoided
	Error          error
}

// RecoveredTransaction represents a recovered transaction without individual proof
type RecoveredTransaction struct {
	Hash        []byte
	SequenceNum uint64
	Timestamp   time.Time
	Type        string
	Data        []byte
}

// BatchProofRecoveryManager placeholder for the collection proof functionality
// This would contain the full implementation from batch_proof_recovery.go
type BatchProofRecoveryManager struct {
	conductor      *CrossChainConductor
	logger         logging.OptionalLogger
	batchThreshold int
	maxBatchSize   int
	totalRequests  int64
	batchRequests  int64
	proofSavings   int64
}

func NewBatchProofRecoveryManager(conductor *CrossChainConductor, logger logging.OptionalLogger) *BatchProofRecoveryManager {
	return &BatchProofRecoveryManager{
		conductor:      conductor,
		logger:         logger.With("module", "batch-recovery").(logging.OptionalLogger),
		batchThreshold: 2,   // Use batch proof when >= 2 transactions
		maxBatchSize:   100, // Maximum 100 transactions per batch
	}
}

func (brm *BatchProofRecoveryManager) Start() {
	brm.logger.Info("Batch proof recovery manager started")
}

func (brm *BatchProofRecoveryManager) Stop() {
	brm.logger.Info("Batch proof recovery manager stopped")
}

func (brm *BatchProofRecoveryManager) RequestBatchRecovery(req *BatchRecoveryRequest) {
	brm.logger.Info("Processing batch recovery request",
		"partition", req.PartitionID,
		"type", req.Type,
		"sequences", len(req.MissingSequences))

	// For now, simulate successful collection proof generation
	// In full implementation, this would generate actual ReceiptList proofs
	go func() {
		time.Sleep(10 * time.Millisecond) // Simulate processing time

		response := &BatchRecoveryResponse{
			PartitionID:    req.PartitionID,
			Type:           req.Type,
			BatchSize:      len(req.MissingSequences),
			ProofSavings:   len(req.MissingSequences) - 1, // One proof instead of many
			ProofGenerated: time.Now(),
			Transactions:   make([]*RecoveredTransaction, len(req.MissingSequences)),
		}

		// Create placeholder recovered transactions
		for i, seq := range req.MissingSequences {
			response.Transactions[i] = &RecoveredTransaction{
				Hash:        []byte(fmt.Sprintf("hash-%d", seq)),
				SequenceNum: seq,
				Timestamp:   time.Now(),
				Type:        req.Type.String(),
				Data:        []byte(fmt.Sprintf("tx-data-%d", seq)),
			}
		}

		atomic.AddInt64(&brm.totalRequests, 1)
		if len(req.MissingSequences) >= brm.batchThreshold {
			atomic.AddInt64(&brm.batchRequests, 1)
			atomic.AddInt64(&brm.proofSavings, int64(response.ProofSavings))
		}

		if req.Callback != nil {
			req.Callback(response)
		}
	}()
}

func (brm *BatchProofRecoveryManager) GetMetrics() map[string]interface{} {
	return map[string]interface{}{
		"total_requests":  atomic.LoadInt64(&brm.totalRequests),
		"batch_requests":  atomic.LoadInt64(&brm.batchRequests),
		"proof_savings":   atomic.LoadInt64(&brm.proofSavings),
		"batch_threshold": brm.batchThreshold,
		"max_batch_size":  brm.maxBatchSize,
	}
}

// CreateProofsForSyntheticTransactionsWithPartitions creates optimized proofs for synthetic transactions
// using the correct partition-specific sequence chains for each destination.
func (cc *CrossChainConductor) CreateProofsForSyntheticTransactionsWithPartitions(
	ctx context.Context,
	batch *database.Batch,
	sourcePartition *url.URL,
	transactions []SyntheticTransaction,
	rootChain *database.Chain,
) ([]*protocol.AnnotatedReceipt, error) {
	if cc.proofService == nil {
		return nil, errors.InternalError.With("proof service not initialized")
	}

	// Group transactions by destination for optimal batching
	type destGroup struct {
		partition string
		requests  []ProofRequest
	}
	destinationGroups := make(map[string]*destGroup)
	
	for _, tx := range transactions {
		dest := tx.Destination.String()
		
		// Parse destination partition
		destPartition, ok := protocol.ParsePartitionUrl(tx.Destination)
		if !ok {
			return nil, errors.InternalError.WithFormat("invalid destination partition: %v", tx.Destination)
		}
		
		if destinationGroups[dest] == nil {
			destinationGroups[dest] = &destGroup{
				partition: destPartition,
				requests:  []ProofRequest{},
			}
		}
		
		// We'll set the chain later when we have it
		destinationGroups[dest].requests = append(destinationGroups[dest].requests, ProofRequest{
			Type:        ProofTypeSynthetic,
			Destination: tx.Destination,
			Sequences:   []uint64{tx.SequenceNum},
			ChainURL:    tx.ChainURL,
			SourceChain: nil, // Will be set below
			RootChain:   rootChain,
		})
	}

	// Get the synthetic ledger account
	syntheticAccount := batch.Account(sourcePartition.JoinPath(protocol.Synthetic))
	
	// Create optimized proofs for each destination
	var allProofs []*protocol.AnnotatedReceipt
	for dest, group := range destinationGroups {
		// Get the partition-specific sequence chain for this destination
		sequenceChain, err := syntheticAccount.SyntheticSequenceChain(group.partition).Get()
		if err != nil {
			return nil, errors.UnknownError.WithFormat("failed to get sequence chain for partition %s: %w", group.partition, err)
		}
		
		// Update all requests with the correct chain
		for i := range group.requests {
			group.requests[i].SourceChain = sequenceChain
		}
		
		// Check if we should use collection proof
		if len(group.requests) >= cc.proofService.batchThreshold {
			// Merge sequences for collection proof
			mergedReq := cc.proofService.mergeSequences(group.requests)
			
			cc.logger.Info("Creating collection proof for synthetic transactions",
				"destination", dest,
				"partition", group.partition,
				"count", len(group.requests),
				"sequences", mergedReq.Sequences)
			
			// Create single collection proof for all transactions to this destination
			resp, err := cc.proofService.CreateProof(ctx, mergedReq)
			if err != nil {
				cc.logger.Error("Failed to create collection proof, falling back to individual",
					"destination", dest,
					"error", err)
				
				// Fallback to individual proofs
				for _, req := range group.requests {
					resp, err := cc.proofService.CreateProof(ctx, req)
					if err != nil {
						return nil, errors.UnknownError.WithFormat("failed to create proof: %w", err)
					}
					allProofs = append(allProofs, resp.Proof)
				}
			} else {
				// Use the same collection proof for all transactions in this group
				for range group.requests {
					allProofs = append(allProofs, resp.Proof)
				}
				
				cc.logger.Info("Collection proof created successfully",
					"destination", dest,
					"partition", group.partition,
					"proof_savings", resp.ProofSavings)
			}
		} else {
			// Create individual proofs for small batches
			for _, req := range group.requests {
				resp, err := cc.proofService.CreateProof(ctx, req)
				if err != nil {
					return nil, errors.UnknownError.WithFormat("failed to create proof: %w", err)
				}
				allProofs = append(allProofs, resp.Proof)
			}
		}
	}

	// Log metrics
	metrics := cc.proofService.GetMetrics()
	cc.logger.Debug("Proof generation metrics",
		"individual_proofs", metrics.IndividualProofsCreated,
		"collection_proofs", metrics.CollectionProofsCreated,
		"proofs_saved", metrics.ProofsSaved)

	return allProofs, nil
}

// CreateProofsForSyntheticTransactions creates optimized proofs for synthetic transactions
// This is the central entry point for all synthetic proof creation
// NOTE: The synthChain parameter should be the partition-specific sequence chain for the destination,
// not the main synthetic chain. This ensures proofs are created from the correct source partition's chain.
// DEPRECATED: Use CreateProofsForSyntheticTransactionsWithPartitions for correct partition-specific chain handling
func (cc *CrossChainConductor) CreateProofsForSyntheticTransactions(
	ctx context.Context,
	transactions []SyntheticTransaction,
	synthChain *database.Chain,
	rootChain *database.Chain,
) ([]*protocol.AnnotatedReceipt, error) {
	if cc.proofService == nil {
		return nil, errors.InternalError.With("proof service not initialized")
	}

	// Group transactions by destination for optimal batching
	destinationGroups := make(map[string][]ProofRequest)
	for _, tx := range transactions {
		dest := tx.Destination.String()
		destinationGroups[dest] = append(destinationGroups[dest], ProofRequest{
			Type:        ProofTypeSynthetic,
			Destination: tx.Destination,
			Sequences:   []uint64{tx.SequenceNum},
			ChainURL:    tx.ChainURL,
			SourceChain: synthChain, // This should be the partition-specific chain
			RootChain:   rootChain,
		})
	}

	// Create optimized proofs for each destination
	var allProofs []*protocol.AnnotatedReceipt
	for dest, requests := range destinationGroups {
		// Check if we should use collection proof
		if len(requests) >= cc.proofService.batchThreshold {
			// Merge sequences for collection proof
			mergedReq := cc.proofService.mergeSequences(requests)

			cc.logger.Info("Creating collection proof for synthetic transactions",
				"destination", dest,
				"count", len(requests),
				"sequences", mergedReq.Sequences)

			// Create single collection proof for all transactions to this destination
			resp, err := cc.proofService.CreateProof(ctx, mergedReq)
			if err != nil {
				cc.logger.Error("Failed to create collection proof, falling back to individual",
					"destination", dest,
					"error", err)

				// Fallback to individual proofs
				for _, req := range requests {
					resp, err := cc.proofService.CreateProof(ctx, req)
					if err != nil {
						return nil, errors.UnknownError.WithFormat("failed to create proof: %w", err)
					}
					allProofs = append(allProofs, resp.Proof)
				}
			} else {
				// Use the same collection proof for all transactions in this group
				for range requests {
					allProofs = append(allProofs, resp.Proof)
				}

				cc.logger.Info("Collection proof created successfully",
					"destination", dest,
					"proof_savings", resp.ProofSavings)
			}
		} else {
			// Create individual proofs for small batches
			for _, req := range requests {
				resp, err := cc.proofService.CreateProof(ctx, req)
				if err != nil {
					return nil, errors.UnknownError.WithFormat("failed to create proof: %w", err)
				}
				allProofs = append(allProofs, resp.Proof)
			}
		}
	}

	// Log metrics
	metrics := cc.proofService.GetMetrics()
	cc.logger.Debug("Proof generation metrics",
		"individual_proofs", metrics.IndividualProofsCreated,
		"collection_proofs", metrics.CollectionProofsCreated,
		"proofs_saved", metrics.ProofsSaved)

	return allProofs, nil
}

// ValidateIncomingProof validates a proof from another partition
func (cc *CrossChainConductor) ValidateIncomingProof(proof *protocol.AnnotatedReceipt) error {
	if cc.proofService == nil {
		return errors.InternalError.With("proof service not initialized")
	}

	return cc.proofService.ValidateProof(proof)
}

// GetProofMetrics returns current proof service metrics
func (cc *CrossChainConductor) GetProofMetrics() ProofMetrics {
	if cc.proofService == nil {
		return ProofMetrics{}
	}

	return cc.proofService.GetMetrics()
}

// SyntheticTransaction represents a synthetic transaction needing a proof
type SyntheticTransaction struct {
	Destination *url.URL
	SequenceNum uint64
	ChainURL    *url.URL
	Hash        []byte
	Source      *url.URL
	Message     messaging.Message
}

// SendCrossChainMessages sends messages using the unified transport layer
// This method supports both anchors and synthetic transactions with collection proofs
func (cc *CrossChainConductor) SendCrossChainMessages(
	ctx context.Context,
	messages []CrossChainMessage,
) error {
	if cc.unifiedTransport == nil {
		return errors.InternalError.With("unified transport not initialized")
	}
	
	// Send through unified transport with automatic batching and collection proofs
	err := cc.unifiedTransport.Send(ctx, messages)
	if err != nil {
		return errors.UnknownError.WithFormat("unified transport send failed: %w", err)
	}
	
	// Log transport metrics
	metrics := cc.unifiedTransport.GetMetrics()
	cc.logger.Info("Unified transport metrics",
		"synthetics_sent", metrics.SyntheticsSent,
		"anchors_sent", metrics.AnchorsSent,
		"collection_proofs", metrics.CollectionProofsUsed,
		"individual_proofs", metrics.IndividualProofsUsed)
	
	return nil
}

// GetBlockIntegration returns the block integration helper for the block executor
func (cc *CrossChainConductor) GetBlockIntegration() *BlockIntegration {
	return cc.blockIntegration
}
