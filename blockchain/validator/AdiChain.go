package validator

import (
	"crypto/sha256"
	"encoding/json"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	pb "github.com/AccumulateNetwork/accumulated/types/proto"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"

	//"crypto/sha256"
	"fmt"
	//"github.com/AccumulateNetwork/SMT/managed"
	acctypes "github.com/AccumulateNetwork/accumulated/types/state"
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

func (v *AdiChain) Check(currentstate *StateEntry, identitychain []byte, chainid []byte, p1 uint64, p2 uint64, data []byte) error {
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

func (v *AdiChain) Validate(currentstate *StateEntry, submission *pb.Submission) (resp *ResponseValidateTX, err error) {
	if currentstate == nil {
		//but this is to be expected...
		return nil, fmt.Errorf("current State Not Defined")
	}

	if currentstate.IdentityState == nil {
		return nil, fmt.Errorf("sponsor identity is not defined")
	}

	ic := api.ADI{}
	err = json.Unmarshal(submission.Data, &ic)
	if err != nil {
		return nil, fmt.Errorf("data payload of submission is not a valid identity create message")
	}

	isc := synthetic.NewIdentityStateCreate(string(ic.URL))
	ledger := types.MarshalBinaryLedgerChainId(submission.Chainid, submission.Data, submission.Timestamp)

	txid := sha256.Sum256(ledger)
	copy(isc.Txid[:], txid[:])
	copy(isc.SourceIdentity[:], submission.Identitychain)
	copy(isc.SourceChainId[:], submission.Chainid)
	err = isc.SetKeyData(acctypes.KeyTypeSha256, ic.PublicKeyHash[:])
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
		ChainUrl(isc.GetChainUrl()).
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
