// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	sortutil "gitlab.com/accumulatenetwork/accumulate/internal/util/sort"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// SignatureContext is the context in which a message is executed.
type SignatureContext struct {
	*MessageContext
	signature   protocol.Signature
	transaction *protocol.Transaction
}

func (s *SignatureContext) Type() protocol.SignatureType { return s.signature.Type() }

// maybeSendAuthoritySignature checks if the authority is ready to send an
// authority signature. Sending an authority signature also clears the active
// signature set.
func (s *SignatureContext) maybeSendAuthoritySignature(batch *database.Batch, authSig *protocol.AuthoritySignature) error {
	ok, err := s.authorityWillVote(batch, authSig.Authority)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	if !ok {
		return nil
	}

	err = clearActiveSignatures(batch, s, authSig.Authority)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	authSig.Vote = protocol.VoteTypeAccept
	authSig.TxID = s.transaction.ID()
	authSig.Cause = s.message.ID()

	err = s.didProduce(
		batch,
		authSig.RoutingLocation(),
		&messaging.SignatureMessage{
			Signature: authSig,
			TxID:      s.transaction.ID(),
		},
	)
	return errors.UnknownError.Wrap(err)
}

// getSigner gets the signature's signer, resolving a LTA to a LID.
func (s *SignatureContext) getSigner() *url.URL {
	signer := s.signature.GetSigner()
	if key, _ := protocol.ParseLiteIdentity(signer); key != nil {
		return signer
	} else if key, _, _ := protocol.ParseLiteTokenAddress(signer); key != nil {
		return signer.RootIdentity()
	} else {
		return signer
	}
}

// getAuthority gets the signature's signer's authority, resolving a LTA to a
// LID and a page to a book.
func (s *SignatureContext) getAuthority() *url.URL {
	signer := s.signature.GetSigner()
	if key, _ := protocol.ParseLiteIdentity(signer); key != nil {
		return signer
	} else if key, _, _ := protocol.ParseLiteTokenAddress(signer); key != nil {
		return signer.RootIdentity()
	} else {
		return signer.Identity()
	}
}

func (s *SignatureContext) authorityWillVote(batch *database.Batch, authority *url.URL) (bool, error) {
	// Delegate to the transaction executor?
	val, ok := getValidator[chain.AuthorityValidator](s.Executor, s.transaction.Body.Type())
	if ok {
		ready, fallback, err := val.AuthorityWillVote(s, batch, s.transaction, authority)
		if err != nil {
			return false, errors.UnknownError.Wrap(err)
		}
		if !fallback {
			return ready, nil
		}
	}

	ok, err := s.AuthorityWillVote(batch, s.Block.Index, s.transaction, authority)
	return ok, errors.UnknownError.Wrap(err)
}

func addSignature(batch *database.Batch, ctx *SignatureContext, signer protocol.Signer, entry *database.SignatureSetEntry) error {
	signerUrl := ctx.getSigner()
	txn := batch.Account(signerUrl).Transaction(ctx.transaction.ID().Hash())

	// Add the signature chain
	err := txn.RecordHistory(ctx.message)
	if err != nil {
		return errors.UnknownError.WithFormat("record history: %w", err)
	}

	// Grab the version from an entry
	all, err := txn.Signatures().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load signature set version: %w", err)
	}
	var version uint64
	if len(all) > 0 {
		version = all[0].Version
	}

	switch {
	case version == entry.Version:
		// Ignore repeated signatures
		_, ok := sortutil.Search(all, func(e *database.SignatureSetEntry) int { return int(e.KeyIndex) - int(entry.KeyIndex) })
		if ok {
			return nil
		}

		// Add to the active set if the signature's signer version is the same
		err = txn.Signatures().Add(entry)

	case version < entry.Version:
		// Replace the active set if the signature's signer version is more recent
		err = txn.Signatures().Put([]*database.SignatureSetEntry{entry})

	default: // version > entry.Version
		// This should be caught elsewhere
		return errors.InternalError.WithFormat("invalid signer version: want %v, got %v", version, entry.Version)
	}
	if err != nil {
		return errors.UnknownError.WithFormat("update active signature set: %w", err)
	}

	// Add the transaction to the authority's pending list if the signer is not
	// yet satisfied
	all, err = txn.Signatures().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load signature set version: %w", err)
	}
	if len(all) < int(signer.GetSignatureThreshold()) {
		err = batch.Account(ctx.getAuthority()).Pending().Add(ctx.transaction.ID())
		if err != nil {
			return errors.UnknownError.WithFormat("update the pending list: %w", err)
		}
	}

	return nil
}

func clearActiveSignatures(batch *database.Batch, ctx *SignatureContext, authUrl *url.URL) error {
	// Remove the transaction from the pending list
	err := batch.Account(authUrl).Pending().Remove(ctx.transaction.ID())
	if err != nil {
		return errors.UnknownError.WithFormat("update the pending list: %w", err)
	}

	// Load the authority
	var authority protocol.Authority
	err = batch.Account(authUrl).Main().GetAs(&authority)
	if err != nil {
		return errors.UnknownError.WithFormat("load the authority: %w", err)
	}

	// Clear the active signature set of every signer
	for _, signer := range authority.GetSigners() {
		err := batch.
			Account(signer).
			Transaction(ctx.transaction.ID().Hash()).
			Signatures().
			Put(nil)
		if err != nil {
			return errors.UnknownError.WithFormat("clear active signature set: %w", err)
		}
	}

	return nil
}

func (s *SignatureContext) signerCanSignTransaction(batch *database.Batch, txn *protocol.Transaction, signer protocol.Signer) error {
	if val, ok := getValidator[chain.SignerCanSignValidator](s.Executor, txn.Body.Type()); ok {
		fallback, err := val.SignerCanSign(s, batch, txn, signer)
		if !fallback || err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	return baseSignerCanSignTransaction(txn, signer)
}

func baseSignerCanSignTransaction(txn *protocol.Transaction, signer protocol.Signer) error {
	switch signer := signer.(type) {
	case *protocol.LiteIdentity:
		// A lite token account is only allowed to sign for itself
		if !signer.Url.Equal(txn.Header.Principal.RootIdentity()) {
			return errors.Unauthorized.WithFormat("%v is not authorized to sign transactions for %v", signer.Url, txn.Header.Principal)
		}
		return nil

	case *protocol.KeyPage:
		// Verify that the key page is allowed to sign the transaction
		bit, ok := txn.Body.Type().AllowedTransactionBit()
		if ok && signer.TransactionBlacklist.IsSet(bit) {
			return errors.Unauthorized.WithFormat("%s is not authorized to sign %v", signer.Url, txn.Body.Type())
		}
		return nil

	default:
		// This should never happen
		return errors.InternalError.WithFormat("unknown signer type %v", signer.Type())
	}
}

// authorityIsAccepted checks that an authority is authorized to sign for an account.
func authorityIsAccepted(batch *database.Batch, txn *protocol.Transaction, sig *protocol.AuthoritySignature) error {
	// Load the principal
	principal, err := batch.Account(txn.Header.Principal).Main().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load principal: %w", err)
	}

	// Get the principal's account auth
	auth, err := getAccountAuthoritySet(batch, principal)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Page belongs to book => authorized
	_, foundAuthority := auth.GetAuthority(sig.Authority)
	if foundAuthority {
		return nil
	}

	// Authorization is disabled and the transaction type does not force authorization => authorized
	if auth.AllAuthoritiesAreDisabled() && !txn.Body.Type().RequireAuthorization() {
		return nil
	}

	// Authorization is enabled => unauthorized
	// Transaction type forces authorization => unauthorized
	return errors.Unauthorized.WithFormat("%v is not authorized to sign transactions for %v", sig.Origin, principal.GetUrl())
}

// signerIsAuthorized calls the transaction executor's SignerIsAuthorized if it
// is defined. Otherwise it calls the default signerIsAuthorized.
func (s *SignatureContext) signerIsAuthorized(batch *database.Batch, sig *protocol.AuthoritySignature) error {
	// Delegate to the transaction executor?
	val, ok := getValidator[chain.SignerValidator](s.Executor, s.transaction.Body.Type())
	if ok {
		fallback, err := val.AuthorityIsAccepted(s, batch, s.transaction, sig)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
		if !fallback {
			return nil
		}
	}

	return authorityIsAccepted(batch, s.transaction, sig)
}

func loadSigner(batch *database.Batch, signerUrl *url.URL) (protocol.Signer, error) {
	// Load signer
	account, err := batch.Account(signerUrl).Main().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load signer: %w", err)
	}

	signer, ok := account.(protocol.Signer)
	if !ok {
		return nil, errors.BadRequest.WithFormat("invalid signer: %v cannot sign transactions", account.Type())
	}

	return signer, nil
}
