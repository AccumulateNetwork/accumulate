package validator

import (
	"encoding/json"
	"github.com/AccumulateNetwork/accumulated/types"
	pb "github.com/AccumulateNetwork/accumulated/types/proto"

	//"crypto/sha256"
	"fmt"
	//"github.com/AccumulateNetwork/SMT/managed"
	acctypes "github.com/AccumulateNetwork/accumulated/types/state"
	cfg "github.com/tendermint/tendermint/config"
	//dbm "github.com/tendermint/tm-db"
	"time"
)

type TokenIssuanceValidator struct {
	ValidatorContext
}

func NewTokenIssuanceValidator() *TokenIssuanceValidator {
	v := TokenIssuanceValidator{}
	//need the chainid, then hash to get first 8 bytes to make the chainid.
	//by definition a chainid of a factoid block is
	//000000000000000000000000000000000000000000000000000000000000000f
	//the id will be 0x0000000f
	chainid := "000000000000000000000000000000000000000000000000000000000000001D" //does this make sense anymore?
	v.SetInfo(chainid, "create-token", pb.AccInstruction_Token_Issue)
	v.ValidatorContext.ValidatorInterface = &v
	return &v
}

func (v *TokenIssuanceValidator) Check(currentstate *StateEntry, identitychain []byte, chainid []byte, p1 uint64, p2 uint64, data []byte) error {
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

func (v *TokenIssuanceValidator) Validate(currentState *StateEntry, submission *pb.Submission) (resp *ResponseValidateTX, err error) {
	if currentState == nil {
		//but this is to be expected...
		return nil, fmt.Errorf("Current State Not Defined")
	}

	if currentState.IdentityState == nil {
		return nil, fmt.Errorf("Identity not defined. Unable to issue token.")
	}

	if currentState.ChainState != nil {
		return nil, fmt.Errorf("Token chain already defined.  Unable to issue token.")
	}

	id := &acctypes.AdiState{}
	err = id.UnmarshalBinary(currentState.IdentityState.Entry)
	if err != nil {
		return nil, err
	}

	ti := &types.Token{}
	err = json.Unmarshal(submission.Data, ti)
	if err != nil {
		return nil, err
	}

	//do some ti validation

	adiState, _, err := types.ParseIdentityChainPath(string(id.AdiChainPath))
	adiToken, tokenChain, err := types.ParseIdentityChainPath(string(ti.URL))
	if err != nil {
		return nil, err
	}

	if adiState != adiToken {
		return nil, fmt.Errorf("ADI URL doesn't match token ADI")
	}
	tas := acctypes.NewTokenAccountState(types.UrlChain(tokenChain), types.UrlChain(tokenChain), ti)

	tasso, err := tas.MarshalBinary()
	if err != nil {
		return nil, err
	}
	resp = &ResponseValidateTX{}
	//so. also need to return the identity chain and chain id these belong to....
	resp.StateData = tasso

	return resp, nil

	//now we need to validate the contents.
	//for _ := range res.Submissions {
	//	//now we need to validate the contents.
	//	//need to validate this: res.Submissions[i].Data()

	return nil, nil
	//return &pb.Submission{}, nil
}

func (v *TokenIssuanceValidator) EndBlock(mdroot []byte) error {
	return nil
}
