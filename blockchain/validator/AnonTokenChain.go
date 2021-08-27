package validator

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	pb "github.com/AccumulateNetwork/accumulated/types/proto"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"
	cfg "github.com/tendermint/tendermint/config"
	"time"
)

type AnonTokenChain struct {
	ValidatorContext

	mdroot [32]byte
}

func NewAnonTokenChain() *AnonTokenChain {
	v := AnonTokenChain{}
	v.SetInfo(api.ChainTypeAnonTokenAccount[:], api.ChainSpecAnonTokenAccount, pb.AccInstruction_Synthetic_Token_Deposit)
	v.ValidatorContext.ValidatorInterface = &v
	return &v
}

func (v *AnonTokenChain) Check(currentstate *StateEntry, identitychain []byte, chainid []byte, p1 uint64, p2 uint64, data []byte) error {
	return nil
}
func (v *AnonTokenChain) Initialize(config *cfg.Config) error {
	return nil
}

func (v *AnonTokenChain) BeginBlock(height int64, time *time.Time) error {
	v.lastHeight = v.currentHeight
	v.lastTime = v.currentTime
	v.currentHeight = height
	v.currentTime = *time

	return nil
}
func (v *AnonTokenChain) processDeposit(currentState *StateEntry, submission *pb.Submission, resp *ResponseValidateTX) error {

	//unmarshal the synthetic transaction based upon submission
	deposit := synthetic.TokenTransactionDeposit{}
	err := json.Unmarshal(submission.Data, &deposit)
	if err != nil {
		return err
	}

	//derive the chain for the token account
	adi, _, err := types.ParseIdentityChainPath(submission.AdiChainPath)
	if err != nil {
		return err
	}

	//the ADI is the Address, so now form the chain from the token type
	url := fmt.Sprintf("%s/%s", adi, deposit.TokenUrl)
	tokenChain := sha256.Sum256([]byte(url))

	//so now look up the token chain from the account
	data := currentState.DB.Get("Entry", "", tokenChain[:])

	//Unmarshal or create the token account
	account := &state.TokenAccount{}
	if data == nil {
		//we need to create a new state object.
		account = state.NewTokenAccount(types.UrlChain(url), types.UrlChain(deposit.TokenUrl))
	} else {
		err = account.UnmarshalBinary(data)
		if err != nil {
			return err
		}
	}

	//now check if the anonymous chain already exists.
	adiStateData := currentState.IdentityState
	chainState := state.Chain{}
	if currentState.IdentityState != nil {
		err := chainState.UnmarshalBinary(adiStateData.Entry)
		if err != nil {
			return err
		}
		if bytes.Compare(chainState.Type.Bytes(), api.ChainTypeAnonTokenAccount[:]) != 0 {
			return fmt.Errorf("adi for an anoymous chain is not an anonymous account")
		}
		//we have an adi state, so now compare the key and validation
	} else {
		//we'll just create an adi state and set the initial values, and lock it so it cannot be updated.
		chainState.SetHeader(types.UrlChain(adi), api.ChainTypeAnonTokenAccount[:])
		//need to flag this as an anonymous account
		data, err := chainState.MarshalBinary()
		if err != nil {
			return nil
		}
		resp.StateData = data //this need to be appended into an array
	}

	//all is good, so subtract the balance
	account.AddBalance(&deposit.DepositAmount)

	data, err = account.MarshalBinary()

	//todo: since we potentially added a state already, this one needs to be appended.
	resp.StateData = data

	return nil
}

func (v *AnonTokenChain) Validate(currentState *StateEntry, submission *pb.Submission) (*ResponseValidateTX, error) {

	resp := &ResponseValidateTX{}

	switch submission.Instruction {
	case pb.AccInstruction_Synthetic_Token_Deposit:
		v.processDeposit(currentState, submission, resp)
	case pb.AccInstruction_Token_Transaction:

	}

	return resp, nil
}

func (v *AnonTokenChain) EndBlock(mdroot []byte) error {
	copy(v.mdroot[:], mdroot[:])
	return nil
}
