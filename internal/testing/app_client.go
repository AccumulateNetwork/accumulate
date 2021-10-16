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
	appWg      *sync.WaitGroup
	blockMu    *sync.Mutex
	blockWg    *sync.WaitGroup
	app        abci.Application
	nextHeight func() int64
	onError    func(err error)

	txResults map[[32]byte]*ctypes.ResultTx
	txMu      *sync.RWMutex
}

var _ relay.Client = (*ABCIApplicationClient)(nil)

func NewABCIApplicationClient(app <-chan abci.Application, nextHeight func() int64, onError func(err error)) *ABCIApplicationClient {
	c := new(ABCIApplicationClient)
	c.appWg = new(sync.WaitGroup)
	c.blockMu = new(sync.Mutex)
	c.blockWg = new(sync.WaitGroup)
	c.nextHeight = nextHeight
	c.onError = onError
	c.EventBus = types.NewEventBus()
	c.txResults = map[[32]byte]*ctypes.ResultTx{}
	c.txMu = new(sync.RWMutex)

	c.appWg.Add(1)
	go func() {
		defer c.appWg.Done()
		c.app = <-app
	}()
	return c
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
	c.blockWg.Wait()
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
	c.sendTx(ctx, tx)
	return &ctypes.ResultBroadcastTx{}, nil
}

func (c *ABCIApplicationClient) BroadcastTxSync(ctx context.Context, tx types.Tx) (*ctypes.ResultBroadcastTx, error) {
	// Wait for CheckTx
	checkChan, _ := c.sendTx(ctx, tx)
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
	checkChan, deliverChan := c.sendTx(ctx, tx)
	cr := <-checkChan
	dr := <-deliverChan
	return &ctypes.ResultBroadcastTxCommit{
		CheckTx:   cr,
		DeliverTx: dr,
		// TODO Hash, Height
	}, nil
}

func (c *ABCIApplicationClient) enterBlock() {
	app := c.App()
	c.blockMu.Lock()
	begin := abci.RequestBeginBlock{}
	begin.Header.Height = c.nextHeight()
	app.BeginBlock(begin)
}

func (c *ABCIApplicationClient) exitBlock() {
	app := c.App()
	app.EndBlock(abci.RequestEndBlock{})

	// Should commit always happen? Or only on success?
	app.Commit()

	//need to add artificial sleep to allow for synth tx's to go through
	time.Sleep(100 * time.Millisecond)
	c.blockMu.Unlock()
}

func (c *ABCIApplicationClient) sendTx(ctx context.Context, tx types.Tx) (<-chan abci.ResponseCheckTx, <-chan abci.ResponseDeliverTx) {
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

	c.blockWg.Add(1)
	go func() {
		defer c.blockWg.Done()

		defer close(checkChan)
		defer close(deliverChan)
		if debugTX {
			defer fmt.Printf("Finished TX %X\n", gtx.TxHash)
		}

		// TODO Roll up multiple TXs into one block?
		c.enterBlock()
		defer c.exitBlock()

		rc := app.CheckTx(abci.RequestCheckTx{Tx: tx})
		checkChan <- rc
		if rc.Code != 0 {
			c.onError(fmt.Errorf("CheckTx failed: %v\n", rc.Log))
			return
		}

		rd := app.DeliverTx(abci.RequestDeliverTx{Tx: tx})
		hash := sha256.Sum256(tx)
		txr := &ctypes.ResultTx{
			// TODO Height, Index
			Hash:     hash[:],
			TxResult: rd,
			Tx:       tx,
		}
		c.txMu.Lock()
		c.txResults[hash] = txr
		c.txMu.Unlock()

		deliverChan <- rd
		if rd.Code != 0 {
			c.onError(fmt.Errorf("DeliverTx failed: %v\n", rd.Log))
			return
		}
	}()

	return checkChan, deliverChan
}

func (c *ABCIApplicationClient) Batch(inBlock func(func(*transactions.GenTransaction))) {
	app := c.App()

	c.blockWg.Add(1)
	defer c.blockWg.Done()

	c.enterBlock()
	defer c.exitBlock()

	inBlock(func(gtx *transactions.GenTransaction) {
		tx, err := gtx.Marshal()
		if err != nil {
			c.onError(err)
			return
		}

		rc := app.CheckTx(abci.RequestCheckTx{Tx: tx})
		if rc.Code != 0 {
			c.onError(fmt.Errorf("CheckTx failed: %v\n", rc.Log))
			return
		}

		rd := app.DeliverTx(abci.RequestDeliverTx{Tx: tx})
		hash := sha256.Sum256(tx)
		txr := &ctypes.ResultTx{
			// TODO Height, Index
			Hash:     hash[:],
			TxResult: rd,
			Tx:       tx,
		}
		c.txMu.Lock()
		c.txResults[hash] = txr
		c.txMu.Unlock()

		if rd.Code != 0 {
			c.onError(fmt.Errorf("DeliverTx failed: %v\n", rd.Log))
			return
		}
	})
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
