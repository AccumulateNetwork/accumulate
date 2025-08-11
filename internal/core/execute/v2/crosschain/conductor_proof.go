// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
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

	receipts := make([]*protocol.AnnotatedReceipt, 0)

	// Process each partition's transactions
	for partition, txns := range partitionMap {
		if len(txns) == 0 {
			continue
		}

		cc.logger.Debug("Creating proofs for partition",
			"partition", partition,
			"transaction_count", len(txns))

		// Determine proof type based on transaction count
		proofType := ProofTypeIndividual
		if len(txns) >= 2 { // Hard-coded threshold of 2+
			proofType = ProofTypeCollection
		}

		// Create proof request
		req := &ProofRequest{
			Type:         proofType,
			Transactions: txns,
			Destination:  partition,
		}

		// Generate proof
		resp, err := cc.proofService.CreateProof(ctx, batch, req)
		if err != nil {
			// Collection proof failure is a hard error - no fallback
			return nil, errors.UnknownError.WithFormat("failed to create %s proof for partition %s: %w",
				proofType, partition, err)
		}

		// Add receipts
		receipts = append(receipts, resp.Receipts...)

		// Log metrics
		if proofType == ProofTypeCollection {
			cc.logger.Info("Created collection proof",
				"partition", partition,
				"transactions", len(txns),
				"proof_size", resp.ProofSize,
				"savings", resp.ProofSavings)
		}
	}

	return receipts, nil
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
		// This is simplified - actual implementation would need proper destination extraction
		dest := "unknown"
		if synth, ok := tx.Body.(*protocol.SyntheticCreateIdentity); ok {
			dest = synth.Url.String()
		}
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

	// Create validation request
	req := &ProofValidationRequest{
		Receipt: proof,
		// Add more validation parameters as needed
	}

	// Validate the proof
	resp, err := cc.proofService.ValidateProof(context.Background(), req)
	if err != nil {
		return errors.UnknownError.WithFormat("proof validation failed: %w", err)
	}

	if !resp.IsValid {
		return errors.BadRequest.WithFormat("invalid proof: %s", resp.Reason)
	}

	// Log successful validation
	cc.logger.Debug("Validated incoming proof",
		"type", resp.ProofType,
		"elements", resp.ElementCount)

	return nil
}

// GetBlockIntegration returns the block integration interface
func (cc *CrossChainConductor) GetBlockIntegration() *BlockIntegration {
	return cc.blockIntegration
}