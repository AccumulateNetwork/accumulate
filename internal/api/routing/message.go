package routing

import (
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
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

	service := new(api.ServiceAddress)
	var err error
	switch msg := msg.(type) {
	case *message.NetworkStatusRequest:
		service.Type = api.ServiceTypeNetwork

		// Route to the requested partition
		if msg.Partition == "" {
			return nil, errors.BadRequest.WithFormat("partition is missing")
		}
		service.Partition = msg.Partition

	case *message.NodeStatusRequest:
		service.Type = api.ServiceTypeNode
		service.Partition = msg.Partition

		// Route to the requested node and partition
		if msg.NodeID == "" {
			return nil, errors.BadRequest.WithFormat("node ID is missing")
		}
		if msg.Partition == "" {
			return nil, errors.BadRequest.WithFormat("partition is missing")
		}

		// Return /p2p/{id}/acc/{service}:{partition}
		c1, err := multiaddr.NewComponent("p2p", msg.NodeID)
		if err != nil {
			return nil, errors.BadRequest.WithFormat("build multiaddr: %w", err)
		}
		c2, err := multiaddr.NewComponent(api.N_ACC, service.String())
		if err != nil {
			return nil, errors.BadRequest.WithFormat("build multiaddr: %w", err)
		}

		return c1.Encapsulate(c2), nil

	case *message.MetricsRequest:
		service.Type = api.ServiceTypeMetrics

		// Route to the requested partition
		if msg.Partition == "" {
			return nil, errors.BadRequest.WithFormat("partition is missing")
		}
		service.Partition = msg.Partition

	case *message.QueryRequest:
		service.Type = api.ServiceTypeQuery

		// Route based on the scope
		if msg.Scope == nil {
			return nil, errors.BadRequest.WithFormat("scope is missing")
		}
		service.Partition, err = r.Router.RouteAccount(msg.Scope)

	case *message.SubmitRequest:
		service.Type = api.ServiceTypeSubmit

		// Route the envelope
		if msg.Envelope == nil {
			return nil, errors.BadRequest.WithFormat("envelope is missing")
		}
		service.Partition, err = RouteEnvelopes(r.Router.RouteAccount, msg.Envelope)

	case *message.ValidateRequest:
		service.Type = api.ServiceTypeValidate

		// Route the envelope
		if msg.Envelope == nil {
			return nil, errors.BadRequest.WithFormat("envelope is missing")
		}
		service.Partition, err = RouteEnvelopes(r.Router.RouteAccount, msg.Envelope)

	case *message.SubscribeRequest:
		service.Type = api.ServiceTypeEvent

		// Route to the requested partition
		if msg.Partition == "" && msg.Account == nil {
			return nil, errors.BadRequest.WithFormat("partition or account is required")
		}
		if msg.Partition != "" {
			service.Partition = msg.Partition
		} else {
			service.Partition, err = r.RouteAccount(msg.Account)
		}

	case *message.FaucetRequest:
		service.Type = api.ServiceTypeFaucet

	default:
		return nil, errors.BadRequest.WithFormat("%v is not routable", msg.Type())
	}
	if err != nil {
		return nil, errors.BadRequest.WithFormat("cannot route request: %w", err)
	}

	_ = "/acc/query:apollo"
	_ = "/acc/query:apollo/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N"

	// Return /acc/{service}:{partition}
	ma, err := multiaddr.NewComponent(api.N_ACC, service.String())
	if err != nil {
		return nil, errors.BadRequest.WithFormat("build multiaddr: %w", err)
	}
	return ma, nil
}
