package abci

import (
	"bytes"
	_ "crypto/sha256"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getsentry/sentry-go"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/crypto"
	"github.com/tendermint/tendermint/libs/log"
	protocrypto "github.com/tendermint/tendermint/proto/tendermint/crypto"
	"github.com/tendermint/tendermint/version"
	"gitlab.com/accumulatenetwork/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/events"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	_ "gitlab.com/accumulatenetwork/accumulate/smt/pmt"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/types/api/query"
)

// Accumulator is an ABCI application that accumulates validated transactions in
// a hash tree.
type Accumulator struct {
	abci.BaseApplication
	AccumulatorOptions
	logger log.Logger

	block          *block.Block
	txct           int64
	timer          time.Time
	didPanic       bool
	lastSnapshot   uint64
	checkTxBatch   *database.Batch
	checkTxMutex   *sync.Mutex
	pendingUpdates abci.ValidatorUpdates
	startTime      time.Time

	onFatal func(error)
}

type AccumulatorOptions struct {
	*config.Config
	Executor *block.Executor
	EventBus *events.Bus
	DB       *database.Database
	Logger   log.Logger
	Address  crypto.Address // This is the address of this node, and is used to determine if the node is the leader
}

// NewAccumulator returns a new Accumulator.
func NewAccumulator(opts AccumulatorOptions) *Accumulator {
	app := &Accumulator{
		AccumulatorOptions: opts,
		logger:             opts.Logger.With("module", "accumulate", "partition", opts.Accumulate.PartitionId),
		checkTxMutex:       &sync.Mutex{},
	}

	events.SubscribeSync(opts.EventBus, app.willChangeGlobals)

	app.Accumulate.AnalysisLog.Init(app.RootDir, app.Accumulate.PartitionId)

	events.SubscribeAsync(opts.EventBus, func(e events.DidSaveSnapshot) {
		atomic.StoreUint64(&app.lastSnapshot, e.MinorIndex)
	})

	app.logger.Info("Starting ABCI application", "accumulate", accumulate.Version, "abci", Version)
	return app
}

var _ abci.Application = (*Accumulator)(nil)

// FOR TESTING ONLY
func (app *Accumulator) OnFatal(f func(error)) {
	app.onFatal = f
}

// fatal is called when a fatal error occurs. If fatal is called, all subsequent
// transactions will fail with CodeDidPanic.
func (app *Accumulator) fatal(err error, setDidPanic bool) {
	if setDidPanic {
		app.didPanic = true
	}

	app.logger.Error("Fatal error", "error", err, "stack", debug.Stack())
	sentry.CaptureException(err)

	if app.onFatal != nil {
		app.onFatal(err)
	}

	// Throw the panic back at Tendermint
	panic(err)
}

// recover will recover from a panic. If a panic occurs, it is passed to fatal
// and code is set to CodeDidPanic (unless the pointer is nil).
func (app *Accumulator) recover(code *uint32, setDidPanic bool) {
	r := recover()
	if r == nil {
		return
	}

	err, ok := r.(error)
	if ok {
		err = fmt.Errorf("panicked: %w", err)
	} else {
		err = fmt.Errorf("panicked: %v", r)
	}
	app.fatal(err, setDidPanic)

	if code != nil {
		*code = uint32(protocol.ErrorCodeDidPanic)
	}
}

// willChangeGlobals is called when the global values are about to change.
// willChangeGlobals populates the validator update list, which is passed to
// Tendermint to update the validator set.
func (app *Accumulator) willChangeGlobals(e events.WillChangeGlobals) error {
	// Compare the old and new partition definitions
	updates, err := e.Old.DiffValidators(e.New, app.Accumulate.PartitionId)
	if err != nil {
		return err
	}

	// Convert the update list into Tendermint validator updates
	for key, typ := range updates {
		key := key // See docs/developer/rangevarref.md
		vu := abci.ValidatorUpdate{
			PubKey: protocrypto.PublicKey{
				Sum: &protocrypto.PublicKey_Ed25519{
					Ed25519: key[:],
				},
			},
		}
		switch typ {
		case core.ValidatorUpdateAdd:
			vu.Power = 1
		case core.ValidatorUpdateRemove:
			vu.Power = 0
		default:
			continue
		}
		app.pendingUpdates = append(app.pendingUpdates, vu)
	}

	// Sort the list so we're deterministic
	sort.Slice(app.pendingUpdates, func(i, j int) bool {
		a := app.pendingUpdates[i].PubKey.GetEd25519()
		b := app.pendingUpdates[j].PubKey.GetEd25519()
		return bytes.Compare(a, b) < 0
	})

	return nil
}

// Info implements github.com/tendermint/tendermint/abci/types.Application.
func (app *Accumulator) Info(req abci.RequestInfo) abci.ResponseInfo {
	defer app.recover(nil, false)

	if app.Accumulate.AnalysisLog.Enabled {
		app.Accumulate.AnalysisLog.InitDataSet("accumulator", logging.DefaultOptions())
		app.Accumulate.AnalysisLog.InitDataSet("executor", logging.DefaultOptions())
		app.Executor.EnableTimers()
	}

	app.startTime = time.Now()

	//todo: load up the merkle databases to the same state we're at...  We will need to rewind.

	// We have two different versions: that of the ABCI application, and that of
	// the executable. The ABCI application version may affect Tendermint. The
	// executable version tells us what commit to look at when debugging a crash
	// log.

	data, err := json.Marshal(struct {
		Version, Commit string
	}{
		Version: accumulate.Version,
		Commit:  accumulate.Commit,
	})
	if err != nil {
		sentry.CaptureException(err)
	}

	batch := app.DB.Begin(false)
	defer batch.Discard()

	var height int64
	var ledger *protocol.SystemLedger
	err = batch.Account(app.Accumulate.Describe.NodeUrl(protocol.Ledger)).GetStateAs(&ledger)
	switch {
	case err == nil:
		height = int64(ledger.Index)
	case errors.Is(err, storage.ErrNotFound):
		// InitChain has not been called yet
		height = 0
	default:
		height = -1
		sentry.CaptureException(err)
	}

	return abci.ResponseInfo{
		Data:             string(data),
		Version:          version.ABCIVersion,
		AppVersion:       Version,
		LastBlockHeight:  height,
		LastBlockAppHash: batch.BptRoot(),
	}
}

// Query implements github.com/tendermint/tendermint/abci/types.Application.
//
// Exposed as Tendermint RPC /abci_query.
func (app *Accumulator) Query(reqQuery abci.RequestQuery) (resQuery abci.ResponseQuery) {
	defer app.recover(&resQuery.Code, false)

	if app.didPanic {
		return abci.ResponseQuery{
			Code: uint32(protocol.ErrorCodeDidPanic),
			Info: "Node state is invalid",
		}
	}

	resQuery.Key = reqQuery.Data
	qu, err := query.UnmarshalRequest(reqQuery.Data)
	if err != nil {
		// sentry.CaptureException(err)
		app.logger.Debug("Query failed", "error", err)
		resQuery.Info = "request is not an Accumulate Query"
		resQuery.Code = uint32(protocol.ErrorCodeEncodingError)
		return resQuery
	}

	batch := app.DB.Begin(false)
	defer batch.Discard()

	k, v, err := app.Executor.Query(batch, qu, reqQuery.Height, reqQuery.Prove)
	if err != nil {
		b, _ := errors.Wrap(errors.StatusUnknownError, err).(*errors.Error).MarshalJSON()
		resQuery.Info = string(b)
		resQuery.Code = uint32(protocol.ErrorCodeFailed)
		return resQuery
	}

	//if we get here, we have a valid state object, so let's return it.
	resQuery.Code = uint32(protocol.ErrorCodeOK)
	//return a generic state object for the chain and let the query deal with decoding it
	resQuery.Key, resQuery.Value = k, v

	///implement lazy sync calls. If a node falls behind it needs to have several query calls
	///1 get current height
	///2 get block data for height X
	///3 get block data for given hash
	return
}

// InitChain implements github.com/tendermint/tendermint/abci/types.Application.
//
// Called when a chain is created.
func (app *Accumulator) InitChain(req abci.RequestInitChain) abci.ResponseInitChain {
	// Check if initialization is required
	var root []byte
	err := app.DB.View(func(batch *database.Batch) (err error) {
		root, err = app.Executor.LoadStateRoot(batch)
		return err
	})
	if err != nil {
		panic(fmt.Errorf("failed to load state hash: %v", err))
	}
	if root != nil {
		return abci.ResponseInitChain{AppHash: root}
	}

	app.logger.Info("Initializing")
	block := new(block.Block)
	block.Index = protocol.GenesisBlock
	block.Time = req.Time
	block.IsLeader = true
	block.Batch = app.DB.Begin(true)
	defer block.Batch.Discard()

	// Initialize the chain
	var snapshot []byte
	err = json.Unmarshal(req.AppStateBytes, &snapshot)
	if err != nil {
		panic(fmt.Errorf("failed to init chain: %+v", err))
	}
	err = app.Executor.RestoreSnapshot(block.Batch, ioutil2.NewBuffer(snapshot))
	if err != nil {
		panic(fmt.Errorf("failed to init chain: %+v", err))
	}

	// Commit the batch
	err = block.Batch.Commit()
	if err != nil {
		panic(fmt.Errorf("failed to commit block: %v", err))
	}

	// Notify the world of the committed block
	err = app.EventBus.Publish(events.DidCommitBlock{
		Index: block.Index,
		Time:  block.Time,
	})
	if err != nil {
		panic(fmt.Errorf("failed to publish block notification: %v", err))
	}

	err = app.DB.View(func(batch *database.Batch) (err error) {
		root, err = app.Executor.LoadStateRoot(batch)
		return err
	})
	if err != nil {
		panic(fmt.Errorf("failed to load state hash: %v", err))
	}

	return abci.ResponseInitChain{AppHash: root}
}

// BeginBlock implements github.com/tendermint/tendermint/abci/types.Application.
func (app *Accumulator) BeginBlock(req abci.RequestBeginBlock) abci.ResponseBeginBlock {
	defer app.recover(nil, true)

	var ret abci.ResponseBeginBlock

	app.block = new(block.Block)
	app.block.IsLeader = bytes.Equal(app.Address.Bytes(), req.Header.GetProposerAddress())
	app.block.Index = uint64(req.Header.Height)
	app.block.Time = req.Header.Time
	app.block.CommitInfo = &req.LastCommitInfo
	app.block.Evidence = req.ByzantineValidators
	app.block.Batch = app.DB.Begin(true)

	//Identify the leader for this block, if we are the proposer... then we are the leader.
	err := app.Executor.BeginBlock(app.block)
	if err != nil {
		app.fatal(err, true)
		return ret
	}

	app.timer = time.Now()

	app.txct = 0

	return ret
}

// CheckTx implements github.com/tendermint/tendermint/abci/types.Application.
//
// Verifies the transaction is sane.
func (app *Accumulator) CheckTx(req abci.RequestCheckTx) (rct abci.ResponseCheckTx) {
	defer app.recover(&rct.Code, true)

	// Is the node borked?
	if app.didPanic {
		return abci.ResponseCheckTx{
			Code: uint32(protocol.ErrorCodeDidPanic),
			Log:  "Node state is invalid",
		}
	}

	// Only use the shared batch when the check type is CheckTxType_New,
	//   we want to avoid changes to variables version increments to and stick and therefore be done multiple times
	var batch *database.Batch
	switch req.Type {
	case abci.CheckTxType_New:
		// TODO I don't think we need a mutex because I think Tendermint
		// guarantees that ABCI calls are non-concurrent
		app.checkTxMutex.Lock()
		defer app.checkTxMutex.Unlock()
		if app.checkTxBatch == nil { // For cases where we haven't started/ended a block yet
			app.checkTxBatch = app.DB.Begin(false)
		}
		batch = app.checkTxBatch
	case abci.CheckTxType_Recheck:
		batch = app.DB.Begin(false)
		defer batch.Discard()
	}

	envelopes, results, respData, err := executeTransactions(app.logger.With("operation", "CheckTx"), checkTx(app.Executor, batch), req.Tx)
	if err != nil {
		b, _ := errors.Wrap(errors.StatusUnknownError, err).(*errors.Error).MarshalJSON()
		var res abci.ResponseCheckTx
		res.Info = string(b)
		res.Code = uint32(protocol.ErrorCodeFailed)
		return res
	}

	var resp abci.ResponseCheckTx
	resp.Data = respData

	// If a user transaction fails, the batch fails
	for i, result := range results {
		if typ := envelopes[i].Transaction.Body.Type(); typ.IsSystem() && resp.Priority < 2 {
			resp.Priority = 2
		} else if typ.IsSynthetic() && resp.Priority < 1 {
			resp.Priority = 1
		}
		if result.Code.Success() {
			continue
		}
		if !envelopes[i].Transaction.Body.Type().IsUser() {
			continue
		}
		resp.Code = uint32(protocol.ErrorCodeUnknownError)
		resp.Log += fmt.Sprintf("envelope(%d/%s) %v;", i, result.Code.String(), result.Error)
	}

	return resp
}

// DeliverTx implements github.com/tendermint/tendermint/abci/types.Application.
//
// Verifies the transaction is valid.
func (app *Accumulator) DeliverTx(req abci.RequestDeliverTx) (rdt abci.ResponseDeliverTx) {
	defer app.recover(&rdt.Code, true)

	// Is the node borked?
	if app.didPanic {
		return abci.ResponseDeliverTx{
			Code: uint32(protocol.ErrorCodeDidPanic),
			Info: "Node state is invalid",
		}
	}

	envelopes, _, respData, err := executeTransactions(app.logger.With("operation", "DeliverTx"), deliverTx(app.Executor, app.block), req.Tx)
	if err != nil {
		b, _ := errors.Wrap(errors.StatusUnknownError, err).(*errors.Error).MarshalJSON()
		var res abci.ResponseDeliverTx
		res.Info = string(b)
		res.Code = uint32(protocol.ErrorCodeFailed)
		return res
	}

	// Deliver never fails, unless the batch cannot be decoded
	app.txct += int64(len(envelopes))
	return abci.ResponseDeliverTx{Code: uint32(protocol.ErrorCodeOK), Data: respData}
}

// EndBlock implements github.com/tendermint/tendermint/abci/types.Application.
func (app *Accumulator) EndBlock(req abci.RequestEndBlock) abci.ResponseEndBlock {
	defer app.recover(nil, true)

	err := app.Executor.EndBlock(app.block)
	if err != nil {
		app.fatal(err, true)
		return abci.ResponseEndBlock{}
	}

	if app.block.State.Empty() {
		return abci.ResponseEndBlock{}
	}

	var resp abci.ResponseEndBlock
	resp.ValidatorUpdates = app.pendingUpdates
	app.pendingUpdates = nil
	return resp
}

// Commit implements github.com/tendermint/tendermint/abci/types.Application.
//
// Commits the transaction block to the chains.
func (app *Accumulator) Commit() abci.ResponseCommit {
	defer app.recover(nil, true)
	defer func() { app.block = nil }()

	tick := time.Now()
	// Is the block empty?
	if app.block.State.Empty() {
		// Discard changes
		app.block.Batch.Discard()

		// Get the old root
		batch := app.DB.Begin(false)
		defer batch.Discard()

		duration := time.Since(app.timer)
		app.logger.Debug("Committed empty block", "duration", duration.String())
		return abci.ResponseCommit{Data: batch.BptRoot()}
	}

	// Commit the batch
	err := app.block.Batch.Commit()

	commitTime := time.Since(tick).Seconds()
	tick = time.Now()

	if err != nil {
		app.fatal(err, true)
		return abci.ResponseCommit{}
	}

	// Notify the world of the committed block
	err = app.EventBus.Publish(events.DidCommitBlock{
		Index: app.block.Index,
		Time:  app.block.Time,
		Major: app.block.State.MakeMajorBlock,
	})
	if err != nil {
		app.fatal(err, true)
		return abci.ResponseCommit{}
	}

	publishEventTime := time.Since(tick).Seconds()

	// Replace start a new checkTx batch
	app.checkTxMutex.Lock()
	defer app.checkTxMutex.Unlock()

	if app.checkTxBatch != nil {
		app.checkTxBatch.Discard()
	}
	app.checkTxBatch = app.DB.Begin(false)

	// Notify the executor that we committed
	var resp abci.ResponseCommit
	batch := app.DB.Begin(false)
	defer batch.Discard()
	resp.Data = batch.BptRoot()

	// Keep this disabled until we have real snapshot support through Tendermint
	if false {
		// Truncate Tendermint's block store to the latest snapshot
		resp.RetainHeight = int64(app.lastSnapshot)
	}

	timeSinceAppStart := time.Since(app.startTime).Seconds()
	ds := app.Accumulate.AnalysisLog.GetDataSet("accumulator")
	if ds != nil {
		blockTime := time.Since(app.timer).Seconds()
		aveBlockTime := 0.0
		estTps := 0.0
		if app.txct != 0 {
			aveBlockTime = blockTime / float64(app.txct)
			estTps = 1.0 / aveBlockTime
		}
		ds.Save("height", app.block.Index, 10, true)
		ds.Save("time_since_app_start", timeSinceAppStart, 6, false)
		ds.Save("block_time", blockTime, 6, false)
		ds.Save("commit_time", commitTime, 6, false)
		ds.Save("event_time", publishEventTime, 6, false)
		ds.Save("ave_block_time", aveBlockTime, 10, false)
		ds.Save("est_tps", estTps, 10, false)
		ds.Save("txct", app.txct, 10, false)

	}

	ds = app.Accumulate.AnalysisLog.GetDataSet("executor")
	if ds != nil {
		ds.Save("height", app.block.Index, 10, true)
		ds.Save("time_since_app_start", timeSinceAppStart, 6, false)
		app.Executor.BlockTimers.Store(ds)
	}

	go app.Accumulate.AnalysisLog.Flush()
	app.logger.Debug("Committed", "minor", app.block.Index, "hash", logging.AsHex(batch.BptRoot()).Slice(0, 4), "major", app.block.State.MakeMajorBlock)
	return resp
}
