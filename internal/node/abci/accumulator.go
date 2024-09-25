// Copyright 2024 The Accumulate Authors
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
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"sync/atomic"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cometbft/cometbft/crypto"
	"github.com/cometbft/cometbft/libs/log"
	"github.com/cometbft/cometbft/node"
	protocrypto "github.com/cometbft/cometbft/proto/tendermint/crypto"
	"github.com/cometbft/cometbft/version"
	"gitlab.com/accumulatenetwork/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	snap2 "gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
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

	snapshots      snapshotManager
	block          execute.Block
	blockState     execute.BlockState
	blockSpan      trace.Span
	txct           int64
	timer          time.Time
	didPanic       bool
	lastSnapshot   uint64
	pendingUpdates abci.ValidatorUpdates
	startTime      time.Time
	ready          bool

	onFatal func(error)

	// DisableLateCommit - DO NOT USE IN PRODUCTION - disables the late-commit
	// logic that prevents consensus failures from being committed.
	DisableLateCommit bool
}

type AccumulatorOptions struct {
	ID                   string // For debugging
	Tracer               trace.Tracer
	Executor             execute.Executor
	EventBus             *events.Bus
	Logger               log.Logger
	Snapshots            *config.Snapshots
	Database             coredb.Beginner
	Address              crypto.Address // This is the address of this node, and is used to determine if the node is the leader
	Genesis              node.GenesisDocProvider
	Partition            string
	RootDir              string
	AnalysisLog          config.AnalysisLog
	MaxEnvelopesPerBlock int
}

// NewAccumulator returns a new Accumulator.
func NewAccumulator(opts AccumulatorOptions) *Accumulator {
	app := &Accumulator{
		AccumulatorOptions: opts,
		logger:             opts.Logger.With("module", "accumulate", "partition", opts.Partition),
	}

	if app.Tracer == nil {
		app.Tracer = otel.Tracer("abci")
	}

	events.SubscribeSync(opts.EventBus, app.willChangeGlobals)

	if app.AnalysisLog.Enabled {
		app.AnalysisLog.Init(app.RootDir, app.Partition)
	}

	events.SubscribeAsync(opts.EventBus, func(e events.DidSaveSnapshot) {
		atomic.StoreUint64(&app.lastSnapshot, e.MinorIndex)
	})

	app.logger.Info("Starting ABCI application", "accumulate", accumulate.Version, "abci", Version)
	return app
}

var _ abci.Application = (*Accumulator)(nil)

func (app *Accumulator) CurrentBlock() execute.Block           { return app.block }
func (app *Accumulator) CurrentBlockState() execute.BlockState { return app.blockState }

func (app *Accumulator) LastBlock() (*execute.BlockParams, [32]byte, error) {
	return app.Executor.LastBlock()
}

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
func (app *Accumulator) recover() {
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
}

// willChangeGlobals is called when the global values are about to change.
// willChangeGlobals populates the validator update list, which is passed to
// Tendermint to update the validator set.
func (app *Accumulator) willChangeGlobals(e events.WillChangeGlobals) error {
	// Don't do anything when the globals are first loaded
	if e.Old == nil {
		return nil
	}

	// Compare the old and new partition definitions
	updates, err := core.DiffValidators(e.Old, e.New, app.Partition)
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

// Info implements github.com/cometbft/cometbft/abci/types.Application.
func (app *Accumulator) Info(context.Context, *abci.RequestInfo) (*abci.ResponseInfo, error) {
	defer app.recover()

	if app.AnalysisLog.Enabled {
		app.AnalysisLog.InitDataSet("accumulator", logging.DefaultOptions())
		app.AnalysisLog.InitDataSet("executor", logging.DefaultOptions())
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
		Panicked        bool
	}{
		Version:  accumulate.Version,
		Commit:   accumulate.Commit,
		Panicked: app.didPanic,
	})
	if err != nil {
		app.logger.Error("Failed to marshal ABCI info", "error", err)
	}

	res := &abci.ResponseInfo{
		Data:       string(data),
		Version:    version.ABCIVersion,
		AppVersion: Version,
	}

	block, hash, err := app.Executor.LastBlock()
	switch {
	case err == nil:
		res.LastBlockHeight = int64(block.Index)
		res.LastBlockAppHash = hash[:]

	case errors.Is(err, errors.NotFound):
		return res, nil

	default:
		return nil, err
	}

	if app.ready {
		return res, nil
	}

	// Check the genesis document
	genDoc, err := app.Genesis()
	if err != nil {
		return nil, err
	}

	// This field is the height of the first block after genesis, so
	// decrementing it makes it the height of genesis
	genDoc.InitialHeight -= 1

	if genDoc.InitialHeight < res.LastBlockHeight {
		app.ready = true
		return res, nil
	}
	if genDoc.InitialHeight > res.LastBlockHeight {
		return nil, errors.FatalError.With("database state is older than genesis")
	}

	if !bytes.Equal(genDoc.AppHash, res.LastBlockAppHash) {
		return nil, errors.FatalError.With("database state does not match genesis")
	}

	// If we're at genesis but the database is already populated, pretend like
	// the database is empty to make Tendermint happy
	res.LastBlockAppHash = nil
	res.LastBlockHeight = 0
	return res, nil
}

// Query implements github.com/cometbft/cometbft/abci/types.Application.
//
// Exposed as Tendermint RPC /abci_query.
func (app *Accumulator) Query(_ context.Context, reqQuery *abci.RequestQuery) (*abci.ResponseQuery, error) {
	switch reqQuery.Path {
	case "/up":
		return &abci.ResponseQuery{Code: uint32(protocol.ErrorCodeOK), Info: "Up"}, nil
	}

	return &abci.ResponseQuery{Code: uint32(protocol.ErrorCodeFailed)}, nil
}

// InitChain implements github.com/cometbft/cometbft/abci/types.Application.
//
// Called when a chain is created.
func (app *Accumulator) InitChain(_ context.Context, req *abci.RequestInitChain) (*abci.ResponseInitChain, error) {
	// Check if initialization is required
	_, root, err := app.Executor.LastBlock()
	switch {
	case err == nil:
		app.ready = true
		return &abci.ResponseInitChain{AppHash: root[:]}, nil
	case errors.Is(err, errors.NotFound):
		// Ok
	default:
		return nil, fmt.Errorf("failed to load state hash: %v", err)
	}

	app.logger.Info("Initializing")

	// Initialize the database
	var snap []byte
	err = json.Unmarshal(req.AppStateBytes, &snap)
	if err != nil {
		return nil, fmt.Errorf("failed to init chain: %+v", err)
	}
	err = snapshot.FullRestore(app.Database, ioutil.NewBuffer(snap), app.logger, config.NetworkUrl{
		URL: protocol.PartitionUrl(app.Partition),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to init chain: %+v", err)
	}

	var initVal []*execute.ValidatorUpdate
	for _, v := range req.Validators {
		if v.PubKey.GetEd25519() == nil {
			return nil, fmt.Errorf("unsupported validator key type")
		}
		initVal = append(initVal, &execute.ValidatorUpdate{
			Type:      protocol.SignatureTypeED25519,
			PublicKey: v.PubKey.GetEd25519(),
			Power:     v.Power,
		})
	}

	additional, err := app.Executor.Init(initVal)
	if err != nil {
		return nil, fmt.Errorf("failed to init chain: %+v", err)
	}

	// Notify the world of the committed block
	err = app.EventBus.Publish(events.DidCommitBlock{
		Init:  true,
		Index: protocol.GenesisBlock,
		Time:  req.Time,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to publish block notification: %v", err)
	}

	var updates []abci.ValidatorUpdate
	for _, key := range additional {
		updates = append(updates, abci.ValidatorUpdate{
			PubKey: protocrypto.PublicKey{Sum: &protocrypto.PublicKey_Ed25519{Ed25519: key.PublicKey}},
			Power:  1,
		})
	}

	// Get the app state hash
	_, root, err = app.Executor.LastBlock()
	if err != nil {
		return nil, fmt.Errorf("failed to load state hash: %v", err)
	}

	app.ready = true
	return &abci.ResponseInitChain{AppHash: root[:], Validators: updates}, nil
}

func (app *Accumulator) PrepareProposal(ctx context.Context, req *abci.RequestPrepareProposal) (*abci.ResponsePrepareProposal, error) {
	if app.MaxEnvelopesPerBlock > 0 && len(req.Txs) > app.MaxEnvelopesPerBlock {
		req.Txs = req.Txs[:app.MaxEnvelopesPerBlock]
	}

	// Cloned from BaseApplication
	txs := make([][]byte, 0, len(req.Txs))
	var totalBytes int64
	for _, tx := range req.Txs {
		totalBytes += int64(len(tx))
		if totalBytes > req.MaxTxBytes {
			break
		}
		txs = append(txs, tx)
	}
	return &abci.ResponsePrepareProposal{Txs: txs}, nil
}

func (app *Accumulator) ProcessProposal(ctx context.Context, req *abci.RequestProcessProposal) (*abci.ResponseProcessProposal, error) {
	return &abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_ACCEPT}, nil
}

func (app *Accumulator) FinalizeBlock(_ context.Context, req *abci.RequestFinalizeBlock) (*abci.ResponseFinalizeBlock, error) {
	defer app.recover()

	// Is the node borked?
	if app.didPanic {
		return nil, errors.FatalError.With("panicked")
	}

	// Commit the previous block
	if app.block != nil && !app.DisableLateCommit {
		err := app.actualCommit()
		if err != nil {
			return nil, err
		}
	}

	// Begin Block
	err := app.beginBlock(RequestBeginBlock{
		Header:              req,
		LastCommitInfo:      req.DecidedLastCommit,
		ByzantineValidators: req.Misbehavior,
	})
	if err != nil {
		return nil, err
	}

	// Deliver Tx
	res := new(abci.ResponseFinalizeBlock)
	for _, tx := range req.Txs {
		r := app.deliverTx(tx)
		res.TxResults = append(res.TxResults, &r)
	}

	// End Block
	end, err := app.endBlock()
	if err != nil {
		return nil, err
	}
	res.ValidatorUpdates = end.ValidatorUpdates

	// Get the new root
	root, err := app.blockState.Hash()
	if err != nil {
		return nil, err
	}
	res.AppHash = root[:]

	// Collect the changeset
	if false && !app.blockState.IsEmpty() {
		app.collectBlock(req.Height, root)
	}

	return res, nil
}

type RequestBeginBlock struct {
	Header              *abci.RequestFinalizeBlock
	LastCommitInfo      abci.CommitInfo
	ByzantineValidators []abci.Misbehavior
}

func (app *Accumulator) beginBlock(req RequestBeginBlock) error {
	ctx, span := app.Tracer.Start(context.Background(), "Block")
	span.SetAttributes(attribute.Int64("height", req.Header.Height))
	app.blockSpan = span

	_, span = app.Tracer.Start(ctx, "BeginBlock")
	defer span.End()
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
		return err
	}

	app.timer = time.Now()

	app.txct = 0
	return nil
}

// CheckTx implements github.com/cometbft/cometbft/abci/types.Application.
//
// Verifies the transaction is sane.
func (app *Accumulator) CheckTx(_ context.Context, req *abci.RequestCheckTx) (rct *abci.ResponseCheckTx, err error) {
	defer app.recover()

	_, span := app.Tracer.Start(context.Background(), "CheckTx")
	defer span.End()

	// Is the node borked?
	if app.didPanic {
		return nil, errors.FatalError.With("panicked")
	}

	// For some reason, if an uninitialized node is configured to sync to a
	// snapshot, Tendermint may call CheckTx before it calls InitChain or
	// ApplySnapshot
	if !app.ready {
		return nil, errors.NotReady.With("not ready")
	}

	messages, results, respData, err := executeTransactions(app.logger.With("operation", "CheckTx"), func(envelope *messaging.Envelope) ([]*protocol.TransactionStatus, error) {
		return app.Executor.Validate(envelope, req.Type == abci.CheckTxType_Recheck)
	}, req.Tx)
	if err != nil {
		b, _ := errors.UnknownError.Wrap(err).(*errors.Error).MarshalJSON()
		var res abci.ResponseCheckTx
		res.Info = string(b)
		res.Log = string(b)
		res.Code = uint32(protocol.ErrorCodeFailed)
		return &res, nil
	}

	var resp abci.ResponseCheckTx
	resp.Data = respData

	// const maxPriority = (1 << 32) - 1

	txns := map[[32]byte]*protocol.Transaction{}
	user := map[[32]byte]messaging.Message{}
	seq := map[[32]byte]uint64{}
	for _, msg := range messages {
		switch msg := msg.(type) {
		case *messaging.TransactionMessage:
			if msg.Transaction.Body.Type().IsUser() {
				user[msg.Hash()] = msg
			}
			txns[msg.Hash()] = msg.Transaction
		case *messaging.SignatureMessage:
			if !msg.Signature.Type().IsSystem() {
				user[msg.Hash()] = msg
			}
			sig, ok := msg.Signature.(*protocol.PartitionSignature)
			if !ok {
				continue
			}
			seq[sig.TransactionHash] = sig.SequenceNumber
		}
	}

	// TODO Prioritization must be done with ABCI++

	// var priority int64
	// if txn.Body.Type().IsSystem() {
	// 	priority = maxPriority
	// } else if txn.Body.Type().IsSynthetic() {
	// 	// Set the priority based on the sequence number to try to keep them in order
	// 	seq := seq[result.TxID.Hash()]
	// 	priority = maxPriority - 1 - int64(seq)
	// }
	// if resp.Priority < priority {
	// 	resp.Priority = priority
	// }

	// When checking an initial submission (as opposed to a gossiped
	// envelope), fail if any user message fails
	for i, result := range results {
		_, ok := user[result.TxID.Hash()]
		if !ok {
			continue
		}

		if result.Error == nil {
			continue
		}
		resp.Code = uint32(protocol.ErrorCodeUnknownError)
		resp.Log += fmt.Sprintf("envelope(%d/%s) %v;", i, result.Code.String(), result.Error)
	}

	// If a transaction body is 64 bytes, the batch fails
	if resp.Code == abci.CodeTypeOK {
		for i, txn := range txns {
			if txn.BodyIs64Bytes() {
				resp.Code = 1
				resp.Log += fmt.Sprintf("envelope(%d) transaction has 64 byte body;", i)
			}
		}
	}

	return &resp, nil
}

// DeliverTx implements github.com/cometbft/cometbft/abci/types.Application.
//
// Verifies the transaction is valid.
func (app *Accumulator) deliverTx(tx []byte) (rdt abci.ExecTxResult) {
	_, span := app.Tracer.Start(app.block.Params().Context, "DeliverTx")
	defer span.End()

	envelopes, _, respData, err := executeTransactions(app.logger.With("operation", "DeliverTx"), app.block.Process, tx)
	if err != nil {
		b, _ := errors.UnknownError.Wrap(err).(*errors.Error).MarshalJSON()
		var res abci.ExecTxResult
		res.Info = string(b)
		res.Log = string(b)
		res.Code = uint32(protocol.ErrorCodeFailed)
		return res
	}

	// Deliver never fails, unless the batch cannot be decoded
	app.txct += int64(len(envelopes))
	return abci.ExecTxResult{Code: uint32(protocol.ErrorCodeOK), Data: respData}
}

type ResponseEndBlock struct {
	ValidatorUpdates []abci.ValidatorUpdate
}

// EndBlock implements github.com/cometbft/cometbft/abci/types.Application.
func (app *Accumulator) endBlock() (ResponseEndBlock, error) {
	defer app.recover()

	_, span := app.Tracer.Start(app.block.Params().Context, "EndBlock")
	defer span.End()

	var err error
	app.blockState, err = app.block.Close()
	if err != nil {
		return ResponseEndBlock{}, err
	}

	if app.blockState.IsEmpty() {
		return ResponseEndBlock{}, nil
	}

	var resp ResponseEndBlock
	resp.ValidatorUpdates = app.pendingUpdates
	app.pendingUpdates = nil
	return resp, nil
}

// Commit implements github.com/cometbft/cometbft/abci/types.Application.
//
// Commits the transaction block to the chains.
func (app *Accumulator) Commit(_ context.Context, req *abci.RequestCommit) (*abci.ResponseCommit, error) {
	// // Keep this disabled until we have real snapshot support through Tendermint
	// if false {
	// 	// Truncate Tendermint's block store to the latest snapshot
	// 	resp.RetainHeight = int64(app.lastSnapshot)
	// }

	// COMMIT DOES NOT COMMIT TO DISK.
	//
	// That is deferred until the next BeginBlock, in order to ensure that
	// nothing is written to disk if there is a consensus failure.
	//
	// If the block is non-empty, we simply return the root hash and let
	// BeginBlock handle the actual commit.
	if !app.DisableLateCommit || app.block == nil {
		return &abci.ResponseCommit{}, nil
	}

	// TESTING ONLY - commit during commit, so that tests can observe state
	// changes at the expected time
	err := app.actualCommit()
	return &abci.ResponseCommit{}, err
}

func (app *Accumulator) cleanupBlock() {
	defer func() { app.block, app.blockState = nil, nil }()
	defer app.blockSpan.End()
}

func (app *Accumulator) actualCommit() error {
	defer app.cleanupBlock()

	_, span := app.Tracer.Start(app.block.Params().Context, "Commit")
	defer span.End()

	// Commit the batch. If the block is empty, the executor will discard it
	// instead.
	tick := time.Now()
	err := app.blockState.Commit()
	commitTime := time.Since(tick).Seconds()
	if err != nil {
		return err
	}

	// Notify the world of the committed block
	tick = time.Now()
	major, _, _ := app.blockState.DidCompleteMajorBlock()
	if !app.blockState.IsEmpty() {
		err = app.EventBus.Publish(events.DidCommitBlock{
			Index: app.block.Params().Index,
			Time:  app.block.Params().Time,
			Major: major,
		})
	}
	publishEventTime := time.Since(tick).Seconds()
	if err != nil {
		return err
	}

	app.collectBlockStats(commitTime, publishEventTime)

	duration := time.Since(app.timer)
	if app.blockState.IsEmpty() {
		app.logger.Debug("Committed empty block", "duration", duration.String())
	} else {
		app.logger.Debug("Committed", "minor", app.block.Params().Index, "major", major, "duration", duration, "count", app.txct)
	}
	return nil
}

func (app *Accumulator) collectBlock(height int64, rootHash [32]byte) {
	dir := filepath.Join(app.RootDir, "blocks")
	err := os.MkdirAll(dir, 0700)
	if err != nil {
		panic(err)
	}

	f, err := os.Create(filepath.Join(dir, fmt.Sprintf("%d.block", height)))
	if err != nil {
		panic(err)
	}
	defer f.Close()

	s, err := snap2.Create(f)
	if err != nil {
		panic(err)
	}

	err = s.WriteHeader(&snap2.Header{
		Version:  snap2.Version2,
		RootHash: rootHash,
	})
	if err != nil {
		panic(err)
	}

	c, err := s.OpenRecords()
	if err != nil {
		panic(err)
	}
	defer c.Close()

	err = c.Collect(app.blockState.ChangeSet(), snap2.CollectOptions{
		Walk: database.WalkOptions{
			Values:   true,
			Modified: true,
		},
	})
	if err != nil {
		panic(err)
	}
}

func (app *Accumulator) collectBlockStats(commitTime, publishEventTime float64) {
	timeSinceAppStart := time.Since(app.startTime).Seconds()
	ds := app.AnalysisLog.GetDataSet("accumulator")
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

	ds = app.AnalysisLog.GetDataSet("executor")
	if ds != nil {
		ds.Save("height", app.block.Params().Index, 10, true)
		ds.Save("time_since_app_start", timeSinceAppStart, 6, false)
		app.Executor.StoreBlockTimers(ds)
	}

	go app.AnalysisLog.Flush()
}
