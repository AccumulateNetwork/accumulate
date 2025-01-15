// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dial

import "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"

type Option interface {
	apply(*dialer)
}

type optionFunc func(*dialer)

func (fn optionFunc) apply(d *dialer) { fn(d) }

// New creates a new [message.Dialer]. New will panic if the connector,
// discoverer, or tracker is unspecified.
func New(opts ...Option) message.Dialer {
	d := &dialer{}
	for _, opt := range opts {
		opt.apply(d)
	}
	if d.tracker == nil {
		panic("missing tracker")
	}
	if d.host == nil {
		panic("missing connector")
	}
	if d.peers == nil {
		panic("missing discoverer")
	}
	return d
}

func WithDiscoverer(v Discoverer) Option {
	return optionFunc(func(d *dialer) {
		d.peers = v
	})
}

func WithConnector(v Connector) Option {
	return optionFunc(func(d *dialer) {
		d.host = v
	})
}

func WithTracker(v Tracker) Option {
	return optionFunc(func(d *dialer) {
		d.tracker = v
	})
}
