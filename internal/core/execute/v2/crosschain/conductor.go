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
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// RecoveryRequest represents a simple gap recovery request
type RecoveryRequest struct {
	Requester  string // Which partition is requesting
	FromNumber uint64 // Starting sequence number needed
}

// MessageType represents the type of cross-partition message
type MessageType int

const (
	MessageTypeAnchor MessageType = iota
	MessageTypeSynthetic
	MessageTypeDirectoryAnchor
	MessageTypeBlockSummary
	MessageTypeOther
)

// DestinationKey uniquely identifies a message type + destination combination
type DestinationKey struct {
	Type        MessageType
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

// CrossChainConductor handles cross-partition message coordination using collection proofs
type CrossChainConductor struct {
	// Infrastructure
	dispatcher execute.Dispatcher
	logger     logging.OptionalLogger
	describe   *config.Describe
	db         database.Beginner

	// Async processing (keep for compatibility)
	syntheticChan chan *SyntheticRequest
	retryChan     chan *PendingTransmission
	stopChan      chan struct{}
	wg            sync.WaitGroup

	// Per-destination blocking and tracking (keep for compatibility)
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

	// Destination send state tracking
	// NOTE: This is separate from destinationQueues because it tracks different concerns:
	// - destinationQueues: Per-message-type transmission state and blocking
	// - destinations: Simple index-based gap recovery state per destination
	// Both are needed for the complete healing mechanism.
	destinations   map[string]*DestinationSendState
	destinationsMu sync.RWMutex


	// Batch proof recovery manager removed - was fake implementation

	// Centralized proof service for construction and validation
	proofService *ProofService

	// Sequence tracker for gap detection and healing
	sequenceTracker *SimpleSequenceTracker

	// Recovery testing (ONLY active with faucet + test flag - NEVER in production)
	// This provides safe testing of the healing mechanism by randomly dropping messages
	// and verifying gap detection, recovery requests, and healing work correctly.
	// See docs/testing/RECOVERY_TESTING.md for complete documentation.
	recoveryTestConfig *RecoveryTestConfig
}

// NewCrossChainConductor creates and starts the conductor
func NewCrossChainConductor(dispatcher execute.Dispatcher, logger logging.OptionalLogger, describe *config.Describe, db database.Beginner) *CrossChainConductor {
	return NewCrossChainConductorWithRecoveryTesting(dispatcher, logger, describe, db, 0)
}

// NewCrossChainConductorWithRecoveryTesting creates conductor with optional recovery testing
// dropsPerMinute: 0 = disabled, >0 = enable testing with that many drops per minute
func NewCrossChainConductorWithRecoveryTesting(dispatcher execute.Dispatcher, logger logging.OptionalLogger, describe *config.Describe, db database.Beginner, dropsPerMinute int) *CrossChainConductor {
	cc := &CrossChainConductor{
		dispatcher:        dispatcher,
		logger:            logger.With("module", "crosschain-conductor").(logging.OptionalLogger),
		describe:          describe,
		db:               db,
		syntheticChan:     make(chan *SyntheticRequest, 100),   // Buffered channel for async processing
		retryChan:         make(chan *PendingTransmission, 50), // Retry queue
		stopChan:          make(chan struct{}),
		destinationQueues: make(map[DestinationKey]*DestinationQueue),
		destinations:      make(map[string]*DestinationSendState),
		maxRetries:        3,               // Retry failed transmissions up to 3 times
		retryDelay:        2 * time.Second, // Wait 2 seconds between retries
	}

	// Initialize centralized proof service (NO CACHING for easier testing)
	cc.proofService = NewProofService(logger)
	cc.proofService.SetDebugMode(true) // Enable debug mode for testing

	// Initialize sequence tracker for gap detection and recovery
	cc.sequenceTracker = NewSimpleSequenceTracker(cc, logger)

	// Initialize recovery testing (ONLY if faucet exists and --dpm > 0)
	cc.recoveryTestConfig = NewRecoveryTestConfig(logger, describe, dropsPerMinute)

	// Start async processors
	cc.wg.Add(3)
	go cc.processSynthetics()
	go cc.monitorTransmissionErrors()
	go cc.processRetries()

	return cc
}

// getMessageType determines the message type for blocking purposes
func (cc *CrossChainConductor) getMessageType(messages []messaging.Message) MessageType {
	// Check the first message to determine type - in practice, envelopes should be homogeneous
	if len(messages) == 0 {
		return MessageTypeOther
	}

	switch messages[0].Type() {
	case messaging.MessageTypeBlockAnchor:
		return MessageTypeAnchor
	case messaging.MessageTypeSynthetic, messaging.MessageTypeBadSynthetic:
		return MessageTypeSynthetic
	default:
		return MessageTypeOther
	}
}

// createDestinationKey creates a key for destination+type combination
func (cc *CrossChainConductor) createDestinationKey(msgType MessageType, destination *url.URL) DestinationKey {
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

// ProcessInbound processes inbound cross-partition messages and detects gaps
// This is the receiving side of the healing mechanism that:
// 1. Tracks sequence heights for anchor and synthetic chains from each source partition
// 2. Detects gaps in sequence numbers
// 3. Sends recovery requests to source partitions for missing entries
// 4. Filters out gap messages to let source partitions resend
//
// IMPORTANT: Anchors and synthetics are tracked separately because:
// - Synthetic transactions: Temporary data, can be pruned after processing
// - Anchor transactions: Permanent data, required for cryptographic proofs
// This separation is essential for proper data lifecycle and storage management.
func (cc *CrossChainConductor) ProcessInbound(ctx context.Context, messages []messaging.Message) []messaging.Message {
	cc.logger.Debug("[HEALING-DEBUG] ProcessInbound called",
		"message_count", len(messages),
		"recovery_testing_enabled", cc.recoveryTestConfig != nil && cc.recoveryTestConfig.IsEnabled())
	if cc.sequenceTracker == nil {
		cc.logger.Error("[HEALING-DEBUG] Sequence tracker not initialized - passing messages through - THIS IS A BUG")
		return messages
	}

	var validMessages []messaging.Message
	var gapsDetected int

	for _, msg := range messages {
		if !cc.isCrossPartitionMessage(msg) {
			// Non-crosschain messages pass through unchanged
			validMessages = append(validMessages, msg)
			continue
		}

		// Handle crosschain messages with gap detection
		switch msg.Type() {
		case messaging.MessageTypeSynthetic, messaging.MessageTypeBadSynthetic:
			var seqMsg *messaging.SequencedMessage
			
			// Handle both SyntheticMessage and BadSyntheticMessage
			if synthMsg, ok := msg.(*messaging.SyntheticMessage); ok {
				if sm, ok := synthMsg.Message.(*messaging.SequencedMessage); ok {
					seqMsg = sm
				}
			} else if badSynthMsg, ok := msg.(*messaging.BadSyntheticMessage); ok {
				if sm, ok := badSynthMsg.Message.(*messaging.SequencedMessage); ok {
					seqMsg = sm
				}
			}
			
			if seqMsg != nil {
				valid, reason, requestRecovery := cc.sequenceTracker.ValidateAndTrackSynthetic(seqMsg)
				cc.logger.Debug("[HEALING-DEBUG] Synthetic message validation",
					"source", seqMsg.Source,
					"sequence", seqMsg.Number,
					"valid", valid,
					"reason", reason,
					"request_recovery", requestRecovery)
				if requestRecovery {
					gapsDetected++
					cc.logger.Error("[HEALING-DEBUG] Gap detected in synthetic messages - RECOVERY NEEDED", 
						"source", seqMsg.Source,
						"sequence", seqMsg.Number,
						"reason", reason)
					// Notify recovery testing that recovery was triggered
					if cc.recoveryTestConfig != nil {
						cc.recoveryTestConfig.OnRecoveryTriggered()
					}
				}
				if valid {
					validMessages = append(validMessages, msg)
					cc.logger.Debug("[HEALING-DEBUG] Accepting synthetic message", 
						"sequence", seqMsg.Number, "source", seqMsg.Source)
				} else {
					cc.logger.Info("[HEALING-DEBUG] Filtering out synthetic message", 
						"sequence", seqMsg.Number, "reason", reason, "source", seqMsg.Source)
				}
			} else {
				// Synthetic without sequenced message - pass through
				validMessages = append(validMessages, msg)
			}

		case messaging.MessageTypeBlockAnchor:
			if anchorMsg, ok := msg.(*messaging.BlockAnchor); ok {
				if seqMsg, ok := anchorMsg.Anchor.(*messaging.SequencedMessage); ok {
					// Extract source and sequence for anchor validation
					valid, reason, requestRecovery := cc.sequenceTracker.ValidateAndTrackAnchor(anchorMsg, seqMsg.Source, seqMsg.Number)
					cc.logger.Debug("[HEALING-DEBUG] Anchor message validation",
						"source", seqMsg.Source,
						"sequence", seqMsg.Number,
						"valid", valid,
						"reason", reason,
						"request_recovery", requestRecovery)
					if requestRecovery {
						gapsDetected++
						cc.logger.Error("[HEALING-DEBUG] Gap detected in anchor messages - RECOVERY NEEDED",
							"source", seqMsg.Source,
							"sequence", seqMsg.Number,
							"reason", reason)
						// Notify recovery testing that recovery was triggered
						if cc.recoveryTestConfig != nil {
							cc.recoveryTestConfig.OnRecoveryTriggered()
						}
					}
					if valid {
						validMessages = append(validMessages, msg)
						cc.logger.Debug("[HEALING-DEBUG] Accepting anchor message", 
							"sequence", seqMsg.Number, "source", seqMsg.Source)
					} else {
						cc.logger.Info("[HEALING-DEBUG] Filtering out anchor message",
							"sequence", seqMsg.Number, "reason", reason, "source", seqMsg.Source)
					}
				} else {
					// Anchor without sequenced message - pass through
					validMessages = append(validMessages, msg)
				}
			}

		default:
			// Other message types pass through unchanged
			validMessages = append(validMessages, msg)
		}
	}

	if gapsDetected > 0 {
		cc.logger.Error("[HEALING-DEBUG] Processed inbound messages with gap detection - GAPS FOUND!",
			"total_messages", len(messages),
			"valid_messages", len(validMessages),
			"gaps_detected", gapsDetected,
			"filtered_out", len(messages)-len(validMessages))
	} else {
		cc.logger.Debug("[HEALING-DEBUG] Processed inbound messages - no gaps",
			"total_messages", len(messages),
			"valid_messages", len(validMessages))
	}

	// Return only valid messages - gap messages are filtered out
	// Source partitions will resend the missing messages
	return validMessages
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

	// Submit to dispatcher (with optional recovery testing)
	env := &messaging.Envelope{Messages: req.Messages}
	
	// RECOVERY TESTING: Drop messages randomly to test recovery mechanism
	// SECURITY: Only active with faucet + ACC_TEST_RECOVERY=true flag
	if cc.recoveryTestConfig != nil && len(env.Messages) > 0 {
		if cc.recoveryTestConfig.ShouldDropMessage(env.Messages[0]) {
			// Simulate network failure - don't submit to dispatcher
			// This will trigger retry logic and eventual recovery
			err := errors.UnknownError.With("RECOVERY TEST: Simulated network failure")
			delete(queue.PendingTx, txID)
			atomic.AddInt64(&cc.syntheticsErrors, 1)
			queue.FailureCount++
			cc.logger.Error("[HEALING-DEBUG] RECOVERY TEST: Simulated message drop - EXPECTING GAP DETECTION",
				"destination", req.Destination, "tx_id", txID, "type", destKey.Type,
				"message_type", env.Messages[0].Type())
			req.ResponseChan <- err
			return
		}
	}
	
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
	
	var err error
	
	// RECOVERY TESTING: Drop retry attempts randomly to test recovery mechanism
	// SECURITY: Only active with faucet + ACC_TEST_RECOVERY=true flag
	if cc.recoveryTestConfig != nil && len(env.Messages) > 0 {
		if cc.recoveryTestConfig.ShouldDropMessage(env.Messages[0]) {
			// Simulate retry failure
			err = errors.UnknownError.With("RECOVERY TEST: Simulated retry failure")
			cc.logger.Error("RECOVERY TEST: Simulated retry drop",
				"tx_id", pending.ID, "attempt", pending.AttemptNum, 
				"type", pending.DestKey.Type, "destination", pending.DestKey.Destination)
		} else {
			err = cc.dispatcher.Submit(pending.Context, pending.Destination, env)
		}
	} else {
		err = cc.dispatcher.Submit(pending.Context, pending.Destination, env)
	}

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



// RequestMissingTransactionsWithBatchProof removed - was using fake BatchProofRecoveryManager
// Real implementation should be added when proper batch recovery is implemented

// handleBatchRecoveryResponse removed - was processing fake recovery data

// HandleRecoveryRequest processes gap recovery by adjusting send height
// Simple implementation: requester wants transactions from FromNumber onwards,
// so we reset our send position to FromNumber-1 for that destination
func (cc *CrossChainConductor) HandleRecoveryRequest(req *RecoveryRequest) error {
	cc.logger.Error("[HEALING-DEBUG] HandleRecoveryRequest called - PROCESSING RECOVERY REQUEST",
		"requester", req.Requester,
		"fromNumber", req.FromNumber,
		"action", "reset_send_position")

	// Parse requester as destination URL
	requesterURL, err := url.Parse(req.Requester)
	if err != nil {
		return fmt.Errorf("invalid requester URL %s: %v", req.Requester, err)
	}

	// Get or create destination state
	cc.destinationsMu.Lock()
	destState, exists := cc.destinations[requesterURL.String()]
	if !exists {
		destState = &DestinationSendState{
			Destination: requesterURL,
			SentTxIndex: 0,
			CurrentTxIndex: 0,
		}
		cc.destinations[requesterURL.String()] = destState
	}

	// Reset send position to FromNumber-1 so next batch starts at FromNumber
	if req.FromNumber > 0 {
		destState.SentTxIndex = req.FromNumber - 1
	} else {
		destState.SentTxIndex = 0
	}
	cc.destinationsMu.Unlock()

	cc.logger.Error("[HEALING-DEBUG] Adjusted send position for gap recovery - READY TO RESEND", 
		"destination", req.Requester,
		"newSentIndex", destState.SentTxIndex,
		"willResendFrom", destState.SentTxIndex+1,
		"recovery_complete", "ready_for_next_batch")

	return nil
}

// SubmitAnchor submits an anchor for transmission
func (cc *CrossChainConductor) SubmitAnchor(req *AnchorRequest) error {
	destKey := cc.createDestinationKey(MessageTypeAnchor, req.Destination)

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

	// Simple recovery: no active sessions to track

	return health
}

// Helper function to get message type name
func (cc *CrossChainConductor) getMessageTypeName(t MessageType) string {
	switch t {
	case MessageTypeAnchor:
		return "anchor"
	case MessageTypeSynthetic:
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

// BatchProofRecoveryManager removed - was placeholder with fake data
// Real implementation should be added when needed

// CreateProofsForSyntheticTransactions creates optimized proofs for synthetic transactions
// This is the central entry point for all synthetic proof creation
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
			SourceChain: synthChain,
			RootChain:   rootChain,
		})
	}

	// Create collection proofs for each destination
	var allProofs []*protocol.AnnotatedReceipt
	for dest, requests := range destinationGroups {
		// Always use collection proof for crosschain operations
		mergedReq := cc.proofService.mergeSequences(requests)

		cc.logger.Info("Creating collection proof for synthetic transactions",
			"destination", dest,
			"count", len(requests),
			"sequences", mergedReq.Sequences)

		// Create single collection proof for all transactions to this destination
		resp, err := cc.proofService.CreateProof(ctx, mergedReq)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("failed to create collection proof for %s: %w", dest, err)
		}
		
		// Use the same collection proof for all transactions in this group
		for range requests {
			allProofs = append(allProofs, resp.Proof)
		}

		cc.logger.Info("Collection proof created successfully",
			"destination", dest,
			"proof_savings", resp.ProofSavings)
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

// GetRecoveryTestMetrics returns recovery testing metrics
// SECURITY: Only returns data if testing is enabled (faucet + flag)
func (cc *CrossChainConductor) GetRecoveryTestMetrics() map[string]interface{} {
	if cc.recoveryTestConfig == nil {
		return map[string]interface{}{
			"enabled": false,
			"reason":  "recovery testing not initialized",
		}
	}
	
	return cc.recoveryTestConfig.GetMetrics()
}

// SyntheticTransaction represents a synthetic transaction needing a proof
type SyntheticTransaction struct {
	Destination *url.URL
	SequenceNum uint64
	Sequence    uint64 // Alias for SequenceNum for compatibility
	ChainURL    *url.URL
	Hash        []byte
}

// Describe returns the partition description
func (cc *CrossChainConductor) Describe() *config.Describe {
	return cc.describe
}

// RequestBatchProofRecovery removed - was using fake BatchProofRecoveryManager
// Real implementation should be added when proper batch recovery is implemented
