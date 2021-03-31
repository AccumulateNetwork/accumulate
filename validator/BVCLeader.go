package validator

import (
	cfg "github.com/tendermint/tendermint/config"
	dbm "github.com/tendermint/tm-db"
	"time"
)

type BVCLeader struct{
	ValidatorContext

	mdroot [32]byte

}

func NewBVCLeader(shard string) *BVCLeader {
	v := FactoidValidator{}
	//need the chainid, then hash to get first 8 bytes to make the chainid.
	//by definition a chainid of a factoid block is
	//000000000000000000000000000000000000000000000000000000000000000f
	//the id will be 0x0000000f
	chainid := "000000000000000000000000000000000000000000000000000000000000000f"
	v.SetInfo(chainid,"factoid")
	v.ValidatorContext.ValidatorInterface = &v
	return &v
}


func (v *BVCLeader) Check(data []byte) *ValidatorInfo {
	return nil
}
func (v *BVCLeader) Initialize(config *cfg.Config) error {
	return nil
}

func (v *BVCLeader) BeginBlock(height int64, time *time.Time) error {
	v.lastHeight = v.currentHeight
	v.lastTime = v.currentTime
	v.currentHeight = height
	v.currentTime = *time

	return nil
}

func (v *BVCLeader) Validate(data []byte) ([]byte, error) {
	//return persistent entry or error
	return nil, nil
}

func (v *BVCLeader) EndBlock(mdroot [32]byte) error  {
	copy(v.mdroot[:], mdroot[:])
	return nil
}