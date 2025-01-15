// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package jsonrpc

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

const ErrCodeProtocol = -33000

func parseRequest[T any](input json.RawMessage) (T, error) {
	var v T
	if len(input) == 0 {
		return v, errors.BadRequest.With("empty request")
	}
	err := json.Unmarshal(input, &v)
	if err != nil {
		return v, errors.EncodingError.WithFormat("unmarshal request: %w", err)
	}
	return v, nil
}

func formatResponse(res interface{}, err error) interface{} {
	if err != nil {
		// Ensure the error is an Error
		type Error errors.Error
		err2 := errors.UnknownError.Wrap(err).(*errors.Error)
		return jsonrpc2.NewError(ErrCodeProtocol-jsonrpc2.ErrorCode(err2.Code), err2.Error(), (*Error)(err2))
	}

	// jsonrpc2 behaves badly if the response is nil
	v := reflect.ValueOf(res)
	if v.Kind() == reflect.Slice && v.IsNil() {
		return json.RawMessage("[]")
	}
	return res
}

type NodeService struct {
	api.NodeService
}

func (s NodeService) methods() jsonrpc2.MethodMap {
	return jsonrpc2.MethodMap{
		"node-info":    s.nodeInfo,
		"find-service": s.findService,
	}
}

func (s NodeService) nodeInfo(ctx context.Context, params json.RawMessage) interface{} {
	req, err := parseRequest[*message.NodeInfoRequest](params)
	if err != nil {
		return formatResponse(nil, err)
	}
	return formatResponse(s.NodeService.NodeInfo(ctx, req.NodeInfoOptions))
}

func (s NodeService) findService(ctx context.Context, params json.RawMessage) interface{} {
	req, err := parseRequest[*message.FindServiceRequest](params)
	if err != nil {
		return formatResponse(nil, err)
	}
	return formatResponse(s.NodeService.FindService(ctx, req.FindServiceOptions))
}

type ConsensusService struct{ api.ConsensusService }

func (s ConsensusService) methods() jsonrpc2.MethodMap {
	return jsonrpc2.MethodMap{
		"consensus-status": s.consensusStatus,
	}
}

func (s ConsensusService) consensusStatus(ctx context.Context, params json.RawMessage) interface{} {
	req, err := parseRequest[*message.ConsensusStatusRequest](params)
	if err != nil {
		return formatResponse(nil, err)
	}
	return formatResponse(s.ConsensusService.ConsensusStatus(ctx, req.ConsensusStatusOptions))
}

type NetworkService struct{ api.NetworkService }

func (s NetworkService) methods() jsonrpc2.MethodMap {
	return jsonrpc2.MethodMap{
		"network-status": s.networkStatus,
	}
}

func (s NetworkService) networkStatus(ctx context.Context, params json.RawMessage) interface{} {
	req, err := parseRequest[*message.NetworkStatusRequest](params)
	if err != nil {
		return formatResponse(nil, err)
	}
	return formatResponse(s.NetworkService.NetworkStatus(ctx, req.NetworkStatusOptions))
}

type SnapshotService struct{ api.SnapshotService }

func (s SnapshotService) methods() jsonrpc2.MethodMap {
	return jsonrpc2.MethodMap{
		"list-snapshots": s.listSnapshots,
	}
}

func (s SnapshotService) listSnapshots(ctx context.Context, params json.RawMessage) interface{} {
	req, err := parseRequest[*message.ListSnapshotsRequest](params)
	if err != nil {
		return formatResponse(nil, err)
	}
	return formatResponse(s.SnapshotService.ListSnapshots(ctx, req.ListSnapshotsOptions))
}

type MetricsService struct{ api.MetricsService }

func (s MetricsService) methods() jsonrpc2.MethodMap {
	return jsonrpc2.MethodMap{
		"metrics": s.metrics,
	}
}

func (s MetricsService) metrics(ctx context.Context, params json.RawMessage) interface{} {
	req, err := parseRequest[*message.MetricsRequest](params)
	if err != nil {
		return formatResponse(nil, err)
	}
	return formatResponse(s.MetricsService.Metrics(ctx, req.MetricsOptions))
}

type Querier struct{ api.Querier }

func (s Querier) methods() jsonrpc2.MethodMap {
	return jsonrpc2.MethodMap{
		"query": s.query,
	}
}

func (s Querier) query(ctx context.Context, params json.RawMessage) interface{} {
	req, err := parseRequest[*message.QueryRequest](params)
	if err != nil {
		return formatResponse(nil, err)
	}
	return formatResponse(s.Querier.Query(ctx, req.Scope, req.Query))
}

type Submitter struct{ api.Submitter }

func (s Submitter) methods() jsonrpc2.MethodMap {
	return jsonrpc2.MethodMap{
		"submit": s.submit,
	}
}

func (s Submitter) submit(ctx context.Context, params json.RawMessage) interface{} {
	req, err := parseRequest[*message.SubmitRequest](params)
	if err != nil {
		return formatResponse(nil, err)
	}
	return formatResponse(s.Submitter.Submit(ctx, req.Envelope, req.SubmitOptions))
}

type Validator struct{ api.Validator }

func (s Validator) methods() jsonrpc2.MethodMap {
	return jsonrpc2.MethodMap{
		"validate": s.validate,
	}
}

func (s Validator) validate(ctx context.Context, params json.RawMessage) interface{} {
	req, err := parseRequest[*message.ValidateRequest](params)
	if err != nil {
		return formatResponse(nil, err)
	}
	return formatResponse(s.Validator.Validate(ctx, req.Envelope, req.ValidateOptions))
}

type Faucet struct{ api.Faucet }

func (s Faucet) methods() jsonrpc2.MethodMap {
	return jsonrpc2.MethodMap{
		"faucet": s.faucet,
	}
}

func (s Faucet) faucet(ctx context.Context, params json.RawMessage) interface{} {
	req, err := parseRequest[*message.FaucetRequest](params)
	if err != nil {
		return formatResponse(nil, err)
	}
	return formatResponse(s.Faucet.Faucet(ctx, req.Account, req.FaucetOptions))
}

type Sequencer struct{ private.Sequencer }

func (s Sequencer) methods() jsonrpc2.MethodMap {
	return jsonrpc2.MethodMap{
		"private-sequence": s.sequence,
	}
}

func (s Sequencer) sequence(ctx context.Context, params json.RawMessage) interface{} {
	req, err := parseRequest[*message.PrivateSequenceRequest](params)
	if err != nil {
		return formatResponse(nil, err)
	}
	return formatResponse(s.Sequencer.Sequence(ctx, req.Source, req.Destination, req.SequenceNumber, req.SequenceOptions))
}
