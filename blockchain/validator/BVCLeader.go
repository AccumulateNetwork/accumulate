package validator

import (
	"github.com/AccumulateNetwork/accumulated/types"
	pb "github.com/AccumulateNetwork/accumulated/types/proto"
	"github.com/AccumulateNetwork/accumulated/types/state"
	cfg "github.com/tendermint/tendermint/config"

	//dbm "github.com/tendermint/tm-db"
	"time"
)

type BVCLeader struct {
	ValidatorContext

	mdroot [32]byte
}

func NewBVCLeader() *BVCLeader {
	v := BVCLeader{}
	v.SetInfo(types.ChainTypeDC[:], types.ChainSpecDC, pb.AccInstruction_State_Query)
	v.ValidatorContext.ValidatorInterface = &v
	return &v
}

func (v *BVCLeader) Check(currentstate *state.StateEntry, submission *pb.GenTransaction) error {
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

func (v *BVCLeader) Validate(currentstate *state.StateEntry, submission *pb.GenTransaction) (*ResponseValidateTX, error) {
	//return persistent entry or error
	return nil, nil
}

func (v *BVCLeader) EndBlock(mdroot []byte) error {
	copy(v.mdroot[:], mdroot[:])
	return nil
}
