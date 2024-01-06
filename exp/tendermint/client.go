// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package tendermint

import (
	"context"

	"github.com/cometbft/cometbft/libs/bytes"
	"github.com/cometbft/cometbft/rpc/client"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/cometbft/cometbft/types"
)

type Client interface {
	client.ABCIClient
	client.NetworkClient
	client.MempoolClient
	client.StatusClient
}

type DeferredClient struct {
	done   chan struct{}
	client Client
}

func NewDeferredClient() *DeferredClient {
	d := new(DeferredClient)
	d.done = make(chan struct{})
	return d
}

/* ***** ABCI ***** */

// Set sets the client. Set will panic if it is called more than once.
func (d *DeferredClient) Set(client Client) {
	d.client = client
	close(d.done)
}

func (d *DeferredClient) ABCIInfo(ctx context.Context) (*ctypes.ResultABCIInfo, error) {
	<-d.done
	return d.client.ABCIInfo(ctx)
}

func (d *DeferredClient) ABCIQuery(ctx context.Context, path string, data bytes.HexBytes) (*ctypes.ResultABCIQuery, error) {
	<-d.done
	return d.client.ABCIQuery(ctx, path, data)
}

func (d *DeferredClient) ABCIQueryWithOptions(ctx context.Context, path string, data bytes.HexBytes, opts client.ABCIQueryOptions) (*ctypes.ResultABCIQuery, error) {
	<-d.done
	return d.client.ABCIQueryWithOptions(ctx, path, data, opts)
}

func (d *DeferredClient) BroadcastTxCommit(ctx context.Context, tx types.Tx) (*ctypes.ResultBroadcastTxCommit, error) {
	<-d.done
	return d.client.BroadcastTxCommit(ctx, tx)
}

func (d *DeferredClient) BroadcastTxAsync(ctx context.Context, tx types.Tx) (*ctypes.ResultBroadcastTx, error) {
	<-d.done
	return d.client.BroadcastTxAsync(ctx, tx)
}

func (d *DeferredClient) BroadcastTxSync(ctx context.Context, tx types.Tx) (*ctypes.ResultBroadcastTx, error) {
	<-d.done
	return d.client.BroadcastTxSync(ctx, tx)
}

/* ***** Network ***** */

func (d *DeferredClient) NetInfo(ctx context.Context) (*ctypes.ResultNetInfo, error) {
	<-d.done
	return d.client.NetInfo(ctx)
}

func (d *DeferredClient) DumpConsensusState(ctx context.Context) (*ctypes.ResultDumpConsensusState, error) {
	<-d.done
	return d.client.DumpConsensusState(ctx)
}

func (d *DeferredClient) ConsensusState(ctx context.Context) (*ctypes.ResultConsensusState, error) {
	<-d.done
	return d.client.ConsensusState(ctx)
}

func (d *DeferredClient) ConsensusParams(ctx context.Context, height *int64) (*ctypes.ResultConsensusParams, error) {
	<-d.done
	return d.client.ConsensusParams(ctx, height)
}

func (d *DeferredClient) Health(ctx context.Context) (*ctypes.ResultHealth, error) {
	<-d.done
	return d.client.Health(ctx)
}

/* ***** Mempool ***** */

func (d *DeferredClient) UnconfirmedTxs(ctx context.Context, limit *int) (*ctypes.ResultUnconfirmedTxs, error) {
	<-d.done
	return d.client.UnconfirmedTxs(ctx, limit)
}

func (d *DeferredClient) NumUnconfirmedTxs(ctx context.Context) (*ctypes.ResultUnconfirmedTxs, error) {
	<-d.done
	return d.client.NumUnconfirmedTxs(ctx)
}

func (d *DeferredClient) CheckTx(ctx context.Context, tx types.Tx) (*ctypes.ResultCheckTx, error) {
	<-d.done
	return d.client.CheckTx(ctx, tx)
}

/* ***** Status ***** */

func (d *DeferredClient) Status(ctx context.Context) (*ctypes.ResultStatus, error) {
	<-d.done
	return d.client.Status(ctx)
}
