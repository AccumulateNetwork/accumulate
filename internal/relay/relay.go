package relay

import (
	"context"
	"fmt"
	"sync"

	"github.com/AccumulateNetwork/accumulated/internal/url"

	"github.com/AccumulateNetwork/accumulated/types/api"

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

type txBatch struct {
	networkId int
	tx        [][]byte
}

type Relay struct {
	client      []client.ABCIClient
	batches     []Batch
	txQueue     []txBatch
	numNetworks int
}

type DispatchStatus struct {
	NetworkId int
	Returns   []interface{}
	Err       error
}

type BatchedStatus struct {
	Status []DispatchStatus
}

//func (d *DispatchStatus) MakeRequests {
//	reqs := make([]types.RPCRequest, 0, len(requests))
//	results := make([]interface{}, 0, len(requests))
//	for _, req := range requests {
//		reqs = append(reqs, req.request)
//		results = append(results, req.result)
//	}
//}

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
	r.txQueue[i].tx = append(r.txQueue[i].tx, tx)
	return nil, nil
	//if r.batches[i] == nil {
	//	return r.client[i].BroadcastTxSync(context.Background(), tx)
	//}
	//return r.batches[i].BroadcastTxSync(context.Background(), tx)
}

// BatchSend
// This will dispatch all the transactions that have been put into batches. The calling function does not have to
// wait for batch to be sent.  This is a fire and forget operation
func (r *Relay) BatchSend() chan BatchedStatus {
	var sendTxsAsBatch []txBatch
	var sendTxsAsSingle []txBatch
	for i, c := range r.client {
		l := len(r.txQueue[i].tx)
		if _, ok := c.(Batchable); l > 1 && ok {
			sendTxsAsBatch = append(sendTxsAsBatch, r.txQueue[i])
		} else {
			sendTxsAsSingle = append(sendTxsAsSingle, r.txQueue[i])
		}
		//reset the queue here.
		r.txQueue[i] = txBatch{}
	}
	stat := make(chan BatchedStatus)
	go dispatch(r.client, sendTxsAsBatch, sendTxsAsSingle, stat)
	r.resetBatches()
	return stat
}

// dispatch
// This function is executed as a go routine to send out all the batches
func dispatch(client []client.ABCIClient, sendAsBatch []txBatch, sendAsSingle []txBatch, stat chan BatchedStatus) {
	var batches []Batch

	for i, txb := range sendAsBatch {
		batches = append(batches, client[txb.networkId].(Batchable).NewBatch())
		for _, tx := range sendAsBatch[i].tx {
			batches[i].BroadcastTxSync(context.Background(), tx)
		}
	}

	bs := BatchedStatus{}

	bstatus := make([]DispatchStatus, len(sendAsBatch))
	//now send out the batches
	batchGroup := sync.WaitGroup{}
	batchGroup.Add(1)
	go disapatchBatch(client, sendAsBatch, &bstatus, &batchGroup)

	singlesGroup := sync.WaitGroup{}
	singlesGroup.Add(1)
	sstatus := make([]DispatchStatus, len(sendAsSingle))
	go dispachSingle(client, sendAsSingle, &sstatus, &singlesGroup)

	batchGroup.Wait()
	bs.Status = append(bstatus, sstatus)
	stat <- bs
}

func disapatchBatch(client []client.ABCIClient, sendBatches []txBatch, status *[]DispatchStatus, group *sync.WaitGroup) {
	var batches []Batch
	for i, txb := range sendBatches {
		batches = append(batches, client[txb.networkId].(Batchable).NewBatch())
		for _, tx := range sendBatches[i].tx {
			batches[i].BroadcastTxSync(context.Background(), tx)
		}
	}

	//now send out the batches
	for i := range batches {
		status[i].Returns, status[i].Err = batches[i].Send(context.Background())
	}
	group.Done()
}

func dispachSingle(client []client.ABCIClient, sendSingles []txBatch, status *[]DispatchStatus, group *sync.WaitGroup) {

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

	u, err := url.Parse(query.Url)
	if err != nil {
		return nil, 0, err
	}

	return query, u.Routing(), nil
}
