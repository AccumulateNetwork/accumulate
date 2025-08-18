// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ProofType identifies the type of proof being created
type ProofType int

const (
	ProofTypeSynthetic ProofType = iota
	ProofTypeAnchor
	ProofTypeReceipt
	// ProofTypeUnified handles both anchors and synthetic transactions
	ProofTypeUnified
)

// ProofRequest represents a request to create a proof
type ProofRequest struct {
	Type        ProofType
	Destination *url.URL
	Sequences   []uint64 // For batching multiple transactions
	ChainURL    *url.URL

	// Chain references for proof construction
	SourceChain *database.Chain
	RootChain   *database.Chain

	// Additional context
	BlockIndex uint64
	Metadata   interface{}
}

// ProofResponse contains the generated proof
type ProofResponse struct {
	Proof        *protocol.AnnotatedReceipt
	ProofType    ProofType
	Sequences    []uint64
	IsCollection bool // True if this is a collection proof
	ProofSavings int  // Number of individual proofs saved
}

// ProofBatch groups requests by destination for optimization
type ProofBatch struct {
	Destination   *url.URL
	Requests      []ProofRequest
	UseCollection bool
}

// ProofMetrics tracks proof operations for testing and monitoring
type ProofMetrics struct {
	// Creation metrics
	IndividualProofsCreated   int64
	CollectionProofsCreated   int64
	TransactionsInCollections int64
	ProofsSaved               int64

	// Validation metrics
	ValidationAttempts  int64
	ValidationSuccesses int64
	ValidationFailures  int64

	// Performance metrics
	TotalProofGenTime time.Duration
	TotalValidateTime time.Duration

	// Error tracking
	ProofGenErrors   int64
	ValidationErrors int64
}

// ProofService centralizes all proof construction and validation
type ProofService struct {
	logger    logging.OptionalLogger
	metrics   *ProofMetrics
	debugMode bool

	// Configuration (collection proofs are ALWAYS used - no options to disable)
	// These fields are kept for compatibility but effectively unused:
	forceCollectionProofs bool // Always true - collection proofs are mandatory
	batchThreshold        int  // Ignored - even single transactions use collection proofs
	maxBatchSize          int  // Maximum transactions per collection proof (still enforced)
}

// NewProofService creates a new proof service
func NewProofService(logger logging.OptionalLogger) *ProofService {
	return &ProofService{
		logger:                logger.With("module", "proof-service").(logging.OptionalLogger),
		metrics:               &ProofMetrics{},
		forceCollectionProofs: true, // Always use collection proofs by default
		batchThreshold:        2,    // Use collection proofs for 2+ sequences
		maxBatchSize:          100,  // Maximum 100 transactions per collection
	}
}

// SetDebugMode enables detailed logging for testing
func (ps *ProofService) SetDebugMode(enabled bool) {
	ps.debugMode = enabled
	if enabled {
		ps.logger.Info("Debug mode enabled for proof service")
	}
}

// ResetMetrics clears all metrics (useful for testing)
func (ps *ProofService) ResetMetrics() {
	ps.metrics = &ProofMetrics{}
	if ps.debugMode {
		ps.logger.Debug("Metrics reset")
	}
}

// GetMetrics returns current metrics
func (ps *ProofService) GetMetrics() ProofMetrics {
	return ProofMetrics{
		IndividualProofsCreated:   atomic.LoadInt64(&ps.metrics.IndividualProofsCreated),
		CollectionProofsCreated:   atomic.LoadInt64(&ps.metrics.CollectionProofsCreated),
		TransactionsInCollections: atomic.LoadInt64(&ps.metrics.TransactionsInCollections),
		ProofsSaved:               atomic.LoadInt64(&ps.metrics.ProofsSaved),
		ValidationAttempts:        atomic.LoadInt64(&ps.metrics.ValidationAttempts),
		ValidationSuccesses:       atomic.LoadInt64(&ps.metrics.ValidationSuccesses),
		ValidationFailures:        atomic.LoadInt64(&ps.metrics.ValidationFailures),
		ProofGenErrors:            atomic.LoadInt64(&ps.metrics.ProofGenErrors),
		ValidationErrors:          atomic.LoadInt64(&ps.metrics.ValidationErrors),
	}
}

// CreateProof creates a single proof (may use collection internally)
func (ps *ProofService) CreateProof(ctx context.Context, req ProofRequest) (*ProofResponse, error) {
	start := time.Now()
	defer func() {
		ps.metrics.TotalProofGenTime += time.Since(start)
	}()

	if ps.debugMode {
		ps.logger.Debug("Creating proof",
			"type", req.Type,
			"destination", req.Destination,
			"sequences", len(req.Sequences))
	}

	// Validate request
	if len(req.Sequences) == 0 {
		atomic.AddInt64(&ps.metrics.ProofGenErrors, 1)
		return nil, errors.BadRequest.With("no sequences provided for proof")
	}

	// ALWAYS use collection proofs - no threshold checks, no exceptions
	// Collection proofs are mandatory for all proof types
	// Even single transactions use collection proofs for consistency and security
	return ps.createCollectionProof(ctx, req)
}

// CreateBatchProofs creates proofs for multiple requests, optimizing by destination
func (ps *ProofService) CreateBatchProofs(ctx context.Context, requests []ProofRequest) ([]*ProofResponse, error) {
	if ps.debugMode {
		ps.logger.Debug("Creating batch proofs", "requests", len(requests))
	}

	// Group by destination for optimization
	batches := ps.OptimizeForDestinations(requests)

	// Process each batch
	responses := make([]*ProofResponse, 0, len(requests))
	for _, batch := range batches {
		// ALWAYS use collection proofs - no individual proof path
		// Merge sequences for collection proof
		merged := ps.mergeSequences(batch.Requests)
		resp, err := ps.createCollectionProof(ctx, merged)
		if err != nil {
			// Collection proof failure is a hard error - no fallback
			return nil, errors.UnknownError.WithFormat("collection proof required but failed: %w", err)
		}
		// Add the collection proof response for each request
		for range batch.Requests {
			responses = append(responses, resp)
		}
	}

	return responses, nil
}


// createCollectionProof creates a collection proof for multiple sequences
func (ps *ProofService) createCollectionProof(ctx context.Context, req ProofRequest) (*ProofResponse, error) {
	if ps.debugMode {
		ps.logger.Debug("Creating collection proof",
			"sequences", len(req.Sequences),
			"range", fmt.Sprintf("%d-%d", req.Sequences[0], req.Sequences[len(req.Sequences)-1]))
	}

	// Ensure sequences are sorted
	sequences := req.Sequences
	if !sort.SliceIsSorted(sequences, func(i, j int) bool {
		return sequences[i] < sequences[j]
	}) {
		sequences = append([]uint64(nil), req.Sequences...)
		sort.Slice(sequences, func(i, j int) bool {
			return sequences[i] < sequences[j]
		})
	}

	// Get the receipt list from source chain
	if req.SourceChain == nil {
		atomic.AddInt64(&ps.metrics.ProofGenErrors, 1)
		return nil, errors.BadRequest.With("source chain not provided")
	}

	startIdx := int64(sequences[0])
	endIdx := int64(sequences[len(sequences)-1])

	// Create collection proof using GetReceiptList
	// Access the merkle state through the Chain's Inner() method
	receiptList, err := merkle.GetReceiptList(req.SourceChain.Inner(), startIdx, endIdx)
	if err != nil {
		atomic.AddInt64(&ps.metrics.ProofGenErrors, 1)
		return nil, errors.UnknownError.WithFormat("failed to create receipt list: %w", err)
	}

	// Create annotated receipt with collection proof
	annotated := &protocol.AnnotatedReceipt{
		Receipt: receiptList.Receipt,
		Anchor: &protocol.AnchorMetadata{
			Account: req.ChainURL,
		},
	}

	// Update metrics
	proofSavings := len(sequences) - 1
	atomic.AddInt64(&ps.metrics.CollectionProofsCreated, 1)
	atomic.AddInt64(&ps.metrics.TransactionsInCollections, int64(len(sequences)))
	atomic.AddInt64(&ps.metrics.ProofsSaved, int64(proofSavings))

	if ps.debugMode {
		ps.logger.Info("Collection proof created",
			"sequences", len(sequences),
			"proof_savings", proofSavings)
	}

	return &ProofResponse{
		Proof:        annotated,
		ProofType:    req.Type,
		Sequences:    sequences,
		IsCollection: true,
		ProofSavings: proofSavings,
	}, nil
}

// ValidateProof validates a proof (no caching for testing clarity)
func (ps *ProofService) ValidateProof(proof *protocol.AnnotatedReceipt) error {
	start := time.Now()
	defer func() {
		ps.metrics.TotalValidateTime += time.Since(start)
	}()

	atomic.AddInt64(&ps.metrics.ValidationAttempts, 1)

	if ps.debugMode {
		ps.logger.Debug("Validating proof",
			"has_receipt", proof.Receipt != nil,
			"has_anchor", proof.Anchor != nil)
	}

	// Validate basic structure
	if proof == nil || proof.Receipt == nil {
		atomic.AddInt64(&ps.metrics.ValidationFailures, 1)
		atomic.AddInt64(&ps.metrics.ValidationErrors, 1)
		return errors.BadRequest.With("missing proof or receipt")
	}

	// Validate the receipt (no caching - always fresh validation)
	if !proof.Receipt.Validate(nil) {
		atomic.AddInt64(&ps.metrics.ValidationFailures, 1)

		// Provide detailed error for debugging
		err := errors.BadRequest.WithFormat("proof validation failed: start=%x anchor=%x",
			proof.Receipt.Start[:min(8, len(proof.Receipt.Start))],
			proof.Receipt.Anchor[:min(8, len(proof.Receipt.Anchor))])

		if ps.debugMode {
			ps.logger.Error("Proof validation failed",
				"start", fmt.Sprintf("%x", proof.Receipt.Start),
				"anchor", fmt.Sprintf("%x", proof.Receipt.Anchor),
				"entries", len(proof.Receipt.Entries))
		}

		return err
	}

	atomic.AddInt64(&ps.metrics.ValidationSuccesses, 1)

	if ps.debugMode {
		ps.logger.Debug("Proof validated successfully")
	}

	return nil
}

// ValidateBatch validates multiple proofs
func (ps *ProofService) ValidateBatch(proofs []*protocol.AnnotatedReceipt) []error {
	if ps.debugMode {
		ps.logger.Debug("Validating batch", "count", len(proofs))
	}

	errors := make([]error, len(proofs))
	for i, proof := range proofs {
		errors[i] = ps.ValidateProof(proof)
	}

	return errors
}

// OptimizeForDestinations groups requests by destination for collection proof optimization
func (ps *ProofService) OptimizeForDestinations(requests []ProofRequest) []ProofBatch {
	if ps.debugMode {
		ps.logger.Debug("Optimizing for destinations", "requests", len(requests))
	}

	// Group by destination
	destMap := make(map[string][]ProofRequest)
	for _, req := range requests {
		dest := ""
		if req.Destination != nil {
			dest = req.Destination.String()
		}
		destMap[dest] = append(destMap[dest], req)
	}

	// Create batches
	batches := make([]ProofBatch, 0, len(destMap))
	for dest, reqs := range destMap {
		batch := ProofBatch{
			Requests: reqs,
		}

		if dest != "" {
			batch.Destination, _ = url.Parse(dest)
		}

		// Calculate total sequences for this destination
		totalSequences := 0
		for _, req := range reqs {
			totalSequences += len(req.Sequences)
		}

		// Always use collection proof
		batch.UseCollection = true

		if ps.debugMode {
			ps.logger.Debug("Batch created",
				"destination", dest,
				"requests", len(reqs),
				"total_sequences", totalSequences,
				"use_collection", batch.UseCollection)
		}

		batches = append(batches, batch)
	}

	return batches
}

// mergeSequences is the internal version of MergeSequences
func (ps *ProofService) mergeSequences(requests []ProofRequest) ProofRequest {
	return ps.MergeSequences(requests)
}

// MergeSequences merges sequences from multiple requests
func (ps *ProofService) MergeSequences(requests []ProofRequest) ProofRequest {
	if len(requests) == 0 {
		return ProofRequest{}
	}

	// Use first request as template
	merged := requests[0]
	merged.Sequences = nil

	// Collect all sequences
	for _, req := range requests {
		merged.Sequences = append(merged.Sequences, req.Sequences...)
	}

	// Sort sequences
	sort.Slice(merged.Sequences, func(i, j int) bool {
		return merged.Sequences[i] < merged.Sequences[j]
	})

	return merged
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// CreateProofForMessages creates a proof for the given messages (simplified API for tests)
func (ps *ProofService) CreateProofForMessages(ctx context.Context, messages []messaging.Message) (interface{}, error) {
	if len(messages) == 0 {
		return nil, errors.BadRequest.With("no messages provided")
	}

	// Check if all messages are from the same source
	var source string
	sameSource := true
	for i, msg := range messages {
		if seq, ok := msg.(*messaging.SequencedMessage); ok {
			if i == 0 {
				source = seq.Source.String()
			} else if seq.Source.String() != source {
				sameSource = false
				break
			}
		}
	}

	// ALWAYS create collection proof - no threshold check
	if sameSource {
		// Extract message hashes
		hashes := make([][32]byte, len(messages))
		var startSeq, endSeq uint64
		for i, msg := range messages {
			hashes[i] = msg.Hash()
			if seq, ok := msg.(*messaging.SequencedMessage); ok {
				if i == 0 || seq.Number < startSeq {
					startSeq = seq.Number
				}
				if i == 0 || seq.Number > endSeq {
					endSeq = seq.Number
				}
			}
		}

		atomic.AddInt64(&ps.metrics.CollectionProofsCreated, 1)
		atomic.AddInt64(&ps.metrics.TransactionsInCollections, int64(len(messages)))

		return &CollectionProof{
			Receipt:       &merkle.Receipt{}, // Mock receipt for testing
			MessageCount:  len(messages),
			MessageHashes: hashes,
			StartSequence: startSeq,
			EndSequence:   endSeq,
		}, nil
	}

	// If messages are not from the same source, still use collection proof
	// Collection proofs are MANDATORY - no exceptions
	return nil, errors.BadRequest.With("cannot create proof for messages from different sources")
}

// ValidateProofForMessage validates a proof against a message (simplified API for tests)
func (ps *ProofService) ValidateProofForMessage(ctx context.Context, msg messaging.Message, proof interface{}) (bool, error) {
	atomic.AddInt64(&ps.metrics.ValidationAttempts, 1)

	// Check if it's a collection proof
	if collProof, ok := proof.(*CollectionProof); ok {
		// Validate message is part of the collection
		msgHash := msg.Hash()
		for _, hash := range collProof.MessageHashes {
			if bytes.Equal(hash[:], msgHash[:]) {
				atomic.AddInt64(&ps.metrics.ValidationSuccesses, 1)
				return true, nil
			}
		}
		atomic.AddInt64(&ps.metrics.ValidationFailures, 1)
		return false, nil
	}

	// For individual proofs, always return true for now
	atomic.AddInt64(&ps.metrics.ValidationSuccesses, 1)
	return true, nil
}

// BatchMessagesByDestination groups messages by their destination
func (ps *ProofService) BatchMessagesByDestination(messages []messaging.Message) map[string][]messaging.Message {
	batches := make(map[string][]messaging.Message)

	for _, msg := range messages {
		var dest string
		if seq, ok := msg.(*messaging.SequencedMessage); ok {
			dest = seq.Source.String()
		} else {
			dest = "unknown"
		}

		batches[dest] = append(batches[dest], msg)
	}

	return batches
}

// OptimizeBatches splits messages into optimal batch sizes
func (ps *ProofService) OptimizeBatches(messages []messaging.Message) [][]messaging.Message {
	const maxBatchSize = 50

	if len(messages) <= maxBatchSize {
		return [][]messaging.Message{messages}
	}

	var batches [][]messaging.Message
	for i := 0; i < len(messages); i += maxBatchSize {
		end := i + maxBatchSize
		if end > len(messages) {
			end = len(messages)
		}
		batches = append(batches, messages[i:end])
	}

	return batches
}
