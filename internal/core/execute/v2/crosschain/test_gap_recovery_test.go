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
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestDestinationSendState tests the destination state tracking
func TestDestinationSendState(t *testing.T) {
	dest := protocol.DnUrl().JoinPath("part1")
	state := NewDestinationSendState(dest)
	
	t.Run("InitialState", func(t *testing.T) {
		require.Equal(t, uint64(0), state.SentTxIndex)
		require.Equal(t, uint64(0), state.CurrentTxIndex)
		require.False(t, state.HasPendingMessages())
		
		start, end := state.GetSendRange()
		require.Equal(t, uint64(0), start)
		require.Equal(t, uint64(0), end)
	})
	
	t.Run("QueueMessages", func(t *testing.T) {
		// Queue some messages
		msg1 := &messaging.TransactionMessage{}
		msg2 := &messaging.TransactionMessage{}
		msg3 := &messaging.TransactionMessage{}
		
		state.QueueMessage(1, msg1)
		state.QueueMessage(2, msg2)
		state.QueueMessage(3, msg3)
		
		require.Equal(t, uint64(3), state.CurrentTxIndex)
		require.True(t, state.HasPendingMessages())
		require.Equal(t, uint64(3), state.GetGapSize())
		
		start, end := state.GetSendRange()
		require.Equal(t, uint64(1), start)
		require.Equal(t, uint64(3), end)
	})
	
	t.Run("MarkSendSuccess", func(t *testing.T) {
		// Mark send successful up to sequence 2
		state.MarkSendSuccess(2)
		
		require.Equal(t, uint64(2), state.SentTxIndex)
		require.Equal(t, uint64(3), state.CurrentTxIndex)
		require.True(t, state.HasPendingMessages())
		require.Equal(t, uint64(1), state.GetGapSize())
		
		// Should have cleaned up sent messages
		messages := state.CollectMessages(1, 2)
		require.Len(t, messages, 0, "Sent messages should be cleaned up")
		
		// Message 3 should still be there
		messages = state.CollectMessages(3, 3)
		require.Len(t, messages, 1)
	})
	
	t.Run("MarkSendFailure", func(t *testing.T) {
		initialSentIndex := state.SentTxIndex
		state.MarkSendFailure()
		
		// SentTxIndex should NOT change on failure
		require.Equal(t, initialSentIndex, state.SentTxIndex)
		require.Equal(t, 1, state.FailureCount)
		require.Equal(t, uint64(1), state.TotalFailed)
	})
	
	t.Run("ResetForGapRecovery", func(t *testing.T) {
		// Current state: SentTxIndex=2, CurrentTxIndex=3
		// Simulate gap recovery request saying "I only have up to 1"
		reset := state.ResetForGapRecovery(1)
		require.True(t, reset, "Should reset when going backwards")
		
		require.Equal(t, uint64(1), state.SentTxIndex)
		require.Equal(t, uint64(3), state.CurrentTxIndex)
		require.Equal(t, uint64(2), state.GetGapSize())
		
		// Next send should include sequences 2 and 3
		start, end := state.GetSendRange()
		require.Equal(t, uint64(2), start)
		require.Equal(t, uint64(3), end)
	})
	
	t.Run("NoResetWhenAhead", func(t *testing.T) {
		// Try to reset to a future sequence
		reset := state.ResetForGapRecovery(5)
		require.False(t, reset, "Should not reset when already behind")
		require.Equal(t, uint64(1), state.SentTxIndex, "Index should not change")
	})
}

// TestGapRecoveryFlow tests the complete gap recovery flow
func TestGapRecoveryFlow(t *testing.T) {
	dispatcher := &MockDispatcher{}
	logger := logging.OptionalLogger{}
	cc := NewCrossChainConductor(dispatcher, logger)
	defer cc.Stop()
	
	dest := protocol.DnUrl().JoinPath("part2")
	
	t.Run("NormalBatchSend", func(t *testing.T) {
		// Queue messages 1-5
		for i := uint64(1); i <= 5; i++ {
			msg := &messaging.SequencedMessage{
				Source:  protocol.DnUrl().JoinPath("part1"),
				Number:  i,
				Message: &messaging.TransactionMessage{},
			}
			cc.QueueMessageForDestination(dest, i, msg)
		}
		
		state := cc.getDestinationState(dest.String())
		require.NotNil(t, state)
		require.Equal(t, uint64(5), state.CurrentTxIndex)
		require.Equal(t, uint64(0), state.SentTxIndex)
		
		// Send batch
		ctx := context.Background()
		err := cc.sendBatchToDestination(ctx, dest)
		require.NoError(t, err)
		
		// Should have advanced SentTxIndex
		require.Equal(t, uint64(5), state.SentTxIndex)
		require.False(t, state.HasPendingMessages())
	})
	
	t.Run("FailedSendDoesNotAdvanceIndex", func(t *testing.T) {
		// Queue messages 6-8
		for i := uint64(6); i <= 8; i++ {
			msg := &messaging.SequencedMessage{
				Source:  protocol.DnUrl().JoinPath("part1"),
				Number:  i,
				Message: &messaging.TransactionMessage{},
			}
			cc.QueueMessageForDestination(dest, i, msg)
		}
		
		state := cc.getDestinationState(dest.String())
		require.Equal(t, uint64(8), state.CurrentTxIndex)
		require.Equal(t, uint64(5), state.SentTxIndex)
		
		// Simulate send failure
		dispatcher.submitFunc = func(ctx context.Context, dest *url.URL, env *messaging.Envelope) error {
			return errors.BadRequest.With("network error")
		}
		
		ctx := context.Background()
		err := cc.sendBatchToDestination(ctx, dest)
		require.Error(t, err)
		
		// SentTxIndex should NOT advance on failure
		require.Equal(t, uint64(5), state.SentTxIndex)
		require.Equal(t, uint64(8), state.CurrentTxIndex)
		require.True(t, state.HasPendingMessages())
		
		// Reset dispatcher
		dispatcher.submitFunc = nil
	})
	
	t.Run("GapRecoveryResetsIndex", func(t *testing.T) {
		state := cc.getDestinationState(dest.String())
		
		// Current state: SentTxIndex=5, CurrentTxIndex=8
		// Destination says "I only have up to sequence 3"
		gapReq := &messaging.RecoveryRequest{
			DestinationPartition: dest.String(),
			MessageType:          "synthetic",
			LastKnownSequence:    3,
		}
		
		ctx := context.Background()
		err := cc.HandleGapRequest(ctx, gapReq)
		require.NoError(t, err)
		
		// SentTxIndex should be reset to 3
		require.Equal(t, uint64(3), state.SentTxIndex)
		require.Equal(t, uint64(8), state.CurrentTxIndex)
		
		// Next send should include sequences 4-8
		start, end := state.GetSendRange()
		require.Equal(t, uint64(4), start)
		require.Equal(t, uint64(8), end)
	})
	
	t.Run("RetryAfterGapRecovery", func(t *testing.T) {
		// Re-queue messages 4-8 since they should still be in the queue after gap recovery
		// In a real system, these would still be in the queue
		for i := uint64(4); i <= 8; i++ {
			msg := &messaging.SequencedMessage{
				Source:  protocol.DnUrl().JoinPath("part1"),
				Number:  i,
				Message: &messaging.TransactionMessage{},
			}
			cc.QueueMessageForDestination(dest, i, msg)
		}
		
		// Now send should succeed and include all missing messages
		ctx := context.Background()
		err := cc.sendBatchToDestination(ctx, dest)
		require.NoError(t, err)
		
		state := cc.getDestinationState(dest.String())
		// Should have sent everything
		require.Equal(t, uint64(8), state.SentTxIndex)
		require.Equal(t, uint64(8), state.CurrentTxIndex)
		require.False(t, state.HasPendingMessages())
	})
}

// TestCumulativeBatchSending tests that failed sends accumulate
func TestCumulativeBatchSending(t *testing.T) {
	dispatcher := &MockDispatcher{}
	logger := logging.OptionalLogger{}
	cc := NewCrossChainConductor(dispatcher, logger)
	defer cc.Stop()
	
	dest := protocol.DnUrl().JoinPath("part2")
	
	// Queue messages 1-3
	for i := uint64(1); i <= 3; i++ {
		msg := &messaging.SequencedMessage{
			Source:  protocol.DnUrl().JoinPath("part1"),
			Number:  i,
			Message: &messaging.TransactionMessage{},
		}
		cc.QueueMessageForDestination(dest, i, msg)
	}
	
	// First send fails
	sendAttempts := 0
	messagesPerAttempt := []int{}
	
	dispatcher.submitFunc = func(ctx context.Context, dest *url.URL, env *messaging.Envelope) error {
		sendAttempts++
		messagesPerAttempt = append(messagesPerAttempt, len(env.Messages))
		
		if sendAttempts == 1 {
			return errors.BadRequest.With("first attempt fails")
		}
		return nil
	}
	
	ctx := context.Background()
	
	// First attempt fails
	err := cc.sendBatchToDestination(ctx, dest)
	require.Error(t, err)
	
	state := cc.getDestinationState(dest.String())
	require.Equal(t, uint64(0), state.SentTxIndex, "Index should not advance on failure")
	
	// Queue more messages (4-5)
	for i := uint64(4); i <= 5; i++ {
		msg := &messaging.SequencedMessage{
			Source:  protocol.DnUrl().JoinPath("part1"),
			Number:  i,
			Message: &messaging.TransactionMessage{},
		}
		cc.QueueMessageForDestination(dest, i, msg)
	}
	
	// Second attempt succeeds and includes ALL messages
	err = cc.sendBatchToDestination(ctx, dest)
	require.NoError(t, err)
	
	require.Equal(t, 2, sendAttempts)
	require.Equal(t, []int{3, 5}, messagesPerAttempt, "Second attempt should include all messages")
	require.Equal(t, uint64(5), state.SentTxIndex, "Index should advance after success")
}

// TestGapRecoveryMetrics tests that metrics are tracked correctly
func TestGapRecoveryMetrics(t *testing.T) {
	dest := protocol.DnUrl().JoinPath("part1")
	state := NewDestinationSendState(dest)
	
	// Simulate some operations
	state.QueueMessage(1, &messaging.TransactionMessage{})
	state.QueueMessage(2, &messaging.TransactionMessage{})
	state.MarkSendSuccess(2)
	
	state.QueueMessage(3, &messaging.TransactionMessage{})
	state.MarkSendFailure()
	
	// Gap recovery
	state.ResetForGapRecovery(0)
	
	metrics := state.GetMetrics()
	require.Equal(t, uint64(1), metrics["total_sent"])
	require.Equal(t, uint64(1), metrics["total_failed"])
	require.Equal(t, uint64(1), metrics["total_gap_resets"])
	require.Equal(t, uint64(2), metrics["largest_gap_reset"])
}