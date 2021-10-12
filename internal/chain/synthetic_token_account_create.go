package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulated/internal/url"
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

	acctUrl, err := url.Parse(tx.SigInfo.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid sponsor URL: %v", err)
	}

	tokenUrl, err := url.Parse(*tac.TokenURL.AsString())
	if err != nil {
		return nil, fmt.Errorf("invalid token URL: %v", err)
	}

	if tac.ToUrl != types.String(tx.SigInfo.URL) {
		return nil, fmt.Errorf("account create URL does not match TX sponsor")
	}

	tas := state.NewTokenAccount(acctUrl.String(), tokenUrl.String())
	//tas.AdiChainPath = submission.? //todo: need to obtain the adi chain path from the submission request
	stateObj := new(state.Object)
	stateObj.Entry, err = tas.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("error marshalling identity state: %v", err)
	}

	chainId := types.Bytes(acctUrl.ResourceChain()).AsBytes32()
	txHash := types.Bytes(tx.TransactionHash()).AsBytes32()
	st.DB.AddStateEntry(&chainId, &txHash, stateObj)
	return new(DeliverTxResult), nil
}
