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

	"github.com/AccumulateNetwork/accumulate"
	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/database"
	"github.com/AccumulateNetwork/accumulate/internal/logging"
	"github.com/AccumulateNetwork/accumulate/protocol"
	_ "github.com/AccumulateNetwork/accumulate/smt/pmt"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	apiQuery "github.com/AccumulateNetwork/accumulate/types/api/query"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/getsentry/sentry-go"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/crypto"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/version"
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
	Address crypto.Address
}

// NewAccumulator returns a new Accumulator.
func NewAccumulator(opts AccumulatorOptions) *Accumulator {
	app := &Accumulator{
		AccumulatorOptions: opts,
		logger:             opts.Logger.With("module", "accumulate", "subnet", opts.Network.ID),
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
func (app *Accumulator) fatal(err error) {
	app.didPanic = true

	app.logger.Error("Fatal error", "error", err, "stack", debug.Stack())
	sentry.CaptureException(err)

	if app.onFatal != nil {
		app.onFatal(err)
	}
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
		*code = protocol.CodeDidPanic
	}
}

// Info implements github.com/tendermint/tendermint/abci/types.Application.
func (app *Accumulator) Info(req abci.RequestInfo) abci.ResponseInfo {
	defer app.recover(nil)

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

	batch := app.DB.Begin()
	defer batch.Discard()

	var height int64
	ledger := protocol.NewInternalLedger()
	err = batch.Record(app.Network.NodeUrl().JoinPath(protocol.Ledger)).GetStateAs(ledger)
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
	defer app.recover(&resQuery.Code)

	if app.didPanic {
		return abci.ResponseQuery{
			Code: protocol.CodeDidPanic,
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
		resQuery.Code = protocol.CodeEncodingError
		return resQuery
	}

	k, v, customErr := app.Chain.Query(qu)
	switch {
	case customErr == nil:
		//Ok

	case errors.Is(customErr.Unwrap(), storage.ErrNotFound):
		resQuery.Info = customErr.Error()
		resQuery.Code = protocol.CodeNotFound
		return resQuery

	default:
		sentry.CaptureException(customErr)
		app.logger.Debug("Query failed", "type", qu.Type.Name(), "error", customErr)
		resQuery.Info = customErr.Error()
		resQuery.Code = uint32(customErr.Code)
		return resQuery
	}

	//if we get here, we have a valid state object, so let's return it.
	resQuery.Code = protocol.CodeOK
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
	batch := app.DB.Begin()
	_, err := batch.Record(app.Network.NodeUrl().JoinPath(protocol.Ledger)).GetState()
	switch {
	case err == nil:
		// InitChain already happened
		return abci.ResponseInitChain{AppHash: batch.RootHash()}
	case !errors.Is(err, storage.ErrNotFound):
		panic(fmt.Errorf("failed to check block index: %v", err))
	}

	app.logger.Info("Initializing")
	err = app.Chain.InitChain(req.AppStateBytes, req.Time, req.InitialHeight)
	if err != nil {
		panic(fmt.Errorf("failed to init chain: %v", err))
	}

	//register a list of the validators.
	for _, v := range req.Validators {
		app.updateValidator(v)
	}

	return abci.ResponseInitChain{AppHash: batch.RootHash()}
}

// BeginBlock implements github.com/tendermint/tendermint/abci/types.Application.
func (app *Accumulator) BeginBlock(req abci.RequestBeginBlock) abci.ResponseBeginBlock {
	defer app.recover(nil)

	var ret abci.ResponseBeginBlock

	//Identify the leader for this block, if we are the proposer... then we are the leader.
	_, err := app.Chain.BeginBlock(BeginBlockRequest{
		IsLeader: bytes.Equal(app.Address.Bytes(), req.Header.GetProposerAddress()),
		Height:   req.Header.Height,
		Time:     req.Header.Time,
	})
	if err != nil {
		app.fatal(err)
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
	defer app.recover(&rct.Code)

	if app.didPanic {
		return abci.ResponseCheckTx{
			Code: protocol.CodeDidPanic,
			Info: "Node state is invalid",
		}
	}

	h := sha256.Sum256(req.Tx)
	txHash := logging.AsHex(h[:])

	//the submission is the format of the Tx input
	env := new(transactions.Envelope)

	//unpack the request
	err := env.UnmarshalBinary(req.Tx)

	//check to see if there was an error decoding the submission
	if err != nil {
		sentry.CaptureException(err)
		app.logger.Info("Check failed", "tx", txHash, "error", err)
		//reject it
		return abci.ResponseCheckTx{Code: protocol.CodeEncodingError, GasWanted: 0,
			Log: "Unable to decode transaction"}
	}

	txid := logging.AsHex(env.Transaction.Hash())

	//create a default response
	ret := abci.ResponseCheckTx{Code: 0, GasWanted: 1, Data: env.Transaction.Origin.ResourceChain(), Log: "CheckTx"}

	customErr := app.Chain.CheckTx(env)

	if customErr != nil {
		sentry.CaptureException(customErr)
		app.logger.Info("Check failed", "type", env.Transaction.Type().Name(), "txid", txid, "hash", txHash, "error", customErr)
		ret.Code = uint32(customErr.Code)
		ret.GasWanted = 0
		ret.GasUsed = 0
		ret.Log = fmt.Sprintf("%s check of %s transaction failed: %v", env.Transaction.Origin.String(), env.Transaction.Type().Name(), customErr)
		return ret
	}

	//if we get here, the TX, passed reasonable check, so allow for dispatching to everyone else
	app.logger.Debug("Check succeeded", "type", env.Transaction.Type().Name(), "txid", txid, "hash", txHash)
	return ret
}

// DeliverTx implements github.com/tendermint/tendermint/abci/types.Application.
//
// Verifies the transaction is valid.
func (app *Accumulator) DeliverTx(req abci.RequestDeliverTx) (rdt abci.ResponseDeliverTx) {
	defer app.recover(&rdt.Code)

	if app.didPanic {
		return abci.ResponseDeliverTx{
			Code: protocol.CodeDidPanic,
			Info: "Node state is invalid",
		}
	}

	h := sha256.Sum256(req.Tx)
	txHash := logging.AsHex(h[:])
	ret := abci.ResponseDeliverTx{GasWanted: 1, GasUsed: 0, Data: []byte(""), Code: protocol.CodeOK}

	env := &transactions.Envelope{}

	//unpack the request
	//how do i detect errors?  This causes segfaults if not tightly checked.
	err := env.UnmarshalBinary(req.Tx)
	if err != nil {
		sentry.CaptureException(err)
		app.logger.Info("Deliver failed", "tx", txHash, "error", err)
		return abci.ResponseDeliverTx{Code: protocol.CodeEncodingError, GasWanted: 0,
			Log: "Unable to decode transaction"}
	}

	txid := logging.AsHex(env.Transaction.Hash())

	//run through the validation node
	customErr := app.Chain.DeliverTx(env)

	if customErr != nil {
		sentry.CaptureException(customErr)
		app.logger.Info("Deliver failed", "type", env.Transaction.Type().Name(), "txid", txid, "hash", txHash, "error", customErr)
		ret.Code = uint32(customErr.Code)
		//we don't care about failure as far as tendermint is concerned, so we should place the log in the pending
		ret.Log = fmt.Sprintf("%s delivery of %s transaction failed: %v", env.Transaction.Origin, env.Transaction.Type().Name(), customErr)
		return ret
	}

	//now we need to store the data returned by the validator and feed into accumulator
	app.txct++

	app.logger.Debug("Deliver succeeded", "type", env.Transaction.Type().Name(), "txid", txid, "hash", txHash)
	return ret
}

// EndBlock implements github.com/tendermint/tendermint/abci/types.Application.
func (app *Accumulator) EndBlock(req abci.RequestEndBlock) (resp abci.ResponseEndBlock) {
	defer app.recover(nil)

	// Select our leader who will initiate consensus on dbvc chain.
	//resp.ConsensusParamUpdates
	//for _, ev := range req.ByzantineValidators {
	//	if ev.Type == types.EvidenceType_DUPLICATE_VOTE {
	//		addr := string(ev.Validator.Address)
	//		if pubKey, ok := app.valAddrToPubKeyMap[addr]; ok {
	//			app.updateValidator(types.ValidatorUpdate{
	//				PubKey: pubKey,
	//				Power:  ev.Validator.Power - 1,
	//			})
	//			app.logger.Info("Decreased val power by 1 because of the equivocation",
	//				"val", addr)
	//		} else {
	//			app.logger.Error("Wanted to punish val, but can't find it",
	//				"val", addr)
	//		}
	//	}
	//}

	return abci.ResponseEndBlock{} //ValidatorUpdates: app.ValUpdates}
}

// Commit implements github.com/tendermint/tendermint/abci/types.Application.
//
// Commits the transaction block to the chains.
func (app *Accumulator) Commit() (resp abci.ResponseCommit) {
	defer app.recover(nil)

	//end the current batch of transactions in the Stateful Merkle Tree

	mdRoot, err := app.Chain.Commit()
	resp.Data = mdRoot

	if err != nil {
		app.fatal(err)
		return
	}

	//this will truncate what tendermint stores since we only care about current state
	//todo: uncomment the next line when we have smt state syncing complete. For now, we are retaining everything for test net
	// if app.RetainBlocks > 0 && app.Height >= app.RetainBlocks {
	// 	resp.RetainHeight = app.Height - app.RetainBlocks + 1
	// }

	duration := time.Since(app.timer)
	app.logger.Info("Committed", "transactions", app.txct, "duration", duration.String(), "tps", float64(app.txct)/duration.Seconds())

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

//updateValidator add, update, or remove a validator
func (app *Accumulator) updateValidator(v abci.ValidatorUpdate) {
	// pubkey, _ := encoding.PubKeyFromProto(v.PubKey)
	// app.logger.Info("Val Pub Key", "address", pubkey.Address())
	/*
	   	if err != nil {
	   		panic(fmt.Errorf("can't decode public key: %w", err))
	   	}
	   	//key := []byte("val:" + string(pubkey.Bytes()))
	   	if v.Power == 0 {
	   		// remove validator
	   		_, found := app.tmvalidators[string(pubkey.Address())]// app.app.state.db.Has(key)
	   		if !found {
	   			pubStr := base64.StdEncoding.EncodeToString(pubkey.Bytes())
	   			return abcitypes.ResponseDeliverTx{
	   				Code: code.CodeTypeUnauthorized,
	   				Log:  fmt.Sprintf("Cannot remove non-existent validator %s", pubStr)}
	   		}
	   //		if !hasKey
	   		//if err = app.app.state.db.Delete(key); err != nil {
	   		//	panic(err)
	   		//}
	   		delete(app.tmvalidators, string(pubkey.Address()))
	   	} else {
	   		// add or update validator
	   		//value := bytes.NewBuffer(make([]byte, 0))
	   		//if err := types.WriteMessage(&v, value); err != nil {
	   		//	return types.ResponseDeliverTx{
	   		//		Code: code.CodeTypeEncodingError,
	   		//		Log:  fmt.Sprintf("Error encoding validator: %v", err)}
	   		//}
	   		//if err = app.app.state.db.Set(key, value.Bytes()); err != nil {
	   		//	panic(err)
	   		//}
	   		app.tmvalidators[string(pubkey.Address())] = pubkey
	   	}
	*/

	// we only update the changes array if we successfully updated the tree
	//app.ValUpdates = append(app.ValUpdates, v)
}
