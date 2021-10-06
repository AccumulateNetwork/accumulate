package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
)

type SynIdentityCreate struct{}

func (SynIdentityCreate) createIdentity() types.TxType {
	return types.TxTypeSyntheticIdentityCreate
}

func (SynIdentityCreate) BeginBlock() {}

func (SynIdentityCreate) CheckTx(st *state.StateEntry, tx *transactions.GenTransaction) error {
	if st == nil {
		//but this is to be expected...
		return fmt.Errorf("current state not defined")
	}

	if st.AdiState != nil {
		return fmt.Errorf("identity already exists")
	}

	is := api.ADI{}
	err := is.UnmarshalBinary(tx.Transaction)
	if err != nil {
		return fmt.Errorf("data payload of submission is not a valid identity state create message")
	}

	return nil
}

func (SynIdentityCreate) DeliverTx(st *state.StateEntry, tx *transactions.GenTransaction) (*DeliverTxResult, error) {
	if st == nil {
		//but this is to be expected...
		return nil, fmt.Errorf("current state not defined")
	}

	if st.AdiState != nil {
		return nil, fmt.Errorf("identity already exists")
	}

	is := api.ADI{}
	err := is.UnmarshalBinary(tx.Transaction)
	if err != nil {
		return nil, fmt.Errorf("data payload of submission is not a valid identity state create message")
	}

	statedata, err := is.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("cannot marshal state object for identity state create")
	}

	res := new(DeliverTxResult)
	res.AddStateData(types.GetIdentityChainFromIdentity(is.URL.AsString()), statedata)
	return res, nil
}
