// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/bullshark"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// newUnseededService is a joining node that restored no checkpoint, as Start
// leaves it: collecting, ordering held, waiting for a seed, with its block
// production loop running. follower puts it in no committee.
func newUnseededService(t *testing.T, follower bool) (*Service, *collectingAdapter, func(round int)) {
	t.Helper()
	svc, ca, author := newJoiningService(t)
	if follower {
		other, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		svc.committee = types.NewCommittee([]types.ValidatorInfo{{PublicKey: other, Stake: 1}}, 1)
	}
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

// A follower is seeded when its join starts, so the state it proves at the
// end of its pull is above the rescue window and is handed off at the first
// time it is asked about (#4530). Seeded at that state instead, the first
// StageThrough was always refused -- the state was the seed -- and every join
// paid another pull round: 16 to 55 s a partition in Docker run
// 20261006T044611Z.
func TestSeedAt_AFollowerSeededWhenItsJoinStartsHandsOffAtItsFirstProvenState(t *testing.T) {
	svc, ca, commit := newUnseededService(t, true)
	require.True(t, svc.AwaitingSeed())

	// The join starts: block 20's ledger, proven, records round 100.
	require.True(t, svc.SeedAt(100))
	require.False(t, svc.AwaitingSeed())
	require.False(t, svc.node.OrderingHeld(), "consensus still orders nothing")
	require.Equal(t, types.Round(100), svc.node.LastCommitRound())
	require.Equal(t, types.Round(100)+bullshark.RescueWindow, svc.seedFloor)

	// It is seeded once: a later answer moves nothing.
	require.False(t, svc.SeedAt(200))
	require.Equal(t, types.Round(100), svc.node.LastCommitRound())

	// While the node pulls, consensus commits and the node collects.
	for _, round := range []int{102, 120, 140, 142, 144} {
		commit(round)
	}

	// The pull ends: block 41, committed at round 142, is proven. It is the
	// first state the service is asked about, and it is taken.
	pullState(t, svc, 41, 142)
	require.NoError(t, svc.StageThrough(42), "the first proven state was refused")
	require.NoError(t, svc.Handoff(41))
	require.False(t, svc.Collecting())
	require.Len(t, ca.blocks, 1)
	require.Equal(t, uint64(42), ca.blocks[0].Index)
	require.Equal(t, types.Round(144), ca.blocks[0].LeaderRound)
}

// Seeding early does not move the floor: a state inside the rescue window
// above the seed is one whose groups may hold certificates the peers
// committed before it, and is refused whenever the node was seeded.
func TestSeedAt_AStateInsideTheRescueWindowIsStillRefused(t *testing.T) {
	svc, ca, commit := newUnseededService(t, true)
	require.True(t, svc.SeedAt(100))
	for _, round := range []int{102, 120, 122} {
		commit(round)
	}

	pullState(t, svc, 30, 120)
	err := svc.StageThrough(31)
	require.ErrorIs(t, err, errors.Conflict)
	require.True(t, svc.Collecting())
	require.Empty(t, ca.blocks)
	require.Equal(t, types.Round(100)+bullshark.RescueWindow, svc.seedFloor, "the refusal moved the seed")
}

// A node in the committee is not seeded before its state is proven. A
// validator whose store was wiped has lost the record of the rounds it
// authored, and a seed taken seconds after it restarts is closer to them than
// one taken at the end of its pull (consensus spec, "Restart"). It is seeded
// by its first StageThrough, as it was.
func TestSeedAt_AValidatorIsSeededOnlyAtItsFirstProvenState(t *testing.T) {
	svc, _, commit := newUnseededService(t, false)
	require.False(t, svc.AwaitingSeed())
	require.False(t, svc.SeedAt(100))
	require.True(t, svc.awaitingSeed)
	require.True(t, svc.node.OrderingHeld())
	require.Zero(t, svc.seedFloor)

	commit(122)
	pullState(t, svc, 30, 120)
	err := svc.StageThrough(31)
	require.ErrorIs(t, err, errors.Conflict, "the state a validator is seeded from is inside its own window")
	require.False(t, svc.awaitingSeed)
	require.Equal(t, types.Round(120)+bullshark.RescueWindow, svc.seedFloor)
}

// A node the join seeded restored no checkpoint, and is refused the
// whole-restart exit as a node still waiting for a seed is: the seed is the
// network's position, not one this node's own state was produced at. Taking
// it for one handed off at the node's own ledger round, below the floor,
// which the join counts as a failed handoff and collects again.
func TestSeedAt_ASeededNodeIsStillRefusedTheWholeRestartExit(t *testing.T) {
	svc, ca, commit := newUnseededService(t, true)
	svc.lastBlockIndex, svc.lastLeaderRound = 2, 6
	pullState(t, svc, 2, 6) // the node's own ledger
	require.True(t, svc.SeedAt(100))
	commit(102)

	err := svc.Resume()
	require.ErrorIs(t, err, errors.NotReady)
	require.True(t, svc.Collecting())
	require.Len(t, svc.Buffered(), 1, "the refusal emptied the buffer")
	require.Empty(t, ca.blocks)
}
