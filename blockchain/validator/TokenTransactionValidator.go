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
	"math/big"
	"time"
)

type TokenTransactionValidator struct {
	ValidatorContext
}

//this token validator belings in both a Token (coinbase) and Token Account validator.
func NewTokenTransactionValidator() *TokenTransactionValidator {
	v := TokenTransactionValidator{}
	v.SetInfo(api.ChainTypeToken[:], "token-transaction", pb.AccInstruction_Token_Transaction)
	v.ValidatorContext.ValidatorInterface = &v
	return &v
}

// canTransact is a helper function to parse and check for errors in the transaction data
func (v *TokenTransactionValidator) canSendTokens(currentState *StateEntry, identityChain []byte, chainId []byte, p1 uint64, p2 uint64, data []byte) (*state.AdiState, *state.TokenAccount, *api.TokenTx, error) {

	//unmarshal the data into a TokenTx structure, if this fails no point in continuing.
	var tx api.TokenTx
	err := json.Unmarshal(data, &tx)
	if err != nil {
		return nil, nil, nil, err
	}

	ids := state.AdiState{}
	err = ids.UnmarshalBinary(currentState.IdentityState.Entry)
	if err != nil {
		return nil, nil, nil, err
	}

	tas := state.TokenAccount{}
	err = tas.UnmarshalBinary(currentState.ChainState.Entry)
	if err != nil {
		return nil, nil, nil, err
	}

	//verify the tx.from is from the same identity
	stateAdiChain := types.GetIdentityChainFromIdentity(string(ids.ChainUrl))
	fromAdiChain := types.GetIdentityChainFromIdentity(string(tx.From))
	if bytes.Compare(stateAdiChain[:], fromAdiChain[:]) != 0 {
		return nil, nil, nil, fmt.Errorf("from state object transaction account doesn't match transaction")
	}

	//verify the tx.from is from the same chain
	stateChainId := types.GetChainIdFromChainPath(string(tas.ChainUrl))
	fromChainId := types.GetChainIdFromChainPath(string(tx.From))
	if bytes.Compare(stateChainId[:], fromChainId[:]) != 0 {
		return nil, nil, nil, fmt.Errorf("from state object transaction account doesn't match transaction")
	}

	//now check to see if we can transact
	//really only need to provide one input...
	amt := types.Amount{}
	for _, val := range tx.To {
		amt.Add(amt.AsBigInt(), val.Amount.AsBigInt())
	}

	//make sure the user has enough in the account to perform the transaction
	if tas.GetBalance().Cmp(amt.AsBigInt()) < 0 {
		///insufficient balance
		return nil, nil, nil, fmt.Errorf("insufficient balance")
	}
	return &ids, &tas, &tx, nil
}

// Check will perform a sanity check to make sure transaction seems reasonable
func (v *TokenTransactionValidator) Check(currentState *StateEntry, identityChain []byte, chainId []byte, p1 uint64, p2 uint64, data []byte) error {
	_, _, _, err := v.canSendTokens(currentState, identityChain, chainId, p1, p2, data)
	return err
}

// Initialize
func (v *TokenTransactionValidator) Initialize(config *cfg.Config) error {
	return nil
}

// BeginBlock Sets time and height information for beginning of block
func (v *TokenTransactionValidator) BeginBlock(height int64, time *time.Time) error {
	v.lastHeight = v.currentHeight
	v.lastTime = v.currentTime
	v.currentHeight = height
	v.currentTime = *time

	return nil
}

// Validate validates a token transaction
func (v *TokenTransactionValidator) Validate(currentState *StateEntry, submission *pb.Submission) (*ResponseValidateTX, error) {
	//need to do everything done in "check" and also create a synthetic transaction to add tokens.
	_, tas, tx, err := v.canSendTokens(currentState, submission.Identitychain, submission.Chainid,
		submission.GetParam1(), submission.GetParam2(), submission.GetData())

	if err != nil {
		return nil, err
	}

	ret := ResponseValidateTX{}
	ret.Submissions = make([]*pb.Submission, len(tx.To)+1)

	txAmt := big.NewInt(0)
	for i, val := range tx.To {
		amt := val.Amount.AsBigInt()

		//accumulate the total amount of the transaction
		txAmt.Add(txAmt, amt)

		//extract the target identity and chain from the url
		adi, chainPath, err := types.ParseIdentityChainPath(string(val.URL))
		if err != nil {
			return nil, err
		}

		//get the identity id from the adi
		idChain := types.GetIdentityChainFromIdentity(adi)
		if idChain == nil {
			return nil, fmt.Errorf("Invalid identity chain for %s", adi)
		}

		//populate the synthetic transaction, each submission will be signed by BVC leader and dispatched
		sub := pb.Submission{}
		ret.Submissions[i] = &sub

		//set the identity chain for the destination
		sub.Identitychain = idChain[:]

		//set the chain id for the destination
		destChainId := types.GetChainIdFromChainPath(chainPath)
		sub.Chainid = destChainId[:]

		//set the transaction instruction type to a synthetic token deposit
		sub.Instruction = pb.AccInstruction_Synthetic_Token_Deposit

		depositTx := synthetic.NewTokenTransactionDeposit()
		txid := sha256.Sum256(types.MarshalBinaryLedgerChainId(submission.Chainid, submission.Data, submission.Timestamp))
		err = depositTx.SetDeposit(txid[:], amt)
		if err != nil {
			return nil, fmt.Errorf("unable to set deposit for synthetic token deposit transaction")
		}

		err = depositTx.SetTokenInfo(types.UrlChain(tas.GetChainUrl()))
		if err != nil {
			return nil, fmt.Errorf("unable to set token information for synthetic token deposit transaction")
		}

		err = depositTx.SetSenderInfo(submission.Identitychain, submission.Chainid)
		if err != nil {
			return nil, fmt.Errorf("unable to set sender info for synthetic token deposit transaction")
		}
	}

	//subtract the transfer amount from the balance
	err = tas.SubBalance(txAmt)
	if err != nil {
		return nil, err
	}

	//issue a state change...
	tasso, err := tas.MarshalBinary()

	if err != nil {
		return nil, err
	}

	//return a transaction state object
	ret.AddStateData(types.GetChainIdFromChainPath(string(tx.From)), tasso)

	return &ret, nil
}

// EndBlock
func (v *TokenTransactionValidator) EndBlock(mdroot []byte) error {
	return nil
}
