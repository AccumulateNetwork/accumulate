package block

import (
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
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
	if txnType != protocol.TransactionTypeRemote {
		txnType = delivery.Transaction.Body.Type()
		_, ok := x.executors[txnType]
		if !ok {
			return nil, protocol.Errorf(protocol.ErrorCodeInvalidRequest, "unsupported transaction type: %v", txnType)
		}
	}

	// Load the transaction
	err := delivery.LoadTransaction(batch)
	if err != nil {
		return nil, err
	}

	// Check that the signatures are valid
	for i, signature := range delivery.Signatures {
		isInitiator := i == 0 && txnType != protocol.TransactionTypeRemote
		if isInitiator {
			// Verify that the initiator signature matches the transaction
			err = validateInitialSignature(delivery.Transaction, signature)
			if err != nil {
				return nil, err
			}
		}

		// Basic validation
		if !signature.Verify(delivery.Transaction.GetHash()) {
			return nil, protocol.Errorf(protocol.ErrorCodeInvalidSignature, "signature %d: invalid", i)
		}

		// Stateful validation (mostly for synthetic transactions)
		switch signature := signature.(type) {
		case *protocol.SyntheticSignature:
			err = verifySyntheticSignature(&x.Network, batch, delivery.Transaction, signature, isInitiator)

		case *protocol.ReceiptSignature:
			err = verifyReceiptSignature(&x.Network, batch, delivery.Transaction, signature, isInitiator)

		case *protocol.InternalSignature:
			err = validateInternalSignature(&x.Network, batch, delivery.Transaction, signature, isInitiator)

		default:
			err = validateNormalSignature(batch, delivery.Transaction, signature, isInitiator)
		}
		if err != nil {
			return nil, protocol.Errorf(protocol.ErrorCodeInvalidSignature, "signature %d: %w", i, err)
		}
	}

	switch {
	case txnType.IsUser():
		err = nil
	case txnType.IsSynthetic():
		err = validateSyntheticEnvelope(&x.Network, batch, delivery)
	case txnType.IsInternal():
		// TODO Validate internal transactions
		err = nil
	default:
		// Should be unreachable
		return nil, protocol.Errorf(protocol.ErrorCodeInternal, "transaction type %v is not user, synthetic, or internal", txnType)
	}
	if err != nil {
		return nil, err
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
		return nil, protocol.Errorf(protocol.ErrorCodeInvalidRequest, "invalid transaction: initiated by receipt signature")
	}

	var signer protocol.Signer
	err = batch.Account(firstSig.GetSigner()).GetStateAs(&signer)
	if err != nil {
		return nil, fmt.Errorf("load signer: %w", err)
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
		return nil, fmt.Errorf("load principal: %w", err)
	case !transactionAllowsMissingPrincipal(delivery.Transaction):
		return nil, fmt.Errorf("load principal: %w", err)
	}

	// Set up the state manager
	st := chain.NewStateManager(batch.Begin(false), x.Network.NodeUrl(), signer.GetUrl(), signer, principal, delivery.Transaction, x.logger.With("operation", "ValidateEnvelope"))
	defer st.Discard()

	// Execute the transaction
	executor, ok := x.executors[delivery.Transaction.Body.Type()]
	if !ok {
		return nil, protocol.Errorf(protocol.ErrorCodeInternal, "missing executor for %v", delivery.Transaction.Body.Type())
	}

	result, err := executor.Validate(st, delivery)
	if err != nil {
		return nil, err
	}

	return result, nil
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
			return fmt.Errorf("synthetic transaction do not support %T signatures", sig)
		}
	}

	if !gotSynthSig {
		return fmt.Errorf("missing synthetic transaction origin")
	}
	if transaction.Body.Type() == protocol.TransactionTypeSyntheticAnchor {
		if !gotED25519Sig {
			return fmt.Errorf("missing ED25519 signature")
		}
	} else {
		if !gotReceiptSig {
			return fmt.Errorf("missing synthetic transaction receipt")
		}
	}

	return nil
}
