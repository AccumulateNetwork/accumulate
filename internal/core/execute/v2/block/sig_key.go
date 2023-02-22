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
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	registerSimpleExec[KeySignature](&signatureExecutors,
		protocol.SignatureTypeLegacyED25519,
		protocol.SignatureTypeED25519,
		protocol.SignatureTypeRCD1,
		protocol.SignatureTypeBTC,
		protocol.SignatureTypeBTCLegacy,
		protocol.SignatureTypeETH,

		// TODO Remove?
		protocol.SignatureTypeDelegated,

		// TODO Remove
		protocol.SignatureTypeRemote,
		protocol.SignatureTypeSet,
	)
}

// KeySignature processes key signatures.
type KeySignature struct{}

func (x KeySignature) Process(batch *database.Batch, ctx *SignatureContext) (*protocol.TransactionStatus, error) {
	status := new(protocol.TransactionStatus)
	status.TxID = ctx.message.ID()
	status.Received = ctx.Block.Index

	// Process the signature
	s, err := ctx.Executor.processSignature2(batch, &chain.Delivery{
		Transaction: ctx.transaction,
		Forwarded:   ctx.isWithin(internal.MessageTypeForwardedMessage),
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

	// If this is the initiator signature (and it's not forwarded) and the
	// transaction requests additional authorities, send out signature requests
	if !ctx.isWithin(internal.MessageTypeForwardedMessage) &&
		protocol.SignatureDidInitiate(ctx.signature, ctx.transaction.Header.Initiator[:], nil) {
		for _, auth := range ctx.transaction.GetAdditionalAuthorities() {
			msg := new(messaging.SignatureRequest)
			msg.Authority = auth
			msg.Cause = ctx.message.ID()
			msg.TxID = ctx.transaction.ID()
			ctx.didProduce(msg.Authority, msg)
		}
	}

	// DELETE Temporarily, don't produce authority signatures for forwarded signatures
	if ctx.isWithin(internal.MessageTypeForwardedMessage) {
		return status, nil
	}

	// Collect delegators and the inner signature
	var delegators []*url.URL
	sig := ctx.signature
	for {
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
	txst, err := batch.Transaction(ctx.transaction.GetHash()).Status().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load status: %w", err)
	}

	signerAuth := sig.GetSigner()
	if key, _ := protocol.ParseLiteIdentity(signerAuth); key != nil {
		// Ok
	} else if key, _, _ := protocol.ParseLiteTokenAddress(signerAuth); key != nil {
		signerAuth = signerAuth.RootIdentity()
	} else {
		signerAuth = signerAuth.Identity()
	}

	ok, err := ctx.Executor.AuthorityIsSatisfied(batch, ctx.transaction, txst, signerAuth)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if ok {
		// TODO Add payment if initiator
		sig := &protocol.AuthoritySignature{
			Signer:    sig.GetSigner(),
			Authority: signerAuth,
			Vote:      protocol.VoteTypeAccept,
			TxID:      ctx.transaction.ID(),
			Initiator: protocol.SignatureDidInitiate(ctx.signature, ctx.transaction.Header.Initiator[:], nil),
			Delegator: delegators,
		}

		// TODO Deduplicate
		ctx.didProduce(
			sig.RoutingLocation(),
			&messaging.UserSignature{
				Signature: sig,
				TxID:      ctx.transaction.ID(),
			},
		)
	}

	return status, nil
}
