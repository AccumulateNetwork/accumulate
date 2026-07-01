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
	if cc.proofService == nil {
		return nil, errors.InternalError.With("proof service not initialized")
	}

	var allProofs []*protocol.AnnotatedReceipt
	
	// Process each destination partition separately for efficient collection proofs
	for destination, destTransactions := range partitionMap {
		if len(destTransactions) == 0 {
			continue
		}
		
		// Extract sequence numbers
		sequences := make([]uint64, len(destTransactions))
		for i := range destTransactions {
			// For synthetic transactions, we'll use simple sequential numbering
			// In reality, sequences would be managed by the executor
			sequences[i] = uint64(i)
		}
		
		// Create proof request for this destination
		req := ProofRequest{
			Type:        ProofTypeSynthetic,
			Destination: nil, // Will be derived from destination string
			Sequences:   sequences,
			ChainURL:    nil, // Will be derived
			// Note: SourceChain and RootChain would need to be derived from batch context
			SourceChain: nil,
			RootChain:   nil,
			BlockIndex:  0, // Would be set by caller
		}
		
		// Create proof using the centralized service
		resp, err := cc.proofService.CreateProof(ctx, req)
		if err != nil {
			cc.logger.Error("Failed to create proof for destination", 
				"destination", destination, 
				"error", err)
			continue // Continue with other destinations
		}
		
		if resp.Proof != nil {
			allProofs = append(allProofs, resp.Proof)
		}
	}
	
	return allProofs, nil
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
		// For now, use a simplified approach - in practice this would examine
		// the transaction body to determine the actual destination partition
		dest := "Directory" // Default to Directory partition for synthetic transactions

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