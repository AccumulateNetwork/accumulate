// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/gossip"
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

// A certificate asked for before a seed is asked for again at once after it.
// A joining node asks for the weak-link parents of the frontier certificates
// it holds; before the seed the answer is held for want of parents in turn,
// and a certificate held does not check, so its digest stays in flight
// (#4597). The seed forgot the held certificate but not the request, and the
// node's next ask for it went unsent for the five-second deduplication
// interval and then until the pending rescan, fifteen seconds after: the
// round two above the seed waited on it, and the libp2p join test pulled past
// its first proven state on every CI run once #4596 made a held certificate
// not check (#4610).
func TestASeedForgetsWhatItAskedForBeforeIt(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()
	p := New(Config{Partition: "test", KeyPair: validators[0].priv}, committee, nil, d, nil)
	p.requestParents = func([]types.CertificateDigest) {}
	config := CertSyncerConfig{PublicKey: validators[0].pub, Partition: "test"}
	config.applyDefaults()
	s := NewCertSyncer(config, d, nil, p.pendingCerts)
	s.SetCertReceivedCallback(func(c *types.Certificate) CertOutcome { return p.receiveCertificate(c, false) })
	p.certSyncer = s

	h := types.NewHeader(validators[1].pub, 38, 1, nil, []types.CertificateDigest{{1, 37}})
	require.NoError(t, h.Sign(validators[1].priv))
	sigs := make([][]byte, len(validators))
	idx := make([]uint16, len(validators))
	for i, v := range validators {
		vote := types.NewVote(h.Digest(), h.Round, h.Epoch, v.pub)
		require.NoError(t, vote.Sign(v.priv))
		sigs[i], idx[i] = vote.Signature, uint16(i)
	}
	parent := types.NewCertificate(h, sigs, idx)

	// Before the seed: asked for, answered, and held.
	s.RequestMissing([]types.CertificateDigest{parent.Digest()})
	s.sendBatchRequest()
	s.handleSyncResponse(&gossip.CertSyncResponse{Certificates: []*types.Certificate{parent}})
	require.Equal(t, 1, p.PendingCertsCount())

	// The seed, as Node.Rejoin makes it.
	p.SetRound(40)
	d.SetLastCommitRound(40)
	p.ForgetHeld()

	// Asked for again, it is sent with the next batch.
	s.RequestMissing([]types.CertificateDigest{parent.Digest()})
	s.batchQueueMu.Lock()
	queued := append([]types.CertificateDigest(nil), s.batchQueue...)
	s.batchQueueMu.Unlock()
	require.Contains(t, queued, parent.Digest(), "a certificate asked for before the seed is not asked for again after it")
}
