// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
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

func (KeySignature) Validate(batch *database.Batch, ctx *SignatureContext) (*protocol.TransactionStatus, error) {
	panic("not implemented")
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
