package testing

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/crypto"
	"github.com/tendermint/tendermint/crypto/ed25519"
	tmbytes "github.com/tendermint/tendermint/libs/bytes"
	"github.com/tendermint/tendermint/libs/log"
	protocrypto "github.com/tendermint/tendermint/proto/tendermint/crypto"
	rpc "github.com/tendermint/tendermint/rpc/client"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	"github.com/tendermint/tendermint/types"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

const debugTX = true

// FakeTendermint is a test harness that facilitates testing the ABCI
// application without creating an actual Tendermint node.
type FakeTendermint struct {
	appWg      *sync.WaitGroup
	app        abci.Application
	db         *database.Database
	network    *config.Network
	nextHeight func() int64
	address    crypto.Address
	logger     log.Logger
	onError    func(err error)
	validators []crypto.PubKey

	stop    chan struct{}
	stopped *sync.WaitGroup
	didStop int32

	txCh     chan *txStatus
	txStatus map[[32]byte]*txStatus
	txPend   map[[32]byte]bool
	txMu     *sync.RWMutex
	txCond   *sync.Cond
	txActive int
	isEvil   bool
}

type txStatus struct {
	Envelopes     []*protocol.Envelope
	Tx            []byte
	Hash          [32]byte
	Height        int64
	Index         uint32
	CheckResult   *abci.ResponseCheckTx
	DeliverResult *abci.ResponseDeliverTx
	Done          bool
}

func NewFakeTendermint(app <-chan abci.Application, db *database.Database, network *config.Network, pubKey crypto.PubKey, logger log.Logger, nextHeight func() int64, onError func(err error), interval time.Duration, isEvil bool) *FakeTendermint {
	c := new(FakeTendermint)
	c.appWg = new(sync.WaitGroup)
	c.db = db
	c.network = network
	c.nextHeight = nextHeight
	c.address = pubKey.Address()
	c.logger = logger
	c.onError = onError
	c.stop = make(chan struct{})
	c.txCh = make(chan *txStatus)
	c.stopped = new(sync.WaitGroup)
	c.txStatus = map[[32]byte]*txStatus{}
	c.txPend = map[[32]byte]bool{}
	c.txMu = new(sync.RWMutex)
	c.txCond = sync.NewCond(c.txMu.RLocker())
	c.isEvil = isEvil

	c.validators = []crypto.PubKey{pubKey}

	c.appWg.Add(1)
	go func() {
		defer c.appWg.Done()
		c.app = <-app
	}()

	go c.execute(interval)
	return c
}

func (c *FakeTendermint) Validators() []crypto.PubKey {
	return c.validators
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
			c.logger.Error("Rejecting invalid transaction", "error", err)
		}
		return nil
	}

	txids := make([][32]byte, len(envelopes))
	c.logTxns("Submitting", envelopes...)
	for i, env := range envelopes {
		copy(txids[i][:], env.GetTxHash())
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
	st.Envelopes = envelopes
	st.Tx = tx
	st.Hash = txh
	c.txActive++
	return st
}

func (c *FakeTendermint) getSynthHeight() int64 {
	batch := c.db.Begin(false)
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
	batch := c.db.Begin(false)
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
	tick := time.NewTicker(interval)

	logger := c.logger.With("scope", "runloop")
	logger.Debug("Start", "interval", interval.String())
	defer logger.Debug("Stop")

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
		logger.Debug("Collecting transactions")
		select {
		case <-c.stop:
			return

		case sub := <-c.txCh:
			queue = append(queue, sub)
			logger.Debug("Got transaction(s)", "count", len(sub.Envelopes))
			continue

		case <-tick.C:
			if c.app == nil {
				logger.Debug("Waiting for ABCI")
				c.App()
				logger.Debug("ABCI ready")
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
			logger.Debug("Got transaction(s)", "count", len(sub.Envelopes))
			goto collect

		default:
			// Done
		}

		height := c.nextHeight()
		logger.Info("Beginning block", "height", height, "queue", len(queue))

		begin := abci.RequestBeginBlock{}
		begin.Header.Height = height
		begin.Header.ProposerAddress = c.address
		if c.isEvil {
			//add evidence of something happening to the evidence chain.
			ev := abci.Evidence{}
			ev.Validator.Address = c.address
			ev.Type = abci.EvidenceType_LIGHT_CLIENT_ATTACK
			ev.Height = height
			ev.Time.Add(interval * time.Duration(ev.Height))
			ev.TotalVotingPower = 1
			begin.ByzantineValidators = append(begin.ByzantineValidators, ev)
		}

		c.app.BeginBlock(begin)

		logger.Info("Processing queue", "height", height, "queue", len(queue))

		// Process the queue
		for _, sub := range queue {
			c.logTxns("Processing", sub.Envelopes...)

			// TODO Index
			sub.Height = begin.Header.Height

			cr := c.app.CheckTx(abci.RequestCheckTx{Tx: sub.Tx})
			sub.CheckResult = &cr
			c.logTxns("Checked", sub.Envelopes...)
			if cr.Code != 0 {
				c.onError(fmt.Errorf("CheckTx failed: %v\n", cr.Log))
				continue
			}

			dr := c.app.DeliverTx(abci.RequestDeliverTx{Tx: sub.Tx})
			sub.DeliverResult = &dr
			c.logTxns("Delivered", sub.Envelopes...)
			if dr.Code != 0 {
				c.onError(fmt.Errorf("DeliverTx failed: %v\n", dr.Log))
			}
		}

		endBlockResp := c.app.EndBlock(abci.RequestEndBlock{})
		c.app.Commit()

		for _, update := range endBlockResp.ValidatorUpdates {
			c.applyValidatorUpdate(&update)
		}

		for _, sub := range queue {
			sub.Done = true
			c.logTxns("Committed", sub.Envelopes...)
		}

		// Ensure Wait waits for synthetic transactions
		if h, ok := c.addSynthTxns(height, synthHeight); ok {
			synthHeight = h
		}

		c.txActive -= len(queue)
		c.txMu.RLock()
		c.txCond.Broadcast()
		c.txMu.RUnlock()

		// Clear the queue (reuse the memory)
		queue = queue[:0]

		if debugTX {
			c.logger.Info("Completed block", "height", height, "tx-pending", c.txActive+len(c.txPend))
		}
	}
}

func (c *FakeTendermint) applyValidatorUpdate(update *abci.ValidatorUpdate) {
	key, ok := update.PubKey.Sum.(*protocrypto.PublicKey_Ed25519)
	if !ok {
		panic(fmt.Errorf("expected ED25519, got %T", update.PubKey.Sum))
	}

	if update.Power > 0 {
		c.validators = append(c.validators, ed25519.PubKey(key.Ed25519))
		return
	}

	for i, v := range c.validators {
		if bytes.Equal(v.Bytes(), key.Ed25519) {
			copy(c.validators[i:], c.validators[i+1:])
			c.validators = c.validators[:len(c.validators)-1]
			return
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

func (c *FakeTendermint) ABCIQuery(ctx context.Context, path string, data tmbytes.HexBytes) (*ctypes.ResultABCIQuery, error) {
	return c.ABCIQueryWithOptions(ctx, path, data, rpc.DefaultABCIQueryOptions)
}

func (c *FakeTendermint) ABCIQueryWithOptions(ctx context.Context, path string, data tmbytes.HexBytes, opts rpc.ABCIQueryOptions) (*ctypes.ResultABCIQuery, error) {
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

func (c *FakeTendermint) logTxns(msg string, env ...*protocol.Envelope) {
	if !debugTX {
		return
	}

	for _, env := range env {
		txt := env.Transaction.Type()
		if !txt.IsInternal() && txt != protocol.TransactionTypeSyntheticAnchor {
			c.logger.Debug(msg, "type", txt, "tx", logging.AsHex(env.GetTxHash()), "env", logging.AsHex(env.EnvHash()))
		}
	}
}
