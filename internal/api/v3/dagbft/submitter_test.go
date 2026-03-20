// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/txtracker"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// mockSubmitter is a mock implementation of TransactionSubmitter.
type mockSubmitter struct {
	submissions atomic.Int32
	err         error
}

func (m *mockSubmitter) Submit(ctx context.Context, tx []byte) error {
	m.submissions.Add(1)
	return m.err
}

// mockValidator is a mock implementation of TransactionValidator.
type mockValidator struct {
	err error
}

func (m *mockValidator) ValidateTransaction(tx []byte) error {
	return m.err
}

func TestDAGSubmitter_Submit(t *testing.T) {
	mock := &mockSubmitter{}
	tracker := txtracker.NewTracker(txtracker.TrackerConfig{})
	defer tracker.Close()

	submitter := NewDAGSubmitter(
		DAGSubmitterConfig{},
		mock,
		nil,
		tracker,
	)
	defer submitter.Close()

	env := &messaging.Envelope{
		Messages: []messaging.Message{
			&messaging.TransactionMessage{
				Transaction: &protocol.Transaction{
					Body: &protocol.SendTokens{},
				},
			},
		},
	}

	err := submitter.SubmitEnvelope(context.Background(), env)
	require.NoError(t, err)

	assert.Equal(t, int32(1), mock.submissions.Load())
}

func TestDAGSubmitter_SubmitRaw(t *testing.T) {
	mock := &mockSubmitter{}
	tracker := txtracker.NewTracker(txtracker.TrackerConfig{})
	defer tracker.Close()

	submitter := NewDAGSubmitter(
		DAGSubmitterConfig{},
		mock,
		nil,
		tracker,
	)
	defer submitter.Close()

	tx := []byte("test transaction")
	err := submitter.SubmitRaw(context.Background(), tx)
	require.NoError(t, err)

	assert.Equal(t, int32(1), mock.submissions.Load())
}

func TestDAGSubmitter_ValidationFailed(t *testing.T) {
	mock := &mockSubmitter{}
	validator := &mockValidator{err: errors.New("invalid transaction")}

	submitter := NewDAGSubmitter(
		DAGSubmitterConfig{},
		mock,
		validator,
		nil,
	)
	defer submitter.Close()

	tx := []byte("test transaction")
	err := submitter.SubmitRaw(context.Background(), tx)
	assert.True(t, errors.Is(err, ErrValidationFailed))
	assert.Equal(t, int32(0), mock.submissions.Load())
}

func TestDAGSubmitter_TransactionTooLarge(t *testing.T) {
	mock := &mockSubmitter{}

	submitter := NewDAGSubmitter(
		DAGSubmitterConfig{
			MaxTxSize: 10,
		},
		mock,
		nil,
		nil,
	)
	defer submitter.Close()

	tx := make([]byte, 100)
	err := submitter.SubmitRaw(context.Background(), tx)
	assert.Equal(t, ErrTransactionTooLarge, err)
	assert.Equal(t, int32(0), mock.submissions.Load())
}

func TestDAGSubmitter_EmptyTransaction(t *testing.T) {
	mock := &mockSubmitter{}

	submitter := NewDAGSubmitter(
		DAGSubmitterConfig{},
		mock,
		nil,
		nil,
	)
	defer submitter.Close()

	err := submitter.SubmitRaw(context.Background(), nil)
	assert.Error(t, err)

	err = submitter.SubmitRaw(context.Background(), []byte{})
	assert.Error(t, err)
}

func TestDAGSubmitter_SubmitError(t *testing.T) {
	mock := &mockSubmitter{err: errors.New("submission failed")}
	tracker := txtracker.NewTracker(txtracker.TrackerConfig{})
	defer tracker.Close()

	submitter := NewDAGSubmitter(
		DAGSubmitterConfig{},
		mock,
		nil,
		tracker,
	)
	defer submitter.Close()

	tx := []byte("test transaction")
	err := submitter.SubmitRaw(context.Background(), tx)
	assert.Error(t, err)
}

func TestDAGSubmitter_BroadcastModeSync(t *testing.T) {
	mock := &mockSubmitter{}
	tracker := txtracker.NewTracker(txtracker.TrackerConfig{})
	defer tracker.Close()

	submitter := NewDAGSubmitter(
		DAGSubmitterConfig{
			SyncTimeout: 100 * time.Millisecond,
		},
		mock,
		nil,
		tracker,
	)
	defer submitter.Close()

	tx := []byte("test transaction")

	// Start submission in goroutine
	done := make(chan error)
	go func() {
		done <- submitter.SubmitRawWithMode(context.Background(), tx, BroadcastModeSync)
	}()

	// Give it time to submit and wait
	time.Sleep(20 * time.Millisecond)

	// Get the hash and mark as batched
	hash := tracker.Track(tx) // This will return the same hash (Track deduplicates)
	tracker.SetStatus(hash, txtracker.StatusBatched)

	// Wait should complete
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("BroadcastModeSync timed out")
	}
}

func TestDAGSubmitter_Closed(t *testing.T) {
	mock := &mockSubmitter{}

	submitter := NewDAGSubmitter(
		DAGSubmitterConfig{},
		mock,
		nil,
		nil,
	)

	submitter.Close()

	err := submitter.SubmitRaw(context.Background(), []byte("test"))
	assert.Equal(t, ErrSubmitterClosed, err)
}

func TestDAGSubmitter_NilEnvelope(t *testing.T) {
	mock := &mockSubmitter{}

	submitter := NewDAGSubmitter(
		DAGSubmitterConfig{},
		mock,
		nil,
		nil,
	)
	defer submitter.Close()

	err := submitter.SubmitEnvelope(context.Background(), nil)
	assert.Error(t, err)
}

func TestDAGSubmitter_Backpressure(t *testing.T) {
	// Simulate backpressure error from the worker
	mock := &mockSubmitter{err: worker.ErrBackpressure}

	submitter := NewDAGSubmitter(
		DAGSubmitterConfig{},
		mock,
		nil,
		nil,
	)
	defer submitter.Close()

	tx := []byte("test transaction")
	err := submitter.SubmitRaw(context.Background(), tx)

	// The error should propagate (will be wrapped by caller with TooManyRequests)
	assert.True(t, errors.Is(err, worker.ErrBackpressure))
	assert.Equal(t, int32(1), mock.submissions.Load())
}
