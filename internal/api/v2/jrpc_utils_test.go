package api

import (
	"context"
	"fmt"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
)

func (m *JrpcMethods) DoExecute(ctx context.Context, req *TxRequest, payload []byte) interface{} {
	return m.execute(ctx, req, payload)
}

func (m *JrpcMethods) Querier() Querier {
	return m.opts.Query
}

func (m *JrpcMethods) GetMethod(name string) jsonrpc2.MethodFunc {
	method := m.methods[name]
	if method == nil {
		panic(fmt.Errorf("method %q not found", name))
	}
	return method
}
