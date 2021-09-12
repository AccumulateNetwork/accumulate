package validator

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	pb "github.com/AccumulateNetwork/accumulated/types/proto"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"
	cfg "github.com/tendermint/tendermint/config"
)

type TokenTransactionValidator struct {
	ValidatorContext
}

//this token validator belings in both a Token (coinbase) and Token Account validator.
func NewTokenTransactionValidator() *TokenTransactionValidator {
	v := TokenTransactionValidator{}
	v.SetInfo(types.ChainTypeToken[:], "token-transaction", pb.AccInstruction_Token_Transaction)
	v.ValidatorContext.ValidatorInterface = &v
	return &v
}

// canTransact is a helper function to parse and check for errors in the transaction data
func canSendTokens(currentState *state.StateEntry, data []byte) (*state.AdiState, *state.TokenAccount, *api.TokenTx, error) {

	if currentState.ChainState == nil {
		return nil, nil, nil, fmt.Errorf("no account exists for the chain")
	}

	if currentState.IdentityState == nil {
		return nil, nil, nil, fmt.Errorf("no identity exists for the chain")
	}
	//unmarshal the data into a TokenTx structure, if this fails no point in continuing.
	var tx api.TokenTx
	err := json.Unmarshal(data, &tx)
	if err != nil {
		return nil, nil, nil, err
	}

	//extract the chain header, we don't need the entire id
	chainHeader := state.Chain{}
	err = chainHeader.UnmarshalBinary(currentState.IdentityState.Entry)
	if err != nil {
		return nil, nil, nil, err
	}

	//now check to see if the chain header is an ADI chain. If so, load the AdiState
	var ids *state.AdiState
	if bytes.Compare(chainHeader.Type[:], types.ChainTypeAdi[:]) == 0 {
		ids = &state.AdiState{}
		err = ids.UnmarshalBinary(currentState.IdentityState.Entry)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	tas := state.TokenAccount{}
	err = tas.UnmarshalBinary(currentState.ChainState.Entry)
	if err != nil {
		return nil, nil, nil, err
	}

	//verify the tx.from is from the same identity
	stateAdiChain := types.GetIdentityChainFromIdentity(chainHeader.ChainUrl.AsString())
	fromAdiChain := types.GetIdentityChainFromIdentity(tx.From.AsString())
	if bytes.Compare(stateAdiChain[:], fromAdiChain[:]) != 0 {
		return nil, nil, nil, fmt.Errorf("from state object transaction account doesn't match transaction")
	}

	//verify the tx.from is from the same chain
	stateChainId := types.GetChainIdFromChainPath(tas.ChainUrl.AsString())
	fromChainId := types.GetChainIdFromChainPath(tx.From.AsString())
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
	return ids, &tas, &tx, nil
}

// Check will perform a sanity check to make sure transaction seems reasonable
func (v *TokenTransactionValidator) Check(currentState *state.StateEntry, identityChain []byte, chainId []byte, p1 uint64, p2 uint64, data []byte) error {
	_, _, _, err := canSendTokens(currentState, data)
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
func (v *TokenTransactionValidator) Validate(currentState *state.StateEntry, submission *pb.Submission) (*ResponseValidateTX, error) {
	//need to do everything done in "check" and also create a synthetic transaction to add tokens.
	ids, tas, tx, err := canSendTokens(currentState, submission.GetData())

	if ids == nil {
		return nil, fmt.Errorf("invalid identity state retrieved for token transaction")
	}

	if err != nil {
		return nil, err
	}

	keyHash := sha256.Sum256(submission.Key)
	if !ids.VerifyKey(keyHash[:]) {
		return nil, fmt.Errorf("key not authorized for signing transaction")
	}

	ts := time.Unix(submission.Timestamp, 0)

	duration := time.Since(ts)
	if duration.Minutes() > 1 {
		return nil, fmt.Errorf("transaction time of validity has elapesd by %f seconds", duration.Seconds()-60)
	}

	txid := sha256.Sum256(types.MarshalBinaryLedgerChainId(submission.Chainid, submission.Data, submission.Timestamp))

	ret := ResponseValidateTX{}
	ret.Submissions = make([]*pb.Submission, len(tx.To)+1)

	txAmt := big.NewInt(0)
	for i, val := range tx.To {
		amt := val.Amount.AsBigInt()

		//accumulate the total amount of the transaction
		txAmt.Add(txAmt, amt)

		//extract the target identity and chain from the url
		adi, chainPath, err := types.ParseIdentityChainPath(val.URL.AsString())
		if err != nil {
			return nil, err
		}

		//get the identity id from the adi
		idChain := types.GetIdentityChainFromIdentity(&adi)
		if idChain == nil {
			return nil, fmt.Errorf("Invalid identity chain for %s", adi)
		}

		//populate the synthetic transaction, each submission will be signed by BVC leader and dispatched
		sub := pb.Submission{}
		ret.Submissions[i] = &sub

		//set the identity chain for the destination
		sub.Identitychain = idChain[:]

		//set the chain id for the destination
		destChainId := types.GetChainIdFromChainPath(&chainPath)
		sub.Chainid = destChainId[:]

		//set the transaction instruction type to a synthetic token deposit
		sub.Instruction = pb.AccInstruction_Synthetic_Token_Deposit

		depositTx := synthetic.NewTokenTransactionDeposit(txid[:], &tas.ChainUrl, &val.URL.String)
		err = depositTx.SetDeposit(&tas.TokenUrl.String, amt)
		if err != nil {
			return nil, fmt.Errorf("unable to set deposit for synthetic token deposit transaction")
		}

		sub.Data, err = depositTx.MarshalBinary()
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
	ret.AddStateData(types.GetChainIdFromChainPath(tx.From.AsString()), tasso)

	return &ret, nil
}

// EndBlock
func (v *TokenTransactionValidator) EndBlock(mdRoot []byte) error {
	_ = mdRoot
	return nil
}
