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

func checkSyntheticDepositCredits(st *state.StateEntry, tx *transactions.GenTransaction) (*protocol.SyntheticDepositCredits, state.Chain, error) {
	if st.ChainHeader == nil {
		return nil, nil, fmt.Errorf("recipient not found")
	}

	body := new(protocol.SyntheticDepositCredits)
	err := tx.As(body)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid payload: %v", err)
	}

	switch st.ChainHeader.Type {
	case types.ChainTypeAnonTokenAccount:
		acct := new(state.TokenAccount)
		err = st.ChainState.As(acct)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid token account: %v", err)
		}
		return body, acct, nil

	case types.ChainTypeMultiSigSpec:
		mss := new(protocol.MultiSigSpec)
		err = st.ChainState.As(mss)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid multi sig spec: %v", err)
		}
		return body, mss, nil

	default:
		return nil, nil, fmt.Errorf("cannot deposit tokens into a %v", st.ChainHeader.Type)
	}
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

	switch chain := chain.(type) {
	case *state.TokenAccount:
		return nil, fmt.Errorf("TODO Support adding credits from an anon account")

	case *protocol.MultiSigSpec:
		chain.Credit(body.Amount)

	default:
		// This should never happen. If we reach here, it's due to developer
		// error. But we still won't panic.
		return nil, fmt.Errorf("cannot deposit tokens into a %T", chain)
	}

	st.ChainState.Entry, err = chain.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state: %v", err)
	}

	txid := types.Bytes(tx.TransactionHash()).AsBytes32()
	st.DB.AddStateEntry(st.ChainId, &txid, st.ChainState)

	return new(DeliverTxResult), nil
}
