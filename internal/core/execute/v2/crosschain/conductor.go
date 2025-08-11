// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// MessageType is imported from unified_transport.go - we use the same type system

// Legacy constants for compatibility - mapped to unified MessageType
const (
	ConductorMessageTypeAnchor    = MessageTypeAnchor
	ConductorMessageTypeSynthetic = MessageTypeSynthetic
	ConductorMessageTypeOther     = MessageTypeBlockSummary // Map "Other" to BlockSummary for now
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

// CrossChainConductor handles async processing of cross-partition transactions
type CrossChainConductor struct {
	// Infrastructure
	dispatcher execute.Dispatcher
	logger     logging.OptionalLogger
	Describe   execute.DescribeShim // Partition description

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
	
	// Sequence tracker for gap detection (simplified, no buffering)
	sequenceTracker *SimpleSequenceTracker
}

// SyntheticRequest represents a request to submit synthetic transactions
type SyntheticRequest struct {
	Messages     []messaging.Message
	Destination  *url.URL
	Context      context.Context
	SubmittedAt  time.Time
	ResponseChan chan error
}

// AnchorRequest represents a request to submit an anchor
type AnchorRequest struct {
	Anchor      protocol.AnchorBody
	Source      *url.URL
	Destination *url.URL
	Sequence    uint64
	SourceChain *url.URL
	RootChain   *url.URL
	BlockIndex  uint64
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
	
	// Initialize simplified sequence tracker (no buffering, immediate recovery)
	cc.sequenceTracker = NewSimpleSequenceTracker(cc, cc.logger)

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

// Stop gracefully stops the conductor
func (cc *CrossChainConductor) Stop() {
	cc.logger.Info("Stopping CrossChainConductor")

	// Signal all goroutines to stop
	close(cc.stopChan)

	// Stop batch proof manager
	if cc.batchProofManager != nil {
		cc.batchProofManager.Stop()
	}

	// Stop recovery manager if it has a Stop method
	// Note: RecoveryManager may not have a Stop method yet

	// Wait for goroutines to finish
	cc.wg.Wait()

	// Clean up pending transmissions
	cc.queuesMutex.Lock()
	for _, queue := range cc.destinationQueues {
		queue.mu.Lock()

		// Fail all pending transmissions
		for txID, pending := range queue.PendingTx {
			if pending.Callback != nil {
				select {
				case pending.Callback <- errors.InternalError.With("conductor stopped"):
				default:
					// Channel might be closed
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