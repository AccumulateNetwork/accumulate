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
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestProofIntegrationBasicFunctionality(t *testing.T) {
	t.Parallel()

	// Create a test conductor with proof service
	logger := logging.OptionalLogger{}
	conductor := &CrossChainConductor{
		logger:       logger,
		proofService: NewProofService(logger),
	}

	// Create proof integration
	pi := NewProofIntegration(conductor)
	require.NotNil(t, pi)
	require.NotNil(t, pi.GetProofService())
}

func TestProofIntegrationWithNilConductor(t *testing.T) {
	t.Parallel()

	// Test with nil conductor
	pi := NewProofIntegration(nil)
	require.NotNil(t, pi)
	require.Nil(t, pi.GetProofService())

	// Validate proof should fall back to standard validation
	err := pi.ValidateProof(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing proof")
}

func TestTransactionInfoStructure(t *testing.T) {
	t.Parallel()

	// Test TransactionInfo structure
	destURL, _ := url.Parse("acc://example/dest")

	txInfo := TransactionInfo{
		Transaction: &protocol.Transaction{},
		Hash:        [32]byte{1, 2, 3},
		Sequence:    42,
		Destination: destURL,
	}

	require.NotNil(t, txInfo.Transaction)
	require.Equal(t, uint64(42), txInfo.Sequence)
	require.Equal(t, destURL, txInfo.Destination)
	require.Equal(t, [32]byte{1, 2, 3}, txInfo.Hash)
}

func TestProofIntegrationMethodsWithNilService(t *testing.T) {
	t.Parallel()

	// Test methods when proof service is not initialized
	pi := NewProofIntegration(nil)
	ctx := context.Background()

	// Test CreateSyntheticProofsWithPartitions
	sourceURL, _ := url.Parse("acc://example/source")
	_, err := pi.CreateSyntheticProofsWithPartitions(ctx, nil, sourceURL, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "proof service not initialized")

	// Test CreateSyntheticProofs
	_, err = pi.CreateSyntheticProofs(ctx, nil, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "proof service not initialized")
}
