// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package adapter

import (
	"context"
	"io"
	"log/slog"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	multiexec "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/multi"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// #4142 (a): an acceptance gate that runs a REAL executor through
// ProduceBlock. The existing bridge tests use a recording fake, which is
// exactly what cannot answer the questions that matter here — ordering,
// re-delivery, and failure paths, through the actual batch→envelope→executor
// seam. This harness genesises a real single-BVN database, builds a real v2
// executor over it, wraps it in ExecutorBridge, and feeds hand-built
// certificates.

// nullDispatcher drops produced messages: the harness has no network, and
// nothing here depends on outbound dispatch.
type nullDispatcher struct{}

func (nullDispatcher) Submit(context.Context, *url.URL, *messaging.Envelope) error { return nil }
func (nullDispatcher) Send(context.Context) <-chan error {
	ch := make(chan error)
	close(ch)
	return ch
}
func (nullDispatcher) Close() {}

type realBridge struct {
	bridge *ExecutorBridge
	db     *database.Database
	index  uint64
	time   time.Time
}

// newRealBridgeDB genesises a fresh single-BVN database with funded lite
// accounts, returning the database and the genesis validator key.
func newRealBridgeDB(t *testing.T, liteKeys ...[]byte) (*database.Database, []byte) {
	t.Helper()
	netInit := simulator.NewSimpleNetwork("RealBridge", 1, 1)
	genesisTime := time.Date(2022, 7, 1, 0, 0, 0, 0, time.UTC)
	values := new(core.GlobalValues)
	values.ExecutorVersion = protocol.ExecutorVersionLatest

	logger := logging.NewSlogLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	docs, err := accumulated.BuildGenesisDocs(netInit, values, genesisTime, logger, nil, nil)
	require.NoError(t, err)

	db := database.OpenInMemory(nil)
	require.NoError(t, snapshot.FullRestore(db, ioutil2.NewBuffer(docs["BVN0"]), nil, config.NetworkUrl{URL: protocol.PartitionUrl("BVN0")}))

	// Fund lite accounts directly — the harness has no faucet.
	require.NoError(t, db.Update(func(batch *database.Batch) error {
		for _, key := range liteKeys {
			lite := protocol.LiteAuthorityForKey(key[32:], protocol.SignatureTypeED25519)
			err := batch.Account(lite).Main().Put(&protocol.LiteIdentity{Url: lite, CreditBalance: 1e9})
			if err != nil {
				return err
			}
			lta := lite.JoinPath("ACME")
			err = batch.Account(lta).Main().Put(&protocol.LiteTokenAccount{Url: lta, TokenUrl: protocol.AcmeUrl(), Balance: *big.NewInt(1e15)})
			if err != nil {
				return err
			}
		}
		return nil
	}))
	return db, netInit.Bvns[0].Nodes[0].PrivValKey
}

// newBridgeOver builds an ExecutorBridge over an existing database —
// "restarting the node" is calling this a second time on the same database.
func newBridgeOver(t *testing.T, db *database.Database, valKey []byte, shards int) *realBridge {
	t.Helper()
	bus := events.NewBus(nil)
	router := routing.NewRouter(routing.RouterOptions{Events: bus})

	exec, err := multiexec.NewExecutor(multiexec.Options{
		Database:        db,
		Key:             valKey,
		Router:          router,
		EventBus:        bus,
		NewDispatcher:   func() multiexec.Dispatcher { return nullDispatcher{} },
		ExecutionShards: shards,
		Describe: multiexec.DescribeShim{
			NetworkType: protocol.PartitionTypeBlockValidator,
			PartitionId: "BVN0",
		},
	})
	require.NoError(t, err)

	_, err = exec.Init(nil)
	require.NoError(t, err)

	params, _, err := exec.LastBlock()
	require.NoError(t, err)

	bridge, err := NewExecutorBridge(ExecutorBridgeConfig{Executor: exec, PartitionID: "BVN0", EventBus: bus})
	require.NoError(t, err)

	return &realBridge{bridge: bridge, db: db, index: params.Index, time: time.Date(2022, 7, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(params.Index) * time.Second)}
}

// newRealBridge builds an ExecutorBridge over a real executor on a fresh
// genesis of the given deterministic network, with funded lite accounts.
func newRealBridge(t *testing.T, shards int, liteKeys ...[]byte) *realBridge {
	t.Helper()
	db, valKey := newRealBridgeDB(t, liteKeys...)
	return newBridgeOver(t, db, valKey, shards)
}

// produce delivers one certificate (a list of batches) as the next block.
func (r *realBridge) produce(t *testing.T, batches ...*types.Batch) ([32]byte, error) {
	t.Helper()
	r.index++
	r.time = r.time.Add(time.Second)
	return r.bridge.ProduceBlock(context.Background(), BlockParams{
		Index:   r.index,
		Time:    r.time,
		Batches: batches,
	})
}

func marshalEnv(t *testing.T, env *messaging.Envelope) []byte {
	t.Helper()
	b, err := env.MarshalBinary()
	require.NoError(t, err)
	return b
}

// transfer builds a signed self-transfer with the given timestamp nonce.
func transfer(t *testing.T, liteKey []byte, amount, timestamp uint64) *messaging.Envelope {
	t.Helper()
	lite := protocol.LiteAuthorityForKey(liteKey[32:], protocol.SignatureTypeED25519)
	env, err := build.Transaction().For(lite.JoinPath("ACME")).
		BurnTokens(amount, 0).
		SignWith(lite).Version(1).Timestamp(timestamp).PrivateKey(liteKey).
		Done()
	require.NoError(t, err)
	return env
}

func liteBalance(t *testing.T, db *database.Database, liteKey []byte) uint64 {
	t.Helper()
	lite := protocol.LiteAuthorityForKey(liteKey[32:], protocol.SignatureTypeED25519)
	var acct *protocol.LiteTokenAccount
	require.NoError(t, db.View(func(batch *database.Batch) error {
		return batch.Account(lite.JoinPath("ACME")).Main().GetAs(&acct)
	}))
	return acct.Balance.Uint64()
}

// Batches execute in the certificate's canonical payload order — the
// invariant every collection proof depends on (#4054), untested until now.
// Timestamps are per-signer nonces, so if batch 2's transaction executed
// before batch 1's, batch 1's would be rejected for a stale timestamp and
// the balance would move only once.
func TestBridge_BatchesExecuteInCanonicalPayloadOrder(t *testing.T) {
	liteKey := acctesting.GenerateKey("bridge", "lite")
	r := newRealBridge(t, 0, liteKey)

	b1 := types.NewBatch([][]byte{marshalEnv(t, transfer(t, liteKey, 1, 1))})
	b2 := types.NewBatch([][]byte{marshalEnv(t, transfer(t, liteKey, 2, 2))})

	before := liteBalance(t, r.db, liteKey)
	_, err := r.produce(t, b1, b2)
	require.NoError(t, err)

	assert.Equal(t, before-3, liteBalance(t, r.db, liteKey),
		"both burns must land: out-of-order execution rejects the earlier timestamp")
}

// The same inputs produce the same block hash on two independent nodes —
// and on nodes that disagree only about the execution shard count (#4145),
// which is the DAG-BFT-shaped half of the shard equivalence gate.
func TestBridge_TwoNodesAgreeOnBlockHash(t *testing.T) {
	// Several distinct identities, so the 8-shard node genuinely spreads the
	// certificate across shards.
	keys := make([][]byte, 6)
	for i := range keys {
		keys[i] = acctesting.GenerateKey("bridge", "lite", i)
	}
	a := newRealBridge(t, 0, keys...)
	b := newRealBridge(t, 8, keys...)

	mk := func() []*types.Batch {
		var txs1, txs2 [][]byte
		for i, key := range keys {
			txs1 = append(txs1, marshalEnv(t, transfer(t, key, uint64(i+1), 1)))
			txs2 = append(txs2, marshalEnv(t, transfer(t, key, uint64(i+2), 2)))
		}
		return []*types.Batch{types.NewBatch(txs1), types.NewBatch(txs2)}
	}

	ha, err := a.produce(t, mk()...)
	require.NoError(t, err)
	hb, err := b.produce(t, mk()...)
	require.NoError(t, err)
	assert.Equal(t, ha, hb, "same certificate, same hash — serial node and 8-shard node must agree")

	for _, key := range keys {
		assert.Equal(t, liteBalance(t, a.db, key), liteBalance(t, b.db, key))
	}
}

// An envelope delivered again in a later certificate executes exactly once:
// the executor reports it already delivered instead of re-applying it.
func TestBridge_RedeliveredEnvelopeExecutesExactlyOnce(t *testing.T) {
	liteKey := acctesting.GenerateKey("bridge", "lite")
	r := newRealBridge(t, 0, liteKey)

	env := marshalEnv(t, transfer(t, liteKey, 5, 1))
	before := liteBalance(t, r.db, liteKey)

	_, err := r.produce(t, types.NewBatch([][]byte{env}))
	require.NoError(t, err)
	require.Equal(t, before-5, liteBalance(t, r.db, liteKey))

	// Deliver the identical envelope again in the next block.
	_, err = r.produce(t, types.NewBatch([][]byte{env}))
	require.NoError(t, err, "re-delivery must not fail the block")
	assert.Equal(t, before-5, liteBalance(t, r.db, liteKey),
		"the burn must not apply twice")
}

// A missing batch fails the whole block rather than executing partially —
// executing a certificate without one of its batches silently diverges this
// node from its peers (#4116/#4119).
func TestBridge_MissingBatchFailsTheBlockRatherThanExecutingPartially(t *testing.T) {
	liteKey := acctesting.GenerateKey("bridge", "lite")
	r := newRealBridge(t, 0, liteKey)

	before := liteBalance(t, r.db, liteKey)
	good := types.NewBatch([][]byte{marshalEnv(t, transfer(t, liteKey, 1, 1))})

	_, err := r.produce(t, good, nil)
	require.Error(t, err, "a nil batch means the complete-set invariant broke upstream")
	assert.Equal(t, before, liteBalance(t, r.db, liteKey),
		"nothing from the certificate may execute")
}

// An unmarshalable transaction is dropped and counted; the rest of the
// certificate executes. Silent loss at this seam is what #4132 was about.
func TestBridge_UnmarshalableTransactionIsCountedNotSwallowed(t *testing.T) {
	liteKey := acctesting.GenerateKey("bridge", "lite")
	r := newRealBridge(t, 0, liteKey)

	before := liteBalance(t, r.db, liteKey)
	batch := types.NewBatch([][]byte{
		[]byte("this is not an envelope"),
		marshalEnv(t, transfer(t, liteKey, 2, 1)),
	})

	_, err := r.produce(t, batch)
	require.NoError(t, err, "one bad transaction must not fail the block")
	assert.Equal(t, before-2, liteBalance(t, r.db, liteKey),
		"the valid transaction still executes")
}
