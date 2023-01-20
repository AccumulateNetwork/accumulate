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
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ValidateEnvelope verifies that the envelope is valid. It checks the basics,
// like the envelope has signatures and a hash and/or a transaction. It
// validates signatures, ensuring they match the transaction hash, reference a
// signator, etc. And more.
//
// ValidateEnvelope should not modify anything. Right now it updates signer
// timestamps and credits, but that will be moved to ProcessSignature.
func (x *Executor) ValidateEnvelope(batch *database.Batch, delivery *chain.Delivery) (protocol.TransactionResult, error) {
	if x.globals.Active.ExecutorVersion.SignatureAnchoringEnabled() && delivery.Transaction.Body == nil {
		return nil, errors.BadRequest.WithFormat("missing body")
	}

	// If the transaction is borked, the transaction type is probably invalid,
	// so check that first. "Invalid transaction type" is a more useful error
	// than "invalid signature" if the real error is the transaction got borked.
	txnType := delivery.Transaction.Body.Type()
	if txnType == protocol.TransactionTypeSystemWriteData {
		// SystemWriteData transactions are purely internal transactions
		return nil, errors.BadRequest.WithFormat("unsupported transaction type: %v", txnType)
	}
	if txnType != protocol.TransactionTypeRemote {
		_, ok := x.executors[txnType]
		if !ok {
			return nil, errors.BadRequest.WithFormat("unsupported transaction type: %v", txnType)
		}
	}

	// Load the transaction
	_, err := delivery.LoadTransaction(batch)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	if x.globals.Active.ExecutorVersion.SignatureAnchoringEnabled() {
		if delivery.Transaction.Header.Principal == nil {
			return nil, errors.BadRequest.WithFormat("missing principal")
		}
		if delivery.Transaction.Body.Type().IsUser() && delivery.Transaction.Header.Initiator == [32]byte{} {
			return nil, errors.BadRequest.WithFormat("missing initiator")
		}
	}

	// Get a temp status - DO NOT STORE THIS
	status, err := batch.Transaction(delivery.Transaction.GetHash()).Status().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load status: %w", err)
	}

	// Check that the signatures are valid
	for i, signature := range delivery.Signatures {
		err = x.ValidateSignature(batch, delivery, status, signature)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("signature %d: %w", i, err)
		}
	}

	switch {
	case txnType.IsUser():
		err = nil
	case txnType.IsSynthetic(), txnType.IsSystem():
		err = validateSyntheticTransactionSignatures(delivery.Transaction, delivery.Signatures)
	default:
		// Should be unreachable
		return nil, errors.InternalError.WithFormat("transaction type %v is not user, synthetic, or internal", txnType)
	}
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Only validate the transaction when we first receive it
	if delivery.Transaction.Body.Type() == protocol.TransactionTypeRemote {
		return new(protocol.EmptyResult), nil
	}

	// Do not run transaction-specific validation for a synthetic transaction. A
	// synthetic transaction will be rejected by `m.validate` unless it is
	// signed by a BVN and can be proved to have been included in a DN block. If
	// `m.validate` succeeeds, we know the transaction came from a BVN, thus it
	// is safe and reasonable to allow the transaction to be delivered.
	//
	// This is important because if a synthetic transaction is rejected during
	// CheckTx, it does not get recorded. If the synthetic transaction is not
	// recorded, the BVN that sent it and the client that sent the original
	// transaction cannot verify that the synthetic transaction was received.
	if delivery.Transaction.Body.Type().IsSynthetic() {
		return new(protocol.EmptyResult), nil
	}

	// Lite token address => lite identity
	var signerUrl *url.URL
	if delivery.Transaction.Body.Type().IsAnchor() {
		signerUrl = x.Describe.OperatorsPage()
	} else {
		signerUrl = delivery.Signatures[0].GetSigner()
		if key, _, _ := protocol.ParseLiteTokenAddress(signerUrl); key != nil {
			signerUrl = signerUrl.RootIdentity()
		}
	}

	var signer protocol.Signer
	err = batch.Account(signerUrl).GetStateAs(&signer)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load signer: %w", err)
	}

	// Do not validate remote transactions
	if !delivery.Transaction.Header.Principal.LocalTo(signer.GetUrl()) {
		return new(protocol.EmptyResult), nil
	}

	// Load the principal
	principal, err := batch.Account(delivery.Transaction.Header.Principal).GetState()
	switch {
	case err == nil:
		// Ok
	case !errors.Is(err, storage.ErrNotFound):
		return nil, errors.UnknownError.WithFormat("load principal: %w", err)
	case delivery.Transaction.Body.Type().IsUser():
		val, ok := getValidator[chain.PrincipalValidator](x, delivery.Transaction.Body.Type())
		if !ok || !val.AllowMissingPrincipal(delivery.Transaction) {
			return nil, errors.NotFound.WithFormat("missing principal: %v not found", delivery.Transaction.Header.Principal)
		}
	}

	// Set up the state manager
	st := chain.NewStateManager(&x.Describe, &x.globals.Active, batch.Begin(false), principal, delivery.Transaction, x.logger.With("operation", "ValidateEnvelope"))
	defer st.Discard()
	st.Pretend = true

	// Execute the transaction
	executor, ok := x.executors[delivery.Transaction.Body.Type()]
	if !ok {
		return nil, errors.InternalError.WithFormat("missing executor for %v", delivery.Transaction.Body.Type())
	}

	result, err := executor.Validate(st, delivery)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return result, nil
}

func (x *Executor) ValidateSignature(batch *database.Batch, delivery *chain.Delivery, status *protocol.TransactionStatus, signature protocol.Signature) error {
	var md sigExecMetadata
	md.IsInitiator = protocol.SignatureDidInitiate(signature, delivery.Transaction.Header.Initiator[:], nil)
	if !signature.Type().IsSystem() {
		md.Location = signature.RoutingLocation()
	}
	_, err := x.validateSignature(batch, delivery, status, signature, md)
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

	// Stateful validation (mostly for synthetic transactions)
	var signer protocol.Signer2
	var delegate protocol.Signer
	switch signature := signature.(type) {
	case *protocol.ReceiptSignature:
		signer = x.globals.Active.AsSigner(x.Describe.PartitionId)
		err = verifyReceiptSignature(delivery.Transaction, signature, md)

	case *protocol.RemoteSignature:
		return nil, errors.BadRequest.With("a remote signature is not allowed outside of a forwarded transaction")

	case *protocol.SignatureSet:
		return nil, errors.BadRequest.With("a signature set is not allowed outside of a forwarded transaction")

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
		if delivery.Transaction.Body.Type().IsUser() {
			signer, err = x.validateKeySignature(batch, delivery, signature, md, !md.Delegated && delivery.Transaction.Header.Principal.LocalTo(md.Location))
		} else {
			signer, err = x.validatePartitionSignature(signature, delivery.Transaction, status)
		}

		// Basic validation
		if !md.Nested() && !signature.Verify(nil, delivery.Transaction.GetHash()) {
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

func validateSyntheticTransactionSignatures(transaction *protocol.Transaction, signatures []protocol.Signature) error {
	// Validate the synthetic transaction header
	if transaction.Body.Type().IsSynthetic() {
		var missing []string
		if transaction.Header.Source == nil {
			missing = append(missing, "source")
		}
		if transaction.Header.Destination == nil {
			missing = append(missing, "destination")
		}
		if transaction.Header.SequenceNumber == 0 {
			missing = append(missing, "sequence number")
		}
		if len(missing) > 0 {
			return errors.BadRequest.WithFormat("invalid synthetic transaction: missing %s", strings.Join(missing, ", "))
		}
	}

	var gotReceiptSig, gotED25519Sig bool
	for _, sig := range signatures {
		switch sig.(type) {
		case *protocol.ReceiptSignature:
			gotReceiptSig = true

		case *protocol.ED25519Signature, *protocol.LegacyED25519Signature:
			gotED25519Sig = true

		default:
			return errors.BadRequest.WithFormat("synthetic transaction do not support %T signatures", sig)
		}
	}

	if !gotED25519Sig {
		return errors.Unauthenticated.WithFormat("missing ED25519 signature")
	}
	if transaction.Body.Type() == protocol.TransactionTypeDirectoryAnchor || transaction.Body.Type() == protocol.TransactionTypeBlockValidatorAnchor {
		return nil
	}

	if !gotReceiptSig {
		return errors.Unauthenticated.WithFormat("missing synthetic transaction receipt")
	}
	return nil
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
