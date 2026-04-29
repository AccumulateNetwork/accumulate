// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package advert publishes the bootstrap-v3 node-state advertisement
// over the existing libp2p service-discovery mechanism (#3991).
//
// Wire format: a ServiceAddress with Type = ServiceTypeBootstrap
// (added in this work) and Argument = "active" or "complete". Peers
// running the v3 dispatch layer can route current-state queries
// (Query, Event, Submit) only to nodes that advertise this service —
// a node still in BOOTING does not advertise, so its view of state
// can't be served to others.
//
// Producer side (this package): wire `Wire` to a nodestate.Machine.
// On every transition to ACTIVE / COMPLETE, the publisher registers
// a marker service with the local p2p Node. The handler is a no-op
// reject — the service exists for discovery, not RPC.
//
// Consumer side: routing rules in pkg/api/v3/p2p/dial that prefer
// peers carrying ServiceTypeBootstrap with the desired Argument.
// That wiring is a separate change tracked in #3991's done-state.
package advert

import (
	"fmt"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
)

// Registrar is the subset of *p2p.Node that advert needs. Avoids a
// hard dependency on the p2p package surface and lets tests fake it.
type Registrar interface {
	RegisterService(sa *api.ServiceAddress, handler func(message.Stream)) bool
}

// Publisher publishes ServiceTypeBootstrap registrations on every
// ACTIVE / COMPLETE transition of the bound machine. Idempotent:
// re-registering an already-registered ServiceAddress is a no-op.
type Publisher struct {
	reg       Registrar
	partition string
}

// New constructs a Publisher bound to reg. partition is the
// partition name carried in the Argument's first segment so peers
// can target the right partition's bootstrap advertisement (e.g.
// "Apollo:active" — partition Apollo is ACTIVE).
func New(reg Registrar, partition string) (*Publisher, error) {
	if reg == nil {
		return nil, fmt.Errorf("advert.New: registrar required")
	}
	if partition == "" {
		return nil, fmt.Errorf("advert.New: partition required")
	}
	return &Publisher{reg: reg, partition: partition}, nil
}

// Wire attaches the publisher to a nodestate.Machine. Every
// transition is observed via Machine.OnChange; on ACTIVE / COMPLETE
// the matching service is registered.
//
// Already-active transitions at Wire time also fire a registration,
// so callers can Wire an already-promoted machine and still publish.
func (p *Publisher) Wire(machine *nodestate.Machine) {
	machine.OnChange(p.onChange)
	if ad := machine.Get(); ad.State == nodestate.StateActive || ad.State == nodestate.StateComplete {
		p.publish(ad.State)
	}
}

func (p *Publisher) onChange(ad nodestate.Advertisement) {
	switch ad.State {
	case nodestate.StateActive, nodestate.StateComplete:
		p.publish(ad.State)
	}
}

// publish registers the marker service for the given state. Idempotent.
func (p *Publisher) publish(state nodestate.State) {
	sa := ServiceAddress(p.partition, state)
	if sa == nil {
		return
	}
	p.reg.RegisterService(sa, noopHandler)
}

// ServiceAddress returns the canonical ServiceAddress for advertising
// `partition`'s bootstrap-v3 state. Returns nil for non-active /
// non-complete states (BOOTING is the absence of advertisement).
func ServiceAddress(partition string, state nodestate.State) *api.ServiceAddress {
	var arg string
	switch state {
	case nodestate.StateActive:
		arg = strings.ToLower(partition) + ":active"
	case nodestate.StateComplete:
		arg = strings.ToLower(partition) + ":complete"
	default:
		return nil
	}
	return &api.ServiceAddress{
		Type:     api.ServiceTypeBootstrap,
		Argument: arg,
	}
}

// noopHandler is the placeholder handler for the marker service.
// The bootstrap advertisement carries no RPC payload — the service
// exists in libp2p's discovery so consumers can find peers in the
// right state. Any incoming stream is closed immediately.
func noopHandler(s message.Stream) {
	// message.Stream is closed by the caller (RegisterService's
	// stream handler) via defer s.Close(). Nothing to do here.
	_ = s
}
