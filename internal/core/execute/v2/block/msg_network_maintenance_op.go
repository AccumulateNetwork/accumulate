// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/internal"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	registerSimpleExec[NetworkMaintenanceOp](&messageExecutors, internal.MessageTypeNetworkMaintenanceOp)
}

// NetworkMaintenanceOp constructs a transaction for the network update and queues it
// for processing.
type NetworkMaintenanceOp struct{}

func (NetworkMaintenanceOp) Validate(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	return nil, errors.InternalError.With("invalid attempt to validate an internal message")
}

func (x NetworkMaintenanceOp) Process(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	msg, ok := ctx.message.(*internal.NetworkMaintenanceOp)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", internal.MessageTypeNetworkMaintenanceOp, ctx.message.Type())
	}

	batch = batch.Begin(true)
	defer batch.Discard()

	// Record the cause (the network maintenance transaction) on the main chain
	h := msg.Cause.Hash()
	err := batch.Account(msg.Operation.ID().Account()).MainChain().Inner().AddEntry(h[:], false)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("update main chain: %w", err)
	}

	// Execute the operation
	switch op := msg.Operation.(type) {
	case *protocol.PendingTransactionGCOperation:
		err = x.processPendingTransactionGC(batch, ctx, op)
	default:
		err = errors.InternalError.WithFormat("unknown operation %v", op.Type())
	}
	switch {
	case err == nil:
		// Ok
	case errors.Code(err).IsClientError():
		// Fail gracefully (do not commit changes). Use the cause as the ID
		// since the operation is never recorded on its own.
		return protocol.NewErrorStatus(msg.Cause, err), nil
	default:
		return nil, errors.UnknownError.Wrap(err)
	}

	err = batch.Commit()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return nil, nil
}

func (NetworkMaintenanceOp) processPendingTransactionGC(batch *database.Batch, ctx *MessageContext, op *protocol.PendingTransactionGCOperation) error {
	account := batch.Account(op.Account)
	pending, err := account.Pending().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load pending: %w", err)
	}
	for _, id := range pending {
		// If the transaction is actually pending, leave it alone
		st, err := batch.Transaction2(id.Hash()).Status().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load transaction status: %w", err)
		}
		if st.Pending() {
			continue
		}

		// Remove it from the pending list
		err = account.Pending().Remove(id)
		if err != nil {
			return errors.UnknownError.WithFormat("clear pending: %w", err)
		}

		// If the account is a key book or page, clear the active signature set
		main, err := account.Main().Get()
		switch {
		case err == nil:
			// Ok
		case errors.Is(err, errors.NotFound):
			// This may be unnecessary, but there have been cases in the past
			// where an account didn't exist but had a pending list
			continue
		default:
			return errors.UnknownError.WithFormat("load account: %w", err)
		}

		var auth protocol.Authority
		switch main := main.(type) {
		case protocol.Authority:
			auth = main
		case protocol.Signer:
			err = batch.Account(main.GetAuthority()).Main().GetAs(&auth)
			if err != nil {
				return errors.UnknownError.WithFormat("load authority: %w", err)
			}
		default:
			continue
		}

		// Clear the active signature set of every signer
		for _, signer := range auth.GetSigners() {
			for _, id := range pending {
				err = batch.
					Account(signer).
					Transaction(id.Hash()).
					Signatures().Put(nil)
				if err != nil {
					return errors.UnknownError.WithFormat("clear the signature set: %w", err)
				}
			}
		}
	}
	return nil
}
