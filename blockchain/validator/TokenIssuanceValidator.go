package validator

import (
	"encoding/json"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	pb "github.com/AccumulateNetwork/accumulated/types/proto"

	//"crypto/sha256"
	"fmt"
	//"github.com/AccumulateNetwork/SMT/managed"
	"github.com/AccumulateNetwork/accumulated/types/state"
	cfg "github.com/tendermint/tendermint/config"

	//dbm "github.com/tendermint/tm-db"
	"time"
)

type TokenIssuanceValidator struct {
	ValidatorContext
}

func NewTokenIssuanceValidator() *TokenIssuanceValidator {
	v := TokenIssuanceValidator{}
	v.SetInfo(api.ChainTypeToken[:], api.ChainSpecToken, pb.AccInstruction_Token_Issue)
	v.ValidatorContext.ValidatorInterface = &v
	return &v
}

func (v *TokenIssuanceValidator) Check(currentstate *state.StateEntry, identitychain []byte, chainid []byte, p1 uint64, p2 uint64, data []byte) error {
	return nil
}
func (v *TokenIssuanceValidator) Initialize(config *cfg.Config) error {
	return nil
}

func (v *TokenIssuanceValidator) BeginBlock(height int64, time *time.Time) error {
	v.lastHeight = v.currentHeight
	v.lastTime = v.currentTime
	v.currentHeight = height
	v.currentTime = *time

	return nil
}

func (v *TokenIssuanceValidator) Validate(currentState *state.StateEntry, submission *pb.Submission) (resp *ResponseValidateTX, err error) {
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
	err = json.Unmarshal(submission.Data, ti)
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
	copy(chainId[:], submission.Chainid)
	resp.AddStateData(&chainId, tasso)

	return resp, nil
}

func (v *TokenIssuanceValidator) EndBlock(mdroot []byte) error {
	return nil
}
