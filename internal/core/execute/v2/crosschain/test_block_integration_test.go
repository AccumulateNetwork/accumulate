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

// TestBlockIntegration tests the integration of CCC with the block executor
func TestBlockIntegration(t *testing.T) {
	dispatcher := &MockDispatcher{}
	logger := logging.OptionalLogger{}
	cc := NewCrossChainConductor(dispatcher, logger)
	defer cc.Stop()

	// Get the block integration component
	bi := cc.blockIntegration
	require.NotNil(t, bi, "Block integration should be initialized")

	t.Run("PrepareBlockMessages", func(t *testing.T) {
		// Create test messages of different types
		messages := []messaging.Message{
			&messaging.SequencedMessage{
				Source:  protocol.DnUrl().JoinPath("part1"),
				Number:  1,
				Message: &messaging.TransactionMessage{},
			},
			&messaging.BlockAnchor{
				Signature: &protocol.ED25519Signature{},
			},
			&messaging.TransactionMessage{}, // Normal message
		}

		// Process through block integration
		prepared := bi.PrepareBlockMessages(context.Background(), messages)

		// Should return all messages (validation happens in ProcessInbound)
		require.Len(t, prepared, 3, "All messages should be prepared")
	})

	t.Run("CollectBlockProofs", func(t *testing.T) {
		// Create synthetic messages that would generate proofs
		synthetics := []messaging.Message{
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

		// Collect proofs for the block
		proofs := bi.CollectBlockProofs(context.Background(), synthetics)

		// For now, this returns empty since proof creation needs more setup
		require.NotNil(t, proofs, "Should return proof collection (even if empty)")
	})

	t.Run("FinalizeBlock", func(t *testing.T) {
		// Test block finalization
		blockHeight := uint64(100)
		blockTime := uint64(1234567890)

		err := bi.FinalizeBlock(context.Background(), blockHeight, blockTime)
		require.NoError(t, err, "Block finalization should succeed")
	})

	t.Run("HandleBlockBoundary", func(t *testing.T) {
		// Test handling of block boundaries
		oldHeight := uint64(100)
		newHeight := uint64(101)

		err := bi.HandleBlockBoundary(context.Background(), oldHeight, newHeight)
		require.NoError(t, err, "Block boundary handling should succeed")
	})
}

// TestBlockIntegrationWithMessages tests message flow through block integration
func TestBlockIntegrationWithMessages(t *testing.T) {
	dispatcher := &MockDispatcher{}
	logger := logging.OptionalLogger{}
	cc := NewCrossChainConductor(dispatcher, logger)
	defer cc.Stop()

	bi := cc.blockIntegration

	t.Run("MessageGrouping", func(t *testing.T) {
		// Create messages from different sources
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
			&messaging.SequencedMessage{
				Source:  protocol.DnUrl().JoinPath("part1"),
				Number:  2,
				Message: &messaging.TransactionMessage{},
			},
		}

		// Group messages by source
		grouped := bi.GroupMessagesBySource(messages)

		// Should have 2 groups (part1 and part2)
		require.Len(t, grouped, 2, "Should have 2 source groups")

		// Check part1 has 2 messages
		part1Key := protocol.DnUrl().JoinPath("part1").String()
		require.Contains(t, grouped, part1Key, "Should have part1 group")
		require.Len(t, grouped[part1Key], 2, "Part1 should have 2 messages")

		// Check part2 has 1 message
		part2Key := protocol.DnUrl().JoinPath("part2").String()
		require.Contains(t, grouped, part2Key, "Should have part2 group")
		require.Len(t, grouped[part2Key], 1, "Part2 should have 1 message")
	})

	t.Run("MessageOrdering", func(t *testing.T) {
		// Create out-of-order messages
		messages := []messaging.Message{
			&messaging.SequencedMessage{
				Source:  protocol.DnUrl().JoinPath("part1"),
				Number:  3,
				Message: &messaging.TransactionMessage{},
			},
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

		// Sort messages by sequence
		sorted := bi.SortMessagesBySequence(messages)

		// Should be in order 1, 2, 3
		require.Len(t, sorted, 3, "Should have all messages")

		// Check ordering
		for i, msg := range sorted {
			if seq, ok := msg.(*messaging.SequencedMessage); ok {
				require.Equal(t, uint64(i+1), seq.Number, "Message %d should have sequence %d", i, i+1)
			}
		}
	})
}

// TestBlockIntegrationAnchors tests anchor handling in block integration
func TestBlockIntegrationAnchors(t *testing.T) {
	dispatcher := &MockDispatcher{}
	logger := logging.OptionalLogger{}
	cc := NewCrossChainConductor(dispatcher, logger)
	defer cc.Stop()

	bi := cc.blockIntegration

	t.Run("AnchorCollection", func(t *testing.T) {
		// Create anchor messages
		anchors := []messaging.Message{
			&messaging.BlockAnchor{
				Signature: &protocol.ED25519Signature{},
				Anchor: &messaging.SequencedMessage{
					Source: protocol.DnUrl().JoinPath("part1"),
					Number: 1,
				},
			},
			&messaging.BlockAnchor{
				Signature: &protocol.ED25519Signature{},
				Anchor: &messaging.SequencedMessage{
					Source: protocol.DnUrl().JoinPath("part2"),
					Number: 1,
				},
			},
		}

		// Collect anchors for the block
		collected := bi.CollectAnchors(anchors)

		// Should have collected all anchors
		require.Len(t, collected, 2, "Should collect all anchors")
	})

	t.Run("AnchorValidation", func(t *testing.T) {
		// Create valid anchor
		validAnchor := &messaging.BlockAnchor{
			Signature: &protocol.ED25519Signature{
				Signer:    protocol.DnUrl().JoinPath("part1"),
				Signature: make([]byte, 64), // Mock signature
				PublicKey: make([]byte, 32), // Mock public key
			},
			Anchor: &messaging.SequencedMessage{
				Source: protocol.DnUrl().JoinPath("part1"),
				Number: 1,
			},
		}

		// Create invalid anchor (no signature)
		invalidAnchor := &messaging.BlockAnchor{
			Signature: nil,
			Anchor: &messaging.SequencedMessage{
				Source: protocol.DnUrl().JoinPath("part1"),
				Number: 2,
			},
		}

		// Validate anchors
		isValid := bi.ValidateAnchor(validAnchor)
		require.True(t, isValid, "Valid anchor should pass validation")

		isInvalid := bi.ValidateAnchor(invalidAnchor)
		require.False(t, isInvalid, "Invalid anchor should fail validation")
	})
}

// TestBlockIntegrationRecovery tests recovery handling during block processing
func TestBlockIntegrationRecovery(t *testing.T) {
	dispatcher := &MockDispatcher{}
	logger := logging.OptionalLogger{}
	cc := NewCrossChainConductor(dispatcher, logger)
	defer cc.Stop()

	bi := cc.blockIntegration

	t.Run("DetectMissingMessages", func(t *testing.T) {
		// Create messages with a gap (missing sequence 2)
		messages := []messaging.Message{
			&messaging.SequencedMessage{
				Source:  protocol.DnUrl().JoinPath("part1"),
				Number:  1,
				Message: &messaging.TransactionMessage{},
			},
			&messaging.SequencedMessage{
				Source:  protocol.DnUrl().JoinPath("part1"),
				Number:  3,
				Message: &messaging.TransactionMessage{},
			},
		}

		// Detect missing sequences
		missing := bi.DetectMissingSequences(messages, protocol.DnUrl().JoinPath("part1"))

		// Should detect sequence 2 is missing
		require.Len(t, missing, 1, "Should detect 1 missing sequence")
		require.Equal(t, uint64(2), missing[0], "Should detect sequence 2 is missing")
	})

	t.Run("TriggerRecovery", func(t *testing.T) {
		// Test triggering recovery for missing messages
		source := protocol.DnUrl().JoinPath("part1")
		missingSeqs := []uint64{2, 3, 4}

		err := bi.TriggerRecovery(context.Background(), source, missingSeqs)
		require.NoError(t, err, "Recovery trigger should succeed")
	})
}
