// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/bullshark"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/persist"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// An executor that takes a checkpoint for the oldest group handed to it, as
// Service.produce does, only when ordering has stopped to wait for it. The
// feeder runs on its own goroutine because ordering may block.
type stalledExec struct {
	t    *testing.T
	r    *dedupRig
	n    *Node
	live map[types.Round][]string // written by the feeder only, read after done
	mu   sync.Mutex
	cps  map[types.Round]*persist.Checkpoint
	at   map[types.Round]types.Round // frontier round when each checkpoint was taken
	// maxLead is the greatest distance from the frontier to the group
	// checkpointed that the executor saw.
	maxLead types.Round
}

func (e *stalledExec) run(dag [][]*types.Certificate) {
	var done atomic.Bool
	var round atomic.Int64
	go func() {
		defer done.Store(true)
		for i, cs := range dag {
			e.r.feed(e.n, e.live, cs...)
			round.Store(int64(i))
		}
	}()
	last := types.Round(1 << 62)
	for !done.Load() {
		time.Sleep(5 * time.Millisecond)
		if lc := e.n.bullshark.LastCommitRound(); lc != last {
			last = lc
			continue
		}
		e.produce(types.Round(round.Load()))
	}
	// Whatever is left is produced at the end of the DAG.
	for e.produce(types.Round(len(dag) - 1)) {
	}
}

// produce checkpoints the oldest group handed over, if any, and reports
// whether there was one.
func (e *stalledExec) produce(frontier types.Round) bool {
	e.n.orderMu.Lock()
	var l types.Round
	if len(e.n.handed) > 0 {
		l = e.n.handed[0].leader
	}
	e.n.orderMu.Unlock()
	if l == 0 {
		return false
	}
	cp := mustCheckpointAt(e.t, e.n, l)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cps[l] = cp
	e.at[l] = frontier
	e.maxLead = max(e.maxLead, frontier-l)
	return true
}

// A node whose executor stalls does not order further than its DAG can
// still checkpoint (#4625): ordering waits for the executor, so the
// checkpoint for the block being produced always finds its tail, and a node
// restored from it orders every later group as the reference does. The DAG
// collects everything more than DAGGCDepth behind the frontier, and a
// checkpoint's tail begins a rescue window and a weak-link window below its
// leader.
func TestCheckpointAt_TailSurvivesOrderingLead(t *testing.T) {
	const depth = 200
	r := newDedupRig(t)
	r.straggler = true
	const last = 700
	var dag [][]*types.Certificate
	var prev []*types.Certificate
	for round := types.Round(0); round <= last; round++ {
		prev = r.build(round, prev)
		dag = append(dag, prev)
	}

	// The reference orders with no state directory and no lag.
	ref := r.nodeAt("", depth)
	refLive := map[types.Round][]string{}
	for _, cs := range dag {
		r.feed(ref, refLive, cs...)
	}
	require.Greater(t, len(refLive), 200)

	dir := t.TempDir()
	e := &stalledExec{t: t, r: r, n: r.nodeAt(dir, depth), live: map[types.Round][]string{},
		cps: map[types.Round]*persist.Checkpoint{}, at: map[types.Round]types.Round{}}
	e.run(dag)

	// Ordering is a function of the DAG, not of the executor's pace.
	require.Equal(t, len(refLive), len(e.live))
	for l, g := range refLive {
		require.Equal(t, g, e.live[l], "leader %d", l)
	}

	// The executor was never so far behind that the tail was out of reach.
	limit := types.Round(depth) - types.Round(bullshark.RescueWindow+types.WeakLinkWindow)
	require.NotEmpty(t, e.cps)
	require.LessOrEqual(t, e.maxLead, limit, "ordering ran past the DAG's retention")

	// The last two checkpoints (the store removes the tails of older ones)
	// restore a node that orders every later group as the reference does.
	all := make([]types.Round, 0, len(e.cps))
	for l := range e.cps {
		all = append(all, l)
	}
	sortRounds(all)
	require.GreaterOrEqual(t, len(all), 2)
	for i, l := range all[len(all)-2:] {
		m := r.nodeAt(dir, depth)
		require.NoError(t, m.Restore(e.cps[l]))
		got := map[types.Round][]string{}
		r.replay(m, got)
		for round := e.at[l] + 1; round <= last; round++ {
			r.feed(m, got, dag[round]...)
		}
		later := 0
		for rl, g := range refLive {
			if rl > l {
				later++
				require.Equal(t, g, got[rl], "restored from %d, leader %d", l, rl)
			}
		}
		if i == 0 {
			require.NotZero(t, later)
		}
	}
}

func mustCheckpointAt(t *testing.T, n *Node, leader types.Round) *persist.Checkpoint {
	t.Helper()
	cp, err := n.CheckpointAt(leader)
	require.NoError(t, err)
	return cp
}

// A checkpoint whose tail the DAG no longer holds is an error, and changes
// nothing: the groups handed over stay listed and the dedup floor stays put
// (#4625).
func TestCheckpointAt_MissingTailIsAnError(t *testing.T) {
	const depth = 100
	r := newDedupRig(t)
	n := r.nodeAt(t.TempDir(), depth)
	live := map[types.Round][]string{}
	var prev []*types.Certificate
	for round := types.Round(0); round <= 300; round++ {
		prev = r.layer(n, live, round, prev)
		// The executor keeps up, so ordering never waits.
		n.orderMu.Lock()
		var l types.Round
		if len(n.handed) > 0 {
			l = n.handed[len(n.handed)-1].leader
		}
		n.orderMu.Unlock()
		if l != 0 {
			mustCheckpointAt(t, n, l)
		}
	}
	require.Greater(t, n.dag.CollectedBelow(), tailFloor(50))

	n.orderMu.Lock()
	n.handed = append(n.handed, handedGroup{leader: 50})
	n.orderMu.Unlock()
	cp, err := n.CheckpointAt(50)
	require.Error(t, err)
	require.Nil(t, cp)
	n.orderMu.Lock()
	require.Len(t, n.handed, 1)
	n.orderMu.Unlock()
}
