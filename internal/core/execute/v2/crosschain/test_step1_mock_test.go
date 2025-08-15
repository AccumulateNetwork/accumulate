// Step 1: Create a working MockDispatcher
package crosschain

import (
	"context"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// MockDispatcher implements execute.Dispatcher for testing
type MockDispatcher struct {
	submitted []MockSubmission
	submitCalls int
	submitFunc func(ctx context.Context, dest *url.URL, env *messaging.Envelope) error
}

type MockSubmission struct {
	destination *url.URL
	envelope    *messaging.Envelope
}

func (m *MockDispatcher) Submit(ctx context.Context, destination *url.URL, envelope *messaging.Envelope) error {
	m.submitCalls++
	m.submitted = append(m.submitted, MockSubmission{
		destination: destination,
		envelope:    envelope,
	})
	if m.submitFunc != nil {
		return m.submitFunc(ctx, destination, envelope)
	}
	return nil
}

func (m *MockDispatcher) Close() {
	// Nothing to close in mock
}

func (m *MockDispatcher) Send(ctx context.Context) <-chan error {
	ch := make(chan error, 1)
	close(ch)
	return ch
}