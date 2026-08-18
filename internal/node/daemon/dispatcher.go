// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package accumulated

import (
	"context"
	"sync"

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
	network string
	router  routing.Router
	dialer  message.Dialer

	// mu guards messages: Submit appends from many goroutines (the healing
	// path dispatches recovered messages concurrently) while Send reads and
	// clears the queue. Without it the slice header is read torn — a nil data
	// pointer with a non-zero length — and RoundTrip crashes the node ranging
	// over it (observed mid-replay during a fast-sync rejoin, #4058).
	mu       sync.Mutex
	messages []message.Message
}

var _ execute.Dispatcher = (*dispatcher)(nil)

// NewDispatcher creates a new dispatcher.
func NewDispatcher(network string, router routing.Router, dialer message.Dialer) *dispatcher {
	d := new(dispatcher)
	d.network = network
	d.router = router
	d.dialer = dialer
	return d
}

func (d *dispatcher) Close() { /* Nothing to do */ }

// Submit routes the account URL, constructs a multiaddr, and queues addressed
// submit requests.
func (d *dispatcher) Submit(ctx context.Context, u *url.URL, env *messaging.Envelope) error {
	// A panic here takes down the node — observed twice from the conductor's
	// healing path during post-fast-sync replay (#4058). Report the problem
	// instead; the caller logs it and healing retries.
	if u == nil {
		return errors.InternalError.With("cannot submit: no destination")
	}
	if d.router == nil {
		return errors.InternalError.With("cannot submit: router not set")
	}

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
	d.mu.Lock()
	d.messages = append(d.messages, &message.Addressed{
		Address: addr,
		Message: &message.SubmitRequest{Envelope: env},
	})
	d.mu.Unlock()
	return nil
}

// Send sends all of the batches asynchronously using one connection per
// partition.
func (d *dispatcher) Send(ctx context.Context) <-chan error {
	d.mu.Lock()
	messages := d.messages
	d.messages = nil
	d.mu.Unlock()

	errs := make(chan error)
	check := func(err error) {
		if err == nil {
			return
		}

		// Benign: the message was already delivered. CometBFT's "tx already in
		// cache" variants went with CometBFT, but this one is ours and still
		// happens, so it is still filtered here. Reporting it would recreate
		// #4054's symptom from the other side — noise that buries the real
		// dispatch failures the fix exists to surface.
		var errObj *errors.Error
		if errors.As(err, &errObj) && errObj.Code == errors.Delivered {
			return
		}

		errs <- err
	}

	// Run asynchronously
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
				check(res.Error)
				return nil

			case *message.SubmitResponse:
				// Check for failed submissions
				for _, sub := range res.Value {
					if sub.Status != nil {
						check(sub.Status.AsError())
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
