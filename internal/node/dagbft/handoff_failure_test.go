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
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/adapter"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// produceFailingAdapter fails every block it is asked to produce, as a
// freshly started executor does when its first block's cache seed reads a
// message the join never pulled (run 20260924T052134Z).
type produceFailingAdapter struct {
	collectingAdapter
	attempts int
}

func (a *produceFailingAdapter) ProduceBlock(_ context.Context, params adapter.BlockParams) ([32]byte, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.attempts++
	return [32]byte{}, errors.NotFound.With("seed synthetic cache: load Directory receipts: Message.c06fcb7d….Main not found")
}

// A handoff whose first buffered group fails to produce must leave the node
// where a join can start again: collecting, with the blocks it collected. It
// leaves it neither: performHandoff (collect.go) takes the buffer, clears
// collecting and sets lastBlockIndex BEFORE producing, so after the failure
// the node is not joining, the 22 collected blocks are gone, a second
// Handoff is refused "not joining", and every group committed from then on
// goes to produce, fails, and is dropped by the production loop -- 457
// "Failed to process committed group" lines on acc-bvn3-val1 in 8 minutes.
func TestHandoff_AFailedHandoffLeavesTheNodeAbleToJoinAgain(t *testing.T) {
	svc, _, author := newJoiningService(t)
	fa := &produceFailingAdapter{}
	svc.adapter = fa
	w := svc.node.Workers()[0]

	// The node stood at block 40, committed at round 1 (#4362: the handoff
	// reads the round from the pulled ledger).
	svc.lastBlockIndex = 40
	svc.lastLeaderRound = 1
	svc.StartCollecting()
	for i := 0; i < 3; i++ {
		b := types.NewBatch([][]byte{{byte(i)}})
		require.NoError(t, w.StoreBatch(b))
		_, err := svc.processCommittedGroup(group(commitCert(author, types.Round(2*i+2), time.Unix(int64(100+i), 0),
			[]types.PayloadEntry{{Digest: b.Digest(), Worker: w.ID()}})))
		require.NoError(t, err)
	}
	require.Len(t, svc.Buffered(), 3)

	pullState(t, svc, 40, 1)
	err := svc.performHandoff(40)
	require.Error(t, err, "precondition: the first buffered group cannot be produced")
	require.Equal(t, 1, fa.attempts)

	require.True(t, svc.Collecting(), "after a failed handoff the node must still be joining, or nothing can join it again")
	require.Len(t, svc.Buffered(), 3, "and must still hold what it collected, or a second handoff has nothing to produce")
	require.Equal(t, uint64(40), svc.lastBlockIndex, "and must still stand where it stood")
}

// produceFailingOnceAdapter fails the n-th block it is asked to produce and
// produces every other: the live run's executor failed its first block's
// cache seed and then went on producing (#4402).
type produceFailingOnceAdapter struct {
	collectingAdapter
	failAt   int // 1-based
	attempts int
}

func (a *produceFailingOnceAdapter) ProduceBlock(ctx context.Context, params adapter.BlockParams) ([32]byte, error) {
	a.mu.Lock()
	a.attempts++
	fail := a.attempts == a.failAt
	a.mu.Unlock()
	if fail {
		return [32]byte{}, errors.NotFound.With("seed synthetic cache: Message.….Main not found")
	}
	return a.collectingAdapter.ProduceBlock(ctx, params)
}

// Run 20260924T052134Z, acc-bvn2-val2's Directory (#4402): its handoff failed,
// and the service then produced the next committed leader rounds as blocks
// 1299 and 1300 on its own numbering (the peers': 1318, 1319) and signed
// Directory anchor 1154 for its 1300 where every other validator's 1154 is
// block 1301. After a failed handoff a node executes nothing — so it commits
// no block and publishes no block event, which is what anchoring runs from —
// until it has joined again; then it produces what it kept, under the
// numbers that are theirs (#4401).
func TestHandoff_AfterAFailedHandoffNothingIsExecutedUntilTheJoinHandsOffAgain(t *testing.T) {
	svc, _, author := newJoiningService(t)
	fa := &produceFailingOnceAdapter{failAt: 1}
	svc.adapter = fa
	var committed []events.DidCommitBlock
	events.SubscribeSync(svc.eventBus, func(e events.DidCommitBlock) error {
		committed = append(committed, e)
		return nil
	})
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
	commit(2)
	commit(4)
	commit(6)
	pullState(t, svc, 40, 1)
	require.NoError(t, svc.stageThroughNow(41))
	require.Len(t, fa.collected, 1, "block 41 is staged before the handoff")
	require.Error(t, svc.performHandoff(40), "precondition: block 41 fails to produce")

	// Groups committed after the failure are kept, not executed.
	commit(8)
	commit(10)
	require.Equal(t, 1, fa.attempts, "no block is produced after the failed handoff")
	require.Empty(t, fa.blocks)
	require.Empty(t, committed, "no block is committed, so nothing is anchored")
	require.True(t, svc.Collecting())
	require.Len(t, svc.Buffered(), 5, "the three it did not produce and the two since")
	require.Equal(t, uint64(40), svc.lastBlockIndex)
	require.NoError(t, svc.stageThroughNow(41))
	require.Len(t, fa.collected, 1, "a group put back in the buffer is not staged twice")

	// The join syncs again and hands off again: every kept group is
	// produced, in order, under its own number.
	require.NoError(t, svc.performHandoff(40))
	require.False(t, svc.Collecting())
	require.Len(t, fa.blocks, 5)
	for i, b := range fa.blocks {
		require.Equal(t, uint64(41+i), b.Index)
		require.Equal(t, types.Round(2*i+2), b.LeaderRound)
	}
	require.Len(t, committed, 5)
}

// A handoff that fails part way keeps the blocks it produced: the node stands
// at the last one, collecting, holding the groups after it.
func TestHandoff_AFailedHandoffKeepsWhatItProducedAndHoldsTheRest(t *testing.T) {
	svc, _, author := newJoiningService(t)
	fa := &produceFailingOnceAdapter{failAt: 2}
	svc.adapter = fa
	w := svc.node.Workers()[0]

	svc.lastBlockIndex = 40
	svc.lastLeaderRound = 1
	svc.StartCollecting()
	for i := 0; i < 3; i++ {
		b := types.NewBatch([][]byte{{byte(i)}})
		require.NoError(t, w.StoreBatch(b))
		_, err := svc.processCommittedGroup(group(commitCert(author, types.Round(2*i+2), time.Unix(int64(100+i), 0),
			[]types.PayloadEntry{{Digest: b.Digest(), Worker: w.ID()}})))
		require.NoError(t, err)
	}
	pullState(t, svc, 40, 1)
	require.Error(t, svc.performHandoff(40))

	require.Len(t, fa.blocks, 1, "block 41 was produced")
	require.True(t, svc.Collecting())
	require.Equal(t, uint64(41), svc.lastBlockIndex, "the node stands at the last block it produced")
	require.Equal(t, types.Round(2), svc.lastLeaderRound)
	buffered := svc.Buffered()
	require.Len(t, buffered, 2)
	require.Equal(t, types.Round(4), buffered[0].Round(), "the group that failed is kept")
	require.Equal(t, types.Round(6), buffered[1].Round())
}
