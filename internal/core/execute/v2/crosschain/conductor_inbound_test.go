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

func TestProcessInbound(t *testing.T) {
	// Create a test conductor
	dispatcher := &MockDispatcher{}
	logger := logging.OptionalLogger{}
	cc := NewCrossChainConductor(dispatcher, logger)
	defer cc.Stop()

	ctx := context.Background()

	t.Run("Pass through non-crosschain messages", func(t *testing.T) {
		// Create a regular transaction message
		txn := &messaging.TransactionMessage{
			Transaction: &protocol.Transaction{
				Header: protocol.TransactionHeader{
					Principal: protocol.AcmeUrl(),
				},
			},
		}

		messages := []messaging.Message{txn}
		result := cc.ProcessInbound(ctx, messages)
		
		require.Len(t, result, 1)
		require.Equal(t, txn, result[0])
	})

	t.Run("Validate synthetic message", func(t *testing.T) {
		// Create a synthetic message
		seq := &messaging.SequencedMessage{
			Source:      protocol.DnUrl(),
			Destination: protocol.AcmeUrl(),
			Number:      1,
			Message: &messaging.TransactionMessage{
				Transaction: &protocol.Transaction{
					Header: protocol.TransactionHeader{
						Principal: protocol.AcmeUrl(),
					},
				},
			},
		}

		messages := []messaging.Message{seq}
		result := cc.ProcessInbound(ctx, messages)
		
		// Should be validated and passed through
		require.Len(t, result, 1)
	})

	t.Run("Handle recovery request", func(t *testing.T) {
		// Create a recovery request
		recovery := &messaging.RecoveryRequest{
			SourcePartition:      "BVN0",
			DestinationPartition: "BVN1",
			MessageType:          "synthetic",
			LastKnownSequence:    5,
		}

		messages := []messaging.Message{recovery}
		result := cc.ProcessInbound(ctx, messages)
		
		// Recovery requests should be handled and not passed through
		require.Len(t, result, 0)
	})

	t.Run("Validate block anchor", func(t *testing.T) {
		// Create a block anchor
		anchor := &messaging.BlockAnchor{
			Signature: &protocol.ED25519Signature{
				PublicKey: make([]byte, 32),
				Signature: make([]byte, 64),
			},
			Anchor: &messaging.SequencedMessage{
				Source:      protocol.DnUrl(),
				Destination: protocol.AcmeUrl(),
				Number:      1,
			},
		}

		messages := []messaging.Message{anchor}
		result := cc.ProcessInbound(ctx, messages)
		
		// Should be validated and passed through
		require.Len(t, result, 1)
	})

	t.Run("Mixed message types", func(t *testing.T) {
		// Create a mix of messages
		txn := &messaging.TransactionMessage{
			Transaction: &protocol.Transaction{
				Header: protocol.TransactionHeader{
					Principal: protocol.AcmeUrl(),
				},
			},
		}
		
		seq := &messaging.SequencedMessage{
			Source:      protocol.DnUrl(),
			Destination: protocol.AcmeUrl(),
			Number:      1,
			Message:     txn,
		}
		
		recovery := &messaging.RecoveryRequest{
			SourcePartition:      "BVN0",
			DestinationPartition: "BVN1",
			MessageType:          "synthetic",
			LastKnownSequence:    5,
		}

		messages := []messaging.Message{txn, seq, recovery}
		result := cc.ProcessInbound(ctx, messages)
		
		// Should have 2 messages (txn and seq), recovery handled separately
		require.Len(t, result, 2)
		require.Equal(t, txn, result[0])
		require.Equal(t, seq, result[1])
	})
}

func TestValidateInboundMessage(t *testing.T) {
	// Create a test conductor
	dispatcher := &MockDispatcher{}
	logger := logging.OptionalLogger{}
	cc := NewCrossChainConductor(dispatcher, logger)
	defer cc.Stop()

	t.Run("Valid sequenced message", func(t *testing.T) {
		seq := &messaging.SequencedMessage{
			Source:      protocol.DnUrl(),
			Destination: protocol.AcmeUrl(),
			Number:      1,
			Message: &messaging.TransactionMessage{
				Transaction: &protocol.Transaction{
					Header: protocol.TransactionHeader{
						Principal: protocol.AcmeUrl(),
					},
				},
			},
		}

		valid, reason := cc.validateInboundMessage(seq)
		require.True(t, valid)
		require.Empty(t, reason)
	})

	t.Run("Block anchor without signature", func(t *testing.T) {
		anchor := &messaging.BlockAnchor{
			Signature: nil, // Missing signature
			Anchor: &messaging.SequencedMessage{
				Source:      protocol.DnUrl(),
				Destination: protocol.AcmeUrl(),
				Number:      1,
			},
		}

		valid, reason := cc.validateInboundMessage(anchor)
		require.False(t, valid)
		require.Equal(t, "missing anchor signature", reason)
	})

	t.Run("Block anchor with signature", func(t *testing.T) {
		anchor := &messaging.BlockAnchor{
			Signature: &protocol.ED25519Signature{
				PublicKey: make([]byte, 32),
				Signature: make([]byte, 64),
			},
			Anchor: &messaging.SequencedMessage{
				Source:      protocol.DnUrl(),
				Destination: protocol.AcmeUrl(),
				Number:      1,
			},
		}

		valid, reason := cc.validateInboundMessage(anchor)
		require.True(t, valid)
		require.Empty(t, reason)
	})
}

func TestIsCrossPartitionMessage(t *testing.T) {
	// Create a test conductor
	dispatcher := &MockDispatcher{}
	logger := logging.OptionalLogger{}
	cc := NewCrossChainConductor(dispatcher, logger)
	defer cc.Stop()

	tests := []struct {
		name     string
		message  messaging.Message
		expected bool
	}{
		{
			name: "Synthetic message",
			message: &messaging.SyntheticMessage{
				Message: &messaging.SequencedMessage{
					Source:      protocol.DnUrl(),
					Destination: protocol.AcmeUrl(),
					Number:      1,
				},
			},
			expected: true,
		},
		{
			name: "Bad synthetic message",
			message: &messaging.BadSyntheticMessage{
				Message: &messaging.SequencedMessage{
					Source:      protocol.DnUrl(),
					Destination: protocol.AcmeUrl(),
					Number:      1,
				},
			},
			expected: true,
		},
		{
			name: "Block anchor",
			message: &messaging.BlockAnchor{
				Anchor: &messaging.SequencedMessage{
					Source:      protocol.DnUrl(),
					Destination: protocol.AcmeUrl(),
					Number:      1,
				},
			},
			expected: true,
		},
		{
			name: "Regular transaction",
			message: &messaging.TransactionMessage{
				Transaction: &protocol.Transaction{
					Header: protocol.TransactionHeader{
						Principal: protocol.AcmeUrl(),
					},
				},
			},
			expected: false,
		},
		{
			name: "Signature message",
			message: &messaging.SignatureMessage{
				Signature: &protocol.ED25519Signature{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cc.isCrossPartitionMessage(tt.message)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestHandleRecoveryRequest(t *testing.T) {
	// Create a test conductor with mock recovery manager
	dispatcher := &MockDispatcher{}
	logger := logging.OptionalLogger{}
	cc := NewCrossChainConductor(dispatcher, logger)
	defer cc.Stop()

	ctx := context.Background()

	t.Run("Valid recovery request", func(t *testing.T) {
		recovery := &messaging.RecoveryRequest{
			SourcePartition:      "BVN0",
			DestinationPartition: "BVN1",
			MessageType:          "synthetic",
			LastKnownSequence:    5,
		}

		// This should not panic and should handle the request
		cc.handleRecoveryRequest(ctx, recovery)
		// Note: Since handleRecoveryRequest runs async and uses internal recovery manager,
		// we can't easily verify the behavior without mocking the recovery manager
	})
}

func TestGetMessageType(t *testing.T) {
	// Create a test conductor
	dispatcher := &MockDispatcher{}
	logger := logging.OptionalLogger{}
	cc := NewCrossChainConductor(dispatcher, logger)
	defer cc.Stop()

	tests := []struct {
		name     string
		messages []messaging.Message
		expected MessageType
	}{
		{
			name: "Block anchor",
			messages: []messaging.Message{
				&messaging.BlockAnchor{},
			},
			expected: MessageTypeAnchor,
		},
		{
			name: "Synthetic message",
			messages: []messaging.Message{
				&messaging.SyntheticMessage{},
			},
			expected: MessageTypeSynthetic,
		},
		{
			name: "Bad synthetic message",
			messages: []messaging.Message{
				&messaging.BadSyntheticMessage{},
			},
			expected: MessageTypeSynthetic,
		},
		{
			name: "Block summary",
			messages: []messaging.Message{
				&messaging.BlockSummary{},
			},
			expected: MessageTypeBlockSummary,
		},
		{
			name: "Empty messages",
			messages: []messaging.Message{},
			expected: MessageTypeBlockSummary,
		},
		{
			name: "Unknown type",
			messages: []messaging.Message{
				&messaging.TransactionMessage{},
			},
			expected: MessageTypeBlockSummary,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cc.getMessageType(tt.messages)
			require.Equal(t, tt.expected, result)
		})
	}
}