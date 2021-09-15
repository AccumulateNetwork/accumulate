package router

import (
	"context"

	ctypes "github.com/tendermint/tendermint/rpc/core/types"

	"github.com/AccumulateNetwork/accumulated/types/proto"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
)

type Bouncer struct {
	rpcClient   []*rpchttp.HTTP
	batches     []*rpchttp.BatchHTTP
	numNetworks int
}

func NewBouncer(clients []*rpchttp.HTTP) *Bouncer {
	bouncer := &Bouncer{}
	bouncer.initialize(clients)
	return bouncer
}

func (b *Bouncer) initialize(clients []*rpchttp.HTTP) error {
	b.rpcClient = clients
	b.numNetworks = len(clients)
	b.resetBatches()

	return nil
}

func (b *Bouncer) resetBatches() {
	b.batches = make([]*rpchttp.BatchHTTP, b.numNetworks)
	for i := range b.batches {
		b.batches[i] = b.rpcClient[i].NewBatch()
	}
}

func (b *Bouncer) BatchTx(tx *proto.GenTransaction) (*ctypes.ResultBroadcastTx, error) {
	data, err := tx.Marshal()
	if err != nil {
		return nil, err
	}
	return b.batches[int(tx.Routing)%b.numNetworks].BroadcastTxAsync(context.Background(), data)
}

func (b *Bouncer) BatchSend() {
	go dispatch(b.batches)
	b.resetBatches()
}

func dispatch(batches []*rpchttp.BatchHTTP) {
	for i := range batches {
		batches[i].Send(context.Background())
	}
}

func (b *Bouncer) SendTx(tx *proto.GenTransaction) (*ctypes.ResultBroadcastTx, error) {
	data, err := tx.Marshal()
	if err != nil {
		return nil, err
	}
	return b.rpcClient[int(tx.Routing)%b.numNetworks].BroadcastTxAsync(context.Background(), data)
}
