package api

import (
	"context"

	"github.com/tendermint/tendermint/libs/bytes"
	core "github.com/tendermint/tendermint/rpc/core/types"
)

//go:generate go run ../../cmd/gentypes --package api types.yml

type Querier interface {
	QueryUrl(url string) (*QueryResponse, error)
	QueryDirectory(url string) (*QueryResponse, error)
	QueryChain(id []byte) (*QueryResponse, error)
	QueryTx(id []byte) (*QueryResponse, error)
	QueryTxHistory(url string, start, count int64) (*QueryMultiResponse, error)
}

// ABCIQueryClient is a subset of from TM/rpc/client.ABCIClient for sending
// queries.
type ABCIQueryClient interface {
	ABCIQuery(ctx context.Context, path string, data bytes.HexBytes) (*core.ResultABCIQuery, error)
}
