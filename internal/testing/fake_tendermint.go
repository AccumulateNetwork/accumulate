package testing

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	mock_api "github.com/AccumulateNetwork/accumulate/internal/mock/api"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/database"
	"github.com/AccumulateNetwork/accumulate/internal/relay"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/types/api/query"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/bytes"
	tmpubsub "github.com/tendermint/tendermint/libs/pubsub"
	tmquery "github.com/tendermint/tendermint/libs/pubsub/query"
	rpc "github.com/tendermint/tendermint/rpc/client"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	"github.com/tendermint/tendermint/types"
)

const debugTX = false

// FakeTendermint is a test harness that facilitates testing the ABCI
// application without creating an actual Tendermint node.
type FakeTendermint struct {
	mock_api.MockRoute
	*types.EventBus

	CreateEmptyBlocks bool

	appWg      *sync.WaitGroup
	app        abci.Application
	db         *database.Database
	network    *config.Network
	nextHeight func() int64
	onError    func(err error)

	stop    chan struct{}
	stopped *sync.WaitGroup
	didStop int32

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
	CheckResult   *abci.ResponseCheckTx
	DeliverResult *abci.ResponseDeliverTx
	Done          bool
}

var _ relay.Client = (*FakeTendermint)(nil)

func NewFakeTendermint(app <-chan abci.Application, db *database.Database, network *config.Network, nextHeight func() int64, onError func(err error), interval time.Duration) *FakeTendermint {
	c := new(FakeTendermint)
	c.appWg = new(sync.WaitGroup)
	c.db = db
	c.network = network
	c.nextHeight = nextHeight
	c.onError = onError
	c.EventBus = types.NewEventBus()
	c.stop = make(chan struct{})
	c.txCh = make(chan *txStatus)
	c.stopped = new(sync.WaitGroup)
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

func (c *FakeTendermint) Shutdown() {
	if !atomic.CompareAndSwapInt32(&c.didStop, 0, 1) {
		return
	}

	close(c.stop)
	c.stopped.Wait()
}

func (c *FakeTendermint) App() abci.Application {
	c.appWg.Wait()
	return c.app
}

// WaitForAll until all pending transactions are done
func (c *FakeTendermint) WaitForAll() {
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

func (c *FakeTendermint) WaitFor(txid [32]byte, waitSynth bool) error {
	if debugTX {
		fmt.Printf("Waiting for transaction %X\n", txid)
	}
	c.txMu.RLock()
	st := c.txStatus[txid]
	for st == nil || !st.Done {
		c.txCond.Wait()
		st = c.txStatus[txid]
	}
	c.txMu.RUnlock()
	if debugTX {
		fmt.Printf("Done waiting for %X\n", txid)
	}

	if !waitSynth {
		return nil
	}

	var err error
	reqTx := new(query.RequestByTxId)
	reqTx.TxId = txid
	q := new(query.Query)
	q.Type = reqTx.Type()
	q.Content, err = reqTx.MarshalBinary()
	if err != nil {
		return err
	}
	data, err := q.MarshalBinary()
	if err != nil {
		return err
	}
	r := c.App().Query(abci.RequestQuery{Data: data})
	if r.Code != 0 {
		return fmt.Errorf("failed: %v", r)
	}

	resTx := new(query.ResponseByTxId)
	err = resTx.UnmarshalBinary(r.Value)
	if err != nil {
		return err
	}

	n := len(resTx.TxSynthTxIds) / 32
	for i := 0; i < n; i++ {
		var id [32]byte
		copy(id[:], resTx.TxSynthTxIds[i*32:])
		err = c.WaitFor(id, true)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *FakeTendermint) SubmitTx(ctx context.Context, tx types.Tx) *txStatus {
	st := c.didSubmit(tx, sha256.Sum256(tx))
	if st == nil {
		return nil
	}

	c.stopped.Add(1)
	defer c.stopped.Done()

	select {
	case <-c.stop:
		return nil
	case c.txCh <- st:
		return st
	}
}

func (c *FakeTendermint) didSubmit(tx []byte, txh [32]byte) *txStatus {
	envelopes, err := transactions.UnmarshalAll(tx)
	if err != nil {
		c.onError(err)
		if debugTX {
			fmt.Printf("Rejecting invalid transaction: %v\n", err)
		}
		return nil
	}

	txids := make([][32]byte, len(envelopes))
	for i, env := range envelopes {
		if debugTX {
			txt := env.Transaction.Type()
			fmt.Printf("Submitting %v %X\n", txt, txh)
		}
		copy(txids[i][:], env.Transaction.Hash())
	}

	c.txMu.Lock()
	defer c.txMu.Unlock()

	for _, txid := range txids {
		delete(c.txPend, txid)
	}

	st, ok := c.txStatus[txh]
	if ok {
		// Ignore duplicate transactions
		return st
	}

	st = new(txStatus)
	c.txStatus[txh] = st
	for _, txid := range txids {
		c.txStatus[txid] = st
	}
	st.Tx = tx
	st.Hash = txh
	c.txActive++
	return st
}

func (c *FakeTendermint) getSynthHeight() int64 {
	batch := c.db.Begin()
	defer batch.Discard()

	// Load the ledger state
	ledger := batch.Account(c.network.NodeUrl(protocol.Ledger))
	ledgerState := protocol.NewInternalLedger()
	err := ledger.GetStateAs(ledgerState)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return 0
		}
		panic(err)
	}

	synthChain, err := ledger.ReadChain(protocol.SyntheticChain)
	if err != nil {
		panic(err)
	}

	return synthChain.Height()
}

func (c *FakeTendermint) addSynthTxns(blockIndex, lastHeight int64) (int64, bool) {
	batch := c.db.Begin()
	defer batch.Discard()

	// Load the ledger state
	ledger := batch.Account(c.network.NodeUrl(protocol.Ledger))
	ledgerState := protocol.NewInternalLedger()
	err := ledger.GetStateAs(ledgerState)
	if err != nil {
		c.onError(err)
		return 0, false
	}

	if ledgerState.Index != blockIndex {
		return 0, false
	}

	synthChain, err := ledger.ReadChain(protocol.SyntheticChain)
	if err != nil {
		c.onError(err)
		return 0, false
	}

	height := synthChain.Height()
	txns, err := synthChain.Entries(height-lastHeight, height)
	if err != nil {
		c.onError(err)
		return 0, false
	}

	c.txMu.Lock()
	defer c.txMu.Unlock()
	for _, h := range txns {
		var h32 [32]byte
		copy(h32[:], h)
		c.txPend[h32] = true
	}

	return height, true
}

// execute accepts incoming transactions and queues them up for the next block
func (c *FakeTendermint) execute(interval time.Duration) {
	c.stopped.Add(1)
	defer c.stopped.Done()

	var queue []*txStatus
	var hadTxnLastTime bool
	tick := time.NewTicker(interval)

	defer func() {
		for _, sub := range queue {
			sub.CheckResult = &abci.ResponseCheckTx{
				Code: 1,
				Info: "Canceled",
				Log:  "Canceled",
			}
			sub.Done = true
		}

		c.txActive = 0
		c.txPend = map[[32]byte]bool{}
		c.txCond.Broadcast()
	}()

	synthHeight := c.getSynthHeight()

	for {
		// Collect transactions, submit at 1Hz
		select {
		case <-c.stop:
			return

		case sub := <-c.txCh:
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
		case <-c.stop:
			return

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
			if debugTX {
				fmt.Printf("Checked %X\n", sub.Hash)
			}
			if cr.Code != 0 {
				c.onError(fmt.Errorf("CheckTx failed: %v\n", cr.Log))
				continue
			}

			dr := c.app.DeliverTx(abci.RequestDeliverTx{Tx: sub.Tx})
			sub.DeliverResult = &dr
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
			sub.Done = true
			if debugTX {
				fmt.Printf("Comitted %X\n", sub.Hash)
			}
		}

		// Ensure Wait waits for synthetic transactions
		if h, ok := c.addSynthTxns(height, synthHeight); ok {
			synthHeight = h
		}

		c.txActive -= len(queue)
		c.txMu.RLock()
		c.txCond.Broadcast()
		c.txMu.RUnlock()

		// Remember if this block had transactions
		hadTxnLastTime = len(queue) > 0

		// Clear the queue (reuse the memory)
		queue = queue[:0]

		if debugTX {
			fmt.Printf("Completed block %d with %d transactions pending\n", height, c.txActive+len(c.txPend))
		}
	}
}

func (c *FakeTendermint) Tx(ctx context.Context, hash []byte, prove bool) (*ctypes.ResultTx, error) {
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

func (c *FakeTendermint) ABCIInfo(context.Context) (*ctypes.ResultABCIInfo, error) {
	r := c.App().Info(abci.RequestInfo{})
	return &ctypes.ResultABCIInfo{Response: r}, nil
}

func (c *FakeTendermint) ABCIQuery(ctx context.Context, path string, data bytes.HexBytes) (*ctypes.ResultABCIQuery, error) {
	return c.ABCIQueryWithOptions(ctx, path, data, rpc.DefaultABCIQueryOptions)
}

func (c *FakeTendermint) ABCIQueryWithOptions(ctx context.Context, path string, data bytes.HexBytes, opts rpc.ABCIQueryOptions) (*ctypes.ResultABCIQuery, error) {
	r := c.App().Query(abci.RequestQuery{Data: data, Path: path, Height: opts.Height, Prove: opts.Prove})
	return &ctypes.ResultABCIQuery{Response: r}, nil
}

func (c *FakeTendermint) CheckTx(ctx context.Context, tx types.Tx) (*ctypes.ResultCheckTx, error) {
	cr := c.App().CheckTx(abci.RequestCheckTx{Tx: tx})
	return &ctypes.ResultCheckTx{ResponseCheckTx: cr}, nil
}

func (c *FakeTendermint) BroadcastTxAsync(ctx context.Context, tx types.Tx) (*ctypes.ResultBroadcastTx, error) {
	// Wait for nothing
	c.SubmitTx(ctx, tx)
	return &ctypes.ResultBroadcastTx{}, nil
}

func (c *FakeTendermint) BroadcastTxSync(ctx context.Context, tx types.Tx) (*ctypes.ResultBroadcastTx, error) {
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

func (c *FakeTendermint) BroadcastTxCommit(ctx context.Context, tx types.Tx) (*ctypes.ResultBroadcastTxCommit, error) {
	return nil, errors.New("not supported")
}

// Stolen from Tendermint
func (c *FakeTendermint) Subscribe(ctx context.Context, subscriber, query string, outCapacity ...int) (out <-chan ctypes.ResultEvent, err error) {
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

func (c *FakeTendermint) eventsRoutine(
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
func (c *FakeTendermint) resubscribe(subscriber string, q tmpubsub.Query) types.Subscription {
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

func (c *FakeTendermint) Unsubscribe(ctx context.Context, subscriber, query string) error {
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
