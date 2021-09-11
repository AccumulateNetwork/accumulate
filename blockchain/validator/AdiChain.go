package validator

import (
	"crypto/sha256"
	"encoding/json"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
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
	v.SetInfo(api.ChainTypeAdi[:], api.ChainSpecAdi, pb.AccInstruction_Identity_Creation)
	v.ValidatorContext.ValidatorInterface = &v
	return &v
}

func (v *AdiChain) Check(currentstate *state.StateEntry, identitychain []byte, chainid []byte, p1 uint64, p2 uint64, data []byte) error {
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

func (v *AdiChain) Validate(currentstate *state.StateEntry, submission *pb.Submission) (resp *ResponseValidateTX, err error) {
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

	ledger := types.MarshalBinaryLedgerChainId(submission.Identitychain, submission.Data, submission.Timestamp)

	txid := sha256.Sum256(ledger)

	err = v.VerifySignatures(ledger, submission.Key, submission.Signature, &adiState)

	if err != nil {
		return nil, fmt.Errorf("error validating the sponsor identity key, %v", err)
	}

	ic := api.ADI{}
	err = json.Unmarshal(submission.Data, &ic)
	if err != nil {
		return nil, fmt.Errorf("data payload of submission is not a valid identity create message")
	}

	isc := synthetic.NewAdiStateCreate(txid[:], &adiState.ChainUrl, &ic.URL, &ic.PublicKeyHash)

	if err != nil {
		return nil, err
	}

	iscData, err := json.Marshal(isc)
	if err != nil {
		return nil, err
	}

	resp = &ResponseValidateTX{}

	builder := pb.SubmissionBuilder{}

	//send of a synthetic transaction to the correct network
	resp.Submissions = make([]*pb.Submission, 1)
	resp.Submissions[0], err = builder.
		Type(v.GetValidatorChainTypeId()).
		Instruction(pb.AccInstruction_Synthetic_Identity_Creation).
		AdiUrl(*isc.ToUrl.AsString()).
		Data(iscData).
		Timestamp(time.Now().Unix()).
		BuildUnsigned()

	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (v *AdiChain) EndBlock(mdroot []byte) error {
	return nil
}
