package validator

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types"
	pb "github.com/AccumulateNetwork/accumulated/types/proto"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"
	cfg "github.com/tendermint/tendermint/config"
	"math/big"
	"time"
)

type TokenTransactionValidator struct {
	ValidatorContext
}

func NewTokenTransactionValidator() *TokenTransactionValidator {
	v := TokenTransactionValidator{}

	//deprecate chainid in this context.. has no meaning.
	chainid := "0000000000000000000000000000000000000000000000000000000000000A75"

	_ = v.SetInfo(chainid, "token-transaction", pb.AccInstruction_Token_Transaction)
	v.ValidatorContext.ValidatorInterface = &v
	return &v
}

// canTransact is a helper function to parse and check for errors in the transaction data
func (v *TokenTransactionValidator) canTransact(currentstate *StateEntry, identitychain []byte, chainid []byte, p1 uint64, p2 uint64, data []byte) (*state.IdentityState, *state.TokenAccountState, *types.TokenTransaction, error) {

	ids := state.IdentityState{}
	err := ids.UnmarshalBinary(currentstate.IdentityState.Entry)
	if err != nil {
		return nil, nil, nil, err
	}

	tas := state.TokenAccountState{}
	err = tas.UnmarshalBinary(currentstate.ChainState.Entry)
	if err != nil {
		return nil, nil, nil, err
	}

	var tx types.TokenTransaction
	err = json.Unmarshal(data, &tx)

	if err != nil {
		return nil, nil, nil, err
	}
	//now check to see if we can transact
	//really only need to provide one input...

	amt := big.NewInt(0)
	for _, val := range tx.Output {
		amt.Add(amt, val)
	}

	if tx.TransferAmount.Cmp(amt) != 0 {
		return nil, nil, nil, fmt.Errorf("Transfer amount (%s) doesn't equal sum of the outputs (%s)",
			tx.TransferAmount.String(), amt.String())
	}

	if tas.GetBalance().Cmp(&tx.TransferAmount) < 0 {
		///insufficient balance
		return nil, nil, nil, fmt.Errorf("Insufficient balance")
	}
	return &ids, &tas, &tx, nil
}

// Check will perform a sanity check to make sure transaction seems reasonable
func (v *TokenTransactionValidator) Check(currentstate *StateEntry, identitychain []byte, chainid []byte, p1 uint64, p2 uint64, data []byte) error {
	_, _, _, err := v.canTransact(currentstate, identitychain, chainid, p1, p2, data)
	return err
}

// Initialize
func (v *TokenTransactionValidator) Initialize(config *cfg.Config) error {
	return nil
}

// BeginBlock Sets time and height information for begining of block
func (v *TokenTransactionValidator) BeginBlock(height int64, time *time.Time) error {
	v.lastHeight = v.currentHeight
	v.lastTime = v.currentTime
	v.currentHeight = height
	v.currentTime = *time

	return nil
}

// Validate validates a token transaction
func (v *TokenTransactionValidator) Validate(currentstate *StateEntry, submission *pb.Submission) (*ResponseValidateTX, error) {
	//need to do everything done in "check" and also create a synthetic transaction to add tokens.
	_, tas, tx, err := v.canTransact(currentstate, submission.Identitychain, submission.Chainid,
		submission.GetParam1(), submission.GetParam2(), submission.GetData())

	if err != nil {
		return nil, err
	}

	ret := ResponseValidateTX{}
	ret.Submissions = make([]pb.Submission, len(tx.Output)+1)

	count := 0
	for outputaddr, val := range tx.Output {
		sub := pb.Submission{}
		adi, chainpath, err := types.ParseIdentityChainPath(outputaddr)
		if err != nil {
			return nil, err
		}
		idchain := types.GetIdentityChainFromIdentity(adi)
		if idchain == nil {
			return nil, fmt.Errorf("Invalid identity chain for %s", adi)
		}
		sub.Identitychain = idchain[:]

		destchainid := types.GetChainIdFromChainPath(chainpath)

		sub.Chainid = destchainid[:]

		sub.Instruction = pb.AccInstruction_Synthetic_Token_Deposit

		deptx := synthetic.NewTokenTransactionDeposit()
		txid := sha256.Sum256(types.MarshalBinaryLedgerChainId(submission.Chainid, submission.Data, submission.Timestamp))
		err = deptx.SetDeposit(txid[:], val)
		if err != nil {
			return nil, fmt.Errorf("unable to set deposit for synthetic token deposit transaction")
		}
		err = deptx.SetTokenInfo(tas.GetIssuerIdentity().Bytes(), tas.GetIssuerChainId().Bytes())
		if err != nil {
			return nil, fmt.Errorf("unable to set token information for synthetic token deposit transaction")
		}

		err = deptx.SetSenderInfo(submission.Identitychain, submission.Chainid)
		if err != nil {
			return nil, fmt.Errorf("unable to set sender info for synthetic token deposit transaction")
		}

		//store the synthetic transactions, each submission will be signed by leader
		ret.Submissions[count] = sub
		count++
	}

	//subtract the transfer amount from the balance
	err = tas.SubBalance(&tx.TransferAmount)
	if err != nil {
		return nil, err
	}

	//issue a state change...
	ret.StateData, err = tas.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return &ret, nil
}

// EndBlock
func (v *TokenTransactionValidator) EndBlock(mdroot []byte) error {
	return nil
}
