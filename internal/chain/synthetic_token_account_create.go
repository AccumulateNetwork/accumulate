package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"
)

type SynthTokenAccountCreate struct{}

func (SynthTokenAccountCreate) Type() types.TxType {
	return types.TxTypeSyntheticTokenAccountCreate
}

func (SynthTokenAccountCreate) CheckTx(st *state.StateEntry, tx *transactions.GenTransaction) error {
	return nil
}

func (SynthTokenAccountCreate) DeliverTx(st *state.StateEntry, tx *transactions.GenTransaction) (*DeliverTxResult, error) {
	if st != nil && st.ChainHeader != nil {
		return nil, fmt.Errorf("account already exists")
	}

	tac := new(synthetic.TokenAccountCreate)
	err := tx.As(tac)
	if err != nil {
		return nil, fmt.Errorf("data payload of submission is not a valid token chain create message")
	}

	adi, chainPath, err := types.ParseIdentityChainPath(tac.ToUrl.AsString())
	if err != nil {
		return nil, err
	}

	issuingidentityhash := types.GetIdentityChainFromIdentity(&adi)
	issuingchainid := types.GetChainIdFromChainPath(&chainPath)

	if issuingidentityhash == nil {
		return nil, fmt.Errorf("issuing identity adi is invalid")
	}

	if issuingchainid == nil {
		return nil, fmt.Errorf("issuing identity chain id is invalid")
	}

	_, chainPathToken, err := types.ParseIdentityChainPath(tac.TokenURL.AsString())
	if err != nil {
		return nil, err
	}

	tas := state.NewTokenAccount(chainPath, chainPathToken)
	//tas.AdiChainPath = submission.? //todo: need to obtain the adi chain path from the submission request
	stateObj := new(state.Object)
	stateObj.Entry, err = tas.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("error marshalling identity state: %v", err)
	}

	txHash := types.Bytes(tx.TransactionHash()).AsBytes32()
	st.DB.AddStateEntry(issuingchainid, &txHash, stateObj)
	return new(DeliverTxResult), nil
}
