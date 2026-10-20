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
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

type tailRig struct {
	t    *testing.T
	keys []ed25519.PrivateKey
	comm *types.Committee
	dir  string
	ts   int64
}

func newTailRig(t *testing.T) *tailRig {
	r := &tailRig{t: t, dir: t.TempDir()}
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

func (r *tailRig) node() *Node {
	n, err := NewNode(NodeConfig{Partition: "t", KeyPair: r.keys[0], StateDir: r.dir, NumWorkers: 1}, r.comm, nil, nil)
	require.NoError(r.t, err)
	n.ctx, n.cancel = context.WithCancel(context.Background())
	r.t.Cleanup(n.cancel)
	return n
}

func (r *tailRig) cert(author int, round types.Round, parents []*types.Certificate) *types.Certificate {
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

func (r *tailRig) drain(n *Node, into map[types.Round][]string) {
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

func (r *tailRig) feed(n *Node, out map[types.Round][]string, cs ...*types.Certificate) {
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

func (r *tailRig) replay(m *Node, out map[types.Round][]string) {
	m.orderMu.Lock()
	replay := m.restored
	m.restored = nil
	m.orderMu.Unlock()
	for _, c := range replay {
		require.True(r.t, m.order(c))
		r.drain(m, out)
	}
}

// The persisted tail is the DAG as of the crash (#4595 path 2). Authored
// headers used to write the DAG from four rounds below their own, while
// headers name weak links down to eight below: a round-105 straggler that
// arrived during round 110, named only as a weak link by the round-110
// certificates, was on disk only if a later checkpoint wrote it, and a
// node restored after a crash at 112 ordered leader 110's group without it.
func TestRestore_TailHoldsEveryCertificateInserted(t *testing.T) {
	r := newTailRig(t)
	n := r.node()
	live := map[types.Round][]string{}

	round := func(rd types.Round, parents []*types.Certificate, authors ...int) []*types.Certificate {
		var cur []*types.Certificate
		for _, a := range authors {
			cur = append(cur, r.cert(a, rd, parents))
		}
		return cur
	}
	all := []int{0, 1, 2, 3}
	var prev []*types.Certificate
	for rd := types.Round(0); rd <= 104; rd++ {
		cur := round(rd, prev, all...)
		r.feed(n, live, cur...)
		prev = cur
	}
	cp := n.CheckpointAt(100) // the executor's checkpoint; its tail holds the DAG through 104
	require.Equal(t, types.Round(100), cp.LastCommitRound)

	own := func(cs []*types.Certificate) *types.Header { return cs[0].Header } // author 0 is this node

	r105 := round(105, prev, 0, 1, 2)
	S := r.cert(3, 105, prev) // the straggler
	require.NoError(t, n.persistAuthored(own(r105)))
	r.feed(n, live, r105...)
	prev = r105
	for rd := types.Round(106); rd <= 109; rd++ {
		cur := round(rd, prev, all...)
		require.NoError(t, n.persistAuthored(own(cur)))
		r.feed(n, live, cur...)
		prev = cur
	}
	r110 := round(110, append(append([]*types.Certificate{}, prev...), S), all...)
	require.NoError(t, n.persistAuthored(own(r110))) // written before S arrives
	r.feed(n, live, S)
	r.feed(n, live, r110...)
	prev = r110
	for rd := types.Round(111); rd <= 112; rd++ {
		cur := round(rd, prev, all...)
		require.NoError(t, n.persistAuthored(own(cur)))
		r.feed(n, live, cur...)
		prev = cur
	}
	require.Equal(t, types.Round(110), n.bullshark.LastCommitRound())

	// Crash; restart from the checkpoint and what the store holds.
	m := r.node()
	m.Restore(cp)
	require.NotNil(t, m.dag.GetByDigest(S.Digest()), "the straggler was not persisted")
	restored := map[types.Round][]string{}
	r.replay(m, restored)
	require.Equal(t, live[110], restored[110], "leader 110's group differs after the restore")
}
