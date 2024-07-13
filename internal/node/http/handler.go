// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package http

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cometbft/cometbft/libs/log"
	"github.com/julienschmidt/httprouter"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	ethimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/ethereum"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	v2 "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/web"
	ethrpc "gitlab.com/accumulatenetwork/accumulate/pkg/api/ethereum"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p/dial"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/rest"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/websocket"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// Handler processes API requests.
type Handler struct {
	logger logging.OptionalLogger
	mux    *httprouter.Router
}

// Options are the options for a [Handler].
type Options struct {
	Logger    log.Logger
	Node      *p2p.Node
	Router    routing.Router
	NetworkId string

	// PeerMap hard-codes the peers used for partitions
	PeerMap map[string][]peer.AddrInfo

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
	var netDial message.Dialer
	if opts.PeerMap != nil {
		netDial = &DumbDialer{
			Peers:     opts.PeerMap,
			Connector: opts.Node.Connector(),
			Self:      opts.Node.ID(),
		}
	} else {
		netDial = opts.Node.DialNetwork()
	}
	client := &message.Client{Transport: &message.RoutedTransport{
		Network: network,
		Router:  routing.MessageRouter{Router: opts.Router},
		Dialer:  netDial,
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
		jsonrpc.Sequencer{Sequencer: client.Private()},
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

	// JSON-RPC API v2
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

	// Ethereum JSON-RPC
	eth := ethrpc.NewHandler(&ethimpl.Service{
		Network: selfClient,
	})

	// REST API
	h.mux, err = rest.NewHandler(
		v2,
		rest.NodeService{NodeService: selfClient},
		rest.ConsensusService{ConsensusService: selfClient},
		rest.NetworkService{NetworkService: client},
		rest.MetricsService{MetricsService: client},
		rest.Querier{Querier: &api.Collator{Querier: client, Network: client}},
		rest.Submitter{Submitter: client},
		rest.Validator{Validator: client},
		rest.Faucet{Faucet: client},
	)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("register API v2: %v", err)
	}

	// Setup mux
	v3h := ws.FallbackTo(v3)
	h.mux.POST("/v3", func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		v3h.ServeHTTP(w, r)
	})

	h.mux.POST("/eth", func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		eth.ServeHTTP(w, r)
	})

	h.mux.GET("/", func(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
		w.Header().Set("Location", "/x")
		w.WriteHeader(http.StatusTemporaryRedirect)
	})

	webex := web.Handler()
	h.mux.GET("/x/", func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
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

// DumbDialer always dials one of the seed nodes
type DumbDialer struct {
	Peers     map[string][]peer.AddrInfo
	Connector dial.Connector
	Self      peer.ID

	count atomic.Int32
}

func (d *DumbDialer) Dial(ctx context.Context, addr multiaddr.Multiaddr) (message.Stream, error) {
	_, id, sa, ip, err := api.UnpackAddress(addr)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if sa == nil {
		return nil, errors.BadRequest.WithFormat("invalid address %v (missing service)", addr)
	}

	if ip != nil && id == "" {
		return nil, errors.BadRequest.WithFormat("cannot specify address without peer ID")
	}

	if id != "" {
		return d.Connector.Connect(ctx, &dial.ConnectionRequest{
			Service:  sa,
			PeerID:   id,
			PeerAddr: ip,
		})
	}

	if sa.Argument == "" {
		return d.Connector.Connect(ctx, &dial.ConnectionRequest{
			Service: sa,
			PeerID:  d.Self,
		})
	}

	// Pick a peer from the map
	peers, ok := d.Peers[strings.ToLower(sa.Argument)]
	if !ok {
		return nil, errors.BadRequest.WithFormat("invalid address %v (unknown partition %q)", addr, sa.Argument)
	}
	peer := peers[d.count.Add(1)%int32(len(peers))]
	if len(peer.Addrs) > 0 {
		ip = peer.Addrs[0]
	}
	return d.Connector.Connect(ctx, &dial.ConnectionRequest{
		Service:  sa,
		PeerID:   peer.ID,
		PeerAddr: ip,
	})
}
