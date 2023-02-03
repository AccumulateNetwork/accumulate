// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"context"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	execute "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/multi"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
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

func (r router) Route(envs ...*messaging.Envelope) (string, error) {
	return routing.RouteEnvelopes(r.RouteAccount, envs...)
}

func (r router) RequestAPIv2(ctx context.Context, partitionId, method string, params, result interface{}) error {
	return r.Partition(partitionId).API.RequestAPIv2(ctx, method, params, result)
}

func (r router) Submit(ctx context.Context, partition string, envelope *messaging.Envelope, pretend, async bool) (*routing.ResponseSubmit, error) {
	x := r.Partition(partition)
	deliveries := x.Submit(pretend, envelope)

	if !pretend && async {
		return new(routing.ResponseSubmit), nil
	}

	var messages []messaging.Message
	for _, delivery := range deliveries {
		messages = append(messages, &messaging.UserTransaction{Transaction: delivery.Transaction})
		for _, sig := range delivery.Signatures {
			messages = append(messages, &messaging.UserSignature{Signature: sig, TransactionHash: delivery.Transaction.ID().Hash()})
		}
	}

	batch := x.Database.Begin(false)
	defer batch.Discard()
	results, err := (*execute.ExecutorV1)(x.Executor).Validate(batch, messages)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	for _, s := range results {
		if !s.Failed() {
			continue
		}

		r.Logger.Info("Transaction failed to check",
			"err", s.Error,
			// "type", d.Transaction.Body.Type(),
			// "txn-hash", logging.AsHex(d.Transaction.GetHash()).Slice(0, 4),
			"code", s.Code,
			// "principal", d.Transaction.Header.Principal,
		)
	}

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
