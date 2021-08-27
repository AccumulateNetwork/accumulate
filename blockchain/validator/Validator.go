package validator

import (
	smtdb "github.com/AccumulateNetwork/SMT/storage/database"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/state"

	pb "github.com/AccumulateNetwork/accumulated/types/proto"
	//nm "github.com/AccumulateNetwork/accumulated/vbc/node"
	cfg "github.com/tendermint/tendermint/config"
	"time"
)

type StateEntry struct {
	IdentityState *state.Object
	ChainState    *state.Object

	DB *smtdb.Manager
}

func NewStateEntry(idstate *state.Object, chainstate *state.Object, db *smtdb.Manager) (*StateEntry, error) {
	se := StateEntry{}
	se.IdentityState = idstate

	se.ChainState = chainstate
	se.DB = db

	return &se, nil
}

type ResponseValidateTX struct {
	StateData   map[types.Bytes32]types.Bytes //acctypes.StateObject
	EventData   []byte                        //this should be events that need to get published
	Submissions []*pb.Submission              //this is a list of submission instructions for the BVC: entry commit/reveal, synth tx, etc.
}

func (r *ResponseValidateTX) AddStateData(chainid *types.Bytes32, stateData []byte) {
	if r.StateData == nil {
		r.StateData = make(map[types.Bytes32]types.Bytes)
	}
	r.StateData[*chainid] = stateData
}

type ValidatorInterface interface {
	Initialize(config *cfg.Config) error //what info do we need here, we need enough info to perform synthetic transactions.
	BeginBlock(height int64, Time *time.Time) error
	Check(currentstate *StateEntry, identitychain []byte, chainid []byte, p1 uint64, p2 uint64, data []byte) error
	Validate(currentstate *StateEntry, submission *pb.Submission) (*ResponseValidateTX, error) //return persistent entry or error
	EndBlock(mdroot []byte) error                                                              //do something with MD root

	SetCurrentBlock(height int64, Time *time.Time, chainid *string) //deprecated
	GetInfo() *ValidatorInfo
	GetCurrentHeight() int64
	GetCurrentTime() *time.Time
	GetCurrentChainId() *string
}

type ValidatorInfo struct {
	chainSpec   string //
	chainTypeId types.Bytes32
	typeid      uint64
}

func (h *ValidatorInfo) SetInfo(chainTypeId types.Bytes, chainSpec string, instructiontype pb.AccInstruction) {
	copy(h.chainTypeId[:], chainTypeId)
	h.chainSpec = chainSpec
	h.typeid = uint64(instructiontype)
}

func (h *ValidatorInfo) GetValidatorChainTypeId() types.Bytes {
	return h.chainTypeId[:]
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
