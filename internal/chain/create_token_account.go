package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

type CreateTokenAccount struct{}

func (CreateTokenAccount) Type() types.TxType { return types.TxTypeCreateTokenAccount }

func (CreateTokenAccount) Validate(st *StateManager, tx *transactions.Envelope) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.CreateTokenAccount)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateTokenAccount), tx.Transaction.Body)
	}

	if !body.Url.Identity().Equal(st.OriginUrl) {
		return nil, fmt.Errorf("%q cannot be the origininator of %q", st.OriginUrl, body.Url)
	}

	account := protocol.NewTokenAccount()
	account.Url = body.Url
	account.TokenUrl = body.TokenUrl
	account.Scratch = body.Scratch
	account.ManagerKeyBook = body.Manager

	err = st.setKeyBook(account, body.KeyBookUrl)
	if err != nil {
		return nil, err
	}

	st.Create(account)
	return nil, nil
}
