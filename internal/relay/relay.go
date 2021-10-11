package relay

import (
	"context"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/types/api"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/tendermint/tendermint/libs/bytes"
	"github.com/tendermint/tendermint/rpc/client"
	"github.com/tendermint/tendermint/rpc/client/http"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	tmtypes "github.com/tendermint/tendermint/types"
)

// Relay is the structure used to relay messages to the correct BVC.  Transactions can either be batched and dispatched
// or they can be sent directly.  They only know about GenTransactions and are routed according to the number of networks
// in the system
type Relay struct {
	client      []client.ABCIClient
	batches     []Batch
	numNetworks int
}

// New Create the new bouncer and initialize it with a client connection to each of the nodes
func New(clients ...client.ABCIClient) *Relay {
	for i, c := range clients {
		if c, ok := c.(*http.HTTP); ok {
			clients[i] = rpcClient{c}
		}
	}

	r := &Relay{}
	r.numNetworks = len(clients)
	r.client = clients
	r.resetBatches()

	return r
}

// resetBatches gets called after each call to BatchSend().  It will thread off the batch of transactions it has, then
// create a new batch by calling this function
func (r *Relay) resetBatches() {
	r.batches = make([]Batch, r.numNetworks)
	for i, c := range r.client {
		if c, ok := c.(Batchable); ok {
			r.batches[i] = c.NewBatch()
		}
	}
}

func (r *Relay) BatchTx(tx tmtypes.Tx) (*ctypes.ResultBroadcastTx, error) {
	gtx, err := decodeTX(tx)
	if err != nil {
		return nil, err
	}
	i := int(gtx.Routing) % r.numNetworks
	if r.batches[i] == nil {
		return r.client[i].BroadcastTxAsync(context.Background(), tx)
	}
	return r.batches[i].BroadcastTxAsync(context.Background(), tx)
}

// BatchSend
// This will dispatch all the transactions that have been put into batches. The calling function does not have to
// wait for batch to be sent.  This is a fire and forget operation
func (r *Relay) BatchSend() chan BatchedStatus {
	sendBatches := make([]Batch, r.numNetworks)
	for i, batch := range r.batches {
		sendBatches[i] = batch
	}
	stat := make(chan BatchedStatus)
	go dispatch(sendBatches, stat)
	r.resetBatches()
	return stat
}

type DispatchStatus struct {
	Returns []interface{}
	Err     error
}

type BatchedStatus struct {
	Status []DispatchStatus
}

// dispatch
// This function is executed as a go routine to send out all the batches
func dispatch(batches []Batch, stat chan BatchedStatus) {
	bs := BatchedStatus{}
	bs.Status = make([]DispatchStatus, len(batches))
	for i := range batches {
		if batches[i] == nil {
			continue
		}

		if batches[i].Count() > 0 {
			bs.Status[i].Returns, bs.Status[i].Err = batches[i].Send(context.Background())
		}
	}
	stat <- bs
}

// SendTx
// This function will send an individual transaction and return the result.  However, this is a broadcast asynchronous
// call to tendermint, so it won't provide tendermint results from DeliverTx
func (r *Relay) SendTx(tx tmtypes.Tx) (*ctypes.ResultBroadcastTx, error) {
	gtx, err := decodeTX(tx)
	if err != nil {
		return nil, err
	}
	return r.client[int(gtx.Routing)%r.numNetworks].BroadcastTxAsync(context.Background(), tx)
}

// Query
// This function will return the state object from the accumulate network for a given URL.
func (r *Relay) Query(data bytes.HexBytes) (ret *ctypes.ResultABCIQuery, err error) {
	_, addr, err := decodeQuery(data)
	if err != nil {
		return nil, err
	}
	return r.client[addr%uint64(r.numNetworks)].ABCIQuery(context.Background(), "/abci_query", data)
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
