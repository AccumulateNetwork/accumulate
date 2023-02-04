// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// dispatcher implements [block.Dispatcher] for the simulator.
//
// dispatcher maintains a separate bundle (slice) of messages for each call to
// Submit to make it easier to write tests that drop certain messages.
type dispatcher struct {
	sim       *Simulator
	envelopes map[string][][]messaging.Message
}

var _ execute.Dispatcher = (*dispatcher)(nil)

// Submit routes the envelope and adds it to the queue for a partition.
func (d *dispatcher) Submit(ctx context.Context, u *url.URL, env *messaging.Envelope) error {
	partition, err := d.sim.router.RouteAccount(u)
	if err != nil {
		return err
	}

	deliveries, err := env.Normalize()
	if err != nil {
		return err
	}

	d.envelopes[partition] = append(d.envelopes[partition], deliveries)
	return nil
}

// Send submits queued envelopes to the respective partitions.
func (d *dispatcher) Send(ctx context.Context) <-chan error {
	envelopes := make(map[string][][]messaging.Message, len(d.envelopes))
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
			for _, envelopes := range envelopes {
				st, err := d.sim.SubmitTo(part, envelopes)
				if err != nil {
					errs <- err
					continue
				}
				for _, st := range st {
					if st.Error == nil {
						continue
					}
					if !st.Failed() {
						continue
					}
					if st.Code == errors.NotAllowed && st.Error.Message == "dropped" {
						continue
					}
					errs <- st.AsError()
				}
			}
		}
	}()
	return errs
}
