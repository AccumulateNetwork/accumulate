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

func (UpdateAccountAuth) AuthorityIsAccepted(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, sig *protocol.AuthoritySignature) (fallback bool, err error) {
	body, ok := transaction.Body.(*protocol.UpdateAccountAuth)
	if !ok {
		return false, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.UpdateAccountAuth), transaction.Body)
	}

	newOwners, err := updateAccountAuth_getNewOwners(batch, body)
	if err != nil {
		return false, err
	}

	return newOwners.AuthorityIsAccepted(delegate, batch, transaction, sig)
}

func (UpdateAccountAuth) TransactionIsReady(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction) (ready, fallback bool, err error) {
	body, ok := transaction.Body.(*protocol.UpdateAccountAuth)
	if !ok {
		return false, false, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.UpdateAccountAuth), transaction.Body)
	}

	newOwners, err := updateAccountAuth_getNewOwners(batch, body)
	if err != nil {
		return false, false, err
	}

	return newOwners.TransactionIsReady(delegate, batch, transaction)
}

func (x UpdateAccountAuth) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	_, err := x.check(st, tx)
	return nil, err
}

func (UpdateAccountAuth) check(st *StateManager, tx *Delivery) (*protocol.UpdateAccountAuth, error) {
	body, ok := tx.Transaction.Body.(*protocol.UpdateAccountAuth)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.UpdateAccountAuth), tx.Transaction.Body)
	}

	for _, op := range body.Operations {
		switch op := op.(type) {
		case *protocol.EnableAccountAuthOperation:
			if op.Authority == nil {
				return nil, errors.BadRequest.WithFormat("authority URL is missing")
			}

		case *protocol.DisableAccountAuthOperation:
			if op.Authority == nil {
				return nil, errors.BadRequest.WithFormat("authority URL is missing")
			}

		case *protocol.AddAccountAuthorityOperation:
			if op.Authority == nil {
				return nil, errors.BadRequest.WithFormat("authority URL is missing")
			}

		case *protocol.RemoveAccountAuthorityOperation:
			if op.Authority == nil {
				return nil, errors.BadRequest.WithFormat("authority URL is missing")
			}

		default:
			return nil, errors.BadRequest.WithFormat("invalid operation: %v", op.Type())
		}
	}

	return body, nil
}

func (x UpdateAccountAuth) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, err := x.check(st, tx)
	if err != nil {
		return nil, err
	}

	account, ok := st.Origin.(protocol.FullAccount)
	if !ok {
		return nil, fmt.Errorf("account type %v does not support advanced auth", st.Origin.Type())
	}
	auth := account.GetAuth()

	for _, op := range body.Operations {
		switch op := op.(type) {
		case *protocol.EnableAccountAuthOperation:
			entry, ok := auth.GetAuthority(op.Authority)
			if !ok {
				return nil, fmt.Errorf("%v is not an authority of %v", op.Authority, st.OriginUrl)
			}
			entry.Disabled = false

		case *protocol.DisableAccountAuthOperation:
			entry, ok := auth.GetAuthority(op.Authority)
			if !ok {
				return nil, fmt.Errorf("%v is not an authority of %v", op.Authority, st.OriginUrl)
			}
			entry.Disabled = true

		case *protocol.AddAccountAuthorityOperation:
			if account.GetUrl().LocalTo(op.Authority) {
				// If the authority is local, make sure it exists
				_, err := st.batch.Account(op.Authority).Main().Get()
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
			if !auth.RemoveAuthority(op.Authority) {
				// We could just ignore this case, but that is not a good user
				// experience
				return nil, fmt.Errorf("no such authority %v", op.Authority)
			}

			switch {
			case len(auth.Authorities) > 0:
				// Ok

			case !st.Globals.ExecutorVersion.V2BaikonurEnabled():
				// Old logic
				return nil, errors.BadRequest.With("removing the last authority from an account is not allowed")

			case account.GetUrl().IsRootIdentity():
				// A root account must retain at least one authority
				return nil, errors.BadRequest.With("removing the last authority from a root account is not allowed")

			default:
				// Verify that there is a parent identity with a non-empty
				// authority set
				err = verifyCanInheritAuth(st.batch, account)
				if err != nil {
					return nil, errors.UnknownError.Wrap(err)
				}
			}

		default:
			return nil, errors.InternalError.WithFormat("invalid operation: %v", op.Type())
		}
	}

	if len(auth.Authorities) > int(st.Globals.Globals.Limits.AccountAuthorities) {
		return nil, errors.BadRequest.WithFormat("account will have too many authorities")
	}

	err = st.Update(st.Origin)
	if err != nil {
		return nil, fmt.Errorf("failed to update %v: %w", st.OriginUrl, err)
	}
	return nil, nil
}

func verifyCanInheritAuth(batch *database.Batch, account protocol.FullAccount) error {
	for a := account; !a.GetUrl().IsRootIdentity(); {
		err := batch.Account(a.GetUrl().Identity()).Main().GetAs(&a)
		if err != nil {
			return errors.UnknownError.WithFormat("inherit authority: %w", err)
		}

		if len(a.GetAuth().Authorities) > 0 {
			return nil
		}
	}
	return errors.BadRequest.WithFormat("cannot remove last authority: no parent from which authorities can be inherited")
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

func updateAccountAuth_getNewOwners(batch *database.Batch, body *protocol.UpdateAccountAuth) (additionalAuthorities, error) {
	var owners []*url.URL
	for _, op := range body.Operations {
		switch op := op.(type) {
		case *protocol.AddAccountAuthorityOperation:
			if op.Authority == nil {
				return nil, fmt.Errorf("invalid payload: authority is nil")
			}

			owners = append(owners, op.Authority)

		default:
			continue
		}
	}

	return owners, nil
}
