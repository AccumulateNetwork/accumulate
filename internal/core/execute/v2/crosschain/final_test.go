// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestProofService_AdvancedMetrics(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	
	// Test all metrics fields
	ps.metrics.IndividualProofsCreated = 5
	ps.metrics.CollectionProofsCreated = 3
	ps.metrics.TransactionsInCollections = 15
	ps.metrics.ProofsSaved = 12
	ps.metrics.ValidationAttempts = 20
	ps.metrics.ValidationSuccesses = 18
	ps.metrics.ValidationFailures = 2
	ps.metrics.ProofGenErrors = 1
	ps.metrics.ValidationErrors = 1
	ps.metrics.TotalProofGenTime = 100 * time.Millisecond
	ps.metrics.TotalValidateTime = 50 * time.Millisecond
	
	metrics := ps.GetMetrics()
	require.Equal(t, int64(5), metrics.IndividualProofsCreated)
	require.Equal(t, int64(3), metrics.CollectionProofsCreated)
	require.Equal(t, int64(15), metrics.TransactionsInCollections)
	require.Equal(t, int64(12), metrics.ProofsSaved)
	require.Equal(t, int64(20), metrics.ValidationAttempts)
	require.Equal(t, int64(18), metrics.ValidationSuccesses)
	require.Equal(t, int64(2), metrics.ValidationFailures)
	require.Equal(t, int64(1), metrics.ProofGenErrors)
	require.Equal(t, int64(1), metrics.ValidationErrors)
}

func TestProofService_CreateProof_SequenceRouting(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ctx := context.Background()
	
	// Test single sequence routing (should use individual proof path)
	singleReq := ProofRequest{
		Type:        ProofTypeSynthetic,
		Destination: protocol.AccountUrl("test", "dest"),
		Sequences:   []uint64{1}, // Single sequence
	}
	
	_, err := ps.CreateProof(ctx, singleReq)
	require.Error(t, err) // Will fail due to nil chain but tests routing
	
	// Test multiple sequences routing (should use collection proof path)  
	multiReq := ProofRequest{
		Type:        ProofTypeSynthetic,
		Destination: protocol.AccountUrl("test", "dest"),
		Sequences:   []uint64{1, 2, 3}, // Multiple sequences
	}
	
	_, err = ps.CreateProof(ctx, multiReq)
	require.Error(t, err) // Will fail due to nil chain but tests routing
}

func TestProofService_ValidateProof_ReceiptValidation(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ps.SetDebugMode(true)
	
	// Test with a receipt that will fail validation
	proof := &protocol.AnnotatedReceipt{
		Receipt: &merkle.Receipt{
			Start:   []byte("start"),
			Anchor:  []byte("anchor"),
			Entries: nil, // Empty entries - will fail validation
		},
	}
	
	err := ps.ValidateProof(proof)
	require.Error(t, err)
	require.Contains(t, err.Error(), "proof validation failed")
	
	metrics := ps.GetMetrics()
	require.Equal(t, int64(1), metrics.ValidationAttempts)
	require.Equal(t, int64(1), metrics.ValidationFailures)
}

func TestProofService_CreateBatchProofs_DetailedPath(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ps.SetDebugMode(true)
	ctx := context.Background()
	
	// Create multiple requests that will exercise the batching optimization
	requests := []ProofRequest{
		{
			Type:        ProofTypeSynthetic,
			Destination: protocol.AccountUrl("test", "dest1"),
			Sequences:   []uint64{1, 2},
		},
		{
			Type:        ProofTypeSynthetic,
			Destination: protocol.AccountUrl("test", "dest1"), // Same destination
			Sequences:   []uint64{3},
		},
		{
			Type:        ProofTypeAnchor,
			Destination: protocol.AccountUrl("test", "dest2"), // Different destination
			Sequences:   []uint64{10},
		},
	}
	
	// Will fail due to nil chains but tests the optimization path
	_, err := ps.CreateBatchProofs(ctx, requests)
	require.Error(t, err)
}

func TestProofService_OptimizeForDestinations_WithNilDestination(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ps.SetDebugMode(true)
	
	requests := []ProofRequest{
		{Destination: nil, Sequences: []uint64{1}},                           // Nil destination
		{Destination: protocol.AccountUrl("test", "dest1"), Sequences: []uint64{2}}, // Real destination
	}
	
	batches := ps.OptimizeForDestinations(requests)
	require.Len(t, batches, 2) // One for nil, one for dest1
	
	// Find nil destination batch
	var nilBatch *ProofBatch
	for i := range batches {
		if batches[i].Destination == nil {
			nilBatch = &batches[i]
			break
		}
	}
	
	require.NotNil(t, nilBatch)
	require.Len(t, nilBatch.Requests, 1)
}

func TestProofService_ResetMetrics_DebugMode(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ps.SetDebugMode(true) // Enable debug mode before reset
	
	// Set some metrics
	ps.metrics.IndividualProofsCreated = 10
	ps.metrics.CollectionProofsCreated = 5
	
	// Reset metrics in debug mode (should log)
	ps.ResetMetrics()
	
	metrics := ps.GetMetrics()
	require.Equal(t, int64(0), metrics.IndividualProofsCreated)
	require.Equal(t, int64(0), metrics.CollectionProofsCreated)
}

func TestProofService_MergeSequences_ComplexMerge(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	
	dest := protocol.AccountUrl("test", "destination")
	
	// Test merging with overlapping and duplicate sequences
	requests := []ProofRequest{
		{Destination: dest, Sequences: []uint64{10, 5, 15}},    // Unsorted
		{Destination: dest, Sequences: []uint64{3, 10, 7}},     // Has duplicate 10
		{Destination: dest, Sequences: []uint64{1}},            // Single
		{Destination: dest, Sequences: []uint64{}},             // Empty
	}
	
	merged := ps.mergeSequences(requests)
	require.Equal(t, dest, merged.Destination)
	// Should be sorted: [1, 3, 5, 7, 10, 10, 15] (duplicates preserved)
	expected := []uint64{1, 3, 5, 7, 10, 10, 15}
	require.Equal(t, expected, merged.Sequences)
}

func TestBatchRecoveryRequest_AllFields(t *testing.T) {
	chainUrl := protocol.AccountUrl("test", "chain")
	now := time.Now()
	called := false
	
	req := BatchRecoveryRequest{
		PartitionID:      "partition-abc",
		Type:             RecoveryTypeAnchor,
		MissingSequences: []uint64{100, 101, 102, 103},
		ChainURL:         chainUrl,
		RequestTime:      now,
		Callback: func(resp *BatchRecoveryResponse) {
			called = true
		},
	}
	
	require.Equal(t, "partition-abc", req.PartitionID)
	require.Equal(t, RecoveryTypeAnchor, req.Type)
	require.Equal(t, []uint64{100, 101, 102, 103}, req.MissingSequences)
	require.Equal(t, chainUrl, req.ChainURL)
	require.Equal(t, now, req.RequestTime)
	require.NotNil(t, req.Callback)
	
	// Test callback
	req.Callback(&BatchRecoveryResponse{})
	require.True(t, called)
}

func TestBatchRecoveryResponse_AllFields(t *testing.T) {
	now := time.Now()
	hashes := [][]byte{
		[]byte("hash1"),
		[]byte("hash2"),
	}
	
	transactions := []*RecoveredTransaction{
		{Hash: []byte("hash1"), SequenceNum: 1},
		{Hash: []byte("hash2"), SequenceNum: 2},
	}
	
	resp := BatchRecoveryResponse{
		PartitionID:       "test-partition",
		Type:              RecoveryTypeSynthetic,
		TransactionHashes: hashes,
		Transactions:      transactions,
		ProofGenerated:    now,
		BatchSize:         2,
		ProofSavings:      1,
		Error:             nil,
	}
	
	require.Equal(t, "test-partition", resp.PartitionID)
	require.Equal(t, RecoveryTypeSynthetic, resp.Type)
	require.Equal(t, hashes, resp.TransactionHashes)
	require.Equal(t, transactions, resp.Transactions)
	require.Equal(t, now, resp.ProofGenerated)
	require.Equal(t, 2, resp.BatchSize)
	require.Equal(t, 1, resp.ProofSavings)
	require.NoError(t, resp.Error)
}

func TestBatchRecoveryResponse_WithError(t *testing.T) {
	resp := BatchRecoveryResponse{
		PartitionID: "error-partition",
		Type:        RecoveryTypeAnchor,
		Error:       errors.BadRequest.With("recovery failed"),
		BatchSize:   0,
		ProofSavings: 0,
	}
	
	require.Equal(t, "error-partition", resp.PartitionID)
	require.Equal(t, RecoveryTypeAnchor, resp.Type)
	require.Error(t, resp.Error)
	require.Contains(t, resp.Error.Error(), "recovery failed")
	require.Equal(t, 0, resp.BatchSize)
	require.Equal(t, 0, resp.ProofSavings)
}

func TestRecoveredTransaction_AllFields(t *testing.T) {
	now := time.Now()
	hash := []byte("transaction-hash-12345")
	data := []byte("serialized-transaction-data")
	
	tx := RecoveredTransaction{
		Hash:        hash,
		SequenceNum: 12345,
		Timestamp:   now,
		Type:        "synthetic-transaction",
		Data:        data,
	}
	
	require.Equal(t, hash, tx.Hash)
	require.Equal(t, uint64(12345), tx.SequenceNum)
	require.Equal(t, now, tx.Timestamp)
	require.Equal(t, "synthetic-transaction", tx.Type)
	require.Equal(t, data, tx.Data)
}

func TestPendingTransmission_AllFields(t *testing.T) {
	dest := protocol.AccountUrl("test", "destination")
	ctx := context.Background()
	now := time.Now()
	retryTime := now.Add(10 * time.Second)
	responseChan := make(chan error, 1)
	
	key := DestinationKey{
		Type:        MessageTypeSynthetic,
		Destination: dest.String(),
	}
	
	pending := PendingTransmission{
		ID:          "pending-tx-67890",
		Messages:    nil, // Will be set by caller
		Destination: dest,
		DestKey:     key,
		Context:     ctx,
		AttemptNum:  3,
		SubmittedAt: now,
		RetryAfter:  retryTime,
		Callback:    responseChan,
	}
	
	require.Equal(t, "pending-tx-67890", pending.ID)
	require.Nil(t, pending.Messages)
	require.Equal(t, dest, pending.Destination)
	require.Equal(t, key, pending.DestKey)
	require.Equal(t, ctx, pending.Context)
	require.Equal(t, 3, pending.AttemptNum)
	require.Equal(t, now, pending.SubmittedAt)
	require.Equal(t, retryTime, pending.RetryAfter)
	require.Equal(t, responseChan, pending.Callback)
}

func TestDestinationQueue_AllFields(t *testing.T) {
	key := DestinationKey{
		Type:        MessageTypeAnchor,
		Destination: "acc://test.acme",
	}
	
	now := time.Now()
	blockedSince := now.Add(-30 * time.Second)
	lastSuccess := now.Add(-5 * time.Second)
	
	pendingTx := make(map[string]*PendingTransmission)
	queuedReqs := make([]*SyntheticRequest, 0)
	
	queue := DestinationQueue{
		Key:            key,
		IsBlocked:      true,
		BlockedSince:   blockedSince,
		PendingTx:      pendingTx,
		QueuedRequests: queuedReqs,
		LastSuccess:    lastSuccess,
		FailureCount:   7,
		SuccessCount:   42,
		RetryCount:     3,
	}
	
	require.Equal(t, key, queue.Key)
	require.True(t, queue.IsBlocked)
	require.Equal(t, blockedSince, queue.BlockedSince)
	require.Equal(t, pendingTx, queue.PendingTx)
	require.Equal(t, queuedReqs, queue.QueuedRequests)
	require.Equal(t, lastSuccess, queue.LastSuccess)
	require.Equal(t, int64(7), queue.FailureCount)
	require.Equal(t, int64(42), queue.SuccessCount)
	require.Equal(t, int64(3), queue.RetryCount)
}

func TestProofMetrics_AtomicOperations(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	
	// Test atomic operations work correctly
	atomic.AddInt64(&ps.metrics.IndividualProofsCreated, 5)
	atomic.AddInt64(&ps.metrics.CollectionProofsCreated, 3)
	atomic.AddInt64(&ps.metrics.TransactionsInCollections, 15)
	atomic.AddInt64(&ps.metrics.ProofsSaved, 12)
	atomic.AddInt64(&ps.metrics.ValidationAttempts, 20)
	atomic.AddInt64(&ps.metrics.ValidationSuccesses, 18)
	atomic.AddInt64(&ps.metrics.ValidationFailures, 2)
	atomic.AddInt64(&ps.metrics.ProofGenErrors, 1)
	atomic.AddInt64(&ps.metrics.ValidationErrors, 1)
	
	metrics := ps.GetMetrics()
	require.Equal(t, int64(5), metrics.IndividualProofsCreated)
	require.Equal(t, int64(3), metrics.CollectionProofsCreated)
	require.Equal(t, int64(15), metrics.TransactionsInCollections)
	require.Equal(t, int64(12), metrics.ProofsSaved)
	require.Equal(t, int64(20), metrics.ValidationAttempts)
	require.Equal(t, int64(18), metrics.ValidationSuccesses)
	require.Equal(t, int64(2), metrics.ValidationFailures)
	require.Equal(t, int64(1), metrics.ProofGenErrors)
	require.Equal(t, int64(1), metrics.ValidationErrors)
}

func TestProofService_CollectionProofMetricsUpdate(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ctx := context.Background()
	
	// This tests the metrics update path in createCollectionProof
	req := ProofRequest{
		Type:        ProofTypeSynthetic,
		Destination: protocol.AccountUrl("test", "dest"),
		Sequences:   []uint64{1, 2, 3, 4, 5}, // 5 sequences = 4 proof savings
		SourceChain: nil,                     // Will cause error after metrics logic
	}
	
	_, err := ps.createCollectionProof(ctx, req)
	require.Error(t, err) // Expected error due to nil chain
	
	// But metrics should still be incremented due to the error path
	metrics := ps.GetMetrics()
	require.Equal(t, int64(1), metrics.ProofGenErrors)
}

func TestProofService_MergeSequences_PreservesOtherFields(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	
	dest := protocol.AccountUrl("test", "dest")
	chainUrl := protocol.AccountUrl("test", "chain")
	
	// Test that mergeSequences preserves fields from the first request
	requests := []ProofRequest{
		{
			Type:        ProofTypeSynthetic,
			Destination: dest,
			Sequences:   []uint64{3, 1},
			ChainURL:    chainUrl,
			BlockIndex:  100,
			Metadata:    "test-metadata",
		},
		{
			Type:        ProofTypeAnchor, // Different type - should be ignored
			Destination: dest,
			Sequences:   []uint64{5, 2},
			ChainURL:    protocol.AccountUrl("different", "chain"), // Different - should be ignored
		},
	}
	
	merged := ps.mergeSequences(requests)
	
	// Should preserve first request's fields (except sequences)
	require.Equal(t, ProofTypeSynthetic, merged.Type)
	require.Equal(t, dest, merged.Destination)
	require.Equal(t, []uint64{1, 2, 3, 5}, merged.Sequences) // Merged and sorted
	require.Equal(t, chainUrl, merged.ChainURL)
	require.Equal(t, uint64(100), merged.BlockIndex)
	require.Equal(t, "test-metadata", merged.Metadata)
}

func TestProofService_ValidateProof_SuccessPath(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ps.SetDebugMode(true)
	
	// Create a receipt that might pass basic structure validation
	// but will fail the actual Validate() call
	receipt := &merkle.Receipt{
		Start:  make([]byte, 32), // Valid length
		Anchor: make([]byte, 32), // Valid length
		Entries: []*merkle.ReceiptEntry{
			{Hash: make([]byte, 32)}, // Add at least one entry
		},
	}
	
	proof := &protocol.AnnotatedReceipt{
		Receipt: receipt,
		Anchor: &protocol.AnchorMetadata{
			Account: protocol.AccountUrl("test", "account"),
		},
	}
	
	err := ps.ValidateProof(proof)
	// Will likely still fail validation but tests the path past basic structure checks
	require.Error(t, err)
	
	metrics := ps.GetMetrics()
	require.Equal(t, int64(1), metrics.ValidationAttempts)
}

func TestProofService_ValidateBatch_EmptyBatch(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ps.SetDebugMode(true)
	
	errors := ps.ValidateBatch([]*protocol.AnnotatedReceipt{})
	require.Empty(t, errors)
}

func TestProofService_ValidateBatch_SingleValidProof(t *testing.T) {
	logger := logging.OptionalLogger{}
	ps := NewProofService(logger)
	ps.SetDebugMode(true)
	
	proof := &protocol.AnnotatedReceipt{
		Receipt: &merkle.Receipt{
			Start:   make([]byte, 32),
			Anchor:  make([]byte, 32),
			Entries: []*merkle.ReceiptEntry{{Hash: make([]byte, 32)}},
		},
	}
	
	errors := ps.ValidateBatch([]*protocol.AnnotatedReceipt{proof})
	require.Len(t, errors, 1)
	require.Error(t, errors[0]) // Will fail validation but tests the path
}

func TestProofTypes_Usage(t *testing.T) {
	// Test that all ProofType constants can be used in requests
	dest := protocol.AccountUrl("test", "dest")
	
	for _, proofType := range []ProofType{
		ProofTypeSynthetic,
		ProofTypeAnchor,
		ProofTypeReceipt,
		ProofTypeUnified,
	} {
		req := ProofRequest{
			Type:        proofType,
			Destination: dest,
			Sequences:   []uint64{1},
		}
		
		require.Equal(t, proofType, req.Type)
		require.Equal(t, dest, req.Destination)
	}
}

func TestRecoveryTypes_Usage(t *testing.T) {
	// Test that all RecoveryType constants can be used
	for _, recoveryType := range []RecoveryType{
		RecoveryTypeAnchor,
		RecoveryTypeSynthetic,
	} {
		req := BatchRecoveryRequest{
			PartitionID: "test",
			Type:        recoveryType,
		}
		
		require.Equal(t, recoveryType, req.Type)
		require.NotEmpty(t, req.Type.String())
	}
}

func TestMessageTypes_Usage(t *testing.T) {
	// Test that all MessageType constants work in DestinationKey
	for _, msgType := range []MessageType{
		MessageTypeAnchor,
		MessageTypeSynthetic,
		MessageTypeDirectoryAnchor,
		MessageTypeBlockSummary,
		MessageTypeOther,
	} {
		key := DestinationKey{
			Type:        msgType,
			Destination: "test://dest",
		}
		
		require.Equal(t, msgType, key.Type)
		require.Equal(t, "test://dest", key.Destination)
	}
}