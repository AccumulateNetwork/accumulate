package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"
)

type SynthIdentityCreate struct{}

func (SynthIdentityCreate) createChain() types.TxType {
	return types.TxTypeSyntheticIdentityCreate
}

func (SynthIdentityCreate) BeginBlock() {}

func (SynthIdentityCreate) CheckTx(st *state.StateEntry, tx *transactions.GenTransaction) error {
	if st == nil {
		//but this is to be expected...
		return fmt.Errorf("current state not defined")
	}

	if st.AdiState != nil {
		return fmt.Errorf("identity already exists")
	}

	isc := new(synthetic.AdiStateCreate)
	err := tx.As(isc)
	if err != nil {
		return fmt.Errorf("data payload of submission is not a valid identity state create message")
	}

	return nil
}

func (SynthIdentityCreate) DeliverTx(st *state.StateEntry, tx *transactions.GenTransaction) (*DeliverTxResult, error) {
	if st == nil {
		//but this is to be expected...
		return nil, fmt.Errorf("current state not defined")
	}

	if st.AdiState != nil {
		return nil, fmt.Errorf("identity already exists")
	}

	isc := new(synthetic.AdiStateCreate)
	err := tx.As(isc)
	if err != nil {
		return nil, fmt.Errorf("data payload of submission is not a valid identity state create message: %v", err)
	}

	idState := state.NewIdentityState(isc.ToUrl)
	idState.KeyType = state.KeyTypeSha256
	idState.KeyData = isc.PublicKeyHash[:]
	stateObj := new(state.Object)
	stateObj.Entry, err = idState.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("error marshalling identity state: %v", err)
	}

	chainId := types.GetIdentityChainFromIdentity(isc.ToUrl.AsString())
	txHash := types.Bytes(tx.TransactionHash()).AsBytes32()
	st.DB.AddStateEntry(chainId, &txHash, stateObj)
	return new(DeliverTxResult), nil
}
