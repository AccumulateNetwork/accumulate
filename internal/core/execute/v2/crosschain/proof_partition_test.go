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
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestPartitionSpecificProofGeneration(t *testing.T) {
	// Create an in-memory database
	store := memory.New(nil)
	db := database.New(store, nil)

	// Create a batch for operations
	batch := db.Begin(true)
	defer batch.Discard()

	// Define source and destination partitions
	sourcePartition := protocol.PartitionUrl("BVN0")
	destPartition1 := protocol.PartitionUrl("BVN1")
	destPartition2 := protocol.PartitionUrl("BVN2")

	// Create synthetic account on source partition
	syntheticURL := sourcePartition.JoinPath(protocol.Synthetic)
	syntheticAccount := batch.Account(syntheticURL)

	// Initialize the synthetic ledger
	ledger := new(protocol.SyntheticLedger)
	ledger.Url = syntheticURL
	err := syntheticAccount.Main().Put(ledger)
	require.NoError(t, err)

	// Add entries to partition-specific sequence chains
	// For BVN1
	chain1 := syntheticAccount.SyntheticSequenceChain("BVN1")
	for i := 0; i < 5; i++ {
		entry := &protocol.IndexEntry{Source: uint64(i)}
		data, err := entry.MarshalBinary()
		require.NoError(t, err)
		err = chain1.Inner().AddEntry(data, false)
		require.NoError(t, err)
	}

	// For BVN2
	chain2 := syntheticAccount.SyntheticSequenceChain("BVN2")
	for i := 0; i < 3; i++ {
		entry := &protocol.IndexEntry{Source: uint64(i + 10)} // Different indices
		data, err := entry.MarshalBinary()
		require.NoError(t, err)
		err = chain2.Inner().AddEntry(data, false)
		require.NoError(t, err)
	}

	// Commit the batch
	err = batch.Commit()
	require.NoError(t, err)

	// Create a new read batch
	batch = db.Begin(false)
	defer batch.Discard()

	// Create the conductor with proof service
	testLogger := logging.NewTestLogger(t, "plain", "info", false)
	optLogger := logging.OptionalLogger{L: testLogger}
	conductor := &CrossChainConductor{
		logger:       optLogger,
		proofService: NewProofService(optLogger),
	}
	conductor.proofService.SetDebugMode(true)

	// Create synthetic transactions for testing
	transactions := []SyntheticTransaction{
		// Multiple transactions to BVN1
		{
			Destination: destPartition1,
			SequenceNum: 1,
			ChainURL:    syntheticURL,
		},
		{
			Destination: destPartition1,
			SequenceNum: 2,
			ChainURL:    syntheticURL,
		},
		{
			Destination: destPartition1,
			SequenceNum: 3,
			ChainURL:    syntheticURL,
		},
		// Transactions to BVN2
		{
			Destination: destPartition2,
			SequenceNum: 1,
			ChainURL:    syntheticURL,
		},
		{
			Destination: destPartition2,
			SequenceNum: 2,
			ChainURL:    syntheticURL,
		},
	}

	// Test the new method with partition-specific chains
	ctx := context.Background()
	proofs, err := conductor.CreateProofsForSyntheticTransactionsWithPartitions(
		ctx,
		batch,
		sourcePartition,
		transactions,
		nil, // No root chain for this test
	)

	require.NoError(t, err, "Should create proofs with partition-specific chains")
	require.NotNil(t, proofs, "Proofs should not be nil")
	require.Len(t, proofs, len(transactions), "Should have proof for each transaction")

	// Verify metrics show collection proofs were created
	metrics := conductor.proofService.GetMetrics()
	t.Logf("Proof metrics: individual=%d, collection=%d, saved=%d",
		metrics.IndividualProofsCreated,
		metrics.CollectionProofsCreated,
		metrics.ProofsSaved)

	// We should have created collection proofs for both partitions
	require.Equal(t, int64(2), metrics.CollectionProofsCreated, "Should create 2 collection proofs (one per partition)")
	require.Equal(t, int64(3), metrics.ProofsSaved, "Should save 3 proofs (5 transactions - 2 collection proofs)")
}

func TestPartitionChainIsolation(t *testing.T) {
	// This test verifies that each partition has its own isolated sequence chain
	store := memory.New(nil)
	db := database.New(store, nil)

	batch := db.Begin(true)
	defer batch.Discard()

	sourcePartition := protocol.PartitionUrl("BVN0")
	syntheticURL := sourcePartition.JoinPath(protocol.Synthetic)
	syntheticAccount := batch.Account(syntheticURL)

	// Initialize the synthetic ledger
	ledger := new(protocol.SyntheticLedger)
	ledger.Url = syntheticURL
	err := syntheticAccount.Main().Put(ledger)
	require.NoError(t, err)

	// Create different chains for different partitions
	partitions := []string{"BVN1", "BVN2", "DN"}

	for _, partition := range partitions {
		chain := syntheticAccount.SyntheticSequenceChain(partition)

		// Add unique entries for each partition
		for i := 0; i < 3; i++ {
			entry := &protocol.IndexEntry{
				Source: uint64(i*100) + uint64(len(partition)), // Unique pattern per partition
			}
			data, err := entry.MarshalBinary()
			require.NoError(t, err)
			err = chain.Inner().AddEntry(data, false)
			require.NoError(t, err)
		}
	}

	// Commit and reload
	err = batch.Commit()
	require.NoError(t, err)

	batch = db.Begin(false)
	defer batch.Discard()
	syntheticAccount = batch.Account(syntheticURL)

	// Verify each partition has its own isolated chain
	for _, partition := range partitions {
		chain, err := syntheticAccount.SyntheticSequenceChain(partition).Get()
		require.NoError(t, err)
		require.Equal(t, int64(3), chain.Height(), "Each partition chain should have 3 entries")

		// Verify the first entry has the expected pattern
		entry, err := chain.Entry(0)
		require.NoError(t, err)

		var indexEntry protocol.IndexEntry
		err = indexEntry.UnmarshalBinary(entry)
		require.NoError(t, err)

		expectedSource := uint64(len(partition))
		require.Equal(t, expectedSource, indexEntry.Source,
			"First entry for partition %s should have source %d", partition, expectedSource)
	}

	t.Log("Successfully verified partition chain isolation")
}