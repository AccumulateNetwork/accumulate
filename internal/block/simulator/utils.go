package simulator

import (
	"encoding"
	"os"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/crypto/ed25519"
	tmtypes "github.com/tendermint/tendermint/types"
	. "gitlab.com/accumulatenetwork/accumulate/internal/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/genesis"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/memory"
	"gitlab.com/accumulatenetwork/accumulate/types/api/query"
)

func InitFromGenesis(t TB, db *database.Database, exec *Executor) {
	t.Helper()

	batch := db.Begin(true)
	defer batch.Discard()

	// Genesis
	temp := memory.New(exec.Logger)
	_, err := genesis.Init(temp, genesis.InitOpts{
		Network:     exec.Network,
		GenesisTime: time.Unix(0, 0),
		Logger:      exec.Logger,
		Router:      exec.Router,
		Validators: []tmtypes.GenesisValidator{
			{PubKey: ed25519.PubKey(exec.Key[32:])},
		},
		Keys: [][]byte{exec.Key},
	})
	require.NoError(tb{t}, err)

	state, err := temp.MarshalJSON()
	require.NoError(tb{t}, err)

	require.NoError(tb{t}, exec.InitFromGenesis(batch, state))
	require.NoError(tb{t}, batch.Commit())
}

func InitFromSnapshot(t TB, db *database.Database, exec *Executor, filename string) {
	t.Helper()

	f, err := os.Open(filename)
	require.NoError(tb{t}, err)
	defer f.Close()
	batch := db.Begin(true)
	defer batch.Discard()
	require.NoError(tb{t}, exec.InitFromSnapshot(batch, f))
	require.NoError(tb{t}, batch.Commit())
}

func ExecuteBlock(t TB, db *database.Database, exec *Executor, block *Block, envelopes ...*protocol.Envelope) ([]*protocol.TransactionStatus, error) {
	t.Helper()

	if block == nil {
		block = new(Block)
		block.IsLeader = true
	}
	block.Batch = db.Begin(true)

	var ledger *protocol.InternalLedger
	require.NoError(tb{t}, block.Batch.Account(exec.Network.Ledger()).GetStateAs(&ledger))

	if block.Index == 0 {
		block.Index = ledger.Index + 1
	}
	if block.Time.IsZero() {
		block.Time = ledger.Timestamp.Add(time.Second)
	}

	err := exec.BeginBlock(block)
	require.NoError(tb{t}, err)

	var results []*protocol.TransactionStatus
	for _, envelope := range envelopes {
		for _, delivery := range NormalizeEnvelope(t, envelope) {
			st, err := DeliverTx(t, exec, block, delivery)
			if err != nil {
				return nil, err
			}
			st.For = *(*[32]byte)(delivery.Transaction.GetHash())
			results = append(results, st)
		}
	}

	require.NoError(tb{t}, exec.EndBlock(block))

	// Is the block empty?
	if block.State.Empty() {
		return results, nil
	}

	// Commit the batch
	require.NoError(tb{t}, block.Batch.Commit())

	return results, nil
}

func CheckTx(t TB, db *database.Database, exec *Executor, delivery *chain.Delivery) (protocol.TransactionResult, error) {
	t.Helper()

	batch := db.Begin(false)
	defer batch.Discard()

	result, err := exec.ValidateEnvelope(batch, delivery)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}
	if result == nil {
		return new(protocol.EmptyResult), nil
	}
	return result, nil
}

func NormalizeEnvelope(t TB, envelope *protocol.Envelope) []*chain.Delivery {
	t.Helper()

	deliveries, err := chain.NormalizeEnvelope(envelope)
	require.NoError(tb{t}, err)
	return deliveries
}

func DeliverTx(t TB, exec *Executor, block *Block, delivery *chain.Delivery) (*protocol.TransactionStatus, error) {
	t.Helper()

	status, err := delivery.LoadTransaction(block.Batch)
	if err != nil {
		if errors.Is(err, errors.StatusDelivered) {
			return status, nil
		}
		return nil, err
	}

	status, err = exec.ExecuteEnvelope(block, delivery)
	if err != nil {
		return nil, err
	}
	return status, nil
}

func RequireSuccess(t TB, results ...*protocol.TransactionStatus) {
	for i, r := range results {
		require.Zerof(tb{t}, r.Code, "Transaction %d failed with code %d: %s", i, r.Code, r.Message)
	}
}

func Query(t TB, db *database.Database, exec *Executor, req query.Request, prove bool) interface{} {
	t.Helper()

	batch := db.Begin(false)
	defer batch.Discard()
	key, value, perr := exec.Query(batch, req, 0, prove)
	if perr != nil {
		require.NoError(tb{t}, perr)
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
		tb{t}.Fatalf("Unknown response type %s", key)
	}

	require.NoError(tb{t}, resp.UnmarshalBinary(value))
	return resp
}
