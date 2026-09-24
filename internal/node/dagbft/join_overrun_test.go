// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// A join whose buffer overruns while it converges starts collecting again and
// hands off from a newer block (executor spec, "Sync", step 5; #4407).
//
// join.Run drives the production Service here: its StartCollecting,
// BufferOverrun, StageThrough and Handoff, the last two served by the block
// production loop as the daemon runs it. Only consensus is a stand-in — the
// network's committed groups are handed to processCommittedGroup, as the loop
// hands them — together with the pull, the peers and the gap check, which are
// not what this is about. Groups are delivered from inside Pull, on the join's
// goroutine, so they never run beside a StageThrough or a Handoff, as in
// production where one loop does both.
//
// test/e2e/join_overrun_resume_test.go drives Run against a fake buffer that
// restarts on StartCollecting after an overrun. The production Service
// returned early while collecting, so converge's overrun branch asked it to
// collect again, found it still overrun, and waited for ever without pulling.
func TestJoinRun_AnOverrunDuringTheJoinResumesFromANewerBlock(t *testing.T) {
	// The pull either reaches the network's block at once, or, for the
	// first two pulls, leaves the state where the node stood: a restarted
	// node's own root is a proven root. Then the first state the join
	// matches after the buffer starts again is one the groups it lost are
	// not in.
	for _, stay := range []int{0, 2} {
		t.Run(fmt.Sprintf("state stays %d pulls", stay), func(t *testing.T) {
			joinThroughAnOverrun(t, stay)
		})
	}
}

func joinThroughAnOverrun(t *testing.T, stay int) {
	svc, ca, author := newJoiningService(t)
	w := svc.node.Workers()[0]
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	svc.ctx = ctx

	old := maxCollectedGroups
	maxCollectedGroups = 3
	defer func() { maxCollectedGroups = old }()

	// The node restarted at block 40, committed at round 1. The network's
	// block n > 40 is committed at round 2(n-40).
	const stood = 40
	svc.lastBlockIndex = stood
	svc.lastLeaderRound = 1
	roundOf := func(n uint64) types.Round {
		if n == stood {
			return 1
		}
		return types.Round(2 * (n - stood))
	}

	// The daemon starts collecting before the service starts.
	svc.StartCollecting()
	svc.wg.Add(1)
	go svc.blockProductionLoop()
	defer func() { cancel(); svc.wg.Wait() }()

	net := &overrunNetwork{head: stood, matched: stood}
	net.commit = func() {
		net.head++
		b := types.NewBatch([][]byte{{byte(net.head)}})
		require.NoError(t, w.StoreBatch(b))
		// A group past the bound is refused, and the loop logs it and
		// goes on; so does this.
		_, _ = svc.processCommittedGroup(group(commitCert(author, roundOf(net.head), time.Unix(int64(net.head), 0),
			[]types.PayloadEntry{{Digest: b.Digest(), Worker: w.ID()}})))
	}
	// The first pull takes long enough for the network to commit more than
	// the buffer holds; each one after, one block. A pull reaches the block
	// before the network's head, and leaves that block's system ledger.
	net.pull = func() {
		n := 1
		if net.pulls == 0 {
			n = maxCollectedGroups + 2
		}
		net.pulls++
		for i := 0; i < n; i++ {
			net.commit()
		}
		if net.pulls > stay {
			net.matched = net.head - 1
		}
		pullState(t, svc, net.matched, roundOf(net.matched))
	}

	outcome, err := join.Run(ctx, join.Options{
		Partition: "bvn1",
		Buffer:    svc,
		Stage:     new(overrunStage),
		State:     net,
		Peers:     overrunPeers{},
		Logger:    slog.New(slog.DiscardHandler),
		Retry:     time.Millisecond,
	})
	require.NoError(t, err, "the join did not hand off after its buffer overran (%d pulls, network at block %d, overrun %v)",
		net.pulls, net.head, svc.BufferOverrun())
	require.Equal(t, join.Joined, outcome)
	require.False(t, svc.Collecting(), "a node that has joined is not collecting")

	require.NotEmpty(t, ca.blocks, "the join handed off from the buffer it started again")
	for _, b := range ca.blocks {
		require.Greater(t, b.Index, uint64(stood+maxCollectedGroups),
			"block %d was produced under a number from before the overrun: the groups lost with the buffer are not in it", b.Index)
		require.Equal(t, roundOf(b.Index), b.LeaderRound,
			"block %d is the group committed at round %d, not %d: the join numbered the new buffer from the wrong block",
			b.Index, roundOf(b.Index), b.LeaderRound)
	}
	require.Equal(t, net.head, svc.lastBlockIndex, "the node stands at the network's block")
}

// overrunNetwork is the join's state: pulls run the network on and leave the
// system ledger of the block before its head.
type overrunNetwork struct {
	head    uint64
	matched uint64
	pulls   int
	commit  func()
	pull    func()
}

func (n *overrunNetwork) Pull(context.Context) error { n.pull(); return nil }

func (n *overrunNetwork) Promote(uint64) {}
func (n *overrunNetwork) Demote(uint64)  {}

func (n *overrunNetwork) Matched(context.Context) (uint64, bool, error) {
	return n.matched, n.pulls > 0, nil
}

// overrunStage finds no gap and settles nothing: staging is not this test's.
type overrunStage struct{}

func (overrunStage) SettleStagingAt(uint64) error { return nil }
func (overrunStage) HasGap(uint64) (bool, error)  { return false, nil }

type overrunPeers struct{}

func (overrunPeers) Validators(context.Context) ([]*api.FindServiceResult, error) {
	return []*api.FindServiceResult{{}}, nil
}

// A buffer started again after an overrun has lost the groups committed
// between where the node stood and the restart: they were refused, or thrown
// away with the buffer. A state at the round the node stood at is then behind
// what the node can produce from, like a state below it, and handing off there
// would number the new buffer's first group as the block after it (#4407).
// The node can hand off only at a state that holds every group it lost.
func TestStartCollecting_AfterAnOverrunTheLostGroupsAreInNoState(t *testing.T) {
	svc, ca, author := newJoiningService(t)
	w := svc.node.Workers()[0]
	commit := func(round int) {
		t.Helper()
		b := types.NewBatch([][]byte{{byte(round)}})
		require.NoError(t, w.StoreBatch(b))
		_, _ = svc.processCommittedGroup(group(commitCert(author, types.Round(round), time.Unix(int64(100+round), 0),
			[]types.PayloadEntry{{Digest: b.Digest(), Worker: w.ID()}})))
	}

	old := maxCollectedGroups
	maxCollectedGroups = 2
	defer func() { maxCollectedGroups = old }()

	svc.lastBlockIndex = 40
	svc.lastLeaderRound = 1
	svc.StartCollecting()
	commit(2)
	commit(4)
	commit(6) // refused: the buffer overruns
	require.True(t, svc.BufferOverrun(), "precondition")

	svc.StartCollecting()
	require.False(t, svc.BufferOverrun(), "collecting again after an overrun starts a new buffer")
	require.Empty(t, svc.Buffered())
	commit(8)
	commit(10)

	// Block 40 at round 1 is where the node stood; rounds 2 to 6 are in
	// neither it nor the buffer.
	pullState(t, svc, 40, 1)
	err := svc.performHandoff(40)
	require.True(t, errors.Is(err, errors.Conflict), "got %v", err)
	pullState(t, svc, 42, 4)
	err = svc.performHandoff(42)
	require.True(t, errors.Is(err, errors.Conflict), "got %v", err)
	require.True(t, svc.Collecting())
	require.Empty(t, ca.blocks, "round 8 is not block 41, nor 43")

	// Block 43 at round 6 holds every group the node lost: round 8 is 44.
	pullState(t, svc, 43, 6)
	require.NoError(t, svc.performHandoff(43))
	require.Len(t, ca.blocks, 2)
	require.Equal(t, uint64(44), ca.blocks[0].Index)
	require.Equal(t, types.Round(8), ca.blocks[0].LeaderRound)
}
