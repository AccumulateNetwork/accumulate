// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (x *Executor) processSignature2(batch *database.Batch, delivery *chain.Delivery, signature protocol.Signature) (*ProcessSignatureState, error) {
	r := x.BlockTimers.Start(BlockTimerTypeProcessSignature)
	defer x.BlockTimers.Stop(r)

	err := x.checkRouting(delivery, signature)
	if err != nil {
		return nil, err
	}

	var md sigExecMetadata
	md.IsInitiator = protocol.SignatureDidInitiate(signature, delivery.Transaction.Header.Initiator[:], nil)
	md.Location = signature.RoutingLocation()
	_, err = x.processSignature(batch, delivery, signature, md)
	if err != nil {
		return nil, err
	}

	return &ProcessSignatureState{}, nil
}

type sigExecMetadata = chain.SignatureValidationMetadata

func (x *Executor) processSignature(batch *database.Batch, delivery *chain.Delivery, signature protocol.Signature, md sigExecMetadata) (protocol.Signer2, error) {
	var signer protocol.Signer
	var delegate protocol.Signer
	var err error
	switch signature := signature.(type) {
	case *protocol.SignatureSet:
		if !delivery.IsForwarded() {
			return nil, errors.BadRequest.With("a signature set is not allowed outside of a forwarded transaction")
		}
		if !md.Forwarded {
			return nil, errors.BadRequest.With("a signature set must be nested within another signature")
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
			return nil, errors.BadRequest.With("a remote signature cannot be nested within another signature")
		}
		if !delivery.IsForwarded() {
			return nil, errors.BadRequest.With("a remote signature is not allowed outside of a forwarded transaction")
		}
		return x.processSignature(batch, delivery, signature.Signature, md.SetForwarded())

	case *protocol.DelegatedSignature:
		s, err := x.processSignature(batch, delivery, signature.Signature, md.SetDelegated())
		if err != nil {
			return nil, errors.UnknownError.WithFormat("process delegated signature: %w", err)
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
		signer, err = x.processKeySignature(batch, delivery, signature, md, !md.Delegated && delivery.Transaction.Header.Principal.LocalTo(md.Location))
		if err != nil {
			return nil, err
		}

		// Basic validation
		if !md.Nested() && !signature.Verify(nil, delivery.Transaction.GetHash()) {
			return nil, errors.BadRequest.WithFormat("invalid signature")
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

	var statusDirty bool
	status, err := batch.Transaction(delivery.Transaction.GetHash()).GetStatus()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load transaction status: %w", err)
	}

	sigToStore := signature
	var delegatedNotReady bool
	switch signature := signature.(type) {
	case *protocol.DelegatedSignature:
		// If the signature is a local delegated signature, check that the delegate
		// is satisfied, and store the full signature set
		if !delegate.GetUrl().LocalTo(md.Location) {
			break
		}

		// Check if the signer is ready
		ready, err := x.SignerIsSatisfied(batch, delivery.Transaction, status, delegate)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		if !ready {
			if x.globals.Active.ExecutorVersion.SignatureAnchoringEnabled() {
				delegatedNotReady = true
				break
			}
			return signer, nil
		}

		// Load all the signatures
		sigset, err := database.GetSignaturesForSigner(batch.Transaction(delivery.Transaction.GetHash()), delegate)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
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

	// Record the initiator (but only if we're at the final destination)
	shouldRecordInit := md.IsInitiator
	if md.Delegated {
		shouldRecordInit = false
	} else if !delivery.Transaction.Header.Principal.LocalTo(md.Location) {
		shouldRecordInit = false
	}
	if shouldRecordInit {
		initUrl := signer.GetUrl()
		if key, _, _ := protocol.ParseLiteTokenAddress(initUrl); key != nil {
			initUrl = initUrl.RootIdentity()
		}
		if status.Initiator != nil && !status.Initiator.Equal(initUrl) {
			// This should be impossible
			return nil, errors.InternalError.WithFormat("initiator is already set and does not match the signature")
		}

		statusDirty = true
		status.Initiator = initUrl
	}

	if statusDirty {
		err = batch.Transaction(delivery.Transaction.GetHash()).PutStatus(status)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	if delegatedNotReady {
		return signer, nil
	}

	// Persist the signature
	sigHash := signature.Hash()
	err = batch.Message2(sigHash).Main().Put(&messaging.UserSignature{
		Signature: sigToStore,
		TxID:      delivery.Transaction.ID(),
	})
	if err != nil {
		return nil, fmt.Errorf("store envelope: %w", err)
	}

	// Add the signature to the signer's chain
	if signer.GetUrl().LocalTo(md.Location) {
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
	isLocalTxn := delivery.Transaction.Header.Principal.LocalTo(md.Location)
	if isLocalTxn {
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
	case *protocol.RemoteSignature,
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
		return errors.BadTimestamp.WithFormat("initial signature does not have a timestamp")
	}

	return nil
}

// validateSigner verifies that the signer is valid and authorized.
func (x *Executor) validateSigner(batch *database.Batch, transaction *protocol.Transaction, signerUrl, location *url.URL, checkAuthz bool, md sigExecMetadata) (protocol.Signer, error) {
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
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	// Delegate to the transaction executor?
	val, ok := getValidator[chain.SignerValidator](x, transaction.Body.Type())
	if ok {
		fallback, err := val.SignerIsAuthorized(x, batch, transaction, signer, md)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
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
		return nil, errors.UnknownError.Wrap(err)
	}

	return signer, nil
}

func loadSigner(batch *database.Batch, signerUrl *url.URL) (protocol.Signer, error) {
	// Load signer
	account, err := batch.Account(signerUrl).GetState()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load signer: %w", err)
	}

	signer, ok := account.(protocol.Signer)
	if !ok {
		return nil, errors.BadRequest.WithFormat("invalid signer: %v cannot sign transactions", account.Type())
	}

	return signer, nil
}

// validateKeySignature verifies that the signature matches the signer state.
func validateKeySignature(transaction *protocol.Transaction, signer protocol.Signer, signature protocol.KeySignature) (protocol.KeyEntry, error) {
	// Check the height
	if transaction.Body.Type().IsUser() && signature.GetSignerVersion() != signer.GetVersion() {
		return nil, errors.BadSignerVersion.WithFormat("invalid version: have %d, got %d", signer.GetVersion(), signature.GetSignerVersion())
	}

	// Find the key entry
	_, entry, ok := signer.EntryByKeyHash(signature.GetPublicKeyHash())
	if !ok {
		return nil, errors.Unauthorized.With("key does not belong to signer")
	}

	// Check the timestamp, except for faucet transactions
	if transaction.Body.Type() != protocol.TransactionTypeAcmeFaucet &&
		signature.GetTimestamp() != 0 &&
		entry.GetLastUsedOn() >= signature.GetTimestamp() {
		return nil, errors.BadTimestamp.WithFormat("invalid timestamp: have %d, got %d", entry.GetLastUsedOn(), signature.GetTimestamp())
	}

	return entry, nil
}

// SignerIsAuthorized verifies that the signer is allowed to sign the transaction
func (x *Executor) SignerIsAuthorized(batch *database.Batch, transaction *protocol.Transaction, signer protocol.Signer, checkAuthz bool) error {
	switch signer := signer.(type) {
	case *protocol.LiteIdentity:
		// Otherwise a lite token account is only allowed to sign for itself
		if !signer.Url.Equal(transaction.Header.Principal.RootIdentity()) {
			return errors.Unauthorized.WithFormat("%v is not authorized to sign transactions for %v", signer.Url, transaction.Header.Principal)
		}

		return nil

	case *protocol.KeyPage:
		// Verify that the key page is allowed to sign the transaction
		bit, ok := transaction.Body.Type().AllowedTransactionBit()
		if ok && signer.TransactionBlacklist.IsSet(bit) {
			return errors.Unauthorized.WithFormat("page %s is not authorized to sign %v", signer.Url, transaction.Body.Type())
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
		return errors.InternalError.WithFormat("unknown signer type %v", signer.Type())
	}

	err := x.verifyPageIsAuthorized(batch, transaction, signer)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	return nil
}

// verifyPageIsAuthorized verifies that the key page is authorized to sign for
// the principal.
func (x *Executor) verifyPageIsAuthorized(batch *database.Batch, transaction *protocol.Transaction, signer protocol.Signer) error {
	// Load the principal
	principal, err := batch.Account(transaction.Header.Principal).GetState()
	if err != nil {
		return errors.UnknownError.WithFormat("load principal: %w", err)
	}

	// Get the principal's account auth
	auth, err := x.GetAccountAuthoritySet(batch, principal)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Get the signer book URL
	signerBook, _, ok := protocol.ParseKeyPageUrl(signer.GetUrl())
	if !ok {
		// If this happens, the database has bad data
		return errors.InternalError.WithFormat("invalid key page URL: %v", signer.GetUrl())
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
	return errors.Unauthorized.WithFormat("%v is not authorized to sign transactions for %v", signer.GetUrl(), principal.GetUrl())
}

// computeSignerFee computes the fee that will be charged to the signer.
//
// If the signature is the initial signature, the fee is the base transaction
// fee + signature data surcharge + transaction data surcharge.
//
// Otherwise, the fee is the base signature fee + signature data surcharge.
func (x *Executor) computeSignerFee(transaction *protocol.Transaction, signature protocol.KeySignature, md sigExecMetadata) (protocol.Fee, error) {
	// Don't charge fees for internal administrative functions
	signer := signature.GetSigner()
	_, isBvn := protocol.ParsePartitionUrl(signer)
	if isBvn || protocol.IsDnUrl(signer) {
		return 0, nil
	}

	// Compute the signature fee
	fee, err := x.globals.Active.Globals.FeeSchedule.ComputeSignatureFee(signature)
	if err != nil {
		return 0, errors.UnknownError.Wrap(err)
	}

	// Only charge the transaction fee for the initial signature
	if !md.IsInitiator {
		return fee, nil
	}

	// Add the transaction fee for the initial signature
	txnFee, err := x.globals.Active.Globals.FeeSchedule.ComputeTransactionFee(transaction)
	if err != nil {
		return 0, errors.UnknownError.Wrap(err)
	}

	// Subtract the base signature fee, but not the oversize surcharge if there is one
	fee += txnFee - protocol.FeeSignature
	return fee, nil
}

// validateKeySignature validates a private key signature.
func (x *Executor) validateKeySignature(batch *database.Batch, delivery *chain.Delivery, signature protocol.KeySignature, md sigExecMetadata, checkAuthz bool) (protocol.Signer, error) {
	// Validate the signer
	signer, err := x.validateSigner(batch, delivery.Transaction, signature.GetSigner(), signature.RoutingLocation(), checkAuthz, md)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Load the signer and validate the signature against it
	_, err = validateKeySignature(delivery.Transaction, signer, signature)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Do not charge fees for synthetic transactions
	if !delivery.Transaction.Body.Type().IsUser() {
		return signer, nil
	}

	// Ensure the signer has sufficient credits for the fee
	fee, err := x.computeSignerFee(delivery.Transaction, signature, md)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if !signer.CanDebitCredits(fee.AsUInt64()) {
		return nil, errors.InsufficientCredits.WithFormat("%v has insufficient credits: have %s, want %s", signer.GetUrl(),
			protocol.FormatAmount(signer.GetCreditBalance(), protocol.CreditPrecisionPower),
			protocol.FormatAmount(fee.AsUInt64(), protocol.CreditPrecisionPower))
	}

	return signer, nil
}

func (x *Executor) processSigner(batch *database.Batch, transaction *protocol.Transaction, signature protocol.Signature, md sigExecMetadata, checkAuthz bool) (protocol.Signer, error) {
	signer, err := x.validateSigner(batch, transaction, signature.GetSigner(), md.Location, checkAuthz, md)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if !transaction.Header.Principal.LocalTo(md.Location) {
		return signer, nil
	}

	record := batch.Transaction(transaction.GetHash())
	status, err := record.GetStatus()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Add all signers to the signer list so that the transaction readiness
	// check knows to look for delegates
	status.AddSigner(signer)
	err = record.PutStatus(status)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
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
		return nil, errors.UnknownError.Wrap(err)
	}

	// TODO If the forwarded signature paid the full fee unnecessarily, refund
	// it

	// Validate the signature against the signer. This should also not fail.
	entry, err := validateKeySignature(delivery.Transaction, signer, signature)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Charge the fee
	fee, err := x.computeSignerFee(delivery.Transaction, signature, md)
	if err != nil {
		return nil, errors.BadRequest.WithFormat("calculating fee: %w", err)
	}
	if !signer.DebitCredits(fee.AsUInt64()) {
		return nil, errors.InsufficientCredits.WithFormat("%v has insufficient credits: have %s, want %s", signer.GetUrl(),
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
		return nil, errors.UnknownError.WithFormat("store signer: %w", err)
	}

	return signer, nil
}

// validationPartitionSignature checks if the key used to sign the synthetic or system transaction belongs to the same subnet
func (x *Executor) validatePartitionSignature(signature protocol.KeySignature, transaction *protocol.Transaction, seq *messaging.SequencedMessage, status *protocol.TransactionStatus) (protocol.Signer2, error) {
	if seq == nil {
		return nil, errors.BadRequest.With("missing sequencing info")
	}
	partition, ok := protocol.ParsePartitionUrl(seq.Source)
	if !ok {
		return nil, errors.BadRequest.With("partition signature source is not a partition")
	}

	signer := x.globals.Active.AsSigner(partition)

	// TODO: Consider checking the version. However this can get messy because
	// it takes some time for changes to propagate, so we'd need an activation
	// height or something.

	_, _, ok = signer.EntryByKeyHash(signature.GetPublicKeyHash())
	if !ok {
		return nil, errors.Unauthorized.WithFormat("key is not an active validator for %s", partition)
	}

	return signer, nil
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
			var sig messaging.MessageWithSignature
			err = batch.Message(e.SignatureHash).Main().GetAs(&sig)
			if err != nil {
				return false, fmt.Errorf("load signature entry %X: %w", e.SignatureHash, err)
			}

			if _, ok := sig.GetSignature().(protocol.KeySignature); ok {
				return true, nil
			}
		}
	}

	return false, nil
}
