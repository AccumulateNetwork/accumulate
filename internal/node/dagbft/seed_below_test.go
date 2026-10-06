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
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/bullshark"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// newUnseededService is a joining node that restored no checkpoint, as Start
// leaves it: collecting, ordering held, waiting for a seed, with its block
// production loop running. commit collects a group at a leader round.
func newUnseededService(t *testing.T) (*Service, *collectingAdapter, func(round int)) {
	t.Helper()
	svc, ca, author := newJoiningService(t)
	svc.node.HoldOrdering()
	svc.awaitingSeed = true
	svc.StartCollecting()

	ctx, cancel := context.WithCancel(context.Background())
	svc.ctx = ctx
	svc.wg.Add(1)
	go svc.blockProductionLoop()
	t.Cleanup(func() { cancel(); svc.wg.Wait() })

	w := svc.node.Workers()[0]
	commit := func(round int) {
		t.Helper()
		b := types.NewBatch([][]byte{{byte(round)}})
		require.NoError(t, w.StoreBatch(b))
		_, err := svc.processCommittedGroup(group(commitCert(author, types.Round(round), time.Unix(int64(100+round), 0),
			[]types.PayloadEntry{{Digest: b.Digest(), Worker: w.ID()}})))
		require.NoError(t, err)
	}
	return svc, ca, commit
}

// The first state a joining node proves is one it can hand off at. Consensus
// is seeded the rescue window and one leader round below that state's round,
// so the state is above the seed's floor; until consensus has ordered up to
// it the node says it has not collected that far (NotReady), and then the
// state is staged and handed off at. Seeded at the state's own round, as it
// was, the state was at the floor and was refused (Conflict), and every join
// pulled another round to a later one: 16 to 55 s a partition in Docker run
// 20261006T044611Z (#4530).
func TestSeed_TheFirstProvenStateIsHandedOffAt(t *testing.T) {
	svc, ca, commit := newUnseededService(t)

	// The pull ends: block 60, committed at round 120, is proven.
	pullState(t, svc, 60, 120)
	err := svc.StageThrough(61)
	require.ErrorIs(t, err, errors.NotReady, "the state that names the seed is not one to pull past")
	require.False(t, svc.awaitingSeed)
	require.False(t, svc.node.OrderingHeld())
	require.Equal(t, types.Round(120)-seedBelow, svc.node.LastCommitRound(), "consensus orders from the rescue window and one leader below the state")
	require.Equal(t, types.Round(118), svc.seedFloor, "the state is not above the floor")
	require.Zero(t, svc.CollectedRound())

	// Consensus orders up from the seed: the groups below the state, the
	// state's own, and the one after it.
	for _, round := range []int{88, 100, 118} {
		commit(round)
	}
	require.ErrorIs(t, svc.StageThrough(61), errors.NotReady, "the state's own group is not collected yet")
	require.Equal(t, uint64(118), svc.CollectedRound())
	commit(120)
	require.ErrorIs(t, svc.StageThrough(61), errors.NotReady, "the group after the state is not collected yet")
	commit(122)

	// The same state, asked about again: staged, and handed off at.
	require.NoError(t, svc.StageThrough(61))
	require.NoError(t, svc.Handoff(60))
	require.False(t, svc.Collecting())
	require.Len(t, ca.blocks, 1, "the groups at or below the state's round are in the state")
	require.Equal(t, uint64(61), ca.blocks[0].Index)
	require.Equal(t, types.Round(122), ca.blocks[0].LeaderRound)
}

// The floor stands: once seeded, a later state at or below the floor is still
// refused -- the groups there may hold certificates the peers committed
// before the seed -- and the seed does not move.
func TestSeed_AStateAtTheFloorIsStillRefused(t *testing.T) {
	svc, ca, commit := newUnseededService(t)
	pullState(t, svc, 60, 120)
	require.ErrorIs(t, svc.StageThrough(61), errors.NotReady)

	for _, round := range []int{100, 118, 120} {
		commit(round)
	}
	// A state the node could only have been handed by a peer serving an
	// older block: at the floor.
	pullState(t, svc, 59, 118)
	err := svc.StageThrough(60)
	require.ErrorIs(t, err, errors.Conflict)
	require.Equal(t, types.Round(118), svc.seedFloor, "the refusal moved the seed")
	require.True(t, svc.Collecting())
	require.Empty(t, ca.blocks)
}

// A network a few blocks old has no round the rescue window below its state:
// the node is seeded at the state's own round and that state is refused, as
// every first state was.
func TestSeed_AStateOfAYoungNetworkSeedsAtItsOwnRound(t *testing.T) {
	svc, _, _ := newUnseededService(t)
	pullState(t, svc, 10, 20)
	require.ErrorIs(t, svc.StageThrough(11), errors.Conflict)
	require.Equal(t, types.Round(20), svc.node.LastCommitRound())
	require.Equal(t, types.Round(20)+bullshark.RescueWindow, svc.seedFloor)
}

// A DAG too shallow to be sure of holding the rounds under the state is not
// seeded below it: no peer could serve them, and consensus would order
// nothing. The node is seeded at the state's own round, as it was.
func TestSeed_AShallowDAGSeedsAtTheStatesOwnRound(t *testing.T) {
	svc, _, _ := newUnseededService(t)
	svc.config.NodeConfig.DAGGCDepth = seedBelowDepth - 1
	pullState(t, svc, 60, 120)
	require.ErrorIs(t, svc.StageThrough(61), errors.Conflict)
	require.Equal(t, types.Round(120), svc.node.LastCommitRound())
}
