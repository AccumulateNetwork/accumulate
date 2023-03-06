// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type sigExecMetadata = chain.SignatureValidationMetadata

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
func (x *Executor) computeSignerFee(transaction *protocol.Transaction, signature protocol.Signature, isInitiator bool) (protocol.Fee, error) {
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
	if !isInitiator {
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
