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
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func TestCollectionProofGeneration(t *testing.T) {
	// Create an in-memory database
	store := memory.New(nil)
	db := database.New(store, nil)

	// Create a batch for operations
	batch := db.Begin(true)
	defer batch.Discard()

	// Create a test account and chain
	accountURL := url.MustParse("acc://test.acme/account")
	account := batch.Account(accountURL)

	// Create a chain and add some entries
	chainName := "test-chain"
	chain := account.MainChain()

	// Add multiple entries to the chain to test batch proofs
	numEntries := 10
	entries := make([][]byte, numEntries)
	for i := 0; i < numEntries; i++ {
		entry := []byte{byte(i)}
		entries[i] = entry
		err := chain.Inner().AddEntry(entry, false)
		require.NoError(t, err)
	}

	// Commit the batch to persist changes
	err := batch.Commit()
	require.NoError(t, err)

	// Create a new read batch to test proof generation
	batch = db.Begin(false)
	defer batch.Discard()

	// Get the chain again
	account = batch.Account(accountURL)
	chain = account.MainChain()

	// Test 1: Verify we can access the Inner() method
	inner := chain.Inner()
	require.NotNil(t, inner, "Inner() should return the MerkleManager")

	// Test 2: Generate a collection proof for a range of entries
	startIdx := int64(2)
	endIdx := int64(7)

	receiptList, err := merkle.GetReceiptList(inner, startIdx, endIdx)
	require.NoError(t, err, "GetReceiptList should succeed with valid merkle state")
	require.NotNil(t, receiptList, "Receipt list should not be nil")

	// Test 3: Verify entries in the receipt list
	require.Equal(t, int(endIdx-startIdx+1), len(receiptList.Elements), "Receipt list should contain correct number of elements")

	for i, elem := range receiptList.Elements {
		expectedEntry := entries[int(startIdx)+i]
		require.Equal(t, expectedEntry, elem, "Receipt list element %d should match original entry", i)
	}

	// Test 4: Test ProofService with collection proofs
	testLogger := logging.NewTestLogger(t, "plain", "info", false)
	optLogger := logging.OptionalLogger{L: testLogger}
	ps := NewProofService(optLogger)
	ps.SetDebugMode(true)

	// Create a proof request for multiple sequences
	sequences := []uint64{2, 3, 4, 5, 6, 7}

	// Convert Chain2 to Chain for the proof request
	chainWrapper, err := chain.Get()
	require.NoError(t, err)

	req := ProofRequest{
		ChainURL:    accountURL.JoinPath(chainName),
		SourceChain: chainWrapper,
		Sequences:   sequences,
	}

	// Generate collection proof
	ctx := context.Background()
	proof, err := ps.createCollectionProof(ctx, req)
	require.NoError(t, err, "Collection proof generation should succeed")
	require.NotNil(t, proof, "Collection proof should not be nil")

	// Verify the proof response
	require.NotNil(t, proof.Proof, "Proof should not be nil")
	require.NotNil(t, proof.Proof.Receipt, "Receipt should not be nil")
	require.True(t, proof.IsCollection, "Should be marked as collection proof")
	require.Equal(t, len(sequences)-1, proof.ProofSavings, "Should have correct proof savings")

	t.Logf("Successfully generated collection proof for %d entries (sequences %d-%d)",
		len(sequences), sequences[0], sequences[len(sequences)-1])
}

func TestCollectionProofWithLargerBatch(t *testing.T) {
	// Create an in-memory database
	store := memory.New(nil)
	db := database.New(store, nil)

	// Create a batch for operations
	batch := db.Begin(true)
	defer batch.Discard()

	// Create a test account and chain
	accountURL := url.MustParse("acc://test.acme/large-batch")
	account := batch.Account(accountURL)

	// Create a chain and add many entries
	chain := account.MainChain()

	// Add 100 entries to test larger batch proofs
	numEntries := 100
	for i := 0; i < numEntries; i++ {
		entry := make([]byte, 32) // Simulate hash-sized entries
		copy(entry, []byte{byte(i >> 8), byte(i)})
		err := chain.Inner().AddEntry(entry, false)
		require.NoError(t, err)
	}

	// Commit the batch
	err := batch.Commit()
	require.NoError(t, err)

	// Create a new read batch
	batch = db.Begin(false)
	defer batch.Discard()

	// Get the chain again
	account = batch.Account(accountURL)
	chain = account.MainChain()

	// Test collection proof for a large batch (50 entries)
	startIdx := int64(25)
	endIdx := int64(74) // 50 entries total

	receiptList, err := merkle.GetReceiptList(chain.Inner(), startIdx, endIdx)
	require.NoError(t, err, "GetReceiptList should handle large batches")
	require.NotNil(t, receiptList, "Receipt list should not be nil for large batch")

	// Verify the batch size
	expectedCount := int(endIdx - startIdx + 1)
	require.Equal(t, expectedCount, len(receiptList.Elements),
		"Receipt list should contain %d elements", expectedCount)

	t.Logf("Successfully generated collection proof for %d entries", expectedCount)

	// Compare proof sizes (this is informational)
	// In a real scenario, we'd compare the size of individual proofs vs collection proof
	t.Logf("Collection proof generated for batch of %d transactions", expectedCount)
	t.Logf("This would replace %d individual proofs with 1 collection proof", expectedCount)
}
