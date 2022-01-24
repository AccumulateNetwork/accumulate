package connections

import (
	"context"
	"github.com/tendermint/tendermint/libs/bytes"
	"github.com/tendermint/tendermint/rpc/client"
	"github.com/tendermint/tendermint/rpc/client/http"
	core "github.com/tendermint/tendermint/rpc/core/types"
	tm "github.com/tendermint/tendermint/types"
)

// ABCIQueryClient is a subset of from TM/rpc/client.ABCIClient for sending
// queries.
type ABCIQueryClient interface {
	ABCIQuery(ctx context.Context, path string, data bytes.HexBytes) (*core.ResultABCIQuery, error)
	ABCIQueryWithOptions(ctx context.Context, path string, data bytes.HexBytes, opts client.ABCIQueryOptions) (*core.ResultABCIQuery, error)
}

// ABCIBroadcastClient is a subset of from TM/rpc/client.ABCIClient for
// broadcasting transactions.
type ABCIBroadcastClient interface {
	CheckTx(ctx context.Context, tx tm.Tx) (*core.ResultCheckTx, error)
	BroadcastTxAsync(context.Context, tm.Tx) (*core.ResultBroadcastTx, error)
	BroadcastTxSync(context.Context, tm.Tx) (*core.ResultBroadcastTx, error)
}

type BatchABCIBroadcastClient interface {
	ABCIBroadcastClient
	NewBatch() *http.BatchHTTP
}
