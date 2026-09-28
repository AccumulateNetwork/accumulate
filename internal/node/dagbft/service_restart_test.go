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
)

// A restarted validator resumes at the consensus position that produced the
// executor's last block, not at round zero (#4238). The service checkpoints
// the position before each block; on restart the checkpoint whose block is
// the executor's last block is restored.
func TestRestart_ResumesAtTheCheckpointedRound(t *testing.T) {
	dir := t.TempDir()
	svc, ca, author := newCommitService(t, 1)
	svc.config.DataDir = dir

	// Two blocks: leaders at rounds 4 and 6. Bullshark's position is set as
	// the ordering loop would have left it before each group is handed over.
	svc.node.Bullshark().SetLastCommitRound(4)
	svc.node.Primary().SetRound(5)
	_, err := svc.processCommittedGroup(group(commitCert(author, 4, time.Unix(100, 0), nil)))
	require.NoError(t, err)
	svc.node.Bullshark().SetLastCommitRound(6)
	svc.node.Primary().SetRound(7)
	_, err = svc.processCommittedGroup(group(commitCert(author, 6, time.Unix(101, 0), nil)))
	require.NoError(t, err)
	require.Len(t, ca.blocks, 2)

	// Restart with the executor at block 2: the position for block 2.
	restarted, ca2, _ := newCommitService(t, 1)
	restarted.config.DataDir = dir
	ca2.last = 2
	require.NoError(t, restarted.initializeGenesis())
	restarted.seedFromCheckpoint()
	require.Equal(t, types.Round(6), restarted.node.LastCommitRound(), "Bullshark resumes after the last committed leader")
	require.Equal(t, types.Round(7), restarted.node.CurrentRound(), "the primary resumes at its round")
	require.Equal(t, types.Round(6), restarted.node.DAG().LastCommitRound())

	// A crash between the checkpoint and the block: the executor holds block
	// 1, and the previous checkpoint is the position that produced it.
	crashed, ca3, _ := newCommitService(t, 1)
	crashed.config.DataDir = dir
	ca3.last = 1
	require.NoError(t, crashed.initializeGenesis())
	crashed.seedFromCheckpoint()
	require.Equal(t, types.Round(4), crashed.node.LastCommitRound())
	require.Equal(t, types.Round(5), crashed.node.CurrentRound())
}

// Without a checkpoint for the executor's block the node starts at round
// zero, as before, and says so; a fresh node with no state seeds nothing.
func TestRestart_NoMatchingCheckpointStartsAtZero(t *testing.T) {
	dir := t.TempDir()
	svc, ca, _ := newCommitService(t, 1)
	svc.config.DataDir = dir
	ca.last = 7
	require.NoError(t, svc.initializeGenesis())
	svc.seedFromCheckpoint()
	require.Equal(t, types.Round(0), svc.node.CurrentRound())
	require.Equal(t, types.Round(0), svc.node.LastCommitRound())
}

// A validator whose last blocks carried nothing restarts with both
// checkpoints above its executor's last block: the executor keeps no empty
// block. It resumes after the older checkpoint's block — every block through
// it was produced — and numbers the next block as its peers do (#4448). The
// Docker BVNs of 2026-09-28 restarted with the executor at 12214 and
// checkpoints for 12216 and 12217, and every one of them started at round
// zero.
func TestRestart_ResumesPastEmptyBlocks(t *testing.T) {
	dir := t.TempDir()
	svc, ca, author := newCommitService(t, 1)
	svc.config.DataDir = dir

	// Four blocks, leaders at rounds 4, 6, 8, 10. Blocks 3 and 4 are empty,
	// so the executor's last block stays at 2.
	for _, r := range []types.Round{4, 6, 8, 10} {
		svc.node.Bullshark().SetLastCommitRound(r)
		svc.node.Primary().SetRound(r + 1)
		_, err := svc.processCommittedGroup(group(commitCert(author, r, time.Unix(int64(100+r), 0), nil)))
		require.NoError(t, err)
	}
	require.Len(t, ca.blocks, 4)

	restarted, ca2, author2 := newCommitService(t, 1)
	restarted.config.DataDir = dir
	ca2.last = 2
	require.NoError(t, restarted.initializeGenesis())
	require.True(t, restarted.seedFromCheckpoint(), "a checkpoint above the executor's last empty-tailed block is a position to resume")
	require.Equal(t, types.Round(8), restarted.node.LastCommitRound(), "resumes after block 3, the older checkpoint's")
	require.Equal(t, types.Round(9), restarted.node.CurrentRound())

	// The group of block 4 is ordered again, and is block 4 again.
	restarted.node.Bullshark().SetLastCommitRound(10)
	_, err := restarted.processCommittedGroup(group(commitCert(author2, 10, time.Unix(110, 0), nil)))
	require.NoError(t, err)
	require.Len(t, ca2.blocks, 1)
	require.Equal(t, uint64(4), ca2.blocks[0].Index, "the next block is numbered after the checkpoint's, as the peers number it")
}
