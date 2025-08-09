// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"
	"fmt"
	"sort"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ProofType identifies the type of proof being created
type ProofType int

const (
	ProofTypeSynthetic ProofType = iota
	ProofTypeAnchor
	ProofTypeReceipt
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

	// Configuration
	batchThreshold int // Minimum transactions for collection proof (default: 2)
	maxBatchSize   int // Maximum transactions per collection proof
}

// NewProofService creates a new proof service
func NewProofService(logger logging.OptionalLogger) *ProofService {
	return &ProofService{
		logger:         logging.OptionalLogger{L: logger.L.With("module", "proof-service")},
		metrics:        &ProofMetrics{},
		batchThreshold: 2,   // Use collection proofs for 2+ transactions
		maxBatchSize:   100, // Maximum 100 transactions per collection
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

	// Decide whether to use collection proof
	if len(req.Sequences) >= ps.batchThreshold {
		return ps.createCollectionProof(ctx, req)
	}

	return ps.createIndividualProof(ctx, req)
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
		if batch.UseCollection {
			// Merge sequences for collection proof
			merged := ps.mergeSequences(batch.Requests)
			resp, err := ps.createCollectionProof(ctx, merged)
			if err != nil {
				// Fallback to individual proofs
				ps.logger.Info("Collection proof failed, falling back to individual",
					"destination", batch.Destination,
					"error", err)
				for _, req := range batch.Requests {
					resp, err := ps.createIndividualProof(ctx, req)
					if err != nil {
						return nil, err
					}
					responses = append(responses, resp)
				}
			} else {
				// Add the collection proof response for each request
				for range batch.Requests {
					responses = append(responses, resp)
				}
			}
		} else {
			// Process individually
			for _, req := range batch.Requests {
				resp, err := ps.createIndividualProof(ctx, req)
				if err != nil {
					return nil, err
				}
				responses = append(responses, resp)
			}
		}
	}

	return responses, nil
}

// createIndividualProof creates a single traditional proof
func (ps *ProofService) createIndividualProof(ctx context.Context, req ProofRequest) (*ProofResponse, error) {
	if ps.debugMode {
		ps.logger.Debug("Creating individual proof",
			"sequence", req.Sequences[0])
	}

	// Get the receipt from source chain
	if req.SourceChain == nil {
		atomic.AddInt64(&ps.metrics.ProofGenErrors, 1)
		return nil, errors.BadRequest.With("source chain not provided")
	}

	sourceReceipt, err := req.SourceChain.Receipt(int64(req.Sequences[0]), req.SourceChain.Height()-1)
	if err != nil {
		atomic.AddInt64(&ps.metrics.ProofGenErrors, 1)
		return nil, errors.UnknownError.WithFormat("failed to create source receipt: %w", err)
	}

	// Combine with root chain if provided
	var finalReceipt *merkle.Receipt
	if req.RootChain != nil {
		rootReceipt, err := req.RootChain.Receipt(req.SourceChain.Height()-1, req.RootChain.Height()-1)
		if err != nil {
			atomic.AddInt64(&ps.metrics.ProofGenErrors, 1)
			return nil, errors.UnknownError.WithFormat("failed to create root receipt: %w", err)
		}

		finalReceipt, err = sourceReceipt.Combine(rootReceipt)
		if err != nil {
			atomic.AddInt64(&ps.metrics.ProofGenErrors, 1)
			return nil, errors.UnknownError.WithFormat("failed to combine receipts: %w", err)
		}
	} else {
		finalReceipt = sourceReceipt
	}

	// Create annotated receipt
	annotated := &protocol.AnnotatedReceipt{
		Receipt: finalReceipt,
		Anchor: &protocol.AnchorMetadata{
			Account: req.ChainURL,
		},
	}

	atomic.AddInt64(&ps.metrics.IndividualProofsCreated, 1)

	return &ProofResponse{
		Proof:        annotated,
		ProofType:    req.Type,
		Sequences:    req.Sequences,
		IsCollection: false,
		ProofSavings: 0,
	}, nil
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
	// Access the merkle state through the Chain's internal methods
	// This is a simplified placeholder - actual implementation would need proper Chain method
	receiptList, err := merkle.GetReceiptList(nil, startIdx, endIdx) // TODO: Get merkle state properly
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

		// Use collection proof if we have enough sequences
		batch.UseCollection = totalSequences >= ps.batchThreshold

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

// mergeSequences merges sequences from multiple requests
func (ps *ProofService) mergeSequences(requests []ProofRequest) ProofRequest {
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
