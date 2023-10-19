// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package http

import (
	"net/http"
	"strings"
	"time"

	"github.com/cometbft/cometbft/libs/log"
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	v2 "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/web"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/websocket"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// Handler processes API requests.
type Handler struct {
	logger logging.OptionalLogger
	mux    *http.ServeMux
}

// Options are the options for a [Handler].
type Options struct {
	Logger    log.Logger
	Node      *p2p.Node
	Router    routing.Router
	NetworkId string

	// For API v2
	Network *config.Describe
	MaxWait time.Duration
}

// NewHandler returns a new Handler.
func NewHandler(opts Options) (*Handler, error) {
	h := new(Handler)
	h.logger.Set(opts.Logger)

	network := opts.NetworkId
	if network == "" {
		if opts.Network == nil {
			return nil, errors.BadRequest.WithFormat("Network or NetworkId must be specified")
		}
		network = opts.Network.Network.Id
	}

	// Message clients
	client := &message.Client{Transport: &message.RoutedTransport{
		Network: network,
		Router:  routing.MessageRouter{Router: opts.Router},
		Dialer:  opts.Node.DialNetwork(),
	}}

	var selfClient *message.Client
	if opts.Network == nil {
		selfClient = client
	} else {
		selfClient = &message.Client{Transport: &message.RoutedTransport{
			Network: opts.Network.Network.Id,
			Router:  unrouter(opts.Network.PartitionId),
			Dialer:  opts.Node.DialSelf(),
		}}
	}

	// JSON-RPC API v3
	v3, err := jsonrpc.NewHandler(
		jsonrpc.NodeService{NodeService: selfClient},
		jsonrpc.ConsensusService{ConsensusService: selfClient},
		jsonrpc.NetworkService{NetworkService: client},
		jsonrpc.MetricsService{MetricsService: client},
		jsonrpc.Querier{Querier: &api.Collator{Querier: client, Network: client}},
		jsonrpc.Submitter{Submitter: client},
		jsonrpc.Validator{Validator: client},
		jsonrpc.Faucet{Faucet: client},
	)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("initialize API v3: %w", err)
	}

	// WebSocket API v3
	ws, err := websocket.NewHandler(
		message.NodeService{NodeService: selfClient},
		message.ConsensusService{ConsensusService: selfClient},
		message.NetworkService{NetworkService: client},
		message.MetricsService{MetricsService: client},
		message.Querier{Querier: &api.Collator{Querier: client, Network: client}},
		message.Submitter{Submitter: client},
		message.Validator{Validator: client},
		message.Faucet{Faucet: client},
		message.EventService{EventService: client},
	)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("initialize websocket API: %w", err)
	}

	v2, err := v2.NewJrpc(v2.Options{
		Logger:        opts.Logger,
		Describe:      opts.Network,
		TxMaxWaitTime: opts.MaxWait,
		LocalV3:       selfClient,
		Querier:       &api.Collator{Querier: client, Network: client},
		Submitter:     client,
		Faucet:        client,
		Validator:     client,
		Sequencer:     client.Private(),
	})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("initialize API v2: %v", err)
	}

	// Set up mux
	h.mux = v2.NewMux()
	h.mux.Handle("/v3", ws.FallbackTo(v3))

	h.mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/x")
		w.WriteHeader(http.StatusTemporaryRedirect)
	})

	webex := web.Handler()
	h.mux.HandleFunc("/x/", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/x")
		r.RequestURI = strings.TrimPrefix(r.RequestURI, "/x")
		webex.ServeHTTP(w, r)
	})

	return h, nil
}

// ServeHTTP implements [http.Handler].
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

// unrouter routes everything to the same partition
type unrouter string

func (r unrouter) Route(msg message.Message) (multiaddr.Multiaddr, error) {
	service := new(api.ServiceAddress)
	service.Argument = string(r)
	switch msg := msg.(type) {
	case *message.NodeInfoRequest:
		// If no peer is specified, don't attach anything else to the address so
		// it's sent to us
		if msg.PeerID == "" {
			return api.ServiceTypeNode.Address().Multiaddr(), nil
		}

		// Send the request to /p2p/{id}/node
		c1, err := multiaddr.NewComponent("p2p", msg.PeerID.String())
		if err != nil {
			return nil, errors.BadRequest.WithFormat("build multiaddr: %w", err)
		}
		c2 := api.ServiceTypeNode.Address().Multiaddr()

		return c1.Encapsulate(c2), nil

	case *message.FindServiceRequest:
		// Route the request to /acc-svc/node (with no argument)
		return api.ServiceTypeNode.Address().Multiaddr(), nil

	case *message.ConsensusStatusRequest:
		// If no node ID is specified, route normally
		if msg.NodeID == "" {
			service.Type = api.ServiceTypeConsensus

			// Respect the partition if it is specified
			if msg.Partition != "" {
				service.Argument = msg.Partition
			}
			break
		}

		// If a node ID is specified, a partition ID must also be specified
		if msg.Partition == "" {
			return nil, errors.BadRequest.WithFormat("missing partition")
		}

		// Send the request to /p2p/{id}/acc-svc/consensus:{partition}
		c1, err := multiaddr.NewComponent("p2p", msg.NodeID)
		if err != nil {
			return nil, errors.BadRequest.WithFormat("build multiaddr: %w", err)
		}
		c2 := api.ServiceTypeConsensus.AddressFor(msg.Partition).Multiaddr()

		return c1.Encapsulate(c2), nil

	case *message.NetworkStatusRequest:
		service.Type = api.ServiceTypeNetwork

		// Respect the partition if it is specified
		if msg.Partition != "" {
			service.Argument = msg.Partition
		}

	case *message.MetricsRequest:
		service.Type = api.ServiceTypeMetrics

		// Respect the partition if it is specified
		if msg.Partition != "" {
			service.Argument = msg.Partition
		}

	case *message.QueryRequest:
		service.Type = api.ServiceTypeQuery

	case *message.SubmitRequest:
		service.Type = api.ServiceTypeSubmit

	case *message.ValidateRequest:
		service.Type = api.ServiceTypeValidate

	case *message.SubscribeRequest:
		service.Type = api.ServiceTypeEvent

		// Respect the partition if it is specified
		if msg.Partition != "" {
			service.Argument = msg.Partition
		}

	case *message.FaucetRequest:
		service.Type = api.ServiceTypeFaucet

	default:
		return nil, errors.BadRequest.WithFormat("%v is not routable", msg.Type())
	}

	// Route the request to /acc-svc/{service}:{partition}
	return service.Multiaddr(), nil
}
