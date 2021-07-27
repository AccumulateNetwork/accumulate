package validator

import (
	"bytes"
	"fmt"
	pb "github.com/AccumulateNetwork/accumulated/api/proto"
	vtypes "github.com/AccumulateNetwork/accumulated/blockchain/validator/types"
	cfg "github.com/tendermint/tendermint/config"
	//dbm "github.com/tendermint/tm-db"
	"time"
)

type EntryValidator struct {
	ValidatorContext

	mdroot [32]byte
}

//transactions are just accounts with balances on a given token chain
//what transaction types should be supported?
type EntryBlock struct {
	chainadi    string   //token chain
	txid        [32]byte //transaction id -- sha256[chainadi | txid] defines the scratch chain for the transaction
	intputddii  string   //ddii includes account?
	inputamount int64

	numsignaturesrequired int
	signature             [64]byte //array?

	//assume fees are paid it ATK
	fee int32 //fees in Atoshies

	outputddii    string
	outputaccount string //?
	outputamount  int64
}

func (tx *EntryValidator) MarshalBinary() ([]byte, error) {
	return nil, nil
}

func (tx *EntryValidator) UnmarshalBinary(data []byte) error {

	return nil
}

func NewEntryValidator() *EntryValidator {
	v := EntryValidator{}
	//need the chainid, then hash to get first 8 bytes to make the chainid.
	//by definition a chainid of a factoid block is
	//000000000000000000000000000000000000000000000000000000000000000f
	//the id will be 0x0000000f
	chainid := "0000000000000000000000000000000000000000000000000000000000000005"
	v.SetInfo(chainid, "entry", pb.AccInstruction_Data_Entry)
	v.ValidatorContext.ValidatorInterface = &v
	return &v
}

func (v *EntryValidator) Check(currentstate *StateEntry, identitychain []byte, chainid []byte, p1 uint64, p2 uint64, data []byte) error {
	return nil
}
func (v *EntryValidator) Initialize(config *cfg.Config) error {
	return nil
}

func (v *EntryValidator) BeginBlock(height int64, time *time.Time) error {

	v.lastHeight = v.currentHeight
	v.lastTime = v.currentTime
	v.currentHeight = height
	v.currentTime = *time

	return nil
}

func (v *EntryValidator) Validate(currentstate *StateEntry, identitychain []byte, chainid []byte, p1 uint64, p2 uint64, data []byte) (*ResponseValidateTX, error) {
	datalen := uint64(len(data))

	//safety check to make sure the P1 is valid
	if datalen < p1 {
		return nil, fmt.Errorf("Invalid offset parameter for entry validation")
	}

	commitlen := datalen - p1

	ischaincommit := commitlen == uint64(vtypes.ChainCommitSize)

	//safety check to make sure length of commit is what is expected
	if !ischaincommit {
		if commitlen != uint64(vtypes.EntryCommitSize) {
			return nil, fmt.Errorf("Insufficent commit data for entry")
		}
	}

	height := p2

	ecr := vtypes.EntryCommitReveal{}
	err := ecr.Unmarshal(data)
	if err != nil {
		return nil, fmt.Errorf("Cannot unpack Entry Commit/Reveal for Data entry")
	}

	//first check if entry was signed by a validator (at height?)
	//is public key a validator? ecr.Segwit.Signature.PublicKey
	//todo: search for validators given the pubkey in entry to make sure it is ok...
	if height != 0 {
		addr := vtypes.GetAddressFromIdentityChain(identitychain)
		LeaderAtHeight(addr, height)
	}

	//second make sure entry hash matches segwit entry hash
	entryhash := vtypes.ComputeEntryHash(data[:p1])
	if !bytes.Equal(ecr.Segwit.Entryhash.Bytes(), entryhash[:]) {
		return nil, fmt.Errorf("Entry Hash does not match commit hash")
	}

	identityentry := vtypes.IdentityState{}
	identityentry.UnmarshalBinary(currentstate.IdentityState.Entry)

	//third check if segwit has a valid signature
	if currentstate != nil {
		copy(ecr.Segwit.Signature.PublicKey, identityentry.Publickey[:])
	}

	if !ecr.Segwit.Valid() {
		return nil, fmt.Errorf("Invalid Segwit signature for entry")
	}

	EcRequired := p1 % 250

	//need to check the fees.
	if ischaincommit {
		EcRequired += 1
	}

	//build up a entry block to submit for finality
	resp := ResponseValidateTX{}
	index := 0
	nextchaintype := GetTypeIdFromName("entry-store")

	if ischaincommit {
		resp.Submissions = make([]pb.Submission, 2)
		resp.Submissions[index].Identitychain = identitychain
		resp.Submissions[index].Chainid = chainid //is this the parent chain ?
		resp.Submissions[index].Type = nextchaintype
		resp.Submissions[index].Instruction = pb.AccInstruction_Data_Chain_Creation
		resp.Submissions[index].Data = make([]byte, 32)
		copy(resp.Submissions[index].Data, ecr.Entry.ChainID[:])
		index++
	} else {
		resp.Submissions = make([]pb.Submission, 1)
	}
	//so for data store, should an entry block be created
	//so even if this is a chain commit, do we actually need to know
	resp.Submissions[index].Identitychain = identitychain              //route to the data store on this network
	resp.Submissions[index].Chainid = chainid                          //store the data under this chain
	resp.Submissions[index].Type = GetTypeIdFromName("entry-store")    //not sure if we really need this...
	resp.Submissions[index].Instruction = pb.AccInstruction_Data_Store //this will authorize the storage of the data
	resp.Submissions[index].Data = data[0:p1]
	//should this be returns or just be locked into a shard
	return &resp, nil
	//return &pb.Submission{}, nil
}

func (v *EntryValidator) EndBlock(mdroot []byte) error {
	copy(v.mdroot[:], mdroot[:])
	//don't think this serves a purpose???
	return nil
}
