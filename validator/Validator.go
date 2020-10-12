package validator

import (
	cfg "github.com/tendermint/tendermint/config"
	nm "github.com/tendermint/tendermint/node"
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
	Validate(tx []byte) uint32
    InitDBs(config *cfg.Config, dbProvider nm.DBProvider) error
}


type ValidationContext struct {
	//tbd
}



