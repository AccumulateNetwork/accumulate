package simulator

import (
	"context"
	"errors"

	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/rpc/client"
	coretypes "github.com/tendermint/tendermint/rpc/core/types"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/types/api/query"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

type router struct {
	*Simulator
}

func (r router) RouteAccount(account *url.URL) (string, error) {
	return routing.RouteAccount(&config.Network{Subnets: r.Subnets}, account)
}

func (r router) Route(envs ...*protocol.Envelope) (string, error) {
	return routing.RouteEnvelopes(&config.Network{Subnets: r.Subnets}, envs...)
}

func (r router) Query(ctx context.Context, subnet string, rawQuery []byte, opts client.ABCIQueryOptions) (*coretypes.ResultABCIQuery, error) {
	qu := new(query.Query)
	require.NoError(r, qu.UnmarshalBinary(rawQuery))

	x := r.Subnet(subnet)
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

func (r router) Submit(ctx context.Context, subnet string, tx []byte, pretend, async bool) (*routing.ResponseSubmit, error) {
	envelopes, err := transactions.UnmarshalAll(tx)
	require.NoError(r, err)

	x := r.Subnet(subnet)
	if !pretend {
		x.Submit(envelopes...)
		if async {
			return new(routing.ResponseSubmit), nil
		}
	}

	results := make([]*protocol.TransactionStatus, len(envelopes))
	for i, envelope := range envelopes {
		status := new(protocol.TransactionStatus)

		result, err := CheckTx(r.TB, x.Database, x.Executor, envelope)
		if err != nil {
			if err, ok := err.(*protocol.Error); ok {
				status.Code = err.Code.GetEnumValue()
			} else {
				status.Code = protocol.ErrorCodeUnknownError.GetEnumValue()
			}
			status.Message = err.Error()
			if status.Code != protocol.ErrorCodeAlreadyDelivered.GetEnumValue() {
				r.Logger.Info("Transaction failed to check",
					"err", err,
					"type", envelope.Type(),
					"txn-hash", logging.AsHex(envelope.GetTxHash()).Slice(0, 4),
					"env-hash", logging.AsHex(envelope.EnvHash()).Slice(0, 4),
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
		if !envelopes[i].Transaction.Type().IsUser() {
			continue
		}
		resp.Code = uint32(protocol.ErrorCodeUnknownError)
		resp.Log = "One or more user transactions failed"
	}

	return resp, nil
}
