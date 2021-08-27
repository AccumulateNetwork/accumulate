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
	//
	//var err error
	//resp := &ResponseValidateTX{}
	//
	//switch {
	//case pb.AccInstruction_Synthetic_Token_Deposit:
	//	err = v.processDeposit(currentState, submission, resp)
	//case pb.AccInstruction_Token_Transaction:
	//	err = v.processSendToken(currentState, submission, resp)
	//default:
	//	err = fmt.Errorf("unable to process anonomous token with invalid instruction, %d", submission.Instruction)
	//}
	//
	//return resp, err
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
	tokenChain := types.GetChainIdFromChainPath(url)

	//so now look up the token chain from the account
	data := currentState.DB.Get("StateEntries", "", tokenChain[:])

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
		resp.AddStateData(types.GetChainIdFromChainPath(adi), data)
	}

	//all is good, so subtract the balance
	err = account.AddBalance(&deposit.DepositAmount)
	if err != nil {
		return fmt.Errorf("unable to add deposit balance to account")
	}

	data, err = account.MarshalBinary()

	//todo: since we potentially added a state already, this one needs to be appended.
	resp.AddStateData(tokenChain, data)

	return nil
}
func (v *AnonTokenChain) processSendToken(currentState *StateEntry, submission *pb.Submission, resp *ResponseValidateTX) error {
	//unmarshal the synthetic transaction based upon submission
	deposit := api.TokenTx{}
	err := json.Unmarshal(submission.Data, &deposit)
	if err != nil {
		return fmt.Errorf("error with send token, %v", err)
	}

	ts := time.Unix(submission.Timestamp, 0)

	duration := time.Since(ts)
	if duration.Minutes() > 1 {
		return fmt.Errorf("transaction time of validity has elapesd by %f seconds", duration.Seconds()-60)
	}

	cs, tokenAccountState, tokenTx, err := canSendTokens(currentState, submission.Data)
	if err != nil {
		return err
	}

	if cs != nil {
		return fmt.Errorf("chain state is of the incorrect type")
	}

	//extract the chain header, we don't need the entire id
	chainHeader := state.Chain{}
	err = chainHeader.UnmarshalBinary(currentState.IdentityState.Entry)
	if err != nil {
		return err
	}

	keyHash := sha256.Sum256(submission.Key)
	checkSum := sha256.Sum256(keyHash[:20])
	addrBytes := append(keyHash[:20], checkSum[:4]...)
	//generate the address from the key hash.
	address := fmt.Sprintf("0x%x", addrBytes)

	if address != string(chainHeader.ChainUrl) {
		return fmt.Errorf("invalid address, public key address is %s but account %s ", address, chainHeader.ChainUrl)
	}

	//verify the from address
	txFromAdi, txFromChain, err := types.ParseIdentityChainPath(string(tokenTx.From))
	if err != nil {
		return fmt.Errorf("unable to parse tokenTx.From, %v", err)
	}

	if txFromAdi != string(chainHeader.ChainUrl) {
		return fmt.Errorf("invalid address in tokenTx.From, from address is %s but account is %s ", address, chainHeader.ChainUrl)
	}

	amt := types.Amount{}
	for _, val := range tokenTx.To {
		amt.Add(amt.AsBigInt(), val.Amount.AsBigInt())
	}

	err = tokenAccountState.SubBalance(amt.AsBigInt())
	if err != nil {
		return fmt.Errorf("error subtracting balance from account acc://%s, amount %s", txFromChain, txFromChain)
	}

	//now build the synthetic transactions.
	resp.Submissions = make([]*pb.Submission, len(tokenTx.To)+1)

	txAmt := big.NewInt(0)
	for i, val := range tokenTx.To {
		amt := val.Amount.AsBigInt()

		//accumulate the total amount of the transaction
		txAmt.Add(txAmt, amt)

		//extract the target identity and chain from the url
		adi, chainPath, err := types.ParseIdentityChainPath(string(val.URL))
		if err != nil {
			return err
		}

		//get the identity id from the adi
		idChain := types.GetIdentityChainFromIdentity(adi)
		if idChain == nil {
			return fmt.Errorf("Invalid identity chain for %s", adi)
		}

		//populate the synthetic transaction, each submission will be signed by BVC leader and dispatched
		sub := pb.Submission{}
		resp.Submissions[i] = &sub

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
			return fmt.Errorf("unable to set deposit for synthetic token deposit transaction, %v", err)
		}

		err = depositTx.SetTokenInfo(types.UrlChain(tokenAccountState.GetChainUrl()))
		if err != nil {
			return fmt.Errorf("unable to set token information for synthetic token deposit transaction, %v", err)
		}

		err = depositTx.SetSenderInfo(submission.Identitychain, submission.Chainid)
		if err != nil {
			return fmt.Errorf("unable to set sender info for synthetic token deposit transaction, %v", err)
		}
	}

	return nil
}

func (v *AnonTokenChain) Validate(currentState *StateEntry, submission *pb.Submission) (*ResponseValidateTX, error) {

	var err error
	resp := &ResponseValidateTX{}

	switch submission.Instruction {
	case pb.AccInstruction_Synthetic_Token_Deposit:
		err = v.processDeposit(currentState, submission, resp)
	case pb.AccInstruction_Token_Transaction:
		err = v.processSendToken(currentState, submission, resp)
	default:
		err = fmt.Errorf("unable to process anonomous token with invalid instruction, %d", submission.Instruction)
	}

	return resp, err
}

func (v *AnonTokenChain) EndBlock(mdroot []byte) error {
	copy(v.mdroot[:], mdroot[:])
	return nil
}
