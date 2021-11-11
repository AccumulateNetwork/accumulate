package api

import (
	"context"
	"encoding/json"

	"github.com/AccumulateNetwork/accumulated"
	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/go-playground/validator/v10"
)

type JrpcOptions struct {
	Prometheus string
	Query      Querier
}

type JrpcMethods struct {
	opts     JrpcOptions
	validate *validator.Validate
}

func NewJrpc(opts JrpcOptions) (*JrpcMethods, error) {
	var err error
	m := new(JrpcMethods)
	m.opts = opts

	m.validate, err = protocol.NewValidator()
	if err != nil {
		return nil, err
	}

	return m, nil
}

func (m *JrpcMethods) Version(_ context.Context, params json.RawMessage) interface{} {
	res := new(QueryResponse)
	res.Type = "version"
	res.Data = map[string]interface{}{
		"version":        accumulated.Version,
		"commit":         accumulated.Commit,
		"versionIsKnown": accumulated.IsVersionKnown(),
	}
	return res
}
