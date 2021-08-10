package validator

import (
	"bytes"
	"fmt"
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
	//need the chainid, then hash to get first 8 bytes to make the chainid.
	//by definition a chainid of a factoid block is
	//000000000000000000000000000000000000000000000000000000000000000f
	//the id will be 0x0000000f
	chainid := "0000000000000000000000000000000000000000000000000000000000000005"
	v.SetInfo(chainid, "synthetic_transaction", pb.AccInstruction_State_Store)
	v.ValidatorContext.ValidatorInterface = &v
	return &v
}

func (v *SyntheticTransactionDepositValidator) Check(currentstate *StateEntry, identitychain []byte, chainid []byte, p1 uint64, p2 uint64, data []byte) error {
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

func (v *SyntheticTransactionDepositValidator) canTransact(currentstate *StateEntry, identitychain []byte, chainid []byte, p1 uint64, p2 uint64, data []byte) (*state.IdentityState, *state.TokenAccountState, *synthetic.TokenTransactionDeposit, error) {

	ttd := synthetic.NewTokenTransactionDeposit()
	err := ttd.UnmarshalBinary(data)

	if err != nil {
		return nil, nil, nil, err
	}

	ids := state.IdentityState{}
	err = ids.UnmarshalBinary(currentstate.IdentityState.Entry)
	if err != nil {
		return nil, nil, nil, err
	}

	tas := state.TokenAccountState{}
	err = tas.UnmarshalBinary(currentstate.ChainState.Entry)
	if err != nil {
		return &ids, nil, ttd, err
	}

	if bytes.Compare(ttd.IssuerIdentity[:], tas.GetIssuerIdentity().Bytes()) != 0 {
		return &ids, &tas, ttd, fmt.Errorf("Invalid token Issuer identity")
	}

	if bytes.Compare(ttd.IssuerChainId[:], tas.GetIssuerChainId().Bytes()) != 0 {
		return &ids, &tas, ttd, fmt.Errorf("Invalid token Issuer chainid")
	}

	if ttd.DepositAmount.Sign() <= 0 {
		return &ids, &tas, ttd, fmt.Errorf("Deposit must be a positive amount")
	}

	return &ids, &tas, ttd, nil
}

func returnToSenderTx(ttd *synthetic.TokenTransactionDeposit, submission *pb.Submission) (*ResponseValidateTX, error) {
	retsub := ResponseValidateTX{}
	retsub.Submissions = make([]pb.Submission, 1)
	rs := &retsub.Submissions[0]
	rs.Identitychain = ttd.SenderIdentity[:]
	rs.Chainid = ttd.SenderChainId[:]
	rs.Instruction = pb.AccInstruction_Synthetic_Token_Deposit
	//this will reverse the deposit and send it back to the sender.
	retdep := synthetic.TokenTransactionDeposit{}
	copy(retdep.Txid[:], ttd.Txid[:])
	copy(retdep.SenderIdentity[:], submission.Identitychain)
	copy(retdep.SenderChainId[:], submission.Chainid)
	copy(retdep.IssuerIdentity[:], ttd.IssuerIdentity[:])
	copy(retdep.IssuerChainId[:], ttd.IssuerChainId[:])
	retdep.Metadata.UnmarshalJSON([]byte("{\"Deposit failed\"}"))
	retdep.DepositAmount.Set(&ttd.DepositAmount)
	retdepdata, err := retdep.MarshalBinary()
	if err != nil {
		//shouldn't get here.
		return nil, err
	}
	rs.Data = retdepdata
	return &retsub, nil
}

func (v *SyntheticTransactionDepositValidator) Validate(currentstate *StateEntry, submission *pb.Submission) (*ResponseValidateTX, error) {

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
	ret.StateData, err = tas.MarshalBinary()

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

	return &ret, nil
}

func (v *SyntheticTransactionDepositValidator) EndBlock(mdroot []byte) error {
	return nil
}
