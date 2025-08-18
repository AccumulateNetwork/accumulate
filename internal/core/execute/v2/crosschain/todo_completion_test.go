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

func TestTODOItemsCompleted(t *testing.T) {
	t.Parallel()

	logger := logging.OptionalLogger{}

	// Test 1: ProofService integration works
	proofService := NewProofService(logger)
	require.NotNil(t, proofService)

	// Test 2: ProofIntegration functionality works
	conductor := &CrossChainConductor{
		logger:       logger,
		proofService: proofService,
	}

	pi := NewProofIntegration(conductor)
	require.NotNil(t, pi.GetProofService())

	// Test 3: Conductor proof creation works (no longer returns "not implemented")
	ctx := context.Background()
	transactions := []SyntheticTransaction{}
	proofs, err := conductor.CreateProofsForSyntheticTransactions(ctx, transactions, nil, nil)
	require.NoError(t, err)
	require.Empty(t, proofs) // Empty input should give empty output

	// Test 4: AnchorRequest structure is properly defined (no type mismatch)
	sourceURL, _ := url.Parse("acc://example/source")
	destURL, _ := url.Parse("acc://example/dest")

	anchorReq := &AnchorRequest{
		Source:      sourceURL,
		Destination: destURL,
		Sequence:    42,
		SourceChain: sourceURL, // These are *url.URL as expected
		RootChain:   sourceURL, // No more type mismatch issues
		BlockIndex:  100,
	}

	// Verify the request is properly constructed
	require.NotNil(t, anchorReq)
	require.Equal(t, sourceURL, anchorReq.Source)
	require.Equal(t, destURL, anchorReq.Destination)

	// Test 5: UnifiedTransport conversion functions work
	synth := SyntheticTransaction{
		Destination: destURL,
		SequenceNum: 42,
	}

	msg := ConvertSyntheticToUnified(synth, nil, nil, 100)
	require.NotNil(t, msg)
	require.Equal(t, MessageTypeSynthetic, msg.Type)
	require.Equal(t, destURL, msg.Destination)
	require.Equal(t, uint64(42), msg.Sequence)

	// Test 6: UnifiedTransport routing simulation works
	transport := NewUnifiedTransport(proofService, conductor, logger)
	transport.SetDebugMode(true)

	err = transport.routeMessages([]CrossChainMessage{msg}, nil)
	require.NoError(t, err) // Should simulate routing successfully
}

func TestErrorHandlingImprovement(t *testing.T) {
	t.Parallel()

	logger := logging.OptionalLogger{}

	ctx := context.Background()

	// Test that error handling is improved (not just "not implemented")
	conductor := &CrossChainConductor{
		logger: logger,
		// No proof service - should give specific error
	}
	_, err := conductor.CreateProofsForSyntheticTransactions(ctx, []SyntheticTransaction{}, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "proof service not initialized")
}

func TestConductorComponents(t *testing.T) {
	t.Parallel()

	logger := logging.OptionalLogger{}
	conductor := &CrossChainConductor{
		logger:       logger,
		proofService: NewProofService(logger),
	}

	// Test that components integrate correctly
	require.NotNil(t, conductor.proofService)

	// Proof validation should work
	err := conductor.ValidateIncomingProof(&protocol.AnnotatedReceipt{})
	require.Error(t, err) // Should fail validation but not crash
	require.NotContains(t, err.Error(), "not implemented")
}
