// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	apiv2 "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/web"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/websocket"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/testing"
	testhttp "gitlab.com/accumulatenetwork/accumulate/test/util/http"
)

// Services returns the simulator's API v3 implementation.
func (s *Simulator) Services() *message.Client { return s.services.Client }

func (n *Node) newApiV2() (*apiv2.JrpcMethods, error) {
	svc := n.partition.sim.services
	return apiv2.NewJrpc(apiv2.Options{
		Logger:        n.logger,
		TxMaxWaitTime: time.Hour,
		LocalV3:       svc.ForPeer(n.peerID),
		Querier:       svc,
		Submitter:     svc,
		Faucet:        svc,
		Validator:     svc,
		Sequencer:     svc.Private(),
		Describe: &config.Describe{
			NetworkType: n.partition.Type,
			PartitionId: n.partition.ID,
		},
	})
}

// ClientV2 returns an API V2 client for the given partition.
func (s *Simulator) ClientV2(part string) *client.Client {
	api, err := s.partitions[part].nodes[0].newApiV2()
	if err != nil {
		panic(err)
	}
	return testing.DirectJrpcClient(api)
}

// NewDirectClientWithHook creates a direct HTTP client and applies the given
// hook.
func (s *Simulator) NewDirectClientWithHook(hook func(http.Handler) http.Handler) *client.Client {
	c, err := client.New("http://direct-jrpc-client")
	if err != nil {
		panic(err)
	}

	api, err := s.partitions[protocol.Directory].nodes[0].newApiV2()
	if err != nil {
		panic(err)
	}

	c.Client.Client = *testhttp.DirectHttpClient(hook(api.NewMux()))
	return c
}

// ListenOptions are options for [Simulator.ListenAndServe].
type ListenOptions struct {
	// ListenHTTPv2 enables API v2 over HTTP.
	ListenHTTPv2 bool

	// ListenHTTPv3 enables API v3 over HTTP.
	ListenHTTPv3 bool

	// ListenWSv3 enables API v3 over WebSockets.
	ListenWSv3 bool

	// ListenP2Pv3 enables API v3 over libp2p.
	ListenP2Pv3 bool

	// HookHTTP hooks into HTTP calls.
	HookHTTP func(http.Handler, http.ResponseWriter, *http.Request)

	// ServeError handles errors produced by http.Server.Serve. If Error is nil,
	// ListenAndServe will panic if http.Server.Serve returns an error.
	ServeError func(error)
}

// ListenAndServe serves the Accumulate API. The simulator must have been
// initialized with a network configuration that includes addresses. At least
// one of the Listen options of opts must be true.
func (s *Simulator) ListenAndServe(ctx context.Context, opts ListenOptions) (err error) {
	if !opts.ListenHTTPv2 &&
		!opts.ListenHTTPv3 &&
		!opts.ListenWSv3 &&
		!opts.ListenP2Pv3 {
		return errors.BadRequest.With("nothing to do")
	}

	// The simulator network configuration must specify a listening address
	if s.partitions[protocol.Directory].nodes[0].network.Listen().String() == "" {
		return errors.BadRequest.With("no address to listen on")
	}

	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		if err != nil {
			cancel()
		}
	}()

	var nodes []*p2p.Node
	for _, part := range s.partitions {
		for _, node := range part.nodes {
			err := node.listenAndServeHTTP(ctx, opts)
			if err != nil {
				return err
			}

			err = node.listenP2P(ctx, opts, &nodes)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (n *Node) listenAndServeHTTP(ctx context.Context, opts ListenOptions) error {
	if !opts.ListenHTTPv2 &&
		!opts.ListenHTTPv3 &&
		!opts.ListenWSv3 {
		return nil
	}

	var mux *http.ServeMux
	if opts.ListenHTTPv2 {
		api, err := n.newApiV2()
		if err != nil {
			return err
		}
		mux = api.NewMux()
	} else {
		mux = new(http.ServeMux)
	}

	var v3 http.Handler
	network := n.partition.sim.services
	if opts.ListenHTTPv3 {
		jrpc, err := jsonrpc.NewHandler(
			jsonrpc.ConsensusService{ConsensusService: n},
			jsonrpc.NetworkService{NetworkService: network},
			jsonrpc.MetricsService{MetricsService: network},
			jsonrpc.Querier{Querier: network},
			jsonrpc.Submitter{Submitter: network},
			jsonrpc.Validator{Validator: network},
		)
		if err != nil {
			return errors.UnknownError.WithFormat("initialize API v3: %w", err)
		}
		v3 = jrpc
	}

	if opts.ListenWSv3 {
		ws, err := websocket.NewHandler(
			message.ConsensusService{ConsensusService: n},
			message.NetworkService{NetworkService: network},
			message.MetricsService{MetricsService: network},
			message.Querier{Querier: network},
			message.Submitter{Submitter: network},
			message.Validator{Validator: network},
			message.EventService{EventService: network},
		)
		if err != nil {
			return errors.UnknownError.WithFormat("initialize websocket API: %w", err)
		}
		if v3 == nil {
			v3 = ws
		} else {
			v3 = ws.FallbackTo(v3)
		}
	}

	if v3 != nil {
		mux.Handle("/v3", v3)
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/x")
		w.WriteHeader(http.StatusTemporaryRedirect)
	})

	webex := web.Handler()
	mux.HandleFunc("/x/", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/x")
		r.RequestURI = strings.TrimPrefix(r.RequestURI, "/x")
		webex.ServeHTTP(w, r)
	})

	var h http.Handler = mux
	if opts.HookHTTP != nil {
		h = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			opts.HookHTTP(mux, w, r)
		})
	}

	// Determine the listening address
	addr := n.network.Listen().PartitionType(n.partition.Type).AccumulateAPI().String()

	// Start the listener
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	go func() { <-ctx.Done(); _ = ln.Close() }()

	// Start the server
	srv := http.Server{Handler: h}
	go func() {
		err := srv.Serve(ln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			if opts.ServeError == nil {
				panic(err)
			} else {
				opts.ServeError(err)
			}
		}
	}()
	go func() { <-ctx.Done(); _ = srv.Shutdown(context.Background()) }()

	n.logger.Info("Node HTTP up", "address", "http://"+addr)
	return nil
}

func (n *Node) listenP2P(ctx context.Context, opts ListenOptions, nodes *[]*p2p.Node) error {
	if !opts.ListenP2Pv3 {
		return nil
	}

	addr1 := n.network.Listen().Scheme("tcp").PartitionType(n.partition.Type).AccumulateP2P().Multiaddr()
	addr2 := n.network.Listen().Scheme("udp").PartitionType(n.partition.Type).AccumulateP2P().Multiaddr()

	p2p, err := p2p.New(p2p.Options{
		Network:       "Simulator",
		Listen:        []multiaddr.Multiaddr{addr1, addr2},
		Key:           n.nodeKey,
		DiscoveryMode: dht.ModeServer,
	})
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		_ = p2p.Close()
	}()

	for _, n := range *nodes {
		err = p2p.ConnectDirectly(n)
		if err != nil {
			return err
		}
	}
	*nodes = append(*nodes, p2p)

	p2p.RegisterService(api.ServiceTypeConsensus.AddressFor(n.partition.ID), n.services.Handle)
	p2p.RegisterService(api.ServiceTypeMetrics.AddressFor(n.partition.ID), n.services.Handle)
	p2p.RegisterService(api.ServiceTypeNetwork.AddressFor(n.partition.ID), n.services.Handle)
	p2p.RegisterService(api.ServiceTypeQuery.AddressFor(n.partition.ID), n.services.Handle)
	p2p.RegisterService(api.ServiceTypeSubmit.AddressFor(n.partition.ID), n.services.Handle)
	p2p.RegisterService(api.ServiceTypeValidate.AddressFor(n.partition.ID), n.services.Handle)
	p2p.RegisterService(api.ServiceTypeEvent.AddressFor(n.partition.ID), n.services.Handle)
	p2p.RegisterService(private.ServiceTypeSequencer.AddressFor(n.partition.ID), n.services.Handle)

	n.logger.Info("Node P2P up", "addresses", p2p.Addresses())
	return nil
}
