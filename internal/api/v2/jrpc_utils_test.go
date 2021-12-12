package api

import (
	"context"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
)

func (m *JrpcMethods) DoExecute(ctx context.Context, req *TxRequest, payload []byte) interface{} {
	return m.execute(ctx, req, payload)
}

func (m *JrpcMethods) Querier() Querier {
	return m.opts.Query
}

func (m *JrpcMethods) GetMethod(name string) jsonrpc2.MethodFunc {
	return m.methods[name]
}
