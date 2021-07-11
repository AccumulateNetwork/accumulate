package validator

import (
	"bytes"
	"fmt"
	pb "github.com/AccumulateNetwork/accumulated/proto"
	vtypes "github.com/AccumulateNetwork/accumulated/validator/types"
	cfg "github.com/tendermint/tendermint/config"
	//dbm "github.com/tendermint/tm-db"
	"time"
)

type EntryValidator struct{
	ValidatorContext

	mdroot [32]byte

}



//transactions are just accounts with balances on a given token chain
//what transaction types should be supported?
type EntryBlock struct {
	chainadi string //token chain
	txid [32]byte //transaction id -- sha256[chainadi | txid] defines the scratch chain for the transaction
	intputddii string //ddii includes account?
	inputamount int64

	numsignaturesrequired int
	signature [64]byte  //array?


	//assume fees are paid it ATK
	fee int32 //fees in Atoshies

	outputddii string
    outputaccount string //?
    outputamount int64
}

func (tx *EntryValidator) MarshalBinary() ([]byte, error){
	return nil, nil
}

func (tx *EntryValidator) UnmarshalBinary(data []byte) error{

	return nil
}

func NewEntryValidator() *EntryValidator {
	v := EntryValidator{}
	//need the chainid, then hash to get first 8 bytes to make the chainid.
	//by definition a chainid of a factoid block is
	//000000000000000000000000000000000000000000000000000000000000000f
	//the id will be 0x0000000f
	chainid := "0000000000000000000000000000000000000000000000000000000000000005"
	v.SetInfo(chainid,"entry")
	v.ValidatorContext.ValidatorInterface = &v
	return &v
}


func (v *EntryValidator) Check(addr uint64, chainid []byte, p1 uint64, p2 uint64, data []byte) error {
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



func (v *EntryValidator) Validate(addr uint64, chainid []byte, p1 uint64, p2 uint64, data []byte) (*ResponseValidateTX,error) {
	datalen := uint64(len(data))

	//safety check to make sure the P1 is valid
	if datalen < p1 {
		return nil, fmt.Errorf("Invalid offset parameter for entry validation")
	}

	commitlen := uint64(len(data)) - p1

	ischaincommit := (commitlen == uint64(vtypes.ChainCommitSize))

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
		LeaderAtHeight(addr, height)

	}

	//second make sure entry hash matches segwit entry hash
	entryhash := vtypes.ComputeEntryHash(data[:p1])
	if !bytes.Equal(ecr.Segwit.Entryhash.Bytes(), entryhash[:] ) {
		return nil, fmt.Errorf("Entry Hash does not match commit hash")
	}

	//third check if segwit is valid
	if !ecr.Segwit.Valid() {
		return nil, fmt.Errorf("Invalid Segwit signature for entry")
	}


	var entryblock [128]byte
	//build up a entry block to submit for finality
	resp := ResponseValidateTX{}
	index := 0
	nextchaintype := GetTypeIdFromName("entry-store")
	if ischaincommit {
		resp.Submissions = make([]pb.Submission,2)
		resp.Submissions[index].Address = addr
		resp.Submissions[index].Chainid = chainid //is this the parent chain ?
		resp.Submissions[index].Type = nextchaintype
		resp.Submissions[index].Instruction = pb.AccInstruction_Data_Chain_Creation
		resp.Submissions[index].Data = make([]byte,32)
		copy(resp.Submissions[index].Data, ecr.Entry.ChainID[:])
		index++
	} else {
		resp.Submissions = make([]pb.Submission,1)
	}

	resp.Submissions[index].Address = addr //TBD: this probably needs to be addressed to the Data Store shard.
	resp.Submissions[index].Chainid = chainid
	resp.Submissions[index].Type = GetTypeIdFromName("entry-store")
	resp.Submissions[index].Instruction = pb.AccInstruction_Data_Entry //really needs to be a directl
	resp.Submissions[index].Data = entryblock[:]
	//should this be returns or just be locked into a shard
	return &resp, nil
	//return &pb.Submission{}, nil
}

func (v *EntryValidator) EndBlock(mdroot []byte) error  {
	copy(v.mdroot[:], mdroot[:])
	//don't think this serves a purpose???
	return nil
}