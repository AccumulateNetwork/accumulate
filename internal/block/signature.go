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

func (x *Executor) ProcessSignature(batch *database.Batch, delivery *chain.Delivery, signature protocol.Signature) (*ProcessSignatureState, error) {
	r := x.BlockTimers.Start(BlockTimerTypeProcessSignature)
	defer x.BlockTimers.Stop(r)

	err := x.checkRouting(delivery, signature)
	if err != nil {
		return nil, err
	}

	// Is this the initial signature?
	initiated, err := hasBeenInitiated(batch, delivery.Transaction)
	if err != nil {
		return nil, err
	}

	var md sigExecMetadata
	md.Initiated = initiated
	if !signature.Type().IsSystem() {
		md.Location = signature.RoutingLocation()
	}
	_, err = x.processSignature(batch, delivery, signature, md)
	if err != nil {
		return nil, err
	}

	return &ProcessSignatureState{}, nil
}

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

type sigExecMetadata struct {
	Location  *url.URL
	Initiated bool
	Delegated bool
	Forwarded bool
}

func (d sigExecMetadata) SetDelegated() sigExecMetadata {
	e := d
	e.Delegated = true
	return e
}

func (d sigExecMetadata) SetForwarded() sigExecMetadata {
	e := d
	e.Forwarded = true
	return e
}

func (d sigExecMetadata) Nested() bool {
	return d.Delegated || d.Forwarded
}

func (x *Executor) processSignature(batch *database.Batch, delivery *chain.Delivery, signature protocol.Signature, md sigExecMetadata) (protocol.Signer, error) {
	var signer, delegate protocol.Signer
	var err error
	switch signature := signature.(type) {
	case *protocol.SyntheticSignature:
		err = verifySyntheticSignature(&x.Describe, batch, delivery.Transaction, signature, md)

	case *protocol.ReceiptSignature:
		err = verifyReceiptSignature(delivery.Transaction, signature, md)

	case *protocol.InternalSignature:
		err = verifyInternalSignature(delivery, signature, md)

	case *protocol.SignatureSet:
		if !delivery.IsForwarded() {
			return nil, errors.New(errors.StatusBadRequest, "a signature set is not allowed outside of a forwarded transaction")
		}
		if !md.Forwarded {
			return nil, errors.New(errors.StatusBadRequest, "a signature set must be nested within another signature")
		}
		signer, err = x.processSigner(batch, delivery.Transaction, signature, md.Location, false)

		// Do not store anything if the set is within a delegated transaction
		if md.Delegated {
			return signer, nil
		}

	case *protocol.RemoteSignature:
		if md.Nested() {
			return nil, errors.New(errors.StatusBadRequest, "a remote signature cannot be nested within another signature")
		}
		if !delivery.IsForwarded() {
			return nil, errors.New(errors.StatusBadRequest, "a remote signature is not allowed outside of a forwarded transaction")
		}
		return x.processSignature(batch, delivery, signature.Signature, md.SetForwarded())

	case *protocol.DelegatedSignature:
		delegate, err = x.processSignature(batch, delivery, signature.Signature, md.SetDelegated())
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
		if signature.Type() != protocol.SignatureTypeReceipt && !signature.Verify(delivery.Transaction.GetHash()) {
			return nil, errors.Format(errors.StatusBadRequest, "invalid signature")
		}

		signer, err = x.processKeySignature(batch, delivery, signature, md.Location, !md.Initiated, !md.Delegated && delivery.Transaction.Header.Principal.LocalTo(md.Location))

		// Do not store anything if the set is within a forwarded delegated transaction
		if md.Forwarded && md.Delegated {
			return signer, nil
		}

	default:
		return nil, fmt.Errorf("unknown signature type %v", signature.Type())
	}
	if err != nil {
		return nil, err
	}

	err = validateInitialSignature(delivery.Transaction, signature, md)
	if err != nil {
		return nil, err
	}

	isSystemSig := signature.Type().IsSystem()
	isUserTxn := delivery.Transaction.Body.Type().IsUser() && !delivery.WasProducedInternally()
	if !isUserTxn {
		if isSystemSig {
			signer, err = loadSigner(batch, x.Describe.OperatorsPage())
		} else {
			signer, err = loadSigner(batch, signature.GetSigner())
		}
		switch {
		case err == nil:
			// Ok
		case errors.Is(err, errors.StatusNotFound):
			signer = &protocol.UnknownSigner{Url: signature.GetSigner()}
		default:
			return nil, err
		}
	}

	// Store the transaction state (without signatures) if it is local to the
	// signature, unless the body is just a hash
	isLocalTxn := !isSystemSig && delivery.Transaction.Header.Principal.LocalTo(md.Location)
	if isLocalTxn && delivery.Transaction.Body.Type() != protocol.TransactionTypeRemote {
		err = batch.Transaction(delivery.Transaction.GetHash()).
			PutState(&database.SigOrTxn{Transaction: delivery.Transaction})
		if err != nil {
			return nil, fmt.Errorf("store transaction: %w", err)
		}
	}

	// If the signature is a local delegated signature, check that the delegate
	// is satisfied, and store the full signature set
	sigToStore := signature
	if delegated, ok := signature.(*protocol.DelegatedSignature); ok && delegate.GetUrl().LocalTo(md.Location) {
		// Check if the signer is ready
		status, err := batch.Transaction(delivery.Transaction.GetHash()).GetStatus()
		if err != nil {
			return nil, errors.Format(errors.StatusUnknown, "load transaction status: %w", err)
		}
		ready, err := x.SignerIsSatisfied(batch, delivery.Transaction, status, delegate)
		if err != nil {
			return nil, errors.Wrap(errors.StatusUnknown, err)
		}
		if !ready {
			return signer, nil
		}

		// Load all the signatures
		sigset, err := GetSignaturesForSigner(batch, batch.Transaction(delivery.Transaction.GetHash()), delegate)
		if err != nil {
			return nil, errors.Wrap(errors.StatusUnknown, err)
		}

		set := new(protocol.SignatureSet)
		set.Vote = protocol.VoteTypeAccept
		set.Signer = signer.GetUrl()
		set.TransactionHash = *(*[32]byte)(delivery.Transaction.GetHash())
		set.Signatures = sigset
		signature := delegated.Copy()
		signature.Signature = set
		sigToStore = signature
	}

	// Persist the signature
	env := new(database.SigOrTxn)
	env.Txid = delivery.Transaction.ID()
	env.Signature = sigToStore
	sigHash := signature.Hash()
	err = batch.Transaction(sigHash).PutState(env)
	if err != nil {
		return nil, fmt.Errorf("store envelope: %w", err)
	}

	// Add the signature to the signer's chain
	if isUserTxn && signer.GetUrl().LocalTo(md.Location) {
		chain, err := batch.Account(signer.GetUrl()).Chain(protocol.SignatureChain, protocol.ChainTypeTransaction)
		if err != nil {
			return nil, fmt.Errorf("load chain: %w", err)
		}
		err = chain.AddEntry(sigHash, true)
		if err != nil {
			return nil, fmt.Errorf("store chain: %w", err)
		}
	}

	// Add the signature to the principal's chain
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

	// Add the signature to the transaction's signature set
	sigSet, err := batch.Transaction(delivery.Transaction.GetHash()).SignaturesForSigner(signer)
	if err != nil {
		return nil, fmt.Errorf("load signatures: %w", err)
	}

	var index int
	switch signature := signature.(type) {
	case *protocol.ReceiptSignature,
		*protocol.SyntheticSignature,
		*protocol.InternalSignature,
		*protocol.RemoteSignature,
		*protocol.SignatureSet:
		index = 0

	case *protocol.DelegatedSignature:
		index, _, _ = signer.EntryByDelegate(delegate.GetUrl())

	case protocol.KeySignature:
		index, _, _ = signer.EntryByKeyHash(signature.GetPublicKeyHash())

	default:
		return nil, fmt.Errorf("unknown signature type %v", signature.Type())
	}

	_, err = sigSet.Add(uint64(index), signature)
	if err != nil {
		return nil, fmt.Errorf("store signature: %w", err)
	}

	return signer, nil
}

// validateInitialSignature verifies that the signature is a valid initial
// signature for the transaction.
func validateInitialSignature(transaction *protocol.Transaction, signature protocol.Signature, md sigExecMetadata) error {
	// Do not check nested signatures or already initiated transactions
	if md.Nested() || md.Initiated {
		return nil
	}

	// Verify the initiator hash matches
	if !protocol.SignatureDidInitiate(signature, transaction.Header.Initiator[:]) {
		return protocol.Errorf(protocol.ErrorCodeInvalidSignature, "initiator signature does not match initiator hash")
	}

	// Timestamps are not used for system signatures
	keysig, ok := signature.(protocol.KeySignature)
	if !ok {
		return nil
	}

	// Require a timestamp for the initiator
	if keysig.GetTimestamp() == 0 {
		return protocol.Errorf(protocol.ErrorCodeInvalidRequest, "initial signature does not have a timestamp")
	}

	return nil
}

// validateSigner verifies that the signer is valid and authorized.
func (x *Executor) validateSigner(batch *database.Batch, transaction *protocol.Transaction, signerUrl, location *url.URL, checkAuthz bool) (protocol.Signer, error) {
	// If the user specifies a lite token address, convert it to a lite
	// identity
	if key, _, _ := protocol.ParseLiteTokenAddress(signerUrl); key != nil {
		signerUrl = signerUrl.RootIdentity()
	}

	var signer protocol.Signer
	var err error
	if !signerUrl.LocalTo(location) {
		signer = &protocol.UnknownSigner{Url: signerUrl}
	} else {
		signer, err = loadSigner(batch, signerUrl)
		if err != nil {
			return nil, errors.Wrap(errors.StatusUnknown, err)
		}
	}

	// Delegate to the transaction executor?
	val, ok := getValidator[chain.SignerValidator](x, transaction.Body.Type())
	if ok {
		fallback, err := val.SignerIsAuthorized(x, batch, transaction, signer, checkAuthz)
		if err != nil {
			return nil, errors.Wrap(errors.StatusUnknown, err)
		}
		if !fallback {
			return signer, nil
		}
	}

	// Do not check authorization for synthetic and system transactions
	if !transaction.Body.Type().IsUser() {
		return signer, nil
	}

	// Verify that the final signer is authorized
	err = x.SignerIsAuthorized(batch, transaction, signer, checkAuthz)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}

	return signer, nil
}

func loadSigner(batch *database.Batch, signerUrl *url.URL) (protocol.Signer, error) {
	// Load signer
	account, err := batch.Account(signerUrl).GetState()
	if err != nil {
		return nil, errors.Format(errors.StatusUnknown, "load signer: %w", err)
	}

	signer, ok := account.(protocol.Signer)
	if !ok {
		return nil, errors.Format(errors.StatusBadRequest, "invalid signer: %v cannot sign transactions", account.Type())
	}

	return signer, nil
}

// validateSignature verifies that the signature matches the signer state.
func validateSignature(transaction *protocol.Transaction, signer protocol.Signer, signature protocol.KeySignature) (protocol.KeyEntry, error) {
	// Check the height
	if transaction.Body.Type().IsUser() && signature.GetSignerVersion() != signer.GetVersion() {
		return nil, errors.Format(errors.StatusBadSignerVersion, "invalid version: have %d, got %d", signer.GetVersion(), signature.GetSignerVersion())
	}

	// Find the key entry
	_, entry, ok := signer.EntryByKeyHash(signature.GetPublicKeyHash())
	if !ok {
		return nil, errors.New(errors.StatusUnauthorized, "key does not belong to signer")
	}

	// Check the timestamp for user transactions, except for faucet transactions
	if transaction.Body.Type() != protocol.TransactionTypeAcmeFaucet &&
		transaction.Body.Type().IsUser() &&
		signature.GetTimestamp() != 0 &&
		entry.GetLastUsedOn() >= signature.GetTimestamp() {
		return nil, errors.Format(errors.StatusBadTimestamp, "invalid timestamp: have %d, got %d", entry.GetLastUsedOn(), signature.GetTimestamp())
	}

	return entry, nil
}

// SignerIsAuthorized verifies that the signer is allowed to sign the transaction
func (x *Executor) SignerIsAuthorized(batch *database.Batch, transaction *protocol.Transaction, signer protocol.Signer, checkAuthz bool) error {
	switch signer := signer.(type) {
	case *protocol.LiteIdentity:
		// Otherwise a lite token account is only allowed to sign for itself
		if !signer.Url.Equal(transaction.Header.Principal.RootIdentity()) {
			return errors.Format(errors.StatusUnauthorized, "%v is not authorized to sign transactions for %v", signer.Url, transaction.Header.Principal)
		}

		return nil

	case *protocol.KeyPage:
		// Verify that the key page is allowed to sign the transaction
		bit, ok := transaction.Body.Type().AllowedTransactionBit()
		if ok && signer.TransactionBlacklist.IsSet(bit) {
			return errors.Format(errors.StatusUnauthorized, "page %s is not authorized to sign %v", signer.Url, transaction.Body.Type())
		}

		if !checkAuthz {
			return nil
		}

	case *protocol.UnknownSigner:
		if !checkAuthz {
			return nil
		}

	default:
		// This should never happen
		return errors.Format(errors.StatusInternalError, "unknown signer type %v", signer.Type())
	}

	err := x.verifyPageIsAuthorized(batch, transaction, signer)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	return nil
}

// verifyPageIsAuthorized verifies that the key page is authorized to sign for
// the principal.
func (x *Executor) verifyPageIsAuthorized(batch *database.Batch, transaction *protocol.Transaction, signer protocol.Signer) error {
	// Load the principal
	principal, err := batch.Account(transaction.Header.Principal).GetState()
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	// Get the principal's account auth
	auth, err := x.GetAccountAuthoritySet(batch, principal)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	// Get the signer book URL
	signerBook, _, ok := protocol.ParseKeyPageUrl(signer.GetUrl())
	if !ok {
		// If this happens, the database has bad data
		return errors.Format(errors.StatusInternalError, "invalid key page URL: %v", signer.GetUrl())
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
		return protocol.Errorf(protocol.ErrorCodeUnauthorized, "%v is not authorized to sign transactions for %v", signer.GetUrl(), principal.GetUrl())
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
	_, isBvn := protocol.ParseSubnetUrl(signer)
	if isBvn || protocol.IsDnUrl(signer) {
		return 0, nil
	}

	// Compute the signature fee
	fee, err := protocol.ComputeSignatureFee(signature)
	if err != nil {
		return 0, errors.Wrap(errors.StatusUnknown, err)
	}
	if !isInitiator {
		return fee, nil
	}

	// Add the transaction fee for the initial signature
	txnFee, err := protocol.ComputeTransactionFee(transaction)
	if err != nil {
		return 0, errors.Wrap(errors.StatusUnknown, err)
	}

	// Subtract the base signature fee, but not the oversize surcharge if there is one
	fee += txnFee - protocol.FeeSignature
	return fee, nil
}

// validateKeySignature validates a private key signature.
func (x *Executor) validateKeySignature(batch *database.Batch, delivery *chain.Delivery, signature protocol.KeySignature, isInitiator, checkAuthz bool) (protocol.Signer, error) {
	// Validate the signer
	signer, err := x.validateSigner(batch, delivery.Transaction, signature.GetSigner(), signature.RoutingLocation(), checkAuthz)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}

	// Load the signer and validate the signature against it
	_, err = validateSignature(delivery.Transaction, signer, signature)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}

	// Do not charge fees for synthetic transactions
	if !delivery.Transaction.Body.Type().IsUser() {
		return signer, nil
	}

	// Ensure the signer has sufficient credits for the fee
	fee, err := computeSignerFee(delivery.Transaction, signature, isInitiator)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}
	if !signer.CanDebitCredits(fee.AsUInt64()) {
		return nil, errors.Format(errors.StatusInsufficientCredits, "%v has insufficient credits: have %s, want %s", signer.GetUrl(),
			protocol.FormatAmount(signer.GetCreditBalance(), protocol.CreditPrecisionPower),
			protocol.FormatAmount(fee.AsUInt64(), protocol.CreditPrecisionPower))
	}

	return signer, nil
}

func (x *Executor) processSigner(batch *database.Batch, transaction *protocol.Transaction, signature protocol.Signature, location *url.URL, checkAuthz bool) (protocol.Signer, error) {
	signer, err := x.validateSigner(batch, transaction, signature.GetSigner(), location, checkAuthz)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}
	if !transaction.Header.Principal.LocalTo(location) {
		return signer, nil
	}

	record := batch.Transaction(transaction.GetHash())
	status, err := record.GetStatus()
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}

	// Add all signers to the signer list so that the transaction readiness
	// check knows to look for delegates
	status.AddSigner(signer)
	err = record.PutStatus(status)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}

	return signer, nil
}

// processKeySignature validates a private key signature and updates the
// signer.
func (x *Executor) processKeySignature(batch *database.Batch, delivery *chain.Delivery, signature protocol.KeySignature, location *url.URL, isInitiator, checkAuthz bool) (protocol.Signer, error) {
	// Validate the signer and/or delegator. This should not fail, because this
	// signature has presumably already passed ValidateEnvelope. But defensive
	// programming is always a good idea.
	signer, err := x.processSigner(batch, delivery.Transaction, signature, location, checkAuthz)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}

	// TODO If the forwarded signature paid the full fee unnecessarily, refund
	// it

	// Validate the signature against the signer. This should also not fail.
	entry, err := validateSignature(delivery.Transaction, signer, signature)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}

	// Do not charge fees or update the nonce for synthetic transactions
	if !delivery.Transaction.Body.Type().IsUser() {
		return signer, nil
	}

	// Charge the fee
	fee, err := computeSignerFee(delivery.Transaction, signature, isInitiator)
	if err != nil {
		return nil, errors.Format(errors.StatusBadRequest, "calculating fee: %w", err)
	}
	if !signer.DebitCredits(fee.AsUInt64()) {
		return nil, errors.Format(errors.StatusInsufficientCredits, "%v has insufficient credits: have %s, want %s", signer.GetUrl(),
			protocol.FormatAmount(signer.GetCreditBalance(), protocol.CreditPrecisionPower),
			protocol.FormatAmount(fee.AsUInt64(), protocol.CreditPrecisionPower))
	}

	// Update the timestamp - the value is validated by validateSignature
	if signature.GetTimestamp() != 0 {
		entry.SetLastUsedOn(signature.GetTimestamp())
	}

	// Store changes to the signer
	err = batch.Account(signer.GetUrl()).PutState(signer)
	if err != nil {
		return nil, errors.Format(errors.StatusUnknown, "store signer: %w", err)
	}

	return signer, nil
}

func verifySyntheticSignature(net *config.Describe, _ *database.Batch, transaction *protocol.Transaction, signature *protocol.SyntheticSignature, md sigExecMetadata) error {
	if md.Nested() {
		return errors.New(errors.StatusBadRequest, "a synthetic signature cannot be nested within another signature")
	}
	if !transaction.Body.Type().IsSynthetic() && !transaction.Body.Type().IsSystem() {
		return fmt.Errorf("synthetic or system signatures are not allowed for non-synthetic or non-system transactions")
	}

	// if !isInitiator {
	// 	return fmt.Errorf("synthetic signatures must be the initiator")
	// }

	if !net.NodeUrl().Equal(signature.DestinationNetwork) {
		return fmt.Errorf("wrong destination network: %v is not this network", signature.DestinationNetwork)
	}

	return nil
}

func verifyReceiptSignature(transaction *protocol.Transaction, receipt *protocol.ReceiptSignature, md sigExecMetadata) error {
	if md.Nested() {
		return errors.New(errors.StatusBadRequest, "a receipt signature cannot be nested within another signature")
	}

	if !transaction.Body.Type().IsSynthetic() && !transaction.Body.Type().IsSystem() {
		return fmt.Errorf("receipt signatures are not allowed for non-synthetic or non-system transactions")
	}

	if !md.Initiated {
		return fmt.Errorf("receipt signatures must not be the initiator")
	}

	if !receipt.Proof.Validate() {
		return fmt.Errorf("invalid receipt")
	}

	return nil
}

func verifyInternalSignature(delivery *chain.Delivery, _ *protocol.InternalSignature, md sigExecMetadata) error {
	if md.Nested() {
		return errors.New(errors.StatusBadRequest, "an internal signature cannot be nested within another signature")
	}

	if !delivery.WasProducedInternally() {
		return errors.New(errors.StatusBadRequest, "an internal signature can only be used for transactions produced by a system transaction")
	}

	if delivery.IsForwarded() {
		return errors.New(errors.StatusBadRequest, "an internal signature cannot be forwarded")
	}

	return nil
}

func GetAllSignatures(batch *database.Batch, transaction *database.Transaction, status *protocol.TransactionStatus, txnInitHash []byte) ([]protocol.Signature, error) {
	signatures := make([]protocol.Signature, 1)

	for _, signer := range status.Signers {
		sigset, err := GetSignaturesForSigner(batch, transaction, signer)
		if err != nil {
			return nil, errors.Wrap(errors.StatusUnknown, err)
		}

		for _, sig := range sigset {
			if protocol.SignatureDidInitiate(sig, txnInitHash) {
				signatures[0] = sig
			} else {
				signatures = append(signatures, sig)
			}
		}
	}

	if signatures[0] == nil {
		signatures = signatures[1:]
	}

	return signatures, nil
}

func GetSignaturesForSigner(batch *database.Batch, transaction *database.Transaction, signer protocol.Signer) ([]protocol.Signature, error) {
	// Load the signature set
	sigset, err := transaction.ReadSignaturesForSigner(signer)
	if err != nil {
		return nil, fmt.Errorf("load signatures set %v: %w", signer.GetUrl(), err)
	}

	entries := sigset.Entries()
	signatures := make([]protocol.Signature, 0, len(entries))
	for _, e := range sigset.Entries() {
		state, err := batch.Transaction(e.SignatureHash[:]).GetState()
		if err != nil {
			return nil, fmt.Errorf("load signature entry %X: %w", e.SignatureHash, err)
		}

		if state.Signature == nil {
			// This should not happen
			continue
		}

		signatures = append(signatures, state.Signature)
	}
	return signatures, nil
}
