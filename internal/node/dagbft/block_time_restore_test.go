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
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/persist"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// pullStateAt is pullState with the time the ledger records for the block.
func pullStateAt(t *testing.T, svc *Service, index uint64, round types.Round, at time.Time) {
	t.Helper()
	u := protocol.PartitionUrl(svc.config.Partition.ID).JoinPath(protocol.Ledger)
	require.NoError(t, svc.config.Database.Update(func(batch *database.Batch) error {
		if err := putNetwork(batch, svc.config.Partition.ID, svc.Committee()); err != nil {
			return err
		}
		return batch.Account(u).Main().Put(&protocol.SystemLedger{Url: u, Index: index, LeaderRound: uint64(round), Timestamp: at})
	}))
}

// A block's time is its leader's stamp raised to after the previous block's
// time (consensus spec, "Restart"). A node that restarts raises the first
// block after the restart exactly as a peer that did not restart raises it:
// the checkpoint the restart resumes from carries its block's time.
func TestBlockTime_ARestartedNodeTimesTheNextBlockAsItsPeers(t *testing.T) {
	dir := t.TempDir()
	first := time.Unix(100, 700_000_000)
	slow := time.Unix(100, 200_000_000) // the next leader's clock is behind

	// A peer that does not restart, and the node before its restart, produce
	// block 1 from the same group.
	peer, peerBlocks, author := newCommitService(t, 1)
	node, _, _ := newCommitService(t, 1)
	node.config.DataDir = dir
	for _, svc := range []*Service{peer, node} {
		svc.node.Bullshark().SetLastCommitRound(4)
		svc.node.Primary().SetRound(5)
		_, err := svc.processCommittedGroup(group(commitCert(author, 4, first, nil)))
		require.NoError(t, err)
	}

	// The restart: the executor stands at block 1, whose ledger records its
	// time to the second.
	restarted, restartedBlocks, _ := newCommitService(t, 1)
	restarted.config.DataDir = dir
	restartedBlocks.last = 1
	pullStateAt(t, restarted, 1, 4, first.Truncate(time.Second))
	require.NoError(t, restarted.initializeGenesis())
	seeded, err := restarted.seedFromCheckpoint()
	require.NoError(t, err)
	require.True(t, seeded, "precondition: the checkpoint for block 1 is restored")

	// Block 2's leader stamped it before block 1's time.
	for _, svc := range []*Service{peer, restarted} {
		_, err := svc.processCommittedGroup(group(commitCert(author, 6, slow, nil)))
		require.NoError(t, err)
	}

	require.Len(t, peerBlocks.blocks, 2)
	require.Len(t, restartedBlocks.blocks, 1)
	want := first.Add(time.Millisecond).UTC()
	require.Equal(t, want, peerBlocks.blocks[1].Time, "precondition: the peer raises the slow stamp")
	require.Equal(t, uint64(2), restartedBlocks.blocks[0].Index)
	require.Equal(t, want, restartedBlocks.blocks[0].Time, "the restarted node must time block 2 as its peer does")
}

// A checkpoint written before checkpoints carried the block's time restores
// none; the node raises the next block's time above the time its ledger
// records for its last block.
func TestBlockTime_ACheckpointWithoutATimeFallsBackToTheLedger(t *testing.T) {
	dir := t.TempDir()
	store := persist.NewStore(dir)
	store.SetFilename(checkpointFile)
	require.NoError(t, store.Save(&persist.Checkpoint{
		Version: 1, Partition: "bvn1", BlockIndex: 1, CurrentRound: 5, LastCommitRound: 4,
	}))

	restarted, ca, author := newCommitService(t, 1)
	restarted.config.DataDir = dir
	ca.last = 1
	recorded := time.Unix(100, 0).UTC()
	pullStateAt(t, restarted, 1, 4, recorded)
	require.NoError(t, restarted.initializeGenesis())
	seeded, err := restarted.seedFromCheckpoint()
	require.NoError(t, err)
	require.True(t, seeded)

	_, err = restarted.processCommittedGroup(group(commitCert(author, 6, time.Unix(50, 0), nil)))
	require.NoError(t, err)
	require.Len(t, ca.blocks, 1)
	require.Equal(t, recorded.Add(time.Millisecond), ca.blocks[0].Time)
}

// A joining node produces the blocks after the state it pulled, and raises
// the first one's time above the time that state's ledger records.
func TestBlockTime_AJoinedNodeTimesTheNextBlockAfterThePulledState(t *testing.T) {
	svc, ca, author := newJoiningService(t)
	ctx, cancel := context.WithCancel(context.Background())
	svc.ctx = ctx
	svc.StartCollecting()
	for i, round := range []types.Round{20, 22} {
		_, err := svc.processCommittedGroup(group(commitCert(author, round, time.Unix(int64(50+i), 0), nil)))
		require.NoError(t, err)
	}

	svc.wg.Add(1)
	go svc.blockProductionLoop()
	defer func() { cancel(); svc.wg.Wait() }()

	recorded := time.Unix(200, 0).UTC()
	pullStateAt(t, svc, 90, 20, recorded)
	require.NoError(t, svc.Handoff(90))

	require.Len(t, ca.blocks, 1)
	require.Equal(t, uint64(91), ca.blocks[0].Index)
	require.Equal(t, recorded.Add(time.Millisecond), ca.blocks[0].Time)
}
