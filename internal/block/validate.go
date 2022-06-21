package block

import (
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

// ValidateEnvelope verifies that the envelope is valid. It checks the basics,
// like the envelope has signatures and a hash and/or a transaction. It
// validates signatures, ensuring they match the transaction hash, reference a
// signator, etc. And more.
//
// ValidateEnvelope should not modify anything. Right now it updates signer
// timestamps and credits, but that will be moved to ProcessSignature.
func (x *Executor) ValidateEnvelope(batch *database.Batch, delivery *chain.Delivery) (protocol.TransactionResult, error) {
	// If the transaction is borked, the transaction type is probably invalid,
	// so check that first. "Invalid transaction type" is a more useful error
	// than "invalid signature" if the real error is the transaction got borked.
	txnType := delivery.Transaction.Body.Type()
	if txnType == protocol.TransactionTypeSystemWriteData {
		// SystemWriteData transactions are purely internal transactions
		return nil, errors.Format(errors.StatusBadRequest, "unsupported transaction type: %v", txnType)
	}
	if txnType != protocol.TransactionTypeRemote {
		_, ok := x.executors[txnType]
		if !ok {
			return nil, errors.Format(errors.StatusBadRequest, "unsupported transaction type: %v", txnType)
		}
	}

	// Load the transaction
	_, err := delivery.LoadTransaction(batch)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}

	// Check that the signatures are valid
	for i, signature := range delivery.Signatures {
		var md sigExecMetadata
		md.Initiated = i > 0 || txnType == protocol.TransactionTypeRemote
		if !signature.Type().IsSystem() {
			md.Location = signature.RoutingLocation()
		}
		_, err := x.validateSignature(batch, delivery, signature, md)
		if err != nil {
			return nil, errors.Format(errors.StatusUnknown, "signature %d: %w", i, err)
		}
	}

	switch {
	case txnType.IsUser():
		err = nil
	case txnType.IsSynthetic(), txnType.IsSystem():
		err = validateSyntheticTransactionSignatures(delivery.Transaction, delivery.Signatures)
	default:
		// Should be unreachable
		return nil, errors.Format(errors.StatusInternalError, "transaction type %v is not user, synthetic, or internal", txnType)
	}
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
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

	// Load the first signer
	firstSig := delivery.Signatures[0]
	if _, ok := firstSig.(*protocol.ReceiptSignature); ok {
		return nil, errors.Format(errors.StatusBadRequest, "invalid transaction: initiated by receipt signature")
	}

	// Lite token address => lite identity
	var signerUrl *url.URL
	if _, ok := firstSig.(*protocol.SyntheticSignature); ok {
		signerUrl = x.Describe.OperatorsPage()
	} else {
		signerUrl = firstSig.GetSigner()
		if key, _, _ := protocol.ParseLiteTokenAddress(signerUrl); key != nil {
			signerUrl = signerUrl.RootIdentity()
		}
	}

	var signer protocol.Signer
	err = batch.Account(signerUrl).GetStateAs(&signer)
	if err != nil {
		return nil, errors.Format(errors.StatusUnknown, "load signer: %w", err)
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
		return nil, errors.Format(errors.StatusUnknown, "load principal: %w", err)
	case delivery.Transaction.Body.Type().IsUser():
		val, ok := getValidator[chain.PrincipalValidator](x, delivery.Transaction.Body.Type())
		if !ok || !val.AllowMissingPrincipal(delivery.Transaction) {
			return nil, errors.NotFound("missing principal: %v not found", delivery.Transaction.Header.Principal)
		}
	}

	// Set up the state manager
	st := chain.NewStateManager(&x.Describe, &x.globals.Active, batch.Begin(false), principal, delivery.Transaction, x.logger.With("operation", "ValidateEnvelope"))
	defer st.Discard()
	st.Pretend = true

	// Execute the transaction
	executor, ok := x.executors[delivery.Transaction.Body.Type()]
	if !ok {
		return nil, errors.Format(errors.StatusInternalError, "missing executor for %v", delivery.Transaction.Body.Type())
	}

	result, err := executor.Validate(st, delivery)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}

	return result, nil
}

func (x *Executor) validateSignature(batch *database.Batch, delivery *chain.Delivery, signature protocol.Signature, md sigExecMetadata) (protocol.Signer, error) {
	err := x.checkRouting(delivery, signature)
	if err != nil {
		return nil, err
	}

	// Verify that the initiator signature matches the transaction
	err = validateInitialSignature(delivery.Transaction, signature, md)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}

	// Stateful validation (mostly for synthetic transactions)
	var signer, delegate protocol.Signer
	switch signature := signature.(type) {
	case *protocol.SyntheticSignature:
		err = verifySyntheticSignature(&x.Describe, batch, delivery.Transaction, signature, md)

	case *protocol.ReceiptSignature:
		err = verifyReceiptSignature(delivery.Transaction, signature, md)

	case *protocol.RemoteSignature:
		return nil, errors.New(errors.StatusBadRequest, "a remote signature is not allowed outside of a forwarded transaction")

	case *protocol.SignatureSet:
		return nil, errors.New(errors.StatusBadRequest, "a signature set is not allowed outside of a forwarded transaction")

	case *protocol.DelegatedSignature:
		delegate, err = x.validateSignature(batch, delivery, signature.Signature, md.SetDelegated())
		if err != nil {
			return nil, err
		}
		if !signature.Delegator.LocalTo(md.Location) {
			return nil, nil
		}

		// Validate the delegator
		signer, err = x.validateSigner(batch, delivery.Transaction, signature.Delegator, md.Location, false)
		if err != nil {
			return nil, errors.Wrap(errors.StatusUnknown, err)
		}

		// Verify delegation
		_, _, ok := signer.EntryByDelegate(delegate.GetUrl())
		if !ok {
			return nil, errors.Format(errors.StatusUnauthorized, "%v is not authorized to sign for %v", delegate.GetUrl(), signature.Delegator)
		}

	case protocol.KeySignature:
		// Basic validation
		if !signature.Verify(delivery.Transaction.GetHash()) {
			return nil, errors.New(errors.StatusBadRequest, "invalid")
		}
		if !delivery.Transaction.Body.Type().IsUser() {
			err = x.validatePartitionSignature(md.Location, signature, delivery.Transaction)
			if err != nil {
				return nil, errors.Wrap(errors.StatusUnknown, err)
			}
		}

		signer, err = x.validateKeySignature(batch, delivery, signature, !md.Initiated, !md.Delegated && delivery.Transaction.Header.Principal.LocalTo(md.Location))

	default:
		return nil, errors.Format(errors.StatusBadRequest, "unknown signature type %v", signature.Type())
	}
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnauthenticated, err)
	}

	return signer, nil
}

func validateSyntheticTransactionSignatures(transaction *protocol.Transaction, signatures []protocol.Signature) error {
	var gotSynthSig, gotReceiptSig, gotED25519Sig bool
	for _, sig := range signatures {
		switch sig.(type) {
		case *protocol.SyntheticSignature:
			gotSynthSig = true

		case *protocol.ReceiptSignature:
			gotReceiptSig = true

		case *protocol.ED25519Signature, *protocol.LegacyED25519Signature:
			gotED25519Sig = true

		default:
			return errors.Format(errors.StatusBadRequest, "synthetic transaction do not support %T signatures", sig)
		}
	}

	if !gotSynthSig {
		return errors.Format(errors.StatusUnauthenticated, "missing synthetic transaction origin")
	}
	if !gotED25519Sig {
		return errors.Format(errors.StatusUnauthenticated, "missing ED25519 signature")
	}
	if transaction.Body.Type() == protocol.TransactionTypeDirectoryAnchor || transaction.Body.Type() == protocol.TransactionTypePartitionAnchor {
		return nil
	}

	if !gotReceiptSig {
		return errors.Format(errors.StatusUnauthenticated, "missing synthetic transaction receipt")
	}
	return nil
}

// checkRouting verifies that the signature was routed to the correct subnet.
func (x *Executor) checkRouting(delivery *chain.Delivery, signature protocol.Signature) error {
	if signature.Type().IsSystem() {
		return nil
	}

	if delivery.Transaction.Body.Type().IsUser() {
		subnet, err := x.Router.RouteAccount(signature.RoutingLocation())
		if err != nil {
			return errors.Wrap(errors.StatusUnknown, err)
		}
		if !strings.EqualFold(subnet, x.Describe.SubnetId) {
			return errors.Format(errors.StatusBadRequest, "signature submitted to %v instead of %v", x.Describe.SubnetId, subnet)
		}
	}

	return nil
}
