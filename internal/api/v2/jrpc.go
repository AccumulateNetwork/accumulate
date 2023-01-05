// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"encoding/json"
	"io"
	stdlog "log"
	"mime"
	"net/http"
	"os"
	"strings"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/go-playground/validator/v10"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/web"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type JrpcMethods struct {
	Options
	querier  *queryFrontend
	methods  jsonrpc2.MethodMap
	validate *validator.Validate
	logger   log.Logger
}

func NewJrpc(opts Options) (*JrpcMethods, error) {
	var err error
	m := new(JrpcMethods)
	m.Options = opts
	m.querier = new(queryFrontend)
	m.querier.Options = opts
	m.querier.backend = new(queryBackend)
	m.querier.backend.Options = opts

	if opts.Key == nil {
		return nil, errors.BadRequest.WithFormat("missing key")
	}

	if opts.Logger != nil {
		m.logger = opts.Logger.With("module", "jrpc")
		m.querier.backend.logger.L = m.logger
	}

	m.validate, err = protocol.NewValidator()
	if err != nil {
		return nil, err
	}

	m.populateMethodTable()
	return m, nil
}

func (m *JrpcMethods) logError(msg string, keyVals ...interface{}) {
	if m.logger != nil {
		m.logger.Error(msg, keyVals...)
	}
}

func (m *JrpcMethods) EnableDebug() {
	m.methods["debug-query-direct"] = func(_ context.Context, params json.RawMessage) interface{} {
		req := new(GeneralQuery)
		err := m.parse(params, req)
		if err != nil {
			return err
		}

		return jrpcFormatResponse(m.querier.QueryUrl(req.Url, req.QueryOptions))
	}
}

func (m *JrpcMethods) NewMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/status", m.jrpc2http(m.Status))
	mux.Handle("/version", m.jrpc2http(m.Version))
	mux.Handle("/describe", m.jrpc2http(m.Describe))
	mux.Handle("/v2", jsonrpc2.HTTPRequestHandler(m.methods, stdlog.New(os.Stdout, "", 0)))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/x")
		w.WriteHeader(http.StatusTemporaryRedirect)
	})

	webex := web.Handler()
	mux.HandleFunc("/x/", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/x")
		r.RequestURI = strings.TrimPrefix(r.RequestURI, "/x")
		webex.ServeHTTP(w, r)
	})
	return mux
}

func (m *JrpcMethods) jrpc2http(jrpc jsonrpc2.MethodFunc) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			res.WriteHeader(http.StatusBadRequest)
			return
		}

		var params json.RawMessage
		mediatype, _, _ := mime.ParseMediaType(req.Header.Get("Content-Type"))
		if mediatype == "application/json" || mediatype == "text/json" {
			params = body
		}

		r := jrpc(req.Context(), params)
		res.Header().Add("Content-Type", "application/json")
		data, err := json.Marshal(r)
		if err != nil {
			m.logError("Failed to marshal status", "error", err)
			res.WriteHeader(http.StatusInternalServerError)
			return
		}

		_, _ = res.Write(data)
	}
}

func (m *JrpcMethods) Status(ctx context.Context, _ json.RawMessage) interface{} {
	if m.ConnectionManager == nil {
		return internalError(errors.InternalError.WithFormat("missing connection manager"))
	}

	conn, err := m.ConnectionManager.SelectConnection(m.Options.Describe.PartitionId, true)
	if err != nil {
		return internalError(err)
	}

	// Get the latest block height and BPT hash from Tendermint RPC
	tmStatus, err := conn.GetABCIClient().Status(ctx)
	if err != nil {
		return internalError(err)
	}

	// Get the latest root chain anchor from the Accumulate API
	apiclient := conn.GetAPIClient()
	rootAnchor, err := getLatestRootChainAnchor(apiclient, m.Options.Describe.Ledger(), ctx)
	if err != nil {
		return internalError(err)
	}

	// Get the latest directory anchor from the Accumulate API
	dnAnchorHeight, err := getLatestDirectoryAnchor(conn, m.Options.Describe.AnchorPool())
	if err != nil {
		return err
	}

	if m.Options.Describe.NetworkType == config.NetworkTypeDirectory {
		status := new(StatusResponse)
		status.Ok = true
		status.DnHeight = tmStatus.SyncInfo.LatestBlockHeight
		status.DnTime = tmStatus.SyncInfo.LatestBlockTime
		if len(tmStatus.SyncInfo.LatestAppHash) == 32 {
			status.DnBptHash = *(*[32]byte)(tmStatus.SyncInfo.LatestBlockHash)
		}
		status.DnRootHash = *rootAnchor
		return status
	}

	status := new(StatusResponse)
	status.Ok = true
	status.BvnHeight = tmStatus.SyncInfo.LatestBlockHeight
	status.BvnTime = tmStatus.SyncInfo.LatestBlockTime
	if len(tmStatus.SyncInfo.LatestAppHash) == 32 {
		status.BvnBptHash = *(*[32]byte)(tmStatus.SyncInfo.LatestBlockHash)
	}
	status.BvnRootHash = *rootAnchor
	status.LastDirectoryAnchorHeight = dnAnchorHeight
	return status
}

func (m *JrpcMethods) Version(_ context.Context, params json.RawMessage) interface{} {
	res := new(ChainQueryResponse)
	res.Type = "version"
	res.Data = VersionResponse{
		Version:        accumulate.Version,
		Commit:         accumulate.Commit,
		VersionIsKnown: accumulate.IsVersionKnown(),
	}
	return res
}

func (m *JrpcMethods) Describe(_ context.Context, params json.RawMessage) interface{} {
	res := new(DescriptionResponse)
	res.Network = m.Options.Describe.Network
	res.PartitionId = m.Options.Describe.PartitionId
	res.NetworkType = m.Options.Describe.NetworkType

	// Load network variable values
	v, err := m.loadGlobals()
	if err != nil {
		res.Error = errors.UnknownError.Wrap(err).(*errors.Error)
	} else {
		res.Values = *v
	}

	return res
}
