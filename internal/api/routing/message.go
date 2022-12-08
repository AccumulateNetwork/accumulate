package routing

import (
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// MessageRouter routes messages using a Router.
type MessageRouter struct {
	Router
}

// Route routes a message.
func (r MessageRouter) Route(msg message.Message) (multiaddr.Multiaddr, error) {
	// If the message already has an address, use that
	addr := message.AddressOf(msg)
	if addr != nil {
		return addr, nil
	}

	var partition string
	var err error
	switch msg := msg.(type) {
	case *message.NetworkStatusRequest:
		// Route to the requested partition
		if msg.Partition == "" {
			return nil, errors.BadRequest.WithFormat("partition is missing")
		}
		partition = msg.Partition

	case *message.MetricsRequest:
		// Route to the requested partition
		if msg.Partition == "" {
			return nil, errors.BadRequest.WithFormat("partition is missing")
		}
		partition = msg.Partition

	case *message.NodeStatusRequest:
		// Route to the requested node and partition
		if msg.NodeID == "" {
			return nil, errors.BadRequest.WithFormat("node ID is missing")
		}
		if msg.Partition == "" {
			return nil, errors.BadRequest.WithFormat("partition is missing")
		}

		// Return /p2p/{id}/acc/{partition}
		c1, err := multiaddr.NewComponent("p2p", msg.NodeID)
		if err != nil {
			return nil, errors.BadRequest.WithFormat("build multiaddr: %w", err)
		}
		c2, err := multiaddr.NewComponent("acc", msg.Partition)
		if err != nil {
			return nil, errors.BadRequest.WithFormat("build multiaddr: %w", err)
		}

		return c1.Encapsulate(c2), nil

	case *message.QueryRequest:
		// Route based on the scope
		if msg.Scope == nil {
			return nil, errors.BadRequest.WithFormat("scope is missing")
		}
		partition, err = r.Router.RouteAccount(msg.Scope)

	case *message.SubmitRequest:
		// Route the envelope
		if msg.Envelope == nil {
			return nil, errors.BadRequest.WithFormat("envelope is missing")
		}
		partition, err = RouteEnvelopes(r.Router.RouteAccount, msg.Envelope)

	case *message.ValidateRequest:
		// Route the envelope
		if msg.Envelope == nil {
			return nil, errors.BadRequest.WithFormat("envelope is missing")
		}
		partition, err = RouteEnvelopes(r.Router.RouteAccount, msg.Envelope)

	case *message.SubscribeRequest:
		// Route to the requested partition
		if msg.Partition == "" {
			return nil, errors.BadRequest.WithFormat("partition is missing")
		}
		partition = msg.Partition

	default:
		return nil, errors.BadRequest.WithFormat("%v is not routable", msg.Type())
	}
	if err != nil {
		return nil, errors.BadRequest.WithFormat("cannot route request: %w", err)
	}

	// Return /acc/{partition}
	ma, err := multiaddr.NewComponent("acc", partition)
	if err != nil {
		return nil, errors.BadRequest.WithFormat("build multiaddr: %w", err)
	}
	return ma, nil
}
