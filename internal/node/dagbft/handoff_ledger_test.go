// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/persist"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// The join calls Handoff(q) and nothing else: the round is not the join's to
// supply. The service reads it, in the block production loop, from the system
// ledger of the state the pull left in the database the daemon configured
// (#4362). This drives that path — Handoff, the loop, the ledger read — and
// not performHandoffAt with a round handed in by the test.
func TestHandoff_ReadsTheRoundFromThePulledLedgerThroughTheLoop(t *testing.T) {
	svc, ca, author := newJoiningService(t)
	w := svc.node.Workers()[0]
	ctx, cancel := context.WithCancel(context.Background())
	svc.ctx = ctx

	// The daemon starts collecting before the service knows its block.
	svc.StartCollecting()
	for _, round := range []int{20, 22, 24, 26} {
		b := types.NewBatch([][]byte{{byte(round)}})
		require.NoError(t, w.StoreBatch(b))
		_, err := svc.processCommittedGroup(group(commitCert(author, types.Round(round), time.Unix(int64(100+round), 0),
			[]types.PayloadEntry{{Digest: b.Digest(), Worker: w.ID()}})))
		require.NoError(t, err)
	}

	svc.wg.Add(1)
	go svc.blockProductionLoop()
	defer func() { cancel(); svc.wg.Wait() }()

	// The pull leaves block 90, committed at round 22.
	pullState(t, svc, 90, 22)
	require.NoError(t, svc.Handoff(90))

	require.False(t, svc.Collecting())
	require.Len(t, ca.blocks, 2, "rounds 20 and 22 are in the state")
	require.Equal(t, uint64(91), ca.blocks[0].Index)
	require.Equal(t, types.Round(24), ca.blocks[0].LeaderRound)
	require.Equal(t, uint64(92), ca.blocks[1].Index)
	require.Equal(t, types.Round(26), ca.blocks[1].LeaderRound)
}

// The block and the round are one record's: a ledger that is not block q is
// not the state q is, and nothing is handed off at it.
func TestHandoff_RefusesALedgerThatIsNotTheMatchedBlock(t *testing.T) {
	svc, ca, author := newJoiningService(t)
	w := svc.node.Workers()[0]
	svc.StartCollecting()
	for _, round := range []int{4, 6} {
		b := types.NewBatch([][]byte{{byte(round)}})
		require.NoError(t, w.StoreBatch(b))
		_, err := svc.processCommittedGroup(group(commitCert(author, types.Round(round), time.Unix(int64(100+round), 0),
			[]types.PayloadEntry{{Digest: b.Digest(), Worker: w.ID()}})))
		require.NoError(t, err)
	}

	// The join matched block 51; the ledger in the store is block 50's, at a
	// round the buffer holds. Handing off would number round 6 as 52.
	pullState(t, svc, 50, 4)
	err := svc.performHandoff(51)
	require.True(t, errors.Is(err, errors.Conflict), "got %v", err)
	require.True(t, svc.Collecting())
	require.Len(t, svc.Buffered(), 2)
	require.Empty(t, ca.blocks)
}

// A node that joined and then executed blocks stands at the round of the last
// block it produced, not at the round it joined at. Syncing again, a state at
// the round it first joined at is behind it: the groups it executed since are
// in neither that state nor the new buffer (#4362).
func TestHandoff_ANodeStandsAtTheRoundOfTheLastBlockItProduced(t *testing.T) {
	svc, ca, author := newJoiningService(t)
	w := svc.node.Workers()[0]
	commit := func(round int) {
		t.Helper()
		b := types.NewBatch([][]byte{{byte(round)}})
		require.NoError(t, w.StoreBatch(b))
		_, err := svc.processCommittedGroup(group(commitCert(author, types.Round(round), time.Unix(int64(100+round), 0),
			[]types.PayloadEntry{{Digest: b.Digest(), Worker: w.ID()}})))
		require.NoError(t, err)
	}

	// Restored at block 40, round 2, and joined there.
	svc.lastBlockIndex = 40
	svc.lastLeaderRound = 2
	svc.StartCollecting()
	pullState(t, svc, 40, 2)
	require.NoError(t, svc.performHandoff(40))
	commit(4)
	commit(6)
	require.Len(t, ca.blocks, 2, "blocks 41 and 42, live")

	svc.StartCollecting()
	commit(8)

	err := svc.performHandoff(40)
	require.True(t, errors.Is(err, errors.Conflict), "got %v", err)
	require.True(t, svc.Collecting())
	require.Len(t, ca.blocks, 2, "round 8 is not block 41")
}

// A ledger that records no leader round — written before v2-kourou — says
// nothing about which collected group is the next block. The handoff waits
// rather than count.
func TestHandoff_WaitsOnALedgerThatRecordsNoRound(t *testing.T) {
	svc, ca, author := newJoiningService(t)
	w := svc.node.Workers()[0]
	svc.lastBlockIndex = 40
	svc.StartCollecting()
	b := types.NewBatch([][]byte{{1}})
	require.NoError(t, w.StoreBatch(b))
	_, err := svc.processCommittedGroup(group(commitCert(author, 2, time.Unix(100, 0),
		[]types.PayloadEntry{{Digest: b.Digest(), Worker: w.ID()}})))
	require.NoError(t, err)

	pullState(t, svc, 40, 0)
	err = svc.performHandoff(40)
	require.True(t, errors.Is(err, errors.NotReady), "got %v", err)
	require.True(t, svc.Collecting())
	require.Len(t, svc.Buffered(), 1)
	require.Empty(t, ca.blocks)
}

// A round within what this node collected that no buffered group was
// committed at is a state its consensus did not produce: refused, buffer
// intact.
func TestHandoffAtLeaderRound_RefusesARoundNoGroupWasCommittedAt(t *testing.T) {
	svc, ca, author := newJoiningService(t)
	w := svc.node.Workers()[0]
	svc.StartCollecting()
	for _, round := range []int{4, 8} {
		b := types.NewBatch([][]byte{{byte(round)}})
		require.NoError(t, w.StoreBatch(b))
		_, err := svc.processCommittedGroup(group(commitCert(author, types.Round(round), time.Unix(int64(100+round), 0),
			[]types.PayloadEntry{{Digest: b.Digest(), Worker: w.ID()}})))
		require.NoError(t, err)
	}

	err := svc.performHandoffAt(60, 6)
	require.True(t, errors.Is(err, errors.Conflict), "got %v", err)
	require.True(t, svc.Collecting())
	require.Len(t, svc.Buffered(), 2)
	require.Empty(t, ca.blocks)
}

// A restarted node's consensus resumes after the round its checkpoint
// recorded, so the groups at or below it are not delivered to the buffer —
// save one delivered again, which proves nothing about the ones beside it. A
// pulled state below that round is refused (the join pulls again), even at
// the round of a group the buffer holds; a state at it hands off, producing
// what was collected after it.
func TestHandoffAtLeaderRound_ARestartedNodeStandsAtItsCheckpointsRound(t *testing.T) {
	svc, ca, author := newJoiningService(t)
	w := svc.node.Workers()[0]

	dir := t.TempDir()
	store := persist.NewStore(dir)
	store.SetFilename(checkpointFile)
	require.NoError(t, store.Save(&persist.Checkpoint{
		Version: 1, Partition: "bvn1", BlockIndex: 40, CurrentRound: 31, LastCommitRound: 30,
	}))
	svc.config.DataDir = dir
	svc.lastBlockIndex = 40
	svc.seedFromCheckpoint()

	// Round 26 is delivered again; 28 and 30 are not, and 32 is new.
	svc.StartCollecting()
	for _, round := range []int{26, 32} {
		b := types.NewBatch([][]byte{{byte(round)}})
		require.NoError(t, w.StoreBatch(b))
		_, err := svc.processCommittedGroup(group(commitCert(author, types.Round(round), time.Unix(int64(100+round), 0),
			[]types.PayloadEntry{{Digest: b.Digest(), Worker: w.ID()}})))
		require.NoError(t, err)
	}

	err := svc.performHandoffAt(38, 26)
	require.True(t, errors.Is(err, errors.Conflict), "got %v", err)
	require.True(t, svc.Collecting())
	require.Empty(t, ca.blocks, "32 is not block 39: 28 and 30 are in neither the buffer nor the state")

	require.NoError(t, svc.performHandoffAt(40, 30))
	require.Len(t, ca.blocks, 1)
	require.Equal(t, uint64(41), ca.blocks[0].Index)
	require.Equal(t, types.Round(32), ca.blocks[0].LeaderRound)
}
