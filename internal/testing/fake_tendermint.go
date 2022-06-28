package testing

import (
	"bytes"
	"context"
	"crypto/sha256"
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
	ctypes "github.com/tendermint/tendermint/rpc/coretypes"
	"github.com/tendermint/tendermint/types"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// FakeTendermint is a test harness that facilitates testing the ABCI
// application without creating an actual Tendermint node.
type FakeTendermint struct {
	appWg      *sync.WaitGroup
	app        abci.Application
	db         *database.Database
	network    *config.Describe
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
	txMu     *sync.RWMutex
	txActive int
	isEvil   bool
}

type txStatus struct {
	Envelopes     []*chain.Delivery
	Tx            []byte
	Hash          [32]byte
	Height        int64
	Index         uint32
	CheckResult   *abci.ResponseCheckTx
	DeliverResult *abci.ResponseDeliverTx
	Done          bool
}

func NewFakeTendermint(app <-chan abci.Application, db *database.Database, network *config.Describe, pubKey crypto.PubKey, logger log.Logger, nextHeight func() int64, onError func(err error), interval time.Duration, isEvil bool) *FakeTendermint {
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
	c.txMu = new(sync.RWMutex)
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

func (c *FakeTendermint) SubmitTx(ctx context.Context, tx types.Tx, check bool) *txStatus {
	st := c.didSubmit(tx, sha256.Sum256(tx))
	if st == nil {
		return nil
	}

	c.stopped.Add(1)
	defer c.stopped.Done()

	if check {
		cr := c.App().CheckTx(abci.RequestCheckTx{Tx: tx, Type: abci.CheckTxType_Recheck})
		st.CheckResult = &cr
		if cr.Code != 0 {
			c.onError(fmt.Errorf("CheckTx failed: %v\n", cr.Log))
			return st
		}
	}

	select {
	case <-c.stop:
		return nil
	case c.txCh <- st:
		return st
	}
}

func (c *FakeTendermint) didSubmit(tx []byte, txh [32]byte) *txStatus {
	env := new(protocol.Envelope)
	err := env.UnmarshalBinary(tx)
	if err != nil {
		c.onError(err)
		c.logger.Error("Rejecting invalid transaction", "error", err)
		return nil
	}

	deliveries, err := chain.NormalizeEnvelope(env)
	if err != nil {
		c.onError(err)
		c.logger.Error("Rejecting invalid transaction", "error", err)
		return nil
	}

	txids := make([][32]byte, len(deliveries))
	c.logTxns("Submitting", deliveries...)
	for i, env := range deliveries {
		copy(txids[i][:], env.Transaction.GetHash())
	}

	c.txMu.Lock()
	defer c.txMu.Unlock()

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
	st.Envelopes = deliveries
	st.Tx = tx
	st.Hash = txh
	c.txActive++
	return st
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
	}()

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
			c.checkResultSet(cr.Data)

			dr := c.app.DeliverTx(abci.RequestDeliverTx{Tx: sub.Tx})
			sub.DeliverResult = &dr
			c.logTxns("Delivered", sub.Envelopes...)
			if dr.Code != 0 {
				c.onError(fmt.Errorf("DeliverTx failed: %v\n", dr.Log))
			}
			c.checkResultSet(dr.Data)
		}

		endBlockResp := c.app.EndBlock(abci.RequestEndBlock{})
		c.app.Commit()

		for _, update := range endBlockResp.ValidatorUpdates {
			c.applyValidatorUpdate(&update) //nolint:rangevarref
		}

		for _, sub := range queue {
			sub.Done = true
			c.logTxns("Committed", sub.Envelopes...)
		}

		c.txActive -= len(queue)
		c.txMu.RLock()
		c.txMu.RUnlock()

		// Clear the queue (reuse the memory)
		queue = queue[:0]
	}
}

func (c *FakeTendermint) checkResultSet(data []byte) {
	rs := new(protocol.TransactionResultSet)
	err := rs.UnmarshalBinary(data)
	if err != nil {
		c.onError(fmt.Errorf("failed to unmarshal results: %v", err))
		return
	}

	for _, r := range rs.Results {
		if r.Error == nil || r.Code == errors.StatusDelivered {
			continue
		}

		c.onError(fmt.Errorf("DeliverTx failed: %+v", r.Error))
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
		return nil, errors.NotFound("not found")
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
	cr := c.App().CheckTx(abci.RequestCheckTx{Tx: tx, Type: abci.CheckTxType_Recheck})
	return &ctypes.ResultCheckTx{ResponseCheckTx: cr}, nil
}

func (c *FakeTendermint) BroadcastTxAsync(ctx context.Context, tx types.Tx) (*ctypes.ResultBroadcastTx, error) {
	// Wait for nothing
	c.SubmitTx(ctx, tx, false)
	return &ctypes.ResultBroadcastTx{}, nil
}

func (c *FakeTendermint) BroadcastTxSync(ctx context.Context, tx types.Tx) (*ctypes.ResultBroadcastTx, error) {
	st := c.SubmitTx(ctx, tx, true)
	if st == nil {
		h := sha256.Sum256(tx)
		return &ctypes.ResultBroadcastTx{
			Code: uint32(protocol.ErrorCodeUnknownError),
			Log:  "An unknown error occured",
			Hash: h[:],
		}, nil
	}

	return &ctypes.ResultBroadcastTx{
		Code:         st.CheckResult.Code,
		Data:         st.CheckResult.Data,
		Log:          st.CheckResult.Log,
		Codespace:    st.CheckResult.Codespace,
		MempoolError: st.CheckResult.MempoolError,
		Hash:         st.Hash[:],
	}, nil
}

func (c *FakeTendermint) logTxns(msg string, env ...*chain.Delivery) {
	for _, env := range env {
		txnType := env.Transaction.Body.Type()
		if !txnType.IsSystem() {
			c.logger.Info(msg, "type", txnType, "tx", logging.AsHex(env.Transaction.GetHash()).Slice(0, 4))
		}
	}
}
