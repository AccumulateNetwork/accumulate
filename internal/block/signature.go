package block

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
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

func (x *Executor) ProcessSignature(batch *database.Batch, delivery *chain.Delivery, signature protocol.Signature) (*ProcessSignatureState, error) {
	err := x.signatureIsWellFormed(delivery, signature)
	if err != nil {
		return nil, err
	}

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
	if signature.Type() != protocol.SignatureTypeReceipt && !signature.Verify(delivery.Transaction.GetHash()) {
		return nil, errors.Format(errors.StatusBadRequest, "invalid signature")
	}

	// Stateful validation (mostly for synthetic transactions)
	var signers []protocol.Signer
	switch signature := signature.(type) {
	case *protocol.SyntheticSignature:
		err = verifySyntheticSignature(&x.Network, batch, delivery.Transaction, signature, !initiated)

	case *protocol.ReceiptSignature:
		err = verifyReceiptSignature(&x.Network, batch, delivery.Transaction, signature, !initiated)

	case *protocol.InternalSignature:
		err = validateInternalSignature(&x.Network, batch, delivery.Transaction, signature, !initiated)

	default:
		signers, err = processNormalSignature(batch, delivery, signature, !initiated)
	}
	if err != nil {
		return nil, err
	}

	isUserTxn := delivery.Transaction.Body.Type().IsUser()
	if !isUserTxn {
		signer, err := loadSignerLocally(batch, signature.GetSigner())
		switch {
		case err == nil:
			signers = []protocol.Signer{signer}
		case errors.Is(err, errors.StatusNotFound):
			signers = []protocol.Signer{&protocol.UnknownSigner{Url: signature.GetSigner()}}
		default:
			return nil, err
		}
	}

	// Store the transaction state (without signatures) if its local and being
	// initiated - synthetic transactions are always treated as local
	isLocalTxn := delivery.Transaction.Header.Principal.LocalTo(signature.RoutingLocation())
	if !initiated && isUserTxn && isLocalTxn {
		stateEnv := new(database.SigOrTxn)
		stateEnv.Transaction = delivery.Transaction
		db := batch.Transaction(delivery.Transaction.GetHash())
		err = db.PutState(stateEnv)
		if err != nil {
			return nil, fmt.Errorf("store transaction: %w", err)
		}
	}

	env := new(database.SigOrTxn)
	env.Hash = *(*[32]byte)(delivery.Transaction.GetHash())
	if fwd, ok := signature.(*protocol.ForwardedSignature); ok {
		env.Signature = fwd.Signature
	} else {
		env.Signature = signature
	}
	sigHash := signature.Hash()
	err = batch.Transaction(sigHash).PutState(env)
	if err != nil {
		return nil, fmt.Errorf("store envelope: %w", err)
	}

	// Add the signature to the signers' chains
	if isUserTxn {
		for _, signer := range signers {
			if !signer.GetUrl().LocalTo(signature.RoutingLocation()) {
				continue
			}

			chain, err := batch.Account(signer.GetUrl()).Chain(protocol.SignatureChain, protocol.ChainTypeTransaction)
			if err != nil {
				return nil, fmt.Errorf("load chain: %w", err)
			}
			err = chain.AddEntry(sigHash, true)
			if err != nil {
				return nil, fmt.Errorf("store chain: %w", err)
			}
		}
	}

	// Add the signature to the principal's chain if it a local user transaction
	if isUserTxn && isLocalTxn {
		chain, err := batch.Account(delivery.Transaction.Header.Principal).Chain(protocol.SignatureChain, protocol.ChainTypeTransaction)
		if err != nil {
			return nil, fmt.Errorf("load chain: %w", err)
		}
		err = chain.AddEntry(sigHash, true)
		if err != nil {
			return nil, fmt.Errorf("store chain: %w", err)
		}
	}

	// Add the signature to the transaction's signature set if it's local or not
	// a user transaction
	if !isUserTxn || isLocalTxn {
		sigSet, err := batch.Transaction(delivery.Transaction.GetHash()).SignaturesForSigner(signers[0])
		if err != nil {
			return nil, fmt.Errorf("load signatures: %w", err)
		}
		_, err = sigSet.Add(signature)
		if err != nil {
			return nil, fmt.Errorf("store signature: %w", err)
		}
	}

	return &ProcessSignatureState{Signers: signers}, nil
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

// validateSigners verifies that the signer is valid and authorized.
func validateSigners(batch *database.Batch, transaction *protocol.Transaction, signature protocol.Signature) ([]protocol.Signer, error) {
	location := signature.RoutingLocation()
	fwdsig, ok := signature.(*protocol.ForwardedSignature)
	if ok {
		signature = fwdsig.Signature
	}

	// Load signers
	var signers []protocol.Signer
	for {
		var signerUrl *url.URL
		del, ok := signature.(*protocol.DelegatedSignature)
		if ok {
			signerUrl = del.Delegator
			signature = del.Signature
		} else {
			signerUrl = signature.GetSigner()
		}

		// If the user specifies a lite token address, convert it to a lite
		// identity
		if key, _, _ := protocol.ParseLiteTokenAddress(signerUrl); key != nil {
			signerUrl = signerUrl.Identity()
		}

		signer, err := validateSigner(batch, transaction, location, signerUrl, fwdsig)
		if err != nil {
			return nil, errors.Wrap(errors.StatusUnknown, err)
		}
		signers = append(signers, signer)

		if !ok {
			break
		}
	}

	// Reverse the slice
	for i := 0; i < len(signers)/2; i++ {
		j := len(signers) - i - 1
		signers[i], signers[j] = signers[j], signers[i]
	}

	// Check delegation
	for i, delegator := range signers[1:] {
		signerAuthority, _, ok := protocol.ParseKeyPageUrl(signers[i].GetUrl())
		if !ok {
			// signerAuthority = signers[i].GetUrl()
			return nil, errors.New(errors.StatusBadRequest, "delegation is only supported for key pages")
		}

		_, _, found := delegator.EntryByDelegate(signerAuthority)
		if _, unknown := delegator.(*protocol.UnknownSigner); !found && !unknown {
			return nil, errors.Format(errors.StatusUnauthorized, "%v is not an authorized delegate of %v", signerAuthority, delegator.GetUrl())
		}
	}

	if !location.LocalTo(transaction.Header.Principal) {
		return signers, nil
	}

	page, ok := signers[len(signers)-1].(*protocol.KeyPage)
	if !ok {
		return signers, nil
	}

	err := verifyPageIsAuthorized(batch, transaction, page)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}
	return signers, nil
}

func validateSigner(batch *database.Batch, transaction *protocol.Transaction, location, signerUrl *url.URL, fwdsig *protocol.ForwardedSignature) (protocol.Signer, error) {
	var signer protocol.Signer
	var err error
	local := location.LocalTo(signerUrl)
	if local {
		signer, err = loadSignerLocally(batch, signerUrl)
	} else if fwdsig != nil {
		signer, err = getForwardedSigner(fwdsig, signerUrl)
	} else {
		return &protocol.UnknownSigner{Url: signerUrl}, nil
	}
	switch {
	case err == nil:
		// Ok
	case local, !errors.Is(err, errors.StatusNotFound):
		return nil, errors.Format(errors.StatusUnknown, "load signer: %w", err)
	default:
		return &protocol.UnknownSigner{Url: signerUrl}, nil
	}

	err = verifySignerCanSign(transaction, signer)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}

	return signer, nil
}

func asSigner(account protocol.Account) (protocol.Signer, error) {
	signer, ok := account.(protocol.Signer)
	if !ok {
		return nil, errors.Format(errors.StatusBadRequest, "invalid signer: %v cannot sign transactions", account.Type())
	}

	return signer, nil
}

func getForwardedSigner(fwdsig *protocol.ForwardedSignature, signerUrl *url.URL) (protocol.Signer, error) {
	account, ok := fwdsig.Signer(signerUrl)
	if ok {
		return asSigner(account)
	}

	return nil, errors.NotFound("forwarded signature: missing signer %v", signerUrl)
}

func loadSignerLocally(batch *database.Batch, signerUrl *url.URL) (protocol.Signer, error) {
	account, err := batch.Account(signerUrl).GetState()
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}

	return asSigner(account)
}

// validateSignature verifies that the signature matches the signer state.
func validateSignature(transaction *protocol.Transaction, signer protocol.Signer, signature protocol.Signature) (protocol.KeyEntry, error) {
	// Check the height
	if signature.GetSignerVersion() != signer.GetVersion() {
		return nil, protocol.Errorf(protocol.ErrorCodeBadVersion, "invalid version: have %d, got %d", signer.GetVersion(), signature.GetSignerVersion())
	}

	// Find the key entry
	_, entry, ok := signer.EntryByKeyHash(signature.GetPublicKeyHash())
	if !ok {
		return nil, fmt.Errorf("key does not belong to signer")
	}

	// Check the timestamp, except for faucet transactions
	if transaction.Body.Type() != protocol.TransactionTypeAcmeFaucet &&
		signature.GetTimestamp() != 0 &&
		entry.GetLastUsedOn() >= signature.GetTimestamp() {
		return nil, protocol.Errorf(protocol.ErrorCodeBadNonce, "invalid timestamp: have %d, got %d", entry.GetLastUsedOn(), signature.GetTimestamp())
	}

	return entry, nil
}

// verifySignerCanSign verifies that the signer is allowed to sign the transaction
func verifySignerCanSign(transaction *protocol.Transaction, signer protocol.Signer) error {
	switch signer := signer.(type) {
	case *protocol.LiteIdentity:
		// A lite token account is only allowed to sign for itself
		if !signer.Url.Equal(transaction.Header.Principal.RootIdentity()) {
			return errors.Format(errors.StatusUnauthorized, "%v is not authorized to sign transactions for %v", signer.Url, transaction.Header.Principal)
		}

	case *protocol.KeyPage:
		// Verify that the key page is allowed to sign the transaction
		bit, ok := transaction.Body.Type().AllowedTransactionBit()
		if ok && signer.TransactionBlacklist.IsSet(bit) {
			return errors.Format(errors.StatusUnauthorized, "page %s is not authorized to sign %v", signer.Url, transaction.Body.Type())
		}

	default:
		// This should never happen
		return errors.Format(errors.StatusInternalError, "unknown signer type %v", signer.Type())
	}

	return nil
}

// verifyPageIsAuthorized verifies that the key page is authorized to sign for
// the principal.
func verifyPageIsAuthorized(batch *database.Batch, transaction *protocol.Transaction, signer *protocol.KeyPage) error {
	// Load the principal
	principal, err := batch.Account(transaction.Header.Principal).GetState()
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	// Get the principal's account auth
	auth, err := getAccountAuth(batch, principal)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	// Get the signer book URL
	signerBook, _, ok := protocol.ParseKeyPageUrl(signer.Url)
	if !ok {
		// If this happens, the database has bad data
		return errors.Format(errors.StatusInternalError, "invalid key page URL: %v", signer.Url)
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

// computeSignerFee computes the fee that will be charged to the signer.
//
// If the signature is the initial signature, the fee is the base transaction
// fee + signature data surcharge + transaction data surcharge.
//
// Otherwise, the fee is the base signature fee + signature data surcharge.
func computeSignerFee(transaction *protocol.Transaction, signature protocol.Signature, isInitiator bool) (protocol.Fee, error) {
	// Don't charge fees for internal administrative functions
	signer := signature.GetSigner()
	_, isBvn := protocol.ParseBvnUrl(signer)
	if isBvn || protocol.IsDnUrl(signer) {
		return 0, nil
	}

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
func validateNormalSignature(batch *database.Batch, delivery *chain.Delivery, signature protocol.Signature, isInitiator bool) error {
	if !delivery.Transaction.Body.Type().IsUser() {
		// TODO Check the key
		return nil
	}

	// Validate the signer
	signers, err := validateSigners(batch, delivery.Transaction, signature)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	// If the signer is remote, don't validate
	if !signers[0].GetUrl().LocalTo(signature.RoutingLocation()) {
		return nil
	}

	// Load the signer and validate the signature against it
	_, err = validateSignature(delivery.Transaction, signers[0], signature)
	if err != nil {
		return err
	}

	// Ensure the signer has sufficient credits for the fee
	fee, err := computeSignerFee(delivery.Transaction, signature, isInitiator)
	if err != nil {
		return fmt.Errorf("calculating fee: %w", err)
	}
	if !signers[0].CanDebitCredits(fee.AsUInt64()) {
		return errors.Format(errors.StatusInsufficientCredits, "insufficient credits: have %s, want %s",
			protocol.FormatAmount(signers[0].GetCreditBalance(), protocol.CreditPrecisionPower),
			protocol.FormatAmount(fee.AsUInt64(), protocol.CreditPrecisionPower))
	}

	return nil
}

func processSigners(batch *database.Batch, transaction *protocol.Transaction, signature protocol.Signature) ([]protocol.Signer, error) {
	signers, err := validateSigners(batch, transaction, signature)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}
	if !transaction.Header.Principal.LocalTo(signature.RoutingLocation()) {
		return signers, nil
	}

	record := batch.Transaction(transaction.GetHash())
	status, err := record.GetStatus()
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}

	// Add all signers to the signer list so that the transaction readiness
	// check knows to look for delegates
	for _, signer := range signers {
		status.AddSigner(signer)
	}
	err = record.PutStatus(status)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}

	return signers, nil
}

// processNormalSignature validates a private key signature and updates the
// signer.
func processNormalSignature(batch *database.Batch, delivery *chain.Delivery, signature protocol.Signature, isInitiator bool) ([]protocol.Signer, error) {
	// Skip for non-user transactions
	if !delivery.Transaction.Body.Type().IsUser() {
		// TODO Check the key
		return nil, nil
	}

	// Validate the signer. This should not fail, because this signature has
	// presumably already passed ValidateEnvelope. But defensive programming is
	// always a good idea.
	signers, err := processSigners(batch, delivery.Transaction, signature)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}

	// If the signer is remote, don't validate
	if !signers[0].GetUrl().LocalTo(signature.RoutingLocation()) {
		return signers, nil
	}

	// TODO If the forwarded signature paid the full fee unnecessarily, refund
	// it

	// Validate the signature against the signer. This should also not fail.
	entry, err := validateSignature(delivery.Transaction, signers[0], signature)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}

	// Charge the fee
	fee, err := computeSignerFee(delivery.Transaction, signature, isInitiator)
	if err != nil {
		return nil, errors.Format(errors.StatusBadRequest, "calculating fee: %w", err)
	}
	if !signers[0].DebitCredits(fee.AsUInt64()) {
		return nil, errors.Format(errors.StatusInsufficientCredits, "insufficient credits: have %s, want %s",
			protocol.FormatAmount(signers[0].GetCreditBalance(), protocol.CreditPrecisionPower),
			protocol.FormatAmount(fee.AsUInt64(), protocol.CreditPrecisionPower))
	}

	// Update the timestamp - the value is validated by validateSignature
	if signature.GetTimestamp() != 0 {
		entry.SetLastUsedOn(signature.GetTimestamp())
	}

	// Store changes to the signer
	err = batch.Account(signature.GetSigner()).PutState(signers[0])
	if err != nil {
		return nil, errors.Format(errors.StatusUnknown, "store signer: %w", err)
	}

	return signers, nil
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

func verifyReceiptSignature(_ *config.Network, _ *database.Batch, transaction *protocol.Transaction, _ *protocol.ReceiptSignature, isInitiator bool) error {
	if !transaction.Body.Type().IsSynthetic() {
		return fmt.Errorf("receipt signatures are not allowed for non-synthetic transactions")
	}

	if isInitiator {
		return fmt.Errorf("receipt signatures must not be the initiator")
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

			if state.Signature == nil {
				// This should not happen
				continue
			}

			if protocol.SignatureDidInitiate(state.Signature, txnInitHash) {
				signatures[0] = state.Signature
			} else {
				signatures = append(signatures, state.Signature)
			}
		}
	}

	if signatures[0] == nil {
		signatures = signatures[1:]
	}

	return signatures, nil
}
