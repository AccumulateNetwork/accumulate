// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	multiexec "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/multi"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// The system ledger records the leader round each block was committed at
// (#4362). A joining node's handoff reads it from the pulled state to know
// which buffered group is the next block; the block time is the leader's
// timestamp but clamped, so it cannot stand in.
//
// A protocol field needs its version gate: from v2-kourou the ledger carries
// the round, and before it a block's ledger is exactly what it was, so a node
// on the old version reads and executes old blocks unchanged.

func TestLeaderRound_TheSystemLedgerRecordsTheBlocksLeaderRound(t *testing.T) {
	n := newLeaderRoundNode(t, protocol.ExecutorVersionV2Kourou)

	n.execute(t, 77)
	require.Equal(t, uint64(77), n.ledger(t).LeaderRound, "the ledger records the round the block was committed at")

	n.execute(t, 80)
	require.Equal(t, uint64(80), n.ledger(t).LeaderRound, "and each block records its own")
}

func TestLeaderRound_BeforeKourouTheLedgerIsUnchanged(t *testing.T) {
	n := newLeaderRoundNode(t, protocol.ExecutorVersionV2Tanegashima)

	n.execute(t, 77)
	ledger := n.ledger(t)
	require.Zero(t, ledger.LeaderRound, "before v2-kourou the round is not recorded")

	// The record carries no field 10, the LeaderRound field, so a node on the
	// old version reads it without knowing the field exists. Comparing the
	// ledger's encoding with a copy whose round is zeroed proved nothing: the
	// round was already zero. The fields the record does carry are walked in
	// their wire order, and the field after the last of them is not 10.
	gotBytes, err := ledger.MarshalBinary()
	require.NoError(t, err)
	require.NotContains(t, encodedFields(t, gotBytes), uint(10), "the pre-kourou ledger encodes without a leader round")

	read := new(protocol.SystemLedger)
	require.NoError(t, read.UnmarshalBinary(gotBytes))
	require.Zero(t, read.LeaderRound)
	require.Equal(t, ledger.Index, read.Index)
}

// encodedFields lists the field numbers a marshalled SystemLedger carries, in
// wire order. It reads each field as the ledger's decoder does, so a field
// the record does not carry is never reported as read.
func encodedFields(t *testing.T, b []byte) []uint {
	t.Helper()
	reader := encoding.NewReader(bytes.NewReader(b))
	reader.ReadEnum(1, new(protocol.AccountType))
	reader.ReadUrl(2)
	reader.ReadUint(3)
	reader.ReadTime(4)
	reader.ReadBigInt(5)
	for reader.ReadValue(6, new(protocol.NetworkAccountUpdate).UnmarshalBinaryFrom) {
	}
	reader.ReadValue(7, func(r io.Reader) error {
		_, err := protocol.UnmarshalAnchorBodyFrom(r)
		return err
	})
	reader.ReadEnum(8, new(protocol.ExecutorVersion))
	for reader.ReadValue(9, new(protocol.PartitionExecutorVersion).UnmarshalBinaryFrom) {
	}
	reader.ReadUint(10)
	seen, err := reader.Reset(nil)
	require.NoError(t, err)
	var fields []uint
	for i, ok := range seen {
		if ok {
			fields = append(fields, uint(i+1))
		}
	}
	return fields
}

// A leaderRoundNode is a single-BVN executor over a real genesis.
type leaderRoundNode struct {
	exec  execute.Executor
	db    *database.Database
	key   []byte
	index uint64
	time  time.Time
	nonce uint64
}

func newLeaderRoundNode(t *testing.T, version protocol.ExecutorVersion) *leaderRoundNode {
	t.Helper()
	netInit := simulator.NewSimpleNetwork(t.Name(), 1, 1)
	genesisTime := time.Date(2022, 7, 1, 0, 0, 0, 0, time.UTC)
	values := new(core.GlobalValues)
	values.ExecutorVersion = version

	logger := logging.NewSlogLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	docs, err := accumulated.BuildGenesisDocs(netInit, values, genesisTime, logger, nil, nil)
	require.NoError(t, err)

	db := database.OpenInMemory(nil)
	require.NoError(t, snapshot.FullRestore(db, ioutil2.NewBuffer(docs["BVN0"]), nil, config.NetworkUrl{URL: protocol.PartitionUrl("BVN0")}))

	// A funded lite account, so a block has something to execute: an empty
	// block is discarded and writes no ledger at all.
	key := acctesting.GenerateKey(t.Name(), "lite")
	require.NoError(t, db.Update(func(batch *database.Batch) error {
		lite := protocol.LiteAuthorityForKey(key[32:], protocol.SignatureTypeED25519)
		err := batch.Account(lite).Main().Put(&protocol.LiteIdentity{Url: lite, CreditBalance: 1e9})
		if err != nil {
			return err
		}
		lta := lite.JoinPath("ACME")
		return batch.Account(lta).Main().Put(&protocol.LiteTokenAccount{Url: lta, TokenUrl: protocol.AcmeUrl(), Balance: *big.NewInt(1e15)})
	}))

	bus := events.NewBus(nil)
	exec, err := multiexec.NewExecutor(multiexec.Options{
		Database:      db,
		Key:           netInit.Bvns[0].Nodes[0].PrivValKey,
		Router:        routing.NewRouter(routing.RouterOptions{Events: bus}),
		EventBus:      bus,
		NewDispatcher: func() multiexec.Dispatcher { return nullDispatcher{} },
		Describe: multiexec.DescribeShim{
			NetworkType: protocol.PartitionTypeBlockValidator,
			PartitionId: "BVN0",
		},
	})
	require.NoError(t, err)
	_, err = exec.Init(nil)
	require.NoError(t, err)

	last, _, err := exec.LastBlock()
	require.NoError(t, err)
	return &leaderRoundNode{exec: exec, db: db, key: key, index: last.Index, time: last.Time}
}

// execute runs the next block, committed at the given leader round, with one
// transaction in it.
func (n *leaderRoundNode) execute(t *testing.T, round uint64) {
	t.Helper()
	n.index++
	n.time = n.time.Add(time.Second)
	n.nonce++

	block, err := n.exec.Begin(execute.BlockParams{
		Context:     context.Background(),
		IsLeader:    true,
		Index:       n.index,
		Time:        n.time,
		LeaderRound: round,
	})
	require.NoError(t, err)

	lite := protocol.LiteAuthorityForKey(n.key[32:], protocol.SignatureTypeED25519)
	env, err := build.Transaction().For(lite.JoinPath("ACME")).
		BurnTokens(1, 0).
		SignWith(lite).Version(1).Timestamp(n.nonce).PrivateKey(n.key).
		Done()
	require.NoError(t, err)
	_, err = block.Process(env)
	require.NoError(t, err)

	state, err := block.Close()
	require.NoError(t, err)
	require.False(t, state.IsEmpty(), "the block must execute something, or it writes no ledger")
	require.NoError(t, state.Commit())
}

func (n *leaderRoundNode) ledger(t *testing.T) *protocol.SystemLedger {
	t.Helper()
	var ledger *protocol.SystemLedger
	require.NoError(t, n.db.View(func(batch *database.Batch) error {
		return batch.Account(protocol.PartitionUrl("BVN0").JoinPath(protocol.Ledger)).Main().GetAs(&ledger)
	}))
	require.Equal(t, n.index, ledger.Index, "the ledger is the block just executed")
	return ledger
}

// nullDispatcher drops produced messages: nothing here depends on dispatch.
type nullDispatcher struct{}

func (nullDispatcher) Submit(context.Context, *url.URL, *messaging.Envelope) error { return nil }
func (nullDispatcher) Send(context.Context) <-chan error {
	ch := make(chan error)
	close(ch)
	return ch
}
func (nullDispatcher) Close() {}
