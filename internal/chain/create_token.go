package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

type CreateToken struct{}

func (CreateToken) Type() types.TxType { return types.TxTypeCreateToken }

func (CreateToken) Validate(st *StateManager, tx *transactions.Envelope) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.CreateToken)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateToken), tx.Transaction.Body)
	}

	if body.Precision > 18 {
		return nil, fmt.Errorf("precision must be in range 0 to 18")
	}

	token := protocol.NewTokenIssuer()
	token.Url = body.Url
	token.Precision = body.Precision
	token.Supply = body.InitialSupply
	token.HasSupplyLimit = body.HasSupplyLimit
	token.Symbol = body.Symbol
	token.Properties = body.Properties
	token.ManagerKeyBook = body.Manager

	err = st.setKeyBook(token, body.KeyBookUrl)
	if err != nil {
		return nil, err
	}

	st.Create(token)
	return nil, nil
}
