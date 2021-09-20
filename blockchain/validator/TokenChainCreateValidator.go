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

type TokenChainCreateValidator struct {
	ValidatorContext

	EV *EntryValidator
}

func NewTokenChainCreateValidator() *TokenChainCreateValidator {
	v := TokenChainCreateValidator{}
	v.SetInfo(types.ChainTypeTokenAccount[:], types.ChainSpecTokenAccount, pb.AccInstruction_Token_URL_Creation)
	v.ValidatorContext.ValidatorInterface = &v
	return &v
}

func (v *TokenChainCreateValidator) Check(currentstate *state.StateEntry, submission *transactions.GenTransaction) error {
	if currentstate == nil {
		return fmt.Errorf("current state not defined")
	}

	if currentstate.IdentityState == nil {
		return fmt.Errorf("identity not defined")
	}

	if currentstate.ChainState != nil {
		return fmt.Errorf("chain already defined")
	}
	tcc := api.TokenAccount{}
	err := json.Unmarshal(submission.Transaction, &tcc)
	if err != nil {
		return fmt.Errorf("data payload of submission is not a valid token chain create message")
	}

	return nil
}

func (v *TokenChainCreateValidator) BeginBlock(height int64, time *time.Time) error {
	v.lastHeight = v.currentHeight
	v.lastTime = v.currentTime
	v.currentHeight = height
	v.currentTime = *time

	return nil
}

func (v *TokenChainCreateValidator) Validate(currentstate *state.StateEntry, submission *transactions.GenTransaction) (resp *ResponseValidateTX, err error) {
	if currentstate == nil {
		return nil, fmt.Errorf("current state not defined")
	}

	if currentstate.IdentityState == nil {
		return nil, fmt.Errorf("identity not defined")
	}

	if currentstate.ChainState != nil {
		return nil, fmt.Errorf("chain already defined")
	}
	tcc := api.TokenAccount{}
	err = json.Unmarshal(submission.Transaction, &tcc)
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
	resp = &ResponseValidateTX{}

	//return a new state object for a token
	resp.AddStateData(types.GetChainIdFromChainPath(&chainPath), statedata)

	return resp, nil
}

func (v *TokenChainCreateValidator) EndBlock(mdroot []byte) error {
	return nil
}
