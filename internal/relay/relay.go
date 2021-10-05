package relay

import (
	"context"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/types/api"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/tendermint/tendermint/libs/bytes"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	tmtypes "github.com/tendermint/tendermint/types"
)

// Relay is the structure used to relay messages to the correct BVC.  Transactions can either be batched and dispatched
// or they can be sent directly.  They only know about GenTransactions and are routed according to the number of networks
// in the system
type Relay struct {
	rpcClient   []*rpchttp.HTTP
	batches     []*rpchttp.BatchHTTP
	numNetworks int
}

// New Create the new bouncer and initialize it with a client connection to each of the nodes
func New(clients ...*rpchttp.HTTP) *Relay {
	bouncer := &Relay{}
	bouncer.initialize(clients)
	return bouncer
}

// initialize will set the initial clients and create a new batch for each client
func (r *Relay) initialize(clients []*rpchttp.HTTP) {
	r.rpcClient = clients
	r.numNetworks = len(clients)
	r.resetBatches()
}

// resetBatches gets called after each call to BatchSend().  It will thread off the batch of transactions it has, then
// create a new batch by calling this function
func (r *Relay) resetBatches() {
	r.batches = make([]*rpchttp.BatchHTTP, r.numNetworks)
	for i := range r.batches {
		r.batches[i] = r.rpcClient[i].NewBatch()
	}
}

func (r *Relay) BatchTx(tx tmtypes.Tx) (*ctypes.ResultBroadcastTx, error) {
	gtx, err := decodeTX(tx)
	if err != nil {
		return nil, err
	}
	return r.batches[int(gtx.Routing)%r.numNetworks].BroadcastTxAsync(context.Background(), tx)
}

// BatchSend
// This will dispatch all the transactions that have been put into batches. The calling function does not have to
// wait for batch to be sent.  This is a fire and forget operation
func (r *Relay) BatchSend() {
	sendBatches := make([]*rpchttp.BatchHTTP, r.numNetworks)
	for i, batch := range r.batches {
		sendBatches[i] = batch
	}
	go dispatch(sendBatches)
	r.resetBatches()
}

// dispatch
// This function is executed as a go routine to send out all the batches
func dispatch(batches []*rpchttp.BatchHTTP) {
	for i := range batches {
		if batches[i].Count() > 0 {
			_, err := batches[i].Send(context.Background())
			if err != nil {
				//	fmt.Println("error sending batch, %v", err)
			}
		}
	}
}

// SendTx
// This function will send an individual transaction and return the result.  However, this is a broadcast asynchronous
// call to tendermint, so it won't provide tendermint results from CheckTx or DeliverTx
func (r *Relay) SendTx(tx tmtypes.Tx) (*ctypes.ResultBroadcastTx, error) {
	gtx, err := decodeTX(tx)
	if err != nil {
		return nil, err
	}
	return r.rpcClient[int(gtx.Routing)%r.numNetworks].BroadcastTxSync(context.Background(), tx)
}

// Query
// This function will return the state object from the accumulate network for a given URL.
func (r *Relay) Query(data bytes.HexBytes) (ret *ctypes.ResultABCIQuery, err error) {
	_, addr, err := decodeQuery(data)
	if err != nil {
		return nil, err
	}
	return r.rpcClient[addr%uint64(r.numNetworks)].ABCIQuery(context.Background(), "/abci_query", data)
}

func decodeTX(tx tmtypes.Tx) (*transactions.GenTransaction, error) {
	gtx := new(transactions.GenTransaction)
	next, err := gtx.UnMarshal(tx)
	if err != nil {
		return nil, err
	}
	if len(next) > 0 {
		return nil, fmt.Errorf("got %d extra byte(s)", len(next))
	}
	return gtx, nil
}

func decodeQuery(data bytes.HexBytes) (*api.Query, uint64, error) {
	query := new(api.Query)
	err := query.UnmarshalBinary(data)
	if err != nil {
		return nil, 0, err
	}
	addr := types.GetAddressFromIdentity(&query.Url)
	return query, addr, nil
}
