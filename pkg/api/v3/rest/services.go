// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package rest

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/libp2p/go-libp2p/core/peer"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"golang.org/x/exp/slog"
)

type Service interface {
	Register(*httprouter.Router) error
}

func NewHandler(services ...Service) (*httprouter.Router, error) {
	r := httprouter.New()
	for _, service := range services {
		err := service.Register(r)
		if err != nil {
			return nil, err
		}
	}

	return r, nil
}

type NodeService struct{ api.NodeService }

func (s NodeService) Register(r *httprouter.Router) error {
	r.GET("/node/info", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		var err error
		tryCall1(&err, w, r, s.NodeService.NodeInfo,
			api.NodeInfoOptions{
				PeerID: parsePeerID.allowMissing().fromQuery(r).try(&err, "peer_id"),
			},
		)
	})

	r.GET("/node/services", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		var err error
		tryCall1(&err, w, r, s.NodeService.FindService, api.FindServiceOptions{
			Network: r.URL.Query().Get("network"),
			Known:   parseBool.allowMissing().fromQuery(r).try(&err, "known"),
			Timeout: parseDuration.allowMissing().fromQuery(r).try(&err, "timeout"),
			Service: parseServiceAddress.allowMissing().fromQuery(r).try(&err, "type"),
		})
	})
	return nil
}

type ConsensusService struct{ api.ConsensusService }

func (s ConsensusService) Register(r *httprouter.Router) error {
	r.GET("/consensus/status", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		var err error
		tryCall1(&err, w, r, s.ConsensusService.ConsensusStatus,
			api.ConsensusStatusOptions{
				NodeID:    r.URL.Query().Get("node_id"),
				Partition: r.URL.Query().Get("partition"),
			},
		)
	})
	return nil
}

type NetworkService struct{ api.NetworkService }

func (s NetworkService) Register(r *httprouter.Router) error {
	r.GET("/network/status", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		var err error
		tryCall1(&err, w, r, s.NetworkService.NetworkStatus,
			api.NetworkStatusOptions{
				Partition: r.URL.Query().Get("partition"),
			},
		)
	})
	return nil
}

type MetricsService struct{ api.MetricsService }

func (s MetricsService) Register(r *httprouter.Router) error {
	r.GET("/metrics", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		var err error
		tryCall1(&err, w, r, s.MetricsService.Metrics,
			api.MetricsOptions{
				Partition: r.URL.Query().Get("partition"),
				Span:      parseUint.allowMissing().fromQuery(r).try(&err, "span"),
			},
		)
	})
	return nil
}

type Submitter struct{ api.Submitter }

func (s Submitter) Register(r *httprouter.Router) error {
	r.POST("/submit", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		var err error
		tryCall2(&err, w, r, s.Submitter.Submit,
			parseJSON[*messaging.Envelope]().fromBody(r).try(&err, ""),
			api.SubmitOptions{
				Verify: ptr(parseBool).allowMissing().fromQuery(r).try(&err, "verify"),
				Wait:   ptr(parseBool).allowMissing().fromQuery(r).try(&err, "wait"),
			},
		)
	})
	return nil
}

type Validator struct{ api.Validator }

func (s Validator) Register(r *httprouter.Router) error {
	r.POST("/validate", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		var err error
		tryCall2(&err, w, r, s.Validator.Validate,
			parseJSON[*messaging.Envelope]().fromBody(r).try(&err, ""),
			api.ValidateOptions{
				Full: ptr(parseBool).allowMissing().fromQuery(r).try(&err, "full"),
			},
		)
	})
	return nil
}

type Faucet struct{ api.Faucet }

func (s Faucet) Register(r *httprouter.Router) error {
	r.POST("/faucet", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		var err error
		tryCall2(&err, w, r, s.Faucet.Faucet,
			parseUrl.fromBody(r).try(&err, ""),
			api.FaucetOptions{
				Token: parseUrl.allowMissing().fromQuery(r).try(&err, "token"),
			},
		)
	})
	return nil
}

func tryCall1[V1, R any](lastErr *error, w http.ResponseWriter, r *http.Request, fn func(context.Context, V1) (R, error), v1 V1) {
	if *lastErr != nil {
		responder{w, r}.write(nil, *lastErr)
	} else {
		responder{w, r}.write(fn(r.Context(), v1))
	}
}

func tryCall2[V1, V2, R any](lastErr *error, w http.ResponseWriter, r *http.Request, fn func(context.Context, V1, V2) (R, error), v1 V1, v2 V2) {
	if *lastErr != nil {
		responder{w, r}.write(nil, *lastErr)
	} else {
		responder{w, r}.write(fn(r.Context(), v1, v2))
	}
}

type parser[T any] func(string) (T, error)

var parseUrl parser[*url.URL] = url.Parse
var parseBytes parser[[]byte] = hex.DecodeString
var parseBool parser[bool] = strconv.ParseBool
var parseDuration parser[time.Duration] = time.ParseDuration
var parsePeerID parser[peer.ID] = peer.Decode
var parseSignatureType = parseEnum("signature type", protocol.SignatureTypeByName)
var parseServiceAddress parser[*api.ServiceAddress] = api.ParseServiceAddress

var parseBytes32 parser[[32]byte] = func(s string) ([32]byte, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return [32]byte{}, err
	}
	if len(b) != 32 {
		return [32]byte{}, errors.BadRequest.WithFormat("not a hash: want 32 bytes, got %d", len(b))
	}
	return *(*[32]byte)(b), nil
}

var parseUint parser[uint64] = func(s string) (uint64, error) {
	return strconv.ParseUint(s, 10, 64)
}

func parseEnum[T any](name string, parse func(string) (T, bool)) parser[T] {
	return func(s string) (T, error) {
		v, ok := parse(s)
		if ok {
			return v, nil
		}
		return v, errors.BadRequest.WithFormat("%q is not a valid %s", s, name)
	}
}

func parseJSON[T any]() parser[T] {
	return func(s string) (T, error) {
		var v T
		err := json.Unmarshal([]byte(s), &v)
		return v, err
	}
}

func ptr[T any](p parser[T]) parser[*T] {
	return func(s string) (*T, error) {
		v, err := p(s)
		if err != nil {
			return nil, err
		}
		return &v, nil
	}
}

func (p parser[T]) allowMissing() parser[T] {
	return func(s string) (T, error) {
		if s == "" {
			var z T
			return z, nil
		}
		return p(s)
	}
}

func (p parser[T]) fromRoute(m httprouter.Params) parser[T] {
	return func(s string) (T, error) {
		v, err := p(m.ByName(s))
		if err != nil {
			return v, errors.BadRequest.WithFormat("url parameter %s: %w", s, err)
		}
		return v, nil
	}
}

func (p parser[T]) fromQuery(r *http.Request) parser[T] {
	return func(s string) (T, error) {
		v, err := p(r.URL.Query().Get(s))
		if err != nil {
			return v, errors.BadRequest.WithFormat("query parameter %s: %w", s, err)
		}
		return v, nil
	}
}

func (p parser[T]) fromBody(r *http.Request) parser[T] {
	return func(string) (T, error) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			var z T
			return z, nil
		}
		return p(string(b))
	}
}

func (p parser[T]) try(lastErr *error, s string) T {
	if *lastErr != nil {
		var z T
		return z
	}
	v, err := p(s)
	if err != nil {
		*lastErr = err
	}
	return v
}

type responder struct {
	w http.ResponseWriter
	r *http.Request
}

func (r responder) write(res any, err error) {
	if err == nil {
		err := json.NewEncoder(r.w).Encode(res)
		if err != nil {
			slog.ErrorCtx(r.r.Context(), "Failed to encode response", "error", err)
		}
		return
	}

	err2 := errors.UnknownError.Wrap(err).(*errors.Error)
	r.w.WriteHeader(int(err2.Code))

	err = json.NewEncoder(r.w).Encode(err2)
	if err != nil {
		slog.ErrorCtx(r.r.Context(), "Failed to encode response", "error", err)
	}
}
