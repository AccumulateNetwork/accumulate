package abci

import (
	"bytes"
	_ "crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"runtime/debug"
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
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	_ "gitlab.com/accumulatenetwork/accumulate/smt/pmt"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	apiQuery "gitlab.com/accumulatenetwork/accumulate/types/api/query"
)

// Accumulator is an ABCI application that accumulates validated transactions in
// a hash tree.
type Accumulator struct {
	abci.BaseApplication
	AccumulatorOptions
	logger log.Logger

	block    *block.Block
	txct     int64
	timer    time.Time
	didPanic bool

	onFatal func(error)
}

type AccumulatorOptions struct {
	Executor *block.Executor
	DB       *database.Database
	Logger   log.Logger
	Network  config.Network
	Address  crypto.Address // This is the address of this node, and is used to determine if the node is the leader
}

// NewAccumulator returns a new Accumulator.
func NewAccumulator(opts AccumulatorOptions) *Accumulator {
	app := &Accumulator{
		AccumulatorOptions: opts,
		logger:             opts.Logger.With("module", "accumulate", "subnet", opts.Network.LocalSubnetID),
	}

	if app.Executor == nil {
		panic("Chain Validator Node not set!")
	}

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

// Info implements github.com/tendermint/tendermint/abci/types.Application.
func (app *Accumulator) Info(req abci.RequestInfo) abci.ResponseInfo {
	defer app.recover(nil, false)

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
	var ledger *protocol.InternalLedger
	err = batch.Account(app.Network.NodeUrl(protocol.Ledger)).GetStateAs(&ledger)
	switch {
	case err == nil:
		height = ledger.Index
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
	qu := new(apiQuery.Query)
	err := qu.UnmarshalBinary(reqQuery.Data)
	if err != nil {
		// sentry.CaptureException(err)
		app.logger.Debug("Query failed", "error", err)
		resQuery.Info = "request is not an Accumulate Query"
		resQuery.Code = uint32(protocol.ErrorCodeEncodingError)
		return resQuery
	}

	batch := app.DB.Begin(false)
	defer batch.Discard()

	k, v, customErr := app.Executor.Query(batch, qu, reqQuery.Height, reqQuery.Prove)
	switch {
	case customErr == nil:
		//Ok

	case errors.Is(customErr, storage.ErrNotFound):
		resQuery.Info = customErr.Error()
		resQuery.Code = uint32(protocol.ErrorCodeNotFound)
		return resQuery

	default:
		sentry.CaptureException(customErr)
		app.logger.Debug("Query failed", "type", qu.Type.Name(), "error", customErr)
		resQuery.Info = customErr.Error()
		resQuery.Code = uint32(customErr.Code)
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
	err = app.Executor.InitFromGenesis(block.Batch, req.AppStateBytes)
	if err != nil {
		panic(fmt.Errorf("failed to init chain: %v", err))
	}

	// Execute the batch
	err = block.Batch.Commit()
	if err != nil {
		panic(fmt.Errorf("failed to commit block: %v", err))
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
	app.block.Index = req.Header.Height
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

	/*
		app.ValUpdates = make([]types.ValidatorUpdate, 0)

		// Punish validators who committed equivocation.
		for _, ev := range req.ByzantineValidators {
			if ev.Type == types.EvidenceType_DUPLICATE_VOTE {
				addr := string(ev.Validator.Address)
				if pubKey, ok := app.valAddrToPubKeyMap[addr]; ok {
					app.updateValidator(types.ValidatorUpdate{
						PubKey: pubKey,
						Power:  ev.Validator.Power - 1,
					})
					app.logger.Info("Decreased val power by 1 because of the equivocation",
						"val", addr)
				} else {
					app.logger.Error("Wanted to punish val, but can't find it",
						"val", addr)
				}
			}
		}

	*/

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

	envelopes, results, respData, err := executeTransactions(app.logger.With("operation", "CheckTx"), checkTx(app.Executor, app.DB), req.Tx)
	if err != nil {
		return abci.ResponseCheckTx{
			Code: uint32(err.Code),
			Log:  err.Error(),
		}
	}

	var resp abci.ResponseCheckTx
	resp.Data = respData

	// If a user transaction fails, the batch fails
	for i, result := range results {
		if result.Code == 0 {
			continue
		}
		if !envelopes[i].Transaction.Type().IsUser() {
			continue
		}
		resp.Code = uint32(protocol.ErrorCodeUnknownError)
		resp.Log = "One or more user transactions failed"
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
		return abci.ResponseDeliverTx{
			Code: uint32(err.Code),
			Log:  err.Error(),
		}
	}

	// Deliver never fails, unless the batch cannot be decoded
	app.txct += int64(len(envelopes))
	return abci.ResponseDeliverTx{Code: uint32(protocol.ErrorCodeOK), Data: respData}
}

// EndBlock implements github.com/tendermint/tendermint/abci/types.Application.
func (app *Accumulator) EndBlock(req abci.RequestEndBlock) abci.ResponseEndBlock {
	defer app.recover(nil, true)

	if app.block.State.Empty() {
		return abci.ResponseEndBlock{}
	}

	err := app.Executor.EndBlock(app.block)
	if err != nil {
		app.fatal(err, true)
		return abci.ResponseEndBlock{}
	}

	var resp abci.ResponseEndBlock
	resp.ValidatorUpdates = getValidatorUpdates(app.block.State.ValidatorsUpdates)
	return resp
}

// getValidatorUpdates adapts the Accumulate ValidatorUpdate struct array to the Tendermint ValidatorUpdate
func getValidatorUpdates(updates []chain.ValidatorUpdate) []abci.ValidatorUpdate {
	validatorUpdates := make([]abci.ValidatorUpdate, len(updates))
	for i, u := range updates {
		var pwr int64
		if u.Enabled {
			pwr = 1
		}

		validatorUpdates[i] = abci.ValidatorUpdate{
			PubKey: protocrypto.PublicKey{
				Sum: &protocrypto.PublicKey_Ed25519{
					Ed25519: u.PubKey,
				},
			},
			Power: pwr,
		}
	}
	return validatorUpdates
}

// Commit implements github.com/tendermint/tendermint/abci/types.Application.
//
// Commits the transaction block to the chains.
func (app *Accumulator) Commit() abci.ResponseCommit {
	defer app.recover(nil, true)
	defer func() { app.block = nil }()

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

	// Execute the batch
	err := app.block.Batch.Commit()
	if err != nil {
		app.fatal(err, true)
		return abci.ResponseCommit{}
	}

	// Notify the executor that we comitted
	batch := app.DB.Begin(false)
	defer batch.Discard()

	//this will truncate what tendermint stores since we only care about current state
	//todo: uncomment the next line when we have smt state syncing complete. For now, we are retaining everything for test net
	// if app.RetainBlocks > 0 && app.Height >= app.RetainBlocks {
	// 	resp.RetainHeight = app.Height - app.RetainBlocks + 1
	// }

	duration := time.Since(app.timer)
	app.logger.Debug("Committed", "transactions", app.txct, "duration", duration.String(), "tps", float64(app.txct)/duration.Seconds())
	return abci.ResponseCommit{Data: batch.BptRoot()}
}

// ListSnapshots implements github.com/tendermint/tendermint/abci/types.Application.
func (app *Accumulator) ListSnapshots(
	req abci.RequestListSnapshots) abci.ResponseListSnapshots {

	req.ProtoMessage()
	return abci.ResponseListSnapshots{}
}

// OfferSnapshot implements github.com/tendermint/tendermint/abci/types.Application.
func (app *Accumulator) OfferSnapshot(
	req abci.RequestOfferSnapshot) abci.ResponseOfferSnapshot {
	return abci.ResponseOfferSnapshot{Result: abci.ResponseOfferSnapshot_ABORT}
}

// LoadSnapshotChunk implements github.com/tendermint/tendermint/abci/types.Application.
func (app *Accumulator) LoadSnapshotChunk(
	req abci.RequestLoadSnapshotChunk) abci.ResponseLoadSnapshotChunk {

	//req.Height
	//resp := abcitypes.ResponseLoadSnapshotChunk{}
	//need to get a block of data between markers.
	//resp.Chunk = app.mm.GetState(req.Height)
	return abci.ResponseLoadSnapshotChunk{}
}

// ApplySnapshotChunk implements github.com/tendermint/tendermint/abci/types.Application.
func (app *Accumulator) ApplySnapshotChunk(
	req abci.RequestApplySnapshotChunk) abci.ResponseApplySnapshotChunk {
	return abci.ResponseApplySnapshotChunk{Result: abci.ResponseApplySnapshotChunk_ABORT}
}
