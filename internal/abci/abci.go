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

	"github.com/tendermint/tendermint/crypto/ed25519"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	apiQuery "gitlab.com/accumulatenetwork/accumulate/types/api/query"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

//go:generate go run github.com/golang/mock/mockgen -source abci.go -destination ../mock/abci/abci.go

// Version is the version of the ABCI applications.
const Version uint64 = 0x1

// BeginBlockRequest is the input parameter to Chain.BeginBlock.
type BeginBlockRequest struct {
	IsLeader bool
	Height   int64
	Time     time.Time
}

// BeginBlockResponse is the return value of Chain.BeginBlock.
type BeginBlockResponse struct{}

// SynthTxnReference is a reference to a produced synthetic transaction.
type SynthTxnReference struct {
	Type  uint64   `json:"type,omitempty" form:"type" query:"type" validate:"required"`
	Hash  [32]byte `json:"hash,omitempty" form:"hash" query:"hash" validate:"required"`
	Url   string   `json:"url,omitempty" form:"url" query:"url" validate:"required,acc-url"`
	TxRef [32]byte `json:"txRef,omitempty" form:"txRef" query:"txRef" validate:"required"`
}

// EndBlockRequest is the input parameter to Chain.EndBlock
type EndBlockRequest struct{}

type EndBlockResponse struct {
	NewValidators []ed25519.PubKey
}

// Chain is the interface for the Accumulate transaction (chain) validator.
type Chain interface {
	Query(q *apiQuery.Query, height int64, prove bool) (k, v []byte, err *protocol.Error)

	InitChain(state []byte, time time.Time, blockIndex int64) ([]byte, error)

	BeginBlock(BeginBlockRequest) (BeginBlockResponse, error)
	CheckTx(*transactions.Envelope) (protocol.TransactionResult, *protocol.Error)
	DeliverTx(*transactions.Envelope) (protocol.TransactionResult, *protocol.Error)
	EndBlock(EndBlockRequest) EndBlockResponse
	Commit() ([]byte, error)
}
