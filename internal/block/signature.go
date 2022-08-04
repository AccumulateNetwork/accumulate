package block

import (
	"bytes"
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

	var md sigExecMetadata
	md.IsInitiator = protocol.SignatureDidInitiate(signature, delivery.Transaction.Header.Initiator[:])
	if !signature.Type().IsSystem() {
		md.Location = signature.RoutingLocation()
	}
	_, err = x.processSignature(batch, delivery, signature, md)
	if err != nil {
		return nil, err
	}

	return &ProcessSignatureState{}, nil
}

type sigExecMetadata struct {
	Location    *url.URL
	IsInitiator bool
	Delegated   bool
	Forwarded   bool
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
	case *protocol.PartitionSignature:
		err = verifyPartitionSignature(&x.Describe, batch, delivery.Transaction, signature, md)
		if err != nil {
			return nil, err
		}

	case *protocol.ReceiptSignature:
		err = verifyReceiptSignature(delivery.Transaction, signature, md)
		if err != nil {
			return nil, err
		}

	case *protocol.InternalSignature:
		err = verifyInternalSignature(delivery, signature, md)
		if err != nil {
			return nil, err
		}

	case *protocol.SignatureSet:
		if !delivery.IsForwarded() {
			return nil, errors.New(errors.StatusBadRequest, "a signature set is not allowed outside of a forwarded transaction")
		}
		if !md.Forwarded {
			return nil, errors.New(errors.StatusBadRequest, "a signature set must be nested within another signature")
		}
		signer, err = x.processSigner(batch, delivery.Transaction, signature, md, !md.Delegated && md.Location.LocalTo(delivery.Transaction.Header.Principal))
		if err != nil {
			return nil, err
		}

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
		if !md.Nested() && !signature.Verify(signature.Metadata().Hash(), delivery.Transaction.GetHash()) {
			return nil, errors.Format(errors.StatusBadRequest, "invalid signature")
		}

		if !signature.Delegator.LocalTo(md.Location) {
			return nil, nil
		}

		// Validate the delegator
		signer, err = x.validateSigner(batch, delivery.Transaction, signature.Delegator, md.Location, false)
		if err != nil {
			return nil, errors.Wrap(errors.StatusUnknownError, err)
		}

		// Verify delegation
		_, _, ok := signer.EntryByDelegate(delegate.GetUrl())
		if !ok {
			return nil, errors.Format(errors.StatusUnauthorized, "%v is not authorized to sign for %v", delegate.GetUrl(), signature.Delegator)
		}

	case protocol.KeySignature:
		// Basic validation
		if !md.Nested() && !signature.Verify(nil, delivery.Transaction.GetHash()) {
			return nil, errors.Format(errors.StatusBadRequest, "invalid signature")
		}

		if !delivery.Transaction.Body.Type().IsUser() {
			err = x.validatePartitionSignature(md.Location, signature, delivery.Transaction)
			if err != nil {
				return nil, errors.Wrap(errors.StatusUnknownError, err)
			}
		}

		signer, err = x.processKeySignature(batch, delivery, signature, md, !md.Delegated && delivery.Transaction.Header.Principal.LocalTo(md.Location))
		if err != nil {
			return nil, err
		}

		// Do not store anything if the set is within a forwarded delegated transaction
		if md.Forwarded && md.Delegated {
			return signer, nil
		}

	default:
		return nil, fmt.Errorf("unknown signature type %v", signature.Type())
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

	var statusDirty bool
	status, err := batch.Transaction(delivery.Transaction.GetHash()).GetStatus()
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "load transaction status: %w", err)
	}

	sigToStore := signature
	switch signature := signature.(type) {
	case *protocol.PartitionSignature:
		// Capture the source, destination, and sequence number in the status
		statusDirty = true
		status.SourceNetwork = signature.SourceNetwork
		status.DestinationNetwork = signature.DestinationNetwork
		status.SequenceNumber = signature.SequenceNumber

	case *protocol.ReceiptSignature:
		statusDirty = true
		if signature.SourceNetwork.Equal(protocol.DnUrl()) {
			status.GotDirectoryReceipt = true
		}

		// Capture the initial receipt
		if status.Proof == nil {
			if !bytes.Equal(delivery.Transaction.GetHash(), signature.Proof.Start) {
				return nil, errors.Format(errors.StatusUnauthorized, "receipt does not match transaction")
			}
			status.Proof = &signature.Proof
			break
		}

		if status.Proof.Contains(&signature.Proof) {
			// We already have the proof, nothing to do
			break
		}

		// Capture subsequent receipts
		status.Proof, err = status.Proof.Combine(&signature.Proof)
		if err != nil {
			return nil, errors.Format(errors.StatusUnauthorized, "combine receipts: %w", err)
		}

	case *protocol.DelegatedSignature:
		// If the signature is a local delegated signature, check that the delegate
		// is satisfied, and store the full signature set
		if !delegate.GetUrl().LocalTo(md.Location) {
			break
		}

		// Check if the signer is ready
		ready, err := x.SignerIsSatisfied(batch, delivery.Transaction, status, delegate)
		if err != nil {
			return nil, errors.Wrap(errors.StatusUnknownError, err)
		}
		if !ready {
			return signer, nil
		}

		// Load all the signatures
		sigset, err := GetSignaturesForSigner(batch, batch.Transaction(delivery.Transaction.GetHash()), delegate)
		if err != nil {
			return nil, errors.Wrap(errors.StatusUnknownError, err)
		}

		set := new(protocol.SignatureSet)
		set.Vote = protocol.VoteTypeAccept
		set.Signer = signer.GetUrl()
		set.TransactionHash = *(*[32]byte)(delivery.Transaction.GetHash())
		set.Signatures = sigset
		signature = signature.Copy()
		signature.Signature = set
		sigToStore = signature
	}

	// Record the initiator
	if md.IsInitiator {
		var initUrl *url.URL
		if signature.Type().IsSystem() {
			initUrl = signer.GetUrl()
		} else {
			initUrl = signature.GetSigner()
			if key, _, _ := protocol.ParseLiteTokenAddress(initUrl); key != nil {
				initUrl = initUrl.RootIdentity()
			}
		}
		if status.Initiator != nil && !status.Initiator.Equal(initUrl) {
			// This should be impossible
			return nil, errors.Format(errors.StatusInternalError, "initiator is already set and does not match the signature")
		}

		statusDirty = true
		status.Initiator = initUrl
	}

	if statusDirty {
		err = batch.Transaction(delivery.Transaction.GetHash()).PutStatus(status)
		if err != nil {
			return nil, errors.Wrap(errors.StatusUnknownError, err)
		}
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
		chain, err := batch.Account(signer.GetUrl()).SignatureChain().Get()
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
		chain, err := batch.Account(delivery.Transaction.Header.Principal).SignatureChain().Get()
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
		*protocol.PartitionSignature,
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
func validateInitialSignature(_ *protocol.Transaction, signature protocol.Signature, md sigExecMetadata) error {
	if !md.IsInitiator {
		return nil
	}

	// Timestamps are not used for system signatures
	keysig, ok := signature.(protocol.KeySignature)
	if !ok {
		return nil
	}

	// Require a timestamp for the initiator
	if keysig.GetTimestamp() == 0 {
		return errors.Format(errors.StatusBadTimestamp, "initial signature does not have a timestamp")
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
			return nil, errors.Wrap(errors.StatusUnknownError, err)
		}
	}

	// Delegate to the transaction executor?
	val, ok := getValidator[chain.SignerValidator](x, transaction.Body.Type())
	if ok {
		fallback, err := val.SignerIsAuthorized(x, batch, transaction, signer, checkAuthz)
		if err != nil {
			return nil, errors.Wrap(errors.StatusUnknownError, err)
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
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}

	return signer, nil
}

func loadSigner(batch *database.Batch, signerUrl *url.URL) (protocol.Signer, error) {
	// Load signer
	account, err := batch.Account(signerUrl).GetState()
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "load signer: %w", err)
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
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	return nil
}

// verifyPageIsAuthorized verifies that the key page is authorized to sign for
// the principal.
func (x *Executor) verifyPageIsAuthorized(batch *database.Batch, transaction *protocol.Transaction, signer protocol.Signer) error {
	// Load the principal
	principal, err := batch.Account(transaction.Header.Principal).GetState()
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "load principal: %w", err)
	}

	// Get the principal's account auth
	auth, err := x.GetAccountAuthoritySet(batch, principal)
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	// Get the signer book URL
	signerBook, _, ok := protocol.ParseKeyPageUrl(signer.GetUrl())
	if !ok {
		// If this happens, the database has bad data
		return errors.Format(errors.StatusInternalError, "invalid key page URL: %v", signer.GetUrl())
	}

	// Page belongs to book => authorized
	_, foundAuthority := auth.GetAuthority(signerBook)
	if foundAuthority {
		return nil
	}

	// Authorization is disabled and the transaction type does not force authorization => authorized
	if auth.AuthDisabled() && !transaction.Body.Type().RequireAuthorization() {
		return nil
	}

	// Authorization is enabled => unauthorized
	// Transaction type forces authorization => unauthorized
	return errors.Format(errors.StatusUnauthorized, "%v is not authorized to sign transactions for %v", signer.GetUrl(), principal.GetUrl())
}

// computeSignerFee computes the fee that will be charged to the signer.
//
// If the signature is the initial signature, the fee is the base transaction
// fee + signature data surcharge + transaction data surcharge.
//
// Otherwise, the fee is the base signature fee + signature data surcharge.
func computeSignerFee(transaction *protocol.Transaction, signature protocol.KeySignature, md sigExecMetadata) (protocol.Fee, error) {
	// Don't charge fees for internal administrative functions
	signer := signature.GetSigner()
	_, isBvn := protocol.ParsePartitionUrl(signer)
	if isBvn || protocol.IsDnUrl(signer) {
		return 0, nil
	}

	// Compute the signature fee
	fee, err := protocol.ComputeSignatureFee(signature)
	if err != nil {
		return 0, errors.Wrap(errors.StatusUnknownError, err)
	}

	// Only charge the transaction fee for the initial signature
	if !md.IsInitiator {
		return fee, nil
	}

	// Add the transaction fee for the initial signature
	txnFee, err := protocol.ComputeTransactionFee(transaction)
	if err != nil {
		return 0, errors.Wrap(errors.StatusUnknownError, err)
	}

	// Subtract the base signature fee, but not the oversize surcharge if there is one
	fee += txnFee - protocol.FeeSignature
	return fee, nil
}

// validateKeySignature validates a private key signature.
func (x *Executor) validateKeySignature(batch *database.Batch, delivery *chain.Delivery, signature protocol.KeySignature, md sigExecMetadata, checkAuthz bool) (protocol.Signer, error) {
	// Validate the signer
	signer, err := x.validateSigner(batch, delivery.Transaction, signature.GetSigner(), signature.RoutingLocation(), checkAuthz)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}

	// Load the signer and validate the signature against it
	_, err = validateSignature(delivery.Transaction, signer, signature)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}

	// Do not charge fees for synthetic transactions
	if !delivery.Transaction.Body.Type().IsUser() {
		return signer, nil
	}

	// Ensure the signer has sufficient credits for the fee
	fee, err := computeSignerFee(delivery.Transaction, signature, md)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}
	if !signer.CanDebitCredits(fee.AsUInt64()) {
		return nil, errors.Format(errors.StatusInsufficientCredits, "%v has insufficient credits: have %s, want %s", signer.GetUrl(),
			protocol.FormatAmount(signer.GetCreditBalance(), protocol.CreditPrecisionPower),
			protocol.FormatAmount(fee.AsUInt64(), protocol.CreditPrecisionPower))
	}

	return signer, nil
}

func (x *Executor) processSigner(batch *database.Batch, transaction *protocol.Transaction, signature protocol.Signature, md sigExecMetadata, checkAuthz bool) (protocol.Signer, error) {
	signer, err := x.validateSigner(batch, transaction, signature.GetSigner(), md.Location, checkAuthz)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}
	if !transaction.Header.Principal.LocalTo(md.Location) {
		return signer, nil
	}

	record := batch.Transaction(transaction.GetHash())
	status, err := record.GetStatus()
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}

	// Add all signers to the signer list so that the transaction readiness
	// check knows to look for delegates
	status.AddSigner(signer)
	err = record.PutStatus(status)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}

	return signer, nil
}

// processKeySignature validates a private key signature and updates the
// signer.
func (x *Executor) processKeySignature(batch *database.Batch, delivery *chain.Delivery, signature protocol.KeySignature, md sigExecMetadata, checkAuthz bool) (protocol.Signer, error) {
	// Validate the signer and/or delegator. This should not fail, because this
	// signature has presumably already passed ValidateEnvelope. But defensive
	// programming is always a good idea.
	signer, err := x.processSigner(batch, delivery.Transaction, signature, md, checkAuthz)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}

	// TODO If the forwarded signature paid the full fee unnecessarily, refund
	// it

	// Validate the signature against the signer. This should also not fail.
	entry, err := validateSignature(delivery.Transaction, signer, signature)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}

	// Do not charge fees or update the nonce for synthetic transactions
	if !delivery.Transaction.Body.Type().IsUser() {
		return signer, nil
	}

	// Charge the fee
	fee, err := computeSignerFee(delivery.Transaction, signature, md)
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
		return nil, errors.Format(errors.StatusUnknownError, "store signer: %w", err)
	}

	return signer, nil
}

func verifyPartitionSignature(net *config.Describe, _ *database.Batch, transaction *protocol.Transaction, signature *protocol.PartitionSignature, md sigExecMetadata) error {
	if md.Nested() {
		return errors.New(errors.StatusBadRequest, "partition signatures cannot be nested within another signature")
	}
	if !transaction.Body.Type().IsSynthetic() && !transaction.Body.Type().IsSystem() {
		return fmt.Errorf("partition signatures are not valid for %v transactions", transaction.Body.Type())
	}

	if !md.IsInitiator {
		return fmt.Errorf("partition signatures must be the initiator")
	}

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
		return fmt.Errorf("receipt signatures are not valid for %v transactions", transaction.Body.Type())
	}

	if md.IsInitiator {
		return fmt.Errorf("receipt signatures must not be the initiator")
	}

	if !receipt.Proof.Validate() {
		return fmt.Errorf("invalid receipt")
	}

	return nil
}

func verifyInternalSignature(delivery *chain.Delivery, _ *protocol.InternalSignature, md sigExecMetadata) error {
	if md.Nested() {
		return errors.New(errors.StatusBadRequest, "internal signatures cannot be nested within another signature")
	}

	if !delivery.WasProducedInternally() {
		return errors.New(errors.StatusBadRequest, "internal signatures can only be used for transactions produced by a system transaction")
	}

	if delivery.IsForwarded() {
		return errors.New(errors.StatusBadRequest, "internal signatures cannot be forwarded")
	}

	return nil
}

func GetSignaturesForSigner(batch *database.Batch, transaction *database.Transaction, signer protocol.Signer) ([]protocol.Signature, error) {
	// Load the signature set
	sigset, err := transaction.ReadSignaturesForSigner(signer)
	if err != nil {
		return nil, fmt.Errorf("load signatures set %v: %w", signer.GetUrl(), err)
	}

	entries := sigset.Entries()
	signatures := make([]protocol.Signature, 0, len(entries))
	for _, e := range entries {
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

//validationPartitionSignature checks if the key used to sign the synthetic or system transaction belongs to the same subnet
func (x *Executor) validatePartitionSignature(location *url.URL, sig protocol.KeySignature, tx *protocol.Transaction) error {
	// TODO AC-1702 Use GetAllSignatures to determine the source
	var sigurl string
	var source *url.URL
	var err error
	skey := sig.GetPublicKey()

	switch txn := tx.Body.(type) {
	case protocol.SynthTxnWithOrigin:
		_, source = txn.GetCause()
	case *protocol.DirectoryAnchor:
		source = txn.Source
	case *protocol.BlockValidatorAnchor:
		source = txn.Source
	default:
		return nil
	}
	sigurl, err = x.Router.RouteAccount(source)

	if err != nil {
		return errors.Format(errors.StatusInternalError, "unable to resolve source of transaction %w", err)
	}
	subnet := x.globals.Active.Network.Partition(sigurl)
	if subnet == nil {
		return errors.Format(errors.StatusUnknownError, "unable to resolve originating subnet of the signature")
	}
	for _, vkey := range subnet.ValidatorKeys {
		if bytes.Equal(vkey, skey) {
			return nil
		}
	}
	return errors.Format(errors.StatusUnauthorized, "the key used to sign does not belong to the originating subnet")
}

func hasKeySignature(batch *database.Batch, status *protocol.TransactionStatus) (bool, error) {
	h := status.TxID.Hash()
	transaction := batch.Transaction(h[:])
	for _, signer := range status.Signers {
		// Load the signature set
		sigset, err := transaction.ReadSignaturesForSigner(signer)
		if err != nil {
			return false, fmt.Errorf("load signatures set %v: %w", signer.GetUrl(), err)
		}

		for _, e := range sigset.Entries() {
			state, err := batch.Transaction(e.SignatureHash[:]).GetState()
			if err != nil {
				return false, fmt.Errorf("load signature entry %X: %w", e.SignatureHash, err)
			}

			if state.Signature == nil {
				// This should not happen
				continue
			}

			if _, ok := state.Signature.(protocol.KeySignature); ok {
				return true, nil
			}
		}
	}

	return false, nil
}
