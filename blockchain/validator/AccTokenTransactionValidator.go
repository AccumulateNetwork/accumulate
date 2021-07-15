package validator

import (
	"bytes"
	pb "github.com/AccumulateNetwork/accumulated/api/proto"

	//"crypto/sha256"
	"fmt"
	//"github.com/AccumulateNetwork/SMT/managed"
	acctypes "github.com/AccumulateNetwork/accumulated/blockchain/validator/types"
	cfg "github.com/tendermint/tendermint/config"
	//dbm "github.com/tendermint/tm-db"
	"time"
)

type AccTokenTransactionValidator struct{
	ValidatorContext

    EV *EntryValidator

}

//transactions are just accounts with balances on a given token chain
//what transaction types should be supported?
//type Identity struct {
//	Version int8
//    DDII string
//    PublicKey managed.Hash
//}
//
//func (tx *Identity) MarshalBinary() ([]byte, error){
//	ret := make([]byte, )
//	return nil, nil
//}
//
//func (tx *Identity) UnmarshalBinary(data []byte) error{
//
//	return nil
//}

func NewAccTokenTransactionValidator() *AccTokenTransactionValidator {
	v := AccTokenTransactionValidator{}
	//need the chainid, then hash to get first 8 bytes to make the chainid.
	//by definition a chainid of a factoid block is
	//000000000000000000000000000000000000000000000000000000000000000f
	//the id will be 0x0000000f
	chainid := "000000000000000000000000000000000000000000000000000000000000001D" //does this make sense anymore?
	v.EV = NewEntryValidator()
	v.SetInfo(chainid,"token-transaction", pb.AccInstruction_Token_Transaction)
	v.ValidatorContext.ValidatorInterface = &v
	return &v
}


func (v *AccTokenTransactionValidator) Check(currentstate *StateEntry, identitychain []byte, chainid []byte, p1 uint64, p2 uint64, data []byte) error {
	return nil
}
func (v *AccTokenTransactionValidator) Initialize(config *cfg.Config) error {
	return nil
}

func (v *AccTokenTransactionValidator) BeginBlock(height int64, time *time.Time) error {
	v.lastHeight = v.currentHeight
	v.lastTime = v.currentTime
	v.currentHeight = height
	v.currentTime = *time

	return nil
}

func (v *AccTokenTransactionValidator) Validate(currentstate *StateEntry, identitychain []byte, chainid []byte, p1 uint64, p2 uint64, data []byte) (*ResponseValidateTX,error) {
	if currentstate == nil {
		//but this is to be expected...
		return nil, fmt.Errorf("Current State Not Defined")
	}

	//this builds the entry if valid
	_, err := v.EV.Validate(currentstate, identitychain, chainid, p1, p2, data)

	if err != nil {
		return nil, err
	}

	//now we need to validate the contents.
	//for _ := range res.Submissions {
	//	//now we need to validate the contents.
	//	//need to validate this: res.Submissions[i].Data()
	//}


	e := acctypes.Entry{}

	e.UnmarshalBinary(data[0:p1])

	// { "name" : "Dennis", "Key" : "whatevs" }
	//rules: ExtID[0] = Identity Name
	//the Sha256(ExtID[0]) should == ChainID
	//if
    if ( len(e.ExtIDs[0]) != 0 ) {
    	return nil, fmt.Errorf("Invalid format: Expecting Identity Name in ExtID[0]")
	}


	//identitychain = managed.Hash( sha256.Sum256(e.ExtIDs[0]) )
	if bytes.Compare(identitychain, e.ChainID.Bytes() ) != 0 {
		return nil, fmt.Errorf("Invalid Entry: Identity name does not match ChainID")
	}
	//self validation...

	//the chain ID needs to be the name of the DDII.
	if e.Version != 1 {
		return nil, fmt.Errorf("Execting Version 1 ")
	}



	return nil, nil
	//return &pb.Submission{}, nil
}

func (v *AccTokenTransactionValidator) EndBlock(mdroot []byte) error  {
	//copy(v.mdroot[:], mdroot[:])
	//don't think this serves a purpose???
	return nil
}