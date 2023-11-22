// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/rs/cors"
	"gitlab.com/accumulatenetwork/accumulate/exp/apiutil"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	nodehttp "gitlab.com/accumulatenetwork/accumulate/internal/node/http"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/exp/slog"
)

var (
	httpRefRouter = ioc.Needs[routing.Router](func(h *HttpService) string { return h.Router.base().refOr("") })
)

func (h *HttpService) Requires() []ioc.Requirement {
	return h.Router.RequiresOr(
		httpRefRouter.Requirement(h),
	)
}

func (h *HttpService) Provides() []ioc.Provided { return nil }

func (h *HttpService) start(inst *Instance) error {
	setDefaultS(&h.Listen, []multiaddr.Multiaddr{mustParseMulti("/ip4/0.0.0.0/tcp/8080/http")})
	setDefault(&h.ReadHeaderTimeout, 10*time.Second)
	setDefault(&h.ConnectionLimit, 500)

	var router routing.Router
	var err error
	if h.Router.base().hasValue() {
		router, err = h.Router.value.create(inst)
	} else {
		router, err = httpRefRouter.Get(inst.services, h)
	}
	if err != nil {
		return err
	}

	apiOpts := nodehttp.Options{
		Logger:    (*logging.Slogger)(inst.logger).With("module", "http"),
		Node:      inst.p2p,
		Router:    router,
		MaxWait:   10 * time.Second,
		NetworkId: inst.network,
	}
	client := &message.Client{Transport: &message.RoutedTransport{
		Network: inst.network,
		Router:  routing.MessageRouter{Router: router},
		Dialer:  inst.p2p.DialNetwork(),
	}}
	timestamps := &apiutil.TimestampService{
		Querier: &api.Collator{Querier: client, Network: client},
		Cache:   memory.New(nil),
	}

	if len(h.PeerMap) > 0 {
		m := map[string][]peer.AddrInfo{}
		apiOpts.PeerMap = m
		for _, p := range h.PeerMap {
			for _, part := range p.Partitions {
				part = strings.ToLower(part)
				m[part] = append(m[part], peer.AddrInfo{
					ID:    p.ID,
					Addrs: p.Addresses,
				})
			}
		}
	}

	api, err := nodehttp.NewHandler(apiOpts)
	if err != nil {
		return err
	}

	api2 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const prefix = "/timestamp/"
		if r.Method != "GET" || !strings.HasPrefix(r.URL.Path, prefix) {
			api.ServeHTTP(w, r)
			return
		}

		var res any
		id, err := url.ParseTxID(r.URL.Path[len(prefix):])
		if err == nil {
			res, err = timestamps.GetTimestamp(r.Context(), id)
		}

		if err == nil {
			err := json.NewEncoder(w).Encode(res)
			if err != nil {
				slog.ErrorCtx(r.Context(), "Failed to encode response", "error", err)
			}
			return
		}

		err2 := errors.UnknownError.Wrap(err).(*errors.Error)
		w.WriteHeader(int(err2.Code))

		err = json.NewEncoder(w).Encode(err2)
		if err != nil {
			slog.ErrorCtx(r.Context(), "Failed to encode response", "error", err)
		}
	})

	c := cors.New(cors.Options{
		AllowedOrigins: h.CorsOrigins,
	})
	server := &http.Server{
		Handler:           c.Handler(api2),
		ReadHeaderTimeout: *h.ReadHeaderTimeout,
	}

	for _, l := range h.Listen {
		var proto, addr, port string
		var secure bool
		var err error
		multiaddr.ForEach(l, func(c multiaddr.Component) bool {
			switch c.Protocol().Code {
			case multiaddr.P_IP4,
				multiaddr.P_IP6,
				multiaddr.P_DNS,
				multiaddr.P_DNS4,
				multiaddr.P_DNS6:
				addr = c.Value()
			case multiaddr.P_TCP,
				multiaddr.P_UDP:
				proto = c.Protocol().Name
				port = c.Value()
			case multiaddr.P_HTTP:
				// Ok
			case multiaddr.P_HTTPS:
				secure = true
			default:
				err = errors.UnknownError.WithFormat("invalid listen address: %v", l)
				return false
			}
			return true
		})
		if err != nil {
			return err
		}
		if proto == "" || port == "" {
			return errors.UnknownError.WithFormat("invalid listen address: %v", l)
		}
		addr += ":" + port

		l, err := net.Listen(proto, addr)
		if err != nil {
			return err
		}
		h.serve(inst, server, l, secure)
	}

	if len(h.LetsEncrypt) > 0 {
		h.serve(inst, server, autocert.NewListener(h.LetsEncrypt...), false)
	}

	inst.cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := server.Shutdown(ctx)
		slog.Error("Server stopped", "error", err)
	})
	return nil
}

func (h *HttpService) serve(inst *Instance, server *http.Server, l net.Listener, secure bool) error {
	if secure && (h.TlsCertPath == "" || h.TlsKeyPath == "") {
		return errors.UnknownError.WithFormat("--tls-cert and --tls-key are required to listen on %v", l)
	}

	scheme := "http"
	if secure {
		scheme = "https"
	}
	slog.InfoCtx(inst.context, "Listening", "module", "http", "address", l.Addr(), "scheme", scheme)

	if *h.ConnectionLimit > 0 {
		pool := make(chan struct{}, *h.ConnectionLimit)
		for {
			select {
			case pool <- struct{}{}:
				continue
			default:
			}
			break
		}
		l = &accumulated.RateLimitedListener{Listener: l, Pool: pool}
	}

	inst.run(func() {
		var err error
		if secure {
			err = server.ServeTLS(l, h.TlsCertPath, h.TlsKeyPath)
		} else {
			err = server.Serve(l)
		}
		slog.Error("Server stopped", "error", err, "address", l.Addr())
	})
	return nil
}
