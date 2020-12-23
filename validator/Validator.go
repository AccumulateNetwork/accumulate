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

	Check(data []byte) error
	Validate(data []byte) error
	InitDBs(config *cfg.Config, dbProvider nm.DBProvider) error
	SetCurrentBlock(height int64,Time *time.Time,chainid *string)
	GetInfo() *ValidatorInfo
	GetCurrentHeight() int64
	GetCurrentTime() *time.Time
	GetCurrentChainId() *string
}

type ValidatorInfo struct {
	address uint64
	chainid [32]byte
	namespace string
}


func (h *ValidatorInfo) SetInfo(chainid string, namespace string) error {

	chainidlen := len(chainid)
	if chainidlen < 32 {
		h.chainid = sha256.Sum256([]byte(chainid))
	} else if chainidlen == 64 {
		_, err := hex.Decode(h.chainid[:],[]byte(chainid))
		if err != nil {
			fmt.Errorf("[Error] cannot decode chainid %s", chainid)
			return err
		}
	} else {
		return fmt.Errorf("[Error] invalid chainid for validator %s", namespace)
	}

	h.address = binary.BigEndian.Uint64(h.chainid[24:])

	h.namespace = namespace
	return nil
}

func (h *ValidatorInfo) GetValidatorChainId() *[32]byte {
	return &h.chainid
}

func (h *ValidatorInfo) GetNamespace() *string {
    return &h.namespace
}

func (h *ValidatorInfo) GetTypeId() uint64 {
	return h.address
}

type ValidatorContext struct {
	ValidatorInterface
	ValidatorInfo
	currentHeight int64
	currentTime time.Time
	lastHeight int64
	lastTime time.Time
	chainId string
	entryDB dbm.DB
}

func (v *ValidatorContext) GetInfo() *ValidatorInfo {
	return &v.ValidatorInfo
}

func (v *ValidatorContext) Check(data []byte) *ValidatorInfo {
	return nil
}

func (v *ValidatorContext) SetCurrentBlock(height int64,time *time.Time,chainid *string) {
	v.lastHeight = v.currentHeight
	v.lastTime = v.currentTime
	v.currentHeight = height
	v.currentTime = *time
	v.chainId = *chainid
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


func (v *ValidatorContext) InitDBs(config *cfg.Config, dbProvider nm.DBProvider) (err error) {

	//v.AccountsDB.Get()
	v.entryDB, err = dbProvider(&nm.DBContext{"entry", config})
	if err != nil {
		return
	}

	//v.KvStoreDB, err = dbProvider(&nm.DBContext{"fctkvStore", config})
	//if err != nil {
	//	return
	//}

	return
}