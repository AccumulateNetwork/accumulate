package router

import (
	"context"

	"github.com/AccumulateNetwork/accumulated/types/proto"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
)

type Bouncer struct {
	rpcClient   []*rpchttp.HTTP
	batch       [2]*rpchttp.BatchHTTP
	numNetworks int
}

func NewBouncer(configFile string, workingDir string) *Bouncer {
	bouncer := &Bouncer{}
	return bouncer
}

func (b *Bouncer) Initialize(clients []*rpchttp.HTTP) error {
	b.rpcClient = clients
	b.numNetworks = len(clients)
	return nil
}

func (b *Bouncer) BatchTx

func (b *Bouncer) BatchTx(tx *proto.GenTransaction) {

}

func (b *Bouncer) BatchSend() {

}

func (b *Bouncer) SendTx(tx *proto.GenTransaction) error {
	data, err := tx.Marshal()
	if err != nil {
		return err
	}
	b.rpcClient[int(tx.Routing)%b.numNetworks].BroadcastTxAsync(context.Background(), data)
	return nil
}
