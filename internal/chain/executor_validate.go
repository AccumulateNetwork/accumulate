package chain

import (
	"crypto/sha256"
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/config"
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
func (x *Executor) ValidateEnvelope(batch *database.Batch, envelope *protocol.Envelope) (*protocol.Transaction, error) {
	// If the transaction is borked, the transaction type is probably invalid,
	// so check that first. "Invalid transaction type" is a more useful error
	// than "invalid signature" if the real error is the transaction got borked.
	txnType := envelope.Type()
	if txnType != protocol.TransactionTypeSignPending {
		txnType = envelope.Transaction.Body.Type()
		_, ok := x.executors[txnType]
		if !ok {
			return nil, protocol.Errorf(protocol.ErrorCodeInvalidRequest, "unsupported transaction type: %v", txnType)
		}
	}

	// An envelope with no signatures is invalid
	if len(envelope.Signatures) == 0 {
		return nil, protocol.Errorf(protocol.ErrorCodeInvalidRequest, "envelope has no signatures")
	}

	// The transaction hash must be specified for signature transactions
	if len(envelope.TxHash) == 0 && txnType == protocol.TransactionTypeSignPending {
		return nil, protocol.Errorf(protocol.ErrorCodeInvalidRequest, "cannot sign pending transaction: missing transaction hash")
	}

	// The transaction hash and/or the transaction itself must be specified
	if len(envelope.TxHash) == 0 && envelope.Transaction == nil {
		return nil, protocol.Errorf(protocol.ErrorCodeInvalidRequest, "envelope has neither transaction nor hash")
	}

	// The transaction hash must be the correct size
	if len(envelope.TxHash) > 0 && len(envelope.TxHash) != sha256.Size {
		return nil, protocol.Errorf(protocol.ErrorCodeInvalidRequest, "transaction hash is the wrong size")
	}

	// If a transaction and a hash are specified, they must match
	if !envelope.VerifyTxHash() {
		return nil, protocol.Errorf(protocol.ErrorCodeInvalidRequest, "transaction hash does not match transaction")
	}

	status, err := batch.Transaction(envelope.GetTxHash()).GetStatus()
	switch {
	case errors.Is(err, storage.ErrNotFound):
		// Not found means the transaction is new

	case err != nil:
		// Unknown error
		return nil, fmt.Errorf("load transaction status: %w", err)

	case status.Delivered:
		// Transaction has already been delivered
		return nil, protocol.Errorf(protocol.ErrorCodeAlreadyDelivered, "transaction has already been delivered")
	}

	// Load previous transaction state
	transaction := envelope.Transaction
	if envelope.Type() == protocol.TransactionTypeSignPending {
		txState, err := batch.Transaction(envelope.GetTxHash()).GetState()
		switch {
		case err == nil:
			// Populate the transaction from the database
			transaction = txState.Transaction

		default:
			// If the envelope does not include the transaction, it must exist
			// in the database
			return nil, fmt.Errorf("load transaction: %v", err)
		}
	}

	// Check that the signatures are valid
	for i, signature := range envelope.Signatures {
		isInitiator := i == 0 && txnType != protocol.TransactionTypeSignPending
		if isInitiator {
			// Verify that the initiator signature matches the transaction
			initHash, err := signature.InitiatorHash()
			if err != nil {
				return nil, protocol.NewError(protocol.ErrorCodeInvalidSignature, err)
			}

			if envelope.Transaction.Header.Initiator != *(*[32]byte)(initHash) {
				return nil, protocol.Errorf(protocol.ErrorCodeInvalidSignature, "initiator signature does not match initiator hash")
			}
		}

		err := x.validateEnvelopeSignature(batch, transaction, signature, isInitiator)
		if err != nil {
			return nil, protocol.Errorf(protocol.ErrorCodeInvalidSignature, "signature %d: %w", i, err)
		}
	}

	switch {
	case txnType.IsUser():
		err = x.validateUserEnvelope(batch, envelope, txnType)
	case txnType.IsSynthetic():
		err = x.validateSyntheticEnvelope(batch, envelope)
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

	return transaction, nil
}

func (x *Executor) validateEnvelopeSignature(batch *database.Batch, transaction *protocol.Transaction, signature protocol.Signature, isInitiator bool) error {
	// Basic validation
	if !signature.Verify(transaction.GetHash()) {
		return errors.New("invalid")
	}

	// Stateful validation (mostly for synthetic transactions)
	switch signature := signature.(type) {
	case *protocol.SyntheticSignature:
		if !transaction.Body.Type().IsSynthetic() {
			return fmt.Errorf("synthetic signatures are not allowed for non-synthetic transactions")
		}

		if !x.Network.NodeUrl().Equal(signature.DestinationNetwork) {
			return fmt.Errorf("wrong destination network: %v is not this network", signature.DestinationNetwork)
		}

		// TODO Check the sequence number

	case *protocol.ReceiptSignature:
		if !transaction.Body.Type().IsSynthetic() {
			return fmt.Errorf("receipt signatures are not allowed for non-synthetic transactions")
		}

		// TODO We should add something so we know which subnet originated
		// the transaction. That way, the DN can also check receipts.
		if x.Network.Type != config.BlockValidator {
			// TODO Check receipts on the DN
			return nil
		}

		// Load the anchor chain
		anchorChain, err := batch.Account(x.Network.AnchorPool()).ReadChain(protocol.AnchorChain(protocol.Directory))
		if err != nil {
			return fmt.Errorf("unable to load the DN intermediate anchor chain: %w", err)
		}

		// Is the result a valid DN anchor?
		_, err = anchorChain.HeightOf(signature.Result)
		switch {
		case err == nil:
			// OK
		case errors.Is(err, storage.ErrNotFound):
			return fmt.Errorf("invalid receipt: result is not a known DN anchor")
		default:
			return fmt.Errorf("unable to check if a DN anchor is valid: %w", err)
		}

	case *protocol.InternalSignature:
		if !transaction.Body.Type().IsInternal() {
			return fmt.Errorf("internal signatures are not allowed for non-internal transactions")
		}

		if !x.Network.NodeUrl().Equal(signature.Network) {
			return fmt.Errorf("wrong destination network: %v is not this network", signature.Network)
		}

		// TODO Check something?

	default:
		if transaction.Body.Type().IsSynthetic() {
			// TODO Check the key
			return nil
		}

		// Require a timestamp for the initiator
		if isInitiator && signature.GetTimestamp() == 0 {
			return protocol.Errorf(protocol.ErrorCodeInvalidRequest, "initial signature does not have a timestamp")
		}

		// Load the signer and validate the signature against it
		signer, _, err := validateSignature(batch, transaction, signature)
		if err != nil {
			return err
		}

		// Ensure the signer has sufficient credits for the fee
		fee, err := computeSignerFee(transaction, signature, isInitiator)
		if err != nil {
			return fmt.Errorf("calculating fee: %w", err)
		}
		if !signer.CanDebitCredits(fee.AsUInt64()) {
			return protocol.Errorf(protocol.ErrorCodeInsufficientCredits, "insufficient credits: have %s, want %s",
				protocol.FormatAmount(signer.GetCreditBalance(), protocol.CreditPrecision),
				protocol.FormatAmount(fee.AsUInt64(), protocol.CreditPrecision))
		}
	}

	return nil
}

func (x *Executor) validateSyntheticEnvelope(batch *database.Batch, envelope *protocol.Envelope) error {
	// TODO Get rid of this hack and actually check the nonce. But first we have
	// to implement transaction batching.
	v := batch.Account(x.Network.NodeUrl()).Index("SeenSynth", envelope.GetTxHash())
	_, err := v.Get()
	if err == nil {
		return protocol.Errorf(protocol.ErrorCodeBadNonce, "duplicate synthetic transaction %X", envelope.GetTxHash())
	} else if errors.Is(err, storage.ErrNotFound) {
		// // TODO We probably shouldn't be writing during validation
		// err = v.Put([]byte{1})
		// if err != nil {
		// 	return err
		// }
	} else {
		return err
	}

	var gotSynthSig, gotReceiptSig, gotED25519Sig bool
	for _, sig := range envelope.Signatures {
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
	if envelope.Transaction.Body.Type() == protocol.TransactionTypeSyntheticAnchor {
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

func (x *Executor) validateUserEnvelope(batch *database.Batch, envelope *protocol.Envelope, txnType protocol.TransactionType) (err error) {
	// Load previous transaction state
	_, err = batch.Transaction(envelope.GetTxHash()).GetState()
	switch {
	case err == nil:
		// The transaction already exists in the database
		return nil

	case !errors.Is(err, storage.ErrNotFound):
		// An unknown error occurred
		return fmt.Errorf("load transaction: %v", err)

	case txnType == protocol.TransactionTypeSignPending:
		// We can't sign a pending transaction if we can't find it
		return err

	default:
		// This is a new transaction
		return nil
	}
}
