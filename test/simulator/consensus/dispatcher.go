// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus

import (
	"context"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

type Dispatcher struct {
	router routing.Router
	mu     *sync.Mutex
	queue  []Message
}

func NewDispatcher(router routing.Router) *Dispatcher {
	return &Dispatcher{router: router, mu: new(sync.Mutex)}
}

// Submit adds an envelope to the queue.
func (d *Dispatcher) Submit(ctx context.Context, dest *url.URL, envelope *messaging.Envelope) error {
	partition, err := d.router.RouteAccount(dest)
	if err != nil {
		return err
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.queue = append(d.queue, &SubmitEnvelope{
		Network:  partition,
		Envelope: envelope,
	})
	return nil
}

// Send does nothing.
func (d *Dispatcher) Send(context.Context) <-chan error {
	ch := make(chan error)
	close(ch)
	return ch
}

// Receive returns the queued messages.
func (d *Dispatcher) Receive(messages ...Message) ([]Message, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	queue := d.queue
	d.queue = nil
	return queue, nil
}
