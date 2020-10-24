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

	_instanceid int64
	_instancename string
	_typename string

}

func (h *ValidatorInfo) SetHeader(id int64, name *string, typename *string) {
	h._instanceid = id
	h._instancename = *name
	h._typename = *typename
}

func (h *ValidatorInfo) GetInstanceName() *string {
	return &h._instancename
}

func (h *ValidatorInfo) GetTypeName() *string {
    return &h._typename
}

func (h *ValidatorInfo) GetInstanceId() int64 {
	return h._instanceid
}

type ValidatorInterface interface {

	Validate(tx []byte) uint32
    InitDBs(config *cfg.Config, dbProvider nm.DBProvider) error
	SetCurrentBlock(head int64,Time *time.Time,chainid *string) uint32
	GetInfo() *ValidatorInfo
	GetCurrentHeight() int64
	GetCurrentTime() *time.Time
	GetCurrentChainId() *string
}


type ValidatorContext struct {
	ValidatorInterface
	_info ValidatorInfo
	_currentHeight int64
	_currentTime time.Time
	_currentChainId string
}

func (v *ValidatorContext) GetInfo() *ValidatorInfo {
	return &v._info
}

func (v *ValidatorContext) SetCurrentBlock(height int64,time *time.Time,chainid *string) {
	v._currentHeight = height
	v._currentTime = *time
	v._currentChainId = *chainid
}

func (v *ValidatorContext) GetCurrentHeight() int64 {
	return v._currentHeight
}

func (v *ValidatorContext) GetCurrentTime() *time.Time {
	return &v._currentTime
}

func (v *ValidatorContext) GetCurrentChainId() *string {
	return &v._currentChainId
}
