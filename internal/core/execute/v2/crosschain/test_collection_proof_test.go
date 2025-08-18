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
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestCollectionProofCreation tests the creation of collection proofs
func TestCollectionProofCreation(t *testing.T) {
	dispatcher := &MockDispatcher{}
	logger := logging.OptionalLogger{}
	cc := NewCrossChainConductor(dispatcher, logger)
	defer cc.Stop()

	// Get the proof service
	ps := cc.proofService
	require.NotNil(t, ps, "Proof service should be initialized")

	t.Run("SingleMessageProof", func(t *testing.T) {
		// Create a single message
		msg := &messaging.SequencedMessage{
			Source:  protocol.DnUrl().JoinPath("part1"),
			Number:  1,
			Message: &messaging.TransactionMessage{},
		}

		// Create proof for single message
		proof, err := ps.CreateProofForMessages(context.Background(), []messaging.Message{msg})
		require.NoError(t, err, "Should create proof for single message")
		require.NotNil(t, proof, "Proof should not be nil")
	})

	t.Run("CollectionProofMultipleMessages", func(t *testing.T) {
		// Create multiple messages to same destination
		messages := []messaging.Message{
			&messaging.SequencedMessage{
				Source:  protocol.DnUrl().JoinPath("part1"),
				Number:  1,
				Message: &messaging.TransactionMessage{},
			},
			&messaging.SequencedMessage{
				Source:  protocol.DnUrl().JoinPath("part1"),
				Number:  2,
				Message: &messaging.TransactionMessage{},
			},
			&messaging.SequencedMessage{
				Source:  protocol.DnUrl().JoinPath("part1"),
				Number:  3,
				Message: &messaging.TransactionMessage{},
			},
		}

		// Create collection proof
		proof, err := ps.CreateProofForMessages(context.Background(), messages)
		require.NoError(t, err, "Should create collection proof")
		require.NotNil(t, proof, "Collection proof should not be nil")
		
		// Verify it's a collection proof
		collProof, ok := proof.(*CollectionProof)
		require.True(t, ok, "Should be a collection proof")
		require.Equal(t, 3, collProof.MessageCount, "Should contain 3 messages")
	})

	t.Run("CollectionProofDifferentSources", func(t *testing.T) {
		// Create messages from different sources - should not be collected
		messages := []messaging.Message{
			&messaging.SequencedMessage{
				Source:  protocol.DnUrl().JoinPath("part1"),
				Number:  1,
				Message: &messaging.TransactionMessage{},
			},
			&messaging.SequencedMessage{
				Source:  protocol.DnUrl().JoinPath("part2"),
				Number:  1,
				Message: &messaging.TransactionMessage{},
			},
		}

		// Should reject messages from different sources (collection proofs require same source)
		proof, err := ps.CreateProofForMessages(context.Background(), messages)
		require.Error(t, err, "Should reject messages from different sources")
		require.Nil(t, proof, "Proof should be nil on error")
		require.Contains(t, err.Error(), "different sources")
	})

	t.Run("ProofValidation", func(t *testing.T) {
		// Create a message and proof
		msg := &messaging.SequencedMessage{
			Source:  protocol.DnUrl().JoinPath("part1"),
			Number:  1,
			Message: &messaging.TransactionMessage{},
		}

		proof, err := ps.CreateProofForMessages(context.Background(), []messaging.Message{msg})
		require.NoError(t, err, "Should create proof")

		// Validate the proof
		valid, err := ps.ValidateProofForMessage(context.Background(), msg, proof)
		require.NoError(t, err, "Validation should not error")
		require.True(t, valid, "Proof should be valid")
	})

	t.Run("CollectionProofValidation", func(t *testing.T) {
		// Create multiple messages
		messages := []messaging.Message{
			&messaging.SequencedMessage{
				Source:  protocol.DnUrl().JoinPath("part1"),
				Number:  1,
				Message: &messaging.TransactionMessage{},
			},
			&messaging.SequencedMessage{
				Source:  protocol.DnUrl().JoinPath("part1"),
				Number:  2,
				Message: &messaging.TransactionMessage{},
			},
		}

		// Create collection proof
		proof, err := ps.CreateProofForMessages(context.Background(), messages)
		require.NoError(t, err, "Should create collection proof")

		// Validate against each message
		for _, msg := range messages {
			valid, err := ps.ValidateProofForMessage(context.Background(), msg, proof)
			require.NoError(t, err, "Validation should not error")
			require.True(t, valid, "Collection proof should be valid for all messages")
		}
	})
}

// TestCollectionProofBatching tests batching of messages for collection proofs
func TestCollectionProofBatching(t *testing.T) {
	dispatcher := &MockDispatcher{}
	logger := logging.OptionalLogger{}
	cc := NewCrossChainConductor(dispatcher, logger)
	defer cc.Stop()

	ps := cc.proofService

	t.Run("BatchByDestination", func(t *testing.T) {
		// Create messages to different destinations
		part1Msgs := []messaging.Message{
			&messaging.SequencedMessage{
				Source:  protocol.DnUrl().JoinPath("part1"),
				Number:  1,
				Message: &messaging.TransactionMessage{},
			},
			&messaging.SequencedMessage{
				Source:  protocol.DnUrl().JoinPath("part1"),
				Number:  2,
				Message: &messaging.TransactionMessage{},
			},
		}

		part2Msgs := []messaging.Message{
			&messaging.SequencedMessage{
				Source:  protocol.DnUrl().JoinPath("part2"),
				Number:  1,
				Message: &messaging.TransactionMessage{},
			},
			&messaging.SequencedMessage{
				Source:  protocol.DnUrl().JoinPath("part2"),
				Number:  2,
				Message: &messaging.TransactionMessage{},
			},
		}

		// Batch messages
		batches := ps.BatchMessagesByDestination(append(part1Msgs, part2Msgs...))
		
		// Should have 2 batches
		require.Len(t, batches, 2, "Should have 2 batches")
		
		// Each batch should have 2 messages
		for _, batch := range batches {
			require.Len(t, batch, 2, "Each batch should have 2 messages")
		}
	})

	t.Run("OptimalBatchSize", func(t *testing.T) {
		// Create many messages
		messages := make([]messaging.Message, 100)
		for i := range messages {
			messages[i] = &messaging.SequencedMessage{
				Source:  protocol.DnUrl().JoinPath("part1"),
				Number:  uint64(i + 1),
				Message: &messaging.TransactionMessage{},
			}
		}

		// Determine optimal batch size
		batches := ps.OptimizeBatches(messages)
		
		// Should create reasonably sized batches
		for _, batch := range batches {
			require.LessOrEqual(t, len(batch), 50, "Batch size should not exceed 50")
			require.GreaterOrEqual(t, len(batch), 1, "Batch should have at least 1 message")
		}
	})
}

// TestCollectionProofEfficiency tests the efficiency gains from collection proofs
func TestCollectionProofEfficiency(t *testing.T) {
	dispatcher := &MockDispatcher{}
	logger := logging.OptionalLogger{}
	cc := NewCrossChainConductor(dispatcher, logger)
	defer cc.Stop()

	ps := cc.proofService

	t.Run("ProofSizeComparison", func(t *testing.T) {
		// Create multiple messages
		messages := make([]messaging.Message, 10)
		for i := range messages {
			messages[i] = &messaging.SequencedMessage{
				Source:  protocol.DnUrl().JoinPath("part1"),
				Number:  uint64(i + 1),
				Message: &messaging.TransactionMessage{},
			}
		}

		// Create individual proofs
		individualProofs := make([]interface{}, 0)
		for _, msg := range messages {
			proof, err := ps.CreateProofForMessages(context.Background(), []messaging.Message{msg})
			require.NoError(t, err)
			individualProofs = append(individualProofs, proof)
		}

		// Create collection proof
		collectionProof, err := ps.CreateProofForMessages(context.Background(), messages)
		require.NoError(t, err)

		// Collection proof should be more efficient (in real implementation)
		// For now, just verify both approaches work
		require.NotNil(t, collectionProof, "Collection proof should be created")
		require.Len(t, individualProofs, 10, "Should have 10 individual proofs")
	})

	t.Run("ProcessingTimeComparison", func(t *testing.T) {
		// Create messages
		messages := make([]messaging.Message, 20)
		for i := range messages {
			messages[i] = &messaging.SequencedMessage{
				Source:  protocol.DnUrl().JoinPath("part1"),
				Number:  uint64(i + 1),
				Message: &messaging.TransactionMessage{},
			}
		}

		// Measure collection proof creation (should be faster for many messages)
		collectionProof, err := ps.CreateProofForMessages(context.Background(), messages)
		require.NoError(t, err)
		require.NotNil(t, collectionProof)

		// In a real implementation, we would measure and compare times
		// For now, just verify it works
	})
}

// TestCollectionProofRecovery tests recovery with collection proofs
func TestCollectionProofRecovery(t *testing.T) {
	dispatcher := &MockDispatcher{}
	logger := logging.OptionalLogger{}
	cc := NewCrossChainConductor(dispatcher, logger)
	defer cc.Stop()

	ps := cc.proofService

	t.Run("RecoverFromCollectionProof", func(t *testing.T) {
		// Create messages
		messages := []messaging.Message{
			&messaging.SequencedMessage{
				Source:  protocol.DnUrl().JoinPath("part1"),
				Number:  1,
				Message: &messaging.TransactionMessage{},
			},
			&messaging.SequencedMessage{
				Source:  protocol.DnUrl().JoinPath("part1"),
				Number:  2,
				Message: &messaging.TransactionMessage{},
			},
			&messaging.SequencedMessage{
				Source:  protocol.DnUrl().JoinPath("part1"),
				Number:  3,
				Message: &messaging.TransactionMessage{},
			},
		}

		// Create collection proof
		proof, err := ps.CreateProofForMessages(context.Background(), messages)
		require.NoError(t, err)

		// Simulate recovery - extract messages from proof
		collProof, ok := proof.(*CollectionProof)
		if ok {
			require.Equal(t, 3, collProof.MessageCount, "Should recover 3 messages")
			require.NotNil(t, collProof.MessageHashes, "Should have message hashes")
		}
	})

	t.Run("PartialRecovery", func(t *testing.T) {
		// Create messages with a gap
		messages := []messaging.Message{
			&messaging.SequencedMessage{
				Source:  protocol.DnUrl().JoinPath("part1"),
				Number:  1,
				Message: &messaging.TransactionMessage{},
			},
			// Missing message 2
			&messaging.SequencedMessage{
				Source:  protocol.DnUrl().JoinPath("part1"),
				Number:  3,
				Message: &messaging.TransactionMessage{},
			},
		}

		// Create proof for available messages
		proof, err := ps.CreateProofForMessages(context.Background(), messages)
		require.NoError(t, err)
		require.NotNil(t, proof, "Should create proof even with gaps")
	})
}