// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/gossip"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// A peer holding n round-0 certificates, its sync responses handled by the
// real receiveCertificate, as production wires it: a copy of a certificate
// the DAG holds is a duplicate before anything is verified, anything else
// verifies against the committee of four or is invalid. The first is the
// committee's genesis certificate; the rest are by keys outside the
// committee, held so there can be more than one answer carries.
func newSuppressionPeer(t *testing.T, n int) (*CertSyncer, []*types.Certificate) {
	t.Helper()
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()
	p := New(Config{Partition: "t4597", KeyPair: validators[0].priv}, committee, nil, d, nil)
	h := types.NewHeader(validators[0].pub, 0, committee.Epoch, nil, nil)
	require.NoError(t, h.Sign(validators[0].priv))
	sigs := make([][]byte, len(validators))
	idx := make([]uint16, len(validators))
	for i, v := range validators {
		vote := types.NewVote(h.Digest(), 0, committee.Epoch, v.pub)
		require.NoError(t, vote.Sign(v.priv))
		sigs[i], idx[i] = vote.Signature, uint16(i)
	}
	genesis := types.NewCertificate(h, sigs, idx)
	require.NoError(t, d.InsertGenesis(genesis))
	certs := []*types.Certificate{genesis}
	for len(certs) < n {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		h := types.NewHeader(pub, 0, 0, nil, nil)
		require.NoError(t, h.Sign(priv))
		c := types.NewCertificate(h, [][]byte{h.Signature}, []uint16{0})
		require.NoError(t, d.InsertGenesis(c))
		certs = append(certs, c)
	}

	config := CertSyncerConfig{PublicKey: validators[0].pub, Partition: "t4597"}
	peer := NewCertSyncer(config, d, nil, p.pendingCerts)
	peer.SetCertReceivedCallback(p.receiveCertificate)
	p.certSyncer = peer
	return peer, certs
}

func digestsOf(certs []*types.Certificate) []types.CertificateDigest {
	var ds []types.CertificateDigest
	for _, c := range certs {
		ds = append(ds, c.Digest())
	}
	return ds
}

// One forged response must not silence the honest answers to a request
// (#4597): a response suppresses only the certificates it delivered that
// verified, so junk suppresses nothing and a partial answer suppresses only
// its part.
func TestSuppressionIsPerVerifiedCertificate(t *testing.T) {
	requester := []byte("the requester's key")
	const id = 77

	t.Run("junk suppresses nothing", func(t *testing.T) {
		peer, certs := newSuppressionPeer(t, 2)
		junk := types.NewCertificate(types.NewHeader(make([]byte, 32), 0, 0, nil, nil), nil, nil)
		peer.handleSyncResponse(&gossip.CertSyncResponse{Certificates: []*types.Certificate{junk}, Requester: requester, RequestID: id})
		peer.window.take()

		resp := peer.answer(&gossip.CertSyncRequest{Digests: digestsOf(certs), Requester: requester, RequestID: id})
		require.NotNil(t, resp, "the peer holding the certificates did not answer")
		require.Equal(t, digestsOf(certs), digestsOf(resp.Certificates))
		require.Equal(t, 0, peer.window.take().suppressed)
	})

	t.Run("partial answer suppresses its part", func(t *testing.T) {
		peer, certs := newSuppressionPeer(t, 2)
		peer.handleSyncResponse(&gossip.CertSyncResponse{Certificates: certs[:1], Requester: requester, RequestID: id})

		resp := peer.answer(&gossip.CertSyncRequest{Digests: digestsOf(certs), Requester: requester, RequestID: id})
		require.NotNil(t, resp, "the certificate the response did not carry was not answered")
		require.Equal(t, digestsOf(certs[1:]), digestsOf(resp.Certificates))
	})

	t.Run("complete answer suppresses the request", func(t *testing.T) {
		peer, certs := newSuppressionPeer(t, 2)
		peer.handleSyncResponse(&gossip.CertSyncResponse{Certificates: certs, Requester: requester, RequestID: id})
		peer.window.take()

		require.Nil(t, peer.answer(&gossip.CertSyncRequest{Digests: digestsOf(certs), Requester: requester, RequestID: id}))
		require.Equal(t, 1, peer.window.take().suppressed)
	})

	t.Run("round request answers what was not delivered", func(t *testing.T) {
		peer, certs := newSuppressionPeer(t, 2)
		peer.handleSyncResponse(&gossip.CertSyncResponse{Certificates: certs[1:], Requester: requester, RequestID: id})

		resp := peer.answer(&gossip.CertSyncRequest{Rounds: []types.Round{0}, Requester: requester, RequestID: id})
		require.NotNil(t, resp)
		require.Equal(t, digestsOf(certs[:1]), digestsOf(resp.Certificates))
	})
}

// #4597, through the real receive path: a certificate's digest covers only
// its header, and receiveCertificate reports a copy of a certificate the DAG
// holds as a duplicate before verifying it. A forged response carrying a
// genuine header with no signatures must therefore not count as having
// delivered the certificate, or one message silences every honest answer.
// A byte-identical copy of the certificate does count.
func TestDefect_ForgedSyncResponseSuppressesHonestAnswer(t *testing.T) {
	requester := []byte("the requester's key")
	const id = 4597
	peer, certs := newSuppressionPeer(t, 1)
	genuine := certs[0]
	request := &gossip.CertSyncRequest{Digests: digestsOf(certs), Requester: requester, RequestID: id}

	forged := types.NewCertificate(genuine.Header, nil, nil)
	peer.handleSyncResponse(&gossip.CertSyncResponse{Certificates: []*types.Certificate{forged}, Requester: requester, RequestID: id})
	require.NotNil(t, peer.answer(request), "a header with no signatures suppressed the honest answer")

	copied := types.NewCertificate(genuine.Header, append([][]byte{}, genuine.Signatures...), append([]uint16{}, genuine.SignedAuthorities...))
	peer.handleSyncResponse(&gossip.CertSyncResponse{Certificates: []*types.Certificate{copied}, Requester: requester, RequestID: id})
	require.Nil(t, peer.answer(request), "an identical copy of the certificate did not suppress the answer")
}

// #4597: responses are handled on one goroutine and requests answered on
// others, so what a response delivered is read while the next response to
// the same request is recorded. A concurrent map read and write kills the
// process.
func TestSuppressionMapIsRaceFree(t *testing.T) {
	requester := []byte("the requester's key")
	const id = 11
	peer, certs := newSuppressionPeer(t, 8)
	const rounds = 2000

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < rounds; i++ {
			c := certs[i%len(certs)]
			peer.handleSyncResponse(&gossip.CertSyncResponse{Certificates: []*types.Certificate{c}, Requester: requester, RequestID: id})
		}
	}()
	for i := 0; i < rounds; i++ {
		peer.answer(&gossip.CertSyncRequest{Rounds: []types.Round{0}, Requester: requester, RequestID: id})
	}
	<-done
}

// #4597: what a response delivered is taken out before the answer is cut to
// what one response carries, or the certificates past the cap are never
// served. The peer holds one certificate more than a response carries, and
// a response delivered all but one of them.
func TestFilteringPrecedesTheCap(t *testing.T) {
	requester := []byte("the requester's key")
	const id = 101
	peer, certs := newSuppressionPeer(t, gossip.MaxSyncCertificates+1)
	withheld, delivered := certs[len(certs)-1], certs[:len(certs)-1]
	peer.handleSyncResponse(&gossip.CertSyncResponse{Certificates: delivered, Requester: requester, RequestID: id})

	// By digest, the withheld certificate named last: the digest loop
	// used to stop at the cap before reaching it.
	resp := peer.answer(&gossip.CertSyncRequest{Digests: digestsOf(certs), Requester: requester, RequestID: id})
	require.NotNil(t, resp, "the certificate no response delivered was not answered")
	require.Equal(t, digestsOf([]*types.Certificate{withheld}), digestsOf(resp.Certificates))

	// By round, the same.
	resp = peer.answer(&gossip.CertSyncRequest{Rounds: []types.Round{0}, Requester: requester, RequestID: id})
	require.NotNil(t, resp, "the certificate no response delivered was not answered")
	require.Equal(t, digestsOf([]*types.Certificate{withheld}), digestsOf(resp.Certificates))
}

// #4597: what is remembered per request is bounded by what one response
// carries, so one requester and ID cannot be filled with the whole DAG.
// Delivering one certificate more than that under one key leaves at least
// one of them unsuppressed.
func TestDeliveredDigestsAreCappedPerRequest(t *testing.T) {
	requester := []byte("the requester's key")
	const id = 5
	peer, certs := newSuppressionPeer(t, gossip.MaxSyncCertificates+1)
	peer.handleSyncResponse(&gossip.CertSyncResponse{Certificates: certs, Requester: requester, RequestID: id})

	resp := peer.answer(&gossip.CertSyncRequest{Rounds: []types.Round{0}, Requester: requester, RequestID: id})
	require.NotNil(t, resp, "one request remembered more than a response carries")
	require.Len(t, resp.Certificates, 1)
}

// #4597: suppression ages out a generation at a time, never all at once.
// After one request more than a generation holds, the oldest request and
// the ones recorded just before the rotation are all still suppressed.
func TestSuppressionIsNeverWipedAllAtOnce(t *testing.T) {
	peer, certs := newSuppressionPeer(t, 1)
	requester := []byte("the requester's key")
	deliver := func(id uint64) {
		peer.handleSyncResponse(&gossip.CertSyncResponse{Certificates: certs, Requester: requester, RequestID: id})
	}
	suppressed := func(id uint64) bool {
		return peer.answer(&gossip.CertSyncRequest{Digests: digestsOf(certs), Requester: requester, RequestID: id}) == nil
	}

	const generation = 4096
	for id := uint64(0); id <= generation+1; id++ {
		deliver(id)
	}
	for _, id := range []uint64{0, 1, generation, generation + 1} {
		require.True(t, suppressed(id), "request %d was forgotten when the generation rotated", id)
	}
}
