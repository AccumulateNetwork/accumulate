package chain

import (
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (x *Executor) ProcessSignature(batch *database.Batch, transaction *protocol.Transaction, signature protocol.Signature) error {
	ctx := &processSignatureContext{
		Executor:    x,
		Batch:       batch,
		Transaction: transaction,
		Signature:   signature,
	}

	// Load the existing signature set
	sigSet, err := batch.Transaction(transaction.GetHash()).GetSignatures()
	if err != nil {
		return fmt.Errorf("load signatures: %w", err)
	}

	// Is this the initial signature?
	isInitiator := sigSet.Count() == 0

	switch signature.Type() {
	case protocol.SignatureTypeUnknown:
		// This should not happen
		return protocol.NewError(protocol.ErrorCodeInvalidRequest, errors.New("signature type is unknown"))

	case protocol.SignatureTypeLegacyED25519,
		protocol.SignatureTypeED25519,
		protocol.SignatureTypeRCD1:
		// These are normal signature types, which support all Signature methods
		err := ctx.processNormalSignature(isInitiator)
		if err != nil {
			return err
		}

	case protocol.SignatureTypeReceipt,
		protocol.SignatureTypeSynthetic,
		protocol.SignatureTypeInternal:
		// These are special signature types, which may not support all
		// Signature methods
		err := ctx.processSystemSignature()
		if err != nil {
			return err
		}

	default:
		// This should not happen
		return fmt.Errorf("unknown signature type %v", signature.Type())
	}

	// Add the signature to the transaction's signature set
	sigSet.Add(signature)
	err = batch.Transaction(transaction.GetHash()).PutSignatures(sigSet)
	if err != nil {
		return fmt.Errorf("store signatures: %w", err)
	}

	// For non-user transactions, do not append to the signature chain
	if !transaction.Body.Type().IsUser() {
		return nil
	}

	// Store the signature as an envelope
	env := new(protocol.Envelope)
	env.TxHash = transaction.GetHash()
	env.Signatures = []protocol.Signature{signature}
	err = batch.Transaction(env.EnvHash()).PutState(env)
	if err != nil {
		return fmt.Errorf("store envelope: %w", err)
	}

	// Add the signature to the signer's chain
	chain, err := batch.Account(signature.GetSigner()).Chain(protocol.SignatureChain, protocol.ChainTypeTransaction)
	if err != nil {
		return fmt.Errorf("load chain: %w", err)
	}
	err = chain.AddEntry(env.EnvHash(), true)
	if err != nil {
		return fmt.Errorf("store chain: %w", err)
	}

	// Add the signature to the principal's chain
	chain, err = batch.Account(transaction.Header.Principal).Chain(protocol.SignatureChain, protocol.ChainTypeTransaction)
	if err != nil {
		return fmt.Errorf("load chain: %w", err)
	}
	err = chain.AddEntry(env.EnvHash(), true)
	if err != nil {
		return fmt.Errorf("store chain: %w", err)
	}

	return nil
}

type processSignatureContext struct {
	*Executor
	Batch       *database.Batch
	Transaction *protocol.Transaction
	Signature   protocol.Signature
}

func (x *processSignatureContext) processSystemSignature() error {
	// This is a placeholder in case we decide to add more validation. These are
	// not currently validated beyond what ValidateEnvelope does.
	return nil
}

func (x *processSignatureContext) processNormalSignature(isInitiator bool) error {
	// Skip for non-user transactions
	if !x.Transaction.Body.Type().IsUser() {
		// TODO Check the key
		return nil
	}

	// For the initiator
	if isInitiator {
		// Require a timestamp
		if isInitiator && x.Signature.GetTimestamp() == 0 {
			return protocol.Errorf(protocol.ErrorCodeInvalidRequest, "initial signature does not have a timestamp")
		}

		// Ensure the initiator hash matches
		initHash, err := x.Signature.InitiatorHash()
		if err != nil {
			return protocol.NewError(protocol.ErrorCodeInvalidSignature, err)
		}
		if x.Transaction.Header.Initiator != *(*[32]byte)(initHash) {
			return protocol.Errorf(protocol.ErrorCodeInvalidSignature, "initiator signature does not match initiator hash")
		}
	}

	// Load the signer and validate the signature against it. This should not
	// fail, because this signature has presumably already passed
	// ValidateEnvelope. But defensive programming is always a good idea.
	signer, entry, err := validateSignature(x.Batch, x.Transaction, x.Signature)
	if err != nil {
		return err
	}

	// Charge the fee
	fee, err := computeSignerFee(x.Transaction, x.Signature, isInitiator)
	if err != nil {
		return fmt.Errorf("calculating fee: %w", err)
	}
	if !signer.DebitCredits(fee.AsUInt64()) {
		return protocol.Errorf(protocol.ErrorCodeInsufficientCredits, "insufficient credits: have %s, want %s",
			protocol.FormatAmount(signer.GetCreditBalance(), protocol.CreditPrecision),
			protocol.FormatAmount(fee.AsUInt64(), protocol.CreditPrecision))
	}

	// Update the timestamp - the value is validated by validateSignature
	if x.Signature.GetTimestamp() != 0 {
		entry.SetLastUsedOn(x.Signature.GetTimestamp())
	}

	// Store changes to the signer
	err = x.Batch.Account(x.Signature.GetSigner()).PutState(signer)
	if err != nil {
		return fmt.Errorf("store signer: %w", err)
	}

	return nil
}

func validateSignature(batch *database.Batch, transaction *protocol.Transaction, signature protocol.Signature) (protocol.SignerAccount, protocol.KeyEntry, error) {
	// Load the signer
	signerUrl := signature.GetSigner()
	account, err := batch.Account(signerUrl).GetState()
	if err != nil {
		return nil, nil, fmt.Errorf("load signer: %w", err)
	}
	if !account.Header().Url.Equal(signerUrl) {
		// Sanity check
		return nil, nil, protocol.Errorf(protocol.ErrorCodeInternal, "invalid state: URL does not match")
	}

	// Validate type-specific rules
	var signer protocol.SignerAccount
	switch account := account.(type) {
	case *protocol.LiteTokenAccount:
		signer = account
		err = validateLiteSignature(transaction, account)
	case *protocol.KeyPage:
		signer = account
		err = validatePageSignature(batch, transaction, account)
	default:
		return nil, nil, protocol.Errorf(protocol.ErrorCodeInvalidRequest, "invalid signer: %v cannot sign transactions", account.Type())
	}
	if err != nil {
		return nil, nil, err
	}

	// Check the height, except for lite accounts
	if signer.Type() != protocol.AccountTypeLiteTokenAccount {
		chain, err := batch.Account(signerUrl).ReadChain(protocol.MainChain)
		if err != nil {
			return nil, nil, protocol.Errorf(protocol.ErrorCodeInternal, "read %v main chain: %v", signerUrl, err)
		}

		if signature.GetSignerHeight() != uint64(chain.Height()) {
			return nil, nil, protocol.Errorf(protocol.ErrorCodeBadVersion, "invalid version: have %d, got %d", chain.Height(), signature.GetSignerHeight())
		}
	}

	// Find the key entry
	_, entry, ok := signer.EntryByKey(signature.GetPublicKey())
	if !ok {
		return nil, nil, fmt.Errorf("key does not belong to signer")
	}

	// Check the timestamp, except for faucet transactions
	if transaction.Body.Type() != protocol.TransactionTypeAcmeFaucet &&
		signature.GetTimestamp() != 0 &&
		entry.GetLastUsedOn() >= signature.GetTimestamp() {
		return nil, nil, protocol.Errorf(protocol.ErrorCodeBadNonce, "invalid timestamp: have %d, got %d", entry.GetLastUsedOn(), signature.GetTimestamp())
	}

	return signer, entry, nil
}

func validateLiteSignature(transaction *protocol.Transaction, signer *protocol.LiteTokenAccount) error {
	// A lite token account is only allowed to sign for itself
	if !signer.Url.Equal(transaction.Header.Principal) {
		return fmt.Errorf("%v is not authorized to sign transactions for %v", signer.Url, transaction.Header.Principal)
	}

	return nil
}

func validatePageSignature(batch *database.Batch, transaction *protocol.Transaction, signer *protocol.KeyPage) error {
	// Load the principal
	principal, err := batch.Account(transaction.Header.Principal).GetState()
	if err != nil {
		return fmt.Errorf("load principal: %w", err)
	}

	// Get the key book URL
	pageBook, _, ok := protocol.ParseKeyPageUrl(signer.Url)
	if !ok {
		// If this happens, the database has bad data
		return fmt.Errorf("invalid key page URL: %v", signer.Url)
	}

	// Verify that the key page is authorized to sign transactions for the book
	var principalBookUrl *url.URL
	switch principal := principal.(type) {
	case *protocol.KeyBook:
		principalBookUrl = principal.Url
	default:
		principalBookUrl = principal.Header().KeyBook
	}
	var book *protocol.KeyBook
	err = batch.Account(principalBookUrl).GetStateAs(&book)
	if err != nil {
		return fmt.Errorf("invalid key book URL: %v, %v", principalBookUrl, err)
	}
	if !principalBookUrl.Equal(pageBook) && book.AuthEnabled {
		return protocol.Errorf(protocol.ErrorCodeUnauthorized, "%v is not authorized to sign transactions for %v", signer.Url, principal.Header().Url)
	}

	// Verify that the key page is allowed to sign the transaction
	bit, ok := transaction.Body.Type().AllowedTransactionBit()
	if ok && signer.TransactionBlacklist.IsSet(bit) {
		return protocol.Errorf(protocol.ErrorCodeUnauthorized, "page %s is not authorized to sign %v", signer.Url, transaction.Body.Type())
	}

	return nil
}

func computeSignerFee(transaction *protocol.Transaction, signature protocol.Signature, isInitiator bool) (protocol.Fee, error) {
	// Compute the signature fee
	fee, err := protocol.ComputeSignatureFee(signature)
	if err != nil {
		return 0, err
	}
	if !isInitiator {
		return fee, nil
	}

	// Add the transaction fee for the initial signature
	txnFee, err := protocol.ComputeTransactionFee(transaction)
	if err != nil {
		return 0, err
	}

	// Subtract the base signature fee, but not the oversize surcharge if there is one
	fee += txnFee - protocol.FeeSignature
	return fee, nil
}
