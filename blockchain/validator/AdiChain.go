package validator

import (
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	pb "github.com/AccumulateNetwork/accumulated/types/proto"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"
	"github.com/tendermint/tendermint/crypto/ed25519"

	//"crypto/sha256"
	"fmt"

	cfg "github.com/tendermint/tendermint/config"
	//dbm "github.com/tendermint/tm-db"
	"time"
)

type AdiChain struct {
	ValidatorContext
}

func NewAdiChain() *AdiChain {
	v := AdiChain{}
	v.SetInfo(types.ChainTypeAdi[:], types.ChainSpecAdi, pb.AccInstruction_Identity_Creation)
	v.ValidatorContext.ValidatorInterface = &v
	return &v
}

func (v *AdiChain) Check(currentstate *state.StateEntry, submission *transactions.GenTransaction) error {
	if currentstate == nil {
		//but this is to be expected...
		return fmt.Errorf("current state not defined")
	}

	return nil
}

func (v *AdiChain) Initialize(config *cfg.Config) error {
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

func (v *AdiChain) Validate(currentstate *state.StateEntry, submission *transactions.GenTransaction) (resp *ResponseValidateTX, err error) {
	if currentstate == nil {
		//but this is to be expected...
		return nil, fmt.Errorf("current State Not Defined")
	}

	if currentstate.IdentityState == nil {
		return nil, fmt.Errorf("sponsor identity is not defined")
	}

	adiState := state.AdiState{}
	err = adiState.UnmarshalBinary(currentstate.IdentityState.Entry)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal adi state entry, %v", err)
	}

	if !adiState.VerifyKey(submission.Signature[0].PublicKey) {
		return nil, fmt.Errorf("key is not supported by current ADI state")
	}

	if !adiState.VerifyAndUpdateNonce(submission.Nonce) {
		return nil, fmt.Errorf("invalid nonce, adi state %d but provided %d", adiState.Nonce, submission.Nonce)
	}

	if !submission.ValidateSig() {
		return nil, fmt.Errorf("error validating the sponsor identity key, %v", err)
	}

	ic := api.ADI{}
	err = ic.UnmarshalBinary(submission.Transaction)

	if err != nil {
		return nil, fmt.Errorf("data payload of submission is not a valid identity create message")
	}

	isc := synthetic.NewAdiStateCreate(submission.TxId(), &adiState.ChainUrl, &ic.URL, &ic.PublicKeyHash)

	if err != nil {
		return nil, err
	}

	iscData, err := isc.MarshalBinary()
	if err != nil {
		return nil, err
	}

	resp = &ResponseValidateTX{}

	//send of a synthetic transaction to the correct network
	resp.Submissions = make([]*transactions.GenTransaction, 1)
	sub := resp.Submissions[0]
	sub.Routing = types.GetAddressFromIdentity(isc.ToUrl.AsString())
	sub.ChainID = types.GetChainIdFromChainPath(isc.ToUrl.AsString()).Bytes()
	sub.Transaction = iscData

	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (v *AdiChain) EndBlock(mdroot []byte) error {
	return nil
}
