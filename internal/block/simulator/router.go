package simulator

import (
	"context"

	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/rpc/client"
	coretypes "github.com/tendermint/tendermint/rpc/coretypes"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types/api/query"
)

type router struct {
	*Simulator
	Router routing.Router
}

func (r router) RouteAccount(account *url.URL) (string, error) {
	if partition, ok := r.routingOverrides[account.IdentityAccountID32()]; ok {
		return partition, nil
	}
	return r.Router.RouteAccount(account)
}

func (r router) Route(envs ...*protocol.Envelope) (string, error) {
	return routing.RouteEnvelopes(r.RouteAccount, envs...)
}

func (r router) Query(ctx context.Context, partition string, rawQuery []byte, opts client.ABCIQueryOptions) (*coretypes.ResultABCIQuery, error) {
	qu, e := query.UnmarshalRequest(rawQuery)
	require.NoError(r, e)

	x := r.Partition(partition)
	batch := x.Database.Begin(false)
	defer batch.Discard()
	k, v, err := x.Executor.Query(batch, qu, opts.Height, opts.Prove)
	if err != nil {
		r.Logger.Error("Query failed", "error", err)
		b, _ := errors.Wrap(errors.StatusUnknownError, err).(*errors.Error).MarshalJSON()
		res := new(coretypes.ResultABCIQuery)
		res.Response.Info = string(b)
		res.Response.Code = uint32(protocol.ErrorCodeFailed)
		return res, nil
	}

	res := new(coretypes.ResultABCIQuery)
	res.Response.Code = uint32(protocol.ErrorCodeOK)
	res.Response.Key, res.Response.Value = k, v
	return res, nil
}

func (r router) RequestAPIv2(ctx context.Context, partitionId, method string, params, result interface{}) error {
	return r.Partition(partitionId).API.RequestAPIv2(ctx, method, params, result)
}

func (r router) Submit(ctx context.Context, partition string, envelope *protocol.Envelope, pretend, async bool) (*routing.ResponseSubmit, error) {
	x := r.Partition(partition)
	deliveries := x.Submit(pretend, envelope)

	if !pretend && async {
		return new(routing.ResponseSubmit), nil
	}

	results := make([]*protocol.TransactionStatus, len(deliveries))
	for i, envelope := range deliveries {
		status := new(protocol.TransactionStatus)

		result, err := CheckTx(r.TB, x.Database, x.Executor, envelope)
		if err != nil {
			status.Set(err)
			if status.Failed() {
				r.Logger.Info("Transaction failed to check",
					"err", err,
					"type", envelope.Transaction.Body.Type(),
					"txn-hash", logging.AsHex(envelope.Transaction.GetHash()).Slice(0, 4),
					"code", status.Code,
					"principal", envelope.Transaction.Header.Principal)
			}
		}

		status.TxID = envelope.Transaction.ID()
		status.Result = result
		results[i] = status
	}

	var err error
	resp := new(routing.ResponseSubmit)
	resp.Data, err = (&protocol.TransactionResultSet{Results: results}).MarshalBinary()
	require.NoError(r, err)

	// If a user transaction fails, the batch fails
	for i, result := range results {
		if result.Code.Success() {
			continue
		}
		if deliveries[i].Transaction.Body.Type().IsUser() {
			resp.Code = uint32(protocol.ErrorCodeUnknownError)
			resp.Log = "One or more user transactions failed"
		}
	}

	return resp, nil
}
