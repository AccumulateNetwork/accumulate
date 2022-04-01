package chain

import (
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

func (x *Executor) ProcessSignature(batch *database.Batch, transaction *protocol.Transaction, signature protocol.Signature) (*ProcessSignatureState, error) {
	// Load the existing signature set
	sigSet, err := batch.Transaction(transaction.GetHash()).GetSignatures()
	if err != nil {
		return nil, fmt.Errorf("load signatures: %w", err)
	}

	// Is this the initial signature?
	isInitiator := sigSet.Count() == 0
	if isInitiator {
		// Verify that the initiator signature matches the transaction
		err = validateInitialSignature(transaction, signature)
		if err != nil {
			return nil, err
		}
	}

	// Basic validation
	if !signature.Verify(transaction.GetHash()) {
		return nil, errors.New("invalid")
	}

	// Stateful validation (mostly for synthetic transactions)
	switch signature := signature.(type) {
	case *protocol.SyntheticSignature:
		err = verifySyntheticSignature(&x.Network, batch, transaction, signature, isInitiator)

	case *protocol.ReceiptSignature:
		err = verifyReceiptSignature(&x.Network, batch, transaction, signature, isInitiator)

	case *protocol.InternalSignature:
		err = validateInternalSignature(&x.Network, batch, transaction, signature, isInitiator)

	default:
		err = processNormalSignature(batch, transaction, signature, isInitiator)
	}
	if err != nil {
		return nil, err
	}

	// Store the transaction state (without signatures)
	stateEnv := new(protocol.Envelope)
	stateEnv.Transaction = transaction
	db := batch.Transaction(transaction.GetHash())
	err = db.PutState(stateEnv)
	if err != nil {
		return nil, fmt.Errorf("store transaction: %w", err)
	}

	// Add the signature to the transaction's signature set
	sigSet.Add(signature)
	err = batch.Transaction(transaction.GetHash()).PutSignatures(sigSet)
	if err != nil {
		return nil, fmt.Errorf("store signatures: %w", err)
	}

	// For non-user transactions, do not append to the signature chain
	if !transaction.Body.Type().IsUser() {
		return &ProcessSignatureState{}, nil
	}

	// Store the signature as an envelope
	env := new(protocol.Envelope)
	env.TxHash = transaction.GetHash()
	env.Signatures = []protocol.Signature{signature}
	err = batch.Transaction(env.EnvHash()).PutState(env)
	if err != nil {
		return nil, fmt.Errorf("store envelope: %w", err)
	}

	// Add the signature to the signer's chain
	chain, err := batch.Account(signature.GetSigner()).Chain(protocol.SignatureChain, protocol.ChainTypeTransaction)
	if err != nil {
		return nil, fmt.Errorf("load chain: %w", err)
	}
	err = chain.AddEntry(env.EnvHash(), true)
	if err != nil {
		return nil, fmt.Errorf("store chain: %w", err)
	}

	// Add the signature to the principal's chain
	chain, err = batch.Account(transaction.Header.Principal).Chain(protocol.SignatureChain, protocol.ChainTypeTransaction)
	if err != nil {
		return nil, fmt.Errorf("load chain: %w", err)
	}
	err = chain.AddEntry(env.EnvHash(), true)
	if err != nil {
		return nil, fmt.Errorf("store chain: %w", err)
	}

	return &ProcessSignatureState{}, nil
}

// validateInitialSignature verifies that the signature is a valid initial
// signature for the transaction.
func validateInitialSignature(transaction *protocol.Transaction, signature protocol.Signature) error {
	// The initial signature must be able to create an initiator hash
	initHash, err := signature.InitiatorHash()
	if err != nil {
		return protocol.NewError(protocol.ErrorCodeInvalidSignature, err)
	}

	// Verify the hash matches
	if transaction.Header.Initiator != *(*[32]byte)(initHash) {
		return protocol.Errorf(protocol.ErrorCodeInvalidSignature, "initiator signature does not match initiator hash")
	}

	// Timestamps are not used for system signatures
	switch signature.Type() {
	case protocol.SignatureTypeSynthetic,
		protocol.SignatureTypeReceipt,
		protocol.SignatureTypeInternal:
		return nil
	}

	// Require a timestamp for the initiator
	if signature.GetTimestamp() == 0 {
		return protocol.Errorf(protocol.ErrorCodeInvalidRequest, "initial signature does not have a timestamp")
	}

	return nil
}

// validateSignature verifies that the signature matches the signer state and
// that the signer is authorized to sign for the principal.
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

		if signature.GetSignerVersion() != uint64(chain.Height()) {
			return nil, nil, protocol.Errorf(protocol.ErrorCodeBadVersion, "invalid version: have %d, got %d", chain.Height(), signature.GetSignerVersion())
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

// validateLiteSignature verifies that the lite token account is authorized to
// sign for the principal.
func validateLiteSignature(transaction *protocol.Transaction, signer *protocol.LiteTokenAccount) error {
	// A lite token account is only allowed to sign for itself
	if !signer.Url.Equal(transaction.Header.Principal) {
		return fmt.Errorf("%v is not authorized to sign transactions for %v", signer.Url, transaction.Header.Principal)
	}

	return nil
}

// validatePageSignature verifies that the key page is authorized to sign for
// the principal.
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
	if !principalBookUrl.Equal(pageBook) {
		return protocol.Errorf(protocol.ErrorCodeUnauthorized, "%v is not authorized to sign transactions for %v", signer.Url, principal.Header().Url)
	}

	// Verify that the key page is allowed to sign the transaction
	bit, ok := transaction.Body.Type().AllowedTransactionBit()
	if ok && signer.TransactionBlacklist.IsSet(bit) {
		return protocol.Errorf(protocol.ErrorCodeUnauthorized, "page %s is not authorized to sign %v", signer.Url, transaction.Body.Type())
	}

	return nil
}

// computeSignerFee computes the fee that will be charged to the signer.
//
// If the signature is the initial signature, the fee is the base transaction
// fee + signature data surcharge + transaction data surcharge.
//
// Otherwise, the fee is the base signature fee + signature data surcharge.
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

// validateNormalSignature validates a private key signature.
func validateNormalSignature(batch *database.Batch, transaction *protocol.Transaction, signature protocol.Signature, isInitiator bool) error {
	if !transaction.Body.Type().IsUser() {
		// TODO Check the key
		return nil
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
			protocol.FormatAmount(signer.GetCreditBalance(), protocol.CreditPrecisionPower),
			protocol.FormatAmount(fee.AsUInt64(), protocol.CreditPrecisionPower))
	}

	return nil
}

// processNormalSignature validates a private key signature and updates the
// signer.
func processNormalSignature(batch *database.Batch, transaction *protocol.Transaction, signature protocol.Signature, isInitiator bool) error {
	// Skip for non-user transactions
	if !transaction.Body.Type().IsUser() {
		// TODO Check the key
		return nil
	}

	// Load the signer and validate the signature against it. This should not
	// fail, because this signature has presumably already passed
	// ValidateEnvelope. But defensive programming is always a good idea.
	signer, entry, err := validateSignature(batch, transaction, signature)
	if err != nil {
		return err
	}

	// Charge the fee
	fee, err := computeSignerFee(transaction, signature, isInitiator)
	if err != nil {
		return fmt.Errorf("calculating fee: %w", err)
	}
	if !signer.DebitCredits(fee.AsUInt64()) {
		return protocol.Errorf(protocol.ErrorCodeInsufficientCredits, "insufficient credits: have %s, want %s",
			protocol.FormatAmount(signer.GetCreditBalance(), protocol.CreditPrecisionPower),
			protocol.FormatAmount(fee.AsUInt64(), protocol.CreditPrecisionPower))
	}

	// Update the timestamp - the value is validated by validateSignature
	if signature.GetTimestamp() != 0 {
		entry.SetLastUsedOn(signature.GetTimestamp())
	}

	// Store changes to the signer
	err = batch.Account(signature.GetSigner()).PutState(signer)
	if err != nil {
		return fmt.Errorf("store signer: %w", err)
	}

	return nil
}

func verifySyntheticSignature(net *config.Network, _ *database.Batch, transaction *protocol.Transaction, signature *protocol.SyntheticSignature, _ bool) error {
	if !transaction.Body.Type().IsSynthetic() {
		return fmt.Errorf("synthetic signatures are not allowed for non-synthetic transactions")
	}

	// if !isInitiator {
	// 	return fmt.Errorf("synthetic signatures must be the initiator")
	// }

	if !net.NodeUrl().Equal(signature.DestinationNetwork) {
		return fmt.Errorf("wrong destination network: %v is not this network", signature.DestinationNetwork)
	}

	// TODO Check the sequence number
	return nil
}

func verifyReceiptSignature(net *config.Network, batch *database.Batch, transaction *protocol.Transaction, signature *protocol.ReceiptSignature, isInitiator bool) error {
	if !transaction.Body.Type().IsSynthetic() {
		return fmt.Errorf("receipt signatures are not allowed for non-synthetic transactions")
	}

	if isInitiator {
		return fmt.Errorf("receipt signatures must not be the initiator")
	}

	// TODO We should add something so we know which subnet originated
	// the transaction. That way, the DN can also check receipts.
	if net.Type == config.Directory {
		// TODO Check receipts on the DN
		return nil
	}

	// Load the anchor chain
	anchorChain, err := batch.Account(net.AnchorPool()).ReadChain(protocol.AnchorChain(protocol.Directory))
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

	return nil
}

func validateInternalSignature(net *config.Network, _ *database.Batch, transaction *protocol.Transaction, signature *protocol.InternalSignature, _ bool) error {
	if !transaction.Body.Type().IsInternal() {
		return fmt.Errorf("internal signatures are not allowed for non-internal transactions")
	}

	if !net.NodeUrl().Equal(signature.Network) {
		return fmt.Errorf("wrong destination network: %v is not this network", signature.Network)
	}

	// TODO Check something?
	return nil
}
