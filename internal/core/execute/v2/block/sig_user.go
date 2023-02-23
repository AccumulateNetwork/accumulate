// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	registerSimpleExec[UserSignature](&signatureExecutors,
		protocol.SignatureTypeDelegated,

		protocol.SignatureTypeLegacyED25519,
		protocol.SignatureTypeED25519,
		protocol.SignatureTypeRCD1,
		protocol.SignatureTypeBTC,
		protocol.SignatureTypeBTCLegacy,
		protocol.SignatureTypeETH,
	)
}

// UserSignature processes user signatures.
type UserSignature struct{}

// userSigContext collects all the bits of data needed to validate and execute a
// signature.
type userSigContext struct {
	*SignatureContext

	// keySig is the innermost signature, which must be a key signature. If the
	// submitted signature is a delegated signature, keySig is the key signature
	// within. If the submitted signature is not delegated, keySig _is_ the
	// submitted signature.
	keySig protocol.KeySignature

	// signer is the signer of the key signature.
	signer protocol.Signer

	// delegators is the delegation chain/path.
	delegators []*url.URL

	// keyEntry is the entry of the signer used to create the key signature.
	keyEntry protocol.KeyEntry

	// keyIndex is the index of the key entry.
	keyIndex int

	// isInitiator is true if the submitted signature is the transaction
	// initiator.
	isInitiator bool

	// fee is the full fee that will be charged when the signature is executed.
	fee protocol.Fee
}

// Validate validates a signature.
func (x UserSignature) Validate(batch *database.Batch, ctx *SignatureContext) (*protocol.TransactionStatus, error) {
	err := x.check(batch, &userSigContext{SignatureContext: ctx})
	return nil, errors.UnknownError.Wrap(err)
}

// check validates the signature and collects all the pieces needed for
// execution.
func (x UserSignature) check(batch *database.Batch, ctx *userSigContext) error {
	sig, ok := ctx.signature.(protocol.UserSignature)
	if !ok {
		return errors.BadRequest.WithFormat("invalid user signature: expected delegated or key, got %v", ctx.signature.Type())
	}

	// Unwrap delegated signatures
	err := x.unwrapDelegated(ctx)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Check routing
	partition, err := ctx.Executor.Router.RouteAccount(ctx.signature.GetSigner())
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	if !strings.EqualFold(partition, ctx.Executor.Describe.PartitionId) {
		return errors.BadRequest.WithFormat("signature submitted to %v instead of %v", ctx.Executor.Describe.PartitionId, partition)
	}

	// Verify the signature signs the transaction
	if !protocol.VerifyUserSignature(sig, ctx.transaction.GetHash()) {
		return errors.Unauthenticated.WithFormat("invalid signature")
	}

	// The initiator must have a timestamp
	ctx.isInitiator = protocol.SignatureDidInitiate(ctx.signature, ctx.transaction.Header.Initiator[:], nil)
	if ctx.isInitiator {
		if ctx.keySig.GetTimestamp() == 0 {
			return errors.BadTimestamp.WithFormat("initial signature does not have a timestamp")
		}
	}

	// Load the signer and verify the signature against it
	err = x.verifySigner(batch, ctx)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Verify the signer can pay
	err = x.verifyCanPay(batch, ctx)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	return nil
}

// unwrapDelegated unwraps a delegated signature, returning the key signature
// within and the list of delegators. unwrapDelegated returns an error if the
// innermost signature is not a key signature, or if the delegation depth
// exceeds the limit.
func (UserSignature) unwrapDelegated(ctx *userSigContext) error {
	// Collect delegators and the inner signature
	for sig := ctx.signature; ctx.keySig == nil; {
		switch s := sig.(type) {
		case *protocol.DelegatedSignature:
			ctx.delegators = append(ctx.delegators, s.Delegator)
			sig = s.Signature
		case protocol.KeySignature:
			ctx.keySig = s
		default:
			return errors.BadRequest.WithFormat("invalid user signature: expected delegated or key, got %v", s.Type())
		}

		// Limit delegation depth
		if len(ctx.delegators) > protocol.DelegationDepthLimit {
			return errors.BadRequest.WithFormat("delegated signature exceeded the depth limit (%d)", protocol.DelegationDepthLimit)
		}
	}

	// Reverse the list since the structure nesting is effectively inverted
	for i, n := 0, len(ctx.delegators); i < n/2; i++ {
		j := n - 1 - i
		ctx.delegators[i], ctx.delegators[j] = ctx.delegators[j], ctx.delegators[i]
	}

	return nil
}

// verifySigner loads the key signature's signer and checks it against the
// signature and the transaction.
func (UserSignature) verifySigner(batch *database.Batch, ctx *userSigContext) error {
	// If the user specifies a lite token address, convert it to a lite
	// identity
	signerUrl := ctx.signature.GetSigner()
	if key, _, _ := protocol.ParseLiteTokenAddress(signerUrl); key != nil {
		signerUrl = signerUrl.RootIdentity()
	}

	var err error
	ctx.signer, err = loadSigner(batch, signerUrl)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Verify that the signer is authorized
	val, ok := getValidator[chain.SignerValidator](ctx.Executor, ctx.transaction.Body.Type())
	var fallback bool
	if ok {
		var md chain.SignatureValidationMetadata
		md.Location = signerUrl
		md.Delegated = ctx.signature.Type() == protocol.SignatureTypeDelegated
		fallback, err = val.SignerIsAuthorized(ctx.Executor, batch, ctx.transaction, ctx.signer, md)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}
	if !ok || fallback {
		err = ctx.Executor.SignerIsAuthorized(batch, ctx.transaction, ctx.signer, false)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	// Check the signer version
	if ctx.transaction.Body.Type().IsUser() && ctx.keySig.GetSignerVersion() != ctx.signer.GetVersion() {
		return errors.BadSignerVersion.WithFormat("invalid version: have %d, got %d", ctx.signer.GetVersion(), ctx.keySig.GetSignerVersion())
	}

	// Find the key entry
	ctx.keyIndex, ctx.keyEntry, ok = ctx.signer.EntryByKeyHash(ctx.keySig.GetPublicKeyHash())
	if !ok {
		return errors.Unauthorized.With("key does not belong to signer")
	}

	// Check the timestamp
	if ctx.keySig.GetTimestamp() != 0 && ctx.keyEntry.GetLastUsedOn() >= ctx.keySig.GetTimestamp() {
		return errors.BadTimestamp.WithFormat("invalid timestamp: have %d, got %d", ctx.keyEntry.GetLastUsedOn(), ctx.keySig.GetTimestamp())
	}

	return nil
}

// verifyCanPay verifies the signer can be charged for recording the signature,
// and verifies the signature and transaction do not exceed certain limits.
func (UserSignature) verifyCanPay(batch *database.Batch, ctx *userSigContext) error {
	// Operators don't have to pay when signing directly with the operators page
	if protocol.DnUrl().LocalTo(ctx.signer.GetUrl()) {
		return nil
	}

	// Check for errors, such as payload is too big
	var err error
	ctx.fee, err = ctx.Executor.computeSignerFee(ctx.transaction, ctx.signature, ctx.isInitiator)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// In the case of a locally signed, non-delegated, non-remote add credits
	// transaction, verify the token account has a sufficient balance
	body, isAddCredits := ctx.transaction.Body.(*protocol.AddCredits)
	isLocal := ctx.signer.GetUrl().LocalTo(ctx.transaction.Header.Principal)
	isDirect := ctx.signature.Type() != protocol.SignatureTypeDelegated
	checkBalance := isAddCredits && isLocal && isDirect

	// In all other cases, verify the signer has at least 0.01 credits
	if !checkBalance {
		minFee := protocol.FeeSignature.GetEnumValue()
		if !ctx.signer.CanDebitCredits(minFee) {
			return errors.InsufficientCredits.WithFormat(
				"insufficient credits: have %s, want %s",
				protocol.FormatAmount(ctx.signer.GetCreditBalance(), protocol.CreditPrecisionPower),
				protocol.FormatAmount(minFee, protocol.CreditPrecisionPower))
		}
		return nil
	}

	// Load the principal
	account, err := batch.Account(ctx.transaction.Header.Principal).Main().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load transaction principal: %w", err)
	}

	// Verify it is a token account
	tokens, ok := account.(protocol.AccountWithTokens)
	if !ok {
		return errors.NotAllowed.WithFormat("%v is not a token account", ctx.transaction.Header.Principal)
	}

	// Verify it is an ACME token account
	if !protocol.AcmeUrl().Equal(tokens.GetTokenUrl()) {
		return errors.NotAllowed.WithFormat("invalid token account: have %v, want %v", tokens.GetTokenUrl(), protocol.AcmeUrl())
	}

	// Verify it has a sufficient balance
	if !tokens.CanDebitTokens(&body.Amount) {
		return errors.InsufficientBalance.WithFormat(
			"insufficient tokens: have %s, want %s",
			protocol.FormatBigAmount(tokens.TokenBalance(), protocol.AcmePrecisionPower),
			protocol.FormatBigAmount(&body.Amount, protocol.AcmePrecisionPower))
	}

	return nil
}

// Process processes a signature.
func (x UserSignature) Process(batch *database.Batch, ctx *SignatureContext) (*protocol.TransactionStatus, error) {
	status := new(protocol.TransactionStatus)
	status.TxID = ctx.message.ID()
	status.Received = ctx.Block.Index

	batch = batch.Begin(true)
	defer batch.Discard()

	// Make sure the block is recorded
	ctx.Block.State.MergeSignature(&ProcessSignatureState{})

	// Process the signature
	ctx2 := &userSigContext{SignatureContext: ctx}
	err := x.check(batch, ctx2)
	if err == nil {
		err = x.process(batch, ctx2)
	}
	switch {
	case err == nil:
		status.Code = errors.Delivered

	case errors.Code(err).IsClientError():
		status.Set(err)
		return status, nil

	default:
		return nil, errors.UnknownError.Wrap(err)
	}

	// Request additional signatures
	x.sendSignatureRequests(batch, ctx2)

	// Verify the signer's authority is satisfied
	ok, err := ctx.authorityIsSatisfied(batch, ctx.getAuthority())
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if !ok {
		err = batch.Commit()
		return status, errors.UnknownError.Wrap(err)
	}

	// Send the authority signature
	x.sendAuthoritySignature(batch, ctx2)

	// Send the credit payment
	err = x.sendCreditPayment(batch, ctx2)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	err = batch.Commit()
	return status, errors.UnknownError.Wrap(err)
}

// process processes the signature.
func (UserSignature) process(batch *database.Batch, ctx *userSigContext) error {
	// Charge the fee. If that fails, attempt to charge 0.01 for recording the
	// failure.
	if !ctx.signer.DebitCredits(ctx.fee.AsUInt64()) {
		// At this point there's nothing we _can_ do if the signer has no
		// credits
		_ = ctx.signer.DebitCredits(protocol.FeeSignature.AsUInt64())

		return errors.InsufficientCredits.WithFormat("%v has insufficient credits: have %s, want %s", ctx.signer.GetUrl(),
			protocol.FormatAmount(ctx.signer.GetCreditBalance(), protocol.CreditPrecisionPower),
			protocol.FormatAmount(ctx.fee.AsUInt64(), protocol.CreditPrecisionPower))
	}

	// Update the timestamp
	if ctx.keySig.GetTimestamp() != 0 {
		ctx.keyEntry.SetLastUsedOn(ctx.keySig.GetTimestamp())
	}

	// Store changes to the signer
	err := batch.Account(ctx.signer.GetUrl()).Main().Put(ctx.signer)
	if err != nil {
		return errors.UnknownError.WithFormat("store signer: %w", err)
	}

	// Add the signature to the signer's chain
	chain, err := batch.Account(ctx.signer.GetUrl()).SignatureChain().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load chain: %w", err)
	}
	err = chain.AddEntry(ctx.signature.Hash(), true)
	if err != nil {
		return errors.UnknownError.WithFormat("store chain: %w", err)
	}

	// Add the signature to the transaction's signature set
	sigSet, err := batch.Transaction(ctx.transaction.GetHash()).SignaturesForSigner(ctx.signer)
	if err != nil {
		return errors.UnknownError.WithFormat("load signatures: %w", err)
	}

	_, err = sigSet.Add(uint64(ctx.keyIndex), ctx.signature)
	if err != nil {
		return errors.UnknownError.WithFormat("store signature: %w", err)
	}

	return nil
}

// sendSignatureRequests sends signature requests so that the transaction
// will appear on the appropriate pending lists.
func (UserSignature) sendSignatureRequests(batch *database.Batch, ctx *userSigContext) {
	// If this is the initiator signature
	if !ctx.isInitiator {
		return
	}

	// Send a notice to the principal
	msg := new(messaging.SignatureRequest)
	msg.Authority = ctx.transaction.Header.Principal
	msg.Cause = ctx.message.ID()
	msg.TxID = ctx.transaction.ID()
	ctx.didProduce(msg.Authority, msg)

	// If transaction requests additional authorities, send out signature requests
	for _, auth := range ctx.transaction.GetAdditionalAuthorities() {
		msg := new(messaging.SignatureRequest)
		msg.Authority = auth
		msg.Cause = ctx.message.ID()
		msg.TxID = ctx.transaction.ID()
		ctx.didProduce(msg.Authority, msg)
	}
}

// sendAuthoritySignature sends the authority signature for the signer.
func (UserSignature) sendAuthoritySignature(batch *database.Batch, ctx *userSigContext) {
	auth := &protocol.AuthoritySignature{
		Signer:    ctx.getSigner(),
		Authority: ctx.getAuthority(),
		Vote:      protocol.VoteTypeAccept,
		TxID:      ctx.transaction.ID(),
		Delegator: ctx.delegators,
	}

	// TODO Deduplicate
	ctx.didProduce(
		auth.RoutingLocation(),
		&messaging.SignatureMessage{
			Signature: auth,
			TxID:      ctx.transaction.ID(),
		},
	)
}

// sendCreditPayment sends the principal a notice that the signer paid and (if
// the signature is the initiator) the transaction was initiated.
func (UserSignature) sendCreditPayment(batch *database.Batch, ctx *userSigContext) error {
	didInit, err := ctx.didInitiate(batch)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	if !didInit {
		return nil
	}

	ctx.didProduce(
		ctx.transaction.Header.Principal,
		&messaging.CreditPayment{
			Paid:      ctx.fee,
			Payer:     ctx.getSigner(),
			TxID:      ctx.transaction.ID(),
			Cause:     ctx.message.ID(),
			Initiator: didInit,
		},
	)
	return nil
}