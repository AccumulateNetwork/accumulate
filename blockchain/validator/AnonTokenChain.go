package validator

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math/big"
	"time"

	"github.com/AccumulateNetwork/accumulated/types"
	types2 "github.com/AccumulateNetwork/accumulated/types/anonaddress"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	pb "github.com/AccumulateNetwork/accumulated/types/proto"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"
	cfg "github.com/tendermint/tendermint/config"
	"github.com/tendermint/tendermint/crypto/ed25519"
)

type AnonTokenChain struct {
	ValidatorContext

	mdroot [32]byte
}

func NewAnonTokenChain() *AnonTokenChain {
	v := &AnonTokenChain{}
	v.SetInfo(types.ChainTypeAnonTokenAccount[:], types.ChainSpecAnonTokenAccount, pb.AccInstruction_Synthetic_Token_Deposit)
	v.ValidatorContext.ValidatorInterface = v
	return v
}

func (v *AnonTokenChain) Check(currentState *state.StateEntry, submission *transactions.GenTransaction) error {
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
func (v *AnonTokenChain) processDeposit(currentState *state.StateEntry, submission *transactions.GenTransaction, resp *ResponseValidateTX) error {

	//unmarshal the synthetic transaction based upon submission
	deposit := synthetic.TokenTransactionDeposit{}
	err := deposit.UnmarshalBinary(submission.Transaction)
	if err != nil {
		return err
	}

	if deposit.TokenUrl != "dc/ACME" {
		return fmt.Errorf("only ACME tokens can be sent to anonymous token chains")
	}

	//derive the chain for the token account
	adi, _, err := types.ParseIdentityChainPath(deposit.ToUrl.AsString())
	if err != nil {
		return err
	}

	//make sure this is an anonymous address
	if err = types2.IsAcmeAddress(adi); err != nil {
		//need to return to sender
		return fmt.Errorf("deposit token account does not exist and the deposit address is not an anonymous account, %v", err)
	}

	//now check if the anonymous chain already exists.
	//adiStateData := currentState.IdentityState
	chainState := state.Chain{}

	// if the identity state is nil, then it means we do not have any anon accts setup yet.
	if currentState.IdentityState == nil {
		//we'll just create an adi state and set the initial values, and lock it so it cannot be updated.
		chainState.SetHeader(types.String(adi), types.ChainTypeAnonTokenAccount[:])
		//need to flag this as an anonymous account
		data, err := chainState.MarshalBinary()
		if err != nil {
			return nil
		}
		resp.AddStateData(types.GetChainIdFromChainPath(&adi), data)
	} else {
		err := chainState.UnmarshalBinary(currentState.IdentityState.Entry)
		if err != nil {
			return err
		}
		if bytes.Compare(chainState.Type.Bytes(), types.ChainTypeAnonTokenAccount[:]) != 0 {
			return fmt.Errorf("adi for an anoymous chain is not an anonymous account")
		}
	}

	//Next GetOrCreateTokenAccount
	//the ADI is the Address, so now form the chain from the token type
	url := fmt.Sprintf("%s/%s", adi, deposit.TokenUrl)
	tokenChain := types.GetChainIdFromChainPath(&url)

	//so now look up the token chain from the account
	//The token state *CAN* be nil, if so we need to create it...
	tokenState, err := currentState.DB.GetCurrentEntry(tokenChain[:])
	//if err != nil {
	//	return fmt.Errorf("unable to retrieve token chain for %s, %v", url, err)
	//}

	//Unmarshal or create the token account
	account := &state.TokenAccount{}
	if tokenState == nil {
		//we need to create a new state object.
		account = state.NewTokenAccount(url, *deposit.TokenUrl.AsString())
	} else {
		err = account.UnmarshalBinary(tokenState.Entry)
		if err != nil {
			return err
		}
	}

	//all is good, so subtract the balance
	err = account.AddBalance(&deposit.DepositAmount)
	if err != nil {
		return fmt.Errorf("unable to add deposit balance to account")
	}

	data, err := account.MarshalBinary()

	//add the token account state to the chain.
	resp.AddStateData(tokenChain, data)

	//if we get here it is successful. Store tx body on main chain, and verification data on pending
	txPendingState := state.NewPendingTransaction(submission)
	txState, txPendingState := state.NewTransaction(txPendingState)

	data, err = txState.MarshalBinary()
	resp.AddStateData(&deposit.Txid, data)

	// since we have a successful transaction, we only need to store the transaction
	// header that we can use to verify what is on the main chain. need to store reason...
	data, _ = txPendingState.MarshalBinary()
	resp.AddPendingData(&deposit.Txid, data)

	return nil
}
func (v *AnonTokenChain) processSendToken(currentState *state.StateEntry, submission *transactions.GenTransaction, resp *ResponseValidateTX) error {
	//make sure identity state exists.  no point in continuing if the anonymous identity was never created
	if currentState.IdentityState == nil {
		return fmt.Errorf("identity state does not exist for anonymous transaction")
	}

	//now check to make sure this is really an anon account
	if bytes.Compare(currentState.AdiHeader.Type.Bytes(), types.ChainTypeAnonTokenAccount[:]) != 0 {
		return fmt.Errorf("account adi is not an anonymous account type")
	}

	var err error
	withdrawal := transactions.TokenSend{} //api.TokenTx{}
	_, err = withdrawal.Unmarshal(submission.Transaction)

	if err != nil {
		return fmt.Errorf("error with send token, %v", err)
	}

	//need to derive chain id for coin type account.
	adi, chain, _ := types.ParseIdentityChainPath(&withdrawal.AccountURL)
	if adi != chain {
		return fmt.Errorf("cannot specify sub accounts for anonymous token chains")
	}
	//specify the acme tokenUrl
	acmeTokenUrl := types.String("dc/ACME")

	//this is the actual account url the acme tokens are being sent from
	withdrawal.AccountURL = fmt.Sprintf("%s/%s", adi, acmeTokenUrl)

	//get the ChainId of the acme account for the anon address.
	accountChainId := types.GetChainIdFromChainPath(&withdrawal.AccountURL)

	//because we use a different chain for the anonymous account, we need to fetch it.
	currentState.ChainId = accountChainId
	currentState.ChainState, err = currentState.DB.GetCurrentEntry(accountChainId[:])
	if err != nil {
		return fmt.Errorf("chain state for account not esablished")
	}

	//now check to see if the account is good to send tokens from
	cs, tokenAccountState, err := canSendTokens(currentState, &withdrawal)
	if err != nil {
		return err
	}

	if cs != nil {
		return fmt.Errorf("chain state is of the incorrect type")
	}

	//so far, so good.  Now we need to check to make sure the signing address is ok.  maybe look at moving this upstream from here.
	address := types2.GenerateAcmeAddress(submission.Signature[0].PublicKey)

	if address != string(currentState.AdiHeader.ChainUrl) {
		return fmt.Errorf("invalid address, public key address is %s but account %s ", address, currentState.AdiHeader.ChainUrl)
	}

	//now build the synthetic transactions.
	resp.Submissions = make([]*transactions.GenTransaction, len(withdrawal.Outputs))

	txid := submission.TransactionHash()
	txAmt := big.NewInt(0)
	amt := types.Amount{}
	for i, val := range withdrawal.Outputs {
		//accumulate the total amount of the transaction
		txAmt.Add(txAmt, amt.SetUint64(val.Amount))

		//extract the target identity and chain from the url
		destAdi, destChainPath, err := types.ParseIdentityChainPath(&val.Dest)
		if err != nil {
			return err
		}
		destUrl := types.String(destChainPath)

		//populate the synthetic transaction, each submission will be signed by BVC leader and dispatched
		sub := &transactions.GenTransaction{}
		resp.Submissions[i] = sub

		//set the identity chain for the destination
		sub.Routing = types.GetAddressFromIdentity(&destAdi)
		sub.ChainID = types.GetChainIdFromChainPath(destUrl.AsString()).Bytes()
		sub.SigInfo = &transactions.SignatureInfo{}
		sub.SigInfo.URL = destAdi

		depositTx := synthetic.NewTokenTransactionDeposit(txid[:], &currentState.AdiHeader.ChainUrl, &destUrl)
		err = depositTx.SetDeposit(&acmeTokenUrl, amt.AsBigInt())
		if err != nil {
			return fmt.Errorf("unable to set deposit for synthetic token deposit transaction, %v", err)
		}

		sub.Transaction, err = depositTx.MarshalBinary()
		if err != nil {
			return fmt.Errorf("unable to marshal synthetic token transaction deposit, %v", err)
		}
	}

	err = tokenAccountState.SubBalance(txAmt)
	if err != nil {
		return fmt.Errorf("error subtracting balance from account acc://%s, %v", currentState.AdiHeader.ChainUrl, err)
	}

	data, _ := tokenAccountState.MarshalBinary()
	resp.AddStateData(accountChainId, data)

	//if we get here it is successful.
	var txHash types.Bytes32
	copy(txHash[:], txid)
	//if we get here it is successful. Store tx body on main chain, and verification data on pending
	txPendingState := state.NewPendingTransaction(submission)
	txState, txPendingState := state.NewTransaction(txPendingState)
	data, _ = txState.MarshalBinary()
	resp.AddStateData(&txHash, data)

	// since we have a successful transaction, we only need to store the transaction
	// header that we can use to verify what is on the main chain. need to store reason...
	// need to redo this based upon updated transactions.SigInfo struct.  Need to store marshaled SigInfo + ED25519
	data, _ = txPendingState.MarshalBinary()
	resp.AddPendingData(&txHash, data)

	return nil
}

func (v *AnonTokenChain) processAdiCreate(currentState *state.StateEntry, submission *transactions.GenTransaction, resp *ResponseValidateTX) error {
	//make sure identity state exists.  no point in continuing if the anonymous identity was never created
	if currentState.IdentityState == nil {
		return fmt.Errorf("identity state does not exist for anonymous account")
	}

	//now check to make sure this is really an anon account
	if bytes.Compare(currentState.AdiHeader.Type.Bytes(), types.ChainTypeAnonTokenAccount[:]) != 0 {
		return fmt.Errorf("account adi is not an anonymous account type")
	}

	//this should be done at a higher level...
	//if !adiState.VerifyKey(submission.Signature[0].PublicKey) {
	//	return fmt.Errorf("key is not supported by current ADI state")
	//}
	//
	//if !adiState.VerifyAndUpdateNonce(submission.Signature[0].Nonce) {
	//	return fmt.Errorf("invalid nonce, adi state %d but provided %d", adiState.Nonce, submission.Signature[0].Nonce)
	//}

	ic := api.ADI{}
	err := ic.UnmarshalBinary(submission.Transaction)

	if err != nil {
		return fmt.Errorf("data payload of submission is not a valid identity create message")
	}

	isc := synthetic.NewAdiStateCreate(submission.TransactionHash(), &currentState.AdiHeader.ChainUrl, &ic.URL, &ic.PublicKeyHash)

	if err != nil {
		return err
	}

	iscData, err := isc.MarshalBinary()
	if err != nil {
		return err
	}

	resp = &ResponseValidateTX{}

	//send of a synthetic transaction to the correct network
	resp.Submissions = make([]*transactions.GenTransaction, 1)
	sub := resp.Submissions[0]
	sub.Routing = types.GetAddressFromIdentity(isc.ToUrl.AsString())
	sub.ChainID = types.GetChainIdFromChainPath(isc.ToUrl.AsString()).Bytes()
	sub.Transaction = iscData

	if err != nil {
		return err
	}

	return nil
}

// VerifySignatures so this is a little complicated because we need to determine the signature
//scheme of the underlying address of the token.
func (v *AnonTokenChain) VerifySignatures(ledger types.Bytes, key types.Bytes,
	sig types.Bytes, adiState *state.AdiState) error {

	keyHash := sha256.Sum256(key.Bytes())
	if !adiState.VerifyKey(keyHash[:]) {
		return fmt.Errorf("key cannot be verified with adi key hash")
	}

	//make sure the request is legit.
	if ed25519.PubKey(key.Bytes()).VerifySignature(ledger, sig.Bytes()) == false {
		return fmt.Errorf("invalid signature")
	}

	return nil
}

func (v *AnonTokenChain) Validate(currentState *state.StateEntry, submission *transactions.GenTransaction) (*ResponseValidateTX, error) {

	var err error
	resp := &ResponseValidateTX{}

	switch t := submission.TransactionType(); t {
	case uint64(pb.AccInstruction_Synthetic_Token_Deposit):
		//need to verify synthetic deposit.
		err = v.processDeposit(currentState, submission, resp)
	case uint64(pb.AccInstruction_Token_Transaction):
		err = v.processSendToken(currentState, submission, resp)
	default:
		err = fmt.Errorf("unable to process anonomous token with invalid instruction, %d", t)
	}

	return resp, err
}

func (v *AnonTokenChain) EndBlock(mdroot []byte) error {
	copy(v.mdroot[:], mdroot[:])
	return nil
}

func (v *AnonTokenChain) QueryState(db *state.StateDB) *state.Object {
	return nil
}
