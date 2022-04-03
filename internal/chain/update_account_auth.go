package chain

import (
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type UpdateAccountAuth struct{}

func (UpdateAccountAuth) Type() protocol.TransactionType {
	return protocol.TransactionTypeUpdateAccountAuth
}

func (UpdateAccountAuth) Validate(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
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
			if account.GetUrl().RootIdentity().Equal(op.Authority.RootIdentity()) {
				// If the authority is local, make sure it exists
				_, err := st.batch.Account(op.Authority).GetState()
				if err != nil {
					return nil, err
				}

			} else {
				// TODO Support remote authorities
				return nil, errors.New("remote authorities are not supported")
			}

			auth.AddAuthority(op.Authority)

		case *protocol.RemoveAccountAuthorityOperation:
			auth.RemoveAuthority(op.Authority)

			// An account must retain at least one authority
			if len(auth.Authorities) == 0 {
				return nil, errors.New("removing the last authority from an account is not allowed")
			}

		default:
			return nil, fmt.Errorf("invalid operation: %v", op.Type())
		}
	}

	st.Update(st.Origin)
	return nil, nil
}
