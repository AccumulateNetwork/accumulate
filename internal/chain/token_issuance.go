package chain

import (
	"encoding/json"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
)

type TokenIssuance struct{}

func (TokenIssuance) Type() types.TxType { return types.TxTypeTokenCreate }

func (TokenIssuance) CheckTx(st *state.StateEntry, tx *transactions.GenTransaction) error {
	return nil
}

func (TokenIssuance) DeliverTx(st *state.StateEntry, tx *transactions.GenTransaction) (*DeliverTxResult, error) {
	if err := tx.SetRoutingChainID(); err != nil {
		return nil, err
	}

	if st == nil {
		//but this is to be expected...
		return nil, fmt.Errorf("current State not defined")
	}

	if st.AdiState == nil {
		return nil, fmt.Errorf("identity not defined, unable to issue token")
	}

	if st.ChainState != nil {
		return nil, fmt.Errorf("token chain already defined, unable to issue token")
	}

	id := &state.AdiState{}
	err := id.UnmarshalBinary(st.AdiState.Entry)
	if err != nil {
		return nil, err
	}

	ti := &api.Token{}
	err = json.Unmarshal(tx.Transaction, ti)
	if err != nil {
		return nil, err
	}

	//do some ti validation

	adiState, _, err := types.ParseIdentityChainPath(id.ChainUrl.AsString())
	adiToken, tokenChain, err := types.ParseIdentityChainPath(ti.URL.AsString())
	if err != nil {
		return nil, err
	}

	if adiState != adiToken {
		return nil, fmt.Errorf("ADI URL doesn't match token ADI")
	}
	tas := state.NewToken(tokenChain)
	tas.Precision = ti.Precision
	tas.Meta = ti.Meta
	tas.Symbol = ti.Symbol

	tasso, err := tas.MarshalBinary()
	if err != nil {
		return nil, err
	}

	//return a new state object for a token
	res := new(DeliverTxResult)
	chainId := types.Bytes32{}
	copy(chainId[:], tx.ChainID)
	res.AddStateData(&chainId, tasso)
	return res, nil
}
