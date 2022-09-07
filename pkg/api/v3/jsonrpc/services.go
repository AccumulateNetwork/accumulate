package jsonrpc

import (
	"context"
	"encoding/json"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
)

//go:generate go run ../../../../tools/cmd/gen-types --package jsonrpc requests.yml --reference ../options.yml

const errCodeProtocol = -33000

type QueryRecordOptions = api.QueryRecordOptions
type QueryRangeOptions = api.QueryRangeOptions
type SubmitOptions = api.SubmitOptions

func parseRequest[T any](input json.RawMessage) (T, error) {
	var v T
	err := json.Unmarshal(input, &v)
	if err != nil {
		return v, errors.Format(errors.StatusEncodingError, "unmarshal request: %w", err)
	}
	return v, nil
}

func formatResponse(res interface{}, err error) interface{} {
	if err == nil {
		return res
	}

	// Ensure the error is an Error
	err2 := errors.Wrap(errors.StatusUnknownError, err).(*errors.Error)
	return jsonrpc2.NewError(errCodeProtocol-jsonrpc2.ErrorCode(err2.Code), err2.Code.String(), err2)
}

type NodeService struct{ api.NodeService }

func (s NodeService) methods() jsonrpc2.MethodMap {
	return jsonrpc2.MethodMap{
		"status":   s.Status,
		"version":  s.Version,
		"describe": s.Describe,
		"metrics":  s.Metrics,
	}
}

func (s NodeService) Status(ctx context.Context, _ json.RawMessage) interface{} {
	return formatResponse(s.NodeService.Status(ctx))
}

func (s NodeService) Version(ctx context.Context, _ json.RawMessage) interface{} {
	return formatResponse(s.NodeService.Version(ctx))
}

func (s NodeService) Describe(ctx context.Context, _ json.RawMessage) interface{} {
	return formatResponse(s.NodeService.Describe(ctx))
}

func (s NodeService) Metrics(ctx context.Context, _ json.RawMessage) interface{} {
	return formatResponse(s.NodeService.Metrics(ctx))
}

type QueryService struct{ api.QueryService }

func (s QueryService) methods() jsonrpc2.MethodMap {
	return jsonrpc2.MethodMap{
		"query-record": s.QueryRecord,
		"query-range":  s.QueryRange,
	}
}

func (s QueryService) QueryRecord(ctx context.Context, params json.RawMessage) interface{} {
	req, err := parseRequest[*QueryRecordRequest](params)
	if err != nil {
		return formatResponse(nil, err)
	}
	return formatResponse(s.QueryService.QueryRecord(ctx, req.Account, req.Fragment, req.QueryRecordOptions))
}

func (s QueryService) QueryRange(ctx context.Context, params json.RawMessage) interface{} {
	req, err := parseRequest[*QueryRangeRequest](params)
	if err != nil {
		return formatResponse(nil, err)
	}
	return formatResponse(s.QueryService.QueryRange(ctx, req.Account, req.Fragment, req.QueryRangeOptions))
}

type SubmitService struct{ api.SubmitService }

func (s SubmitService) methods() jsonrpc2.MethodMap {
	return jsonrpc2.MethodMap{
		"submit": s.Submit,
	}
}

func (s SubmitService) Submit(ctx context.Context, params json.RawMessage) interface{} {
	req, err := parseRequest[*SubmitRequest](params)
	if err != nil {
		return formatResponse(nil, err)
	}
	return formatResponse(s.SubmitService.Submit(ctx, req.Envelope, req.SubmitOptions))
}

type FaucetService struct{ api.FaucetService }

func (s FaucetService) methods() jsonrpc2.MethodMap {
	return jsonrpc2.MethodMap{
		"faucet": s.Faucet,
	}
}

func (s FaucetService) Faucet(ctx context.Context, params json.RawMessage) interface{} {
	req, err := parseRequest[*FaucetRequest](params)
	if err != nil {
		return formatResponse(nil, err)
	}
	return formatResponse(s.FaucetService.Faucet(ctx, req.Account, req.SubmitOptions))
}
