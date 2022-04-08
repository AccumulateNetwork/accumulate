package block

import (
	"crypto/sha256"
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

func hasBeenInitiated(batch *database.Batch, transaction *protocol.Transaction) (bool, error) {
	// Always assume remote transactions have been initiated
	if transaction.Body.Type() == protocol.TransactionTypeRemote {
		return true, nil
	}

	// Load the transaction status
	status, err := batch.Transaction(transaction.GetHash()).GetStatus()
	if err != nil {
		return false, fmt.Errorf("load object metadata: %w", err)
	}

	return status.Initiator != nil, nil
}

func (x *Executor) ProcessSignature(batch *database.Batch, delivery *Delivery, signature protocol.Signature) (*ProcessSignatureState, error) {
	// Is this the initial signature?
	initiated, err := hasBeenInitiated(batch, delivery.Transaction)
	if err != nil {
		return nil, err
	}
	if !initiated {
		// Verify that the initiator signature matches the transaction
		err = validateInitialSignature(delivery.Transaction, signature)
		if err != nil {
			return nil, err
		}
	}

	// Basic validation
	if !signature.Verify(delivery.Transaction.GetHash()) {
		return nil, errors.New("invalid")
	}

	// Stateful validation (mostly for synthetic transactions)
	switch signature := signature.(type) {
	case *protocol.SyntheticSignature:
		err = verifySyntheticSignature(&x.Network, batch, delivery.Transaction, signature, !initiated)

	case *protocol.ReceiptSignature:
		err = verifyReceiptSignature(&x.Network, batch, delivery.Transaction, signature, !initiated)

	case *protocol.InternalSignature:
		err = validateInternalSignature(&x.Network, batch, delivery.Transaction, signature, !initiated)

	case *protocol.ForwardedSignature:
		err = processForwardedSignature(batch, delivery, signature, !initiated)

	default:
		err = processNormalSignature(batch, delivery.Transaction, signature, !initiated)
	}
	if err != nil {
		return nil, err
	}

	fwdSig, forwarded := signature.(*protocol.ForwardedSignature)
	if forwarded {
		signature = fwdSig.Signature
	}

	// Store the transaction state (without signatures) if its local or
	// forwarded and being initiated - synthetic transactions are always treated
	// as local
	remote := delivery.Transaction.Body.Type().IsUser() && !delivery.Transaction.Header.Principal.LocalTo(signature.GetSigner())
	if !initiated && (!remote || forwarded) {
		stateEnv := new(protocol.Envelope)
		stateEnv.Transaction = delivery.Transaction
		db := batch.Transaction(delivery.Transaction.GetHash())
		err = db.PutState(stateEnv)
		if err != nil {
			return nil, fmt.Errorf("store transaction: %w", err)
		}
	}

	// Hash the signature
	sigData, err := signature.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("marshal signature: %w", err)
	}
	sigHash := sha256.Sum256(sigData)

	// Store the signature as an envelope
	env := new(protocol.Envelope)
	env.TxHash = delivery.Transaction.GetHash()
	env.Signatures = []protocol.Signature{signature}
	err = batch.Transaction(sigHash[:]).PutState(env)
	if err != nil {
		return nil, fmt.Errorf("store envelope: %w", err)
	}

	// Add the signature to the signer's chain (if it's a user transaction)
	if !forwarded && delivery.Transaction.Body.Type().IsUser() {
		chain, err := batch.Account(signature.GetSigner()).Chain(protocol.SignatureChain, protocol.ChainTypeTransaction)
		if err != nil {
			return nil, fmt.Errorf("load chain: %w", err)
		}
		err = chain.AddEntry(sigHash[:], true)
		if err != nil {
			return nil, fmt.Errorf("store chain: %w", err)
		}
	}

	// Add the signature to the principal's chain if it's local
	if !remote || forwarded {
		chain, err := batch.Account(delivery.Transaction.Header.Principal).Chain(protocol.SignatureChain, protocol.ChainTypeTransaction)
		if err != nil {
			return nil, fmt.Errorf("load chain: %w", err)
		}
		err = chain.AddEntry(sigHash[:], true)
		if err != nil {
			return nil, fmt.Errorf("store chain: %w", err)
		}
	}

	// Add the signature to the transaction's signature set
	var sigSet *database.SignatureSet
	if forwarded {
		sigSet, err = batch.Transaction(delivery.Transaction.GetHash()).SignaturesForSigner(fwdSig.Signer)
	} else {
		sigSet, err = batch.Transaction(delivery.Transaction.GetHash()).Signatures(signature.GetSigner())
	}
	if err != nil {
		return nil, fmt.Errorf("load signatures: %w", err)
	}
	_, err = sigSet.Add(signature)
	if err != nil {
		return nil, fmt.Errorf("store signature: %w", err)
	}

	return &ProcessSignatureState{Remote: remote && !forwarded}, nil
}

// validateInitialSignature verifies that the signature is a valid initial
// signature for the transaction.
func validateInitialSignature(transaction *protocol.Transaction, signature protocol.Signature) error {
	// Verify the initiator hash matches
	if !protocol.SignatureDidInitiate(signature, transaction.Header.Initiator[:]) {
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
func validateSignature(batch *database.Batch, transaction *protocol.Transaction, signature protocol.Signature) (protocol.Signer, protocol.KeyEntry, error) {
	signerUrl := signature.GetSigner()
	remote := !transaction.Header.Principal.LocalTo(signerUrl)
	forwarded := signature.Type() == protocol.SignatureTypeForwarded

	var account protocol.Account
	var err error
	switch signature := signature.(type) {
	case *protocol.ForwardedSignature:
		account = signature.Signer

	default:
		// Load the signer
		account, err = batch.Account(signerUrl).GetState()
		if err != nil {
			return nil, nil, fmt.Errorf("load signer: %w", err)
		}
	}
	if !account.GetUrl().Equal(signerUrl) {
		// Sanity check
		return nil, nil, protocol.Errorf(protocol.ErrorCodeInternal, "invalid state: URL does not match")
	}

	// Validate type-specific rules
	var signer protocol.Signer
	switch account := account.(type) {
	case *protocol.LiteTokenAccount:
		signer = account
		if remote && !forwarded {
			err = validateRemoteLiteSignature(transaction, account)
		} else {
			err = validateLocalLiteSignature(transaction, account)
		}
	case *protocol.KeyPage:
		signer = account
		if remote && !forwarded {
			err = validateRemotePageSignature(batch, transaction, account)
		} else {
			err = validateLocalPageSignature(batch, transaction, account)
		}
	default:
		return nil, nil, protocol.Errorf(protocol.ErrorCodeInvalidRequest, "invalid signer: %v cannot sign transactions", account.Type())
	}
	if err != nil {
		return nil, nil, err
	}

	// Check the height
	if signature.GetSignerVersion() != signer.GetVersion() {
		return nil, nil, protocol.Errorf(protocol.ErrorCodeBadVersion, "invalid version: have %d, got %d", signer.GetVersion(), signature.GetSignerVersion())
	}

	// Find the key entry
	_, entry, ok := signer.EntryByKeyHash(signature.GetPublicKeyHash())
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

// validateLocalLiteSignature verifies that the lite token account is authorized to
// sign for the principal.
func validateLocalLiteSignature(transaction *protocol.Transaction, signer *protocol.LiteTokenAccount) error {
	// A lite token account is only allowed to sign for itself
	if !signer.Url.Equal(transaction.Header.Principal) {
		return protocol.Errorf(protocol.ErrorCodeUnauthorized, "%v is not authorized to sign transactions for %v", signer.Url, transaction.Header.Principal)
	}

	return nil
}

// validateRemoteLiteSignature verifies that the lite token account is
// authorized to sign for the principal.
func validateRemoteLiteSignature(transaction *protocol.Transaction, signer *protocol.LiteTokenAccount) error {
	return protocol.NewError(protocol.ErrorCodeUnauthorized, errors.New("remote signatures are not supported for lite accounts"))
}

// validateLocalPageSignature verifies that the key page is authorized to sign for
// the principal.
func validateLocalPageSignature(batch *database.Batch, transaction *protocol.Transaction, signer *protocol.KeyPage) error {
	// Load the principal
	principal, err := batch.Account(transaction.Header.Principal).GetState()
	if err != nil {
		return fmt.Errorf("load principal: %w", err)
	}

	// Get the principal's account auth
	auth, err := getAccountAuth(batch, principal)
	if err != nil {
		return fmt.Errorf("unable to load authority of %v: %w", transaction.Header.Principal, err)
	}

	// Verify that the key page is allowed to sign the transaction
	bit, ok := transaction.Body.Type().AllowedTransactionBit()
	if ok && signer.TransactionBlacklist.IsSet(bit) {
		return protocol.Errorf(protocol.ErrorCodeUnauthorized, "page %s is not authorized to sign %v", signer.Url, transaction.Body.Type())
	}

	// Get the signer book URL
	signerBook, _, ok := protocol.ParseKeyPageUrl(signer.Url)
	if !ok {
		// If this happens, the database has bad data
		return fmt.Errorf("invalid key page URL: %v", signer.Url)
	}

	_, foundAuthority := auth.GetAuthority(signerBook)
	switch {
	case foundAuthority:
		// Page belongs to book => authorized
		return nil

	case auth.AuthDisabled() && !transaction.Body.Type().RequireAuthorization():
		// Authorization is disabled and the transaction type does not force authorization => authorized
		return nil

	default:
		// Authorization is enabled => unauthorized
		// Transaction type forces authorization => unauthorized
		return protocol.Errorf(protocol.ErrorCodeUnauthorized, "%v is not authorized to sign transactions for %v", signer.Url, principal.GetUrl())
	}
}

// validateRemotePageSignature verifies that the key page is authorized to sign for
// the principal.
func validateRemotePageSignature(_ *database.Batch, transaction *protocol.Transaction, signer *protocol.KeyPage) error {
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

// processForwardedSignature validates a forwarded private key signature.
func processForwardedSignature(batch *database.Batch, delivery *Delivery, signature *protocol.ForwardedSignature, _ bool) error {
	// TODO If the forwarded signature paid the full fee unnecessarily, refund
	// it

	// Forwarded signatures are only legal within a synthetic forwarded
	// transaction
	if !delivery.IsForwarded() {
		return fmt.Errorf("invalid forwarded signature")
	}

	// Validate type-specific rules
	switch account := signature.Signer.(type) {
	case *protocol.LiteTokenAccount:
		return validateLocalLiteSignature(delivery.Transaction, account)
	case *protocol.KeyPage:
		return validateLocalPageSignature(batch, delivery.Transaction, account)
	default:
		return protocol.Errorf(protocol.ErrorCodeInvalidRequest, "invalid signer: %v cannot sign transactions", account.Type())
	}
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

func getAllSignatures(batch *database.Batch, transaction *database.Transaction, status *protocol.TransactionStatus, txnInitHash []byte) ([]protocol.Signature, error) {
	signatures := make([]protocol.Signature, 1)

	for _, signer := range status.Signers {
		// Load the signature set
		sigset, err := transaction.ReadSignaturesForSigner(signer)
		if err != nil {
			return nil, fmt.Errorf("load signatures set %v: %w", signer.GetUrl(), err)
		}

		for _, entryHash := range sigset.EntryHashes() {
			state, err := batch.Transaction(entryHash[:]).GetState()
			if err != nil {
				return nil, fmt.Errorf("load signature entry %X: %w", entryHash, err)
			}

			for _, sig := range state.Signatures {
				if protocol.SignatureDidInitiate(sig, txnInitHash) {
					signatures[0] = sig
				} else {
					signatures = append(signatures, sig)
				}
			}
		}
	}

	if signatures[0] == nil {
		signatures = signatures[1:]
	}

	return signatures, nil
}
