package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
)

type SyntheticDepositCredits struct{}

func (SyntheticDepositCredits) Type() types.TxType { return types.TxTypeSyntheticDepositCredits }

func checkSyntheticDepositCredits(st *state.StateEntry, tx *transactions.GenTransaction) (*protocol.SyntheticDepositCredits, creditChain, error) {
	if st.ChainHeader == nil {
		return nil, nil, fmt.Errorf("recipient not found")
	}

	body := new(protocol.SyntheticDepositCredits)
	err := tx.As(body)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid payload: %v", err)
	}

	var account creditChain
	switch st.ChainHeader.Type {
	case types.ChainTypeAnonTokenAccount:
		account = new(protocol.AnonTokenAccount)

	case types.ChainTypeSigSpec:
		account = new(protocol.SigSpec)

	default:
		return nil, nil, fmt.Errorf("cannot deposit tokens into a %v", st.ChainHeader.Type)
	}

	err = st.ChainState.As(account)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid state: %v", err)
	}

	return body, account, nil
}

func (SyntheticDepositCredits) CheckTx(st *state.StateEntry, tx *transactions.GenTransaction) error {
	_, _, err := checkSyntheticDepositCredits(st, tx)
	return err
}

func (SyntheticDepositCredits) DeliverTx(st *state.StateEntry, tx *transactions.GenTransaction) (*DeliverTxResult, error) {
	body, chain, err := checkSyntheticDepositCredits(st, tx)
	if err != nil {
		return nil, err
	}

	chain.CreditCredits(body.Amount)

	st.ChainState.Entry, err = chain.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state: %v", err)
	}

	txid := types.Bytes(tx.TransactionHash()).AsBytes32()
	st.DB.AddStateEntry(st.ChainId, &txid, st.ChainState)

	return new(DeliverTxResult), nil
}
