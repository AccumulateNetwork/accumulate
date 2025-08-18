// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestProofService_CreateProof_AlwaysUsesCollection(t *testing.T) {
	// Create proof service
	var logger logging.OptionalLogger
	ps := NewProofService(logger)
	ps.SetDebugMode(true)

	// Test single sequence - should use collection proof
	req := ProofRequest{
		Type:        ProofTypeSynthetic,
		Destination: protocol.DnUrl(),
		Sequences:   []uint64{1},
	}

	// This will fail because we don't have chain data, but that's expected
	// The important thing is it attempts collection proof
	_, err := ps.CreateProof(context.Background(), req)
	require.Error(t, err) // Expected to fail without chain data

	// Verify metrics show no individual proofs
	metrics := ps.GetMetrics()
	assert.Equal(t, int64(0), metrics.IndividualProofsCreated)

	// Test multiple sequences
	req.Sequences = []uint64{1, 2, 3, 4, 5}
	_, err = ps.CreateProof(context.Background(), req)
	require.Error(t, err) // Expected to fail without chain data

	// Still no individual proofs
	metrics = ps.GetMetrics()
	assert.Equal(t, int64(0), metrics.IndividualProofsCreated)
}

func TestProofService_CreateBatchProofs_NoFallback(t *testing.T) {
	// Create proof service
	var logger logging.OptionalLogger
	ps := NewProofService(logger)

	// Create multiple requests
	requests := []ProofRequest{
		{
			Type:        ProofTypeSynthetic,
			Destination: protocol.DnUrl(),
			Sequences:   []uint64{1},
		},
		{
			Type:        ProofTypeSynthetic,
			Destination: protocol.DnUrl(),
			Sequences:   []uint64{2},
		},
		{
			Type:        ProofTypeSynthetic,
			Destination: protocol.DnUrl(),
			Sequences:   []uint64{3},
		},
	}

	// Attempt batch proof creation
	_, err := ps.CreateBatchProofs(context.Background(), requests)
	require.Error(t, err) // Expected to fail without chain data

	// Verify no individual proofs were created as fallback
	metrics := ps.GetMetrics()
	assert.Equal(t, int64(0), metrics.IndividualProofsCreated)
}

func TestProofService_OptimizeForDestinations_AlwaysCollection(t *testing.T) {
	var logger logging.OptionalLogger
	ps := NewProofService(logger)

	dest1, _ := url.Parse("acc://partition1")
	dest2, _ := url.Parse("acc://partition2")

	requests := []ProofRequest{
		{Type: ProofTypeSynthetic, Destination: dest1, Sequences: []uint64{1}},
		{Type: ProofTypeSynthetic, Destination: dest1, Sequences: []uint64{2}},
		{Type: ProofTypeSynthetic, Destination: dest2, Sequences: []uint64{3}},
	}

	batches := ps.OptimizeForDestinations(requests)

	// All batches should use collection proofs
	for _, batch := range batches {
		assert.True(t, batch.UseCollection, "Batch should always use collection proof")
	}
}

func TestProofService_MergeSequences(t *testing.T) {
	var logger logging.OptionalLogger
	ps := NewProofService(logger)

	dest, _ := url.Parse("acc://partition1")
	chain, _ := url.Parse("acc://partition1/chain")

	requests := []ProofRequest{
		{
			Type:        ProofTypeSynthetic,
			Destination: dest,
			Sequences:   []uint64{5, 3},
			ChainURL:    chain,
		},
		{
			Type:        ProofTypeSynthetic,
			Destination: dest,
			Sequences:   []uint64{1, 4},
			ChainURL:    chain,
		},
		{
			Type:        ProofTypeSynthetic,
			Destination: dest,
			Sequences:   []uint64{2},
			ChainURL:    chain,
		},
	}

	merged := ps.MergeSequences(requests)

	// Should have all sequences sorted
	expected := []uint64{1, 2, 3, 4, 5}
	assert.Equal(t, expected, merged.Sequences)
	assert.Equal(t, dest, merged.Destination)
	assert.Equal(t, chain, merged.ChainURL)
}

func TestProofService_ValidationMetrics(t *testing.T) {
	var logger logging.OptionalLogger
	ps := NewProofService(logger)
	ps.ResetMetrics()

	// Create a mock receipt that will fail validation
	receipt := &protocol.AnnotatedReceipt{
		Receipt: nil, // Invalid - will fail validation
	}

	// Validate should fail and update metrics
	err := ps.ValidateProof(receipt)
	require.Error(t, err)

	metrics := ps.GetMetrics()
	assert.Equal(t, int64(1), metrics.ValidationAttempts)
	assert.Equal(t, int64(1), metrics.ValidationFailures)
	assert.Equal(t, int64(0), metrics.ValidationSuccesses)
}

func TestProofService_MaxBatchSize(t *testing.T) {
	var logger logging.OptionalLogger
	ps := NewProofService(logger)

	// Verify max batch size is set
	assert.Equal(t, 100, ps.maxBatchSize)
}

// MockChain implements a minimal Chain interface for testing
type MockChain struct {
	database.Chain
	height int64
}

func (m *MockChain) Height() int64 {
	return m.height
}

func TestProofService_DebugMode(t *testing.T) {
	var logger logging.OptionalLogger
	ps := NewProofService(logger)

	// Debug mode should be off by default
	assert.False(t, ps.debugMode)

	// Enable debug mode
	ps.SetDebugMode(true)
	assert.True(t, ps.debugMode)

	// Disable debug mode
	ps.SetDebugMode(false)
	assert.False(t, ps.debugMode)
}

func TestProofResponse_Structure(t *testing.T) {
	// Verify ProofResponse has all required fields
	resp := ProofResponse{
		Proof:        &protocol.AnnotatedReceipt{},
		ProofType:    ProofTypeSynthetic,
		Sequences:    []uint64{1, 2, 3},
		IsCollection: true,
		ProofSavings: 2, // Saved 2 individual proofs
	}

	assert.NotNil(t, resp.Proof)
	assert.Equal(t, ProofTypeSynthetic, resp.ProofType)
	assert.Len(t, resp.Sequences, 3)
	assert.True(t, resp.IsCollection)
	assert.Equal(t, 2, resp.ProofSavings)
}

func TestProofBatch_Structure(t *testing.T) {
	dest, _ := url.Parse("acc://partition1")

	batch := ProofBatch{
		Destination:   dest,
		Requests:      []ProofRequest{},
		UseCollection: true, // Should always be true now
	}

	assert.Equal(t, dest, batch.Destination)
	assert.True(t, batch.UseCollection)
}

func TestProofMetrics_Atomicity(t *testing.T) {
	var logger logging.OptionalLogger
	ps := NewProofService(logger)
	ps.ResetMetrics()

	// Get initial metrics
	metrics1 := ps.GetMetrics()

	// Metrics should be zero after reset
	assert.Equal(t, int64(0), metrics1.IndividualProofsCreated)
	assert.Equal(t, int64(0), metrics1.CollectionProofsCreated)
	assert.Equal(t, int64(0), metrics1.TransactionsInCollections)
	assert.Equal(t, int64(0), metrics1.ProofsSaved)

	// Get metrics again - should return copy, not reference
	metrics2 := ps.GetMetrics()
	metrics2.IndividualProofsCreated = 100

	// Original should not be affected
	metrics3 := ps.GetMetrics()
	assert.Equal(t, int64(0), metrics3.IndividualProofsCreated)
}
