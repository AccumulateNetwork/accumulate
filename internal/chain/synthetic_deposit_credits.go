package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type SyntheticDepositCredits struct{}

func (SyntheticDepositCredits) Type() types.TxType { return types.TxTypeSyntheticDepositCredits }

func checkSyntheticDepositCredits(st *StateManager, tx *transactions.GenTransaction) (*protocol.SyntheticDepositCredits, creditChain, error) {
	body := new(protocol.SyntheticDepositCredits)
	err := tx.As(body)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid payload: %v", err)
	}

	var account creditChain
	switch sponsor := st.Sponsor.(type) {
	case *protocol.AnonTokenAccount:
		account = sponsor

	case *protocol.SigSpec:
		account = sponsor

	default:
		return nil, nil, fmt.Errorf("cannot deposit tokens into a %v", st.Sponsor.Header().Type)
	}

	return body, account, nil
}

func (SyntheticDepositCredits) CheckTx(st *StateManager, tx *transactions.GenTransaction) error {
	_, _, err := checkSyntheticDepositCredits(st, tx)
	return err
}

func (SyntheticDepositCredits) DeliverTx(st *StateManager, tx *transactions.GenTransaction) error {
	body, chain, err := checkSyntheticDepositCredits(st, tx)
	if err != nil {
		return err
	}

	chain.CreditCredits(body.Amount)
	st.Update(chain)
	return nil
}
