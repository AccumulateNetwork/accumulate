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

// authorityIsReady verifies that the authority is ready to vote.
func (s *SignatureContext) authorityIsReady(batch *database.Batch, authority *url.URL) (bool, error) {
	status, err := batch.Transaction(s.transaction.GetHash()).Status().Get()
	if err != nil {
		return false, errors.UnknownError.WithFormat("load status: %w", err)
	}

	// Delegate to the transaction executor?
	val, ok := getValidator[chain.AuthorityValidator](s.Executor, s.transaction.Body.Type())
	if ok {
		ready, fallback, err := val.AuthorityIsReady(s.Executor, batch, s.transaction, status, authority)
		if err != nil {
			return false, errors.UnknownError.Wrap(err)
		}
		if !fallback {
			return ready, nil
		}
	}

	ok, err = s.Executor.AuthorityIsReady(batch, s.transaction, status, authority)
	return ok, errors.UnknownError.Wrap(err)
}

func addSignature(batch *database.Batch, ctx *SignatureContext, signer protocol.Signer, entry *database.SignatureSetEntry) error {
	signerUrl := ctx.getSigner()
	set := batch.Account(signerUrl).Transaction(ctx.transaction.ID().Hash()).Signatures()

	// Add the signature to the signer's chain
	chain := batch.Account(signer.GetUrl()).SignatureChain()
	head, err := chain.Head().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load chain: %w", err)
	}
	entry.ChainIndex = uint64(head.Count)
	err = chain.Inner().AddHash(ctx.signature.Hash(), false)
	if err != nil {
		return errors.UnknownError.WithFormat("store chain: %w", err)
	}

	// Grab the version from an entry
	all, err := set.Active().Get()
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
		err = set.Active().Add(entry)

	case version < entry.Version:
		// Replace the active set if the signature's signer version is more recent
		err = set.Active().Put([]*database.SignatureSetEntry{entry})

	default: // version > entry.Version
		// This should be caught elsewhere
		return errors.InternalError.WithFormat("invalid signer version: want %v, got %v", version, entry.Version)
	}
	if err != nil {
		return errors.UnknownError.WithFormat("update active signature set: %w", err)
	}

	// Add the transaction to the authority's pending list if the signer is not
	// yet satisfied
	all, err = set.Active().Get()
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

func clearActiveSignatures(batch *database.Batch, ctx *SignatureContext) error {
	// Remove the transaction from the pending list
	authUrl := ctx.getAuthority()
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
			Active().
			Put(nil)
		if err != nil {
			return errors.UnknownError.WithFormat("clear active signature set: %w", err)
		}
	}

	return nil
}
