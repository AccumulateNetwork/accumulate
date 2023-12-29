// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package tendermint

import (
	"context"

	"github.com/cometbft/cometbft/libs/bytes"
	"github.com/cometbft/cometbft/libs/log"
	"github.com/cometbft/cometbft/rpc/client"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/cometbft/cometbft/types"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
)

type Client interface {
	client.ABCIClient
	client.NetworkClient
	client.MempoolClient
	client.StatusClient
}

type DeferredClient ioc.PromisedOf[client.Client]

var _ ioc.Promised = (*DeferredClient)(nil)
var _ client.Client = (*DeferredClient)(nil)
var _ DispatcherClient = (*DeferredClient)(nil)

func NewDeferredClient() *DeferredClient {
	return (*DeferredClient)(ioc.NewPromisedOf[client.Client]())
}

func (d *DeferredClient) Resolve(v any) error {
	return (*ioc.PromisedOf[client.Client])(d).Resolve(v)
}

func (d *DeferredClient) get() client.Client {
	return (*ioc.PromisedOf[client.Client])(d).Get()
}

/* ***** Methods ***** */

func (d *DeferredClient) ABCIInfo(a0 context.Context) (*coretypes.ResultABCIInfo, error) {
	return d.get().ABCIInfo(a0)
}

func (d *DeferredClient) ABCIQuery(a0 context.Context, a1 string, a2 bytes.HexBytes) (*coretypes.ResultABCIQuery, error) {
	return d.get().ABCIQuery(a0, a1, a2)
}

func (d *DeferredClient) ABCIQueryWithOptions(a0 context.Context, a1 string, a2 bytes.HexBytes, a3 client.ABCIQueryOptions) (*coretypes.ResultABCIQuery, error) {
	return d.get().ABCIQueryWithOptions(a0, a1, a2, a3)
}

func (d *DeferredClient) Block(a0 context.Context, a1 *int64) (*coretypes.ResultBlock, error) {
	return d.get().Block(a0, a1)
}

func (d *DeferredClient) BlockByHash(a0 context.Context, a1 []uint8) (*coretypes.ResultBlock, error) {
	return d.get().BlockByHash(a0, a1)
}

func (d *DeferredClient) BlockResults(a0 context.Context, a1 *int64) (*coretypes.ResultBlockResults, error) {
	return d.get().BlockResults(a0, a1)
}

func (d *DeferredClient) BlockSearch(a0 context.Context, a1 string, a2 *int, a3 *int, a4 string) (*coretypes.ResultBlockSearch, error) {
	return d.get().BlockSearch(a0, a1, a2, a3, a4)
}

func (d *DeferredClient) BlockchainInfo(a0 context.Context, a1 int64, a2 int64) (*coretypes.ResultBlockchainInfo, error) {
	return d.get().BlockchainInfo(a0, a1, a2)
}

func (d *DeferredClient) BroadcastEvidence(a0 context.Context, a1 types.Evidence) (*coretypes.ResultBroadcastEvidence, error) {
	return d.get().BroadcastEvidence(a0, a1)
}

func (d *DeferredClient) BroadcastTxAsync(a0 context.Context, a1 types.Tx) (*coretypes.ResultBroadcastTx, error) {
	return d.get().BroadcastTxAsync(a0, a1)
}

func (d *DeferredClient) BroadcastTxCommit(a0 context.Context, a1 types.Tx) (*coretypes.ResultBroadcastTxCommit, error) {
	return d.get().BroadcastTxCommit(a0, a1)
}

func (d *DeferredClient) BroadcastTxSync(a0 context.Context, a1 types.Tx) (*coretypes.ResultBroadcastTx, error) {
	return d.get().BroadcastTxSync(a0, a1)
}

func (d *DeferredClient) CheckTx(a0 context.Context, a1 types.Tx) (*coretypes.ResultCheckTx, error) {
	return d.get().CheckTx(a0, a1)
}

func (d *DeferredClient) Commit(a0 context.Context, a1 *int64) (*coretypes.ResultCommit, error) {
	return d.get().Commit(a0, a1)
}

func (d *DeferredClient) ConsensusParams(a0 context.Context, a1 *int64) (*coretypes.ResultConsensusParams, error) {
	return d.get().ConsensusParams(a0, a1)
}

func (d *DeferredClient) ConsensusState(a0 context.Context) (*coretypes.ResultConsensusState, error) {
	return d.get().ConsensusState(a0)
}

func (d *DeferredClient) DumpConsensusState(a0 context.Context) (*coretypes.ResultDumpConsensusState, error) {
	return d.get().DumpConsensusState(a0)
}

func (d *DeferredClient) Genesis(a0 context.Context) (*coretypes.ResultGenesis, error) {
	return d.get().Genesis(a0)
}

func (d *DeferredClient) GenesisChunked(a0 context.Context, a1 uint) (*coretypes.ResultGenesisChunk, error) {
	return d.get().GenesisChunked(a0, a1)
}

func (d *DeferredClient) Header(a0 context.Context, a1 *int64) (*coretypes.ResultHeader, error) {
	return d.get().Header(a0, a1)
}

func (d *DeferredClient) HeaderByHash(a0 context.Context, a1 bytes.HexBytes) (*coretypes.ResultHeader, error) {
	return d.get().HeaderByHash(a0, a1)
}

func (d *DeferredClient) Health(a0 context.Context) (*coretypes.ResultHealth, error) {
	return d.get().Health(a0)
}

func (d *DeferredClient) IsRunning() bool {
	return d.get().IsRunning()
}

func (d *DeferredClient) NetInfo(a0 context.Context) (*coretypes.ResultNetInfo, error) {
	return d.get().NetInfo(a0)
}

func (d *DeferredClient) NumUnconfirmedTxs(a0 context.Context) (*coretypes.ResultUnconfirmedTxs, error) {
	return d.get().NumUnconfirmedTxs(a0)
}

func (d *DeferredClient) OnReset() error {
	return d.get().OnReset()
}

func (d *DeferredClient) OnStart() error {
	return d.get().OnStart()
}

func (d *DeferredClient) OnStop() {
	d.get().OnStop()
}

func (d *DeferredClient) Quit() <-chan struct{} {
	return d.get().Quit()
}

func (d *DeferredClient) Reset() error {
	return d.get().Reset()
}

func (d *DeferredClient) SetLogger(a0 log.Logger) {
	d.get().SetLogger(a0)
}

func (d *DeferredClient) Start() error {
	return d.get().Start()
}

func (d *DeferredClient) Status(a0 context.Context) (*coretypes.ResultStatus, error) {
	return d.get().Status(a0)
}

func (d *DeferredClient) Stop() error {
	return d.get().Stop()
}

func (d *DeferredClient) String() string {
	return d.get().String()
}

func (d *DeferredClient) Subscribe(a0 context.Context, a1 string, a2 string, a3 ...int) (<-chan coretypes.ResultEvent, error) {
	return d.get().Subscribe(a0, a1, a2, a3...)
}

func (d *DeferredClient) Tx(a0 context.Context, a1 []uint8, a2 bool) (*coretypes.ResultTx, error) {
	return d.get().Tx(a0, a1, a2)
}

func (d *DeferredClient) TxSearch(a0 context.Context, a1 string, a2 bool, a3 *int, a4 *int, a5 string) (*coretypes.ResultTxSearch, error) {
	return d.get().TxSearch(a0, a1, a2, a3, a4, a5)
}

func (d *DeferredClient) UnconfirmedTxs(a0 context.Context, a1 *int) (*coretypes.ResultUnconfirmedTxs, error) {
	return d.get().UnconfirmedTxs(a0, a1)
}

func (d *DeferredClient) Unsubscribe(a0 context.Context, a1 string, a2 string) error {
	return d.get().Unsubscribe(a0, a1, a2)
}

func (d *DeferredClient) UnsubscribeAll(a0 context.Context, a1 string) error {
	return d.get().UnsubscribeAll(a0, a1)
}

func (d *DeferredClient) Validators(a0 context.Context, a1 *int64, a2 *int, a3 *int) (*coretypes.ResultValidators, error) {
	return d.get().Validators(a0, a1, a2, a3)
}
