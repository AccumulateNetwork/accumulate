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

type txnCreate struct{}

func (txnCreate) get(transaction *protocol.Transaction) []*url.URL {
	switch body := transaction.Body.(type) {
	case *protocol.CreateDataAccount:
		return body.Authorities
	case *protocol.CreateIdentity:
		return body.Authorities
	case *protocol.CreateKeyBook:
		return body.Authorities
	case *protocol.CreateToken:
		return body.Authorities
	case *protocol.CreateTokenAccount:
		return body.Authorities
	}
	return nil
}

// SignerIsAuthorized authorizes a signer if it belongs to an additional
// authority. Otherwise, it falls back to the default.
func (x txnCreate) SignerIsAuthorized(delegate chain.AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, signer protocol.Signer, _ chain.SignatureValidationMetadata) (fallback bool, err error) {
	signerBook, _, ok := protocol.ParseKeyPageUrl(signer.GetUrl())
	if !ok {
		// Fallback to general authorization
		return true, nil
	}

	for _, authority := range x.get(transaction) {
		// If we are adding a book, that book is authorized to sign the
		// transaction
		if !authority.Equal(signerBook) {
			continue
		}

		return false, delegate.SignerIsAuthorized(batch, transaction, signer, false)
	}

	// Fallback to general authorization
	return true, nil
}

// TransactionIsReady verifies that each additional authority is satisfied.
func (x txnCreate) TransactionIsReady(delegate chain.AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus) (ready, fallback bool, err error) {
	for _, authority := range x.get(transaction) {
		ok, err := delegate.AuthorityIsSatisfied(batch, transaction, status, authority)
		if !ok || err != nil {
			return false, false, err
		}
	}

	// Fallback to general authorization
	return false, true, nil
}

// checkCreateAdiAccount verifies that the principal is an ADI and contains the
// given URL.
func (txnCreate) checkCreateAdiAccount(batch *database.Batch, ctx *TransactionContext, account *url.URL) error {
	principal, err := batch.Account(ctx.transaction.Header.Principal).Main().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load principal: %w", err)
	}

	// ADI accounts can only be created within an ADI
	if _, ok := principal.(*protocol.ADI); !ok {
		return errors.BadRequest.WithFormat("invalid principal: want account type %v, got %v", protocol.AccountTypeIdentity, principal.Type())
	}

	// The origin must be the parent
	if !account.Identity().Equal(ctx.transaction.Header.Principal) {
		return errors.BadRequest.WithFormat("invalid principal: cannot create %v as a child of %v", account, ctx.transaction.Header.Principal)
	}

	dir, err := batch.Account(account.Identity()).Directory().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load directory index: %w", err)
	}

	if len(dir)+1 > int(ctx.Executor.globals.Active.Globals.Limits.IdentityAccounts) {
		return errors.BadRequest.WithFormat("identity would have too many accounts")
	}

	return nil
}

// setAuth
func (x txnCreate) setAuth(batch *database.Batch, ctx *TransactionContext, account protocol.FullAccount) error {
	authorities := x.get(ctx.transaction)
	switch {
	case len(authorities) > 0:
		// If the user specified a list of authorities, use them

	case len(account.GetAuth().Authorities) > 0:
		// If the account already has an authority, there's nothing to do
		return nil

	default:
		// Otherwise, inherit
		err := x.inheritAuth(batch, ctx, account)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	for _, authority := range authorities {
		if authority == nil {
			return errors.BadRequest.WithFormat("authority URL is nil")
		}
		account.GetAuth().AddAuthority(authority)
	}

	return nil
}

func (txnCreate) inheritAuth(batch *database.Batch, ctx *TransactionContext, account protocol.FullAccount) error {
	if !ctx.transaction.Header.Principal.LocalTo(account.GetUrl()) {
		return errors.BadRequest.With("cannot inherit from principal: belongs to a different domain")
	}

	principal, err := batch.Account(ctx.transaction.Header.Principal).Main().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load principal: %w", err)
	}

	full, ok := principal.(protocol.FullAccount)
	if !ok {
		return errors.BadRequest.With("cannot inherit from principal: not a full account")
	}

	// Inherit auth from the principal
	auth := account.GetAuth()
	*auth = *full.GetAuth()
	return nil
}
