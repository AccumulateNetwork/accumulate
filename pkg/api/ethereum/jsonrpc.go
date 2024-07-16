// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package ethrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/utils/jsonrpc"
)

type JSONRPCClient struct {
	Client   jsonrpc.HTTPClient
	Endpoint string
}

type JSONRPCHandler struct {
	Service Service
}

var _ Service = (*JSONRPCClient)(nil)

func NewClient(endpoint string) Service {
	return &JSONRPCClient{Endpoint: accumulate.ResolveWellKnownEndpoint(endpoint, "eth")}
}

func NewHandler(service Service) http.Handler {
	return jsonrpc.HTTPHandler{H: &JSONRPCHandler{Service: service}}
}

func (c *JSONRPCClient) EthChainId(ctx context.Context) (*Number, error) {
	return clientCall[*Number](c, ctx, "eth_chainId", []any{})
}

func (h *JSONRPCHandler) eth_chainId(ctx context.Context, _ json.RawMessage) (any, error) {
	return h.Service.EthChainId(ctx)
}

func (h *JSONRPCHandler) net_version(ctx context.Context, _ json.RawMessage) (any, error) {
	id, err := h.Service.EthChainId(ctx)
	if err != nil {
		return nil, err
	}
	return id.Int().Uint64(), nil
}

func (c *JSONRPCClient) EthBlockNumber(ctx context.Context) (*Number, error) {
	return clientCall[*Number](c, ctx, "eth_blockNumber", []any{})
}

func (h *JSONRPCHandler) eth_blockNumber(ctx context.Context, _ json.RawMessage) (any, error) {
	return h.Service.EthBlockNumber(ctx)
}

func (c *JSONRPCClient) EthGasPrice(ctx context.Context) (*Number, error) {
	return clientCall[*Number](c, ctx, "eth_gasPrice", []any{})
}

func (h *JSONRPCHandler) eth_gasPrice(ctx context.Context, _ json.RawMessage) (any, error) {
	return h.Service.EthGasPrice(ctx)
}

func (c *JSONRPCClient) EthGetBalance(ctx context.Context, addr Address, block string) (*Number, error) {
	return clientCall[*Number](c, ctx, "eth_getBalance", []any{addr, block})
}

func (h *JSONRPCHandler) eth_getBalance(ctx context.Context, raw json.RawMessage) (any, error) {
	return handlerCall2(ctx, raw, h.Service.EthGetBalance)
}

func (c *JSONRPCClient) EthGetBlockByNumber(ctx context.Context, block string, expand bool) (*BlockData, error) {
	return clientCall[*BlockData](c, ctx, "eth_getBlockByNumber", []any{block, expand})
}

func (h *JSONRPCHandler) eth_getBlockByNumber(ctx context.Context, raw json.RawMessage) (any, error) {
	return handlerCall2(ctx, raw, h.Service.EthGetBlockByNumber)
}

func (c *JSONRPCClient) AccTypedData(ctx context.Context, txn *protocol.Transaction, sig protocol.Signature) (*encoding.EIP712Call, error) {
	return clientCall[*encoding.EIP712Call](c, ctx, "acc_typedData", []any{txn, sig})
}

func (h *JSONRPCHandler) acc_typedData(ctx context.Context, raw json.RawMessage) (any, error) {
	return handlerCall2(ctx, raw, func(ctx context.Context, txn *protocol.Transaction, sig *signatureWrapper) (any, error) {
		return h.Service.AccTypedData(ctx, txn, sig.V)
	})
}

func clientCall[V any](c *JSONRPCClient, ctx context.Context, method string, params any) (V, error) {
	var v V
	err := c.Client.Call(ctx, c.Endpoint, method, params, &v)
	return v, err
}

func handlerCall2[V1, V2, R any](ctx context.Context, raw json.RawMessage, fn func(context.Context, V1, V2) (R, error)) (any, error) {
	var v1 V1
	var v2 V2
	err := decodeN(raw, &v1, &v2)
	if err != nil {
		return nil, err
	}
	return fn(ctx, v1, v2)
}

func decodeN(raw json.RawMessage, v ...any) error {
	var params []json.RawMessage
	err := json.Unmarshal(raw, &params)
	if err != nil {
		return jsonrpc.InvalidParams.With(err, err)
	}
	if len(params) != len(v) {
		err := fmt.Errorf("expected %d parameters, got %d", len(v), len(params))
		return jsonrpc.InvalidParams.With(err, err)
	}
	for i, param := range params {
		err = json.Unmarshal(param, v[i])
		if err != nil {
			return jsonrpc.InvalidParams.With(err, err)
		}
	}
	return nil
}

func (h *JSONRPCHandler) ServeJSONRPC(ctx context.Context, method string, params json.RawMessage) (result any, err error) {
	switch method {
	case "net_version":
		return h.net_version(ctx, params)

	case "eth_chainId":
		return h.eth_chainId(ctx, params)
	case "eth_blockNumber":
		return h.eth_blockNumber(ctx, params)
	case "eth_gasPrice":
		return h.eth_gasPrice(ctx, params)
	case "eth_getBalance":
		return h.eth_getBalance(ctx, params)
	case "eth_getBlockByNumber":
		return h.eth_getBlockByNumber(ctx, params)

	case "acc_typedData":
		return h.acc_typedData(ctx, params)
	}

	/*
	   Missing "eth_getCode"
	   Missing "eth_estimateGas"
	   Missing "eth_getTransactionCount"
	*/
	return nil, &jsonrpc.Error{
		Code:    jsonrpc.MethodNotFound,
		Message: fmt.Sprintf("%q is not a supported method", method),
	}
}

type signatureWrapper struct {
	V protocol.Signature
}

func (s *signatureWrapper) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.V)
}

func (s *signatureWrapper) UnmarshalJSON(b []byte) error {
	var err error
	s.V, err = protocol.UnmarshalSignatureJSON(b)
	return err
}
