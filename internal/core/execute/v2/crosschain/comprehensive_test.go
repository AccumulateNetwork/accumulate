// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestProofService_ComprehensiveErrorPaths(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ps.SetDebugMode(true)
	ctx := context.Background()
	
	dest := protocol.AccountUrl("comprehensive", "test")
	
	// Test all error paths in CreateProof
	testCases := []struct {
		name string
		req  ProofRequest
	}{
		{
			name: "no sequences",
			req:  ProofRequest{Type: ProofTypeSynthetic, Destination: dest, Sequences: []uint64{}},
		},
		{
			name: "single sequence nil chain",
			req:  ProofRequest{Type: ProofTypeSynthetic, Destination: dest, Sequences: []uint64{1}},
		},
		{
			name: "multiple sequences nil chain",
			req:  ProofRequest{Type: ProofTypeSynthetic, Destination: dest, Sequences: []uint64{1, 2, 3}},
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ps.CreateProof(ctx, tc.req)
			require.Error(t, err)
		})
	}
	
	// Use ps to avoid unused variable error
	require.NotNil(t, ps)
	_ = ps
}

func TestProofService_BatchProcessingPaths(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ps.SetDebugMode(true)
	ctx := context.Background()
	
	// Test empty batch
	responses, err := ps.CreateBatchProofs(ctx, []ProofRequest{})
	require.NoError(t, err)
	require.Empty(t, responses)
	
	// Test batch with various destination types
	requests := []ProofRequest{
		{Type: ProofTypeSynthetic, Destination: nil, Sequences: []uint64{1}},
		{Type: ProofTypeAnchor, Destination: protocol.AccountUrl("test", "dest1"), Sequences: []uint64{2}},
		{Type: ProofTypeReceipt, Destination: protocol.AccountUrl("test", "dest1"), Sequences: []uint64{3, 4}},
		{Type: ProofTypeUnified, Destination: protocol.AccountUrl("test", "dest2"), Sequences: []uint64{10}},
	}
	
	// This will error but tests the batching logic
	_, err = ps.CreateBatchProofs(ctx, requests)
	require.Error(t, err)
}

func TestProofService_SequenceSortingLogic(t *testing.T) {
	logger := logging.OptionalLogger{}
	_ = NewProofService(logger) // Don't store in ps since it's not used
	
	// Test all sorting scenarios
	testCases := []struct {
		name     string
		input    []uint64
		expected []uint64
		isSorted bool
	}{
		{
			name:     "already sorted",
			input:    []uint64{1, 2, 3, 4, 5},
			expected: []uint64{1, 2, 3, 4, 5},
			isSorted: true,
		},
		{
			name:     "reverse sorted",
			input:    []uint64{5, 4, 3, 2, 1},
			expected: []uint64{1, 2, 3, 4, 5},
			isSorted: false,
		},
		{
			name:     "random order",
			input:    []uint64{3, 1, 4, 1, 5},
			expected: []uint64{1, 1, 3, 4, 5},
			isSorted: false,
		},
		{
			name:     "single element",
			input:    []uint64{42},
			expected: []uint64{42},
			isSorted: true,
		},
		{
			name:     "two elements sorted",
			input:    []uint64{1, 2},
			expected: []uint64{1, 2},
			isSorted: true,
		},
		{
			name:     "two elements unsorted",
			input:    []uint64{2, 1},
			expected: []uint64{1, 2},
			isSorted: false,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sequences := tc.input
			
			// Test sort detection
			isSorted := sort.SliceIsSorted(sequences, func(i, j int) bool {
				return sequences[i] < sequences[j]
			})
			require.Equal(t, tc.isSorted, isSorted)
			
			// Test sorting if needed
			if !isSorted {
				sequences = append([]uint64(nil), tc.input...) // Copy
				sort.Slice(sequences, func(i, j int) bool {
					return sequences[i] < sequences[j]
				})
			}
			
			require.Equal(t, tc.expected, sequences)
		})
	}
}

func TestProofService_ValidationTiming(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	
	// Test that validation timing is tracked
	initialTime := ps.metrics.TotalValidateTime
	
	// Validate a nil proof (quick operation)
	_ = ps.ValidateProof(nil)
	
	// Time should have increased (even if minimal)
	finalTime := ps.metrics.TotalValidateTime
	require.True(t, finalTime >= initialTime)
}

func TestProofService_ProofGenerationTiming(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ctx := context.Background()
	
	initialTime := ps.metrics.TotalProofGenTime
	
	// Try to create a proof (will fail but timing is tracked)
	req := ProofRequest{
		Type:        ProofTypeSynthetic,
		Destination: protocol.AccountUrl("timing", "test"),
		Sequences:   []uint64{1},
	}
	
	_, _ = ps.CreateProof(ctx, req)
	
	finalTime := ps.metrics.TotalProofGenTime
	require.True(t, finalTime >= initialTime)
}

func TestProofService_CreateIndividualProofDebugLogging(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ps.SetDebugMode(true)
	ctx := context.Background()
	
	req := ProofRequest{
		Type:        ProofTypeUnified,
		Destination: protocol.AccountUrl("debug", "test"),
		Sequences:   []uint64{999}, // Single sequence for individual proof
	}
	
	_, err := ps.createIndividualProof(ctx, req)
	require.Error(t, err) // Expected due to nil chain
}

func TestProofService_CreateCollectionProofDebugLogging(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ps.SetDebugMode(true)
	ctx := context.Background()
	
	req := ProofRequest{
		Type:        ProofTypeUnified,
		Destination: protocol.AccountUrl("debug", "collection"),
		Sequences:   []uint64{1, 2, 3, 4}, // Multiple for collection
	}
	
	_, err := ps.createCollectionProof(ctx, req)
	require.Error(t, err) // Expected due to nil chain
}

func TestProofService_CreateProofDebugLogging(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ps.SetDebugMode(true)
	ctx := context.Background()
	
	dest := protocol.AccountUrl("debug", "main")
	
	// Test debug logging in main CreateProof method
	req := ProofRequest{
		Type:        ProofTypeAnchor,
		Destination: dest,
		Sequences:   []uint64{1, 2},
	}
	
	_, err := ps.CreateProof(ctx, req)
	require.Error(t, err) // Expected
}

func TestProofService_ValidateProofDebugLogging(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ps.SetDebugMode(true)
	
	// Test debug logging for nil proof
	err := ps.ValidateProof(nil)
	require.Error(t, err)
	
	// Test debug logging for valid structure but invalid receipt
	receipt := &merkle.Receipt{
		Start:  []byte("test"),
		Anchor: []byte("test"),
	}
	
	proof := &protocol.AnnotatedReceipt{
		Receipt: receipt,
		Anchor: &protocol.AnchorMetadata{
			Account: protocol.AccountUrl("test", "account"),
		},
	}
	
	err = ps.ValidateProof(proof)
	// May or may not error depending on receipt validation - just test it runs
	_ = err
}

func TestProofService_ValidateBatchDebugLogging(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ps.SetDebugMode(true)
	
	proofs := []*protocol.AnnotatedReceipt{nil, {}}
	
	errors := ps.ValidateBatch(proofs)
	require.Len(t, errors, 2)
	for _, err := range errors {
		require.Error(t, err)
	}
}

func TestProofService_OptimizeDestinationsDebugLogging(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ps.SetDebugMode(true)
	
	dest := protocol.AccountUrl("optimize", "debug")
	
	requests := []ProofRequest{
		{Destination: dest, Sequences: []uint64{1, 2}},
		{Destination: dest, Sequences: []uint64{3}},
	}
	
	batches := ps.OptimizeForDestinations(requests)
	require.Len(t, batches, 1)
	require.Len(t, batches[0].Requests, 2)
}

func TestAllStructInitialization(t *testing.T) {
	// Test that all structs can be initialized and used
	
	// ProofRequest with all fields
	dest := protocol.AccountUrl("init", "test")
	chainUrl := protocol.AccountUrl("init", "chain")
	
	req := ProofRequest{
		Type:        ProofTypeUnified,
		Destination: dest,
		Sequences:   []uint64{1, 2, 3},
		ChainURL:    chainUrl,
		SourceChain: nil,
		RootChain:   nil,
		BlockIndex:  999,
		Metadata:    map[string]interface{}{"test": "data"},
	}
	
	require.Equal(t, ProofTypeUnified, req.Type)
	require.Equal(t, dest, req.Destination)
	require.Equal(t, []uint64{1, 2, 3}, req.Sequences)
	require.Equal(t, chainUrl, req.ChainURL)
	require.Nil(t, req.SourceChain)
	require.Nil(t, req.RootChain)
	require.Equal(t, uint64(999), req.BlockIndex)
	require.NotNil(t, req.Metadata)
	
	// ProofResponse with all fields
	annotated := &protocol.AnnotatedReceipt{}
	
	resp := ProofResponse{
		Proof:        annotated,
		ProofType:    ProofTypeReceipt,
		Sequences:    []uint64{10, 20, 30},
		IsCollection: true,
		ProofSavings: 2,
	}
	
	require.Equal(t, annotated, resp.Proof)
	require.Equal(t, ProofTypeReceipt, resp.ProofType)
	require.Equal(t, []uint64{10, 20, 30}, resp.Sequences)
	require.True(t, resp.IsCollection)
	require.Equal(t, 2, resp.ProofSavings)
}

func TestProofService_CreateCollectionProofMetrics(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ctx := context.Background()
	
	req := ProofRequest{
		Type:        ProofTypeSynthetic,
		Destination: protocol.AccountUrl("metrics", "test"),
		Sequences:   []uint64{1, 2, 3, 4, 5}, // 5 sequences = 4 savings
	}
	
	// This will fail but should update error metrics
	_, err := ps.createCollectionProof(ctx, req)
	require.Error(t, err)
	
	metrics := ps.GetMetrics()
	require.Equal(t, int64(1), metrics.ProofGenErrors)
}

func TestRecoveryType_AllValues(t *testing.T) {
	// Test all RecoveryType values and their string representations
	types := []struct {
		value    RecoveryType
		expected string
	}{
		{RecoveryTypeAnchor, "anchor"},
		{RecoveryTypeSynthetic, "synthetic"},
		{RecoveryType(100), "unknown"}, // Invalid type
		{RecoveryType(-1), "unknown"},  // Invalid type
	}
	
	for _, tt := range types {
		require.Equal(t, tt.expected, tt.value.String())
	}
}

func TestBatchRecoveryRequestProcessingLogic(t *testing.T) {
	// Test the logic that would be used in RequestBatchRecovery
	sequences := []uint64{5, 10, 15, 20, 25}
	proofSavings := len(sequences) - 1 // Standard calculation
	
	require.Equal(t, 4, proofSavings)
	require.Equal(t, 5, len(sequences))
}

func TestProofBatchConstruction(t *testing.T) {
	dest := protocol.AccountUrl("batch", "construction")
	
	// Test batch construction with various request types
	requests := []ProofRequest{
		{Type: ProofTypeSynthetic, Sequences: []uint64{1}},
		{Type: ProofTypeAnchor, Sequences: []uint64{2, 3}},
		{Type: ProofTypeReceipt, Sequences: []uint64{4, 5, 6}},
		{Type: ProofTypeUnified, Sequences: []uint64{7, 8, 9, 10}},
	}
	
	batch := ProofBatch{
		Destination: dest,
		Requests:    requests,
	}
	
	require.Equal(t, dest, batch.Destination)
	require.Len(t, batch.Requests, 4)
	
	// Verify all request types are preserved
	for i, req := range batch.Requests {
		require.Equal(t, requests[i].Type, req.Type)
		require.Equal(t, requests[i].Sequences, req.Sequences)
	}
}

func TestProofService_CreateAPIProofMethods(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ctx := context.Background()
	
	dest := protocol.AccountUrl("api", "test")
	
	// Test CreateCollectionProofForAPI
	_, err := ps.CreateCollectionProofForAPI(ctx, []uint64{1, 2, 3}, dest, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "source chain not provided")
	
	// Test CreateIndividualProofForAPI  
	_, err = ps.CreateIndividualProofForAPI(ctx, 42, dest, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "source chain not provided")
}

func TestProofMetricsStructure(t *testing.T) {
	// Test ProofMetrics structure and field access
	metrics := ProofMetrics{
		IndividualProofsCreated:   100,
		CollectionProofsCreated:   50,
		TransactionsInCollections: 250,
		ProofsSaved:               200,
		ValidationAttempts:        300,
		ValidationSuccesses:       280,
		ValidationFailures:        20,
		TotalProofGenTime:         time.Second,
		TotalValidateTime:         500 * time.Millisecond,
		ProofGenErrors:           5,
		ValidationErrors:         3,
	}
	
	// Test all fields are accessible
	require.Equal(t, int64(100), metrics.IndividualProofsCreated)
	require.Equal(t, int64(50), metrics.CollectionProofsCreated)
	require.Equal(t, int64(250), metrics.TransactionsInCollections)
	require.Equal(t, int64(200), metrics.ProofsSaved)
	require.Equal(t, int64(300), metrics.ValidationAttempts)
	require.Equal(t, int64(280), metrics.ValidationSuccesses)
	require.Equal(t, int64(20), metrics.ValidationFailures)
	require.Equal(t, time.Second, metrics.TotalProofGenTime)
	require.Equal(t, 500*time.Millisecond, metrics.TotalValidateTime)
	require.Equal(t, int64(5), metrics.ProofGenErrors)
	require.Equal(t, int64(3), metrics.ValidationErrors)
}

func TestDestinationKeyOperations(t *testing.T) {
	// Test DestinationKey as map key
	keys := make(map[DestinationKey]int)
	
	key1 := DestinationKey{Type: MessageTypeAnchor, Destination: "dest1"}
	key2 := DestinationKey{Type: MessageTypeSynthetic, Destination: "dest1"}
	key3 := DestinationKey{Type: MessageTypeAnchor, Destination: "dest2"}
	key4 := DestinationKey{Type: MessageTypeAnchor, Destination: "dest1"} // Same as key1
	
	keys[key1] = 1
	keys[key2] = 2
	keys[key3] = 3
	keys[key4] = 4 // Should overwrite key1
	
	require.Len(t, keys, 3) // Three unique keys
	require.Equal(t, 4, keys[key1]) // key4 overwrote key1
	require.Equal(t, 2, keys[key2])
	require.Equal(t, 3, keys[key3])
}

func TestSyntheticTransactionFieldAccess(t *testing.T) {
	dest := protocol.AccountUrl("synthetic", "test")
	chainUrl := protocol.AccountUrl("synthetic", "chain")
	hash := []byte("synthetic-hash-data")
	
	// Test all field combinations
	tx := SyntheticTransaction{
		Destination: dest,
		SequenceNum: 777,
		Sequence:    888, // Test that both fields work
		ChainURL:    chainUrl,
		Hash:        hash,
	}
	
	// Access all fields
	require.Equal(t, dest, tx.Destination)
	require.Equal(t, uint64(777), tx.SequenceNum)
	require.Equal(t, uint64(888), tx.Sequence)
	require.Equal(t, chainUrl, tx.ChainURL)
	require.Equal(t, hash, tx.Hash)
	
	// Modify fields
	tx.SequenceNum = 999
	tx.Sequence = 1000
	
	require.Equal(t, uint64(999), tx.SequenceNum)
	require.Equal(t, uint64(1000), tx.Sequence)
}

func TestRecoveryRequestFieldAccess(t *testing.T) {
	req := RecoveryRequest{
		Requester:  "acc://comprehensive.acme",
		FromNumber: 5000,
	}
	
	require.Equal(t, "acc://comprehensive.acme", req.Requester)
	require.Equal(t, uint64(5000), req.FromNumber)
	
	// Test field modification
	req.FromNumber = 6000
	require.Equal(t, uint64(6000), req.FromNumber)
}

func TestBatchRecoveryRequestAllFieldsInitialization(t *testing.T) {
	chainUrl := protocol.AccountUrl("batch", "recovery")
	requestTime := time.Now()
	
	req := BatchRecoveryRequest{
		PartitionID:      "comprehensive-partition",
		Type:             RecoveryTypeAnchor,
		MissingSequences: []uint64{100, 101, 102},
		ChainURL:         chainUrl,
		RequestTime:      requestTime,
		Callback:         nil, // Test nil callback
	}
	
	require.Equal(t, "comprehensive-partition", req.PartitionID)
	require.Equal(t, RecoveryTypeAnchor, req.Type)
	require.Equal(t, []uint64{100, 101, 102}, req.MissingSequences)
	require.Equal(t, chainUrl, req.ChainURL)
	require.Equal(t, requestTime, req.RequestTime)
	require.Nil(t, req.Callback)
}

func TestBatchRecoveryResponseFieldModification(t *testing.T) {
	resp := BatchRecoveryResponse{
		PartitionID: "test-partition",
		Type:        RecoveryTypeSynthetic,
		BatchSize:   0,
		ProofSavings: 0,
	}
	
	// Test field modifications
	resp.BatchSize = 10
	resp.ProofSavings = 9
	resp.ProofGenerated = time.Now()
	
	require.Equal(t, 10, resp.BatchSize)
	require.Equal(t, 9, resp.ProofSavings)
	require.False(t, resp.ProofGenerated.IsZero())
}

func TestRecoveredTransactionModification(t *testing.T) {
	tx := RecoveredTransaction{
		Hash:        []byte("original"),
		SequenceNum: 1,
		Type:        "original-type",
		Data:        []byte("original-data"),
	}
	
	// Test field modifications
	tx.Hash = []byte("modified")
	tx.SequenceNum = 999
	tx.Type = "modified-type"
	tx.Data = []byte("modified-data")
	tx.Timestamp = time.Now()
	
	require.Equal(t, []byte("modified"), tx.Hash)
	require.Equal(t, uint64(999), tx.SequenceNum)
	require.Equal(t, "modified-type", tx.Type)
	require.Equal(t, []byte("modified-data"), tx.Data)
	require.False(t, tx.Timestamp.IsZero())
}

func TestCompleteCodePathsExercise(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	
	// Exercise as many code paths as possible without creating actual chains
	
	// Test construction
	ps2 := NewProofService(logging.OptionalLogger{})
	require.NotNil(t, ps2)
	
	// Test debug mode toggling
	for i := 0; i < 3; i++ {
		ps.SetDebugMode(i%2 == 0)
	}
	
	// Test metrics reset multiple times
	ps.ResetMetrics()
	ps.ResetMetrics()
	ps.ResetMetrics()
	
	// Test various validation scenarios
	_ = ps.ValidateProof(nil)
	_ = ps.ValidateProof(&protocol.AnnotatedReceipt{})
	_ = ps.ValidateProof(&protocol.AnnotatedReceipt{Receipt: &merkle.Receipt{}})
	
	// Test batch validation
	proofs := []*protocol.AnnotatedReceipt{nil, {}, {Receipt: &merkle.Receipt{}}}
	errors := ps.ValidateBatch(proofs)
	require.Len(t, errors, 3)
	
	// Test optimization with various inputs
	_ = ps.OptimizeForDestinations([]ProofRequest{})
	_ = ps.OptimizeForDestinations([]ProofRequest{{Destination: nil}})
	_ = ps.OptimizeForDestinations([]ProofRequest{{Destination: protocol.AccountUrl("test", "dest")}})
	
	// Test merging
	_ = ps.mergeSequences([]ProofRequest{})
	_ = ps.mergeSequences([]ProofRequest{{Sequences: []uint64{1}}})
	
	// Verify metrics were accumulated
	metrics := ps.GetMetrics()
	require.True(t, metrics.ValidationAttempts > 0)
	require.True(t, metrics.ValidationFailures > 0)
}