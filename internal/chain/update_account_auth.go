package chain

import (
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/v1"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type UpdateAccountAuth struct{}

var _ SignerValidator = (*UpdateAccountAuth)(nil)

func (UpdateAccountAuth) Type() protocol.TransactionType {
	return protocol.TransactionTypeUpdateAccountAuth
}

func (UpdateAccountAuth) SignerIsAuthorized(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, signer protocol.Signer, checkAuthz bool) (fallback bool, err error) {
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
				_, err := st.batch.Account(op.Authority).GetState()
				if err != nil {
					return nil, err
				}
			}
			// TODO Require a proof of the existence of the remote authority

			auth.AddAuthority(op.Authority)

		case *protocol.RemoveAccountAuthorityOperation:
			if !auth.RemoveAuthority(op.Authority) {
				// We could just ignore this case, but that is not a good user
				// experience
				return nil, fmt.Errorf("no such authority %v", op.Authority)
			}

			// An account must retain at least one authority
			if len(auth.Authorities) == 0 {
				return nil, errors.New("removing the last authority from an account is not allowed")
			}

		default:
			return nil, fmt.Errorf("invalid operation: %v", op.Type())
		}
	}

	err := st.Update(st.Origin)
	if err != nil {
		return nil, fmt.Errorf("failed to update %v: %w", st.OriginUrl, err)
	}
	return nil, nil
}
