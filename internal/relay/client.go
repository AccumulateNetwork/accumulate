package relay

import (
	"context"

	"github.com/tendermint/tendermint/rpc/client"
	"github.com/tendermint/tendermint/rpc/client/http"
)

type Batchable interface {
	client.ABCIClient
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
