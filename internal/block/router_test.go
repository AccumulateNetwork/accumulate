package block_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/rpc/client"
	core "github.com/tendermint/tendermint/rpc/core/types"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/types/api/query"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

type TestRouter struct {
	testing.TB
	Logger    log.Logger
	Network   *config.Network
	Executors map[string]*RouterExecEntry
}

type RouterExecEntry struct {
	mu        sync.Mutex
	submitted []*protocol.Envelope

	Database *database.Database
	Executor *block.Executor
}

func (r *RouterExecEntry) Submit(envelopes ...*protocol.Envelope) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.submitted = append(r.submitted, envelopes...)
}

func (r *RouterExecEntry) TakeSubmitted() []*protocol.Envelope {
	r.mu.Lock()
	defer r.mu.Unlock()
	submitted := r.submitted
	r.submitted = nil
	return submitted
}

func (r *TestRouter) Subnet(id string) *RouterExecEntry {
	e, ok := r.Executors[id]
	require.Truef(r, ok, "Unknown subnet %q", id)
	return e
}

func (r *TestRouter) QueryUrl(url *url.URL, req queryRequest, prove bool) interface{} {
	subnet, err := routing.RouteAccount(r.Network, url)
	require.NoError(r, err)
	x := r.Subnet(subnet)
	return Query(r, x.Database, x.Executor, req, prove)
}

func (r *TestRouter) InitChain() {
	for _, subnet := range r.Network.Subnets {
		x := r.Subnet(subnet.ID)
		InitChain(r, x.Database, x.Executor)
	}
	r.WaitForGovernor()
}

func (r *TestRouter) WaitForGovernor() {
	for _, subnet := range r.Network.Subnets {
		r.Subnet(subnet.ID).Executor.WaitForGovernor()
	}
}

func (r *TestRouter) ExecuteBlock(envelopes ...*protocol.Envelope) {
	for _, envelope := range envelopes {
		subnet, err := routing.RouteEnvelopes(r.Network, envelope)
		require.NoError(r, err)
		x := r.Subnet(subnet)
		x.Submit(envelope)
	}

	wg := new(sync.WaitGroup)
	wg.Add(len(r.Network.Subnets))

	for _, subnet := range r.Network.Subnets {
		go func(x *RouterExecEntry) {
			defer wg.Done()
			submitted := x.TakeSubmitted()
			ExecuteBlock(r, x.Database, x.Executor, nil, submitted...)
		}(r.Subnet(subnet.ID))
	}

	wg.Wait()
}

func (r *TestRouter) RouteAccount(account *url.URL) (string, error) {
	return routing.RouteAccount(r.Network, account)
}

func (r *TestRouter) Route(envs ...*protocol.Envelope) (string, error) {
	return routing.RouteEnvelopes(r.Network, envs...)
}

func (r *TestRouter) Query(ctx context.Context, subnet string, rawQuery []byte, opts client.ABCIQueryOptions) (*core.ResultABCIQuery, error) {
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
		res := new(core.ResultABCIQuery)
		res.Response.Info = err.Error()
		res.Response.Code = uint32(protocol.ErrorCodeNotFound)
		return res, nil

	default:
		r.Logf("Query failed: %v", err)
		res := new(core.ResultABCIQuery)
		res.Response.Info = err.Error()
		res.Response.Code = uint32(err.Code)
		return res, nil
	}

	res := new(core.ResultABCIQuery)
	res.Response.Code = uint32(protocol.ErrorCodeOK)
	res.Response.Key, res.Response.Value = k, v
	return res, nil
}

func (r *TestRouter) Submit(ctx context.Context, subnet string, tx []byte, pretend, async bool) (*routing.ResponseSubmit, error) {
	envelopes, err := transactions.UnmarshalAll(tx)
	require.NoError(r, err)

	for _, env := range envelopes {
		if env.Type() == protocol.TransactionTypeSyntheticAnchor && subnet != "BlockValidator" {
			print("")
		}
	}

	x := r.Subnet(subnet)
	if !pretend {
		x.Submit(envelopes...)
		if async {
			return new(routing.ResponseSubmit), nil
		}
	}

	results := make([]*protocol.TransactionStatus, len(envelopes))
	for i, env := range envelopes {
		status := new(protocol.TransactionStatus)

		if env.Type() == protocol.TransactionTypeSyntheticAnchor && subnet != "BlockValidator" {
			print("")
		}

		result, err := CheckTx(r.TB, x.Database, x.Executor, env)
		if err != nil {
			r.Logf("Failed to Check: %v", err)
			if err, ok := err.(*protocol.Error); ok {
				status.Code = err.Code.GetEnumValue()
			} else {
				status.Code = protocol.ErrorCodeUnknownError.GetEnumValue()
			}
			status.Message = err.Error()
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
