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

// #4469: a certificate the node needs to advance, whose parents are missing,
// must have those parents asked for even when the pending buffer is full and
// will not hold it.
//
// A node seeded by Rejoin at round P accepts certificates at P and P+1
// without their parents, but a certificate at P+2 names, as weak links, the
// certificates of rounds P-6..P, which no round request asks for (round
// catch-up starts one round below the node's own). While the buffer has room
// the certificate is held and its parents requested; once it is full --
// and a joining node fills it with the frontier certificates gossip keeps
// delivering, which are never pruned because they are above its round --
// the certificate is dropped without the ask. Every catch-up response
// re-delivers it and it is dropped again, for ever: a follower's Directory
// sat at P+2 for four minutes (Docker run 20260929T152146Z) and consim
// reproduces it (TestFollowerJoinCatchesUp).
func TestRefusedCertificateStillRequestsItsParents(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()
	p := New(Config{Partition: "test", KeyPair: validators[0].priv}, committee, nil, d, nil)
	p.SetRound(1)

	var asked []types.CertificateDigest
	p.requestParents = func(ds []types.CertificateDigest) { asked = append(asked, ds...) }

	certWithParent := func(author int, round types.Round, parent types.CertificateDigest) *types.Certificate {
		h := types.NewHeader(validators[author].pub, round, 1, nil, []types.CertificateDigest{parent})
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

	// Fill the buffer: one slot, taken by a certificate far ahead.
	p.pendingCerts = NewPendingCertificates(1)
	require.Equal(t, CertBuffered, p.receiveCertificate(certWithParent(1, 50, types.CertificateDigest{7})))
	asked = nil

	// The certificate this node needs next, naming a parent it lacks.
	absent := types.CertificateDigest{9, 9, 9}
	next := certWithParent(2, 1, absent)
	require.Equal(t, CertDropped, p.receiveCertificate(next), "the buffer is full, so it is not held")
	require.Equal(t, []types.CertificateDigest{absent}, asked,
		"a refused certificate's missing parents must still be requested, or it is refused on every re-delivery")
}
