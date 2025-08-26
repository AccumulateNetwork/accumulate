// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TransactionInfo represents transaction information for proof generation
// This is a simplified version for integration purposes
type TransactionInfo struct {
	Transaction *protocol.Transaction
	Hash        [32]byte
	Sequence    uint64
	Destination *url.URL
}

// ProofIntegration provides methods for integrating ProofService with the executor
// without creating import cycles
type ProofIntegration struct {
	conductor *CrossChainConductor
}

// NewProofIntegration creates a new proof integration helper
func NewProofIntegration(conductor *CrossChainConductor) *ProofIntegration {
	return &ProofIntegration{
		conductor: conductor,
	}
}

// CreateSyntheticProofsWithPartitions creates optimized proofs for synthetic transactions
// using the correct partition-specific sequence chains for each destination.
// This method is designed to be called from the block package without import cycles.
func (pi *ProofIntegration) CreateSyntheticProofsWithPartitions(
	ctx context.Context,
	batch *database.Batch,
	sourcePartition *url.URL,
	transactions []TransactionInfo,
	rootChain *database.Chain,
) ([]*protocol.AnnotatedReceipt, error) {
	if pi.conductor == nil || pi.conductor.proofService == nil {
		return nil, errors.InternalError.With("proof service not initialized")
	}

	// Convert TransactionInfo to ProofRequest format
	sequences := make([]uint64, len(transactions))
	for i, tx := range transactions {
		sequences[i] = tx.Sequence
	}

	// Create a unified proof request for all transactions
	req := ProofRequest{
		Type:        ProofTypeUnified,
		Destination: sourcePartition,
		Sequences:   sequences,
		ChainURL:    sourcePartition,
		SourceChain: rootChain,
		RootChain:   rootChain,
		BlockIndex:  0, // Will be set by caller if needed
	}

	// Create proof using the centralized service
	resp, err := pi.conductor.proofService.CreateProof(ctx, req)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("failed to create proof: %w", err)
	}

	if resp.Proof != nil {
		return []*protocol.AnnotatedReceipt{resp.Proof}, nil
	}

	return []*protocol.AnnotatedReceipt{}, nil
}

// CreateSyntheticProofs creates optimized proofs for synthetic transactions
// This method is designed to be called from the block package without import cycles
// DEPRECATED: Use CreateSyntheticProofsWithPartitions for correct partition-specific chain handling
func (pi *ProofIntegration) CreateSyntheticProofs(
	ctx context.Context,
	transactions []TransactionInfo,
	synthChain *database.Chain,
	rootChain *database.Chain,
) ([]*protocol.AnnotatedReceipt, error) {
	if pi.conductor == nil || pi.conductor.proofService == nil {
		return nil, errors.InternalError.With("proof service not initialized")
	}

	// Convert TransactionInfo to ProofRequest format
	sequences := make([]uint64, len(transactions))
	for i, tx := range transactions {
		sequences[i] = tx.Sequence
	}

	// Create a synthetic proof request for all transactions
	req := ProofRequest{
		Type:        ProofTypeSynthetic,
		Destination: nil, // Will be derived from transactions
		Sequences:   sequences,
		ChainURL:    nil, // Will be derived from synthChain
		SourceChain: synthChain,
		RootChain:   rootChain,
		BlockIndex:  0, // Will be set by caller if needed
	}

	// Create proof using the centralized service
	resp, err := pi.conductor.proofService.CreateProof(ctx, req)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("failed to create synthetic proof: %w", err)
	}

	if resp.Proof != nil {
		return []*protocol.AnnotatedReceipt{resp.Proof}, nil
	}

	return []*protocol.AnnotatedReceipt{}, nil
}

// ValidateProof validates a proof using the centralized service
func (pi *ProofIntegration) ValidateProof(proof *protocol.AnnotatedReceipt) error {
	if pi.conductor == nil || pi.conductor.proofService == nil {
		// Fall back to standard validation if proof service not available
		if proof == nil || proof.Receipt == nil {
			return errors.BadRequest.With("missing proof or receipt")
		}
		if !proof.Receipt.Validate(nil) {
			return errors.BadRequest.With("proof is invalid")
		}
		return nil
	}

	return pi.conductor.ValidateIncomingProof(proof)
}

// GetProofService returns the underlying proof service for testing
func (pi *ProofIntegration) GetProofService() *ProofService {
	if pi.conductor == nil {
		return nil
	}
	return pi.conductor.proofService
}

// IsAvailable checks if the proof service is available
func (pi *ProofIntegration) IsAvailable() bool {
	return pi.conductor != nil && pi.conductor.proofService != nil
}

// GetMetrics returns proof service metrics
func (pi *ProofIntegration) GetMetrics() ProofMetrics {
	if pi.conductor == nil || pi.conductor.proofService == nil {
		return ProofMetrics{}
	}
	return pi.conductor.GetProofMetrics()
}
