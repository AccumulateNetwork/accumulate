// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
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

	// streams is what each collected block reports on its streams.
	streams []execute.CollectedStream
}

func (a *collectingAdapter) CollectBlock(_ context.Context, params adapter.BlockParams) (*execute.CollectedBlock, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.collected = append(a.collected, params)
	return &execute.CollectedBlock{Held: len(params.Batches), Streams: a.streams}, nil
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
		Database:   database.OpenInMemory(nil),
	})
	require.NoError(t, err)
	svc.node = node
	svc.committee = committee
	svc.ctx = context.Background()
	return svc, ca, pub
}

// pullState puts in the service's database the system ledger a join's pull
// leaves there: the block the pulled state is and the leader round that
// committed it. The handoff reads both from it (#4362).
func pullState(t *testing.T, svc *Service, index uint64, round types.Round) {
	t.Helper()
	u := protocol.PartitionUrl(svc.config.Partition.ID).JoinPath(protocol.Ledger)
	require.NoError(t, svc.config.Database.Update(func(batch *database.Batch) error {
		return batch.Account(u).Main().Put(&protocol.SystemLedger{Url: u, Index: index, LeaderRound: uint64(round)})
	}))
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
	require.Empty(t, ca.collected,
		"and stages nothing as it collects: the blocks are buffered (#4398)")

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

	// The pull reached block 41, which is the first of them: it is in the
	// state already, so only 42 and 43 are produced.
	pullState(t, svc, 41, 2)
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

	// The node stood at block 40, committed at round 10, and collects the
	// group committed at round 12.
	svc.lastBlockIndex = 40
	svc.lastLeaderRound = 10
	svc.StartCollecting()
	b := types.NewBatch([][]byte{{1}})
	require.NoError(t, w.StoreBatch(b))
	_, err := svc.processCommittedGroup(group(commitCert(author, 12, time.Unix(100, 0),
		[]types.PayloadEntry{{Digest: b.Digest(), Worker: w.ID()}})))
	require.NoError(t, err)

	pullState(t, svc, 45, 20)
	err = svc.performHandoff(45)
	require.Error(t, err)
	require.True(t, errors.Is(err, errors.NotReady), "got %v", err)
	require.True(t, svc.Collecting(), "and it is still joining")
	require.Len(t, svc.Buffered(), 1, "with its buffer intact")
	require.Empty(t, ca.blocks)

	// A block behind where the node stood is refused outright.
	pullState(t, svc, 39, 8)
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

// The daemon starts collecting before the service starts, and the join starts
// collecting again when it runs. Neither may cost the buffer its map onto
// blocks (#4351): the block the node stands at is only known once Start has
// restored it, and a group collected between Start and the join's call is
// block R + 1 whoever calls StartCollecting after it.
func TestHandoff_StartingToCollectTwiceKeepsTheMapOntoBlocks(t *testing.T) {
	svc, ca, author := newJoiningService(t)
	w := svc.node.Workers()[0]

	// cmd/accumulated/run/dagbft.go: StartCollecting, then Start, which
	// restores the block the node stands at.
	svc.StartCollecting()
	svc.lastBlockIndex = 40

	// A group is committed before the join's goroutine runs: block 41.
	b := types.NewBatch([][]byte{{1}})
	require.NoError(t, w.StoreBatch(b))
	_, err := svc.processCommittedGroup(group(commitCert(author, 2, time.Unix(100, 0),
		[]types.PayloadEntry{{Digest: b.Digest(), Worker: w.ID()}})))
	require.NoError(t, err)

	// join.Run starts collecting too.
	svc.StartCollecting()
	require.Len(t, svc.Buffered(), 1, "what was collected is kept")

	b = types.NewBatch([][]byte{{2}})
	require.NoError(t, w.StoreBatch(b))
	_, err = svc.processCommittedGroup(group(commitCert(author, 4, time.Unix(101, 0),
		[]types.PayloadEntry{{Digest: b.Digest(), Worker: w.ID()}})))
	require.NoError(t, err)

	// The state is block 41: only block 42, the second group, is produced.
	pullState(t, svc, 41, 2)
	require.NoError(t, svc.performHandoff(41))
	require.Len(t, ca.blocks, 1)
	require.Equal(t, uint64(42), ca.blocks[0].Index)
	require.Equal(t, types.Round(4), ca.blocks[0].LeaderRound, "the group committed second is block 42")
}

// A node whose executed block's root differs from its proven root collects
// again and syncs again (executor spec, "Sync", step 4). The second collection
// maps onto the blocks after the one the node had executed to, and the second
// handoff produces only what the newer state does not have.
func TestHandoff_CollectingAgainAfterAHandoffMapsOntoTheBlocksAfterIt(t *testing.T) {
	svc, ca, author := newJoiningService(t)
	w := svc.node.Workers()[0]
	commit := func(round int, tag byte) {
		t.Helper()
		b := types.NewBatch([][]byte{{tag}})
		require.NoError(t, w.StoreBatch(b))
		_, err := svc.processCommittedGroup(group(commitCert(author, types.Round(round), time.Unix(int64(100+round), 0),
			[]types.PayloadEntry{{Digest: b.Digest(), Worker: w.ID()}})))
		require.NoError(t, err)
	}

	// Joined at 40, committed at round 1, then executed 41 and 42.
	svc.lastBlockIndex = 40
	svc.lastLeaderRound = 1
	svc.StartCollecting()
	pullState(t, svc, 40, 1)
	require.NoError(t, svc.performHandoff(40))
	commit(2, 1)
	commit(4, 2)
	require.Len(t, ca.blocks, 2)
	require.Equal(t, uint64(42), svc.lastBlockIndex)

	// Block 42 diverged: the join collects again. Blocks 43, 44 and 45 are
	// collected, none executed.
	svc.StartCollecting()
	commit(6, 3)
	commit(8, 4)
	commit(10, 5)
	require.Len(t, ca.blocks, 2, "a node syncing again executes nothing")
	require.Len(t, svc.Buffered(), 3)

	// A pull behind where the node stood cannot be handed off at: those
	// blocks are not in the buffer.
	pullState(t, svc, 41, 2)
	err := svc.performHandoff(41)
	require.True(t, errors.Is(err, errors.Conflict), "got %v", err)
	require.True(t, svc.Collecting())

	// The peers' state is block 44: only 45 is produced.
	pullState(t, svc, 44, 8)
	require.NoError(t, svc.performHandoff(44))
	require.False(t, svc.Collecting())
	require.Len(t, ca.blocks, 3)
	require.Equal(t, uint64(45), ca.blocks[2].Index)
	require.Equal(t, types.Round(10), ca.blocks[2].LeaderRound, "the group committed third since collecting again is block 45")
	require.Equal(t, uint64(45), svc.lastBlockIndex)
}

// Staging at the handoff holds everything collected through Q + 1 and nothing
// after it (executor spec, "Sync", step 5; #4398). The join stages through
// Q + 1 once the state matches at Q; the groups after Q + 1 are only buffered,
// and reach staging by being produced, as they reach a peer's.
//
// Run 20260924T052134Z: every collected group went into staging as it
// arrived, so a BVN node that joined at 203 with eight groups buffered
// executed 204 holding what 205-211 brought, delivered more than its peers
// and diverged on its first block.
func TestStageThrough_StagesThroughTheBlockAfterTheStateAndNoneAfterIt(t *testing.T) {
	svc, ca, author := newJoiningService(t)
	w := svc.node.Workers()[0]
	var digests []types.BatchDigest
	commit := func(round int) {
		t.Helper()
		b := types.NewBatch([][]byte{{byte(round)}})
		require.NoError(t, w.StoreBatch(b))
		digests = append(digests, b.Digest())
		_, err := svc.processCommittedGroup(group(commitCert(author, types.Round(round), time.Unix(int64(100+round), 0),
			[]types.PayloadEntry{{Digest: b.Digest(), Worker: w.ID()}})))
		require.NoError(t, err)
	}

	// The node stood at block 40, round 1, and collects blocks 41-44.
	svc.lastBlockIndex = 40
	svc.lastLeaderRound = 1
	svc.StartCollecting()
	commit(2)
	commit(4)
	commit(6)
	commit(8)
	require.Empty(t, ca.collected, "collecting stages nothing")

	// The state matched at 41 (round 2): staging takes 41 and 42, the block
	// after the state, and not 43 or 44.
	pullState(t, svc, 41, 2)
	require.NoError(t, svc.stageThroughNow(42))
	require.Len(t, ca.collected, 2, "staged through the block after the state and no further")
	require.Equal(t, digests[0], ca.collected[0].Batches[0].Digest(), "in the order consensus committed them")
	require.Equal(t, digests[1], ca.collected[1].Batches[0].Digest())
	require.Equal(t, types.Round(4), ca.collected[1].LeaderRound, "the block after the state is the first group above its round")
	require.Equal(t, uint64(0), ca.collected[0].Index, "a collected block has no index until the handoff")
	require.Equal(t, time.Unix(102, 0).UTC(), ca.collected[0].Time, "the block time is the leader's, unclamped")

	// A group committed now is only buffered.
	commit(10)
	require.Len(t, ca.collected, 2, "a group collected after the block after the state is not staged")

	// Asked again for the same block, staging does not take anything twice.
	require.NoError(t, svc.stageThroughNow(42))
	require.Len(t, ca.collected, 2)

	// The sync advanced to 42 (a gap at 42): staging takes 43 and only 43.
	pullState(t, svc, 42, 4)
	require.NoError(t, svc.stageThroughNow(43))
	require.Len(t, ca.collected, 3)
	require.Equal(t, digests[2], ca.collected[2].Batches[0].Digest())

	// The handoff at 42 produces 43, 44 and 45; nothing more is staged by
	// collecting, and the groups after 43 reach staging by being produced.
	require.NoError(t, svc.performHandoff(42))
	require.Len(t, ca.collected, 3, "the handoff stages nothing: it produces")
	require.Len(t, ca.blocks, 3)
	require.Equal(t, uint64(43), ca.blocks[0].Index)
	require.Equal(t, types.Round(6), ca.blocks[0].LeaderRound)
	require.Equal(t, uint64(45), ca.blocks[2].Index)
}

// The block after the state not collected yet is a wait: the gap check would
// not see what it carries. A state that is not this node's to hand off from is
// refused as the handoff refuses it. Neither stages anything.
func TestStageThrough_WaitsForTheBlockAfterTheStateAndRefusesAStateItCannotHandOffFrom(t *testing.T) {
	svc, ca, author := newJoiningService(t)
	w := svc.node.Workers()[0]
	svc.lastBlockIndex = 40
	svc.lastLeaderRound = 1
	svc.StartCollecting()
	b := types.NewBatch([][]byte{{1}})
	require.NoError(t, w.StoreBatch(b))
	_, err := svc.processCommittedGroup(group(commitCert(author, 2, time.Unix(100, 0),
		[]types.PayloadEntry{{Digest: b.Digest(), Worker: w.ID()}})))
	require.NoError(t, err)

	// The state is 41, the only group collected; 42 has not arrived.
	pullState(t, svc, 41, 2)
	err = svc.stageThroughNow(42)
	require.True(t, errors.Is(err, errors.NotReady), "got %v", err)
	require.Empty(t, ca.collected)

	// A ledger that is not block q is not the state q is.
	err = svc.stageThroughNow(43)
	require.True(t, errors.Is(err, errors.Conflict), "got %v", err)

	// A state behind where the node stood.
	pullState(t, svc, 39, 1)
	svc.lastLeaderRound = 2
	err = svc.stageThroughNow(40)
	require.True(t, errors.Is(err, errors.Conflict), "got %v", err)
	require.Empty(t, ca.collected)
	require.True(t, svc.Collecting())
	require.Len(t, svc.Buffered(), 1)
}

// A handoff with nothing buffered after the state still moves the node to the
// state's round (#4362 review F3, note_3896023512). The node stands at the
// state's block and round whether or not it has anything to produce; a node
// that then syncs again must refuse a state behind that round as Conflict —
// the groups between were committed before it collected again — rather than
// wait for them as though they were still on their way.
func TestHandoff_AnEmptyTailHandoffStandsAtTheStatesRound(t *testing.T) {
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

	// The node stood at 40, round 1, and collected 41 and 42 (rounds 2, 4).
	svc.lastBlockIndex = 40
	svc.lastLeaderRound = 1
	svc.StartCollecting()
	commit(2)
	commit(4)

	// The state is 42: everything collected is in it, nothing to produce.
	pullState(t, svc, 42, 4)
	require.NoError(t, svc.performHandoff(42))
	require.Empty(t, ca.blocks)
	require.Equal(t, uint64(42), svc.lastBlockIndex)
	require.Equal(t, types.Round(4), svc.lastLeaderRound, "the node stands at the state's round")

	// It syncs again. A state at 41 is behind where it stands.
	svc.StartCollecting()
	pullState(t, svc, 41, 2)
	err := svc.performHandoff(41)
	require.True(t, errors.Is(err, errors.Conflict), "got %v", err)
}

// Run 20260924T111811Z: "Staged the buffered groups through the block after
// the state … notStaged=5 … staged=1", then a gap at the same block that the
// log could not place (#4432). The line says, per stream, how many arrivals
// the staged groups held and why the rest were not held, and says that the
// groups after the block are not staged because they reach staging only by
// being produced.
func TestStageThrough_SaysPerStreamWhatWasStagedAndWhatWasNot(t *testing.T) {
	svc, ca, author := newJoiningService(t)
	var out bytes.Buffer
	svc.logger.L = logging.NewSlogLogger(slog.New(slog.NewTextHandler(&out, nil)))

	stream := execute.StreamID{
		Ledger: protocol.PartitionUrl("bvn1").JoinPath(protocol.Synthetic),
		Source: protocol.PartitionUrl("BVN0"),
	}
	ca.streams = []execute.CollectedStream{{
		ID:      stream,
		Held:    3,
		NotHeld: map[execute.NotHeldReason]int{execute.NotHeldDelivered: 1, execute.NotHeldUnattested: 2},
	}}

	w := svc.node.Workers()[0]
	commit := func(round int) {
		t.Helper()
		b := types.NewBatch([][]byte{{byte(round)}})
		require.NoError(t, w.StoreBatch(b))
		_, err := svc.processCommittedGroup(group(commitCert(author, types.Round(round), time.Unix(int64(100+round), 0),
			[]types.PayloadEntry{{Digest: b.Digest(), Worker: w.ID()}})))
		require.NoError(t, err)
	}
	svc.lastBlockIndex = 40
	svc.lastLeaderRound = 1
	svc.StartCollecting()
	for _, r := range []int{2, 4, 6, 8, 10} {
		commit(r)
	}

	// 41 and 42 staged, both reporting the stream; 43-45 after the block.
	pullState(t, svc, 41, 2)
	require.NoError(t, svc.stageThroughNow(42))
	line := out.String()
	require.Contains(t, line, "Staged the buffered groups through the block after the state")
	require.Contains(t, line, "staged=2")
	require.Contains(t, line, "after=3", "three groups are after the block: staged only by being produced")
	require.Contains(t, line, "bvn-BVN0.acme->bvn-bvn1.acme/synthetic held=6 delivered=2 unattested=4",
		"the stream's arrivals over both groups: held, and why the rest were not")
}
