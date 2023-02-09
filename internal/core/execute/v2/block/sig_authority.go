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
	registerSimpleExec[AuthoritySignature](&signatureExecutors, protocol.SignatureTypeAuthority)
}

// AuthoritySignature processes delegated signatures.
type AuthoritySignature struct{}

func (x AuthoritySignature) Process(batch *database.Batch, ctx *SignatureContext) (*protocol.TransactionStatus, error) {
	sig, ok := ctx.signature.(*protocol.AuthoritySignature)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid signature type: expected %v, got %v", protocol.SignatureTypeAuthority, ctx.signature.Type())
	}

	// An authority signature MUST NOT be submitted directly
	if !ctx.isWithin(messaging.MessageTypeSynthetic, internal.MessageTypeMessageIsReady) {
		return protocol.NewErrorStatus(ctx.message.ID(), errors.BadRequest.WithFormat("a non-synthetic message cannot carry an %v signature", ctx.signature.Type())), nil
	}

	// Make sure the block gets recorded
	ctx.state.Set(ctx.message.Hash(), new(chain.ProcessTransactionState))

	batch = batch.Begin(true)
	defer batch.Discard()

	// If the signature has already been processed, return the stored status
	hash := sig.Hash()
	status, err := batch.Transaction(hash).Status().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load status: %w", err)
	}

	if status.Code != 0 {
		return status, nil //nolint:nilerr // False positive
	}

	// Initialize the status
	status.TxID = ctx.message.ID()
	status.Received = ctx.Block.Index

	// Check the message for basic validity
	err = x.check(batch, ctx, sig)
	var err2 *errors.Error
	switch {
	case err == nil:
		// Process the signature (update the transaction status)
		if len(sig.Delegator) > 0 {
			err = x.processDelegated(batch, ctx, sig)
		} else {
			err = x.processDirect(batch, ctx, sig)
		}
		switch {
		case err == nil:
			// Ok
			status.Code = errors.Delivered

		case errors.As(err, &err2) && err2.Code.IsClientError():
			// Record the error
			status.Set(err)

		default:
			// A system error occurred
			return nil, errors.UnknownError.Wrap(err)
		}

	case errors.As(err, &err2) && err2.Code.IsClientError():
		// Record the error
		status.Set(err)

	default:
		// A system error occurred
		return nil, errors.UnknownError.Wrap(err)
	}

	// Once a signature has been included in the block, record the signature and
	// its status not matter what, unless there is a system error
	err = batch.Message2(hash).Main().Put(ctx.message)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store signature: %w", err)
	}

	err = batch.Transaction(hash).Status().Put(status)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store status: %w", err)
	}

	if status.Failed() || len(sig.Delegator) > 0 {
		err = batch.Commit()
		return status, errors.UnknownError.Wrap(err)
	}

	// TODO Don't do this unless all authorities are satisfied

	// Process the transaction
	_, err = ctx.callMessageExecutor(batch, &messaging.UserTransaction{Transaction: ctx.transaction})
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	err = batch.Commit()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return status, nil
}

func (AuthoritySignature) check(batch *database.Batch, ctx *SignatureContext, sig *protocol.AuthoritySignature) error {
	if sig.Signer == nil {
		return errors.BadRequest.With("missing signer")
	}
	if sig.TxID == nil {
		return errors.BadRequest.With("missing transaction ID")
	}

	if !ctx.transaction.Body.Type().IsUser() {
		return errors.BadRequest.WithFormat("cannot sign a %v transaction with an authority signature", ctx.transaction.Body.Type())
	}

	return nil
}

// processDirect processes a direct authority's signature.
func (x AuthoritySignature) processDirect(batch *database.Batch, ctx *SignatureContext, sig *protocol.AuthoritySignature) error {
	// Check for a previous vote
	hash := ctx.signature.Hash()
	principal := batch.Account(sig.TxID.Account())
	vote := principal.Transaction(sig.TxID.Hash()).Vote(sig.Authority)
	_, err := vote.Get()
	switch {
	case err == nil:
		return errors.Conflict.WithFormat("%v has already voted on %v", sig.Authority, sig.TxID)
	case !errors.Is(err, errors.NotFound):
		return errors.UnknownError.With("load previous vote: %w", err)
	}

	// Verify the signer is authorized to sign for the principal
	err = x.signerIsAuthorized(batch, ctx, sig)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Add the signature to the principal's chain
	err = principal.SignatureChain().Inner().AddHash(hash, false)
	if err != nil {
		return errors.UnknownError.WithFormat("add to signature chain: %w", err)
	}

	// Record the vote
	err = vote.Put(*(*[32]byte)(hash))
	if err != nil {
		return errors.UnknownError.With("store vote: %w", err)
	}

	return nil
}

// processDelegated processes a delegated authority's signature.
func (x AuthoritySignature) processDelegated(batch *database.Batch, ctx *SignatureContext, sig *protocol.AuthoritySignature) error {
	// Load the delegator
	signer, err := loadSigner(batch, sig.Delegator[0])
	if err != nil {
		return errors.UnknownError.WithFormat("load delegator: %w", err)
	}

	// Verify that the authority is a delegate
	index, _, ok := signer.EntryByDelegate(sig.Signer)
	if !ok {
		return errors.BadRequest.WithFormat("%v is not a delegate of %v", sig.Signer, sig.Delegator[0])
	}

	// Verify that the delegator can sign this transaction
	err = x.signerCanSign(batch, ctx, sig, signer)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Add the signature to the signer's chain
	hash := ctx.signature.Hash()
	err = batch.Account(signer.GetUrl()).SignatureChain().Inner().AddHash(hash, false)
	if err != nil {
		return errors.UnknownError.WithFormat("add to signature chain: %w", err)
	}

	// Add the signature to the transaction's signature set
	set, err := batch.Transaction(ctx.transaction.GetHash()).SignaturesForSigner(signer)
	if err != nil {
		return errors.UnknownError.WithFormat("load signatures: %w", err)
	}

	_, err = set.Add(uint64(index), ctx.signature)
	if err != nil {
		return errors.UnknownError.WithFormat("add signature: %w", err)
	}

	// If the signer's authority is satisfied, send the next authority signature
	signerAuth := signer.GetAuthority()
	ok, err = ctx.authorityIsSatisfied(batch, signerAuth)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	if !ok {
		return nil
	}

	auth := &protocol.AuthoritySignature{
		Signer:    signer.GetUrl(),
		Authority: signerAuth,
		Vote:      protocol.VoteTypeAccept,
		TxID:      ctx.transaction.ID(),
		Delegator: sig.Delegator[1:],
	}

	// TODO Deduplicate
	ctx.didProduce(
		auth.RoutingLocation(),
		&messaging.UserSignature{
			Signature: auth,
			TxID:      ctx.transaction.ID(),
		},
	)

	return nil
}

func (AuthoritySignature) signerCanSign(batch *database.Batch, ctx *SignatureContext, sig *protocol.AuthoritySignature, signer protocol.Signer) error {
	var md chain.SignatureValidationMetadata
	md.Location = sig.Delegator[0]
	md.Delegated = true
	md.Forwarded = true

	// Delegate to the transaction executor?
	val, ok := getValidator[chain.SignerValidator](ctx.Executor, ctx.transaction.Body.Type())
	if ok {
		fallback, err := val.SignerIsAuthorized(ctx.Executor, batch, ctx.transaction, signer, md)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
		if !fallback {
			return nil
		}
	}

	// Verify the signer is allowed to sign this transaction
	switch signer := signer.(type) {
	case *protocol.LiteIdentity:
		// Otherwise a lite token account is only allowed to sign for itself
		if !signer.Url.Equal(ctx.transaction.Header.Principal.RootIdentity()) {
			return errors.Unauthorized.WithFormat("%v is not authorized to sign transactions for %v", signer.Url, ctx.transaction.Header.Principal)
		}

	case *protocol.KeyPage:
		// Verify that the key page is allowed to sign the transaction
		bit, ok := ctx.transaction.Body.Type().AllowedTransactionBit()
		if ok && signer.TransactionBlacklist.IsSet(bit) {
			return errors.Unauthorized.WithFormat("page %s is not authorized to sign %v", signer.Url, ctx.transaction.Body.Type())
		}

	default:
		// This should never happen
		return errors.InternalError.WithFormat("unknown signer type %v", signer.Type())
	}

	return nil
}

func (AuthoritySignature) signerIsAuthorized(batch *database.Batch, ctx *SignatureContext, sig *protocol.AuthoritySignature) error {
	var md chain.SignatureValidationMetadata
	md.Location = sig.TxID.Account()
	md.Delegated = len(sig.Delegator) > 0
	md.Forwarded = true

	// Delegate to the transaction executor?
	val, ok := getValidator[chain.SignerValidator](ctx.Executor, ctx.transaction.Body.Type())
	if ok {
		fallback, err := val.SignerIsAuthorized(ctx.Executor, batch, ctx.transaction, &protocol.UnknownSigner{Url: sig.Signer}, md)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
		if !fallback {
			return nil
		}
	}

	// Load the principal
	principal, err := batch.Account(ctx.transaction.Header.Principal).GetState()
	if err != nil {
		return errors.UnknownError.WithFormat("load principal: %w", err)
	}

	// Get the principal's account auth
	auth, err := ctx.Executor.GetAccountAuthoritySet(batch, principal)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Page belongs to book => authorized
	_, foundAuthority := auth.GetAuthority(sig.Authority)
	if foundAuthority {
		return nil
	}

	// Authorization is disabled and the transaction type does not force authorization => authorized
	if auth.AuthDisabled() && !ctx.transaction.Body.Type().RequireAuthorization() {
		return nil
	}

	// Authorization is enabled => unauthorized
	// Transaction type forces authorization => unauthorized
	return errors.Unauthorized.WithFormat("%v is not authorized to sign transactions for %v", sig.Signer, principal.GetUrl())
}
