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
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/bullshark"
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
	n, err := NewNode(NodeConfig{Partition: "t", KeyPair: r.keys[0], StateDir: r.dir, NumWorkers: 1}, r.comm, nil, nil)
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

	// A node restored from the checkpoint orders the next group as the
	// node that wrote it did.
	m := r.node()
	m.Restore(cp)
	restored := map[types.Round][]string{}
	r.replay(m, restored)
	require.Equal(t, live[102], restored[102], "leader 102's group differs after a restore from leader 100's checkpoint")
}
