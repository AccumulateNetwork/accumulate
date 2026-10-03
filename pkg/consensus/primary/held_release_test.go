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

// A peer's certificate held waiting for this node's own certificate is
// released when this node forms and inserts it (#4515). The own certificate
// is inserted on its own goroutine, which released nothing; its gossip copies
// come back as duplicates, which released nothing either. A held certificate
// is never retried on re-delivery, so the miss was permanent: consim
// TestRestartOneDirectoryValidatorOfTwelve wedged with 240 held, one of them
// waiting only on parents already in the DAG and the rest waiting on it.
func TestOwnCertificateReleasesTheCertificatesHeldForIt(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()
	p := New(Config{Partition: "test", KeyPair: validators[0].priv}, committee, nil, d, nil)

	var genesis []*types.Certificate
	for i := range validators {
		c := buildCert(t, validators, i, 0, nil)
		require.NoError(t, d.InsertGenesis(c))
		genesis = append(genesis, c)
	}
	var round1 []*types.Certificate
	for i := range validators {
		c := buildCert(t, validators, i, 1, genesis)
		require.NoError(t, d.Insert(c))
		round1 = append(round1, c)
	}

	// This node's own header at round 2, and the certificate its votes form.
	own := buildCert(t, validators, 0, 2, round1)
	digest := own.Header.Digest()

	// A peer's round-3 certificate naming it arrives first, and is held.
	h := types.NewHeader(validators[1].pub, 3, 1, nil, []types.CertificateDigest{own.Digest()})
	require.NoError(t, h.Sign(validators[1].priv))
	sigs := make([][]byte, len(validators))
	idx := make([]uint16, len(validators))
	for i, v := range validators {
		vote := types.NewVote(h.Digest(), 3, 1, v.pub)
		require.NoError(t, vote.Sign(v.priv))
		sigs[i], idx[i] = vote.Signature, uint16(i)
	}
	peer := types.NewCertificate(h, sigs, idx)
	require.Equal(t, CertBuffered, p.receiveCertificate(peer))

	// The votes arrive and this node forms and inserts its certificate.
	p.pendingMu.Lock()
	p.ourHeaders[digest] = own.Header
	for _, v := range validators {
		vote := types.NewVote(digest, 2, 1, v.pub)
		require.NoError(t, vote.Sign(v.priv))
		p.pendingVotes[digest] = append(p.pendingVotes[digest], vote)
	}
	p.tryCreateCertificateLocked(digest)
	p.pendingMu.Unlock()
	p.wg.Wait()

	require.True(t, d.Contains(types.CertificateDigest(digest)), "precondition: the own certificate is in the DAG")
	require.False(t, p.pendingCerts.Contains(peer.Digest()), "the peer certificate is still held, waiting for a parent already in the DAG")
	require.True(t, d.Contains(peer.Digest()), "and it is inserted")
}
