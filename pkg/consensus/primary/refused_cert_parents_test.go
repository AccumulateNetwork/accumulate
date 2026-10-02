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

	// A refused certificate far ahead is left to round catch-up: asking for
	// the parents of every frontier certificate would pull the frontier by
	// digest alongside gossip.
	asked = nil
	require.Equal(t, CertDropped, p.receiveCertificate(certWithParent(3, 60, types.CertificateDigest{8})))
	require.Empty(t, asked)
}

// #4469: a digest request is never larger than a response can answer. A peer
// answers at most MaxSyncCertificates certificates and leaves the rest
// unanswered until the 30 s retry, so a joining node asking for several
// hundred parents at once waited on the ones it needed next.
func TestDigestRequestsFitOneResponse(t *testing.T) {
	digests := make([]types.CertificateDigest, 2*gossip.MaxSyncCertificates+7)
	for i := range digests {
		digests[i][0], digests[i][1] = byte(i), byte(i>>8)
	}
	chunks := digestChunks(digests)
	require.Len(t, chunks, 3)
	var all []types.CertificateDigest
	for _, c := range chunks {
		require.LessOrEqual(t, len(c), gossip.MaxSyncCertificates)
		all = append(all, c...)
	}
	require.Equal(t, digests, all, "every digest is asked for, in order")
	require.Empty(t, digestChunks(nil))
}

// #4469: a certificate the full pending buffer will refuse costs no
// verification, however many times it arrives. Refused certificates never
// enter the DAG, so the DAG dedup never sees their copies; each copy from
// gossip and from every peer answering a sync request was verified again
// (an ed25519 check per signature). A joining node, its buffer full of
// frontier certificates, spent its time on that while its sync responses
// queued and were dropped behind it, including the ones carrying the
// parents it needed: consim TestFollowerJoinCatchesUp under -race, 0/10.
func TestRefusedCertificatesCostNoVerification(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}
	committee := newTestCommittee(validators, 1)
	p := New(Config{Partition: "t4469-verify", KeyPair: validators[0].priv}, committee, nil, newTestDAG(), nil)
	p.SetRound(1)
	var asked []types.CertificateDigest
	p.requestParents = func(ds []types.CertificateDigest) { asked = append(asked, ds...) }

	cert := func(author int, round types.Round, parent types.CertificateDigest) *types.Certificate {
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

	// Fill the one-slot buffer with a certificate far ahead.
	p.pendingCerts = NewPendingCertificates(1)
	held := cert(1, 50, types.CertificateDigest{7})
	require.Equal(t, CertBuffered, p.receiveCertificate(held))
	require.Equal(t, uint64(1), p.verifications.Load(), "the first delivery is verified")
	asked = nil

	const n = 20
	refused := cert(2, 60, types.CertificateDigest{8})
	for range n {
		require.Equal(t, CertBuffered, p.receiveCertificate(held), "already held")
		require.Equal(t, CertDropped, p.receiveCertificate(refused), "the buffer is full and it is ahead of the next round")
	}
	require.Equal(t, uint64(1), p.verifications.Load(),
		"%d re-deliveries of a held certificate and %d of a refused one were verified", n, n)
	require.Empty(t, asked, "a refused certificate far ahead is left to round catch-up")

	// A certificate the node needs next still takes the full path: verified,
	// refused by the full buffer, and its parents asked for.
	absent := types.CertificateDigest{9, 9, 9}
	require.Equal(t, CertDropped, p.receiveCertificate(cert(3, 2, absent)))
	require.Equal(t, uint64(2), p.verifications.Load())
	require.Equal(t, []types.CertificateDigest{absent}, asked)

	// A forged certificate the node needs is still caught by verification.
	forged := cert(1, 2, absent)
	forged.Signatures[0] = make([]byte, len(forged.Signatures[0]))
	require.Equal(t, CertInvalid, p.receiveCertificate(forged))
	require.Equal(t, uint64(3), p.verifications.Load())
}
