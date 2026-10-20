// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bullshark

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/dag"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// arrivalNet builds certificates for n authors; arrivalNode is
// Node.processBullshark reduced to insert, order, collect.
type arrivalNet struct {
	t    *testing.T
	pubs []ed25519.PublicKey
	comm *types.Committee
	ts   int64
}

func newArrivalNet(t *testing.T, n int) *arrivalNet {
	net := &arrivalNet{t: t}
	infos := make([]types.ValidatorInfo, n)
	for i := range infos {
		pub, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		net.pubs = append(net.pubs, pub)
		infos[i] = types.ValidatorInfo{PublicKey: pub, Stake: 1}
	}
	net.comm = types.NewCommittee(infos, 0)
	return net
}

func (net *arrivalNet) cert(author int, r types.Round, parents ...*types.Certificate) *types.Certificate {
	var ds []types.CertificateDigest
	for _, p := range parents {
		ds = append(ds, p.Digest())
	}
	h := types.NewHeader(net.pubs[author], r, 0, nil, ds)
	net.ts++
	h.Timestamp = net.ts
	return types.NewCertificate(h, nil, nil)
}

func (net *arrivalNet) leader(r types.Round) int {
	k := selectByStake(net.comm, ComputeLeaderSeed(r))
	for i, p := range net.pubs {
		if p.Equal(ed25519.PublicKey(k)) {
			return i
		}
	}
	panic("no leader")
}

func (net *arrivalNet) full(last types.Round) [][]*types.Certificate {
	rounds := make([][]*types.Certificate, last+1)
	for r := types.Round(0); r <= last; r++ {
		for a := range net.pubs {
			var ps []*types.Certificate
			if r > 0 {
				ps = rounds[r-1]
			}
			rounds[r] = append(rounds[r], net.cert(a, r, ps...))
		}
	}
	return rounds
}

type arrivalNode struct {
	t    *testing.T
	d    *dag.DAG
	bs   *Bullshark
	gc   types.Round
	outs []string
}

func newArrivalNode(t *testing.T, net *arrivalNet, gc types.Round) *arrivalNode {
	d := dag.NewDAG(gc)
	return &arrivalNode{t: t, d: d, bs: New(net.comm, d), gc: gc}
}

// deliver inserts what it can and orders it; it reports the certificates
// the DAG refused for missing parents, which a primary would hold.
func (n *arrivalNode) deliver(cs ...*types.Certificate) (refused []*types.Certificate) {
	for _, c := range cs {
		var err error
		if c.Round() == 0 {
			err = n.d.InsertGenesis(c)
		} else {
			err = n.d.Insert(c)
		}
		if err != nil {
			refused = append(refused, c)
			continue
		}
		for _, o := range n.bs.ProcessCertificate(c) {
			n.outs = append(n.outs, fmt.Sprintf("L%d:%s", o.Leader, o.Certificate.Digest().String()[:8]))
		}
		if lc := n.bs.LastCommitRound(); lc > n.gc {
			n.d.GarbageCollect(lc)
		}
	}
	return refused
}

// Two nodes that receive the same certificates in different orders commit
// the same sequence (#4595 path 1). A late certificate S of round L is
// named by one round-L+1 certificate C. Node A gets S then C; node B
// commits leader L, then gets C: before the fix the DAG took C without S,
// since L+1 was at the commit floor plus one, and leader L+2's group held S
// on A and not on B.
func TestOrdering_IndependentOfArrivalOrderAtTheCommitFloor(t *testing.T) {
	net := newArrivalNet(t, 4)
	const gc = types.Round(100)
	const L = types.Round(110)
	full := net.full(L - 1)

	la := net.leader(L)
	s := (la + 1) % 4 // the straggler's author
	c := (la + 2) % 4 // the author whose round-L+1 header names the straggler

	var rL, rLnoS []*types.Certificate
	var S *types.Certificate
	for a := range net.pubs {
		cc := net.cert(a, L, full[L-1]...)
		rL = append(rL, cc)
		if a == s {
			S = cc
		} else {
			rLnoS = append(rLnoS, cc)
		}
	}
	var rL1 []*types.Certificate
	var C *types.Certificate
	for a := range net.pubs {
		if a == c {
			C = net.cert(a, L+1, rL...)
			rL1 = append(rL1, C)
		} else {
			rL1 = append(rL1, net.cert(a, L+1, rLnoS...))
		}
	}
	var rest [][]*types.Certificate
	prev := rL1
	for r := L + 2; r <= L+5; r++ {
		var cur []*types.Certificate
		for a := range net.pubs {
			cur = append(cur, net.cert(a, r, prev...))
		}
		rest = append(rest, cur)
		prev = cur
	}

	A := newArrivalNode(t, net, gc)
	B := newArrivalNode(t, net, gc)
	for _, rr := range full {
		require.Empty(t, A.deliver(rr...))
		require.Empty(t, B.deliver(rr...))
	}
	require.Greater(t, A.bs.LastCommitRound(), gc, "past the GC depth")
	require.Equal(t, A.outs, B.outs)

	require.Empty(t, A.deliver(rL...))
	require.Empty(t, A.deliver(rL1...))
	require.Empty(t, B.deliver(rLnoS...))
	for _, cc := range rL1 {
		if cc != C {
			require.Empty(t, B.deliver(cc))
		}
	}
	require.Equal(t, L, B.bs.LastCommitRound())

	// C names S, which B does not hold: C waits for it.
	held := B.deliver(C)
	require.Equal(t, []*types.Certificate{C}, held, "C entered B's DAG without its parent S")

	require.Empty(t, A.deliver(rest[0]...))
	require.Empty(t, A.deliver(rest[1]...))
	// B's later rounds name C, so they wait too, as a primary would hold them.
	var waiting []*types.Certificate
	waiting = append(waiting, B.deliver(rest[0]...)...)
	waiting = append(waiting, B.deliver(rest[1]...)...)
	require.Empty(t, B.deliver(S))
	require.Empty(t, B.deliver(C))
	require.Empty(t, B.deliver(waiting...))
	require.Empty(t, A.deliver(rest[2]...))
	require.Empty(t, A.deliver(rest[3]...))
	require.Empty(t, B.deliver(rest[2]...))
	require.Empty(t, B.deliver(rest[3]...))

	require.Equal(t, A.outs, B.outs, "the two nodes committed different sequences")
}

// Ordering stops rather than proceeds over a collected round (#4595 path
// 3). Collection on round advance keeps the DAG bounded while the executor
// is halted (#4239); a node whose commits trail the frontier by more than
// the depth has then lost rounds its next ordering walk still reaches, and
// used to order a group over the gap that its peers did not.
func TestOrdering_StopsAtACollectedRound(t *testing.T) {
	net := newArrivalNet(t, 4)
	const gc = types.Round(10)
	n := newArrivalNode(t, net, gc)
	full := net.full(5)
	for _, rr := range full {
		require.Empty(t, n.deliver(rr...))
	}
	committed := n.bs.LastCommitRound()
	require.Greater(t, committed, types.Round(0))

	// The frontier runs 60 rounds ahead with nothing ordered.
	prev := full[5]
	var certs []*types.Certificate
	for r := types.Round(6); r <= 65; r++ {
		var cur []*types.Certificate
		for a := range net.pubs {
			c := net.cert(a, r, prev...)
			require.NoError(t, n.d.Insert(c))
			cur = append(cur, c)
		}
		certs = append(certs, cur...)
		prev = cur
	}
	require.Greater(t, n.d.CollectedBelow(), committed, "the walk's rounds were collected")

	before := len(n.outs)
	for _, c := range certs {
		n.bs.ProcessCertificate(c)
	}
	require.Equal(t, before, len(n.outs), "ordered a group over collected rounds")
	require.Equal(t, committed, n.bs.LastCommitRound())
}
