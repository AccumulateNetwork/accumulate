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
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/adapter"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// collectingAdapter is a recording adapter that can also collect a block into
// staging without executing it.
type collectingAdapter struct {
	commitAdapter
	collected []adapter.BlockParams
}

func (a *collectingAdapter) CollectBlock(_ context.Context, params adapter.BlockParams) (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.collected = append(a.collected, params)
	return len(params.Batches), nil
}

// newJoiningService is newCommitService with an adapter that can collect.
func newJoiningService(t *testing.T) (*Service, *collectingAdapter, ed25519.PublicKey) {
	t.Helper()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	committee := types.NewCommittee([]types.ValidatorInfo{{PublicKey: pub, Stake: 1}}, 1)
	nodeCfg := consensus.NodeConfig{Partition: "bvn1", KeyPair: priv, NumWorkers: 1}
	node, err := consensus.NewNode(nodeCfg, committee, nil, nil)
	require.NoError(t, err)

	ca := &collectingAdapter{commitAdapter: commitAdapter{hash: [32]byte{0xAA}}}
	svc, err := NewService(ServiceConfig{
		Partition:  &protocol.PartitionInfo{ID: "bvn1", Type: protocol.PartitionTypeBlockValidator},
		NodeConfig: nodeCfg,
		Adapter:    ca,
		EventBus:   events.NewBus(nil),
	})
	require.NoError(t, err)
	svc.node = node
	svc.committee = committee
	svc.ctx = context.Background()
	return svc, ca, pub
}

// A joining node collects every committed group and executes none of them:
// the block index does not move, no block is produced, and the group is kept
// in order so it can be produced from Q + 1 when the join hands off
// (executor spec, "Sync"; #4292).
func TestProcessCommittedGroup_CollectingBuffersInsteadOfExecuting(t *testing.T) {
	svc, ca, author := newJoiningService(t)
	w := svc.node.Workers()[0]
	svc.StartCollecting()
	require.True(t, svc.Collecting())

	b1 := types.NewBatch([][]byte{[]byte("one")})
	b2 := types.NewBatch([][]byte{[]byte("two")})
	require.NoError(t, w.StoreBatch(b1))
	require.NoError(t, w.StoreBatch(b2))

	_, err := svc.processCommittedGroup(group(commitCert(author, 2, time.Unix(100, 0),
		[]types.PayloadEntry{{Digest: b1.Digest(), Worker: w.ID()}})))
	require.NoError(t, err)
	_, err = svc.processCommittedGroup(group(commitCert(author, 4, time.Unix(101, 0),
		[]types.PayloadEntry{{Digest: b2.Digest(), Worker: w.ID()}})))
	require.NoError(t, err)

	require.Empty(t, ca.blocks, "a joining node produces no blocks")
	require.Len(t, ca.collected, 2, "every committed group is collected into staging")
	require.Equal(t, b1.Digest(), ca.collected[0].Batches[0].Digest())
	require.Equal(t, uint64(0), ca.collected[0].Index, "a collected block has no index until the handoff")
	require.Equal(t, time.Unix(100, 0).UTC(), ca.collected[0].Time, "the block time is the leader's, unclamped")

	require.Equal(t, uint64(0), svc.lastBlockIndex, "collecting does not advance the block index")

	buffered := svc.Buffered()
	require.Len(t, buffered, 2)
	require.Equal(t, types.Round(2), buffered[0].Round(), "the buffer keeps consensus order")
	require.Equal(t, types.Round(4), buffered[1].Round())
	require.Equal(t, b2.Digest(), buffered[1].Batches[0].Digest(), "the buffer keeps the batches")

	// The handoff takes the buffer and leaves collecting mode in one step.
	taken := svc.StopCollecting()
	require.Len(t, taken, 2)
	require.False(t, svc.Collecting())
	require.Empty(t, svc.Buffered())

	// And from there the service produces blocks again.
	b3 := types.NewBatch([][]byte{[]byte("three")})
	require.NoError(t, w.StoreBatch(b3))
	_, err = svc.processCommittedGroup(group(commitCert(author, 6, time.Unix(102, 0),
		[]types.PayloadEntry{{Digest: b3.Digest(), Worker: w.ID()}})))
	require.NoError(t, err)
	require.Len(t, ca.blocks, 1, "a node that has joined executes")
	require.Equal(t, uint64(1), ca.blocks[0].Index)
}

// The handoff: at the block the join matched, the node leaves collecting
// mode, stands at that block, and produces every group it buffered from the
// next one, in order (executor spec, "Sync", step 4; #4294).
func TestHandoff_ProducesTheBufferFromQPlusOne(t *testing.T) {
	svc, ca, author := newJoiningService(t)
	w := svc.node.Workers()[0]
	svc.StartCollecting()

	for i := 0; i < 3; i++ {
		b := types.NewBatch([][]byte{{byte(i)}})
		require.NoError(t, w.StoreBatch(b))
		_, err := svc.processCommittedGroup(group(commitCert(author, types.Round(2*i+2), time.Unix(int64(100+i), 0),
			[]types.PayloadEntry{{Digest: b.Digest(), Worker: w.ID()}})))
		require.NoError(t, err)
	}
	require.Len(t, svc.Buffered(), 3)
	require.Empty(t, ca.blocks)

	// The state pull reached block 40; staging was settled there.
	require.NoError(t, svc.performHandoff(40))

	require.False(t, svc.Collecting(), "a node that has joined is not collecting")
	require.Empty(t, svc.Buffered(), "the buffer is spent")
	require.Len(t, ca.blocks, 3, "every buffered group produced a block")
	require.Equal(t, uint64(41), ca.blocks[0].Index, "the first block after the block the state is")
	require.Equal(t, uint64(42), ca.blocks[1].Index)
	require.Equal(t, uint64(43), ca.blocks[2].Index)
	require.Equal(t, types.Round(2), ca.blocks[0].LeaderRound, "in the order consensus committed them")
	require.Equal(t, types.Round(6), ca.blocks[2].LeaderRound)
	require.Equal(t, uint64(43), svc.lastBlockIndex)

	// And the next committed group is produced, not collected.
	b := types.NewBatch([][]byte{[]byte("after")})
	require.NoError(t, w.StoreBatch(b))
	_, err := svc.processCommittedGroup(group(commitCert(author, 8, time.Unix(200, 0),
		[]types.PayloadEntry{{Digest: b.Digest(), Worker: w.ID()}})))
	require.NoError(t, err)
	require.Len(t, ca.blocks, 4)
	require.Equal(t, uint64(44), ca.blocks[3].Index)
}

// A handoff is refused when the node is not joining, and when the buffer
// overran — in which case the blocks since the snapshot are not all in hand
// and producing them would skip one.
func TestHandoff_RefusedWhenTheJoinCannotBeExact(t *testing.T) {
	svc, ca, _ := newJoiningService(t)

	require.Error(t, svc.performHandoff(10), "a node that is not joining has nothing to hand off")

	svc.StartCollecting()
	svc.mu.Lock()
	svc.bufferOverrun = true
	svc.mu.Unlock()
	err := svc.performHandoff(10)
	require.Error(t, err)
	require.True(t, errors.Is(err, errors.NotReady), "got %v", err)
	require.True(t, svc.Collecting(), "and it is still joining")
	require.Empty(t, ca.blocks)
}

// An adapter that cannot collect must refuse, not execute: a node that
// executed a block while joining would execute it from a staging its peers do
// not have, and its root chain would never match again (#4290).
func TestProcessCommittedGroup_CollectingRefusesAnExecutingAdapter(t *testing.T) {
	svc, ca, author := newCommitService(t, 1)
	w := svc.node.Workers()[0]
	svc.StartCollecting()

	b := types.NewBatch([][]byte{[]byte("tx")})
	require.NoError(t, w.StoreBatch(b))

	_, err := svc.processCommittedGroup(group(commitCert(author, 2, time.Unix(100, 0),
		[]types.PayloadEntry{{Digest: b.Digest(), Worker: w.ID()}})))
	require.Error(t, err)
	require.Empty(t, ca.blocks, "nothing may be executed while joining")
	require.Empty(t, svc.Buffered())
}

// The buffer is bounded: past the bound the blocks since the snapshot are no
// longer all in hand, so the join cannot be exact and says so rather than
// growing without limit (#4292; DIFFERENCES E11).
func TestProcessCommittedGroup_CollectingBoundsTheBuffer(t *testing.T) {
	svc, ca, author := newJoiningService(t)
	w := svc.node.Workers()[0]
	svc.StartCollecting()

	old := maxCollectedGroups
	maxCollectedGroups = 2
	defer func() { maxCollectedGroups = old }()

	var lastErr error
	for i := 0; i < 3; i++ {
		b := types.NewBatch([][]byte{{byte(i)}})
		require.NoError(t, w.StoreBatch(b))
		_, lastErr = svc.processCommittedGroup(group(commitCert(author, types.Round(2*i+2), time.Unix(int64(100+i), 0),
			[]types.PayloadEntry{{Digest: b.Digest(), Worker: w.ID()}})))
	}
	require.Error(t, lastErr, "the group past the bound is refused")
	require.True(t, svc.BufferOverrun(), "and the join knows it must start again")
	require.Len(t, svc.Buffered(), 2)
	require.Empty(t, ca.blocks)
}
