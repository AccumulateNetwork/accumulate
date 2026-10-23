// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/dag"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// TestVoteRequiresParentQuorum: a voter signs a header only when its parents
// of the round before carry a stake quorum (Narwhal 4.1, #4591). It used to
// check only that each named parent was present, which a header with no
// parents passed.
func TestVoteRequiresParentQuorum(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()
	genesis := createGenesisCertificates(t, validators, committee, d)
	var parents []types.CertificateDigest
	for _, c := range genesis {
		parents = append(parents, c.Digest())
	}
	p := New(Config{Partition: "test", KeyPair: validators[0].priv}, committee, nil, d, nil)
	p.SetRound(1)

	header := func(v *testValidator, round types.Round, parents []types.CertificateDigest) *types.Header {
		h := types.NewHeader(v.pub, round, 1, nil, parents)
		require.NoError(t, h.Sign(v.priv))
		return h
	}
	none := header(validators[1], 1, nil)
	p.OnHeaderReceived(none)
	require.False(t, votedFor(p, none), "a round-1 header with no parents must not be voted")
	two := header(validators[2], 1, parents[:2])
	p.OnHeaderReceived(two)
	require.False(t, votedFor(p, two), "two of four parents are not a quorum")
	three := header(validators[3], 1, parents[:3])
	p.OnHeaderReceived(three)
	require.True(t, votedFor(p, three))
	// A parent named twice is refused even with a quorum among the rest:
	// Certificate.Verify refuses the repeat, so the vote would be for a
	// certificate that can never exist.
	repeat := header(validators[1], 1, []types.CertificateDigest{parents[0], parents[1], parents[2], parents[0]})
	p.OnHeaderReceived(repeat)
	require.False(t, votedFor(p, repeat), "a header naming a parent twice must not be voted")

	// At round 2, a weak link to round 0 is allowed and not counted: the
	// round-1 parents alone must carry the quorum.
	var round1 []types.CertificateDigest
	for _, v := range validators[1:] {
		c := types.NewCertificate(header(v, 1, parents), nil, nil)
		require.NoError(t, d.Insert(c))
		round1 = append(round1, c.Digest())
	}
	p.SetRound(2)
	onlyWeak := header(validators[1], 2, parents)
	p.OnHeaderReceived(onlyWeak)
	require.False(t, votedFor(p, onlyWeak), "round-0 parents do not certify a round-2 header")
	weak := header(validators[2], 2, append(parents[:1], round1...))
	p.OnHeaderReceived(weak)
	require.True(t, votedFor(p, weak))
}

// TestHeaderBuilderParentsByStake: the builder needs a stake quorum of the
// previous round's certificates as parents, not a count (#4591): with
// unequal stake a count of 2f+1 validators can be well short of two thirds
// of the stake, and a stake quorum can be fewer validators than that.
func TestHeaderBuilderParentsByStake(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}
	stakes := func(whale int) *types.Committee {
		infos := make([]types.ValidatorInfo, len(validators))
		for i, v := range validators {
			infos[i] = types.ValidatorInfo{PublicKey: v.pub, Stake: 1}
		}
		infos[whale].Stake = 100
		return types.NewCommittee(infos, 1)
	}
	genesisOf := func(committee *types.Committee, d *dag.DAG, from, to int) {
		for _, v := range validators[from:to] {
			h := types.NewHeader(v.pub, 0, 1, nil, nil)
			require.NoError(t, h.Sign(v.priv))
			require.NoError(t, d.InsertGenesis(types.NewCertificate(h, nil, nil)))
		}
	}

	// Three of four validators' certificates carry 3 of 103 stake: a count
	// quorum, not a stake quorum.
	committee := stakes(3)
	d := newTestDAG()
	genesisOf(committee, d, 0, 3)
	p := New(Config{Partition: "test", KeyPair: validators[0].priv}, committee, nil, d, nil)
	p.SetRound(1)
	_, err := p.CreateHeader()
	require.ErrorIs(t, err, ErrNotEnoughParents)

	// One certificate carrying 100 of 103 stake is a quorum on its own.
	committee = stakes(0)
	d = newTestDAG()
	genesisOf(committee, d, 0, 1)
	p = New(Config{Partition: "test", KeyPair: validators[1].priv}, committee, nil, d, nil)
	p.SetRound(1)
	h, err := p.CreateHeader()
	require.NoError(t, err)
	require.Len(t, h.Parents, 1)
}

func votedFor(p *Primary, h *types.Header) bool {
	p.pendingMu.Lock()
	defer p.pendingMu.Unlock()
	return p.sentVotes[h.Digest()] != nil
}
