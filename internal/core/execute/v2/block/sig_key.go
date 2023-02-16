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
	registerSimpleExec[KeySignature](&signatureExecutors,
		protocol.SignatureTypeDelegated,

		protocol.SignatureTypeLegacyED25519,
		protocol.SignatureTypeED25519,
		protocol.SignatureTypeRCD1,
		protocol.SignatureTypeBTC,
		protocol.SignatureTypeBTCLegacy,
		protocol.SignatureTypeETH,
	)
}

// KeySignature processes key signatures.
type KeySignature struct{}

func (x KeySignature) Validate(batch *database.Batch, ctx *SignatureContext) (*protocol.TransactionStatus, error) {
	err := x.check(batch, ctx)
	return nil, errors.UnknownError.Wrap(err)
}

func (x KeySignature) check(batch *database.Batch, ctx *SignatureContext) error {
	isig, ok := ctx.signature.(protocol.UserSignature)
	if !ok {
		return errors.BadRequest.WithFormat("invalid user signature: expected delegated or key, got %v", ctx.signature.Type())
	}

	// Unwrap delegated signatures
	keySig, delegators, err := x.unwrapDelegated(ctx)
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
	if !isig.Verify(ctx.signature.Hash(), ctx.transaction.GetHash()) {
		return errors.Unauthenticated.WithFormat("signature does not match transaction")
	}

	// The initiator must have a timestamp
	if protocol.SignatureDidInitiate(ctx.signature, ctx.transaction.Header.Initiator[:], nil) {
		if keySig.GetTimestamp() == 0 {
			return errors.BadTimestamp.WithFormat("initial signature does not have a timestamp")
		}
	}

	// Load the signer and verify the signature against it
	signer, err := x.verifySigner(batch, ctx, keySig)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Ensure the signer has _some_ credits
	minimumFee := protocol.FeeSignature.GetEnumValue()
	if !signer.CanDebitCredits(minimumFee) {
		return errors.InsufficientCredits.WithFormat("insufficient credits: have %s, want %s",
			protocol.FormatAmount(signer.GetCreditBalance(), protocol.CreditPrecisionPower),
			protocol.FormatAmount(minimumFee, protocol.CreditPrecisionPower))
	}

	_ = delegators
	return nil
}

func (KeySignature) unwrapDelegated(ctx *SignatureContext) (protocol.KeySignature, []*url.URL, error) {
	// Collect delegators and the inner signature
	var delegators []*url.URL
	var keySig protocol.KeySignature
	for sig := ctx.signature; keySig == nil; {
		switch s := sig.(type) {
		case *protocol.DelegatedSignature:
			delegators = append(delegators, s.Delegator)
			sig = s.Signature
		case protocol.KeySignature:
			keySig = s
		default:
			return nil, nil, errors.BadRequest.WithFormat("invalid user signature: expected delegated or key, got %v", s.Type())
		}

		// Limit delegation depth
		if len(delegators) > protocol.DelegationDepthLimit {
			return nil, nil, errors.BadRequest.WithFormat("delegated signature exceeded the depth limit (%d)", protocol.DelegationDepthLimit)
		}
	}

	// Reverse the list since the structure nesting is effectively inverted
	for i, n := 0, len(delegators); i < n/2; i++ {
		j := n - 1 - i
		delegators[i], delegators[j] = delegators[j], delegators[i]
	}

	return keySig, delegators, nil
}

func (KeySignature) verifySigner(batch *database.Batch, ctx *SignatureContext, keySig protocol.KeySignature) (protocol.Signer, error) {
	// If the user specifies a lite token address, convert it to a lite
	// identity
	signerUrl := ctx.signature.GetSigner()
	if key, _, _ := protocol.ParseLiteTokenAddress(signerUrl); key != nil {
		signerUrl = signerUrl.RootIdentity()
	}

	signer, err := loadSigner(batch, signerUrl)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Verify that the signer is authorized
	val, ok := getValidator[chain.SignerValidator](ctx.Executor, ctx.transaction.Body.Type())
	var fallback bool
	if ok {
		var md chain.SignatureValidationMetadata
		md.Location = signerUrl
		fallback, err = val.SignerIsAuthorized(ctx.Executor, batch, ctx.transaction, signer, md)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}
	if !ok || fallback {
		err = ctx.Executor.SignerIsAuthorized(batch, ctx.transaction, signer, false)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	// Check the signer version
	if ctx.transaction.Body.Type().IsUser() && keySig.GetSignerVersion() != signer.GetVersion() {
		return nil, errors.BadSignerVersion.WithFormat("invalid version: have %d, got %d", signer.GetVersion(), keySig.GetSignerVersion())
	}

	// Find the key entry
	_, entry, ok := signer.EntryByKeyHash(keySig.GetPublicKeyHash())
	if !ok {
		return nil, errors.Unauthorized.With("key does not belong to signer")
	}

	// Check the timestamp
	if keySig.GetTimestamp() != 0 && entry.GetLastUsedOn() >= keySig.GetTimestamp() {
		return nil, errors.BadTimestamp.WithFormat("invalid timestamp: have %d, got %d", entry.GetLastUsedOn(), keySig.GetTimestamp())
	}

	return signer, nil
}

func (x KeySignature) Process(batch *database.Batch, ctx *SignatureContext) (*protocol.TransactionStatus, error) {
	status := new(protocol.TransactionStatus)
	status.TxID = ctx.message.ID()
	status.Received = ctx.Block.Index

	// Process the signature
	s, err := ctx.Executor.processSignature2(batch, &chain.Delivery{
		Transaction: ctx.transaction,
	}, ctx.signature)
	ctx.Block.State.MergeSignature(s)
	if err == nil {
		status.Code = errors.Delivered
	} else {
		status.Set(err)
	}
	if status.Failed() {
		return status, nil
	}

	// If this is the initiator signature
	if protocol.SignatureDidInitiate(ctx.signature, ctx.transaction.Header.Initiator[:], nil) {
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

	// Collect delegators and the inner signature
	var delegators []*url.URL
	for sig := ctx.signature; ; {
		del, ok := sig.(*protocol.DelegatedSignature)
		if !ok {
			break
		}
		delegators = append(delegators, del.Delegator)
		sig = del.Signature
	}
	for i, n := 0, len(delegators); i < n/2; i++ {
		j := n - 1 - i
		delegators[i], delegators[j] = delegators[j], delegators[i]
	}

	// If the signer's authority is satisfied, send the next authority signature
	signerAuth := ctx.getAuthority()
	ok, err := ctx.authorityIsSatisfied(batch, signerAuth)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if !ok {
		return status, nil
	}

	// TODO Add payment if initiator
	auth := &protocol.AuthoritySignature{
		Signer:    ctx.getSigner(),
		Authority: signerAuth,
		Vote:      protocol.VoteTypeAccept,
		TxID:      ctx.transaction.ID(),
		Delegator: delegators,
	}

	// TODO Deduplicate
	ctx.didProduce(
		auth.RoutingLocation(),
		&messaging.UserSignature{
			Signature: auth,
			TxID:      ctx.transaction.ID(),
		},
	)

	// Send a payment
	didInit, err := ctx.didInitiate(batch)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	ctx.didProduce(
		ctx.transaction.Header.Principal,
		&messaging.CreditPayment{
			// TODO Add amount paid
			Payer:     ctx.getSigner(),
			TxID:      ctx.transaction.ID(),
			Cause:     ctx.message.ID(),
			Initiator: didInit,
		},
	)

	return status, nil
}
