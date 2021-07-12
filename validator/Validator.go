package validator

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	smtdb "github.com/AccumulateNetwork/SMT/storage/database"

	//"encoding/binary"
	"github.com/AccumulateNetwork/SMT/smt"

	//"encoding/binary"
	"encoding/hex"
	"fmt"
	pb "github.com/AccumulateNetwork/accumulated/proto"
	nm "github.com/AccumulateNetwork/accumulated/vbc/node"
	cfg "github.com/tendermint/tendermint/config"
	dbm "github.com/tendermint/tm-db"
	time "time"
)

//should define return codes for validation...
type ValidationCode uint32

const (
	Success ValidationCode = 0
	BufferUnderflow = 1
	BufferOverflow = 2
	InvalidSignature = 3
	Fail = 4
)

type TXEvidence struct {
	DDII []byte
	Sig []byte
}

type TXEntry struct {
	Evidence TXEvidence
	ExtIDs *[]byte
	Data []byte
}

type StateEntry struct {
	StateHash []byte
	PrevStateHash []byte
	DDIIPubKey []byte //this is the active pubkey in the keychain for the DDII to verify data against.
	EntryHash []byte //not sure this is needed since it is baked into state hash...
	Entry []byte


	DB *smtdb.Manager
}

func (app StateEntry) Marshal() ([]byte, error){
	var ret []byte

	ret = append(ret, app.StateHash...)
	ret = append(ret, app.PrevStateHash...)
	ret = append(ret, app.DDIIPubKey...)
	ret = append(ret, app.EntryHash...)
	ret = append(ret, app.Entry...)

	return ret, nil
}


func (app StateEntry) Unmarshal(data []byte,db *smtdb.Manager) error {
	if len(data) < 32 + 32 + 32 + 32 + 1 {
		return fmt.Errorf("Insufficient data to unmarshall State Entry.")
	}

	app.StateHash = smt.Hash{}.Bytes()
	app.PrevStateHash = smt.Hash{}.Bytes()
	app.DDIIPubKey = smt.Hash{}.Bytes()
	app.EntryHash = smt.Hash{}.Bytes()


	i := 0
	i += copy(app.StateHash,data[i:32])
	i += copy(app.PrevStateHash,data[i:32])
	i += copy(app.DDIIPubKey, data[i:i+32])
	i += copy(app.EntryHash, data[i:i+32])
	entryhash := sha256.Sum256(data[i:])
	if bytes.Compare(app.EntryHash,entryhash[:]) != 0 {
		return fmt.Errorf("Entry Hash does not match the data hash")
	}
	app.Entry = data[i:]
	
	app.DB = db
	return nil
}


type Fee struct {
	TimeStamp      int64        // 8
	DDII           smt.Hash     // 32
	ChainID        [33]byte     // 33
	Credits        int8         // 1
	SignatureIdx   int8         // 1
	Signature      []byte       // 64 minimum
	// 1 end byte ( 140 bytes for FEE)
	Transaction    []byte       // Transaction
}

func (f Fee) MarshalBinary() ([]byte, error) {
	//smt
	return nil,nil
}




type ResponseValidateTX struct{
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
	Check(currentstate *StateEntry, addr uint64, chainid []byte, p1 uint64, p2 uint64, data []byte) error
	Validate(currentstate *StateEntry, addr uint64, chainid []byte, p1 uint64, p2 uint64, data []byte) (*ResponseValidateTX, error) //return persistent entry or error
	EndBlock(mdroot []byte) error  //do something with MD root

	InitDBs(config *cfg.Config, dbProvider nm.DBProvider) error  //deprecated
	SetCurrentBlock(height int64,Time *time.Time,chainid *string) //deprecated
	GetInfo() *ValidatorInfo
	GetCurrentHeight() int64
	GetCurrentTime() *time.Time
	GetCurrentChainId() *string
}

type ValidatorInfo struct {
	chainadi  string   //
	chainid   smt.Hash //derrived from chain adi
	namespace string
    typeid    uint64

}

//This function will build a chain from an DDII / ADI.  If the string is 64 characters in length, then it is assumed
//to be a hex encoded ChainID instead.
func BuildChainIdFromAdi(chainadi *string) ([]byte, error) {

	chainidlen := len(*chainadi)
	var chainid smt.Hash

	if chainidlen < 32 {
		chainid = sha256.Sum256([]byte(*chainadi))
	} else if chainidlen == 64 {
		_, err := hex.Decode(chainid[:],[]byte(*chainadi))
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
//	return BuildChainAddress(smt.Hash(chainid))
//}
//func BuildChainAddress(chainid smt.Hash) uint64 {
//	hash :=sha256.Sum256(chainid.Bytes())
//	return binary.BigEndian.Uint64(hash[:])
//}
//
//hash :=sha256.Sum256(val.GetValidatorChainId())
//app.chainval[binary.BigEndian.Uint64(hash[:])] = val

func (h *ValidatorInfo) SetInfo(chainadi string, namespace string) error {
	chainid, _ := BuildChainIdFromAdi(&chainadi)
	h.chainid.Extract(chainid)
	h.chainadi = chainadi
	h.namespace = namespace
//	h.address = binary.BigEndian.Uint64(h.chainid[24:])
	h.typeid = GetTypeIdFromName(h.namespace)
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
	//chainId       smt.Hash
	entryDB       dbm.DB
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

//func (v *ValidatorContext) GetChainId() *smt.Hash {
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
