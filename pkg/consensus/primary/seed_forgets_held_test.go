// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// A certificate held before a seed is taken after it. A joining node's DAG
// has a floor of zero until it is seeded, so every certificate gossip brings
// it meanwhile is held for want of parents. The seed makes the ones at its
// round and the round after it acceptable without parents -- and while they
// stayed held, every later copy was refused as already held, the node's DAG
// stayed empty, and it ordered nothing: three runs in four of the libp2p join
// test once the join seeded below its state instead of at it (#4530).
func TestASeedForgetsWhatWasHeldBeforeIt(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()
	p := New(Config{Partition: "test", KeyPair: validators[0].priv}, committee, nil, d, nil)
	p.requestParents = func([]types.CertificateDigest) {}

	cert := func(author int, round types.Round) *types.Certificate {
		h := types.NewHeader(validators[author].pub, round, 1, nil, []types.CertificateDigest{{byte(author), byte(round)}})
		require.NoError(t, h.Sign(validators[author].priv))
		sigs := make([][]byte, len(validators))
		idx := make([]uint16, len(validators))
		for i, v := range validators {
			vote := types.NewVote(h.Digest(), round, 1, v.pub)
			require.NoError(t, vote.Sign(v.priv))
			sigs[i], idx[i] = vote.Signature, uint16(i)
		}
		return types.NewCertificate(h, sigs, idx)
	}

	// Before the seed: the frontier's certificates arrive and are held.
	at, after, above := cert(1, 40), cert(2, 41), cert(3, 42)
	for _, c := range []*types.Certificate{at, after, above} {
		require.Equal(t, CertBuffered, p.receiveCertificate(c, false))
	}
	require.Equal(t, 3, p.PendingCertsCount())

	// The seed, as Node.Rejoin makes it.
	p.SetRound(40)
	d.SetLastCommitRound(40)
	require.Equal(t, 3, p.ForgetHeld())
	require.Zero(t, p.PendingCertsCount())

	// Delivered again, the ones at the seed and the round after it are
	// taken; the one above still wants its parents and is held again.
	require.Equal(t, CertInserted, p.receiveCertificate(at, false), "a certificate at the seed is still held from before it")
	require.Equal(t, CertInserted, p.receiveCertificate(after, false))
	require.Equal(t, CertBuffered, p.receiveCertificate(above, false))
	require.True(t, d.Contains(at.Digest()))
	require.True(t, d.Contains(after.Digest()))
}
