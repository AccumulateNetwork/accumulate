// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/bullshark"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/persist"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// splitRig feeds a node the way its primary does: certificates enter the DAG
// on the gossip goroutine as they arrive, and ordering learns of each through
// a bounded channel with a non-blocking send (primary.signalNewCertificate),
// on its own goroutine (Node.processBullshark). The dedup rig's feed inserts
// and orders in one goroutine, so anything that blocks order() there blocks
// insertion too, which is not what production does: the DAG collects on
// every round advance whether or not ordering runs.
type splitRig struct {
	t        *testing.T
	r        *dedupRig
	n        *Node
	newCerts chan *types.Certificate
	sent     atomic.Int64 // notifications the channel took
	dropped  atomic.Int64 // notifications the full channel lost
	ordered  atomic.Int64 // certificates order() returned for
	live     map[types.Round][]string
	stopped  chan struct{}
}

func newSplitRig(t *testing.T, r *dedupRig, n *Node, chanSize int) *splitRig {
	s := &splitRig{t: t, r: r, n: n, newCerts: make(chan *types.Certificate, chanSize),
		live: map[types.Round][]string{}, stopped: make(chan struct{})}
	go s.orderLoop()
	t.Cleanup(func() { n.cancel(); <-s.stopped })
	return s
}

// insert is the gossip path: every certificate goes into the DAG, and
// ordering is told, or not, as the primary tells it.
func (s *splitRig) insert(rounds ...[]*types.Certificate) {
	for _, cs := range rounds {
		for _, c := range cs {
			var err error
			if c.Round() == 0 {
				err = s.n.dag.InsertGenesis(c)
			} else {
				err = s.n.dag.Insert(c)
			}
			require.NoError(s.t, err)
			select {
			case s.newCerts <- c:
				s.sent.Add(1)
			default:
				s.dropped.Add(1)
			}
		}
	}
}

// orderLoop is processBullshark.
func (s *splitRig) orderLoop() {
	defer close(s.stopped)
	for {
		select {
		case <-s.n.ctx.Done():
			return
		case c := <-s.newCerts:
			if !s.n.order(c) {
				return
			}
			s.r.drain(s.n, s.live)
			s.n.dag.GarbageCollect(s.n.bullshark.LastCommitRound())
			s.ordered.Add(1)
		}
	}
}

// sync waits until ordering has taken everything it was told of, so that
// live may be read.
func (s *splitRig) sync() {
	deadline := time.Now().Add(30 * time.Second)
	for s.ordered.Load() != s.sent.Load() {
		if time.Now().After(deadline) {
			s.t.Fatalf("ordering stopped: %d of %d certificates ordered", s.ordered.Load(), s.sent.Load())
		}
		time.Sleep(time.Millisecond)
	}
}

func buildDAG(r *dedupRig, last types.Round) [][]*types.Certificate {
	var dag [][]*types.Certificate
	var prev []*types.Certificate
	for round := types.Round(0); round <= last; round++ {
		prev = r.build(round, prev)
		dag = append(dag, prev)
	}
	return dag
}

func mustCheckpointAt(t *testing.T, n *Node, leader types.Round) *persist.Checkpoint {
	t.Helper()
	cp, err := n.CheckpointAt(leader)
	require.NoError(t, err)
	return cp
}

// The executor stalls until ordering has run more than the DAG's depth ahead
// of it, then catches up, taking a checkpoint for each group before producing
// it as Service.produce does (#4625).
//
// Ordering never waits for the executor. The DAG collects on the gossip
// goroutine as the frontier advances, whatever ordering does, so a wait in
// ordering would hold this node's commits while the tail was collected all
// the same, and nothing would ever lift it. Every group is handed over as the
// reference orders it, and the groups handed over stay bounded (#4624).
//
// A checkpoint is refused exactly while the DAG no longer holds the group's
// tail, or the group was dropped past the depth: the service writes none,
// and a restart meanwhile starts at round zero (DIFFERENCES C29). Once the
// executor is back within the window checkpoints succeed again, and the last
// two restore a node that orders every later group as the reference does.
func TestCheckpointAt_StalledExecutorRecovers(t *testing.T) {
	const depth = 200
	const last = 1000
	r := newDedupRig(t)
	r.straggler = true
	dag := buildDAG(r, last)

	ref := r.nodeAt("", depth)
	refLive := map[types.Round][]string{}
	for _, cs := range dag {
		r.feed(ref, refLive, cs...)
	}
	require.Greater(t, len(refLive), 300)

	dir := t.TempDir()
	n := r.nodeAt(dir, depth)
	s := newSplitRig(t, r, n, 1<<16)

	cps := map[types.Round]*persist.Checkpoint{}
	at := map[types.Round]types.Round{}
	var refused, taken int
	var firstTaken, lastRefused types.Round
	next := 0
	checkHanded := func() {
		t.Helper()
		n.orderMu.Lock()
		defer n.orderMu.Unlock()
		require.LessOrEqual(t, len(n.handed), depth/2+4)
		if lc := n.bullshark.LastCommitRound(); len(n.handed) > 0 && lc > depth {
			require.GreaterOrEqual(t, n.handed[0].leader, lc-depth)
		}
	}
	// produce takes the next k groups the executor was handed, as the service
	// does: a checkpoint, refused or not, then the block.
	produce := func(frontier types.Round, k int) {
		t.Helper()
		var ls []types.Round
		for l := range s.live {
			ls = append(ls, l)
		}
		sortRounds(ls)
		for ; k > 0 && next < len(ls); k-- {
			l := ls[next]
			next++
			n.orderMu.Lock()
			gone, dropped := n.dag.CollectedBelow(), n.handedDropped
			n.orderMu.Unlock()
			cp, err := n.CheckpointAt(l)
			if tailFloor(l) < gone || l < dropped {
				require.Error(t, err, "leader %d at frontier %d: tail floor %d, collected below %d, dropped %d", l, frontier, tailFloor(l), gone, dropped)
				require.Nil(t, cp)
				refused++
				lastRefused = l
				continue
			}
			require.NoError(t, err, "leader %d at frontier %d", l, frontier)
			if taken == 0 {
				firstTaken = l
			}
			taken++
			cps[l] = cp
			at[l] = frontier
		}
	}

	// The executor stalls while ordering runs three depths ahead.
	s.insert(dag[:601]...)
	s.sync()
	checkHanded()
	require.Greater(t, n.bullshark.LastCommitRound(), types.Round(590), "ordering waited for the executor")
	require.Equal(t, types.Round(600-depth), n.dag.CollectedBelow())
	produce(600, 1)
	require.Equal(t, 1, refused)

	// Then it catches up: three groups per round against the half a group
	// per round ordering adds.
	for round := types.Round(601); round <= last; round++ {
		s.insert(dag[round])
		s.sync()
		checkHanded()
		produce(round, 3)
	}
	require.Zero(t, s.dropped.Load())
	t.Logf("refused %d (last at leader %d), taken %d (first at leader %d, frontier %d)",
		refused, lastRefused, taken, firstTaken, at[firstTaken])
	require.Greater(t, refused, 1)
	require.Greater(t, taken, 10)
	require.Less(t, lastRefused, firstTaken, "a checkpoint was refused after one was taken")
	limit := types.Round(depth) - types.Round(bullshark.RescueWindow+types.WeakLinkWindow)
	require.LessOrEqual(t, at[firstTaken]-firstTaken, limit)

	// Ordering is a function of the DAG, not of the executor's pace.
	require.Equal(t, len(refLive), len(s.live))
	for l, g := range refLive {
		require.Equal(t, g, s.live[l], "leader %d", l)
	}

	// The last two checkpoints (the store removes the tails of older ones)
	// restore a node that orders every later group as the reference does.
	var all []types.Round
	for l := range cps {
		all = append(all, l)
	}
	sortRounds(all)
	for i, l := range all[len(all)-2:] {
		m := r.nodeAt(dir, depth)
		require.NoError(t, m.Restore(cps[l]))
		got := map[types.Round][]string{}
		r.replay(m, got)
		for round := at[l] + 1; round <= last; round++ {
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
		// The executor keeps up, so every checkpoint finds its tail.
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

// Between the lead at which the DAG has collected a group's tail floor and
// the lead at which #4624 drops the group, the base wrote a checkpoint whose
// tail began where collection left it; a node restored from it took in
// nothing. Here it is refused (#4625).
func TestCheckpointAt_RefusedInTheGapBelowTheDrop(t *testing.T) {
	const depth = 200
	r := newDedupRig(t)
	r.straggler = true
	dag := buildDAG(r, 1000)
	n := r.nodeAt(t.TempDir(), depth)
	live := map[types.Round][]string{}
	for _, cs := range dag {
		r.feed(n, live, cs...)
	}
	n.orderMu.Lock()
	require.NotEmpty(t, n.handed)
	L := n.handed[0].leader
	n.orderMu.Unlock()
	require.Less(t, tailFloor(L), n.dag.CollectedBelow(), "the tail is still in the DAG")
	require.GreaterOrEqual(t, L, n.handedDropped, "#4624 dropped the group")
	cp, err := n.CheckpointAt(L)
	require.Error(t, err)
	require.Nil(t, cp)
}
