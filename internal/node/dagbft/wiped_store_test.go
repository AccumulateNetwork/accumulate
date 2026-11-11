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
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A node whose store was wiped and whose consensus checkpoint survived is at
// genesis with a checkpoint far above it. That is not a run of empty blocks:
// the store never executed the blocks below the checkpoint. The node does not
// resume past them; it discards the checkpoint and, joining, orders nothing
// until the join names a round (#4484).
//
// Docker run 20260929T152146Z, follower-cycles cycle 5: three followers'
// Directory nodes logged "The executor's last blocks were empty; resuming
// after the checkpointed block lastBlock=1 resumeAfter=107048".
func TestRestart_AWipedStoreDoesNotResumeFromItsCheckpoint(t *testing.T) {
	dir := t.TempDir()

	// Before the wipe: blocks 107047 and 107048, each checkpointed as it is
	// produced.
	svc, _, author := newCommitService(t, 1)
	svc.config.DataDir = dir
	svc.lastBlockIndex = 107046
	for i, r := range []types.Round{214122, 214124} {
		svc.node.Bullshark().SetLastCommitRound(r)
		svc.node.Primary().SetRound(r + 1)
		_, err := svc.processCommittedGroup(group(commitCert(author, r, time.Unix(int64(100+i), 0), nil)))
		require.NoError(t, err)
	}

	// The store is wiped and genesis loaded again: the executor stands at
	// the genesis block. The node is joining.
	restarted, ca, author := newJoiningService(t)
	restarted.config.DataDir = dir
	ca.last = protocol.GenesisBlock
	require.NoError(t, restarted.initializeGenesis())
	seeded, err := restarted.seedFromCheckpoint()
	require.NoError(t, err)
	require.False(t, seeded,
		"a store at genesis resumed from a checkpoint 107047 blocks above it, as if they were empty")
	require.Equal(t, uint64(protocol.GenesisBlock), restarted.lastBlockIndex, "the next block is numbered from the checkpoint, not the store")
	require.Equal(t, types.Round(0), restarted.node.LastCommitRound())

	// What Start does for a joining node with no checkpoint (#4405).
	restarted.StartCollecting()
	restarted.awaitingSeed = true
	ctx, cancel := context.WithCancel(context.Background())
	restarted.ctx = ctx
	restarted.wg.Add(1)
	go restarted.blockProductionLoop()
	defer func() { cancel(); restarted.wg.Wait() }()

	_, err = restarted.processCommittedGroup(group(commitCert(author, 214126, time.Unix(200, 0), nil)))
	require.NoError(t, err)

	// The whole-restart exit (#4447) resumes a node from its own block when
	// its peers are joining too. From a wiped store that block is genesis,
	// and resuming would execute block 107049 on the genesis state.
	err = restarted.Resume()
	require.ErrorIs(t, err, errors.NotReady, "a wiped store resumed and executed from genesis")
	require.Empty(t, ca.blocks)
	require.True(t, restarted.Collecting())
}
