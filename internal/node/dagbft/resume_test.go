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
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// A validator of a partition that restarted as a whole resumes from the
// consensus position its checkpoint restored, and never from a join's seed
// (#4447, #4405). Resume produces the groups committed after the checkpoint's
// round, from the block after the node's own, and leaves consensus where the
// checkpoint put it. A node that restored no checkpoint is refused, NotReady,
// and is not seeded: the round its ledger records is not a position its
// consensus ordered from.
func TestResume_ProducesFromTheCheckpointAndNeverSeeds(t *testing.T) {
	dir := t.TempDir()

	// Before the restart: blocks 1 and 2, at leader rounds 4 and 6, each
	// checkpointed as it is produced.
	svc, _, author := newCommitService(t, 1)
	svc.config.DataDir = dir
	for i, r := range []types.Round{4, 6} {
		svc.node.Bullshark().SetLastCommitRound(r)
		svc.node.Primary().SetRound(r + 1)
		_, err := svc.processCommittedGroup(group(commitCert(author, r, time.Unix(int64(100+i), 0), nil)))
		require.NoError(t, err)
	}

	// The restart: the executor stands at block 2, and the checkpoint for
	// block 2 is restored. The node collects, as a joining node does.
	restarted, ca, author := newJoiningService(t)
	restarted.config.DataDir = dir
	ca.last = 2
	require.NoError(t, restarted.initializeGenesis())
	require.True(t, restarted.seedFromCheckpoint(), "precondition: the checkpoint for block 2 is restored")
	restarted.StartCollecting()
	ctx, cancel := context.WithCancel(context.Background())
	restarted.ctx = ctx
	restarted.wg.Add(1)
	go restarted.blockProductionLoop()
	defer func() { cancel(); restarted.wg.Wait() }()

	// While it asked its peers, consensus committed rounds 8 and 10.
	for i, r := range []types.Round{8, 10} {
		_, err := restarted.processCommittedGroup(group(commitCert(author, r, time.Unix(int64(200+i), 0), nil)))
		require.NoError(t, err)
	}
	require.Empty(t, ca.blocks, "precondition: a collecting node produces nothing")

	require.NoError(t, restarted.Resume())
	require.False(t, restarted.Collecting())
	require.Len(t, ca.blocks, 2)
	require.Equal(t, uint64(3), ca.blocks[0].Index)
	require.Equal(t, types.Round(8), ca.blocks[0].LeaderRound)
	require.Equal(t, uint64(4), ca.blocks[1].Index)
	require.Equal(t, types.Round(10), ca.blocks[1].LeaderRound)
	require.Zero(t, restarted.seedFloor, "consensus was seeded by the join instead of resumed from the checkpoint")

	// No checkpoint for the executor's block: the node is waiting for a
	// seed, as Start leaves it. Resume refuses and does not seed.
	unseeded, ca2, _ := newJoiningService(t)
	unseeded.config.DataDir = t.TempDir()
	ca2.last = 2
	require.NoError(t, unseeded.initializeGenesis())
	require.False(t, unseeded.seedFromCheckpoint())
	unseeded.awaitingSeed = true
	pullState(t, unseeded, 2, 6) // the node's own ledger, which records the round
	unseeded.StartCollecting()
	ctx2, cancel2 := context.WithCancel(context.Background())
	unseeded.ctx = ctx2
	unseeded.wg.Add(1)
	go unseeded.blockProductionLoop()
	defer func() { cancel2(); unseeded.wg.Wait() }()

	err := unseeded.Resume()
	require.ErrorIs(t, err, errors.NotReady)
	require.True(t, unseeded.Collecting())
	require.True(t, unseeded.awaitingSeed, "Resume seeded consensus from the node's ledger")
	require.Empty(t, ca2.blocks)
}
