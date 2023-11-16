// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package accumulated

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/exp/tendermint"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// dispatcher implements [block.Dispatcher].
type dispatcher struct {
	network  string
	router   routing.Router
	dialer   message.Dialer
	messages []message.Message
}

var _ execute.Dispatcher = (*dispatcher)(nil)

// newDispatcher creates a new dispatcher.
func newDispatcher(network string, router routing.Router, dialer message.Dialer) *dispatcher {
	d := new(dispatcher)
	d.network = network
	d.router = router
	d.dialer = dialer
	return d
}

// Submit routes the account URL, constructs a multiaddr, and queues addressed
// submit requests.
func (d *dispatcher) Submit(ctx context.Context, u *url.URL, env *messaging.Envelope) error {
	// If there's something wrong with the envelope, it's better for that error
	// to be logged closer to the source, at the sending side instead of the
	// receiving side
	_, err := env.Normalize()
	if err != nil {
		return err
	}

	// Route the account
	partition, err := d.router.RouteAccount(u)
	if err != nil {
		return err
	}

	// Construct the multiaddr, /acc/{network}/acc-svc/submit:{partition}
	addr, err := api.ServiceTypeSubmit.AddressFor(partition).MultiaddrFor(d.network)
	if err != nil {
		return err
	}

	// Queue a pre-addressed message
	d.messages = append(d.messages, &message.Addressed{
		Address: addr,
		Message: &message.SubmitRequest{Envelope: env},
	})
	return nil
}

// Send sends all of the batches asynchronously using one connection per
// partition.
func (d *dispatcher) Send(ctx context.Context) <-chan error {
	messages := d.messages
	d.messages = nil

	// Run asynchronously
	errs := make(chan error)
	go func() {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		defer close(errs)

		// Create a client using a batch dialer, but DO NOT set the router - all
		// the messages are already addressed
		tr := new(message.RoutedTransport)
		tr.Dialer = message.BatchDialer(ctx, d.dialer)

		// Submit all messages over a single stream
		err := tr.RoundTrip(ctx, messages, func(res, req message.Message) error {
			_ = req // Ignore unused warning

			switch res := res.(type) {
			case *message.ErrorResponse:
				// Handle error
				tendermint.CheckDispatchError(res.Error, errs)
				return nil

			case *message.SubmitResponse:
				// Check for failed submissions
				for _, sub := range res.Value {
					if sub.Status != nil {
						tendermint.CheckDispatchError(sub.Status.AsError(), errs)
					}
				}
				return nil

			default:
				return errors.Conflict.WithFormat("invalid response: want %T, got %T", (*message.SubmitResponse)(nil), res)
			}
		})
		if err != nil {
			errs <- errors.UnknownError.WithFormat("send requests: %w", err)
		}
	}()

	// Let the caller wait for errors
	return errs
}
