// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// These drive the convergence loop's state machine (executor spec, "Sync",
// §4 and §5): sync to B, ask whether the block collected as B+1 can be
// executed, and either hand off or advance.
//
// What they do NOT prove is that the pull converges or that the gap check
// reads what a block carried -- those are the state half and the executor's,
// and they are proven where they live: test/e2e/restart_rejoin_pull_test.go
// drives the pull through the production wiring against a restarted node's
// own database, and internal/core/execute/v2/block's collect tests drive the
// gap check through CollectBlock.

// fakeBuffer stands for the DAG service's collecting mode.
type fakeBuffer struct {
	collecting bool
	overrun    bool
	handedOff  uint64

	// gapsUntil is the last synced block at which the collected block b+1
	// still has a gap. Above it the block is clean.
	gapsUntil uint64

	// notCollectedAbove is the block above which nothing has been collected
	// yet, so GapsAt is a wait rather than an answer. Zero means everything.
	notCollectedAbove uint64

	asked []uint64

	onHandoff func()
}

func (b *fakeBuffer) Collecting() bool    { return b.collecting }
func (b *fakeBuffer) BufferOverrun() bool { return b.overrun }

func (b *fakeBuffer) GapsAt(q uint64) ([]execute.StreamGap, error) {
	b.asked = append(b.asked, q)
	if b.notCollectedAbove != 0 && q >= b.notCollectedAbove {
		return nil, errors.NotReady.WithFormat("block %d has not been collected", q+1)
	}
	if q <= b.gapsUntil {
		return []execute.StreamGap{{
			ID:        execute.StreamID{Ledger: protocol.PartitionUrl("BVN0").JoinPath(protocol.Synthetic), Source: protocol.PartitionUrl("BVN1")},
			Delivered: 103,
			Through:   105,
			Missing:   [][2]uint64{{104, 104}},
		}}, nil
	}
	return nil, nil
}

func (b *fakeBuffer) Handoff(q uint64) error {
	if b.onHandoff != nil {
		b.onHandoff()
	}
	b.handedOff = q
	b.collecting = false
	return nil
}

// fakeStage stands for the executor's staging half.
type fakeStage struct {
	settled  uint64
	onSettle func()
}

func (s *fakeStage) SettleStagingAt(q uint64) error {
	if s.onSettle != nil {
		s.onSettle()
	}
	s.settled = q
	return nil
}

// fakeState stands for the state pull. It reports a synced block once enough
// rounds have run, and Advance moves it to the next anchored block.
type fakeState struct {
	pulls     int
	matchAt   uint64
	matchFrom int // the pull round at which the root matches
	step      uint64
	advances  int
	handedOff uint64
	handoffs  int
	onHandoff func()
}

func (s *fakeState) Pull(context.Context) error { s.pulls++; return nil }

func (s *fakeState) Matched(context.Context) (uint64, bool, error) {
	if s.pulls < s.matchFrom {
		return 0, false, nil
	}
	return s.matchAt, true, nil
}

func (s *fakeState) Advance() {
	s.advances++
	step := s.step
	if step == 0 {
		step = 6
	}
	s.matchAt += step
}

func (s *fakeState) Handoff(_ context.Context, q uint64) error {
	if s.onHandoff != nil {
		s.onHandoff()
	}
	s.handoffs++
	s.handedOff = q
	return nil
}

func run(t *testing.T, opts Options) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	opts.Retry = time.Millisecond
	return Run(ctx, opts)
}

// The join pulls until the root matches an anchored block, finds no gap in
// the block after it, and hands off there.
func TestJoin_SyncsThenExecutesTheBlockAfter(t *testing.T) {
	buf := &fakeBuffer{collecting: true}
	stage := new(fakeStage)
	state := &fakeState{matchAt: 20, matchFrom: 2}

	require.NoError(t, run(t, Options{Partition: "BVN0", Buffer: buf, Stage: stage, State: state}))

	require.Equal(t, uint64(20), buf.handedOff, "handed off at the block the state is")
	require.Equal(t, uint64(20), stage.settled, "staging settled at the same block")
	require.GreaterOrEqual(t, state.pulls, 2)
	require.Zero(t, state.advances, "nothing had a gap, so nothing advanced")
}

// TEST (b): a peer holds an entry from before this node started listening.
//
// The node has block B's state and the block collected as B+1 delivers #105
// while the pulled state says 103 was delivered and nothing holds #104. That
// entry arrived before this node was listening, and executing B+1 without it
// delivers a shorter run than the peers do -- the #4290 divergence.
//
// So it does not execute. It advances the sync to the next anchored block,
// where everything it lacks below is at or under the new Delivered, and asks
// the same question of the block after THAT one. It executes the first block
// with no gap, and it settles staging at the block it executes from.
func TestJoin_AGapAdvancesTheSyncInsteadOfExecuting(t *testing.T) {
	buf := &fakeBuffer{collecting: true, gapsUntil: 30}
	stage := new(fakeStage)
	state := &fakeState{matchAt: 20, matchFrom: 1, step: 6}

	require.NoError(t, run(t, Options{Partition: "BVN0", Buffer: buf, Stage: stage, State: state}))

	// 20, 26, 32: the first two had a gap and the third did not.
	require.Equal(t, []uint64{20, 26, 32}, buf.asked)
	require.Equal(t, 2, state.advances, "it advanced once per gap and no more")
	require.Equal(t, uint64(32), buf.handedOff, "it executed from the first block with no gap")
	require.Equal(t, uint64(32), stage.settled, "staging is settled at the block it executes from, not at the first one it synced to")
}

// A gap never ends in executing anyway. While the block after the state has a
// gap the node hands off nothing, whatever else happens.
func TestJoin_AGapNeverExecutes(t *testing.T) {
	buf := &fakeBuffer{collecting: true, gapsUntil: 1 << 40}
	stage := new(fakeStage)
	state := &fakeState{matchAt: 20, matchFrom: 1, step: 6}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err := Run(ctx, Options{Partition: "BVN0", Buffer: buf, Stage: stage, State: state, Retry: time.Millisecond})
	require.Error(t, err, "the join cannot finish while every block after the state has a gap")
	require.Zero(t, buf.handedOff)
	require.Zero(t, stage.settled)
	require.True(t, buf.collecting, "a node that cannot execute keeps collecting, which is the safe state")
	require.Greater(t, state.advances, 3, "it kept advancing rather than giving up or executing")
}

// The state can run ahead of the blocks consensus has delivered. That is a
// wait, not a gap: nothing advances and nothing is handed off.
func TestJoin_WaitsForTheBlockAfterTheStateToBeCollected(t *testing.T) {
	buf := &fakeBuffer{collecting: true, notCollectedAbove: 20}
	stage := new(fakeStage)
	state := &fakeState{matchAt: 20, matchFrom: 1}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := Run(ctx, Options{Partition: "BVN0", Buffer: buf, Stage: stage, State: state, Retry: time.Millisecond})
	require.Error(t, err)
	require.Zero(t, buf.handedOff)
	require.Zero(t, state.advances, "waiting for a block is not a gap and must not advance the sync")
}

// TEST (d), the ordering half: the pulled definition is published BEFORE the
// first block is executed. With #4301 (c) that is the only way a definition
// moves, so a handoff that produced blocks first would run them on the
// committee this node had before it went away (#4366's M3).
func TestJoin_PublishesWhatItPulledBeforeItExecutes(t *testing.T) {
	buf := &fakeBuffer{collecting: true}
	stage := new(fakeStage)
	state := &fakeState{matchAt: 20, matchFrom: 1}

	var order []string
	buf.onHandoff = func() { order = append(order, "produce") }
	state.onHandoff = func() { order = append(order, "publish") }
	stage.onSettle = func() { order = append(order, "settle") }

	require.NoError(t, run(t, Options{Partition: "BVN0", Buffer: buf, Stage: stage, State: state}))
	require.Equal(t, []string{"publish", "settle", "produce"}, order)
	require.Equal(t, uint64(20), state.handedOff)
}

// A buffer that overran can no longer say which collected group is which
// block, and guessing is the divergence the join exists to prevent. It is a
// fault, and the node keeps collecting rather than executing.
func TestJoin_ABufferOverrunIsAFaultNotARetry(t *testing.T) {
	buf := &fakeBuffer{collecting: true, overrun: true}
	stage := new(fakeStage)
	state := &fakeState{matchAt: 20, matchFrom: 1}

	err := run(t, Options{Partition: "BVN0", Buffer: buf, Stage: stage, State: state})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no longer all in hand")
	require.Zero(t, buf.handedOff)
	require.True(t, buf.collecting)
}

// A join needs all three halves. There is no "execute anyway" and there must
// not be one: only a node that has executed no block may execute without
// asking anyone, and such a node does not join at all (#4304).
func TestJoin_NeedsEveryHalf(t *testing.T) {
	require.Error(t, run(t, Options{Partition: "BVN0"}))
	require.Error(t, run(t, Options{Partition: "BVN0", Buffer: &fakeBuffer{}, Stage: new(fakeStage)}))
}
