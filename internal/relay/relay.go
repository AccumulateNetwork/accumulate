package relay

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/networks/connections"
	"reflect"
	"strings"
	"sync"

	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/bytes"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	tmtypes "github.com/tendermint/tendermint/types"
)

const debugTxSend = false

var ErrTimedOut = errors.New("timed out")

// Relay is the structure used to relay messages to the correct BVC.  Transactions can either be batched and dispatched
// or they can be sent directly.  They only know about GenTransactions and are routed according to the number of networks
// in the system

type txBatch struct {
	route connections.Route
	tx    [][]byte
}

type Relay struct {
	connRouter  connections.ConnectionRouter
	connMgr     connections.ConnectionManager
	batches     map[connections.Route]*txBatch
	numNetworks uint64
	mutex       *sync.Mutex

	stopResults chan struct{}
	resultMu    *sync.Mutex
	results     map[[32]byte]chan<- abci.TxResult
}

// New Create the new bouncer and initialize it with a client connection to each of the nodes
func New(connRouter connections.ConnectionRouter, connMgr connections.ConnectionManager) *Relay {
	/*
	   //	relayClient, ok := route.(relay.Client)

	   	for i, c := range clients {
	   		if c, ok := c.(*http.HTTP); ok {
	   			clients[i] = rpcClient{c}
	   		}
	   	}
	*/
	r := &Relay{}
	r.connRouter = connRouter
	r.connMgr = connMgr
	r.mutex = new(sync.Mutex)
	r.resultMu = new(sync.Mutex)
	r.results = map[[32]byte]chan<- abci.TxResult{}
	r.resetBatches()

	return r
}

// resetBatches gets called after each call to BatchSend().  It will thread off the batch of transactions it has, then
// create a new batch by calling this function
func (r *Relay) resetBatches() {
	for key := range r.batches {
		delete(r.batches, key)
	}
}

func (r *Relay) Start() error {
	var failed bool

	if r.stopResults != nil {
		return errors.New("already started")
	}

	allNodeContexts := r.connMgr.GetAllNodeContexts()
	r.stopResults = make(chan struct{})
	cases := make([]reflect.SelectCase, 0, len(allNodeContexts)+1)
	cases = append(cases, reflect.SelectCase{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(r.stopResults),
	})

	for _, nodeCtx := range allNodeContexts { // TODO All nodes or just BVN's?
		svc := nodeCtx.GetService()
		if svc != nil {
			err := svc.Start()
			if err != nil {
				failed = true
				return err
			}
		}

		// Subscribing to all transactions is messy, but making one subscription
		// per TX hash does not work well. Tendermint does not clean up
		// subscriptions sufficiently well, so old unused channels lead to
		// blockages.
		ch, err := nodeCtx.GetRawClient().Subscribe(context.Background(), "acc-relay", "tm.event = 'Tx'")
		if err != nil {
			failed = true
			return err
		}
		cases = append(cases, reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(ch),
		})

		defer func() {
			if failed && svc != nil {
				_ = svc.Stop()
			}
		}()
	}

	go func() {
		for {
			_, v, ok := reflect.Select(cases)
			if !ok {
				// Must have closed the stop channel
				return
			}

			re := v.Interface().(ctypes.ResultEvent)
			data, ok := re.Data.(tmtypes.EventDataTx)
			if !ok {
				continue
			}

			hash := sha256.Sum256(data.Tx)
			r.resultMu.Lock()
			ch := r.results[hash]
			delete(r.results, hash)
			r.resultMu.Unlock()

			select {
			case ch <- data.TxResult:
			default:
			}
		}
	}()

	return nil
}

func (r *Relay) Stop() error {
	var errs []string

	for _, nodeCtx := range r.connMgr.GetAllNodeContexts() {
		svc := nodeCtx.GetService()
		if svc == nil {
			continue
		}

		err := svc.Stop()
		if err != nil {
			errs = append(errs, err.Error())
		}
	}

	close(r.stopResults)
	r.stopResults = nil

	if len(errs) == 0 {
		return nil
	}
	return errors.New(strings.Join(errs, ";"))
}

func (r *Relay) SubscribeTx(hash [32]byte, ch chan<- abci.TxResult) error {
	if r.stopResults == nil {
		return errors.New("Cannot subscribe: WebSocket client has not been started")
	}

	r.resultMu.Lock()
	r.results[hash] = ch
	r.resultMu.Unlock()
	return nil
}

//BatchTx
//appends the transaction to the transaction queue and returns the tx hash used to track
//in tendermint, the index within the queue, and the network the transaction will be submitted on
func (r *Relay) BatchTx(accUrl *url.URL, tx tmtypes.Tx) (ti TransactionInfo, err error) {
	route, batch, err := r.getRouteAndBatch(accUrl)
	if err != nil {
		return ti, err
	}

	r.mutex.Lock()

	batch.tx = append(batch.tx, tx)
	batch.route = route
	r.mutex.Unlock()
	index := len(batch.tx) - 1
	txReference := sha256.Sum256(tx)
	ti.ReferenceId = txReference[:]
	ti.QueueIndex = index
	ti.Route = route
	return ti, nil
}

// BatchSend
// This will dispatch all the transactions that have been put into batches. The calling function does not have to
// wait for batch to be sent.  This is a fire and forget operation
func (r *Relay) BatchSend() <-chan BatchedStatus {
	var sendTxsAsBatch []txBatch
	var sendTxsAsSingle []txBatch
	//sort out the requests
	r.mutex.Lock()

	for _, txb := range r.batches {
		l := len(txb.tx)
		bbc := txb.route.GetBatchBroadcastClient()
		if bbc != nil {
			sendTxsAsBatch = append(sendTxsAsBatch, *txb)
		} else if l > 0 {
			sendTxsAsSingle = append(sendTxsAsSingle, *txb)
		}
		//reset the queue here.
		delete(r.batches, txb.route)
	}
	stat := make(chan BatchedStatus, 1)
	go dispatch(sendTxsAsBatch, sendTxsAsSingle, stat)
	r.resetBatches()

	r.mutex.Unlock()
	return stat
}

func (r *Relay) getRouteAndBatch(u *url.URL) (connections.Route, *txBatch, error) {
	route, err := r.connRouter.SelectRoute(u, false)
	if err != nil {
		return nil, nil, err
	}

	if route.GetNetworkGroup() == connections.Local {
		return route, nil, nil
	}

	batch := r.batches[route]
	if batch == nil {
		batch = new(txBatch)
		r.batches[route] = batch
	}
	return route, batch, nil
}

// dispatch
// This function is executed as a go routine to send out all the batches
func dispatch(sendAsBatch []txBatch, sendAsSingle []txBatch, stat chan BatchedStatus) {
	defer close(stat)

	batchedStatus := make(chan BatchedStatus)
	singlesStatus := make(chan BatchedStatus)

	//now send out the batches
	go dispatchBatch(sendAsBatch, batchedStatus)
	go dispatchSingles(sendAsSingle, singlesStatus)

	bStatus := <-batchedStatus
	sStatus := <-singlesStatus

	bs := BatchedStatus{}
	bs.Status = append(bStatus.Status, sStatus.Status...)
	stat <- bs
}

func dispatchBatch(sendBatches []txBatch, status chan BatchedStatus) {
	defer close(status)

	bs := BatchedStatus{}
	bs.Status = make([]DispatchStatus, len(sendBatches))

	var batches []Batch
	for i, txb := range sendBatches {
		batches = append(batches, txb.route.GetBatchBroadcastClient().NewBatch())
		for _, tx := range sendBatches[i].tx {
			if debugTxSend {
				fmt.Printf("Send TX %X\n", sha256.Sum256(tx))
			}
			_, _ = batches[i].BroadcastTxSync(context.Background(), tx)
		}
	}

	//now send out the batches
	for i := range batches {
		bs.Status[i].Returns, bs.Status[i].Err = batches[i].Send(context.Background())
	}

	status <- bs
}

func dispatchSingles(sendSingles []txBatch, status chan BatchedStatus) {
	defer close(status)

	bs := BatchedStatus{}
	bs.Status = make([]DispatchStatus, len(sendSingles))

	for i, single := range sendSingles {
		bs.Status[i].Returns = make([]interface{}, len(single.tx))
		for j := range single.tx {
			if debugTxSend {
				fmt.Printf("Send TX %X\n", sha256.Sum256(single.tx[j]))
			}
			bs.Status[i].Returns[j], bs.Status[i].Err = single.route.GetBroadcastClient().BroadcastTxSync(context.Background(), single.tx[j])
			bs.Status[i].Route = single.route
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

	route, err := r.connRouter.SelectRoute(gtx.Transaction.Origin, false)
	if err != nil {
		return nil, err
	}

	return route.GetBroadcastClient().BroadcastTxAsync(context.Background(), tx)
}

// Query
// This function will return the state object from the accumulate network for a given URL.
func (r *Relay) Query(route connections.Route, data bytes.HexBytes) (ret *ctypes.ResultABCIQuery, err error) {
	return route.GetQueryClient().ABCIQuery(context.Background(), "/abci_query", data)
}

// QueryByUrl
// This function will return the state object from the accumulate network for a given URL.
func (r *Relay) QueryByUrl(adiUrl *url.URL, data bytes.HexBytes) (ret *ctypes.ResultABCIQuery, err error) {
	route, err := r.GetConnectionRouter().SelectRoute(adiUrl, false)
	if err != nil {
		return nil, err
	}

	return route.GetQueryClient().ABCIQuery(context.Background(), "/abci_query", data)
}

func (r *Relay) GetTx(route connections.Route, hash []byte) (*ctypes.ResultTx, error) {
	return route.GetRawClient().Tx(context.Background(), hash, false)
}

func (r *Relay) GetConnectionRouter() connections.ConnectionRouter {
	return r.connRouter
}

func decodeTX(tx tmtypes.Tx) (*transactions.Envelope, error) {
	gtx := new(transactions.Envelope)
	err := gtx.UnmarshalBinary(tx)
	if err != nil {
		return nil, err
	}
	return gtx, nil
}
