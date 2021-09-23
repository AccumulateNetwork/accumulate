package validator

import (
	"fmt"
	"time"

	cfg "github.com/tendermint/tendermint/config"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	pb "github.com/AccumulateNetwork/accumulated/types/proto"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"
	"github.com/tendermint/tendermint/crypto/ed25519"
)

type AdiChain struct {
	ValidatorContext
}

func NewAdiChain() *AdiChain {
	v := AdiChain{}
	//the only transactions an adi chain can process is to create another identity or update key (TBD)
	v.SetInfo(types.ChainTypeAdi, pb.AccInstruction_Identity_Creation)
	v.ValidatorContext.ValidatorInterface = &v
	return &v
}

func (v *AdiChain) Initialize(config *cfg.Config, db *state.StateDB) error {
	v.db = db
	return nil
}

func (v *AdiChain) Check(currentstate *state.StateEntry, submission *transactions.GenTransaction) error {
	if currentstate == nil {
		//but this is to be expected...
		return fmt.Errorf("current state not defined")
	}

	return nil
}

func (v *AdiChain) BeginBlock(height int64, time *time.Time) error {
	v.lastHeight = v.currentHeight
	v.lastTime = v.currentTime
	v.currentHeight = height
	v.currentTime = *time

	return nil
}

func (v *AdiChain) VerifySignatures(ledger types.Bytes, key types.Bytes,
	sig types.Bytes, adiState *state.AdiState) error {

	//if this is a synthtic transaction, we need to verify the sender is a legit bvc validator by checkit
	//the dbvc receipt fpr the synth tx. This would be the appropriate place to this.

	//verify the key is valid against what is stored with the identity.
	if !adiState.VerifyKey(key) {
		return fmt.Errorf("key cannot be verified with adi key hash")
	}

	//make sure the request is legit.
	if ed25519.PubKey(key.Bytes()).VerifySignature(ledger, sig.Bytes()) == false {
		return fmt.Errorf("invalid signature")
	}

	return nil
}

func (v *AdiChain) processAdiCreate(currentstate *state.StateEntry, submission *transactions.GenTransaction, resp *ResponseValidateTX) error {
	if currentstate == nil {
		//but this is to be expected...
		return fmt.Errorf("current State Not Defined")
	}

	if currentstate.IdentityState == nil {
		return fmt.Errorf("sponsor identity is not defined")
	}

	if submission.Signature == nil {
		return fmt.Errorf("no signatures available")
	}

	adiState := state.AdiState{}
	err := adiState.UnmarshalBinary(currentstate.IdentityState.Entry)
	if err != nil {
		return fmt.Errorf("unable to unmarshal adi state entry, %v", err)
	}

	//this should be done at a higher level...
	if !adiState.VerifyKey(submission.Signature[0].PublicKey) {
		return fmt.Errorf("key is not supported by current ADI state")
	}

	if !adiState.VerifyAndUpdateNonce(submission.SigInfo.Nonce) {
		return fmt.Errorf("invalid nonce, adi state %d but provided %d", adiState.Nonce, submission.SigInfo.Nonce)
	}

	ic := api.ADI{}
	err = ic.UnmarshalBinary(submission.Transaction)

	if err != nil {
		return fmt.Errorf("data payload of submission is not a valid identity create message")
	}

	isc := synthetic.NewAdiStateCreate(submission.TransactionHash(), &adiState.ChainUrl, &ic.URL, &ic.PublicKeyHash)

	if err != nil {
		return err
	}

	iscData, err := isc.MarshalBinary()
	if err != nil {
		return err
	}

	//send of a synthetic transaction to the correct network
	sub := new(transactions.GenTransaction)
	sub.Routing = types.GetAddressFromIdentity(isc.ToUrl.AsString())
	sub.ChainID = types.GetChainIdFromChainPath(isc.ToUrl.AsString()).Bytes()
	sub.SigInfo = &transactions.SignatureInfo{}
	sub.SigInfo.URL = *isc.ToUrl.AsString()
	sub.Transaction = iscData

	resp.AddSyntheticTransaction(sub)

	if err != nil {
		return err
	}

	return nil
}

func (v *AdiChain) Validate(currentState *state.StateEntry, submission *transactions.GenTransaction) (resp *ResponseValidateTX, err error) {
	resp = new(ResponseValidateTX)
	err = v.processAdiCreate(currentState, submission, resp)
	return resp, err
}

func (v *AdiChain) EndBlock(mdroot []byte) error {
	return nil
}
