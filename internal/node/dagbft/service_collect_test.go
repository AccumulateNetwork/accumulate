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
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
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

func (a *collectingAdapter) CollectBlock(_ context.Context, params adapter.BlockParams) (*execute.CollectedBlock, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.collected = append(a.collected, params)
	return &execute.CollectedBlock{Held: len(params.Batches)}, nil
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

	// A joining node collects into ITS OWN stage from the moment it starts
	// listening. There is no peer's stage to load into first and nothing to
	// wait for before holding what arrives: no peer is ever asked what it
	// holds (#4322).
	require.Len(t, ca.collected, 2, "every group is taken into staging as it arrives")
	require.Equal(t, b1.Digest(), ca.collected[0].Batches[0].Digest(), "in the order consensus committed them")
	require.Equal(t, b2.Digest(), ca.collected[1].Batches[0].Digest())
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

// The handoff: at the block the join matched, the node leaves collecting mode,
// stands at that block, and produces the buffered groups the block it stands
// at does NOT already contain, from the next block on (executor spec, "Sync",
// step 4; #4294).
//
// The buffered groups are blocks in order from where the node stood when it
// started collecting. A state that already contains some of them must not have
// them produced again: producing the first of them as block q+1 would execute
// an old block's transactions against a newer state under a number that is not
// theirs.
func TestHandoff_ProducesOnlyWhatTheStateDoesNotHave(t *testing.T) {
	svc, ca, author := newJoiningService(t)
	w := svc.node.Workers()[0]

	// The node stands at block 40 and starts collecting: the groups it
	// buffers are blocks 41, 42 and 43.
	svc.lastBlockIndex = 40
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

	require.Equal(t, []uint64{41, 42, 43}, []uint64{
		svc.Buffered()[0].Block, svc.Buffered()[1].Block, svc.Buffered()[2].Block},
		"each group is STAMPED with its block when it arrives, from the block the "+
			"node's own executor reached -- not counted at the handoff (#4351)")

	// The pull reached block 41, which is the first of them: it is in the
	// state already, so only 42 and 43 are produced.
	require.NoError(t, svc.performHandoff(41))

	require.False(t, svc.Collecting(), "a node that has joined is not collecting")
	require.Empty(t, svc.Buffered(), "the buffer is spent")
	require.Len(t, ca.blocks, 2, "the block the state already contains is not produced again")
	require.Equal(t, uint64(42), ca.blocks[0].Index)
	require.Equal(t, uint64(43), ca.blocks[1].Index)
	require.Equal(t, types.Round(4), ca.blocks[0].LeaderRound, "in the order consensus committed them")
	require.Equal(t, types.Round(6), ca.blocks[1].LeaderRound)
	require.Equal(t, uint64(43), svc.lastBlockIndex)

	// And the next committed group is produced, not collected.
	b := types.NewBatch([][]byte{[]byte("after")})
	require.NoError(t, w.StoreBatch(b))
	_, err := svc.processCommittedGroup(group(commitCert(author, 8, time.Unix(200, 0),
		[]types.PayloadEntry{{Digest: b.Digest(), Worker: w.ID()}})))
	require.NoError(t, err)
	require.Len(t, ca.blocks, 3)
	require.Equal(t, uint64(44), ca.blocks[2].Index)
}

// A handoff at a block the node has not collected through yet is not a
// handoff: the blocks between are still on their way, and producing what comes
// after them would give those blocks the wrong numbers. The join waits.
func TestHandoff_WaitsForTheBlocksItHasNotCollected(t *testing.T) {
	svc, ca, author := newJoiningService(t)
	w := svc.node.Workers()[0]

	svc.lastBlockIndex = 40
	svc.StartCollecting()
	b := types.NewBatch([][]byte{{1}})
	require.NoError(t, w.StoreBatch(b))
	_, err := svc.processCommittedGroup(group(commitCert(author, 2, time.Unix(100, 0),
		[]types.PayloadEntry{{Digest: b.Digest(), Worker: w.ID()}})))
	require.NoError(t, err)

	err = svc.performHandoff(45)
	require.Error(t, err)
	require.True(t, errors.Is(err, errors.NotReady), "got %v", err)
	require.True(t, svc.Collecting(), "and it is still joining")
	require.Len(t, svc.Buffered(), 1, "with its buffer intact")
	require.Empty(t, ca.blocks)

	// A block behind where the node stood is refused outright: every group
	// the buffer holds is above it, so the group numbered 40 is not there
	// and the blocks between were never collected.
	err = svc.performHandoff(39)
	require.Error(t, err)
	require.True(t, errors.Is(err, errors.Conflict), "got %v", err)
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
