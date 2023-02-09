// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/internal"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	registerSimpleExec[SignatureRequest](&messageExecutors, messaging.MessageTypeSignatureRequest)
}

// SignatureRequest lists a transaction as pending on an authority.
type SignatureRequest struct{}

func (x SignatureRequest) Process(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	req, ok := ctx.message.(*messaging.SignatureRequest)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeSignatureRequest, ctx.message.Type())
	}

	// Must be synthetic
	if !ctx.isWithin(messaging.MessageTypeSynthetic, internal.MessageTypeMessageIsReady) {
		return protocol.NewErrorStatus(req.ID(), errors.BadRequest.WithFormat("cannot execute %v outside of a synthetic message", req.Type())), nil
	}

	// Basic validation
	if req.Authority == nil {
		return protocol.NewErrorStatus(req.ID(), errors.BadRequest.With("missing authority")), nil
	}
	if req.TxID == nil {
		return protocol.NewErrorStatus(req.ID(), errors.BadRequest.With("missing transaction ID")), nil
	}
	if req.Cause == nil {
		return protocol.NewErrorStatus(req.ID(), errors.BadRequest.With("missing cause")), nil
	}

	// Add a transaction state to ensure the block gets recorded
	ctx.state.Set(req.Hash(), new(chain.ProcessTransactionState))

	batch = batch.Begin(true)
	defer batch.Discard()

	// If the message has already been processed, return its recorded status
	status, err := batch.Transaction2(req.Hash()).Status().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load status: %w", err)
	}
	if status.Delivered() {
		return status, nil
	}

	// Check if the transaction has already been recorded
	pending := batch.Account(req.Authority).Pending()
	_, err = pending.Index(req.TxID)
	switch {
	case err == nil:
		// Already recorded as pending

	case errors.Is(err, errors.NotFound):
		err = x.record(batch, ctx, req)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}

	default:
		return nil, errors.UnknownError.WithFormat("load pending: %w", err)
	}

	// Record the message
	err = batch.Message(req.Hash()).Main().Put(req)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store message: %w", err)
	}

	err = batch.Message(req.Hash()).Cause().Add(req.TxID)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("add cause: %w", err)
	}

	// Record the status
	status = new(protocol.TransactionStatus)
	status.Received = ctx.Block.Index
	status.TxID = req.ID()
	status.Code = errors.Delivered
	h := req.Hash()
	err = batch.Transaction(h[:]).Status().Put(status)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store status: %w", err)
	}

	err = batch.Commit()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return status, nil
}

func (SignatureRequest) record(batch *database.Batch, ctx *MessageContext, req *messaging.SignatureRequest) error {
	// Record the transaction as pending
	err := batch.Account(req.Authority).Pending().Add(req.TxID)
	if err != nil {
		return errors.UnknownError.WithFormat("add pending: %w", err)
	}

	// Get the transaction from the message bundle (or the database) and store
	// it into the database. This is a hack to make the account's BPT entry
	// work. The fact that the BPT entry is hashing the binary-marshalled value
	// of SigOrTxn is positively awful, but that's what it's doing.
	//
	// FIXME... but not today
	txn, err := ctx.getTransaction(batch, req.TxID.Hash())
	if err != nil {
		return errors.UnknownError.WithFormat("load transaction: %w", err)
	}
	err = batch.Message(req.TxID.Hash()).Main().Put(&messaging.UserTransaction{Transaction: txn})
	if err != nil {
		return errors.UnknownError.WithFormat("store transaction: %w", err)
	}

	// If the 'authority' is the principal, send a signature request to each authority
	if !req.Authority.Equal(req.TxID.Account()) {
		return nil
	}

	principal, err := batch.Account(req.TxID.Account()).Main().Get()
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, errors.NotFound):
		// Ok but don't send out any notices
		return nil
	default:
		return errors.UnknownError.WithFormat("load principal: %w", err)
	}

	auth, err := ctx.Executor.GetAccountAuthoritySet(batch, principal)
	if err != nil {
		return errors.UnknownError.WithFormat("get authority set: %w", err)
	}

	for _, auth := range auth.Authorities {
		// Don't send another request to the current account
		if auth.Url.Equal(req.TxID.Account()) {
			continue
		}

		msg := new(messaging.SignatureRequest)
		msg.Authority = auth.Url
		msg.Cause = ctx.message.ID()
		msg.TxID = req.TxID
		ctx.didProduce(msg.Authority, msg)
	}

	return nil
}
