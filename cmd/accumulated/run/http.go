// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

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
)

const (
	DefaultHTTPReadHeaderTimeout = 10 * time.Second
	DefaultHTTPConnectionLimit   = 500
	DefaultHTTPMaxWait           = 10 * time.Second
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
	setDefaultVal(&h.Listen, []multiaddr.Multiaddr{multiaddr.StringCast("/ip4/0.0.0.0/tcp/8080/http")})
	h.applyHttpDefaults()

	if len(h.Listen) == 0 {
		return errors.BadRequest.With("no listen addresses specified")
	}

	var router routing.Router
	var err error
	if h.Router.base().hasValue() {
		h.Router.value.PeerMap = h.PeerMap
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
		MaxWait:   DefaultHTTPMaxWait,
		NetworkId: inst.config.Network,
	}
	client := &message.Client{Transport: &message.RoutedTransport{
		Network: inst.config.Network,
		Router:  routing.MessageRouter{Router: router},
		Dialer:  inst.p2p.DialNetwork(),
	}}
	timestamps := &apiutil.TimestampService{
		Querier: &api.Collator{Querier: client, Network: client},
		Cache:   memory.New(nil),
	}

	if len(h.PeerMap) > 0 {
		apiOpts.PeerMap = peersForDumbDialer(h.PeerMap)
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
				slog.ErrorContext(r.Context(), "Failed to encode response", "error", err)
			}
			return
		}

		err2 := errors.UnknownError.Wrap(err).(*errors.Error)
		w.WriteHeader(int(err2.Code))

		err = json.NewEncoder(w).Encode(err2)
		if err != nil {
			slog.ErrorContext(r.Context(), "Failed to encode response", "error", err)
		}
	})

	c := cors.New(cors.Options{
		AllowedOrigins: h.CorsOrigins,
	})
	server, err := h.startHTTP(inst, c.Handler(api2))
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	if len(h.LetsEncrypt) > 0 {
		err = h.serveHTTP(inst, server, autocert.NewListener(h.LetsEncrypt...), false)
		if err != nil {
			return err
		}
	}

	return nil
}

func (h *HttpListener) applyHttpDefaults() {
	setDefaultPtr(&h.ReadHeaderTimeout, DefaultHTTPReadHeaderTimeout)
	setDefaultPtr(&h.ConnectionLimit, DefaultHTTPConnectionLimit)
}

func (h *HttpListener) startHTTP(inst *Instance, handler http.Handler) (*http.Server, error) {
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: *h.ReadHeaderTimeout,
	}

	inst.cleanup(server.Shutdown)

	for _, l := range h.Listen {
		l, secure, err := httpListen(l)
		if err != nil {
			return nil, err
		}
		err = h.serveHTTP(inst, server, l, secure)
		if err != nil {
			return nil, err
		}
	}
	return server, nil
}

func (h *HttpListener) serveHTTP(inst *Instance, server *http.Server, l net.Listener, secure bool) error {
	if secure && (h.TlsCertPath == "" || h.TlsKeyPath == "") {
		return errors.UnknownError.WithFormat("--tls-cert and --tls-key are required to listen on %v", l)
	}

	scheme := "http"
	if secure {
		scheme = "https"
	}
	slog.InfoContext(inst.context, "Listening", "module", "http", "address", l.Addr(), "scheme", scheme)

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
