// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package routing

import (
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// MessageRouter routes messages using a Router.
type MessageRouter struct {
	Router Router
}

// Route routes a message.
func (r MessageRouter) Route(msg message.Message) (multiaddr.Multiaddr, error) {
	// If the message already has an address, use that
	addr := message.AddressOf(msg)
	if addr != nil {
		return addr, nil
	}

	service := new(api.ServiceAddress)
	var err error
	switch msg := msg.(type) {
	case *message.NodeInfoRequest:
		// If no peer ID is specified, return /acc-svc/node so the request is
		// sent to this node
		service.Type = api.ServiceTypeNode
		if msg.PeerID == "" {
			return service.Multiaddr(), nil
		}

		// Send the request to /p2p/{id}/acc-svc/{service}:{partition}
		c1, err := multiaddr.NewComponent("p2p", msg.PeerID.String())
		if err != nil {
			return nil, errors.BadRequest.WithFormat("build multiaddr: %w", err)
		}
		c2 := service.Multiaddr()

		return c1.Encapsulate(c2), nil

	case *message.FindServiceRequest:
		service.Type = api.ServiceTypeNode

	case *message.NetworkStatusRequest:
		service.Type = api.ServiceTypeNetwork

		// Route to the requested partition
		if msg.Partition == "" {
			service.Argument = protocol.Directory
		} else {
			service.Argument = msg.Partition
		}

	case *message.ListSnapshotsRequest:
		service.Type = api.ServiceTypeSnapshot

		// Route to the requested node and partition
		if msg.NodeID == "" {
			return nil, errors.BadRequest.With("node ID is missing")
		}
		if msg.Partition == "" {
			service.Argument = protocol.Directory
		} else {
			service.Argument = msg.Partition
		}

		// Return /p2p/{id}/acc/{service}:{partition}
		c1, err := multiaddr.NewComponent("p2p", msg.NodeID)
		if err != nil {
			return nil, errors.BadRequest.WithFormat("build multiaddr: %w", err)
		}
		c2 := service.Multiaddr()

		return c1.Encapsulate(c2), nil

	case *message.ConsensusStatusRequest:
		service.Type = api.ServiceTypeConsensus

		// Route to the requested node and partition
		if msg.NodeID == "" {
			return nil, errors.BadRequest.With("node ID is missing")
		}
		if msg.Partition == "" {
			service.Argument = protocol.Directory
		} else {
			service.Argument = msg.Partition
		}

		// Return /p2p/{id}/acc/{service}:{partition}
		c1, err := multiaddr.NewComponent("p2p", msg.NodeID)
		if err != nil {
			return nil, errors.BadRequest.WithFormat("build multiaddr: %w", err)
		}
		c2 := service.Multiaddr()

		return c1.Encapsulate(c2), nil

	case *message.MetricsRequest:
		service.Type = api.ServiceTypeMetrics

		// Route to the requested partition
		if msg.Partition == "" {
			service.Argument = protocol.Directory
		} else {
			service.Argument = msg.Partition
		}

	case *message.QueryRequest:
		service.Type = api.ServiceTypeQuery

		// Route based on the scope
		if msg.Scope == nil {
			return nil, errors.BadRequest.With("scope is missing")
		}
		if r.Router == nil {
			return nil, errors.NotReady.With("cannot route: router not setup")
		}
		service.Argument, err = r.Router.RouteAccount(msg.Scope)

	case *message.SubmitRequest:
		service.Type = api.ServiceTypeSubmit

		// Route the envelope
		if msg.Envelope == nil {
			return nil, errors.BadRequest.With("envelope is missing")
		}
		if r.Router == nil {
			return nil, errors.NotReady.With("cannot route: router not setup")
		}
		service.Argument, err = r.Router.Route(msg.Envelope)

	case *message.ValidateRequest:
		service.Type = api.ServiceTypeValidate

		// Route the envelope
		if msg.Envelope == nil {
			return nil, errors.BadRequest.With("envelope is missing")
		}
		if r.Router == nil {
			return nil, errors.NotReady.With("cannot route: router not setup")
		}
		service.Argument, err = r.Router.Route(msg.Envelope)

	case *message.SubscribeRequest:
		service.Type = api.ServiceTypeEvent

		// Route to the requested partition
		switch {
		case msg.Partition != "":
			service.Argument = msg.Partition
		case msg.Account != nil:
			if r.Router == nil {
				return nil, errors.NotReady.With("cannot route: router not setup")
			}
			service.Argument, err = r.Router.RouteAccount(msg.Account)
		default:
			service.Argument = protocol.Directory
		}

	case *message.FaucetRequest:
		if msg.Account == nil {
			return nil, errors.BadRequest.With("account is missing")
		}

		token := msg.Token
		if token == nil {
			_, t, _ := protocol.ParseLiteTokenAddress(msg.Account)
			if t != nil {
				token = t
			} else {
				return nil, errors.BadRequest.WithFormat("%v is not a lite token address and the request does not specify a token type", t)
			}
		}

		service = api.ServiceTypeFaucet.AddressForUrl(token)

	case *message.PrivateSequenceRequest:
		service.Type = private.ServiceTypeSequencer

		var ok bool
		service.Argument, ok = protocol.ParsePartitionUrl(msg.Source)
		if !ok {
			return nil, errors.BadRequest.WithFormat("%v is not a partition URL", msg.Source)
		}

		if msg.NodeID == "" {
			return service.Multiaddr(), nil
		}

		// Send the request to /p2p/{id}/acc-svc/{service}:{partition}
		c1, err := multiaddr.NewComponent("p2p", msg.NodeID.String())
		if err != nil {
			return nil, errors.BadRequest.WithFormat("build multiaddr: %w", err)
		}
		c2 := service.Multiaddr()

		return c1.Encapsulate(c2), nil

	default:
		return nil, errors.BadRequest.WithFormat("%v is not routable", msg.Type())
	}
	if err != nil {
		return nil, errors.BadRequest.WithFormat("cannot route request: %w", err)
	}

	// Return /acc-svc/{service}:{partition}
	return service.Multiaddr(), nil
}
