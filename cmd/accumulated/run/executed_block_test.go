// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

// THE DAEMON'S VIEW OF ITS OWN BLOCK MUST SURVIVE THE PULL (#4344).
//
// `lastExecutedBlock` took the node's "own" block from `<partition>/ledger`,
// and that is an ACCOUNT — one of the accounts the join's pull fetches from a
// peer and settles into this store. acc-bvn1-val1's own log names them:
//
//	Pulled accounts were given up on unanchored: ... first=acc://bvn-BVN1.acme/ledger
//	Pulled accounts were given up on unanchored: ... first=acc://dn.acme/ledger
//
// Today a wrong value there only decides `nodeMustJoin`, which is harmless.
// But it is also what decides whether this node joins at all, and
// that branch had no test at all (#4320).
//
// The store here is filled by the REAL EXECUTOR — the simulator runs it
// through real blocks — so the record the daemon reads is one the executor
// wrote, not one this test wrote. The overwrite is the pull's own write,
// `batch.Account(u).Main().Put(...)` (pull.go:418). Then the daemon reads
// again, exactly as `start` does on a restart over the store a half-finished
// pull left behind.
func TestThePullCannotMoveTheDaemonsOwnBlock(t *testing.T) {
	const part = "BVN0"

	sim := harness.NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(harness.GenesisTime),
	)
	sim.StepN(10)
	db := sim.S.Database(part)

	// The executor recorded the block it executed, in a record no pull
	// writes. This is the write half of the fix, and it fails if the executor
	// records nothing.
	var executed, ledgerIndex uint64
	batch := db.Begin(false)
	var err error
	executed, err = batch.SystemData(part).ExecutedBlock().Get()
	require.NoError(t, err, "the executor recorded no block of its own (#4344)")
	var ledger *protocol.SystemLedger
	require.NoError(t, batch.Account(protocol.PartitionUrl(part).JoinPath(protocol.Ledger)).Main().GetAs(&ledger))
	ledgerIndex = ledger.Index
	batch.Discard()
	require.Equal(t, ledgerIndex, executed,
		"the executor's own record disagrees with the block it executed")
	require.Greater(t, executed, uint64(protocol.GenesisBlock), "the simulator executed no block of its own")

	// What the daemon does at start-up.
	svc := &DAGBFTService{Partition: &protocol.PartitionInfo{ID: part, Type: protocol.PartitionTypeBlockValidator}}
	require.NoError(t, svc.noteExecutedBlock(db))
	require.Equal(t, executed, svc.lastExecuted)

	// The pull, doing what the pull does: a peer's ledger, settled into this
	// store. The peer is ahead — that is why this node is pulling.
	peerBlock := executed + 1000
	write := db.Begin(true)
	account := write.Account(protocol.PartitionUrl(part).JoinPath(protocol.Ledger))
	require.NoError(t, account.Main().GetAs(&ledger))
	ledger.Index = peerBlock
	require.NoError(t, account.Main().Put(ledger))
	require.NoError(t, write.Commit())

	// The divergence a monitor sees: the ledger now answers the peer's block
	// while this node has executed its own. Asserted, so that a change which
	// stopped the pull from touching the ledger would be noticed here rather
	// than making this test vacuous.
	require.Equal(t, peerBlock, ledgerIndexOf(t, db, part),
		"the pull did not overwrite the ledger, so this test proves nothing")

	// A daemon restarting over that store.
	again := &DAGBFTService{Partition: &protocol.PartitionInfo{ID: part, Type: protocol.PartitionTypeBlockValidator}}
	require.NoError(t, again.noteExecutedBlock(db))
	require.Equal(t, executed, again.lastExecuted,
		"the daemon took its own block from an account the pull overwrote (#4344)")

	// And that is the number the buffer numbers its first collected group
	// from: the first group collected is the block after it, and every group
	// after that is the next block (#4351). The daemon hands it in rather
	// than the service reading it, because the service reads it before
	// Start() has set it and would read zero.
	rec := new(recordingCollector)
	rec.StartCollecting(again.lastExecuted)
	require.Equal(t, executed, rec.from,
		"the buffer numbers its blocks from an account the pull overwrote (#4344, #4351)")
}

// recordingCollector stands in for the consensus service's collecting mode.
type recordingCollector struct{ from uint64 }

func (r *recordingCollector) StartCollecting(from uint64) { r.from = from }

func ledgerIndexOf(t *testing.T, db database.Beginner, partition string) uint64 {
	t.Helper()
	batch := db.Begin(false)
	defer batch.Discard()
	var ledger *protocol.SystemLedger
	require.NoError(t, batch.Account(protocol.PartitionUrl(partition).JoinPath(protocol.Ledger)).Main().GetAs(&ledger))
	return ledger.Index
}
