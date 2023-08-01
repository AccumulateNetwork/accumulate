// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"context"
	"fmt"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Router struct {
	tree       *routing.RouteTree
	logger     logging.OptionalLogger
	partitions map[string]*Partition
	overrides  map[[32]byte]string
}

// ResponseSubmit is the response from a call to Submit.
type ResponseSubmit struct {
	Code         uint32
	Data         []byte
	Log          string
	Info         string
	Codespace    string
	MempoolError string
}

func newRouter(logger log.Logger, partitions map[string]*Partition) *Router {
	r := new(Router)
	r.logger.Set(logger, "module", "router")
	r.partitions = partitions
	r.overrides = map[[32]byte]string{}
	return r
}

func (r *Router) willChangeGlobals(e events.WillChangeGlobals) error {
	tree, err := routing.NewRouteTree(e.New.Routing)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	r.tree = tree
	return nil
}

func (r *Router) SetRoute(account *url.URL, partition string) {
	if _, ok := r.partitions[partition]; !ok {
		panic(fmt.Errorf("%s is not a partition", partition))
	}
	if partition == "" {
		delete(r.overrides, account.IdentityAccountID32())
	} else {
		r.overrides[account.IdentityAccountID32()] = partition
	}
}

func (r *Router) RouteAccount(account *url.URL) (string, error) {
	if part, ok := r.overrides[account.IdentityAccountID32()]; ok {
		return part, nil
	}
	if r.tree == nil {
		return "", errors.InternalError.With("the routing table has not been initialized")
	}
	if protocol.IsUnknown(account) {
		return "", errors.BadRequest.With("URL is unknown, cannot route")
	}
	return r.tree.Route(account)
}

func (r *Router) Route(envs ...*messaging.Envelope) (string, error) {
	return routing.RouteEnvelopes(r.RouteAccount, envs...)
}

func (r *Router) Submit(ctx context.Context, partition string, envelope *messaging.Envelope, pretend, async bool) (*ResponseSubmit, error) {
	if async {
		go func() {
			_, err := r.Submit(ctx, partition, envelope, pretend, false)
			switch {
			case err == nil:
				// Ok

			case errors.Is(err, errors.FatalError):
				panic(fmt.Errorf("fatal error during async submit: %w", err))

			default:
				r.logger.Error("Error during async submit", "partition", partition, "pretend", pretend, "error", err)
			}
		}()
		return new(ResponseSubmit), nil
	}

	p, ok := r.partitions[partition]
	if !ok {
		return nil, errors.BadRequest.WithFormat("%s is not a partition", partition)
	}

	messages, err := envelope.Normalize()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("submit: %w", err)
	}

	msgById := map[[32]byte]messaging.Message{}
	for _, msg := range messages {
		msgById[msg.ID().Hash()] = msg
	}

	resp := new(ResponseSubmit)
	results, err := p.Submit(envelope, pretend)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// This should more or less preserve the original behavior of failing if a
	// user transaction fails
	for _, st := range results {
		if !st.Failed() {
			continue
		}

		// If there's no message with the given ID, fail
		msg, ok := msgById[st.TxID.Hash()]
		if !ok {
			goto failed
		}

		switch msg := msg.(type) {
		case *messaging.TransactionMessage:
			// If a user transaction fails, fail
			if msg.Transaction.Body.Type().IsUser() {
				goto failed
			}

		case *messaging.SignatureMessage:
			// If there's no message with the transaction ID, or that message is
			// not a transaction, or its a user transaction, fail
			msg2, ok := msgById[msg.TxID.Hash()]
			if !ok {
				goto failed
			}
			txn, ok := msg2.(*messaging.TransactionMessage)
			if !ok {
				goto failed
			}
			if txn.Transaction.Body.Type().IsUser() {
				goto failed
			}
		}

		continue

	failed:
		resp.Code = uint32(protocol.ErrorCodeUnknownError)
		resp.Log = "One or more user transactions failed"
		break
	}

	resp.Data, err = (&protocol.TransactionResultSet{Results: results}).MarshalBinary()
	if err != nil {
		return nil, errors.FatalError.WithFormat("marshal results: %w", err)
	}

	return resp, nil
}
