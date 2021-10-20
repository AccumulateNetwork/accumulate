package testing

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/AccumulateNetwork/accumulated/internal/relay"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
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
	nextHeight func() int64
	onError    func(err error)

	txCh      chan submittedTx
	txResults map[[32]byte]*ctypes.ResultTx
	txMu      *sync.RWMutex
	txWg      *sync.WaitGroup
}

type submittedTx struct {
	Tx   types.Tx
	Done chan abci.ResponseDeliverTx
}

var _ relay.Client = (*ABCIApplicationClient)(nil)

func NewABCIApplicationClient(app <-chan abci.Application, nextHeight func() int64, onError func(err error), interval time.Duration) *ABCIApplicationClient {
	c := new(ABCIApplicationClient)
	c.appWg = new(sync.WaitGroup)
	c.txWg = new(sync.WaitGroup)
	c.nextHeight = nextHeight
	c.onError = onError
	c.EventBus = types.NewEventBus()
	c.txCh = make(chan submittedTx)
	c.txResults = map[[32]byte]*ctypes.ResultTx{}
	c.txMu = new(sync.RWMutex)

	c.appWg.Add(1)
	go func() {
		defer c.appWg.Done()
		c.app = <-app
	}()

	go c.blockify(interval)
	return c
}

func (c *ABCIApplicationClient) Shutdown() {
	close(c.txCh)
}

func (c *ABCIApplicationClient) Tx(ctx context.Context, hash []byte, prove bool) (*ctypes.ResultTx, error) {
	var h [32]byte
	copy(h[:], hash)

	c.txMu.RLock()
	r := c.txResults[h]
	c.txMu.RUnlock()

	if r == nil {
		return nil, errors.New("not found")
	}

	return r, nil
}

func (c *ABCIApplicationClient) App() abci.Application {
	c.appWg.Wait()
	return c.app
}

// Wait until all pending transactions are done
func (c *ABCIApplicationClient) Wait() {
	c.txWg.Wait()
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

func (c *ABCIApplicationClient) BroadcastTxAsync(ctx context.Context, tx types.Tx) (*ctypes.ResultBroadcastTx, error) {
	// Wait for nothing
	c.SubmitTx(ctx, tx)
	return &ctypes.ResultBroadcastTx{}, nil
}

func (c *ABCIApplicationClient) BroadcastTxSync(ctx context.Context, tx types.Tx) (*ctypes.ResultBroadcastTx, error) {
	// Wait for CheckTx
	checkChan, _ := c.SubmitTx(ctx, tx)
	cr := <-checkChan
	return &ctypes.ResultBroadcastTx{
		Code:         cr.Code,
		Data:         cr.Data,
		Log:          cr.Log,
		Codespace:    cr.Codespace,
		MempoolError: cr.MempoolError,
		// TODO Hash
	}, nil
}

func (c *ABCIApplicationClient) BroadcastTxCommit(ctx context.Context, tx types.Tx) (*ctypes.ResultBroadcastTxCommit, error) {
	checkChan, deliverChan := c.SubmitTx(ctx, tx)
	cr := <-checkChan
	dr := <-deliverChan
	return &ctypes.ResultBroadcastTxCommit{
		CheckTx:   cr,
		DeliverTx: dr,
		// TODO Hash, Height
	}, nil
}

// blockify accepts incoming transactions and queues them up for the next block
func (c *ABCIApplicationClient) blockify(interval time.Duration) {
	var queue []submittedTx
	tick := time.NewTicker(interval)

	defer func() {
		for _, sub := range queue {
			c.txWg.Done()
			close(sub.Done)
		}
	}()

	for {
		// Collect transactions, submit at 1Hz
		select {
		case sub, ok := <-c.txCh:
			if !ok {
				return
			}
			queue = append(queue, sub)
			continue

		case <-tick.C:
			if len(queue) == 0 && !c.CreateEmptyBlocks {
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

		// Callers shouldn't return until the block is over
		c.txWg.Add(1)

		begin := abci.RequestBeginBlock{}
		begin.Header.Height = c.nextHeight()
		c.app.BeginBlock(begin)

		// Process the queue
		for _, sub := range queue {
			result := c.app.DeliverTx(abci.RequestDeliverTx{Tx: sub.Tx})
			hash := sha256.Sum256(sub.Tx)
			rCore := &ctypes.ResultTx{
				// TODO Height, Index
				Hash:     hash[:],
				Tx:       sub.Tx,
				TxResult: result,
			}
			rAbci := abci.TxResult{
				// TODO Height, Index
				Tx:     sub.Tx,
				Result: result,
			}

			if result.Code != 0 {
				c.onError(fmt.Errorf("DeliverTx failed: %v\n", result.Log))
			}

			c.txMu.Lock()
			c.txResults[hash] = rCore
			c.txMu.Unlock()

			err := c.PublishEventTx(types.EventDataTx{TxResult: rAbci})
			if err != nil {
				c.onError(err)
			}

			sub.Done <- result
			close(sub.Done)
			c.txWg.Done()
		}

		// Clear the queue (reuse the memory)
		queue = queue[:0]

		c.app.EndBlock(abci.RequestEndBlock{})
		c.app.Commit()

		c.txWg.Done()
	}
}

func (c *ABCIApplicationClient) SubmitTx(ctx context.Context, tx types.Tx) (<-chan abci.ResponseCheckTx, <-chan abci.ResponseDeliverTx) {
	app := c.App()
	checkChan := make(chan abci.ResponseCheckTx, 1)
	deliverChan := make(chan abci.ResponseDeliverTx, 1)

	gtx := new(transactions.GenTransaction)
	if debugTX {
		_, err := gtx.UnMarshal(tx)
		if err != nil {
			// This should never be enabled outside of debugging, so panicking
			// is borderline acceptable
			panic(err)
		}
		fmt.Printf("Got TX %X\n", gtx.TransactionHash())
	}

	c.txWg.Add(1)
	go func() {
		defer close(checkChan)

		rc := app.CheckTx(abci.RequestCheckTx{Tx: tx})
		checkChan <- rc
		if rc.Code != 0 {
			c.onError(fmt.Errorf("CheckTx failed: %v\n", rc.Log))
			close(deliverChan)
			c.txWg.Done()
			return
		}

		c.txCh <- submittedTx{Tx: tx, Done: deliverChan}
	}()

	return checkChan, deliverChan
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
