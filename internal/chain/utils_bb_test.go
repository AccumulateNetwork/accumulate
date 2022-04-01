package chain_test

// Blackbox testing utilities

import (
	"encoding"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"github.com/tendermint/tendermint/libs/log"
	tmtypes "github.com/tendermint/tendermint/types"
	"gitlab.com/accumulatenetwork/accumulate/config"
	. "gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/genesis"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/memory"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/query"
)

func SetupExecNetwork(t testing.TB) *TestRouter {
	const nameD = protocol.Directory
	const nameB = "BlockValidator"

	// Loggers
	var logger log.Logger
	if acctesting.LogConsole {
		w, err := logging.NewConsoleWriter("plain")
		require.NoError(t, err)
		level, writer, err := logging.ParseLogLevel(acctesting.DefaultLogLevels, w)
		require.NoError(t, err)
		logger, err = logging.NewTendermintLogger(zerolog.New(writer), level, false)
		require.NoError(t, err)
	} else {
		logger = logging.NewTestLogger(t, "plain", acctesting.DefaultLogLevels, false)
	}

	loggerD := logger.With("subnet", nameD)
	loggerB := logger.With("subnet", nameB)

	// Node keys
	keyD := acctesting.GenerateKey(t.Name(), nameD)
	keyB := acctesting.GenerateKey(t.Name(), nameB)

	// Databases
	dbD := database.OpenInMemory(loggerD)
	dbB := database.OpenInMemory(loggerB)

	// Network configuration
	subnets := []config.Subnet{
		{Type: config.Directory, ID: nameD, Nodes: []config.Node{{Type: config.Validator, Address: nameD}}},
		{Type: config.BlockValidator, ID: nameB, Nodes: []config.Node{{Type: config.Validator, Address: nameB}}},
	}
	networkD := config.Network{
		Type:          config.Directory,
		LocalSubnetID: nameD,
		LocalAddress:  nameD,
		Subnets:       subnets,
	}
	networkB := config.Network{
		Type:          config.BlockValidator,
		LocalSubnetID: nameB,
		LocalAddress:  nameB,
		Subnets:       subnets,
	}

	// Initialize the router
	router := &TestRouter{
		TB:      t,
		Logger:  logger.With("module", "test-router"),
		Network: &config.Network{Subnets: subnets},
	}

	// Create the executors
	execD, err := NewNodeExecutor(ExecutorOptions{
		Logger:  loggerD,
		Key:     keyD,
		Network: networkD,
		Router:  router,
	}, dbD)
	require.NoError(t, err)
	execB, err := NewNodeExecutor(ExecutorOptions{
		Logger:  loggerB,
		Key:     keyB,
		Network: networkB,
		Router:  router,
	}, dbB)
	require.NoError(t, err)

	// Populate the router
	router.Executors = map[string]*RouterExecEntry{
		nameD: {Database: dbD, Executor: execD},
		nameB: {Database: dbB, Executor: execB},
	}

	// Start the executors
	require.NoError(t, execD.Start())
	require.NoError(t, execB.Start())

	return router
}

func SetupExecSingle(t testing.TB) (*database.Database, *Executor) {
	logger := logging.NewTestLogger(t, "plain", acctesting.DefaultLogLevels, false)
	db := database.OpenInMemory(logger)
	key := acctesting.GenerateKey(t.Name())
	subnetID := strings.ReplaceAll(t.Name(), "_", "-")
	network := config.Network{
		Type:          config.BlockValidator,
		LocalSubnetID: subnetID,
		Subnets: []config.Subnet{
			{Type: config.Directory, ID: protocol.Directory},
			{Type: config.BlockValidator, ID: subnetID},
		},
	}
	exec, err := NewNodeExecutor(ExecutorOptions{
		Logger:  logger,
		Key:     key,
		Network: network,
		Router:  acctesting.NullRouter{},
	}, db)
	require.NoError(t, err)
	require.NoError(t, exec.Start())

	return db, exec
}

func InitChain(t testing.TB, db *database.Database, exec *Executor) {
	block := new(Block)
	block.Index = protocol.GenesisBlock
	block.Time = time.Unix(0, 0)
	block.IsLeader = true
	block.Batch = db.Begin(true)
	defer block.Batch.Discard()

	// Genesis
	temp := memory.New(exec.Logger)
	_, err := genesis.Init(temp, genesis.InitOpts{
		Network:     exec.Network,
		GenesisTime: block.Time,
		Logger:      exec.Logger,
		Validators: []tmtypes.GenesisValidator{
			{PubKey: ed25519.PubKey(exec.Key[32:])},
		},
	})
	require.NoError(t, err)

	state, err := temp.MarshalJSON()
	require.NoError(t, err)

	_, err = exec.InitChain(block, state)
	require.NoError(t, err)

	require.NoError(t, block.Batch.Commit())
}

func ExecuteBlock(t testing.TB, db *database.Database, exec *Executor, block *Block, envelopes ...*protocol.Envelope) []*protocol.TransactionStatus {
	if block == nil {
		block = new(Block)
		block.IsLeader = true
	}
	block.Batch = db.Begin(true)

	var ledger *protocol.InternalLedger
	require.NoError(t, block.Batch.Account(exec.Network.Ledger()).GetStateAs(&ledger))

	if block.Index == 0 {
		block.Index = ledger.Index + 1
	}
	if block.Time.IsZero() {
		block.Time = ledger.Timestamp.Add(time.Second)
	}

	_, err := exec.BeginBlock(block)
	require.NoError(t, err)

	results := make([]*protocol.TransactionStatus, len(envelopes))
	for i, envelope := range envelopes {
		results[i] = DeliverTx(t, exec, block, envelope)
	}

	require.NoError(t, exec.EndBlock(block))

	// Is the block empty?
	if block.State.Empty() {
		return nil
	}

	// Commit the batch
	require.NoError(t, block.Batch.Commit())

	// Notify the executor that we comitted
	batch := db.Begin(false)
	defer batch.Discard()
	require.NoError(t, exec.DidCommit(block, batch))

	return results
}

func CheckTx(t testing.TB, db *database.Database, exec *Executor, envelope *protocol.Envelope) (protocol.TransactionResult, error) {
	batch := db.Begin(false)
	defer batch.Discard()

	result, err := exec.ValidateEnvelope(batch, envelope)
	if err != nil {
		return nil, protocol.NewError(protocol.ErrorCodeUnknownError, err)
	}
	if result == nil {
		return new(protocol.EmptyResult), nil
	}
	return result, nil
}

func DeliverTx(t testing.TB, exec *Executor, block *Block, envelope *protocol.Envelope) *protocol.TransactionStatus {
	// Process signatures
	batch := block.Batch.Begin(true)
	defer batch.Discard()

	transaction, err := exec.LoadTransaction(batch, envelope)
	if err != nil {
		perr := protocol.NewError(protocol.ErrorCodeUnknownError, err)
		return &protocol.TransactionStatus{
			Code:    perr.Code.GetEnumValue(),
			Message: perr.Error(),
		}
	}
	// require.NoError(t, err)

	for _, signature := range envelope.Signatures {
		s, err := exec.ProcessSignature(batch, transaction, signature)
		require.NoError(t, err) // Signatures must be valid
		block.State.MergeSignature(s)
	}

	require.NoError(t, batch.Commit())

	// Process the transaction
	batch = block.Batch.Begin(true)
	defer batch.Discard()

	transaction, err = exec.LoadTransaction(batch, envelope)
	require.NoError(t, err)

	status, txnState, err := exec.ProcessTransaction(batch, transaction)
	require.NoError(t, err)

	if !envelope.Type().IsInternal() && envelope.Type() != protocol.TransactionTypeSyntheticAnchor {
		exec.Logger.Info("Transaction delivered",
			"module", "test-router",
			"block", block.Index,
			"type", envelope.Type(),
			"txn-hash", logging.AsHex(envelope.GetTxHash()).Slice(0, 4),
			"env-hash", logging.AsHex(envelope.EnvHash()).Slice(0, 4),
			"code", status.Code,
			"message", status.Message,
			"principal", envelope.Transaction.Header.Principal)
	}

	err = exec.ProduceSynthetic(batch, transaction, txnState.ProducedTxns)
	require.NoError(t, err)
	block.State.MergeTransaction(txnState)

	require.NoError(t, batch.Commit())

	return status
}

func RequireSuccess(t testing.TB, results ...*protocol.TransactionStatus) {
	for i, r := range results {
		require.Zerof(t, r.Code, "Transaction %d failed with code %d: %s", i, r.Code, r.Message)
	}
}

type queryRequest interface {
	encoding.BinaryMarshaler
	Type() types.QueryType
}

func Query(t testing.TB, db *database.Database, exec *Executor, req queryRequest, prove bool) interface{} {
	var err error
	qr := new(query.Query)
	qr.Type = req.Type()
	qr.Content, err = req.MarshalBinary()
	require.NoError(t, err)

	batch := db.Begin(false)
	defer batch.Discard()
	key, value, perr := exec.Query(batch, qr, 0, prove)
	if perr != nil {
		require.NoError(t, perr)
	}

	var resp encoding.BinaryUnmarshaler
	switch string(key) {
	case "account":
		resp = new(query.ResponseAccount)

	case "tx":
		resp = new(query.ResponseByTxId)

	case "tx-history":
		resp = new(query.ResponseTxHistory)

	case "chain-range":
		resp = new(query.ResponseChainRange)

	case "chain-entry":
		resp = new(query.ResponseChainEntry)

	case "data-entry":
		resp = new(query.ResponseDataEntry)

	case "data-entry-set":
		resp = new(query.ResponseDataEntrySet)

	case "pending":
		resp = new(query.ResponsePending)

	default:
		t.Fatalf("Unknown response type %s", key)
	}

	require.NoError(t, resp.UnmarshalBinary(value))
	return resp
}
