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
	"gitlab.com/accumulatenetwork/accumulate/internal/node/abci"
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

func (r router) Route(envs ...*protocol.Envelope) (string, error) {
	return routing.RouteEnvelopes(r.RouteAccount, envs...)
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

	messages := make([]messaging.Message, len(deliveries))
	for i, delivery := range deliveries {
		messages[i] = &messaging.LegacyMessage{
			Transaction: delivery.Transaction,
			Signatures:  delivery.Signatures,
		}
	}

	batch := x.Database.Begin(false)
	defer batch.Discard()
	results := abci.ValidateEnvelopeSet((*execute.ExecutorV1)(x.Executor), batch, messages)
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
