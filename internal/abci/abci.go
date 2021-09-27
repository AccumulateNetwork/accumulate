// Package abci implements the Accumulate ABCI applications.
//
// Transaction Processing
//
// Tendermint processes transactions in the following phases:
//
//  • BeginBlock
//  • [CheckTx]
//  • [DeliverTx]
//  • EndBlock
//  • Commit
package abci

import (
	"time"

	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/proto"
)

// Version is the version of the ABCI applications.
const Version uint64 = 0x1

type BeginBlockRequest struct {
	IsLeader bool
	Height   int64
	Time     time.Time
}

type EndBlockRequest struct{}

type Chain interface {
	Query(*proto.Query) ([]byte, error)

	BeginBlock(BeginBlockRequest)
	CheckTx(*transactions.GenTransaction) error
	DeliverTx(*transactions.GenTransaction) error
	EndBlock(EndBlockRequest)
	Commit() ([]byte, error)
}
