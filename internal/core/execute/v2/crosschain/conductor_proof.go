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
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// CreateProofsForSyntheticTransactionsWithPartitions creates proofs for synthetic transactions
// grouped by destination partition for efficient collection proofs
func (cc *CrossChainConductor) CreateProofsForSyntheticTransactionsWithPartitions(
	ctx context.Context,
	batch *database.Batch,
	transactions []*protocol.Transaction,
	partitionMap map[string][]*protocol.Transaction,
) ([]*protocol.AnnotatedReceipt, error) {
	// TODO: This method needs to be properly implemented with correct types
	// The ProofService doesn't have ProofTypeIndividual or ProofTypeCollection constants
	// and the ProofRequest/ProofResponse types don't match what's being used here
	return nil, errors.InternalError.With("CreateProofsForSyntheticTransactionsWithPartitions not implemented")
}

// CreateProofsForSyntheticTransactions creates proofs for synthetic transactions
// This is a simpler interface that doesn't require pre-partitioning
func (cc *CrossChainConductor) CreateProofsForSyntheticTransactions(
	ctx context.Context,
	batch *database.Batch,
	transactions []*protocol.Transaction,
) ([]*protocol.AnnotatedReceipt, error) {
	if cc.proofService == nil {
		return nil, errors.InternalError.With("proof service not initialized")
	}

	// Group transactions by destination
	partitionMap := make(map[string][]*protocol.Transaction)
	for _, tx := range transactions {
		// Extract destination from transaction
		// TODO: Implement proper destination extraction from transaction
		dest := "unknown"
		// if synth, ok := tx.Body.(*protocol.SyntheticCreateIdentity); ok {
		// 	// SyntheticCreateIdentity doesn't have a Url field
		// 	dest = "unknown" 
		// }
		// Add more transaction type handling as needed

		partitionMap[dest] = append(partitionMap[dest], tx)
	}

	// Use the partition-aware method
	return cc.CreateProofsForSyntheticTransactionsWithPartitions(ctx, batch, transactions, partitionMap)
}

// ValidateIncomingProof validates an incoming proof from another partition
func (cc *CrossChainConductor) ValidateIncomingProof(proof *protocol.AnnotatedReceipt) error {
	if cc.proofService == nil {
		return errors.InternalError.With("proof service not initialized")
	}

	// Validate the proof
	err := cc.proofService.ValidateProof(proof)
	if err != nil {
		return errors.UnknownError.WithFormat("proof validation failed: %w", err)
	}

	// Log successful validation
	cc.logger.Debug("Validated incoming proof")

	return nil
}

// GetBlockIntegration returns the block integration interface
func (cc *CrossChainConductor) GetBlockIntegration() *BlockIntegration {
	return cc.blockIntegration
}