package validator

import (
	"encoding/json"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	pb "github.com/AccumulateNetwork/accumulated/types/proto"
	//"crypto/sha256"
	"fmt"
	//"github.com/AccumulateNetwork/SMT/managed"
	acctypes "github.com/AccumulateNetwork/accumulated/types/state"
	cfg "github.com/tendermint/tendermint/config"
	//dbm "github.com/tendermint/tm-db"
	"time"
)

//todo fold this into the AdiChain validator

type SyntheticIdentityStateCreateValidator struct {
	ValidatorContext

	EV *EntryValidator
}

func NewSyntheticIdentityStateCreateValidator() *SyntheticIdentityStateCreateValidator {
	v := SyntheticIdentityStateCreateValidator{}
	//this needs to be changed to use AdiChain
	v.SetInfo(api.ChainTypeAdi[:], "create-identity-state", pb.AccInstruction_Synthetic_Identity_Creation)
	v.ValidatorContext.ValidatorInterface = &v
	return &v
}

func (v *SyntheticIdentityStateCreateValidator) Check(currentstate *StateEntry, identitychain []byte, chainid []byte, p1 uint64, p2 uint64, data []byte) error {
	if currentstate == nil {
		//but this is to be expected...
		return fmt.Errorf("current state not defined")
	}

	if currentstate.IdentityState != nil {
		return fmt.Errorf("identity already exists")
	}

	is := acctypes.AdiState{}
	err := json.Unmarshal(data, &is)
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

func (v *SyntheticIdentityStateCreateValidator) Validate(currentstate *StateEntry, submission *pb.Submission) (resp *ResponseValidateTX, err error) {
	if currentstate == nil {
		//but this is to be expected...
		return nil, fmt.Errorf("current state not defined")
	}

	if currentstate.IdentityState != nil {
		return nil, fmt.Errorf("identity already exists")
	}

	is := acctypes.AdiState{}
	err = json.Unmarshal(submission.Data, &is)
	if err != nil {
		return nil, fmt.Errorf("data payload of submission is not a valid identity state create message")
	}

	statedata, err := is.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("cannot marshal state object for identity state create")
	}
	resp = &ResponseValidateTX{}

	resp.AddStateData(types.GetIdentityChainFromIdentity(string(is.ChainUrl)), statedata)

	return resp, nil
}

func (v *SyntheticIdentityStateCreateValidator) EndBlock(mdroot []byte) error {
	return nil
}
