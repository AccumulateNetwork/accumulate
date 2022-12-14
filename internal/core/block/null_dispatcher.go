package block

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// nullDispatcher is a [Dispatcher] that discards everything.
type nullDispatcher struct{}

func (nullDispatcher) Submit(context.Context, *url.URL, *protocol.Envelope) error { return nil }

func (nullDispatcher) Send(context.Context) <-chan error {
	ch := make(chan error)
	close(ch)
	return ch
}
