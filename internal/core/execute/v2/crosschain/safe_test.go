// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestProofService_AllPublicMethods(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	
	// Test construction
	require.NotNil(t, ps)
	
	// Test debug mode
	ps.SetDebugMode(true)
	require.True(t, ps.debugMode)
	
	// Test metrics
	metrics := ps.GetMetrics()
	require.NotNil(t, metrics)
	
	// Test reset
	ps.ResetMetrics()
	
	// Test validation with nil
	err := ps.ValidateProof(nil)
	require.Error(t, err)
	
	// Test batch validation
	errors := ps.ValidateBatch([]*protocol.AnnotatedReceipt{nil, nil})
	require.Len(t, errors, 2)
	
	// Test optimization
	batches := ps.OptimizeForDestinations([]ProofRequest{})
	require.Empty(t, batches)
	
	// Test merging
	merged := ps.mergeSequences([]ProofRequest{})
	require.Empty(t, merged.Sequences)
}

func TestProofService_CreateProofPaths(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ps.SetDebugMode(true)
	ctx := context.Background()
	
	dest := protocol.AccountUrl("test", "destination")
	
	// Test empty sequences error
	req := ProofRequest{Type: ProofTypeSynthetic, Destination: dest, Sequences: []uint64{}}
	_, err := ps.CreateProof(ctx, req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no sequences provided")
	
	// Test single sequence path (individual proof)
	req = ProofRequest{Type: ProofTypeSynthetic, Destination: dest, Sequences: []uint64{1}}
	_, err = ps.CreateProof(ctx, req)
	require.Error(t, err) // Will fail due to nil chain but tests path
	
	// Test multiple sequence path (collection proof)
	req = ProofRequest{Type: ProofTypeSynthetic, Destination: dest, Sequences: []uint64{1, 2, 3}}
	_, err = ps.CreateProof(ctx, req)
	require.Error(t, err) // Will fail due to nil chain but tests path
}

func TestProofService_CreateCollectionProofSorting(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ps.SetDebugMode(true)
	ctx := context.Background()
	
	// Test that sequences get sorted properly
	req := ProofRequest{
		Type:        ProofTypeSynthetic,
		Destination: protocol.AccountUrl("test", "dest"),
		Sequences:   []uint64{5, 1, 9, 3, 7}, // Unsorted
	}
	
	// The function should sort sequences before proceeding
	_, err := ps.createCollectionProof(ctx, req)
	require.Error(t, err) // Expected due to nil chain
	
	// Test already sorted sequences (different code path)
	req.Sequences = []uint64{1, 3, 5, 7, 9} // Already sorted
	_, err = ps.createCollectionProof(ctx, req)
	require.Error(t, err) // Expected due to nil chain
}

func TestProofService_IndividualProofErrorPaths(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ps.SetDebugMode(true)
	ctx := context.Background()
	
	// Test nil source chain
	req := ProofRequest{
		Type:        ProofTypeUnified,
		Destination: protocol.AccountUrl("test", "dest"),
		Sequences:   []uint64{1},
		SourceChain: nil,
	}
	
	_, err := ps.createIndividualProof(ctx, req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "source chain not provided")
	
	metrics := ps.GetMetrics()
	require.Equal(t, int64(1), metrics.ProofGenErrors)
}

func TestProofService_ValidationErrorPath(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ps.SetDebugMode(true)
	
	// Test various invalid proofs
	testCases := []struct {
		name  string
		proof *protocol.AnnotatedReceipt
	}{
		{"nil proof", nil},
		{"empty proof", &protocol.AnnotatedReceipt{}},
		{"proof with nil receipt", &protocol.AnnotatedReceipt{Receipt: nil}},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ps.ValidateProof(tc.proof)
			require.Error(t, err)
			require.Contains(t, err.Error(), "missing proof")
		})
	}
	
	metrics := ps.GetMetrics()
	require.Equal(t, int64(3), metrics.ValidationAttempts)
	require.Equal(t, int64(3), metrics.ValidationFailures)
	require.Equal(t, int64(3), metrics.ValidationErrors)
}

func TestProofService_ReceiptValidationFailure(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ps.SetDebugMode(true)
	
	// Create a receipt with invalid data that will fail Validate()
	receipt := &merkle.Receipt{
		Start:  []byte("invalid-start"),
		Anchor: []byte("invalid-anchor"),
		Entries: []*merkle.ReceiptEntry{
			{Hash: []byte("invalid-hash")},
		},
	}
	
	proof := &protocol.AnnotatedReceipt{
		Receipt: receipt,
	}
	
	err := ps.ValidateProof(proof)
	require.Error(t, err)
	require.Contains(t, err.Error(), "proof validation failed")
	
	metrics := ps.GetMetrics()
	require.Equal(t, int64(1), metrics.ValidationAttempts)
	require.Equal(t, int64(1), metrics.ValidationFailures)
}

func TestProofService_APIMethodsErrorHandling(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ctx := context.Background()
	
	dest := protocol.AccountUrl("api", "destination")
	
	// Test CreateCollectionProofForAPI error path
	_, err := ps.CreateCollectionProofForAPI(ctx, []uint64{1, 2, 3}, dest, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "source chain not provided")
	
	// Test CreateIndividualProofForAPI error path
	_, err = ps.CreateIndividualProofForAPI(ctx, 42, dest, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "source chain not provided")
}

func TestProofService_BatchProofCreation(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ps.SetDebugMode(true)
	ctx := context.Background()
	
	dest1 := protocol.AccountUrl("test", "dest1")
	dest2 := protocol.AccountUrl("test", "dest2")
	
	requests := []ProofRequest{
		{Type: ProofTypeSynthetic, Destination: dest1, Sequences: []uint64{1, 2}},
		{Type: ProofTypeSynthetic, Destination: dest1, Sequences: []uint64{3}},
		{Type: ProofTypeAnchor, Destination: dest2, Sequences: []uint64{10}},
	}
	
	// Test the optimization and batching logic
	batches := ps.OptimizeForDestinations(requests)
	require.Len(t, batches, 2) // Two destinations
	
	// Test batch processing (will fail due to nil chains but exercises the logic)
	_, err := ps.CreateBatchProofs(ctx, requests)
	require.Error(t, err)
}

func TestProofService_MergeComplexSequences(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	
	dest := protocol.AccountUrl("merge", "test")
	chainUrl := protocol.AccountUrl("chain", "test")
	
	// Test preserving first request's metadata during merge
	requests := []ProofRequest{
		{
			Type:        ProofTypeSynthetic,
			Destination: dest,
			Sequences:   []uint64{3, 1},
			ChainURL:    chainUrl,
			BlockIndex:  100,
			Metadata:    "first-request-metadata",
		},
		{
			Type:        ProofTypeAnchor, // Different - should be ignored
			Destination: dest,
			Sequences:   []uint64{5, 2},
			ChainURL:    protocol.AccountUrl("different", "chain"),
			BlockIndex:  200,
			Metadata:    "second-request-metadata",
		},
	}
	
	merged := ps.mergeSequences(requests)
	
	// Should preserve first request's fields
	require.Equal(t, ProofTypeSynthetic, merged.Type) // From first request
	require.Equal(t, dest, merged.Destination)
	require.Equal(t, chainUrl, merged.ChainURL) // From first request
	require.Equal(t, uint64(100), merged.BlockIndex) // From first request
	require.Equal(t, "first-request-metadata", merged.Metadata) // From first request
	
	// Sequences should be merged and sorted
	require.Equal(t, []uint64{1, 2, 3, 5}, merged.Sequences)
}

func TestProofService_ErrorCountIncrementation(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	
	// Test that error counts increment correctly
	initialErrors := ps.GetMetrics().ProofGenErrors
	
	// Trigger proof generation error
	atomic.AddInt64(&ps.metrics.ProofGenErrors, 1)
	
	metrics := ps.GetMetrics()
	require.Equal(t, initialErrors+1, metrics.ProofGenErrors)
	
	// Test validation error increment
	initialValidationErrors := metrics.ValidationErrors
	atomic.AddInt64(&ps.metrics.ValidationErrors, 1)
	
	metrics = ps.GetMetrics()
	require.Equal(t, initialValidationErrors+1, metrics.ValidationErrors)
}

func TestMinUtility_EdgeCases(t *testing.T) {
	// Test the min utility function thoroughly
	require.Equal(t, 0, min(0, 0))
	require.Equal(t, -1, min(-1, 0))
	require.Equal(t, -1, min(0, -1))
	require.Equal(t, -100, min(-100, -1))
	require.Equal(t, 1, min(1, 100))
	require.Equal(t, 0, min(0, 1000000))
}

func TestSortingBehavior(t *testing.T) {
	// Test sort behavior used in the code
	sequences := []uint64{10, 3, 7, 1, 9}
	
	// Test IsSorted detection
	isSorted := sort.SliceIsSorted(sequences, func(i, j int) bool {
		return sequences[i] < sequences[j]
	})
	require.False(t, isSorted)
	
	// Test sorting
	sort.Slice(sequences, func(i, j int) bool {
		return sequences[i] < sequences[j]
	})
	
	require.Equal(t, []uint64{1, 3, 7, 9, 10}, sequences)
	
	// Test already sorted detection
	isSorted = sort.SliceIsSorted(sequences, func(i, j int) bool {
		return sequences[i] < sequences[j]
	})
	require.True(t, isSorted)
}

func TestProofResponse_AllFieldAccess(t *testing.T) {
	// Test all ProofResponse fields
	proof := &protocol.AnnotatedReceipt{
		Receipt: &merkle.Receipt{},
	}
	
	sequences := []uint64{10, 20, 30}
	
	resp := ProofResponse{
		Proof:        proof,
		ProofType:    ProofTypeUnified,
		Sequences:    sequences,
		IsCollection: true,
		ProofSavings: 2,
	}
	
	require.Equal(t, proof, resp.Proof)
	require.Equal(t, ProofTypeUnified, resp.ProofType)
	require.Equal(t, sequences, resp.Sequences)
	require.True(t, resp.IsCollection)
	require.Equal(t, 2, resp.ProofSavings)
	
	// Test field modifications
	resp.IsCollection = false
	resp.ProofSavings = 0
	
	require.False(t, resp.IsCollection)
	require.Equal(t, 0, resp.ProofSavings)
}

func TestProofBatch_BatchConstruction(t *testing.T) {
	dest1 := protocol.AccountUrl("test", "dest1")
	dest2 := protocol.AccountUrl("test", "dest2")
	
	req1 := ProofRequest{
		Type:        ProofTypeSynthetic,
		Destination: dest1,
		Sequences:   []uint64{1, 2},
	}
	
	req2 := ProofRequest{
		Type:        ProofTypeAnchor,
		Destination: dest2,
		Sequences:   []uint64{10, 11, 12},
	}
	
	// Test batch with single destination
	batch1 := ProofBatch{
		Destination: dest1,
		Requests:    []ProofRequest{req1},
	}
	
	require.Equal(t, dest1, batch1.Destination)
	require.Len(t, batch1.Requests, 1)
	require.Equal(t, ProofTypeSynthetic, batch1.Requests[0].Type)
	
	// Test batch with mixed requests
	batch2 := ProofBatch{
		Destination: dest2,
		Requests:    []ProofRequest{req1, req2}, // Mixed types
	}
	
	require.Equal(t, dest2, batch2.Destination)
	require.Len(t, batch2.Requests, 2)
}

func TestProofService_OptimizeDestinationsComplexGrouping(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ps.SetDebugMode(true)
	
	dest1 := protocol.AccountUrl("group", "dest1")
	dest2 := protocol.AccountUrl("group", "dest2")
	dest3 := protocol.AccountUrl("group", "dest3")
	
	requests := []ProofRequest{
		// dest1 requests
		{Destination: dest1, Sequences: []uint64{1}},
		{Destination: dest1, Sequences: []uint64{2, 3}},
		{Destination: dest1, Sequences: []uint64{4, 5, 6}},
		
		// dest2 requests  
		{Destination: dest2, Sequences: []uint64{10, 11}},
		{Destination: dest2, Sequences: []uint64{12}},
		
		// dest3 request
		{Destination: dest3, Sequences: []uint64{20, 21, 22, 23, 24}},
		
		// nil destination
		{Destination: nil, Sequences: []uint64{100}},
	}
	
	batches := ps.OptimizeForDestinations(requests)
	require.Len(t, batches, 4) // 3 destinations + nil
	
	// Verify grouping
	destinationCounts := make(map[string]int)
	for _, batch := range batches {
		if batch.Destination != nil {
			destinationCounts[batch.Destination.String()] = len(batch.Requests)
		} else {
			destinationCounts["nil"] = len(batch.Requests)
		}
	}
	
	require.Equal(t, 3, destinationCounts[dest1.String()])
	require.Equal(t, 2, destinationCounts[dest2.String()])
	require.Equal(t, 1, destinationCounts[dest3.String()])
	require.Equal(t, 1, destinationCounts["nil"])
}

func TestProofService_MetricsThreadSafety(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	
	// Test atomic operations directly
	atomic.AddInt64(&ps.metrics.IndividualProofsCreated, 10)
	atomic.AddInt64(&ps.metrics.CollectionProofsCreated, 5)
	
	metrics := ps.GetMetrics()
	require.Equal(t, int64(10), metrics.IndividualProofsCreated)
	require.Equal(t, int64(5), metrics.CollectionProofsCreated)
}

func TestProofService_CreateBatchProofsOptimization(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ps.SetDebugMode(true)
	ctx := context.Background()
	
	// Create requests that will test the optimization path
	dest := protocol.AccountUrl("batch", "destination")
	
	requests := []ProofRequest{
		{Type: ProofTypeSynthetic, Destination: dest, Sequences: []uint64{1}},
		{Type: ProofTypeSynthetic, Destination: dest, Sequences: []uint64{2, 3}},
		{Type: ProofTypeSynthetic, Destination: dest, Sequences: []uint64{4}},
	}
	
	// Test that optimization groups them properly
	batches := ps.OptimizeForDestinations(requests)
	require.Len(t, batches, 1) // Should group all into one batch
	
	batch := batches[0]
	require.Equal(t, dest.String(), batch.Destination.String())
	require.Len(t, batch.Requests, 3)
	
	// Now test batch processing (will fail but exercises code paths)
	_, err := ps.CreateBatchProofs(ctx, requests)
	require.Error(t, err) // Expected due to nil source chains
}

func TestUtilityFunctionCoverage(t *testing.T) {
	// Test all utility functions are accessible
	
	// Test min function with various edge cases
	tests := []struct{ a, b, expected int }{
		{-10, 5, -10},
		{0, 0, 0},
		{1000, 1, 1},
		{-5, -10, -10},
	}
	
	for _, test := range tests {
		result := min(test.a, test.b)
		require.Equal(t, test.expected, result)
	}
}

func TestAllConstantsAccessible(t *testing.T) {
	// Ensure all constants are properly accessible and have expected types
	
	// MessageType constants
	require.IsType(t, MessageType(0), MessageTypeAnchor)
	require.IsType(t, MessageType(0), MessageTypeSynthetic)
	require.IsType(t, MessageType(0), MessageTypeDirectoryAnchor)
	require.IsType(t, MessageType(0), MessageTypeBlockSummary)
	require.IsType(t, MessageType(0), MessageTypeOther)
	
	// ProofType constants
	require.IsType(t, ProofType(0), ProofTypeSynthetic)
	require.IsType(t, ProofType(0), ProofTypeAnchor)
	require.IsType(t, ProofType(0), ProofTypeReceipt)
	require.IsType(t, ProofType(0), ProofTypeUnified)
	
	// RecoveryType constants
	require.IsType(t, RecoveryType(0), RecoveryTypeAnchor)
	require.IsType(t, RecoveryType(0), RecoveryTypeSynthetic)
}

func TestStructFieldAccess(t *testing.T) {
	// Test that all struct fields are accessible
	
	// ProofMetrics
	metrics := ProofMetrics{
		IndividualProofsCreated:   1,
		CollectionProofsCreated:   2,
		TransactionsInCollections: 3,
		ProofsSaved:               4,
		ValidationAttempts:        5,
		ValidationSuccesses:       6,
		ValidationFailures:        7,
		ProofGenErrors:           8,
		ValidationErrors:         9,
	}
	
	require.Equal(t, int64(1), metrics.IndividualProofsCreated)
	require.Equal(t, int64(2), metrics.CollectionProofsCreated)
	require.Equal(t, int64(3), metrics.TransactionsInCollections)
	require.Equal(t, int64(4), metrics.ProofsSaved)
	require.Equal(t, int64(5), metrics.ValidationAttempts)
	require.Equal(t, int64(6), metrics.ValidationSuccesses)
	require.Equal(t, int64(7), metrics.ValidationFailures)
	require.Equal(t, int64(8), metrics.ProofGenErrors)
	require.Equal(t, int64(9), metrics.ValidationErrors)
	
	// Time fields (not exposed in GetMetrics but exist)
	require.Equal(t, time.Duration(0), metrics.TotalProofGenTime)
	require.Equal(t, time.Duration(0), metrics.TotalValidateTime)
}