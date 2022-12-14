package simulator

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/chain"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// dispatcher implements [block.Dispatcher] for the simulator.
type dispatcher struct {
	sim       *Simulator
	envelopes map[string][]*chain.Delivery
}

var _ block.Dispatcher = (*dispatcher)(nil)

func (d *dispatcher) Submit(ctx context.Context, u *url.URL, env *protocol.Envelope) error {
	partition, err := d.sim.router.RouteAccount(u)
	if err != nil {
		return err
	}

	deliveries, err := chain.NormalizeEnvelope(env)
	if err != nil {
		return err
	}

	d.envelopes[partition] = append(d.envelopes[partition], deliveries...)
	return nil
}

func (d *dispatcher) Send(ctx context.Context) <-chan error {
	envelopes := make(map[string][]*chain.Delivery, len(d.envelopes))
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
				st, err := d.sim.SubmitTo(part, envelope)
				if err != nil {
					errs <- err
				} else if st.Error != nil && st.Code != errors.Delivered {
					errs <- st.AsError()
				}
			}
		}
	}()
	return errs
}
