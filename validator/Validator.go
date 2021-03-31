package validator

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
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

type ValidatorInterface interface {

	Initialize(config *cfg.Config) error //what info do we need here, we need enough info to perform synthetic transactions.
	BeginBlock(height int64, Time *time.Time) error
	Check(ins int32, p1 uint64, p2 uint64, data []byte) error
	Validate(data []byte) ([]byte, error) //return persistent entry or error
	EndBlock(mdroot []byte) error  //do something with MD root


	InitDBs(config *cfg.Config, dbProvider nm.DBProvider) error  //deprecated
	SetCurrentBlock(height int64,Time *time.Time,chainid *string) //deprecated
	GetInfo() *ValidatorInfo
	GetCurrentHeight() int64
	GetCurrentTime() *time.Time
	GetCurrentChainId() *string

}

type ValidatorInfo struct {
	chainadi     string   //
	chainid [32]byte //derrived from chain adi
	namespace   string

}


func (h *ValidatorInfo) SetInfo(chainadi string, namespace string) error {

	chainidlen := len(chainadi)
	if chainidlen < 32 {
		h.chainid = sha256.Sum256([]byte(chainadi))
	} else if chainidlen == 64 {
		_, err := hex.Decode(h.chainid[:],[]byte(chainadi))
		if err != nil {
			fmt.Errorf("[Error] cannot decode chainid %s", chainadi)
			return err
		}
	} else {
		return fmt.Errorf("[Error] invalid chainid for validator on shard %s", namespace)
	}

	h.chainadi = chainadi;
//	h.address = binary.BigEndian.Uint64(h.chainid[24:])

	h.namespace = namespace
	return nil
}

func (h *ValidatorInfo) GetValidatorChainId() *[32]byte {
	return &h.chainid
}

func (h *ValidatorInfo) GetNamespace() *string {
    return &h.namespace
}

func (h *ValidatorInfo) GetChainAdi() uint64 {
	return h.chainadi
}

type ValidatorContext struct {
	ValidatorInterface
	ValidatorInfo
	currentHeight int64
	currentTime   time.Time
	lastHeight    int64
	lastTime      time.Time
	chainId       string
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

func (v *ValidatorContext) GetChainId() *string {
	return &v.chainId
}

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