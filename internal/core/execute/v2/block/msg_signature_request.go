// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/internal"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
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

func (x SignatureRequest) Validate(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	_, err := x.check(batch, ctx)
	return nil, errors.UnknownError.Wrap(err)
}

func (SignatureRequest) check(batch *database.Batch, ctx *MessageContext) (*messaging.SignatureRequest, error) {
	req, ok := ctx.message.(*messaging.SignatureRequest)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeSignatureRequest, ctx.message.Type())
	}

	// Must be synthetic
	if !ctx.isWithin(messaging.MessageTypeSynthetic, internal.MessageTypeMessageIsReady, internal.MessageTypePseudoSynthetic) {
		return nil, errors.BadRequest.WithFormat("cannot execute %v outside of a synthetic message", req.Type())
	}

	// Basic validation
	if req.Authority == nil {
		return nil, errors.BadRequest.With("missing authority")
	}
	if req.TxID == nil {
		return nil, errors.BadRequest.With("missing transaction ID")
	}
	if req.Cause == nil {
		return nil, errors.BadRequest.With("missing cause")
	}

	return req, nil
}

func (x SignatureRequest) Process(batch *database.Batch, ctx *MessageContext) (_ *protocol.TransactionStatus, err error) {
	batch = batch.Begin(true)
	defer func() { commitOrDiscard(batch, &err) }()

	// Check if the message has already been processed
	status, err := ctx.checkStatus(batch)
	if err != nil || status.Delivered() {
		return status, err
	}

	// Add a transaction state to ensure the block gets recorded
	ctx.state.Set(ctx.message.Hash(), new(chain.ProcessTransactionState))

	// Process the message
	req, err := x.check(batch, ctx)
	if err == nil {
		err = x.process(batch, ctx, req)
	}

	// Record the message and its status
	err = ctx.recordMessageAndStatus(batch, status, errors.Delivered, err)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return status, nil
}

func (x SignatureRequest) process(batch *database.Batch, ctx *MessageContext, req *messaging.SignatureRequest) error {
	// If the 'authority' is not the principal, verify it exists and is an
	// authority
	var invalid bool
	if ctx.GetActiveGlobals().ExecutorVersion.V2BaikonurEnabled() && !req.Authority.Equal(req.TxID.Account()) {
		ok, err := x.authorityIsValid(batch, ctx, req)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
		if !ok {
			invalid = true
		}
	}

	if !invalid {
		// Check if the transaction has already been recorded
		pending := batch.Account(req.Authority).Pending()
		_, err := pending.Index(req.TxID)
		switch {
		case err == nil:
			// Already recorded as pending
			return nil

		case errors.Is(err, errors.NotFound):
			// Ok

		default:
			return errors.UnknownError.WithFormat("load pending: %w", err)
		}

		// Record the transaction as pending
		err = batch.Account(req.Authority).Pending().Add(req.TxID)
		if err != nil {
			return errors.UnknownError.WithFormat("add pending: %w", err)
		}
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
	err = batch.Message(req.TxID.Hash()).Main().Put(&messaging.TransactionMessage{Transaction: txn})
	if err != nil {
		return errors.UnknownError.WithFormat("store transaction: %w", err)
	}

	// Add the message to the authority's signature chain
	err = batch.Account(req.Authority).
		Transaction(req.TxID.Hash()).
		RecordHistory(ctx.message)
	if err != nil {
		return errors.UnknownError.WithFormat("record history: %w", err)
	}

	// If the 'authority' is the principal, send a signature request to each authority
	if req.Authority.Equal(req.TxID.Account()) {
		return x.requestSignaturesFromAuthorities(batch, ctx, req)
	}
	return nil
}

func (SignatureRequest) authorityIsValid(batch *database.Batch, ctx *MessageContext, req *messaging.SignatureRequest) (bool, error) {
	var message string
	account, err := batch.Account(req.Authority).Main().Get()
	switch {
	case err == nil:
		if _, ok := account.(protocol.Authority); ok {
			return true, nil
		}
		message = req.Authority.String() + " is not an authority"

	case errors.Is(err, errors.NotFound):
		message = req.Authority.String() + " does not exist"

	default:
		// Unknown error
		return true, errors.UnknownError.WithFormat("load authority: %w", err)
	}

	// The invalid authority abstains from the transaction
	authSig := &protocol.AuthoritySignature{
		Authority: req.Authority,
		Origin:    req.Authority,
		TxID:      req.TxID,
		Cause:     req.ID(),
		Vote:      protocol.VoteTypeAbstain,
		Memo:      message,
	}

	err = ctx.didProduce(
		batch,
		authSig.RoutingLocation(),
		&messaging.SignatureMessage{
			Signature: authSig,
			TxID:      req.TxID,
		},
	)
	return false, errors.UnknownError.Wrap(err)
}

func (SignatureRequest) requestSignaturesFromAuthorities(batch *database.Batch, ctx *MessageContext, req *messaging.SignatureRequest) error {
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

	auth, err := ctx.getAccountAuthoritySet(batch, principal)
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
		msg.Cause = req.ID()
		msg.TxID = req.TxID
		err = ctx.didProduce(batch, msg.Authority, msg)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	return nil
}
