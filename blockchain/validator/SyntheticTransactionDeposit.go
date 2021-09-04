package validator

import (
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	pb "github.com/AccumulateNetwork/accumulated/types/proto"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"
	cfg "github.com/tendermint/tendermint/config"
	//dbm "github.com/tendermint/tm-db"
	"time"
)

type SyntheticTransactionDepositValidator struct {
	ValidatorContext
}

func NewSyntheticTransactionDepositValidator() *SyntheticTransactionDepositValidator {
	v := SyntheticTransactionDepositValidator{}
	v.SetInfo(api.ChainTypeTokenAccount[:], "synthetic-transaction-deposit", pb.AccInstruction_Synthetic_Token_Deposit)
	v.ValidatorContext.ValidatorInterface = &v
	return &v
}

func (v *SyntheticTransactionDepositValidator) Check(currentstate *state.StateEntry, identitychain []byte, chainid []byte, p1 uint64, p2 uint64, data []byte) error {
	_, _, _, err := v.canTransact(currentstate, identitychain, chainid, p1, p2, data)
	return err
}
func (v *SyntheticTransactionDepositValidator) Initialize(config *cfg.Config) error {
	return nil
}

func (v *SyntheticTransactionDepositValidator) BeginBlock(height int64, time *time.Time) error {
	v.lastHeight = v.currentHeight
	v.lastTime = v.currentTime
	v.currentHeight = height
	v.currentTime = *time

	return nil
}

func (v *SyntheticTransactionDepositValidator) canTransact(currentstate *state.StateEntry, identitychain []byte, chainid []byte, p1 uint64, p2 uint64, data []byte) (*state.AdiState, *state.TokenAccount, *synthetic.TokenTransactionDeposit, error) {

	ttd := synthetic.NewTokenTransactionDeposit()
	err := ttd.UnmarshalBinary(data)

	if err != nil {
		return nil, nil, nil, err
	}

	ids := state.AdiState{}
	err = ids.UnmarshalBinary(currentstate.IdentityState.Entry)
	if err != nil {
		return nil, nil, nil, err
	}

	tas := state.TokenAccount{}
	err = tas.UnmarshalBinary(currentstate.ChainState.Entry)
	if err != nil {
		return &ids, nil, ttd, err
	}

	if ttd.TokenUrl != types.String(tas.GetChainUrl()) {
		return &ids, &tas, ttd, fmt.Errorf("Invalid token Issuer identity")
	}

	if ttd.DepositAmount.Sign() <= 0 {
		return &ids, &tas, ttd, fmt.Errorf("Deposit must be a positive amount")
	}

	return &ids, &tas, ttd, nil
}

func returnToSenderTx(ttd *synthetic.TokenTransactionDeposit, submission *pb.Submission) (*ResponseValidateTX, error) {
	retsub := ResponseValidateTX{}
	retsub.Submissions = make([]*pb.Submission, 1)
	retsub.Submissions[0] = &pb.Submission{}
	rs := retsub.Submissions[0]
	rs.Identitychain = ttd.SourceAdiChain[:]
	rs.Chainid = ttd.SourceChainId[:]
	rs.Instruction = pb.AccInstruction_Synthetic_Token_Deposit
	//this will reverse the deposit and send it back to the sender.
	retdep := synthetic.TokenTransactionDeposit{}
	copy(retdep.Txid[:], ttd.Txid[:])
	copy(retdep.SourceAdiChain[:], submission.Identitychain)
	copy(retdep.SourceChainId[:], submission.Chainid)
	retdep.TokenUrl = ttd.TokenUrl
	err := retdep.Metadata.UnmarshalJSON([]byte("{\"deposit failed\"}"))
	if err != nil {
		return nil, err
	}
	retdep.DepositAmount.Set(&ttd.DepositAmount)
	retdepdata, err := retdep.MarshalBinary()
	if err != nil {
		//shouldn't get here.
		return nil, err
	}
	rs.Data = retdepdata
	return &retsub, nil
}

func (v *SyntheticTransactionDepositValidator) Validate(currentstate *state.StateEntry, submission *pb.Submission) (*ResponseValidateTX, error) {

	_, tas, ttd, err := v.canTransact(currentstate, submission.Identitychain, submission.Chainid,
		submission.Param1, submission.Param2, submission.Data)

	if ttd != nil && err != nil {
		//return to sender...
		rts, err2 := returnToSenderTx(ttd, submission)
		if err2 != nil {
			//shouldn't get here.
			return nil, err2
		}
		return rts, err
	}

	if err != nil {
		//the transaction data is bad and cannot credit nor return funds.
		return nil, err
	}

	//Modify the state object -> add the deposit amount to the balance
	err = tas.AddBalance(&ttd.DepositAmount)
	if err != nil {
		rts, err2 := returnToSenderTx(ttd, submission)
		if err2 != nil {
			//shouldn't get here.
			return nil, err2
		}
		return rts, err
	}

	//Tell BVC to modify the state for the chain
	ret := ResponseValidateTX{}
	//TODO: should we send back an ack tx to the sender? ret.Submissions = make([]pb.Submission, 1)

	//Marshal the state change...
	stateData, err := tas.MarshalBinary()

	//make sure marshalling went ok, if it didn't send the transaction back to sender.
	if err != nil {
		//we failed to marshal the token account state, shouldn't get here...
		rts, err2 := returnToSenderTx(ttd, submission)
		if err2 != nil {
			//shouldn't get here.
			return nil, err2
		}
		return rts, err
	}

	ret.AddStateData(types.GetChainIdFromChainPath(tas.ChainUrl.AsString()), stateData)

	return &ret, nil
}

func (v *SyntheticTransactionDepositValidator) EndBlock(mdroot []byte) error {
	return nil
}
