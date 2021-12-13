package connections

import (
	"context"
	"github.com/tendermint/tendermint/libs/bytes"
	core "github.com/tendermint/tendermint/rpc/core/types"
	tm "github.com/tendermint/tendermint/types"
)

// ABCIQueryClient is a subset of from TM/rpc/client.ABCIClient for sending
// queries.
type ABCIQueryClient interface {
	ABCIQuery(ctx context.Context, path string, data bytes.HexBytes) (*core.ResultABCIQuery, error)
}

// ABCIBroadcastClient is a subset of from TM/rpc/client.ABCIClient for
// broadcasting transactions.
type ABCIBroadcastClient interface {
	CheckTx(ctx context.Context, tx tm.Tx) (*core.ResultCheckTx, error)
	BroadcastTxAsync(context.Context, tm.Tx) (*core.ResultBroadcastTx, error)
	BroadcastTxSync(context.Context, tm.Tx) (*core.ResultBroadcastTx, error)
}
