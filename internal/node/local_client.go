package node

import (
	"context"
	"errors"

	"github.com/tendermint/tendermint/libs/bytes"
	"github.com/tendermint/tendermint/rpc/client"
	"github.com/tendermint/tendermint/rpc/client/local"
	core "github.com/tendermint/tendermint/rpc/coretypes"
	tm "github.com/tendermint/tendermint/types"
)

var ErrNotInitialized = errors.New("not initialized")

// LocalClient is a proxy for local.Local that is used when a local client is
// needed before the node has been started.
type LocalClient struct {
	client *local.Local
	queue  []tm.Tx
}

// NewLocalClient creates a new LocalClient.
func NewLocalClient() *LocalClient {
	c := new(LocalClient)
	return c
}

// Set sets the local.Local client, transmitting any queued transactions.
func (c *LocalClient) Set(client *local.Local) {
	c.client = client
	queue := c.queue
	c.queue = nil
	for _, tx := range queue {
		_, _ = client.BroadcastTxAsync(context.Background(), tx)
	}
}

// ABCIQueryWithOptions implements client.ABCIClient.ABCIQueryWithOptions. Returns ErrNotInitialized
// if Set has not been called.
func (c *LocalClient) ABCIQueryWithOptions(ctx context.Context, path string, data bytes.HexBytes, opts client.ABCIQueryOptions) (*core.ResultABCIQuery, error) {
	if c.client == nil {
		return nil, ErrNotInitialized
	}
	return c.client.ABCIQueryWithOptions(ctx, path, data, opts)
}

// CheckTx implements client.MempoolClient.CheckTx. Returns ErrNotInitialized if
// Set has not been called.
func (c *LocalClient) CheckTx(ctx context.Context, tx tm.Tx) (*core.ResultCheckTx, error) {
	if c.client == nil {
		return nil, ErrNotInitialized
	}
	return c.client.CheckTx(ctx, tx)
}

// BroadcastTxAsync implements client.ABCIClient.BroadcastTxAsync. If Set has
// not been called, the transaction is queued and will be sent when Set is
// called.
func (c *LocalClient) BroadcastTxAsync(ctx context.Context, tx tm.Tx) (*core.ResultBroadcastTx, error) {
	if c.client == nil {
		c.queue = append(c.queue, tx)
		return &core.ResultBroadcastTx{Hash: tx.Hash()}, nil
	}
	return c.client.BroadcastTxAsync(ctx, tx)
}

// BroadcastTxSync implements client.ABCIClient.BroadcastTxSync. Returns
// ErrNotInitialized if Set has not been called.
func (c *LocalClient) BroadcastTxSync(ctx context.Context, tx tm.Tx) (*core.ResultBroadcastTx, error) {
	if c.client == nil {
		return nil, ErrNotInitialized
	}
	return c.client.BroadcastTxSync(ctx, tx)
}

// BroadcastTxSync implements client.ABCIClient.BroadcastTxSync. Returns
// ErrNotInitialized if Set has not been called.
func (c *LocalClient) Tx(ctx context.Context, hash []byte, prove bool) (*core.ResultTx, error) {
	if c.client == nil {
		return nil, ErrNotInitialized
	}
	return c.client.Tx(ctx, hash, prove)
}

// Subscribe implements client.EventsClient.Subscribe. Returns
// ErrNotInitialized if Set has not been called.
func (c *LocalClient) Subscribe(ctx context.Context, subscriber, query string, outCapacity ...int) (out <-chan core.ResultEvent, err error) {
	if c.client == nil {
		return nil, ErrNotInitialized
	}
	return c.client.Subscribe(ctx, subscriber, query, outCapacity...)
}
