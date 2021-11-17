package api

import "context"

func (m *JrpcMethods) DoExecute(ctx context.Context, req *TxRequest, payload []byte) interface{} {
	return m.execute(ctx, req, payload)
}
