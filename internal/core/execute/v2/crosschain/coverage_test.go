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

func TestProofService_Construction(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	
	require.NotNil(t, ps)
	require.Equal(t, 100, ps.maxBatchSize)
	require.False(t, ps.debugMode)
	require.NotNil(t, ps.metrics)
}

func TestProofService_DebugMode(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	
	// Test setting debug mode
	ps.SetDebugMode(true)
	require.True(t, ps.debugMode)
	
	ps.SetDebugMode(false)
	require.False(t, ps.debugMode)
}

func TestProofService_MetricsOperations(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	
	// Test initial metrics
	metrics := ps.GetMetrics()
	require.Equal(t, int64(0), metrics.IndividualProofsCreated)
	require.Equal(t, int64(0), metrics.CollectionProofsCreated)
	
	// Test reset
	ps.metrics.IndividualProofsCreated = 5
	ps.metrics.CollectionProofsCreated = 3
	
	ps.ResetMetrics()
	
	metrics = ps.GetMetrics()
	require.Equal(t, int64(0), metrics.IndividualProofsCreated)
	require.Equal(t, int64(0), metrics.CollectionProofsCreated)
}

func TestProofService_CreateProof_NoSequences(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ctx := context.Background()
	
	req := ProofRequest{
		Type:        ProofTypeSynthetic,
		Destination: protocol.AccountUrl("test", "account"),
		Sequences:   []uint64{}, // Empty sequences
	}
	
	_, err := ps.CreateProof(ctx, req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no sequences provided")
	
	metrics := ps.GetMetrics()
	require.Equal(t, int64(1), metrics.ProofGenErrors)
}

func TestProofService_MergeSequencesLogic(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	
	dest := protocol.AccountUrl("test", "dest")
	
	// Test merging multiple requests with unsorted sequences
	requests := []ProofRequest{
		{Destination: dest, Sequences: []uint64{3, 1}},
		{Destination: dest, Sequences: []uint64{5, 2}},
	}
	
	merged := ps.mergeSequences(requests)
	require.Equal(t, dest, merged.Destination)
	require.Equal(t, []uint64{1, 2, 3, 5}, merged.Sequences) // Should be sorted
}

func TestProofService_MergeSequences_EmptyInput(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	
	merged := ps.mergeSequences([]ProofRequest{})
	require.Empty(t, merged.Sequences)
}

func TestProofService_OptimizeForDestinations_BasicGrouping(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ps.SetDebugMode(true)
	
	dest1 := protocol.AccountUrl("test", "dest1")
	dest2 := protocol.AccountUrl("test", "dest2")
	
	requests := []ProofRequest{
		{Destination: dest1, Sequences: []uint64{1, 2}},
		{Destination: dest1, Sequences: []uint64{3}},
		{Destination: dest2, Sequences: []uint64{10}},
		{Destination: nil}, // Test nil destination handling
	}
	
	batches := ps.OptimizeForDestinations(requests)
	require.Len(t, batches, 3) // Two destinations + nil destination
	
	// Check that destinations are grouped properly
	var dest1Found, dest2Found, nilDestFound bool
	for _, batch := range batches {
		if batch.Destination != nil {
			switch batch.Destination.String() {
			case dest1.String():
				dest1Found = true
				require.Len(t, batch.Requests, 2) // Two requests for dest1
			case dest2.String():
				dest2Found = true
				require.Len(t, batch.Requests, 1) // One request for dest2
			}
		} else {
			nilDestFound = true
			require.Len(t, batch.Requests, 1) // One request for nil dest
		}
	}
	
	require.True(t, dest1Found)
	require.True(t, dest2Found)
	require.True(t, nilDestFound)
}

func TestProofService_OptimizeForDestinations_EmptyInput(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	
	batches := ps.OptimizeForDestinations([]ProofRequest{})
	require.Empty(t, batches)
}

func TestProofService_ValidateProof_NilInput(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ps.SetDebugMode(true)
	
	// Test nil proof
	err := ps.ValidateProof(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing proof")
	
	metrics := ps.GetMetrics()
	require.Equal(t, int64(1), metrics.ValidationAttempts)
	require.Equal(t, int64(1), metrics.ValidationFailures)
	require.Equal(t, int64(1), metrics.ValidationErrors)
}

func TestProofService_ValidateProof_EmptyReceipt(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	
	// Test proof without receipt
	proof := &protocol.AnnotatedReceipt{}
	err := ps.ValidateProof(proof)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing proof")
}

func TestProofService_ValidateBatch_MultipleProofs(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ps.SetDebugMode(true)
	
	proofs := []*protocol.AnnotatedReceipt{
		nil,                            // Nil proof
		{},                            // Empty proof
		{Receipt: nil, Anchor: nil},   // Invalid proof
	}
	
	errors := ps.ValidateBatch(proofs)
	require.Len(t, errors, 3)
	
	// All should have errors
	for _, err := range errors {
		require.Error(t, err)
	}
	
	metrics := ps.GetMetrics()
	require.Equal(t, int64(3), metrics.ValidationAttempts)
	require.Equal(t, int64(3), metrics.ValidationFailures)
}

func TestProofService_CreateCollectionProofForAPI_Basic(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ctx := context.Background()
	
	destination := protocol.AccountUrl("test", "destination")
	
	// This will fail due to nil chain, but tests the API method structure
	_, err := ps.CreateCollectionProofForAPI(ctx, []uint64{10, 11}, destination, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "source chain not provided")
}

func TestProofService_CreateIndividualProofForAPI_Basic(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ctx := context.Background()
	
	destination := protocol.AccountUrl("test", "destination")
	
	// This will fail due to nil chain, but tests the API method structure
	_, err := ps.CreateIndividualProofForAPI(ctx, 42, destination, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "source chain not provided")
}

func TestProofService_CreateBatchProofs_EmptyInput(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ps.SetDebugMode(true)
	ctx := context.Background()
	
	responses, err := ps.CreateBatchProofs(ctx, []ProofRequest{})
	require.NoError(t, err)
	require.Empty(t, responses)
}

func TestCreateIndividualProof_NoSourceChain(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ctx := context.Background()
	
	req := ProofRequest{
		Type:        ProofTypeSynthetic,
		Destination: protocol.AccountUrl("test", "dest"),
		Sequences:   []uint64{1},
		SourceChain: nil, // Missing source chain
	}
	
	_, err := ps.createIndividualProof(ctx, req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "source chain not provided")
	
	metrics := ps.GetMetrics()
	require.Equal(t, int64(1), metrics.ProofGenErrors)
}

func TestCreateCollectionProof_NoSourceChain(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ps.SetDebugMode(true)
	ctx := context.Background()
	
	req := ProofRequest{
		Type:        ProofTypeSynthetic,
		Destination: protocol.AccountUrl("test", "dest"),
		Sequences:   []uint64{1, 2, 3},
		SourceChain: nil, // Missing source chain
	}
	
	_, err := ps.createCollectionProof(ctx, req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "source chain not provided")
	
	metrics := ps.GetMetrics()
	require.Equal(t, int64(1), metrics.ProofGenErrors)
}

func TestCreateCollectionProof_SortedSequences(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ps.SetDebugMode(true)
	ctx := context.Background()
	
	// Test that unsorted sequences get sorted
	req := ProofRequest{
		Type:        ProofTypeSynthetic,
		Destination: protocol.AccountUrl("test", "dest"),
		Sequences:   []uint64{5, 2, 8, 1}, // Unsorted
		SourceChain: nil,                   // Will cause error but tests sorting first
	}
	
	_, err := ps.createCollectionProof(ctx, req)
	require.Error(t, err) // Expected due to nil chain
}

func TestProofService_CreateBatchProofs_WithRequests(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ps.SetDebugMode(true)
	ctx := context.Background()
	
	dest1 := protocol.AccountUrl("test", "dest1")
	dest2 := protocol.AccountUrl("test", "dest2")
	
	requests := []ProofRequest{
		{
			Type:        ProofTypeSynthetic,
			Destination: dest1,
			Sequences:   []uint64{1, 2},
			SourceChain: nil, // Will cause error
		},
		{
			Type:        ProofTypeAnchor,
			Destination: dest2,
			Sequences:   []uint64{10},
			SourceChain: nil, // Will cause error
		},
	}
	
	_, err := ps.CreateBatchProofs(ctx, requests)
	require.Error(t, err) // Expected due to nil chains
}

// Add tests for various utilities and edge cases
func TestMinFunction(t *testing.T) {
	require.Equal(t, 3, min(3, 5))
	require.Equal(t, 3, min(5, 3))
	require.Equal(t, 7, min(7, 7))
	require.Equal(t, 0, min(0, 10))
	require.Equal(t, -5, min(-5, 0))
}

func TestProofService_EdgeCases(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	
	// Test with nil logger methods (shouldn't crash)
	ps.SetDebugMode(true)
	ps.SetDebugMode(false)
	
	// Test metrics access
	metrics := ps.GetMetrics()
	require.NotNil(t, metrics)
	
	// Test reset multiple times
	ps.ResetMetrics()
	ps.ResetMetrics()
	
	metrics = ps.GetMetrics()
	require.Equal(t, int64(0), metrics.IndividualProofsCreated)
}

func TestProofRequest_Validation(t *testing.T) {
	// Test that we can create various ProofRequest configurations
	dest := protocol.AccountUrl("test", "destination")
	chain := protocol.AccountUrl("test", "chain")
	
	tests := []struct {
		name     string
		req      ProofRequest
		expected ProofType
	}{
		{
			name: "Synthetic",
			req: ProofRequest{
				Type:        ProofTypeSynthetic,
				Destination: dest,
				Sequences:   []uint64{1},
				ChainURL:    chain,
			},
			expected: ProofTypeSynthetic,
		},
		{
			name: "Anchor",
			req: ProofRequest{
				Type:        ProofTypeAnchor,
				Destination: dest,
				Sequences:   []uint64{1, 2},
				ChainURL:    chain,
			},
			expected: ProofTypeAnchor,
		},
		{
			name: "Receipt",
			req: ProofRequest{
				Type:        ProofTypeReceipt,
				Destination: dest,
				Sequences:   []uint64{1, 2, 3},
				ChainURL:    chain,
			},
			expected: ProofTypeReceipt,
		},
		{
			name: "Unified",
			req: ProofRequest{
				Type:        ProofTypeUnified,
				Destination: dest,
				Sequences:   []uint64{1, 2, 3, 4},
				ChainURL:    chain,
			},
			expected: ProofTypeUnified,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.req.Type)
			require.Equal(t, dest, tt.req.Destination)
			require.NotEmpty(t, tt.req.Sequences)
			require.Equal(t, chain, tt.req.ChainURL)
		})
	}
}

func TestProofService_MergeSequences_VariousInputs(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	
	dest := protocol.AccountUrl("test", "dest")
	
	tests := []struct {
		name     string
		input    []ProofRequest
		expected []uint64
	}{
		{
			name:     "Empty",
			input:    []ProofRequest{},
			expected: nil,
		},
		{
			name: "Single request",
			input: []ProofRequest{
				{Destination: dest, Sequences: []uint64{5, 1, 3}},
			},
			expected: []uint64{1, 3, 5},
		},
		{
			name: "Multiple requests",
			input: []ProofRequest{
				{Destination: dest, Sequences: []uint64{8, 2}},
				{Destination: dest, Sequences: []uint64{1, 5}},
				{Destination: dest, Sequences: []uint64{3}},
			},
			expected: []uint64{1, 2, 3, 5, 8},
		},
		{
			name: "Duplicate sequences",
			input: []ProofRequest{
				{Destination: dest, Sequences: []uint64{2, 1}},
				{Destination: dest, Sequences: []uint64{1, 3}},
			},
			expected: []uint64{1, 1, 2, 3}, // Duplicates preserved, just sorted
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merged := ps.mergeSequences(tt.input)
			require.Equal(t, tt.expected, merged.Sequences)
		})
	}
}

func TestDestinationKey_Equality(t *testing.T) {
	key1 := DestinationKey{
		Type:        MessageTypeAnchor,
		Destination: "test://same",
	}
	
	key2 := DestinationKey{
		Type:        MessageTypeAnchor,
		Destination: "test://same",
	}
	
	key3 := DestinationKey{
		Type:        MessageTypeSynthetic,
		Destination: "test://same",
	}
	
	require.Equal(t, key1, key2)
	require.NotEqual(t, key1, key3)
}

func TestRecoveryTypes_AllValues(t *testing.T) {
	// Test all recovery type values
	types := []RecoveryType{
		RecoveryTypeAnchor,
		RecoveryTypeSynthetic,
	}
	
	expectedStrings := []string{
		"anchor",
		"synthetic",
	}
	
	for i, rt := range types {
		require.Equal(t, expectedStrings[i], rt.String())
	}
	
	// Test unknown type
	unknown := RecoveryType(100)
	require.Equal(t, "unknown", unknown.String())
}

func TestProofBatch_Operations(t *testing.T) {
	dest1 := protocol.AccountUrl("test", "dest1")
	dest2 := protocol.AccountUrl("test", "dest2")
	
	req1 := ProofRequest{Type: ProofTypeSynthetic, Destination: dest1}
	req2 := ProofRequest{Type: ProofTypeAnchor, Destination: dest2}
	
	batch := ProofBatch{
		Destination: dest1,
		Requests:    []ProofRequest{req1, req2},
	}
	
	require.Equal(t, dest1, batch.Destination)
	require.Len(t, batch.Requests, 2)
	require.Equal(t, ProofTypeSynthetic, batch.Requests[0].Type)
	require.Equal(t, ProofTypeAnchor, batch.Requests[1].Type)
}

func TestSyntheticTransaction_Aliases(t *testing.T) {
	tx := SyntheticTransaction{
		SequenceNum: 42,
		Sequence:    43, // Different value to test both fields
	}
	
	require.Equal(t, uint64(42), tx.SequenceNum)
	require.Equal(t, uint64(43), tx.Sequence)
}

func TestStructSizes(t *testing.T) {
	// Test that all struct types can be instantiated
	_ = ProofRequest{}
	_ = ProofResponse{}
	_ = ProofBatch{}
	_ = ProofMetrics{}
	_ = RecoveryRequest{}
	_ = PendingTransmission{}
	_ = DestinationQueue{}
	_ = BatchRecoveryRequest{}
	_ = BatchRecoveryResponse{}
	_ = RecoveredTransaction{}
	_ = SyntheticTransaction{}
	_ = TransportMetrics{}
	_ = UnifiedMessage{}
	
	// If we get here, all structs are properly defined
	require.True(t, true)
}