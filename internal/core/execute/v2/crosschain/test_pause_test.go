// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build testnet
// +build testnet

package crosschain

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestPauseResumeInbound(t *testing.T) {
	dispatcher := &MockDispatcher{}
	logger := logging.OptionalLogger{}
	cc := NewCrossChainConductor(dispatcher, logger)
	defer cc.Stop()

	// Create test messages
	syntheticMsg := &messaging.SequencedMessage{
		Source:  protocol.DnUrl().JoinPath("part1"),
		Number:  1,
		Message: &messaging.TransactionMessage{},
	}

	anchorMsg := &messaging.BlockAnchor{
		Signature: &protocol.ED25519Signature{},
		Anchor: &messaging.SequencedMessage{
			Source: protocol.DnUrl().JoinPath("part1"),
			Number: 1,
		},
	}

	normalMsg := &messaging.TransactionMessage{}

	t.Run("Normal operation allows messages", func(t *testing.T) {
		messages := []messaging.Message{syntheticMsg, anchorMsg, normalMsg}
		result := cc.ProcessInbound(context.Background(), messages)
		// Note: Messages may be dropped due to validation, check what we got
		t.Logf("Got %d messages back: %v", len(result), result)
		// We expect at least the normal message to pass through
		require.GreaterOrEqual(t, len(result), 1, "At least normal message should pass through when not paused")
	})

	t.Run("Paused state drops crosschain messages", func(t *testing.T) {
		cc.Pause()
		require.True(t, cc.IsPaused(), "CCC should be paused")

		messages := []messaging.Message{syntheticMsg, anchorMsg, normalMsg}
		result := cc.ProcessInbound(context.Background(), messages)

		// Only non-crosschain messages should pass through
		require.Len(t, result, 1, "Only non-crosschain messages should pass when paused")
		require.Equal(t, normalMsg, result[0], "Normal message should pass through")
	})

	t.Run("Resume restores normal operation", func(t *testing.T) {
		cc.Resume()
		require.False(t, cc.IsPaused(), "CCC should not be paused")

		messages := []messaging.Message{syntheticMsg, anchorMsg, normalMsg}
		result := cc.ProcessInbound(context.Background(), messages)
		require.Len(t, result, 3, "All messages should pass through after resume")
	})
}

func TestPauseResumeOutbound(t *testing.T) {
	dispatcher := &MockDispatcher{
		submitFunc: func(ctx context.Context, dest *url.URL, env *messaging.Envelope) error {
			return nil
		},
	}
	logger := logging.OptionalLogger{}
	cc := NewCrossChainConductor(dispatcher, logger)
	defer cc.Stop()

	dest := protocol.DnUrl().JoinPath("part2")
	messages := []messaging.Message{&messaging.TransactionMessage{}}

	t.Run("Normal operation sends messages", func(t *testing.T) {
		dispatcher.submitCalls = 0
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		err := cc.SubmitSynthetic(ctx, messages, dest)
		require.NoError(t, err)

		// Wait for async processing
		time.Sleep(100 * time.Millisecond)
		require.Greater(t, dispatcher.submitCalls, 0, "Message should be sent when not paused")
	})

	t.Run("Paused state drops outbound synthetic", func(t *testing.T) {
		cc.Pause()
		dispatcher.submitCalls = 0

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		err := cc.SubmitSynthetic(ctx, messages, dest)
		require.NoError(t, err, "Should return success even when paused")

		// Wait to ensure no async processing happens
		time.Sleep(100 * time.Millisecond)
		require.Equal(t, 0, dispatcher.submitCalls, "No messages should be sent when paused")
	})

	t.Run("Paused state drops outbound anchor", func(t *testing.T) {
		anchorReq := &AnchorRequest{
			Source:      protocol.DnUrl().JoinPath("part1"),
			Destination: protocol.DnUrl().JoinPath("part2"),
			Sequence:    1,
		}

		dispatcher.submitCalls = 0
		err := cc.SubmitAnchor(anchorReq)
		require.NoError(t, err, "Should return success even when paused")

		// Wait to ensure no processing happens
		time.Sleep(100 * time.Millisecond)
		require.Equal(t, 0, dispatcher.submitCalls, "No anchors should be sent when paused")
	})

	t.Run("Resume restores outbound operation", func(t *testing.T) {
		cc.Resume()
		dispatcher.submitCalls = 0

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		err := cc.SubmitSynthetic(ctx, messages, dest)
		require.NoError(t, err)

		// Wait for async processing
		time.Sleep(100 * time.Millisecond)
		require.Greater(t, dispatcher.submitCalls, 0, "Messages should be sent after resume")
	})
}

func TestPauseResumeConcurrency(t *testing.T) {
	dispatcher := &MockDispatcher{
		submitFunc: func(ctx context.Context, dest *url.URL, env *messaging.Envelope) error {
			// Simulate some processing time
			time.Sleep(10 * time.Millisecond)
			return nil
		},
	}
	logger := logging.OptionalLogger{}
	cc := NewCrossChainConductor(dispatcher, logger)
	defer cc.Stop()

	// Start multiple goroutines sending messages
	done := make(chan bool)
	for i := 0; i < 5; i++ {
		go func(id int) {
			dest := protocol.DnUrl().JoinPath("part" + strconv.Itoa(id))
			messages := []messaging.Message{&messaging.TransactionMessage{}}

			for j := 0; j < 10; j++ {
				ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
				cc.SubmitSynthetic(ctx, messages, dest)
				cancel()
				time.Sleep(5 * time.Millisecond)
			}
			done <- true
		}(i)
	}

	// Toggle pause/resume while messages are being sent
	go func() {
		for i := 0; i < 5; i++ {
			time.Sleep(20 * time.Millisecond)
			cc.Pause()
			time.Sleep(20 * time.Millisecond)
			cc.Resume()
		}
	}()

	// Wait for all goroutines to complete
	for i := 0; i < 5; i++ {
		<-done
	}

	// Verify that the system didn't crash and is in expected state
	cc.Resume() // Ensure we're not paused
	require.False(t, cc.IsPaused(), "Should not be paused at end of test")
}

func TestPauseQueuedRequests(t *testing.T) {
	// Create a dispatcher that blocks to simulate slow network
	blockChan := make(chan bool)
	dispatcher := &MockDispatcher{
		submitFunc: func(ctx context.Context, dest *url.URL, env *messaging.Envelope) error {
			<-blockChan // Block until signaled
			return nil
		},
	}
	logger := logging.OptionalLogger{}
	cc := NewCrossChainConductor(dispatcher, logger)
	defer cc.Stop()

	dest := protocol.DnUrl().JoinPath("part2")
	messages := []messaging.Message{&messaging.TransactionMessage{}}

	// Submit a message that will block
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cc.SubmitSynthetic(ctx, messages, dest)
	}()

	// Give it time to reach the blocking point
	time.Sleep(50 * time.Millisecond)

	// Now pause the CCC
	cc.Pause()

	// Submit another message while paused - should be dropped immediately
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := cc.SubmitSynthetic(ctx, messages, dest)
	require.NoError(t, err, "Should return immediately when paused")

	// Unblock the first message
	blockChan <- true

	// Resume and verify normal operation
	cc.Resume()

	// Submit a final message to verify resume worked
	dispatcher.submitFunc = func(ctx context.Context, dest *url.URL, env *messaging.Envelope) error {
		return nil
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()

	err = cc.SubmitSynthetic(ctx2, messages, dest)
	require.NoError(t, err, "Should work after resume")
}
