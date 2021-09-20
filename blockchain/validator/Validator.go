package validator

import (
	"fmt"
	"time"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	pb "github.com/AccumulateNetwork/accumulated/types/proto"
	"github.com/AccumulateNetwork/accumulated/types/state"
	cfg "github.com/tendermint/tendermint/config"
)

type ResponseValidateTX struct {
	StateData     map[types.Bytes32]types.Bytes  //acctypes.StateObject
	MainChainData map[types.Bytes32]types.Bytes  //stuff to store on pending chain.
	PendingData   map[types.Bytes32]types.Bytes  //stuff to store on pending chain.
	EventData     []byte                         //this should be events that need to get published
	Submissions   []*transactions.GenTransaction //this is a list of synthetic transactions
}

func (r *ResponseValidateTX) AddMainChainData(chainid *types.Bytes32, data []byte) {
	if r.MainChainData == nil {
		r.MainChainData = make(map[types.Bytes32]types.Bytes)
	}
	r.MainChainData[*chainid] = data
}

func (r *ResponseValidateTX) AddStateData(chainid *types.Bytes32, data []byte) {
	if r.StateData == nil {
		r.StateData = make(map[types.Bytes32]types.Bytes)
	}
	r.StateData[*chainid] = data
}

func (r *ResponseValidateTX) AddPendingData(txId *types.Bytes32, data []byte) {
	if r.PendingData == nil {
		r.PendingData = make(map[types.Bytes32]types.Bytes)
	}
	r.PendingData[*txId] = data
}

func (r *ResponseValidateTX) AddSyntheticTransaction(tx *transactions.GenTransaction) {
	r.Submissions = append(r.Submissions, tx)
}

type ValidatorInterface interface {
	Initialize(config *cfg.Config, db *state.StateDB) error
	BeginBlock(height int64, Time *time.Time) error
	Check(currentstate *state.StateEntry, submission *transactions.GenTransaction) error
	Validate(currentstate *state.StateEntry, submission *transactions.GenTransaction) (*ResponseValidateTX, error) //return persistent entry or error
	EndBlock([]byte) error                                                                                         //do something with MD root?

	SetCurrentBlock(height int64, Time *time.Time, chainid *string) //deprecated
	GetInfo() *ValidatorInfo
	GetCurrentHeight() int64
	GetCurrentTime() *time.Time
	GetCurrentChainId() *string
}

type ValidatorInfo struct {
	chainSpec   string //
	chainTypeId uint64
	typeid      uint64
}

func (h *ValidatorInfo) SetInfo(chainTypeId uint64, instructionType pb.AccInstruction) {
	h.chainTypeId = chainTypeId
	h.typeid = uint64(instructionType)
}

func (h *ValidatorInfo) GetValidatorChainTypeId() uint64 {
	return h.chainTypeId
}

func (h *ValidatorInfo) GetChainSpec() *string {
	return &h.chainSpec
}

func (h *ValidatorInfo) GetTypeId() uint64 {
	return h.typeid
}

type ValidatorContext struct {
	ValidatorInterface
	ValidatorInfo
	currentHeight int64
	currentTime   time.Time
	lastHeight    int64
	lastTime      time.Time
	db            *state.StateDB

	validators    map[uint64]*ValidatorContext            //validators keeps a map of child chain validators
	validatorsIns map[pb.AccInstruction]*ValidatorContext //validators keeps a map of child chain validators
}

func (v *ValidatorContext) addValidator(context *ValidatorContext) {
	if v.validatorsIns == nil {
		v.validatorsIns = make(map[pb.AccInstruction]*ValidatorContext)
	}
	if v.validators == nil {
		v.validators = make(map[uint64]*ValidatorContext)
	}

	var key = context.GetValidatorChainTypeId()

	v.validators[key] = context
	v.validatorsIns[pb.AccInstruction(context.GetInfo().GetTypeId())] = context
}

func (v *ValidatorContext) Initialize(config *cfg.Config, db *state.StateDB) error {
	v.db = db
	return nil
}

func (v *ValidatorContext) getValidatorByIns(ins pb.AccInstruction) (*ValidatorContext, error) {
	if val, ok := v.validatorsIns[ins]; ok {
		return val, nil
	}

	return nil, fmt.Errorf("validator not found for instruction, %d", ins)
}

func (v *ValidatorContext) getValidatorByType(validatorType uint64) (*ValidatorContext, error) {

	if val, ok := v.validators[validatorType]; ok {
		return val, nil
	}

	return nil, fmt.Errorf("validator not found for type")
}

func (v *ValidatorContext) GetInfo() *ValidatorInfo {
	return &v.ValidatorInfo
}

func (v *ValidatorContext) GetLastHeight() int64 {
	return v.lastHeight
}

func (v *ValidatorContext) GetLastTime() *time.Time {
	return &v.lastTime
}

func (v *ValidatorContext) GetCurrentHeight() int64 {
	return v.currentHeight
}

func (v *ValidatorContext) GetCurrentTime() *time.Time {
	return &v.currentTime
}

// BeginBlock will set block parameters
func (v *BlockValidatorChain) BeginBlock(height int64, time *time.Time) error {
	v.lastHeight = v.currentHeight
	v.lastTime = v.currentTime
	v.currentHeight = height
	v.currentTime = *time

	for _, v := range v.validators {
		v.BeginBlock(height, time)
	}

	return nil
}
