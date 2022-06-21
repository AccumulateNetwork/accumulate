package simulator

import (
	"context"

	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/rpc/client"
	coretypes "github.com/tendermint/tendermint/rpc/coretypes"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
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
	switch {
	case err == nil:
		//Ok

	case errors.Is(err, storage.ErrNotFound):
		res := new(coretypes.ResultABCIQuery)
		res.Response.Info = err.Error()
		res.Response.Code = uint32(protocol.ErrorCodeNotFound)
		return res, nil

	default:
		r.Logf("Query failed: %v", err)
		res := new(coretypes.ResultABCIQuery)
		res.Response.Info = err.Error()
		res.Response.Code = uint32(err.Code)
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
	if !pretend {
		x.Submit(envelope)
		if async {
			return new(routing.ResponseSubmit), nil
		}
	}

	deliveries, err := chain.NormalizeEnvelope(envelope)
	require.NoErrorf(r, err, "Normalizing envelopes for %s", partition)
	results := make([]*protocol.TransactionStatus, len(deliveries))
	for i, envelope := range deliveries {
		status := new(protocol.TransactionStatus)

		result, err := CheckTx(r.TB, x.Database, x.Executor, envelope)
		if err != nil {
			if err1, ok := err.(*protocol.Error); ok {
				status.Code = err1.Code.GetEnumValue()
			} else if err2, ok := err.(*errors.Error); ok {
				status.Code = err2.Code.GetEnumValue()
				status.Error = err2
			} else {
				status.Code = protocol.ErrorCodeUnknownError.GetEnumValue()
			}
			status.Message = err.Error()
			if status.Code != protocol.ErrorCodeAlreadyDelivered.GetEnumValue() {
				r.Logger.Info("Transaction failed to check",
					"err", err,
					"type", envelope.Transaction.Body.Type(),
					"txn-hash", logging.AsHex(envelope.Transaction.GetHash()).Slice(0, 4),
					"code", status.Code,
					"message", status.Message,
					"principal", envelope.Transaction.Header.Principal)
			}
		}

		status.Result = result
		results[i] = status
	}

	resp := new(routing.ResponseSubmit)
	resp.Data, err = (&protocol.TransactionResultSet{Results: results}).MarshalBinary()
	require.NoError(r, err)

	// If a user transaction fails, the batch fails
	for i, result := range results {
		if result.Code == 0 {
			continue
		}
		if deliveries[i].Transaction.Body.Type().IsUser() {
			resp.Code = uint32(protocol.ErrorCodeUnknownError)
			resp.Log = "One or more user transactions failed"
		}
	}

	return resp, nil
}
