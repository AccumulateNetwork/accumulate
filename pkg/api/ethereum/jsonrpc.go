package ethrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
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

func NewClient(endpoint string) Service {
	return &JSONRPCClient{Endpoint: accumulate.ResolveWellKnownEndpoint(endpoint, "eth")}
}

func NewHandler(service Service) http.Handler {
	return jsonrpc.HTTPHandler{H: &JSONRPCHandler{Service: service}}
}

func (c *JSONRPCClient) EthChainId(ctx context.Context) (Bytes, error) {
	return call[Bytes](c, ctx, "eth_chainId", json.RawMessage("{}"))
}

func (h *JSONRPCHandler) eth_chainId(ctx context.Context, _ json.RawMessage) (any, error) {
	return h.Service.EthChainId(ctx)
}

func (h *JSONRPCHandler) net_version(ctx context.Context, _ json.RawMessage) (any, error) {
	id, err := h.Service.EthChainId(ctx)
	if err != nil {
		return nil, err
	}
	ver := new(big.Int)
	ver.SetBytes(id)
	return ver, nil
}

func (c *JSONRPCClient) EthBlockNumber(ctx context.Context) (uint64, error) {
	return call[uint64](c, ctx, "eth_blockNumber", json.RawMessage("{}"))
}

func (h *JSONRPCHandler) eth_blockNumber(ctx context.Context, _ json.RawMessage) (any, error) {
	return h.Service.EthBlockNumber(ctx)
}

func (c *JSONRPCClient) AccTypedData(ctx context.Context, txn *protocol.Transaction, sig protocol.Signature) (*encoding.EIP712Call, error) {
	return call[*encoding.EIP712Call](c, ctx, "acc_typedData", map[string]any{
		"transaction": txn,
		"signature":   sig,
	})
}

func (h *JSONRPCHandler) acc_typedData(ctx context.Context, raw json.RawMessage) (any, error) {
	var params struct {
		Transaction *protocol.Transaction                          `json:"transaction"`
		Signature   encoding.JsonUnmarshalWith[protocol.Signature] `json:"signature"`
	}
	params.Signature.Func = protocol.UnmarshalSignatureJSON

	err := json.Unmarshal(raw, &params)
	if err != nil {
		return nil, jsonrpc.InvalidParams.With(err, err)
	}

	if params.Transaction == nil {
		return nil, jsonrpc.InvalidParams.With("missing transaction", nil)
	}
	if params.Signature.Value == nil {
		return nil, jsonrpc.InvalidParams.With("missing signature", nil)
	}

	return h.Service.AccTypedData(ctx, params.Transaction, params.Signature.Value)
}

func call[V any](c *JSONRPCClient, ctx context.Context, method string, params any) (V, error) {
	var v V
	err := c.Client.Call(ctx, c.Endpoint, method, params, &v)
	return v, err
}

func (h *JSONRPCHandler) ServeJSONRPC(ctx context.Context, method string, params json.RawMessage) (result any, err error) {
	switch method {
	case "net_version":
		return h.net_version(ctx, params)

	case "eth_chainId":
		return h.eth_chainId(ctx, params)
	case "eth_blockNumber":
		return h.eth_blockNumber(ctx, params)

	case "acc_typedData":
		return h.acc_typedData(ctx, params)
	}

	return nil, &jsonrpc.Error{
		Code:    jsonrpc.MethodNotFound,
		Message: fmt.Sprintf("%q is not a supported method", method),
	}
}
