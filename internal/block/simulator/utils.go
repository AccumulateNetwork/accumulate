package simulator

import (
	"encoding"
	"os"

	"github.com/stretchr/testify/require"
	. "gitlab.com/accumulatenetwork/accumulate/internal/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types/api/query"
)

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
		return nil, errors.Wrap(errors.StatusUnknownError, err)
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
