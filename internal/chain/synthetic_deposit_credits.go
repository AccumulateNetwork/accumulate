package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

type SyntheticDepositCredits struct{}

func (SyntheticDepositCredits) Type() types.TxType { return types.TxTypeSyntheticDepositCredits }

func (SyntheticDepositCredits) Validate(st *StateManager, tx *transactions.Envelope) (protocol.TransactionResult, error) {
	body := new(protocol.SyntheticDepositCredits)
	err := tx.As(body)
	if err != nil {
		return nil, fmt.Errorf("invalid payload: %v", err)
	}

	var account creditChain
	switch origin := st.Origin.(type) {
	case *protocol.LiteTokenAccount:
		account = origin

	case *protocol.KeyPage:
		account = origin

	default:
		return nil, fmt.Errorf("invalid origin record: want account type %v or %v, got %v", protocol.AccountTypeLiteTokenAccount, protocol.AccountTypeKeyPage, st.Origin.Header().Type)
	}

	account.CreditCredits(body.Amount)
	st.Update(account)
	return nil, nil
}
