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
	body, ok := tx.Transaction.Body.(*protocol.SyntheticDepositCredits)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.SyntheticDepositCredits), tx.Transaction.Body)
	}

	var account creditChain
	switch origin := st.Origin.(type) {
	case *protocol.LiteTokenAccount:
		account = origin

	case *protocol.KeyPage:
		account = origin

	default:
		return nil, fmt.Errorf("invalid origin record: want account type %v or %v, got %v", protocol.AccountTypeLiteTokenAccount, protocol.AccountTypeKeyPage, st.Origin.GetType())
	}

	account.CreditCredits(body.Amount.Uint64())
	st.Update(account)
	return nil, nil
}
