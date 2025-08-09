package main

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// BatchProofRecoveryManager uses collection proofs for efficient batch recovery
type BatchProofRecoveryManager struct {
	conductor *CrossChainConductor
	logger    logging.OptionalLogger
	client    interface{} // API client interface
	db        interface{} // Database interface

	// Batch configuration
	batchThreshold int           // Minimum transactions before using batch proof
	maxBatchSize   int           // Maximum transactions per batch
	proofTimeout   time.Duration // Timeout for proof generation

	// Active recovery sessions
	activeRecovery map[string]*BatchRecoverySession
	recoveryQueue  chan *BatchRecoveryRequest
	mu             sync.RWMutex

	// Metrics
	totalRequests    int64
	batchRequests    int64
	individualProofs int64
	proofSavings     int64 // Number of individual proofs saved
}

// BatchRecoveryRequest represents a request for batch recovery
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

// BatchRecoverySession tracks an ongoing batch recovery
type BatchRecoverySession struct {
	PartitionID string
	Requests    []*BatchRecoveryRequest
	StartTime   time.Time
	LastUpdate  time.Time
	Status      BatchRecoveryStatus
}

type BatchRecoveryStatus int

const (
	BatchRecoveryPending BatchRecoveryStatus = iota
	BatchRecoveryGeneratingProof
	BatchRecoveryComplete
	BatchRecoveryFailed
)

func NewBatchProofRecoveryManager(conductor *CrossChainConductor, logger logging.OptionalLogger) *BatchProofRecoveryManager {
	return &BatchProofRecoveryManager{
		conductor:      conductor,
		logger:         logger.With("module", "batch-recovery"),
		batchThreshold: 2,   // Use batch proof when >= 2 transactions
		maxBatchSize:   100, // Maximum 100 transactions per batch
		proofTimeout:   30 * time.Second,
		activeRecovery: make(map[string]*BatchRecoverySession),
		recoveryQueue:  make(chan *BatchRecoveryRequest, 1000),
	}
}

func (brm *BatchProofRecoveryManager) Start() {
	brm.logger.Info("Starting batch proof recovery manager")

	// Start recovery processor
	go brm.processRecoveryQueue()

	// Start batch optimizer
	go brm.optimizeBatches()
}

func (brm *BatchProofRecoveryManager) Stop() {
	close(brm.recoveryQueue)
	brm.logger.Info("Batch proof recovery manager stopped")
}

// RequestBatchRecovery requests recovery with automatic batch optimization
func (brm *BatchProofRecoveryManager) RequestBatchRecovery(req *BatchRecoveryRequest) {
	brm.recoveryQueue <- req
}

// processRecoveryQueue processes incoming recovery requests
func (brm *BatchProofRecoveryManager) processRecoveryQueue() {
	for req := range brm.recoveryQueue {
		brm.handleRecoveryRequest(req)
	}
}

func (brm *BatchProofRecoveryManager) handleRecoveryRequest(req *BatchRecoveryRequest) {
	brm.mu.Lock()
	defer brm.mu.Unlock()

	sessionKey := fmt.Sprintf("%s-%s", req.PartitionID, req.Type.String())

	// Check if we should use batch processing
	if len(req.MissingSequences) >= brm.batchThreshold {
		brm.logger.Info("Using batch proof for recovery",
			"partition", req.PartitionID,
			"sequences", len(req.MissingSequences),
			"threshold", brm.batchThreshold)

		go brm.processBatchRecovery(req)
		brm.batchRequests++
	} else {
		brm.logger.Info("Using individual proofs for recovery",
			"partition", req.PartitionID,
			"sequences", len(req.MissingSequences))

		go brm.processIndividualRecovery(req)
		brm.individualProofs++
	}

	brm.totalRequests++
}

// processBatchRecovery handles recovery using collection proofs
func (brm *BatchProofRecoveryManager) processBatchRecovery(req *BatchRecoveryRequest) {
	brm.logger.Info("Processing batch recovery",
		"partition", req.PartitionID,
		"type", req.Type,
		"count", len(req.MissingSequences))

	startTime := time.Now()

	// Sort sequences for efficient batch processing
	sequences := make([]uint64, len(req.MissingSequences))
	copy(sequences, req.MissingSequences)
	sort.Slice(sequences, func(i, j int) bool {
		return sequences[i] < sequences[j]
	})

	// Process in batches if too many sequences
	batchSize := min(len(sequences), brm.maxBatchSize)

	for i := 0; i < len(sequences); i += batchSize {
		end := min(i+batchSize, len(sequences))
		batch := sequences[i:end]

		response, err := brm.generateCollectionProof(req, batch)
		if err != nil {
			brm.logger.Error("Failed to generate collection proof",
				"partition", req.PartitionID,
				"batch", fmt.Sprintf("%d-%d", batch[0], batch[len(batch)-1]),
				"error", err)

			// Fallback to individual proofs
			brm.processIndividualRecoveryBatch(req, batch)
			continue
		}

		// Calculate proof savings
		proofSavings := len(batch) - 1 // We made 1 proof instead of N proofs
		brm.proofSavings += int64(proofSavings)

		response.ProofSavings = proofSavings
		response.ProofGenerated = time.Now()

		brm.logger.Info("Generated collection proof",
			"partition", req.PartitionID,
			"batch_size", len(batch),
			"proof_savings", proofSavings,
			"generation_time", time.Since(startTime))

		// Send response
		if req.Callback != nil {
			req.Callback(response)
		}
	}
}

// generateCollectionProof creates a ReceiptList proof for a batch of transactions
func (brm *BatchProofRecoveryManager) generateCollectionProof(req *BatchRecoveryRequest, sequences []uint64) (*BatchRecoveryResponse, error) {
	response := &BatchRecoveryResponse{
		PartitionID: req.PartitionID,
		Type:        req.Type,
		BatchSize:   len(sequences),
	}

	// Get the Merkle chain for the appropriate ledger
	chain, err := brm.getChainForRecovery(req.PartitionID, req.Type)
	if err != nil {
		return nil, fmt.Errorf("failed to get chain: %w", err)
	}

	// Determine range for collection proof
	startIdx := int64(sequences[0])
	endIdx := int64(sequences[len(sequences)-1])

	brm.logger.Debug("Creating collection proof",
		"partition", req.PartitionID,
		"start_idx", startIdx,
		"end_idx", endIdx,
		"count", len(sequences))

	// Generate collection proof using ReceiptList
	receiptList, err := merkle.GetReceiptList(chain, startIdx, endIdx)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ReceiptList: %w", err)
	}

	response.CollectionProof = receiptList
	response.TransactionHashes = make([][]byte, len(receiptList.Elements))
	copy(response.TransactionHashes, receiptList.Elements)

	// Retrieve transaction data for each sequence
	transactions := make([]*RecoveredTransaction, 0, len(sequences))

	for _, seq := range sequences {
		tx, err := brm.getTransactionData(req.PartitionID, req.Type, seq)
		if err != nil {
			brm.logger.Warn("Failed to retrieve transaction data",
				"partition", req.PartitionID,
				"sequence", seq,
				"error", err)
			continue
		}

		transactions = append(transactions, tx)
	}

	response.Transactions = transactions

	brm.logger.Info("Collection proof generated successfully",
		"partition", req.PartitionID,
		"proof_elements", len(response.TransactionHashes),
		"transactions", len(transactions))

	return response, nil
}

// processIndividualRecovery handles recovery using individual proofs (fallback)
func (brm *BatchProofRecoveryManager) processIndividualRecovery(req *BatchRecoveryRequest) {
	brm.logger.Info("Processing individual recovery",
		"partition", req.PartitionID,
		"count", len(req.MissingSequences))

	// Process each transaction with its own proof
	brm.processIndividualRecoveryBatch(req, req.MissingSequences)
}

func (brm *BatchProofRecoveryManager) processIndividualRecoveryBatch(req *BatchRecoveryRequest, sequences []uint64) {
	for _, seq := range sequences {
		// Generate individual proof for each transaction
		// This is the traditional approach - less efficient
		tx, err := brm.getTransactionWithProof(req.PartitionID, req.Type, seq)
		if err != nil {
			brm.logger.Error("Failed to get transaction with proof",
				"partition", req.PartitionID,
				"sequence", seq,
				"error", err)
			continue
		}

		// Send individual response
		response := &BatchRecoveryResponse{
			PartitionID:  req.PartitionID,
			Type:         req.Type,
			Transactions: []*RecoveredTransaction{tx},
			BatchSize:    1,
			ProofSavings: 0, // No savings with individual proofs
		}

		if req.Callback != nil {
			req.Callback(response)
		}
	}
}

// optimizeBatches periodically optimizes pending recovery requests into batches
func (brm *BatchProofRecoveryManager) optimizeBatches() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		brm.mu.Lock()

		// Look for sessions that can be batched together
		for sessionKey, session := range brm.activeRecovery {
			if session.Status == BatchRecoveryPending &&
				time.Since(session.LastUpdate) > 50*time.Millisecond &&
				len(session.Requests) >= brm.batchThreshold {

				brm.logger.Debug("Optimizing batch",
					"session", sessionKey,
					"requests", len(session.Requests))

				// Merge requests into optimized batches
				go brm.processMergedBatch(session)
				session.Status = BatchRecoveryGeneratingProof
			}
		}

		brm.mu.Unlock()
	}
}

func (brm *BatchProofRecoveryManager) processMergedBatch(session *BatchRecoverySession) {
	// Merge all sequences from the session requests
	allSequences := make([]uint64, 0)

	for _, req := range session.Requests {
		allSequences = append(allSequences, req.MissingSequences...)
	}

	// Remove duplicates and sort
	sequenceMap := make(map[uint64]bool)
	for _, seq := range allSequences {
		sequenceMap[seq] = true
	}

	uniqueSequences := make([]uint64, 0, len(sequenceMap))
	for seq := range sequenceMap {
		uniqueSequences = append(uniqueSequences, seq)
	}

	sort.Slice(uniqueSequences, func(i, j int) bool {
		return uniqueSequences[i] < uniqueSequences[j]
	})

	brm.logger.Info("Processing merged batch",
		"partition", session.PartitionID,
		"original_requests", len(session.Requests),
		"unique_sequences", len(uniqueSequences))

	// Create merged request
	mergedReq := &BatchRecoveryRequest{
		PartitionID:      session.PartitionID,
		Type:             session.Requests[0].Type, // Assume all same type
		MissingSequences: uniqueSequences,
		ChainURL:         session.Requests[0].ChainURL,
		RequestTime:      session.StartTime,
		Callback: func(response *BatchRecoveryResponse) {
			// Distribute response to all original callbacks
			for _, req := range session.Requests {
				if req.Callback != nil {
					req.Callback(response)
				}
			}
		},
	}

	// Process the merged batch
	brm.processBatchRecovery(mergedReq)

	// Clean up session
	brm.mu.Lock()
	sessionKey := fmt.Sprintf("%s-%s", session.PartitionID, session.Requests[0].Type.String())
	delete(brm.activeRecovery, sessionKey)
	brm.mu.Unlock()
}

// Helper methods (would integrate with actual chain/database interfaces)

func (brm *BatchProofRecoveryManager) getChainForRecovery(partitionID string, recoveryType RecoveryType) (*merkle.Chain, error) {
	// This would return the appropriate Merkle chain based on the recovery type
	// For anchors: return anchor chain
	// For synthetic transactions: return synthetic chain

	brm.logger.Debug("Getting chain for recovery",
		"partition", partitionID,
		"type", recoveryType)

	// Placeholder - would implement actual chain retrieval
	return nil, fmt.Errorf("chain retrieval not implemented")
}

func (brm *BatchProofRecoveryManager) getTransactionData(partitionID string, recoveryType RecoveryType, sequence uint64) (*RecoveredTransaction, error) {
	// This would retrieve the actual transaction data from the ledger
	return &RecoveredTransaction{
		Hash:        []byte(fmt.Sprintf("hash-%d", sequence)),
		SequenceNum: sequence,
		Timestamp:   time.Now(),
		Type:        recoveryType.String(),
		Data:        []byte(fmt.Sprintf("tx-data-%d", sequence)),
	}, nil
}

func (brm *BatchProofRecoveryManager) getTransactionWithProof(partitionID string, recoveryType RecoveryType, sequence uint64) (*RecoveredTransaction, error) {
	// This would generate an individual Merkle proof for the transaction
	return brm.getTransactionData(partitionID, recoveryType, sequence)
}

// GetMetrics returns performance metrics for the batch proof system
func (brm *BatchProofRecoveryManager) GetMetrics() map[string]interface{} {
	brm.mu.RLock()
	defer brm.mu.RUnlock()

	return map[string]interface{}{
		"total_requests":     brm.totalRequests,
		"batch_requests":     brm.batchRequests,
		"individual_proofs":  brm.individualProofs,
		"proof_savings":      brm.proofSavings,
		"batch_threshold":    brm.batchThreshold,
		"max_batch_size":     brm.maxBatchSize,
		"active_sessions":    len(brm.activeRecovery),
		"efficiency_percent": brm.calculateEfficiency(),
	}
}

func (brm *BatchProofRecoveryManager) calculateEfficiency() float64 {
	if brm.totalRequests == 0 {
		return 0
	}
	return float64(brm.proofSavings) / float64(brm.totalRequests) * 100
}

// Recovery types
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
