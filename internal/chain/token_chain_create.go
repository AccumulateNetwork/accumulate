package chain

import (
	"encoding/json"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/proto"
	"github.com/AccumulateNetwork/accumulated/types/state"
)

type TokenChainCreate struct{}

func (TokenChainCreate) chainType() chainTypeId {
	return chainTypeId(types.ChainTypeTokenAccount)
}

func (TokenChainCreate) instruction() proto.AccInstruction {
	return proto.AccInstruction_Token_URL_Creation
}

func (TokenChainCreate) BeginBlock() {}

func (TokenChainCreate) CheckTx(st *state.StateEntry, tx *transactions.GenTransaction) error {
	if st == nil {
		return fmt.Errorf("current state not defined")
	}

	if st.IdentityState == nil {
		return fmt.Errorf("identity not defined")
	}

	if st.ChainState != nil {
		return fmt.Errorf("chain already defined")
	}
	tcc := api.TokenAccount{}
	err := json.Unmarshal(tx.Transaction, &tcc)
	if err != nil {
		return fmt.Errorf("data payload of submission is not a valid token chain create message")
	}

	return nil
}

func (TokenChainCreate) DeliverTx(st *state.StateEntry, tx *transactions.GenTransaction) (*DeliverTxResult, error) {
	if st == nil {
		return nil, fmt.Errorf("current state not defined")
	}

	if st.IdentityState == nil {
		return nil, fmt.Errorf("identity not defined")
	}

	if st.ChainState != nil {
		return nil, fmt.Errorf("chain already defined")
	}
	tcc := api.TokenAccount{}
	err := json.Unmarshal(tx.Transaction, &tcc)
	if err != nil {
		return nil, fmt.Errorf("data payload of submission is not a valid token chain create message")
	}

	adi, chainPath, err := types.ParseIdentityChainPath(tcc.URL.AsString())
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

	_, chainPathToken, err := types.ParseIdentityChainPath(tcc.TokenURL.AsString())
	if err != nil {
		return nil, err
	}

	tas := state.NewTokenAccount(chainPath, chainPathToken)
	//tas.AdiChainPath = submission.? //todo: need to obtain the adi chain path from the submission request
	statedata, err := tas.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("cannot marshal state object for identity state create")
	}

	//return a new state object for a token
	res := new(DeliverTxResult)
	res.AddStateData(types.GetChainIdFromChainPath(&chainPath), statedata)
	return res, nil
}
