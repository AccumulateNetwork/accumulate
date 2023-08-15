// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator/services"
)

// fakeDispatcher drops everything
type fakeDispatcher struct{}

func (fakeDispatcher) Submit(ctx context.Context, dest *url.URL, envelope *messaging.Envelope) error {
	// Drop it
	return nil
}

func (fakeDispatcher) Send(context.Context) <-chan error {
	// Nothing to do
	ch := make(chan error)
	close(ch)
	return ch
}

// dispatcher implements [block.Dispatcher] for the simulator.
//
// dispatcher maintains a separate bundle (slice) of messages for each call to
// Submit to make it easier to write tests that drop certain messages.
type dispatcher struct {
	client    *services.Network
	router    routing.Router
	envelopes map[string][]*messaging.Envelope
}

var _ execute.Dispatcher = (*dispatcher)(nil)

// Submit routes the envelope and adds it to the queue for a partition.
func (d *dispatcher) Submit(ctx context.Context, u *url.URL, env *messaging.Envelope) error {
	partition, err := d.router.RouteAccount(u)
	if err != nil {
		return err
	}

	d.envelopes[partition] = append(d.envelopes[partition], env)
	return nil
}

// Send submits queued envelopes to the respective partitions.
func (d *dispatcher) Send(ctx context.Context) <-chan error {
	envelopes := make(map[string][]*messaging.Envelope, len(d.envelopes))
	for p, e := range d.envelopes {
		envelopes[p] = e
	}

	// The compiler optimizes this to an O(1) operation
	for p := range d.envelopes {
		delete(d.envelopes, p)
	}

	// Run the dispatch asynchronously and return a channel because that's what
	// the dispatcher interface expects
	errs := make(chan error)
	go func() {
		defer close(errs)

		for part, envelopes := range envelopes {
			for _, envelope := range envelopes {
				addr := api.ServiceTypeSubmit.AddressFor(part).Multiaddr()
				st, err := d.client.ForAddress(addr).Submit(ctx, envelope, api.SubmitOptions{})
				if err != nil {
					errs <- err
					continue
				}
				for _, st := range st {
					if st.Success {
						continue
					}
					if st.Status == nil && st.Status.Error == nil {
						errs <- errors.UnknownError.With(st.Message)
						continue
					}
					if st.Status.Code == errors.NotAllowed && st.Status.Error.Message == "dropped" {
						continue
					}
					errs <- st.Status.AsError()
				}
			}
		}
	}()
	return errs
}
