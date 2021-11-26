package relay

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/bytes"
	"github.com/tendermint/tendermint/rpc/client/http"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	tmtypes "github.com/tendermint/tendermint/types"
)

const debugTxSend = false

var ErrTimedOut = errors.New("timed out")

// Relay is the structure used to relay messages to the correct BVC.  Transactions can either be batched and dispatched
// or they can be sent directly.  They only know about GenTransactions and are routed according to the number of networks
// in the system

type txBatch struct {
	networkId int
	tx        [][]byte
}

type Relay struct {
	client      []Client
	txQueue     []txBatch
	numNetworks uint64
	mutex       *sync.Mutex

	stopResults chan struct{}
	resultMu    *sync.Mutex
	results     map[[32]byte]chan<- abci.TxResult
}

// New Create the new bouncer and initialize it with a client connection to each of the nodes
func New(clients ...Client) *Relay {
	for i, c := range clients {
		if c, ok := c.(*http.HTTP); ok {
			clients[i] = rpcClient{c}
		}
	}

	r := &Relay{}
	r.numNetworks = uint64(len(clients))
	r.client = clients
	r.mutex = new(sync.Mutex)
	r.resultMu = new(sync.Mutex)
	r.results = map[[32]byte]chan<- abci.TxResult{}
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

func (r *Relay) GetNetworkCount() uint64 {
	return r.numNetworks
}

func (r *Relay) Start() error {
	var failed bool

	if r.stopResults != nil {
		return errors.New("already started")
	}

	r.stopResults = make(chan struct{})
	cases := make([]reflect.SelectCase, 0, len(r.client)+1)
	cases = append(cases, reflect.SelectCase{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(r.stopResults),
	})

	for _, c := range r.client {
		err := c.Start()
		if err != nil {
			failed = true
			return err
		}

		// Subscribing to all transactions is messy, but making one subscription
		// per TX hash does not work well. Tendermint does not clean up
		// subscriptions sufficiently well, so old unused channels lead to
		// blockages.
		ch, err := c.Subscribe(context.Background(), "acc-relay", "tm.event = 'Tx'")
		if err != nil {
			failed = true
			return err
		}
		cases = append(cases, reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(ch),
		})

		c := c // Do not capture loop var
		defer func() {
			if failed {
				_ = c.Stop()
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

	for _, c := range r.client {
		err := c.Stop()
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
func (r *Relay) BatchSend() <-chan BatchedStatus {
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
	stat := make(chan BatchedStatus, 1)
	go dispatch(r.client, sendTxsAsBatch, sendTxsAsSingle, stat)
	r.resetBatches()

	r.mutex.Unlock()
	return stat
}

// dispatch
// This function is executed as a go routine to send out all the batches
func dispatch(client []Client, sendAsBatch []txBatch, sendAsSingle []txBatch, stat chan BatchedStatus) {
	defer close(stat)

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

func dispatchBatch(client []Client, sendBatches []txBatch, status chan BatchedStatus) {
	defer close(status)

	bs := BatchedStatus{}
	bs.Status = make([]DispatchStatus, len(sendBatches))

	var batches []Batch
	for i, txb := range sendBatches {
		batches = append(batches, client[txb.networkId].(Batchable).NewBatch())
		for _, tx := range sendBatches[i].tx {
			if debugTxSend {
				fmt.Printf("Send TX %X\n", sha256.Sum256(tx))
			}
			bs.Status[i].NetworkId = txb.networkId
			batches[i].BroadcastTxSync(context.Background(), tx)
		}
	}

	//now send out the batches
	for i := range batches {
		bs.Status[i].Returns, bs.Status[i].Err = batches[i].Send(context.Background())
	}

	status <- bs
}

func dispatchSingles(client []Client, sendSingles []txBatch, status chan BatchedStatus) {
	defer close(status)

	bs := BatchedStatus{}
	bs.Status = make([]DispatchStatus, len(sendSingles))

	for i, single := range sendSingles {
		bs.Status[i].NetworkId = single.networkId
		bs.Status[i].Returns = make([]interface{}, len(single.tx))
		for j := range single.tx {
			if debugTxSend {
				fmt.Printf("Send TX %X\n", sha256.Sum256(single.tx[j]))
			}
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
func (r *Relay) Query(routing uint64, data bytes.HexBytes) (ret *ctypes.ResultABCIQuery, err error) {
	return r.client[r.GetNetworkId(routing)].ABCIQuery(context.Background(), "/abci_query", data)
}

func (r *Relay) GetTx(routing uint64, hash []byte) (*ctypes.ResultTx, error) {
	return r.client[r.GetNetworkId(routing)].Tx(context.Background(), hash, false)
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
