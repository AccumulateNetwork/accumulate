// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/internal"
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

func (x AuthoritySignature) Validate(batch *database.Batch, ctx *SignatureContext) (*protocol.TransactionStatus, error) {
	// An authority signature must be synthetic, so only do enough validation to
	// make sure its valid. Properly produced synthetic messages should _always_
	// be recorded, even if the accounts involved don't exist or are invalid.
	_, err := x.check(batch, ctx)
	return nil, errors.UnknownError.Wrap(err)
}

func (AuthoritySignature) check(batch *database.Batch, ctx *SignatureContext) (*protocol.AuthoritySignature, error) {
	sig, ok := ctx.signature.(*protocol.AuthoritySignature)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid signature type: expected %v, got %v", protocol.SignatureTypeAuthority, ctx.signature.Type())
	}

	if sig.Origin == nil {
		return nil, errors.BadRequest.With("missing origin")
	}
	if sig.TxID == nil {
		return nil, errors.BadRequest.With("missing transaction ID")
	}

	if !ctx.transaction.Body.Type().IsUser() {
		return nil, errors.BadRequest.WithFormat("cannot sign a %v transaction with an authority signature", ctx.transaction.Body.Type())
	}

	// An authority signature MUST NOT be submitted directly
	if !ctx.isWithin(messaging.MessageTypeSynthetic, internal.MessageTypeMessageIsReady, internal.MessageTypePseudoSynthetic) {
		return nil, errors.BadRequest.WithFormat("a non-synthetic message cannot carry an %v signature", ctx.signature.Type())
	}

	return sig, nil
}

func (x AuthoritySignature) Process(batch *database.Batch, ctx *SignatureContext) (_ *protocol.TransactionStatus, err error) {
	batch = batch.Begin(true)
	defer func() { commitOrDiscard(batch, &err) }()

	// Check the message for basic validity
	sig, err := x.check(batch, ctx)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Process the signature (update the transaction status)
	if len(sig.Delegator) > 0 {
		err = x.processDelegated(batch, ctx, sig)
	} else {
		err = x.processDirect(batch, ctx, sig)
	}
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Record the cause
	err = batch.Message(ctx.message.Hash()).Cause().Add(sig.Cause)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store message cause: %w", err)
	}

	// Once a signature has been included in the block, record the signature and
	// its status not matter what, unless there is a system error
	if len(sig.Delegator) > 0 {
		return nil, nil
	}

	// TODO Don't do this unless all authorities are satisfied

	// Process the transaction
	_, err = ctx.callMessageExecutor(batch, &messaging.TransactionMessage{Transaction: ctx.transaction})
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return nil, nil
}

// processDirect processes a direct authority's signature.
func (x AuthoritySignature) processDirect(batch *database.Batch, ctx *SignatureContext, sig *protocol.AuthoritySignature) error {
	hash := ctx.signature.Hash()
	entry := new(database.VoteEntry)
	entry.Authority = sig.Authority
	entry.Hash = *(*[32]byte)(hash)

	// Check for a previous vote
	txn := batch.Account(sig.TxID.Account()).Transaction(sig.TxID.Hash())
	vote := txn.Votes()
	_, err := vote.Find(entry)
	switch {
	case err == nil:
		return errors.Conflict.WithFormat("%v has already voted on %v", sig.Authority, sig.TxID)
	case !errors.Is(err, errors.NotFound):
		return errors.UnknownError.With("load previous vote: %w", err)
	}

	// Verify the transaction has been initiated
	isInit, _, err := transactionIsInitiated(batch, ctx.transaction.ID())
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	if !isInit {
		return errors.NotAllowed.WithFormat("%v has not been initiated", ctx.transaction.ID())
	}

	// Verify the origin is still at the version it voted at
	err = x.checkOriginVersion(batch, ctx, sig)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Verify the signer is authorized to sign for the principal
	err = ctx.signerIsAuthorized(batch, sig)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Add the signature to the principal's chain
	err = txn.RecordHistory(ctx.message)
	if err != nil {
		return errors.UnknownError.WithFormat("record history: %w", err)
	}

	// Record the vote
	err = vote.Add(entry)
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
	keyIndex, _, ok := signer.EntryByDelegate(sig.Authority)
	if !ok {
		return errors.BadRequest.WithFormat("%v is not a delegate of %v", sig.Authority, sig.Delegator[0])
	}

	// Verify the delegator is allowed to sign this type of transaction
	err = ctx.signerCanSignTransaction(batch, ctx.transaction, signer)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Verify the origin is still at the version it voted at
	err = x.checkOriginVersion(batch, ctx, sig)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Add the signature to the transaction's signature set and chain
	entry := &database.SignatureSetEntry{
		KeyIndex: uint64(keyIndex),
		Version:  signer.GetVersion(),
		Hash:     ctx.message.Hash(),
		Path:     sig.Delegator,
	}
	if ctx.GetActiveGlobals().ExecutorVersion.V2BaikonurEnabled() {
		// The path should not include the current signer
		entry.Path = sig.Delegator[1:]
	}
	err = addSignature(batch, ctx, signer, entry)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Send the next authority signature if the signer's authority is satisfied
	return ctx.maybeSendAuthoritySignature(batch, &protocol.AuthoritySignature{
		Authority: signer.GetAuthority(),
		Delegator: sig.Delegator[1:],
	})
}

// checkOriginVersion verifies that the origin of an authority signature is
// still at the version it was at when it produced the signature, if the origin
// is on this partition. The vote was produced when the signature that completed
// it was processed, and the transaction executes afterwards, possibly after
// another transaction in the same block has changed the origin. A key signature
// is refused if its signer version is out of date; the authority's vote is held
// to the same standard when it is consumed (#4614).
func (AuthoritySignature) checkOriginVersion(batch *database.Batch, ctx *SignatureContext, sig *protocol.AuthoritySignature) error {
	if !ctx.GetActiveGlobals().ExecutorVersion.V2TanegashimaEnabled() {
		return nil
	}

	// A signature produced before this was enabled, or by an authority that
	// does not exist (which abstains), does not state a version
	if sig.OriginVersion == 0 {
		return nil
	}

	// The origin can only be checked if it is on this partition
	partition, err := ctx.Executor.Router.RouteAccount(sig.Origin)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	if !strings.EqualFold(partition, ctx.Executor.Describe.PartitionId) {
		return nil
	}

	origin, err := loadSigner(batch, sig.Origin)
	if err != nil {
		return errors.UnknownError.WithFormat("load origin: %w", err)
	}
	if origin.GetVersion() != sig.OriginVersion {
		return errors.BadSignerVersion.WithFormat("invalid version: have %d, got %d", origin.GetVersion(), sig.OriginVersion)
	}
	return nil
}
