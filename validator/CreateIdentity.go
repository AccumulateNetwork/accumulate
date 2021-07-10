package validator

import (
	cfg "github.com/tendermint/tendermint/config"
	//dbm "github.com/tendermint/tm-db"
	"time"
)

type CreateIdentityValidator struct{
	ValidatorContext


}

//transactions are just accounts with balances on a given token chain
//what transaction types should be supported?
type CreateIdentity struct {



}

func (tx *CreateIdentity) MarshalBinary() ([]byte, error){
	return nil, nil
}

func (tx *CreateIdentity) UnmarshalBinary(data []byte) error{

	return nil
}

func NewCreateIdentityValidator() *CreateIdentityValidator {
	v := CreateIdentityValidator{}
	//need the chainid, then hash to get first 8 bytes to make the chainid.
	//by definition a chainid of a factoid block is
	//000000000000000000000000000000000000000000000000000000000000000f
	//the id will be 0x0000000f
	chainid := "0000000000000000000000000000000000000000000000000000000000000005"
	v.SetInfo(chainid,"synthetic_transaction")
	v.ValidatorContext.ValidatorInterface = &v
	return &v
}


func (v *CreateIdentityValidator) Check(addr uint64, chainid []byte, p1 uint64, p2 uint64, data []byte) error {
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

func (v *CreateIdentityValidator) Validate(addr uint64, chainid []byte, p1 uint64, p2 uint64, data []byte) (*ResponseValidateTX,error) {
	return nil, nil
	//return &pb.Submission{}, nil
}

func (v *CreateIdentityValidator) EndBlock(mdroot []byte) error  {
	//copy(v.mdroot[:], mdroot[:])
	//don't think this serves a purpose???
	return nil
}