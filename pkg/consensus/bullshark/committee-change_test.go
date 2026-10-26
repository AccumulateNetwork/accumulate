// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bullshark

import (
	"crypto/ed25519"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/dag"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// orderer is one node's ordering: its own DAG and Bullshark, fed the same
// certificates as every other.
type orderer struct {
	t    *testing.T
	d    *dag.DAG
	bs   *Bullshark
	outs []string
}

func newOrderer(t *testing.T, c *types.Committee) *orderer {
	d := dag.NewDAG(2000)
	return &orderer{t: t, d: d, bs: New(c, d)}
}

func (o *orderer) deliver(certs ...*types.Certificate) {
	for _, c := range certs {
		if c.Round() == 0 {
			require.NoError(o.t, o.d.InsertGenesis(c))
		} else {
			require.NoError(o.t, o.d.Insert(c))
		}
		for _, out := range o.bs.ProcessCertificate(c) {
			o.outs = append(o.outs, fmt.Sprintf("L%d:%x", out.Leader, out.Certificate.Digest().String()[:8]))
		}
	}
}

// leaderKey is the key the committee elects at round.
func leaderKey(c *types.Committee, round types.Round) ed25519.PublicKey {
	return selectByStake(c, ComputeLeaderSeed(round))
}

// A committee change takes effect at its activation round on every node,
// whenever each node takes it in (#4593). One node schedules it before it
// orders anything, another only after it has ordered every leader below the
// activation round; both order the same leaders, the old committee's below
// that round and the new one's from it.
func TestACommitteeChangeOrdersTheSameLeadersWhereverANodeTakesItIn(t *testing.T) {
	h := newTestHelper(t, 4)
	next := types.NewCommittee(h.committee.Validators[:3], 2)

	// An activation round at which the two committees elect different
	// leaders, so ordering it with the wrong one shows.
	var activation types.Round
	for r := types.Round(8); r < 400; r += 2 {
		if !leaderKey(h.committee, r).Equal(leaderKey(next, r)) {
			activation = r
			break
		}
	}
	require.NotZero(t, activation)
	next.ActivationRound = activation

	rounds := make([][]*types.Certificate, activation+6)
	for r := range rounds {
		for i := range h.keys {
			var parents []*types.Certificate
			if r > 0 {
				parents = rounds[r-1]
			}
			rounds[r] = append(rounds[r], h.createCert(i, types.Round(r), parents))
		}
	}

	early, late, never := newOrderer(t, h.committee), newOrderer(t, h.committee), newOrderer(t, h.committee)
	require.NoError(t, early.bs.UpdateCommittee(next))
	for r := types.Round(0); r < activation; r++ {
		early.deliver(rounds[r]...)
		late.deliver(rounds[r]...)
		never.deliver(rounds[r]...)
	}
	require.Equal(t, activation-2, late.bs.LastCommitRound(), "every leader below the activation round is ordered")
	require.NoError(t, late.bs.UpdateCommittee(next))
	for r := activation; r < types.Round(len(rounds)); r++ {
		early.deliver(rounds[r]...)
		late.deliver(rounds[r]...)
		never.deliver(rounds[r]...)
	}

	require.Equal(t, early.outs, late.outs)
	require.NotEqual(t, early.outs, never.outs, "the activation round is one the committees disagree on")
	require.True(t, late.bs.LeaderForRound(activation).Author().Equal(leaderKey(next, activation)))
	require.True(t, late.bs.LeaderForRound(activation-2).Author().Equal(leaderKey(h.committee, activation-2)))

	// A round already ordered keeps the committee it was ordered with.
	third := types.NewCommittee(h.committee.Validators[1:], 3)
	third.ActivationRound = late.bs.LastCommitRound()
	require.Error(t, late.bs.UpdateCommittee(third))
	require.Same(t, next, late.bs.Committees().Latest())
}
