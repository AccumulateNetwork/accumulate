package relay

import (
	"context"
	"fmt"
	"github.com/AccumulateNetwork/accumulate/networks/connections"

	"github.com/tendermint/tendermint/libs/bytes"
	"github.com/tendermint/tendermint/rpc/client"
	"github.com/tendermint/tendermint/rpc/client/http"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	"github.com/tendermint/tendermint/types"
)

type Client interface {
	// client.ABCIClient
	ABCIQuery(ctx context.Context, path string, data bytes.HexBytes) (*ctypes.ResultABCIQuery, error)
	BroadcastTxAsync(context.Context, types.Tx) (*ctypes.ResultBroadcastTx, error)
	BroadcastTxSync(context.Context, types.Tx) (*ctypes.ResultBroadcastTx, error)

	// client.SignClient
	Tx(ctx context.Context, hash []byte, prove bool) (*ctypes.ResultTx, error)

	// client.EventsClient
	Subscribe(ctx context.Context, subscriber, query string, outCapacity ...int) (out <-chan ctypes.ResultEvent, err error)
}

type Batchable interface {
	Client
	NewBatch() Batch
}

type Batch interface {
	client.ABCIClient
	Send(context.Context) ([]interface{}, error)
	Count() int
	Clear() int
}

type rpcClient struct {
	*http.HTTP
}

func (c rpcClient) NewBatch() Batch {
	return c.HTTP.NewBatch()
}

type TransactionInfo struct {
	ReferenceId []byte //tendermint reference id
	QueueIndex  int    //the transaction's place in line
	Route       connections.Route
}

type DispatchStatus struct {
	Route   connections.Route
	Returns []interface{}
	Err     error
}

type BatchedStatus struct {
	Status []DispatchStatus
}

func (bs *BatchedStatus) ResolveTransactionResponse(ti TransactionInfo) (*ctypes.ResultBroadcastTx, error) {
	for _, s := range bs.Status {
		if ti.Route == s.Route {
			if len(s.Returns) < ti.QueueIndex {
				return nil, fmt.Errorf("invalid queue length for batch dispatch response, unable to find transaction")
			}
			r, ok := s.Returns[ti.QueueIndex].(*ctypes.ResultBroadcastTx)
			if !ok {
				return nil, fmt.Errorf("unable to resolve return interface as ctypes.ResultBroadcastTx")
			}
			return r, s.Err
		}
	}
	return nil, fmt.Errorf("transaction response not found from batch dispatch")
}
