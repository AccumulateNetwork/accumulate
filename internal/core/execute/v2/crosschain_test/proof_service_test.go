// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain_test

import (
	"context"
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/crosschain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// createTestChain creates a test Merkle chain with entries
func createTestChain(t *testing.T, entries int) *database.Chain {
	store := storage.NewMemory(nil)
	chain := merkle.NewChain(store, nil, merkle.ChainTypeTransaction, "test", nil)
	
	for i := 0; i < entries; i++ {
		data := []byte{byte(i)}
		hash := sha256.Sum256(data)
		_, err := chain.AddEntry(hash[:], false)
		require.NoError(t, err)
	}
	
	return &database.Chain{Merkle: chain}
}

func TestProofService_CreateIndividualProof(t *testing.T) {
	logger := logging.NewTestLogger(t, "error", false)
	ps := crosschain.NewProofService(logger)
	ps.SetDebugMode(true)
	
	// Create test chains
	sourceChain := createTestChain(t, 10)
	rootChain := createTestChain(t, 10)
	
	ctx := context.Background()
	req := crosschain.ProofRequest{
		Type:        crosschain.ProofTypeSynthetic,
		Destination: url.MustParse("acc://bvn0"),
		Sequences:   []uint64{5},
		ChainURL:    url.MustParse("acc://dn/synthetic"),
		SourceChain: sourceChain,
		RootChain:   rootChain,
	}
	
	resp, err := ps.CreateProof(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	
	assert.Equal(t, crosschain.ProofTypeSynthetic, resp.ProofType)
	assert.Equal(t, []uint64{5}, resp.Sequences)
	assert.False(t, resp.IsCollection)
	assert.Equal(t, 0, resp.ProofSavings)
	assert.NotNil(t, resp.Proof)
	assert.NotNil(t, resp.Proof.Receipt)
	
	// Check metrics
	metrics := ps.GetMetrics()
	assert.Equal(t, int64(1), metrics.IndividualProofsCreated)
	assert.Equal(t, int64(0), metrics.CollectionProofsCreated)
}

func TestProofService_CreateCollectionProof(t *testing.T) {
	logger := logging.NewTestLogger(t, "error", false)
	ps := crosschain.NewProofService(logger)
	ps.SetDebugMode(true)
	
	// Create test chains
	sourceChain := createTestChain(t, 20)
	rootChain := createTestChain(t, 20)
	
	ctx := context.Background()
	req := crosschain.ProofRequest{
		Type:        crosschain.ProofTypeSynthetic,
		Destination: url.MustParse("acc://bvn0"),
		Sequences:   []uint64{5, 6, 7, 8, 9}, // 5 sequences should trigger collection proof
		ChainURL:    url.MustParse("acc://dn/synthetic"),
		SourceChain: sourceChain,
		RootChain:   rootChain,
	}
	
	resp, err := ps.CreateProof(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	
	assert.Equal(t, crosschain.ProofTypeSynthetic, resp.ProofType)
	assert.Equal(t, []uint64{5, 6, 7, 8, 9}, resp.Sequences)
	assert.True(t, resp.IsCollection)
	assert.Equal(t, 4, resp.ProofSavings) // Saved 4 individual proofs
	assert.NotNil(t, resp.Proof)
	assert.NotNil(t, resp.Proof.Receipt)
	
	// Check metrics
	metrics := ps.GetMetrics()
	assert.Equal(t, int64(0), metrics.IndividualProofsCreated)
	assert.Equal(t, int64(1), metrics.CollectionProofsCreated)
	assert.Equal(t, int64(5), metrics.TransactionsInCollections)
	assert.Equal(t, int64(4), metrics.ProofsSaved)
}

func TestProofService_BatchThreshold(t *testing.T) {
	logger := logging.NewTestLogger(t, "error", false)
	ps := crosschain.NewProofService(logger)
	ps.SetDebugMode(true)
	
	// Create test chains
	sourceChain := createTestChain(t, 10)
	
	ctx := context.Background()
	
	// Test with 1 sequence (should use individual proof)
	req1 := crosschain.ProofRequest{
		Type:        crosschain.ProofTypeSynthetic,
		Destination: url.MustParse("acc://bvn0"),
		Sequences:   []uint64{1},
		ChainURL:    url.MustParse("acc://dn/synthetic"),
		SourceChain: sourceChain,
	}
	
	resp1, err := ps.CreateProof(ctx, req1)
	require.NoError(t, err)
	assert.False(t, resp1.IsCollection)
	
	// Test with 2 sequences (should use collection proof with default threshold of 2)
	req2 := crosschain.ProofRequest{
		Type:        crosschain.ProofTypeSynthetic,
		Destination: url.MustParse("acc://bvn0"),
		Sequences:   []uint64{1, 2},
		ChainURL:    url.MustParse("acc://dn/synthetic"),
		SourceChain: sourceChain,
	}
	
	resp2, err := ps.CreateProof(ctx, req2)
	require.NoError(t, err)
	assert.True(t, resp2.IsCollection)
	assert.Equal(t, 1, resp2.ProofSavings)
}

func TestProofService_ValidateProof(t *testing.T) {
	logger := logging.NewTestLogger(t, "error", false)
	ps := crosschain.NewProofService(logger)
	ps.SetDebugMode(true)
	
	// Create a valid proof
	receipt := &merkle.Receipt{
		Start:  []byte{1, 2, 3, 4},
		Anchor: []byte{5, 6, 7, 8},
		Entries: []*merkle.ReceiptEntry{
			{Hash: []byte{9, 10, 11, 12}, Right: true},
		},
	}
	
	validProof := &protocol.AnnotatedReceipt{
		Receipt: receipt,
		Anchor: &protocol.AnchorMetadata{
			Account: url.MustParse("acc://dn"),
		},
	}
	
	// Test valid proof (NO CACHING - always validates)
	err := ps.ValidateProof(validProof)
	require.NoError(t, err)
	
	// Validate again - should still validate (no cache)
	err = ps.ValidateProof(validProof)
	require.NoError(t, err)
	
	// Check metrics - should show 2 validation attempts
	metrics := ps.GetMetrics()
	assert.Equal(t, int64(2), metrics.ValidationAttempts)
	assert.Equal(t, int64(2), metrics.ValidationSuccesses)
	assert.Equal(t, int64(0), metrics.ValidationFailures)
	
	// Test invalid proof
	invalidProof := &protocol.AnnotatedReceipt{
		Receipt: nil,
	}
	
	err = ps.ValidateProof(invalidProof)
	require.Error(t, err)
	
	// Check metrics
	metrics = ps.GetMetrics()
	assert.Equal(t, int64(3), metrics.ValidationAttempts)
	assert.Equal(t, int64(2), metrics.ValidationSuccesses)
	assert.Equal(t, int64(1), metrics.ValidationFailures)
}

func TestProofService_NoCaching(t *testing.T) {
	// This test verifies that the ProofService does NOT cache validation results
	// per the user's requirement for easier testing
	
	logger := logging.NewTestLogger(t, "error", false)
	ps := crosschain.NewProofService(logger)
	ps.SetDebugMode(true)
	
	// Create a proof that we'll validate multiple times
	receipt := &merkle.Receipt{
		Start:  []byte{1, 2, 3, 4},
		Anchor: []byte{5, 6, 7, 8},
		Entries: []*merkle.ReceiptEntry{
			{Hash: []byte{9, 10, 11, 12}, Right: true},
		},
	}
	
	proof := &protocol.AnnotatedReceipt{
		Receipt: receipt,
		Anchor: &protocol.AnchorMetadata{
			Account: url.MustParse("acc://dn"),
		},
	}
	
	// Reset metrics to start fresh
	ps.ResetMetrics()
	
	// Validate the same proof 5 times
	for i := 0; i < 5; i++ {
		err := ps.ValidateProof(proof)
		require.NoError(t, err)
	}
	
	// Without caching, all 5 validations should have been performed
	metrics := ps.GetMetrics()
	assert.Equal(t, int64(5), metrics.ValidationAttempts, "All validations should run without caching")
	assert.Equal(t, int64(5), metrics.ValidationSuccesses, "All validations should succeed")
}