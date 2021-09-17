package validator

import (
	"fmt"
	"time"

	"github.com/AccumulateNetwork/accumulated/types/api/transactions"

	"github.com/AccumulateNetwork/accumulated/types/api"

	"github.com/AccumulateNetwork/accumulated/types"
	pb "github.com/AccumulateNetwork/accumulated/types/proto"
	"github.com/AccumulateNetwork/accumulated/types/state"
	cfg "github.com/tendermint/tendermint/config"
)

//todo fold this into the AdiChain validator

type SyntheticIdentityStateCreateValidator struct {
	ValidatorContext

	EV *EntryValidator
}

func NewSyntheticIdentityStateCreateValidator() *SyntheticIdentityStateCreateValidator {
	v := SyntheticIdentityStateCreateValidator{}
	//this needs to be changed to use AdiChain
	v.SetInfo(types.ChainTypeAdi[:], "create-identity-state", pb.AccInstruction_Synthetic_Identity_Creation)
	v.ValidatorContext.ValidatorInterface = &v
	return &v
}

func (v *SyntheticIdentityStateCreateValidator) Check(currentstate *state.StateEntry, submission *transactions.GenTransaction) error {
	if currentstate == nil {
		//but this is to be expected...
		return fmt.Errorf("current state not defined")
	}

	if currentstate.IdentityState != nil {
		return fmt.Errorf("identity already exists")
	}

	is := api.ADI{}
	err := is.UnmarshalBinary(submission.Transaction)
	if err != nil {
		return fmt.Errorf("data payload of submission is not a valid identity state create message")
	}

	return nil
}

func (v *SyntheticIdentityStateCreateValidator) Initialize(config *cfg.Config) error {
	return nil
}

func (v *SyntheticIdentityStateCreateValidator) BeginBlock(height int64, time *time.Time) error {
	v.lastHeight = v.currentHeight
	v.lastTime = v.currentTime
	v.currentHeight = height
	v.currentTime = *time

	return nil
}

func (v *SyntheticIdentityStateCreateValidator) Validate(currentstate *state.StateEntry, submission *transactions.GenTransaction) (resp *ResponseValidateTX, err error) {
	if currentstate == nil {
		//but this is to be expected...
		return nil, fmt.Errorf("current state not defined")
	}

	if currentstate.IdentityState != nil {
		return nil, fmt.Errorf("identity already exists")
	}

	is := api.ADI{}
	err = is.UnmarshalBinary(submission.Transaction)
	if err != nil {
		return nil, fmt.Errorf("data payload of submission is not a valid identity state create message")
	}

	statedata, err := is.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("cannot marshal state object for identity state create")
	}
	resp = &ResponseValidateTX{}

	resp.AddStateData(types.GetIdentityChainFromIdentity(is.URL.AsString()), statedata)

	return resp, nil
}

func (v *SyntheticIdentityStateCreateValidator) EndBlock(mdRoot []byte) error {
	_ = mdRoot
	return nil
}
