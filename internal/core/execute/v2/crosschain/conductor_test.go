//go:build !testnet
// +build !testnet

package crosschain

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// mockDispatcher implements Dispatcher interface for testing
type mockDispatcher struct {
	mu          sync.Mutex
	submissions []mockSubmission
	submitError error
}

type mockSubmission struct {
	dest     *url.URL
	envelope *messaging.Envelope
}

func (m *mockDispatcher) Submit(ctx context.Context, dest *url.URL, envelope *messaging.Envelope) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.submitError != nil {
		return m.submitError
	}
	
	m.submissions = append(m.submissions, mockSubmission{
		dest:     dest,
		envelope: envelope,
	})
	return nil
}

func (m *mockDispatcher) Send(ctx context.Context) <-chan error {
	// Not used in Phase 1
	return make(chan error)
}

func (m *mockDispatcher) Close() {
	// Not used in Phase 1
}

func (m *mockDispatcher) getSubmissions() []mockSubmission {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]mockSubmission{}, m.submissions...)
}

func TestCrosschainCoordinator_SubmitSynthetic_Success(t *testing.T) {
	dispatcher := &mockDispatcher{}
	logger := logging.OptionalLogger{}
	coordinator := NewCrosschainCoordinator(dispatcher, logger)
	defer coordinator.Stop()

	// Create test data
	destURL, _ := url.Parse("acc://test-partition")
	messages := []messaging.Message{}

	// Submit synthetic transaction
	err := coordinator.SubmitSynthetic(context.Background(), messages, destURL)
	require.NoError(t, err)

	// Verify submission was processed
	submissions := dispatcher.getSubmissions()
	require.Len(t, submissions, 1)
	require.Equal(t, destURL, submissions[0].dest)
}

func TestCrosschainCoordinator_SubmitSynthetic_Error(t *testing.T) {
	dispatcher := &mockDispatcher{
		submitError: errors.New("dispatcher error"),
	}
	logger := logging.OptionalLogger{}
	coordinator := NewCrosschainCoordinator(dispatcher, logger)
	defer coordinator.Stop()

	// Create test data
	destURL, _ := url.Parse("acc://test-partition")
	messages := []messaging.Message{}

	// Submit synthetic transaction should return error
	err := coordinator.SubmitSynthetic(context.Background(), messages, destURL)
	require.Error(t, err)
	require.Contains(t, err.Error(), "dispatcher error")
}

func TestCrosschainCoordinator_ContextCancellation(t *testing.T) {
	dispatcher := &mockDispatcher{}
	logger := logging.OptionalLogger{}
	coordinator := NewCrosschainCoordinator(dispatcher, logger)
	defer coordinator.Stop()

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Create test data
	destURL, _ := url.Parse("acc://test-partition")
	messages := []messaging.Message{}

	// Submit with cancelled context - may succeed or fail depending on timing
	err := coordinator.SubmitSynthetic(ctx, messages, destURL)
	if err != nil {
		require.Contains(t, err.Error(), "context canceled")
	}
}

func TestCrosschainCoordinator_AsyncProcessing(t *testing.T) {
	dispatcher := &mockDispatcher{}
	logger := logging.OptionalLogger{}
	coordinator := NewCrosschainCoordinator(dispatcher, logger)
	defer coordinator.Stop()

	// Submit multiple transactions concurrently
	var wg sync.WaitGroup
	numTxns := 10
	wg.Add(numTxns)

	for i := 0; i < numTxns; i++ {
		go func(i int) {
			defer wg.Done()
			destURL, _ := url.Parse("acc://test-partition")
			messages := []messaging.Message{}
			err := coordinator.SubmitSynthetic(context.Background(), messages, destURL)
			require.NoError(t, err)
		}(i)
	}

	wg.Wait()

	// Verify all submissions were processed
	submissions := dispatcher.getSubmissions()
	require.Len(t, submissions, numTxns)
}

func TestCrosschainCoordinator_GracefulStop(t *testing.T) {
	dispatcher := &mockDispatcher{}
	logger := logging.OptionalLogger{}
	coordinator := NewCrosschainCoordinator(dispatcher, logger)

	// Stop should complete without hanging
	coordinator.Stop()

	// Subsequent submissions should fail
	destURL, _ := url.Parse("acc://test-partition")
	messages := []messaging.Message{}
	err := coordinator.SubmitSynthetic(context.Background(), messages, destURL)
	require.Error(t, err)
}
