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

type ValidatorInterface interface {

	Validate(data []byte) error
	InitDBs(config *cfg.Config, dbProvider nm.DBProvider) error
	SetCurrentBlock(height int64,Time *time.Time,chainid *string)
	GetInfo() *ValidatorInfo
	GetCurrentHeight() int64
	GetCurrentTime() *time.Time
	GetCurrentChainId() *string
}

type ValidatorInfo struct {
	typeid int64
	instancename string
	namespace string
}


func (h *ValidatorInfo) SetInfo(id int64, name string, namespace string) {
	h.typeid = id
	h.instancename = name
	h.namespace = namespace
}

func (h *ValidatorInfo) GetInstanceName() *string {
	return &h.instancename
}

func (h *ValidatorInfo) GetNamespace() *string {
    return &h.namespace
}

func (h *ValidatorInfo) GetTypeId() int64 {
	return h.typeid
}

type ValidatorContext struct {
	ValidatorInterface
	ValidatorInfo
	currentHeight int64
	currentTime time.Time
	lastHeight int64
	lastTime time.Time
	chainId string
}

func (v *ValidatorContext) GetInfo() *ValidatorInfo {
	return &v.ValidatorInfo
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
