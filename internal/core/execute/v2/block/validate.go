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
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (x *Executor) validateSignature2(batch *database.Batch, delivery *chain.Delivery, signature protocol.Signature) error {
	status, err := batch.Transaction(delivery.Transaction.GetHash()).Status().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load status: %w", err)
	}

	var md sigExecMetadata
	md.IsInitiator = protocol.SignatureDidInitiate(signature, delivery.Transaction.Header.Initiator[:], nil)
	if !signature.Type().IsSystem() {
		md.Location = signature.RoutingLocation()
	}
	_, err = x.validateSignature(batch, delivery, status, signature, md)
	return errors.UnknownError.Wrap(err)
}

func (x *Executor) validateSignature(batch *database.Batch, delivery *chain.Delivery, status *protocol.TransactionStatus, signature protocol.Signature, md sigExecMetadata) (protocol.Signer2, error) {
	err := x.checkRouting(delivery, signature)
	if err != nil {
		return nil, err
	}

	// Verify that the initiator signature matches the transaction
	err = validateInitialSignature(delivery.Transaction, signature, md)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	var signer protocol.Signer2
	var delegate protocol.Signer
	switch signature := signature.(type) {
	case *protocol.DelegatedSignature:
		if !md.Nested() {
			// Limit delegation depth
			for i, sig := 1, signature.Signature; ; i++ {
				if i > protocol.DelegationDepthLimit {
					return nil, errors.BadRequest.WithFormat("delegated signature exceeded the depth limit (%d)", protocol.DelegationDepthLimit)
				}
				if del, ok := sig.(*protocol.DelegatedSignature); ok {
					sig = del.Signature
				} else {
					break
				}
			}
		}

		s, err := x.validateSignature(batch, delivery, status, signature.Signature, md.SetDelegated())
		if err != nil {
			return nil, errors.UnknownError.WithFormat("validate delegated signature: %w", err)
		}
		if !md.Nested() && !signature.Verify(signature.Metadata().Hash(), delivery.Transaction.GetHash()) {
			return nil, errors.BadRequest.WithFormat("invalid signature")
		}
		if !signature.Delegator.LocalTo(md.Location) {
			return nil, nil
		}

		// Validate the delegator
		signer, err = x.validateSigner(batch, delivery.Transaction, signature.Delegator, md.Location, false, md)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}

		// Verify delegation
		var ok bool
		delegate, ok = s.(protocol.Signer)
		if !ok {
			// The only non-account signer is the network signer which is only
			// used for system signatures, so this should never happen
			return nil, errors.InternalError.WithFormat("delegate is not an account")
		}
		_, _, ok = signer.EntryByDelegate(delegate.GetUrl())
		if !ok {
			return nil, errors.Unauthorized.WithFormat("%v is not authorized to sign for %v", delegate.GetUrl(), signature.Delegator)
		}

	case protocol.KeySignature:
		hash := delivery.Transaction.GetHash()
		if delivery.Transaction.Body.Type().IsUser() {
			signer, err = x.validateKeySignature(batch, delivery, signature, md, !md.Delegated && delivery.Transaction.Header.Principal.LocalTo(md.Location))
		} else {
			h := delivery.Sequence.Hash()
			hash = h[:]
			signer, err = x.validatePartitionSignature(signature, delivery.Transaction, delivery.Sequence, status)
		}

		// Basic validation
		if !md.Nested() && !signature.Verify(nil, hash) {
			return nil, errors.BadRequest.With("invalid")
		}

	default:
		return nil, errors.BadRequest.WithFormat("unknown signature type %v", signature.Type())
	}
	if err != nil {
		return nil, errors.Unauthenticated.Wrap(err)
	}

	return signer, nil
}

// checkRouting verifies that the signature was routed to the correct partition.
func (x *Executor) checkRouting(delivery *chain.Delivery, signature protocol.Signature) error {
	if signature.Type().IsSystem() {
		return nil
	}

	if delivery.Transaction.Body.Type().IsUser() {
		partition, err := x.Router.RouteAccount(signature.RoutingLocation())
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
		if !strings.EqualFold(partition, x.Describe.PartitionId) {
			return errors.BadRequest.WithFormat("signature submitted to %v instead of %v", x.Describe.PartitionId, partition)
		}
	}

	return nil
}
