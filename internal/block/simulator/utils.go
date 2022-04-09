package simulator

import (
	"encoding"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/crypto/ed25519"
	tmtypes "github.com/tendermint/tendermint/types"
	. "gitlab.com/accumulatenetwork/accumulate/internal/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/genesis"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/memory"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/query"
)

func InitChain(t testing.TB, db *database.Database, exec *Executor) {
	t.Helper()

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
	t.Helper()

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

	err := exec.BeginBlock(block)
	require.NoError(t, err)

	var results []*protocol.TransactionStatus
	for _, envelope := range envelopes {
		for _, delivery := range NormalizeEnvelope(t, envelope) {
			st := DeliverTx(t, exec, block, delivery)
			st.For = *(*[32]byte)(delivery.Transaction.GetHash())
			results = append(results, st)
		}
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

func CheckTx(t testing.TB, db *database.Database, exec *Executor, delivery *chain.Delivery) (protocol.TransactionResult, error) {
	t.Helper()

	batch := db.Begin(false)
	defer batch.Discard()

	result, err := exec.ValidateEnvelope(batch, delivery)
	if err != nil {
		return nil, protocol.NewError(protocol.ErrorCodeUnknownError, err)
	}
	if result == nil {
		return new(protocol.EmptyResult), nil
	}
	return result, nil
}

func NormalizeEnvelope(t testing.TB, envelope *protocol.Envelope) []*chain.Delivery {
	t.Helper()

	deliveries, err := chain.NormalizeEnvelope(envelope)
	require.NoError(t, err)
	return deliveries
}

func DeliverTx(t testing.TB, exec *Executor, block *Block, delivery *chain.Delivery) *protocol.TransactionStatus {
	t.Helper()

	err := delivery.LoadTransaction(block.Batch)
	if err != nil {
		var perr *protocol.Error
		if !errors.As(err, &perr) {
			require.NoError(t, err)
		}

		if perr.Code != protocol.ErrorCodeAlreadyDelivered {
			require.NoError(t, err)
		}

		return &protocol.TransactionStatus{
			Delivered: true,
			Code:      perr.Code.GetEnumValue(),
			Message:   perr.Error(),
		}
	}

	status, err := exec.ExecuteEnvelope(block, delivery)
	require.NoError(t, err)
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
	t.Helper()

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
