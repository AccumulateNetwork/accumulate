package validator

import (
	cfg "github.com/tendermint/tendermint/config"
	nm "github.com/tendermint/tendermint/node"
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

type ValidatorInfo struct {
	instanceid int64
	instancename string
	typename string
}

func (h *ValidatorInfo) SetInfo(id int64, name string, typename string) {
	h.instanceid = id
	h.instancename = name
	h.typename = typename

}

func (h *ValidatorInfo) GetInstanceName() *string {
	return &h.instancename
}

func (h *ValidatorInfo) GetTypeName() *string {
    return &h.typename
}

func (h *ValidatorInfo) GetInstanceId() int64 {
	return h.instanceid
}

type ValidatorInterface interface {

	Validate(tx []byte) uint32
    InitDBs(config *cfg.Config, dbProvider nm.DBProvider) error
	SetCurrentBlock(height int64,Time *time.Time,chainid *string)
	GetInfo() *ValidatorInfo
	GetCurrentHeight() int64
	GetCurrentTime() *time.Time
	GetCurrentChainId() *string
}


type ValidatorContext struct {
	ValidatorInterface
	ValidatorInfo
	currentHeight int64
	currentTime time.Time
	currentChainId string
}

func (v *ValidatorContext) GetInfo() *ValidatorInfo {
	return &v.ValidatorInfo
}


func (v *ValidatorContext) SetCurrentBlock(height int64,time *time.Time,chainid *string) {
	v.currentHeight = height
	v.currentTime = *time
	v.currentChainId = *chainid
}

func (v *ValidatorContext) GetCurrentHeight() int64 {
	return v.currentHeight
}

func (v *ValidatorContext) GetCurrentTime() *time.Time {
	return &v.currentTime
}

func (v *ValidatorContext) GetCurrentChainId() *string {
	return &v.currentChainId
}
