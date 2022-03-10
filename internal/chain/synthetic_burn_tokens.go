package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

type SyntheticBurnTokens struct{}

func (SyntheticBurnTokens) Type() types.TxType { return types.TxTypeSyntheticBurnTokens }

func (SyntheticBurnTokens) Validate(st *StateManager, tx *transactions.Envelope) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.SyntheticBurnTokens)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.SyntheticBurnTokens), tx.Transaction.Body)
	}

	account := protocol.NewTokenIssuer()
	switch origin := st.Origin.(type) {
	case *protocol.TokenIssuer:
		account = origin
	default:
		return nil, fmt.Errorf("invalid origin record: want chain type %v, got %v", protocol.AccountTypeTokenIssuer, origin.GetType())
	}

	account.Issued.Sub(&account.Issued, &body.Amount)

	st.Update(account)
	return nil, nil
}
