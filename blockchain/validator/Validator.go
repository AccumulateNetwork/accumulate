package validator

import (
	"crypto/sha256"
	"encoding/binary"
	smtdb "github.com/AccumulateNetwork/SMT/storage/database"
	acctypes "github.com/AccumulateNetwork/accumulated/blockchain/validator/types"

	//"encoding/binary"
	"github.com/AccumulateNetwork/SMT/managed"

	//"encoding/binary"
	"encoding/hex"
	"fmt"
	pb "github.com/AccumulateNetwork/accumulated/api/proto"
	//nm "github.com/AccumulateNetwork/accumulated/vbc/node"
	cfg "github.com/tendermint/tendermint/config"
	dbm "github.com/tendermint/tm-db"
	"time"
)

//should define return codes for validation...
type ValidationCode uint32

const (
	Success          ValidationCode = 0
	BufferUnderflow                 = 1
	BufferOverflow                  = 2
	InvalidSignature                = 3
	Fail                            = 4
)

type TXEvidence struct {
	DDII []byte
	Sig  []byte
}

type TXEntry struct {
	Evidence TXEvidence
	ExtIDs   *[]byte
	Data     []byte
}

type StateEntry struct {
	IdentityState *acctypes.StateObject
	ChainState    *acctypes.StateObject
	//database to query other stuff if needed???
	DB *smtdb.Manager
}

func NewStateEntry(idstate *acctypes.StateObject, chainstate *acctypes.StateObject, db *smtdb.Manager) (*StateEntry, error) {
	se := StateEntry{}
	se.IdentityState = idstate

	se.ChainState = chainstate
	se.DB = db

	return &se, nil
}

type Fee struct {
	TimeStamp    int64        // 8
	DDII         managed.Hash // 32
	ChainID      [33]byte     // 33
	Credits      int8         // 1
	SignatureIdx int8         // 1
	Signature    []byte       // 64 minimum
	// 1 end byte ( 140 bytes for FEE)
	Transaction []byte // Transaction
}

func (f Fee) MarshalBinary() ([]byte, error) {
	//smt
	return nil, nil
}

type ResponseValidateTX struct {
	StateData   []byte          //acctypes.StateObject
	Submissions []pb.Submission //this is a list of submission instructions for the BVC: entry commit/reveal, synth tx, etc.
}

func LeaderAtHeight(addr uint64, height uint64) *[32]byte {
	//todo: implement...
	//lookup the public key for the addr at given height
	return nil
}

type ValidatorInterface interface {
	Initialize(config *cfg.Config) error //what info do we need here, we need enough info to perform synthetic transactions.
	BeginBlock(height int64, Time *time.Time) error
	Check(currentstate *StateEntry, identitychain []byte, chainid []byte, p1 uint64, p2 uint64, data []byte) error
	Validate(currentstate *StateEntry, identitychain []byte, chainid []byte, p1 uint64, p2 uint64, data []byte) (*ResponseValidateTX, error) //return persistent entry or error
	EndBlock(mdroot []byte) error                                                                                                            //do something with MD root

	//InitDBs(config *cfg.Config, dbProvider nm.DBProvider) error  //deprecated
	SetCurrentBlock(height int64, Time *time.Time, chainid *string) //deprecated
	GetInfo() *ValidatorInfo
	GetCurrentHeight() int64
	GetCurrentTime() *time.Time
	GetCurrentChainId() *string
}

type ValidatorInfo struct {
	chainadi  string       //
	chainid   managed.Hash //derrived from chain adi
	namespace string
	typeid    uint64
}

//This function will build a chain from an DDII / ADI.  If the string is 64 characters in length, then it is assumed
//to be a hex encoded ChainID instead.
func BuildChainIdFromAdi(chainadi *string) ([]byte, error) {

	chainidlen := len(*chainadi)
	var chainid managed.Hash

	if chainidlen < 32 {
		chainid = sha256.Sum256([]byte(*chainadi))
	} else if chainidlen == 64 {
		_, err := hex.Decode(chainid[:], []byte(*chainadi))
		if err != nil {
			fmt.Errorf("[Error] cannot decode chainid %s", chainadi)
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("[Error] invalid chainid for validator on shard %s", chainadi)
	}

	return chainid.Bytes(), nil
}

//func BuildChainAddressFromAdi(chain *string) uint64 {
//	chainid,_ := BuildChainIdFromAdi(chain)
//	return BuildChainAddress(managed.Hash(chainid))
//}
//func BuildChainAddress(chainid managed.Hash) uint64 {
//	hash :=sha256.Sum256(chainid.Bytes())
//	return binary.BigEndian.Uint64(hash[:])
//}
//
//hash :=sha256.Sum256(val.GetValidatorChainId())
//app.chainval[binary.BigEndian.Uint64(hash[:])] = val

func (h *ValidatorInfo) SetInfo(chainadi string, namespace string, instructiontype pb.AccInstruction) error {
	chainid, _ := BuildChainIdFromAdi(&chainadi)
	h.chainid.Extract(chainid)
	h.chainadi = chainadi
	h.namespace = namespace
	//	h.address = binary.BigEndian.Uint64(h.chainid[24:])
	h.typeid = uint64(instructiontype) //GetTypeIdFromName(h.namespace)
	h.namespace = namespace
	return nil
}

func (h *ValidatorInfo) GetValidatorChainId() []byte {
	return h.chainid[:]
}

func (h *ValidatorInfo) GetNamespace() *string {
	return &h.namespace
}

func (h *ValidatorInfo) GetChainAdi() *string {
	return &h.chainadi
}

func GetTypeIdFromName(name string) uint64 {
	b := sha256.Sum256([]byte(name))
	return binary.BigEndian.Uint64(b[:8])
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
	//chainId       managed.Hash
	entryDB dbm.DB
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

//func (v *ValidatorContext) GetChainId() *managed.Hash {
//	return &v.chainId
//}

//
//func (v *ValidatorContext) InitDBs(config *cfg.Config, dbProvider nm.DBProvider) (err error) {
//
//	//v.AccountsDB.Get()
//	v.entryDB, err = dbProvider(&nm.DBContext{"entry", config})
//	if err != nil {
//		return
//	}
//
//	//v.KvStoreDB, err = dbProvider(&nm.DBContext{"fctkvStore", config})
//	//if err != nil {
//	//	return
//	//}
//
//	return
//}
