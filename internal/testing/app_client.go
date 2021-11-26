package testing

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/AccumulateNetwork/accumulate/internal/relay"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/bytes"
	tmpubsub "github.com/tendermint/tendermint/libs/pubsub"
	tmquery "github.com/tendermint/tendermint/libs/pubsub/query"
	rpc "github.com/tendermint/tendermint/rpc/client"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	"github.com/tendermint/tendermint/types"
)

const debugTX = false

type ABCIApplicationClient struct {
	*types.EventBus

	CreateEmptyBlocks bool

	appWg      *sync.WaitGroup
	app        abci.Application
	db         *state.StateDB
	nextHeight func() int64
	onError    func(err error)

	done   chan struct{}
	doneMu *sync.Mutex

	txCh     chan *txStatus
	txStatus map[[32]byte]*txStatus
	txPend   map[[32]byte]bool
	txMu     *sync.RWMutex
	txCond   *sync.Cond
	txActive int
}

type txStatus struct {
	Tx            []byte
	Hash          [32]byte
	Height        int64
	Index         uint32
	DidCheck      chan struct{}
	DidDeliver    chan struct{}
	DidCommit     chan struct{}
	CheckResult   *abci.ResponseCheckTx
	DeliverResult *abci.ResponseDeliverTx
	Done          bool
}

var _ relay.Client = (*ABCIApplicationClient)(nil)

func NewABCIApplicationClient(app <-chan abci.Application, db *state.StateDB, nextHeight func() int64, onError func(err error), interval time.Duration) *ABCIApplicationClient {
	c := new(ABCIApplicationClient)
	c.appWg = new(sync.WaitGroup)
	c.db = db
	c.nextHeight = nextHeight
	c.onError = onError
	c.EventBus = types.NewEventBus()
	c.done = make(chan struct{})
	c.doneMu = new(sync.Mutex)
	c.txCh = make(chan *txStatus)
	c.txStatus = map[[32]byte]*txStatus{}
	c.txPend = map[[32]byte]bool{}
	c.txMu = new(sync.RWMutex)
	c.txCond = sync.NewCond(c.txMu.RLocker())

	c.appWg.Add(1)
	go func() {
		defer c.appWg.Done()
		c.app = <-app
	}()

	go c.execute(interval)
	return c
}

func (c *ABCIApplicationClient) Shutdown() {
	c.doneMu.Lock()
	defer c.doneMu.Unlock()

	select {
	case <-c.done:
		return
	default:
	}

	close(c.txCh)
	<-c.done
}

func (c *ABCIApplicationClient) App() abci.Application {
	c.appWg.Wait()
	return c.app
}

// Wait until all pending transactions are done
func (c *ABCIApplicationClient) Wait() {
	if debugTX {
		fmt.Printf("Waiting for transactions\n")
	}
	c.txMu.RLock()
	defer c.txMu.RUnlock()
	for c.txActive > 0 || len(c.txPend) > 0 {
		c.txCond.Wait()
	}
	if debugTX {
		fmt.Printf("Done waiting\n")
	}
}

func (c *ABCIApplicationClient) SubmitTx(ctx context.Context, tx types.Tx) *txStatus {
	st := c.didSubmit(tx, sha256.Sum256(tx))
	if st == nil {
		return nil
	}

	c.txCh <- st
	return st
}

func (c *ABCIApplicationClient) didSubmit(tx []byte, txh [32]byte) *txStatus {
	gtx := new(transactions.GenTransaction)
	_, err := gtx.UnMarshal(tx)
	if err != nil {
		c.onError(err)
		if debugTX {
			fmt.Printf("Rejecting invalid transaction: %v\n", err)
		}
		return nil
	}

	if debugTX {
		txt := gtx.TransactionType()
		fmt.Printf("Submitting %v %X\n", txt, txh)
	}

	var txid [32]byte
	copy(txid[:], gtx.TransactionHash())

	c.txMu.Lock()
	defer c.txMu.Unlock()

	delete(c.txPend, txid)

	st, ok := c.txStatus[txh]
	if ok {
		// Ignore duplicate transactions
		return st
	}

	st = new(txStatus)
	c.txStatus[txh] = st
	st.Tx = tx
	st.Hash = txh
	st.DidCheck = make(chan struct{})
	st.DidDeliver = make(chan struct{})
	st.DidCommit = make(chan struct{})
	c.txActive++
	return st
}

func (c *ABCIApplicationClient) addSynthTxns() {
	// Check for synthetic transactions in the anchor chain
	head, height, err := c.db.GetAnchorHead()
	if errors.Is(err, storage.ErrNotFound) {
		return
	} else if err != nil {
		c.onError(err)
		return
	}

	count := height - head.PreviousHeight - int64(len(head.Chains)) - 1
	if count == 0 {
		return
	}

	if debugTX {
		fmt.Printf("The last block created %d synthetic transactions\n", count)
	}

	// Pull the transaction IDs from the anchor chain
	txns, err := c.db.GetAnchors(height-count-1, height-1)
	if err != nil {
		c.onError(err)
		return
	}

	c.txMu.Lock()
	defer c.txMu.Unlock()
	for _, h := range txns {
		c.txPend[h] = true
	}
}

// execute accepts incoming transactions and queues them up for the next block
func (c *ABCIApplicationClient) execute(interval time.Duration) {
	defer close(c.done)

	var queue []*txStatus
	var hadTxnLastTime bool
	tick := time.NewTicker(interval)

	for {
		// Collect transactions, submit at 1Hz
		select {
		case sub, ok := <-c.txCh:
			if !ok {
				for _, sub := range queue {
					sub.CheckResult = &abci.ResponseCheckTx{
						Code: 1,
						Info: "Canceled",
						Log:  "Canceled",
					}
					close(sub.DidCheck)
					close(sub.DidDeliver)
					close(sub.DidCommit)
					sub.Done = true
				}
				return
			}
			queue = append(queue, sub)
			continue

		case <-tick.C:
			if len(queue) == 0 && !c.CreateEmptyBlocks && !hadTxnLastTime {
				continue
			}
			if c.app == nil {
				continue
			}
		}

		// Collect any queued up sends
	collect:
		select {
		case sub := <-c.txCh:
			queue = append(queue, sub)
			goto collect
		default:
			// Done
		}

		height := c.nextHeight()
		if debugTX {
			fmt.Printf("Beginning block %d with %d transactions queued\n", height, len(queue))
		}

		begin := abci.RequestBeginBlock{}
		begin.Header.Height = height
		c.app.BeginBlock(begin)

		// Process the queue
		for _, sub := range queue {
			// TODO Index
			sub.Height = begin.Header.Height

			cr := c.app.CheckTx(abci.RequestCheckTx{Tx: sub.Tx})
			sub.CheckResult = &cr
			close(sub.DidCheck)
			if debugTX {
				fmt.Printf("Checked %X\n", sub.Hash)
			}
			if cr.Code != 0 {
				c.onError(fmt.Errorf("CheckTx failed: %v\n", cr.Log))
				close(sub.DidDeliver)
				continue
			}

			dr := c.app.DeliverTx(abci.RequestDeliverTx{Tx: sub.Tx})
			sub.DeliverResult = &dr
			close(sub.DidDeliver)
			if debugTX {
				fmt.Printf("Delivered %X\n", sub.Hash)
			}
			if dr.Code != 0 {
				c.onError(fmt.Errorf("DeliverTx failed: %v\n", dr.Log))
			}

			err := c.PublishEventTx(types.EventDataTx{TxResult: abci.TxResult{
				Height: sub.Height,
				Index:  sub.Index,
				Tx:     sub.Tx,
				Result: dr,
			}})
			if err != nil {
				c.onError(err)
			}
		}

		c.app.EndBlock(abci.RequestEndBlock{})
		c.app.Commit()

		for _, sub := range queue {
			close(sub.DidCommit)
			sub.Done = true
			if debugTX {
				fmt.Printf("Comitted %X\n", sub.Hash)
			}
		}

		// Ensure Wait waits for synthetic transactions
		c.addSynthTxns()

		c.txActive -= len(queue)
		c.txCond.Broadcast()

		// Remember if this block had transactions
		hadTxnLastTime = len(queue) > 0

		// Clear the queue (reuse the memory)
		queue = queue[:0]

		if debugTX {
			fmt.Printf("Completed block %d with %d transactions pending\n", height, c.txActive+len(c.txPend))
		}
	}
}

func (c *ABCIApplicationClient) Tx(ctx context.Context, hash []byte, prove bool) (*ctypes.ResultTx, error) {
	var h [32]byte
	copy(h[:], hash)

	c.txMu.RLock()
	st := c.txStatus[h]
	c.txMu.RUnlock()

	if st == nil || st.DeliverResult == nil {
		return nil, errors.New("not found")
	}
	return &ctypes.ResultTx{
		Hash:     st.Hash[:],
		Height:   st.Height,
		Index:    st.Index,
		Tx:       st.Tx,
		TxResult: *st.DeliverResult,
	}, nil
}

func (c *ABCIApplicationClient) ABCIInfo(context.Context) (*ctypes.ResultABCIInfo, error) {
	r := c.App().Info(abci.RequestInfo{})
	return &ctypes.ResultABCIInfo{Response: r}, nil
}

func (c *ABCIApplicationClient) ABCIQuery(ctx context.Context, path string, data bytes.HexBytes) (*ctypes.ResultABCIQuery, error) {
	return c.ABCIQueryWithOptions(ctx, path, data, rpc.DefaultABCIQueryOptions)
}

func (c *ABCIApplicationClient) ABCIQueryWithOptions(ctx context.Context, path string, data bytes.HexBytes, opts rpc.ABCIQueryOptions) (*ctypes.ResultABCIQuery, error) {
	r := c.App().Query(abci.RequestQuery{Data: data, Path: path, Height: opts.Height, Prove: opts.Prove})
	return &ctypes.ResultABCIQuery{Response: r}, nil
}

func (c *ABCIApplicationClient) CheckTx(ctx context.Context, tx types.Tx) (*ctypes.ResultCheckTx, error) {
	cr := c.App().CheckTx(abci.RequestCheckTx{Tx: tx})
	return &ctypes.ResultCheckTx{ResponseCheckTx: cr}, nil
}

func (c *ABCIApplicationClient) BroadcastTxAsync(ctx context.Context, tx types.Tx) (*ctypes.ResultBroadcastTx, error) {
	// Wait for nothing
	c.SubmitTx(ctx, tx)
	return &ctypes.ResultBroadcastTx{}, nil
}

func (c *ABCIApplicationClient) BroadcastTxSync(ctx context.Context, tx types.Tx) (*ctypes.ResultBroadcastTx, error) {
	cr := c.app.CheckTx(abci.RequestCheckTx{Tx: tx})
	if cr.Code != 0 {
		c.onError(fmt.Errorf("CheckTx failed: %v\n", cr.Log))
	} else {
		c.SubmitTx(ctx, tx)
	}

	hash := sha256.Sum256(tx)
	return &ctypes.ResultBroadcastTx{
		Code:         cr.Code,
		Data:         cr.Data,
		Log:          cr.Log,
		Codespace:    cr.Codespace,
		MempoolError: cr.MempoolError,
		Hash:         hash[:],
	}, nil
}

func (c *ABCIApplicationClient) BroadcastTxCommit(ctx context.Context, tx types.Tx) (*ctypes.ResultBroadcastTxCommit, error) {
	st := c.SubmitTx(ctx, tx)
	<-st.DidCommit

	r := new(ctypes.ResultBroadcastTxCommit)
	r.Hash = st.Hash[:]
	r.Height = st.Height
	r.CheckTx = *st.CheckResult
	if st.CheckResult.Code != 0 {
		return r, nil
	}

	r.DeliverTx = *st.DeliverResult
	return r, nil
}

// Stolen from Tendermint
func (c *ABCIApplicationClient) Subscribe(ctx context.Context, subscriber, query string, outCapacity ...int) (out <-chan ctypes.ResultEvent, err error) {
	q, err := tmquery.New(query)
	if err != nil {
		return nil, fmt.Errorf("failed to parse query: %w", err)
	}

	outCap := 1
	if len(outCapacity) > 0 {
		outCap = outCapacity[0]
	}

	var sub types.Subscription
	if outCap > 0 {
		sub, err = c.EventBus.Subscribe(ctx, subscriber, q, outCap)
	} else {
		sub, err = c.EventBus.SubscribeUnbuffered(ctx, subscriber, q)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe: %w", err)
	}
	if sub == nil {
		return nil, fmt.Errorf("node is shut down")
	}

	outc := make(chan ctypes.ResultEvent, outCap)
	go c.eventsRoutine(sub, subscriber, q, outc)

	return outc, nil
}

func (c *ABCIApplicationClient) eventsRoutine(
	sub types.Subscription,
	subscriber string,
	q tmpubsub.Query,
	outc chan<- ctypes.ResultEvent) {
	for {
		select {
		case msg := <-sub.Out():
			result := ctypes.ResultEvent{
				SubscriptionID: msg.SubscriptionID(),
				Query:          q.String(),
				Data:           msg.Data(),
				Events:         msg.Events(),
			}

			if cap(outc) == 0 {
				outc <- result
			} else {
				select {
				case outc <- result:
				default:
					c.Logger.Error("wanted to publish ResultEvent, but out channel is full", "result", result, "query", result.Query)
				}
			}
		case <-sub.Canceled():
			if sub.Err() == tmpubsub.ErrUnsubscribed {
				return
			}

			c.Logger.Error("subscription was canceled, resubscribing...", "err", sub.Err(), "query", q.String())
			sub = c.resubscribe(subscriber, q)
			if sub == nil { // client was stopped
				return
			}
		case <-c.Quit():
			return
		}
	}
}

// Try to resubscribe with exponential backoff.
func (c *ABCIApplicationClient) resubscribe(subscriber string, q tmpubsub.Query) types.Subscription {
	attempts := 0
	for {
		if !c.IsRunning() {
			return nil
		}

		sub, err := c.EventBus.Subscribe(context.Background(), subscriber, q)
		if err == nil {
			return sub
		}

		attempts++
		time.Sleep((10 << uint(attempts)) * time.Millisecond) // 10ms -> 20ms -> 40ms
	}
}

func (c *ABCIApplicationClient) Unsubscribe(ctx context.Context, subscriber, query string) error {
	args := tmpubsub.UnsubscribeArgs{Subscriber: subscriber}
	var err error
	args.Query, err = tmquery.New(query)
	if err != nil {
		// if this isn't a valid query it might be an ID, so
		// we'll try that. It'll turn into an error when we
		// try to unsubscribe. Eventually, perhaps, we'll want
		// to change the interface to only allow
		// unsubscription by ID, but that's a larger change.
		args.ID = query
	}
	return c.EventBus.Unsubscribe(ctx, args)
}
