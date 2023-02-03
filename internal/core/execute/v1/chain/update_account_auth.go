// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type UpdateAccountAuth struct{}

var _ SignerValidator = (*UpdateAccountAuth)(nil)

func (UpdateAccountAuth) Type() protocol.TransactionType {
	return protocol.TransactionTypeUpdateAccountAuth
}

func (UpdateAccountAuth) SignerIsAuthorized(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, signer protocol.Signer, md SignatureValidationMetadata) (fallback bool, err error) {
	body, ok := transaction.Body.(*protocol.UpdateAccountAuth)
	if !ok {
		return false, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.UpdateAccountAuth), transaction.Body)
	}

	signerBook, _, ok := protocol.ParseKeyPageUrl(signer.GetUrl())
	if !ok {
		// Fallback to general authorization
		return true, nil
	}

	for _, op := range body.Operations {
		op, ok := op.(*protocol.AddAccountAuthorityOperation)
		if !ok {
			continue
		}

		if op.Authority == nil {
			return false, fmt.Errorf("invalid payload: authority is nil")
		}

		// If we are adding a book, that book is authorized to sign the
		// transaction
		if !op.Authority.Equal(signerBook) {
			continue
		}

		return false, delegate.SignerIsAuthorized(batch, transaction, signer, false)
	}

	// Fallback to general authorization
	return true, nil
}

func (UpdateAccountAuth) TransactionIsReady(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus) (ready, fallback bool, err error) {
	body, ok := transaction.Body.(*protocol.UpdateAccountAuth)
	if !ok {
		return false, false, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.UpdateAccountAuth), transaction.Body)
	}

	for _, op := range body.Operations {
		op, ok := op.(*protocol.AddAccountAuthorityOperation)
		if !ok {
			continue
		}

		if op.Authority == nil {
			return false, false, fmt.Errorf("invalid payload: authority is nil")
		}

		ok, err := delegate.AuthorityIsSatisfied(batch, transaction, status, op.Authority)
		if !ok || err != nil {
			return false, false, err
		}
	}

	// Fallback to general authorization
	return false, true, nil
}

func (UpdateAccountAuth) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (UpdateAccountAuth{}).Validate(st, tx)
}

func (UpdateAccountAuth) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.UpdateAccountAuth)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.UpdateAccountAuth), tx.Transaction.Body)
	}

	account, ok := st.Origin.(protocol.FullAccount)
	if !ok {
		return nil, fmt.Errorf("account type %v does not support advanced auth", st.Origin.Type())
	}
	auth := account.GetAuth()

	for _, op := range body.Operations {
		switch op := op.(type) {
		case *protocol.EnableAccountAuthOperation:
			if op.Authority == nil {
				return nil, errors.BadRequest.WithFormat("authority URL is missing")
			}
			entry, ok := auth.GetAuthority(op.Authority)
			if !ok {
				return nil, fmt.Errorf("%v is not an authority of %v", op.Authority, st.OriginUrl)
			}
			entry.Disabled = false

		case *protocol.DisableAccountAuthOperation:
			if op.Authority == nil {
				return nil, errors.BadRequest.WithFormat("authority URL is missing")
			}
			entry, ok := auth.GetAuthority(op.Authority)
			if !ok {
				return nil, fmt.Errorf("%v is not an authority of %v", op.Authority, st.OriginUrl)
			}
			entry.Disabled = true

		case *protocol.AddAccountAuthorityOperation:
			if op.Authority == nil {
				return nil, errors.BadRequest.WithFormat("authority URL is missing")
			}

			if account.GetUrl().LocalTo(op.Authority) {
				// If the authority is local, make sure it exists
				_, err := st.batch.Account(op.Authority).GetState()
				if err != nil {
					return nil, err
				}
			}
			// TODO Require a proof of the existence of the remote authority

			if err := verifyIsNotPage(auth, op.Authority); err != nil {
				return nil, errors.UnknownError.WithFormat("invalid authority %v: %w", op.Authority, err)
			}

			_, new := auth.AddAuthority(op.Authority)
			if !new {
				return nil, fmt.Errorf("duplicate authority %v", op.Authority)
			}

		case *protocol.RemoveAccountAuthorityOperation:
			if op.Authority == nil {
				return nil, errors.BadRequest.WithFormat("authority URL is missing")
			}

			if !auth.RemoveAuthority(op.Authority) {
				// We could just ignore this case, but that is not a good user
				// experience
				return nil, fmt.Errorf("no such authority %v", op.Authority)
			}

			// An account must retain at least one authority
			if len(auth.Authorities) == 0 {
				return nil, errors.BadRequest.With("removing the last authority from an account is not allowed")
			}

		default:
			return nil, fmt.Errorf("invalid operation: %v", op.Type())
		}
	}

	if len(auth.Authorities) > int(st.Globals.Globals.Limits.AccountAuthorities) {
		return nil, errors.BadRequest.WithFormat("account will have too many authorities")
	}

	err := st.Update(st.Origin)
	if err != nil {
		return nil, fmt.Errorf("failed to update %v: %w", st.OriginUrl, err)
	}
	return nil, nil
}

// verifyIsNotPage returns an error if the url is a page belonging to one of the
// account's authorities.
//
// Given:
// - 'foo/bar/1' is the new authority
// - 'foo/bar' is an existing authority
// And:
// - If 'foo/bar' is an ADI, 'foo/bar/1' could be any type of account
// - If 'foo/bar is a book, 'foo/bar/1' can be a page
// - There are no other cases where 'foo/bar' can contain 'foo/bar/1'
// - An ADI cannot be an authority
// Then:
// - 'foo/bar' must be a book and 'foo/bar/1' must be a page of that book
func verifyIsNotPage(auth *protocol.AccountAuth, account *url.URL) error {
	book, _, ok := protocol.ParseKeyPageUrl(account)
	if !ok {
		return nil
	}

	_, ok = auth.GetAuthority(book)
	if !ok {
		return nil
	}

	return errors.BadRequest.WithFormat("a key page is not a valid authority")
}
