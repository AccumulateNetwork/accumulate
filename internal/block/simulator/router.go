package simulator

import (
	"context"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type router struct {
	*Simulator
}

func (r router) RouteAccount(account *url.URL) (string, error) {
	if subnet, ok := r.routingOverrides[account.IdentityAccountID32()]; ok {
		return subnet, nil
	}
	return routing.RouteAccount(&config.Network{Subnets: r.Subnets}, account)
}

func (r router) Route(envs ...*protocol.Envelope) (string, error) {
	return routing.RouteEnvelopes(r.RouteAccount, envs...)
}

func (r router) RequestAPIv2(ctx context.Context, subnetId, method string, params, result interface{}) error {
	return r.Subnet(subnetId).API.RequestAPIv2(ctx, method, params, result)
}

func (r router) Submit(ctx context.Context, subnet string, envelope *protocol.Envelope, pretend, async bool) (*routing.ResponseSubmit, error) {
	x := r.Subnet(subnet)
	if !pretend {
		x.Submit(envelope)
		if async {
			return new(routing.ResponseSubmit), nil
		}
	}

	deliveries, err := chain.NormalizeEnvelope(envelope)
	require.NoErrorf(r, err, "Normalizing envelopes for %s", subnet)
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
