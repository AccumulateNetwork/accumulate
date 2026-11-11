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
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/bullshark"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// changeNet is four validators, a committee of them and the committee that
// replaces it, without validator 3, and every round of a DAG in which each
// certificate names every certificate of the round before.
type changeNet struct {
	t      *testing.T
	keys   []ed25519.PrivateKey
	old    *types.Committee
	next   *types.Committee
	rounds [][]*types.Certificate
}

func newChangeNet(t *testing.T, top types.Round) *changeNet {
	n := &changeNet{t: t}
	var infos []types.ValidatorInfo
	for i := 0; i < 4; i++ {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		n.keys = append(n.keys, priv)
		infos = append(infos, types.ValidatorInfo{PublicKey: pub, Stake: 1})
	}
	n.old = types.NewCommittee(infos, 1)
	n.next = types.NewCommittee(infos[:3], 2)
	var ts int64
	for r := types.Round(0); r <= top; r++ {
		var parents []types.CertificateDigest
		if r > 0 {
			for _, p := range n.rounds[r-1] {
				parents = append(parents, p.Digest())
			}
		}
		var round []*types.Certificate
		for _, key := range n.keys {
			h := types.NewHeader(key.Public().(ed25519.PublicKey), r, 1, nil, parents)
			ts++
			h.Timestamp = ts
			require.NoError(t, h.Sign(key))
			round = append(round, types.NewCertificate(h, nil, nil))
		}
		n.rounds = append(n.rounds, round)
	}
	return n
}

// leader is the key a committee of equal stakes elects at round.
func leader(c *types.Committee, round types.Round) ed25519.PublicKey {
	seed := bullshark.ComputeLeaderSeed(round)
	return c.Validators[binary.BigEndian.Uint64(seed[:8])%uint64(c.Len())].PublicKey
}

// differ is the leader rounds in [from, to] the two committees elect
// different leaders at.
func (n *changeNet) differ(from, to types.Round) []types.Round {
	var out []types.Round
	for r := from; r <= to; r += 2 {
		if !leader(n.old, r).Equal(leader(n.next, r)) {
			out = append(out, r)
		}
	}
	return out
}

func (n *changeNet) node(dir string, c *types.Committee) *Node {
	node, err := NewNode(NodeConfig{Partition: "t", KeyPair: n.keys[0], StateDir: dir, NumWorkers: 1}, c, nil, nil)
	require.NoError(n.t, err)
	node.ctx, node.cancel = context.WithCancel(context.Background())
	n.t.Cleanup(node.cancel)
	return node
}

// feed inserts certificates into a node's DAG and orders each.
func feed(t *testing.T, node *Node, certs ...*types.Certificate) {
	for _, c := range certs {
		var err error
		if c.Round() == 0 {
			err = node.dag.InsertGenesis(c)
		} else {
			err = node.dag.Insert(c)
		}
		require.NoError(t, err)
		require.True(t, node.order(c))
	}
}

// drain takes the groups the node has handed to the executor.
func drain(node *Node) [][]*types.Certificate {
	var out [][]*types.Certificate
	for {
		select {
		case g := <-node.committed:
			out = append(out, g)
		default:
			return out
		}
	}
}

// describe is a group as its leader round and certificate digests.
func describe(g []*types.Certificate) string {
	s := fmt.Sprint(g[len(g)-1].Round())
	for _, c := range g {
		s += fmt.Sprintf(" %d:%x", c.Round(), c.Digest().String()[:8])
	}
	return s
}

// executor stands in for the service: it produces the groups in order,
// executing the committee change in the block of leader changeAt.
type executor struct {
	t        *testing.T
	node     *Node
	changeAt types.Round
	next     *types.Committee
	queue    [][]*types.Certificate
	produced []string
}

// run produces every group handed over, and every group ordering hands over
// as the round limit rises behind them.
func (x *executor) run() {
	for {
		x.queue = append(x.queue, drain(x.node)...)
		if len(x.queue) == 0 {
			return
		}
		g := x.queue[0]
		x.queue = x.queue[1:]
		round := g[len(g)-1].Round()
		if round == x.changeAt {
			c := x.next.Clone()
			c.ActivationRound = round + types.CommitteeActivationMargin
			require.NoError(x.t, x.node.UpdateCommittee(c))
		}
		x.produced = append(x.produced, describe(g))
		x.node.ReportExecuted(round)
		require.True(x.t, x.node.orderDeferred())
	}
}

// Two nodes execute the block that carries a committee change at different
// points of their ordering, one at once and one only after its consensus has
// taken in every round past the activation round, and still order the same
// leaders across the switch (#4593): the change takes effect at the round
// fixed by the leader that committed it, and the lagging node orders no
// leader above its round limit until it has executed that block.
func TestTwoNodesExecutingACommitteeChangeAtDifferentPointsOrderTheSameLeaders(t *testing.T) {
	const changeAt = 10
	activation := changeAt + types.CommitteeActivationMargin
	top := activation + 40
	net := newChangeNet(t, top)
	require.NotEmpty(t, net.differ(activation, top-2), "the committees elect different leaders somewhere above the activation round")

	prompt := &executor{t: t, node: net.node("", net.old), changeAt: changeAt, next: net.next}
	lagging := &executor{t: t, node: net.node("", net.old), changeAt: changeAt, next: net.next}
	prompt.node.StateAt(0)
	lagging.node.StateAt(0)

	for _, round := range net.rounds {
		feed(t, prompt.node, round...)
		prompt.run()
	}

	for _, round := range net.rounds {
		feed(t, lagging.node, round...)
	}
	require.Less(t, lagging.node.LastCommitRound(), activation,
		"a node that has executed nothing orders nothing at or above the activation round of a change it may not have executed")
	lagging.run()

	require.Equal(t, prompt.produced, lagging.produced)
	require.Equal(t, top-2, lagging.node.LastCommitRound())
	for _, r := range []types.Round{changeAt, activation - 2, activation, top - 2} {
		c := net.old
		if r >= activation {
			c = net.next
		}
		require.True(t, lagging.node.Bullshark().LeaderForRound(r).Author().Equal(leader(c, r)), "leader of round %d", r)
	}
}

// A restart that replays the persisted tail orders the same leaders as the
// node did before it stopped (#4593). The tail straddles a committee change;
// the restarted node is built with the committee its executor's state names,
// the new one, and orders each round with the committee the checkpoint says
// it had. Without the checkpoint's committees it orders the rounds below the
// activation round with the new committee and its groups differ.
func TestARestartReplaysTheTailWithTheCommitteesItWasOrderedWith(t *testing.T) {
	const checkpointAt, activation, top = 20, 40, 41
	var net *changeNet
	for net == nil || len(net.differ(checkpointAt+2, activation-2)) == 0 {
		net = newChangeNet(t, top)
	}
	dir := t.TempDir()

	n := net.node(dir, net.old)
	next := net.next.Clone()
	next.ActivationRound = activation
	require.NoError(t, n.UpdateCommittee(next))
	live := map[types.Round]string{}
	for _, round := range net.rounds {
		feed(t, n, round...)
		for _, g := range drain(n) {
			live[g[len(g)-1].Round()] = describe(g)
		}
	}
	cp := n.CheckpointAt(checkpointAt)

	replay := func(m *Node) map[types.Round]string {
		m.orderMu.Lock()
		certs := m.restored
		m.restored = nil
		m.orderMu.Unlock()
		out := map[types.Round]string{}
		for _, c := range certs {
			require.True(t, m.order(c))
			for _, g := range drain(m) {
				out[g[len(g)-1].Round()] = describe(g)
			}
		}
		return out
	}

	// The groups above the checkpoint that differ from the live node's.
	differing := func(restored map[types.Round]string) int {
		n := 0
		for r := types.Round(checkpointAt + 2); r <= top-1; r += 2 {
			if live[r] != restored[r] {
				n++
			}
		}
		return n
	}

	m := net.node(dir, next)
	require.NoError(t, m.Restore(cp))
	require.Zero(t, differing(replay(m)))

	stripped := *cp
	stripped.Committees = nil
	k := net.node(dir, next)
	require.NoError(t, k.Restore(&stripped))
	require.NotZero(t, differing(replay(k)), "the committee the state names orders the tail differently")
}
