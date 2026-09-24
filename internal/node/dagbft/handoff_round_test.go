// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// The handoff picks the buffered groups by the leader round the pulled state's
// system ledger records, not by counting groups from where the node stood when
// it started collecting (#4362).
//
// Counting assumes the first buffered group is the block after the one the
// node stood at. A restarted node's consensus can deliver again groups its
// store already executed, and counting then produces them a second time under
// numbers that are not theirs. The round cannot be wrong that way: a group is
// in the pulled state exactly when its round is at or below the round the
// state's last block was committed at.
func TestHandoffAtLeaderRound_ProducesTheGroupsAboveThePulledRound(t *testing.T) {
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

	// The node stands at block 40 and collects five groups. The first two,
	// rounds 2 and 4, are blocks its store already has, delivered again; 6 is
	// block 41, 8 is block 42 and 10 is block 43.
	svc.lastBlockIndex = 40
	svc.StartCollecting()
	for _, round := range []int{2, 4, 6, 8, 10} {
		commit(round)
	}
	require.Len(t, svc.Buffered(), 5)

	// The pulled state is block 42, committed at round 8: only round 10 is
	// produced, and it is block 43.
	require.NoError(t, svc.performHandoffAt(42, 8))
	require.False(t, svc.Collecting(), "a node that has handed off is not collecting")
	require.Empty(t, svc.Buffered(), "the buffer is spent")
	require.Len(t, ca.blocks, 1, "the groups at or below the pulled round are in the state already")
	require.Equal(t, uint64(43), ca.blocks[0].Index)
	require.Equal(t, types.Round(10), ca.blocks[0].LeaderRound, "the group above the pulled round is the next block")
	require.Equal(t, uint64(43), svc.lastBlockIndex)
}

// Where the node stood when it started collecting does not enter into it. A
// node that starts collecting before its service knows its block (the daemon
// calls StartCollecting before Start) still hands off at the pulled state's
// block and round.
func TestHandoffAtLeaderRound_DoesNotDependOnWhereCollectingStarted(t *testing.T) {
	svc, ca, author := newJoiningService(t)
	w := svc.node.Workers()[0]

	svc.StartCollecting()
	for _, round := range []int{12, 14, 16} {
		b := types.NewBatch([][]byte{{byte(round)}})
		require.NoError(t, w.StoreBatch(b))
		_, err := svc.processCommittedGroup(group(commitCert(author, types.Round(round), time.Unix(int64(100+round), 0),
			[]types.PayloadEntry{{Digest: b.Digest(), Worker: w.ID()}})))
		require.NoError(t, err)
	}

	// The node never learned its block; the pulled state is block 70 at
	// round 12.
	require.NoError(t, svc.performHandoffAt(70, 12))
	require.Len(t, ca.blocks, 2)
	require.Equal(t, uint64(71), ca.blocks[0].Index)
	require.Equal(t, types.Round(14), ca.blocks[0].LeaderRound)
	require.Equal(t, uint64(72), ca.blocks[1].Index)
	require.Equal(t, types.Round(16), ca.blocks[1].LeaderRound)
	require.Equal(t, uint64(72), svc.lastBlockIndex)
}

// A pulled state committed at a round the buffer has not reached is not a
// handoff: the groups up to it are still on their way. The join waits, still
// collecting, with its buffer intact.
func TestHandoffAtLeaderRound_WaitsForARoundItHasNotCollected(t *testing.T) {
	svc, ca, author := newJoiningService(t)
	w := svc.node.Workers()[0]

	svc.lastBlockIndex = 40
	svc.StartCollecting()
	b := types.NewBatch([][]byte{{1}})
	require.NoError(t, w.StoreBatch(b))
	_, err := svc.processCommittedGroup(group(commitCert(author, 2, time.Unix(100, 0),
		[]types.PayloadEntry{{Digest: b.Digest(), Worker: w.ID()}})))
	require.NoError(t, err)

	err = svc.performHandoffAt(45, 20)
	require.Error(t, err)
	require.True(t, errors.Is(err, errors.NotReady), "got %v", err)
	require.True(t, svc.Collecting(), "and it is still joining")
	require.Len(t, svc.Buffered(), 1, "with its buffer intact")
	require.Empty(t, ca.blocks)
}
