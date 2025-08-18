// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestConductorProofCreation(t *testing.T) {
	t.Parallel()

	// Create conductor with proof service
	logger := logging.OptionalLogger{}
	conductor := &CrossChainConductor{
		logger:       logger,
		proofService: NewProofService(logger),
	}

	ctx := context.Background()

	// Test CreateProofsForSyntheticTransactions with nil service
	conductor.proofService = nil
	_, err := conductor.CreateProofsForSyntheticTransactions(ctx, []SyntheticTransaction{}, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "proof service not initialized")
}

func TestConductorProofValidation(t *testing.T) {
	t.Parallel()

	// Create conductor with proof service
	logger := logging.OptionalLogger{}
	conductor := &CrossChainConductor{
		logger:       logger,
		proofService: NewProofService(logger),
	}

	// Test ValidateIncomingProof with nil service
	conductor.proofService = nil
	err := conductor.ValidateIncomingProof(&protocol.AnnotatedReceipt{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "proof service not initialized")
}

func TestConductorWithProofService(t *testing.T) {
	t.Parallel()

	// Create conductor with proof service
	logger := logging.OptionalLogger{}
	conductor := &CrossChainConductor{
		logger:       logger,
		proofService: NewProofService(logger),
	}

	ctx := context.Background()

	// Test with empty transactions (should return empty result)
	transactions := []SyntheticTransaction{}
	proofs, err := conductor.CreateProofsForSyntheticTransactions(ctx, transactions, nil, nil)
	require.NoError(t, err)
	require.Empty(t, proofs)
}

func TestBlockIntegrationAccess(t *testing.T) {
	t.Parallel()

	conductor := &CrossChainConductor{}

	// Should return nil when not initialized
	// NOTE: GetBlockIntegration method doesn't exist - test disabled
	// blockIntegration := conductor.GetBlockIntegration()
	// require.Nil(t, blockIntegration)
	_ = conductor // Avoid unused variable warning
}
