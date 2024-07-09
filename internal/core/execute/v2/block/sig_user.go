// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"math/big"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	// Delegated signatures
	registerSimpleExec[UserSignature](&signatureExecutors,
		protocol.SignatureTypeDelegated,
	)

	// Regular signatures
	registerSimpleExec[UserSignature](&signatureExecutors,
		protocol.SignatureTypeLegacyED25519,
		protocol.SignatureTypeED25519,
		protocol.SignatureTypeRCD1,
		protocol.SignatureTypeBTC,
		protocol.SignatureTypeBTCLegacy,
		protocol.SignatureTypeETH,
	)

	// Vandenberg: RSA, ECDSA, and EIP-712 signatures
	registerConditionalExec[UserSignature](&signatureExecutors,
		func(ctx *SignatureContext) bool { return ctx.GetActiveGlobals().ExecutorVersion.V2VandenbergEnabled() },
		protocol.SignatureTypeRsaSha256,
		protocol.SignatureTypeEcdsaSha256,
		protocol.SignatureTypeEip712TypedData,
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

	verifySignature := protocol.VerifyUserSignature
	if !ctx.Executor.globals.Active.ExecutorVersion.V2BaikonurEnabled() {
		//if ethereum RSV based verification is not enabled, then revert to old methods
		verifySignature = protocol.VerifyUserSignatureV1
	}

	// Verify the signature signs the transaction
	if !verifySignature(sig, ctx.transaction) {
		return errors.Unauthenticated.WithFormat("invalid signature")
	}

	// Check if the signature initiates the transaction
	err = x.checkInit(ctx)
	if err != nil {
		return errors.UnknownError.Wrap(err)
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

	// A lite identity may not require additional authorities
	if ctx.GetActiveGlobals().ExecutorVersion.V2VandenbergEnabled() &&
		ctx.signer.Type() == protocol.AccountTypeLiteIdentity &&
		len(ctx.transaction.Header.Authorities) > 0 &&
		ctx.isInitiator {
		return errors.BadRequest.WithFormat("a transaction initiated by a lite identity cannot require additional authorities")
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

func (UserSignature) checkInit(ctx *userSigContext) error {
	var initMerkle bool
	ctx.isInitiator, initMerkle = protocol.SignatureDidInitiate(ctx.signature, ctx.transaction.Header.Initiator[:], nil)

	switch {
	case ctx.isInitiator && ctx.keySig.GetTimestamp() == 0:
		// The initiator must have a timestamp
		return errors.BadTimestamp.WithFormat("initial signature does not have a timestamp")

	case ctx.keySig.GetVote() == protocol.VoteTypeSuggest && ctx.GetActiveGlobals().ExecutorVersion.V2VandenbergEnabled():
		// A suggestion must be the initiator and must use a plain initiator
		// hash
		if !ctx.isInitiator {
			return errors.BadRequest.With("suggestions cannot be secondary signatures")
		}
		if initMerkle {
			return errors.BadRequest.With("suggestions cannot use Merkle initiator hashes")
		}

	case ctx.isInitiator && ctx.keySig.GetVote() != protocol.VoteTypeAccept:
		// The initiator must accept the transaction. The Merkle style of
		// initiator hash does not incorporate the vote type so accepting
		// non-accept votes is potentially problematic. Also, is there any
		// reason to initiate and reject or abstain?
		return errors.BadRequest.WithFormat("initial signature cannot be a %v vote", ctx.keySig.GetVote())
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

	// Verify the signer is allowed to sign
	err = ctx.signerCanSignTransaction(batch, ctx.transaction, ctx.signer)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Check the signer version
	if ctx.transaction.Body.Type().IsUser() && ctx.keySig.GetSignerVersion() != ctx.signer.GetVersion() {
		return errors.BadSignerVersion.WithFormat("invalid version: have %d, got %d", ctx.signer.GetVersion(), ctx.keySig.GetSignerVersion())
	}

	// Find the key entry
	var ok bool
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
func (x UserSignature) verifyCanPay(batch *database.Batch, ctx *userSigContext) error {
	// Operators don't have to pay when signing directly with the operators page
	if protocol.DnUrl().LocalTo(ctx.signer.GetUrl()) {
		return nil
	}

	// Check for errors, such as payload is too big
	var err error
	ctx.fee, err = x.computeSignerFee(ctx)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// In the case of a locally signed, non-delegated, non-remote add or burn
	// credits transaction, verify the account has a sufficient balance
	isLocal := ctx.signer.GetUrl().LocalTo(ctx.transaction.Header.Principal)
	isDirect := ctx.signature.Type() != protocol.SignatureTypeDelegated

	if isLocal && isDirect {
		switch body := ctx.transaction.Body.(type) {
		case *protocol.AddCredits:
			return x.verifyTokenBalance(batch, ctx, &body.Amount)

		case *protocol.BurnCredits:
			return x.verifyCreditBalance(batch, ctx, body.Amount)
		}
	}

	// In all other cases, verify the signer has at least 0.01 credits
	minFee := protocol.FeeSignature.GetEnumValue()
	if !ctx.signer.CanDebitCredits(minFee) {
		return errors.InsufficientCredits.WithFormat(
			"insufficient credits: have %s, want %s",
			protocol.FormatAmount(ctx.signer.GetCreditBalance(), protocol.CreditPrecisionPower),
			protocol.FormatAmount(minFee, protocol.CreditPrecisionPower))
	}
	return nil
}

func (UserSignature) verifyTokenBalance(batch *database.Batch, ctx *userSigContext, amount *big.Int) error {
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
	if !tokens.CanDebitTokens(amount) {
		return errors.InsufficientBalance.WithFormat(
			"insufficient tokens: have %s, want %s",
			protocol.FormatBigAmount(tokens.TokenBalance(), protocol.AcmePrecisionPower),
			protocol.FormatBigAmount(amount, protocol.AcmePrecisionPower))
	}

	return nil
}
func (UserSignature) verifyCreditBalance(batch *database.Batch, ctx *userSigContext, amount uint64) error {
	// Load the principal
	account, err := batch.Account(ctx.transaction.Header.Principal).Main().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load transaction principal: %w", err)
	}

	// Verify it is a credit account
	var credits protocol.AccountWithCredits
	switch account := account.(type) {
	case protocol.AccountWithCredits:
		credits = account
	case *protocol.LiteTokenAccount:
		err := batch.Account(account.Url.RootIdentity()).Main().GetAs(&account)
		if err != nil {
			return errors.UnknownError.WithFormat("load lite identity: %w", err)
		}
	default:
		return errors.BadRequest.WithFormat("invalid principal: want a signer, got %v", account.Type())
	}

	// Verify it has a sufficient balance
	if !credits.CanDebitCredits(amount) {
		return errors.InsufficientBalance.WithFormat(
			"insufficient credits: have %s, want %s",
			protocol.FormatAmount(credits.GetCreditBalance(), protocol.CreditPrecisionPower),
			protocol.FormatAmount(amount, protocol.CreditPrecisionPower))
	}

	return nil
}

// Process processes a signature.
func (x UserSignature) Process(batch *database.Batch, ctx *SignatureContext) (_ *protocol.TransactionStatus, err error) {
	batch = batch.Begin(true)
	defer func() { commitOrDiscard(batch, &err) }()

	// Process the signature
	ctx2 := &userSigContext{SignatureContext: ctx}
	err = x.check(batch, ctx2)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	err = x.process(batch, ctx2)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Request additional signatures
	err = x.sendSignatureRequests(batch, ctx2)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Send the credit payment
	err = x.sendCreditPayment(batch, ctx2)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Don't send an authority signature if it's a suggestion
	if ctx2.keySig.GetVote() == protocol.VoteTypeSuggest && ctx.GetActiveGlobals().ExecutorVersion.V2VandenbergEnabled() {
		return nil, nil
	}

	// Send the authority signature if the authority is ready
	err = ctx.maybeSendAuthoritySignature(batch, &protocol.AuthoritySignature{
		Authority: ctx.getAuthority(),
		Delegator: ctx2.delegators,
	})
	return nil, errors.UnknownError.Wrap(err)
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

	if ctx.keySig.GetVote() == protocol.VoteTypeSuggest && ctx.GetActiveGlobals().ExecutorVersion.V2VandenbergEnabled() {
		// If it's a signature, add it to the chain only
		err = batch.Account(ctx.getSigner()).
			Transaction(ctx.transaction.ID().Hash()).
			RecordHistory(ctx.message)
	} else {
		// Otherwise, add it to  the signature set and chain
		err = addSignature(batch, ctx.SignatureContext, ctx.signer, &database.SignatureSetEntry{
			KeyIndex: uint64(ctx.keyIndex),
			Version:  ctx.keySig.GetSignerVersion(),
			Hash:     ctx.message.Hash(),
			Path:     ctx.delegators,
		})
	}
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	return nil
}

// sendSignatureRequests sends signature requests so that the transaction
// will appear on the appropriate pending lists.
func (UserSignature) sendSignatureRequests(batch *database.Batch, ctx *userSigContext) error {
	// If this is the initiator signature
	if !ctx.isInitiator {
		return nil
	}

	// Send a notice to the principal, unless principal is a lite identity
	_, err := protocol.ParseLiteAddress(ctx.transaction.Header.Principal)
	isLite := err == nil && ctx.GetActiveGlobals().ExecutorVersion.V2VandenbergEnabled()
	if !isLite {
		msg := new(messaging.SignatureRequest)
		msg.Authority = ctx.transaction.Header.Principal
		msg.Cause = ctx.message.ID()
		msg.TxID = ctx.transaction.ID()
		err := ctx.didProduce(batch, msg.Authority, msg)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	// If transaction requests additional authorities, send out signature requests
	authorities := ctx.transaction.GetAdditionalAuthorities()
	if ctx.GetActiveGlobals().ExecutorVersion.V2BaikonurEnabled() {
		authorities = append(authorities, ctx.transaction.Header.Authorities...)
	}
	for _, auth := range authorities {
		msg := new(messaging.SignatureRequest)
		msg.Authority = auth
		msg.Cause = ctx.message.ID()
		msg.TxID = ctx.transaction.ID()
		err := ctx.didProduce(batch, msg.Authority, msg)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	return nil
}

// sendCreditPayment sends the principal a notice that the signer paid and (if
// the signature is the initiator) the transaction was initiated.
func (UserSignature) sendCreditPayment(batch *database.Batch, ctx *userSigContext) error {
	didInit, _ := protocol.SignatureDidInitiate(ctx.signature, ctx.transaction.Header.Initiator[:], nil)

	// Don't send a payment if we didn't pay anything. Since we don't (yet)
	// support anyone besides the initiator paying, this means only the
	// initiator should send a payment.
	if !didInit {
		return nil
	}

	return ctx.didProduce(
		batch,
		ctx.transaction.Header.Principal,
		&messaging.CreditPayment{
			Paid:      ctx.fee,
			Payer:     ctx.getSigner(),
			TxID:      ctx.transaction.ID(),
			Cause:     ctx.message.ID(),
			Initiator: didInit,
		},
	)
}

// computeSignerFee computes the fee that will be charged to the signer.
//
// If the signature is the initial signature, the fee is the base transaction
// fee + signature data surcharge + transaction data surcharge.
//
// Otherwise, the fee is the base signature fee + signature data surcharge.
func (UserSignature) computeSignerFee(ctx *userSigContext) (protocol.Fee, error) {
	// Don't charge fees for internal administrative functions
	signer := ctx.signature.GetSigner()
	_, isBvn := protocol.ParsePartitionUrl(signer)
	if isBvn || protocol.IsDnUrl(signer) {
		return 0, nil
	}

	// Compute the signature fee
	fee, err := ctx.GetActiveGlobals().Globals.FeeSchedule.ComputeSignatureFee(ctx.signature)
	if err != nil {
		return 0, errors.UnknownError.Wrap(err)
	}

	// Only charge the transaction fee for the initial signature
	if !ctx.isInitiator {
		return fee, nil
	}

	// Add the transaction fee for the initial signature
	txnFee, err := ctx.GetActiveGlobals().Globals.FeeSchedule.ComputeTransactionFee(ctx.transaction)
	if err != nil {
		return 0, errors.UnknownError.Wrap(err)
	}

	// Subtract the base signature fee, but not the oversize surcharge if there is one
	fee += txnFee - protocol.FeeSignature
	return fee, nil
}
