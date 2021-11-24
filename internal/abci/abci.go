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

	"github.com/AccumulateNetwork/accumulate/protocol"
	apiQuery "github.com/AccumulateNetwork/accumulate/types/api/query"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

//go:generate go run github.com/golang/mock/mockgen -source abci.go -destination ../mock/abci/abci.go

// Version is the version of the ABCI applications.
const Version uint64 = 0x1

type BeginBlockRequest struct {
	IsLeader bool
	Height   int64
	Time     time.Time
}

type EndBlockRequest struct{}

type Chain interface {
	Query(*apiQuery.Query) (k, v []byte, err error)

	BeginBlock(BeginBlockRequest)
	CheckTx(*transactions.GenTransaction) error
	DeliverTx(*transactions.GenTransaction) (*protocol.TxResult, error)
	EndBlock(EndBlockRequest)
	Commit() ([]byte, error)
}

type State interface {
	// BlockIndex returns the current block index/height of the chain
	BlockIndex() int64

	// RootHash returns the root hash of the chain
	RootHash() []byte
}
