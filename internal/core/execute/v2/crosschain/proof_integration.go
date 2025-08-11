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

	// Convert to SyntheticTransaction format
	syntheticTxs := make([]SyntheticTransaction, len(transactions))
	for i, tx := range transactions {
		syntheticTxs[i] = SyntheticTransaction{
			Destination: tx.Destination,
			SequenceNum: tx.SequenceNum,
			ChainURL:    tx.ChainURL,
			Hash:        tx.Hash,
			Source:      sourcePartition,
			Message:     nil, // Not needed for proof generation
		}
	}

	// Use the conductor's new method with partition-specific chains
	return pi.conductor.CreateProofsForSyntheticTransactionsWithPartitions(
		ctx,
		batch,
		sourcePartition,
		syntheticTxs,
		rootChain,
	)
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

	// Convert to SyntheticTransaction format
	syntheticTxs := make([]SyntheticTransaction, len(transactions))
	for i, tx := range transactions {
		syntheticTxs[i] = SyntheticTransaction{
			Destination: tx.Destination,
			SequenceNum: tx.SequenceNum,
			ChainURL:    tx.ChainURL,
			Hash:        tx.Hash,
			Source:      nil, // Source partition not available in deprecated method
			Message:     nil, // Not needed for proof generation
		}
	}

	// Use the conductor's method (deprecated path)
	return pi.conductor.CreateProofsForSyntheticTransactions(
		ctx,
		syntheticTxs,
		synthChain,
		rootChain,
	)
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

// TransactionInfo contains information about a transaction needing a proof
// This avoids importing types from other packages
type TransactionInfo struct {
	Destination *url.URL
	SequenceNum uint64
	ChainURL    *url.URL
	Hash        []byte
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
