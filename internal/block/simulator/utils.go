package simulator

import (
	"encoding"
	"os"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/crypto/ed25519"
	. "gitlab.com/accumulatenetwork/accumulate/internal/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/genesis"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/memory"
	"gitlab.com/accumulatenetwork/accumulate/types/api/query"
)

func InitGenesis(t TB, exec *Executor, genesisTime time.Time, genesisValues *core.GlobalValues, netValMap genesis.NetworkOperators) genesis.Bootstrap {
	t.Helper()

	// Genesis
	temp := memory.New(exec.Logger)
	bootstrap, err := genesis.Init(temp, genesis.InitOpts{
		Describe:         exec.Describe,
		GenesisTime:      genesisTime,
		NetworkOperators: netValMap,
		Logger:           exec.Logger,
		GenesisGlobals:   genesisValues,
		Operators: []*genesis.Operator{
			{PubKey: ed25519.PubKey(exec.Key[32:]), Validator: true},
		},
	})
	require.NoError(tb{t}, err)
	return bootstrap
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
