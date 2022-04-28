package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type SyntheticDepositCredits struct{}

func (SyntheticDepositCredits) Type() protocol.TransactionType {
	return protocol.TransactionTypeSyntheticDepositCredits
}

func (SyntheticDepositCredits) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (SyntheticDepositCredits{}).Validate(st, tx)
}

func (SyntheticDepositCredits) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.SyntheticDepositCredits)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.SyntheticDepositCredits), tx.Transaction.Body)
	}

	var account protocol.Signer
	switch origin := st.Origin.(type) {
	case *protocol.LiteIdentity:
		account = origin

	case *protocol.KeyPage:
		account = origin

	default:
		return nil, fmt.Errorf("invalid origin record: want account type %v or %v, got %v", protocol.AccountTypeLiteTokenAccount, protocol.AccountTypeKeyPage, st.Origin.Type())
	}

	account.CreditCredits(body.Amount)
	st.Update(account)
	return nil, nil
}
