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

	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/p2p"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/web"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/websocket"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	testhttp "gitlab.com/accumulatenetwork/accumulate/test/util/http"
)

// Services returns the simulator's API v3 implementation.
func (s *Simulator) Services() *simService { return (*simService)(s) }

// ClientV2 returns an API V2 client for the given partition.
func (s *Simulator) ClientV2(part string) *client.Client {
	return s.partitions[part].nodes[0].clientV2
}

// NewDirectClientWithHook creates a direct HTTP client and applies the given
// hook.
func (s *Simulator) NewDirectClientWithHook(hook func(http.Handler) http.Handler) *client.Client {
	c, err := client.New("http://direct-jrpc-client")
	if err != nil {
		panic(err)
	}

	h := s.partitions[protocol.Directory].nodes[0].apiV2.NewMux()
	c.Client.Client = *testhttp.DirectHttpClient(hook(h))
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
	if s.init.Bvns[0].Nodes[0].Listen().String() == "" {
		return errors.BadRequest.With("no address to listen on")
	}

	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		if err != nil {
			cancel()
		}
	}()

	bootstrap := []multiaddr.Multiaddr{
		s.partitions[protocol.Directory].nodes[0].init.Listen().Scheme("tcp").Directory().AccumulateP2P().WithKey().Multiaddr(),
	}

	for _, part := range s.partitions {
		for _, node := range part.nodes {
			err := node.listenAndServeHTTP(ctx, opts, s.Services())
			if err != nil {
				return err
			}

			err = node.listenP2P(ctx, opts, bootstrap)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (n *Node) listenAndServeHTTP(ctx context.Context, opts ListenOptions, services *simService) error {
	if !opts.ListenHTTPv2 &&
		!opts.ListenHTTPv3 &&
		!opts.ListenWSv3 {
		return nil
	}

	var mux *http.ServeMux
	if opts.ListenHTTPv2 {
		mux = n.apiV2.NewMux()
	} else {
		mux = new(http.ServeMux)
	}

	var v3 http.Handler
	if opts.ListenHTTPv3 {
		jrpc, err := jsonrpc.NewHandler(
			n.logger.With("module", "api"),
			jsonrpc.ConsensusService{ConsensusService: (*nodeService)(n)},
			jsonrpc.NetworkService{NetworkService: services},
			jsonrpc.MetricsService{MetricsService: services},
			jsonrpc.Querier{Querier: services},
			jsonrpc.Submitter{Submitter: services},
			jsonrpc.Validator{Validator: services},
		)
		if err != nil {
			return errors.UnknownError.WithFormat("initialize API v3: %w", err)
		}
		v3 = jrpc
	}

	if opts.ListenWSv3 {
		ws, err := websocket.NewHandler(
			n.logger.With("module", "api"),
			message.ConsensusService{ConsensusService: (*nodeService)(n)},
			message.NetworkService{NetworkService: services},
			message.MetricsService{MetricsService: services},
			message.Querier{Querier: services},
			message.Submitter{Submitter: services},
			message.Validator{Validator: services},
			message.EventService{EventService: services},
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
	var addr string
	if n.partition.Type == config.Directory {
		addr = n.init.Listen().Directory().AccumulateAPI().String()
	} else {
		addr = n.init.Listen().BlockValidator().AccumulateAPI().String()
	}

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

func (n *Node) listenP2P(ctx context.Context, opts ListenOptions, bootstrap []multiaddr.Multiaddr) error {
	if !opts.ListenP2Pv3 {
		return nil
	}

	var addr1, addr2 multiaddr.Multiaddr
	if n.partition.Type == config.Directory {
		addr1 = n.init.Listen().Scheme("tcp").Directory().AccumulateP2P().Multiaddr()
		addr2 = n.init.Listen().Scheme("udp").Directory().AccumulateP2P().Multiaddr()
	} else {
		addr1 = n.init.Listen().Scheme("tcp").BlockValidator().AccumulateP2P().Multiaddr()
		addr2 = n.init.Listen().Scheme("udp").BlockValidator().AccumulateP2P().Multiaddr()
	}

	h, err := message.NewHandler(
		n.logger.With("module", "acc"),
		&message.ConsensusService{ConsensusService: (*nodeService)(n)},
		&message.MetricsService{MetricsService: (*nodeService)(n)},
		&message.NetworkService{NetworkService: (*nodeService)(n)},
		&message.Querier{Querier: (*nodeService)(n)},
		&message.Submitter{Submitter: (*nodeService)(n)},
		&message.Validator{Validator: (*nodeService)(n)},
		&message.EventService{EventService: (*nodeService)(n)},
		&message.Sequencer{Sequencer: n.seqSvc},
	)
	if err != nil {
		return err
	}

	p2p, err := p2p.New(p2p.Options{
		Logger:         n.logger.With("module", "acc"),
		Network:        n.simulator.init.Id,
		Listen:         []multiaddr.Multiaddr{addr1, addr2},
		BootstrapPeers: bootstrap,
		Key:            n.nodeKey,
	})
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		_ = p2p.Close()
	}()

	p2p.RegisterService(api.ServiceTypeConsensus.AddressFor(n.partition.ID), h.Handle)
	p2p.RegisterService(api.ServiceTypeMetrics.AddressFor(n.partition.ID), h.Handle)
	p2p.RegisterService(api.ServiceTypeNetwork.AddressFor(n.partition.ID), h.Handle)
	p2p.RegisterService(api.ServiceTypeQuery.AddressFor(n.partition.ID), h.Handle)
	p2p.RegisterService(api.ServiceTypeSubmit.AddressFor(n.partition.ID), h.Handle)
	p2p.RegisterService(api.ServiceTypeValidate.AddressFor(n.partition.ID), h.Handle)
	p2p.RegisterService(api.ServiceTypeEvent.AddressFor(n.partition.ID), h.Handle)
	p2p.RegisterService(private.ServiceTypeSequencer.AddressFor(n.partition.ID), h.Handle)

	n.logger.Info("Node P2P up", "addresses", p2p.Addrs())
	return nil
}
