package simulator

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/block"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// dispatcher implements [block.Dispatcher] for the simulator.
type dispatcher struct {
	sim       *Simulator
	envelopes map[string][]*protocol.Envelope
}

var _ block.Dispatcher = (*dispatcher)(nil)

func (d *dispatcher) Submit(ctx context.Context, u *url.URL, env *protocol.Envelope) error {
	partition, err := d.sim.router.RouteAccount(u)
	if err != nil {
		return err
	}

	d.envelopes[partition] = append(d.envelopes[partition], env)
	return nil
}

func (d *dispatcher) Send(ctx context.Context) <-chan error {
	envelopes := make(map[string][]*protocol.Envelope, len(d.envelopes))
	for p, e := range d.envelopes {
		envelopes[p] = e
	}

	// The compiler optimizes this to an O(1) operation
	for p := range d.envelopes {
		delete(d.envelopes, p)
	}

	errs := make(chan error)
	go func() {
		defer close(errs)

		for part, envelopes := range envelopes {
			for _, envelope := range envelopes {
				err := d.sim.SubmitTo(part, envelope)
				if err == nil {
					continue
				}
				var err2 *errors.Error
				if errors.As(err, &err2) && err2.Code == errors.Delivered {
					continue
				}
				errs <- err
			}
		}
	}()
	return errs
}
