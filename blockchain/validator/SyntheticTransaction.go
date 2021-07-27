package validator

import (
	pb "github.com/AccumulateNetwork/accumulated/api/proto"
	cfg "github.com/tendermint/tendermint/config"
	//dbm "github.com/tendermint/tm-db"
	"time"
)

type SyntheticTransactionValidator struct {
	ValidatorContext

	mdroot [32]byte
}

//transactions are just accounts with balances on a given token chain
//what transaction types should be supported?
type SyntheticTransaction struct {
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

func (tx *SyntheticTransaction) MarshalBinary() ([]byte, error) {
	return nil, nil
}

func (tx *SyntheticTransaction) UnmarshalBinary(data []byte) error {

	return nil
}

func NewSyntheticTransactionValidator() *SyntheticTransactionValidator {
	v := SyntheticTransactionValidator{}
	//need the chainid, then hash to get first 8 bytes to make the chainid.
	//by definition a chainid of a factoid block is
	//000000000000000000000000000000000000000000000000000000000000000f
	//the id will be 0x0000000f
	chainid := "0000000000000000000000000000000000000000000000000000000000000005"
	v.SetInfo(chainid, "synthetic_transaction", pb.AccInstruction_State_Store)
	v.ValidatorContext.ValidatorInterface = &v
	return &v
}

func (v *SyntheticTransactionValidator) Check(currentstate *StateEntry, identitychain []byte, chainid []byte, p1 uint64, p2 uint64, data []byte) error {
	return nil
}
func (v *SyntheticTransactionValidator) Initialize(config *cfg.Config) error {
	return nil
}

func (v *SyntheticTransactionValidator) BeginBlock(height int64, time *time.Time) error {
	v.lastHeight = v.currentHeight
	v.lastTime = v.currentTime
	v.currentHeight = height
	v.currentTime = *time

	return nil
}

func (v *SyntheticTransactionValidator) Validate(currentstate *StateEntry, identitychain []byte, chainid []byte, p1 uint64, p2 uint64, data []byte) (*ResponseValidateTX, error) {
	return nil, nil
	//return &pb.Submission{}, nil
}

func (v *SyntheticTransactionValidator) EndBlock(mdroot []byte) error {
	copy(v.mdroot[:], mdroot[:])
	//don't think this serves a purpose???
	return nil
}
