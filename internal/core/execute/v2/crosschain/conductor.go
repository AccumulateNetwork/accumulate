// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
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
	// Configuration
	config ConductorConfig

	// Infrastructure
	dispatcher execute.Dispatcher
	logger     logging.OptionalLogger
	Describe   execute.DescribeShim // Partition description

	// Async processing
	syntheticChan chan *SyntheticRequest
	retryChan     chan *PendingTransmission
	stopChan      chan struct{}
	wg            sync.WaitGroup

	// Per-destination blocking and tracking (legacy - being replaced by destinationStates)
	destinationQueues map[DestinationKey]*DestinationQueue
	queuesMutex       sync.RWMutex
	maxRetries        int
	retryDelay        time.Duration
	txIDCounter       int64

	// NEW: Simple index-based tracking per destination for gap recovery
	destinationStates map[string]*DestinationSendState
	statesMutex       sync.RWMutex

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
		config: ConductorConfig{
			ForceCollectionProofs:  true, // Always use collection proofs
			CollectionMaxBatchSize: 100,  // Maximum 100 transactions per collection
		},
		dispatcher:         dispatcher,
		logger:             logger.With("module", "crosschain-conductor").(logging.OptionalLogger),
		syntheticChan:      make(chan *SyntheticRequest, 100),   // Buffered channel for async processing
		retryChan:          make(chan *PendingTransmission, 50), // Retry queue
		stopChan:           make(chan struct{}),
		destinationQueues:  make(map[DestinationKey]*DestinationQueue),
		destinationStates:  make(map[string]*DestinationSendState), // NEW: Index-based tracking
		maxRetries:         3,               // Retry failed transmissions up to 3 times
		retryDelay:         2 * time.Second, // Wait 2 seconds between retries
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

	// Log configuration
	cc.logger.Info("CrossChain Conductor started with collection proofs active",
		"force_collection_proofs", cc.config.ForceCollectionProofs,
		"max_batch_size", cc.config.CollectionMaxBatchSize)

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
	msgType MessageType,
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
	msgType MessageType,
	missingSequences []uint64,
	chainURL *url.URL,
) error {
	if cc.batchProofManager == nil {
		return errors.NotReady.With("batch proof recovery manager not initialized")
	}

	// Convert MessageType to RecoveryType
	var recoveryType RecoveryType
	switch msgType {
	case MessageTypeAnchor:
		recoveryType = RecoveryTypeAnchor
	case MessageTypeSynthetic:
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

// BatchProofRecoveryManager placeholder for the collection proof functionality
// This would contain the full implementation from batch_proof_recovery.go
type BatchProofRecoveryManager struct {
	conductor      *CrossChainConductor
	logger         logging.OptionalLogger
	// batchThreshold int // Reserved for future use
	maxBatchSize   int
	totalRequests  int64
	batchRequests  int64
	proofSavings   int64
}

func NewBatchProofRecoveryManager(conductor *CrossChainConductor, logger logging.OptionalLogger) *BatchProofRecoveryManager {
	return &BatchProofRecoveryManager{
		conductor:    conductor,
		logger:       logger.With("module", "batch-recovery").(logging.OptionalLogger),
		maxBatchSize: 100, // Maximum 100 transactions per batch
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
		// Always use collection proofs
		atomic.AddInt64(&brm.batchRequests, 1)
		atomic.AddInt64(&brm.proofSavings, int64(response.ProofSavings))

		if req.Callback != nil {
			req.Callback(response)
		}
	}()
}

func (brm *BatchProofRecoveryManager) GetMetrics() map[string]interface{} {
	return map[string]interface{}{
		"total_requests": atomic.LoadInt64(&brm.totalRequests),
		"batch_requests": atomic.LoadInt64(&brm.batchRequests),
		"proof_savings":  atomic.LoadInt64(&brm.proofSavings),
		"max_batch_size": brm.maxBatchSize,
	}
}

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

	// Create optimized proofs for each destination
	var allProofs []*protocol.AnnotatedReceipt
	for dest, requests := range destinationGroups {
		// Always use collection proof (no threshold check)
		// Merge sequences for collection proof
		mergedReq := cc.proofService.MergeSequences(requests)

		cc.logger.Info("Creating collection proof for synthetic transactions",
			"destination", dest,
			"count", len(requests),
			"sequences", mergedReq.Sequences)

		// Create single collection proof for all transactions to this destination
		resp, err := cc.proofService.CreateProof(ctx, mergedReq)
		if err != nil {
			// Collection proof creation should always succeed
			// Log the error but continue (no fallback)
			cc.logger.Error("Collection proof creation failed unexpectedly",
				"destination", dest,
				"error", err)
			// Just return the error - no fallback
			return nil, errors.UnknownError.WithFormat("failed to create collection proof: %w", err)
		}

		// Use the same collection proof for all transactions in this group
		for range requests {
			allProofs = append(allProofs, resp.Proof)
		}

		cc.logger.Info("Collection proof created successfully",
			"destination", dest,
			"transactions", len(requests),
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

// SyntheticTransaction represents a synthetic transaction needing a proof
type SyntheticTransaction struct {
	Destination *url.URL
	SequenceNum uint64
	ChainURL    *url.URL
	Hash        []byte
}
