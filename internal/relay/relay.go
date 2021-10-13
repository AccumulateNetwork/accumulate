package relay

import (
	"context"
	"crypto/sha256"
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
	numNetworks uint64
	mutex       sync.Mutex
}

// New Create the new bouncer and initialize it with a client connection to each of the nodes
func New(clients ...client.ABCIClient) *Relay {
	for i, c := range clients {
		if c, ok := c.(*http.HTTP); ok {
			clients[i] = rpcClient{c}
		}
	}

	r := &Relay{}
	r.numNetworks = uint64(len(clients))
	r.client = clients
	r.resetBatches()

	return r
}

// resetBatches gets called after each call to BatchSend().  It will thread off the batch of transactions it has, then
// create a new batch by calling this function
func (r *Relay) resetBatches() {
	r.txQueue = make([]txBatch, r.numNetworks)
}

func (r *Relay) GetNetworkId(routing uint64) int {
	return int(routing % r.numNetworks)
}

//BatchTx
//appends the transaction to the transaction queue and returns the tx hash used to track
//in tendermint, the index within the queue, and the network the transaction will be submitted on
func (r *Relay) BatchTx(routing uint64, tx tmtypes.Tx) (ti TransactionInfo) {
	i := r.GetNetworkId(routing)
	r.mutex.Lock()
	r.txQueue[i].tx = append(r.txQueue[i].tx, tx)
	r.txQueue[i].networkId = i
	r.mutex.Unlock()
	index := len(r.txQueue[i].tx) - 1
	txReference := sha256.Sum256(tx)
	ti.ReferenceId = txReference[:]
	ti.NetworkId = i
	ti.QueueIndex = index
	return ti
}

// BatchSend
// This will dispatch all the transactions that have been put into batches. The calling function does not have to
// wait for batch to be sent.  This is a fire and forget operation
func (r *Relay) BatchSend() chan BatchedStatus {
	var sendTxsAsBatch []txBatch
	var sendTxsAsSingle []txBatch
	//sort out the requests
	r.mutex.Lock()

	for i, c := range r.client {
		l := len(r.txQueue[i].tx)
		if _, ok := c.(Batchable); l > 1 && ok {
			sendTxsAsBatch = append(sendTxsAsBatch, r.txQueue[i])
		} else if l > 0 {
			sendTxsAsSingle = append(sendTxsAsSingle, r.txQueue[i])
		}
		//reset the queue here.
		r.txQueue[i] = txBatch{}
	}
	stat := make(chan BatchedStatus)
	go dispatch(r.client, sendTxsAsBatch, sendTxsAsSingle, stat)
	r.resetBatches()

	r.mutex.Unlock()
	return stat
}

// dispatch
// This function is executed as a go routine to send out all the batches
func dispatch(client []client.ABCIClient, sendAsBatch []txBatch, sendAsSingle []txBatch, stat chan BatchedStatus) {

	batchedStatus := make(chan BatchedStatus)
	singlesStatus := make(chan BatchedStatus)

	//now send out the batches
	go dispatchBatch(client, sendAsBatch, batchedStatus)
	go dispatchSingles(client, sendAsSingle, singlesStatus)

	bStatus := <-batchedStatus
	sStatus := <-singlesStatus

	bs := BatchedStatus{}
	bs.Status = append(bStatus.Status, sStatus.Status...)
	stat <- bs
}

func dispatchBatch(client []client.ABCIClient, sendBatches []txBatch, status chan BatchedStatus) {

	bs := BatchedStatus{}
	bs.Status = make([]DispatchStatus, len(sendBatches))

	var batches []Batch
	for i, txb := range sendBatches {
		batches = append(batches, client[txb.networkId].(Batchable).NewBatch())
		for _, tx := range sendBatches[i].tx {
			batches[i].BroadcastTxSync(context.Background(), tx)
		}
	}

	//now send out the batches
	for i := range batches {
		bs.Status[i].Returns, bs.Status[i].Err = batches[i].Send(context.Background())
	}

	status <- bs
}

func dispatchSingles(client []client.ABCIClient, sendSingles []txBatch, status chan BatchedStatus) {

	bs := BatchedStatus{}
	bs.Status = make([]DispatchStatus, len(sendSingles))

	for i, single := range sendSingles {
		bs.Status[i].Returns = make([]interface{}, len(single.tx))
		for j := range single.tx {
			bs.Status[i].Returns[j], bs.Status[i].Err = client[single.networkId].BroadcastTxSync(context.Background(), single.tx[j])
		}
	}

	status <- bs
}

// SendTx
// This function will send an individual transaction and return the result.  However, this is a broadcast asynchronous
// call to tendermint, so it won't provide tendermint results from DeliverTx
func (r *Relay) SendTx(tx tmtypes.Tx) (*ctypes.ResultBroadcastTx, error) {
	gtx, err := decodeTX(tx)
	if err != nil {
		return nil, err
	}
	return r.client[r.GetNetworkId(gtx.Routing)].BroadcastTxAsync(context.Background(), tx)
}

// Query
// This function will return the state object from the accumulate network for a given URL.
func (r *Relay) Query(data bytes.HexBytes) (ret *ctypes.ResultABCIQuery, err error) {
	_, addr, err := decodeQuery(data)
	if err != nil {
		return nil, err
	}
	return r.client[r.GetNetworkId(addr)].ABCIQuery(context.Background(), "/abci_query", data)
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
