package networks

import (
	"context"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/proto"
	proto1 "github.com/golang/protobuf/proto"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
)

// Bouncer is the structure used to relay messages to the correct BVC.  Transactions can either be batched and dispatched
// or they can be sent directly.  They only know about GenTransactions and are routed according to the number of networks
// in the system
type Bouncer struct {
	rpcClient   []*rpchttp.HTTP
	batches     []*rpchttp.BatchHTTP
	numNetworks int
}

// NewBouncer Create the new bouncer and initialize it with a client connection to each of the nodes
func NewBouncer(clients []*rpchttp.HTTP) *Bouncer {
	bouncer := &Bouncer{}
	bouncer.initialize(clients)
	return bouncer
}

// initialize will set the initial clients and create a new batch for each client
func (b *Bouncer) initialize(clients []*rpchttp.HTTP) error {
	b.rpcClient = clients
	b.numNetworks = len(clients)
	b.resetBatches()

	return nil
}

// resetBatches gets called after each call to BatchSend().  It will thread off the batch of transactions it has, then
// create a new batch by calling this function
func (b *Bouncer) resetBatches() {
	b.batches = make([]*rpchttp.BatchHTTP, b.numNetworks)
	for i := range b.batches {
		b.batches[i] = b.rpcClient[i].NewBatch()
	}
}

func (b *Bouncer) BatchTx(tx *transactions.GenTransaction) (*ctypes.ResultBroadcastTx, error) {
	data, err := tx.Marshal()
	if err != nil {
		return nil, err
	}
	return b.batches[int(tx.Routing)%b.numNetworks].BroadcastTxAsync(context.Background(), data)
}

// BatchSend
// This will dispatch all the transactions that have been put into batches. The calling function does not have to
// wait for batch to be sent.  This is a fire and forget operation
func (b *Bouncer) BatchSend() {
	go dispatch(b.batches)
	b.resetBatches()
}

// dispatch
// This function is executed as a go routine to send out all the batches
func dispatch(batches []*rpchttp.BatchHTTP) {
	for i := range batches {
		batches[i].Send(context.Background())
	}
}

// SendTx
// This function will send an individual transaction and return the result.  However, this is a broadcast asynchronous
// call to tendermint, so it won't provide tendermint results from CheckTx or DeliverTx
func (b *Bouncer) SendTx(tx *transactions.GenTransaction) (*ctypes.ResultBroadcastTx, error) {
	data, err := tx.Marshal()
	if err != nil {
		return nil, err
	}
	return b.rpcClient[int(tx.Routing)%b.numNetworks].BroadcastTxAsync(context.Background(), data)
}

// Query
// This function will return the state object from the accumulate network for a given URL.
func (b *Bouncer) Query(url *string, txId []byte) (ret *ctypes.ResultABCIQuery, err error) {
	addr := types.GetAddressFromIdentity(url)

	pq := proto.Query{}
	pq.ChainUrl = *url
	adiChain := types.GetIdentityChainFromIdentity(url)
	chainId := types.GetChainIdFromChainPath(url)
	pq.AdiChain = adiChain.Bytes()
	pq.ChainId = chainId.Bytes()
	pq.Ins = proto.AccInstruction_State_Query

	data, err := proto1.Marshal(&pq)
	if err != nil {
		return nil, err
	}

	return b.rpcClient[addr%uint64(b.numNetworks)].ABCIQuery(context.Background(), "/abci_query", data)
}
