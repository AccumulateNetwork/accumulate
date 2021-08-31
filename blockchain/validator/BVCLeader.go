package validator

import (
	"github.com/AccumulateNetwork/accumulated/types/api"
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
	v.SetInfo(api.ChainTypeDC[:], api.ChainSpecDC, pb.AccInstruction_State_Query)
	v.ValidatorContext.ValidatorInterface = &v
	return &v
}

func (v *BVCLeader) Check(currentstate *state.StateEntry, identitychain []byte, chainid []byte, p1 uint64, p2 uint64, data []byte) error {
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

func (v *BVCLeader) Validate(currentstate *state.StateEntry, submission *pb.Submission) (*ResponseValidateTX, error) {
	//return persistent entry or error
	return nil, nil
}

func (v *BVCLeader) EndBlock(mdroot []byte) error {
	copy(v.mdroot[:], mdroot[:])
	return nil
}
