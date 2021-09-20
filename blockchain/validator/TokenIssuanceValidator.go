package validator

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	pb "github.com/AccumulateNetwork/accumulated/types/proto"
	"github.com/AccumulateNetwork/accumulated/types/state"
)

type TokenIssuanceValidator struct {
	ValidatorContext
}

func NewTokenIssuanceValidator() *TokenIssuanceValidator {
	v := TokenIssuanceValidator{}
	v.SetInfo(types.ChainTypeToken[:], types.ChainSpecToken, pb.AccInstruction_Token_Issue)
	v.ValidatorContext.ValidatorInterface = &v
	return &v
}

func (v *TokenIssuanceValidator) Check(currentstate *state.StateEntry, submission *transactions.GenTransaction) error {
	return nil
}

func (v *TokenIssuanceValidator) BeginBlock(height int64, time *time.Time) error {
	v.lastHeight = v.currentHeight
	v.lastTime = v.currentTime
	v.currentHeight = height
	v.currentTime = *time

	return nil
}

func (v *TokenIssuanceValidator) Validate(currentState *state.StateEntry, submission *transactions.GenTransaction) (resp *ResponseValidateTX, err error) {
	if err := submission.SetRoutingChainID(); err != nil {
		return nil, err
	}

	if currentState == nil {
		//but this is to be expected...
		return nil, fmt.Errorf("current State not defined")
	}

	if currentState.IdentityState == nil {
		return nil, fmt.Errorf("identity not defined, unable to issue token")
	}

	if currentState.ChainState != nil {
		return nil, fmt.Errorf("token chain already defined, unable to issue token")
	}

	id := &state.AdiState{}
	err = id.UnmarshalBinary(currentState.IdentityState.Entry)
	if err != nil {
		return nil, err
	}

	ti := &api.Token{}
	err = json.Unmarshal(submission.Transaction, ti)
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
	resp = &ResponseValidateTX{}

	//return a new state object for a token
	chainId := types.Bytes32{}
	copy(chainId[:], submission.ChainID)
	resp.AddStateData(&chainId, tasso)

	return resp, nil
}

func (v *TokenIssuanceValidator) EndBlock(mdRoot []byte) error {
	_ = mdRoot
	return nil
}
