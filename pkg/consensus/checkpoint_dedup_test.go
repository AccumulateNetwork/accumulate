// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/bullshark"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/persist"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// A four-validator DAG fed to a Node certificate by certificate, with the
// groups the node handed to the executor recorded per leader.
type dedupRig struct {
	t    *testing.T
	keys []ed25519.PrivateKey
	comm *types.Committee
	dir  string
	ts   int64
	// straggler: the last validator's certificates are referenced by the
	// others only every 20th round, so they commit late, in a later
	// leader's group, rounds below it.
	straggler bool
}

func newDedupRig(t *testing.T) *dedupRig {
	r := &dedupRig{t: t, dir: t.TempDir()}
	var infos []types.ValidatorInfo
	for i := 0; i < 4; i++ {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		r.keys = append(r.keys, priv)
		infos = append(infos, types.ValidatorInfo{PublicKey: pub, Stake: 1})
	}
	r.comm = types.NewCommittee(infos, 0)
	return r
}

func (r *dedupRig) node() *Node {
	return r.nodeWithGCDepth(0)
}

func (r *dedupRig) nodeWithGCDepth(depth types.Round) *Node {
	return r.nodeAt(r.dir, depth)
}

// nodeAt is a node with its state in dir; an empty dir is a node without a
// DAG store.
func (r *dedupRig) nodeAt(dir string, depth types.Round) *Node {
	n, err := NewNode(NodeConfig{Partition: "t", KeyPair: r.keys[0], StateDir: dir, NumWorkers: 1, DAGGCDepth: depth}, r.comm, nil, nil)
	require.NoError(r.t, err)
	n.ctx, n.cancel = context.WithCancel(context.Background())
	r.t.Cleanup(n.cancel)
	return n
}

func (r *dedupRig) cert(author int, round types.Round, parents []*types.Certificate) *types.Certificate {
	var ds []types.CertificateDigest
	for _, p := range parents {
		ds = append(ds, p.Digest())
	}
	h := types.NewHeader(r.keys[author].Public().(ed25519.PublicKey), round, 0, nil, ds)
	r.ts++
	h.Timestamp = r.ts
	require.NoError(r.t, h.Sign(r.keys[author]))
	return types.NewCertificate(h, nil, nil)
}

// drain records what the node handed to the executor, by leader round.
func (r *dedupRig) drain(n *Node, into map[types.Round][]string) {
	for {
		select {
		case g := <-n.committed:
			leader := g[len(g)-1].Round()
			for _, c := range g {
				into[leader] = append(into[leader], fmt.Sprintf("%d:%s", c.Round(), c.Digest().String()[:8]))
			}
		default:
			return
		}
	}
}

func (r *dedupRig) feed(n *Node, out map[types.Round][]string, cs ...*types.Certificate) {
	for _, c := range cs {
		var err error
		if c.Round() == 0 {
			err = n.dag.InsertGenesis(c)
		} else {
			err = n.dag.Insert(c)
		}
		require.NoError(r.t, err)
		require.True(r.t, n.order(c))
		r.drain(n, out)
	}
}

// replay orders what Restore loaded above the checkpoint, as Start does.
func (r *dedupRig) replay(m *Node, out map[types.Round][]string) {
	m.orderMu.Lock()
	replay := m.restored
	m.restored = nil
	m.orderMu.Unlock()
	for _, c := range replay {
		require.True(r.t, m.order(c))
		r.drain(m, out)
	}
}

// The checkpoint of the block being produced carries the committed-digest
// set as it was when that block's leader was ordered, whatever the ordering
// lead (#4594). Bullshark has ordered to leader 208 while the executor is
// producing leader 100's block; before the fix its dedup set had been pruned
// 64 rounds below 208, so the checkpoint for leader 100 held nothing and a
// node restored from it re-committed leaders 70..100's certificates inside
// leader 102's group.
func TestCheckpointAt_DedupSurvivesOrderingLead(t *testing.T) {
	r := newDedupRig(t)
	n := r.node()
	live := map[types.Round][]string{}
	var prev []*types.Certificate
	for round := types.Round(0); round <= 210; round++ {
		var cur []*types.Certificate
		for a := range r.keys {
			var ps []*types.Certificate
			if round > 0 {
				ps = prev
			}
			cur = append(cur, r.cert(a, round, ps))
		}
		r.feed(n, live, cur...)
		prev = cur
	}
	require.Equal(t, types.Round(208), n.bullshark.LastCommitRound())

	cp := n.CheckpointAt(100)
	require.Equal(t, types.Round(100), cp.LastCommitRound)

	// Every certificate committed within the rescue window below leader 100
	// is in the checkpoint, and nothing ordered after leader 100 is.
	var oldest, newest types.Round = 1 << 62, 0
	for _, round := range cp.Committed {
		if round < oldest {
			oldest = round
		}
		if round > newest {
			newest = round
		}
	}
	require.LessOrEqual(t, oldest, types.Round(100-bullshark.RescueWindow), "the checkpoint's dedup set does not reach back a rescue window")
	require.Equal(t, types.Round(100), newest, "the checkpoint's dedup set counts groups ordered after leader 100")
	// The copy holds the window the checkpoint keeps, not the whole set.
	require.GreaterOrEqual(t, oldest, types.Round(100-2*bullshark.RescueWindow))

	// A node restored from the checkpoint orders the next group as the
	// node that wrote it did.
	m := r.node()
	require.NoError(t, m.Restore(cp))
	restored := map[types.Round][]string{}
	r.replay(m, restored)
	require.Equal(t, live[102], restored[102], "leader 102's group differs after a restore from leader 100's checkpoint")
}

// build is one round of the DAG on top of prev.
func (r *dedupRig) build(round types.Round, prev []*types.Certificate) []*types.Certificate {
	var cur []*types.Certificate
	for a := range r.keys {
		var ps []*types.Certificate
		if round > 0 {
			ps = prev
			if r.straggler && a < len(r.keys)-1 && round%20 != 1 {
				ps = prev[:len(prev)-1]
			}
		}
		cur = append(cur, r.cert(a, round, ps))
	}
	return cur
}

// layer feeds one round of the four-validator DAG and returns it.
func (r *dedupRig) layer(n *Node, out map[types.Round][]string, round types.Round, prev []*types.Certificate) []*types.Certificate {
	cur := r.build(round, prev)
	r.feed(n, out, cur...)
	return cur
}

// A node that orders and never checkpoints (collecting, or halted on
// divergence) still bounds its dedup set: the floor is never more than
// DAGGCDepth below the last commit (#4594).
func TestDedupSetBoundedWithoutCheckpoint(t *testing.T) {
	const depth = 200
	r := newDedupRig(t)
	n := r.nodeWithGCDepth(depth)
	live := map[types.Round][]string{}
	var prev []*types.Certificate
	for round := types.Round(0); round <= 1000; round++ {
		prev = r.layer(n, live, round, prev)
	}
	require.Greater(t, n.bullshark.LastCommitRound(), types.Round(900))
	require.LessOrEqual(t, len(n.bullshark.GetCommitted()), (depth+64+8)*4)
}

// The executor stalls, then trails ordering by more than the prune window
// and takes a checkpoint for every group before producing it, as
// Service.produce does. The set stays within the lead plus the window, and a node restored from the
// checkpoint at leader 300 orders group 302 as the live node did (#4594).
func TestCheckpointAt_DedupWindowUnderLag(t *testing.T) {
	const lag = 108
	r := newDedupRig(t)
	r.straggler = true
	n := r.node()
	live := map[types.Round][]string{}
	var leaders []types.Round
	next := 0
	var cp300 *persist.Checkpoint
	var prev []*types.Certificate
	for round := types.Round(0); round <= 420; round++ {
		prev = r.layer(n, live, round, prev)
		// Pruning follows the next commit, so the bound is checked as
		// ordering leaves the set: lead of the oldest unproduced group plus
		// the window.
		if next < len(leaders) {
			lead := int(n.bullshark.LastCommitRound() - leaders[next])
			require.LessOrEqual(t, len(n.bullshark.GetCommitted()), (lead+64+16)*4)
		}
		// Executor: stalled until round 360, then produces every group
		// whose leader is lag rounds behind.
		for round >= 360 {
			var ls []types.Round
			for l := range live {
				ls = append(ls, l)
			}
			sortRounds(ls)
			leaders = ls
			if next >= len(leaders) || leaders[next]+lag > n.bullshark.LastCommitRound() {
				break
			}
			l := leaders[next]
			next++
			cp := n.CheckpointAt(l)
			if l == 300 {
				cp300 = cp
			}
		}
	}
	require.NotNil(t, cp300, "the executor never reached leader 300")
	m := r.node()
	require.NoError(t, m.Restore(cp300))
	restored := map[types.Round][]string{}
	r.replay(m, restored)
	require.NotEmpty(t, live[302])
	require.Equal(t, live[302], restored[302])
}

func sortRounds(rs []types.Round) {
	sort.Slice(rs, func(i, j int) bool { return rs[i] < rs[j] })
}

// The executor's lead over one node, as CheckpointAt calls and as ordering
// proceeds.
type lagExec struct {
	n       *Node
	live    map[types.Round][]string
	next    int
	lag     types.Round
	stall   types.Round
	cps     map[types.Round]*persist.Checkpoint
	cpRound map[types.Round]types.Round
}

// step produces every group the executor has caught up to, checkpointing
// each first. A zero lag never produces.
func (e *lagExec) step(round types.Round) {
	if e.lag == 0 {
		return
	}
	for round >= e.stall {
		ls := make([]types.Round, 0, len(e.live))
		for l := range e.live {
			ls = append(ls, l)
		}
		sortRounds(ls)
		if e.next >= len(ls) || ls[e.next]+e.lag > e.n.bullshark.LastCommitRound() {
			return
		}
		l := ls[e.next]
		e.next++
		e.cps[l] = e.n.CheckpointAt(l)
		e.cpRound[l] = round
	}
}

// Ordering is the same on nodes whose floors differ: the default depth, a
// DAGGCDepth below the lag that clips the floor, a node with no state
// directory (it prunes at the frontier), a node that never checkpoints, and a
// depth below the prune window. The last two checkpoints of the default-depth
// node restore a node that catches up and orders every later group as the
// live node did. The prune margin is pinned to the round: a margin of
// rescueWindow-1 already orders a leader differently from a frontier node.
// A restore from a checkpoint older than DAGGCDepth below the frontier is
// #4625's territory and not covered here.
func TestDedup_OrderingSameAcrossFloors(t *testing.T) {
	r := newDedupRig(t)
	r.straggler = true
	const max = 900
	var dag [][]*types.Certificate
	var prev []*types.Certificate
	for round := types.Round(0); round <= max; round++ {
		prev = r.build(round, prev)
		dag = append(dag, prev)
	}

	dir := t.TempDir()
	mk := func(dir string, depth, lag, stall types.Round) *lagExec {
		return &lagExec{n: r.nodeAt(dir, depth), live: map[types.Round][]string{}, lag: lag, stall: stall,
			cps: map[types.Round]*persist.Checkpoint{}, cpRound: map[types.Round]types.Round{}}
	}
	execs := []*lagExec{
		mk(dir, 0, 150, 360),           // default depth, lag 150
		mk(t.TempDir(), 100, 150, 360), // depth below the lag: floor clipped
		mk("", 0, 150, 360),            // no state directory: the reference
		mk(t.TempDir(), 100, 0, 0),     // never checkpoints
		mk(t.TempDir(), 40, 300, 100),  // depth below the prune window
	}
	for round, cs := range dag {
		for _, e := range execs {
			r.feed(e.n, e.live, cs...)
			e.step(types.Round(round))
		}
	}
	ref := execs[2].live
	require.Greater(t, len(ref), 300)
	for i, e := range execs {
		require.Equal(t, len(ref), len(e.live), "node %d", i)
		for l, g := range ref {
			require.Equal(t, g, e.live[l], "node %d leader %d", i, l)
		}
	}

	var leaders []types.Round
	for l := range execs[0].cps {
		leaders = append(leaders, l)
	}
	sortRounds(leaders)
	require.GreaterOrEqual(t, len(leaders), 2)
	for _, L := range leaders[len(leaders)-2:] {
		m := r.nodeAt(dir, 0)
		require.NoError(t, m.Restore(execs[0].cps[L]))
		got := map[types.Round][]string{}
		r.replay(m, got)
		for round := execs[0].cpRound[L] + 1; round <= max; round++ {
			r.feed(m, got, dag[round]...)
		}
		later := 0
		for l, g := range ref {
			if l > L {
				later++
				require.Equal(t, g, got[l], "restored from %d, leader %d", L, l)
			}
		}
		require.NotZero(t, later)
		for l := range got {
			require.Greater(t, l, L)
		}
	}
}

// A node without a DAG store reports no floor, whatever checkpoints it
// takes, and prunes at the frontier: the set stays bounded (#4594).
func TestCheckpoint_NoStateDirKeepsFrontierPruning(t *testing.T) {
	r := newDedupRig(t)
	n := r.nodeAt("", 100)
	live := map[types.Round][]string{}
	var prev []*types.Certificate
	for round := types.Round(0); round <= 1000; round++ {
		prev = r.layer(n, live, round, prev)
		if round == 50 {
			n.Checkpoint()
		}
	}
	require.LessOrEqual(t, len(n.bullshark.GetCommitted()), (100+64+8)*4)
}
