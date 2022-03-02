package abci

import (
	"bytes"
	"crypto/sha256"
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
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	_ "gitlab.com/accumulatenetwork/accumulate/smt/pmt"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/types"
	apiQuery "gitlab.com/accumulatenetwork/accumulate/types/api/query"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

// Accumulator is an ABCI application that accumulates validated transactions in
// a hash tree.
type Accumulator struct {
	abci.BaseApplication
	AccumulatorOptions
	logger log.Logger

	txct     int64
	timer    time.Time
	didPanic bool

	onFatal func(error)
}

type AccumulatorOptions struct {
	Chain   Chain
	DB      *database.Database
	Logger  log.Logger
	Network config.Network
	Address crypto.Address // This is the address of this node, and is used to determine if the node is the leader
}

// NewAccumulator returns a new Accumulator.
func NewAccumulator(opts AccumulatorOptions) *Accumulator {
	app := &Accumulator{
		AccumulatorOptions: opts,
		logger:             opts.Logger.With("module", "accumulate", "subnet", opts.Network.LocalSubnetID),
	}

	if app.Chain == nil {
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
	ledger := protocol.NewInternalLedger()
	err = batch.Account(app.Network.NodeUrl(protocol.Ledger)).GetStateAs(ledger)
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
		LastBlockAppHash: batch.RootHash(),
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

	k, v, customErr := app.Chain.Query(qu, reqQuery.Height, reqQuery.Prove)
	switch {
	case customErr == nil:
		//Ok

	case errors.Is(customErr.Unwrap(), storage.ErrNotFound):
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
	app.logger.Info("Initializing")
	root, err := app.Chain.InitChain(req.AppStateBytes, req.Time, req.InitialHeight)
	if err != nil {
		panic(fmt.Errorf("failed to init chain: %v", err))
	}

	return abci.ResponseInitChain{AppHash: root}
}

// BeginBlock implements github.com/tendermint/tendermint/abci/types.Application.
func (app *Accumulator) BeginBlock(req abci.RequestBeginBlock) abci.ResponseBeginBlock {
	defer app.recover(nil, true)

	var ret abci.ResponseBeginBlock

	//Identify the leader for this block, if we are the proposer... then we are the leader.
	_, err := app.Chain.BeginBlock(BeginBlockRequest{
		IsLeader: bytes.Equal(app.Address.Bytes(), req.Header.GetProposerAddress()),
		Height:   req.Header.Height,
		Time:     req.Header.Time,
	})
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
			Info: "Node state is invalid",
		}
	}

	h := sha256.Sum256(req.Tx)
	tmHash := logging.AsHex(h[:])

	// Unmarshal all of the envelopes
	// Unmarshal all of the envelopes
	envelopes, err := transactions.UnmarshalAll(req.Tx)
	if err != nil {
		sentry.CaptureException(err)
		app.logger.Info("Check failed", "tx", tmHash, "error", err)
		return abci.ResponseCheckTx{Code: uint32(protocol.ErrorCodeEncodingError), Log: "Unable to decode transaction"}
	}

	// Check all of the transactions
	resp := abci.ResponseCheckTx{Code: uint32(protocol.ErrorCodeOK)}
	for _, env := range envelopes {
		txid := logging.AsHex(env.GetTxHash())
		result, err := app.Chain.CheckTx(env)
		if err != nil {
			sentry.CaptureException(err)
			app.logger.Info("Check failed", "type", env.Transaction.Type().String(), "txid", txid, "hash", tmHash, "error", err, "origin", env.Transaction.Origin)
			return abci.ResponseCheckTx{
				Code: uint32(err.Code),
				Log:  fmt.Sprintf("%s check of %s transaction failed: %v", env.Transaction.Origin.String(), env.Transaction.Type().String(), err),
			}
		}

		typ := env.Transaction.Type()
		if !typ.IsInternal() && typ != types.TxTypeSyntheticAnchor {
			app.logger.Debug("Check succeeded", "type", typ, "txid", txid, "hash", tmHash)
		}

		data, err2 := result.MarshalBinary()
		if err2 != nil {
			app.logger.Error("Check - failed to marshal transaction result", "error", err2, "type", typ, "txid", txid, "hash", tmHash)
			continue
		}

		resp.Data = append(resp.Data, data...)
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

	h := sha256.Sum256(req.Tx)
	tmHash := logging.AsHex(h[:])

	// Unmarshal all of the envelopes
	envelopes, err := transactions.UnmarshalAll(req.Tx)
	if err != nil {
		sentry.CaptureException(err)
		app.logger.Info("Deliver failed", "tx", tmHash, "error", err)
		return abci.ResponseDeliverTx{Code: uint32(protocol.ErrorCodeEncodingError), Log: "Unable to decode transaction"}
	}

	// Deliver all of the transactions
	resp := abci.ResponseCheckTx{Code: uint32(protocol.ErrorCodeOK)}
	for _, env := range envelopes {
		txid := logging.AsHex(env.GetTxHash())
		result, err := app.Chain.DeliverTx(env)
		if err != nil {
			sentry.CaptureException(err)
			app.logger.Info("Deliver failed", "type", env.Transaction.Type().String(), "txid", txid, "hash", tmHash, "error", err, "origin", env.Transaction.Origin)
			// Whether or not the transaction succeeds does not matter to Tendermint
			continue
		}

		typ := env.Transaction.Type()
		if !typ.IsInternal() && typ != types.TxTypeSyntheticAnchor {
			app.logger.Debug("Deliver succeeded", "type", typ, "txid", txid, "hash", tmHash)
		}

		data, err2 := result.MarshalBinary()
		if err2 != nil {
			app.logger.Error("Check - failed to marshal transaction result", "error", err2, "type", typ, "txid", txid, "hash", tmHash)
			continue
		}

		resp.Data = append(resp.Data, data...)
	}

	app.txct += int64(len(envelopes))
	return abci.ResponseDeliverTx{Code: uint32(protocol.ErrorCodeOK)}
}

// EndBlock implements github.com/tendermint/tendermint/abci/types.Application.
func (app *Accumulator) EndBlock(req abci.RequestEndBlock) (resp abci.ResponseEndBlock) {
	defer app.recover(nil, true)

	r := app.Chain.EndBlock(EndBlockRequest{})
	resp.ValidatorUpdates = make([]abci.ValidatorUpdate, len(r.NewValidators))
	for i, key := range r.NewValidators {
		resp.ValidatorUpdates[i] = abci.ValidatorUpdate{
			PubKey: protocrypto.PublicKey{
				Sum: &protocrypto.PublicKey_Ed25519{
					Ed25519: key,
				},
			},
			Power: 1,
		}
	}

	return resp
}

// Commit implements github.com/tendermint/tendermint/abci/types.Application.
//
// Commits the transaction block to the chains.
func (app *Accumulator) Commit() (resp abci.ResponseCommit) {
	defer app.recover(nil, true)

	//end the current batch of transactions in the Stateful Merkle Tree

	mdRoot, err := app.Chain.Commit()
	resp.Data = mdRoot

	if err != nil {
		app.fatal(err, true)
		return
	}

	//this will truncate what tendermint stores since we only care about current state
	//todo: uncomment the next line when we have smt state syncing complete. For now, we are retaining everything for test net
	// if app.RetainBlocks > 0 && app.Height >= app.RetainBlocks {
	// 	resp.RetainHeight = app.Height - app.RetainBlocks + 1
	// }

	duration := time.Since(app.timer)
	app.logger.Debug("Committed", "transactions", app.txct, "duration", duration.String(), "tps", float64(app.txct)/duration.Seconds())

	return resp
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
