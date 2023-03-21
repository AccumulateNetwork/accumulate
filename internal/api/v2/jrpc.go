// Copyright 2023 The Accumulate Authors
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

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/go-playground/validator/v10"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type JrpcMethods struct {
	Options
	methods  jsonrpc2.MethodMap
	validate *validator.Validate
	logger   log.Logger
}

func NewJrpc(opts Options) (*JrpcMethods, error) {
	var err error
	m := new(JrpcMethods)
	m.Options = opts

	if opts.Logger != nil {
		m.logger = opts.Logger.With("module", "jrpc")
	}

	if opts.LocalV3 == nil ||
		opts.Querier == nil ||
		opts.Submitter == nil ||
		opts.Network == nil ||
		opts.Faucet == nil ||
		opts.Validator == nil ||
		opts.Sequencer == nil {
		return nil, errors.BadRequest.With("missing P2P clients")
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

func (m *JrpcMethods) NewMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/status", m.jrpc2http(m.Status))
	mux.Handle("/version", m.jrpc2http(m.Version))
	mux.Handle("/describe", m.jrpc2http(m.Describe))
	mux.Handle("/v2", jsonrpc2.HTTPRequestHandler(m.methods, stdlog.New(os.Stdout, "", 0)))
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
	s, err := m.LocalV3.ConsensusStatus(ctx, api.ConsensusStatusOptions{})
	if err != nil {
		return accumulateError(err)
	}

	status := new(StatusResponse)
	status.Ok = true
	status.LastDirectoryAnchorHeight = s.LastBlock.DirectoryAnchorHeight
	switch s.PartitionType {
	case protocol.PartitionTypeDirectory:
		status.DnHeight = s.LastBlock.Height
		status.DnTime = s.LastBlock.Time
		status.DnRootHash = s.LastBlock.ChainRoot
		status.DnBptHash = s.LastBlock.StateRoot
	case protocol.PartitionTypeBlockValidator:
		status.BvnHeight = s.LastBlock.Height
		status.BvnTime = s.LastBlock.Time
		status.BvnRootHash = s.LastBlock.ChainRoot
		status.BvnBptHash = s.LastBlock.StateRoot
	}
	return status
}

func (m *JrpcMethods) Version(ctx context.Context, _ json.RawMessage) interface{} {
	node, err := m.LocalV3.ConsensusStatus(ctx, api.ConsensusStatusOptions{})
	if err != nil {
		return accumulateError(err)
	}
	res := new(ChainQueryResponse)
	res.Type = "version"
	res.Data = VersionResponse{
		Version:        node.Version,
		Commit:         node.Commit,
		VersionIsKnown: node.Commit != "",
	}
	return res
}

func (m *JrpcMethods) Describe(ctx context.Context, _ json.RawMessage) interface{} {
	net, err := m.LocalV3.NetworkStatus(ctx, api.NetworkStatusOptions{})
	if err != nil {
		return accumulateError(err)
	}

	res := new(DescriptionResponse)
	if m.Options.Describe != nil {
		res.PartitionId = m.Options.Describe.PartitionId
		res.NetworkType = m.Options.Describe.NetworkType
		res.Network = m.Options.Describe.Network
	}
	res.Values.Globals = net.Globals
	res.Values.Network = net.Network
	res.Values.Oracle = net.Oracle
	res.Values.Routing = net.Routing
	return res
}
