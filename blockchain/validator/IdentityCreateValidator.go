package validator

import (
	"crypto/sha256"
	"encoding/json"
	"github.com/AccumulateNetwork/accumulated/types"
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

type CreateIdentityValidator struct {
	ValidatorContext
}

func NewCreateIdentityValidator() *CreateIdentityValidator {
	v := CreateIdentityValidator{}
	//need the chainid, then hash to get first 8 bytes to make the chainid.
	//by definition a chainid of a factoid block is
	//000000000000000000000000000000000000000000000000000000000000000f
	//the id will be 0x0000000f
	chainid := "000000000000000000000000000000000000000000000000000000000000001D" //does this make sense anymore?
	//v.EV = NewEntryValidator()
	v.SetInfo(chainid, "create-identity", pb.AccInstruction_Identity_Creation)
	v.ValidatorContext.ValidatorInterface = &v
	return &v
}

func (v *CreateIdentityValidator) Check(currentstate *StateEntry, identitychain []byte, chainid []byte, p1 uint64, p2 uint64, data []byte) error {
	if currentstate == nil {
		//but this is to be expected...
		return fmt.Errorf("current state not defined")
	}

	return nil
}

func (v *CreateIdentityValidator) Initialize(config *cfg.Config) error {
	return nil
}

func (v *CreateIdentityValidator) BeginBlock(height int64, time *time.Time) error {
	v.lastHeight = v.currentHeight
	v.lastTime = v.currentTime
	v.currentHeight = height
	v.currentTime = *time

	return nil
}

func (v *CreateIdentityValidator) Validate(currentstate *StateEntry, submission *pb.Submission) (resp *ResponseValidateTX, err error) {
	if currentstate == nil {
		//but this is to be expected...
		return nil, fmt.Errorf("current State Not Defined")
	}

	if currentstate.IdentityState == nil {
		return nil, fmt.Errorf("sponsor identity is not defined")
	}

	ic := types.ADI{}
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
	iscdata, err := json.Marshal(isc)
	if err != nil {
		return nil, err
	}

	destid, destchainpath, err := types.ParseIdentityChainPath(string(isc.AdiChainPath))
	if err != nil {
		return nil, fmt.Errorf("invalid adi chain path")
	}
	destidhash := sha256.Sum256([]byte(destid))
	destchainid := sha256.Sum256([]byte(destchainpath))

	resp = &ResponseValidateTX{}

	//send of a synthetic transaction to the correct network
	resp.Submissions = make([]pb.Submission, 1)
	sub := &resp.Submissions[0]
	sub.Instruction = pb.AccInstruction_Synthetic_Identity_Creation
	sub.Timestamp = time.Now().Unix()
	sub.Data = iscdata
	sub.Chainid = destchainid[:]
	sub.Identitychain = destidhash[:]

	return resp, nil
}

func (v *CreateIdentityValidator) EndBlock(mdroot []byte) error {
	return nil
}
