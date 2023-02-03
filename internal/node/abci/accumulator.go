// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package abci

import (
	"bytes"
	"context"
	_ "crypto/sha256"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/crypto"
	"github.com/tendermint/tendermint/libs/log"
	protocrypto "github.com/tendermint/tendermint/proto/tendermint/crypto"
	"github.com/tendermint/tendermint/version"
	"gitlab.com/accumulatenetwork/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	_ "gitlab.com/accumulatenetwork/accumulate/internal/database/smt/pmt"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Accumulator is an ABCI application that accumulates validated transactions in
// a hash tree.
type Accumulator struct {
	abci.BaseApplication
	AccumulatorOptions
	logger log.Logger

	block          execute.Block
	blockState     execute.BlockState
	blockSpan      trace.Span
	txct           int64
	timer          time.Time
	didPanic       bool
	lastSnapshot   uint64
	checkTxBatch   *database.Batch
	checkTxMutex   *sync.Mutex
	pendingUpdates abci.ValidatorUpdates
	startTime      time.Time
	ready          bool

	onFatal func(error)
}

type AccumulatorOptions struct {
	*config.Config
	Tracer   trace.Tracer
	Executor execute.Executor
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

	if app.Tracer == nil {
		app.Tracer = otel.Tracer("abci")
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
func (app *Accumulator) fatal(err error) {
	app.didPanic = true
	app.logger.Error("Fatal error", "error", err, "stack", debug.Stack())

	if app.onFatal != nil {
		app.onFatal(err)
	}

	_ = app.EventBus.Publish(events.FatalError{Err: err})

	// Throw the panic back at Tendermint
	panic(err)
}

// recover will recover from a panic. If a panic occurs, it is passed to fatal
// and code is set to CodeDidPanic (unless the pointer is nil).
func (app *Accumulator) recover(code *uint32) {
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
	app.fatal(err)

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
func (app *Accumulator) Info(abci.RequestInfo) abci.ResponseInfo {
	defer app.recover(nil)

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
		app.logger.Error("Failed to marshal ABCI info", "error", err)
	}

	batch := app.DB.Begin(false)
	defer batch.Discard()

	var height int64
	var ledger *protocol.SystemLedger
	err = batch.Account(app.Accumulate.Describe.NodeUrl(protocol.Ledger)).GetStateAs(&ledger)
	switch {
	case err == nil:
		height = int64(ledger.Index)
		app.ready = true
	case errors.Is(err, storage.ErrNotFound):
		// InitChain has not been called yet
		height = 0
	default:
		height = -1
		app.logger.Error("Failed to load system ledger", "error", err)
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
	switch reqQuery.Path {
	case "/up":
		return abci.ResponseQuery{Code: uint32(protocol.ErrorCodeOK), Info: "Up"}
	}

	return abci.ResponseQuery{Code: uint32(protocol.ErrorCodeFailed)}
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
		app.ready = true
		return abci.ResponseInitChain{AppHash: root}
	}

	app.logger.Info("Initializing")

	// Initialize the chain
	var snapshot []byte
	err = json.Unmarshal(req.AppStateBytes, &snapshot)
	if err != nil {
		panic(fmt.Errorf("failed to init chain: %+v", err))
	}
	err = app.Executor.RestoreSnapshot(app.DB, ioutil2.NewBuffer(snapshot))
	if err != nil {
		panic(fmt.Errorf("failed to init chain: %+v", err))
	}

	// Notify the world of the committed block
	err = app.EventBus.Publish(events.DidCommitBlock{
		Index: protocol.GenesisBlock,
		Time:  req.Time,
	})
	if err != nil {
		panic(fmt.Errorf("failed to publish block notification: %v", err))
	}

	// Compare initial validators with genesis
	additional, err := app.Executor.InitChainValidators(req.Validators)
	if err != nil {
		panic(err)
	}

	var updates []abci.ValidatorUpdate
	for _, key := range additional {
		updates = append(updates, abci.ValidatorUpdate{
			PubKey: protocrypto.PublicKey{Sum: &protocrypto.PublicKey_Ed25519{Ed25519: key}},
			Power:  1,
		})
	}

	// Get the app state hash
	err = app.DB.View(func(batch *database.Batch) (err error) {
		root, err = app.Executor.LoadStateRoot(batch)
		return err
	})
	if err != nil {
		panic(fmt.Errorf("failed to load state hash: %v", err))
	}

	app.ready = true
	return abci.ResponseInitChain{AppHash: root, Validators: updates}
}

// BeginBlock implements github.com/tendermint/tendermint/abci/types.Application.
func (app *Accumulator) BeginBlock(req abci.RequestBeginBlock) abci.ResponseBeginBlock {
	defer app.recover(nil)

	ctx, span := app.Tracer.Start(context.Background(), "Block")
	span.SetAttributes(attribute.Int64("height", req.Header.Height))
	app.blockSpan = span

	_, span = app.Tracer.Start(ctx, "BeginBlock")
	defer span.End()

	var ret abci.ResponseBeginBlock

	//Identify the leader for this block, if we are the proposer... then we are the leader.
	isLeader := bytes.Equal(app.Address.Bytes(), req.Header.GetProposerAddress())

	var err error
	app.block, err = app.Executor.Begin(execute.BlockParams{
		Context:    ctx,
		IsLeader:   isLeader,
		Index:      uint64(req.Header.Height),
		Time:       req.Header.Time,
		CommitInfo: &req.LastCommitInfo,
		Evidence:   req.ByzantineValidators,
	})
	if err != nil {
		app.fatal(err)
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
	defer app.recover(&rct.Code)

	_, span := app.Tracer.Start(context.Background(), "CheckTx")
	defer span.End()

	// Is the node borked?
	if app.didPanic {
		return abci.ResponseCheckTx{
			Code: uint32(protocol.ErrorCodeDidPanic),
			Log:  "Node state is invalid",
		}
	}

	// For some reason, if an uninitialized node is configured to sync to a
	// snapshot, Tendermint may call CheckTx before it calls InitChain or
	// ApplySnapshot
	if !app.ready {
		return abci.ResponseCheckTx{
			Code: uint32(protocol.ErrorCodeUnknownError),
			Log:  "Node is not ready",
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

	messages, results, respData, err := executeTransactions(app.logger.With("operation", "CheckTx"), func(messages []messaging.Message) ([]*protocol.TransactionStatus, error) {
		return app.Executor.Validate(batch, messages)
	}, req.Tx)
	if err != nil {
		b, _ := errors.UnknownError.Wrap(err).(*errors.Error).MarshalJSON()
		var res abci.ResponseCheckTx
		res.Info = string(b)
		res.Log = string(b)
		res.Code = uint32(protocol.ErrorCodeFailed)
		return res
	}

	var resp abci.ResponseCheckTx
	resp.Data = respData

	const maxPriority = (1 << 32) - 1

	txns := map[[32]byte]protocol.TransactionType{}
	seq := map[[32]byte]uint64{}
	for _, msg := range messages {
		switch msg := msg.(type) {
		case *messaging.UserTransaction:
			txns[*(*[32]byte)(msg.Transaction.GetHash())] = msg.Transaction.Body.Type()
		case *messaging.UserSignature:
			sig, ok := msg.Signature.(*protocol.PartitionSignature)
			if !ok {
				continue
			}
			seq[sig.TransactionHash] = sig.SequenceNumber
		}
	}

	// If a user transaction fails, the batch fails
	for i, result := range results {
		typ, ok := txns[result.TxID.Hash()]
		if !ok {
			continue
		}

		var priority int64
		if typ.IsSystem() {
			priority = maxPriority
		} else if typ.IsSynthetic() {
			// Set the priority based on the sequence number to try to keep them in order
			seq := seq[result.TxID.Hash()]
			priority = maxPriority - 1 - int64(seq)
		}
		if resp.Priority < priority {
			resp.Priority = priority
		}
		if result.Error == nil {
			continue
		}
		if !result.Code.Success() && !typ.IsUser() {
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
	defer app.recover(&rdt.Code)

	_, span := app.Tracer.Start(app.block.Params().Context, "DeliverTx")
	defer span.End()

	// Is the node borked?
	if app.didPanic {
		return abci.ResponseDeliverTx{
			Code: uint32(protocol.ErrorCodeDidPanic),
			Info: "Node state is invalid",
		}
	}

	envelopes, _, respData, err := executeTransactions(app.logger.With("operation", "DeliverTx"), app.block.Process, req.Tx)
	if err != nil {
		b, _ := errors.UnknownError.Wrap(err).(*errors.Error).MarshalJSON()
		var res abci.ResponseDeliverTx
		res.Info = string(b)
		res.Log = string(b)
		res.Code = uint32(protocol.ErrorCodeFailed)
		return res
	}

	// Deliver never fails, unless the batch cannot be decoded
	app.txct += int64(len(envelopes))
	return abci.ResponseDeliverTx{Code: uint32(protocol.ErrorCodeOK), Data: respData}
}

// EndBlock implements github.com/tendermint/tendermint/abci/types.Application.
func (app *Accumulator) EndBlock(req abci.RequestEndBlock) abci.ResponseEndBlock {
	defer app.recover(nil)

	_, span := app.Tracer.Start(app.block.Params().Context, "EndBlock")
	defer span.End()

	var err error
	app.blockState, err = app.block.Close()
	if err != nil {
		app.fatal(err)
		return abci.ResponseEndBlock{}
	}

	if app.blockState.IsEmpty() {
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
	defer app.recover(nil)
	defer func() { app.block, app.blockState = nil, nil }()
	defer app.blockSpan.End()

	_, span := app.Tracer.Start(app.block.Params().Context, "Commit")
	defer span.End()

	tick := time.Now()
	// Is the block empty?
	if app.blockState.IsEmpty() {
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
			ds.Save("height", app.block.Params().Index, 10, true)
			ds.Save("time_since_app_start", timeSinceAppStart, 6, false)
			ds.Save("block_time", blockTime, 6, false)
			ds.Save("ave_block_time", aveBlockTime, 10, false)
			ds.Save("est_tps", estTps, 10, false)
			ds.Save("txct", app.txct, 10, false)
			app.blockSpan.SetAttributes(
				attribute.Float64("time_since_app_start", timeSinceAppStart),
				attribute.Float64("block_time", blockTime),
				attribute.Float64("ave_block_time", aveBlockTime),
				attribute.Float64("est_tps", estTps),
				attribute.Int64("txct", app.txct),
			)
		}

		ds = app.Accumulate.AnalysisLog.GetDataSet("executor")
		if ds != nil {
			ds.Save("height", app.block.Params().Index, 10, true)
			ds.Save("time_since_app_start", timeSinceAppStart, 6, false)
			app.Executor.StoreBlockTimers(ds)
		}

		go app.Accumulate.AnalysisLog.Flush()

		// Discard changes
		app.blockState.Discard()

		// Get the old root
		batch := app.DB.Begin(false)
		defer batch.Discard()

		duration := time.Since(app.timer)
		app.logger.Debug("Committed empty block", "duration", duration.String())
		return abci.ResponseCommit{Data: batch.BptRoot()}
	}

	// Commit the batch
	err := app.blockState.Commit()

	commitTime := time.Since(tick).Seconds()
	tick = time.Now()

	if err != nil {
		app.fatal(err)
		return abci.ResponseCommit{}
	}

	// Notify the world of the committed block
	major, _, _ := app.blockState.DidCompleteMajorBlock()
	err = app.EventBus.Publish(events.DidCommitBlock{
		Index: app.block.Params().Index,
		Time:  app.block.Params().Time,
		Major: major,
	})
	if err != nil {
		app.fatal(err)
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
		ds.Save("height", app.block.Params().Index, 10, true)
		ds.Save("time_since_app_start", timeSinceAppStart, 6, false)
		ds.Save("block_time", blockTime, 6, false)
		ds.Save("commit_time", commitTime, 6, false)
		ds.Save("event_time", publishEventTime, 6, false)
		ds.Save("ave_block_time", aveBlockTime, 10, false)
		ds.Save("est_tps", estTps, 10, false)
		ds.Save("txct", app.txct, 10, false)
		app.blockSpan.SetAttributes(
			attribute.Float64("time_since_app_start", timeSinceAppStart),
			attribute.Float64("block_time", blockTime),
			attribute.Float64("commit_time", commitTime),
			attribute.Float64("event_time", publishEventTime),
			attribute.Float64("ave_block_time", aveBlockTime),
			attribute.Float64("est_tps", estTps),
			attribute.Int64("txct", app.txct),
		)

	}

	ds = app.Accumulate.AnalysisLog.GetDataSet("executor")
	if ds != nil {
		ds.Save("height", app.block.Params().Index, 10, true)
		ds.Save("time_since_app_start", timeSinceAppStart, 6, false)
		app.Executor.StoreBlockTimers(ds)
	}

	go app.Accumulate.AnalysisLog.Flush()
	duration := time.Since(app.timer)
	app.logger.Debug("Committed", "minor", app.block.Params().Index, "hash", logging.AsHex(batch.BptRoot()).Slice(0, 4), "major", major, "duration", duration, "count", app.txct)
	return resp
}
